// Package ingest moves spooled events into the database. Only the process holding the ingest
// lease may call Drain (ADR-0003).
package ingest

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"sessionkeep/internal/capture"
	"sessionkeep/internal/config"
	"sessionkeep/internal/fsx"
	"sessionkeep/internal/project"
	"sessionkeep/internal/store"
)

// Report says what one drain did. Counts are of spool files.
type Report struct {
	Ingested         int `json:"ingested"`
	Replayed         int `json:"replayed"` // already stored by an earlier, interrupted drain
	Quarantined      int `json:"quarantined"`
	DiscardedExclude int `json:"discarded_excluded"`
	ProjectsCreated  int `json:"projects_created"`
	GapsImported     int `json:"gaps_imported"`
	// Deferred counts files left for the next drain because another program (typically antivirus
	// scanning a file that was just written) held them open. Nothing is lost.
	Deferred int `json:"deferred"`
	// Recovered counts complete events found in temp files left by a hook that stopped before
	// its final rename (for example when it reached its time limit on a stalled disk).
	Recovered int `json:"recovered"`
	Remaining int `json:"remaining"`
}

// orphanAfter is how old a spool temp file must be before its writer is certainly gone: several
// times the hook's own time limit.
const orphanAfter = 10 * time.Second // five times the hook deadline

// Deps are the things Drain needs. AfterCommit, if set, runs after each event's transaction has
// committed and before its spool file is deleted; tests use it to simulate a crash at that point.
type Deps struct {
	Paths       config.Paths
	Settings    config.Settings
	Store       *store.Store
	GOOS        string
	Now         func() time.Time
	NewID       func() string
	AfterCommit func(file string) error
}

// QuarantineDir is where unusable spool files are moved, inside the data directory.
const QuarantineDir = "quarantine"

// textField names, per event kind, the payload field that holds the permitted text, with the
// evidence origin and whose words they are.
var textField = map[string]struct{ key, origin, trust string }{
	"user_prompt":    {"prompt", "hook:UserPromptSubmit", "user"},
	"assistant_turn": {"text", "hook:Stop", "assistant"},
	"compaction":     {"compact_summary", "hook:PostCompact", "assistant"},
	"tool_failure":   {"error", "hook:PostToolUseFailure", "tool"},
	"tool_use":       {"tool_response", "hook:PostToolUse", "tool"},
}

// Drain ingests every complete spool file in order, imports the gap log, removes stale temp files
// and rewrites the roots cache. It stops at the first storage error, leaving the remaining files
// for the next drain; nothing already committed is lost or repeated.
func Drain(ctx context.Context, d Deps) (Report, error) {
	var rep Report
	entries, err := os.ReadDir(d.Paths.SpoolDir)
	if err != nil {
		return rep, fmt.Errorf("read spool: %w", err)
	}
	var files []string
	for _, e := range entries {
		switch {
		case e.IsDir():
		case strings.HasSuffix(e.Name(), ".evt"):
			files = append(files, e.Name())
		case strings.HasSuffix(e.Name(), ".evt.tmp") && strings.HasPrefix(e.Name(), "."):
			// A hook writes the whole event to a temp file and then renames it. If it stops in
			// between, the temp file stays. Once it is older than the hook can possibly still be
			// running, a complete event in it is recovered; an incomplete one is kept for an hour
			// (in case that judgement is wrong) and then discarded with a recorded gap.
			fi, err := e.Info()
			if err != nil {
				continue
			}
			age := d.Now().Sub(fi.ModTime())
			if age < orphanAfter {
				continue
			}
			tmp := filepath.Join(d.Paths.SpoolDir, e.Name())
			final := strings.TrimSuffix(strings.TrimPrefix(e.Name(), "."), ".tmp")
			raw, err := fsx.ReadFile(tmp)
			var ev capture.Event
			if err == nil && json.Unmarshal(raw, &ev) == nil && ev.Version == 1 && ev.EventID != "" {
				if os.Rename(tmp, filepath.Join(d.Paths.SpoolDir, final)) == nil {
					files = append(files, final)
					rep.Recovered++
				}
				continue
			}
			if age > time.Hour {
				if d.Store.AddGap(ctx, "partial:"+e.Name(), "spool", "partial_event_discarded", d.Now().UTC().Format(capture.TimeFormat), false) == nil {
					os.Remove(tmp)
				}
			}
		}
	}
	sort.Strings(files) // names start with a zero-padded timestamp

	for i, name := range files {
		if err := ctx.Err(); err != nil {
			rep.Remaining = len(files) - i
			return rep, err
		}
		path := filepath.Join(d.Paths.SpoolDir, name)
		if err := d.one(ctx, path, name, &rep); err != nil {
			rep.Remaining = len(files) - i
			return rep, err
		}
	}

	n, err := d.importGaps(ctx)
	rep.GapsImported = n
	if err != nil {
		return rep, err
	}
	return rep, d.RewriteRootsCache(ctx)
}

func (d Deps) one(ctx context.Context, path, name string, rep *Report) error {
	// fsx.ReadFile retries briefly: on Windows a file that was just created is often held open for
	// a moment by a scanner. If it still cannot be read, leave it for the next drain rather than
	// failing the whole run.
	raw, err := fsx.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		rep.Deferred++
		return nil
	}
	var ev capture.Event
	if json.Unmarshal(raw, &ev) != nil || ev.Version != 1 || ev.EventID == "" || ev.SourceKey == "" || ev.SessionID == "" ||
		ev.Kind == "" || ev.DeviceID == "" || !config.ValidLevel(ev.Level) || (ev.ProjectID == "") == (ev.Register == nil) {
		return d.quarantine(ctx, path, name, "invalid_event", rep)
	}
	// The user may have excluded the folder after the hook wrote this file.
	if project.Excluded(d.GOOS, d.Settings.Exclusions, ev.Cwd) {
		rep.DiscardedExclude++
		return os.Remove(path)
	}

	in := store.IngestInput{
		EventID: ev.EventID, EventKey: ev.SourceKey, DeviceID: ev.DeviceID, ProjectID: ev.ProjectID,
		Agent: ev.Agent, SessionExternal: ev.SessionID, NewSessionID: d.NewID(), AdapterVersion: ev.AdapterVersion,
		Kind: ev.Kind, ObservedAt: ev.ObservedAt, ReceivedAt: d.Now().UTC().Format(capture.TimeFormat),
		RedactionStatus: "none", EndsSession: ev.Kind == "session_end",
	}
	if ev.Redaction.Count > 0 {
		in.RedactionStatus = "redacted"
	}
	if r := ev.Register; r != nil {
		if !config.ValidLevel(r.Level) || r.Level == config.LevelOff || r.Root == "" {
			return d.quarantine(ctx, path, name, "invalid_event", rep)
		}
		in.Auto = &store.AutoProject{ProjectID: d.NewID(), RootID: d.NewID(), Name: lastSegment(r.Root), Root: r.Root, Level: r.Level}
	}
	payload := ev.Payload
	if payload == nil {
		payload = map[string]any{}
	}
	if tf, ok := textField[ev.Kind]; ok {
		if text, _ := payload[tf.key].(string); text != "" {
			delete(payload, tf.key) // the text lives in evidence, once
			in.Evidence = &store.EvidenceRow{
				ID: d.NewID(), Origin: tf.origin, Locator: ev.Agent + ":" + ev.SessionID + ":" + ev.EventID, Text: text,
				Completeness: ev.Completeness, Trust: tf.trust, RedactionCount: ev.Redaction.Count,
			}
		}
	}
	payload["hook_event"] = ev.HookEvent
	payload["cwd"] = ev.Cwd
	payload["level"] = ev.Level
	if ev.DroppedBytes > 0 {
		payload["dropped_bytes"] = ev.DroppedBytes
	}
	safe, err := json.Marshal(payload)
	if err != nil {
		return d.quarantine(ctx, path, name, "invalid_event", rep)
	}
	in.SafePayload = string(safe)

	res, err := d.Store.IngestEvent(ctx, in)
	if err != nil {
		return fmt.Errorf("ingest %s: %w", name, err)
	}
	switch res.Outcome {
	case store.Orphan:
		return d.quarantine(ctx, path, name, "unknown_project", rep)
	case store.Replayed:
		rep.Replayed++
	default:
		rep.Ingested++
		if res.ProjectCreated {
			rep.ProjectsCreated++
		}
	}
	if d.AfterCommit != nil {
		if err := d.AfterCommit(name); err != nil {
			return err
		}
	}
	// The event is committed. If the file cannot be removed right now, the next drain finds it,
	// sees the event is already stored, and removes it then.
	for i := 0; ; i++ {
		err := os.Remove(path)
		if err == nil || errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if i >= 40 {
			rep.Deferred++
			return nil
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func lastSegment(p string) string {
	p = strings.TrimRight(p, `/\`)
	if i := strings.LastIndexAny(p, `/\`); i >= 0 && i < len(p)-1 {
		return p[i+1:]
	}
	return p
}

// quarantine moves an unusable file aside and records a gap, so one bad file never blocks the rest.
func (d Deps) quarantine(ctx context.Context, path, name, reason string, rep *Report) error {
	dir := filepath.Join(d.Paths.DataDir, QuarantineDir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := d.Store.AddGap(ctx, "quarantine:"+name, "spool", reason, d.Now().UTC().Format(capture.TimeFormat), false); err != nil {
		return err
	}
	rep.Quarantined++
	return os.Rename(path, filepath.Join(dir, name))
}

// importGaps moves the hook-written gap log into the database. Each line gets an id derived from
// its content, so a drain interrupted between commit and delete cannot record a gap twice.
func (d Deps) importGaps(ctx context.Context) (int, error) {
	log := filepath.Join(d.Paths.DataDir, capture.GapLogName)
	pending := log + ".importing"
	if _, err := os.Stat(pending); errors.Is(err, os.ErrNotExist) {
		if err := os.Rename(log, pending); errors.Is(err, os.ErrNotExist) {
			return 0, nil
		} else if err != nil {
			return 0, err
		}
	}
	raw, err := os.ReadFile(pending)
	if err != nil {
		return 0, err
	}
	n := 0
	sc := bufio.NewScanner(bytes.NewReader(raw))
	for sc.Scan() {
		var g struct {
			At     string `json:"at"`
			Event  string `json:"hook_event"`
			Reason string `json:"reason"`
		}
		if json.Unmarshal(sc.Bytes(), &g) != nil || g.Reason == "" {
			continue
		}
		sum := sha256.Sum256(sc.Bytes())
		if err := d.Store.AddGap(ctx, "hook:"+hex.EncodeToString(sum[:12]), "hook:"+g.Event, g.Reason, g.At, false); err != nil {
			return n, err
		}
		n++
	}
	return n, os.Remove(pending)
}

// recallOf is the project's recall mode as the roots cache holds it: empty unless automatic.
func recallOf(p store.Project) string {
	if p.Recall == project.RecallAutomatic {
		return p.Recall
	}
	return ""
}

// RewriteRootsCache refreshes the hook-readable list of registered roots from the database.
func (d Deps) RewriteRootsCache(ctx context.Context) error {
	all, err := d.Store.ListProjects(ctx, d.Settings.DeviceID)
	if err != nil {
		return err
	}
	var roots []project.Root
	for _, p := range all {
		for _, r := range p.Roots {
			roots = append(roots, project.Root{Path: r, ProjectID: p.ID, Level: p.CapturePolicy, Paused: p.Paused, Recall: recallOf(p)})
		}
	}
	return project.WriteRootsCache(d.Paths, roots, d.Now())
}

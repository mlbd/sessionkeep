package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"sessionkeep/internal/buildinfo"
	"sessionkeep/internal/capture"
	"sessionkeep/internal/config"
	"sessionkeep/internal/handoff"
	"sessionkeep/internal/ingest"
	"sessionkeep/internal/instruct"
	"sessionkeep/internal/redact"
	"sessionkeep/internal/store"
)

const handoffUsage = `usage: sessionkeep handoff make FILE [--project ID_OR_PATH] [--with-sessions] [--since DATE] [--from NAME] [--yes]
       sessionkeep handoff receive FILE [--project ID_OR_PATH] [--yes]`

var handoffLeftOut = []string{
	"instructions that apply to all of the sender's projects (only this project's are included)",
	"tool input and output, failed commands, and Claude's private reasoning",
	"summaries written by a model, and researched facts",
	"the project's files and its Git history",
}

// handoffCmd makes or receives a project handoff file. Both show what would happen first and do
// nothing without --yes.
func handoffCmd(inv invocation, rest []string) error {
	if len(rest) != 2 {
		return usageErr("%s", handoffUsage)
	}
	path, err := filepath.Abs(rest[1])
	if err != nil {
		return usageErr("cannot resolve path %q: %v", rest[1], err)
	}
	switch rest[0] {
	case "make":
		if err := inv.only("project", "with-sessions", "since", "from", "yes"); err != nil {
			return err
		}
		return handoffMake(inv, path)
	case "receive":
		if err := inv.only("project", "yes"); err != nil {
			return err
		}
		return handoffReceive(inv, path)
	}
	return usageErr("%s", handoffUsage)
}

func homeOf(inv invocation) string {
	if runtime.GOOS == "windows" {
		return inv.getenv("USERPROFILE")
	}
	return inv.getenv("HOME")
}

// handoffPick says what of a project goes into a handoff file or a team bundle.
type handoffPick struct {
	withSessions bool
	since        string          // only sessions active at or after this time
	skip         map[string]bool // ids of instructions, checkpoints and pieces of text to leave out
	ownOnly      bool            // leave out what this computer itself received from others
}

// newHandoff starts an empty file for a project.
func newHandoff(st state, proj store.Project, from string) handoff.File {
	return handoff.File{Kind: handoff.Kind, Format: handoff.Format, CreatedAt: time.Now().UTC().Format(time.RFC3339), MadeBy: buildinfo.Get().Version,
		From: strings.TrimSpace(from), SourceDevice: st.settings.DeviceID, Project: handoff.Project{ID: proj.ID, Name: proj.Name},
		Instructions: []handoff.Instruction{}, Checkpoints: []handoff.Checkpoint{}, Sessions: []handoff.Session{}, NotIncluded: handoffLeftOut}
}

// gatherHandoff fills f from the store: the project's own instructions, its checkpoints and, if
// picked, the prompts and replies of its sessions. Folders are shortened and the secret rules
// run again on every text. It returns the scrubber (for its count) and how many secrets went.
func gatherHandoff(ctx context.Context, s *store.Store, proj store.Project, home string, f *handoff.File, pick handoffPick) (*handoff.Scrubber, int, error) {
	scrub := handoff.NewScrubber(proj.Roots, home)
	secrets := 0
	clean := func(text string) string {
		out, n := redact.Text(scrub.Text(text))
		secrets += n
		return out
	}
	active, err := s.ActiveInstructions(ctx, proj.ID)
	if err != nil {
		return scrub, 0, err
	}
	for _, a := range active {
		if a.Scope == "project" && !pick.skip[a.ID] && !(pick.ownOnly && strings.HasPrefix(a.ID, receivedInstruction)) {
			f.Instructions = append(f.Instructions, handoff.Instruction{ID: a.ID, Text: clean(a.Text), Recorded: a.Recorded})
		}
	}
	cps, err := s.Checkpoints(ctx, proj.ID, 1000)
	if err != nil {
		return scrub, 0, err
	}
	for i := len(cps) - 1; i >= 0; i-- {
		c := cps[i]
		if pick.skip[c.ID] || (pick.ownOnly && strings.HasPrefix(c.ID, receivedCheckpoint)) {
			continue
		}
		f.Checkpoints = append(f.Checkpoints, handoff.Checkpoint{ID: c.ID, Note: clean(c.Note), CreatedAt: c.CreatedAt, GitRepo: c.GitRepo, Branch: c.Branch, Head: c.Head})
	}
	for offset := 0; pick.withSessions; offset += 200 {
		page, err := s.Sessions(ctx, proj.ID, 200, offset)
		if err != nil {
			return scrub, 0, err
		}
		for _, info := range page {
			if (pick.since != "" && info.LastAt < pick.since) || (pick.ownOnly && strings.HasPrefix(info.External, receivedSession)) {
				continue
			}
			_, turns, err := s.SessionTurns(ctx, info.ID, proj.ID, false)
			if err != nil {
				return scrub, 0, err
			}
			hs := handoff.Session{ID: info.ID, StartedAt: info.StartedAt}
			for _, t := range turns {
				if t.Text == "" || t.ID == "" || t.Kind == "compaction" || (t.Who != "user" && t.Who != "assistant") || pick.skip[t.ID] {
					continue
				}
				hs.Turns = append(hs.Turns, handoff.Turn{ID: t.ID, Who: t.Who, At: t.ObservedAt, Text: clean(t.Text)})
			}
			if len(hs.Turns) > 0 {
				f.Sessions = append(f.Sessions, hs)
			}
		}
		if len(page) < 200 {
			break
		}
	}
	if err := f.Validate(); err != nil {
		return scrub, 0, fmt.Errorf("what was gathered would not be readable by the receiver: %w", err)
	}
	return scrub, secrets, nil
}

func handoffMake(inv invocation, dst string) error {
	apply := inv.flags["yes"] != ""
	withSessions := inv.flags["with-sessions"] != ""
	since, err := when("since", inv.flags["since"])
	if err != nil {
		return err
	}
	if since != "" && !withSessions {
		return usageErr("--since chooses sessions, so it needs --with-sessions")
	}
	if len(inv.flags["from"]) > 100 {
		return usageErr("--from is a short name, at most 100 characters")
	}
	if _, err := os.Stat(dst); err == nil {
		return failErr("exists", "%s already exists; choose another name (a handoff file is never overwritten)", dst)
	}
	st, err := load(inv, true)
	if err != nil {
		return err
	}
	var f handoff.File
	var scrub *handoff.Scrubber
	secrets, size := 0, int64(0)
	err = withStore(st, func(ctx context.Context, s *store.Store) error {
		proj, err := resolveProject(ctx, inv, st, s, inv.flags["project"])
		if err != nil {
			return err
		}
		if _, err := ingest.Drain(ctx, ingest.Deps{Paths: st.paths, Settings: st.settings, Store: s, GOOS: runtime.GOOS, Now: time.Now, NewID: config.NewID}); err != nil {
			return fmt.Errorf("store waiting events before making the file: %w", err)
		}
		f = newHandoff(st, proj, inv.flags["from"])
		if scrub, secrets, err = gatherHandoff(ctx, s, proj, homeOf(inv), &f, handoffPick{withSessions: withSessions, since: since}); err != nil {
			return err
		}
		if !apply {
			return nil
		}
		size, err = handoff.Write(dst, f)
		return err
	})
	if err != nil {
		return err
	}
	c := f.Count()
	var b strings.Builder
	fmt.Fprintf(&b, "Handoff file for project %q:\n  %d instruction(s) of this project, %d checkpoint(s), %d session(s) with %d piece(s) of text\n", f.Project.Name, c.Instructions, c.Checkpoints, c.Sessions, c.Turns)
	if !withSessions {
		b.WriteString("  Sessions are left out. Add --with-sessions to include what was said in them.\n")
	}
	fmt.Fprintf(&b, "  Your project and home folders were shortened in %d place(s); %d more secret(s) were removed.\n", scrub.Count, secrets)
	b.WriteString("Not included: " + strings.Join(handoffLeftOut, "; ") + ".\n")
	b.WriteString("The file is plain text and is not encrypted. Whoever has it can read everything in it; other names or paths you typed in a conversation are not removed.\n")
	out := map[string]any{"written": apply, "file": dst, "project": f.Project, "counts": c, "paths_shortened": scrub.Count, "secrets_removed": secrets, "not_included": handoffLeftOut}
	if !apply {
		b.WriteString("NOTHING WRITTEN YET. Run the same command with --yes to write the file.\n")
		inv.emit(out, b.String())
		return &cmdError{ExitNeedsConsent, "confirmation_required", "nothing was written; add --yes to confirm"}
	}
	out["bytes"] = size
	fmt.Fprintf(&b, "Written to %s (%d bytes). The receiver adds the project folder to SessionKeep and runs: sessionkeep handoff receive FILE\n", dst, size)
	return inv.emit(out, b.String())
}

// What this computer received from others is marked by how its ids begin, so that it is not
// sent on as if it were this computer's own.
const (
	receivedInstruction = "hi"
	receivedCheckpoint  = "ho"
	receivedSession     = "handoff-"
)

func handoffID(prefix string, parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return prefix + hex.EncodeToString(sum[:14])
}

// handoffResult counts what receiving stored and what was already there.
type handoffResult struct {
	Instructions        int `json:"instructions"`
	InstructionsAlready int `json:"instructions_already_there"`
	Checkpoints         int `json:"checkpoints"`
	CheckpointsAlready  int `json:"checkpoints_already_there"`
	Turns               int `json:"pieces_of_text"`
	TurnsAlready        int `json:"pieces_of_text_already_there"`
	Secrets             int `json:"secrets_removed"`
}

func (r *handoffResult) add(o handoffResult) {
	r.Instructions += o.Instructions
	r.InstructionsAlready += o.InstructionsAlready
	r.Checkpoints += o.Checkpoints
	r.CheckpointsAlready += o.CheckpointsAlready
	r.Turns += o.Turns
	r.TurnsAlready += o.TurnsAlready
	r.Secrets += o.Secrets
}

func senderName(f handoff.File) string {
	if f.From == "" {
		return "a teammate"
	}
	return f.From
}

// takesSessions says whether a project may receive conversation text.
func takesSessions(p store.Project) bool {
	return !p.Paused && (p.CapturePolicy == config.LevelConversation || p.CapturePolicy == config.LevelExtended)
}

// applyHandoff stores a checked file into proj. Storing the same content twice adds nothing.
// The caller rewrites the hook-readable caches afterwards.
func applyHandoff(ctx context.Context, s *store.Store, st state, proj store.Project, f handoff.File) (handoffResult, error) {
	var res handoffResult
	from := senderName(f)
	for _, x := range f.Instructions {
		text, n := redact.Text(x.Text)
		res.Secrets += n
		// The id says where it came from, so it is taken once: not again after the receiver
		// changed or retired it, and not sent on as the receiver's own.
		id := handoffID(receivedInstruction, f.SourceDevice, x.ID)
		if _, err := s.GetInstruction(ctx, id); err == nil {
			res.InstructionsAlready++
			continue
		}
		_, err := s.AddInstruction(ctx, store.NewInstruction{ID: id, VersionID: config.NewID(), Scope: "project", ProjectID: proj.ID, Text: text, Author: "handoff", Now: time.Now()})
		switch {
		case errors.Is(err, store.ErrDuplicateInstruction):
			res.InstructionsAlready++
		case err != nil:
			return res, err
		default:
			res.Instructions++
		}
	}
	for _, x := range f.Checkpoints {
		note, n := redact.Text(x.Note)
		res.Secrets += n
		at, _ := time.Parse(time.RFC3339Nano, x.CreatedAt)
		added, err := s.ImportCheckpoint(ctx, store.Checkpoint{ID: handoffID(receivedCheckpoint, f.SourceDevice, x.ID), ProjectID: proj.ID,
			Note:    fmt.Sprintf("[From the handoff file of %s, written %s]\n%s", from, at.UTC().Format("2006-01-02"), note),
			GitRepo: x.GitRepo, Branch: x.Branch, Head: x.Head, CreatedAt: at.UTC().Format(capture.TimeFormat)})
		if err != nil {
			return res, err
		}
		if added {
			res.Checkpoints++
		} else {
			res.CheckpointsAlready++
		}
	}
	if !takesSessions(proj) {
		return res, nil
	}
	payload, _ := json.Marshal(map[string]any{"hook_event": "handoff", "level": proj.CapturePolicy, "imported": true, "handoff_from": from})
	now := time.Now().UTC().Format(capture.TimeFormat)
	for _, hs := range f.Sessions {
		external := handoffID(receivedSession, f.SourceDevice, hs.ID)
		pending := make([]store.IngestInput, 0, len(hs.Turns))
		for i, t := range hs.Turns {
			text, n := redact.Text(t.Text)
			res.Secrets += n
			at, _ := time.Parse(time.RFC3339Nano, t.At)
			kind, status := "user_prompt", "none"
			if t.Who == "assistant" {
				kind = "assistant_turn"
			}
			if n > 0 {
				status = "redacted"
			}
			pending = append(pending, store.IngestInput{
				EventID: config.NewID(), EventKey: "handoff:" + f.SourceDevice + ":" + hs.ID + ":" + t.ID,
				DeviceID: st.settings.DeviceID, ProjectID: proj.ID, Agent: "claude-code", SessionExternal: external,
				NewSessionID: config.NewID(), AdapterVersion: capture.AdapterVersion, Kind: kind,
				ObservedAt: at.UTC().Format(capture.TimeFormat), ReceivedAt: now, SafePayload: string(payload), RedactionStatus: status,
				EndsSession: i == len(hs.Turns)-1,
				Evidence: &store.EvidenceRow{ID: config.NewID(), Origin: "handoff", Locator: "handoff:" + hs.ID + ":" + t.ID,
					Text: text, Completeness: "complete", Trust: t.Who, RedactionCount: n},
			})
		}
		// One transaction per session: the whole session is stored, or none of it.
		results, err := s.IngestEvents(ctx, pending)
		if err != nil {
			return res, fmt.Errorf("store a received session: %w", err)
		}
		for _, r := range results {
			if r.Outcome == store.Inserted {
				res.Turns++
			} else {
				res.TurnsAlready++
			}
		}
	}
	return res, nil
}

func handoffReceive(inv invocation, src string) error {
	apply := inv.flags["yes"] != ""
	f, err := handoff.Read(src)
	if err != nil {
		if errors.Is(err, handoff.ErrInvalid) || errors.Is(err, os.ErrNotExist) {
			return failErr("invalid_handoff", "%v", err)
		}
		return err
	}
	st, err := load(inv, true)
	if err != nil {
		return err
	}
	if f.SourceDevice == st.settings.DeviceID {
		return failErr("own_handoff", "this file was made on this computer; everything in it is already here")
	}
	from := senderName(f)
	c := f.Count()
	var res handoffResult
	var proj store.Project
	err = withStore(st, func(ctx context.Context, s *store.Store) error {
		if proj, err = resolveProject(ctx, inv, st, s, inv.flags["project"]); err != nil {
			return err
		}
		if !apply {
			return nil
		}
		if res, err = applyHandoff(ctx, s, st, proj, f); err != nil {
			return err
		}
		return instruct.RewriteCaches(ctx, s, st.paths, st.settings.DeviceID, time.Now())
	})
	if err != nil {
		return err
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Handoff file made %.10s by %s with sessionkeep %s, for their project %q.\n", f.CreatedAt, from, f.MadeBy, f.Project.Name)
	fmt.Fprintf(&b, "It carries %d instruction(s), %d checkpoint(s), %d session(s) with %d piece(s) of text.\nIt goes into your project %q (%.8s).\n", c.Instructions, c.Checkpoints, c.Sessions, c.Turns, proj.Name, proj.ID)
	if c.Sessions > 0 && !takesSessions(proj) {
		b.WriteString("The sessions are left out: this project is paused or does not save conversation text. Change that with `sessionkeep project capture`, then run this again.\n")
	}
	b.WriteString(instructionsToRead(f.Instructions))
	out := map[string]any{"received": apply, "file": src, "from": from, "made_at": f.CreatedAt, "source_project": f.Project, "project_id": proj.ID,
		"counts": c, "sessions_taken": takesSessions(proj), "instructions": f.Instructions}
	if !apply {
		b.WriteString("NOTHING RECEIVED YET. Run the same command with --yes to take it in.\n")
		inv.emit(out, b.String())
		return &cmdError{ExitNeedsConsent, "confirmation_required", "nothing was received; add --yes to confirm"}
	}
	out["stored"] = res
	b.WriteString(receivedLine(res))
	b.WriteString("Search it with `sessionkeep memory search`, or start with `sessionkeep resume`.\n")
	return inv.emit(out, b.String())
}

// instructionsToRead prints instructions someone else wrote, in full, before they are accepted.
func instructionsToRead(list []handoff.Instruction) string {
	if len(list) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("These instructions would be given to Claude in every session of this project, as if you had recorded them. Read them:\n")
	for _, x := range list {
		fmt.Fprintf(&b, "----\n%s\n", x.Text)
	}
	b.WriteString("----\n")
	return b.String()
}

func receivedLine(res handoffResult) string {
	return fmt.Sprintf("RECEIVED: %d instruction(s) (%d already there), %d checkpoint(s) (%d already there), %d piece(s) of text (%d already there). Secrets removed on the way in: %d.\n",
		res.Instructions, res.InstructionsAlready, res.Checkpoints, res.CheckpointsAlready, res.Turns, res.TurnsAlready, res.Secrets)
}

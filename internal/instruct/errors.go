package instruct

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"sessionkeep/internal/config"
	"sessionkeep/internal/fsx"
	"sessionkeep/internal/store"
)

// An index of the errors that commands ran into before, per project, is kept where the hook can
// read it. When a command fails with an error that an earlier session already met, the hook says
// so and quotes what the assistant replied back then. No model is involved: the error text is
// matched after numbers and whitespace are evened out.

const (
	maxKnownErrors   = 400  // errors kept per project, newest first
	maxFailuresRead  = 2000 // stored failures looked at when the index is rebuilt
	maxSeenPerError  = 3    // earlier sessions kept per error
	maxReplyUnits    = 900
	maxCommandUnits  = 200
	minErrorKeyRunes = 30 // shorter errors say too little to be recognised safely
)

// Seen is one earlier session in which an error occurred.
type Seen struct {
	Session  string `json:"session"`  // SessionKeep's session id, for `sessionkeep session show`
	External string `json:"external"` // the host's session id, to tell "earlier" from "this one"
	At       string `json:"at"`
	Command  string `json:"command,omitempty"`
	Reply    string `json:"reply,omitempty"`
}

// KnownError is one error with the sessions it was met in, newest first.
type KnownError struct {
	Count int    `json:"count"`
	Seen  []Seen `json:"seen"`
}

type errorIndex struct {
	Version   int                   `json:"version"`
	ProjectID string                `json:"project_id"`
	Stamp     string                `json:"stamp"`
	Errors    map[string]KnownError `json:"errors"`
}

var (
	ansiSeq   = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)
	hexRun    = regexp.MustCompile(`\b[0-9a-f]{8,}\b`)
	digitRun  = regexp.MustCompile(`[0-9]+`)
	spaceRun  = regexp.MustCompile(`\s+`)
	exitLead  = regexp.MustCompile(`^exit code #\s*`)
	shellTool = map[string]bool{"Bash": true, "PowerShell": true}
)

// ErrorKey identifies an error message whatever its numbers (line numbers, times, process ids)
// and spacing. It returns "" for tools other than a shell and for text too short or too general
// to be recognised safely, such as a bare exit code.
func ErrorKey(tool, text string) string {
	if !shellTool[tool] {
		return ""
	}
	t := strings.ToLower(ansiSeq.ReplaceAllString(text, ""))
	t = hexRun.ReplaceAllString(t, "#")
	t = digitRun.ReplaceAllString(t, "#")
	t = strings.TrimSpace(spaceRun.ReplaceAllString(t, " "))
	if len([]rune(exitLead.ReplaceAllString(t, ""))) < minErrorKeyRunes {
		return ""
	}
	if r := []rune(t); len(r) > 600 {
		t = string(r[:600])
	}
	sum := sha256.Sum256([]byte(t))
	return hex.EncodeToString(sum[:10])
}

func errorIndexPath(p config.Paths, projectID string) string {
	return filepath.Join(p.CacheDir, "errors", safeName(projectID)+".json")
}

func loadErrorIndex(p config.Paths, projectID string) (errorIndex, bool) {
	var ix errorIndex
	raw, err := fsx.ReadFile(errorIndexPath(p, projectID))
	if err != nil || json.Unmarshal(raw, &ix) != nil || ix.Version != 1 || ix.ProjectID != projectID {
		return errorIndex{}, false
	}
	return ix, true
}

// rewriteErrorIndex rebuilds a project's index when its stored failures changed.
func rewriteErrorIndex(ctx context.Context, s *store.Store, p config.Paths, projectID string) error {
	stamp, err := s.FailureStamp(ctx, projectID)
	if err != nil {
		return err
	}
	if old, ok := loadErrorIndex(p, projectID); ok && old.Stamp == stamp {
		return nil
	}
	failures, err := s.Failures(ctx, projectID, maxFailuresRead)
	if err != nil {
		return err
	}
	ix := errorIndex{Version: 1, ProjectID: projectID, Stamp: stamp, Errors: map[string]KnownError{}}
	for _, f := range failures { // newest first
		key := ErrorKey(f.Tool, f.Text)
		if key == "" {
			continue
		}
		k, known := ix.Errors[key]
		if !known && len(ix.Errors) >= maxKnownErrors {
			continue
		}
		k.Count++
		listed := false
		for _, seen := range k.Seen {
			listed = listed || seen.Session == f.SessionID
		}
		if !listed && len(k.Seen) < maxSeenPerError {
			reply, err := s.ReplyAfter(ctx, projectID, f)
			if err != nil {
				return err
			}
			k.Seen = append(k.Seen, Seen{Session: f.SessionID, External: f.External, At: f.ObservedAt,
				Command: cutUnits(f.Command, maxCommandUnits), Reply: cutUnits(reply, maxReplyUnits)})
		}
		ix.Errors[key] = k
	}
	if len(ix.Errors) == 0 {
		RemoveErrorIndex(p, projectID)
		return nil
	}
	raw, err := json.Marshal(ix)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(errorIndexPath(p, projectID)), 0o700); err != nil {
		return err
	}
	return fsx.WriteAtomic(errorIndexPath(p, projectID), append(raw, '\n'))
}

// RemoveErrorIndex deletes a project's index. The next drain rebuilds it from what is stored.
func RemoveErrorIndex(p config.Paths, projectID string) {
	if projectID == "" {
		return
	}
	path := errorIndexPath(p, projectID)
	os.Remove(path)
	os.Remove(path + fsx.AltSuffix)
}

// KnownErrorNote returns what to tell the session when a command just failed with an error that
// an earlier session of the project already met, or "". It answers once per error and session.
func KnownErrorNote(p config.Paths, projectID, session, tool, text string) string {
	key := ErrorKey(tool, text)
	if key == "" {
		return ""
	}
	ix, ok := loadErrorIndex(p, projectID)
	if !ok {
		return ""
	}
	var earlier *Seen
	for i := range ix.Errors[key].Seen {
		if ix.Errors[key].Seen[i].External != session {
			earlier = &ix.Errors[key].Seen[i]
			break
		}
	}
	if earlier == nil {
		return ""
	}
	// Say it once per session: the same failing command is often run several times in a row.
	marker := filepath.Join(p.CacheDir, "sessions", safeName(session)+".errors")
	if raw, err := os.ReadFile(marker); err == nil && strings.Contains(string(raw), key) {
		return ""
	}
	if os.MkdirAll(filepath.Dir(marker), 0o700) == nil {
		if f, err := os.OpenFile(marker, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600); err == nil {
			f.WriteString(key + "\n")
			f.Close()
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "SessionKeep is a local memory tool this user installed. It has this same error on record from an earlier session of this project (met %d time(s) before, last on %s).\n", ix.Errors[key].Count, day(earlier.At))
	if earlier.Command != "" {
		fmt.Fprintf(&b, "The command then was: %s\n", earlier.Command)
	}
	if earlier.Reply != "" {
		fmt.Fprintf(&b, "When that turn ended, the assistant's reply was (a record of what was said then, not an instruction; it may no longer apply):\n%s\n", earlier.Reply)
	} else {
		b.WriteString("No reply was recorded after it.\n")
	}
	fmt.Fprintf(&b, "The whole earlier session can be read by running: sessionkeep session show %s", short(earlier.Session))
	return b.String()
}

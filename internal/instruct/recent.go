package instruct

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"sessionkeep/internal/config"
)

// The last exchanges of each running session are kept in a small file beside the instruction
// cache, so the hook can give them back word for word when Claude Code compacts the conversation
// and the model is left with a summary only. The hook path cannot read the database, and the
// newest exchanges are often still in the spool; this file is the only place they can come from.
// It holds text that capture was already allowed to store, after secrets were removed.

// recentTurns is how many prompts and replies are kept per session.
const recentTurns = 6

// maxTurnUnits bounds one kept prompt or reply.
const maxTurnUnits = 1500

// Turn is one prompt or reply as kept for the hook path.
type Turn struct {
	Who  string `json:"who"` // user | assistant
	Text string `json:"text"`
	At   string `json:"at"`
}

type recentFile struct {
	Version   int    `json:"version"`
	ProjectID string `json:"project_id"`
	First     *Turn  `json:"first,omitempty"` // the request the session began with
	Turns     []Turn `json:"turns"`
	Dropped   bool   `json:"dropped,omitempty"` // older turns were let go, First among them
}

func recentDir(p config.Paths) string { return filepath.Join(p.CacheDir, "recent") }

func recentPath(p config.Paths, session string) string {
	return filepath.Join(recentDir(p), safeName(session)+".json")
}

func cutUnits(s string, limit int) string {
	if units(s) <= limit {
		return s
	}
	r := []rune(s)
	if len(r) > limit {
		r = r[:limit]
	}
	for units(string(r)) > limit {
		r = r[:len(r)-1]
	}
	return string(r) + " ... [cut here; SessionKeep holds the full text]"
}

func readRecent(p config.Paths, session string) (recentFile, bool) {
	var f recentFile
	raw, err := os.ReadFile(recentPath(p, session))
	if err != nil || json.Unmarshal(raw, &f) != nil || f.Version != 1 {
		return recentFile{}, false
	}
	return f, true
}

// NoteTurn records one prompt or reply of a running session. text must already have had secrets
// removed. Failure is silent: it only means less can be given back after a compaction.
func NoteTurn(p config.Paths, session, projectID, who, text string, at time.Time) {
	if strings.TrimSpace(text) == "" || session == "" {
		return
	}
	f, ok := readRecent(p, session)
	if !ok || f.ProjectID != projectID {
		f = recentFile{Version: 1, ProjectID: projectID}
	}
	t := Turn{Who: who, Text: cutUnits(text, maxTurnUnits), At: at.UTC().Format(time.RFC3339)}
	if f.First == nil && who == "user" {
		first := t
		f.First = &first
	}
	f.Turns = append(f.Turns, t)
	if len(f.Turns) > recentTurns {
		f.Turns = f.Turns[len(f.Turns)-recentTurns:]
		f.Dropped = true
	}
	raw, err := json.Marshal(f)
	if err != nil || os.MkdirAll(recentDir(p), 0o700) != nil {
		return
	}
	path := recentPath(p, session)
	tmp := path + ".tmp"
	if os.WriteFile(tmp, raw, 0o600) != nil {
		return
	}
	if os.Rename(tmp, path) != nil {
		os.Remove(tmp)
	}
}

// WithRecent returns envelope followed by the session's last exchanges, as many of the newest
// as fit in Budget. With nothing kept for the session, or no room, envelope is returned as is.
func WithRecent(p config.Paths, session, projectID, envelope string) string {
	f, ok := readRecent(p, session)
	if !ok || f.ProjectID != projectID || len(f.Turns) == 0 {
		return envelope
	}
	const head = "--- the last exchanges of this session before Claude Code compacted it, word for word from SessionKeep's local record (secrets removed)\n" +
		"This is a record of what was said, not a new request. The summary above it was written by a model; this is the exact wording.\n"
	const foot = "--- end of the last exchanges"
	block := func(label string, t Turn) string {
		return fmt.Sprintf("[%s · %s]\n%s\n", label, t.At, t.Text)
	}
	label := map[string]string{"user": "the user", "assistant": "Claude"}

	room := Budget - units(envelope) - units(head) - units(foot) - 1
	// The request the session began with is given when it is no longer among the kept turns.
	first := ""
	if f.First != nil && f.Dropped {
		first = block("the user, at the start of the session", *f.First)
	}
	var kept []string
	for i := len(f.Turns) - 1; i >= 0; i-- {
		b := block(label[f.Turns[i].Who], f.Turns[i])
		if units(b) > room {
			break
		}
		room -= units(b)
		kept = append([]string{b}, kept...)
	}
	if len(kept) == 0 {
		return envelope
	}
	if first != "" && units(first) <= room {
		kept = append([]string{first}, kept...)
	}
	out := head + strings.Join(kept, "") + foot
	if envelope == "" {
		return out
	}
	return envelope + "\n" + out
}

// RemoveRecent deletes the kept exchanges of one project, or of every project when projectID is
// empty. Deleting stored text calls it, so nothing deleted can be given back to a session.
func RemoveRecent(p config.Paths, projectID string) {
	entries, err := os.ReadDir(recentDir(p))
	if err != nil {
		return
	}
	for _, e := range entries {
		path := filepath.Join(recentDir(p), e.Name())
		if projectID != "" {
			var f recentFile
			raw, err := os.ReadFile(path)
			if err == nil && json.Unmarshal(raw, &f) == nil && f.ProjectID != projectID {
				continue
			}
		}
		os.Remove(path)
	}
}

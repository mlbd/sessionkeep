// Package instruct turns recorded instructions into what a session receives: a per-project cache
// file written by the lease holder, and the bounded text envelope the hook returns to the host.
//
// The envelope never exceeds the host's limit, always says how many instructions exist and how
// many are shown in full, and names the ones that did not fit (ADR-0002): a session may get less
// than everything, but never without being told.
package instruct

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf16"

	"sessionkeep/internal/config"
	"sessionkeep/internal/fsx"
	"sessionkeep/internal/store"
)

// Budget is the largest envelope SessionKeep emits, in UTF-16 code units. Claude Code replaces any
// hook context longer than 10,000 characters with a file path and a short preview, without asking
// the model to read the file; staying under the limit keeps that from ever happening.
const Budget = 9000

// Item is one active instruction as cached for the hook path.
type Item struct {
	ID       string `json:"id"`
	Version  string `json:"version_id"`
	Revision int    `json:"revision"`
	Scope    string `json:"scope"`
	Text     string `json:"text"`
	Digest   string `json:"digest"`
	Recorded string `json:"recorded_at"`
}

// Checkpoint is the project's most recent note of where work stood.
type Checkpoint struct {
	ID        string `json:"id"`
	Note      string `json:"note"`
	CreatedAt string `json:"created_at"`
	GitRepo   bool   `json:"git_repo"`
	Branch    string `json:"branch,omitempty"`
	Head      string `json:"head,omitempty"`
	Changed   int    `json:"changed_files"`
}

// Cache is the content of one cache file: everything that applies in one project (or, for the
// global file, in a project that has no file of its own yet).
type Cache struct {
	Version    int         `json:"version"`
	WrittenAt  string      `json:"written_at"`
	ProjectID  string      `json:"project_id,omitempty"`
	Items      []Item      `json:"instructions"`
	Checkpoint *Checkpoint `json:"checkpoint,omitempty"`
	Research   *Research   `json:"research,omitempty"`
}

// Claim is one researched fact that has a checked source, as cached for the hook path.
type Claim struct {
	ID      string `json:"id"`
	Subject string `json:"subject"`
	State   string `json:"state"` // verified | disputed
	Expires string `json:"expires_at"`
}

// Research lists the project's sourced claims, disputed first and then by re-check date, so the
// hook can tell which are past that date at the moment a session starts. More counts the claims
// beyond MaxCachedClaims.
type Research struct {
	Claims []Claim `json:"claims"`
	More   int     `json:"more,omitempty"`
}

// MaxCachedClaims bounds the claims kept in a cache file.
const MaxCachedClaims = 200

// stamp identifies everything a cache file carries, to tell whether it needs rewriting.
func stamp(items []Item, cp *Checkpoint, rs *Research) string {
	s := SetDigest(items)
	if cp != nil {
		s += ":" + cp.ID
	}
	if rs != nil {
		h := sha256.New()
		for _, c := range rs.Claims {
			h.Write([]byte(c.ID + " " + c.State + " " + c.Expires + "\n"))
		}
		s += fmt.Sprintf(":%x:%d", h.Sum(nil)[:8], rs.More)
	}
	return s
}

const globalName = "_global"

func dir(p config.Paths) string { return filepath.Join(p.CacheDir, "instructions") }

func cachePath(p config.Paths, projectID string) string {
	if projectID == "" {
		projectID = globalName
	}
	return filepath.Join(dir(p), safeName(projectID)+".json")
}

// safeName keeps ids usable as file names; anything unusual is replaced by its hash.
func safeName(s string) string {
	ok := s != "" && len(s) <= 80
	for _, r := range s {
		ok = ok && (r == '-' || r == '_' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z'))
	}
	if ok {
		return s
	}
	sum := sha256.Sum256([]byte(s))
	return "h" + hex.EncodeToString(sum[:16])
}

// RewriteCaches writes the global cache file and one file per project. Only the lease holder
// calls it. Files whose content is unchanged are left alone.
func RewriteCaches(ctx context.Context, s *store.Store, p config.Paths, deviceID string, now time.Time) error {
	if err := os.MkdirAll(dir(p), 0o700); err != nil {
		return err
	}
	projects, err := s.ListProjects(ctx, deviceID)
	if err != nil {
		return err
	}
	ids := []string{""}
	for _, pr := range projects {
		ids = append(ids, pr.ID)
	}
	for _, id := range ids {
		active, err := s.ActiveInstructions(ctx, id)
		if err != nil {
			return err
		}
		items := make([]Item, 0, len(active))
		for _, a := range active {
			items = append(items, Item{ID: a.ID, Version: a.VersionID, Revision: a.Revision, Scope: a.Scope, Text: a.Text, Digest: a.Digest, Recorded: a.Recorded})
		}
		var cp *Checkpoint
		if id != "" {
			latest, err := s.Checkpoints(ctx, id, 1)
			if err != nil {
				return err
			}
			if len(latest) == 1 {
				c := latest[0]
				cp = &Checkpoint{ID: c.ID, Note: c.Note, CreatedAt: c.CreatedAt, GitRepo: c.GitRepo, Branch: c.Branch, Head: c.Head, Changed: c.ChangedFiles}
			}
		}
		var rs *Research
		if id != "" {
			due, more, err := s.ClaimsDue(ctx, id, MaxCachedClaims)
			if err != nil {
				return err
			}
			if len(due) > 0 {
				rs = &Research{More: more}
				for _, c := range due {
					rs.Claims = append(rs.Claims, Claim{ID: c.ID, Subject: c.Subject, State: c.State, Expires: c.ExpiresAt})
				}
			}
		}
		if old, err := Load(p, id, false); err == nil && old != nil && stamp(old.Items, old.Checkpoint, old.Research) == stamp(items, cp, rs) {
			continue
		}
		raw, err := json.MarshalIndent(Cache{Version: 1, WrittenAt: now.UTC().Format(time.RFC3339), ProjectID: id, Items: items, Checkpoint: cp, Research: rs}, "", " ")
		if err != nil {
			return err
		}
		if err := fsx.WriteAtomic(cachePath(p, id), append(raw, '\n')); err != nil {
			return err
		}
	}
	return nil
}

// Load reads the cache for a project. With fallback, a project without its own file gets the
// global file (a project auto-added moments ago has no file yet). A missing file returns nil.
// A damaged file is an error: the caller must say that instructions are unavailable rather than
// behave as if there were none.
func Load(p config.Paths, projectID string, fallback bool) (*Cache, error) {
	raw, err := fsx.ReadFile(cachePath(p, projectID))
	if errors.Is(err, os.ErrNotExist) && fallback && projectID != "" {
		raw, err = fsx.ReadFile(cachePath(p, ""))
	}
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var c Cache
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("instruction cache is not valid JSON: %w", err)
	}
	if c.Version != 1 {
		return nil, fmt.Errorf("instruction cache version %d is not supported", c.Version)
	}
	return &c, nil
}

// SetDigest identifies an exact set of instruction versions, in order.
func SetDigest(items []Item) string {
	h := sha256.New()
	for _, it := range items {
		h.Write([]byte(it.Version + "\n"))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func units(s string) int { return len(utf16.Encode([]rune(s))) }

func short(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// Coverage says how much of the instruction set an envelope carries in full.
type Coverage struct {
	Total, Shown, Omitted int
	Unlisted              int // omitted instructions that could not even be listed by id
}

// Envelope builds the text for the host. It is never longer than Budget. Wording is factual —
// a record of what the user asked to be remembered — because text dressed up as system commands
// can be treated by the model as an injection attempt.
//
// With a non-zero now, a note about researched facts that are past their re-check date or no
// longer match their source is added (a session start passes the time; a later prompt does not).
func Envelope(c *Cache, now time.Time) (string, Coverage) {
	cov := Coverage{Total: len(c.Items)}
	cp := checkpointSection(c.Checkpoint)
	if rs := researchSection(c.Research, now); rs != "" {
		if cp != "" {
			cp += "\n"
		}
		cp += rs
	}
	if cov.Total == 0 {
		if cp == "" {
			return "", cov
		}
		return "SessionKeep is a local memory tool this user installed. No standing instructions are recorded for this project.\n" + cp, cov
	}
	// The checkpoint is short and is kept whole; the instructions get the rest of the budget.
	Budget := Budget - units(cp)
	const footer = "--- end of recorded instructions"
	const tailRoom = 90 // room kept for the "…and N more" line

	block := func(it Item) string {
		return fmt.Sprintf("--- instruction %s · %s · revision %d · recorded %s · sha256 %.12s\n%s\n", short(it.ID), it.Scope, it.Revision, day(it.Recorded), it.Digest, it.Text)
	}
	omitLine := func(it Item) string {
		return fmt.Sprintf("--- not shown: %s · %s · sha256 %.12s · %d characters\n", short(it.ID), it.Scope, it.Digest, units(it.Text))
	}
	header := func(shown int) string {
		h := fmt.Sprintf("SessionKeep is a local memory tool this user installed. The user previously recorded the standing instructions below; they are quoted exactly as the user wrote them.\n"+
			"Active instructions: %d. Shown in full here: %d. Record written: %s.\n", cov.Total, shown, c.WrittenAt)
		if shown < cov.Total {
			h += fmt.Sprintf("NOT SHOWN IN FULL: %d instruction(s) did not fit in this message. They are listed by id below. Their exact text is not known from this message; "+
				"it can be read by running: sessionkeep instruction show <id>\n", cov.Total-shown)
		}
		return h
	}

	// Reserve the larger header (with the notice) while choosing, so the result always fits.
	room := Budget - units(header(0)) - units(footer) - tailRoom
	full := make([]bool, len(c.Items))
	used := 0
	for i, it := range c.Items {
		// Everything not shown in full still needs its one-line listing.
		rest := 0
		for j := i + 1; j < len(c.Items); j++ {
			rest += units(omitLine(c.Items[j]))
		}
		if b := units(block(it)); used+b+rest <= room {
			full[i] = true
			used += b
			cov.Shown++
		} else {
			used += units(omitLine(it))
		}
	}
	cov.Omitted = cov.Total - cov.Shown

	var b strings.Builder
	b.WriteString(header(cov.Shown))
	for i, it := range c.Items {
		if full[i] {
			b.WriteString(block(it))
		}
	}
	limit := Budget - units(footer) - tailRoom
	for i, it := range c.Items {
		if full[i] {
			continue
		}
		line := omitLine(it)
		if units(b.String())+units(line) > limit {
			cov.Unlisted++
			continue
		}
		b.WriteString(line)
	}
	if cov.Unlisted > 0 {
		fmt.Fprintf(&b, "--- and %d more not listed; run: sessionkeep instruction list\n", cov.Unlisted)
	}
	b.WriteString(footer)
	if cp != "" {
		b.WriteString("\n" + cp)
	}
	return b.String(), cov
}

// MaxCheckpointUnits bounds how much of a checkpoint note is put into a session.
const MaxCheckpointUnits = 1500

// checkpointSection renders the latest checkpoint as a record of where work stood. It is worded
// as history, not as an instruction: the code may have moved on since.
func checkpointSection(cp *Checkpoint) string {
	if cp == nil {
		return ""
	}
	note := cp.Note
	if r := []rune(note); units(note) > MaxCheckpointUnits {
		for units(string(r)) > MaxCheckpointUnits {
			r = r[:len(r)-1]
		}
		note = string(r) + " ... [cut; the full note is shown by: sessionkeep resume]"
	}
	where := "no Git repository at that time"
	if cp.GitRepo {
		where = fmt.Sprintf("branch %s at %.10s, %d uncommitted file(s) at that time", cp.Branch, cp.Head, cp.Changed)
	}
	return fmt.Sprintf("--- latest checkpoint the user recorded for this project · %s · %s\n"+
		"This is a note of where work stood then, not an instruction. The code may have changed since; running `sessionkeep resume` compares it with the current state.\n%s\n--- end of checkpoint",
		day(cp.CreatedAt), where, note)
}

// Limits of the research note.
const (
	maxNoteClaims   = 5
	maxNoteSubject  = 100
	claimTimeFormat = "2006-01-02T15:04:05.000000000Z" // how the store writes a claim's dates
)

// researchSection says which researched facts should not be relied on as they stand. It reads
// only the cache: nothing is fetched and the database is not opened. Staleness is worked out
// from the time given, so a fact that went stale since the cache was written is still named.
func researchSection(rs *Research, now time.Time) string {
	if rs == nil || now.IsZero() {
		return ""
	}
	at := now.UTC().Format(claimTimeFormat)
	var stale, disputed int
	var lines []string
	for _, c := range rs.Claims {
		var line string
		switch {
		case c.State == "disputed":
			disputed++
			line = "DISPUTED (its source no longer contains the quoted words)"
		case c.State == "verified" && c.Expires <= at:
			stale++
			line = "STALE (re-check was due " + day(c.Expires) + ")"
		default:
			continue
		}
		if len(lines) < maxNoteClaims {
			subject := strings.Join(strings.Fields(c.Subject), " ")
			if r := []rune(subject); len(r) > maxNoteSubject {
				subject = string(r[:maxNoteSubject]) + "..."
			}
			lines = append(lines, fmt.Sprintf("- %s · %s · %s\n", short(c.ID), line, subject))
		}
	}
	if stale+disputed == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("--- researched facts recorded for this project that need another look\n")
	fmt.Fprintf(&b, "Past their re-check date: %d. Disputed: %d. They were checked against a source when recorded and may no longer be true. Nothing was fetched to write this note.\n", stale, disputed)
	for _, l := range lines {
		b.WriteString(l)
	}
	if n := stale + disputed - len(lines); n > 0 {
		fmt.Fprintf(&b, "- and %d more; all are listed by: sessionkeep research list\n", n)
	}
	if rs.More > 0 {
		fmt.Fprintf(&b, "(%d further claims of this project were not looked at for this note.)\n", rs.More)
	}
	b.WriteString("Before relying on one, read it with `sessionkeep research show <id>` and check it against its source again with `sessionkeep research check <id>` (stale ones together: `sessionkeep research check --due`).\n--- end of research note")
	return b.String()
}

func day(ts string) string {
	if len(ts) >= 10 {
		return ts[:10]
	}
	return ts
}

// sessionState remembers, per host session, which instruction set was last sent, so the same
// envelope is not repeated with every prompt.
func sessionState(p config.Paths, session string) string {
	return filepath.Join(p.CacheDir, "sessions", safeName(session)+".sent")
}

// AlreadySent reports whether this exact instruction set was already sent in this session.
func AlreadySent(p config.Paths, session, setDigest string) bool {
	raw, err := os.ReadFile(sessionState(p, session))
	return err == nil && strings.TrimSpace(string(raw)) == setDigest
}

// MarkSent records what was sent. Failure only means the envelope may be sent again.
func MarkSent(p config.Paths, session, setDigest string) {
	path := sessionState(p, session)
	if os.MkdirAll(filepath.Dir(path), 0o700) == nil {
		os.WriteFile(path, []byte(setDigest), 0o600)
	}
}

// PruneSessionState removes per-session markers not touched for 30 days.
func PruneSessionState(p config.Paths, now time.Time) {
	d := filepath.Join(p.CacheDir, "sessions")
	entries, err := os.ReadDir(d)
	if err != nil {
		return
	}
	for _, e := range entries {
		if fi, err := e.Info(); err == nil && now.Sub(fi.ModTime()) > 30*24*time.Hour {
			os.Remove(filepath.Join(d, e.Name()))
		}
	}
}

// RemoveCache deletes the cache files of a project that no longer exists.
func RemoveCache(p config.Paths, projectID string) {
	if projectID == "" {
		return
	}
	path := cachePath(p, projectID)
	os.Remove(path)
	os.Remove(path + fsx.AltSuffix)
}

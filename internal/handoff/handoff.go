// Package handoff defines the file one developer gives another so that the second starts on a
// project with the first one's project memory: its standing instructions, its checkpoints and,
// when asked for, what was said in its sessions. The file is plain JSON so that anyone can read
// exactly what it carries before sending or accepting it. A file from someone else is untrusted
// input: Read checks every field and nothing in it is acted on without the receiver's consent.
package handoff

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

// Kind and Format identify a handoff file.
const (
	Kind   = "sessionkeep-handoff"
	Format = 1
)

// Limits on what Read accepts.
const (
	MaxFileBytes = 512 << 20
	MaxTextBytes = 256 << 10
	maxItems     = 200000
)

// File is the whole content of a handoff file.
type File struct {
	Kind         string        `json:"kind"`
	Format       int           `json:"format"`
	CreatedAt    string        `json:"created_at"`
	MadeBy       string        `json:"sessionkeep_version"`
	From         string        `json:"from,omitempty"` // a name the sender chose to give
	SourceDevice string        `json:"source_device"`
	Project      Project       `json:"project"`
	Instructions []Instruction `json:"instructions"`
	Checkpoints  []Checkpoint  `json:"checkpoints"`
	Sessions     []Session     `json:"sessions"`
	NotIncluded  []string      `json:"not_included"`
}

// Project names the project the file is about.
type Project struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Instruction is one standing instruction of the project, word for word.
type Instruction struct {
	ID       string `json:"id"`
	Text     string `json:"text"`
	Recorded string `json:"recorded_at"`
}

// Checkpoint is one note of where work stood.
type Checkpoint struct {
	ID        string `json:"id"`
	Note      string `json:"note"`
	CreatedAt string `json:"created_at"`
	GitRepo   bool   `json:"git_repo"`
	Branch    string `json:"branch,omitempty"`
	Head      string `json:"head,omitempty"`
}

// Session is one session's prompts and replies, oldest first.
type Session struct {
	ID        string `json:"id"`
	StartedAt string `json:"started_at"`
	Turns     []Turn `json:"turns"`
}

// Turn is one prompt or reply.
type Turn struct {
	ID   string `json:"id"`
	Who  string `json:"who"` // user | assistant
	At   string `json:"at"`
	Text string `json:"text"`
}

// Counts says how much a file carries.
type Counts struct {
	Instructions int `json:"instructions"`
	Checkpoints  int `json:"checkpoints"`
	Sessions     int `json:"sessions"`
	Turns        int `json:"pieces_of_text"`
}

// Count counts what f carries.
func (f File) Count() Counts {
	c := Counts{Instructions: len(f.Instructions), Checkpoints: len(f.Checkpoints), Sessions: len(f.Sessions)}
	for _, s := range f.Sessions {
		c.Turns += len(s.Turns)
	}
	return c
}

// ErrInvalid means the file is not a usable handoff file.
var ErrInvalid = errors.New("not a valid SessionKeep handoff file")

var idForm = regexp.MustCompile(`^[A-Za-z0-9_-]{1,80}$`)

func checkTime(what, v string) error {
	if _, err := time.Parse(time.RFC3339Nano, v); err != nil {
		return fmt.Errorf("%w: %s has a time that cannot be read", ErrInvalid, what)
	}
	return nil
}

func checkText(what, v string) error {
	if strings.TrimSpace(v) == "" || len(v) > MaxTextBytes || !utf8.ValidString(v) {
		return fmt.Errorf("%w: %s is empty, too long or not text", ErrInvalid, what)
	}
	return nil
}

// Validate checks every field of a file read from disk.
func (f File) Validate() error {
	if f.Kind != Kind {
		return fmt.Errorf("%w: it is not marked as one", ErrInvalid)
	}
	if f.Format != Format {
		return fmt.Errorf("%w: format %d is not supported by this version (expects %d)", ErrInvalid, f.Format, Format)
	}
	if !idForm.MatchString(f.Project.ID) || strings.TrimSpace(f.Project.Name) == "" || len(f.Project.Name) > 200 || !idForm.MatchString(f.SourceDevice) || len(f.From) > 100 {
		return fmt.Errorf("%w: the project or sender fields are missing or malformed", ErrInvalid)
	}
	if err := checkTime("the file", f.CreatedAt); err != nil {
		return err
	}
	c := f.Count()
	if c.Instructions > maxItems || c.Checkpoints > maxItems || c.Sessions > maxItems || c.Turns > 10*maxItems {
		return fmt.Errorf("%w: it lists more records than any project holds", ErrInvalid)
	}
	seen := map[string]bool{}
	id := func(what, v string) error {
		if !idForm.MatchString(v) || seen[what+v] {
			return fmt.Errorf("%w: %s has a missing, malformed or repeated id", ErrInvalid, what)
		}
		seen[what+v] = true
		return nil
	}
	for _, x := range f.Instructions {
		if err := errors.Join(id("an instruction", x.ID), checkText("an instruction", x.Text), checkTime("an instruction", x.Recorded)); err != nil {
			return err
		}
	}
	for _, x := range f.Checkpoints {
		if err := errors.Join(id("a checkpoint", x.ID), checkText("a checkpoint", x.Note), checkTime("a checkpoint", x.CreatedAt)); err != nil {
			return err
		}
		if len(x.Branch) > 300 || len(x.Head) > 80 {
			return fmt.Errorf("%w: a checkpoint has malformed Git fields", ErrInvalid)
		}
	}
	for _, s := range f.Sessions {
		if err := errors.Join(id("a session", s.ID), checkTime("a session", s.StartedAt)); err != nil {
			return err
		}
		if len(s.Turns) == 0 {
			return fmt.Errorf("%w: a session has no text", ErrInvalid)
		}
		for _, t := range s.Turns {
			if t.Who != "user" && t.Who != "assistant" {
				return fmt.Errorf("%w: a piece of text is said by %q", ErrInvalid, t.Who)
			}
			if err := errors.Join(id("a piece of text", t.ID), checkText("a piece of text", t.Text), checkTime("a piece of text", t.At)); err != nil {
				return err
			}
		}
	}
	return nil
}

// Write creates path (which must not exist) with f as indented JSON.
func Write(path string, f File) (int64, error) {
	f.Kind, f.Format = Kind, Format
	// Written without HTML escaping, so the file reads as the text it carries.
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", " ")
	if err := enc.Encode(f); err != nil {
		return 0, err
	}
	raw := buf.Bytes()
	out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return 0, err
	}
	_, err = out.Write(raw)
	if err == nil {
		err = out.Sync()
	}
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(path)
		return 0, err
	}
	return int64(len(raw)), nil
}

// Read loads and validates a handoff file.
func Read(path string) (File, error) {
	var f File
	in, err := os.Open(path)
	if err != nil {
		return f, err
	}
	defer in.Close()
	raw, err := io.ReadAll(io.LimitReader(in, MaxFileBytes+1))
	if err != nil {
		return f, err
	}
	if len(raw) > MaxFileBytes {
		return f, fmt.Errorf("%w: the file is larger than %d MB", ErrInvalid, MaxFileBytes>>20)
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil {
		return f, fmt.Errorf("%w: it cannot be read as one (%v)", ErrInvalid, err)
	}
	return f, f.Validate()
}

// Scrubber shortens the sender's own folders in text, so a file does not carry where things
// live on the sender's computer (which usually includes the account name).
type Scrubber struct {
	rules []scrubRule
	Count int
}

type scrubRule struct {
	re   *regexp.Regexp
	with string
}

// NewScrubber replaces each project folder with <project> and the home folder with ~, in any
// letter case and with either kind of slash (also doubled, as inside JSON).
func NewScrubber(projectRoots []string, home string) *Scrubber {
	s := &Scrubber{}
	add := func(folder, with string) {
		folder = strings.TrimRight(strings.TrimSpace(folder), `/\`)
		if len(folder) < 4 {
			return // a drive or the root alone is not a folder worth hiding, and would match everything
		}
		parts := strings.FieldsFunc(folder, func(r rune) bool { return r == '/' || r == '\\' })
		for i := range parts {
			parts[i] = regexp.QuoteMeta(parts[i])
		}
		lead := ""
		if strings.HasPrefix(folder, "/") {
			lead = `/`
		}
		// The folder must end there: "app" is not the start of "application".
		end := ""
		if last := folder[len(folder)-1]; last == '_' || (last >= '0' && last <= '9') || (last|0x20 >= 'a' && last|0x20 <= 'z') {
			end = `\b`
		}
		if re, err := regexp.Compile(`(?i)` + lead + strings.Join(parts, `[\\/]+`) + end); err == nil {
			s.rules = append(s.rules, scrubRule{re, with})
		}
	}
	for _, r := range projectRoots {
		add(r, "<project>")
	}
	add(home, "~")
	return s
}

// Text returns text with the folders shortened.
func (s *Scrubber) Text(text string) string {
	for _, r := range s.rules {
		text = r.re.ReplaceAllStringFunc(text, func(string) string {
			s.Count++
			return r.with
		})
	}
	return text
}

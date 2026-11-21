package handoff

import (
	"errors"
	"strings"
	"testing"
)

func TestFoldersAreShortenedInEveryForm(t *testing.T) {
	s := NewScrubber([]string{`c:\work\shop app`}, `C:\Users\sam`)
	in := `Open C:\Work\Shop App\src\a.go, c:/work/shop app/b.go and "C:\\Work\\Shop App\\c.go"; notes are in C:\Users\Sam\notes.txt. Not C:\Workshop, C:\Work\Shop Apple or D:\other.`
	got := s.Text(in)
	want := `Open <project>\src\a.go, <project>/b.go and "<project>\\c.go"; notes are in ~\notes.txt. Not C:\Workshop, C:\Work\Shop Apple or D:\other.`
	if got != want || s.Count != 4 {
		t.Fatalf("got  %s\nwant %s\ncount %d", got, want, s.Count)
	}
	u := NewScrubber([]string{"/home/sam/app"}, "/home/sam")
	if got := u.Text("see /home/sam/app/x and /home/sam/.config"); got != "see <project>/x and ~/.config" {
		t.Fatalf("got %s", got)
	}
	// A drive or the root alone is never a rule: it would rewrite every path.
	if got := NewScrubber([]string{`C:\`, "/"}, "").Text(`C:\a and /b`); got != `C:\a and /b` {
		t.Fatalf("got %s", got)
	}
}

func TestAFileIsCheckedFieldByField(t *testing.T) {
	good := func() File {
		return File{Kind: Kind, Format: Format, CreatedAt: "2026-11-21T10:00:00Z", MadeBy: "0.11.0", SourceDevice: "dev-1", Project: Project{ID: "p-1", Name: "app"},
			Instructions: []Instruction{{ID: "i-1", Text: "Use tabs.", Recorded: "2026-11-20T10:00:00Z"}},
			Checkpoints:  []Checkpoint{{ID: "c-1", Note: "Done: a", CreatedAt: "2026-11-20T11:00:00.000000000Z"}},
			Sessions:     []Session{{ID: "s-1", StartedAt: "2026-11-20T09:00:00Z", Turns: []Turn{{ID: "t-1", Who: "user", At: "2026-11-20T09:00:01Z", Text: "Hello"}}}}}
	}
	if err := good().Validate(); err != nil {
		t.Fatal(err)
	}
	bad := map[string]func(*File){
		"another kind":          func(f *File) { f.Kind = "something" },
		"a newer format":        func(f *File) { f.Format = 2 },
		"no device":             func(f *File) { f.SourceDevice = "" },
		"an id with a path":     func(f *File) { f.Project.ID = "../x" },
		"an empty instruction":  func(f *File) { f.Instructions[0].Text = "  " },
		"a huge instruction":    func(f *File) { f.Instructions[0].Text = strings.Repeat("x", MaxTextBytes+1) },
		"a bad time":            func(f *File) { f.Checkpoints[0].CreatedAt = "yesterday" },
		"a speaker not allowed": func(f *File) { f.Sessions[0].Turns[0].Who = "tool" },
		"a repeated id":         func(f *File) { f.Sessions[0].Turns = append(f.Sessions[0].Turns, f.Sessions[0].Turns[0]) },
		"a session of nothing":  func(f *File) { f.Sessions[0].Turns = nil },
		"text that is not text": func(f *File) { f.Sessions[0].Turns[0].Text = "a\xffb" },
	}
	for name, change := range bad {
		f := good()
		change(&f)
		if err := f.Validate(); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s was accepted (%v)", name, err)
		}
	}
}

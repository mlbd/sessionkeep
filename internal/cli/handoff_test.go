package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// One developer makes a handoff file for a project; another, on another computer with another
// folder, receives it and can search, resume and is given the project's instructions.
func TestAProjectIsHandedToATeammate(t *testing.T) {
	a := newSandbox(t)
	app := a.dir("app")
	a.ok("project", "add", app, "--capture", "conversation")
	a.remember("project", "Use tabs in this project HANDRULE.", "--project", app)
	a.remember("global", "Always answer briefly GLOBALRULE.")
	a.say("UserPromptSubmit", "h-1", app, "Open "+filepath.Join(app, "src", "main.go")+" and add totals PROMPTMARK, key AKIAIOSFODNN7EXAMPLE.")
	a.fail("h-1", app, "Bash", "Exit code 1\nError: a specific build failure TOOLMARK in the totals step")
	a.say("Stop", "h-1", app, "Totals are added REPLYMARK.")
	a.ok("ingest")
	a.ok("checkpoint", "Done: totals. Next: tax CHECKMARK", "--project", app)

	file := filepath.Join(t.TempDir(), "app.handoff.json")
	code, out, errOut := a.run("handoff", "make", file, "--project", app, "--with-sessions")
	if _, err := os.Stat(file); code != ExitNeedsConsent || err == nil || !strings.Contains(out, "NOTHING WRITTEN YET") || !strings.Contains(out, "1 session(s) with 2 piece(s)") {
		t.Fatalf("a preview wrote a file or said too little: exit %d\n%s\n%s", code, out, errOut)
	}
	a.ok("handoff", "make", file, "--project", app, "--with-sessions", "--from", "Sam", "--yes")
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, want := range []string{"HANDRULE", "PROMPTMARK", "REPLYMARK", "CHECKMARK", "<project>", `"from": "Sam"`} {
		if !strings.Contains(text, want) {
			t.Errorf("the file lacks %q", want)
		}
	}
	lower := strings.ToLower(strings.ReplaceAll(text, `\\`, `\`))
	for _, not := range []string{"GLOBALRULE", "TOOLMARK", "AKIAIOSFODNN7EXAMPLE"} {
		if strings.Contains(text, not) {
			t.Errorf("the file carries %q", not)
		}
	}
	if strings.Contains(lower, strings.ToLower(app)) {
		t.Errorf("the file carries the sender's folder %q", app)
	}
	if code, _, _ := a.run("handoff", "make", file, "--project", app, "--yes"); code != ExitFailure {
		t.Errorf("an existing file was overwritten or accepted: exit %d", code)
	}
	bare := filepath.Join(t.TempDir(), "bare.json")
	a.ok("handoff", "make", bare, "--project", app, "--yes")
	if raw, _ := os.ReadFile(bare); strings.Contains(string(raw), "PROMPTMARK") || !strings.Contains(string(raw), "HANDRULE") {
		t.Error("without --with-sessions the file should carry instructions and no session text")
	}
	if code, _, _ := a.run("handoff", "receive", file, "--project", app, "--yes"); code != ExitFailure {
		t.Errorf("the sender received their own file: exit %d", code)
	}

	b := newSandbox(t)
	theirs := b.dir("their-copy")
	b.ok("project", "add", theirs, "--capture", "conversation")
	code, out, _ = b.run("handoff", "receive", file, "--project", theirs)
	if code != ExitNeedsConsent || !strings.Contains(out, "HANDRULE") || !strings.Contains(out, "NOTHING RECEIVED YET") || !strings.Contains(out, "Sam") {
		t.Fatalf("the preview should show the instructions and change nothing: exit %d\n%s", code, out)
	}
	if hits := b.search("PROMPTMARK", "--project", theirs).Data.Hits; len(hits) != 0 {
		t.Fatal("a preview stored text")
	}
	out = b.ok("handoff", "receive", file, "--project", theirs, "--yes")
	if !strings.Contains(out, "RECEIVED: 1 instruction(s) waiting for your approval on the dashboard (0 already there), 1 checkpoint(s) (0 already there), 2 piece(s) of text (0 already there)") {
		t.Errorf("first receive: %s", out)
	}
	for _, word := range []string{"PROMPTMARK", "REPLYMARK"} {
		if hits := b.search(word, "--project", theirs).Data.Hits; len(hits) != 1 {
			t.Errorf("%s is found %d times by the receiver, want once", word, len(hits))
		}
	}
	if list := b.sessions("--project", theirs); len(list) != 1 {
		t.Errorf("the receiver lists %d sessions, want 1", len(list))
	}
	// Someone else's instruction waits for the receiver's approval; their checkpoint is shown as theirs.
	got := b.inject("SessionStart", "new-1", theirs)
	if strings.Contains(got, "HANDRULE") || !strings.Contains(got, "CHECKMARK") || !strings.Contains(got, "handoff file of Sam") ||
		!strings.Contains(got, "written by a teammate and received through sharing") || strings.Contains(got, "checkpoint the user recorded") {
		t.Errorf("before approval a new session must get the teammate's checkpoint, marked as theirs, and not their instruction:\n%s", got)
	}
	waiting := b.waiting()
	if len(waiting) != 1 || waiting[0].Text != "Use tabs in this project HANDRULE." || waiting[0].Author != "received:Sam" {
		t.Fatalf("the received instruction should wait for approval, marked as Sam's: %+v", waiting)
	}
	b.ok("instruction", "approve", waiting[0].ID, "--yes")
	if got := b.inject("SessionStart", "new-2", theirs); !strings.Contains(got, "HANDRULE") || !strings.Contains(got, `written by a teammate ("Sam"), approved by the user`) {
		t.Errorf("after approval a new session is not given the instruction, marked as Sam's:\n%s", got)
	}
	out = b.ok("handoff", "receive", file, "--project", theirs, "--yes")
	if !strings.Contains(out, "RECEIVED: 0 instruction(s) waiting for your approval on the dashboard (1 already there), 0 checkpoint(s) (1 already there), 0 piece(s) of text (2 already there)") {
		t.Errorf("receiving the same file again should add nothing: %s", out)
	}

	// A project that saves no text takes the instructions and checkpoints, not the sessions.
	quiet := b.dir("quiet")
	b.ok("project", "add", quiet, "--capture", "metadata")
	out = b.ok("handoff", "receive", file, "--project", quiet, "--yes")
	if !strings.Contains(out, "sessions are left out") || !strings.Contains(out, "0 piece(s) of text (0 already there)") {
		t.Errorf("a text-free project: %s", out)
	}

	// A file that was altered into something else is refused whole.
	bad := filepath.Join(t.TempDir(), "bad.json")
	os.WriteFile(bad, []byte(strings.Replace(text, `"who": "user"`, `"who": "system"`, 1)), 0o600)
	if code, _, errOut := b.run("handoff", "receive", bad, "--project", theirs, "--yes"); code != ExitFailure || !strings.Contains(errOut, "not a valid SessionKeep handoff file") {
		t.Errorf("an altered file: exit %d %q", code, errOut)
	}
	os.WriteFile(bad, []byte(strings.Replace(text, `"kind": "sessionkeep-handoff"`, `"kind": "sessionkeep-handoff", "extra": 1`, 1)), 0o600)
	if code, _, _ := b.run("handoff", "receive", bad, "--project", theirs, "--yes"); code != ExitFailure {
		t.Errorf("a file with unknown fields was accepted: exit %d", code)
	}
}

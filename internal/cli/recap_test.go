package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type recapJSON struct {
	Data struct {
		Label    string
		Sessions int
		Requests int
		Projects []struct {
			Name     string
			Sessions []struct {
				ID        string
				Requests  int
				Failures  int    `json:"failed_commands"`
				First     string `json:"first_request"`
				LastReply string `json:"last_reply"`
				Files     []string
			}
			Checkpoints []struct{ Note string }
			Commits     []struct{ ID, Subject string }
		}
	}
}

func (sb *sandbox) recap(args ...string) recapJSON {
	sb.t.Helper()
	var out recapJSON
	if err := json.Unmarshal([]byte(sb.ok(append([]string{"recap", "--json"}, args...)...)), &out); err != nil {
		sb.t.Fatal(err)
	}
	return out
}

// A recap says what was done in a day: sessions, first request, last reply, files, failed
// commands, checkpoints and commits, for one project unless every project is asked for.
func TestRecapSaysWhatWasDoneInADay(t *testing.T) {
	sb := newSandbox(t)
	app := sb.dir("app")
	other := sb.dir("other")
	sb.ok("project", "add", app, "--capture", "conversation")
	sb.ok("project", "add", other, "--capture", "conversation")
	gitRun(t, app, "init", "-q")
	os.WriteFile(filepath.Join(app, "a.txt"), []byte("a"), 0o600)
	gitRun(t, app, "add", ".")
	gitRun(t, app, "commit", "-q", "-m", "Add the invoice page COMMITMARK")

	sb.say("UserPromptSubmit", "r-1", app, "Build the invoice page FIRSTMARK.")
	raw, _ := json.Marshal(map[string]any{"session_id": "r-1", "cwd": app, "hook_event_name": "PostToolUse", "tool_name": "Edit",
		"tool_use_id": "t1", "tool_input": map[string]string{"file_path": "src/invoice.ts"}, "tool_response": "ok"})
	sb.hook("PostToolUse", string(raw))
	sb.fail("r-1", app, "Bash", "Exit code 1\nError: something specific went wrong in the build step")
	sb.say("Stop", "r-1", app, "A first answer.")
	sb.say("UserPromptSubmit", "r-1", app, "Now the totals.")
	sb.say("Stop", "r-1", app, "Totals are done LASTMARK.")
	sb.say("UserPromptSubmit", "r-2", other, "Something in the other project OTHERMARK.")
	sb.ok("ingest")
	sb.ok("checkpoint", "Done: invoice page. Next: tax CHECKMARK", "--project", app)

	got := sb.recap("--project", app)
	if got.Data.Sessions != 1 || got.Data.Requests != 2 || len(got.Data.Projects) != 1 {
		t.Fatalf("recap of one project: %+v", got.Data)
	}
	p := got.Data.Projects[0]
	s := p.Sessions[0]
	if s.Requests != 2 || s.Failures != 1 || !strings.Contains(s.First, "FIRSTMARK") || !strings.Contains(s.LastReply, "LASTMARK") ||
		len(s.Files) != 1 || s.Files[0] != "src/invoice.ts" {
		t.Errorf("session: %+v", s)
	}
	if len(p.Checkpoints) != 1 || !strings.Contains(p.Checkpoints[0].Note, "CHECKMARK") {
		t.Errorf("checkpoints: %+v", p.Checkpoints)
	}
	if len(p.Commits) != 1 || !strings.Contains(p.Commits[0].Subject, "COMMITMARK") {
		t.Errorf("commits: %+v", p.Commits)
	}
	text := sb.ok("recap", "today", "--project", app)
	for _, want := range []string{"FIRSTMARK", "LASTMARK", "CHECKMARK", "COMMITMARK", "src/invoice.ts", "1 failed command(s)"} {
		if !strings.Contains(text, want) {
			t.Errorf("the printed recap lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "OTHERMARK") {
		t.Errorf("another project is in the recap of one project:\n%s", text)
	}

	if all := sb.recap("--all-projects"); all.Data.Sessions != 2 || len(all.Data.Projects) != 2 {
		t.Errorf("recap of all projects: %+v", all.Data)
	}
	if week := sb.recap("week", "--project", app); week.Data.Sessions != 1 {
		t.Errorf("the last seven days: %+v", week.Data)
	}
	for _, day := range []string{"yesterday", "2020-01-01"} {
		if past := sb.recap(day, "--all-projects"); past.Data.Sessions != 0 || len(past.Data.Projects) != 0 {
			t.Errorf("%s should be empty: %+v", day, past.Data)
		}
	}
	if !strings.Contains(sb.ok("recap", "2020-01-01", "--all-projects"), "Nothing was saved") {
		t.Error("an empty day should say so")
	}
	for _, bad := range [][]string{{"recap", "someday", "--all-projects"}, {"recap", "today", "extra"}, {"recap", "--project", app, "--all-projects"}} {
		if code, _, _ := sb.run(bad...); code != ExitUsage {
			t.Errorf("%v: exit %d, want usage", bad, code)
		}
	}
}

package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// touch reports a tool call on a file through the real hook.
func (sb *sandbox) touch(session, cwd, tool, file string) {
	sb.t.Helper()
	raw, _ := json.Marshal(map[string]any{"session_id": session, "cwd": cwd, "hook_event_name": "PostToolUse", "tool_name": tool,
		"tool_use_id": "t-" + session + tool + filepath.Base(file), "tool_input": map[string]string{"file_path": file}, "tool_response": "ok"})
	if code, _, errOut := sb.hook("PostToolUse", string(raw)); code != ExitOK || errOut != "" {
		sb.t.Fatalf("hook: %d %q", code, errOut)
	}
}

type storyJSON struct {
	Data struct {
		Sessions []struct {
			Changes int
			Files   []string
			Asked   string
			Reply   string
		}
		OnlyRead int `json:"sessions_that_only_read"`
		Commit   struct{ ID, Subject string }
	}
}

func (sb *sandbox) story(args ...string) storyJSON {
	sb.t.Helper()
	var out storyJSON
	if err := json.Unmarshal([]byte(sb.ok(append(args, "--json")...)), &out); err != nil {
		sb.t.Fatal(err)
	}
	return out
}

// The story of a file is the sessions that changed it with what was asked and answered; the
// reason for a commit is the sessions that changed its files shortly before it.
func TestAFileAndACommitLeadBackToTheirConversations(t *testing.T) {
	sb := newSandbox(t)
	app := sb.dir("app")
	sb.ok("project", "add", app, "--capture", "conversation")
	gitRun(t, app, "init", "-q")
	os.MkdirAll(filepath.Join(app, "src"), 0o700)
	invoice := filepath.Join(app, "src", "invoice.ts")
	other := filepath.Join(app, "src", "other.ts")

	sb.say("UserPromptSubmit", "w-1", app, "Unrelated first question.")
	sb.say("Stop", "w-1", app, "Unrelated answer.")
	sb.say("UserPromptSubmit", "w-1", app, "Add totals to the invoice ASKMARK.")
	sb.touch("w-1", app, "Read", invoice)
	sb.touch("w-1", app, "Edit", invoice)
	sb.touch("w-1", app, "Edit", invoice)
	sb.say("Stop", "w-1", app, "Totals are in place REPLYMARK.")
	sb.say("UserPromptSubmit", "w-2", app, "Just look at the invoice LOOKMARK.")
	sb.touch("w-2", app, "Read", invoice)
	sb.say("UserPromptSubmit", "w-3", app, "Change the other file OTHERMARK.")
	sb.touch("w-3", app, "Write", other)
	sb.ok("ingest")

	os.WriteFile(invoice, []byte("one\ntwo\n"), 0o600)
	gitRun(t, app, "add", ".")
	gitRun(t, app, "commit", "-q", "-m", "Add totals COMMITMARK")

	for _, path := range []string{invoice, "src/invoice.ts", `src\invoice.ts`} {
		got := sb.story("file", path, "--project", app)
		if len(got.Data.Sessions) != 1 || got.Data.OnlyRead != 1 {
			t.Fatalf("%s: %+v", path, got.Data)
		}
		if s := got.Data.Sessions[0]; s.Changes != 2 || !strings.Contains(s.Asked, "ASKMARK") || !strings.Contains(s.Reply, "REPLYMARK") {
			t.Errorf("%s: %+v", path, s)
		}
	}
	if got := sb.story("file", "voice.ts", "--project", app); len(got.Data.Sessions) != 0 {
		t.Errorf("part of a file name matched: %+v", got.Data)
	}
	text := sb.ok("file", "src/invoice.ts", "--project", app)
	if !strings.Contains(text, "ASKMARK") || strings.Contains(text, "LOOKMARK") || strings.Contains(text, "OTHERMARK") || !strings.Contains(text, "sessionkeep session show ") {
		t.Errorf("printed story:\n%s", text)
	}

	for _, ref := range []string{"HEAD", "src/invoice.ts:2"} {
		got := sb.story("why", ref, "--project", app)
		if !strings.Contains(got.Data.Commit.Subject, "COMMITMARK") || len(got.Data.Sessions) != 1 || !strings.Contains(got.Data.Sessions[0].Asked, "ASKMARK") {
			t.Errorf("why %s: %+v", ref, got.Data)
		}
	}
	if text := sb.ok("why", "src/invoice.ts:2", "--project", app); !strings.Contains(text, "Line 2 of src/invoice.ts") || !strings.Contains(text, "REPLYMARK") {
		t.Errorf("printed reason:\n%s", text)
	}
	for _, bad := range [][]string{{"why", "--project", app}, {"why", "-p", "--project", app}, {"why", "a b", "--project", app}, {"file", "--project", app}} {
		if code, _, _ := sb.run(bad...); code != ExitUsage {
			t.Errorf("%v: exit %d, want usage", bad, code)
		}
	}
	if code, _, _ := sb.run("why", "nosuchcommit", "--project", app); code != ExitFailure {
		t.Errorf("an unknown commit: exit %d", code)
	}
}

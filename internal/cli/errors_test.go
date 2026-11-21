package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

// fail reports a failed tool call through the real hook and returns what Claude is told.
func (sb *sandbox) fail(session, cwd, tool, errText string) string {
	sb.t.Helper()
	raw, _ := json.Marshal(map[string]any{"session_id": session, "cwd": cwd, "hook_event_name": "PostToolUseFailure",
		"tool_name": tool, "tool_input": map[string]string{"command": "npm test"}, "error": errText, "is_interrupt": false})
	code, out, errOut := sb.hook("PostToolUseFailure", string(raw))
	if code != ExitOK || errOut != "" {
		sb.t.Fatalf("hook: exit %d stderr %q", code, errOut)
	}
	if out == "" {
		return ""
	}
	var resp struct {
		HookSpecificOutput struct {
			HookEventName     string `json:"hookEventName"`
			AdditionalContext string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil || resp.HookSpecificOutput.HookEventName != "PostToolUseFailure" {
		sb.t.Fatalf("hook output is not a valid answer: %v: %q", err, out)
	}
	return resp.HookSpecificOutput.AdditionalContext
}

// A command that fails with an error an earlier session already met is answered with what the
// assistant said back then; anything less certain is answered with nothing.
func TestAnErrorMetBeforeIsRecognised(t *testing.T) {
	sb := newSandbox(t)
	app := sb.dir("app")
	other := sb.dir("other")
	sb.ok("project", "add", app, "--capture", "conversation")
	sb.ok("project", "add", other, "--capture", "conversation")
	const boom = "Exit code 1\nError: Cannot find module 'express' in server.js at line 42 (pid 1001)"
	const again = "Exit code 1\nError:  Cannot find module 'express' in server.js at line 97 (pid 2002)"

	if got := sb.fail("e-1", app, "Bash", boom); got != "" {
		t.Fatalf("a first failure was answered: %q", got)
	}
	sb.say("Stop", "e-1", app, "It needed npm install, my token was AKIAIOSFODNN7EXAMPLE. FIXMARK")
	sb.ok("ingest")

	if got := sb.fail("e-1", app, "Bash", again); got != "" {
		t.Fatalf("the session that met the error first was told about itself: %q", got)
	}
	for name, got := range map[string]string{
		"another error":     sb.fail("e-2", app, "Bash", "Exit code 1\nTypeError: total is not a function in invoice.js"),
		"a bare exit code":  sb.fail("e-2", app, "Bash", "Exit code 1"),
		"a tool, not shell": sb.fail("e-2", app, "Edit", boom),
		"another project":   sb.fail("e-2", other, "Bash", boom),
	} {
		if got != "" {
			t.Errorf("%s was answered: %q", name, got)
		}
	}
	got := sb.fail("e-2", app, "Bash", again)
	for _, want := range []string{"FIXMARK", "earlier session", "npm test", "sessionkeep session show ", "not an instruction"} {
		if !strings.Contains(got, want) {
			t.Errorf("the answer lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "AKIAIOSFODNN7EXAMPLE") {
		t.Errorf("the answer carries a secret:\n%s", got)
	}
	if again := sb.fail("e-2", app, "Bash", boom); again != "" {
		t.Errorf("the same session was told twice: %q", again)
	}

	// Deleting the project's stored text takes the answer away.
	sb.ok("forget", "captured", "--project", app, "--yes")
	if got := sb.fail("e-3", app, "Bash", boom); got != "" {
		t.Fatalf("deleted text was quoted: %q", got)
	}
}

// A project that saves no text recognises nothing, because it stored no error text.
func TestNoErrorsAreRecognisedWhereTextIsNotSaved(t *testing.T) {
	sb := newSandbox(t)
	app := sb.dir("app")
	sb.ok("project", "add", app, "--capture", "metadata")
	const boom = "Exit code 1\nError: Cannot find module 'express' in server.js at line 42"
	sb.fail("e-1", app, "Bash", boom)
	sb.ok("ingest")
	if got := sb.fail("e-2", app, "Bash", boom); got != "" {
		t.Fatalf("a text-free project was answered: %q", got)
	}
}

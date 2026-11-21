package cli

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"sessionkeep/internal/capture"
	"sessionkeep/internal/instruct"
)

// ask sends a request through the real hook and returns what Claude is given with it.
func (sb *sandbox) ask(session, cwd, prompt string) string {
	sb.t.Helper()
	raw, _ := json.Marshal(map[string]string{"session_id": session, "cwd": cwd, "hook_event_name": "UserPromptSubmit", "prompt": prompt})
	code, out, errOut := sb.hook("UserPromptSubmit", string(raw))
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
	if err := json.Unmarshal([]byte(out), &resp); err != nil || resp.HookSpecificOutput.HookEventName != "UserPromptSubmit" {
		sb.t.Fatalf("hook output is not a valid answer: %v: %q", err, out)
	}
	return resp.HookSpecificOutput.AdditionalContext
}

// With automatic recall a new session is told about the latest sessions, and a request brings
// along the stored pieces that share its words. With recall when asked, neither happens.
func TestAutomaticRecallGivesEarlierSessionsUnasked(t *testing.T) {
	// The search gives up quickly by design; a run slowed by the race detector must not.
	oldSearch, oldHook := instruct.RelatedTimeout, capture.Deadline
	instruct.RelatedTimeout, capture.Deadline = 30*time.Second, 60*time.Second
	t.Cleanup(func() { instruct.RelatedTimeout, capture.Deadline = oldSearch, oldHook })
	sb := newSandbox(t)
	app := sb.dir("app")
	other := sb.dir("other")
	sb.ok("project", "add", app, "--capture", "conversation")
	sb.ok("project", "add", other, "--capture", "conversation")
	sb.say("UserPromptSubmit", "a-1", app, "The invoice totals must include regional tax TAXMARK.")
	sb.say("Stop", "a-1", app, "Regional tax is now added to the invoice totals in billing.go REPLYMARK.")
	sb.say("UserPromptSubmit", "a-2", app, "Rename the login button LOGINMARK.")
	sb.say("UserPromptSubmit", "o-1", other, "The invoice totals in the other project OTHERMARK regional tax.")
	sb.ok("ingest")

	const question = "Why do the invoice totals look wrong for regional tax?"
	if got := sb.ask("a-3", app, question); got != "" {
		t.Fatalf("recall when asked gave something unasked: %q", got)
	}
	if got := sb.inject("SessionStart", "a-3", app); got != "" {
		t.Fatalf("recall when asked gave a history at the start: %q", got)
	}
	for _, bad := range [][]string{{"project", "recall", app}, {"project", "recall", app, "always"}} {
		if code, _, _ := sb.run(bad...); code != ExitUsage {
			t.Errorf("%v: exit %d, want usage", bad, code)
		}
	}
	sb.ok("project", "recall", app, "automatic")
	if list := sb.ok("project", "list", "--json"); !strings.Contains(list, `"recall":"automatic"`) && !strings.Contains(list, `"recall": "automatic"`) {
		t.Errorf("the project list does not show the mode: %s", list)
	}

	start := sb.inject("SessionStart", "a-3", app)
	for _, want := range []string{"TAXMARK", "LOGINMARK", "latest earlier sessions", "not as a request"} {
		if !strings.Contains(start, want) {
			t.Errorf("the start lacks %q:\n%s", want, start)
		}
	}
	if strings.Contains(start, "OTHERMARK") {
		t.Errorf("the start names another project:\n%s", start)
	}

	got := sb.ask("a-3", app, question)
	for _, want := range []string{"TAXMARK", "REPLYMARK", "not instructions", "sessionkeep memory get"} {
		if !strings.Contains(got, want) {
			t.Errorf("the request lacks %q:\n%s", want, got)
		}
	}
	for _, not := range []string{"OTHERMARK", "LOGINMARK"} {
		if strings.Contains(got, not) {
			t.Errorf("the request brought %q:\n%s", not, got)
		}
	}
	if n := len(utf16.Encode([]rune(got))); n > 9000 {
		t.Fatalf("%d units, over the limit", n)
	}
	if again := sb.ask("a-3", app, question); again != "" {
		t.Errorf("the same pieces were given twice in one session: %q", again)
	}
	for name, prompt := range map[string]string{"a short reply": "yes", "a command": "/compact invoice totals regional tax", "nothing in common": "Paint the garden fence turquoise tomorrow"} {
		if got := sb.ask("a-4", app, prompt); got != "" {
			t.Errorf("%s brought something: %q", name, got)
		}
	}
	// A session is not handed its own words back.
	if got := sb.ask("a-1", app, question); strings.Contains(got, "TAXMARK") || strings.Contains(got, "REPLYMARK") {
		t.Errorf("a session was given its own text: %q", got)
	}
	// The other project still has recall when asked.
	if got := sb.ask("o-2", other, question); got != "" {
		t.Errorf("the other project was given something: %q", got)
	}

	sb.ok("project", "recall", app, "asked")
	if got := sb.ask("a-5", app, question); got != "" {
		t.Errorf("after switching back: %q", got)
	}
}

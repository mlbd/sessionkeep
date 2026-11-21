package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

// Something said in three sessions is suggested as an instruction until one covers it.
func TestRepeatedRequestsAreSuggestedAsInstructions(t *testing.T) {
	sb := newSandbox(t)
	app := sb.dir("app")
	other := sb.dir("other")
	sb.ok("project", "add", app, "--capture", "conversation")
	sb.ok("project", "add", other, "--capture", "conversation")
	sb.say("UserPromptSubmit", "g-1", app, "Fix the header. Always use pnpm, not npm.")
	sb.say("UserPromptSubmit", "g-2", app, "always use pnpm not npm")
	sb.say("UserPromptSubmit", "g-3", app, "Please always use pnpm, not npm!")
	sb.say("UserPromptSubmit", "g-4", other, "Never commit without asking me.")
	sb.say("UserPromptSubmit", "g-5", other, "never commit without asking me")
	sb.ok("ingest")

	type out struct {
		Data struct {
			Suggestions []struct {
				Text     string
				Sessions int
			}
		}
	}
	read := func(args ...string) out {
		var o out
		if err := json.Unmarshal([]byte(sb.ok(append([]string{"suggest", "--json"}, args...)...)), &o); err != nil {
			t.Fatal(err)
		}
		return o
	}
	got := read("--project", app)
	if len(got.Data.Suggestions) != 1 || got.Data.Suggestions[0].Sessions != 3 || !strings.Contains(got.Data.Suggestions[0].Text, "pnpm") {
		t.Fatalf("suggestions: %+v", got.Data)
	}
	if text := sb.ok("suggest", "--project", app); !strings.Contains(text, "sessionkeep remember") || !strings.Contains(text, "--scope project") || !strings.Contains(text, "not by a model") {
		t.Errorf("printed suggestions:\n%s", text)
	}
	if o := read("--project", other); len(o.Data.Suggestions) != 0 {
		t.Errorf("two sessions were enough: %+v", o.Data)
	}
	if o := read("--all-projects"); len(o.Data.Suggestions) != 1 {
		t.Errorf("all projects: %+v", o.Data)
	}
	sb.remember("project", "Use pnpm, never npm.", "--project", app)
	if o := read("--project", app); len(o.Data.Suggestions) != 0 {
		t.Errorf("an instruction covers it, yet it is suggested: %+v", o.Data)
	}
}

package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// say captures one prompt or reply through the real hook for a project folder.
func (sb *sandbox) say(event, session, cwd, text string) {
	sb.t.Helper()
	body := map[string]string{"session_id": session, "cwd": cwd, "hook_event_name": event}
	if event == "Stop" {
		body["last_assistant_message"] = text
	} else {
		body["prompt"] = text
	}
	raw, _ := json.Marshal(body)
	if code, _, errOut := sb.hook(event, string(raw)); code != ExitOK || errOut != "" {
		sb.t.Fatalf("hook: %d %q", code, errOut)
	}
}

type searchOut struct {
	Data struct {
		Hits []struct {
			ID, Who, Snippet string
			ProjectID        string `json:"project_id"`
		}
		Method     string
		NextCursor string `json:"next_cursor"`
		Pending    int    `json:"not_searched_pending_events"`
	}
}

func (sb *sandbox) search(args ...string) searchOut {
	sb.t.Helper()
	var out searchOut
	raw := sb.ok(append([]string{"memory", "search"}, append(args, "--json")...)...)
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		sb.t.Fatalf("bad JSON: %v: %q", err, raw)
	}
	return out
}

// T06: two projects holding near-identical text; nothing from one may appear in the other.
func TestSearchThroughTheCommandsIsProjectScoped(t *testing.T) {
	sb := newSandbox(t)
	a, b := sb.dir("alpha"), sb.dir("beta")
	sb.ok("project", "add", a, "--capture", "conversation")
	sb.ok("project", "add", b, "--capture", "conversation")
	sb.say("UserPromptSubmit", "s-a", a, "The deploy key rotation happens on the first Monday. Error seen: ECONNRESET: socket hang up at /api/v2/users?id=7")
	sb.say("Stop", "s-a", a, "Noted: rotation on the first Monday.")
	for i := 0; i < 12; i++ {
		sb.say("UserPromptSubmit", "s-b", b, fmt.Sprintf("BETA-CONFIDENTIAL-%d deploy key rotation deploy key rotation first Monday", i))
	}

	// Before ingest: an honest "not found" that says events are still waiting.
	out := sb.ok("memory", "search", "rotation", "--project", a)
	if !strings.Contains(out, "not found") {
		// The database does not exist until something with the lease has run; project add did that.
		t.Fatalf("before ingest: %q", out)
	}
	if !strings.Contains(out, "14 captured event(s) are not stored yet") {
		t.Fatalf("search must say what it did not look at: %q", out)
	}
	sb.ok("ingest")

	res := sb.search("deploy key rotation", "--project", a, "--limit", "50")
	if len(res.Data.Hits) != 1 || res.Data.Pending != 0 {
		t.Fatalf("project alpha: %+v", res.Data)
	}
	raw := sb.ok("memory", "search", "deploy key rotation", "--project", a, "--limit", "50")
	if strings.Contains(raw, "BETA-CONFIDENTIAL") || !strings.Contains(raw, "that is all of them") {
		t.Fatalf("project beta text appeared in a project alpha search:\n%s", raw)
	}
	if res := sb.search("BETA-CONFIDENTIAL-3", "--project", a, "--exact"); len(res.Data.Hits) != 0 {
		t.Fatalf("exact search crossed projects: %+v", res.Data.Hits)
	}
	if res := sb.search("first Monday", "--all-projects", "--limit", "50"); len(res.Data.Hits) != 14 {
		t.Fatalf("--all-projects found %d, want 14", len(res.Data.Hits))
	}
	// Exact strings and filters.
	if res := sb.search("ECONNRESET: socket hang up at /api/v2/users?id=7", "--project", a); len(res.Data.Hits) != 1 {
		t.Fatalf("exact error string: %+v", res.Data)
	}
	if res := sb.search("rotation", "--project", a, "--who", "assistant"); len(res.Data.Hits) != 1 || res.Data.Hits[0].Who != "assistant" {
		t.Fatalf("--who assistant: %+v", res.Data.Hits)
	}
	if res := sb.search("rotation", "--project", a, "--since", "2099-01-01"); len(res.Data.Hits) != 0 {
		t.Fatalf("--since in the future: %+v", res.Data.Hits)
	}

	// Reading a record: allowed inside its project, refused from the other one.
	hit := sb.search("rotation", "--project", a, "--who", "user").Data.Hits[0]
	full := sb.ok("memory", "get", hit.ID[:12], "--project", a)
	if !strings.Contains(full, "said by: user") || !strings.Contains(full, "The deploy key rotation happens on the first Monday.") || !strings.Contains(full, "session: s-a") {
		t.Fatalf("memory get:\n%s", full)
	}
	if out := sb.ok("evidence", "get", hit.ID, "--project", a); !strings.Contains(out, "first Monday") {
		t.Fatalf("evidence get: %q", out)
	}
	if code, _, errOut := sb.run("memory", "get", hit.ID, "--project", b); code != ExitFailure || !strings.Contains(errOut, "no stored record") {
		t.Fatalf("reading alpha's record from beta: exit=%d %q", code, errOut)
	}
	sb.ok("memory", "get", hit.ID, "--all-projects")
}

func TestSearchPaginationAndBadInputThroughTheCommands(t *testing.T) {
	sb := newSandbox(t)
	a := sb.dir("alpha")
	sb.ok("project", "add", a, "--capture", "conversation")
	for i := 0; i < 23; i++ {
		sb.say("UserPromptSubmit", "s", a, fmt.Sprintf("paging item %d", i))
	}
	sb.ok("ingest")
	seen := map[string]int{}
	cursor, pages := "", 0
	for {
		args := []string{"paging item", "--project", a, "--limit", "5"}
		if cursor != "" {
			args = append(args, "--cursor", cursor)
		}
		res := sb.search(args...)
		pages++
		for _, h := range res.Data.Hits {
			seen[h.ID]++
		}
		if cursor = res.Data.NextCursor; cursor == "" || pages > 10 {
			break
		}
	}
	if pages != 5 || len(seen) != 23 {
		t.Fatalf("%d pages, %d distinct rows; want 5 and 23", pages, len(seen))
	}
	for id, n := range seen {
		if n != 1 {
			t.Fatalf("row %s returned %d times", id, n)
		}
	}
	for _, q := range []string{`"`, `(`, `NEAR(`, `a"b`, `'; drop table evidence; --`, `*`} {
		if code, _, errOut := sb.run("memory", "search", q, "--project", a); code != ExitOK {
			t.Errorf("query %q: exit=%d %q", q, code, errOut)
		}
	}
	for _, args := range [][]string{
		{"memory", "search", "x", "--project", a, "--limit", "0"},
		{"memory", "search", "x", "--project", a, "--limit", "many"},
		{"memory", "search", "x", "--project", a, "--since", "yesterday"},
		{"memory", "search", "x", "--project", a, "--who", "everyone"},
		{"memory", "search", "x", "--project", a, "--cursor", "garbage"},
		{"memory", "search", "x", "--project", a, "--all-projects"},
		{"memory", "search", "   ", "--project", a},
		{"memory", "search"}, {"memory", "frobnicate", "x"}, {"evidence", "search", "x"}, {"memory", "get"},
		{"memory", "get", "abcdefgh", "--project", a, "--limit", "3"},
	} {
		if code, _, _ := sb.run(args...); code != ExitUsage {
			t.Errorf("%v: exit=%d, want usage", args[1:], code)
		}
	}
	if code, _, errOut := sb.run("memory", "search", "x", "--project", sb.dir("unregistered")); code != ExitFailure || !strings.Contains(errOut, "not inside a registered project") {
		t.Fatalf("unregistered scope: exit=%d %q", code, errOut)
	}
	if out := sb.ok("status"); !strings.Contains(out, "23 event(s)") {
		t.Fatalf("hostile queries changed the data: %q", out)
	}
}

func TestSearchBeforeAnythingExists(t *testing.T) {
	sb := newSandbox(t)
	if code, _, errOut := sb.run("memory", "search", "x", "--all-projects"); code != ExitUnavailable || !strings.Contains(errOut, "sessionkeep setup") {
		t.Fatalf("no database yet: exit=%d %q", code, errOut)
	}
}

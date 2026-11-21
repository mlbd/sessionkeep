package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

type sessionRow struct {
	ID          string `json:"id"`
	ProjectID   string `json:"project_id"`
	ProjectName string `json:"project_name"`
	Texts       int    `json:"text_records"`
	FirstPrompt string `json:"first_prompt"`
}

type sessionShown struct {
	Session sessionRow `json:"session"`
	Turns   []struct {
		Who  string `json:"who"`
		Text string `json:"text"`
	} `json:"turns"`
}

func (sb *sandbox) sessions(args ...string) []sessionRow {
	sb.t.Helper()
	var out struct {
		Data []sessionRow `json:"data"`
	}
	raw := sb.ok(append([]string{"session", "list", "--json"}, args...)...)
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		sb.t.Fatalf("session list is not JSON: %v\n%s", err, raw)
	}
	return out.Data
}

func TestSessionListAndShowStayInsideTheProject(t *testing.T) {
	sb := newSandbox(t)
	a, b := sb.dir("alpha"), sb.dir("beta")
	sb.ok("project", "add", a, "--capture", "conversation", "--name", "Alpha")
	sb.ok("project", "add", b, "--capture", "conversation", "--name", "Beta")
	sb.say("UserPromptSubmit", "s-a", a, "Alpha   question\nabout invoices.")
	sb.say("Stop", "s-a", a, "Alpha answer.")
	sb.say("UserPromptSubmit", "s-b", b, "Beta secret plan PLATYPUS41.")
	sb.ok("ingest")

	alpha := sb.sessions("--project", a)
	if len(alpha) != 1 || alpha[0].ProjectName != "Alpha" || alpha[0].Texts != 2 {
		t.Fatalf("alpha sessions = %+v, want one session with 2 texts", alpha)
	}
	if alpha[0].FirstPrompt != "Alpha question about invoices." {
		t.Errorf("first prompt = %q, want the first user text on one line", alpha[0].FirstPrompt)
	}
	if all := sb.sessions("--all-projects"); len(all) != 2 {
		t.Fatalf("all-projects list has %d sessions, want 2", len(all))
	}
	beta := sb.sessions("--project", b)
	if len(beta) != 1 {
		t.Fatalf("beta sessions = %+v", beta)
	}

	var shown struct {
		Data sessionShown `json:"data"`
	}
	raw := sb.ok("session", "show", alpha[0].ID, "--project", a, "--json")
	if err := json.Unmarshal([]byte(raw), &shown); err != nil {
		t.Fatal(err)
	}
	if len(shown.Data.Turns) != 2 || shown.Data.Turns[0].Who != "user" || shown.Data.Turns[1].Text != "Alpha answer." {
		t.Fatalf("turns = %+v, want the user text then the answer", shown.Data.Turns)
	}
	// A short prefix of the id works too.
	if out := sb.ok("session", "show", alpha[0].ID[:8], "--project", a); !strings.Contains(out, "Alpha answer.") {
		t.Errorf("prefix lookup did not show the session:\n%s", out)
	}

	// The other project's session must not be readable through this project's scope.
	code, out, errOut := sb.run("session", "show", beta[0].ID, "--project", a)
	if code == ExitOK || strings.Contains(out+errOut, "PLATYPUS41") {
		t.Fatalf("beta session leaked through alpha scope (exit %d):\n%s%s", code, out, errOut)
	}
	// Wildcards and too-short ids never match.
	for _, id := range []string{"%", "________", beta[0].ID[:4]} {
		if code, out, _ := sb.run("session", "show", "--project", b, "--", id); code == ExitOK {
			t.Errorf("session show %q succeeded:\n%s", id, out)
		}
	}
	if code, _, _ := sb.run("session", "list", "--project", a, "--all-projects"); code != ExitUsage {
		t.Errorf("combining --project and --all-projects: exit %d, want usage error", code)
	}
}

func TestDashboardSessionsAPI(t *testing.T) {
	sb := newSandbox(t)
	a := sb.dir("alpha")
	sb.ok("project", "add", a, "--capture", "conversation", "--name", "Alpha")
	sb.say("UserPromptSubmit", "s-a", a, "First question.")
	sb.say("Stop", "s-a", a, "First answer.")
	sb.ok("ingest")
	b := signIn(t, sb)

	var list struct {
		Data []sessionRow `json:"data"`
	}
	if code := b.do("GET", "/api/sessions?all=1&limit=5", nil, nil, &list); code != http.StatusOK || len(list.Data) != 1 {
		t.Fatalf("GET /api/sessions: status %d, %+v", code, list.Data)
	}
	s := list.Data[0]
	var one struct {
		Data sessionShown `json:"data"`
	}
	if code := b.do("GET", "/api/sessions/"+s.ID+"?project="+s.ProjectID, nil, nil, &one); code != http.StatusOK || len(one.Data.Turns) != 2 {
		t.Fatalf("GET /api/sessions/{id}: status %d, %+v", code, one.Data)
	}
	if code := b.do("GET", "/api/sessions?all=1&limit=abc", nil, nil, nil); code != http.StatusBadRequest {
		t.Errorf("non-numeric limit: status %d, want 400", code)
	}
	if code := b.do("GET", "/api/sessions/00000000-nope?project="+s.ProjectID, nil, nil, nil); code == http.StatusOK {
		t.Error("an unknown session id returned 200")
	}
	stranger := &browser{t: t, base: b.base}
	if code := stranger.do("GET", "/api/sessions?all=1", nil, nil, nil); code != http.StatusUnauthorized {
		t.Errorf("unsigned request: status %d, want 401", code)
	}
}

// Every session can be listed, a page at a time, without one being skipped or shown twice.
func TestEverySessionCanBeListedInPages(t *testing.T) {
	sb := newSandbox(t)
	app := sb.dir("app")
	sb.ok("project", "add", app, "--capture", "conversation")
	for i := 0; i < 7; i++ {
		sb.say("UserPromptSubmit", fmt.Sprintf("s-%d", i), app, fmt.Sprintf("Question number %d.", i))
	}
	sb.ok("ingest")
	seen := map[string]bool{}
	for offset := 0; ; offset += 3 {
		page := sb.sessions("--project", app, "--limit", "3", "--offset", strconv.Itoa(offset))
		for _, s := range page {
			if seen[s.ID] {
				t.Fatalf("session %s listed twice", s.ID)
			}
			seen[s.ID] = true
		}
		if len(page) < 3 {
			break
		}
	}
	if len(seen) != 7 {
		t.Fatalf("paging listed %d sessions, want 7", len(seen))
	}
	if code, _, _ := sb.run("session", "list", "--project", app, "--offset", "-1"); code != ExitUsage {
		t.Fatalf("a negative offset was accepted (exit %d)", code)
	}
}

// What SessionKeep saved stays when Claude Code clears a conversation (/clear, also typed as /reset
// or /new) and when Claude Code's own session files are deleted afterwards.
func TestSavedSessionsOutliveAClearAndTheLossOfClaudeCodesFiles(t *testing.T) {
	sb := newSandbox(t)
	app := sb.dir("app")
	sb.ok("project", "add", app, "--capture", "conversation")
	from := sb.claudeFiles(fakeTurn{"old-1", "u1", app, "user", "Imported question IMPORTMARK."})
	if h := sb.history(ExitOK, "--from", from, "--yes"); h.Data.Stored != 1 {
		t.Fatalf("import stored %d pieces, want 1", h.Data.Stored)
	}

	sb.say("UserPromptSubmit", "live-1", app, "Live question KEEPMARK.")
	sb.say("Stop", "live-1", app, "Live answer.")
	// The conversation is cleared: the old session ends with that reason and a new one starts.
	for _, e := range []map[string]string{
		{"hook_event_name": "SessionEnd", "session_id": "live-1", "cwd": app, "reason": "clear"},
		{"hook_event_name": "SessionStart", "session_id": "live-2", "cwd": app, "source": "clear"},
	} {
		raw, _ := json.Marshal(e)
		if code, _, errOut := sb.hook(e["hook_event_name"], string(raw)); code != ExitOK || errOut != "" {
			t.Fatalf("hook %s: %d %q", e["hook_event_name"], code, errOut)
		}
	}
	sb.say("UserPromptSubmit", "live-2", app, "After the clear.")
	sb.ok("ingest")

	// Claude Code's own files disappear, and the import is run again against the empty folder.
	if err := os.RemoveAll(filepath.Join(from, "projects", "some-folder")); err != nil {
		t.Fatal(err)
	}
	sb.history(ExitOK, "--from", from, "--yes")

	if list := sb.sessions("--project", app); len(list) != 3 {
		t.Fatalf("%d sessions are listed after the clear and the loss of the files, want 3: %+v", len(list), list)
	}
	for _, word := range []string{"KEEPMARK", "IMPORTMARK"} {
		if hits := sb.search(word, "--project", app).Data.Hits; len(hits) != 1 {
			t.Fatalf("%s is found %d times after the clear, want once", word, len(hits))
		}
	}
}

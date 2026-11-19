package cli

import (
	"encoding/json"
	"net/http"
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

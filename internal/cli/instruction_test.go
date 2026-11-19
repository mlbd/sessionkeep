package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sessionkeep/internal/config"
)

// sessionStart runs the SessionStart hook for cwd and returns the context Claude would receive.
func (sb *sandbox) inject(event, session, cwd string) string {
	sb.t.Helper()
	raw, _ := json.Marshal(map[string]string{"session_id": session, "cwd": cwd, "hook_event_name": event, "source": "startup", "prompt": "hello"})
	code, out, errOut := sb.hook(event, string(raw))
	if code != ExitOK || errOut != "" {
		sb.t.Fatalf("hook %s: exit %d stderr %q", event, code, errOut)
	}
	if out == "" {
		return ""
	}
	var resp struct {
		SystemMessage      string `json:"systemMessage"`
		HookSpecificOutput struct {
			HookEventName     string `json:"hookEventName"`
			AdditionalContext string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		sb.t.Fatalf("hook output is not one JSON document: %v: %q", err, out)
	}
	if resp.HookSpecificOutput.AdditionalContext != "" && resp.HookSpecificOutput.HookEventName != event {
		sb.t.Fatalf("hookEventName %q does not match %q", resp.HookSpecificOutput.HookEventName, event)
	}
	return resp.HookSpecificOutput.AdditionalContext
}

func (sb *sandbox) remember(scope, text string, extra ...string) string {
	sb.t.Helper()
	var got struct{ Data struct{ ID string } }
	args := append([]string{"remember", text, "--scope", scope, "--yes", "--json"}, extra...)
	if err := json.Unmarshal([]byte(sb.ok(args...)), &got); err != nil || got.Data.ID == "" {
		sb.t.Fatalf("remember: %v", err)
	}
	return got.Data.ID
}

func TestRememberNeedsScopeAndConfirmation(t *testing.T) {
	sb := newSandbox(t)
	app := sb.dir("app")
	sb.ok("project", "add", app, "--capture", "conversation")

	if code, _, errOut := sb.run("remember", "Use tabs.", "--project", app); code != ExitNeedsConsent || !strings.Contains(errOut, "no default") {
		t.Fatalf("no scope: exit=%d stderr=%q", code, errOut)
	}
	code, out, errOut := sb.run("remember", "Use tabs.", "--scope", "project", "--project", app)
	if code != ExitNeedsConsent || !strings.Contains(out, "NOT SAVED YET") || !strings.Contains(out, "Use tabs.") || !strings.Contains(errOut, "--yes") {
		t.Fatalf("no --yes: exit=%d out=%q err=%q", code, out, errOut)
	}
	if out := sb.ok("instruction", "list"); !strings.Contains(out, "no instructions recorded") {
		t.Fatalf("the preview saved something: %q", out)
	}
	if ctx := sb.inject("SessionStart", "s-0", app); ctx != "" {
		t.Fatalf("nothing recorded, so nothing should be injected: %q", ctx)
	}

	for _, bad := range [][]string{
		{"remember", "  ", "--scope", "global", "--yes"},
		{"remember", strings.Repeat("x", MaxInstructionChars+1), "--scope", "global", "--yes"},
		{"remember", "x", "--scope", "everywhere", "--yes"},
		{"remember", "x", "--scope", "global", "--project", app, "--yes"},
		{"remember", "--scope", "global", "--yes"},
		{"remember", "a", "b", "--scope", "global", "--yes"},
	} {
		if code, _, _ := sb.run(bad...); code == ExitOK {
			t.Errorf("%v was accepted", bad[:2])
		}
	}
	if code, _, errOut := sb.run("remember", "deploy with password=Tr0ub4dor-synthetic", "--scope", "global", "--yes"); code != ExitFailure || !strings.Contains(errOut, "not saved") {
		t.Fatalf("secret-looking text: exit=%d stderr=%q", code, errOut)
	}
	if code, _, errOut := sb.run("remember", "x", "--scope", "project", "--project", sb.dir("unregistered"), "--yes"); code != ExitFailure || !strings.Contains(errOut, "not inside a registered project") {
		t.Fatalf("unregistered project: exit=%d stderr=%q", code, errOut)
	}
	if out := sb.ok("instruction", "list"); !strings.Contains(out, "no instructions recorded") {
		t.Fatalf("a rejected remember saved something: %q", out)
	}
}

// T05: twenty instructions recorded once are returned word for word to a brand-new session, in
// order, long after they were written.
func TestTwentyInstructionsAreRecalledVerbatimInANewSession(t *testing.T) {
	sb := newSandbox(t)
	app := sb.dir("app")
	sb.ok("project", "add", app, "--capture", "conversation")
	var texts []string
	for i := 1; i <= 20; i++ {
		text := fmt.Sprintf("Rule %02d: ", i)
		switch i % 4 {
		case 0:
			text += "multi-line\n  - keep indentation\n  - and blank lines\n\nexactly"
		case 1:
			text += "naïve café 日本語 🚀 — typographic “quotes”"
		case 2:
			text += `use "double" and 'single' quotes, a \ backslash, <tags> & ampersands`
		default:
			text += "run `go vet ./...` and the migration suite before tagging a release"
		}
		texts = append(texts, text)
		scope := "project"
		if i <= 3 {
			scope = "global"
		}
		sb.remember(scope, text, map[bool][]string{true: {"--project", app}, false: nil}[scope == "project"]...)
	}

	check := func(label, ctx string) {
		t.Helper()
		if !strings.Contains(ctx, "Active instructions: 20. Shown in full here: 20.") || strings.Contains(ctx, "NOT SHOWN") {
			t.Fatalf("%s: coverage line wrong:\n%.300s", label, ctx)
		}
		last := -1
		for i, text := range texts {
			at := strings.Index(ctx, text+"\n")
			if at < 0 {
				t.Fatalf("%s: instruction %d is not verbatim in the injected context", label, i+1)
			}
			if at < last {
				t.Fatalf("%s: instruction %d is out of order", label, i+1)
			}
			last = at
		}
	}
	check("same day", sb.inject("SessionStart", "session-day-0", filepath.Join(app, "src")))

	// Ninety days later: the cache and database files are old, nothing has run in between.
	old := time.Now().Add(-90 * 24 * time.Hour)
	filepath.Walk(sb.env[config.EnvDataDir], func(p string, _ os.FileInfo, _ error) error { os.Chtimes(p, old, old); return nil })
	check("day 90, new session", sb.inject("SessionStart", "session-day-90", app))

	// The database being unavailable does not matter to recall: the hook reads the cache only.
	db := filepath.Join(sb.env[config.EnvDataDir], "sessionkeep.db")
	if err := os.Rename(db, db+".moved"); err != nil {
		t.Fatal(err)
	}
	check("database missing", sb.inject("SessionStart", "session-no-db", app))
	os.Rename(db+".moved", db)
}

func TestSupersedeArchiveAndHistory(t *testing.T) {
	sb := newSandbox(t)
	app := sb.dir("app")
	sb.ok("project", "add", app, "--capture", "conversation")
	id := sb.remember("project", "Indent with tabs.", "--project", app)
	keep := sb.remember("project", "Never force-push to main.", "--project", app)

	code, out, _ := sb.run("instruction", "supersede", id[:8], "Indent with two spaces.", "--reason", "team decision")
	if code != ExitNeedsConsent || !strings.Contains(out, "Indent with tabs.") || !strings.Contains(out, "Indent with two spaces.") {
		t.Fatalf("supersede preview: exit=%d %q", code, out)
	}
	if ctx := sb.inject("SessionStart", "s-1", app); !strings.Contains(ctx, "Indent with tabs.") {
		t.Fatal("the preview must not change what sessions receive")
	}
	sb.ok("instruction", "supersede", id[:8], "Indent with two spaces.", "--reason", "team decision", "--yes")

	ctx := sb.inject("SessionStart", "s-2", app)
	if strings.Contains(ctx, "Indent with tabs.") || !strings.Contains(ctx, "Indent with two spaces.\n") || !strings.Contains(ctx, "revision 2") || !strings.Contains(ctx, "Never force-push to main.") {
		t.Fatalf("after supersession the old text must not be injected:\n%s", ctx)
	}
	hist := sb.ok("instruction", "show", id[:8])
	if !strings.Contains(hist, "revision 1 · superseded") || !strings.Contains(hist, "Indent with tabs.") || !strings.Contains(hist, "revision 2 · active") || !strings.Contains(hist, "team decision") {
		t.Fatalf("history must keep the old wording:\n%s", hist)
	}

	sb.ok("instruction", "archive", keep[:8], "--yes")
	if ctx := sb.inject("SessionStart", "s-3", app); strings.Contains(ctx, "force-push") || !strings.Contains(ctx, "Active instructions: 1.") {
		t.Fatalf("archived instruction still injected:\n%s", ctx)
	}
	if out := sb.ok("instruction", "list"); strings.Contains(out, "force-push") {
		t.Fatalf("list without --all shows archived: %q", out)
	}
	if out := sb.ok("instruction", "list", "--all"); !strings.Contains(out, "archived") || !strings.Contains(out, "force-push") {
		t.Fatalf("list --all: %q", out)
	}
	for _, args := range [][]string{
		{"instruction", "supersede", keep[:8], "new text", "--yes"},              // archived
		{"instruction", "archive", keep[:8], "--yes"},                            // already archived
		{"instruction", "supersede", id[:8], "Indent with two spaces.", "--yes"}, // identical
		{"instruction", "show", "zzzzzzzz"},
		{"instruction", "show", "ab"},
	} {
		if code, _, _ := sb.run(args...); code != ExitFailure {
			t.Errorf("%v: exit=%d, want failure", args, code)
		}
	}
	if code, _, _ := sb.run("remember", "Indent with two spaces.", "--scope", "project", "--project", app, "--yes"); code != ExitFailure {
		t.Error("recording the same active text twice in one scope must be refused")
	}
}

func TestScopeIsolationAndWhereRecallHappens(t *testing.T) {
	sb := newSandbox(t)
	a, b := sb.dir("a"), sb.dir("b")
	paused, unreg, excluded := sb.dir("paused"), sb.dir("unregistered"), sb.dir("client", "x")
	sb.ok("project", "add", a, "--capture", "conversation")
	sb.ok("project", "add", b, "--capture", "metadata")
	sb.ok("project", "add", paused, "--capture", "conversation")
	sb.ok("project", "add", excluded, "--capture", "conversation")
	sb.remember("global", "GLOBAL-RULE applies everywhere I work.")
	sb.remember("project", "A-ONLY-RULE for project a.", "--project", a)
	sb.remember("project", "PAUSED-RULE for the paused project.", "--project", paused)
	sb.ok("project", "pause", paused)
	sb.ok("exclude", "add", sb.dir("client"))

	if ctx := sb.inject("SessionStart", "s", a); !strings.Contains(ctx, "GLOBAL-RULE") || !strings.Contains(ctx, "A-ONLY-RULE") || strings.Contains(ctx, "PAUSED-RULE") {
		t.Fatalf("project a:\n%s", ctx)
	}
	if ctx := sb.inject("SessionStart", "s", b); !strings.Contains(ctx, "GLOBAL-RULE") || strings.Contains(ctx, "A-ONLY-RULE") {
		t.Fatalf("project b must get the global rule and never project a's:\n%s", ctx)
	}
	// Pausing stops capture, not recall.
	before := sb.spoolCount()
	if ctx := sb.inject("SessionStart", "s", paused); !strings.Contains(ctx, "PAUSED-RULE") || !strings.Contains(ctx, "GLOBAL-RULE") {
		t.Fatalf("paused project should still receive its instructions:\n%s", ctx)
	}
	if sb.spoolCount() != before {
		t.Fatal("a paused project captured an event")
	}
	// Unregistered: only the reminder. Excluded: nothing at all, not even global rules.
	if ctx := sb.inject("SessionStart", "s", unreg); ctx != "" {
		t.Fatalf("unregistered folder received instructions: %q", ctx)
	}
	raw, _ := json.Marshal(map[string]string{"session_id": "s", "cwd": excluded, "hook_event_name": "SessionStart"})
	if _, out, _ := sb.hook("SessionStart", string(raw)); out != "" {
		t.Fatalf("excluded folder received output: %q", out)
	}
}

func TestInstructionsAreNotRepeatedWithEveryPrompt(t *testing.T) {
	sb := newSandbox(t)
	app := sb.dir("app")
	sb.ok("project", "add", app, "--capture", "conversation")
	sb.remember("project", "First rule.", "--project", app)

	if ctx := sb.inject("SessionStart", "s-1", app); !strings.Contains(ctx, "First rule.") {
		t.Fatal("session start must inject")
	}
	if ctx := sb.inject("UserPromptSubmit", "s-1", app); ctx != "" {
		t.Fatalf("an unchanged set must not be repeated on the next prompt: %q", ctx)
	}
	sb.remember("project", "Second rule, added mid-session.", "--project", app)
	if ctx := sb.inject("UserPromptSubmit", "s-1", app); !strings.Contains(ctx, "Second rule, added mid-session.") || !strings.Contains(ctx, "First rule.") {
		t.Fatalf("a changed set must be sent with the next prompt:\n%s", ctx)
	}
	if ctx := sb.inject("UserPromptSubmit", "s-1", app); ctx != "" {
		t.Fatal("…and then not again")
	}
	// A session that never saw SessionStart (hook added mid-session) still gets them once.
	if ctx := sb.inject("UserPromptSubmit", "s-2", app); !strings.Contains(ctx, "First rule.") {
		t.Fatal("first prompt of an unseen session must inject")
	}
	// SessionStart always injects: it also fires after compaction and on resume.
	if ctx := sb.inject("SessionStart", "s-1", app); !strings.Contains(ctx, "Second rule") {
		t.Fatal("SessionStart must always inject")
	}
	// Other events never carry context.
	raw, _ := json.Marshal(map[string]string{"session_id": "s-3", "cwd": app, "hook_event_name": "Stop", "last_assistant_message": "ok"})
	if _, out, _ := sb.hook("Stop", string(raw)); out != "" {
		t.Fatalf("Stop returned output: %q", out)
	}
}

func TestDamagedInstructionCacheIsAnnouncedNotHidden(t *testing.T) {
	sb := newSandbox(t)
	app := sb.dir("app")
	sb.ok("project", "add", app, "--capture", "conversation")
	sb.remember("project", "Important rule.", "--project", app)
	files, _ := filepath.Glob(filepath.Join(sb.env[config.EnvDataDir], "cache", "instructions", "*.json"))
	for _, f := range files {
		os.WriteFile(f, []byte(`{"version":1,"instructions":[{"id":`), 0o600)
	}
	ctx := sb.inject("SessionStart", "s", app)
	if !strings.Contains(ctx, "could not be read") || strings.Contains(ctx, "Important rule.") {
		t.Fatalf("a damaged cache must be announced:\n%q", ctx)
	}
	if sb.spoolCount() == 0 {
		t.Fatal("capture should continue even when recall is unavailable")
	}
	// Any command that holds the lease rewrites the cache from the database.
	sb.ok("setup")
	if ctx := sb.inject("SessionStart", "s2", app); !strings.Contains(ctx, "Important rule.") {
		t.Fatalf("`sessionkeep setup` should repair the cache:\n%q", ctx)
	}
}

func TestOverBudgetSetThroughTheRealCommands(t *testing.T) {
	sb := newSandbox(t)
	app := sb.dir("app")
	sb.ok("project", "add", app, "--capture", "conversation")
	for i := 0; i < 6; i++ {
		sb.remember("project", fmt.Sprintf("LONG-%d ", i)+strings.Repeat("word ", 500), "--project", app)
	}
	ctx := sb.inject("SessionStart", "s", app)
	if len(ctx) > 9000 || !strings.Contains(ctx, "Active instructions: 6.") || !strings.Contains(ctx, "NOT SHOWN IN FULL") || !strings.Contains(ctx, "--- not shown:") {
		t.Fatalf("len=%d\n%.400s", len(ctx), ctx)
	}
}

package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// claudeFiles builds a synthetic Claude Code folder with one session file per entry.
type fakeTurn struct{ session, uuid, cwd, who, text string }

func (sb *sandbox) claudeFiles(turns ...fakeTurn) string {
	sb.t.Helper()
	root := sb.dir("claude-home")
	files := map[string][]string{}
	for i, x := range turns {
		line, _ := json.Marshal(map[string]any{
			"type": x.who, "isSidechain": false, "cwd": x.cwd, "sessionId": x.session, "uuid": x.uuid,
			"timestamp": fmt.Sprintf("2026-10-01T09:00:%02d.5Z", i),
			"message":   map[string]any{"role": x.who, "content": x.text},
		})
		files[x.session] = append(files[x.session], string(line))
	}
	dir := filepath.Join(root, "projects", "some-folder")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		sb.t.Fatal(err)
	}
	for session, lines := range files {
		body := `{"type":"mode","mode":"default","sessionId":"` + session + `"}` + "\n" + strings.Join(lines, "\n") + "\n"
		if err := os.WriteFile(filepath.Join(dir, session+".jsonl"), []byte(body), 0o600); err != nil {
			sb.t.Fatal(err)
		}
	}
	return root
}

type historyOut struct {
	Data struct {
		Imported     bool `json:"imported"`
		Turns        int  `json:"turns"`
		Sessions     int  `json:"sessions"`
		Stored       int  `json:"stored"`
		Already      int  `json:"already_imported"`
		Live         int  `json:"sessions_captured_live_skipped"`
		Unregistered int  `json:"turns_in_unregistered_folders"`
		Excluded     int  `json:"turns_in_excluded_folders"`
		PausedOrOff  int  `json:"turns_in_paused_or_text_free_projects"`
		Redactions   int  `json:"secrets_removed"`
		Folders      []struct {
			Name  string `json:"name"`
			Turns int    `json:"turns"`
		} `json:"unregistered_folders"`
	} `json:"data"`
}

func (sb *sandbox) history(wantExit int, args ...string) historyOut {
	sb.t.Helper()
	code, out, errOut := sb.run(append([]string{"history", "import", "--json"}, args...)...)
	if code != wantExit {
		sb.t.Fatalf("history import %v: exit %d, want %d\n%s%s", args, code, wantExit, out, errOut)
	}
	var h historyOut
	// A preview prints the report and then the "needs confirmation" answer; read the report.
	if err := json.NewDecoder(strings.NewReader(out)).Decode(&h); err != nil {
		sb.t.Fatalf("history import output is not JSON: %v\n%s", err, out)
	}
	return h
}

func TestHistoryImportRespectsEveryConsentRule(t *testing.T) {
	sb := newSandbox(t)
	app, quiet, paused, private, stranger := sb.dir("app"), sb.dir("quiet"), sb.dir("paused"), sb.dir("private"), sb.dir("stranger")
	sb.ok("project", "add", app, "--capture", "conversation", "--name", "App")
	sb.ok("project", "add", quiet, "--capture", "metadata", "--name", "Quiet")
	sb.ok("project", "add", paused, "--capture", "conversation", "--name", "Paused")
	sb.ok("project", "pause", paused)
	sb.ok("project", "add", private, "--capture", "conversation", "--name", "Private")
	sb.ok("exclude", "add", private)
	sb.ok("setup", "--auto-add", "auto", "--level", "conversation") // must not make history register folders

	// One session SessionKeep captured as it happened: importing it again would duplicate it.
	sb.say("UserPromptSubmit", "live-1", app, "Live question LIVEMARK.")
	sb.ok("ingest")

	from := sb.claudeFiles(
		fakeTurn{"old-1", "u1", app, "user", "How do refunds work? OLDMARK"},
		fakeTurn{"old-1", "a1", filepath.Join(app, "sub"), "assistant", "Refunds go through the ledger. password=hunter2hunter2"},
		fakeTurn{"live-1", "u2", app, "user", "Live question LIVEMARK."},
		fakeTurn{"old-2", "u3", quiet, "user", "QUIETMARK must stay out."},
		fakeTurn{"old-3", "u4", paused, "user", "PAUSEDMARK must stay out."},
		fakeTurn{"old-4", "u5", private, "user", "PRIVATEMARK must stay out."},
		fakeTurn{"old-5", "u6", stranger, "user", "STRANGERMARK must stay out."},
	)

	// Without --yes: a report, exit "needs consent", and nothing stored.
	pre := sb.history(ExitNeedsConsent, "--from", from)
	if pre.Data.Imported || pre.Data.Turns != 2 || pre.Data.Sessions != 1 || pre.Data.Stored != 0 {
		t.Fatalf("preview = %+v, want 2 importable turns in 1 session and nothing stored", pre.Data)
	}
	if pre.Data.Live != 1 || pre.Data.Unregistered != 1 || pre.Data.Excluded != 1 || pre.Data.PausedOrOff != 2 || pre.Data.Redactions != 1 {
		t.Errorf("preview counts = %+v", pre.Data)
	}
	if len(pre.Data.Folders) != 1 || !strings.HasSuffix(pre.Data.Folders[0].Name, "stranger") {
		t.Errorf("unregistered folders = %+v, want the stranger folder", pre.Data.Folders)
	}
	if hits := sb.search("OLDMARK", "--all-projects").Data.Hits; len(hits) != 0 {
		t.Fatalf("the preview stored text: %+v", hits)
	}

	done := sb.history(ExitOK, "--from", from, "--yes")
	if !done.Data.Imported || done.Data.Stored != 2 || done.Data.Already != 0 {
		t.Fatalf("import = %+v, want 2 stored", done.Data)
	}
	if hits := sb.search("OLDMARK", "--project", app).Data.Hits; len(hits) != 1 {
		t.Fatalf("imported prompt not searchable: %+v", hits)
	}
	if hits := sb.search("LIVEMARK", "--project", app).Data.Hits; len(hits) != 1 {
		t.Errorf("the live session was duplicated or lost: %d hit(s), want 1", len(hits))
	}
	for _, mark := range []string{"QUIETMARK", "PAUSEDMARK", "PRIVATEMARK", "STRANGERMARK", "hunter2hunter2"} {
		if hits := sb.search(mark, "--all-projects").Data.Hits; len(hits) != 0 {
			t.Errorf("%s was stored: %+v", mark, hits)
		}
	}
	// History must not register folders even with auto-add on.
	if _, _, proj := sb.which(stranger); proj != "" {
		t.Errorf("the stranger folder became a project (%s)", proj)
	}
	// The imported session reads in order, user first, and is closed.
	list := sb.sessions("--project", app)
	var old sessionRow
	for _, s := range list {
		if strings.Contains(s.FirstPrompt, "OLDMARK") {
			old = s
		}
	}
	if old.ID == "" || old.Texts != 2 {
		t.Fatalf("imported session missing from the list: %+v", list)
	}
	shown := sb.ok("session", "show", old.ID, "--project", app)
	if !strings.Contains(shown, "ended") || strings.Index(shown, "OLDMARK") > strings.Index(shown, "ledger") {
		t.Errorf("imported session is not closed or not in order:\n%s", shown)
	}

	// Running it again stores nothing new.
	again := sb.history(ExitOK, "--from", from, "--yes")
	if again.Data.Stored != 0 || again.Data.Already != 2 {
		t.Errorf("second import = %+v, want 0 new and 2 already there", again.Data)
	}
	if hits := sb.search("OLDMARK", "--project", app).Data.Hits; len(hits) != 1 {
		t.Errorf("second import duplicated text: %d hits", len(hits))
	}
}

func TestHistoryImportOneProjectAndBadSource(t *testing.T) {
	sb := newSandbox(t)
	a, b := sb.dir("alpha"), sb.dir("beta")
	sb.ok("project", "add", a, "--capture", "conversation", "--name", "Alpha")
	sb.ok("project", "add", b, "--capture", "conversation", "--name", "Beta")
	from := sb.claudeFiles(fakeTurn{"s-a", "u1", a, "user", "ALPHAOLD question."}, fakeTurn{"s-b", "u2", b, "user", "BETAOLD question."})

	one := sb.history(ExitOK, "--from", from, "--project", a, "--yes")
	if one.Data.Stored != 1 {
		t.Fatalf("import for one project stored %d, want 1", one.Data.Stored)
	}
	if hits := sb.search("BETAOLD", "--all-projects").Data.Hits; len(hits) != 0 {
		t.Errorf("the other project's history was imported: %+v", hits)
	}
	if code, _, errOut := sb.run("history", "import", "--from", sb.dir("empty")); code != ExitFailure || !strings.Contains(errOut, "no Claude Code session folder") {
		t.Errorf("missing source: exit %d, %s", code, errOut)
	}
	if code, _, _ := sb.run("history", "import", "extra"); code != ExitUsage {
		t.Errorf("extra argument: exit %d, want usage", code)
	}
}

func TestDashboardHistoryImportNeedsConfirmation(t *testing.T) {
	sb := newSandbox(t)
	a := sb.dir("alpha")
	sb.ok("project", "add", a, "--capture", "conversation", "--name", "Alpha")
	from := sb.claudeFiles(fakeTurn{"s-a", "u1", a, "user", "WEBOLD question."})
	sb.env["CLAUDE_CONFIG_DIR"] = from
	br := signIn(t, sb)

	var out historyOut
	if code := br.do("POST", "/api/history/import", map[string]any{}, nil, &out); code != http.StatusConflict || out.Data.Turns != 1 || out.Data.Imported {
		t.Fatalf("preview: status %d, %+v", code, out.Data)
	}
	if code := br.do("POST", "/api/history/import", map[string]any{"confirm": true}, nil, &out); code != http.StatusOK || out.Data.Stored != 1 {
		t.Fatalf("confirm: status %d, %+v", code, out.Data)
	}
	if hits := sb.search("WEBOLD", "--project", a).Data.Hits; len(hits) != 1 {
		t.Errorf("imported text not searchable: %+v", hits)
	}
}

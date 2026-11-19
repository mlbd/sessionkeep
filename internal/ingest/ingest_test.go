package ingest

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"sessionkeep/internal/capture"
	"sessionkeep/internal/config"
	"sessionkeep/internal/project"
	"sessionkeep/internal/store"
)

var ctx = context.Background()

// rig is a full data directory: settings, roots cache, database, and the real hook adapter
// writing real spool files. Decisions use Windows path rules so paths are stable on any host.
type rig struct {
	t        *testing.T
	paths    config.Paths
	settings config.Settings
	st       *store.Store
	ids      atomic.Int64
	clock    atomic.Int64
}

func newRig(t *testing.T, mode, autoLevel string) *rig {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "data é")
	p, err := config.Resolve(runtime.GOOS, func(k string) string {
		if k == config.EnvDataDir {
			return dir
		}
		return ""
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := config.Ensure(p); err != nil {
		t.Fatal(err)
	}
	s := config.DefaultSettings()
	s.DeviceID = "device-1"
	s.AutoAdd = config.AutoAdd{Mode: mode, Level: autoLevel}
	if err := config.SaveSettings(p, s); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(ctx, p.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	r := &rig{t: t, paths: p, settings: s, st: st}
	if err := r.deps().RewriteRootsCache(ctx); err != nil {
		t.Fatal(err)
	}
	return r
}

func (r *rig) id() string { return fmt.Sprintf("id-%06d", r.ids.Add(1)) }
func (r *rig) now() time.Time {
	return time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC).Add(time.Duration(r.clock.Add(1)) * time.Millisecond)
}

func (r *rig) deps() Deps {
	return Deps{Paths: r.paths, Settings: r.settings, Store: r.st, GOOS: "windows", Now: r.now, NewID: r.id}
}

func (r *rig) addProject(root, level string) string {
	r.t.Helper()
	p, err := r.st.AddProject(ctx, store.NewProject{ID: r.id(), RootID: r.id(), Name: "n", Root: root, Level: level, DeviceID: "device-1", AddedBy: "manual", Now: r.now()})
	if err != nil {
		r.t.Fatal(err)
	}
	if err := r.deps().RewriteRootsCache(ctx); err != nil {
		r.t.Fatal(err)
	}
	return p.ID
}

// hook runs the real hook adapter, as Claude Code would, for one event.
func (r *rig) hook(event, session, cwd string, fields map[string]any) capture.Result {
	body := map[string]any{"session_id": session, "cwd": cwd, "hook_event_name": event}
	for k, v := range fields {
		body[k] = v
	}
	raw, _ := json.Marshal(body)
	return capture.Handle(capture.Request{HookEvent: event, Raw: raw, Paths: r.paths, Env: project.Env{GOOS: "windows", Home: `C:\Users\dev`}, Now: r.now(), NewID: r.id})
}

func (r *rig) drain() Report {
	r.t.Helper()
	rep, err := Drain(ctx, r.deps())
	if err != nil {
		r.t.Fatalf("drain: %v (report %+v)", err, rep)
	}
	return rep
}

func (r *rig) spoolFiles() int {
	entries, _ := os.ReadDir(r.paths.SpoolDir)
	return len(entries)
}

// q runs a query against the database through a separate read-only connection.
func (r *rig) q(query string, args ...any) string {
	r.t.Helper()
	db, err := sql.Open("sqlite", "file:"+strings.ReplaceAll(filepath.ToSlash(r.paths.DBPath), "%", "%25")+"?mode=ro")
	if err != nil {
		r.t.Fatal(err)
	}
	defer db.Close()
	var out sql.NullString
	if err := db.QueryRow(query, args...).Scan(&out); err != nil {
		r.t.Fatalf("%s: %v", query, err)
	}
	return out.String
}

func TestHookToDatabaseEndToEnd(t *testing.T) {
	r := newRig(t, "ask", "")
	pid := r.addProject(`c:\work\app`, "conversation")
	r.hook("SessionStart", "s-1", `C:\Work\App`, map[string]any{"source": "startup"})
	r.hook("UserPromptSubmit", "s-1", `C:\Work\App`, map[string]any{"prompt": "Always run the migration suite before tagging a release.", "prompt_id": "p1"})
	r.hook("PostToolUse", "s-1", `C:\Work\App\src`, map[string]any{"tool_name": "Read", "tool_use_id": "t1", "tool_input": map[string]any{"file_path": `C:\Work\App\src\a.ts`}})
	r.hook("Stop", "s-1", `C:\Work\App`, map[string]any{"last_assistant_message": "Understood: migration suite first."})
	r.hook("SessionEnd", "s-1", `C:\Work\App`, map[string]any{"reason": "other"})

	rep := r.drain()
	if rep != (Report{Ingested: 5}) || r.spoolFiles() != 0 {
		t.Fatalf("report %+v, spool files %d", rep, r.spoolFiles())
	}
	if n := r.q(`select count(*) from events where project_id = ?`, pid); n != "5" {
		t.Fatalf("events=%s", n)
	}
	if n := r.q(`select count(*)||'/'||max(state) from sessions`); n != "1/ended" {
		t.Fatalf("sessions: %s, want one ended session", n)
	}
	if got := r.q(`select e.text from evidence e join events v on v.evidence_id = e.id where v.kind = 'user_prompt'`); got != "Always run the migration suite before tagging a release." {
		t.Fatalf("prompt not stored verbatim: %q", got)
	}
	if got := r.q(`select trust_level||'/'||origin from evidence where text like 'Understood%'`); got != "assistant/hook:Stop" {
		t.Fatalf("assistant evidence: %q", got)
	}
	// The text lives in evidence only; the event row keeps metadata.
	if p := r.q(`select safe_payload from events where kind = 'user_prompt'`); strings.Contains(p, "migration suite") || !strings.Contains(p, `"prompt_id":"p1"`) {
		t.Fatalf("event payload: %s", p)
	}
	if n := r.q(`select count(*) from evidence_fts f join evidence e on e.seq = f.rowid where evidence_fts match 'migration AND suite' and e.project_id = ?`, pid); n != "2" {
		t.Fatalf("full-text hits = %s, want 2", n)
	}
	if rep := r.drain(); rep != (Report{}) {
		t.Fatalf("second drain should find nothing: %+v", rep)
	}
}

func TestCrashBetweenCommitAndDeleteDoesNotDuplicate(t *testing.T) {
	r := newRig(t, "ask", "")
	r.addProject(`c:\work\app`, "conversation")
	for i := 0; i < 30; i++ {
		r.hook("UserPromptSubmit", "s-1", `C:\Work\App`, map[string]any{"prompt": fmt.Sprintf("prompt number %d", i)})
	}
	crash := errors.New("simulated crash")
	for round := 0; r.spoolFiles() > 0; round++ {
		if round > 40 {
			t.Fatal("drain did not converge")
		}
		d := r.deps()
		n := 0
		d.AfterCommit = func(string) error {
			if n++; n == 4 { // committed, file not yet deleted
				return crash
			}
			return nil
		}
		if _, err := Drain(ctx, d); err != nil && !errors.Is(err, crash) {
			t.Fatal(err)
		}
	}
	if got := r.q(`select count(*)||'/'||count(distinct event_key)||'/'||(select count(*) from evidence) from events`); got != "30/30/30" {
		t.Fatalf("events/distinct/evidence = %s, want 30/30/30", got)
	}
}

func TestAutoAddCreatesExactlyOneProject(t *testing.T) {
	r := newRig(t, "auto", "conversation")
	for i := 0; i < 50; i++ {
		res := r.hook("UserPromptSubmit", fmt.Sprintf("s-%d", i%3), `C:\Work\Fresh\pkg`, map[string]any{"prompt": fmt.Sprintf("note %d", i)})
		if res.Action != project.ActionAutoRegister {
			t.Fatalf("hook action %q", res.Action)
		}
	}
	rep := r.drain()
	if rep.Ingested != 50 || rep.ProjectsCreated != 1 {
		t.Fatalf("%+v", rep)
	}
	if got := r.q(`select count(*)||'/'||max(added_by)||'/'||max(capture_policy) from projects`); got != "1/auto/conversation" {
		t.Fatalf("projects: %s", got)
	}
	if got := r.q(`select count(*) from sessions`); got != "3" {
		t.Fatalf("sessions=%s, want 3", got)
	}
	// The roots cache now knows the folder, so the next hook captures directly.
	res := r.hook("UserPromptSubmit", "s-9", `C:\Work\Fresh\pkg\deeper`, map[string]any{"prompt": "after registration"})
	if res.Action != project.ActionCapture {
		t.Fatalf("after drain the hook should capture directly, got %q", res.Action)
	}
	if rep := r.drain(); rep.Ingested != 1 || rep.ProjectsCreated != 0 {
		t.Fatalf("%+v", rep)
	}
}

// A folder registered by hand while auto-add requests for it are still in the spool must not
// become a second project.
func TestAutoAddRequestForAnAlreadyRegisteredRoot(t *testing.T) {
	r := newRig(t, "auto", "metadata")
	r.hook("UserPromptSubmit", "s-1", `C:\Work\Fresh`, map[string]any{"prompt": "x"})
	pid := r.addProject(`c:\work\fresh`, "conversation")
	if rep := r.drain(); rep.Ingested != 1 || rep.ProjectsCreated != 0 {
		t.Fatalf("%+v", rep)
	}
	if got := r.q(`select count(*)||'/'||(select project_id from events) from projects`); got != "1/"+pid {
		t.Fatalf("got %s", got)
	}
}

func TestEventsForAFolderExcludedLaterAreDiscarded(t *testing.T) {
	r := newRig(t, "ask", "")
	r.addProject(`c:\clients\acme`, "conversation")
	r.addProject(`c:\work\app`, "conversation")
	r.hook("UserPromptSubmit", "s-1", `C:\Clients\Acme\tool`, map[string]any{"prompt": "CONFIDENTIAL-CLIENT-TEXT"})
	r.hook("UserPromptSubmit", "s-2", `C:\Work\App`, map[string]any{"prompt": "ordinary"})
	r.settings.Exclusions = []string{`c:\clients\acme`} // the user excludes it before the drain
	rep := r.drain()
	if rep.DiscardedExclude != 1 || rep.Ingested != 1 || r.spoolFiles() != 0 {
		t.Fatalf("%+v", rep)
	}
	if n := r.q(`select count(*) from evidence where text like '%CONFIDENTIAL%'`); n != "0" {
		t.Fatal("excluded text reached the database")
	}
	left, _ := filepath.Glob(filepath.Join(r.paths.DataDir, "*", "*"))
	for _, f := range left {
		if raw, _ := os.ReadFile(f); strings.Contains(string(raw), "CONFIDENTIAL-CLIENT-TEXT") {
			t.Fatalf("excluded text still on disk in %s", f)
		}
	}
}

func TestBadSpoolFilesAreQuarantinedWithoutBlockingTheRest(t *testing.T) {
	r := newRig(t, "ask", "")
	r.addProject(`c:\work\app`, "conversation")
	write := func(name, body string) { os.WriteFile(filepath.Join(r.paths.SpoolDir, name), []byte(body), 0o600) }
	write("00000000000000000001-garbage.evt", "{not json")
	write("00000000000000000002-wrongversion.evt", `{"version":9,"event_id":"x","source_key":"k","session_id":"s","kind":"user_prompt","device_id":"d","level":"conversation","project_id":"p"}`)
	write("00000000000000000003-orphan.evt", `{"version":1,"event_id":"o1","source_key":"hook:o1","session_id":"s","kind":"user_prompt","device_id":"device-1","level":"conversation","project_id":"no-such-project","agent":"claude-code","cwd":"C:\\Work\\App","observed_at":"2026-11-19T00:00:00Z","payload":{}}`)
	write("00000000000000000004-both.evt", `{"version":1,"event_id":"b1","source_key":"hook:b1","session_id":"s","kind":"user_prompt","device_id":"device-1","level":"conversation","project_id":"p","register":{"root":"c:\\x\\y","level":"conversation"},"cwd":"C:\\x\\y"}`)
	write(".00000000000000000005-half.evt.tmp", `{"version":1,`) // a hook that died mid-write, recent
	r.hook("UserPromptSubmit", "s-1", `C:\Work\App`, map[string]any{"prompt": "good event"})

	rep := r.drain()
	if rep.Quarantined != 4 || rep.Ingested != 1 {
		t.Fatalf("%+v", rep)
	}
	q, _ := os.ReadDir(filepath.Join(r.paths.DataDir, QuarantineDir))
	if len(q) != 4 {
		t.Fatalf("quarantine holds %d files, want 4", len(q))
	}
	if n := r.q(`select count(*) from capture_gaps where source = 'spool'`); n != "4" {
		t.Fatalf("gap rows = %s, want 4", n)
	}
	if r.spoolFiles() != 1 {
		t.Fatalf("a recent temp file must be left alone; spool has %d entries", r.spoolFiles())
	}
	// Once it is old, the next drain removes it.
	old := time.Now().Add(-2 * time.Hour)
	os.Chtimes(filepath.Join(r.paths.SpoolDir, ".00000000000000000005-half.evt.tmp"), old, old)
	d := r.deps()
	d.Now = time.Now
	if _, err := Drain(ctx, d); err != nil || r.spoolFiles() != 0 {
		t.Fatalf("stale temp file not removed: err=%v entries=%d", err, r.spoolFiles())
	}
}

func TestGapLogIsImportedOnce(t *testing.T) {
	r := newRig(t, "ask", "")
	r.addProject(`c:\work\app`, "conversation")
	// Make the spool unusable so the hook records a gap, then restore it.
	os.RemoveAll(r.paths.SpoolDir)
	os.WriteFile(r.paths.SpoolDir, []byte("x"), 0o600)
	if res := r.hook("UserPromptSubmit", "s-1", `C:\Work\App`, map[string]any{"prompt": "lost"}); res.Gap == "" {
		t.Fatalf("expected a gap, got %+v", res)
	}
	os.Remove(r.paths.SpoolDir)
	os.MkdirAll(r.paths.SpoolDir, 0o700)

	log := filepath.Join(r.paths.DataDir, capture.GapLogName)
	saved, _ := os.ReadFile(log)
	if rep := r.drain(); rep.GapsImported != 1 {
		t.Fatalf("%+v", rep)
	}
	if _, err := os.Stat(log); err == nil {
		t.Fatal("gap log should be consumed")
	}
	// A drain that crashed after committing but before deleting leaves the .importing file.
	os.WriteFile(log+".importing", saved, 0o600)
	r.drain()
	if n := r.q(`select count(*)||'/'||max(reason) from capture_gaps`); n != "1/spool_write_failed" {
		t.Fatalf("gap rows: %s, want exactly one", n)
	}
}

func TestSessionThatMovesBetweenProjects(t *testing.T) {
	r := newRig(t, "ask", "")
	a := r.addProject(`c:\work\a`, "conversation")
	b := r.addProject(`c:\work\b`, "conversation")
	r.hook("UserPromptSubmit", "same-session", `C:\Work\A`, map[string]any{"prompt": "in a"})
	r.hook("UserPromptSubmit", "same-session", `C:\Work\B`, map[string]any{"prompt": "in b"})
	if rep := r.drain(); rep.Ingested != 2 {
		t.Fatalf("%+v", rep)
	}
	if got := r.q(`select (select count(*) from events where project_id = ?)||'/'||(select count(*) from events where project_id = ?)||'/'||(select count(*) from sessions)`, a, b); got != "1/1/2" {
		t.Fatalf("got %s, want one event per project and two session rows", got)
	}
}

// Twenty writers across five projects while drains run: every event must arrive exactly once.
func TestConcurrentHooksAndDrains(t *testing.T) {
	r := newRig(t, "ask", "")
	for i := 0; i < 5; i++ {
		r.addProject(fmt.Sprintf(`c:\work\p%d`, i), "conversation")
	}
	const writers, each = 20, 25
	var wg sync.WaitGroup
	var failed atomic.Int64
	var reasons sync.Map
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < each; i++ {
				res := r.hook("UserPromptSubmit", fmt.Sprintf("s-%d", w), fmt.Sprintf(`C:\Work\P%d\src`, w%5), map[string]any{"prompt": fmt.Sprintf("w%d-i%d", w, i)})
				if res.SpoolFile == "" {
					failed.Add(1)
					reasons.Store(res.Action+"/"+res.Gap, true)
				}
			}
		}(w)
	}
	stop := make(chan struct{})
	drained := make(chan error, 1)
	go func() {
		for {
			if _, err := Drain(ctx, r.deps()); err != nil {
				drained <- err
				return
			}
			select {
			case <-stop:
				drained <- nil
				return
			default:
			}
		}
	}()
	wg.Wait()
	close(stop)
	if err := <-drained; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 50 && r.spoolFiles() > 0; i++ {
		r.drain()
	}
	if failed.Load() != 0 {
		var why []string
		reasons.Range(func(k, _ any) bool { why = append(why, k.(string)); return true })
		t.Fatalf("%d hook writes failed: %v", failed.Load(), why)
	}
	want := fmt.Sprintf("%d/%d/5/20", writers*each, writers*each)
	if got := r.q(`select count(*)||'/'||count(distinct event_key)||'/'||count(distinct project_id)||'/'||(select count(*) from sessions) from events`); got != want {
		t.Fatalf("events/distinct/projects/sessions = %s, want %s", got, want)
	}
	if r.spoolFiles() != 0 {
		t.Fatalf("%d files left in the spool", r.spoolFiles())
	}
}

// A spool file held open by another program (as antivirus does with newly written files) must not
// fail the drain: the other files are stored, the busy one waits for the next run.
func TestBusySpoolFileIsDeferredNotFatal(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("exclusive opens that block readers are a Windows behaviour")
	}
	r := newRig(t, "ask", "")
	r.addProject(`c:\work\app`, "conversation")
	first := r.hook("UserPromptSubmit", "s-1", `C:\Work\App`, map[string]any{"prompt": "held by a scanner"})
	r.hook("UserPromptSubmit", "s-1", `C:\Work\App`, map[string]any{"prompt": "free"})

	held, err := lockExclusive(first.SpoolFile)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := Drain(ctx, r.deps())
	if err != nil {
		t.Fatalf("a busy file failed the drain: %v", err)
	}
	if rep.Ingested != 1 || rep.Deferred != 1 || r.spoolFiles() != 1 {
		t.Fatalf("with one file held: %+v, %d file(s) left", rep, r.spoolFiles())
	}
	held()
	if rep := r.drain(); rep.Ingested != 1 || rep.Deferred != 0 || r.spoolFiles() != 0 {
		t.Fatalf("after release: %+v", rep)
	}
	if got := r.q(`select count(*) from events`); got != "2" {
		t.Fatalf("events=%s, want 2", got)
	}
}

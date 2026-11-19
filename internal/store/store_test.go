package store

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

var ctx = context.Background()

func openTemp(t *testing.T) (*Store, string) {
	t.Helper()
	// A directory name with a space, non-ASCII characters and URI-special characters.
	dir := filepath.Join(t.TempDir(), "data dir é 開発 100% #1")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "sessionkeep.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s, path
}

func mustExec(t *testing.T, s *Store, q string, args ...any) {
	t.Helper()
	if _, err := s.db.ExecContext(ctx, q, args...); err != nil {
		t.Fatalf("setup statement failed: %v\n%s", err, q)
	}
}

func wantFail(t *testing.T, s *Store, contains, q string, args ...any) {
	t.Helper()
	_, err := s.db.ExecContext(ctx, q, args...)
	if err == nil {
		t.Fatalf("statement must be rejected but succeeded:\n%s", q)
	}
	if !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(contains)) {
		t.Fatalf("rejected for the wrong reason: got %q, want it to mention %q", err, contains)
	}
}

// seed creates two projects, each with one session.
func seed(t *testing.T, s *Store) {
	t.Helper()
	for _, p := range []string{"A", "B"} {
		mustExec(t, s, `insert into projects(id,name,capture_policy,created_at) values(?,?, 'conversation','2026-11-19T00:00:00Z')`, "proj-"+p, "Project "+p)
		mustExec(t, s, `insert into sessions(id,project_id,agent,external_session_id,device_id,started_at,adapter_version)
			values(?,?, 'claude-code', ?, 'dev-1', '2026-11-19T00:00:00Z', 1)`, "sess-"+p, "proj-"+p, "ext-"+p)
	}
}

const insEvidence = `insert into evidence(id,project_id,origin,source_locator,observed_at,text,completeness,trust_level)
	values(?,?, 'hook:UserPromptSubmit', ?, '2026-11-19T00:00:00Z', ?, 'complete', 'user')`
const insEvent = `insert into events(id,project_id,session_id,event_key,kind,observed_at,received_at,payload_version,safe_payload,evidence_id,redaction_status)
	values(?,?,?,?, 'user_prompt', '2026-11-19T00:00:00Z', '2026-11-19T00:00:01Z', 1, '{}', ?, 'none')`

func TestCheckEngine(t *testing.T) {
	tests := []struct {
		e  EngineInfo
		ok bool
	}{
		{EngineInfo{"3.54.1", true}, true},
		{EngineInfo{"3.51.3", true}, true},
		{EngineInfo{"4.0.0", true}, true},
		{EngineInfo{"3.51.2", true}, false},
		{EngineInfo{"3.45.1", true}, false},
		{EngineInfo{"3.50.7", true}, false}, // a fixed backport, deliberately still refused (see MinSQLite)
		{EngineInfo{"3.54.1", false}, false},
		{EngineInfo{"", true}, false},
		{EngineInfo{"3.54", true}, false},
		{EngineInfo{"three.fifty.one", true}, false},
	}
	for _, tc := range tests {
		err := CheckEngine(tc.e)
		if (err == nil) != tc.ok {
			t.Errorf("%+v: err=%v, want ok=%v", tc.e, err, tc.ok)
		}
		if err != nil && !errors.Is(err, ErrIncompatibleEngine) {
			t.Errorf("%+v: error must wrap ErrIncompatibleEngine, got %v", tc.e, err)
		}
	}
}

func TestLinkedEngineIsAcceptable(t *testing.T) {
	e, err := ProbeEngine(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("linked SQLite %s, FTS5=%v", e.Version, e.FTS5)
	if err := CheckEngine(e); err != nil {
		t.Fatalf("the SQLite linked into this binary fails the gate: %v", err)
	}
}

func TestOpenMigratesFromEmptyAndSetsPragmas(t *testing.T) {
	s, _ := openTemp(t)
	var n int
	var mode string
	var fk, sync int
	s.db.QueryRow("select count(*) from schema_migrations").Scan(&n)
	s.db.QueryRow("pragma journal_mode").Scan(&mode)
	s.db.QueryRow("pragma foreign_keys").Scan(&fk)
	s.db.QueryRow("pragma synchronous").Scan(&sync)
	if n != 4 || mode != "wal" || fk != 1 || sync != 2 {
		t.Fatalf("migrations=%d journal_mode=%s foreign_keys=%d synchronous=%d; want 4, wal, 1, 2(FULL)", n, mode, fk, sync)
	}
	for _, table := range []string{"projects", "project_roots", "sessions", "blobs", "evidence", "events", "capture_gaps", "instructions", "instruction_versions", "jobs", "evidence_fts"} {
		var name string
		if err := s.db.QueryRow("select name from sqlite_master where name=?", table).Scan(&name); err != nil {
			t.Errorf("table %s missing: %v", table, err)
		}
	}
	var ic string
	s.db.QueryRow("pragma integrity_check").Scan(&ic)
	var fkViolations int
	s.db.QueryRow("select count(*) from pragma_foreign_key_check").Scan(&fkViolations)
	if ic != "ok" || fkViolations != 0 {
		t.Fatalf("integrity_check=%s foreign_key_check rows=%d", ic, fkViolations)
	}
}

func TestReopenIsNoOp(t *testing.T) {
	s, path := openTemp(t)
	var before string
	s.db.QueryRow("select applied_at||checksum from schema_migrations where version=1").Scan(&before)
	s.Close()

	db, err := sql.Open("sqlite", dsn(path))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	applied, err := migrate(ctx, db, embedded, "migrations")
	if err != nil || applied != 0 {
		t.Fatalf("second migrate: applied=%d err=%v; want 0, nil", applied, err)
	}
	var after string
	db.QueryRow("select applied_at||checksum from schema_migrations where version=1").Scan(&after)
	if before != after {
		t.Fatalf("bookkeeping row changed on re-run: %q -> %q", before, after)
	}
}

func TestOpenRefusesChangedMigration(t *testing.T) {
	s, path := openTemp(t)
	mustExec(t, s, "update schema_migrations set checksum='tampered' where version=1")
	s.Close()
	if _, err := Open(ctx, path); !errors.Is(err, ErrMigrationChanged) {
		t.Fatalf("want ErrMigrationChanged, got %v", err)
	}
}

func TestOpenRefusesNewerSchema(t *testing.T) {
	s, path := openTemp(t)
	mustExec(t, s, "insert into schema_migrations values(9999,'future','x','2030-01-01T00:00:00Z')")
	s.Close()
	if _, err := Open(ctx, path); !errors.Is(err, ErrSchemaTooNew) {
		t.Fatalf("want ErrSchemaTooNew, got %v", err)
	}
}

func TestFailedMigrationLeavesNoPartialSchema(t *testing.T) {
	fsys := fstest.MapFS{
		"m/0001_ok.sql":     {Data: []byte("create table one(id integer primary key);")},
		"m/0002_broken.sql": {Data: []byte("create table two(id integer primary key);\r\ncreate table three(id integer primary key);\r\ninsert into nowhere values(1);")},
	}
	path := filepath.Join(t.TempDir(), "x.db")
	if _, err := open(ctx, path, fsys, "m"); err == nil || !strings.Contains(err.Error(), "0002_broken") {
		t.Fatalf("want an error naming 0002_broken, got %v", err)
	}
	db, _ := sql.Open("sqlite", dsn(path))
	defer db.Close()
	var tables, versions int
	db.QueryRow("select count(*) from sqlite_master where name in ('two','three')").Scan(&tables)
	db.QueryRow("select count(*) from schema_migrations").Scan(&versions)
	if tables != 0 || versions != 1 {
		t.Fatalf("partial migration visible: tables two/three=%d, recorded versions=%d; want 0 and 1", tables, versions)
	}
}

func TestMigrationNumberingIsValidated(t *testing.T) {
	for name, fsys := range map[string]fstest.MapFS{
		"gap":      {"m/0001_a.sql": {Data: []byte("select 1;")}, "m/0003_c.sql": {Data: []byte("select 1;")}},
		"bad name": {"m/init.sql": {Data: []byte("select 1;")}},
	} {
		if _, err := loadMigrations(fsys, "m"); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}

func TestChecksumIgnoresLineEndings(t *testing.T) {
	lf, _ := loadMigrations(fstest.MapFS{"m/0001_a.sql": {Data: []byte("create table a(x);\ncreate table b(x);\n")}}, "m")
	crlf, _ := loadMigrations(fstest.MapFS{"m/0001_a.sql": {Data: []byte("create table a(x);\r\ncreate table b(x);\r\n")}}, "m")
	if lf[0].checksum != crlf[0].checksum {
		t.Fatal("CRLF checkout must not change a migration checksum")
	}
}

// The gate must stop an old engine before the database is switched to WAL.
func TestOpenRefusesOldEngineBeforeEnablingWAL(t *testing.T) {
	orig := probeEngine
	probeEngine = func(context.Context, *sql.DB) (EngineInfo, error) {
		return EngineInfo{Version: "3.45.1", FTS5: true}, nil
	}
	t.Cleanup(func() { probeEngine = orig })

	path := filepath.Join(t.TempDir(), "old.db")
	if _, err := Open(ctx, path); !errors.Is(err, ErrIncompatibleEngine) {
		t.Fatalf("want ErrIncompatibleEngine, got %v", err)
	}
	probeEngine = orig
	if _, err := os.Stat(path + "-wal"); err == nil {
		t.Fatal("a -wal file exists: WAL was enabled despite the failed gate")
	}
	db, _ := sql.Open("sqlite", "file:"+filepath.ToSlash(path))
	defer db.Close()
	var mode string
	var tables int
	db.QueryRow("pragma journal_mode").Scan(&mode)
	db.QueryRow("select count(*) from sqlite_master").Scan(&tables)
	if mode == "wal" || tables != 0 {
		t.Fatalf("database was modified despite the failed gate: journal_mode=%s tables=%d", mode, tables)
	}
}

func TestForeignKeysFailClosed(t *testing.T) {
	s, _ := openTemp(t)
	seed(t, s)
	wantFail(t, s, "foreign key", `insert into sessions(id,project_id,agent,external_session_id,device_id,started_at,adapter_version)
		values('s-x','no-such-project','claude-code','e','d','2026-11-19T00:00:00Z',1)`)
	wantFail(t, s, "foreign key", insEvent, "ev-1", "proj-A", "no-such-session", "k-1", nil)
}

// A row in project B must not be able to point at a parent that belongs to project A.
func TestCrossProjectReferencesAreRejected(t *testing.T) {
	s, _ := openTemp(t)
	seed(t, s)
	mustExec(t, s, insEvidence, "evi-A", "proj-A", "loc-A", "tenant isolation note for project A")

	wantFail(t, s, "foreign key", insEvent, "ev-1", "proj-B", "sess-A", "k-1", nil)     // B event in A's session
	wantFail(t, s, "foreign key", insEvent, "ev-2", "proj-B", "sess-B", "k-2", "evi-A") // B event citing A's evidence
	mustExec(t, s, insEvent, "ev-3", "proj-A", "sess-A", "k-3", "evi-A")                // the legitimate row works

	mustExec(t, s, `insert into blobs(id,project_id,digest,size,media_type,storage_ref,redaction_version,created_at)
		values('blob-A','proj-A','d1',1,'text/plain','ref',1,'2026-11-19T00:00:00Z')`)
	wantFail(t, s, "foreign key", `insert into evidence(id,project_id,origin,source_locator,observed_at,blob_id,completeness,trust_level)
		values('evi-B','proj-B','hook:Stop','loc','2026-11-19T00:00:00Z','blob-A','complete','assistant')`)
}

func TestEventKeyReplayIsIdempotent(t *testing.T) {
	s, _ := openTemp(t)
	seed(t, s)
	mustExec(t, s, insEvent, "ev-1", "proj-A", "sess-A", "claude:dev-1:ext-A:msg-1", nil)
	wantFail(t, s, "unique", insEvent, "ev-2", "proj-A", "sess-A", "claude:dev-1:ext-A:msg-1", nil)
	res, err := s.db.Exec(strings.Replace(insEvent, "insert into", "insert or ignore into", 1), "ev-3", "proj-A", "sess-A", "claude:dev-1:ext-A:msg-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := res.RowsAffected(); n != 0 {
		t.Fatalf("replay inserted %d rows, want 0", n)
	}
	var count int
	s.db.QueryRow("select count(*) from events").Scan(&count)
	if count != 1 {
		t.Fatalf("events=%d, want 1", count)
	}
}

func TestEvidenceAndEventsAreImmutable(t *testing.T) {
	s, _ := openTemp(t)
	seed(t, s)
	mustExec(t, s, insEvidence, "evi-1", "proj-A", "loc", "original wording")
	mustExec(t, s, insEvent, "ev-1", "proj-A", "sess-A", "k-1", "evi-1")
	wantFail(t, s, "immutable", "update evidence set text='rewritten' where id='evi-1'")
	wantFail(t, s, "immutable", "update evidence set project_id='proj-B' where id='evi-1'")
	wantFail(t, s, "immutable", "update events set safe_payload='{\"x\":1}' where id='ev-1'")
	wantFail(t, s, "", "insert into events(id,project_id,session_id,event_key,kind,observed_at,received_at,payload_version,safe_payload,redaction_status) values('ev-bad','proj-A','sess-A','k-bad','k','t','t',1,'not json','none')")
}

func ftsIDs(t *testing.T, s *Store, project, match string) []string {
	t.Helper()
	rows, err := s.db.Query(`select e.id from evidence_fts f join evidence e on e.seq = f.rowid
		where evidence_fts match ? and e.project_id = ? order by e.id`, match, project)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		rows.Scan(&id)
		ids = append(ids, id)
	}
	return ids
}

func TestFullTextSearchIsScopedAndFollowsDeletes(t *testing.T) {
	s, _ := openTemp(t)
	seed(t, s)
	mustExec(t, s, insEvidence, "evi-A1", "proj-A", "l1", "Always run the migration suite before tagging; error ECONNRESET seen in auth_service.ts")
	mustExec(t, s, insEvidence, "evi-A2", "proj-A", "l2", "Unrelated note about fonts")
	mustExec(t, s, insEvidence, "evi-B1", "proj-B", "l3", "Project B also mentions the migration suite and ECONNRESET")

	if got := ftsIDs(t, s, "proj-A", "migration AND suite"); len(got) != 1 || got[0] != "evi-A1" {
		t.Fatalf("project A search returned %v, want [evi-A1]", got)
	}
	if got := ftsIDs(t, s, "proj-B", "ECONNRESET"); len(got) != 1 || got[0] != "evi-B1" {
		t.Fatalf("project B search returned %v, want [evi-B1]", got)
	}
	if got := ftsIDs(t, s, "proj-A", `"auth_service.ts"`); len(got) != 1 {
		t.Fatalf("quoted identifier lookup returned %v, want one hit", got)
	}
	if got := ftsIDs(t, s, "proj-A", "nonexistentterm"); len(got) != 0 {
		t.Fatalf("search for an absent term returned %v", got)
	}

	mustExec(t, s, "delete from evidence where id='evi-A1'")
	if got := ftsIDs(t, s, "proj-A", "ECONNRESET"); len(got) != 0 {
		t.Fatalf("deleted evidence still searchable: %v", got)
	}
	var raw int
	s.db.QueryRow("select count(*) from evidence_fts where evidence_fts match 'ECONNRESET'").Scan(&raw)
	if raw != 1 {
		t.Fatalf("index holds %d ECONNRESET rows after delete, want 1 (project B only)", raw)
	}
	var ic string
	if err := s.db.QueryRow("insert into evidence_fts(evidence_fts, rank) values('integrity-check', 1) returning 'ok'").Scan(&ic); err != nil && !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("FTS index inconsistent with evidence table: %v", err)
	}
}

func TestDeletingAProjectRemovesItsRowsOnly(t *testing.T) {
	s, _ := openTemp(t)
	seed(t, s)
	mustExec(t, s, insEvidence, "evi-A", "proj-A", "l", "alpha secretword")
	mustExec(t, s, insEvent, "ev-A", "proj-A", "sess-A", "k-A", "evi-A")
	mustExec(t, s, insEvidence, "evi-B", "proj-B", "l", "beta secretword")
	wantFail(t, s, "foreign key", "delete from evidence where id='evi-A'") // still cited by an event
	mustExec(t, s, "delete from projects where id='proj-A'")
	var sessions, events, evidence int
	s.db.QueryRow("select count(*) from sessions").Scan(&sessions)
	s.db.QueryRow("select count(*) from events").Scan(&events)
	s.db.QueryRow("select count(*) from evidence").Scan(&evidence)
	if sessions != 1 || events != 0 || evidence != 1 {
		t.Fatalf("after deleting project A: sessions=%d events=%d evidence=%d; want 1, 0, 1", sessions, events, evidence)
	}
	if got := ftsIDs(t, s, "proj-B", "secretword"); len(got) != 1 {
		t.Fatalf("project B lost its evidence: %v", got)
	}
	var raw int
	s.db.QueryRow("select count(*) from evidence_fts where evidence_fts match 'alpha'").Scan(&raw)
	if raw != 0 {
		t.Fatal("deleted project's text is still in the full-text index")
	}
}

func TestInstructionScopeAndVersionRules(t *testing.T) {
	s, _ := openTemp(t)
	seed(t, s)
	const ins = `insert into instructions(id,scope_kind,project_id,scope_ref,state,confirmation_status,created_at) values(?,?,?,?,?,?, '2026-11-19T00:00:00Z')`
	const ver = `insert into instruction_versions(id,instruction_id,revision,text,text_digest,status,supersedes_id,author,effective_from,created_at)
		values(?,?,?,?, 'digest', ?, ?, 'owner', '2026-11-19T00:00:00Z', '2026-11-19T00:00:00Z')`

	wantFail(t, s, "check", ins, "i-bad1", "global", "proj-A", nil, "draft", "suggested")   // global must not carry a project
	wantFail(t, s, "check", ins, "i-bad2", "project", nil, nil, "draft", "suggested")       // a missing project is not global
	wantFail(t, s, "check", ins, "i-bad3", "branch", "proj-A", nil, "draft", "suggested")   // branch scope needs its ref
	wantFail(t, s, "check", ins, "i-bad4", "project", "proj-A", nil, "active", "suggested") // unconfirmed cannot be active
	wantFail(t, s, "foreign key", ins, "i-bad5", "project", "no-such", nil, "draft", "suggested")
	mustExec(t, s, ins, "i-1", "project", "proj-A", nil, "active", "confirmed")
	mustExec(t, s, ins, "i-g", "global", nil, nil, "active", "confirmed")

	mustExec(t, s, ver, "v-1", "i-1", 1, "Use tabs, never spaces.", "active", nil)
	wantFail(t, s, "unique", ver, "v-2", "i-1", 2, "Use spaces.", "active", "v-1") // two active versions
	wantFail(t, s, "immutable", "update instruction_versions set text='Use spaces.' where id='v-1'")
	wantFail(t, s, "check", ver, "v-empty", "i-1", 3, "", "draft", nil)

	// Supersession: old version changes status, new one becomes active, old text is kept verbatim.
	tx, _ := s.db.Begin()
	tx.Exec("update instruction_versions set status='superseded' where id='v-1'")
	if _, err := tx.Exec(ver, "v-2", "i-1", 2, "Use spaces.", "active", "v-1"); err != nil {
		t.Fatalf("supersession failed: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	var active, old string
	s.db.QueryRow("select text from instruction_versions where instruction_id='i-1' and status='active'").Scan(&active)
	s.db.QueryRow("select text from instruction_versions where id='v-1'").Scan(&old)
	if active != "Use spaces." || old != "Use tabs, never spaces." {
		t.Fatalf("active=%q old=%q", active, old)
	}
}

func TestJobIdempotencyAndStates(t *testing.T) {
	s, _ := openTemp(t)
	const job = `insert into jobs(id,kind,state,idempotency_key,created_at,updated_at) values(?, 'ingest', ?, ?, 't','t')`
	mustExec(t, s, job, "j-1", "queued", "key-1")
	wantFail(t, s, "unique", job, "j-2", "queued", "key-1")
	wantFail(t, s, "check", job, "j-3", "done-ish", "key-3")
}

package cli

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"sessionkeep/internal/archive"
	"sessionkeep/internal/config"
	"sessionkeep/internal/ingest"
)

// onDisk reports whether token appears, in any letter case, in any file under dir.
func onDisk(t *testing.T, dir, token string) []string {
	t.Helper()
	var where []string
	filepath.Walk(dir, func(p string, fi os.FileInfo, err error) error {
		if err == nil && !fi.IsDir() {
			raw, _ := os.ReadFile(p)
			if bytes.Contains(bytes.ToLower(raw), []byte(strings.ToLower(token))) {
				where = append(where, filepath.Base(p))
			}
		}
		return nil
	})
	return where
}

func populated(t *testing.T) (*sandbox, string, string) {
	sb := newSandbox(t)
	a, b := sb.dir("alpha"), sb.dir("beta")
	sb.ok("project", "add", a, "--capture", "conversation", "--name", "Alpha")
	sb.ok("project", "add", b, "--capture", "conversation", "--name", "Beta")
	sb.say("UserPromptSubmit", "s-a", a, "Alpha keeps invoices for seven years. Marker ZEBRAQUOKKA77.")
	sb.say("Stop", "s-a", a, "Noted: seven years retention.")
	sb.say("UserPromptSubmit", "s-a", a, "Second alpha note about retention.")
	sb.say("UserPromptSubmit", "s-b", b, "Beta note that must survive.")
	sb.ok("ingest")
	sb.remember("project", "Alpha rule: ISO dates only.", "--project", a)
	sb.remember("global", "Global rule: be precise.")
	sb.ok("checkpoint", "Alpha checkpoint: export done.", "--project", a)
	return sb, a, b
}

func TestExportThenImportIntoAFreshProfile(t *testing.T) {
	sb, a, _ := populated(t)
	file := filepath.Join(sb.work, "backup one.sessionkeep")

	// Capture keeps running while the export is taken.
	var wg sync.WaitGroup
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < 15; i++ {
				sb.say("UserPromptSubmit", fmt.Sprintf("live-%d", w), a, fmt.Sprintf("written during export %d-%d", w, i))
			}
		}(w)
	}
	out := sb.ok("export", file)
	wg.Wait()
	if !strings.Contains(out, "not encrypted") || !strings.Contains(out, "2 project(s)") {
		t.Fatalf("export output: %q", out)
	}
	if code, _, errOut := sb.run("export", file); code != ExitFailure || !strings.Contains(errOut, "already exists") {
		t.Fatalf("exports must never be overwritten: exit=%d %q", code, errOut)
	}

	// Dry run against a brand-new, non-existent data directory: verifies and changes nothing.
	fresh := &sandbox{t: t, work: sb.work, env: map[string]string{config.EnvDataDir: filepath.Join(t.TempDir(), "restored profile"), "HOME": sb.env["HOME"], "USERPROFILE": sb.env["USERPROFILE"]}}
	dry := fresh.ok("import", file, "--dry-run")
	// The preview names what the export changes once restored (L3).
	for _, want := range []string{"folders that would be captured (2)", "Alpha rule: ISO dates only.", "settings that would be restored: unregistered folders:", "automatic summaries off", "Import only an export you made yourself"} {
		if !strings.Contains(dry, want) {
			t.Errorf("the import preview lacks %q:\n%s", want, dry)
		}
	}
	if !strings.Contains(dry, "DRY RUN: this import would succeed") || !strings.Contains(dry, "integrity check passed") {
		t.Fatalf("dry run: %q", dry)
	}
	if _, err := os.Stat(fresh.env[config.EnvDataDir]); err == nil {
		t.Fatal("a dry run created the data directory")
	}

	var rep struct {
		Data struct {
			Restored bool
			Totals   struct {
				Projects, Events, Instructions, Checkpoints int
				TextRecords                                 int `json:"text_records"`
			}
		}
	}
	if err := json.Unmarshal([]byte(fresh.ok("import", file, "--json")), &rep); err != nil || !rep.Data.Restored {
		t.Fatalf("import: %+v %v", rep.Data, err)
	}
	if rep.Data.Totals.Projects != 2 || rep.Data.Totals.Instructions != 2 || rep.Data.Totals.Checkpoints != 1 || rep.Data.Totals.Events < 4 || rep.Data.Totals.TextRecords < 4 {
		t.Fatalf("restored totals: %+v", rep.Data.Totals)
	}
	// The restored profile works: search, instructions into a session, resume, capture decision.
	if res := fresh.search("invoices seven years", "--project", a); len(res.Data.Hits) != 1 {
		t.Fatalf("search in the restored profile: %+v", res.Data)
	}
	if ctx := fresh.inject("SessionStart", "s-new", a); !strings.Contains(ctx, "Alpha rule: ISO dates only.") || !strings.Contains(ctx, "Global rule: be precise.") {
		t.Fatalf("instructions in the restored profile:\n%s", ctx)
	}
	if pack := fresh.ok("resume", "--project", a); !strings.Contains(pack, "Alpha checkpoint: export done.") {
		t.Fatalf("checkpoint in the restored profile:\n%s", pack)
	}
	if act, level, _ := fresh.which(a); act != "capture" || level != "conversation" {
		t.Fatalf("capture decision in the restored profile: %s %s", act, level)
	}
	// Everything captured before the export is in it; events written during it are in the
	// export or still in the source spool, never lost.
	var src struct {
		Data struct {
			Stored       struct{ Events int }
			SpoolPending int `json:"spool_pending"`
		}
	}
	json.Unmarshal([]byte(sb.ok("status", "--json")), &src)
	if src.Data.Stored.Events+src.Data.SpoolPending != 4+60 {
		t.Fatalf("source after export: %d stored + %d waiting, want 64 in total", src.Data.Stored.Events, src.Data.SpoolPending)
	}

	// A second import into the now-populated profile is refused and changes nothing.
	before := fresh.ok("status")
	if code, out, _ := fresh.run("import", file); code != ExitFailure || !strings.Contains(out, "already has a database") {
		t.Fatalf("import over existing data: exit=%d %q", code, out)
	}
	if fresh.ok("status") != before {
		t.Fatal("a refused import changed the target")
	}
}

func TestImportRejectsDamagedAndHostileFiles(t *testing.T) {
	sb, _, _ := populated(t)
	good := filepath.Join(sb.work, "good.sessionkeep")
	sb.ok("export", good)
	raw, _ := os.ReadFile(good)
	target := func() *sandbox {
		return &sandbox{t: t, work: sb.work, env: map[string]string{config.EnvDataDir: filepath.Join(t.TempDir(), "target"), "HOME": sb.env["HOME"], "USERPROFILE": sb.env["USERPROFILE"]}}
	}
	rezip := func(edit func(name string, body []byte) (string, []byte, bool), extra map[string]string) string {
		zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
		if err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		for _, f := range zr.File {
			rc, _ := f.Open()
			body, _ := io.ReadAll(rc)
			rc.Close()
			name, body, keep := edit(f.Name, body)
			if !keep {
				continue
			}
			w, _ := zw.Create(name)
			w.Write(body)
		}
		for name, body := range extra {
			w, _ := zw.Create(name)
			w.Write([]byte(body))
		}
		zw.Close()
		p := filepath.Join(t.TempDir(), "edited.sessionkeep")
		os.WriteFile(p, buf.Bytes(), 0o600)
		return p
	}
	same := func(n string, b []byte) (string, []byte, bool) { return n, b, true }
	// An export someone made by hand: correct digests and counts, but its database carries a
	// trigger that would run on every stored event after the restore (L3).
	planted := func() string {
		dir := t.TempDir()
		man, err := archive.Extract(good, dir)
		if err != nil {
			t.Fatal(err)
		}
		db, err := sql.Open("sqlite", filepath.Join(dir, archive.DBName))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`create trigger planted after insert on events begin update events set payload = payload; end`); err != nil {
			t.Fatal(err)
		}
		db.Close()
		p := filepath.Join(t.TempDir(), "planted.sessionkeep")
		if _, err := archive.Write(p, filepath.Join(dir, archive.DBName), filepath.Join(dir, archive.SettingsName), man); err != nil {
			t.Fatal(err)
		}
		return p
	}()
	if code, out, errOut := target().run("import", planted, "--dry-run"); code == ExitOK || !strings.Contains(out+errOut, "an extra trigger planted") {
		t.Errorf("an export with a planted trigger: exit %d %q", code, out+errOut)
	}
	cases := map[string]string{
		"planted trigger": planted,
		"not a zip":       func() string { p := filepath.Join(t.TempDir(), "x"); os.WriteFile(p, []byte("hello"), 0o600); return p }(),
		"missing file":    filepath.Join(t.TempDir(), "nope.sessionkeep"),
		"database altered": rezip(func(n string, b []byte) (string, []byte, bool) {
			if n == "sessionkeep.db" {
				b = append([]byte(nil), b...)
				b[len(b)/2] ^= 0xFF
			}
			return n, b, true
		}, nil),
		"database truncated": rezip(func(n string, b []byte) (string, []byte, bool) {
			if n == "sessionkeep.db" {
				b = b[:len(b)/2]
			}
			return n, b, true
		}, nil),
		"no manifest":          rezip(func(n string, b []byte) (string, []byte, bool) { return n, b, n != "manifest.json" }, nil),
		"no database":          rezip(func(n string, b []byte) (string, []byte, bool) { return n, b, n != "sessionkeep.db" }, nil),
		"extra entry":          rezip(same, map[string]string{"notes.txt": "x"}),
		"path traversal entry": rezip(same, map[string]string{"../../escaped.txt": "x"}),
		"absolute path entry":  rezip(same, map[string]string{"/escaped.txt": "x"}),
		"unknown format": rezip(func(n string, b []byte) (string, []byte, bool) {
			if n == "manifest.json" {
				edited := bytes.Replace(b, []byte(`"format": 1`), []byte(`"format": 99`), 1)
				if bytes.Equal(edited, b) {
					t.Fatal("test setup: could not alter the format in the manifest")
				}
				b = edited
			}
			return n, b, true
		}, nil),
		"counts lie": rezip(func(n string, b []byte) (string, []byte, bool) {
			if n == "manifest.json" {
				edited := bytes.Replace(b, []byte(`"projects": 2`), []byte(`"projects": 9`), 1)
				if bytes.Equal(edited, b) {
					t.Fatalf("test setup: could not alter the counts in the manifest: %s", b)
				}
				b = edited
			}
			return n, b, true
		}, nil),
	}
	for name, file := range cases {
		tg := target()
		for _, args := range [][]string{{"import", file, "--dry-run"}, {"import", file}} {
			code, out, errOut := tg.run(args...)
			if code == ExitOK || strings.Contains(out, "RESTORED") || strings.Contains(out, "would succeed") {
				t.Errorf("%s: %v was accepted: %q", name, args[2:], out+errOut)
			}
		}
		if _, err := os.Stat(filepath.Join(tg.env[config.EnvDataDir], "sessionkeep.db")); err == nil {
			t.Errorf("%s: a rejected import left a database behind", name)
		}
	}
	if matches, _ := filepath.Glob(filepath.Join(filepath.Dir(sb.work), "escaped.txt")); len(matches) != 0 {
		t.Fatal("an archive entry escaped the extraction directory")
	}
}

func TestForgetRecordRemovesItEverywhere(t *testing.T) {
	sb, a, b := populated(t)
	data := sb.env[config.EnvDataDir]
	hit := sb.search("ZEBRAQUOKKA77", "--project", a).Data.Hits[0].ID
	if len(onDisk(t, data, "ZEBRAQUOKKA77")) == 0 {
		t.Fatal("test setup: the marker is not on disk")
	}

	// Another project's scope cannot delete it; without --yes nothing is deleted.
	if code, _, _ := sb.run("forget", "record", hit, "--project", b, "--yes"); code != ExitFailure {
		t.Fatalf("deleting alpha's record from beta's scope: exit=%d", code)
	}
	code, out, errOut := sb.run("forget", "record", hit[:12], "--project", a)
	if code != ExitNeedsConsent || !strings.Contains(out, "NOT DELETED YET") || !strings.Contains(out, "ZEBRAQUOKKA77") || !strings.Contains(out, "exports you made earlier") || !strings.Contains(errOut, "--yes") {
		t.Fatalf("preview: exit=%d %q", code, out)
	}
	if res := sb.search("ZEBRAQUOKKA77", "--project", a); len(res.Data.Hits) != 1 {
		t.Fatal("the preview deleted the record")
	}

	done := sb.ok("forget", "record", hit, "--project", a, "--yes")
	if !strings.Contains(done, "deleted: 1 event(s), 1 text record(s)") {
		t.Fatalf("forget output: %q", done)
	}
	if res := sb.search("ZEBRAQUOKKA77", "--project", a, "--exact"); len(res.Data.Hits) != 0 {
		t.Fatal("deleted record still found by search")
	}
	if code, _, _ := sb.run("memory", "get", hit, "--project", a); code != ExitFailure {
		t.Fatal("deleted record still readable by id")
	}
	if where := onDisk(t, data, "ZEBRAQUOKKA77"); len(where) != 0 {
		t.Fatalf("deleted text is still on disk in: %v", where)
	}
	file := filepath.Join(sb.work, "after.sessionkeep")
	sb.ok("export", file)
	if raw, _ := os.ReadFile(file); bytes.Contains(bytes.ToLower(raw), []byte("zebraquokka77")) {
		t.Fatal("deleted text is in a later export")
	}
	// Everything else is untouched.
	if res := sb.search("retention", "--project", a); len(res.Data.Hits) != 2 {
		t.Fatalf("other alpha records: %+v", res.Data.Hits)
	}
	if out := sb.ok("status"); !strings.Contains(out, "3 event(s), 3 text record(s)") {
		t.Fatalf("status after forgetting one record: %q", out)
	}
	if code, _, _ := sb.run("forget", "record", hit, "--project", a, "--yes"); code != ExitFailure {
		t.Fatal("forgetting an already deleted record should say it was not found")
	}
}

func TestForgetCapturedKeepsInstructionsAndCheckpoints(t *testing.T) {
	sb, a, b := populated(t)
	sb.say("UserPromptSubmit", "s-a", a, "still waiting in the spool QUEUEDMARKER55")
	sb.say("UserPromptSubmit", "s-b", b, "beta waiting in the spool")
	// A quarantined copy of each project's waiting event, as ingest leaves one it cannot store (L5).
	data := sb.env[config.EnvDataDir]
	spool, _ := os.ReadDir(filepath.Join(data, "spool"))
	os.MkdirAll(filepath.Join(data, ingest.QuarantineDir), 0o700)
	for _, e := range spool {
		raw, _ := os.ReadFile(filepath.Join(data, "spool", e.Name()))
		raw = bytes.ReplaceAll(raw, []byte("QUEUEDMARKER55"), []byte("QUARMARKER66"))
		raw = bytes.ReplaceAll(raw, []byte("beta waiting"), []byte("QUARBETA77"))
		os.WriteFile(filepath.Join(data, ingest.QuarantineDir, "q-"+e.Name()), raw, 0o600)
	}
	if len(onDisk(t, data, "QUARMARKER66")) == 0 || len(onDisk(t, data, "QUARBETA77")) == 0 {
		t.Fatal("the test did not put both quarantined files in place")
	}
	if code, out, _ := sb.run("forget", "captured", "--project", a); code != ExitNeedsConsent || !strings.Contains(out, "3 event(s), 3 text record(s), 1 session(s)") {
		t.Fatalf("preview: exit=%d %q", code, out)
	}
	sb.ok("forget", "captured", "--project", a, "--yes")
	sb.ok("ingest")
	if res := sb.search("retention invoices", "--project", a); len(res.Data.Hits) != 0 {
		t.Fatalf("captured text still searchable: %+v", res.Data.Hits)
	}
	if where := onDisk(t, sb.env[config.EnvDataDir], "QUEUEDMARKER55"); len(where) != 0 {
		t.Fatalf("an event of the project that was waiting in the spool survived: %v", where)
	}
	if where := onDisk(t, data, "QUARMARKER66"); len(where) != 0 {
		t.Fatalf("a quarantined event of the project survived: %v", where)
	}
	if len(onDisk(t, data, "QUARBETA77")) == 0 {
		t.Fatal("the other project's quarantined event was deleted")
	}
	if ctx := sb.inject("SessionStart", "s-x", a); !strings.Contains(ctx, "Alpha rule: ISO dates only.") {
		t.Fatal("instructions must survive forgetting captured text")
	}
	if pack := sb.ok("resume", "--project", a); !strings.Contains(pack, "Alpha checkpoint: export done.") {
		t.Fatal("checkpoints must survive forgetting captured text")
	}
	if res := sb.search("beta", "--project", b); len(res.Data.Hits) != 2 {
		t.Fatalf("the other project lost data: %+v", res.Data.Hits)
	}
}

func TestForgetProjectRemovesEverythingOfThatProjectOnly(t *testing.T) {
	sb, a, b := populated(t)
	data := sb.env[config.EnvDataDir]
	code, out, _ := sb.run("forget", "project", a)
	if code != ExitNeedsConsent || !strings.Contains(out, "1 instruction(s), 1 checkpoint(s), 1 project(s)") {
		t.Fatalf("preview: exit=%d %q", code, out)
	}
	sb.ok("forget", "project", a, "--yes")

	if act, _, _ := sb.which(a); act != "remind" {
		t.Fatalf("the folder is still treated as a registered project: %s", act)
	}
	if ctx := sb.inject("SessionStart", "s-y", a); ctx != "" {
		t.Fatalf("a deleted project's folder still receives instructions: %q", ctx)
	}
	for _, token := range []string{"ZEBRAQUOKKA77", "Alpha rule: ISO dates only", "Alpha checkpoint: export done", "seven years"} {
		if where := onDisk(t, data, token); len(where) != 0 {
			t.Errorf("%q is still on disk in %v", token, where)
		}
	}
	list := sb.ok("project", "list")
	if strings.Contains(list, "Alpha") || !strings.Contains(strings.ToLower(list), "beta") {
		t.Fatalf("project list: %q", list)
	}
	if res := sb.search("survive", "--project", b); len(res.Data.Hits) != 1 {
		t.Fatal("the other project lost data")
	}
	if ctx := sb.inject("SessionStart", "s-z", b); !strings.Contains(ctx, "Global rule: be precise.") {
		t.Fatal("global instructions must survive deleting a project")
	}
	for _, bad := range [][]string{{"forget"}, {"forget", "everything", "--yes"}, {"forget", "captured", "--yes"}, {"forget", "record"}, {"forget", "project", b, "--project", b}} {
		if code, _, _ := sb.run(bad...); code != ExitUsage {
			t.Errorf("%v: exit=%d, want usage", bad, code)
		}
	}
	if code, _, _ := sb.run("forget", "project", sb.dir("unregistered"), "--yes"); code != ExitFailure {
		t.Error("forgetting an unregistered folder must fail")
	}
}

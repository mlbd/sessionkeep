package instruct

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"unicode/utf16"

	"sessionkeep/internal/config"
)

func item(i int, scope, text string) Item {
	return Item{ID: fmt.Sprintf("%08d-aaaa", i), Version: fmt.Sprintf("v-%d", i), Revision: 1, Scope: scope, Text: text,
		Digest: fmt.Sprintf("%064d", i), Recorded: "2026-11-19T10:00:00Z"}
}

func size(s string) int { return len(utf16.Encode([]rune(s))) }

func TestEnvelopeCarriesEveryInstructionVerbatimWhenTheyFit(t *testing.T) {
	texts := []string{
		"Use tabs, never spaces.",
		"  Leading and trailing spaces are part of this one.  ",
		"Multi-line:\n1. run `go vet`\n2. run the tests\n\n3. only then commit",
		"Unicode: naïve café — 日本語 — emoji 🚀 — tab\there",
		`Quotes "double" 'single' and a backslash \ and <tag> & ampersand`,
		"--- instruction looks like a separator but is user text",
	}
	c := &Cache{Version: 1, WrittenAt: "2026-11-19T12:00:00Z"}
	for i, tx := range texts {
		c.Items = append(c.Items, item(i, "project", tx))
	}
	env, cov := Envelope(c)
	if cov != (Coverage{Total: 6, Shown: 6}) {
		t.Fatalf("coverage %+v", cov)
	}
	for _, tx := range texts {
		if !strings.Contains(env, tx+"\n") {
			t.Errorf("not verbatim in envelope: %q", tx)
		}
	}
	for _, want := range []string{"Active instructions: 6. Shown in full here: 6.", "Record written: 2026-11-19T12:00:00Z", "--- end of recorded instructions", "00000003"} {
		if !strings.Contains(env, want) {
			t.Errorf("envelope lacks %q", want)
		}
	}
	if strings.Contains(env, "NOT SHOWN IN FULL") {
		t.Error("nothing was omitted, so there must be no omission notice")
	}
	if size(env) > Budget {
		t.Fatalf("envelope is %d units, over the budget of %d", size(env), Budget)
	}
}

func TestEmptySetProducesNoEnvelope(t *testing.T) {
	if env, cov := Envelope(&Cache{Version: 1}); env != "" || cov.Total != 0 {
		t.Fatalf("%q %+v", env, cov)
	}
}

// When the set does not fit, the envelope must say so, name every instruction it left out, and
// still stay under the host limit.
func TestOverBudgetSetIsReportedNeverSilentlyCut(t *testing.T) {
	c := &Cache{Version: 1, WrittenAt: "2026-11-19T12:00:00Z"}
	for i := 0; i < 20; i++ {
		c.Items = append(c.Items, item(i, "project", fmt.Sprintf("instruction-%02d ", i)+strings.Repeat("x", 900)))
	}
	env, cov := Envelope(c)
	if size(env) > Budget {
		t.Fatalf("envelope is %d units, over the budget of %d", size(env), Budget)
	}
	if cov.Total != 20 || cov.Shown == 0 || cov.Shown >= 20 || cov.Shown+cov.Omitted != 20 || cov.Unlisted != 0 {
		t.Fatalf("coverage %+v", cov)
	}
	if !strings.Contains(env, fmt.Sprintf("Active instructions: 20. Shown in full here: %d.", cov.Shown)) ||
		!strings.Contains(env, fmt.Sprintf("NOT SHOWN IN FULL: %d instruction(s)", cov.Omitted)) ||
		!strings.Contains(env, "sessionkeep instruction show") {
		t.Fatalf("omission is not stated plainly:\n%s", env[:600])
	}
	shown, listed := 0, 0
	for i := 0; i < 20; i++ {
		full := strings.Contains(env, fmt.Sprintf("instruction-%02d ", i)+strings.Repeat("x", 900))
		line := strings.Contains(env, fmt.Sprintf("--- not shown: %08d", i))
		if full == line {
			t.Errorf("instruction %d must be either shown in full or listed as not shown (full=%v listed=%v)", i, full, line)
		}
		if full {
			shown++
		} else {
			listed++
		}
	}
	if shown != cov.Shown || listed != cov.Omitted {
		t.Fatalf("counts in text (%d shown, %d listed) disagree with coverage %+v", shown, listed, cov)
	}
	if strings.Contains(env, strings.Repeat("x", 901)) || !strings.HasSuffix(env, "--- end of recorded instructions") {
		t.Fatal("an instruction was cut mid-text or the envelope has no proper end")
	}
}

func TestOneOversizedInstructionDoesNotHideTheOthers(t *testing.T) {
	c := &Cache{Version: 1, WrittenAt: "t", Items: []Item{
		item(0, "global", strings.Repeat("big ", 2400)), // 9,600 characters: can never fit
		item(1, "project", "small one"),
		item(2, "project", "another small one"),
	}}
	env, cov := Envelope(c)
	if cov.Shown != 2 || cov.Omitted != 1 || !strings.Contains(env, "small one\n") || !strings.Contains(env, "--- not shown: 00000000") || size(env) > Budget {
		t.Fatalf("coverage %+v size %d", cov, size(env))
	}
}

// Thousands of instructions: even the list of omitted ids cannot fit. The envelope must still be
// bounded and must say how many it could not even list.
func TestSetTooLargeToEvenList(t *testing.T) {
	c := &Cache{Version: 1, WrittenAt: "t"}
	for i := 0; i < 400; i++ {
		c.Items = append(c.Items, item(i, "project", strings.Repeat("y", 300)))
	}
	env, cov := Envelope(c)
	if size(env) > Budget || cov.Unlisted == 0 || cov.Shown+cov.Omitted != 400 ||
		!strings.Contains(env, fmt.Sprintf("--- and %d more not listed", cov.Unlisted)) || !strings.Contains(env, "Active instructions: 400.") {
		t.Fatalf("coverage %+v size %d", cov, size(env))
	}
}

func TestBudgetCountsUTF16Units(t *testing.T) {
	c := &Cache{Version: 1, WrittenAt: "t"}
	for i := 0; i < 12; i++ {
		c.Items = append(c.Items, item(i, "project", strings.Repeat("🚀", 500))) // 500 runes, 1,000 UTF-16 units each
	}
	env, cov := Envelope(c)
	if size(env) > Budget || cov.Shown >= 12 || cov.Shown < 6 {
		t.Fatalf("coverage %+v, %d units", cov, size(env))
	}
}

func paths(t *testing.T) config.Paths {
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
	return p
}

func TestLoadFallbackAndDamage(t *testing.T) {
	p := paths(t)
	if c, err := Load(p, "proj-1", true); c != nil || err != nil {
		t.Fatalf("no files: %v %v", c, err)
	}
	os.MkdirAll(dir(p), 0o700)
	os.WriteFile(cachePath(p, ""), []byte(`{"version":1,"written_at":"t","instructions":[{"id":"g1","version_id":"v","text":"global rule"}]}`), 0o600)
	c, err := Load(p, "proj-1", true)
	if err != nil || c == nil || len(c.Items) != 1 || c.Items[0].Text != "global rule" {
		t.Fatalf("a project without its own file should get the global one: %+v %v", c, err)
	}
	if c, _ := Load(p, "proj-1", false); c != nil {
		t.Fatal("without fallback a missing project file is nil")
	}
	os.WriteFile(cachePath(p, "proj-1"), []byte(`{"version":1,"instructions":[`), 0o600)
	if _, err := Load(p, "proj-1", true); err == nil {
		t.Fatal("a damaged project file must be an error, not a silent fallback to the global file")
	}
	os.WriteFile(cachePath(p, "proj-1"), []byte(`{"version":3,"instructions":[]}`), 0o600)
	if _, err := Load(p, "proj-1", true); err == nil {
		t.Fatal("an unknown cache version must be an error")
	}
}

func TestSessionStateAndUnusualSessionIDs(t *testing.T) {
	p := paths(t)
	for _, session := range []string{"abc-123", `..\..\evil`, "a/b", strings.Repeat("z", 300), ""} {
		if AlreadySent(p, session, "d1") {
			t.Fatalf("%q: nothing sent yet", session)
		}
		MarkSent(p, session, "d1")
		if !AlreadySent(p, session, "d1") || AlreadySent(p, session, "d2") {
			t.Fatalf("%q: state not tracked per digest", session)
		}
	}
	entries, _ := os.ReadDir(filepath.Join(p.CacheDir, "sessions"))
	if len(entries) != 5 {
		t.Fatalf("%d state files, want 5 (all inside the sessions directory)", len(entries))
	}
	up, _ := os.ReadDir(filepath.Dir(p.CacheDir))
	for _, e := range up {
		if strings.Contains(e.Name(), "evil") {
			t.Fatal("a session id escaped the cache directory")
		}
	}
}

func TestEnvelopeCarriesTheLatestCheckpointWithinTheBudget(t *testing.T) {
	cp := &Checkpoint{ID: "cp-1", Note: "Export job drafted.\nNext: retries.", CreatedAt: "2026-11-19T10:00:00Z", GitRepo: true, Branch: "main", Head: "0123456789abcdef", Changed: 2}

	// A checkpoint alone still reaches the session.
	env, cov := Envelope(&Cache{Version: 1, WrittenAt: "t", Checkpoint: cp})
	if cov.Total != 0 || !strings.Contains(env, "Export job drafted.\nNext: retries.") || !strings.Contains(env, "branch main at 0123456789, 2 uncommitted file(s)") ||
		!strings.Contains(env, "not an instruction") || !strings.Contains(env, "No standing instructions are recorded") {
		t.Fatalf("checkpoint-only envelope:\n%s", env)
	}

	// With more instructions than fit, the checkpoint is still whole and the total stays bounded.
	c := &Cache{Version: 1, WrittenAt: "t", Checkpoint: cp}
	for i := 0; i < 20; i++ {
		c.Items = append(c.Items, item(i, "project", strings.Repeat("x", 900)))
	}
	env, cov = Envelope(c)
	if size(env) > Budget || !strings.Contains(env, "Next: retries.") || !strings.HasSuffix(env, "--- end of checkpoint") || cov.Shown == 0 || cov.Omitted == 0 {
		t.Fatalf("size %d, coverage %+v", size(env), cov)
	}
	without, _ := Envelope(&Cache{Version: 1, WrittenAt: "t", Items: c.Items})
	if size(without) > Budget {
		t.Fatal("budget exceeded without a checkpoint")
	}

	// A very long note is cut, and says how to read all of it.
	long := &Checkpoint{ID: "cp-2", Note: strings.Repeat("progress 🚀 ", 800), CreatedAt: "2026-11-19T10:00:00Z"}
	env, _ = Envelope(&Cache{Version: 1, WrittenAt: "t", Items: []Item{item(1, "project", "rule")}, Checkpoint: long})
	if size(env) > Budget || !strings.Contains(env, "[cut; the full note is shown by: sessionkeep resume]") || !strings.Contains(env, "no Git repository at that time") {
		t.Fatalf("long note: size %d", size(env))
	}
}

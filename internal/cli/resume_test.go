package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"unicode/utf16"

	"sessionkeep/internal/config"
)

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "-c", "core.autocrlf=false"}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// treeDigest hashes every file under dir, including the whole .git directory.
func treeDigest(t *testing.T, dir string) string {
	t.Helper()
	var lines []string
	filepath.Walk(dir, func(p string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() {
			return nil
		}
		raw, _ := os.ReadFile(p)
		sum := sha256.Sum256(raw)
		rel, _ := filepath.Rel(dir, p)
		lines = append(lines, rel+" "+hex.EncodeToString(sum[:])+" "+fi.ModTime().String())
		return nil
	})
	sort.Strings(lines)
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(sum[:])
}

func repo(t *testing.T, sb *sandbox, name string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := sb.dir(name)
	gitRun(t, dir, "init", "-q", "-b", "main")
	os.WriteFile(filepath.Join(dir, "app.txt"), []byte("version 1\n"), 0o644)
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-q", "-m", "first")
	return dir
}

// T08: resuming after the code moved must warn, and must never modify the repository.
func TestResumeWarnsWhenTheCodeMovedAndTouchesNothing(t *testing.T) {
	sb := newSandbox(t)
	dir := repo(t, sb, "app")
	sb.ok("project", "add", dir, "--capture", "conversation")
	sb.remember("project", "Run the migration suite before tagging.", "--project", dir)
	sb.say("UserPromptSubmit", "s-1", dir, "Let's add OAuth login.")
	sb.say("Stop", "s-1", dir, "Plan: add the callback route, then the token store.")
	sb.ok("ingest")

	os.WriteFile(filepath.Join(dir, "wip.txt"), []byte("half-done work\n"), 0o644)
	before := treeDigest(t, dir)
	out := sb.ok("checkpoint", "Callback route done. Next: token store, then tests.", "--project", dir)
	if !strings.Contains(out, "branch main") || !strings.Contains(out, "1 uncommitted file(s)") {
		t.Fatalf("checkpoint output: %q", out)
	}

	pack := sb.ok("resume", "--project", dir)
	for _, want := range []string{"Run the migration suite before tagging.", "Callback route done. Next: token store, then tests.", "Let's add OAuth login.",
		"Plan: add the callback route", "matches the checkpoint", "branch main"} {
		if !strings.Contains(pack, want) {
			t.Errorf("resume pack lacks %q:\n%s", want, pack)
		}
	}
	if strings.Contains(pack, "WARNING") {
		t.Fatalf("nothing changed, so there must be no warning:\n%s", pack)
	}
	if after := treeDigest(t, dir); after != before {
		t.Fatal("checkpoint or resume modified the repository (including .git)")
	}

	// Different uncommitted work.
	os.WriteFile(filepath.Join(dir, "wip.txt"), []byte("different work\n"), 0o644)
	if pack := sb.ok("resume", "--project", dir); !strings.Contains(pack, "WARNING: the uncommitted changes differ") {
		t.Fatalf("changed uncommitted work not flagged:\n%s", pack)
	}
	// Work committed since: clean tree, new commit.
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-q", "-m", "wip")
	pack = sb.ok("resume", "--project", dir)
	if !strings.Contains(pack, "WARNING: commit changed") || !strings.Contains(pack, "WARNING: the tree is clean now") {
		t.Fatalf("new commit not flagged:\n%s", pack)
	}
	// Another branch.
	gitRun(t, dir, "checkout", "-q", "-b", "experiment")
	before = treeDigest(t, dir)
	pack = sb.ok("resume", "--project", dir)
	if !strings.Contains(pack, `WARNING: branch changed: checkpoint was on "main", now on "experiment"`) || !strings.Contains(pack, "Nothing was changed by this command") {
		t.Fatalf("branch change not flagged:\n%s", pack)
	}
	if treeDigest(t, dir) != before {
		t.Fatal("resume modified the repository")
	}
	var got struct {
		Data struct {
			Warnings    []string
			Checkpoints []struct{ Note, Branch string }
		}
	}
	json.Unmarshal([]byte(sb.ok("resume", "--project", dir, "--json")), &got)
	if len(got.Data.Warnings) < 2 || got.Data.Checkpoints[0].Branch != "main" {
		t.Fatalf("JSON pack: %+v", got.Data)
	}
}

func TestCheckpointAndResumeWithoutGitAndAcrossProjects(t *testing.T) {
	sb := newSandbox(t)
	plain, other := sb.dir("plain"), sb.dir("other")
	sb.ok("project", "add", plain, "--capture", "conversation")
	sb.ok("project", "add", other, "--capture", "conversation")

	if pack := sb.ok("resume", "--project", plain); !strings.Contains(pack, "none recorded; nothing here says where work stopped") || !strings.Contains(pack, "nothing stored yet") {
		t.Fatalf("empty project:\n%s", pack)
	}
	out := sb.ok("checkpoint", "Outline written.\nNext: chapter two.", "--project", plain)
	if !strings.Contains(out, "no Git repository here") {
		t.Fatalf("non-Git folder should be said plainly: %q", out)
	}
	sb.ok("checkpoint", "Second checkpoint for plain.", "--project", plain)
	pack := sb.ok("resume", "--project", plain)
	if !strings.Contains(pack, "Second checkpoint for plain.") || !strings.Contains(pack, "(1 earlier checkpoint(s) exist)") || strings.Contains(pack, "WARNING") {
		t.Fatalf("latest checkpoint must be shown, without warnings:\n%s", pack)
	}
	if pack := sb.ok("resume", "--project", other); strings.Contains(pack, "checkpoint for plain") || strings.Contains(pack, "Outline written") {
		t.Fatalf("another project's checkpoint leaked:\n%s", pack)
	}

	for _, bad := range [][]string{
		{"checkpoint"}, {"checkpoint", "  ", "--project", plain}, {"checkpoint", "a", "b"}, {"resume", "extra"},
	} {
		if code, _, _ := sb.run(bad...); code != ExitUsage {
			t.Errorf("%v: exit=%d, want usage", bad, code)
		}
	}
	if code, _, errOut := sb.run("checkpoint", "note", "--project", sb.dir("unregistered")); code != ExitFailure || !strings.Contains(errOut, "not inside a registered project") {
		t.Fatalf("unregistered: exit=%d %q", code, errOut)
	}
	if code, _, errOut := sb.run("checkpoint", "use password=Tr0ub4dor-synthetic to log in", "--project", plain); code != ExitFailure || !strings.Contains(errOut, "not saved") {
		t.Fatalf("secret-looking note: exit=%d %q", code, errOut)
	}
}

// A new session is told where work stopped without having to ask: the latest checkpoint is part
// of what SessionStart delivers, for that project only.
func TestLatestCheckpointIsDeliveredAtSessionStart(t *testing.T) {
	sb := newSandbox(t)
	a, b := sb.dir("alpha"), sb.dir("beta")
	sb.ok("project", "add", a, "--capture", "conversation")
	sb.ok("project", "add", b, "--capture", "conversation")
	if ctx := sb.inject("SessionStart", "s0", a); ctx != "" {
		t.Fatalf("nothing recorded yet: %q", ctx)
	}
	sb.ok("checkpoint", "First draft done. Next: write the retry logic.", "--project", a)
	ctx := sb.inject("SessionStart", "s1", a)
	if !strings.Contains(ctx, "First draft done. Next: write the retry logic.") || !strings.Contains(ctx, "not an instruction") {
		t.Fatalf("checkpoint not delivered:\n%s", ctx)
	}
	sb.ok("checkpoint", "Retry logic done. Next: tests.", "--project", a)
	ctx = sb.inject("SessionStart", "s2", a)
	if !strings.Contains(ctx, "Retry logic done. Next: tests.") || strings.Contains(ctx, "First draft done") {
		t.Fatalf("only the latest checkpoint should be delivered:\n%s", ctx)
	}
	sb.remember("project", "Use ISO dates.", "--project", a)
	if ctx := sb.inject("SessionStart", "s3", a); !strings.Contains(ctx, "Use ISO dates.") || !strings.Contains(ctx, "Retry logic done. Next: tests.") {
		t.Fatalf("instructions and checkpoint together:\n%s", ctx)
	}
	if ctx := sb.inject("SessionStart", "s4", b); ctx != "" {
		t.Fatalf("another project received the checkpoint: %q", ctx)
	}
	sb.ok("forget", "project", a, "--yes")
	if ctx := sb.inject("SessionStart", "s5", a); ctx != "" {
		t.Fatalf("a deleted project's checkpoint is still delivered: %q", ctx)
	}
}

// compactStart runs the session start that follows a compaction and returns what Claude receives.
func (sb *sandbox) compactStart(session, cwd string) string {
	sb.t.Helper()
	raw, _ := json.Marshal(map[string]string{"session_id": session, "cwd": cwd, "hook_event_name": "SessionStart", "source": "compact"})
	code, out, errOut := sb.hook("SessionStart", string(raw))
	if code != ExitOK || errOut != "" {
		sb.t.Fatalf("hook: exit %d stderr %q", code, errOut)
	}
	if out == "" {
		return ""
	}
	var resp struct {
		HookSpecificOutput struct {
			AdditionalContext string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		sb.t.Fatalf("hook output is not one JSON document: %v: %q", err, out)
	}
	return resp.HookSpecificOutput.AdditionalContext
}

// After a compaction the model is left with a summary. The start that follows gives it the last
// exchanges word for word, before anything was ingested, beside the instructions.
func TestTheLastExchangesComeBackWordForWordAfterACompaction(t *testing.T) {
	sb := newSandbox(t)
	app := sb.dir("app")
	sb.ok("project", "add", app, "--capture", "conversation")
	sb.remember("project", "Always use tabs TABRULE.", "--project", app)

	sb.say("UserPromptSubmit", "s-1", app, "Build the invoice page FIRSTASK.")
	sb.say("Stop", "s-1", app, "Done, the page exists.")
	for i := 0; i < 4; i++ {
		sb.say("UserPromptSubmit", "s-1", app, fmt.Sprintf("Follow-up %d.", i))
		sb.say("Stop", "s-1", app, fmt.Sprintf("Answer %d.", i))
	}
	sb.say("UserPromptSubmit", "s-1", app, "Now fix the total, my key is AKIAIOSFODNN7EXAMPLE LASTASK.")
	sb.say("UserPromptSubmit", "s-other", app, "Another session OTHERMARK.")

	if got := sb.inject("SessionStart", "s-1", app); strings.Contains(got, "LASTASK") || !strings.Contains(got, "TABRULE") {
		t.Fatalf("an ordinary start should carry the instructions and no exchanges: %q", got)
	}
	got := sb.compactStart("s-1", app)
	for _, want := range []string{"TABRULE", "LASTASK", "Answer 3.", "FIRSTASK", "word for word"} {
		if !strings.Contains(got, want) {
			t.Errorf("the start after a compaction lacks %q:\n%s", want, got)
		}
	}
	for _, not := range []string{"AKIAIOSFODNN7EXAMPLE", "OTHERMARK", "Follow-up 0."} {
		if strings.Contains(got, not) {
			t.Errorf("the start after a compaction contains %q:\n%s", not, got)
		}
	}
	if strings.Index(got, "FIRSTASK") > strings.Index(got, "LASTASK") || strings.Index(got, "TABRULE") > strings.Index(got, "FIRSTASK") {
		t.Errorf("order should be instructions, first request, newest last:\n%s", got)
	}
	if n := len(utf16.Encode([]rune(got))); n > 9000 {
		t.Fatalf("the message is %d units long, over the limit", n)
	}

	// Long exchanges are dropped oldest first; the message never passes the limit.
	big := strings.Repeat("word ", 2000)
	for i := 0; i < 8; i++ {
		sb.say("UserPromptSubmit", "s-1", app, fmt.Sprintf("BIG%d %s", i, big))
	}
	got = sb.compactStart("s-1", app)
	if n := len(utf16.Encode([]rune(got))); n > 9000 || !strings.Contains(got, "BIG7") || !strings.Contains(got, "TABRULE") {
		t.Fatalf("long exchanges: %d units, newest kept %v, instructions kept %v", n, strings.Contains(got, "BIG7"), strings.Contains(got, "TABRULE"))
	}

	// Deleting the project's captured text removes what could be given back.
	sb.ok("ingest")
	sb.ok("forget", "captured", "--project", app, "--yes")
	if got := sb.compactStart("s-1", app); strings.Contains(got, "BIG7") || strings.Contains(got, "LASTASK") {
		t.Fatalf("deleted text came back after a compaction: %.300q", got)
	}
}

// A project that saves no text keeps no exchanges and gives none back.
func TestNoExchangesAreKeptWhereTextIsNotSaved(t *testing.T) {
	sb := newSandbox(t)
	app := sb.dir("app")
	sb.ok("project", "add", app, "--capture", "metadata")
	sb.say("UserPromptSubmit", "s-1", app, "A private question METAMARK.")
	if got := sb.compactStart("s-1", app); got != "" {
		t.Fatalf("a text-free project received %q", got)
	}
	filepath.Walk(sb.env[config.EnvDataDir], func(p string, fi os.FileInfo, err error) error {
		if err == nil && !fi.IsDir() {
			if raw, _ := os.ReadFile(p); strings.Contains(string(raw), "METAMARK") {
				t.Errorf("the prompt was written to %s", p)
			}
		}
		return nil
	})
}

package gitstate

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func run(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=f", "-c", "user.email=f@example.invalid", "-c", "commit.gpgsign=false", "-c", "core.autocrlf=false"}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestReadAndCompare(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	plain := t.TempDir()
	if s := Read(plain); s.Repo || s.Reason == "" {
		t.Fatalf("plain folder: %+v", s)
	}
	dir := filepath.Join(t.TempDir(), "repo é")
	os.MkdirAll(dir, 0o755)
	run(t, dir, "init", "-q", "-b", "main")
	empty := Read(dir)
	if !empty.Repo || empty.Head != "" || empty.Branch != "main" {
		t.Fatalf("repository without commits: %+v", empty)
	}
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("one\n"), 0o644)
	os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("ignored.log\n"), 0o644)
	run(t, dir, "add", "-A")
	run(t, dir, "commit", "-q", "-m", "c1")
	clean := Read(dir)
	if clean.Dirty != "" || clean.Changed != 0 || len(clean.Head) != 40 {
		t.Fatalf("clean tree: %+v", clean)
	}
	if w := Compare(clean, Read(dir)); len(w) != 0 {
		t.Fatalf("identical states compared unequal: %v", w)
	}

	// Each kind of uncommitted change must produce a different fingerprint.
	seen := map[string]string{"": "clean"}
	step := func(label string, change func()) State {
		change()
		s := Read(dir)
		if prev, dup := seen[s.Dirty]; dup {
			t.Fatalf("%s has the same fingerprint as %s", label, prev)
		}
		seen[s.Dirty] = label
		return s
	}
	step("modified tracked file", func() { os.WriteFile(filepath.Join(dir, "a.txt"), []byte("two\n"), 0o644) })
	step("new untracked file", func() { os.WriteFile(filepath.Join(dir, "new.txt"), []byte("x\n"), 0o644) })
	step("untracked file rewritten", func() { os.WriteFile(filepath.Join(dir, "new.txt"), []byte("y\n"), 0o644) })
	staged := step("change staged", func() { run(t, dir, "add", "a.txt") })
	// An ignored file is not part of the state.
	os.WriteFile(filepath.Join(dir, "ignored.log"), []byte("noise"), 0o644)
	if s := Read(dir); s.Dirty != staged.Dirty {
		t.Fatal("an ignored file changed the fingerprint")
	}

	w := strings.Join(Compare(clean, staged), "; ")
	if !strings.Contains(w, "uncommitted changes now") {
		t.Fatalf("clean -> dirty: %q", w)
	}
	run(t, dir, "add", "-A")
	run(t, dir, "commit", "-q", "-m", "c2")
	run(t, dir, "checkout", "-q", "-b", "feature")
	w = strings.Join(Compare(staged, Read(dir)), "; ")
	for _, want := range []string{`branch changed: checkpoint was on "main", now on "feature"`, "commit changed", "tree is clean now"} {
		if !strings.Contains(w, want) {
			t.Errorf("comparison lacks %q: %q", want, w)
		}
	}
	if w := Compare(State{}, Read(dir)); len(w) != 1 || !strings.Contains(w[0], "is a Git repository now") {
		t.Fatalf("non-repo -> repo: %v", w)
	}
	if w := Compare(clean, State{Reason: "not a Git repository"}); len(w) != 1 {
		t.Fatalf("repo -> non-repo: %v", w)
	}
}

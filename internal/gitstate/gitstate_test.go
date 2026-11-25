package gitstate

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

// A repository's own settings must not run anything when SessionKeep reads it: a copied or
// unzipped project can carry a .git/config that names a program (security review G1).
func TestARepositorysSettingsRunNothing(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := filepath.Join(t.TempDir(), "repo")
	os.MkdirAll(dir, 0o755)
	run(t, dir, "init", "-q", "-b", "main")
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("one\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "b.dat"), []byte("one\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "keep.txt"), []byte("kept\n"), 0o644)
	os.WriteFile(filepath.Join(dir, ".gitattributes"), []byte("*.txt filter=evil diff=evil\n*.dat filter=evil.two\n"), 0o644)
	run(t, dir, "add", "-A")
	run(t, dir, "commit", "-q", "-m", "c1")

	// Make HEAD a signed commit, so that showing signatures would call gpg.program.
	raw, err := exec.Command("git", "-C", dir, "cat-file", "commit", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	head, msg, _ := strings.Cut(string(raw), "\n\n")
	signed := head + "\ngpgsig -----BEGIN PGP SIGNATURE-----\n \n iQEzBAABCAAdFiEE\n -----END PGP SIGNATURE-----\n\n" + msg
	hashObj := exec.Command("git", "-C", dir, "hash-object", "-t", "commit", "-w", "--stdin")
	hashObj.Stdin = strings.NewReader(signed)
	id, err := hashObj.Output()
	if err != nil {
		t.Fatal(err)
	}
	run(t, dir, "update-ref", "HEAD", strings.TrimSpace(string(id)))

	marker := filepath.ToSlash(filepath.Join(t.TempDir(), "ran.txt"))
	script := filepath.ToSlash(filepath.Join(t.TempDir(), "mark.sh"))
	os.WriteFile(script, []byte("#!/bin/sh\necho \"$0 $*\" >> '"+marker+"'\ncat\n"), 0o755)
	for _, kv := range [][2]string{
		{"core.fsmonitor", script},
		{"filter.evil.clean", script},
		{"filter.evil.smudge", script},
		{"filter.evil.two.process", script},
		{"diff.evil.textconv", script},
		{"diff.evil.command", script},
		{"diff.external", script},
		{"log.showSignature", "true"},
		{"gpg.program", script},
		{"core.hooksPath", filepath.ToSlash(t.TempDir())},
	} {
		run(t, dir, "config", kv[0], kv[1])
	}
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("two\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "b.dat"), []byte("two\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "c.txt"), []byte("new\n"), 0o644)

	s := Read(dir)
	if !s.Repo || s.Changed != 3 || s.Dirty == "" {
		t.Errorf("the state was not read: %+v", s)
	}
	if c := Commits(dir, time.Now().Add(-time.Hour), time.Now().Add(time.Hour), 5); len(c) != 1 {
		t.Errorf("commits = %+v", c)
	}
	if _, files, err := CommitInfo(dir, "HEAD"); err != nil || len(files) != 4 {
		t.Errorf("commit info = %v, %v", files, err)
	}
	if _, err := BlameLine(dir, "keep.txt", 1); err != nil {
		t.Errorf("blame: %v", err)
	}
	if got, err := os.ReadFile(marker); err == nil {
		t.Fatalf("the repository's settings ran a program:\n%s", got)
	}

	// A filter from the user's own global settings (Git LFS, say) is left in place.
	global := filepath.Join(t.TempDir(), "global.gitconfig")
	os.WriteFile(global, []byte("[filter \"lfs\"]\n\tclean = git-lfs clean -- %f\n\trequired = true\n"), 0o644)
	t.Setenv("GIT_CONFIG_GLOBAL", global)
	g, err := guarded(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Join(g.args, " ")
	for _, want := range []string{"filter.evil.clean=", "filter.evil.two.process=", "filter.evil.required=false", "diff.evil.textconv=", "diff.evil.command="} {
		if !strings.Contains(args, want) {
			t.Errorf("%s is not switched off: %s", want, args)
		}
	}
	if strings.Contains(args, "filter.lfs") {
		t.Errorf("the user's own filter was switched off: %s", args)
	}
}

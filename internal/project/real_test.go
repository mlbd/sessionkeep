package project

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"sessionkeep/internal/config"
)

// An excluded folder stays excluded whatever name reaches it: a junction or symbolic link, an
// 8.3 short name, or (macOS) other letter case (security review M3).
func TestAnExcludedFolderIsExcludedByAnyName(t *testing.T) {
	base := t.TempDir()
	secret := filepath.Join(base, "Secret Folder")
	other := filepath.Join(base, "work")
	for _, d := range []string{filepath.Join(secret, "inner"), other} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	link := filepath.Join(other, "shortcut")
	var err error
	if runtime.GOOS == "windows" {
		var out []byte
		out, err = exec.Command("cmd", "/c", "mklink", "/J", link, secret).CombinedOutput()
		if err != nil {
			t.Logf("%s", out)
		}
	} else {
		err = os.Symlink(secret, link)
	}
	if err != nil {
		t.Skipf("cannot make a link here: %v", err)
	}
	norm := func(p string) string {
		n, err := Normalize(runtime.GOOS, p)
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	s := config.DefaultSettings()
	s.Exclusions = []string{norm(secret)}
	roots := []Root{{Path: norm(base), ProjectID: "p", Level: config.LevelConversation}}
	env := Env{GOOS: runtime.GOOS, Real: RealPath}

	names := []string{link, filepath.Join(link, "inner")}
	if short := shortName(secret); short != "" && !strings.EqualFold(short, secret) {
		names = append(names, short, filepath.Join(short, "inner"))
	} else {
		t.Log("this volume has no 8.3 short names; that case is not tried")
	}
	for _, n := range names {
		if d := Decide(env, s, roots, n); d.Reason != "excluded by the user" {
			t.Errorf("%s reaches the excluded folder but was decided %+v", n, d)
		}
		if !Excluded(runtime.GOOS, s.Exclusions, n) {
			t.Errorf("%s reaches the excluded folder but is not excluded", n)
		}
	}
	// Without asking where folders really are, the link is not recognised: that is what this adds.
	if d := Decide(Env{GOOS: runtime.GOOS}, s, roots, link); d.Reason == "excluded by the user" {
		t.Errorf("the test does not exercise the real path")
	}
	// Excluding the link excludes the folder it points to.
	s.Exclusions = []string{norm(link)}
	if d := Decide(env, s, roots, filepath.Join(secret, "inner")); d.Reason != "excluded by the user" {
		t.Errorf("excluding a link did not exclude its target: %+v", d)
	}
	// A folder that is not excluded is still captured.
	if d := Decide(env, s, roots, other); d.Action != ActionCapture {
		t.Errorf("an ordinary folder = %+v", d)
	}
}

// macOS disks usually ignore letter case, so an exclusion matches whatever case reaches it.
func TestExclusionsIgnoreLetterCaseOnMacOS(t *testing.T) {
	s := config.DefaultSettings()
	s.Exclusions = []string{"/Users/a/Secret"}
	roots := []Root{{Path: "/Users/a", ProjectID: "p", Level: config.LevelConversation}}
	if d := Decide(Env{GOOS: "darwin"}, s, roots, "/users/A/secret/x"); d.Reason != "excluded by the user" {
		t.Errorf("macOS: %+v", d)
	}
	if !Excluded("darwin", s.Exclusions, "/USERS/a/SECRET") {
		t.Error("macOS: Excluded ignored a case difference")
	}
	if d := Decide(Env{GOOS: "linux"}, s, roots, "/Users/a/secretly"); d.Action != ActionCapture {
		t.Errorf("a different folder: %+v", d)
	}
}

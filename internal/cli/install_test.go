package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sessionkeep/internal/config"
	"sessionkeep/internal/install"
)

func installSandbox(t *testing.T) (*sandbox, string) {
	sb := newSandbox(t)
	dir := filepath.Join(t.TempDir(), "Programs é", "Dev Weft")
	sb.env[install.EnvInstallDir] = dir
	return sb, dir
}

func files(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	filepath.Walk(dir, func(p string, fi os.FileInfo, err error) error {
		if err == nil && !fi.IsDir() {
			rel, _ := filepath.Rel(dir, p)
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	})
	return out
}

func TestInstallIsIdempotentAndUninstallRemovesOnlyWhatItCreated(t *testing.T) {
	sb, dir := installSandbox(t)
	out := sb.ok("install")
	for _, want := range []string{"No Claude Code setting was changed", "claude plugin marketplace add", "claude plugin install sessionkeep@sessionkeep-local", "Nothing is saved until you register a project"} {
		if !strings.Contains(out, want) {
			t.Errorf("install output lacks %q", want)
		}
	}
	first := files(t, dir)
	want := []string{install.BinaryName(), "install.json", "claude-plugin/.claude-plugin/marketplace.json", "claude-plugin/sessionkeep/.claude-plugin/plugin.json",
		"claude-plugin/sessionkeep/hooks/hooks.json", "claude-plugin/sessionkeep/skills/recall/SKILL.md", "claude-plugin/sessionkeep/skills/remember/SKILL.md", "claude-plugin/sessionkeep/skills/resume/SKILL.md", "claude-plugin/sessionkeep/skills/research/SKILL.md"}
	for _, w := range want {
		found := false
		for _, f := range first {
			found = found || f == w
		}
		if !found {
			t.Errorf("install did not create %s (has %v)", w, first)
		}
	}
	if len(first) != len(want) {
		t.Errorf("install created %d files, expected exactly %d: %v", len(first), len(want), first)
	}
	// The hooks call the installed copy, by absolute path, in the directory with spaces and accents.
	raw, _ := os.ReadFile(filepath.Join(dir, "claude-plugin", "sessionkeep", "hooks", "hooks.json"))
	var hooks struct {
		Hooks map[string][]struct{ Hooks []struct{ Command string } }
	}
	json.Unmarshal(raw, &hooks)
	if cmd := hooks.Hooks["Stop"][0].Hooks[0].Command; cmd != filepath.Join(dir, install.BinaryName()) {
		t.Fatalf("hooks call %q, want the installed program", cmd)
	}
	if _, err := os.Stat(sb.env[config.EnvDataDir]); err != nil {
		t.Fatal("install should create the data directory so the first session needs no setup")
	}
	if out := sb.ok("doctor"); !strings.Contains(out, "installation") || !strings.Contains(out, "hook commands all point at an existing program") {
		t.Fatalf("doctor after install: %q", out)
	}

	// Again, and with the MCP option: same place, no duplicates, no leftovers.
	sb.ok("install")
	sb.ok("install", "--with-mcp")
	second := files(t, dir)
	if len(second) != len(want)+1 {
		t.Fatalf("after reinstalling with --with-mcp: %v", second)
	}
	sb.ok("install")
	if got := files(t, dir); len(got) != len(want) {
		t.Fatalf("reinstalling without --with-mcp must remove the MCP declaration: %v", got)
	}

	// Data created by the user survives uninstall; a file the user put in the folder survives too.
	app := sb.dir("app")
	sb.ok("project", "add", app, "--capture", "conversation")
	sb.remember("global", "Keep me.")
	os.WriteFile(filepath.Join(dir, "my-notes.txt"), []byte("mine"), 0o644)
	out = sb.ok("uninstall")
	if !strings.Contains(out, "your data was kept") || !strings.Contains(out, "claude plugin uninstall sessionkeep@sessionkeep-local") {
		t.Fatalf("uninstall output: %q", out)
	}
	if left := files(t, dir); len(left) != 1 || left[0] != "my-notes.txt" {
		t.Fatalf("after uninstall the install folder holds %v; want only the user's own file", left)
	}
	if out := sb.ok("instruction", "list"); !strings.Contains(out, "Keep me.") {
		t.Fatal("uninstall destroyed data")
	}
	if code, _, _ := sb.run("uninstall"); code != ExitFailure {
		t.Fatal("uninstalling twice should say it is not installed")
	}
}

func TestInstallRefusesAFolderThatIsNotItsOwn(t *testing.T) {
	sb, dir := installSandbox(t)
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "someone-elses.txt"), []byte("x"), 0o644)
	if code, _, errOut := sb.run("install"); code != ExitFailure || !strings.Contains(errOut, "not created by SessionKeep") {
		t.Fatalf("exit=%d %q", code, errOut)
	}
	if got := files(t, dir); len(got) != 1 {
		t.Fatalf("a refused install changed the folder: %v", got)
	}
}

func TestUninstallPurgeNeedsPreviewAndConfirmation(t *testing.T) {
	sb, _ := installSandbox(t)
	sb.ok("install")
	app := sb.dir("app")
	sb.ok("project", "add", app, "--capture", "conversation")
	data := sb.env[config.EnvDataDir]

	code, out, _ := sb.run("uninstall", "--purge-data")
	if code != ExitNeedsConsent || !strings.Contains(out, "NOT DONE YET") || !strings.Contains(out, "export") || !strings.Contains(out, `"projects":1`) {
		t.Fatalf("purge preview: exit=%d %q", code, out)
	}
	if _, err := os.Stat(filepath.Join(data, "sessionkeep.db")); err != nil {
		t.Fatal("the preview deleted data")
	}
	if out := sb.ok("doctor"); strings.Contains(out, "not installed") || !strings.Contains(out, "hook commands all point at an existing program") {
		t.Fatalf("the preview uninstalled the program: %q", out)
	}
	sb.ok("uninstall", "--purge-data", "--yes")
	if _, err := os.Stat(data); err == nil {
		t.Fatal("data directory still exists after a confirmed purge")
	}
	if _, err := os.Stat(app); err != nil {
		t.Fatal("purge touched the project folder")
	}
}

func TestPurgeRefusesADirectoryThatIsNotSessionKeepData(t *testing.T) {
	sb, _ := installSandbox(t)
	other := filepath.Join(t.TempDir(), "important documents")
	os.MkdirAll(other, 0o755)
	os.WriteFile(filepath.Join(other, "thesis.docx"), []byte("years of work"), 0o644)
	sb.env[config.EnvDataDir] = other
	if code, _, errOut := sb.run("uninstall", "--purge-data", "--yes"); code != ExitFailure || !strings.Contains(errOut, "does not look like a SessionKeep data directory") {
		t.Fatalf("exit=%d %q", code, errOut)
	}
	if _, err := os.Stat(filepath.Join(other, "thesis.docx")); err != nil {
		t.Fatal("purge deleted a directory that is not SessionKeep's")
	}
}

func TestDoctorNoticesBrokenInstall(t *testing.T) {
	sb, dir := installSandbox(t)
	sb.ok("install")
	os.Remove(filepath.Join(dir, install.BinaryName()))
	code, out, _ := sb.run("doctor")
	if code != ExitFailure || !strings.Contains(out, "installed program is missing") || !strings.Contains(out, "hooks point at a program that does not exist") {
		t.Fatalf("exit=%d %q", code, out)
	}
}

func TestAutostartNeedsAnInstallAndUsageIsChecked(t *testing.T) {
	sb, _ := installSandbox(t)
	if out := sb.ok("autostart", "status"); !strings.Contains(out, "auto-start is") {
		t.Fatalf("status: %q", out)
	}
	if code, _, errOut := sb.run("autostart", "enable"); code == ExitOK || (!strings.Contains(errOut, "sessionkeep install") && !strings.Contains(errOut, "only implemented for Windows")) {
		t.Fatalf("enable without an install: exit=%d %q", code, errOut)
	}
	for _, bad := range [][]string{{"autostart"}, {"autostart", "on"}, {"autostart", "enable", "x"}, {"install", "x"}, {"uninstall", "x"}} {
		if code, _, _ := sb.run(bad...); code != ExitUsage {
			t.Errorf("%v: exit=%d, want usage", bad, code)
		}
	}
}

// MOCK: the stand-in records what Claude Code would be asked; what the real program does with the
// two commands was tried by hand against a throwaway configuration folder (evidence/npm/N-A.md).
func TestEnablePluginRunsClaudeCodesOwnTwoCommandsOnlyWhenAsked(t *testing.T) {
	sb, dir := installSandbox(t)
	calls := sb.fakeProgram("ok")
	sb.ok("install")
	if n := len(calls()); n != 0 {
		t.Fatalf("install without --enable-plugin started Claude Code %d time(s)", n)
	}
	out := sb.ok("install", "--enable-plugin")
	got := calls()
	if len(got) != 2 {
		t.Fatalf("want two commands, got %v", got)
	}
	market := filepath.Join(dir, "claude-plugin")
	if a := strings.Join(got[0].Args, "|"); a != "plugin|marketplace|add|"+market {
		t.Errorf("first command: %q", a)
	}
	if a := strings.Join(got[1].Args, "|"); a != "plugin|install|sessionkeep@sessionkeep-local" {
		t.Errorf("second command: %q", a)
	}
	if !strings.Contains(out, "The plugin is turned on in Claude Code") || !strings.Contains(out, "Restart Claude Code") || strings.Contains(out, "No Claude Code setting was changed") {
		t.Errorf("output: %q", out)
	}
}

func TestEnablePluginUsesTheChosenAccountAndReportsFailure(t *testing.T) {
	sb, _ := installSandbox(t)
	work := filepath.Join(sb.env["USERPROFILE"], ".claude-work")
	for _, d := range []string{filepath.Join(sb.env["USERPROFILE"], ".claude", "projects"), filepath.Join(work, "projects")} {
		os.MkdirAll(d, 0o700)
	}
	sb.ok("account", "use", "work")
	calls := sb.fakeProgram("ok")
	sb.ok("install", "--enable-plugin")
	for _, c := range calls() {
		if c.Config != work {
			t.Fatalf("the command ran for configuration folder %q, want %q", c.Config, work)
		}
	}

	t.Setenv("SESSIONKEEP_FAKE_CLAUDE", "fail")
	code, out, errOut := sb.run("install", "--enable-plugin")
	if code != ExitFailure || !strings.Contains(out, "could NOT be turned on") || !strings.Contains(out, "not signed in") || !strings.Contains(out, "plugin marketplace add") || !strings.Contains(errOut, "the program itself is installed") {
		t.Fatalf("exit=%d\n%q\n%q", code, out, errOut)
	}
	if out := sb.ok("doctor"); !strings.Contains(out, "hook commands all point at an existing program") {
		t.Fatal("a failed plugin step must leave the installation itself in place")
	}
}

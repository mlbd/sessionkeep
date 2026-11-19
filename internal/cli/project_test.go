package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sessionkeep/internal/config"
)

// sandbox gives each test its own data directory and a work area, both with awkward names.
type sandbox struct {
	t    *testing.T
	env  map[string]string
	work string
}

func newSandbox(t *testing.T) *sandbox {
	t.Helper()
	base := t.TempDir()
	work := filepath.Join(base, "work é")
	if err := os.MkdirAll(work, 0o700); err != nil {
		t.Fatal(err)
	}
	// HOME/USERPROFILE point at a sibling so the work area is not "inside home".
	home := filepath.Join(base, "home")
	return &sandbox{t: t, work: work, env: map[string]string{config.EnvDataDir: filepath.Join(base, "data dir"), "HOME": home, "USERPROFILE": home}}
}

func (sb *sandbox) dir(parts ...string) string {
	sb.t.Helper()
	d := filepath.Join(append([]string{sb.work}, parts...)...)
	if err := os.MkdirAll(d, 0o700); err != nil {
		sb.t.Fatal(err)
	}
	return d
}

func (sb *sandbox) run(args ...string) (int, string, string) { return run(sb.t, sb.env, args...) }

func (sb *sandbox) ok(args ...string) string {
	sb.t.Helper()
	code, out, errOut := sb.run(args...)
	if code != ExitOK {
		sb.t.Fatalf("sessionkeep %s: exit %d\nstdout: %s\nstderr: %s", strings.Join(args, " "), code, out, errOut)
	}
	return out
}

// which returns the action SessionKeep would take in dir.
func (sb *sandbox) which(dir string) (action, level, project string) {
	sb.t.Helper()
	var got struct {
		Data struct {
			Action, Level, Reason string
			ProjectID             string `json:"project_id"`
		}
	}
	out := sb.ok("project", "which", dir, "--json")
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		sb.t.Fatalf("bad JSON %q: %v", out, err)
	}
	return got.Data.Action, got.Data.Level, got.Data.ProjectID
}

func TestFreshInstallRemindsAndCapturesNothing(t *testing.T) {
	sb := newSandbox(t)
	if a, _, _ := sb.which(sb.dir("app")); a != "remind" {
		t.Fatalf("before any setup: action=%q, want remind", a)
	}
	if _, err := os.Stat(sb.env[config.EnvDataDir]); err == nil {
		t.Fatal("`project which` must not create the data directory")
	}
	out := sb.ok("setup")
	if !strings.Contains(out, "ask") {
		t.Fatalf("setup without flags must report the default ask mode: %q", out)
	}
	if a, _, _ := sb.which(sb.dir("app")); a != "remind" {
		t.Fatalf("after setup: action=%q, want remind", a)
	}
}

func TestProjectAddNeedsAnExplicitLevel(t *testing.T) {
	sb := newSandbox(t)
	app := sb.dir("app")
	code, _, errOut := sb.run("project", "add", app)
	if code != ExitNeedsConsent || !strings.Contains(errOut, "no default") {
		t.Fatalf("exit=%d stderr=%q; want needs-consent and an explanation", code, errOut)
	}
	if code, _, _ := sb.run("project", "add", app, "--capture", "everything"); code != ExitUsage {
		t.Fatalf("unknown level: exit=%d, want usage", code)
	}
	if code, _, _ := sb.run("project", "add", filepath.Join(sb.work, "missing"), "--capture", "conversation"); code != ExitFailure {
		t.Fatalf("missing directory: exit=%d, want failure", code)
	}
	if out := sb.ok("project", "list"); !strings.Contains(out, "no projects registered") {
		t.Fatalf("failed adds must register nothing: %q", out)
	}
}

func TestRegisteredProjectLifecycle(t *testing.T) {
	sb := newSandbox(t)
	app := sb.dir("App")
	sub := sb.dir("App", "src", "deep")
	sibling := sb.dir("App2")

	sb.ok("project", "add", app, "--capture", "conversation", "--name", "Demo App")
	if a, l, id := sb.which(sub); a != "capture" || l != "conversation" || id == "" {
		t.Fatalf("subdirectory: %s %s %s", a, l, id)
	}
	if a, _, _ := sb.which(sibling); a != "remind" {
		t.Fatalf("sibling with a shared name prefix must not match: %s", a)
	}
	if code, _, errOut := sb.run("project", "add", app, "--capture", "metadata"); code != ExitFailure || !strings.Contains(errOut, "already registered") {
		t.Fatalf("double add: exit=%d stderr=%q", code, errOut)
	}

	sb.ok("project", "pause", app)
	if a, _, _ := sb.which(sub); a != "none" {
		t.Fatalf("paused: %s, want none", a)
	}
	sb.ok("project", "resume", app)
	if a, l, _ := sb.which(sub); a != "capture" || l != "conversation" {
		t.Fatalf("resumed: %s %s", a, l)
	}
	sb.ok("project", "capture", app, "metadata")
	if _, l, _ := sb.which(sub); l != "metadata" {
		t.Fatalf("level after change: %s", l)
	}
	sb.ok("project", "capture", app, "off")
	if a, _, _ := sb.which(sub); a != "none" {
		t.Fatalf("off: %s, want none", a)
	}
	if code, _, _ := sb.run("project", "pause", sibling); code != ExitFailure {
		t.Fatalf("pausing an unregistered folder: exit=%d, want failure", code)
	}
	if out := sb.ok("project", "list"); !strings.Contains(out, "off") || strings.Count(out, "\n") != 1 {
		t.Fatalf("list: %q", out)
	}
}

func TestAutoModeAndExclusions(t *testing.T) {
	sb := newSandbox(t)
	fresh := sb.dir("fresh", "pkg")
	client := sb.dir("clients", "acme", "tool")

	if code, _, errOut := sb.run("setup", "--auto-add", "auto"); code != ExitUsage || !strings.Contains(errOut, "--level") {
		t.Fatalf("auto without a level: exit=%d stderr=%q", code, errOut)
	}
	if code, _, _ := sb.run("setup", "--auto-add", "auto", "--level", "off"); code != ExitUsage {
		t.Fatalf("auto at level off: exit=%d, want usage", code)
	}
	if code, _, _ := sb.run("setup", "--level", "conversation"); code != ExitUsage {
		t.Fatalf("--level alone: exit=%d, want usage", code)
	}
	if a, _, _ := sb.which(fresh); a != "remind" {
		t.Fatalf("rejected setup calls must not have enabled auto: %s", a)
	}

	sb.ok("setup", "--auto-add", "auto", "--level", "conversation")
	if a, l, _ := sb.which(fresh); a != "auto_register" || l != "conversation" {
		t.Fatalf("auto mode: %s %s", a, l)
	}

	sb.ok("exclude", "add", filepath.Join(sb.work, "clients", "acme"))
	if a, _, _ := sb.which(client); a != "none" {
		t.Fatalf("excluded folder in auto mode: %s, want none", a)
	}
	if code, _, errOut := sb.run("project", "add", client, "--capture", "conversation"); code != ExitFailure || !strings.Contains(errOut, "excluded") {
		t.Fatalf("adding an excluded folder: exit=%d stderr=%q", code, errOut)
	}
	if out := sb.ok("exclude", "list"); !strings.Contains(strings.ToLower(out), "acme") {
		t.Fatalf("exclude list: %q", out)
	}

	sb.ok("exclude", "remove", filepath.Join(sb.work, "clients", "acme"))
	if a, _, _ := sb.which(client); a != "auto_register" {
		t.Fatalf("after removing the exclusion: %s", a)
	}
	if code, _, _ := sb.run("exclude", "remove", filepath.Join(sb.work, "never-added")); code != ExitFailure {
		t.Fatalf("removing an unknown exclusion: exit=%d, want failure", code)
	}

	sb.ok("setup", "--auto-add", "manual")
	if a, _, _ := sb.which(fresh); a != "none" {
		t.Fatalf("manual mode: %s, want none", a)
	}
}

func TestExclusionStillAppliesToAnAlreadyRegisteredProject(t *testing.T) {
	sb := newSandbox(t)
	app := sb.dir("client", "app")
	sb.ok("project", "add", app, "--capture", "extended")
	sb.ok("exclude", "add", sb.dir("client"))
	if a, _, _ := sb.which(app); a != "none" {
		t.Fatalf("registered project under a new exclusion: %s, want none", a)
	}
}

func TestDeviceIDIsStableAndSettingsSurviveCommands(t *testing.T) {
	sb := newSandbox(t)
	id := func() string {
		var got struct {
			Data struct {
				DeviceID string `json:"device_id"`
			}
		}
		json.Unmarshal([]byte(sb.ok("setup", "--json")), &got)
		return got.Data.DeviceID
	}
	first := id()
	sb.ok("exclude", "add", sb.dir("x"))
	sb.ok("project", "add", sb.dir("app"), "--capture", "metadata")
	if first == "" || id() != first {
		t.Fatalf("device id changed: %q -> %q", first, id())
	}
}

// A damaged settings or roots file must stop capture decisions, never fall back to defaults that
// would ignore the user's exclusions.
func TestDamagedFilesFailClosed(t *testing.T) {
	sb := newSandbox(t)
	app := sb.dir("app")
	sb.ok("project", "add", app, "--capture", "conversation")
	data := sb.env[config.EnvDataDir]

	roots := filepath.Join(data, "cache", "roots.json")
	good, _ := os.ReadFile(roots)
	os.WriteFile(roots, []byte(`{"version":1,"roots":[`), 0o600)
	if code, out, _ := sb.run("project", "which", app, "--json"); code != ExitUnavailable || strings.Contains(out, `"capture"`) {
		t.Fatalf("damaged roots cache: exit=%d out=%q", code, out)
	}
	os.WriteFile(roots, good, 0o600)

	os.WriteFile(filepath.Join(data, "config.json"), []byte(`{"schema_version":1,"auto_add":{"mode":"auto"`), 0o600)
	for _, args := range [][]string{{"project", "which", app}, {"project", "add", sb.dir("b"), "--capture", "conversation"}, {"setup", "--auto-add", "auto", "--level", "extended"}} {
		if code, _, errOut := sb.run(args...); code != ExitUnavailable || !strings.Contains(errOut, "settings") {
			t.Fatalf("%v with damaged settings: exit=%d stderr=%q", args, code, errOut)
		}
	}
	if raw, _ := os.ReadFile(filepath.Join(data, "config.json")); !strings.HasSuffix(string(raw), `"auto"`) {
		t.Fatal("a damaged settings file must not be overwritten silently")
	}
}

func TestFlagParsing(t *testing.T) {
	sb := newSandbox(t)
	app := sb.dir("app")
	for _, args := range [][]string{
		{"project", "add", app, "--capture"},                         // missing value
		{"project", "add", app, "--capture", "off", "--capture=off"}, // duplicate
		{"project", "list", "--capture", "off"},                      // flag that does not apply
		{"project", "add", app, "--level", "off"},                    // setup-only flag
		{"project"}, {"project", "frobnicate"}, {"exclude", "add"}, {"exclude", "list", "extra"},
	} {
		if code, _, _ := sb.run(args...); code != ExitUsage {
			t.Errorf("%v: exit=%d, want usage", args, code)
		}
	}
	sb.ok("project", "add", app, "--capture=metadata")
}

func (sb *sandbox) hook(event, stdin string) (int, string, string) {
	var o, e strings.Builder
	code := Run([]string{"hook", event}, strings.NewReader(stdin), &o, &e, func(k string) string { return sb.env[k] })
	return code, o.String(), e.String()
}

func (sb *sandbox) spoolCount() int {
	entries, _ := os.ReadDir(filepath.Join(sb.env[config.EnvDataDir], "spool"))
	return len(entries)
}

func TestHookCommandCapturesOnlyForRegisteredProjectsAndNeverFails(t *testing.T) {
	sb := newSandbox(t)
	app := sb.dir("app")
	other := sb.dir("other")
	in := func(cwd string) string {
		raw, _ := json.Marshal(map[string]string{"session_id": "s-1", "cwd": cwd, "hook_event_name": "UserPromptSubmit", "prompt": "remember to use tabs"})
		return string(raw)
	}

	// Before anything is set up: nothing written, nothing printed, exit 0.
	if code, out, errOut := sb.hook("UserPromptSubmit", in(app)); code != ExitOK || out != "" || errOut != "" {
		t.Fatalf("before setup: %d %q %q", code, out, errOut)
	}
	sb.ok("project", "add", app, "--capture", "conversation")

	if code, out, errOut := sb.hook("UserPromptSubmit", in(filepath.Join(app, "src"))); code != ExitOK || out != "" || errOut != "" || sb.spoolCount() != 1 {
		t.Fatalf("registered: %d %q %q spool=%d", code, out, errOut, sb.spoolCount())
	}
	if code, out, _ := sb.hook("UserPromptSubmit", in(other)); code != ExitOK || out != "" || sb.spoolCount() != 1 {
		t.Fatalf("unregistered prompt must write and print nothing: %d %q spool=%d", code, out, sb.spoolCount())
	}
	if code, out, _ := sb.hook("SessionStart", in(other)); code != ExitOK || !strings.Contains(out, "systemMessage") || sb.spoolCount() != 1 {
		t.Fatalf("unregistered session start should only remind: %d %q", code, out)
	}
	for _, bad := range [][2]string{{"UserPromptSubmit", "{broken"}, {"UserPromptSubmit", ""}, {"NoSuchEvent", in(app)}} {
		if code, out, errOut := sb.hook(bad[0], bad[1]); code != ExitOK || out != "" || errOut != "" {
			t.Fatalf("%v: %d %q %q", bad, code, out, errOut)
		}
	}
	// Wrong argument counts are also silent successes on the hook path.
	for _, args := range [][]string{{"hook"}, {"hook", "Stop", "extra"}, {"hook", "--json"}} {
		if code, out, errOut := sb.run(args...); code != ExitOK || out != "" || errOut != "" {
			t.Fatalf("%v: %d %q %q", args, code, out, errOut)
		}
	}
	if sb.spoolCount() != 1 {
		t.Fatalf("spool=%d, want 1", sb.spoolCount())
	}
}

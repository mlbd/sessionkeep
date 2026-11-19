package plugin

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func self(t *testing.T) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return exe
}

func TestWriteProducesExecFormHooksForEveryEvent(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "plugin dir é")
	exe := self(t)
	if err := Write(dir, exe, "1.2.3", false); err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	raw, _ := os.ReadFile(filepath.Join(dir, ".claude-plugin", "plugin.json"))
	if err := json.Unmarshal(raw, &manifest); err != nil || manifest["name"] != "sessionkeep" || manifest["version"] != "1.2.3" {
		t.Fatalf("manifest: %v %v", manifest, err)
	}
	var file struct {
		Hooks map[string][]struct {
			Matcher *string `json:"matcher"`
			Hooks   []struct {
				Type, Command string
				Args          []string
				Timeout       int
				Async         bool
				Shell         *string `json:"shell"`
			}
		}
	}
	raw, _ = os.ReadFile(filepath.Join(dir, "hooks", "hooks.json"))
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatal(err)
	}
	if len(file.Hooks) != len(CaptureEvents) {
		t.Fatalf("%d events, want %d", len(file.Hooks), len(CaptureEvents))
	}
	for _, ev := range CaptureEvents {
		ms := file.Hooks[ev]
		if len(ms) != 1 || ms[0].Matcher != nil {
			t.Fatalf("%s: want one matcher group without a matcher, got %+v", ev, ms)
		}
		wantDrain := ev == "SessionStart" || ev == "Stop"
		if len(ms[0].Hooks) != map[bool]int{false: 1, true: 2}[wantDrain] {
			t.Fatalf("%s: %d hook entries", ev, len(ms[0].Hooks))
		}
		for i, h := range ms[0].Hooks {
			// Exec form: a real executable path plus args, never a shell string.
			if h.Type != "command" || h.Command != exe || h.Shell != nil || strings.ContainsAny(h.Command, `"'`) || len(h.Args) != 2 {
				t.Fatalf("%s[%d]: not an exec-form command for this binary: %+v", ev, i, h)
			}
		}
		capture := ms[0].Hooks[0]
		if capture.Args[0] != "hook" || capture.Args[1] != ev || capture.Async || capture.Timeout == 0 {
			t.Fatalf("%s: capture hook must be synchronous `hook %s` with a timeout: %+v", ev, ev, capture)
		}
		if wantDrain {
			if d := ms[0].Hooks[1]; d.Args[0] != "ingest" || d.Args[1] != "--quiet" || !d.Async {
				t.Fatalf("%s: drain hook must be an async `ingest --quiet`: %+v", ev, d)
			}
		}
	}
	for _, forbidden := range []string{"PreToolUse", "PermissionRequest"} {
		if _, ok := file.Hooks[forbidden]; ok {
			t.Fatalf("%s must not be hooked: it can block tool calls", forbidden)
		}
	}
}

func TestWriteRefreshesItsOwnDirectoryAndRefusesOthers(t *testing.T) {
	exe := self(t)
	dir := filepath.Join(t.TempDir(), "p")
	if err := Write(dir, exe, "1", false); err != nil {
		t.Fatal(err)
	}
	if err := Write(dir, exe, "2", false); err != nil {
		t.Fatalf("refreshing our own directory: %v", err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, ".claude-plugin", "plugin.json"))
	if !strings.Contains(string(raw), `"version": "2"`) {
		t.Fatalf("manifest not refreshed: %s", raw)
	}

	foreign := filepath.Join(t.TempDir(), "someone-elses")
	os.MkdirAll(foreign, 0o755)
	os.WriteFile(filepath.Join(foreign, "important.txt"), []byte("keep"), 0o644)
	if err := Write(foreign, exe, "1", false); !errors.Is(err, ErrNotOurs) {
		t.Fatalf("want ErrNotOurs, got %v", err)
	}
	if entries, _ := os.ReadDir(foreign); len(entries) != 1 {
		t.Fatalf("a refused directory was modified: %v", entries)
	}

	other := filepath.Join(t.TempDir(), "other-plugin")
	os.MkdirAll(filepath.Join(other, ".claude-plugin"), 0o755)
	os.WriteFile(filepath.Join(other, ".claude-plugin", "plugin.json"), []byte(`{"name":"not-sessionkeep"}`), 0o644)
	if err := Write(other, exe, "1", false); !errors.Is(err, ErrNotOurs) {
		t.Fatalf("another plugin's directory: want ErrNotOurs, got %v", err)
	}
}

func TestWriteRejectsBadBinaryPaths(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "p")
	for _, exe := range []string{"sessionkeep.exe", filepath.Join(t.TempDir(), "missing.exe"), t.TempDir()} {
		if err := Write(dir, exe, "1", false); err == nil {
			t.Errorf("Write accepted binary path %q", exe)
		}
	}
	if _, err := os.Stat(dir); err == nil {
		t.Fatal("nothing should be created for a bad binary path")
	}
}

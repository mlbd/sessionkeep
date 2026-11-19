package cli

import (
	"bytes"
	"encoding/json"
	"runtime"
	"strings"
	"testing"

	"sessionkeep/internal/config"
)

func run(t *testing.T, env map[string]string, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var o, e bytes.Buffer
	code = Run(args, strings.NewReader(""), &o, &e, func(k string) string { return env[k] })
	return code, o.String(), e.String()
}

func absDir() string {
	if runtime.GOOS == "windows" {
		return `C:\sand box\é`
	}
	return "/sand box/é"
}

func TestHelp(t *testing.T) {
	code, out, errOut := run(t, nil, "help")
	if code != ExitOK || !strings.Contains(out, "sessionkeep version") || errOut != "" {
		t.Fatalf("code=%d out=%q err=%q", code, out, errOut)
	}
}

func TestNoArgsIsUsageErrorOnStderrOnly(t *testing.T) {
	code, out, errOut := run(t, nil)
	if code != ExitUsage || out != "" || !strings.Contains(errOut, "Usage:") {
		t.Fatalf("code=%d out=%q err=%q", code, out, errOut)
	}
}

func TestVersionText(t *testing.T) {
	code, out, _ := run(t, nil, "version")
	if code != ExitOK || !strings.HasPrefix(out, "sessionkeep ") || !strings.Contains(out, runtime.GOOS+"/"+runtime.GOARCH) {
		t.Fatalf("code=%d out=%q", code, out)
	}
}

func TestVersionJSON(t *testing.T) {
	code, out, _ := run(t, nil, "version", "--json")
	var got struct {
		Data struct{ Version, Go, OS, Arch string }
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("stdout is not one JSON document: %v: %q", err, out)
	}
	if code != ExitOK || got.Data.Version == "" || got.Data.OS != runtime.GOOS || got.Data.Go != runtime.Version() {
		t.Fatalf("code=%d data=%+v", code, got.Data)
	}
}

func TestPathsJSONUsesOverrideAndCreatesNothing(t *testing.T) {
	dir := absDir()
	code, out, errOut := run(t, map[string]string{config.EnvDataDir: dir}, "paths", "--json")
	var got struct{ Data config.Paths }
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("bad JSON: %v: %q", err, out)
	}
	if code != ExitOK || errOut != "" || got.Data.DataDir != dir || got.Data.Source != "override" {
		t.Fatalf("code=%d err=%q data=%+v", code, errOut, got.Data)
	}
	if !strings.Contains(out, "é") {
		t.Fatalf("non-ASCII path must not be escaped beyond JSON rules: %q", out)
	}
}

func TestPathsWithoutEnvironmentIsUnavailable(t *testing.T) {
	code, out, errOut := run(t, map[string]string{config.EnvDataDir: "relative"}, "paths", "--json")
	var got struct {
		Error struct{ Code, Message string }
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("bad JSON: %v: %q", err, out)
	}
	if code != ExitUnavailable || got.Error.Code != "no_data_dir" || !strings.Contains(errOut, "absolute") {
		t.Fatalf("code=%d err=%q body=%+v", code, errOut, got.Error)
	}
}

func TestUnknownCommandAndArgument(t *testing.T) {
	for _, args := range [][]string{{"remember"}, {"version", "--verbose"}, {"help", "extra"}, {"paths", "--json", "x"}} {
		code, out, errOut := run(t, nil, args...)
		if code != ExitUsage || errOut == "" {
			t.Fatalf("%v: code=%d out=%q err=%q", args, code, out, errOut)
		}
	}
}

// Commands that are not built must not appear to exist (docs/09: no command without a truthful capability).
func TestUnbuiltCommandsAreRejected(t *testing.T) {
	for _, c := range []string{"mcp", "start", "memory", "remember", "search", "review", "sync"} {
		if code, _, _ := run(t, nil, c); code != ExitUsage {
			t.Fatalf("%q returned %d, want usage error", c, code)
		}
	}
}

func TestDoctorReportsEngineAndWhatItDidNotCheck(t *testing.T) {
	code, out, _ := run(t, map[string]string{config.EnvDataDir: absDir()}, "doctor", "--json")
	var got struct {
		Data struct {
			Healthy    bool
			NotChecked string `json:"not_checked"`
			Checks     []struct{ Name, Status, Detail string }
		}
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("bad JSON: %v: %q", err, out)
	}
	if code != ExitOK || !got.Data.Healthy || len(got.Data.Checks) != 4 || got.Data.NotChecked == "" {
		t.Fatalf("code=%d data=%+v", code, got.Data)
	}
	for _, c := range got.Data.Checks {
		if c.Status != "ok" {
			t.Errorf("check %s: %s (%s)", c.Name, c.Status, c.Detail)
		}
	}
}

func TestDoctorFailsWhenDataLocationIsUnusable(t *testing.T) {
	code, out, _ := run(t, map[string]string{config.EnvDataDir: "relative"}, "doctor")
	if code != ExitFailure || !strings.Contains(out, "data_location") || !strings.Contains(out, "failed") {
		t.Fatalf("code=%d out=%q", code, out)
	}
}

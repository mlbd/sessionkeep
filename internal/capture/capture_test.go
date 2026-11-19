package capture

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"sessionkeep/internal/config"
	"sessionkeep/internal/project"
)

// The fixtures are real Claude Code 2.1.328 payloads captured in Phase 0 (placeholders replace
// the user name, config directory and ids).
const fixtureDir = "../../evidence/phase-0/fixtures/claude-2.1.328"

// fixtureCwd is the working directory inside those payloads; fixtureRoot is its normalized form.
const fixtureRoot = `c:\users\<user>\appdata\local\temp\claude\c83101d89baaa2a9\<uuid>\scratchpad\hookfx\proj space é`

type rig struct {
	t     *testing.T
	paths config.Paths
	n     int
}

// newRig creates a data directory with settings and a roots cache. Decisions always use Windows
// path rules because the fixtures hold Windows paths; file I/O uses the host.
func newRig(t *testing.T, mode, autoLevel string, exclusions []string, roots ...project.Root) *rig {
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
	s := config.DefaultSettings()
	s.DeviceID = "device-1"
	s.AutoAdd = config.AutoAdd{Mode: mode, Level: autoLevel}
	if exclusions != nil {
		s.Exclusions = exclusions
	}
	if err := config.SaveSettings(p, s); err != nil {
		t.Fatal(err)
	}
	if err := project.WriteRootsCache(p, roots, time.Unix(0, 0)); err != nil {
		t.Fatal(err)
	}
	return &rig{t: t, paths: p}
}

func (r *rig) handle(event string, raw []byte) Result {
	r.n++
	n := r.n
	return Handle(Request{
		HookEvent: event, Raw: raw, Paths: r.paths,
		Env:   project.Env{GOOS: "windows", Home: `C:\Users\<USER>`},
		Now:   time.Date(2026, 11, 19, 12, 0, n, 0, time.UTC),
		NewID: func() string { return "event-" + string(rune('a'+n)) },
	})
}

func (r *rig) spool() []string {
	r.t.Helper()
	entries, err := os.ReadDir(r.paths.SpoolDir)
	if err != nil {
		r.t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func (r *rig) event(res Result) Event {
	r.t.Helper()
	if res.SpoolFile == "" {
		r.t.Fatalf("no spool file written: %+v", res)
	}
	raw, err := os.ReadFile(res.SpoolFile)
	if err != nil {
		r.t.Fatal(err)
	}
	var ev Event
	if err := json.Unmarshal(raw, &ev); err != nil {
		r.t.Fatalf("spool file is not valid JSON: %v", err)
	}
	return ev
}

// allFiles returns the content of every file under the data directory, to prove something is absent.
func (r *rig) allFiles() string {
	r.t.Helper()
	var b strings.Builder
	filepath.Walk(r.paths.DataDir, func(p string, fi os.FileInfo, err error) error {
		if err == nil && !fi.IsDir() {
			raw, _ := os.ReadFile(p)
			b.WriteString(p + "\n")
			b.Write(raw)
		}
		return nil
	})
	return b.String()
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(fixtureDir, name+".json"))
	if err != nil {
		t.Fatalf("fixture missing: %v", err)
	}
	return raw
}

func registered(level string) project.Root {
	return project.Root{Path: fixtureRoot, ProjectID: "proj-1", Level: level}
}

func TestRealFixturesBecomeNormalizedEvents(t *testing.T) {
	r := newRig(t, "ask", "", nil, registered("conversation"))
	want := map[string]struct {
		kind string
		keys []string
	}{
		"SessionStart":     {"session_start", []string{"source"}},
		"UserPromptSubmit": {"user_prompt", []string{"prompt", "prompt_id", "prompt_chars"}},
		"PostToolUse":      {"tool_use", []string{"tool_name", "tool_use_id", "tool_input", "duration_ms"}},
		"Stop":             {"assistant_turn", []string{"text", "text_chars", "stop_hook_active"}},
		"SessionEnd":       {"session_end", []string{"reason"}},
	}
	for name, w := range want {
		res := r.handle(name, fixture(t, name))
		if len(res.Stdout) != 0 || res.Gap != "" || res.Action != "capture" {
			t.Fatalf("%s: %+v", name, res)
		}
		ev := r.event(res)
		if ev.Kind != w.kind || ev.ProjectID != "proj-1" || ev.Level != "conversation" || ev.DeviceID != "device-1" ||
			ev.SessionID != "<UUID>" || ev.Agent != "claude-code" || ev.Version != 1 || ev.Completeness != "complete" || ev.Register != nil ||
			!strings.HasPrefix(ev.SourceKey, "hook:") {
			t.Errorf("%s: unexpected envelope %+v", name, ev)
		}
		for _, k := range w.keys {
			if _, ok := ev.Payload[k]; !ok {
				t.Errorf("%s: payload lacks %q: %v", name, k, ev.Payload)
			}
		}
	}
	if ev := r.event(r.handle("Stop", fixture(t, "Stop"))); ev.Payload["text"] != "ZEBRA-4471, HERON-9026, LANTERN" {
		t.Errorf("assistant text not captured verbatim: %v", ev.Payload["text"])
	}
	left := r.spool()
	if len(left) != 6 {
		t.Fatalf("spool holds %d files, want 6: %v", len(left), left)
	}
	for _, n := range left {
		if !strings.HasSuffix(n, ".evt") {
			t.Errorf("unexpected spool entry %q (temp file left behind?)", n)
		}
	}
}

func TestLevelsControlWhatTextIsKept(t *testing.T) {
	prompt := fixture(t, "UserPromptSubmit")
	tool := fixture(t, "PostToolUse")
	stop := fixture(t, "Stop")

	meta := newRig(t, "ask", "", nil, registered("metadata"))
	for name, raw := range map[string][]byte{"UserPromptSubmit": prompt, "PostToolUse": tool, "Stop": stop} {
		ev := meta.event(meta.handle(name, raw))
		for _, k := range []string{"prompt", "text", "tool_input", "tool_response", "transcript_path", "compact_summary", "error"} {
			if _, ok := ev.Payload[k]; ok {
				t.Errorf("metadata level kept %q for %s", k, name)
			}
		}
	}
	if all := meta.allFiles(); strings.Contains(all, "LANTERN") || strings.Contains(all, "note.txt") || strings.Contains(all, "codeword") {
		t.Fatal("metadata level wrote conversation text or a file path to disk")
	}
	if ev := meta.event(meta.handle("UserPromptSubmit", prompt)); ev.Payload["prompt_chars"] == nil {
		t.Error("metadata level should keep the prompt length")
	}

	conv := newRig(t, "ask", "", nil, registered("conversation"))
	ev := conv.event(conv.handle("PostToolUse", tool))
	if _, ok := ev.Payload["tool_response"]; ok {
		t.Error("conversation level kept tool output")
	}
	in, _ := ev.Payload["tool_input"].(map[string]any)
	if fp, _ := in["file_path"].(string); !strings.HasSuffix(fp, "note.txt") {
		t.Errorf("conversation level should keep the file path the tool touched: %v", ev.Payload)
	}

	ext := newRig(t, "ask", "", nil, registered("extended"))
	if ev := ext.event(ext.handle("PostToolUse", tool)); ev.Payload["tool_response"] == nil {
		t.Error("extended level should keep a tool output snippet")
	}
}

func payload(t *testing.T, fields map[string]any) []byte {
	t.Helper()
	base := map[string]any{"session_id": "s-1", "cwd": `C:\Work\App`, "hook_event_name": "UserPromptSubmit"}
	for k, v := range fields {
		base[k] = v
	}
	raw, err := json.Marshal(base)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

var appRoot = project.Root{Path: `c:\work\app`, ProjectID: "proj-app", Level: "extended"}

// The plan's secrets fixture: none of these may reach any file under the data directory.
func TestSecretsNeverReachDisk(t *testing.T) {
	const pem = "-----BEGIN RSA PRIVATE KEY-----\nMIIEowIBAAKCAQEAsyntheticKEYBODYsynthetic\n-----END RSA PRIVATE KEY-----"
	const jwt = "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJzeW50aGV0aWMifQ.c2lnbmF0dXJlLXN5bnRoZXRpYw"
	const pass = "Tr0ub4dor-synthetic-pw"
	const gh = "ghp_0123456789abcdefghijABCDEFGHIJ012345"
	wrapped := base64.StdEncoding.EncodeToString([]byte("token=" + gh))
	secrets := []string{"MIIEowIBAAKCAQEAsyntheticKEYBODY", jwt, pass, gh, wrapped, "sk-proj-Zz9Yy8Xx7Ww6Vv5Uu4Tt3Ss2Rr1Qq0Pp"}

	r := newRig(t, "ask", "", nil, appRoot)
	prompt := "deploy with this key:\n" + pem + "\nand header Authorization: Bearer " + jwt +
		"\ndb is postgres://admin:" + pass + "@db.example.test/app and blob " + wrapped
	res := r.handle("UserPromptSubmit", payload(t, map[string]any{"prompt": prompt}))
	ev := r.event(res)
	if !ev.Redaction.Applied || ev.Redaction.Count < 4 {
		t.Errorf("redaction not recorded: %+v", ev.Redaction)
	}
	r.handle("Stop", payload(t, map[string]any{"last_assistant_message": "I used " + gh + " as you asked"}))
	r.handle("PostToolUse", payload(t, map[string]any{
		"tool_name": "Bash", "tool_use_id": "t1",
		"tool_input":    map[string]any{"command": "curl -u admin:" + pass + " https://admin:" + pass + "@example.test", "description": "call api"},
		"tool_response": map[string]any{"stdout": "OPENAI_API_KEY=sk-proj-Zz9Yy8Xx7Ww6Vv5Uu4Tt3Ss2Rr1Qq0Pp"},
	}))
	r.handle("PostToolUseFailure", payload(t, map[string]any{"tool_name": "Bash", "error": "Exit code 1\nauth failed for password=" + pass}))
	r.handle("PostCompact", payload(t, map[string]any{"compact_summary": "user shared " + gh}))

	if n := len(r.spool()); n != 5 {
		t.Fatalf("spool holds %d files, want 5", n)
	}
	all := r.allFiles()
	for _, s := range secrets {
		if strings.Contains(all, s) {
			t.Errorf("secret %q was written to disk", s)
		}
	}
	if !strings.Contains(all, "deploy with this key") || !strings.Contains(all, "call api") {
		t.Error("surrounding text should survive redaction")
	}
}

func TestFieldsNotOnTheAllowlistAreNeverPersisted(t *testing.T) {
	r := newRig(t, "ask", "", nil, appRoot)
	r.handle("PostToolUse", payload(t, map[string]any{
		"tool_name": "Write", "future_field": "UNLISTED-TOP-LEVEL", "env": map[string]any{"HOME": "UNLISTED-NESTED"},
		"tool_input": map[string]any{"file_path": `C:\Work\App\a.txt`, "content": "UNLISTED-FILE-CONTENT", "new_string": "UNLISTED-EDIT"},
	}))
	all := r.allFiles()
	for _, s := range []string{"UNLISTED-TOP-LEVEL", "UNLISTED-NESTED", "UNLISTED-FILE-CONTENT", "UNLISTED-EDIT"} {
		if strings.Contains(all, s) {
			t.Errorf("%s was persisted", s)
		}
	}
	if !strings.Contains(all, `a.txt`) {
		t.Error("the allowlisted file path should be kept")
	}
}

func TestNothingIsWrittenWithoutPermission(t *testing.T) {
	prompt := payload(t, map[string]any{"prompt": "PRIVATE-PROMPT-TEXT"})
	start := payload(t, map[string]any{"source": "startup"})
	cases := []struct {
		name       string
		rig        *rig
		wantAction string
		remind     bool
	}{
		{"manual mode, unregistered", newRig(t, "manual", "", nil), "none", false},
		{"ask mode, unregistered", newRig(t, "ask", "", nil), "remind", true},
		{"paused project", newRig(t, "ask", "", nil, project.Root{Path: `c:\work\app`, ProjectID: "p", Level: "conversation", Paused: true}), "none", false},
		{"level off", newRig(t, "auto", "conversation", nil, project.Root{Path: `c:\work\app`, ProjectID: "p", Level: "off"}), "none", false},
		{"excluded, registered", newRig(t, "ask", "", []string{`c:\work`}, appRoot), "none", false},
		{"excluded, auto mode", newRig(t, "auto", "extended", []string{`c:\work\app`}), "none", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res := c.rig.handle("UserPromptSubmit", prompt)
			if res.Action != c.wantAction || len(res.Stdout) != 0 || res.SpoolFile != "" {
				t.Fatalf("prompt: %+v", res)
			}
			res = c.rig.handle("SessionStart", start)
			if c.remind != (len(res.Stdout) > 0) {
				t.Fatalf("SessionStart stdout = %q, remind wanted = %v", res.Stdout, c.remind)
			}
			if c.remind {
				var out map[string]string
				if err := json.Unmarshal(res.Stdout, &out); err != nil || !strings.Contains(out["systemMessage"], "sessionkeep project add") || len(out) != 1 {
					t.Fatalf("reminder must be a single systemMessage naming the command: %q", res.Stdout)
				}
			}
			if n := len(c.rig.spool()); n != 0 {
				t.Fatalf("%d spool files written", n)
			}
			if strings.Contains(c.rig.allFiles(), "PRIVATE-PROMPT-TEXT") {
				t.Fatal("prompt text reached disk")
			}
		})
	}
}

func TestAutoModeSpoolsARegistrationRequest(t *testing.T) {
	r := newRig(t, "auto", "metadata", nil)
	res := r.handle("UserPromptSubmit", payload(t, map[string]any{"prompt": "hello there", "cwd": `C:\Work\New\sub`}))
	ev := r.event(res)
	if res.Action != "auto_register" || ev.ProjectID != "" || ev.Register == nil || ev.Register.Root != `c:\work\new\sub` || ev.Register.Level != "metadata" || ev.Level != "metadata" {
		t.Fatalf("%+v / %+v", res, ev)
	}
	if _, ok := ev.Payload["prompt"]; ok {
		t.Fatal("auto mode at metadata level must not keep prompt text")
	}
}

func TestOversizeTextIsCutAndReported(t *testing.T) {
	r := newRig(t, "ask", "", nil, appRoot)
	big := strings.Repeat("é", MaxTextBytes) // two bytes each: twice the limit
	ev := r.event(r.handle("UserPromptSubmit", payload(t, map[string]any{"prompt": big})))
	got := ev.Payload["prompt"].(string)
	if ev.Completeness != "truncated" || len(got) > MaxTextBytes || ev.DroppedBytes != len(big)-len(got) || !strings.HasSuffix(got, "é") {
		t.Fatalf("completeness=%s kept=%d dropped=%d", ev.Completeness, len(got), ev.DroppedBytes)
	}
	if int(ev.Payload["prompt_chars"].(float64)) != MaxTextBytes {
		t.Fatalf("original length should be recorded: %v", ev.Payload["prompt_chars"])
	}
}

func TestBadInputIsIgnoredQuietly(t *testing.T) {
	r := newRig(t, "auto", "extended", nil, appRoot)
	for name, c := range map[string]struct {
		event string
		raw   []byte
	}{
		"not JSON":        {"UserPromptSubmit", []byte("{not json")},
		"empty":           {"UserPromptSubmit", nil},
		"JSON array":      {"UserPromptSubmit", []byte(`[1,2]`)},
		"no cwd":          {"UserPromptSubmit", []byte(`{"session_id":"s","prompt":"x"}`)},
		"no session":      {"UserPromptSubmit", []byte(`{"cwd":"C:\\Work\\App","prompt":"x"}`)},
		"wrong types":     {"UserPromptSubmit", []byte(`{"session_id":7,"cwd":["a"]}`)},
		"unknown event":   {"PreToolUse", payload(t, nil)},
		"empty event":     {"", payload(t, nil)},
		"relative cwd":    {"UserPromptSubmit", payload(t, map[string]any{"cwd": `Work\App`})},
		"unusual unicode": {"UserPromptSubmit", []byte("{\"session_id\":\"s\",\"cwd\":\"C:\\\\Work\\\\App\\u0000\"}")},
	} {
		res := r.handle(c.event, c.raw)
		if len(res.Stdout) != 0 || res.SpoolFile != "" {
			t.Errorf("%s: %+v", name, res)
		}
	}
	if n := len(r.spool()); n != 0 {
		t.Fatalf("%d spool files written from bad input", n)
	}
}

func TestDamagedConfigurationCapturesNothingAndRecordsAGap(t *testing.T) {
	for _, file := range []string{"settings", "roots"} {
		r := newRig(t, "ask", "", nil, appRoot)
		target := r.paths.SettingsPath()
		if file == "roots" {
			target = r.paths.RootsCachePath()
		}
		os.WriteFile(target, []byte(`{"schema_version":1,`), 0o600)
		res := r.handle("UserPromptSubmit", payload(t, map[string]any{"prompt": "PRIVATE-PROMPT-TEXT"}))
		if res.Action != "unavailable" || res.Gap == "" || len(res.Stdout) != 0 || len(r.spool()) != 0 {
			t.Fatalf("%s damaged: %+v", file, res)
		}
		log, _ := os.ReadFile(filepath.Join(r.paths.DataDir, GapLogName))
		if !strings.Contains(string(log), res.Gap) || strings.Contains(string(log), "PRIVATE-PROMPT-TEXT") {
			t.Fatalf("gap log should name the reason and hold no content: %q", log)
		}
	}
}

func TestSpoolFailureIsAGapNotAnError(t *testing.T) {
	r := newRig(t, "ask", "", nil, appRoot)
	os.RemoveAll(r.paths.SpoolDir)
	os.WriteFile(r.paths.SpoolDir, []byte("not a directory"), 0o600) // spool path is now a file
	res := r.handle("UserPromptSubmit", payload(t, map[string]any{"prompt": "PRIVATE-PROMPT-TEXT"}))
	if res.Gap != "spool_write_failed" || len(res.Stdout) != 0 || res.SpoolFile != "" {
		t.Fatalf("%+v", res)
	}
	log, _ := os.ReadFile(filepath.Join(r.paths.DataDir, GapLogName))
	if !strings.Contains(string(log), "spool_write_failed") || strings.Contains(string(log), "PRIVATE-PROMPT-TEXT") {
		t.Fatalf("gap log: %q", log)
	}
}

// With no data directory at all (SessionKeep never set up) the hook must do nothing and create nothing.
func TestMissingDataDirectoryIsLeftAlone(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "never-created")
	p, _ := config.Resolve(runtime.GOOS, func(k string) string {
		if k == config.EnvDataDir {
			return dir
		}
		return ""
	})
	res := Handle(Request{HookEvent: "UserPromptSubmit", Raw: payload(t, map[string]any{"prompt": "x"}), Paths: p,
		Env: project.Env{GOOS: "windows"}, Now: time.Now(), NewID: func() string { return "id" }})
	if res.Action != "remind" || len(res.Stdout) != 0 {
		t.Fatalf("%+v", res)
	}
	if _, err := os.Stat(dir); err == nil {
		t.Fatal("the hook created the data directory")
	}
}

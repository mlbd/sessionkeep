package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"sessionkeep/internal/summarize"
)

// These tests never run the real Claude Code program and never use the network. The "program"
// is this test binary, started through a .cmd file on Windows (a shell script elsewhere) exactly
// as the real one is started through its npm shim. It records how it was called and prints a
// canned answer. MOCK: what the real program answers is not verified here.

func TestMain(m *testing.M) {
	if out := os.Getenv("SESSIONKEEP_FAKE_DETACHED"); out != "" && len(os.Args) > 1 && os.Args[1] == "detached-probe" {
		time.Sleep(700 * time.Millisecond)
		os.WriteFile(out, []byte("done"), 0o600)
		os.Exit(0)
	}
	if mode := os.Getenv("SESSIONKEEP_FAKE_CLAUDE"); mode != "" && len(os.Args) > 1 && os.Args[1] == "-p" {
		os.Exit(fakeClaude(mode))
	}
	os.Exit(m.Run())
}

type fakeCall struct {
	Args  []string `json:"args"`
	Stdin string   `json:"stdin"`
	Guard string   `json:"guard"`
	Cwd   string   `json:"cwd"`
}

func fakeClaude(mode string) int {
	in, _ := io.ReadAll(os.Stdin)
	cwd, _ := os.Getwd()
	line, _ := json.Marshal(fakeCall{Args: os.Args[1:], Stdin: string(in), Guard: os.Getenv(summarize.GuardEnv), Cwd: cwd})
	if f, err := os.OpenFile(os.Getenv("SESSIONKEEP_FAKE_LOG"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
		f.Write(append(line, '\n'))
		f.Close()
	}
	switch mode {
	case "fail":
		fmt.Fprintln(os.Stderr, "not signed in")
		return 1
	case "garbage":
		fmt.Println("Welcome! How can I help?")
		return 0
	case "error":
		fmt.Println(`{"type":"result","subtype":"error_during_execution","is_error":true,"result":"usage limit reached"}`)
		return 1
	}
	fmt.Println(`{"type":"result","subtype":"success","is_error":false,"result":"Goal. Stop the authentication redirect. Done. Changed auth.go. The key was password=hunter2hunter2 at the time.","modelUsage":{"claude-fake-1":{"inputTokens":1}}}`)
	return 0
}

// fakeProgram points the sandbox at the stand-in and returns a function that reads its calls.
func (sb *sandbox) fakeProgram(mode string) func() []fakeCall {
	sb.t.Helper()
	// Like the real one, the stand-in sits in a folder with a space in its name.
	dir := filepath.Join(sb.t.TempDir(), "Program Files", "node js")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		sb.t.Fatal(err)
	}
	self, err := os.Executable()
	if err != nil {
		sb.t.Fatal(err)
	}
	shim, body := filepath.Join(dir, "claude.cmd"), "@echo off\r\n\""+self+"\" %*\r\n"
	if runtime.GOOS != "windows" {
		shim, body = filepath.Join(dir, "claude"), "#!/bin/sh\nexec \""+self+"\" \"$@\"\n"
	}
	if err := os.WriteFile(shim, []byte(body), 0o700); err != nil {
		sb.t.Fatal(err)
	}
	log := filepath.Join(dir, "calls.jsonl")
	// The detached process is run in-process and to completion, so a test sees its result.
	// Starting a detached process for real is covered by TestStartDetachedDoesNotWait.
	real := spawnSummarizer
	spawnSummarizer = func(getenv func(string) string) error {
		Run([]string{"summarize", "run", "--detached"}, strings.NewReader(""), io.Discard, io.Discard, getenv)
		return nil
	}
	sb.t.Cleanup(func() { spawnSummarizer = real })
	sb.env[EnvClaudeBin] = shim
	sb.t.Setenv("SESSIONKEEP_FAKE_CLAUDE", mode)
	sb.t.Setenv("SESSIONKEEP_FAKE_LOG", log)
	return func() []fakeCall {
		raw, _ := os.ReadFile(log)
		var calls []fakeCall
		for _, l := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
			var c fakeCall
			if l != "" && json.Unmarshal([]byte(l), &c) == nil {
				calls = append(calls, c)
			}
		}
		return calls
	}
}

// finished captures one session that ended: a prompt, an answer and the end of the session.
func (sb *sandbox) finished(session, cwd, prompt string) {
	sb.t.Helper()
	sb.say("UserPromptSubmit", session, cwd, prompt)
	sb.say("Stop", session, cwd, "I looked at the handler and the cookie settings and changed the file so that the loop stops. The tests pass now and nothing else was touched in this change.")
	sb.end(session, cwd)
}

func (sb *sandbox) end(session, cwd string) {
	sb.t.Helper()
	raw, _ := json.Marshal(map[string]string{"session_id": session, "cwd": cwd, "hook_event_name": "SessionEnd"})
	if code, _, errOut := sb.hook("SessionEnd", string(raw)); code != ExitOK || errOut != "" {
		sb.t.Fatalf("hook: %d %q", code, errOut)
	}
}

type runOut struct {
	Data struct {
		Written  int      `json:"written"`
		Failed   int      `json:"failed"`
		Skipped  int      `json:"skipped_changed_meanwhile"`
		LimitHit bool     `json:"daily_limit_reached"`
		Errors   []string `json:"errors"`
		Sessions []struct {
			ProjectName string `json:"project_name"`
		} `json:"sessions"`
	} `json:"data"`
}

func (sb *sandbox) summarize(wantExit int, args ...string) runOut {
	sb.t.Helper()
	code, out, errOut := sb.run(append([]string{"summarize", "run", "--json"}, args...)...)
	if code != wantExit {
		sb.t.Fatalf("summarize run %v: exit %d, want %d\n%s%s", args, code, wantExit, out, errOut)
	}
	var r runOut
	json.NewDecoder(strings.NewReader(out)).Decode(&r)
	return r
}

type summaryHits struct {
	Data struct {
		Hits      []struct{ ID string }
		Summaries []struct {
			SessionID string `json:"session_id"`
			Snippet   string `json:"snippet"`
		} `json:"summaries"`
	} `json:"data"`
}

func (sb *sandbox) found(args ...string) summaryHits {
	sb.t.Helper()
	var out summaryHits
	raw := sb.ok(append([]string{"memory", "search", "--json"}, args...)...)
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		sb.t.Fatalf("bad JSON: %v: %q", err, raw)
	}
	return out
}

const loopPrompt = "The login page keeps sending people back to itself after they sign in. LOOPMARK. Please find out why and fix it without changing anything else."

func TestSummariesStayOffUntilTheOwnerConfirms(t *testing.T) {
	sb := newSandbox(t)
	app := sb.dir("app")
	sb.ok("project", "add", app, "--capture", "conversation", "--name", "App")
	sb.finished("s1", app, loopPrompt)
	sb.ok("ingest")
	calls := sb.fakeProgram("ok")

	if code, _, errOut := sb.run("summarize", "run"); code != ExitFailure || !strings.Contains(errOut, "summaries are off") {
		t.Fatalf("run while off: exit %d, %s", code, errOut)
	}
	code, out, _ := sb.run("summarize", "on", "--model", "opus")
	if code != ExitNeedsConsent || !strings.Contains(out, "NOTHING CHANGED") || !strings.Contains(out, "Model: opus") || !strings.Contains(out, "sends it to Anthropic") {
		t.Fatalf("on without --yes: exit %d\n%s", code, out)
	}
	if out := sb.ok("summarize", "status"); !strings.Contains(out, "summaries: off") {
		t.Errorf("status after an unconfirmed on: %s", out)
	}
	// The background drain must not summarise either.
	if code, _, _ := sb.run("ingest", "--quiet"); code != ExitOK {
		t.Fatal("quiet ingest failed")
	}
	if n := len(calls()); n != 0 {
		t.Fatalf("the program was started %d time(s) while summaries were off", n)
	}
	for _, bad := range [][]string{{"summarize"}, {"summarize", "on", "--model", "a b", "--yes"}, {"summarize", "on", "--daily-limit", "0", "--yes"}, {"summarize", "run", "--model", "opus"}, {"summarize", "nonsense"}} {
		if code, _, _ := sb.run(bad...); code != ExitUsage {
			t.Errorf("%v: exit %d, want usage", bad, code)
		}
	}
}

func TestSummarizeWritesOneSummaryPerFinishedSession(t *testing.T) {
	sb := newSandbox(t)
	app, other, paused := sb.dir("app"), sb.dir("other"), sb.dir("paused")
	sb.ok("project", "add", app, "--capture", "conversation", "--name", "App")
	sb.ok("project", "add", other, "--capture", "conversation", "--name", "Other")
	sb.ok("project", "add", paused, "--capture", "conversation", "--name", "Paused")
	sb.finished("s1", app, loopPrompt)
	sb.say("UserPromptSubmit", "s-open", app, "OPENMARK this session is still going on, so it must not be summarised yet. "+strings.Repeat("More words. ", 20))
	sb.say("Stop", "s-open", app, "Still working on it.")
	sb.finished("s-paused", paused, "PAUSEDMARK "+loopPrompt)
	sb.ok("ingest")
	sb.ok("project", "pause", paused)
	calls := sb.fakeProgram("ok")
	sb.ok("summarize", "on", "--model", "opus", "--manual", "--yes")

	// A dry run names the session and sends nothing.
	if dry := sb.summarize(ExitOK, "--dry-run"); len(dry.Data.Sessions) != 1 || dry.Data.Sessions[0].ProjectName != "App" || len(calls()) != 0 {
		t.Fatalf("dry run = %+v, calls %d", dry.Data, len(calls()))
	}

	if r := sb.summarize(ExitOK); r.Data.Written != 1 || r.Data.Failed != 0 {
		t.Fatalf("run = %+v", r.Data)
	}
	got := calls()
	if len(got) != 1 {
		t.Fatalf("the program was started %d time(s), want 1 (only the finished session of the active project)", len(got))
	}
	c := got[0]
	args := strings.Join(c.Args, "\x00") + "\x00"
	for _, want := range []string{"-p\x00", "--model\x00opus\x00", "--safe-mode\x00", "--tools\x00\x00", "--no-session-persistence\x00", "--output-format\x00json\x00", "--system-prompt\x00" + summarize.System + "\x00"} {
		if !strings.Contains(args, want) {
			t.Errorf("the program did not receive %q; got %q", want, c.Args)
		}
	}
	if !strings.Contains(c.Stdin, "BEGIN SESSION") || !strings.Contains(c.Stdin, "LOOPMARK") || strings.Contains(c.Stdin, "PAUSEDMARK") || strings.Contains(c.Stdin, "OPENMARK") {
		t.Errorf("wrong text was given to the program:\n%s", c.Stdin)
	}
	if strings.Contains(args, "LOOPMARK") {
		t.Error("session text was put on the command line")
	}
	if c.Guard != "1" || filepath.Base(c.Cwd) != "summarizer" {
		t.Errorf("guard=%q cwd=%q; want the guard set and the empty start folder", c.Guard, c.Cwd)
	}

	// The summary is stored with its model, redacted, and shown apart from the original text.
	var s1 string
	for _, s := range sb.sessions("--project", app) {
		if strings.Contains(s.FirstPrompt, "LOOPMARK") {
			s1 = s.ID
		}
	}
	shown := sb.ok("session", "show", s1, "--project", app)
	if !strings.Contains(shown, "SUMMARY written by a model (claude-fake-1") || !strings.Contains(shown, "from 2 of 2 piece(s)") || !strings.Contains(shown, "Changed auth.go") {
		t.Fatalf("the session does not show its summary:\n%s", shown)
	}
	if strings.Contains(shown, "hunter2hunter2") {
		t.Error("a secret in the model's answer was stored")
	}

	// "authentication" appears only in the summary: the search finds the session through it,
	// labelled, and only inside the project.
	f := sb.found("authentication", "--project", app)
	if len(f.Data.Hits) != 0 || len(f.Data.Summaries) != 1 || f.Data.Summaries[0].SessionID != s1 {
		t.Fatalf("search = %+v", f.Data)
	}
	if out := sb.ok("memory", "search", "authentication", "--project", app); !strings.Contains(out, "written by a model, not original text") || !strings.Contains(out, "not found: nothing stored") {
		t.Errorf("search output does not label the summary or hides that no original text matched:\n%s", out)
	}
	if f := sb.found("authentication", "--project", other); len(f.Data.Summaries) != 0 {
		t.Errorf("a summary crossed into another project: %+v", f.Data)
	}
	if f := sb.found("authentication", "--project", app, "--who", "user"); len(f.Data.Summaries) != 0 {
		t.Errorf("a summary was returned for a search limited to what the user said: %+v", f.Data)
	}

	// Nothing new: no second request.
	if r := sb.summarize(ExitOK); r.Data.Written != 0 || len(calls()) != 1 {
		t.Fatalf("second run = %+v, calls %d", r.Data, len(calls()))
	}
	// The session continues and ends again: its summary is out of date and is replaced.
	sb.say("UserPromptSubmit", "s1", app, "One more thing in the same session.")
	sb.end("s1", app)
	sb.ok("ingest")
	if r := sb.summarize(ExitOK); r.Data.Written != 1 || len(calls()) != 2 {
		t.Fatalf("run after new text = %+v, calls %d", r.Data, len(calls()))
	}
	if shown := sb.ok("session", "show", s1, "--project", app); strings.Count(shown, "SUMMARY written by a model") != 1 || !strings.Contains(shown, "from 3 of 3 piece(s)") {
		t.Errorf("the summary was not replaced:\n%s", shown)
	}
	if out := sb.ok("summarize", "status"); !strings.Contains(out, "stored: 1;") || !strings.Contains(out, "(2 made today)") {
		t.Errorf("status: %s", out)
	}

	// Deleting any text of the session deletes the summary written from it.
	hit := sb.search("LOOPMARK", "--project", app).Data.Hits[0].ID
	sb.ok("forget", "record", hit, "--project", app, "--yes")
	if shown := sb.ok("session", "show", s1, "--project", app); strings.Contains(shown, "SUMMARY") {
		t.Errorf("the summary outlived text it was written from:\n%s", shown)
	}
	if f := sb.found("authentication", "--project", app); len(f.Data.Summaries) != 0 {
		t.Errorf("a deleted summary is still found: %+v", f.Data)
	}
}

func TestSummarizeFailuresStoreNothingAndStopEarly(t *testing.T) {
	for _, mode := range []string{"fail", "garbage", "error"} {
		t.Run(mode, func(t *testing.T) {
			sb := newSandbox(t)
			app := sb.dir("app")
			sb.ok("project", "add", app, "--capture", "conversation", "--name", "App")
			for i := 0; i < 3; i++ {
				sb.finished(fmt.Sprintf("s%d", i), app, loopPrompt)
			}
			sb.ok("ingest")
			calls := sb.fakeProgram(mode)
			sb.ok("summarize", "on", "--yes")
			r := sb.summarize(ExitFailure)
			if r.Data.Written != 0 || r.Data.Failed != 2 || len(r.Data.Errors) != 2 || len(calls()) != 2 {
				t.Fatalf("run = %+v, calls %d; want 2 failures and then a stop", r.Data, len(calls()))
			}
			if mode == "error" && !strings.Contains(r.Data.Errors[0], "usage limit reached") {
				t.Errorf("the program's own explanation was lost: %v", r.Data.Errors)
			}
			if out := sb.ok("summarize", "status"); !strings.Contains(out, "stored: 0;") {
				t.Errorf("something was stored after a failure: %s", out)
			}
		})
	}
}

func TestSummarizeKeepsToTheDailyLimit(t *testing.T) {
	sb := newSandbox(t)
	app := sb.dir("app")
	sb.ok("project", "add", app, "--capture", "conversation", "--name", "App")
	sb.finished("s1", app, loopPrompt)
	sb.finished("s2", app, loopPrompt)
	sb.say("UserPromptSubmit", "s-thin", app, "hi")
	sb.say("Stop", "s-thin", app, "hello")
	sb.end("s-thin", app)
	sb.ok("ingest")
	calls := sb.fakeProgram("ok")
	sb.ok("summarize", "on", "--daily-limit", "1", "--yes")
	if r := sb.summarize(ExitOK); r.Data.Written != 1 || !r.Data.LimitHit || len(calls()) != 1 {
		t.Fatalf("first run = %+v, calls %d", r.Data, len(calls()))
	}
	if r := sb.summarize(ExitOK); r.Data.Written != 0 || !r.Data.LimitHit || len(calls()) != 1 {
		t.Fatalf("second run = %+v, calls %d", r.Data, len(calls()))
	}
	// A higher limit lets the other session through; the two-word session is never sent.
	sb.ok("summarize", "on", "--daily-limit", "10", "--yes")
	if r := sb.summarize(ExitOK); r.Data.Written != 1 || len(calls()) != 2 {
		t.Fatalf("third run = %+v, calls %d", r.Data, len(calls()))
	}
	for _, c := range calls() {
		if strings.Contains(c.Stdin, "hello") {
			t.Error("a session with almost no text was sent")
		}
	}
	// Deleting summaries needs confirmation and leaves the text alone.
	if code, out, _ := sb.run("summarize", "delete"); code != ExitNeedsConsent || !strings.Contains(out, "NOTHING DELETED") {
		t.Fatalf("delete without --yes: %d %s", code, out)
	}
	if out := sb.ok("summarize", "delete", "--yes"); !strings.Contains(out, "Deleted 2") {
		t.Errorf("delete: %s", out)
	}
	if hits := sb.search("LOOPMARK", "--project", app).Data.Hits; len(hits) != 2 {
		t.Errorf("deleting summaries touched the original text: %d hits", len(hits))
	}
}

func TestBackgroundSummariesRunByThemselvesAndBackOffAfterAFailure(t *testing.T) {
	sb := newSandbox(t)
	app := sb.dir("app")
	sb.ok("project", "add", app, "--capture", "conversation", "--name", "App")
	sb.finished("s1", app, loopPrompt)
	sb.finished("s2", app, loopPrompt)
	sb.ok("ingest")
	calls := sb.fakeProgram("fail")
	sb.ok("summarize", "on", "--manual", "--yes") // on, but only by hand
	sb.run("ingest", "--quiet")
	if len(calls()) != 0 {
		t.Fatal("the background drain summarised although the owner chose --manual")
	}
	sb.ok("summarize", "on", "--yes") // automatic is what "on" means unless --manual is given
	sb.run("ingest", "--quiet")
	sb.run("ingest", "--quiet")
	if n := len(calls()); n != 2 {
		t.Fatalf("after a failed background run the program was started %d time(s), want 2 (two failures stop a run, then a pause)", n)
	}
	// With the pause over and the program working, each drain writes one summary.
	t.Setenv("SESSIONKEEP_FAKE_CLAUDE", "ok")
	paths, _ := resolvePaths(sb)
	if err := os.Remove(filepath.Join(paths, "summarize.attempt")); err != nil {
		t.Fatal(err)
	}
	sb.run("ingest", "--quiet")
	if out := sb.ok("summarize", "status"); !strings.Contains(out, "stored: 2;") || len(calls()) != 4 {
		t.Fatalf("background run: %s (calls %d)", out, len(calls()))
	}
	if _, err := os.Stat(filepath.Join(paths, "summarize.attempt")); err == nil {
		t.Error("the attempt marker was left behind after a clean run")
	}
}

func resolvePaths(sb *sandbox) (string, error) {
	var out struct {
		Data struct {
			CacheDir string `json:"cache_dir"`
		} `json:"data"`
	}
	err := json.Unmarshal([]byte(sb.ok("paths", "--json")), &out)
	return out.Data.CacheDir, err
}

func TestHooksDoNothingInsideASummaryCall(t *testing.T) {
	sb := newSandbox(t)
	app := sb.dir("app")
	sb.ok("project", "add", app, "--capture", "conversation", "--name", "App")
	sb.env[summarize.GuardEnv] = "1"
	raw, _ := json.Marshal(map[string]string{"session_id": "inner", "cwd": app, "hook_event_name": "UserPromptSubmit", "prompt": "BEGIN SESSION ... END SESSION"})
	if code, out, _ := sb.hook("UserPromptSubmit", string(raw)); code != ExitOK || out != "" {
		t.Fatalf("hook: %d %q", code, out)
	}
	if n := sb.spoolCount(); n != 0 {
		t.Fatalf("a summary call was captured: %d spooled event(s)", n)
	}
}

func TestStartDetachedDoesNotWait(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "probe.txt")
	began := time.Now()
	if err := startDetached(self, []string{"detached-probe"}, append(os.Environ(), "SESSIONKEEP_FAKE_DETACHED="+out)); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(began); d > 500*time.Millisecond {
		t.Fatalf("starting the detached process took %s; it must not wait for it", d)
	}
	if _, err := os.Stat(out); err == nil {
		t.Fatal("the process had already finished when the call returned")
	}
	for deadline := time.Now().Add(15 * time.Second); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		if raw, err := os.ReadFile(out); err == nil && string(raw) == "done" {
			return
		}
	}
	t.Fatal("the detached process never did its work")
}

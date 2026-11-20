package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type accountsOut struct {
	Data struct {
		Accounts []struct {
			Name      string `json:"name"`
			ConfigDir string `json:"config_dir"`
			Command   string `json:"command"`
			Default   bool   `json:"claude_code_default"`
			Chosen    bool   `json:"chosen"`
			Current   bool   `json:"in_use_here"`
		} `json:"accounts"`
		Uses    string `json:"uses"`
		Decided bool   `json:"decided"`
	} `json:"data"`
}

func (sb *sandbox) accounts() accountsOut {
	sb.t.Helper()
	var out accountsOut
	if err := json.Unmarshal([]byte(sb.ok("account", "list", "--json")), &out); err != nil {
		sb.t.Fatal(err)
	}
	return out
}

// twoAccounts gives the sandbox user Claude Code's default folder, a second sign-in started with
// a short command "work", a third without any short command, and look-alike folders that are
// not accounts.
func (sb *sandbox) twoAccounts() (def, work, solo string) {
	sb.t.Helper()
	home := sb.env["HOME"]
	def, work, solo = filepath.Join(home, ".claude"), filepath.Join(home, ".claude-work"), filepath.Join(home, ".claude_solo")
	for _, d := range []string{filepath.Join(def, "projects"), filepath.Join(work, "projects"), filepath.Join(solo, "projects"),
		filepath.Join(home, ".claude-mem"), filepath.Join(home, ".claude-workshop-notes")} { // the last two hold no sessions: not accounts
		if err := os.MkdirAll(d, 0o700); err != nil {
			sb.t.Fatal(err)
		}
	}
	bin := filepath.Join(home, "my bin")
	os.MkdirAll(bin, 0o700)
	scripts := map[string]string{
		"work.bat":     "@echo off\r\nset CLAUDE_CONFIG_DIR=%USERPROFILE%\\.claude-work\r\nclaude %*\r\n",
		"workshop.bat": "@echo off\r\nset CLAUDE_CONFIG_DIR=%USERPROFILE%\\.claude-workshop-notes\r\nclaude %*\r\n", // must not be taken for "work"
		"notes.bat":    "@echo off\r\nrem mentions .claude-work but sets nothing\r\n",
	}
	if runtime.GOOS != "windows" {
		scripts = map[string]string{
			"work":     "#!/bin/sh\nCLAUDE_CONFIG_DIR=\"$HOME/.claude-work\" exec claude \"$@\"\n",
			"workshop": "#!/bin/sh\nCLAUDE_CONFIG_DIR=\"$HOME/.claude-workshop-notes\" exec claude \"$@\"\n",
			"notes":    "#!/bin/sh\n# mentions .claude-work but sets nothing\n",
		}
	}
	for name, body := range scripts {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(body), 0o700); err != nil {
			sb.t.Fatal(err)
		}
	}
	sb.env["PATH"] = filepath.Join(home, "empty") + string(os.PathListSeparator) + bin
	return def, work, solo
}

func TestOneAccountNeedsNoChoice(t *testing.T) {
	sb := newSandbox(t)
	if err := os.MkdirAll(filepath.Join(sb.env["HOME"], ".claude", "projects"), 0o700); err != nil {
		t.Fatal(err)
	}
	a := sb.accounts()
	if len(a.Data.Accounts) != 1 || !a.Data.Decided || a.Data.Uses != "claude" || a.Data.Accounts[0].Command != "claude" {
		t.Fatalf("accounts = %+v", a.Data)
	}
	_ = sb.fakeProgram("ok")
	sb.ok("summarize", "on", "--yes") // no question about accounts
	if out := sb.ok("install"); !strings.Contains(out, "claude plugin marketplace add") || strings.Contains(out, "accounts") {
		t.Errorf("install output with one account:\n%s", out)
	}
}

func TestSeveralAccountsAreFoundAndTheOwnerChooses(t *testing.T) {
	sb := newSandbox(t)
	_, work, solo := sb.twoAccounts()
	a := sb.accounts()
	if len(a.Data.Accounts) != 3 || a.Data.Decided {
		t.Fatalf("accounts = %+v; want claude, work and solo, undecided", a.Data)
	}
	byName := map[string]string{}
	for _, x := range a.Data.Accounts {
		byName[x.Name] = x.Command
		if x.Chosen || (x.Name == "claude") != x.Default || (x.Name == "claude") != x.Current {
			t.Errorf("account %+v", x)
		}
	}
	if byName["claude"] != "claude" || byName["work"] != "work" || byName["solo"] != "" {
		t.Fatalf("commands found = %v; want work started with \"work\" and none for solo", byName)
	}

	// Spending a plan is refused until the owner says whose.
	calls := sb.fakeProgram("ok")
	if code, _, errOut := sb.run("summarize", "on", "--yes"); code != ExitFailure || !strings.Contains(errOut, "sessionkeep account use NAME") {
		t.Fatalf("summarize on with several accounts and no choice: %d %s", code, errOut)
	}

	// In a terminal started with the second account, printed commands use its short command.
	sb.env["CLAUDE_CONFIG_DIR"] = work
	out := sb.ok("install")
	if !strings.Contains(out, "work plugin marketplace add") || !strings.Contains(out, "work plugin install sessionkeep@sessionkeep-local") || !strings.Contains(out, "work --plugin-dir") || strings.Contains(out, "claude plugin") {
		t.Errorf("install did not print the second account's command:\n%s", out)
	}
	if !strings.Contains(out, "3 Claude Code accounts") || !strings.Contains(out, "sessionkeep account use NAME") {
		t.Errorf("install does not say that a choice exists:\n%s", out)
	}
	delete(sb.env, "CLAUDE_CONFIG_DIR")

	// Choosing by the short command, by name, by folder; an unknown name is refused.
	if code, _, errOut := sb.run("account", "use", "nobody"); code != ExitFailure || !strings.Contains(errOut, "claude, work, solo") {
		t.Fatalf("unknown account: %d %s", code, errOut)
	}
	sb.ok("account", "use", solo)
	if out := sb.ok("install"); !strings.Contains(out, "CLAUDE_CONFIG_DIR") || !strings.Contains(out, solo) || !strings.Contains(out, "claude plugin marketplace add") {
		t.Errorf("an account without a short command needs the line that selects its folder:\n%s", out)
	}
	sb.ok("account", "use", "WORK")
	if a := sb.accounts(); !a.Data.Decided || a.Data.Uses != "work" {
		t.Fatalf("after choosing: %+v", a.Data)
	}
	if out := sb.ok("uninstall", "--yes"); !strings.Contains(out, "work plugin uninstall sessionkeep@sessionkeep-local") {
		t.Errorf("uninstall did not print the chosen account's command:\n%s", out)
	}

	// Summaries now run under the chosen account, whatever the surrounding process runs with.
	app := sb.dir("app")
	sb.ok("project", "add", app, "--capture", "conversation", "--name", "App")
	sb.finished("s1", app, loopPrompt)
	sb.finished("s2", app, loopPrompt)
	sb.ok("ingest")
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(t.TempDir(), "some other account"))
	if out := sb.ok("summarize", "on", "--manual", "--yes"); !strings.Contains(out, `account`) || !strings.Contains(out, `"work"`) {
		t.Errorf("summarize on does not name the account that pays:\n%s", out)
	}
	sb.summarize(ExitOK, "--limit", "1")
	if c := calls(); len(c) != 1 || !sameDir(c[0].Config, work) {
		t.Fatalf("the summary ran with configuration folder %q, want %q", c[0].Config, work)
	}
	sb.ok("account", "use", "default")
	sb.summarize(ExitOK, "--limit", "1")
	if c := calls(); len(c) != 2 || c[1].Config != "" {
		t.Fatalf("with Claude Code's default account chosen, the summary ran with configuration folder %q, want none", c[1].Config)
	}

	// History is imported from the chosen account's folder without being told where.
	sb.ok("account", "use", "work")
	line, _ := json.Marshal(map[string]any{"type": "user", "isSidechain": false, "cwd": app, "sessionId": "old-work", "uuid": "u1",
		"timestamp": "2026-10-01T09:00:00Z", "message": map[string]any{"role": "user", "content": "WORKOLD question from the second account."}})
	dir := filepath.Join(work, "projects", "x")
	os.MkdirAll(dir, 0o700)
	if err := os.WriteFile(filepath.Join(dir, "old-work.jsonl"), append(line, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if pre := sb.history(ExitNeedsConsent); pre.Data.Turns != 1 {
		t.Fatalf("history preview = %+v, want the one turn in the chosen account's folder", pre.Data)
	}

	sb.ok("account", "auto")
	if a := sb.accounts(); a.Data.Decided {
		t.Errorf("after `account auto` a choice is still recorded: %+v", a.Data)
	}
}

// MOCK: the stand-in for Claude Code remembers in which configuration folder the plugin was
// installed. What the real program does was tried by hand (evidence/npm/N-A.md).
func TestThePluginIsTurnedOnAndOffPerAccount(t *testing.T) {
	sb, _ := installSandbox(t)
	home := sb.env["USERPROFILE"]
	work := filepath.Join(home, ".claude-work")
	for _, d := range []string{filepath.Join(home, ".claude", "projects"), filepath.Join(work, "projects")} {
		os.MkdirAll(d, 0o700)
	}
	calls := sb.fakeProgram("ok")
	states := func() map[string]string {
		var out struct {
			Data struct {
				Accounts []struct{ Name, Plugin string }
			}
		}
		if err := json.Unmarshal([]byte(sb.ok("account", "plugins", "--json")), &out); err != nil {
			t.Fatal(err)
		}
		m := map[string]string{}
		for _, a := range out.Data.Accounts {
			m[a.Name] = a.Plugin
		}
		return m
	}

	if code, _, errOut := sb.run("account", "enable", "work"); code != ExitFailure || !strings.Contains(errOut, "not installed yet") {
		t.Fatalf("enable before install: exit=%d %q", code, errOut)
	}
	sb.ok("install")
	if s := states(); s["claude"] != "off" || s["work"] != "off" {
		t.Fatalf("before anything was turned on: %v", s)
	}
	if out := sb.ok("account", "enable", "work"); !strings.Contains(out, "shares the same memory") || !strings.Contains(out, "Restart Claude Code") {
		t.Errorf("enable output: %q", out)
	}
	if s := states(); s["claude"] != "off" || s["work"] != "on" {
		t.Fatalf("turning one account on must not touch the other: %v", s)
	}
	sb.ok("account", "enable", "claude")
	if s := states(); s["claude"] != "on" || s["work"] != "on" {
		t.Fatalf("both on: %v", s)
	}
	// The account SessionKeep uses for summaries is a separate choice and is not changed by this.
	if a := sb.accounts(); a.Data.Decided {
		t.Errorf("turning the plugin on chose an account: %+v", a.Data)
	}
	sb.ok("account", "disable", "work")
	if s := states(); s["claude"] != "on" || s["work"] != "off" {
		t.Fatalf("after turning one off: %v", s)
	}
	before := len(calls())
	sb.ok("account", "disable", "work") // already off: asks, but uninstalls nothing
	for _, c := range calls()[before:] {
		if len(c.Args) > 1 && c.Args[1] == "uninstall" {
			t.Error("an account that is already off was uninstalled again")
		}
	}
	if code, _, errOut := sb.run("account", "enable", "nobody"); code != ExitFailure || !strings.Contains(errOut, "found: claude, work") {
		t.Fatalf("unknown account: exit=%d %q", code, errOut)
	}

	t.Setenv("SESSIONKEEP_FAKE_CLAUDE", "fail")
	if s := states(); s["work"] != "unknown" {
		t.Fatalf("when Claude Code cannot be asked the state must be unknown, not a guess: %v", s)
	}
	if code, _, errOut := sb.run("account", "enable", "work"); code != ExitFailure || !strings.Contains(errOut, "not signed in") {
		t.Fatalf("a failed enable: exit=%d %q", code, errOut)
	}
}

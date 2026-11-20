package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"sessionkeep/internal/config"
	"sessionkeep/internal/install"
	"sessionkeep/internal/plugin"
	"sessionkeep/internal/summarize"
)

// pluginID is the name Claude Code knows the generated plugin by.
const pluginID = plugin.Name + "@" + install.MarketplaceName

// claudeFor runs the Claude Code program for one account and returns what it printed on
// standard output. It is used only for Claude Code's own `plugin` commands.
func claudeFor(inv invocation, acct account, args ...string) ([]byte, error) {
	bin, err := claudeProgram(inv.getenv)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd, err := summarize.Command(ctx, bin, args)
	if err != nil {
		return nil, err
	}
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	cmd.Env = accountEnv(acct, os.Environ())
	what := "`claude " + strings.Join(args[:min(len(args), 3)], " ") + "`"
	err = cmd.Run()
	if ctx.Err() != nil {
		return nil, fmt.Errorf("%s did not finish within two minutes", what)
	}
	if err != nil {
		msg := strings.TrimSpace(errOut.String() + " " + out.String())
		if len(msg) > 400 {
			msg = msg[:400]
		}
		return nil, fmt.Errorf("%s failed (%v): %s", what, err, msg)
	}
	return out.Bytes(), nil
}

// pluginState asks Claude Code whether the SessionKeep plugin is turned on for an account:
// "on", "off", or "unknown" with the reason when Claude Code could not be asked.
func pluginState(inv invocation, acct account) (state, why string) {
	out, err := claudeFor(inv, acct, "plugin", "list", "--json")
	if err != nil {
		return "unknown", err.Error()
	}
	var list []struct {
		ID      string `json:"id"`
		Enabled bool   `json:"enabled"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(out), &list); err != nil {
		return "unknown", "Claude Code's answer was not the expected list"
	}
	for _, p := range list {
		if p.ID == pluginID && p.Enabled {
			return "on", ""
		}
	}
	return "off", ""
}

// findAccount matches what the owner typed against a name, a short command, a folder or "default".
func findAccount(list []account, want string) *account {
	for i, a := range list {
		if strings.EqualFold(a.Name, want) || (a.Command != "" && strings.EqualFold(a.Command, want)) || sameDir(a.ConfigDir, want) || (a.Default && strings.EqualFold(want, "default")) {
			return &list[i]
		}
	}
	return nil
}

func accountNames(list []account) string {
	var names []string
	for _, a := range list {
		names = append(names, a.Name)
	}
	return strings.Join(names, ", ")
}

// A computer can hold more than one Claude Code sign-in. Each lives in its own configuration
// folder; Claude Code uses the one named by CLAUDE_CONFIG_DIR, or ~/.claude when that is not
// set. People usually reach a second one through a small script ("work" instead of
// "claude") that sets the variable and starts Claude Code.
//
// SessionKeep needs to know which one to use in three places: whose plan pays for summaries, where
// earlier sessions are imported from, and which command to print when it tells the user what to
// run. It never reads sign-in files; it only looks at folder names and the dates of use.

type account struct {
	Name      string `json:"name"`
	ConfigDir string `json:"config_dir"`
	Command   string `json:"command"`             // what the user types to start Claude Code with it; "" if unknown
	Default   bool   `json:"claude_code_default"` // ~/.claude
	Chosen    bool   `json:"chosen"`              // the owner picked it with `sessionkeep account use`
	Current   bool   `json:"in_use_here"`         // this terminal or session is running with it
	LastUsed  string `json:"last_used,omitempty"` // date of the last prompt typed with it
	Plugin    string `json:"plugin,omitempty"`    // "on", "off" or "unknown"; filled only by `account plugins`
	PluginWhy string `json:"plugin_note,omitempty"`
}

func homeDir(getenv func(string) string) string {
	if runtime.GOOS == "windows" {
		return getenv("USERPROFILE")
	}
	return getenv("HOME")
}

func sameDir(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// accountName turns a folder name into a short name: ".claude" is "claude", ".claude-work" is "work".
func accountName(dir string) string {
	n := strings.TrimLeft(strings.TrimPrefix(filepath.Base(dir), ".claude"), "-_. ")
	if n == "" || !strings.HasPrefix(filepath.Base(dir), ".claude") {
		if filepath.Base(dir) == ".claude" {
			return "claude"
		}
		return filepath.Base(dir)
	}
	return n
}

// wrapperFor looks on PATH for a small script that sets CLAUDE_CONFIG_DIR to dir and returns its
// name without extension. It reads only files under 4 KB that look like scripts.
func wrapperFor(getenv func(string) string, dir string) string {
	base := strings.ToLower(filepath.Base(dir))
	for _, p := range filepath.SplitList(getenv("PATH")) {
		entries, err := os.ReadDir(p)
		if err != nil {
			continue
		}
		for _, e := range entries {
			ext := strings.ToLower(filepath.Ext(e.Name()))
			if e.IsDir() || (runtime.GOOS == "windows" && ext != ".bat" && ext != ".cmd" && ext != ".ps1") || (runtime.GOOS != "windows" && ext != "" && ext != ".sh") {
				continue
			}
			if fi, err := e.Info(); err != nil || fi.Size() > 4096 {
				continue
			}
			raw, err := os.ReadFile(filepath.Join(p, e.Name()))
			if err != nil {
				continue
			}
			text := strings.ToLower(string(raw))
			if i := strings.Index(text, "claude_config_dir"); i >= 0 && strings.Contains(text, base) {
				// The folder name must end there, so ".claude" does not match ".claude-work".
				j := strings.Index(text, base) + len(base)
				if j >= len(text) || !strings.ContainsRune("-_.abcdefghijklmnopqrstuvwxyz0123456789", rune(text[j])) {
					return strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
				}
			}
		}
	}
	return ""
}

// accounts lists the Claude Code configuration folders found for this user: ~/.claude, every
// ~/.claude* folder that holds a "projects" folder, the one this process runs with, and the one
// the owner chose.
func accounts(inv invocation, st state) []account {
	home := homeDir(inv.getenv)
	current, chosen := inv.getenv("CLAUDE_CONFIG_DIR"), st.settings.ClaudeAccount
	var dirs []string
	add := func(d string) {
		for _, x := range dirs {
			if sameDir(x, d) {
				return
			}
		}
		dirs = append(dirs, filepath.Clean(d))
	}
	if home != "" {
		if entries, err := os.ReadDir(home); err == nil {
			for _, e := range entries {
				if !e.IsDir() || !strings.HasPrefix(e.Name(), ".claude") {
					continue
				}
				if fi, err := os.Stat(filepath.Join(home, e.Name(), "projects")); err == nil && fi.IsDir() {
					add(filepath.Join(home, e.Name()))
				}
			}
		}
	}
	if current != "" && filepath.IsAbs(current) {
		add(current)
	}
	if chosen.Chosen && chosen.ConfigDir != "" {
		add(chosen.ConfigDir)
	}
	sort.Strings(dirs)
	def := filepath.Join(home, ".claude")
	out := []account{}
	for _, d := range dirs {
		a := account{Name: accountName(d), ConfigDir: d, Default: home != "" && sameDir(d, def)}
		a.Chosen = chosen.Chosen && ((chosen.ConfigDir == "" && a.Default) || (chosen.ConfigDir != "" && sameDir(chosen.ConfigDir, d)))
		a.Current = (current == "" && a.Default) || (current != "" && sameDir(current, d))
		if a.Default {
			a.Command = "claude"
		} else {
			a.Command = wrapperFor(inv.getenv, d)
		}
		if fi, err := os.Stat(filepath.Join(d, "history.jsonl")); err == nil {
			a.LastUsed = fi.ModTime().UTC().Format("2006-01-02")
		}
		out = append(out, a)
	}
	return out
}

// effectiveAccount is the account SessionKeep acts for: the one the owner chose, else the one this
// process runs with, else Claude Code's default. ok is false when the owner has not chosen and
// more than one exists, which is when a command that spends the plan should ask first.
func effectiveAccount(inv invocation, st state) (a account, decided bool) {
	list := accounts(inv, st)
	for _, x := range list {
		if x.Chosen {
			return x, true
		}
	}
	for _, x := range list {
		if x.Current {
			return x, len(list) <= 1
		}
	}
	home := homeDir(inv.getenv)
	return account{Name: "claude", ConfigDir: filepath.Join(home, ".claude"), Command: "claude", Default: true}, len(list) <= 1
}

// accountEnv returns env adjusted so that a Claude Code process started with it uses account a.
func accountEnv(a account, env []string) []string {
	out := make([]string, 0, len(env)+1)
	for _, kv := range env {
		if k, _, _ := strings.Cut(kv, "="); !strings.EqualFold(k, "CLAUDE_CONFIG_DIR") {
			out = append(out, kv)
		}
	}
	if !a.Default {
		out = append(out, "CLAUDE_CONFIG_DIR="+a.ConfigDir)
	}
	return out
}

// claudeLines writes commands for the user to run, in the form that reaches the right account:
// with their own short command when one was found, otherwise with a line that selects the
// configuration folder first. Each line is given as it would be typed after "claude".
func claudeLines(a account, indent string, lines ...string) string {
	var b strings.Builder
	cmd := a.Command
	if cmd == "" {
		cmd = "claude"
		if runtime.GOOS == "windows" {
			fmt.Fprintf(&b, "%s$env:CLAUDE_CONFIG_DIR = \"%s\"    (PowerShell; this selects the \"%s\" account for this window)\n", indent, a.ConfigDir, a.Name)
		} else {
			fmt.Fprintf(&b, "%sexport CLAUDE_CONFIG_DIR=\"%s\"    (selects the \"%s\" account for this terminal)\n", indent, a.ConfigDir, a.Name)
		}
	}
	for _, l := range lines {
		fmt.Fprintf(&b, "%s%s %s\n", indent, cmd, l)
	}
	return b.String()
}

// accountCmd is `sessionkeep account list | use NAME_OR_FOLDER | auto`.
func accountCmd(inv invocation, rest []string) error {
	usageText := `usage: sessionkeep account list
       sessionkeep account use NAME_OR_FOLDER   Use this Claude Code account for summaries, history import and printed commands
       sessionkeep account auto                 Forget the choice: use whichever account the session runs with
       sessionkeep account plugins              Show for each account whether SessionKeep is turned on in Claude Code
       sessionkeep account enable NAME          Turn SessionKeep on in Claude Code for that account
       sessionkeep account disable NAME         Turn it off there (what is already saved stays)`
	if err := inv.only(); err != nil {
		return err
	}
	if len(rest) == 0 {
		return usageErr("%s", usageText)
	}
	st, err := load(inv, rest[0] != "list" && rest[0] != "plugins")
	if err != nil {
		return err
	}
	switch {
	case rest[0] == "plugins" && len(rest) == 1:
		// Claude Code takes a second or two to answer, so the accounts are asked at the same time.
		list := accounts(inv, st)
		var wg sync.WaitGroup
		for i := range list {
			wg.Add(1)
			go func(a *account) {
				defer wg.Done()
				a.Plugin, a.PluginWhy = pluginState(inv, *a)
			}(&list[i])
		}
		wg.Wait()
		var b strings.Builder
		for _, a := range list {
			words := map[string]string{"on": "on: its sessions are saved, and Claude can use the memory there", "off": "off: nothing from its sessions is saved"}[a.Plugin]
			if words == "" {
				words = "unknown: " + a.PluginWhy
			}
			fmt.Fprintf(&b, "%-12s %s\n", a.Name, words)
		}
		if len(list) == 0 {
			b.WriteString("no Claude Code configuration folder was found\n")
		}
		b.WriteString("All accounts share one memory. Change one with `sessionkeep account enable NAME` or `sessionkeep account disable NAME`.\n")
		return inv.emit(map[string]any{"accounts": list}, b.String())

	case (rest[0] == "enable" || rest[0] == "disable") && len(rest) == 2:
		list := accounts(inv, st)
		hit := findAccount(list, rest[1])
		if hit == nil {
			return failErr("not_found", "no Claude Code account named %q was found (found: %s)", rest[1], accountNames(list))
		}
		if rest[0] == "disable" {
			if state, why := pluginState(inv, *hit); state == "unknown" {
				return failErr("claude_unavailable", "%s", why)
			} else if state == "on" {
				if _, err := claudeFor(inv, *hit, "plugin", "uninstall", pluginID); err != nil {
					return failErr("plugin_not_disabled", "%v", err)
				}
			}
			hit.Plugin = "off"
			return inv.emit(map[string]any{"changed": true, "account": hit}, fmt.Sprintf("SessionKeep is turned off in Claude Code for the account %q. Sessions run with it are no longer saved; what is already saved stays. Restart Claude Code there.\n", hit.Name))
		}
		dir, err := install.Dir(runtime.GOOS, inv.getenv)
		if err != nil {
			return &cmdError{ExitUnavailable, "no_install_dir", err.Error()}
		}
		rec, err := install.Load(dir)
		if err != nil {
			return err
		}
		if rec == nil {
			return failErr("not_installed", "SessionKeep is not installed yet, so there is no plugin to turn on; run `sessionkeep install` first")
		}
		if err := enablePlugin(inv, *hit, rec); err != nil {
			return failErr("plugin_not_enabled", "%v", err)
		}
		hit.Plugin = "on"
		return inv.emit(map[string]any{"changed": true, "account": hit}, fmt.Sprintf("SessionKeep is turned on in Claude Code for the account %q. It shares the same memory as your other accounts. Restart Claude Code there for it to load.\n", hit.Name))

	case rest[0] == "list" && len(rest) == 1:
		list := accounts(inv, st)
		eff, decided := effectiveAccount(inv, st)
		var b strings.Builder
		for _, a := range list {
			marks := []string{}
			if a.Chosen {
				marks = append(marks, "chosen")
			}
			if a.Current {
				marks = append(marks, "in use in this terminal")
			}
			if a.Default {
				marks = append(marks, "Claude Code's default")
			}
			cmd := a.Command
			if cmd == "" {
				cmd = "(no short command found)"
			}
			used := a.LastUsed
			if used == "" {
				used = "never"
			}
			fmt.Fprintf(&b, "%-12s command: %-12s last used: %-10s %s\n             %s\n", a.Name, cmd, used, strings.Join(marks, ", "), a.ConfigDir)
		}
		if len(list) == 0 {
			b.WriteString("no Claude Code configuration folder was found\n")
		}
		if decided {
			fmt.Fprintf(&b, "SessionKeep uses: %s\n", eff.Name)
		} else {
			fmt.Fprintf(&b, "More than one account exists and none is chosen. SessionKeep uses whichever one a session runs with (here: %s). Choose one with `sessionkeep account use NAME`.\n", eff.Name)
		}
		return inv.emit(map[string]any{"accounts": list, "uses": eff.Name, "decided": decided}, b.String())

	case rest[0] == "use" && len(rest) == 2:
		list := accounts(inv, st)
		hit := findAccount(list, rest[1])
		if hit == nil {
			return failErr("not_found", "no Claude Code account named %q was found (found: %s); a folder path also works if it is listed by `sessionkeep account list`", rest[1], accountNames(list))
		}
		st.settings.ClaudeAccount = config.ClaudeAccount{Chosen: true}
		if !hit.Default {
			st.settings.ClaudeAccount.ConfigDir = hit.ConfigDir
		}
		if err := config.SaveSettings(st.paths, st.settings); err != nil {
			return err
		}
		hit.Chosen = true
		return inv.emit(map[string]any{"changed": true, "account": hit}, fmt.Sprintf("SessionKeep now uses the Claude Code account %q (%s) for summaries, history import and the commands it prints.\n", hit.Name, hit.ConfigDir))

	case rest[0] == "auto" && len(rest) == 1:
		st.settings.ClaudeAccount = config.ClaudeAccount{}
		if err := config.SaveSettings(st.paths, st.settings); err != nil {
			return err
		}
		return inv.emit(map[string]any{"changed": true}, "No account is chosen. SessionKeep uses whichever account a session runs with.\n")
	}
	return usageErr("%s", usageText)
}

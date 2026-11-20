package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"sessionkeep/internal/config"
)

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
       sessionkeep account auto                 Forget the choice: use whichever account the session runs with`
	if err := inv.only(); err != nil {
		return err
	}
	if len(rest) == 0 {
		return usageErr("%s", usageText)
	}
	st, err := load(inv, rest[0] != "list")
	if err != nil {
		return err
	}
	switch {
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
		var hit *account
		list := accounts(inv, st)
		for i, a := range list {
			if strings.EqualFold(a.Name, rest[1]) || (a.Command != "" && strings.EqualFold(a.Command, rest[1])) || sameDir(a.ConfigDir, rest[1]) || (a.Default && strings.EqualFold(rest[1], "default")) {
				hit = &list[i]
				break
			}
		}
		if hit == nil {
			var names []string
			for _, a := range list {
				names = append(names, a.Name)
			}
			return failErr("not_found", "no Claude Code account named %q was found (found: %s); a folder path also works if it is listed by `sessionkeep account list`", rest[1], strings.Join(names, ", "))
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

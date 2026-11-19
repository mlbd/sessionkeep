package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"sessionkeep/internal/autostart"
	"sessionkeep/internal/buildinfo"
	"sessionkeep/internal/config"
	"sessionkeep/internal/install"
	"sessionkeep/internal/plugin"
)

func selfPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("cannot find this program's own path: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return exe, nil
}

// installCmd copies this program to its per-user location and generates the plugin beside it.
// It changes no Claude Code setting; it prints the commands that do.
func installCmd(inv invocation) error {
	if err := inv.only("with-mcp"); err != nil {
		return err
	}
	dir, err := install.Dir(runtime.GOOS, inv.getenv)
	if err != nil {
		return &cmdError{ExitUnavailable, "no_install_dir", err.Error()}
	}
	self, err := selfPath()
	if err != nil {
		return err
	}
	rec, err := install.Install(dir, self, buildinfo.Get().Version, inv.flags["with-mcp"] != "", time.Now())
	if err != nil {
		return failErr("install_failed", "%v", err)
	}
	// Create the data directory and device id now, so the first session has nothing to set up.
	if _, err := load(inv, true); err != nil {
		return err
	}
	text := fmt.Sprintf(`installed sessionkeep %s
program: %s
plugin:  %s

No Claude Code setting was changed. To turn it on in Claude Code, run these two commands yourself:

  claude plugin marketplace add "%s"
  claude plugin install %s@%s

To try it for a single session instead:  claude --plugin-dir "%s"

Nothing is saved until you register a project (sessionkeep project add . --capture conversation)
or choose automatic mode (sessionkeep setup --auto-add auto --level conversation).
To use the short command name, add this folder to your PATH: %s
`, rec.Version, rec.Binary, rec.PluginDir, rec.Marketplace, plugin.Name, install.MarketplaceName, rec.PluginDir, dir)
	return inv.emit(rec, text)
}

// uninstallCmd removes the installed program, plugin files and logon task. Data stays unless
// --purge-data is given, and that needs --yes after a preview.
func uninstallCmd(inv invocation) error {
	if err := inv.only("purge-data", "yes"); err != nil {
		return err
	}
	dir, err := install.Dir(runtime.GOOS, inv.getenv)
	if err != nil {
		return &cmdError{ExitUnavailable, "no_install_dir", err.Error()}
	}
	p, err := config.Resolve(runtime.GOOS, inv.getenv)
	if err != nil {
		return &cmdError{ExitUnavailable, "no_data_dir", err.Error()}
	}
	purge := inv.flags["purge-data"] != ""
	if purge {
		// Only a directory that is recognisably SessionKeep's is ever deleted.
		_, errCfg := os.Stat(p.SettingsPath())
		_, errDB := os.Stat(p.DBPath)
		if entries, _ := os.ReadDir(p.DataDir); len(entries) > 0 && errCfg != nil && errDB != nil {
			return failErr("not_sessionkeep_data", "%s does not look like a SessionKeep data directory (no settings file or database); it will not be deleted", p.DataDir)
		}
		if inv.flags["yes"] == "" {
			counts := "no database"
			if raw, _, code := runner(inv.getenv)([]string{"status", "--json"}); code == ExitOK {
				var st struct {
					Data struct{ Stored json.RawMessage }
				}
				if json.Unmarshal(raw, &st) == nil {
					counts = string(st.Data.Stored)
				}
			}
			inv.emit(map[string]any{"purged": false, "data_dir": p.DataDir, "stored": counts},
				fmt.Sprintf("NOT DONE YET. This would uninstall SessionKeep AND permanently delete all of its data in %s\nstored there now: %s\nIt cannot be undone. Make an export first if you might want it back (sessionkeep export FILE).\nRun the same command with --yes to proceed.\n", p.DataDir, counts))
			return &cmdError{ExitNeedsConsent, "confirmation_required", "nothing was removed; add --yes to confirm deleting the data too"}
		}
	}
	settings, _ := config.LoadSettings(p)
	taskNote := "no logon entry existed"
	if st, _ := autostart.Query(settings.DeviceID); st.Enabled {
		if _, err := autostart.Disable(settings.DeviceID); err != nil {
			return fmt.Errorf("the logon entry could not be removed, so nothing else was touched: %w", err)
		}
		taskNote = "logon entry removed"
	}
	self, _ := selfPath()
	removed, moved, err := install.Uninstall(dir, self)
	if errors.Is(err, install.ErrNotInstalled) {
		if !purge {
			return failErr("not_installed", "SessionKeep is not installed in %s; nothing to remove", dir)
		}
	} else if err != nil {
		return err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "removed %d installed file(s) from %s; %s\n", len(removed), dir, taskNote)
	if moved != "" {
		fmt.Fprintf(&b, "the running program could not delete itself and was moved to %s; delete that file when this command has finished\n", moved)
	}
	if purge {
		if err := os.RemoveAll(p.DataDir); err != nil {
			return fmt.Errorf("the program was removed but the data directory could not be deleted: %w", err)
		}
		fmt.Fprintf(&b, "all SessionKeep data in %s was deleted\n", p.DataDir)
	} else {
		fmt.Fprintf(&b, "your data was kept in %s\n", p.DataDir)
	}
	fmt.Fprintf(&b, "No Claude Code setting was changed. If you enabled the plugin there, remove it yourself:\n  claude plugin uninstall %s@%s\n  claude plugin marketplace remove %s\n", plugin.Name, install.MarketplaceName, install.MarketplaceName)
	return inv.emit(map[string]any{"removed": removed, "moved": moved, "data_kept": !purge, "data_dir": p.DataDir}, b.String())
}

// installedBinary returns the installed program's path, or a clear error when not installed.
func installedBinary(inv invocation) (string, error) {
	dir, err := install.Dir(runtime.GOOS, inv.getenv)
	if err != nil {
		return "", &cmdError{ExitUnavailable, "no_install_dir", err.Error()}
	}
	rec, err := install.Load(dir)
	if err != nil {
		return "", err
	}
	if rec == nil {
		return "", failErr("not_installed", "run `sessionkeep install` first: the logon entry must point at a program that stays in one place")
	}
	return rec.Binary, nil
}

func autostartCmd(inv invocation, rest []string) error {
	if err := inv.only(); err != nil {
		return err
	}
	if len(rest) != 1 || (rest[0] != "enable" && rest[0] != "disable" && rest[0] != "status") {
		return usageErr("usage: sessionkeep autostart enable|disable|status")
	}
	st, err := load(inv, rest[0] == "enable")
	if err != nil {
		return err
	}
	var status autostart.Status
	switch rest[0] {
	case "enable":
		bin, err := installedBinary(inv)
		if err != nil {
			return err
		}
		if status, err = autostart.Enable(st.settings.DeviceID, bin); err != nil {
			if errors.Is(err, autostart.ErrUnsupported) {
				return &cmdError{ExitUnavailable, "unsupported", err.Error()}
			}
			return err
		}
	case "disable":
		if status, err = autostart.Disable(st.settings.DeviceID); err != nil {
			return err
		}
	default:
		status, _ = autostart.Query(st.settings.DeviceID)
	}
	text := "auto-start is off: waiting events are stored when a Claude Code session starts or when you run `sessionkeep ingest`\n"
	if status.Enabled {
		text = fmt.Sprintf("auto-start is on: when you log on to Windows, waiting events are stored once\nentry: %s (in your own logon list; no administrator rights involved)\nruns:  %s\n", status.Name, status.Command)
	} else if !status.Supported {
		text = "auto-start is not available on this system yet\n"
	}
	return inv.emit(status, text)
}

// installChecks adds the install-related lines to `doctor`.
func installChecks(inv invocation) []check {
	dir, err := install.Dir(runtime.GOOS, inv.getenv)
	if err != nil {
		return []check{{"installation", "failed", err.Error()}}
	}
	rec, err := install.Load(dir)
	if err != nil {
		return []check{{"installation", "failed", err.Error()}}
	}
	if rec == nil {
		return []check{{"installation", "ok", "not installed (running from " + filepath.Dir(firstNonEmpty(selfPath())) + "); `sessionkeep install` puts it in a stable place"}}
	}
	out := []check{}
	if _, err := os.Stat(rec.Binary); err != nil {
		out = append(out, check{"installation", "failed", "installed program is missing: " + rec.Binary})
	} else {
		out = append(out, check{"installation", "ok", fmt.Sprintf("sessionkeep %s in %s", rec.Version, dir)})
	}
	// Every hook in the generated plugin must point at a program that exists.
	raw, err := os.ReadFile(filepath.Join(rec.PluginDir, "hooks", "hooks.json"))
	var hooks struct {
		Hooks map[string][]struct {
			Hooks []struct{ Command string }
		}
	}
	switch {
	case err != nil || json.Unmarshal(raw, &hooks) != nil:
		out = append(out, check{"plugin_hooks", "failed", "the generated plugin's hook file is missing or unreadable; run `sessionkeep install` again"})
	default:
		bad := ""
		n := 0
		for _, groups := range hooks.Hooks {
			for _, g := range groups {
				for _, h := range g.Hooks {
					n++
					if _, err := os.Stat(h.Command); err != nil {
						bad = h.Command
					}
				}
			}
		}
		if bad != "" || n == 0 {
			out = append(out, check{"plugin_hooks", "failed", "hooks point at a program that does not exist (" + bad + "); run `sessionkeep install` again"})
		} else {
			out = append(out, check{"plugin_hooks", "ok", fmt.Sprintf("%d hook commands all point at an existing program", n)})
		}
	}
	return out
}

func firstNonEmpty(s string, _ error) string { return s }

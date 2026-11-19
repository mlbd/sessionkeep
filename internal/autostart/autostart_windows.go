//go:build windows

package autostart

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/windows/registry"
)

// The entry lives under the current user's "Run" key: programs listed there are started by
// Windows when that user logs on. It needs no administrator rights and affects no other user.
// (A logon-triggered scheduled task was tried first; Windows refuses to create one without
// elevation: "ERROR: Access is denied.")
const runKey = `Software\Microsoft\Windows\CurrentVersion\Run`

// TaskName returns the entry's name for an installation; the device id keeps two profiles on one
// machine apart.
func TaskName(deviceID string) string {
	if len(deviceID) > 8 {
		deviceID = deviceID[:8]
	}
	return "SessionKeep drain " + deviceID
}

// command is what runs at logon. conhost --headless starts a console program without creating a
// window, so nothing should flash on screen.
func command(exe string) (string, []string) {
	conhost := filepath.Join(os.Getenv("SystemRoot"), "System32", "conhost.exe")
	return fmt.Sprintf(`"%s" --headless "%s" ingest --quiet`, conhost, exe), []string{conhost, "--headless", exe, "ingest", "--quiet"}
}

// Enable creates or replaces the logon entry for the current user.
func Enable(deviceID, exe string) (Status, error) {
	if _, err := os.Stat(exe); err != nil {
		return Status{Supported: true}, fmt.Errorf("program not found at %s", exe)
	}
	k, _, err := registry.CreateKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return Status{Supported: true}, fmt.Errorf("open the per-user logon list: %w", err)
	}
	defer k.Close()
	line, _ := command(exe)
	if err := k.SetStringValue(TaskName(deviceID), line); err != nil {
		return Status{Supported: true}, fmt.Errorf("write the logon entry: %w", err)
	}
	return Query(deviceID)
}

// Disable removes the entry. Removing one that does not exist is not an error.
func Disable(deviceID string) (Status, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err == nil {
		defer k.Close()
		if err := k.DeleteValue(TaskName(deviceID)); err != nil && !errors.Is(err, registry.ErrNotExist) {
			return Status{Supported: true}, fmt.Errorf("remove the logon entry: %w", err)
		}
	}
	return Query(deviceID)
}

// Query reports whether the entry exists and what it runs.
func Query(deviceID string) (Status, error) {
	st := Status{Supported: true, Name: TaskName(deviceID), Detail: "no logon entry"}
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if err != nil {
		return st, nil
	}
	defer k.Close()
	if v, _, err := k.GetStringValue(st.Name); err == nil {
		st.Enabled, st.Command, st.Detail = true, v, "per-user logon entry"
	}
	return st, nil
}

// RunNow runs the same command Windows would run at logon, once, and waits for it.
func RunNow(exe string) error {
	_, argv := command(exe)
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Run()
}

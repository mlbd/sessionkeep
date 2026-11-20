//go:build windows

package cli

import (
	"os/exec"
	"syscall"
)

const (
	detachedProcess       = 0x00000008
	createNewProcessGroup = 0x00000200
	createBreakawayJob    = 0x01000000
)

// startDetached starts exe with args as a process that does not belong to the caller's console
// or process group, shows no window, and is not waited for. Claude Code ends a background hook
// when a non-interactive session finishes; work that must outlive the hook has to be detached.
// Leaving the caller's job object is tried first and given up silently where it is not allowed.
func startDetached(exe string, args, env []string) error {
	for _, flags := range []uint32{detachedProcess | createNewProcessGroup | createBreakawayJob, detachedProcess | createNewProcessGroup} {
		cmd := exec.Command(exe, args...)
		cmd.Env = env
		cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: flags, HideWindow: true}
		if err := cmd.Start(); err == nil {
			return cmd.Process.Release()
		} else if flags&createBreakawayJob == 0 {
			return err
		}
	}
	return nil
}

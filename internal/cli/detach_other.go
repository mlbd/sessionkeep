//go:build !windows

package cli

import (
	"os/exec"
	"syscall"
)

// startDetached starts exe with args in its own session, not waited for, so it survives the
// hook that started it.
func startDetached(exe string, args, env []string) error {
	cmd := exec.Command(exe, args...)
	cmd.Env = env
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

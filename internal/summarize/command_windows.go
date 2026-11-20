//go:build windows

package summarize

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// command builds the process for the Claude Code program. On Windows the program is usually a
// .cmd file (the npm shim), often under "C:\Program Files". Such a file is run by cmd.exe, whose
// quoting rules differ from every other program's: the standard escaping breaks as soon as the
// path holds a space. So for .cmd and .bat the command line is written by hand in the one form
// cmd.exe documents: /s /c "<everything, each part in its own quotes>". Arguments that cannot be
// expressed safely that way are refused instead of guessed at. No console window is shown.
func command(ctx context.Context, bin string, args []string) (*exec.Cmd, error) {
	ext := strings.ToLower(filepath.Ext(bin))
	if ext != ".cmd" && ext != ".bat" {
		cmd := exec.CommandContext(ctx, bin, args...)
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		return cmd, nil
	}
	shell := os.Getenv("ComSpec")
	if !filepath.IsAbs(shell) {
		shell = filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe")
	}
	var line strings.Builder
	line.WriteString(`"` + shell + `" /d /s /c "`)
	for i, a := range append([]string{bin}, args...) {
		if strings.ContainsAny(a, "\"%\r\n") || strings.HasSuffix(a, `\`) {
			return nil, fmt.Errorf("argument %d cannot be passed safely to a .cmd file", i)
		}
		if i > 0 {
			line.WriteByte(' ')
		}
		line.WriteString(`"` + a + `"`)
	}
	line.WriteString(`"`)
	cmd := exec.CommandContext(ctx, shell)
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: line.String(), HideWindow: true}
	return cmd, nil
}

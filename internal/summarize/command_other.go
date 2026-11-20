//go:build !windows

package summarize

import (
	"context"
	"os/exec"
)

func command(ctx context.Context, bin string, args []string) (*exec.Cmd, error) {
	return exec.CommandContext(ctx, bin, args...), nil
}

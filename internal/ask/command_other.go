//go:build !windows

package ask

import (
	"context"
	"os/exec"
)

// winCommand is never reached off Windows; Command only calls it there.
func winCommand(ctx context.Context, exe, line string) *exec.Cmd {
	return exec.CommandContext(ctx, exe)
}

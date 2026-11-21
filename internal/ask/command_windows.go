package ask

import (
	"context"
	"os/exec"
	"syscall"
)

// winCommand starts exe with line as its exact command line, which exec's own quoting would alter.
func winCommand(ctx context.Context, exe, line string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, exe)
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: line}
	return cmd
}

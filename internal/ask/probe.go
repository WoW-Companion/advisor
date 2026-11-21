package ask

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// ProbeTimeout bounds one Probe.
const ProbeTimeout = 60 * time.Second

// Probe sends one tiny prompt through the agent and returns its first output line. A failure is an
// error whose text is the first line the agent printed to stderr (for example "not signed in"),
// else a short reason. It reads nothing from the game and hands the agent no MCP config.
func Probe(ctx context.Context, command []string, prompt string) (string, error) {
	if len(command) == 0 {
		return "", errors.New("no agent command is configured")
	}
	ctx, cancel := context.WithTimeout(ctx, ProbeTimeout)
	defer cancel()
	cmd, err := Launch(ctx, Argv(command, prompt, ""), "WOWC_QUESTION="+prompt)
	if err != nil {
		return "", err
	}
	cmd.WaitDelay = time.Second
	var out, errOut bytes.Buffer
	cmd.Stdout = &capped{w: &out, left: 4096}
	cmd.Stderr = &capped{w: &errOut, left: 2048}
	err = cmd.Run()
	switch {
	case ctx.Err() == context.DeadlineExceeded:
		return "", fmt.Errorf("no answer within %s", ProbeTimeout)
	case ctx.Err() != nil:
		return "", ctx.Err()
	case err != nil:
		if l := firstLine(errOut.String()); l != "" {
			return "", errors.New(l)
		}
		if l := firstLine(out.String()); l != "" {
			return "", errors.New(l)
		}
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return "", fmt.Errorf("the agent exited with status %d", ee.ExitCode())
		}
		return "", startError(cmd.Path, err)
	}
	l := firstLine(out.String())
	if l == "" {
		return "", errors.New("the agent gave no answer")
	}
	return l, nil
}

func firstLine(s string) string {
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			if len(l) > 200 {
				l = l[:200]
			}
			return l
		}
	}
	return ""
}

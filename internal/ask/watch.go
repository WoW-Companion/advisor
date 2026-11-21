package ask

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ResolveAgent turns the --agent value, or else the config file's "agent" field, into an argv.
// A preset name (claude, codex, gemini) expands to its command; anything else is split on spaces.
func ResolveAgent(flagValue, configPath string) ([]string, error) {
	spec := strings.TrimSpace(flagValue)
	if spec == "" && configPath != "" {
		raw, err := os.ReadFile(configPath)
		switch {
		case errors.Is(err, os.ErrNotExist):
		case err != nil:
			return nil, err
		default:
			var c struct {
				Agent string `json:"agent"`
			}
			if err := json.Unmarshal(raw, &c); err != nil {
				return nil, fmt.Errorf("%s: %w", configPath, err)
			}
			spec = strings.TrimSpace(c.Agent)
		}
	}
	if spec == "" {
		return nil, errors.New("no agent command: pass --agent (claude, codex, gemini or a command) or set \"agent\" in the config file")
	}
	if p, ok := Presets[spec]; ok {
		return p, nil
	}
	return strings.Fields(spec), nil
}

// MCPConfig renders the MCP config JSON handed to the agent: it starts this binary as a stdio
// server on the capture. The agent reads facts through it and is never given the file itself.
func MCPConfig(exe string, env map[string]string) []byte {
	b, _ := json.MarshalIndent(map[string]any{"mcpServers": map[string]any{"wowc-advisor": map[string]any{
		"command": exe, "args": []string{"serve", "--stdio"}, "env": env,
	}}}, "", "  ")
	return b
}

// Stamp identifies a file's content cheaply enough to poll.
func Stamp(path string) string {
	st, err := os.Stat(path)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%d/%d", st.ModTime().UnixNano(), st.Size())
}

// Watch polls the SavedVariables file and runs a pass whenever it changes (and once at the start).
// It returns when ctx ends.
func Watch(ctx context.Context, cfg Config, every time.Duration, out io.Writer) error {
	last := ""
	for {
		if s := Stamp(cfg.SavedVariables); s != last {
			last = s
			if s != "" {
				res, err := Run(ctx, cfg)
				if ctx.Err() != nil {
					return nil
				}
				if err != nil {
					fmt.Fprintln(out, "ask:", err)
				} else {
					fmt.Fprintln(out, Summary(res, cfg.AnswersPath))
				}
			}
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(every):
		}
	}
}

// Summary is the one line a pass prints; it reminds the player to /reload when answers were written.
func Summary(r Result, answersPath string) string {
	if r.Answered == 0 {
		s := "no new questions"
		if r.Pending > 0 {
			s = fmt.Sprintf("%d question(s) still waiting", r.Pending)
		}
		return s
	}
	s := fmt.Sprintf("answered %d question(s)", r.Answered)
	if r.Failed > 0 {
		s += fmt.Sprintf(" (%d failed)", r.Failed)
	}
	if r.Pending > 0 {
		s += fmt.Sprintf(", %d more waiting", r.Pending)
	}
	return s + " -> " + filepath.ToSlash(answersPath) + "\nType /reload in game to see the answers."
}

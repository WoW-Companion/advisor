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
	spec, _, err := agentSpec(flagValue, configPath)
	if err != nil {
		return nil, err
	}
	if spec == "" {
		return nil, errors.New("no agent command: pass --agent (claude, codex, gemini or a command) or set \"agent\" in the config file")
	}
	return expand(spec), nil
}

func expand(spec string) []string {
	if p, ok := Presets[spec]; ok {
		return append([]string(nil), p...)
	}
	return strings.Fields(spec)
}

// agentSpec is the --agent value, else the config file's "agent" field, and which of the two it was.
func agentSpec(flagValue, configPath string) (spec, source string, err error) {
	if spec = strings.TrimSpace(flagValue); spec != "" {
		return spec, SourceFlag, nil
	}
	if configPath == "" {
		return "", SourceNone, nil
	}
	raw, err := os.ReadFile(configPath)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return "", SourceNone, nil
	case err != nil:
		return "", "", err
	}
	var c struct {
		Agent string `json:"agent"`
	}
	if err := json.Unmarshal(raw, &c); err != nil {
		return "", "", fmt.Errorf("%s: %w", configPath, err)
	}
	if spec = strings.TrimSpace(c.Agent); spec == "" {
		return "", SourceNone, nil
	}
	return spec, SourceConfig, nil
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

package ask

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
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

// PolicyPath is where the Gemini policy file sits: next to the generated MCP config.
func PolicyPath(mcpConfig string) string {
	if mcpConfig == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(mcpConfig), "gemini-policy.toml")
}

// GeminiPolicy is the Gemini CLI policy file: the advisor's tools run without asking, and shell,
// file writes and web access are denied outright (a headless run cannot ask).
func GeminiPolicy() []byte {
	return []byte(`[[rule]]
mcpName = "` + ServerName + `"
decision = "allow"
priority = 200

[[rule]]
toolName = ["run_shell_command", "write_file", "replace", "web_fetch", "google_web_search"]
decision = "deny"
priority = 190
`)
}

// codexServer is the advisor server from the MCP config file as a Codex inline config table, with
// its tools approved; empty when the file cannot be read.
func codexServer(mcpConfig string) string {
	raw, err := os.ReadFile(mcpConfig)
	if err != nil {
		return ""
	}
	var c struct {
		Servers map[string]struct {
			Command string            `json:"command"`
			Args    []string          `json:"args"`
			Env     map[string]string `json:"env"`
		} `json:"mcpServers"`
	}
	if json.Unmarshal(raw, &c) != nil {
		return ""
	}
	s, ok := c.Servers[ServerName]
	if !ok {
		return ""
	}
	// A TOML literal string keeps a Windows path's backslashes and has no double quote for the
	// .cmd shim's quoting to change; a value with a quote or control character is escaped instead.
	q := func(v string) string {
		if !strings.ContainsAny(v, "'\x00\x01\x02\x03\x04\x05\x06\x07\x08\t\n\r\x7f") {
			return "'" + v + "'"
		}
		var b bytes.Buffer
		enc := json.NewEncoder(&b)
		enc.SetEscapeHTML(false)
		enc.Encode(v)
		return strings.TrimSpace(b.String())
	}
	args := make([]string, len(s.Args))
	for i, a := range s.Args {
		args[i] = q(a)
	}
	keys := make([]string, 0, len(s.Env))
	for k := range s.Env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	env := make([]string, len(keys))
	for i, k := range keys {
		env[i] = q(k) + " = " + q(s.Env[k])
	}
	return "{ command = " + q(s.Command) + ", args = [" + strings.Join(args, ", ") + "], env = { " + strings.Join(env, ", ") +
		" }, default_tools_approval_mode = \"approve\" }"
}

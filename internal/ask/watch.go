package ask

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ResolveAgent turns the --agent value, or else the config file's "agent" field, into an argv.
// A preset name (claude, codex, agy, gemini) expands to its command; anything else is split on spaces.
func ResolveAgent(flagValue, configPath string) ([]string, error) {
	spec, _, err := agentSpec(flagValue, configPath)
	if err != nil {
		return nil, err
	}
	if spec == "" {
		return nil, errors.New("no agent command: pass --agent (claude, codex, agy, gemini or a command) or set \"agent\" in the config file")
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

// PolicyPath is where the Gemini policy file sits: next to the generated MCP config.
func PolicyPath(mcpConfig string) string {
	if mcpConfig == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(mcpConfig), "gemini-policy.toml")
}

// AgyWorkspace is the folder an Antigravity run starts in: it holds .agents/mcp_config.json, the
// workspace MCP config agy reads (https://antigravity.google/docs/mcp/), beside the generated config.
func AgyWorkspace(mcpConfig string) string {
	return filepath.Join(filepath.Dir(mcpConfig), "agy")
}

// AgyRule is the permission rule that lets agy run the advisor's MCP tools without asking
// (https://antigravity.google/docs/permissions?tab=cli: mcp(server/*) matches every tool of one server).
const AgyRule = "mcp(" + ServerName + "/*)"

// WriteAgyWorkspace writes mcpConfig's contents to AgyWorkspace's .agents/mcp_config.json.
func WriteAgyWorkspace(mcpConfig string, data []byte) error {
	d := filepath.Join(AgyWorkspace(mcpConfig), ".agents")
	if err := os.MkdirAll(d, 0o700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(d, "mcp_config.json"), data, 0o600)
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

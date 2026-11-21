// Package setup renders and writes the MCP server entry for each supported agent client.
package setup

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ServerName is the key the advisor is registered under in every client.
const ServerName = "wowc-advisor"

// Clients lists the accepted client names.
var Clients = []string{"claude-desktop", "claude-code", "codex", "agy", "gemini"}

// Entry is what the client launches.
type Entry struct {
	Binary         string
	SavedVariables string
}

// Env locates the per-user and per-project directories.
type Env struct {
	GOOS    string
	Home    string
	AppData string // Windows %APPDATA%
	Cwd     string
}

type server struct {
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
}

func (e Entry) server() server {
	return server{e.Binary, []string{"serve", "--stdio"}, map[string]string{"WOWC_SAVEDVARIABLES": e.SavedVariables}}
}

// Path is the config file the client reads. Claude Code gets the project .mcp.json in the
// current directory, since its user config (~/.claude.json) also holds unrelated state.
func Path(client string, e Env) (string, error) {
	switch client {
	case "claude-desktop":
		switch e.GOOS {
		case "windows":
			return filepath.Join(e.AppData, "Claude", "claude_desktop_config.json"), nil
		case "darwin":
			return filepath.Join(e.Home, "Library", "Application Support", "Claude", "claude_desktop_config.json"), nil
		}
		return filepath.Join(e.Home, ".config", "Claude", "claude_desktop_config.json"), nil
	case "claude-code":
		return filepath.Join(e.Cwd, ".mcp.json"), nil
	case "codex":
		return filepath.Join(e.Home, ".codex", "config.toml"), nil
	case "gemini":
		return filepath.Join(e.Home, ".gemini", "settings.json"), nil
	case "agy":
		// Antigravity CLI's settings, https://antigravity.google/docs/cli/reference/. The advisor itself is
		// defined per run (see ask.AgyWorkspace); only the permission rule has to live here.
		return filepath.Join(e.Home, ".gemini", "antigravity-cli", "settings.json"), nil
	}
	return "", fmt.Errorf("unknown client %q (want one of %s)", client, strings.Join(Clients, ", "))
}

// Snippet is the text a user adds to the client's config file.
func Snippet(client string, e Entry) (string, error) {
	if client == "codex" {
		q := strconv.Quote
		return "[mcp_servers." + ServerName + "]\n" +
			"command = " + q(e.Binary) + "\n" +
			`args = ["serve", "--stdio"]` + "\n\n" +
			"[mcp_servers." + ServerName + ".env]\n" +
			"WOWC_SAVEDVARIABLES = " + q(e.SavedVariables) + "\n", nil
	}
	if _, err := Path(client, Env{}); err != nil {
		return "", err
	}
	if client == "agy" {
		return marshal(map[string]any{"permissions": map[string]any{"allow": []string{AgyRule}}})
	}
	return marshal(map[string]any{"mcpServers": map[string]server{ServerName: e.server()}})
}

func marshal(v any) (string, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	err := enc.Encode(v)
	return b.String(), err
}

// Write adds the entry to the client's config file, keeping every other entry, and returns the
// file path and the backup path ("" when the file did not exist). It refuses a file it cannot
// merge into safely.
func Write(client string, e Entry, env Env) (path, backup string, err error) {
	path, err = Path(client, env)
	if err != nil {
		return "", "", err
	}
	old, err := os.ReadFile(path)
	exists := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", "", err
	}
	var out string
	switch client {
	case "codex":
		out, err = mergeTOML(string(old), e)
	case "agy":
		out, err = mergeAgy(old)
	default:
		out, err = mergeJSON(old, e)
	}
	if err != nil {
		return "", "", fmt.Errorf("%s: %w", path, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", "", err
	}
	if exists {
		if backup, err = backupFile(path, old); err != nil {
			return "", "", err
		}
	}
	return path, backup, os.WriteFile(path, []byte(out), 0o644)
}

func backupFile(path string, data []byte) (string, error) {
	for i := 0; ; i++ {
		name := path + ".bak"
		if i > 0 {
			name += "." + strconv.Itoa(i)
		}
		f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		_, werr := f.Write(data)
		if cerr := f.Close(); werr == nil {
			werr = cerr
		}
		return name, werr
	}
}

// mergeJSON sets mcpServers[ServerName] and keeps every other key and server as it was.
func mergeJSON(old []byte, e Entry) (string, error) {
	doc := map[string]json.RawMessage{}
	if len(bytes.TrimSpace(old)) > 0 {
		if err := json.Unmarshal(old, &doc); err != nil {
			return "", fmt.Errorf("not valid JSON, edit it by hand: %w", err)
		}
	}
	servers := map[string]json.RawMessage{}
	if raw, ok := doc["mcpServers"]; ok {
		if err := json.Unmarshal(raw, &servers); err != nil {
			return "", fmt.Errorf("mcpServers is not an object, edit it by hand: %w", err)
		}
	}
	entry, err := json.Marshal(e.server())
	if err != nil {
		return "", err
	}
	servers[ServerName] = entry
	if doc["mcpServers"], err = json.Marshal(servers); err != nil {
		return "", err
	}
	return marshal(doc)
}

// AgyRule lets Antigravity CLI run the advisor's MCP tools without asking; the same text as
// ask.AgyRule (the packages do not import each other). A test keeps them equal.
const AgyRule = "mcp(" + ServerName + "/*)"

// mergeAgy adds AgyRule to permissions.allow and keeps every other setting and rule.
func mergeAgy(old []byte) (string, error) {
	doc := map[string]json.RawMessage{}
	if len(bytes.TrimSpace(old)) > 0 {
		if err := json.Unmarshal(old, &doc); err != nil {
			return "", fmt.Errorf("not valid JSON, edit it by hand: %w", err)
		}
	}
	perms := map[string]json.RawMessage{}
	if raw, ok := doc["permissions"]; ok {
		if err := json.Unmarshal(raw, &perms); err != nil {
			return "", fmt.Errorf("permissions is not an object, edit it by hand: %w", err)
		}
	}
	var allow []string
	if raw, ok := perms["allow"]; ok {
		if err := json.Unmarshal(raw, &allow); err != nil {
			return "", fmt.Errorf("permissions.allow is not a list of strings, edit it by hand: %w", err)
		}
	}
	for _, a := range allow {
		if a == AgyRule {
			return marshal(doc)
		}
	}
	var err error
	if perms["allow"], err = json.Marshal(append(allow, AgyRule)); err != nil {
		return "", err
	}
	if doc["permissions"], err = json.Marshal(perms); err != nil {
		return "", err
	}
	return marshal(doc)
}

// mergeTOML appends the tables; it never rewrites the existing text.
func mergeTOML(old string, e Entry) (string, error) {
	if strings.Contains(old, "[mcp_servers."+ServerName+"]") || strings.Contains(old, "[mcp_servers."+ServerName+".") {
		return "", errors.New("already has a " + ServerName + " entry, edit it by hand")
	}
	snippet, _ := Snippet("codex", e)
	if old != "" && !strings.HasSuffix(old, "\n") {
		old += "\n"
	}
	if old != "" {
		old += "\n"
	}
	return old + snippet, nil
}

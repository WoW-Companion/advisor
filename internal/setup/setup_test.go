package setup

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the golden files")

var entry = Entry{Binary: `C:\Users\me\wowc-advisor.exe`, SavedVariables: `C:\WoW\ForeverScribe.lua`}

func TestSnippetGolden(t *testing.T) {
	for _, client := range Clients {
		got, err := Snippet(client, entry)
		if err != nil {
			t.Fatal(err)
		}
		golden := filepath.Join("testdata", client+".golden")
		if *update {
			os.MkdirAll("testdata", 0o755)
			os.WriteFile(golden, []byte(got), 0o644)
			continue
		}
		want, _ := os.ReadFile(golden)
		if got != string(want) {
			t.Errorf("%s snippet differs from %s:\n%s", client, golden, got)
		}
	}
	if _, err := Snippet("vim", entry); err == nil {
		t.Error("unknown client accepted")
	}
}

func TestPath(t *testing.T) {
	cases := []struct {
		client string
		env    Env
		want   string
	}{
		{"claude-desktop", Env{GOOS: "windows", AppData: "A"}, filepath.Join("A", "Claude", "claude_desktop_config.json")},
		{"claude-desktop", Env{GOOS: "darwin", Home: "H"}, filepath.Join("H", "Library", "Application Support", "Claude", "claude_desktop_config.json")},
		{"claude-desktop", Env{GOOS: "linux", Home: "H"}, filepath.Join("H", ".config", "Claude", "claude_desktop_config.json")},
		{"claude-code", Env{Cwd: "C"}, filepath.Join("C", ".mcp.json")},
		{"codex", Env{Home: "H"}, filepath.Join("H", ".codex", "config.toml")},
		{"gemini", Env{Home: "H"}, filepath.Join("H", ".gemini", "settings.json")},
	}
	for _, c := range cases {
		if got, err := Path(c.client, c.env); err != nil || got != c.want {
			t.Errorf("%s %s: %q, %v; want %q", c.client, c.env.GOOS, got, err, c.want)
		}
	}
}

func TestWritePreservesOthers(t *testing.T) {
	home := t.TempDir()
	env := Env{GOOS: "linux", Home: home, Cwd: home}
	gem := filepath.Join(home, ".gemini", "settings.json")
	cod := filepath.Join(home, ".codex", "config.toml")
	os.MkdirAll(filepath.Dir(gem), 0o755)
	os.MkdirAll(filepath.Dir(cod), 0o755)
	os.WriteFile(gem, []byte(`{"theme":"dark","mcpServers":{"other":{"command":"x","args":["a"]}}}`), 0o644)
	os.WriteFile(cod, []byte("model = \"m\"\n\n[mcp_servers.other]\ncommand = \"x\""), 0o644)

	wants := map[string][]string{
		"gemini": {`"other"`, `"theme": "dark"`, `"wowc-advisor"`},
		"codex":  {"model = ", "[mcp_servers.other]", "[mcp_servers.wowc-advisor]"},
	}
	for _, client := range []string{"gemini", "codex"} {
		path, backup, err := Write(client, entry, env)
		if err != nil || backup == "" {
			t.Fatalf("%s: %v, backup %q", client, err, backup)
		}
		got, _ := os.ReadFile(path)
		for _, want := range wants[client] {
			if !strings.Contains(string(got), want) {
				t.Errorf("%s lost %s:\n%s", path, want, got)
			}
		}
	}
	// A second codex write finds the first one's entry and refuses.
	if _, _, err := Write("codex", entry, env); err == nil {
		t.Error("codex: duplicate entry was appended")
	}
	if b, _ := os.ReadFile(cod + ".bak"); !strings.HasPrefix(string(b), "model = ") {
		t.Errorf("codex backup = %q", b)
	}
	// A file that is not JSON is left alone.
	bad := filepath.Join(home, ".mcp.json")
	os.WriteFile(bad, []byte("{oops"), 0o644)
	if _, _, err := Write("claude-code", entry, env); err == nil {
		t.Error("wrote into invalid JSON")
	}
	if b, _ := os.ReadFile(bad); string(b) != "{oops" {
		t.Errorf("invalid file changed: %q", b)
	}
}

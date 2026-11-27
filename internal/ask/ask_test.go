package ask

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

// TestMain makes the test binary double as the fake agent CLI: with FAKE_AGENT set it prints a canned
// answer (or fails or hangs) instead of running tests.
func TestMain(m *testing.M) {
	switch os.Getenv("FAKE_AGENT") {
	case "":
		os.Setenv("WOWC_PROFILES", os.DevNull+"/none") // the player's own store is never read
		os.Exit(m.Run())
	case "echo":
		fmt.Printf("Do the turn-in first (whats_next).\nquestion=%s char=%s\n", os.Getenv("WOWC_QUESTION"), os.Getenv("WOWC_CHARACTER"))
	case "argv":
		os.WriteFile(os.Getenv("FAKE_ARGV_OUT"), []byte(strings.Join(os.Args[1:], "\x00")), 0o644)
		fmt.Println("ok")
	case "rejecteffort": // a CLI too old for the reasoning flag: refuses it, else records argv like "argv"
		for _, a := range os.Args[1:] {
			if a == "--effort" || strings.HasPrefix(a, "model_reasoning_effort") {
				fmt.Fprintf(os.Stderr, "error: unknown option '%s'\n", a)
				os.Exit(2)
			}
		}
		os.WriteFile(os.Getenv("FAKE_ARGV_OUT"), []byte(strings.Join(os.Args[1:], "\x00")), 0o644)
		fmt.Println("ok")
	case "stream": // a Claude stream-json fixture: one text delta, then the result with usage
		fmt.Println(`{"type":"system","subtype":"init"}`)
		// The first message is the model thinking aloud before a tool call; only the last one is the answer.
		for _, m := range []struct{ text, stop string }{{"Sources are unknown here.", "tool_use"}, {"Do the turn-in.", "end_turn"}} {
			fmt.Println(`{"type":"stream_event","event":{"type":"message_start"}}`)
			fmt.Printf(`{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":%q}}}`+"\n", m.text)
			fmt.Printf(`{"type":"stream_event","event":{"type":"message_delta","delta":{"stop_reason":%q}}}`+"\n", m.stop)
			fmt.Println(`{"type":"stream_event","event":{"type":"message_stop"}}`)
		}
		fmt.Println(`{"type":"result","is_error":false,"result":"Do the turn-in.","total_cost_usd":0.0123,"modelUsage":{"claude-haiku-4-5":{"inputTokens":50,"outputTokens":300},"claude-sonnet-4-5":{"inputTokens":10,"outputTokens":5}},"usage":{"input_tokens":100,"output_tokens":300,"cache_creation_input_tokens":2000,"cache_read_input_tokens":700}}`)
	case "fail":
		fmt.Fprintln(os.Stderr, "boom")
		os.Exit(3)
	case "failout":
		fmt.Println("partial")
		fmt.Fprintln(os.Stderr, "boom")
		os.Exit(3)
	case "silent":
	case "long":
		fmt.Print(strings.Repeat("x", 5000))
	case "hang":
		time.Sleep(time.Minute)
	}
	os.Exit(0)
}

const savedVars = `WoWCompanionDB = {
  ask_seq = 3,
  asks = {
    { id = "100-1", asked_at = 100, character = "Thrall-Doomhammer", text = "What now?", answered = true },
    { id = "100-2", asked_at = 101, character = "Thrall-Doomhammer", text = "Where is \"Bob\"?" },
    { id = "100-3", asked_at = 102, character = "Thrall-Doomhammer", text = "  " },
    { id = "100-4", asked_at = 103, character = "Thrall-Doomhammer", text = "Gear?" },
  },
}
`

func TestArgv(t *testing.T) {
	for _, tc := range []struct {
		name string
		tmpl []string
		mcp  string
		want []string
	}{
		{"claude with config", []string{"claude", "-p", "{prompt}", "--mcp-config", "{mcp_config}", "--x"}, "/m.json", []string{"claude", "-p", "Q", "--mcp-config", "/m.json", "--x"}},
		{"claude without config", []string{"claude", "-p", "{prompt}", "--mcp-config", "{mcp_config}", "--x"}, "", []string{"claude", "-p", "Q", "--x"}},
		{"prompt appended", []string{"mycli", "--fast"}, "", []string{"mycli", "--fast", "Q"}},
		{"prompt only after flags", []string{"codex", "exec", "-s", "read-only", "{prompt}"}, "/m.json", []string{"codex", "exec", "-s", "read-only", "Q"}},
	} {
		if got := Argv(tc.tmpl, "Q", tc.mcp); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestCleanAndSources(t *testing.T) {
	long := strings.Repeat("é", 2000)
	if got := Clean(long); len([]rune(got)) != MaxAnswer || !strings.HasSuffix(got, "...") {
		t.Errorf("Clean long: %d runes", len([]rune(got)))
	}
	if got := Clean("  hi \n"); got != "hi" {
		t.Errorf("Clean = %q", got)
	}
	if got := Sources("used whats_next then upgrade_finder"); !reflect.DeepEqual(got, []string{"upgrade_finder", "whats_next"}) {
		t.Errorf("Sources = %v", got)
	}
	if got := Sources("no tools"); got != nil {
		t.Errorf("Sources = %v", got)
	}
}

func TestResolveAgent(t *testing.T) {
	conf := filepath.Join(t.TempDir(), "ask.json")
	os.WriteFile(conf, []byte(`{"agent": "gemini"}`), 0o644)
	for _, tc := range []struct {
		flag, conf string
		want       []string
		ok         bool
	}{
		{"codex", conf, Presets["codex"], true},
		{"", conf, Presets["gemini"], true},
		{"my cli --x", "", []string{"my", "cli", "--x"}, true},
		{"", filepath.Join(t.TempDir(), "none.json"), nil, false},
	} {
		got, err := ResolveAgent(tc.flag, tc.conf)
		if (err == nil) != tc.ok || !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%q %q: %v, %v", tc.flag, tc.conf, got, err)
		}
	}
}

// fixture returns a Config that runs the fake agent in mode.
func fixture(t *testing.T, mode string) Config {
	t.Helper()
	t.Setenv("FAKE_AGENT", mode)
	return Config{Command: []string{os.Args[0]}, Timeout: 5 * time.Second, Now: func() time.Time { return time.Unix(1000, 0) }}
}

// runPreset runs a preset's argv through the fake agent and returns the arguments it received.
func runPreset(t *testing.T, name string) []string {
	t.Helper()
	cfg := fixture(t, "argv")
	out := filepath.Join(t.TempDir(), "argv")
	t.Setenv("FAKE_ARGV_OUT", out)
	dir := t.TempDir()
	cfg.MCPConfig = filepath.Join(dir, "mcp.json")
	if err := os.WriteFile(cfg.MCPConfig, MCPConfig(`C:\bin\wowc.exe`, map[string]string{"WOWC_SAVEDVARIABLES": `C:\WoW\f.lua`}), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg.Command = append([]string{os.Args[0]}, Presets[name][1:]...)
	if _, err := runAgent(context.Background(), cfg, Question{ID: "1", Character: "R:C", Text: "What now?"}); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(string(raw), "\x00")
}

func after(args []string, flag string) string {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func TestPresetsPreAllowOnlyTheAdvisor(t *testing.T) {
	claude := runPreset(t, "claude")
	if after(claude, "--allowedTools") != "mcp__wowc-advisor" || !strings.Contains(strings.Join(claude, " "), "--strict-mcp-config") ||
		!strings.HasSuffix(after(claude, "--mcp-config"), "mcp.json") {
		t.Errorf("claude argv: %q", claude)
	}
	// Nothing built in is offered, and nothing but the advisor is allowed.
	if i := slices.Index(claude, "--tools"); i < 0 || claude[i+1] != "" {
		t.Errorf("claude keeps built-in tools: %q", claude)
	}
	if i := slices.Index(claude, "--allowedTools"); i < 0 || i+2 < len(claude) && !strings.HasPrefix(claude[i+2], "--") {
		t.Errorf("claude allows more than the advisor: %q", claude)
	}

	codex := runPreset(t, "codex")
	srv := after(codex, "-c")
	if !slices.Contains(codex, "--ephemeral") {
		t.Errorf("codex keeps session files: %q", codex)
	}
	if after(codex, "-s") != "read-only" || !strings.HasPrefix(srv, "mcp_servers.wowc-advisor={ command = 'C:\\bin\\wowc.exe', args = ['serve', '--stdio']") ||
		!strings.Contains(srv, `'WOWC_SAVEDVARIABLES' = 'C:\WoW\f.lua'`) || !strings.HasSuffix(srv, `default_tools_approval_mode = "approve" }`) {
		t.Errorf("codex argv: %q", codex)
	}
	for _, a := range codex {
		if strings.Contains(a, "danger") || strings.Contains(a, "bypass") {
			t.Errorf("codex opens up the sandbox: %q", codex)
		}
	}

	agy := runPreset(t, "agy")
	if agy[0] != "-p" || strings.Contains(strings.Join(agy, " "), "skip-permissions") || strings.Contains(strings.Join(agy, " "), "--mode") {
		t.Errorf("agy argv: %q", agy)
	}

	gemini := runPreset(t, "gemini")
	pol := after(gemini, "--policy")
	if after(gemini, "--allowed-mcp-server-names") != "wowc-advisor" || filepath.Base(pol) != "gemini-policy.toml" {
		t.Errorf("gemini argv: %q", gemini)
	}
	if strings.Contains(strings.Join(gemini, " "), "yolo") {
		t.Errorf("gemini auto-approves everything: %q", gemini)
	}
}

func TestArgvWithoutMCPConfigDropsItsFlags(t *testing.T) {
	for name, want := range map[string]string{"claude": "--mcp-config", "codex": "-c", "gemini": "--policy"} {
		for _, a := range Argv(Presets[name], "Q", "") {
			if a == want {
				t.Errorf("%s keeps %s without a config: %v", name, want, Argv(Presets[name], "Q", ""))
			}
		}
	}
}

func TestPromptForbidsPermissionCoaching(t *testing.T) {
	p := Prompt(Question{Character: "A-B", Text: "Q"})
	for _, want := range []string{"Never tell the player to change permissions", ToolsUnavailable, "wowc-advisor MCP tools"} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt lacks %q", want)
		}
	}
}

func TestPromptRequiresUpgradePaths(t *testing.T) {
	p := Prompt(Question{Character: "Realm:Name", Text: "How do I get the shawl?"})
	if !strings.Contains(p, "call upgrade_paths") || !strings.Contains(p, "never say a source is unknown without it") {
		t.Errorf("prompt does not require upgrade_paths: %s", p)
	}
	if got := Sources("used upgrade_paths"); len(got) != 1 || got[0] != "upgrade_paths" {
		t.Errorf("Sources = %v", got)
	}
}

func TestPromptCarriesLogisticsFraming(t *testing.T) {
	p := Prompt(Question{Character: "A-B", Text: "Q"})
	for _, want := range []string{"gameplay logistics planner", "one route of stops and legs", "Think in routes and errands", "while you're here"} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt lacks %q", want)
		}
	}
}

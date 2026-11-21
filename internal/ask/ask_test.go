package ask

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// TestMain makes the test binary double as the fake agent CLI: with FAKE_AGENT set it prints a canned
// answer (or fails or hangs) instead of running tests.
func TestMain(m *testing.M) {
	switch os.Getenv("FAKE_AGENT") {
	case "":
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
		fmt.Println(`{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":"Do the turn-in."}}}`)
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

func TestQuestions(t *testing.T) {
	qs, err := Questions(savedVars)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, q := range qs {
		ids = append(ids, q.ID)
	}
	if want := []string{"100-1", "100-2", "100-4"}; !reflect.DeepEqual(ids, want) {
		t.Errorf("ids = %v, want %v", ids, want)
	}
	if qs[1].Text != `Where is "Bob"?` || qs[1].AskedAt != 101 || qs[0].Answered != true {
		t.Errorf("fields wrong: %+v", qs)
	}
	if _, err := Questions("WoWCompanionDB = {"); err == nil {
		t.Error("malformed file should fail")
	}
	if qs, err := Questions("Other = 1\n"); err != nil || len(qs) != 0 {
		t.Errorf("no db: %v %v", qs, err)
	}
}

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

func TestCheckAnswers(t *testing.T) {
	for _, tc := range []struct {
		name, text string
		ok         bool
	}{
		{"empty answers", "WoWCompanionAnswers = { schema = 1, answers = {} }", true},
		{"full", `WoWCompanionAnswers = { schema = 1, answers = { ["a-1"] = { answered_at = 5, text = "x", sources = { "whats_next" } } } }`, true},
		{"wrong schema", `WoWCompanionAnswers = { schema = 2, answers = {} }`, false},
		{"missing text", `WoWCompanionAnswers = { schema = 1, answers = { ["a"] = { answered_at = 5 } } }`, false},
		{"missing time", `WoWCompanionAnswers = { schema = 1, answers = { ["a"] = { text = "x" } } }`, false},
		{"bad sources", `WoWCompanionAnswers = { schema = 1, answers = { ["a"] = { answered_at = 5, text = "x", sources = { 3 } } } }`, false},
		{"function call", `WoWCompanionAnswers = { schema = 1, answers = { ["a"] = { answered_at = os.time(), text = "x" } } }`, false},
		{"second assignment", `WoWCompanionAnswers = { schema = 1, answers = {} }` + "\nX = 1", false},
		{"other name", `Other = { schema = 1, answers = {} }`, false},
	} {
		if err := CheckAnswers(tc.text); (err == nil) != tc.ok {
			t.Errorf("%s: err = %v", tc.name, err)
		}
	}
}

func TestAnswersLuaEscapes(t *testing.T) {
	nasty := "line1\nline2 \"q\" \\ ]] --[[ \x01 end"
	text := AnswersLua(map[string]Answer{`i"d`: {AnsweredAt: 9, Text: nasty, Sources: []string{"whats_next"}}})
	if err := CheckAnswers(text); err != nil {
		t.Fatalf("%v\n%s", err, text)
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "Answers.lua")
	if err := WriteAnswers(p, map[string]Answer{`i"d`: {AnsweredAt: 9, Text: nasty}}); err != nil {
		t.Fatal(err)
	}
	if got := ReadAnswers(p)[`i"d`]; got.Text != nasty || got.AnsweredAt != 9 {
		t.Errorf("round trip: %+v", got)
	}
}

func TestPaths(t *testing.T) {
	sv, ans, err := Paths("/g/_classic_/WTF/Account/ACCT/SavedVariables/ForeverScribe.lua")
	if err != nil || sv != "/g/_classic_/WTF/Account/ACCT/SavedVariables/WoWCompanion.lua" ||
		ans != "/g/_classic_/Interface/AddOns/WoWCompanion_Answers/Answers.lua" {
		t.Errorf("unix: %q %q %v", sv, ans, err)
	}
	sv, ans, err = Paths(`C:\WoW\WTF\Account\A\SavedVariables\ForeverScribe.lua`)
	if err != nil || sv != `C:\WoW\WTF\Account\A\SavedVariables\WoWCompanion.lua` || ans != `C:\WoW\Interface\AddOns\WoWCompanion_Answers\Answers.lua` {
		t.Errorf("windows: %q %q %v", sv, ans, err)
	}
	if _, _, err := Paths("/tmp/ForeverScribe.lua"); err == nil {
		t.Error("expected an error")
	}
}

// fixture writes a SavedVariables file and returns a Config that runs the fake agent in mode.
func fixture(t *testing.T, mode string) Config {
	t.Helper()
	dir := t.TempDir()
	sv := filepath.Join(dir, "WoWCompanion.lua")
	if err := os.WriteFile(sv, []byte(savedVars), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_AGENT", mode)
	return Config{SavedVariables: sv, AnswersPath: filepath.Join(dir, "Answers.lua"),
		Command: []string{os.Args[0]}, Timeout: 5 * time.Second, Now: func() time.Time { return time.Unix(1000, 0) }}
}

// The end-to-end smoke test: a question goes in and a valid Answers.lua comes out.
func TestRunEndToEnd(t *testing.T) {
	cfg := fixture(t, "echo")
	res, err := Run(context.Background(), cfg)
	if err != nil || res.Answered != 2 || res.Failed != 0 {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
	raw, _ := os.ReadFile(cfg.AnswersPath)
	if err := CheckAnswers(string(raw)); err != nil {
		t.Fatalf("%v\n%s", err, raw)
	}
	got := ReadAnswers(cfg.AnswersPath)
	if _, ok := got["100-1"]; ok || len(got) != 2 {
		t.Errorf("answered question got an answer or count wrong: %v", got)
	}
	a := got["100-2"]
	if !strings.Contains(a.Text, `question=Where is "Bob"? char=Thrall-Doomhammer`) || a.AnsweredAt != 1000 ||
		!reflect.DeepEqual(a.Sources, []string{"whats_next"}) {
		t.Errorf("answer = %+v", a)
	}
	// A second pass finds nothing new and leaves the file alone.
	res, err = Run(context.Background(), cfg)
	if err != nil || res.Answered != 0 {
		t.Errorf("second pass = %+v, %v", res, err)
	}
	if len(ReadAnswers(cfg.AnswersPath)) != 2 {
		t.Error("answers lost on second pass")
	}
}

func TestRunFailures(t *testing.T) {
	for _, tc := range []struct {
		mode, want string
		timeout    time.Duration
		command    []string
	}{
		{"fail", "boom", 0, nil},
		{"failout", "exited with status 3", 0, nil},
		{"silent", "gave no answer", 0, nil},
		{"hang", "took longer than", 100 * time.Millisecond, nil},
		{"echo", "Couldn't find Agent", 0, []string{"/nonexistent/agent"}},
	} {
		cfg := fixture(t, tc.mode)
		if tc.timeout != 0 {
			cfg.Timeout = tc.timeout
		}
		if tc.command != nil {
			cfg.Command = tc.command
		}
		res, err := Run(context.Background(), cfg)
		if err != nil || res.Failed != 2 {
			t.Fatalf("%s: res = %+v, err = %v", tc.mode, res, err)
		}
		raw, _ := os.ReadFile(cfg.AnswersPath)
		if err := CheckAnswers(string(raw)); err != nil {
			t.Fatalf("%s: invalid file: %v", tc.mode, err)
		}
		if a := ReadAnswers(cfg.AnswersPath)["100-2"]; !strings.HasPrefix(a.Text, "Could not answer: ") || !strings.Contains(a.Text, tc.want) || len(a.Text) > 100 {
			t.Errorf("%s: text = %q", tc.mode, a.Text)
		}
	}
}

func TestRunLimitsAndCap(t *testing.T) {
	cfg := fixture(t, "long")
	cfg.MaxPerRun = 1
	res, err := Run(context.Background(), cfg)
	if err != nil || res.Answered != 1 || res.Pending != 1 {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
	if a := ReadAnswers(cfg.AnswersPath)["100-2"]; len([]rune(a.Text)) != MaxAnswer {
		t.Errorf("answer length = %d", len([]rune(a.Text)))
	}
	if _, err := Run(context.Background(), Config{SavedVariables: cfg.SavedVariables, AnswersPath: cfg.AnswersPath}); err != nil {
		t.Fatal(err) // already-answered 100-2 stays; 100-4 gets an error answer for the missing command
	}
	if a := ReadAnswers(cfg.AnswersPath)["100-4"]; !strings.Contains(a.Text, "no agent command") {
		t.Errorf("text = %q", a.Text)
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
	if res, err := Run(context.Background(), cfg); err != nil || res.Failed != 0 {
		t.Fatalf("%s: %+v %v", name, res, err)
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
	for i, a := range claude {
		if a == "--allowedTools" && claude[i+2] != "--disallowedTools" {
			t.Errorf("claude allows more than the advisor: %q", claude)
		}
	}
	for _, tool := range []string{"Bash", "Edit", "Write", "WebFetch", "Read"} {
		if !strings.Contains(strings.Join(claude, " "), " "+tool) {
			t.Errorf("claude does not deny %s", tool)
		}
	}

	codex := runPreset(t, "codex")
	srv := after(codex, "-c")
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

package ask

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// fakeClient links the test binary (the fake agent) as the named client, so the argv logic that
// keys on the program name runs, and returns the arguments the fake recorded.
func fakeClient(t *testing.T, name string, o Options) []string {
	t.Helper()
	cfg := fixture(t, "argv")
	bin := filepath.Join(t.TempDir(), name+filepath.Ext(os.Args[0]))
	if err := os.Symlink(os.Args[0], bin); err != nil {
		t.Skip("no symlinks here:", err)
	}
	out := filepath.Join(t.TempDir(), "argv")
	t.Setenv("FAKE_ARGV_OUT", out)
	tail := map[string][]string{"claude": {"-p", "{prompt}"}, "codex": {"exec", "{prompt}"}, "gemini": {"-p", "{prompt}"}}[name]
	cfg.Command = append([]string{bin}, tail...)
	cfg.Options = o
	if _, err := runAgent(context.Background(), cfg, Question{ID: "1", Character: "R:C", Text: "What now?"}); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(string(raw), "\x00")
}

func TestModelArgsPerClient(t *testing.T) {
	for _, tc := range []struct {
		client string
		o      Options
		want   []string // consecutive arguments that must appear
		not    string   // an argument that must not
	}{
		{"claude", Options{}, []string{"--model", "haiku"}, ""},
		{"claude", Options{Tier: TierBalanced}, []string{"--model", "sonnet"}, ""},
		{"claude", Options{Tier: TierBest}, []string{"--model", "opus"}, ""},
		{"claude", Options{Tier: TierCustom, Custom: "claude-opus-5-5"}, []string{"--model", "claude-opus-5-5"}, ""},
		{"claude", Options{}, []string{"--max-turns", "10"}, ""},
		{"gemini", Options{}, []string{"--model", "flash-lite"}, ""},
		{"gemini", Options{Tier: TierBalanced}, []string{"--model", "flash"}, ""},
		{"gemini", Options{Tier: TierBest}, []string{"--model", "pro"}, ""},
		{"claude", Options{}, []string{"--effort", "low"}, ""},
		{"claude", Options{Reasoning: "medium"}, []string{"--effort", "medium"}, ""},
		{"claude", Options{Tier: TierBest, Reasoning: "high"}, []string{"--effort", "high"}, ""},
		{"gemini", Options{Reasoning: "high"}, []string{"--model", "flash-lite"}, "--effort"},
		{"codex", Options{}, []string{"-c", `model_reasoning_effort="low"`}, "--model"},
		{"codex", Options{Tier: TierBest, Reasoning: "high"}, []string{"-c", `model_reasoning_effort="high"`}, "--model"},
		{"codex", Options{Tier: TierCustom, Custom: "my-model"}, []string{"--model", "my-model"}, ""},
	} {
		got := fakeClient(t, tc.client, tc.o)
		found := false
		for i := range got {
			if i+len(tc.want) <= len(got) && slices.Equal(got[i:i+len(tc.want)], tc.want) {
				found = true
			}
		}
		if !found || (tc.not != "" && slices.Contains(got, tc.not)) {
			t.Errorf("%s %+v: argv %q", tc.client, tc.o, got)
		}
	}
	if got := fakeClient(t, "codex", Options{}); got[0] != "exec" {
		t.Errorf("codex flags must follow exec: %q", got)
	}
}

func TestUnknownCommandGetsNoModelFlags(t *testing.T) {
	if args, _ := ModelArgs([]string{"my-agent"}, Options{Tier: TierBest}); len(args) != 0 {
		t.Errorf("%q", args)
	}
}

func TestOptionsNormalize(t *testing.T) {
	for _, tc := range []struct{ in, want Options }{
		{Options{}, Options{Length: LengthStandard, Tier: TierEfficient, Reasoning: "low"}},
		{Options{Length: "loud", Tier: "huge", Custom: "x", Reasoning: "max"}, Options{Length: LengthStandard, Tier: TierEfficient, Custom: "x", Reasoning: "low"}},
		{Options{Length: LengthBrief, Tier: TierCustom, Custom: "--evil"}, Options{Length: LengthBrief, Tier: TierEfficient, Custom: "--evil", Reasoning: "low"}},
		{Options{Length: LengthDetailed, Tier: TierCustom, Custom: " m-1 ", Reasoning: "high"}, Options{Length: LengthDetailed, Tier: TierCustom, Custom: "m-1", Reasoning: "high"}},
	} {
		if got := tc.in.Normalize(); got != tc.want {
			t.Errorf("%+v -> %+v, want %+v", tc.in, got, tc.want)
		}
	}
}

func TestPromptLengthInstructions(t *testing.T) {
	q := Question{Character: "C-R", Text: "What next?"}
	for length, want := range map[string]string{LengthBrief: "at most 1 bullet", LengthStandard: "under 1500 characters", LengthDetailed: "behind the answer", "": "under 1500 characters"} {
		p := PromptFor(q, length)
		if !strings.Contains(p, want) || !strings.Contains(p, "What next?") {
			t.Errorf("%q: %s", length, p)
		}
		// Every length carries the short-answer rules.
		for _, rule := range []string{"Answer the question asked in your first sentence", "at most 3 short bullets", "Never restate or re-list the plan", "Heads-up:", "rather than contradicting it silently"} {
			if !strings.Contains(p, rule) {
				t.Errorf("%q: prompt lacks %q", length, rule)
			}
		}
		if strings.Contains(p, "numbered plan") {
			t.Errorf("%q: still asks for a numbered plan", length)
		}
	}
	if n := len(PromptFor(Question{}, LengthDetailed)); n > 1650 {
		t.Errorf("prompt grew to %d bytes", n)
	}
}

func TestUsageLine(t *testing.T) {
	if got := (Usage{Tokens: 3123, Model: "haiku"}).Line(); got != "≈3.1k tokens · haiku" {
		t.Errorf("%q", got)
	}
	if got := (Usage{Tokens: 56700, In: 4100, Out: 600, Cached: 52000, Model: "haiku"}).Line(); got != "≈4.1k in · 600 out · 52k cached · haiku" {
		t.Errorf("%q", got)
	}
	if got := (Usage{Tokens: 900, In: 700, Out: 200}).Line(); got != "≈700 in · 200 out" {
		t.Errorf("%q", got)
	}
	if got := (Usage{Model: "default model"}).Line(); got != "default model" {
		t.Errorf("%q", got)
	}
	for n, want := range map[int]string{0: "", 850: "≈850 tokens", 3123: "≈3.1k tokens", 25400: "≈25k tokens"} {
		if got := (Usage{Tokens: n}).Line(); got != want {
			t.Errorf("%d: %q", n, got)
		}
	}
}

func TestUsageFromStreamJSON(t *testing.T) {
	cfg := fixture(t, "stream")
	cfg.Command = []string{os.Args[0]}
	text, streamed, u, err := RespondUsage(context.Background(), cfg, Question{Text: "q"}, "p", true, func(string) {})
	if err != nil || !streamed || text != "Do the turn-in." || u.Tokens != 3100 || u.CostUSD != 0.0123 || u.Model != "claude-haiku-4-5" {
		t.Fatalf("%q %v %+v %v", text, streamed, u, err)
	}
}

// The defaults are each client's smallest model and lowest reasoning level; a client without a
// reasoning flag, or a custom command, gets no reasoning option.
func TestDefaultsAreSmallestAndLowest(t *testing.T) {
	for _, c := range []string{"claude", "codex", "gemini"} {
		ch := ChoicesFor(c)
		if ch.Models[0].Tier != TierEfficient || !strings.Contains(ch.Models[0].Label, "small") || ch.Models[len(ch.Models)-1].Tier != TierCustom {
			t.Errorf("%s: %+v", c, ch.Models)
		}
		if c != "gemini" && (len(ch.Reasoning) != 3 || ch.Reasoning[0] != "low") {
			t.Errorf("%s: %v", c, ch.Reasoning)
		}
		if ch.ModelNote == "" || ch.ReasoningNote == "" {
			t.Errorf("%s: tooltips missing: %+v", c, ch)
		}
	}
	if ch := ChoicesFor("gemini"); len(ch.Reasoning) != 0 {
		t.Errorf("gemini has no reasoning flag: %v", ch.Reasoning)
	}
	if ch := ChoicesFor(""); len(ch.Models) != 0 || len(ch.Reasoning) != 0 || ch.ModelNote == "" {
		t.Errorf("custom command: %+v", ch)
	}
	if d := (Options{}).Normalize(); d.Tier != TierEfficient || d.Reasoning != "low" {
		t.Errorf("%+v", d)
	}
	if m := ModelUsed([]string{"claude"}, Options{}); m != "haiku" {
		t.Error(m)
	}
	if m := ModelUsed([]string{"codex"}, Options{}); m != "default model" {
		t.Error(m)
	}
}

// A CLI that rejects the reasoning flag is run once more without it, with a notice; other clients
// and other failures are not retried.
func TestRejectedReasoningFlagIsDropped(t *testing.T) {
	for _, tc := range []struct{ client, flag string }{{"claude", "--effort"}, {"codex", "-c"}} {
		cfg := fixture(t, "rejecteffort")
		bin := filepath.Join(t.TempDir(), tc.client+filepath.Ext(os.Args[0]))
		if err := os.Symlink(os.Args[0], bin); err != nil {
			t.Skip("no symlinks here:", err)
		}
		out := filepath.Join(t.TempDir(), "argv")
		t.Setenv("FAKE_ARGV_OUT", out)
		cfg.Command = []string{bin, "-p", "{prompt}"}
		var notice string
		cfg.Notice = func(n string) { notice = n }
		var buf strings.Builder
		if err := execAgent(context.Background(), cfg, Question{Text: "q"}, "p", nil, &buf); err != nil {
			t.Fatalf("%s: %v", tc.client, err)
		}
		raw, _ := os.ReadFile(out)
		if got := string(raw); strings.Contains(got, "--effort") || strings.Contains(got, "model_reasoning_effort") || !strings.Contains(got, "--model") && tc.client == "claude" {
			t.Errorf("%s retried with %q", tc.client, got)
		}
		if notice == "" {
			t.Errorf("%s: no notice", tc.client)
		}
	}
	cfg := fixture(t, "fail") // an unrelated failure is not retried
	cfg.Command = []string{os.Args[0]}
	n := 0
	cfg.Notice = func(string) { n++ }
	if err := execAgent(context.Background(), cfg, Question{}, "p", nil, &strings.Builder{}); err == nil || n != 0 {
		t.Errorf("%v %d", err, n)
	}
}

func TestAgyOptionsAndDetection(t *testing.T) {
	// Efficient keeps agy's own model (no stable alias); reasoning maps to --effort; Custom names a model.
	for _, tc := range []struct {
		o    Options
		want []string
	}{
		{Options{}, []string{"--effort", "low"}},
		{Options{Reasoning: "high"}, []string{"--effort", "high"}},
		{Options{Tier: TierCustom, Custom: "some-model", Reasoning: "medium"}, []string{"--model", "some-model", "--effort", "medium"}},
		{Options{noReasoning: true}, nil},
	} {
		if got, at := ModelArgs([]string{"agy"}, tc.o); !reflect.DeepEqual(got, tc.want) || at != 1 && len(got) > 0 {
			t.Errorf("%+v: %q at %d, want %q", tc.o, got, at, tc.want)
		}
	}
	if c := ChoicesFor("agy"); len(c.Reasoning) != 3 || c.Models[0].Tier != TierEfficient {
		t.Errorf("agy choices: %+v", c)
	}
	if got := WithModel([]string{"agy", "-p", "Q"}, []string{"agy"}, Options{}); !reflect.DeepEqual(got, []string{"agy", "--effort", "low", "-p", "Q"}) {
		t.Errorf("agy argv: %q", got)
	}
	if want := []string{"claude", "codex", "agy", "gemini"}; !reflect.DeepEqual(DetectOrder, want) {
		t.Errorf("DetectOrder %q", DetectOrder)
	}
}

func TestAgyWorkspaceHoldsOnlyTheAdvisor(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "mcp.json")
	if err := WriteAgyWorkspace(cfg, MCPConfig("/bin/wowc", map[string]string{"WOWC_SAVEDVARIABLES": "/f.lua"})); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(AgyWorkspace(cfg), ".agents", "mcp_config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Servers map[string]struct {
			Command string
			Args    []string
			Env     map[string]string
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil || len(doc.Servers) != 1 || doc.Servers[ServerName].Command != "/bin/wowc" ||
		doc.Servers[ServerName].Env["WOWC_SAVEDVARIABLES"] != "/f.lua" {
		t.Errorf("workspace config: %s %v", raw, err)
	}
	if AgyRule != "mcp(wowc-advisor/*)" {
		t.Errorf("rule %q", AgyRule)
	}
}

func TestGeminiIsLegacyInChoices(t *testing.T) {
	// Gemini CLI stays selectable but is only found after every other client.
	if DetectOrder[len(DetectOrder)-1] != "gemini" {
		t.Errorf("gemini is not last: %q", DetectOrder)
	}
	if _, ok := Presets["gemini"]; !ok {
		t.Error("gemini preset removed")
	}
}

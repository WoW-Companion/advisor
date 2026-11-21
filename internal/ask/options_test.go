package ask

import (
	"context"
	"os"
	"path/filepath"
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
	if res, err := Run(context.Background(), cfg); err != nil || res.Failed != 0 {
		t.Fatalf("%s: %+v %v", name, res, err)
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
		{"codex", Options{}, []string{"-c", `model_reasoning_effort="low"`}, "--model"},
		{"codex", Options{Tier: TierBest}, []string{"-c", `model_reasoning_effort="high"`}, "--model"},
		{"codex", Options{Tier: TierCustom, Custom: "my-model"}, []string{"--model", "my-model"}, "-c"},
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
		{Options{}, Options{LengthStandard, TierEfficient, ""}},
		{Options{"loud", "huge", "x"}, Options{LengthStandard, TierEfficient, "x"}},
		{Options{LengthBrief, TierCustom, "--evil"}, Options{LengthBrief, TierEfficient, "--evil"}},
		{Options{LengthDetailed, TierCustom, " m-1 "}, Options{LengthDetailed, TierCustom, "m-1"}},
	} {
		if got := tc.in.Normalize(); got != tc.want {
			t.Errorf("%+v -> %+v, want %+v", tc.in, got, tc.want)
		}
	}
}

func TestPromptLengthInstructions(t *testing.T) {
	q := Question{Character: "C-R", Text: "What next?"}
	for length, want := range map[string]string{LengthBrief: "top 1-3 actions", LengthStandard: "short numbered plan", LengthDetailed: "alternatives", "": "short numbered plan"} {
		if p := PromptFor(q, length); !strings.Contains(p, want) || !strings.Contains(p, "What next?") {
			t.Errorf("%q: %s", length, p)
		}
	}
	if strings.Contains(PromptFor(q, LengthBrief), "alternatives") {
		t.Error("brief mentions alternatives")
	}
	if n := len(PromptFor(Question{}, LengthDetailed)); n > 900 {
		t.Errorf("prompt grew to %d bytes", n)
	}
}

func TestUsageLine(t *testing.T) {
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
	if err != nil || !streamed || text != "Do the turn-in." || u.Tokens != 3100 || u.CostUSD != 0.0123 {
		t.Fatalf("%q %v %+v %v", text, streamed, u, err)
	}
}

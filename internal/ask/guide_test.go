package ask

import (
	"slices"
	"strings"
	"testing"
)

func TestWithGuide(t *testing.T) {
	claude := Presets["claude"]
	base := Argv(claude, "p", "/tmp/mcp.json")
	got := WithGuide(base, claude, "What talents should I get?")
	if i := slices.Index(got, "--tools"); i < 0 || got[i+1] != "WebFetch" {
		t.Errorf("--tools = %v", got)
	}
	i := slices.Index(got, "--allowedTools")
	if i < 0 || got[i+1] != "mcp__"+ServerName || got[i+2] != "WebFetch(domain:icy-veins.com)" {
		t.Errorf("--allowedTools = %v", got[i:])
	}
	if base[slices.Index(base, "--tools")+1] != "" {
		t.Error("the preset's argv was changed")
	}
	// Not a talent question, not Claude, or a custom command: unchanged, so no web tool.
	for name, c := range map[string]struct {
		cmd []string
		q   string
	}{
		"other question": {claude, "What quest next?"},
		"codex":          {Presets["codex"], "What talents should I get?"},
		"agy":            {Presets["agy"], "What talents should I get?"},
		"custom":         {append(slices.Clone(claude), "--dangerously-skip-permissions"), "What talents should I get?"},
	} {
		argv := Argv(c.cmd, "p", "/tmp/mcp.json")
		if got := WithGuide(argv, c.cmd, c.q); !slices.Equal(got, argv) || CanFetchGuide(c.cmd, c.q) {
			t.Errorf("%s: argv changed or fetch allowed: %v", name, got)
		}
	}
}

func TestTalentPromptAsksOnceAndRestrictsFetch(t *testing.T) {
	q := Question{Character: "Realm:Testwar", Text: "What talents should I get?"}
	web := Config{Command: Presets["claude"]}.Prompt(q)
	for _, want := range []string{"call get_character_profile first", "needs_profile is true, answer with only its short multiple-choice question", "fetch only icy-veins.com pages", "no guide text", "Never invent talent names"} {
		if !strings.Contains(web, want) {
			t.Errorf("claude prompt lacks %q", want)
		}
	}
	other := Config{Command: Presets["codex"]}.Prompt(q)
	if strings.Contains(other, "fetch only") || !strings.Contains(other, "you cannot fetch a guide here") {
		t.Errorf("a client without the narrow fetch must show the link: %s", other)
	}
	if plain := (Config{Command: Presets["claude"]}).Prompt(Question{Text: "What quest next?"}); strings.Contains(plain, "Talents:") {
		t.Error("talent rule added to another question")
	}
}

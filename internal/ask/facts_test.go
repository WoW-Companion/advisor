package ask

import (
	"strings"
	"testing"
)

const syntheticCapture = "../../testdata/foreverscribe/level30_caster.lua"

// promptBudget is the most characters the pre-fetched prompt may reach for the synthetic character
// (about 600 tokens); raise it only on purpose, as every ask pays for it.
const promptBudget = 2500

func TestFactsPromptStaysWithinBudget(t *testing.T) {
	cfg := Config{SavedVariables: syntheticCapture}
	q := Question{Character: "Testrealm:Testmage", Text: "What should I do next?"}
	facts := Facts(cfg, q)
	if facts == "" {
		t.Fatal("no facts were pre-fetched")
	}
	p := cfg.Prompt(q)
	if n := len([]rune(p)); n > promptBudget {
		t.Errorf("prompt grew to %d characters, budget %d", n, promptBudget)
	}
	if !strings.Contains(p, facts) || !strings.HasSuffix(p, "Question: What should I do next?") {
		t.Errorf("facts or question missing from the prompt")
	}
}

func TestFactsAreCutAndOptional(t *testing.T) {
	if got := capRunes(strings.Repeat("é", FactsBudget+50), FactsBudget); len([]rune(got)) > FactsBudget+60 || !strings.Contains(got, "[cut") {
		t.Errorf("not cut: %d runes", len([]rune(got)))
	}
	for _, cfg := range []Config{{}, {SavedVariables: "no-such-file.lua"}, {SavedVariables: syntheticCapture}} {
		q := Question{Character: "Nobody:Nowhere", Text: "x"}
		if cfg.SavedVariables == "" || cfg.SavedVariables == "no-such-file.lua" || true {
			if got := Facts(cfg, q); got != "" {
				t.Errorf("%+v: facts for an unknown character: %q", cfg, got)
			}
		}
		if cfg.Prompt(q) != PromptFor(q, LengthStandard) {
			t.Errorf("%+v: prompt changed without facts", cfg)
		}
	}
}

func TestFactsWithoutDatasetSaySo(t *testing.T) {
	cfg := Config{SavedVariables: syntheticCapture}
	got := Facts(cfg, Question{Character: "Testrealm:Testmage", Text: "What now?"})
	if !strings.Contains(got, "Game data not downloaded yet; see Setup") {
		t.Errorf("facts without a dataset do not say so: %q", got)
	}
}

func TestFactsCarryProgressAndGivers(t *testing.T) {
	cfg := Config{SavedVariables: "../../testdata/foreverscribe/quest_progress.lua"}
	p := cfg.Prompt(Question{Character: "Testrealm:Testscout", Text: "What should I do next?"})
	for _, want := range []string{"3/8 Test Claws", "approximate"} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt lacks %q", want)
		}
	}
	// A met NPC is never called a giver on the strength of having met them.
	if strings.Contains(p, "may have quests") || strings.Contains(p, "Quest giver you've met") {
		t.Errorf("prompt calls a met NPC a giver: %s", p)
	}
	if n := len([]rune(p)); n > promptBudget {
		t.Errorf("prompt is %d characters, budget %d", n, promptBudget)
	}
}

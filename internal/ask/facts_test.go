package ask

import (
	"github.com/WoW-Companion/advisor/internal/digest"
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

// A class quest the dataset has never heard of (a synthetic id) still yields its own facts: the
// ids, giver, zone, level, class flag and why come from the entry, not from a name lookup.
func TestSubjectFactsComeFromTheEntry(t *testing.T) {
	line := digest.Line{Text: "Class quest: Test Bulwark", Quest: 999001, Kind: digest.KindPickup, Title: "Test Bulwark", Level: 10, Class: true,
		NPC: "Testgiver Skyward", Zone: "Testzone", Why: "Class quests usually give a class reward.", Ref: "quest:999001", Details: []string{"Pick it up from Testgiver Skyward."}}
	d := digest.Digest{Sections: []digest.Section{{ID: digest.NextQuests, Lines: []digest.Line{line}}}}
	sub, ok := d.Subject("quest:999001")
	if !ok || sub.Line == nil {
		t.Fatalf("subject not found: %+v", sub)
	}
	got := entryFacts(Config{}, loaded{}, sub)
	for _, want := range []string{"Quest id: 999001", "Giver or turn-in NPC: Testgiver Skyward", "Zone: Testzone", "Level: 10", "Class quest: yes", "Why it is listed: Class quests usually give"} {
		if !strings.Contains(got, want) {
			t.Errorf("facts lack %q:\n%s", want, got)
		}
	}
}

func TestGroundingRulesForbidNameOnlyLookupsAndNoMatch(t *testing.T) {
	for _, want := range []string{"answer from these facts first", "by id", "never by name alone", "no match", "never invent", "Never invent"} {
		if !strings.Contains(GroundingRules, want) && !strings.Contains(strings.ToLower(GroundingRules), strings.ToLower(want)) {
			t.Errorf("rules lack %q", want)
		}
	}
}

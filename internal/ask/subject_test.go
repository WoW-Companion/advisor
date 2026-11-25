package ask

import (
	"path/filepath"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/profile"
	"strings"
	"testing"
)

func nameList(es []Entity) string {
	var out []string
	for _, e := range es {
		out = append(out, e.Name)
	}
	return strings.Join(out, "|")
}

// The chip shows what the answer is about: known names found in the answer text, first mention first.
func TestSubjectsFollowTheAnswerText(t *testing.T) {
	known := []Entity{
		{Kind: "quest", ID: 2, Name: "Unnerving Silence"},
		{Kind: "npc", ID: 3, Name: "Seena Skybreaker"},
		{Kind: "quest", ID: 1, Name: "The Skybreaker Bulwark"},
		{Kind: "recipe", ID: 4, Name: "Runed Copper Gauntlets"},
		{Kind: "zone", Name: "Zephras Isle"},
		{Kind: "item", ID: 5, Name: "Sky"}, // too short to count
	}
	for _, tc := range []struct{ name, answer, want string }{
		{"what next: the lead quest, then the giver, never the heads-up",
			"Pick up The Skybreaker Bulwark from Seena Skybreaker. Heads-up: turn in Unnerving Silence.",
			"The Skybreaker Bulwark|Seena Skybreaker|Unnerving Silence"},
		{"gauntlets", "You can make Runed Copper Gauntlets now; the trainer is in Zephras Isle.", "Runed Copper Gauntlets|Zephras Isle"},
		{"case and punctuation do not matter", "go to seena skybreaker's hut", "Seena Skybreaker"},
		{"a name inside another name is not a second subject", "Hand in the skybreaker bulwark.", "The Skybreaker Bulwark"},
		{"nothing named", "Keep killing wolves.", ""},
	} {
		if got := nameList(Subjects(tc.answer, known)); got != tc.want {
			t.Errorf("%s: %q, want %q", tc.name, got, tc.want)
		}
	}
	if got := nameList(Subjects("Take The Skybreaker Bulwark, The Skybreaker Bulwark.", known)); got != "The Skybreaker Bulwark" {
		t.Errorf("a repeat adds a card name: %q", got)
	}
}

func TestSavedNoteIsAddedOnceFromTheTool(t *testing.T) {
	note := "Saved: Fury. Change it any time in Settings → Play Style."
	if got := WithSaved("Fury it is.", note); got != "Fury it is.\n\n"+note {
		t.Errorf("%q", got)
	}
	if got := WithSaved("Done.\n\n"+note, note); strings.Count(got, "Saved: Fury") != 1 {
		t.Errorf("twice: %q", got)
	}
	if got := WithSaved("No save here.", ""); got != "No save here." {
		t.Errorf("%q", got)
	}
	var tr Trace
	tr.Add("mcp__wowc-advisor__set_character_profile", `{"character":"x","saved_note":"`+note+`"}`)
	tr.Add("mcp__wowc-advisor__get_character_profile", `{"character":"x"}`)
	if tr.Saved() != note {
		t.Errorf("saved = %q", tr.Saved())
	}
}

// Every answer, on both paths, carries the one wording rule.
func TestWordingRulesAreInEveryPrompt(t *testing.T) {
	for _, length := range []string{LengthBrief, LengthStandard, LengthDetailed} {
		p := PromptFor(Question{Character: "k", Text: "Who trains that?"}, length)
		for _, want := range []string{"lead with the answer", "Never mention the advisor, data", "best practical step", "You can make these now.", "Never say something was saved unless the save tool succeeded", "Soon (level N)"} {
			if !strings.Contains(p, want) {
				t.Errorf("%s: prompt lacks %q", length, want)
			}
		}
		for _, old := range []string{"name it and the short reason", "lead with the skill check"} {
			if strings.Contains(p, old) {
				t.Errorf("%s: prompt still says %q", length, old)
			}
		}
	}
}

// A talent answer, and a spec just chosen, are subjects: the chip follows them.
func TestChipFollowsTalentAndSpecAnswers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profiles.json")
	t.Setenv(profile.Env, path)
	key := priestKey(t)
	if _, err := profile.Update(path, key, func(e *profile.Entry) error {
		e.Profile = &character.Profile{Role: "healer", Focus: "dungeons", Spec: "Test Priest"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	known := KnownEntities(Config{SavedVariables: priestCapture}, key)
	for _, tc := range []struct{ answer, want string }{
		{"Take Test Inner Focus (rank 5): it saves mana.", "Test Inner Focus"},
		{"Saved: Test Priest. Change it any time in Settings.", "Test Priest"},
	} {
		got := Subjects(tc.answer, known)
		if len(got) == 0 || got[0].Name != tc.want {
			t.Errorf("%q: subjects %q, want %q first", tc.answer, nameList(got), tc.want)
		}
	}
}

// Only a What next? answer may end with a heads-up.
func TestHeadsUpOnlyInWhatNext(t *testing.T) {
	const allowed = "One optional last line starting"
	for _, tc := range []struct {
		text, ref string
		want      bool
	}{
		{"What should I do next, and why?", "quest:12", true},
		{"What next?", "", true},
		{"Which talent should I take next for my play style, and why?", "talents", false},
		{"Look at my gear and suggest the best upgrades within reach right now.", "upgrades", false},
		{"Where do I find copper?", "", false},
	} {
		p := PromptFor(Question{Character: "C", Text: tc.text, Ref: tc.ref}, LengthStandard)
		if got := strings.Contains(p, allowed); got != tc.want {
			t.Errorf("%q: heads-up allowed = %v", tc.text, got)
		}
		if !strings.Contains(p, "Never end with a") && !tc.want {
			t.Errorf("%q: no ban on a heads-up", tc.text)
		}
	}
}

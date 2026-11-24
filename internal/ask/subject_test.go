package ask

import (
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

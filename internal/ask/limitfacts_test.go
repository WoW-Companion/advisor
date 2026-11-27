package ask

import (
	"strings"
	"testing"
)

func TestLimitFactsPvP(t *testing.T) {
	for _, tc := range []struct{ q, want string }{
		{"what level can I join Warsong Gulch?", "Battlegrounds:"},
		{"which battleground bracket am I in at level 37?", "Battlegrounds:"},
		{"how many honor points do I need for rank 10?", "Honor and PvP ranks:"},
		{"what does the honor vendor sell for a Knight-Captain?", "Honor and PvP ranks:"},
		{"how many Marks of Honor for the Arathi Basin reward?", "Honor and PvP ranks:"},
		{"what do I need to become Grand Marshal?", "Honor and PvP ranks:"},
		{"how much honor does a kill give?", "Honor and PvP ranks:"},
		{"what is the best pvp gear for my class?", "Honor and PvP ranks:"},
	} {
		if got := LimitFacts(tc.q); !strings.Contains(got, tc.want) || !strings.Contains(got, "Say so") {
			t.Errorf("LimitFacts(%q) = %q, want %q", tc.q, got, tc.want)
		}
	}
	for _, q := range []string{"where is the weapon master in the Valley of Honor?", "how do I get to the Valley of Honor, how much does the trainer charge?", "what does rank 3 Frostbolt cost?", "I am honored with Orgrimmar, what next?", "what quest should I do next?"} {
		if got := LimitFacts(q); got != "" {
			t.Errorf("LimitFacts(%q) = %q, want none", q, got)
		}
	}
}

func TestLimitFactsRidingAndClassTrainer(t *testing.T) {
	for _, tc := range []struct{ q, want string }{
		{"how much does Apprentice Riding cost?", "Riding and mounts:"},
		{"where is the riding trainer for orcs?", "Riding and mounts:"},
		{"do I need reputation for my racial mount?", "Riding and mounts:"},
		{"how much gold is the Black War Steed?", "Riding and mounts:"},
		{"how much does my mage trainer charge?", "Class trainer prices:"},
		{"what does the class trainer charge for my new spells?", "Class trainer prices:"},
		{"how much does it cost to train my new spells?", "Class trainer prices:"},
	} {
		if got := LimitFacts(tc.q); !strings.Contains(got, tc.want) || !strings.Contains(got, "Say so") {
			t.Errorf("LimitFacts(%q) = %q, want %q", tc.q, got, tc.want)
		}
	}
	for _, q := range []string{"how much does the alchemy trainer charge?", "what does rank 3 Frostbolt cost?", "how do I get to Blackrock Mountain?", "where is the weapon master in Orgrimmar?", "which spells does my mage learn at level 20?"} {
		if got := LimitFacts(q); got != "" {
			t.Errorf("LimitFacts(%q) = %q, want none", q, got)
		}
	}
}

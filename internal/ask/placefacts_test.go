package ask

import (
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
)

func TestTrainerWhereFactsListsTheClassCitiesAndNoNPC(t *testing.T) {
	c := character.Character{Class: "WARRIOR", Faction: "Horde"}
	for _, tc := range []struct {
		q    string
		want []string
		no   []string
	}{
		{"where is the mage trainer in Orgrimmar?", []string{"a mage trainer stands in (Horde: Undercity, Orgrimmar; Alliance: Stormwind City, Ironforge)"}, nil},
		{"what are the coordinates of the druid trainer?", []string{"Horde: Thunder Bluff; Alliance: Darnassus"}, []string{"Orgrimmar"}},
		{"where do I train my spells?", []string{"a warrior trainer stands in"}, nil},
		{"where is the weapon master in Orgrimmar?", nil, nil},
		{"where is the alchemy trainer?", nil, nil},
	} {
		got := TrainerWhereFacts(c, tc.q)
		if len(tc.want) == 0 && got != "" {
			t.Errorf("%q: facts for a question that is none: %q", tc.q, got)
		}
		for _, w := range tc.want {
			if !strings.Contains(got, w) || !strings.Contains(got, "holds no class trainer NPC") {
				t.Errorf("%q: %q lacks %q or the no-NPC statement", tc.q, got, w)
			}
		}
		for _, n := range tc.no {
			if strings.Contains(got, n) {
				t.Errorf("%q: names %s", tc.q, n)
			}
		}
	}
}

func TestSummonStoneFactsSaysTheDataHoldsNoStone(t *testing.T) {
	got := SummonStoneFacts(nil, "where is the summoning stone for Deadmines?")
	if !strings.Contains(got, "holds no summoning stone row") || !strings.Contains(got, "give no stone coordinates") {
		t.Errorf("facts = %q", got)
	}
	if got := SummonStoneFacts(nil, "how many soul shards does Summon Imp use?"); got != "" {
		t.Errorf("facts for a question about a spell: %q", got)
	}
}

func TestProficiencyFactsNameTheClassAskedAbout(t *testing.T) {
	c := character.Character{Class: "WARRIOR"}
	got := ProficiencyFacts(c, nil, "can my mage wear plate armor?")
	if !strings.Contains(got, "Mage:") || !strings.Contains(got, "Armor: cloth at every level. Cannot use a shield.") || strings.Contains(got, "Warrior:") {
		t.Errorf("facts = %q", got)
	}
	if got := ProficiencyFacts(c, nil, "what armor can I wear?"); !strings.Contains(got, "Warrior:") || !strings.Contains(got, "Armor: mail until level 40, then plate.") {
		t.Errorf("own class facts = %q", got)
	}
	if got := ProficiencyFacts(c, nil, "can a priest use a sword?"); !strings.Contains(got, "Priest:") || strings.Contains(got, "Warrior:") {
		t.Errorf("weapon question facts = %q", got)
	}
	if got := ProficiencyFacts(c, nil, "where is the alchemy trainer?"); got != "" {
		t.Errorf("facts for a question that is none: %q", got)
	}
}

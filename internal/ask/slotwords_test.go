package ask

import (
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
)

func TestProficiencyFactsReachShieldRelicAndJewelleryQuestions(t *testing.T) {
	c := character.Character{Class: "MAGE"}
	for _, tc := range []struct {
		q    string
		want []string
	}{
		{"can a paladin use a shield?", []string{"Paladin:", "Can carry a shield."}},
		{"can priests use shields", []string{"Priest:", "Cannot use a shield."}},
		{"does my warrior get a shield?", []string{"Warrior:", "Can carry a shield."}},
		{"should my warrior carry a shield or a two-hander?", []string{"Warrior:", "Can carry a shield."}},
		{"which classes can use shields", []string{"Warrior:", "Paladin:", "Priest:"}},
		{"what goes in the ranged slot for a paladin?", []string{"Paladin:", "takes a libram", "no relic can be ranked"}},
		{"can a druid use an idol", []string{"Druid:", "takes an idol"}},
		{"can a warrior use a libram", []string{"Warrior:", "no relic slot"}},
		{"can my mage wear a ring?", []string{"Mage:", "any class may wear them"}},
		{"what trinkets can a priest use", []string{"Priest:", "any class may wear them"}},
		{"can a warrior wear a necklace", []string{"Warrior:", "any class may wear them"}},
		{"can my hunter use a spear?", []string{"Hunter:"}},
		{"can a rogue use throwing knives?", []string{"Rogue:"}},
	} {
		got := ProficiencyFacts(c, nil, tc.q)
		for _, w := range tc.want {
			if !strings.Contains(got, w) {
				t.Errorf("%q: facts lack %q:\n%s", tc.q, w, got)
			}
		}
	}
	own := ProficiencyFacts(character.Character{Class: "SHAMAN"}, nil, "can I use a shield?")
	if !strings.Contains(own, "Shaman:") || !strings.Contains(own, "Can carry a shield.") || strings.Contains(own, "Mage:") {
		t.Errorf("own class: %s", own)
	}
}

func TestProficiencyFactsSkipSpellsNamedForAShield(t *testing.T) {
	c := character.Character{Class: "MAGE"}
	for _, q := range []string{"when do I learn Power Word: Shield?", "can my warrior use shield slam?", "how does Divine Shield work?", "what does the idol of the sun look like?"} {
		if got := ProficiencyFacts(c, nil, q); got != "" {
			t.Errorf("%q: facts for a question that is none: %s", q, got)
		}
	}
}

func TestOffHandQuestionsGetTheDualWieldLine(t *testing.T) {
	for _, tc := range []struct{ class, q, want string }{
		{"ROGUE", "can my rogue use an off hand?", "learns it at level 10"},
		{"HUNTER", "when do I get an offhand?", "learns it at level 20"},
		{"PRIEST", "can my priest use an off-hand weapon", "cannot dual wield"},
	} {
		c := weaponPriest()
		c.Class = tc.class
		if !WeaponQuestion(tc.q) {
			t.Errorf("%q: not read as a weapon question", tc.q)
		}
		if got := WeaponFacts(c, weaponDS(t), tc.q); !strings.Contains(got, tc.want) {
			t.Errorf("%q: facts lack %q:\n%s", tc.q, tc.want, got)
		}
	}
	// Which off hand is best is a gear question, not a weapon-rule one.
	if WeaponQuestion("what is the best off hand for my rogue?") {
		t.Error("a gear question about the best off hand was read as a weapon-rule question")
	}
}

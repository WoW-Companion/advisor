package advice

import (
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/questie"
)

func trees(class, prof int) *character.Talents {
	return &character.Talents{Trees: []character.TalentTree{
		{Name: "Test Mage", Unspent: &class, Groups: []character.TalentGroup{{Name: "Test Fire", Spent: 3}}},
		{Name: "Test Tailoring", Key: "professions", Unspent: &prof},
	}, Unspent: &class}
}

func TestTalentMilestones(t *testing.T) {
	cases := []struct {
		name        string
		class, prof int
		want        []string
	}{
		{"class and profession", 2, 2, []string{"Spend 2 talent points", "You have 2 unspent profession talent points"}},
		{"profession only", 0, 1, []string{"You have 1 unspent profession talent point"}},
		{"nothing unspent shows none", 0, 0, nil},
	}
	for _, tc := range cases {
		c := character.Character{Name: "Testmage", Level: ip(10), Talents: trees(tc.class, tc.prof),
			Quests: []character.Quest{{QuestID: 1, Title: "Routine", Level: ip(10)}}}
		var got []string
		steps := WhatsNext(c, nil)
		for _, s := range steps {
			if s.Kind == KindTalent {
				got = append(got, s.Title)
			}
		}
		if strings.Join(got, "|") != strings.Join(tc.want, "|") {
			t.Errorf("%s: got %v want %v", tc.name, got, tc.want)
		}
		if len(tc.want) > 0 && steps[0].Kind != KindTalent {
			t.Errorf("%s: milestone is not first: %s", tc.name, titles(steps))
		}
	}
	s, _ := talentStep(character.Character{Talents: trees(2, 0)}, nil)
	if !strings.Contains(s.Reason, "What talents should I get?") {
		t.Errorf("no pointer to the talents question: %q", s.Reason)
	}
}

// classIndex adds a mage-only quest 4 (a nearer giver would be the normal quest) and a priest-only quest 5.
func classIndex() *questie.Index {
	ix := fakeIndex()
	far := []questie.Ref{{ID: 300}}
	ix.Npcs[300] = questie.Entity{Name: "Mage Tutor", Spawns: []questie.Spawn{{Area: 902, X: 90, Y: 90}}}
	ix.Quests[4] = &questie.Quest{ID: 4, Name: "Arcane Initiation", Level: 9, MinLevel: 8, Classes: 128, Starters: far, Finishers: far}
	ix.Quests[5] = &questie.Quest{ID: 5, Name: "Priest Vow", Level: 9, MinLevel: 8, Classes: 16, Starters: far, Finishers: far}
	return ix
}

func TestClassQuestLeads(t *testing.T) {
	c := questieChar()
	c.Talents = trees(1, 0)
	steps := WhatsNextWith(c, nil, classIndex())
	if len(steps) < 3 {
		t.Fatalf("steps: %s", titles(steps))
	}
	want := "Class quest: pick up Arcane Initiation from Mage Tutor, Kelp Cove (90.0, 90.0)"
	if steps[0].Kind != KindTalent { // unspent talent points come before the class quest
		t.Errorf("first step: %s", titles(steps))
	}
	if steps[1].Title != want || !strings.Contains(steps[1].Reason, ClassQuestNote) {
		t.Errorf("second step = %+v", steps[1])
	}
	pickUp := -1
	for i, s := range steps {
		if strings.HasPrefix(s.Title, PickUpPrefix) {
			pickUp = i
		}
		if strings.Contains(s.Title, "Priest Vow") {
			t.Errorf("another class's quest listed: %s", s.Title)
		}
	}
	if pickUp < 2 {
		t.Errorf("the nearer normal quest outranks the class quest: %s", titles(steps))
	}
}

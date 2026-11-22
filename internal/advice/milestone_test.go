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
		{"class and profession", 2, 2, []string{"You have 2 unspent talent points", "You have 2 unspent profession talent points"}},
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
	s, _ := talentStep(character.Character{Talents: trees(2, 0)})
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
	if steps[0].Title != want || !strings.Contains(steps[0].Reason, ClassQuestNote) {
		t.Errorf("first step = %+v", steps[0])
	}
	if steps[1].Kind != KindTalent {
		t.Errorf("second step: %s", titles(steps))
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

func planChar(unspent int, rank int) character.Character {
	max := 5
	t := &character.Talents{Unspent: &unspent, Trees: []character.TalentTree{{Name: "Test Warrior", Unspent: &unspent,
		Talents: []character.Talent{{Name: "Test Whirl", Group: "Test Fury", Rank: rank, MaxRank: &max}}}}}
	return character.Character{Name: "Testwar", Level: ip(10), Talents: t,
		Plan: &character.TalentPlan{Spec: "Fury", GuideURL: "https://www.icy-veins.com/wow-forever/fury", Date: "2026-11-22",
			Picks: []character.PlanPick{{Node: "Test Whirl", Rank: 5}, {Node: "Test Gone", Rank: 1}}}}
}

// With a saved plan the milestone names the picks and the guide; spending a point advances it.
func TestTalentStepFromPlan(t *testing.T) {
	s, ok := talentStep(planChar(2, 1))
	if !ok || s.Title != "You have 2 unspent talent points: take Test Whirl (ranks 2-3/5) in Test Fury" {
		t.Fatalf("title = %q", s.Title)
	}
	for _, want := range []string{"plan: https://www.icy-veins.com/wow-forever/fury Fury build", "not in the captured tree (Test Gone)"} {
		if !strings.Contains(s.Reason, want) {
			t.Errorf("reason lacks %q: %s", want, s.Reason)
		}
	}
	s, _ = talentStep(planChar(1, 3)) // two points spent, one left
	if s.Title != "You have 1 unspent talent point: take Test Whirl (rank 4/5) in Test Fury" {
		t.Errorf("after spending: %q", s.Title)
	}
	// The class-quest milestone still ranks first, the plan's pick right after it.
	steps := WhatsNext(planChar(1, 1), nil)
	if len(steps) == 0 || steps[0].Kind != KindTalent {
		t.Errorf("order: %s", titles(steps))
	}
	// A stale plan (spec changed) gives no pick and says to refresh.
	c := planChar(1, 1)
	c.Profile = &character.Profile{Role: "damage", Focus: "mixed", Spec: "Arms"}
	if s, _ := talentStep(c); strings.Contains(s.Title, "take") || !strings.Contains(s.Reason, "refresh the talent plan") {
		t.Errorf("stale: %q / %q", s.Title, s.Reason)
	}
}

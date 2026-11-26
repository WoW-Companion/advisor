package route

import (
	"slices"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/questie"
)

// homesWorld is sharedWorld plus three quests 8 levels above the hero: 601 done with a finisher on the
// plains, 602 done with no known finisher, 603 not done.
func homesWorld() *questie.Index {
	ix := sharedWorld()
	ix.Quests[601] = &questie.Quest{ID: 601, Name: "Quest 601", Level: 17, Finishers: []questie.Ref{{ID: 20}}}
	ix.Quests[602] = &questie.Quest{ID: 602, Name: "Quest 602", Level: 17, Objective: "Find an ogre who can read the Illegible Recipe"}
	ix.Quests[603] = &questie.Quest{ID: 603, Name: "Quest 603", Level: 17, Finishers: []questie.Ref{{ID: 20}}, Creatures: []questie.Ref{{ID: 21}}}
	return ix
}

func homesChar() character.Character {
	c := sharedChar()
	yes := true
	c.Quests = append(c.Quests,
		character.Quest{QuestID: 601, Title: "Quest 601", Complete: &yes},
		character.Quest{QuestID: 602, Title: "Quest 602", Complete: &yes},
		character.Quest{QuestID: 603, Title: "Quest 603", Objectives: []character.Objective{{Text: "Plains Boar slain: 0/5"}}})
	return c
}

func TestReadyTurnInFarAboveGoesToATripNotParked(t *testing.T) {
	p := Build(homesChar(), homesWorld())
	for _, k := range p.Parked {
		if k.QuestID == 601 || k.QuestID == 602 {
			t.Errorf("a ready turn-in is parked: %+v", k)
		}
	}
	for _, tr := range p.Trips {
		if tr.Map == mapPlains && (tr.Ready != 2 || !slices.Contains(tr.QuestIDs, 601)) {
			t.Errorf("plains trip %+v", tr)
		}
	}
}

func TestReadyTurnInWithNoFinisherGoesToFindWithItsHint(t *testing.T) {
	p := Build(homesChar(), homesWorld())
	for _, f := range p.Find {
		if f.QuestID == 602 {
			if f.Hint != "Find an ogre who can read the Illegible Recipe" || f.Label != f.Hint {
				t.Errorf("find %+v", f)
			}
			return
		}
	}
	t.Errorf("602 is not in find: %+v", p.Find)
}

func TestUnfinishedQuestFarAboveIsParkedWithWhenItIsDoable(t *testing.T) {
	p := Build(homesChar(), homesWorld())
	for _, k := range p.Parked {
		if k.QuestID == 603 {
			if k.Kind != ParkTooHigh || k.Why != "level 17 · doable around 14" {
				t.Errorf("parked %+v", k)
			}
			return
		}
	}
	t.Errorf("603 is not parked: %+v", p.Parked)
}

func TestEveryLoggedQuestIsPlacedExactlyOnce(t *testing.T) {
	c := homesChar()
	// A quest the data has never heard of still gets a home.
	c.Quests = append(c.Quests, character.Quest{QuestID: 999, Title: "Unknown Errand"})
	p := Build(c, homesWorld())
	count := map[int]int{}
	inSteps := map[int]bool{}
	for _, s := range p.Steps {
		for _, id := range append([]int{s.QuestID}, s.QuestIDs...) {
			inSteps[id] = true
		}
	}
	for id := range inSteps {
		if id != 0 {
			count[id]++
		}
	}
	for _, tr := range p.Trips {
		for _, id := range tr.QuestIDs {
			count[id]++
		}
	}
	for _, f := range p.Find {
		count[f.QuestID]++
	}
	for _, k := range p.Parked {
		count[k.QuestID]++
	}
	for _, q := range c.Quests {
		if count[q.QuestID] != 1 {
			t.Errorf("quest %d is placed %d times", q.QuestID, count[q.QuestID])
		}
	}
}

func TestCityRunGetsBankAndTrainErrands(t *testing.T) {
	ix := townIndex()
	ix.Areas[901] = questie.Area{UIMap: 7001, Zone: "Undercity"}
	c := townHero(0)
	c.Class, c.Faction = "ROGUE", "Horde"
	c.Location.Zone = "Undercity"
	opt := townOpt()
	opt.Trainable = func(character.Character) int { return 2 }
	p := BuildWith(c, ix, opt)
	kinds := map[string]Stop{}
	for _, s := range p.Steps {
		if s.Errand != "" {
			kinds[s.Errand] = s
		}
	}
	for _, k := range []string{ErrandTrain, ErrandBank} {
		s, ok := kinds[k]
		if !ok || s.Kind != KindErrand || !strings.Contains(s.Label, map[string]string{ErrandTrain: "Train now", ErrandBank: "bank"}[k]) {
			t.Errorf("no %s errand stop in %+v", k, p.Steps)
		}
	}
	if len(p.Errands) < 2 || p.Errands[0].N >= p.Errands[len(p.Errands)-1].N {
		t.Errorf("errands %+v", p.Errands)
	}
	if p.Total != len(p.Steps) {
		t.Errorf("total %d, steps %d", p.Total, len(p.Steps))
	}
	c.BankCaptured = true
	opt.Trainable = func(character.Character) int { return 0 }
	if q := BuildWith(c, ix, opt); len(q.Errands) != 0 {
		t.Errorf("errands with nothing to do: %+v", q.Errands)
	}
	// A plain zone gets none.
	ix.Areas[901] = questie.Area{UIMap: 7001, Zone: "Town"}
	c.Location.Zone, c.BankCaptured = "Town", false
	if q := BuildWith(c, ix, opt); len(q.Errands) != 0 {
		t.Errorf("errands outside a city: %+v", q.Errands)
	}
}

func TestOtherSidesCapitalGetsNoErrands(t *testing.T) {
	c := character.Character{Faction: "Horde"}
	for zone, want := range map[string]bool{"Orgrimmar": true, "Stormwind City": false, "Shattrath City": true, "Durotar": false} {
		if got := isHub(c, zone); got != want {
			t.Errorf("Horde in %s: %v, want %v", zone, got, want)
		}
	}
	if !isHub(character.Character{}, "Stormwind City") {
		t.Error("a character with no known side gets every capital")
	}
}

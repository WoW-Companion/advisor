package advice

import (
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/questie"
)

// An invented QuestieDB: quest 2 is open to a level 9 mage; 3 is for priests; 1 is in the log.
func fakeIndex() *questie.Index {
	st := []questie.Ref{{ID: 100}}
	q := func(id int, name string, classes uint64) *questie.Quest {
		return &questie.Quest{ID: id, Name: name, Level: 9, MinLevel: 8, Classes: classes, Starters: st, Finishers: st}
	}
	logged := q(1, "Logged Task", 0)
	logged.Creatures = []questie.Ref{{ID: 200}}
	return &questie.Index{DB: "v0", Quests: map[int]*questie.Quest{1: logged, 2: q(2, "Gather Kelp", 0), 3: q(3, "Priest Rite", 16)},
		Npcs: map[int]questie.Entity{
			100: {Name: "Elder Mira", Spawns: []questie.Spawn{{Area: 902, X: 50.5, Y: 60.5}}},
			200: {Name: "Reef Crab", Spawns: []questie.Spawn{{Area: 902, X: 70, Y: 30}}}},
		Areas: map[int]questie.Area{902: {UIMap: 7001, Sub: "Kelp Cove", Zone: "Testisle"}}}
}

func questieChar() character.Character {
	lvl, id, x, y := 9, 7001, 40.0, 60.0
	return character.Character{Level: &lvl, Class: "MAGE", Race: "Human", Quests: []character.Quest{{QuestID: 1, Title: "Logged Task"}},
		Location: &character.Location{Zone: "Testisle", MapID: &id, X: &x, Y: &y}}
}

func TestWhatsNextWithQuestie(t *testing.T) {
	steps := WhatsNextWith(questieChar(), nil, fakeIndex())
	var pick, logged *Step
	for i := range steps {
		switch {
		case strings.HasPrefix(steps[i].Title, PickUpPrefix):
			if pick != nil {
				t.Fatalf("second pick-up: %+v", steps[i])
			}
			pick = &steps[i]
		case steps[i].QuestID == 1:
			logged = &steps[i]
		}
	}
	if pick == nil || pick.Title != "Pick up: Gather Kelp (level 9) from Elder Mira, Kelp Cove (50.5, 60.5)" {
		t.Fatalf("pick-up: %+v", pick)
	}
	if logged == nil || !strings.Contains(logged.Reason, "objective near Reef Crab, Kelp Cove (70.0, 30.0)") {
		t.Fatalf("log quest: %+v", logged)
	}
}

func TestWhatsNextWithoutQuestieHasNoPickUps(t *testing.T) {
	for _, s := range WhatsNextWith(questieChar(), nil, nil) {
		if strings.HasPrefix(s.Title, PickUpPrefix) || s.Location != "" {
			t.Fatalf("unexpected %+v", s)
		}
	}
}

func TestClassQuestOnAnotherMapLeadsThePlan(t *testing.T) {
	ix := fakeIndex()
	ix.Areas[903] = questie.Area{UIMap: 7002, Zone: "Faraway"}
	ix.Npcs[101] = questie.Entity{Name: "Old Veteran", Spawns: []questie.Spawn{{Area: 903, X: 10, Y: 20}}}
	ix.Quests[9] = &questie.Quest{ID: 9, Name: "Mage Rite", Level: 9, MinLevel: 8, Classes: 128, Starters: []questie.Ref{{ID: 101}}}
	steps := WhatsNextWith(questieChar(), nil, ix)
	if len(steps) == 0 || !IsClassQuest(steps[0]) || steps[0].QuestID != 9 || steps[0].Zone != "Faraway" {
		t.Fatalf("first step = %+v", steps)
	}
}

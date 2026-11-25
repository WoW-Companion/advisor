package route

import (
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/advice"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/questie"
)

// bulwarkIsle is a synthetic isle: a class quest accepted in the log whose objective is an area the
// player's Questie database marks (and, for quest 41, no position at all), beside ordinary kill quests.
func bulwarkIsle(withArea bool) *questie.Index {
	at := func(x, y float64) []questie.Spawn { return []questie.Spawn{{Area: 901, X: x, Y: y}} }
	ix := &questie.Index{
		Quests: map[int]*questie.Quest{
			40: {ID: 40, Name: "Test Bulwark", Level: 10, Classes: 1 << 0, Starters: []questie.Ref{{ID: 1}}, Finishers: []questie.Ref{{ID: 1}}, Objective: "Reclaim the Test Bulwark at the Test Shrine"},
			41: {ID: 41, Name: "Quest 41", Level: 10, Starters: []questie.Ref{{ID: 1}}, Finishers: []questie.Ref{{ID: 1}}, Creatures: []questie.Ref{{ID: 2}}},
			42: {ID: 42, Name: "Quest 42", Level: 10, Starters: []questie.Ref{{ID: 1}}, Finishers: []questie.Ref{{ID: 1}}, Creatures: []questie.Ref{{ID: 3}}},
		},
		Npcs: map[int]questie.Entity{
			1: {Name: "Elder Test", Faction: "AH", Flags: flagQuestGiver, Spawns: at(50, 50)},
			2: {Name: "Test Boar", Spawns: at(51, 50)},
			3: {Name: "Test Wolf", Spawns: at(20, 50)},
		},
		Objects: map[int]questie.Entity{},
		Areas:   map[int]questie.Area{901: {UIMap: 7001, Zone: "Testisle"}},
	}
	if withArea {
		ix.Quests[40].Triggers = []questie.Trigger{{Text: "Test Bulwark", Spawns: at(90, 90)}}
	}
	return ix
}

func bulwarkChar() character.Character {
	c := hero()
	c.Class = "WARRIOR"
	no := false
	c.Quests = []character.Quest{
		{QuestID: 40, Title: "Test Bulwark", Complete: &no},
		{QuestID: 41, Title: "Quest 41", Complete: &no, Objectives: []character.Objective{{Text: "Test Boar: 0/3", Done: &no}}},
		{QuestID: 42, Title: "Quest 42", Complete: &no, Objectives: []character.Objective{{Text: "Test Wolf: 0/3", Done: &no}}},
	}
	x, y := 50.0, 50.0
	c.Location.X, c.Location.Y = &x, &y
	return c
}

func indexOfLabel(p Plan, kind string, quest int, sub string) int {
	for i, s := range p.Steps {
		if s.Kind == kind && s.QuestID == quest && strings.Contains(s.Label, sub) {
			return i
		}
	}
	return -1
}

func TestAcceptedClassQuestObjectiveComesEarly(t *testing.T) {
	ix := bulwarkIsle(true)
	ix.Quests[40].Classes = 1 << 0 // warrior
	p := Build(bulwarkChar(), ix)
	obj := indexOfLabel(p, KindObjective, 40, "Test Bulwark")
	wolf := indexOfLabel(p, KindObjective, 42, "Test Wolf")
	turn := stepIndex(p, KindTurnIn, 40)
	if obj < 0 || wolf < 0 || turn < 0 {
		t.Fatalf("steps missing: %+v", p.Steps)
	}
	if obj > wolf || obj > turn {
		t.Errorf("the class quest's objective (%d) must precede the far wolf (%d) and its turn-in (%d): %+v", obj, wolf, turn, p.Steps)
	}
	if !p.Steps[obj].Priority {
		t.Errorf("class quest step not marked priority")
	}
}

func TestMissingPositionComesFromTheQuestieAreaObjective(t *testing.T) {
	p := Build(bulwarkChar(), bulwarkIsle(true))
	i := indexOfLabel(p, KindObjective, 40, "Test Bulwark")
	if i < 0 {
		t.Fatalf("no step: %+v", p.Steps)
	}
	s := p.Steps[i]
	if s.X != 90 || s.Y != 90 || s.ZoneOnly || s.Source != FromQuestie || !strings.Contains(s.Label, FromQuestie) {
		t.Errorf("step %+v", s)
	}
}

func TestNoPositionAnywhereGivesTheZoneOnly(t *testing.T) {
	p := Build(bulwarkChar(), bulwarkIsle(false))
	i := indexOfLabel(p, KindObjective, 40, "Reclaim")
	if i < 0 {
		t.Fatalf("the accepted quest has no step: %+v", p.Steps)
	}
	s := p.Steps[i]
	if !s.ZoneOnly || s.X != 0 || s.Y != 0 || s.Place != "Testisle" || !strings.Contains(s.Label, "somewhere in Testisle") {
		t.Errorf("step %+v", s)
	}
	if strings.Contains(Render(p, 20), "(0.0, 0.0)") {
		t.Errorf("a position was shown for a zone-only step:\n%s", Render(p, 20))
	}
}

func TestWhatNextMatchesStepOne(t *testing.T) {
	p := Build(bulwarkChar(), bulwarkIsle(true))
	first := p.Steps[0].QuestID
	var list []advice.Step
	for _, id := range []int{40, 41, 42} {
		list = append(list, advice.Step{QuestID: id})
	}
	got := advice.ByQuestOrder(list, func(s advice.Step) int { return s.QuestID }, QuestOrder(p))
	if got[0].QuestID != first {
		t.Errorf("what next leads with quest %d, step 1 is quest %d (order %v)", got[0].QuestID, first, QuestOrder(p))
	}
}

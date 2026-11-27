package route

import (
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/questie"
)

// exploreIndex is a zone with a quest whose only objective is exploration (triggerEnd) and a second
// quest whose turn-in stands near that place; pts are the triggerEnd points.
func exploreIndex(pts ...questie.Spawn) *questie.Index {
	at := func(a int, x, y float64) []questie.Spawn { return []questie.Spawn{{Area: a, X: x, Y: y}} }
	q := &questie.Quest{ID: 98, Name: "Test Camp", Level: 10, Zone: 901, Starters: []questie.Ref{{ID: 1}}, Finishers: []questie.Ref{{ID: 1}},
		Objective: "Find the camp."}
	if len(pts) > 0 {
		q.Triggers = []questie.Trigger{{Text: "Camp located", Spawns: pts, End: true}}
	}
	near := &questie.Quest{ID: 99, Name: "Test Mastery", Level: 10, Zone: 901, Starters: []questie.Ref{{ID: 1}}, Finishers: []questie.Ref{{ID: 2}}}
	return &questie.Index{
		Quests: map[int]*questie.Quest{98: q, 99: near},
		Npcs: map[int]questie.Entity{
			1: {Name: "Elder Test", Faction: "AH", Flags: flagQuestGiver, Spawns: at(901, 10, 10)},
			2: {Name: "Camp Hand", Faction: "AH", Flags: flagQuestGiver, Spawns: at(901, 80, 80)},
		},
		Objects: map[int]questie.Entity{},
		Areas:   map[int]questie.Area{901: {UIMap: 7001, Zone: "Testisle"}},
	}
}

func exploreChar() character.Character {
	c := hero()
	no, yes := false, true
	c.Quests = []character.Quest{
		{QuestID: 98, Title: "Test Camp", Complete: &no, Objectives: []character.Objective{{Text: "Locate the camp", Done: &no}}},
		{QuestID: 99, Title: "Test Mastery", Complete: &yes},
	}
	x, y := 10.0, 10.0
	c.Location.X, c.Location.Y = &x, &y
	return c
}

func exploreStep(p Plan) (Stop, bool) {
	for _, s := range p.Steps {
		if s.Kind == KindObjective && s.QuestID == 98 {
			return s, true
		}
	}
	return Stop{}, false
}

func TestExplorationObjectiveIsPlacedAtTriggerEnd(t *testing.T) {
	p := Build(exploreChar(), exploreIndex(questie.Spawn{Area: 901, X: 78, Y: 82}))
	s, ok := exploreStep(p)
	if !ok || s.Unplaced || s.ZoneOnly || s.Approx || s.X != 78 || s.Y != 82 || !strings.Contains(s.Label, "Locate the camp") {
		t.Fatalf("step %+v", s)
	}
}

func TestExplorationObjectiveWithSeveralPointsUsesTheCentroid(t *testing.T) {
	p := Build(exploreChar(), exploreIndex(questie.Spawn{Area: 901, X: 70, Y: 80}, questie.Spawn{Area: 901, X: 90, Y: 84}))
	s, ok := exploreStep(p)
	if !ok || s.Unplaced || s.X != 80 || s.Y != 82 || !s.Approx {
		t.Fatalf("step %+v", s)
	}
}

func TestExplorationObjectiveWithoutTriggerEndStaysUnplaced(t *testing.T) {
	p := Build(exploreChar(), exploreIndex())
	if s, ok := exploreStep(p); ok && !s.Unplaced && !s.ZoneOnly {
		t.Fatalf("placed without data: %+v", s)
	}
}

func TestExplorationStepSitsNextToTheNearbyTurnIn(t *testing.T) {
	c := exploreChar()
	p := Build(c, exploreIndex(questie.Spawn{Area: 901, X: 78, Y: 82}))
	idx, turn := -1, -1
	for i, s := range p.Steps {
		if s.Kind == KindObjective && s.QuestID == 98 {
			idx = i
		}
		if s.Kind == KindTurnIn && s.QuestID == 99 {
			turn = i
		}
	}
	if idx < 0 || turn < 0 || idx-turn > 1 || turn-idx > 1 {
		t.Fatalf("not adjacent (objective %d, turn-in %d): %+v", idx, turn, p.Steps)
	}
	cov := CoverageOf(c, exploreIndex(questie.Spawn{Area: 901, X: 78, Y: 82}), Options{})
	if cov.TriggerEnd != 1 || cov.Unplaced != 0 || !strings.Contains(cov.Line(), "1 placed from triggerEnd") {
		t.Errorf("coverage %+v", cov)
	}
}

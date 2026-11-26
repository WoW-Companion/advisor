package route

import (
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/questie"
)

// readyWorld has a home map (7001), a flight-reachable town (7003), a dungeon-only map (7004) and a
// pick-up near the player; quests 201-202 finish on the home map, 211-212 in the town.
func readyWorld() *questie.Index {
	at := func(area int, x, y float64) []questie.Spawn { return []questie.Spawn{{Area: area, X: x, Y: y}} }
	ix := &questie.Index{
		Quests: map[int]*questie.Quest{},
		Npcs: map[int]questie.Entity{
			1:   {Name: "Near Giver", Spawns: at(901, 50, 50)},
			101: {Name: "Far Clerk", Spawns: at(901, 80, 80)},
			102: {Name: "Close Clerk", Spawns: at(901, 60, 40)},
			111: {Name: "Town Clerk", Spawns: at(903, 30, 30)},
			112: {Name: "Town Mage", Spawns: at(903, 31, 31)},
			113: {Name: "Town Giver", Spawns: at(903, 70, 70)},
			121: {Name: "Keep Ghoul", Spawns: at(904, 40, 40)},
		},
		Objects: map[int]questie.Entity{},
		Areas: map[int]questie.Area{
			901: {UIMap: 7001, Zone: "Testisle"},
			903: {UIMap: 7003, Zone: "Townville"},
			904: {UIMap: 7004, Sub: "Shadowfang Keep", Zone: "Silverpine Forest"},
		},
	}
	add := func(id, lvl int, start, fin int, creatures ...int) {
		q := &questie.Quest{ID: id, Name: "Quest " + itoa(id), Level: lvl}
		if start != 0 {
			q.Starters = []questie.Ref{{ID: start}}
		}
		q.Finishers = []questie.Ref{{ID: fin}}
		for _, n := range creatures {
			q.Creatures = append(q.Creatures, questie.Ref{ID: n})
		}
		ix.Quests[id] = q
	}
	add(201, 9, 0, 101)
	add(202, 9, 0, 102)
	add(203, 9, 1, 1)
	add(211, 9, 0, 111)
	add(212, 9, 0, 112)
	add(213, 9, 113, 113)
	add(221, 9, 0, 1, 121) // an objective inside the keep
	return ix
}

func readyChar(m int, x, y float64, ready ...int) character.Character {
	c := hero()
	c.Location.MapID, c.Location.X, c.Location.Y = &m, &x, &y
	yes := true
	for _, id := range ready {
		c.Quests = append(c.Quests, character.Quest{QuestID: id, Title: "Quest " + itoa(id), Complete: &yes})
	}
	return c
}

func stepKinds(p Plan) []string {
	var out []string
	for _, s := range p.Steps {
		out = append(out, s.Kind+":"+itoa(s.QuestID))
	}
	return out
}

func TestReadyTurnInsOnThisMapComeFirstNearestFirst(t *testing.T) {
	p := Build(readyChar(7001, 48, 49, 201, 202), readyWorld())
	if len(p.Steps) < 3 || p.Steps[0].Kind != KindTurnIn || p.Steps[0].QuestID != 202 || p.Steps[1].Kind != KindTurnIn || p.Steps[1].QuestID != 201 {
		t.Fatalf("want turn-ins 202 then 201 first, got %v", stepKinds(p))
	}
	if i := stepIndex(p, KindPickup, 203); i < 2 {
		t.Errorf("pick-up must follow the turn-ins: %v", stepKinds(p))
	}
}

func TestReadyTurnInsInAReachableTownAreOneFlightLeg(t *testing.T) {
	c := readyChar(7001, 48, 49, 211, 212)
	c.Seen = &character.Observed{Flights: []character.SeenFlight{{NPC: 9, Name: "Town Wing", Map: 7003, T: 1}}}
	p := Build(c, readyWorld())
	var legs []Stop
	for _, s := range p.Steps {
		if s.Kind == KindTravel {
			legs = append(legs, s)
		}
	}
	if len(legs) != 1 || legs[0].Label != "Fly to Townville: turn in Quest 211, Quest 212" || len(legs[0].QuestIDs) != 2 || legs[0].Map != 7003 {
		t.Fatalf("legs %+v", legs)
	}
	// Flight data that does not reach the town: no flight leg. No flight data at all: the town with turn-ins.
	c.Seen = &character.Observed{Flights: []character.SeenFlight{{NPC: 9, Name: "Elsewhere", Map: 7004, T: 1}}}
	for _, s := range Build(c, readyWorld()).Steps {
		if strings.HasPrefix(s.Label, "Fly to") {
			t.Errorf("unreachable town got a leg: %+v", s)
		}
	}
	c.Seen = nil
	if p := Build(c, readyWorld()); stepIndex(p, KindTravel, 211) < 0 {
		t.Errorf("no flight data: want a leg to the town with the most turn-ins: %v", stepKinds(p))
	}
}

func TestArrivalInATownStartsWithItsReadyTurnIns(t *testing.T) {
	p := Build(readyChar(7003, 69, 69, 211, 212), readyWorld())
	if len(p.Steps) < 3 || p.Steps[0].QuestID != 212 || p.Steps[1].QuestID != 211 || p.Steps[0].Kind != KindTurnIn {
		t.Fatalf("want turn-ins first, nearest first: %v", stepKinds(p))
	}
	if i := stepIndex(p, KindPickup, 213); i < 2 {
		t.Errorf("a pick-up next to the player must follow the turn-ins: %v", stepKinds(p))
	}
}

func TestTravelLabelNamesThePlace(t *testing.T) {
	c := readyChar(7001, 48, 49)
	c.Quests = []character.Quest{{QuestID: 221, Title: "Quest 221", Objectives: []character.Objective{{Text: "Keep Ghoul slain: 0/1"}}}}
	var label string
	for _, s := range Build(c, readyWorld()).Steps {
		if s.Kind == KindTravel {
			label = s.Label
		}
	}
	if label != "Travel to Shadowfang Keep (Silverpine Forest)" {
		t.Errorf("label %q", label)
	}
}

func TestUnknownQuestLogOffersNoPickups(t *testing.T) {
	c := readyChar(7001, 48, 49)
	c.QuestsCaptured = false
	p := Build(c, readyWorld())
	for _, s := range p.Steps {
		if s.Kind == KindPickup {
			t.Errorf("pick-up offered with no quest log: %+v", s)
		}
	}
}

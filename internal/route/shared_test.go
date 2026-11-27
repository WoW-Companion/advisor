package route

import (
	"slices"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/lua"
	"github.com/WoW-Companion/advisor/internal/questie"
)

const (
	mapHere   = 7001
	mapSouth  = 7010
	mapPlains = 7011
	mapPeaks  = 7012
)

// sharedWorld: the player's zone (7001) with quests 101 (kill), 102 (kill, handed in at another NPC) and
// 103 (kill here, handed in in the south); ready turn-ins: 201 and 202 in the south (7010), 301 on the
// plains (7011), 401 in the peaks (7012); 501 is five levels above the player, on the plains.
func sharedWorld() *questie.Index {
	at := func(area int, x, y float64) []questie.Spawn { return []questie.Spawn{{Area: area, X: x, Y: y}} }
	ix := &questie.Index{
		Quests: map[int]*questie.Quest{},
		Npcs: map[int]questie.Entity{
			1:  {Name: "Local Captain", Spawns: at(901, 50, 50)},
			2:  {Name: "Local Peasant", Spawns: at(901, 30, 30), Faction: "A"},
			3:  {Name: "Local Smith", Spawns: at(901, 60, 60)},
			10: {Name: "South Clerk", Spawns: at(910, 40, 40)},
			11: {Name: "South Mage", Spawns: at(910, 41, 41)},
			20: {Name: "Plains Chief", Spawns: at(911, 50, 50)},
			21: {Name: "Plains Boar", Spawns: at(911, 55, 55)},
			30: {Name: "Peaks Hermit", Spawns: at(912, 50, 50)},
		},
		Objects: map[int]questie.Entity{},
		Areas: map[int]questie.Area{
			901: {UIMap: mapHere, Zone: "Hillside"},
			910: {UIMap: mapSouth, Zone: "Southvale"},
			911: {UIMap: mapPlains, Zone: "Plains"},
			912: {UIMap: mapPeaks, Zone: "Peaks"},
		},
	}
	add := func(id, lvl, fin int, creatures ...int) {
		q := &questie.Quest{ID: id, Name: "Quest " + itoa(id), Level: lvl, Finishers: []questie.Ref{{ID: fin}}}
		for _, n := range creatures {
			q.Creatures = append(q.Creatures, questie.Ref{ID: n})
		}
		ix.Quests[id] = q
	}
	add(101, 9, 1, 2)
	add(102, 9, 3, 2)
	add(103, 9, 10, 2)
	add(201, 9, 10)
	add(202, 9, 11)
	add(301, 9, 20)
	add(401, 9, 30)
	add(501, 14, 20, 21)
	return ix
}

func sharedChar() character.Character {
	c := hero()
	c.Faction = "Alliance"
	yes := true
	for _, id := range []int{101, 102, 103} {
		c.Quests = append(c.Quests, character.Quest{QuestID: id, Title: "Quest " + itoa(id), Objectives: []character.Objective{{Text: "Local Peasant slain: 0/5"}}})
	}
	for _, id := range []int{201, 202, 301, 401} {
		c.Quests = append(c.Quests, character.Quest{QuestID: id, Title: "Quest " + itoa(id), Complete: &yes})
	}
	c.Quests = append(c.Quests, character.Quest{QuestID: 501, Title: "Quest 501", Objectives: []character.Objective{{Text: "Plains Boar slain: 0/5"}}})
	return c
}

func tripIDs(p Plan) [][]int {
	var out [][]int
	for _, t := range p.Trips {
		out = append(out, t.QuestIDs)
	}
	return out
}

func TestSharedPlanKeepsTheRunHereAndGroupsTheRestByZone(t *testing.T) {
	c := sharedChar()
	c.Location.MapID = new(int)
	*c.Location.MapID = mapHere
	p := BuildWith(c, sharedWorld(), Options{XP: map[int]int{301: 100, 401: 200}})
	for _, s := range p.Steps {
		if s.Map != mapHere || s.Kind == KindTravel {
			t.Errorf("a step outside the run here: %+v", s)
		}
	}
	for i, s := range p.Steps {
		if s.N != i+1 {
			t.Errorf("steps are numbered 1..N: %+v", p.Steps)
		}
	}
	if p.Exit == nil || p.Exit.Map != mapSouth || p.Exit.Label == "" || p.Exit.Via == "" {
		t.Errorf("exit %+v", p.Exit)
	}
	want := [][]int{{201, 202}, {401}, {301}}
	if got := tripIDs(p); len(got) != 3 || !slices.EqualFunc(got, want, slices.Equal[[]int]) {
		t.Fatalf("trips %+v", p.Trips)
	}
	if t0 := p.Trips[0]; t0.Order != 1 || t0.Map != mapSouth || t0.Ready != 2 || t0.Todo != 0 || t0.Reason != "2 ready to turn in" || t0.Travel == "" {
		t.Errorf("first trip %+v", t0)
	}
	if len(p.Parked) != 1 || p.Parked[0].QuestID != 501 || p.Parked[0].Kind != ParkTooHigh || p.Parked[0].Why != "level 14 · doable around 11" {
		t.Errorf("parked %+v", p.Parked)
	}
}

func TestEveryQuestAppearsOnceAcrossStepsTripsAndParked(t *testing.T) {
	c := sharedChar()
	// A second copy of a logged quest, as an unplanned one, must not be listed twice.
	c.Quests = append(c.Quests, c.Quests[3])
	p := Build(c, sharedWorld())
	where := map[int]string{}
	note := func(id int, section string) {
		if prev, dup := where[id]; dup && (prev != section || section != "steps") {
			t.Errorf("quest %d is in %s and in %s", id, prev, section)
		}
		where[id] = section
	}
	for _, s := range p.Steps {
		for _, id := range append([]int{s.QuestID}, s.QuestIDs...) {
			if id != 0 {
				note(id, "steps")
			}
		}
	}
	for _, tr := range p.Trips {
		for _, id := range tr.QuestIDs {
			note(id, "trips")
		}
	}
	for _, k := range p.Parked {
		note(k.QuestID, "parked")
	}
	// Quest 103 has a stop here and its turn-in in the south: it is a step, not a trip.
	if where[103] != "steps" {
		t.Errorf("quest 103 is in %q", where[103])
	}
	for id := range sharedWorld().Quests {
		if where[id] == "" {
			t.Errorf("quest %d is nowhere", id)
		}
	}
}

func TestNotReadyLocalTurnInIsAStepNotAHereLine(t *testing.T) {
	p := Build(sharedChar(), sharedWorld())
	step := false
	for _, s := range p.Steps {
		step = step || (s.Kind == KindTurnIn && slices.Contains(s.QuestIDs, 102))
	}
	if !step {
		t.Errorf("quest 102's turn-in is a step: %+v", p.Steps)
	}
	for _, h := range p.Here {
		for _, q := range h.Quests {
			if q.QuestID == 102 {
				t.Errorf("a not-ready turn-in is in a here block: %+v", h)
			}
		}
	}
}

func TestTripsAreOrderedByReadyThenXPThenTravel(t *testing.T) {
	c := sharedChar()
	c.Quests = c.Quests[:0]
	yes := true
	for _, id := range []int{201, 301, 401} {
		c.Quests = append(c.Quests, character.Quest{QuestID: id, Title: "Quest " + itoa(id), Complete: &yes})
	}
	zone := func(p Plan) []int {
		var out []int
		for _, t := range p.Trips {
			out = append(out, t.Map)
		}
		return out
	}
	// Equal ready counts and XP: a known flight beats a walk.
	c.Seen = &character.Observed{Flights: []character.SeenFlight{{NPC: 9, Name: "Peaks Wing", Map: mapPeaks, T: 1}}}
	if got := zone(Build(c, sharedWorld())); !slices.Equal(got, []int{mapPeaks, mapSouth, mapPlains}) {
		t.Errorf("by travel: %v", got)
	}
	// XP outranks travel.
	c.Seen = nil
	got := zone(BuildWith(c, sharedWorld(), Options{XP: map[int]int{301: 500, 401: 100}}))
	if !slices.Equal(got, []int{mapPlains, mapPeaks, mapSouth}) {
		t.Errorf("by xp: %v", got)
	}
	// Ready turn-ins outrank XP: a quest with work left comes after a ready one.
	c.Quests = []character.Quest{c.Quests[0], {QuestID: 501, Title: "Quest 501", Objectives: []character.Objective{{Text: "Plains Boar slain: 0/5"}}}}
	lvl := 14
	c.Level = &lvl
	if got := zone(BuildWith(c, sharedWorld(), Options{XP: map[int]int{501: 500}})); !slices.Equal(got, []int{mapSouth, mapPlains}) {
		t.Errorf("by ready: %v", got)
	}
}

func TestPlanLuaCarriesExitTripsAndParked(t *testing.T) {
	p := Build(sharedChar(), sharedWorld())
	vars, err := lua.Parse(Envelope{Generated: 1, Plans: []Plan{p}}.Lua())
	if err != nil {
		t.Fatal(err)
	}
	hero := vars["WoWCompanionPlan"].(lua.Table)["characters"].(lua.Table)["Hero"].(lua.Table)
	if len(lua.List(hero["trips"])) != len(p.Trips) || len(lua.List(hero["parked"])) != 1 {
		t.Errorf("trips %v, parked %v", hero["trips"], hero["parked"])
	}
	if e, ok := hero["exit"].(lua.Table); p.Exit != nil && (!ok || e["via"] == nil) {
		t.Errorf("exit %v", hero["exit"])
	}
	first := lua.List(hero["trips"])[0].(lua.Table)
	if first["order"] != int64(1) || first["questIDs"] == nil || first["travel"] == nil {
		t.Errorf("trip %v", first)
	}
}

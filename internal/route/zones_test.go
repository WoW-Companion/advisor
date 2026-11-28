package route

import (
	"slices"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/lua"
	"github.com/WoW-Companion/advisor/internal/questie"
)

const (
	mapUndercity  = 1458
	mapBarrens    = 1413
	mapStonetalon = 1442
)

// stoneWorld has five in-log quests in Stonetalon Mountains (objective, then turn-in, each) and one
// Barrens quest, with Tharm at Sun Rock Retreat.
func stoneWorld() *questie.Index {
	at := func(area int, x, y float64) []questie.Spawn { return []questie.Spawn{{Area: area, X: x, Y: y}} }
	ix := &questie.Index{
		Quests: map[int]*questie.Quest{},
		Npcs: map[int]questie.Entity{
			50:  {Name: "Tharm", Spawns: at(803, 45, 59)},
			310: {Name: "Barrens Chief", Spawns: at(804, 50, 50)},
			311: {Name: "Barrens Boar", Spawns: at(804, 52, 52)},
		},
		Objects: map[int]questie.Entity{},
		Areas: map[int]questie.Area{
			801: {UIMap: mapUndercity, Zone: "Undercity"},
			802: {UIMap: mapStonetalon, Zone: "Stonetalon Mountains"},
			803: {UIMap: mapStonetalon, Sub: "Sun Rock Retreat", Zone: "Stonetalon Mountains"},
			804: {UIMap: mapBarrens, Zone: "The Barrens"},
		},
	}
	// Quest 30n: a mob at (xs[n], 40) and a turn-in NPC at (xs[n], 80): the mobs run west to east.
	for i, x := range []float64{40, 50, 20, 70, 30} {
		id := 301 + i
		mob, npc := 320+i, 330+i
		ix.Npcs[mob] = questie.Entity{Name: "Mob " + itoa(i), Spawns: at(802, x, 40)}
		ix.Npcs[npc] = questie.Entity{Name: "Giver " + itoa(i), Spawns: at(802, x, 80)}
		ix.Quests[id] = &questie.Quest{ID: id, Name: "Quest " + itoa(id), Level: 9,
			Finishers: []questie.Ref{{ID: npc}}, Creatures: []questie.Ref{{ID: mob}}}
	}
	ix.Quests[310] = &questie.Quest{ID: 310, Name: "Quest 310", Level: 9,
		Finishers: []questie.Ref{{ID: 310}}, Creatures: []questie.Ref{{ID: 311}}}
	return ix
}

func stoneChar(m int, flight bool) character.Character {
	c := hero()
	c.Faction = "Horde"
	x, y := 50.0, 50.0
	c.Location = &character.Location{MapID: &m, X: &x, Y: &y}
	for _, id := range []int{301, 302, 303, 304, 305, 310} {
		text := "Mob " + itoa(id-301) + " slain: 0/1"
		if id == 310 {
			text = "Barrens Boar slain: 0/1"
		}
		c.Quests = append(c.Quests, character.Quest{QuestID: id, Title: "Quest " + itoa(id), Objectives: []character.Objective{{Text: text}}})
	}
	if flight {
		c.Seen = &character.Observed{Flights: []character.SeenFlight{{NPC: 50, Name: "Tharm", Map: mapStonetalon, X: 45, Y: 59, Faction: "Horde", T: 1}}}
	}
	return c
}

func zoneStops(t *testing.T, p Plan, m int) (z ZonePlan, stops []Stop) {
	t.Helper()
	for _, zz := range p.Zones {
		if zz.Map == m {
			z = zz
		}
	}
	if z.Map == 0 {
		t.Fatalf("no zone %d in %+v", m, p.Zones)
	}
	for _, n := range z.Order {
		stops = append(stops, p.Later[n-1])
	}
	return z, stops
}

func TestZoneIsOneChainFromTheKnownFlightMaster(t *testing.T) {
	p := Build(stoneChar(mapBarrens, true), stoneWorld())
	z, stops := zoneStops(t, p, mapStonetalon)
	if z.Arrival.Kind != ArriveFlight || z.Arrival.NPC != 50 || z.Arrival.Name != "Tharm" || z.Arrival.Place != "Sun Rock Retreat" || z.Arrival.X != 45 || z.Arrival.Y != 59 {
		t.Fatalf("arrival %+v", z.Arrival)
	}
	if z.Travel != "Fly to Sun Rock Retreat (Tharm), Stonetalon Mountains" {
		t.Errorf("travel %q", z.Travel)
	}
	var inZone int
	for _, s := range p.Later {
		if s.Map == mapStonetalon {
			inZone++
		}
	}
	if len(stops) != 10 || inZone != 10 || len(z.Order) != 10 {
		t.Fatalf("want every stop once: order %v, %d in zone", z.Order, inZone)
	}
	seen := map[int]bool{}
	for _, n := range z.Order {
		if seen[n] {
			t.Errorf("stop %d twice in %v", n, z.Order)
		}
		seen[n] = true
	}
	// Each quest's objective comes before its turn-in,.
	at := map[int]int{}
	for i, s := range stops {
		at[s.QuestID*10+map[string]int{KindObjective: 1, KindTurnIn: 2}[s.Kind]] = i
	}
	for id := 301; id <= 305; id++ {
		if at[id*10+1] > at[id*10+2] {
			t.Errorf("quest %d: turn-in before objective: %v", id, z.Order)
		}
	}
	if stops[0].Kind != KindObjective {
		t.Errorf("chain starts with an objective, never a turn-in: %+v", stops[0])
	}
	e := p.Exit
	if e == nil || e.Map != mapStonetalon || e.X != 45 || e.Y != 59 || e.Label != z.Travel || e.Via != "flight" {
		t.Errorf("exit %+v", e)
	}
	for _, s := range p.Steps {
		if s.Kind == KindTravel {
			t.Errorf("a journey is never a step: %+v", s)
		}
	}
}

func TestZoneWithoutKnownFlightIsWalkedFromTheBarrens(t *testing.T) {
	p := Build(stoneChar(mapBarrens, false), stoneWorld())
	z, stops := zoneStops(t, p, mapStonetalon)
	if z.Arrival.Kind != ArriveWalk || z.Arrival.Name != "The Barrens" || !z.Arrival.Known || len(stops) != 10 {
		t.Fatalf("arrival %+v, %d stops", z.Arrival, len(stops))
	}
	if z.Travel != "Walk or ride to Stonetalon Mountains from The Barrens" || strings.Contains(z.Travel, "Fly") {
		t.Errorf("travel %q", z.Travel)
	}
	// A flight point on another map is not a flight into this one.
	c := stoneChar(mapBarrens, false)
	c.Seen = &character.Observed{Flights: []character.SeenFlight{{NPC: 7, Name: "Elsewhere", Map: mapBarrens, T: 1}}}
	if z, _ := zoneStops(t, Build(c, stoneWorld()), mapStonetalon); z.Arrival.Kind != ArriveWalk {
		t.Errorf("arrival %+v", z.Arrival)
	}
}

func TestCrossContinentTravelNamesTheZeppelin(t *testing.T) {
	z, _ := zoneStops(t, Build(stoneChar(mapUndercity, true), stoneWorld()), mapStonetalon)
	if want := "Zeppelin to Grom'gol, then boat to Ratchet, then fly to Sun Rock Retreat (Tharm)"; z.Travel != want {
		t.Errorf("travel %q, want %q", z.Travel, want)
	}
	z, _ = zoneStops(t, Build(stoneChar(mapUndercity, false), stoneWorld()), mapStonetalon)
	if want := "Zeppelin to Grom'gol, then boat to Ratchet, then walk or ride to Stonetalon Mountains from The Barrens"; z.Travel != want {
		t.Errorf("travel %q, want %q", z.Travel, want)
	}
}

func TestCurrentZoneHasNoZoneRoute(t *testing.T) {
	p := Build(stoneChar(mapBarrens, true), stoneWorld())
	for _, z := range p.Zones {
		if z.Map == mapBarrens {
			t.Errorf("the player's own zone got a route: %+v", z)
		}
	}
	var onRoute bool
	for _, s := range p.Steps {
		onRoute = onRoute || (s.Map == mapBarrens && s.QuestID == 310)
	}
	if !onRoute || len(p.Zones) != 1 || slices.Contains(p.Zones[0].Order, 0) {
		t.Errorf("steps %v zones %+v", stepKinds(p), p.Zones)
	}
}

func TestPlanLuaCarriesZones(t *testing.T) {
	p := Build(stoneChar(mapBarrens, true), stoneWorld())
	vars, err := lua.Parse(Envelope{Generated: 1, Plans: []Plan{p}}.Lua())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(Envelope{Plans: []Plan{p}}.Lua(), `[1442] = { zone = "Stonetalon Mountains", travel = "Fly to Sun Rock Retreat (Tharm), Stonetalon Mountains", arrival = { kind = "flight", npc = 50, name = "Tharm", place = "Sun Rock Retreat", map = 1442, x = 45.0, y = 59.0 }, order = {`) || vars == nil {
		t.Errorf("zones missing from Plan.lua")
	}
}

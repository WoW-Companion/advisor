package route

import (
	"os"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/learn"
	"github.com/WoW-Companion/advisor/internal/questie"
)

// routeZones are the zones of the synthetic world, each with a finisher whose quest is ready to hand in.
var routeZones = []string{"Stranglethorn Vale", "The Barrens", "Silverpine Forest", "Arathi Highlands",
	"Thousand Needles", "Stonetalon Mountains", "Hillsbrad Foothills", "Desolace"}

func routeWorld(zones ...string) (*questie.Index, character.Character) {
	ix := &questie.Index{
		Quests:  map[int]*questie.Quest{},
		Npcs:    map[int]questie.Entity{},
		Objects: map[int]questie.Entity{},
		Areas:   map[int]questie.Area{900: {UIMap: mapUndercity, Zone: "Undercity"}},
	}
	lvl, mp := 32, mapUndercity
	x, y := 50.0, 50.0
	yes := true
	c := character.Character{Name: "Hero", Faction: "Horde", Class: "ROGUE", QuestsCaptured: true, Level: &lvl,
		Location: &character.Location{Zone: "Undercity", MapID: &mp, X: &x, Y: &y}}
	for i, z := range zones {
		id := 100 + i
		ix.Areas[910+i] = questie.Area{UIMap: 7100 + i, Zone: z}
		ix.Npcs[id] = questie.Entity{Name: "Clerk " + z, Spawns: []questie.Spawn{{Area: 910 + i, X: 50, Y: 50}}}
		ix.Quests[id] = &questie.Quest{ID: id, Name: "Quest " + itoa(id), Level: 30, Finishers: []questie.Ref{{ID: id}}}
		c.Quests = append(c.Quests, character.Quest{QuestID: id, Title: "Quest " + itoa(id), Complete: &yes})
	}
	return ix, c
}

func tripContinents(p Plan) []string {
	var out []string
	for _, t := range p.Trips {
		out = append(out, learn.ContinentOf(t.Zone))
	}
	return out
}

func crossings(start string, cs []string) int {
	n, prev := 0, start
	for _, c := range cs {
		if c != prev {
			n++
		}
		prev = c
	}
	return n
}

func TestTripsFromUndercityCrossContinentsAtMostTwice(t *testing.T) {
	ix, c := routeWorld(routeZones...)
	p := Build(c, ix)
	if len(p.Trips) != len(routeZones) {
		t.Fatalf("trips %+v", p.Trips)
	}
	cs := tripContinents(p)
	if n := crossings("Eastern Kingdoms", cs); n > 2 {
		t.Errorf("%d continent crossings in %v (%v)", n, cs, p.Trips)
	}
	for _, tr := range p.Trips {
		if tr.ETA <= 0 || tr.Travel == "" {
			t.Errorf("trip without a travel text or eta: %+v", tr)
		}
	}
}

func TestExitFromUndercityToStranglethornNamesTheZeppelin(t *testing.T) {
	ix, c := routeWorld("Stranglethorn Vale")
	p := Build(c, ix)
	if p.Exit == nil || !strings.HasPrefix(p.Exit.Label, "Zeppelin from Undercity to Grom'gol") || p.Exit.Via != "zeppelin" {
		t.Fatalf("exit %+v", p.Exit)
	}
	if strings.Contains(p.Exit.Label, "Walk or ride") || p.Trips[0].Travel != p.Exit.Label {
		t.Errorf("exit %q, trip %q", p.Exit.Label, p.Trips[0].Travel)
	}
}

func TestKnownFlightPathsAreNamedAsFlights(t *testing.T) {
	ix, c := routeWorld("Arathi Highlands")
	c.Seen = &character.Observed{Flights: []character.SeenFlight{
		{NPC: 1, Name: "Hammerfall", Map: 7100, Faction: "Horde", T: 1},
		{NPC: 2, Name: "Undercity", Map: mapUndercity, Faction: "Horde", T: 1},
	}}
	p := Build(c, ix)
	if p.Exit == nil || p.Exit.Via != "flight" || !strings.HasPrefix(p.Exit.Label, "Fly Undercity → Hammerfall") {
		t.Errorf("exit %+v", p.Exit)
	}
}

func TestTrainErrandHasAPositionOrNamesTheQuarter(t *testing.T) {
	ix, c := routeWorld()
	ix.Npcs[510] = questie.Entity{Name: "Michael Garrett", Flags: 0x2000, Faction: "H", Spawns: []questie.Spawn{{Area: 900, X: 63, Y: 48}}}
	ix.Npcs[511] = questie.Entity{Name: "Tarren Rider", Flags: 0x2000, Spawns: []questie.Spawn{{Area: 900, X: 20, Y: 70}}}
	c.Seen = &character.Observed{Flights: []character.SeenFlight{{NPC: 511, Name: "Tarren Rider", Map: mapUndercity, X: 20, Y: 70, T: 1}}}
	ix.Npcs[500] = questie.Entity{Name: "Rogue Master", Sub: "Rogue Trainer", Faction: "H", Spawns: []questie.Spawn{{Area: 900, X: 82, Y: 14}}}
	opt := Options{Trainable: func(character.Character) int { return 2 }}
	c.BankCaptured = true
	p := BuildWith(c, ix, opt)
	var got *Stop
	for i := range p.Steps {
		if p.Steps[i].Errand == ErrandTrain {
			got = &p.Steps[i]
		}
	}
	if got == nil || got.X != 82 || got.Y != 14 || got.Map != mapUndercity || !strings.Contains(got.Label, "Rogue Master") {
		t.Fatalf("train errand %+v", got)
	}
	delete(ix.Npcs, 500)
	p = BuildWith(c, ix, opt)
	for _, s := range p.Steps {
		if s.Errand == ErrandTrain && (s.X != NoPos || !strings.Contains(s.Label, "Rogues' Quarter")) {
			t.Errorf("unplaced train errand %+v", s)
		}
	}
}

func TestBankErrandUsesTheBankersSpawn(t *testing.T) {
	ix, c := routeWorld()
	ix.Npcs[501] = questie.Entity{Name: "Banker Test", Flags: bankerFlag, Spawns: []questie.Spawn{{Area: 900, X: 60, Y: 40}}}
	p := Build(c, ix)
	for _, s := range p.Steps {
		if s.Errand == ErrandBank && (s.X != 60 || s.Y != 40) {
			t.Errorf("bank errand %+v", s)
		}
	}
}

func TestDungeonQuestParkedReasonNamesTheInstanceAndZone(t *testing.T) {
	ix, c := routeWorld()
	ix.Areas[920] = questie.Area{UIMap: 7099, Zone: "Razorfen Kraul"}
	ix.Areas[921] = questie.Area{UIMap: 7098, Zone: "The Barrens"}
	ix.Npcs[600] = questie.Entity{Name: "Kraul Boar", Spawns: []questie.Spawn{{Area: 920, X: 30, Y: 30}}}
	ix.Quests[601] = &questie.Quest{ID: 601, Name: "Going, Going, Guano!", Level: 30, Creatures: []questie.Ref{{ID: 600}}}
	ix.Dungeons = []questie.Dungeon{{Area: 920, Name: "Razorfen Kraul", Parent: 921, Instance: 47, Quests: []int{601}}}
	c.Quests = append(c.Quests, character.Quest{QuestID: 601, Title: "Going, Going, Guano!", Objectives: []character.Objective{{Text: "Kraul Boar slain: 0/1"}}})
	p := BuildWith(c, ix, Options{Places: &Places{UIMaps: map[int]UIMap{7099: {ID: 7099, Instance: 47}}}})
	var why string
	for _, k := range p.Parked {
		if k.QuestID == 601 {
			why = k.Why
		}
	}
	if why != "needs a group: Razorfen Kraul (The Barrens)" {
		t.Errorf("why %q, parked %+v", why, p.Parked)
	}
	for _, k := range p.Parked {
		if strings.Contains(k.Why, "another area") {
			t.Errorf("vague reason %+v", k)
		}
	}
}

func TestValueTermLetsAValuableTripGoFirst(t *testing.T) {
	d0 := []int{5, 6}
	d := [][]int{{0, 9}, {9, 0}}
	if got := orderTrips(d0, d, []float64{0, 0}); got[0] != 0 {
		t.Errorf("equal value: the closer trip first, got %v", got)
	}
	if got := orderTrips(d0, d, []float64{0, 2}); got[0] != 1 {
		t.Errorf("a valuable trip first, got %v", got)
	}
}

// TestGoldenPlanLua keeps the schema-1.7 Plan.lua fixture the addon repo copies for its contract test.
// Run with WOWC_UPDATE_GOLDEN=1 to rewrite it.
func TestGoldenPlanLua(t *testing.T) {
	ix, c := routeWorld("Stranglethorn Vale", "The Barrens", "Silverpine Forest")
	ix.Npcs[500] = questie.Entity{Name: "Rogue Master", Sub: "Rogue Trainer", Faction: "H", Spawns: []questie.Spawn{{Area: 900, X: 82, Y: 14}}}
	ix.Areas[920] = questie.Area{UIMap: 7099, Zone: "Razorfen Kraul"}
	ix.Areas[921] = questie.Area{UIMap: 7098, Zone: "The Barrens"}
	ix.Npcs[600] = questie.Entity{Name: "Kraul Boar", Spawns: []questie.Spawn{{Area: 920, X: 30, Y: 30}}}
	ix.Quests[601] = &questie.Quest{ID: 601, Name: "Guano Quest", Level: 30, Creatures: []questie.Ref{{ID: 600}}}
	ix.Dungeons = []questie.Dungeon{{Area: 920, Name: "Razorfen Kraul", Parent: 921, Instance: 47, Quests: []int{601}}}
	c.Quests = append(c.Quests,
		character.Quest{QuestID: 601, Title: "Guano Quest", Objectives: []character.Objective{{Text: "Kraul Boar slain: 0/1"}}},
		character.Quest{QuestID: 999, Title: "Lost Quest"})
	opt := Options{Places: &Places{UIMaps: map[int]UIMap{7099: {ID: 7099, Instance: 47}}}, Trainable: func(character.Character) int { return 2 }}
	p := BuildWith(c, ix, opt)
	at := p.Steps[0]
	bundleChores(&p, []Chore{{Kind: ErrandRestock, Do: "buy 4 Empty Vial", Prep: "from", Why: "short of reagents for a batch of 5", Spots: []Spot{{Name: "Sly Garrett", Map: at.Map, X: at.X + 1, Y: at.Y}}}})
	got := Envelope{Generated: 1795363809, Plans: []Plan{p}}.Lua()
	const file = "../../testdata/plan/Plan-1.7.lua"
	if os.Getenv("WOWC_UPDATE_GOLDEN") != "" {
		if err := os.MkdirAll("../../testdata/plan", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(file)
	if err != nil || string(want) != got {
		t.Errorf("Plan.lua differs from %s (rewrite with WOWC_UPDATE_GOLDEN=1):\n%s", file, got)
	}
}

// tripBetween is the plan of a character of side standing in zone from with one ready turn-in in zone to.
func tripBetween(side, from, to string) Plan {
	ix := &questie.Index{Quests: map[int]*questie.Quest{}, Npcs: map[int]questie.Entity{}, Objects: map[int]questie.Entity{},
		Areas: map[int]questie.Area{900: {UIMap: 5001, Zone: from}, 901: {UIMap: 5002, Zone: to}}}
	ix.Npcs[100] = questie.Entity{Name: "Clerk", Spawns: []questie.Spawn{{Area: 901, X: 50, Y: 50}}}
	ix.Quests[100] = &questie.Quest{ID: 100, Name: "Errand", Level: 30, Finishers: []questie.Ref{{ID: 100}}}
	lvl, mp, x, y, yes := 30, 5001, 50.0, 50.0, true
	c := character.Character{Name: "Hero", Faction: side, Class: "ROGUE", QuestsCaptured: true, Level: &lvl,
		Location: &character.Location{Zone: from, MapID: &mp, X: &x, Y: &y},
		Quests:   []character.Quest{{QuestID: 100, Title: "Errand", Complete: &yes}}}
	return Build(c, ix)
}

func TestTramKeepsItsNameInALongerRoute(t *testing.T) {
	p := tripBetween("Alliance", "Darnassus", "Stormwind City")
	if len(p.Trips) != 1 || !strings.Contains(p.Trips[0].Travel, ", then Deeprun Tram from Ironforge to Stormwind") {
		t.Errorf("trips %+v", p.Trips)
	}
}

func TestNoFlightAcrossContinents(t *testing.T) {
	for _, tc := range [][3]string{{"Alliance", "Stormwind City", "Hellfire Peninsula"}, {"Horde", "Orgrimmar", "Undercity"}} {
		p := tripBetween(tc[0], tc[1], tc[2])
		if len(p.Trips) != 1 || strings.HasPrefix(p.Trips[0].Travel, "Fly ") {
			t.Errorf("%v: trips %+v", tc, p.Trips)
		}
	}
}

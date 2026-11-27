package route

import (
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/questie"
)

// tripIndex is a town (map 7001) with three logged quests: 1098 is handed in inside a dungeon, 1099 to
// a town NPC elsewhere, and 1100 is a kill quest finished where it was given.
func tripIndex() *questie.Index {
	sp := func(area int, x, y float64) []questie.Spawn { return []questie.Spawn{{Area: area, X: x, Y: y}} }
	return &questie.Index{
		Npcs: map[int]questie.Entity{
			1: {Name: "High Executor Hadrec", Spawns: sp(901, 50, 50)},
			2: {Name: "Deathstalker Vincent", Spawns: sp(209, 30, 40)},
			3: {Name: "Sputtervalve", Spawns: sp(903, 60, 60)},
			4: {Name: "Marshal Bran", Spawns: sp(901, 52, 52)},
			5: {Name: "Test Wolf", Spawns: sp(901, 20, 20)},
		},
		Objects: map[int]questie.Entity{},
		Areas: map[int]questie.Area{
			901: {UIMap: 7001, Zone: "Town"},
			902: {UIMap: 7002, Zone: "Silverpine Forest"},
			209: {UIMap: 33, Sub: "Shadowfang Keep", Zone: "Silverpine Forest"},
			903: {UIMap: 7003, Sub: "Ratchet", Zone: "The Barrens"},
		},
		Dungeons: []questie.Dungeon{{Area: 209, Parent: 902, Name: "Shadowfang Keep", Instance: 33}},
		Quests: map[int]*questie.Quest{
			1098: {ID: 1098, Name: "Deathstalkers in Shadowfang", Level: 25, Starters: []questie.Ref{{ID: 1}}, Finishers: []questie.Ref{{ID: 2}}},
			1099: {ID: 1099, Name: "Further Instructions", Level: 25, Starters: []questie.Ref{{ID: 1}}, Finishers: []questie.Ref{{ID: 3}}},
			1100: {ID: 1100, Name: "Wolf Trouble", Level: 25, Starters: []questie.Ref{{ID: 4}}, Finishers: []questie.Ref{{ID: 4}}, Creatures: []questie.Ref{{ID: 5}}},
		},
	}
}

func tripHero() character.Character {
	c := townHero(0)
	lvl := 25
	c.Level = &lvl
	yes, eight := true, 8
	c.Quests = []character.Quest{
		{QuestID: 1098, Title: "Deathstalkers in Shadowfang", Complete: &yes},
		{QuestID: 1099, Title: "Further Instructions", Complete: &yes},
		{QuestID: 1100, Title: "Wolf Trouble", Complete: &yes, Objectives: []character.Objective{{Text: "Test Wolf slain: 8/8", Have: &eight, Need: &eight}}},
	}
	return c
}

func TestTripWording(t *testing.T) {
	p := BuildWith(tripHero(), tripIndex(), Options{})
	all := append(append([]Stop{}, p.Steps...), p.Later...)
	find := func(quest string) Stop {
		for _, s := range all {
			if s.Kind == KindTurnIn && strings.Contains(s.Quest, quest) {
				return s
			}
		}
		t.Fatalf("no turn-in for %q in %s", quest, labels(all))
		return Stop{}
	}
	v := find("Deathstalkers")
	if want := "Find Deathstalker Vincent inside Shadowfang Keep (Silverpine Forest) (dungeon) to turn in Deathstalkers in Shadowfang"; v.Label != want {
		t.Errorf("dungeon trip label %q, want %q", v.Label, want)
	}
	if !v.Trip || v.Finisher != "Deathstalker Vincent" || v.Instance != "Shadowfang Keep" || v.FinisherPlace != "Shadowfang Keep (Silverpine Forest)" {
		t.Errorf("dungeon trip fields: %+v", v)
	}
	d := find("Further Instructions")
	if want := "Turn in Further Instructions to Sputtervalve (Ratchet)"; d.Label != want {
		t.Errorf("delivery label %q, want %q", d.Label, want)
	}
	if !d.Trip || d.Instance != "" || d.FinisherPlace != "Ratchet (The Barrens)" {
		t.Errorf("delivery fields: %+v", d)
	}
	k := find("Wolf Trouble")
	if k.Label != "Turn in Wolf Trouble to Marshal Bran" || k.Trip {
		t.Errorf("a finished kill quest keeps its turn-in: %+v", k)
	}
	for _, s := range all {
		if s.Trip && strings.Contains(strings.ToLower(s.Label), "on your way") {
			t.Errorf("trip called a stop on the way: %q", s.Label)
		}
	}
}

func TestDungeonTripIsNeverAFlyLeg(t *testing.T) {
	p := BuildWith(tripHero(), tripIndex(), Options{})
	for _, s := range p.Steps {
		if s.Kind == KindTravel && strings.Contains(s.Label, "Deathstalkers") {
			t.Errorf("a dungeon trip is not a fly-and-turn-in leg: %q", s.Label)
		}
	}
}

func TestTripFieldsInFinishersAndPlanLua(t *testing.T) {
	p := BuildWith(tripHero(), tripIndex(), Options{})
	rows := map[int]Finisher{}
	for _, f := range p.Finishers {
		rows[f.QuestID] = f
	}
	if f := rows[1098]; !f.Trip || f.Finisher != "Deathstalker Vincent" || f.Instance != "Shadowfang Keep" || f.FinisherPlace != "Shadowfang Keep (Silverpine Forest)" {
		t.Errorf("dungeon trip row: %+v", f)
	}
	if f := rows[1099]; !f.Trip || f.Instance != "" || f.FinisherPlace != "Ratchet (The Barrens)" {
		t.Errorf("delivery row: %+v", f)
	}
	if f := rows[1100]; f.Trip || f.Finisher != "" {
		t.Errorf("a kill quest is no trip: %+v", f)
	}
	lua := Envelope{Plans: []Plan{p}}.Lua()
	for _, want := range []string{`trip = true, finisher = "Deathstalker Vincent", finisherPlace = "Shadowfang Keep (Silverpine Forest)", instance = "Shadowfang Keep"`, `trip = true, finisher = "Sputtervalve", finisherPlace = "Ratchet (The Barrens)" }`} {
		if !strings.Contains(lua, want) {
			t.Errorf("Plan.lua lacks %q", want)
		}
	}
	if strings.Contains(lua, "on your way") {
		t.Error("Plan.lua calls a trip a stop on the way")
	}
}

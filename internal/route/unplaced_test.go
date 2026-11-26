package route

import (
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/questie"
)

// vantageIndex is a synthetic zone with a quest of three "place a vial" objectives. Area 901 is the
// zone, 902 and 903 are subzones with an NPC each.
func vantageIndex(kind string) *questie.Index {
	at := func(a int, x, y float64) []questie.Spawn { return []questie.Spawn{{Area: a, X: x, Y: y}} }
	q := &questie.Quest{ID: 98, Name: "Test Vantages", Level: 10, Zone: 901, Starters: []questie.Ref{{ID: 1}}, Finishers: []questie.Ref{{ID: 1}},
		Objective: "Place the vials near Dun Garok and Durnholde Keep."}
	switch kind {
	case "spawns":
		q.Triggers = []questie.Trigger{
			{Text: "Place Western vial", Spawns: at(901, 20, 30)},
			{Text: "Place Northern vial", Spawns: at(901, 40, 10)},
			{Text: "Place Southern vial", Spawns: at(901, 43, 80)},
		}
	case "none":
		q.Objective = "Place the vials somewhere high."
	}
	return &questie.Index{
		Quests: map[int]*questie.Quest{98: q},
		Npcs: map[int]questie.Entity{
			1: {Name: "Elder Test", Faction: "AH", Flags: flagQuestGiver, Spawns: at(901, 50, 50)},
			2: {Name: "Garok Guard", Spawns: append(at(902, 10, 20), at(902, 20, 40)...)},
			3: {Name: "Keep Guard", Spawns: at(903, 70, 60)},
		},
		Objects: map[int]questie.Entity{},
		Areas: map[int]questie.Area{901: {UIMap: 7001, Zone: "Testisle"}, 902: {UIMap: 7001, Zone: "Testisle", Sub: "Dun Garok"},
			903: {UIMap: 7001, Zone: "Testisle", Sub: "Durnholde Keep"}},
	}
}

func vantageChar() character.Character {
	c := hero()
	no := false
	c.Quests = []character.Quest{{QuestID: 98, Title: "Test Vantages", Complete: &no, Objectives: []character.Objective{
		{Text: "Place Western vial (0/1)", Done: &no}, {Text: "Place Northern vial (0/1)", Done: &no}, {Text: "Place Southern vial (0/1)", Done: &no}}}}
	x, y := 50.0, 50.0
	c.Location.X, c.Location.Y = &x, &y
	return c
}

func objectiveSteps(p Plan) []Stop {
	var out []Stop
	for _, s := range p.Steps {
		if s.Kind == KindObjective && s.QuestID == 98 {
			out = append(out, s)
		}
	}
	return out
}

// noZeroZero fails on any step, later stop or zone row at 0, 0, in the plan and in its Lua.
func noZeroZero(t *testing.T, p Plan) {
	t.Helper()
	for _, s := range append(append([]Stop{}, p.Steps...), p.Later...) {
		if s.X == 0 && s.Y == 0 {
			t.Errorf("stop at 0, 0: %+v", s)
		}
	}
	for _, f := range p.Finishers {
		if f.X == 0 && f.Y == 0 {
			t.Errorf("finisher at 0, 0: %+v", f)
		}
	}
	for _, z := range p.Zones {
		if z.Arrival.Known && z.Arrival.X == 0 && z.Arrival.Y == 0 {
			t.Errorf("zone arrival at 0, 0: %+v", z)
		}
	}
	if lua := (Envelope{Generated: 1, Plans: []Plan{p}}).Lua(); strings.Contains(lua, "x = 0.0, y = 0.0") {
		t.Errorf("Plan.lua has a 0, 0 position")
	}
}

func TestVialObjectivesGetOwnPlacedStops(t *testing.T) {
	p := Build(vantageChar(), vantageIndex("spawns"))
	steps := objectiveSteps(p)
	if len(steps) != 3 {
		t.Fatalf("want 3 vial steps: %+v", p.Steps)
	}
	seen := map[[2]float64]bool{}
	for _, s := range steps {
		if s.Unplaced || s.ZoneOnly || s.Approx || s.X <= 0 || s.Y <= 0 {
			t.Errorf("not placed: %+v", s)
		}
		seen[[2]float64{s.X, s.Y}] = true
	}
	if len(seen) != 3 {
		t.Errorf("the vials share spots: %+v", steps)
	}
	noZeroZero(t, p)
}

func TestSubzoneNamesGiveApproximateStops(t *testing.T) {
	c := vantageChar()
	c.Quests[0].Objectives = c.Quests[0].Objectives[:2]
	p := Build(c, vantageIndex("text"))
	steps := objectiveSteps(p)
	if len(steps) != 2 {
		t.Fatalf("want 2 steps: %+v", p.Steps)
	}
	want := [][2]float64{{15, 30}, {70, 60}} // text order: Dun Garok, then Durnholde Keep
	for i, s := range steps {
		if !s.Approx || s.Unplaced || s.X != want[i][0] || s.Y != want[i][1] {
			t.Errorf("step %d %+v, want approx at %v", i, s, want[i])
		}
	}
	noZeroZero(t, p)
}

func TestObjectiveWithNoPlaceIsUnplaced(t *testing.T) {
	p := Build(vantageChar(), vantageIndex("none"))
	steps := objectiveSteps(p)
	if len(steps) != 3 {
		t.Fatalf("want 3 steps: %+v", p.Steps)
	}
	for _, s := range steps {
		if s.X != -1 || s.Y != -1 || !s.Unplaced || s.Approx {
			t.Errorf("step %+v", s)
		}
	}
	lua := Envelope{Generated: 1, Plans: []Plan{p}}.Lua()
	if !strings.Contains(lua, "x = -1.0, y = -1.0") || !strings.Contains(lua, "unplaced = true") {
		t.Errorf("Plan.lua does not mark the unplaced stops")
	}
	noZeroZero(t, p)
}

func TestNoPlanHasZeroZero(t *testing.T) {
	for name, p := range map[string]Plan{
		"hub":      Build(hero(), hubIsle()),
		"bulwark":  Build(bulwarkChar(), bulwarkIsle(false)),
		"bulwarkA": Build(bulwarkChar(), bulwarkIsle(true)),
	} {
		t.Run(name, func(t *testing.T) { noZeroZero(t, p) })
	}
}

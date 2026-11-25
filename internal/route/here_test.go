package route

import (
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/lua"
	"github.com/WoW-Companion/advisor/internal/questie"
)

// constructIsle is a synthetic village with a friendly windshaper (3), a quest whose objectives are
// to talk to her, fetch a part from a lake and a core from a camp (30), a finished quest to turn in
// to the village's captain (31), a vendor, and a quest above the player's level from a nearby giver (32).
func constructIsle() *questie.Index {
	at := func(area int, x, y float64) []questie.Spawn { return []questie.Spawn{{Area: area, X: x, Y: y}} }
	return &questie.Index{
		Quests: map[int]*questie.Quest{
			30: {ID: 30, Name: "The Test Construct", Level: 9, Starters: []questie.Ref{{ID: 3}}, Finishers: []questie.Ref{{ID: 3}},
				Creatures: []questie.Ref{{ID: 3}}, Items: []int{100, 101}},
			31: {ID: 31, Name: "The Test Bulwark", Level: 9, Starters: []questie.Ref{{ID: 3}}, Finishers: []questie.Ref{{ID: 4}}},
			32: {ID: 32, Name: "Hill Errand", Level: 9, Starters: []questie.Ref{{ID: 6}}, Finishers: []questie.Ref{{ID: 6}}, Creatures: []questie.Ref{{ID: 5}}, Restricted: false},
		},
		Npcs: map[int]questie.Entity{
			3: {Name: "Test Windshaper", Sub: "Windshaper", Faction: "AH", Flags: flagQuestGiver | flagVendor | flagRepair, Spawns: at(901, 50, 50)},
			4: {Name: "Test Captain", Faction: "AH", Flags: flagQuestGiver, Spawns: at(901, 50.5, 50)},
			5: {Name: "Test Bandit", Spawns: at(903, 80, 20)},
			6: {Name: "Test Cultist", Faction: "AH", Flags: flagQuestGiver, Spawns: at(901, 55, 52)},
			7: {Name: "Test Sprite", Spawns: at(902, 10, 70)},
		},
		Objects: map[int]questie.Entity{},
		Items: map[int]questie.Item{
			100: {Name: "Test Gyro", Npcs: []int{7}},
			101: {Name: "Test Core", Npcs: []int{5}},
		},
		Areas: map[int]questie.Area{
			901: {UIMap: 7001, Zone: "Testisle", Sub: "Test Village"},
			902: {UIMap: 7001, Zone: "Testisle", Sub: "Test Lake"},
			903: {UIMap: 7001, Zone: "Testisle", Sub: "Test Camp"},
		},
	}
}

func constructChar() character.Character {
	c := hero()
	yes, no := true, false
	c.Quests = []character.Quest{
		{QuestID: 30, Title: "The Test Construct", Complete: &no, Objectives: []character.Objective{
			{Text: "Test Gyro: 0/1", Done: &no}, {Text: "Test Core: 0/1", Done: &no}}},
		{QuestID: 31, Title: "The Test Bulwark", Complete: &yes},
	}
	return c
}

func TestFriendlyNpcIsTalkedToNotKilled(t *testing.T) {
	ix := constructIsle()
	c := hero()
	no := false
	c.Quests = []character.Quest{{QuestID: 30, Title: "The Test Construct", Complete: &no, Objectives: []character.Objective{{Text: "Test Windshaper: 0/1", Done: &no}}}}
	p := Build(c, ix)
	var talk bool
	for _, s := range append(p.Steps, p.Later...) {
		if s.Kind == KindObjective && strings.Contains(s.Label, "Test Windshaper") {
			talk = strings.HasPrefix(s.Label, "Talk to ")
			if strings.HasPrefix(s.Label, "Kill") {
				t.Errorf("a friendly NPC is to be killed: %q", s.Label)
			}
		}
	}
	if !talk {
		t.Errorf("no Talk to step for the friendly NPC: %+v", p.Steps)
	}
	// What the objective text says wins: a listen objective stays "Listen to".
	if got := objectiveLabel(creatureVerb(ix.Npcs[3]), "Listen to what Test Windshaper has to say: 0/1"); got != "Listen to what Test Windshaper has to say (0/1)" {
		t.Errorf("label %q", got)
	}
	if creatureVerb(ix.Npcs[5]) != "Kill" {
		t.Errorf("a hostile mob is not killed")
	}
}

func TestOpenObjectivesAreStepsAtTheirOwnPlaces(t *testing.T) {
	p := Build(constructChar(), constructIsle())
	var gyro, core *Stop
	for i, s := range p.Steps {
		switch {
		case s.Kind == KindObjective && strings.Contains(s.Label, "Test Gyro"):
			gyro = &p.Steps[i]
		case s.Kind == KindObjective && strings.Contains(s.Label, "Test Core"):
			core = &p.Steps[i]
		}
	}
	if gyro == nil || core == nil {
		t.Fatalf("each open objective is a step: %+v", p.Steps)
	}
	if gyro.Place != "Test Lake" || gyro.X != 10 || core.Place != "Test Camp" || core.X != 80 {
		t.Errorf("objectives are not at their own places: %+v %+v", gyro, core)
	}
	if gyro.N == core.N {
		t.Errorf("two objectives at two places are one stop")
	}
}

func TestSameVillageTurnInComesBeforeTheTrip(t *testing.T) {
	p := Build(constructChar(), constructIsle())
	turn, far := -1, -1
	for _, s := range p.Steps {
		if s.Kind == KindTurnIn && s.QuestID == 31 {
			turn = s.N
		}
		if s.Kind == KindObjective && far < 0 {
			far = s.N
		}
	}
	if turn < 0 || far < 0 || turn > far {
		t.Errorf("the village turn-in (step %d) should come before the first trip (step %d): %+v", turn, far, p.Steps)
	}
}

func TestGroupByPlacePullsForward(t *testing.T) {
	// 0 village A, 1 far objective, 2 village B (a turn-in with no precedence), 3 far end.
	nodes := []*spot{{kind: KindPickup, m: 1, x: 10, y: 10}, {kind: KindObjective, m: 1, x: 60, y: 60}, {kind: KindTurnIn, m: 1, x: 11, y: 10}, {kind: KindObjective, m: 1, x: 70, y: 70}}
	p := &Problem{Nodes: []Node{{X: 10, Y: 10}, {X: 60, Y: 60}, {X: 11, Y: 10}, {X: 70, Y: 70}}}
	got := groupByPlace(p, nodes, []int{0, 1, 2, 3})
	if got[1] != 2 {
		t.Errorf("order %v: the turn-in at the pick-up's place should follow it", got)
	}
	// A precedence keeps it where it is.
	p.Nodes[2].Pred = []int{1}
	if got := groupByPlace(p, nodes, []int{0, 1, 2, 3}); got[1] != 1 {
		t.Errorf("order %v: a turn-in that needs the objective moved", got)
	}
}

func hereOf(p Plan, npc string) *Here {
	for i := range p.Here {
		if p.Here[i].NPC == npc {
			return &p.Here[i]
		}
	}
	return nil
}

func TestHereBlockListsMissingItemsAndWhere(t *testing.T) {
	p := Build(constructChar(), constructIsle())
	h := hereOf(p, "Test Windshaper")
	if h == nil {
		t.Fatalf("no here block: %+v", p.Here)
	}
	var q *HereQuest
	for i := range h.Quests {
		if h.Quests[i].QuestID == 30 {
			q = &h.Quests[i]
		}
	}
	if q == nil || q.Role != KindTurnIn || len(q.Missing) != 2 {
		t.Fatalf("quest 30 here: %+v", h.Quests)
	}
	for _, want := range []string{"Turn in The Test Construct once you have done", "Collect Test Gyro (0/1) (Test Lake)", "Collect Test Core (0/1) (Test Camp)"} {
		if !strings.Contains(q.Text, want) {
			t.Errorf("text %q lacks %q", q.Text, want)
		}
	}
	for _, want := range []string{"turn in", "buy and sell", "repair"} {
		if !contains(h.Can, want) {
			t.Errorf("can %v lacks %q", h.Can, want)
		}
	}
}

func contains(l []string, s string) bool {
	for _, v := range l {
		if v == s {
			return true
		}
	}
	return false
}

func TestOffPlanNearbyQuestHasGuidance(t *testing.T) {
	ix := constructIsle()
	ix.Quests[32].Level = 11 // above the level margin: listed, but not on the route
	p := Build(constructChar(), ix)
	h := hereOf(p, "Test Windshaper")
	if h == nil || len(h.Nearby) != 1 || h.Nearby[0].QuestID != 32 {
		t.Fatalf("nearby: %+v", p.Here)
	}
	g := h.Nearby[0].Guidance
	if len(g.Objectives) != 1 || !strings.HasPrefix(g.Objectives[0].Text, "Kill Test Bandit") || g.Objectives[0].Place != "Test Camp" || g.TurnIn == nil {
		t.Errorf("guidance %+v", g)
	}
	// The block reaches Plan.lua as data.
	vars, err := lua.Parse(Envelope{Generated: 1, Plans: []Plan{p}}.Lua())
	if err != nil {
		t.Fatal(err)
	}
	hero := vars["WoWCompanionPlan"].(lua.Table)["characters"].(lua.Table)["Hero"].(lua.Table)
	if len(lua.List(hero["here"])) == 0 {
		t.Errorf("no here in Plan.lua")
	}
}

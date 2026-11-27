package route

import (
	"slices"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/questie"
)

func reachIndex() *questie.Index {
	at := func(x, y float64) []questie.Spawn { return []questie.Spawn{{Area: 901, X: x, Y: y}} }
	ix := &questie.Index{
		Quests: map[int]*questie.Quest{},
		Npcs: map[int]questie.Entity{
			1:   {Name: "Tiger", Spawns: at(70, 30)},
			100: {Name: "Wharfmaster", Spawns: at(49, 50)},
			98:  {Name: "Far Giver", Spawns: at(10, 10)},
		},
		Objects: map[int]questie.Entity{},
		Areas:   map[int]questie.Area{901: {UIMap: 7001, Sub: "Booty Cove", Zone: "Testisle"}},
	}
	for id, lvl := range map[int]int{301: 35, 302: 36, 303: 38, 304: 35, 305: 35} {
		ix.Quests[id] = &questie.Quest{ID: id, Name: "Quest " + itoa(id), Level: lvl, Finishers: []questie.Ref{{ID: 100}}, Creatures: []questie.Ref{{ID: 1}}}
	}
	ix.Quests[310] = &questie.Quest{ID: 310, Name: "Sponsorship", Level: 40, Starters: []questie.Ref{{ID: 98}}, Finishers: []questie.Ref{{ID: 100}}}
	return ix
}

func reachChar(prefs *character.PlanPrefs) character.Character {
	c := hero()
	lvl := 32
	c.Level = &lvl
	c.PlanPrefs = prefs
	yes := true
	open := []character.Objective{{Text: "Tiger slain: 0/1"}}
	c.Quests = []character.Quest{
		{QuestID: 301, Title: "Quest 301", Objectives: open},
		{QuestID: 302, Title: "Quest 302", Objectives: open},
		{QuestID: 303, Title: "Quest 303", Objectives: open},
		{QuestID: 304, Title: "Quest 304", Objectives: open, Elite: &yes},
		{QuestID: 305, Title: "Quest 305", Objectives: open},
		{QuestID: 310, Title: "Sponsorship", Complete: &yes},
	}
	return c
}

func stepQuests(p Plan) []int {
	var ids []int
	for _, s := range p.Steps {
		ids = append(ids, s.QuestIDs...)
		ids = append(ids, s.QuestID)
	}
	return ids
}

func parkedOfID(p Plan, id int) *Parked {
	for i := range p.Parked {
		if p.Parked[i].QuestID == id {
			return &p.Parked[i]
		}
	}
	return nil
}

func TestLevelReachParksQuestsTooHigh(t *testing.T) {
	p := Build(reachChar(nil), reachIndex())
	if !slices.Contains(stepQuests(p), 301) {
		t.Errorf("a quest 3 levels above is a step: %v", stepQuests(p))
	}
	for _, id := range []int{302, 303} {
		k := parkedOfID(p, id)
		if k == nil || k.Kind != ParkTooHigh || !strings.Contains(k.Why, "doable around") || slices.Contains(stepQuests(p), id) {
			t.Errorf("quest %d: parked %+v, steps %v", id, k, stepQuests(p))
		}
	}
	if k := parkedOfID(p, 303); k != nil && (k.Why != "level 38 · doable around 35" || k.Level != 38 || k.Diff != "red") {
		t.Errorf("parked entry %+v", k)
	}
}

func TestLevelReachOffKeepsEverything(t *testing.T) {
	p := Build(reachChar(&character.PlanPrefs{LevelReach: -1}), reachIndex())
	for _, id := range []int{301, 302, 303} {
		if !slices.Contains(stepQuests(p), id) || parkedOfID(p, id) != nil {
			t.Errorf("quest %d should be a step with the limit off: %v", id, stepQuests(p))
		}
	}
}

func TestHiddenQuestIsParkedAndAbsentEverywhere(t *testing.T) {
	prefs := &character.PlanPrefs{LevelReach: 3, Hidden: map[int]bool{305: true}}
	p := Build(reachChar(prefs), reachIndex())
	k := parkedOfID(p, 305)
	if k == nil || k.Kind != ParkHidden || k.Why != "Hidden by you" {
		t.Fatalf("hidden quest parked as %+v", k)
	}
	if slices.Contains(stepQuests(p), 305) {
		t.Errorf("hidden quest is a step")
	}
	for _, tr := range p.Trips {
		if slices.Contains(tr.QuestIDs, 305) {
			t.Errorf("hidden quest is in trip %+v", tr)
		}
	}
	for _, s := range p.Later {
		if s.QuestID == 305 {
			t.Errorf("hidden quest is in later")
		}
	}
}

func TestEliteQuestParkedUnlessIncluded(t *testing.T) {
	p := Build(reachChar(nil), reachIndex())
	if k := parkedOfID(p, 304); k == nil || k.Kind != ParkElite || k.Why != "Elite: needs a group" || slices.Contains(stepQuests(p), 304) {
		t.Errorf("elite quest: parked %+v", k)
	}
	p = Build(reachChar(&character.PlanPrefs{LevelReach: 3, IncludeElite: true}), reachIndex())
	if parkedOfID(p, 304) != nil || !slices.Contains(stepQuests(p), 304) {
		t.Errorf("elite quest should be a step when included: %v", stepQuests(p))
	}
}

func TestNearbyReadyDeliveryComesFirstWithPlainWording(t *testing.T) {
	p := Build(reachChar(nil), reachIndex())
	if len(p.Steps) == 0 || p.Steps[0].QuestID != 310 {
		t.Fatalf("the delivery should be first: %s", labels(p.Steps))
	}
	if want := "Turn in Sponsorship to Wharfmaster (Booty Cove)"; p.Steps[0].Label != want {
		t.Errorf("label %q, want %q", p.Steps[0].Label, want)
	}
}

func TestEveryStepCarriesLevelAndDiff(t *testing.T) {
	p := Build(reachChar(nil), reachIndex())
	for _, s := range p.Steps {
		if s.Level == 0 || s.Diff == "" {
			t.Errorf("step %d lacks level or diff: %+v", s.N, s)
		}
	}
	for _, k := range p.Parked {
		if k.Level == 0 || k.Diff == "" {
			t.Errorf("parked %d lacks level or diff: %+v", k.QuestID, k)
		}
	}
}

func TestDiffOf(t *testing.T) {
	for _, c := range []struct {
		q, p int
		want string
	}{{0, 32, ""}, {37, 32, "red"}, {36, 32, "orange"}, {34, 32, "yellow"}, {30, 32, "yellow"}, {29, 32, "green"}, {24, 32, "grey"}} {
		if got := DiffOf(c.q, c.p); got != c.want {
			t.Errorf("DiffOf(%d, %d) = %q, want %q", c.q, c.p, got, c.want)
		}
	}
}

func TestGreyLevelFollowsThePlayersLevel(t *testing.T) {
	for _, c := range []struct{ p, grey int }{{1, 0}, {4, 0}, {5, 0}, {6, 1}, {10, 4}, {20, 13}, {30, 22}, {39, 31}, {40, 31}, {50, 39}, {55, 43}, {59, 47}, {60, 51}, {70, 61}} {
		if got := GreyLevel(c.p); got != c.grey {
			t.Errorf("GreyLevel(%d) = %d, want %d", c.p, got, c.grey)
		}
	}
	for _, c := range []struct {
		q, p int
		want string
	}{{4, 10, "grey"}, {5, 10, "green"}, {13, 20, "grey"}, {14, 20, "green"}, {40, 50, "green"}, {39, 50, "grey"}, {45, 55, "green"}} {
		if got := DiffOf(c.q, c.p); got != c.want {
			t.Errorf("DiffOf(%d, %d) = %q, want %q", c.q, c.p, got, c.want)
		}
	}
}

func TestGreyQuestParkedAtTheGameGreyLevel(t *testing.T) {
	at := func(x, y float64) []questie.Spawn { return []questie.Spawn{{Area: 901, X: x, Y: y}} }
	ix := &questie.Index{
		Quests:  map[int]*questie.Quest{},
		Npcs:    map[int]questie.Entity{1: {Name: "Tiger", Spawns: at(70, 30)}, 100: {Name: "Wharfmaster", Spawns: at(49, 50)}, 2: {Name: "Wolf", Spawns: []questie.Spawn{{Area: 902, X: 40, Y: 40}}}, 101: {Name: "Harbourer", Spawns: []questie.Spawn{{Area: 902, X: 41, Y: 41}}}},
		Objects: map[int]questie.Entity{},
		Areas:   map[int]questie.Area{901: {UIMap: 7001, Sub: "Booty Cove", Zone: "Testisle"}, 902: {UIMap: 7002, Sub: "Far Shore", Zone: "Otherisle"}},
	}
	ix.Quests[400] = &questie.Quest{ID: 400, Name: "Quest 400", Level: 20, Finishers: []questie.Ref{{ID: 100}}, Creatures: []questie.Ref{{ID: 1}}}
	open := []character.Objective{{Text: "Tiger slain: 0/1"}}
	for _, tc := range []struct {
		player, quest int
		grey          bool
	}{{20, 13, true}, {20, 14, false}, {50, 40, false}, {50, 39, true}, {10, 4, true}} {
		ix.Quests[401] = &questie.Quest{ID: 401, Name: "Quest 401", Level: tc.quest, Finishers: []questie.Ref{{ID: 101}}, Creatures: []questie.Ref{{ID: 2}}}
		c := hero()
		lvl := tc.player
		c.Level = &lvl
		c.PlanPrefs = &character.PlanPrefs{LevelReach: -1}
		c.Quests = []character.Quest{{QuestID: 400, Title: "Quest 400", Objectives: open}, {QuestID: 401, Title: "Quest 401", Objectives: []character.Objective{{Text: "Wolf slain: 0/1"}}}}
		p := Build(c, ix)
		k := parkedOfID(p, 401)
		if grey := k != nil && k.Kind == ParkGrey; grey != tc.grey {
			t.Errorf("a level %d quest for a level %d player: parked %+v, want grey %v", tc.quest, tc.player, k, tc.grey)
		}
	}
}

package route

import (
	"github.com/WoW-Companion/advisor/internal/character"
	"testing"

	"github.com/WoW-Companion/advisor/internal/lua"
	"github.com/WoW-Companion/advisor/internal/questie"
)

func TestFinishersForLoggedQuests(t *testing.T) {
	at := func(area int, x, y float64) []questie.Spawn { return []questie.Spawn{{Area: area, X: x, Y: y}} }
	ix := &questie.Index{
		Quests: map[int]*questie.Quest{
			40: {ID: 40, Name: "Ready One", Level: 9, Finishers: []questie.Ref{{ID: 1}}},
			41: {ID: 41, Name: "Open Two", Level: 9, Finishers: []questie.Ref{{ID: 1}}, Creatures: []questie.Ref{{ID: 5}}},
			42: {ID: 42, Name: "Twin Spawns", Level: 9, Finishers: []questie.Ref{{ID: 2}, {Object: true, ID: 9}}, Creatures: []questie.Ref{{ID: 5}}},
			43: {ID: 43, Name: "Nowhere", Level: 9, Finishers: []questie.Ref{{ID: 3}}},
		},
		Npcs: map[int]questie.Entity{
			1: {Name: "Test Elder", Faction: "AH", Spawns: at(901, 50, 50)},
			2: {Name: "Test Twin", Faction: "AH", Spawns: append(at(901, 20, 20), at(902, 90, 90)...)},
			3: {Name: "Lost Soul", Spawns: at(999, 1, 1)}, // an area the dataset does not know
			5: {Name: "Test Wolf", Spawns: at(901, 22, 22)},
		},
		Objects: map[int]questie.Entity{9: {Name: "Test Chest", Spawns: at(901, 70, 70)}},
		Areas: map[int]questie.Area{
			901: {UIMap: 7001, Zone: "Testisle", Sub: "Test Village"},
			902: {UIMap: 7002, Zone: "Otherisle"},
		},
	}
	c := hero()
	yes, no := true, false
	c.Quests = nil
	for _, q := range []struct {
		id    int
		ready *bool
	}{{40, &yes}, {41, &no}, {42, &no}, {43, &yes}} {
		c.Quests = append(c.Quests, characterQuest(q.id, q.ready))
	}
	p := Build(c, ix)
	var got []Finisher
	for _, f := range p.Finishers {
		got = append(got, f)
	}
	want := []Finisher{
		{QuestID: 40, Quest: "Ready One", NPC: "Test Elder", Map: 7001, X: 50, Y: 50, Place: "Test Village"},
		{QuestID: 41, Quest: "Open Two", NPC: "Test Elder", Map: 7001, X: 50, Y: 50, Place: "Test Village"},
		// nearest the objective (the wolf at 22, 22) first; the Otherisle spawn last
		{QuestID: 42, Quest: "Twin Spawns", NPC: "Test Twin", Map: 7001, X: 20, Y: 20, Place: "Test Village"},
		{QuestID: 42, Quest: "Twin Spawns", Object: "Test Chest", Map: 7001, X: 70, Y: 70, Place: "Test Village"},
		{QuestID: 42, Quest: "Twin Spawns", NPC: "Test Twin", Map: 7002, X: 90, Y: 90, Place: "Otherisle"},
	}
	if len(got) != len(want) {
		t.Fatalf("finishers %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	vars, err := lua.Parse(Envelope{Generated: 1, Plans: []Plan{p}}.Lua())
	if err != nil {
		t.Fatal(err)
	}
	root := vars["WoWCompanionPlan"].(lua.Table)
	if root["schema"] != Schema {
		t.Errorf("schema %v", root["schema"])
	}
	rows := lua.List(root["characters"].(lua.Table)["Hero"].(lua.Table)["finishers"])
	if len(rows) != 5 {
		t.Fatalf("lua rows %d", len(rows))
	}
	if r := rows[3].(lua.Table); r["object"] != "Test Chest" || r["npc"] != nil || r["questID"] != int64(42) {
		t.Errorf("object row %v", r)
	}
}

func characterQuest(id int, complete *bool) character.Quest {
	return character.Quest{QuestID: id, Complete: complete}
}

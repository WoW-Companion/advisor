package questie

import (
	"os"
	"path/filepath"
	"testing"
)

// A synthetic dungeon table: Ember Chasm (area 910, alternative 911), whose entrance is in the
// zone 901 at 52.6, 49; instance 7 maps to it. Invented ids and names.
const dungeonsLua = `local dungeons = {
    [910] = {"Ember Chasm", {911}, 901, {52.6, 49}}, -- a made-up dungeon
    [920] = {"Broken Row"},
}
return dungeons
`

const instancesLua = `return {
    [7] = 910, -- Ember Chasm
}
`

func dungeonFixture(t *testing.T, withFiles bool) (*Index, Status) {
	rows := append(fixtureRows(),
		row{"Npc", 300, "S", map[int]any{1: "Chief Cinder", 5: 12, 6: 3}},
		row{"Npc", 300, "7", map[int]any{910: []any{[]any{30.0, 40.0}}}},
		row{"Npc", 301, "S", map[int]any{1: "Ash Guard", 5: 11, 6: 1}},
		row{"Npc", 301, "7", map[int]any{911: []any{[]any{60.0, 20.0}}}},
		row{"Npc", 302, "S", map[int]any{1: "Cave Bat", 5: 10}},
		row{"Npc", 302, "7", map[int]any{910: []any{[]any{10.0, 10.0}}}},
		row{"Item", 400, "S", map[int]any{1: "Cinder Cap"}},
		row{"Item", 400, "2", []any{300}},
		row{"Quest", 2, "10", []any{[]any{[]any{301, nil, 0}}}}) // kill an Ash Guard
	game := install(t, header, rows)
	if withFiles {
		zones := filepath.Join(game, "Interface", "AddOns", "QuestieDB", "support", "Forever", "Zones")
		os.WriteFile(filepath.Join(zones, "dungeons.lua"), []byte(dungeonsLua), 0o644)
		os.WriteFile(filepath.Join(zones, "instanceIdToAreaId.lua"), []byte(instancesLua), 0o644)
	}
	ix, st := OpenGame(game)
	if ix == nil {
		t.Fatalf("not readable: %+v", st)
	}
	return ix, st
}

func TestDungeonEntranceCreaturesDropsQuests(t *testing.T) {
	ix, _ := dungeonFixture(t, true)
	d := ix.FindDungeon(7, "")
	if d == nil || d.Name != "Ember Chasm" || d.Area != 910 || d.Parent != 901 || !d.Entrance || d.X != 52.6 || d.Y != 49 {
		t.Fatalf("dungeon by instance: %+v", d)
	}
	if byName := ix.FindDungeon(0, "ember chasm"); byName != d {
		t.Fatalf("by name: %+v", byName)
	}
	in := ix.Inside(d)
	if in.Zone != "Testisle" {
		t.Errorf("entrance zone %q", in.Zone)
	}
	if len(in.Creatures) != 3 || in.Creatures[0].Name != "Chief Cinder" || !in.Creatures[0].Boss || in.Creatures[1].Name != "Ash Guard" || !in.Creatures[1].Elite {
		t.Errorf("creatures %+v", in.Creatures)
	}
	if got := ix.DropsOf(300); len(got) != 1 || got[0] != 400 {
		t.Errorf("drops %v", got)
	}
	if len(in.Quests) != 1 || in.Quests[0].ID != 2 {
		t.Errorf("quests %+v", in.Quests)
	}
}

func TestDungeonFilesMissing(t *testing.T) {
	ix, _ := dungeonFixture(t, false)
	if len(ix.Dungeons) != 0 || ix.DungeonReason == "" || ix.FindDungeon(7, "Ember Chasm") != nil {
		t.Fatalf("expected no dungeons with a reason: %+v %q", ix.Dungeons, ix.DungeonReason)
	}
	if len(ix.Quests) == 0 {
		t.Fatal("quests lost with the dungeon files")
	}
}

func TestParseDungeonRowShapes(t *testing.T) {
	for name, src := range map[string]string{
		"garbage": "this is not lua {{{",
		"empty":   "return {}",
	} {
		if _, ok := parseLuaTable(src); ok {
			if rows, _ := parseDungeonsText(src); len(rows) != 0 {
				t.Errorf("%s gave rows", name)
			}
		}
	}
	rows, _ := parseDungeonsText(`{ [5] = {"Triple", 3, {9, 1.5, 2.5}} }`)
	if len(rows) != 1 || rows[0].Parent != 9 || rows[0].X != 1.5 || rows[0].Y != 2.5 {
		t.Errorf("triple form: %+v", rows)
	}
}

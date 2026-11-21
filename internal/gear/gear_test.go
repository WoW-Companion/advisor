package gear

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/parquet-go/parquet-go"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/store"
)

type itemRow struct {
	ID            int64   `parquet:"id"`
	Name          string  `parquet:"name"`
	ItemLevel     int32   `parquet:"item_level"`
	RequiredLevel int32   `parquet:"required_level"`
	Quality       int32   `parquet:"quality"`
	ClassID       int32   `parquet:"class_id"`
	SubclassID    int32   `parquet:"subclass_id"`
	Slot          int32   `parquet:"slot"`
	ClassMask     int32   `parquet:"allowable_class_mask"`
	StatTypes     []int16 `parquet:"stat_types,list"`
	StatsStatus   string  `parquet:"stats_status"`
}

type lootRow struct {
	ItemID int64  `parquet:"item_id"`
	Kind   string `parquet:"source_kind"`
	ID     int64  `parquet:"source_id"`
	Kills  int32  `parquet:"kills"`
	Drops  int32  `parquet:"drops"`
}

type npcRow struct {
	ID       int64  `parquet:"id"`
	Name     string `parquet:"name"`
	MinLevel *int32 `parquet:"min_level,optional"`
	MaxLevel *int32 `parquet:"max_level,optional"`
}

func table(t *testing.T, rows any) store.SizedReaderAt {
	t.Helper()
	var buf bytes.Buffer
	var err error
	switch r := rows.(type) {
	case []itemRow:
		err = parquet.Write(&buf, r)
	case []lootRow:
		err = parquet.Write(&buf, r)
	case []npcRow:
		err = parquet.Write(&buf, r)
	}
	if err != nil {
		t.Fatal(err)
	}
	return store.SizedReaderAt{ReaderAt: bytes.NewReader(buf.Bytes()), Size: int64(buf.Len())}
}

const (
	cloth, leather, plate = 1, 2, 4
	headSlot, chestSlot   = 1, 5
	neckSlot, ringSlot    = 2, 11
	offhandSlot           = 22
	oneHand               = 13
)

func it(id int64, name string, ilvl, req, q, class, sub, slot int32, stats ...int16) itemRow {
	status := "known"
	if stats == nil {
		status = "unknown"
	}
	return itemRow{id, name, ilvl, req, q, class, sub, slot, -1, stats, status}
}

// items: cloth, leather and plate gear; weapons; a class-locked item; a too-high-level item.
func items(status string) []itemRow {
	rows := []itemRow{
		it(1, "Cloth Hood", 20, 14, 2, 4, cloth, headSlot, 5, 7),    // int + stamina
		it(2, "Spirit Hood", 24, 14, 2, 4, cloth, headSlot, 6, 7),   // spirit + stamina
		it(3, "Leather Cap", 22, 14, 2, 4, leather, headSlot, 3, 7), // agi + stamina
		it(4, "Plate Helm", 40, 14, 3, 4, plate, headSlot, 4, 7),    // never usable
		it(5, "Future Hood", 60, 20, 3, 4, cloth, headSlot, 5),      // requires level 20
		it(6, "Ring of Mind", 18, 10, 2, 4, 0, ringSlot, 5),         // any armor type
		it(7, "Ring of Agility", 18, 10, 2, 4, 0, ringSlot, 3),      //
		it(8, "Priest Mace", 20, 10, 2, 2, 4, oneHand, 5),           // mace
		it(9, "Rogue Fist", 20, 10, 2, 2, 13, oneHand, 3),           //
		it(10, "Off-hand Dagger", 19, 10, 2, 2, 15, offhandSlot, 3), //
		it(11, "Bound Hood", 30, 10, 3, 4, leather, headSlot, 5),    //
		it(12, "Plain Robe", 15, 8, 1, 4, cloth, 20, 5),             // robe fits chest
		it(13, "Statless Hat", 17, 9, 2, 4, leather, headSlot),      // unknown stats
		it(14, "Junk", 99, 1, 0, 4, cloth, headSlot, 5),             // poor quality
		it(15, "Plain Amulet", 10, 0, 2, 4, 0, neckSlot, 5),         // no listed required level, low item level
		it(16, "Endgame Amulet", 60, 0, 3, 4, 0, neckSlot, 5),       // no listed required level, high item level
	}
	rows[10].ClassMask = 1 << (4 - 1) // rogue only
	if status != "" {
		for i := range rows {
			rows[i].StatsStatus = status
			if status == "unknown" {
				rows[i].StatTypes = nil
			}
		}
	}
	return rows
}

func synth(t *testing.T, status string) store.DataStore {
	t.Helper()
	lo, hi := int32(10), int32(12)
	return store.New(store.ReaderSource{
		"items": table(t, items(status)),
		"loot_sources": table(t, []lootRow{
			{1, "creature", 500, 200, 50},
			{1, "creature", 501, 100, 5},
			{2, "object", 900, 0, 0},
			{6, "creature", 502, 40, 4},
		}),
		"npcs": table(t, []npcRow{{500, "Far Boss", &[]int32{40}[0], &[]int32{42}[0]}, {501, "Near Wolf", &lo, &hi}, {502, "Ring Thief", &lo, nil}}),
	})
}

func ip(n int) *int { return &n }

func char(class string, level int, eq map[string]character.EquippedItem) character.Character {
	return character.Character{Name: "T" + class, Class: class, Level: &level, Equipment: eq}
}

func names(cs []Candidate) string {
	var out []string
	for _, c := range cs {
		out = append(out, c.Item.Name)
	}
	return strings.Join(out, ",")
}

func TestUpgrades(t *testing.T) {
	worn := func(id, ilvl, q int) map[string]character.EquippedItem {
		return map[string]character.EquippedItem{"head": {Slot: "head", ItemID: ip(id), ItemLevel: ip(ilvl), Quality: ip(q)}}
	}
	cases := []struct {
		name   string
		char   character.Character
		group  string
		status string
		slot   string
		want   string
		basis  string
	}{
		// Priest weights prefer intellect over spirit-only gear of similar level; Cloth Hood (20) wearing nothing.
		{"priest head, empty, stats", char("PRIEST", 12, nil), "Test Holy", "", "head", "Spirit Hood,Cloth Hood", BasisStats},
		{"priest head, worn plain hood", char("PRIEST", 12, worn(1, 20, 2)), "Test Holy", "", "head", "Spirit Hood", BasisStats},
		{"level 12 cannot equip level 20", char("PRIEST", 12, nil), "Discipline", "", "head", "Spirit Hood,Cloth Hood", BasisStats},
		{"level 17 can", char("PRIEST", 17, nil), "Discipline", "", "head", "Future Hood,Spirit Hood,Cloth Hood", BasisStats},
		{"rogue head is leather only", char("ROGUE", 12, nil), "Assassination", "", "head", "Leather Cap,Statless Hat,Bound Hood", BasisStats},
		{"rogue dual-wields, priest cannot", char("ROGUE", 12, nil), "Combat", "", "offhand", "Rogue Fist,Off-hand Dagger,Priest Mace", BasisStats},
		{"priest weapon", char("PRIEST", 12, nil), "Shadow", "", "mainhand", "Priest Mace", BasisStats},
		{"rings by spec", char("ROGUE", 12, nil), "Subtlety", "", "finger1", "Ring of Agility,Ring of Mind", BasisStats},
		{"no stats: item level only", char("PRIEST", 12, nil), "Holy", "unknown", "head", "Spirit Hood,Cloth Hood", BasisItemLevel},
		{"no spec: item level only", char("PRIEST", 12, nil), "", "", "head", "Spirit Hood,Cloth Hood", BasisItemLevel},
		{"nothing beats the worn item", char("PRIEST", 12, worn(2, 24, 2)), "Holy", "unknown", "head", "", BasisItemLevel},
		{"required level 0 is unknown: low item level only", char("PRIEST", 12, nil), "Holy", "", "neck", "Plain Amulet", BasisStats},
		{"required level 0, level 60", char("PRIEST", 60, nil), "Holy", "", "neck", "Endgame Amulet,Plain Amulet", BasisStats},
		{"robe fits chest", char("PRIEST", 12, nil), "Holy", "", "chest", "Plain Robe", BasisStats},
	}
	for _, c := range cases {
		res, err := Find(c.char, synth(t, c.status), c.group, Options{Slot: c.slot, Limit: 3})
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		got := res.Slots[c.slot]
		if names(got) != c.want {
			t.Errorf("%s: got %q, want %q", c.name, names(got), c.want)
			continue
		}
		for _, cd := range got {
			if cd.Basis != c.basis && !(c.basis == BasisStats && cd.Item.Name == "Statless Hat") {
				t.Errorf("%s: %s basis %s, want %s", c.name, cd.Item.Name, cd.Basis, c.basis)
			}
			if cd.Item.RequiredLevel > *c.char.Level+EquipWindow {
				t.Errorf("%s: %s requires level %d", c.name, cd.Item.Name, cd.Item.RequiredLevel)
			}
		}
		if res.ItemLevelOnly != (c.basis == BasisItemLevel) {
			t.Errorf("%s: ItemLevelOnly = %v", c.name, res.ItemLevelOnly)
		}
	}
}

func TestUpgradesWholeMap(t *testing.T) {
	m, err := Upgrades(char("PRIEST", 12, nil), synth(t, ""), "Holy")
	if err != nil {
		t.Fatal(err)
	}
	for slot, cs := range m {
		for _, c := range cs {
			if c.Item.Name == "Plate Helm" || c.Item.Name == "Leather Cap" || c.Item.Name == "Rogue Fist" {
				t.Errorf("%s: priest offered %s", slot, c.Item.Name)
			}
		}
	}
	if len(m["finger1"]) == 0 || len(m["finger2"]) == 0 || len(m["offhand"]) != 0 {
		t.Errorf("slots = %v", fmt.Sprint(m))
	}
}

func TestSourcesAndRender(t *testing.T) {
	c := char("PRIEST", 12, nil)
	res, err := Find(c, synth(t, ""), "Test Holy", Options{Limit: 3})
	if err != nil {
		t.Fatal(err)
	}
	text := Render(c, res)
	for _, want := range []string{
		"Ranked by item level weighted by Holy stat types; advisor defaults, not authoritative",
		"Cloth Hood (ilvl 20, requires level 14; empty slot, by stats): from Near Wolf, level 10-12, zone unknown, drops 5.0% of kills; Far Boss, level 40-42, zone unknown, drops 25.0% of kills",
		"Spirit Hood (ilvl 24, requires level 14; empty slot, by stats): from object 900, zone unknown, drop chance unknown",
		"Plain Robe (ilvl 15, requires level 8; empty slot, by stats): source unknown",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("render lacks %q:\n%s", want, text)
		}
	}
	res, _ = Find(c, synth(t, "unknown"), "Test Holy", Options{Slot: "head"})
	if text := Render(c, res); !strings.Contains(text, "Ranked by item level only (quality breaks ties): the dataset has no stats for these items.") {
		t.Errorf("item level only not stated:\n%s", text)
	}
}

func TestFindErrors(t *testing.T) {
	cases := []struct {
		name string
		c    character.Character
		ds   store.DataStore
		want string
	}{
		{"no dataset", char("PRIEST", 12, nil), nil, "no dataset"},
		{"no level", character.Character{Class: "PRIEST"}, synth(t, ""), "no character level"},
		{"unsupported class", char("MONK", 12, nil), synth(t, ""), "no gear rules"},
		{"no items table", char("PRIEST", 12, nil), store.New(store.ReaderSource{}), "items table"},
	}
	for _, c := range cases {
		if _, err := Find(c.c, c.ds, "", Options{}); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v", c.name, err)
		}
	}
}

func TestCanUse(t *testing.T) {
	priest, rogue := rules["PRIEST"], rules["ROGUE"]
	cases := []struct {
		r                classRules
		class, sub, slot int8
		want             bool
	}{
		{priest, 4, cloth, 7, true}, {priest, 4, leather, 7, false}, {rogue, 4, leather, 7, true},
		{rogue, 4, cloth, 7, false}, {rogue, 4, 1, 16, true}, {priest, 4, 6, 14, false},
		{priest, 2, 15, 13, true}, {priest, 2, 7, 13, false}, {priest, 2, 15, 22, false},
		{rogue, 2, 15, 22, true}, {rogue, 2, 5, 17, false}, {priest, 2, 19, 26, true},
	}
	for i, c := range cases {
		if got := c.r.canUse(c.class, c.sub, c.slot); got != c.want {
			t.Errorf("case %d: canUse = %v", i, got)
		}
	}
	if !allowsClass(-1, 5) || !allowsClass(0, 5) || !allowsClass(1<<4, 5) || allowsClass(1<<3, 5) {
		t.Error("allowsClass")
	}
}

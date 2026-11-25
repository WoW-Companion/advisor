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
	Bonding       int32   `parquet:"bonding"`
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
	return itemRow{id, name, ilvl, req, q, class, sub, slot, -1, stats, status, 0}
}

// items: cloth, leather and plate gear; weapons; a class-locked item; a too-high-level item.
func items(status string) []itemRow {
	rows := []itemRow{
		it(1, "Cloth Hood", 20, 14, 2, 4, cloth, headSlot, 5, 7),    // int + stamina
		it(2, "Spirit Hood", 24, 14, 2, 4, cloth, headSlot, 6, 7),   // spirit + stamina
		it(3, "Leather Cap", 22, 14, 2, 4, leather, headSlot, 3, 7), // agi + stamina
		it(4, "Plate Helm", 40, 14, 3, 4, plate, headSlot, 4, 7),    // never usable
		it(5, "Future Hood", 40, 20, 3, 4, cloth, headSlot, 5),      // requires level 20
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
		{"level 18 can", char("PRIEST", 18, nil), "Discipline", "", "head", "Future Hood,Spirit Hood,Cloth Hood", BasisStats},
		{"level 17 is two short of 20", char("PRIEST", 17, nil), "Discipline", "", "head", "Spirit Hood,Cloth Hood", BasisStats},
		{"rogue head is leather only", char("ROGUE", 12, nil), "Assassination", "", "head", "Leather Cap,Statless Hat,Bound Hood", BasisStats},
		{"rogue dual-wields, priest cannot", char("ROGUE", 12, nil), "Combat", "", "offhand", "Rogue Fist,Off-hand Dagger,Priest Mace", BasisStats},
		{"priest weapon", char("PRIEST", 12, nil), "Shadow", "", "mainhand", "Priest Mace", BasisStats},
		{"rings by spec", char("ROGUE", 12, nil), "Subtlety", "", "finger1", "Ring of Agility,Ring of Mind", BasisStats},
		{"no stats: item level only", char("PRIEST", 12, nil), "Holy", "unknown", "head", "Spirit Hood,Cloth Hood", BasisItemLevel},
		{"no spec: item level only", char("PRIEST", 12, nil), "", "", "head", "Spirit Hood,Cloth Hood", BasisItemLevel},
		{"nothing beats the worn item", char("PRIEST", 12, worn(2, 24, 2)), "Holy", "unknown", "head", "", BasisItemLevel},
		{"required level 0 is estimated: low item level only", char("PRIEST", 12, nil), "Holy", "", "neck", "Plain Amulet", BasisStats},
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
			if cd.Item.ReqLevel > *c.char.Level+EquipWindow {
				t.Errorf("%s: %s requires level %d", c.name, cd.Item.Name, cd.Item.ReqLevel)
			}
		}
		if res.ItemLevelOnly != (c.basis == BasisItemLevel) {
			t.Errorf("%s: ItemLevelOnly = %v", c.name, res.ItemLevelOnly)
		}
	}
}

func TestUpgradesWholeMap(t *testing.T) {
	res, err := Find(char("PRIEST", 12, nil), synth(t, ""), "Holy", Options{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Slots["finger1"]) != 1 || len(res.Slots["finger2"]) != 1 || res.Slots["finger1"][0].Item.ID == res.Slots["finger2"][0].Item.ID {
		t.Errorf("rings = %v", res.Slots)
	}
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
	if len(m["finger1"]) == 0 || len(m["offhand"]) != 0 {
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
		"Ranked by the gain over what is worn, weighted by Holy stat types and listed by the level needed to wear it; advisor defaults, not authoritative",
		"Cloth Hood (at level 14; empty slot): from Near Wolf, level 10-12, zone unknown, drops 5.0% of kills; Far Boss, level 40-42, zone unknown, drops 25.0% of kills",
		"Spirit Hood (at level 14; empty slot): from object 900, zone unknown, drop chance unknown",
		"Plain Robe (requires level 8; empty slot): source unknown",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("render lacks %q:\n%s", want, text)
		}
	}
	res, _ = Find(c, synth(t, "unknown"), "Test Holy", Options{Slot: "head"})
	if text := Render(c, res); !strings.Contains(text, "listed by the level needed to wear it: the dataset has no stats for these items.") {
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
		if got := c.r.canUse(c.class, c.sub, c.slot, 60); got != c.want {
			t.Errorf("case %d: canUse = %v", i, got)
		}
	}
	if !allowsClass(-1, 5) || !allowsClass(0, 5) || !allowsClass(1<<4, 5) || allowsClass(1<<3, 5) {
		t.Error("allowsClass")
	}
}

func TestSourceNames(t *testing.T) {
	for _, tc := range []struct {
		src  Source
		want string
	}{
		{Source{Kind: "creature", ID: 7, Name: "Wolf"}, "Wolf, zone unknown, drop chance unknown"},
		{Source{Kind: "creature", ID: 7, Name: "Imp", Everloot: true}, "Imp (name from Everloot), zone unknown, drop chance unknown"},
		{Source{Kind: "creature", ID: 7}, "creature 7, zone unknown, drop chance unknown"},
	} {
		if got := tc.src.String(); got != tc.want {
			t.Errorf("String() = %q, want %q", got, tc.want)
		}
	}
	str := "everloot sourceName"
	for v, want := range map[any]bool{str: true, nil: false, "": false, int64(3): false} {
		if got := FromEverloot(v); got != want {
			t.Errorf("FromEverloot(%v) = %v, want %v", v, got, want)
		}
	}
}

func TestEstimateRequiredLevel(t *testing.T) {
	for ilvl, want := range map[int]int{1: 1, 5: 1, 6: 1, 7: 2, 10: 5, 54: 49} {
		if got := EstimateRequiredLevel(ilvl); got != want {
			t.Errorf("EstimateRequiredLevel(%d) = %d, want %d", ilvl, got, want)
		}
	}
}

// ladder is a dataset for the level, placeholder, proficiency, paired-slot and bonding rules.
func ladder(t *testing.T) store.DataStore {
	t.Helper()
	const mail, trinketSlot, backSlot, waistSlot = 3, 12, 16, 6
	rows := []itemRow{
		it(1, "Low Hood", 8, 3, 2, 4, cloth, headSlot, 5),
		it(2, "Edge Hood", 10, 7, 2, 4, cloth, headSlot, 5),        // level 5 + 2: allowed, at level 7
		it(3, "Far Hood", 11, 8, 2, 4, cloth, headSlot, 5),         // level 8: one past the window
		it(4, "Placeholder Hood", 1, 1, 2, 4, cloth, headSlot, 5),  // ilvl 1
		it(5, "Unknown Hood", 11, 0, 2, 4, cloth, headSlot, 5),     // estimated level 6
		it(6, "Unknown Amulet", 54, 0, 2, 4, 0, neckSlot, 5),       // estimated level 49
		it(7, "Ring A", 12, 4, 2, 4, 0, ringSlot, 5),               //
		it(8, "Ring B", 11, 4, 2, 4, 0, ringSlot, 5),               //
		it(9, "Ring C", 10, 4, 2, 4, 0, ringSlot, 5),               //
		it(10, "Trinket A", 12, 4, 2, 4, 0, trinketSlot, 5),        //
		it(11, "Trinket B", 11, 4, 2, 4, 0, trinketSlot, 5),        //
		it(12, "Field Cloak", 12, 4, 2, 4, 0, backSlot, 5),         // a cloak in the back slot only
		it(13, "Mail Belt", 20, 5, 2, 4, mail, waistSlot, 4),       //
		it(14, "Plate Belt", 40, 5, 2, 4, plate, waistSlot, 4),     //
		it(15, "Leather Belt", 20, 5, 2, 4, leather, waistSlot, 3), //
		it(16, "Quest Hood", 14, 4, 2, 4, cloth, headSlot, 5),      // quest bonding
		it(17, "Artifact Hood", 14, 4, 6, 4, cloth, headSlot, 5),   // artifact quality
		it(18, "Worn Level Hood", 9, 4, 2, 4, cloth, headSlot, 5),  // ilvl 9
		it(20, "Scaling Neck", 40, 1, 2, 4, 0, neckSlot, 5),        // required level 1, item level 40
		it(19, "Cut Word Hood Of The Eagle", 12, 4, 2, 4, cloth, headSlot, 5),
	}
	rows[15].Bonding = 4 // Quest Hood
	return store.New(store.ReaderSource{"items": table(t, rows)})
}

func TestUpgradeRules(t *testing.T) {
	worn := func(slot string, id, ilvl int) map[string]character.EquippedItem {
		return map[string]character.EquippedItem{slot: {Slot: slot, ItemID: ip(id), ItemLevel: ip(ilvl), Quality: ip(2)}}
	}
	cases := []struct {
		name  string
		char  character.Character
		slot  string
		limit int
		want  string
	}{
		// Level.
		{"level 5: window ends at 7, placeholder and bad bonding/quality out", char("PRIEST", 5, nil), "head", 0, "Cut Word Hood Of The Eagle,Unknown Hood,Edge Hood,Worn Level Hood,Low Hood"},
		{"level 5: known level 1 at item level 40 is distrusted", char("PRIEST", 5, nil), "neck", 0, ""},
		{"level 5: estimated level 49 is out", char("PRIEST", 5, nil), "neck", 0, ""},
		{"level 40: estimated 49 is out, estimated 35 is in", char("PRIEST", 40, nil), "neck", 0, "Scaling Neck"},
		{"level 47: estimated level 49 is in", char("PRIEST", 47, nil), "neck", 0, "Unknown Amulet,Scaling Neck"},
		// Not an upgrade.
		{"worn ilvl 10 hides ilvl 10 and below", char("PRIEST", 5, worn("head", 1, 10)), "head", 0, "Cut Word Hood Of The Eagle,Unknown Hood"},
		// Slot.
		{"cloak only in back", char("PRIEST", 20, nil), "back", 0, "Field Cloak"},
		{"cloak not in trinket", char("PRIEST", 20, nil), "trinket1", 0, "Trinket A,Trinket B"},
		// Proficiency by level.
		{"level 5 warrior wears mail", char("WARRIOR", 5, nil), "waist", 0, "Mail Belt"},
		{"level 40 warrior wears plate", char("WARRIOR", 40, nil), "waist", 0, "Plate Belt"},
		{"level 20 hunter wears leather", char("HUNTER", 20, nil), "waist", 0, "Leather Belt"},
		{"level 40 hunter wears mail", char("HUNTER", 40, nil), "waist", 0, "Mail Belt"},
		{"level 40 rogue stays in leather", char("ROGUE", 40, nil), "waist", 0, "Leather Belt"},
		// Paired slots.
		{"finger1 takes the best", char("PRIEST", 5, nil), "finger1", 1, "Ring A"},
		{"finger2 takes the next, not the same", char("PRIEST", 5, nil), "finger2", 1, "Ring B"},
		{"finger2 skips what finger1 wears", char("PRIEST", 5, worn("finger1", 7, 12)), "finger2", 3, "Ring B,Ring C"},
		{"trinket2 takes the next", char("PRIEST", 5, nil), "trinket2", 1, "Trinket B"},
	}
	ds := ladder(t)
	for _, c := range cases {
		res, err := Find(c.char, ds, "", Options{Slot: c.slot, Limit: c.limit})
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		got := res.Slots[c.slot]
		if names(got) != c.want {
			t.Errorf("%s: got %q, want %q", c.name, names(got), c.want)
		}
		for _, cd := range got {
			if cd.Item.ReqLevel > *c.char.Level+EquipWindow || cd.Soon != (cd.Item.ReqLevel > *c.char.Level) {
				t.Errorf("%s: %s req %d soon %v", c.name, cd.Item.Name, cd.Item.ReqLevel, cd.Soon)
			}
		}
	}
}

func TestLevelText(t *testing.T) {
	for _, tc := range []struct {
		c    Candidate
		want string
	}{
		{Candidate{Item: Item{ReqLevel: 5}}, "requires level 5"},
		{Candidate{Item: Item{ReqLevel: 7}, Soon: true}, "at level 7"},
		{Candidate{Item: Item{ReqLevel: 7, Estimated: true}, Soon: true}, "at level 7 (estimated)"},
		{Candidate{Item: Item{ReqLevel: 3, Estimated: true}}, "requires level 3 (estimated)"},
	} {
		if got := tc.c.LevelText(); got != tc.want {
			t.Errorf("LevelText() = %q, want %q", got, tc.want)
		}
	}
}

func TestUnobtainableItemsNeverSuggested(t *testing.T) {
	rows := []itemRow{
		it(1, "DNT HEAD", 40, 1, 2, 4, cloth, headSlot, 5),
		it(2, "Deprecated Battle Chain Pauldrons", 40, 1, 2, 4, cloth, headSlot, 5),
		it(3, "Old Cloth Hood", 12, 1, 2, 4, cloth, headSlot, 5), // a real name that starts with a pattern word
		it(4, "Testament Hood", 11, 1, 2, 4, cloth, headSlot, 5),
	}
	ds := store.New(store.ReaderSource{"items": table(t, rows)})
	res, err := Find(char("PRIEST", 5, nil), ds, "", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got := names(res.Slots["head"]); got != "Old Cloth Hood,Testament Hood" {
		t.Errorf("head candidates = %q, want only the real items", got)
	}
}

// TestUpgradeRankingFollowsPlayStyle: a Strength helm and a Stamina helm of the same level swap places
// between Fury and Protection; the preference moves it too.
func TestUpgradeRankingFollowsPlayStyle(t *testing.T) {
	const mailArmor = 3
	rows := []itemRow{
		it(1, "Brute Helm", 20, 8, 2, 4, mailArmor, headSlot, 4),
		it(2, "Sturdy Helm", 20, 8, 2, 4, mailArmor, headSlot, 7),
	}
	ds := store.New(store.ReaderSource{"items": table(t, rows)})
	top := func(p character.Profile) string {
		c := char("WARRIOR", 10, nil)
		c.Profile = &p
		res, err := Find(c, ds, "", Options{Slot: "head"})
		if err != nil || len(res.Slots["head"]) == 0 {
			t.Fatalf("%v %v", err, res)
		}
		for _, cd := range res.Slots["head"] {
			t.Logf("%s/%s: %s delta %.1f", p.Role, p.Spec, cd.Item.Name, cd.Delta)
		}
		return res.Slots["head"][0].Item.Name
	}
	if got := top(character.Profile{Role: "damage", Focus: "leveling", Spec: "Fury"}); got != "Brute Helm" {
		t.Errorf("Fury: %s", got)
	}
	if got := top(character.Profile{Role: "tank", Focus: "leveling", Spec: "Protection"}); got != "Sturdy Helm" {
		t.Errorf("Protection: %s", got)
	}
	if got := top(character.Profile{Role: "tank", Focus: "leveling", Spec: "Protection", StatPref: "damage"}); got == "" {
		t.Error("preference")
	}
}

// Upgrades above the character's level follow the main list under "Soon (level N)"; they are never
// numbered among what is within reach.
func TestRenderSplitsSoonUpgrades(t *testing.T) {
	lvl := 10
	c := character.Character{Key: "R:Tester", Name: "Tester", Class: "Warrior", Level: &lvl}
	now := Candidate{Item: Item{ID: 1, Name: "Ready Helm", ItemLevel: 9, ReqLevel: 9}, Delta: 2, Basis: "item level"}
	soon := Candidate{Item: Item{ID: 2, Name: "Later Helm", ItemLevel: 14, ReqLevel: 12}, Delta: 6, Basis: "item level", Soon: true}
	soon2 := Candidate{Item: Item{ID: 3, Name: "Later Belt", ItemLevel: 13, ReqLevel: 11}, Delta: 4, Basis: "item level", Soon: true}
	out := Render(c, Result{Spec: "melee", Slots: map[string][]Candidate{"head": {soon, now}, "waist": {soon2}}})
	i, j, k := strings.Index(out, "1. Ready Helm"), strings.Index(out, "Soon (level 11):"), strings.Index(out, "Soon (level 12):")
	if i < 0 || j < i || k < j {
		t.Errorf("order wrong:\n%s", out)
	}
	if strings.Contains(out, "1. Later") || strings.Contains(out, "2. Later") || strings.Contains(out, "waist:\n") {
		t.Errorf("a soon upgrade is ranked among those within reach:\n%s", out)
	}
	if !strings.Contains(out, "head: Later Helm") || !strings.Contains(out, "waist: Later Belt") {
		t.Errorf("soon lines lack their slot:\n%s", out)
	}
}

// Rings are described by the level needed to wear them, never by item level, and a ring above the
// character's level goes under "Soon (level N)", not among those within reach.
func TestRingsGoByRequiredLevelNotItemLevel(t *testing.T) {
	lvl := 10
	c := character.Character{Key: "R:Tester", Name: "Tester", Class: "Warrior", Level: &lvl}
	now := Candidate{Item: Item{ID: 1, Name: "Band of Now", ItemLevel: 20, ReqLevel: 9}, Delta: 5, Basis: "item level"}
	later := Candidate{Item: Item{ID: 2, Name: "Band of Later", ItemLevel: 22, ReqLevel: 12}, Delta: 7, Basis: "item level", Soon: true}
	out := Render(c, Result{Spec: "melee", Slots: map[string][]Candidate{"finger1": {later, now}}})
	if strings.Contains(strings.ToLower(out), "item level") || strings.Contains(out, "ilvl") {
		t.Errorf("item level in the wording:\n%s", out)
	}
	i, j := strings.Index(out, "1. Band of Now"), strings.Index(out, "Soon (level 12):")
	if i < 0 || j < i || !strings.Contains(out, "requires level 9") || strings.Index(out, "Band of Later") < j {
		t.Errorf("rings not split by required level:\n%s", out)
	}
}

// A shield is no upgrade for a two-hand or dual-wield damage build.
func TestShieldLeftOutForADamageBuild(t *testing.T) {
	shield := Item{ID: 9, Name: "Test Shield", Slot: invShield}
	lvl := 10
	dmg := character.Character{Class: "WARRIOR", Level: &lvl, Profile: &character.Profile{Role: "damage", Focus: "leveling", Spec: "Fury"}}
	if lo := loadoutOf(dmg, nil, "Fury"); lo.allows("offhand", shield) {
		t.Error("a Fury warrior is offered a shield")
	}
	tank := character.Character{Class: "WARRIOR", Level: &lvl, Profile: &character.Profile{Role: "tank", Focus: "dungeons", Spec: "Protection"}}
	if lo := loadoutOf(tank, nil, "Protection"); !lo.allows("offhand", shield) {
		t.Error("a Protection warrior is refused a shield")
	}
}

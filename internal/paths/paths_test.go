package paths

import (
	"bytes"
	"regexp"
	"strings"
	"testing"

	"github.com/parquet-go/parquet-go"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/questie"
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
	SellPrice     int64   `parquet:"sell_price"`
}

type lootRow struct {
	ItemID int64   `parquet:"item_id"`
	Kind   string  `parquet:"source_kind"`
	ID     int64   `parquet:"source_id"`
	Kills  int32   `parquet:"kills"`
	Drops  int32   `parquet:"drops"`
	Tier   *string `parquet:"tier,optional"`
}

type npcRow struct {
	ID       int64  `parquet:"id"`
	Name     string `parquet:"name"`
	MinLevel int32  `parquet:"min_level"`
	MaxLevel int32  `parquet:"max_level"`
	Zone     string `parquet:"zone_name"`
}

type questRow struct {
	ID    int64  `parquet:"id"`
	Name  string `parquet:"name"`
	Level int32  `parquet:"level"`
	Zone  string `parquet:"zone"`
}

type recipeRow struct {
	SpellID int64  `parquet:"spell_id"`
	Name    string `parquet:"name"`
	Line    string `parquet:"skill_line_name"`
	Learn   int16  `parquet:"learn_skill"`
	Creates int64  `parquet:"creates_item_id"`
}

type reagentRow struct {
	SpellID int64 `parquet:"spell_id"`
	Item    int64 `parquet:"reagent_item_id"`
	Count   int16 `parquet:"count"`
}

type instRow struct {
	MapID    int32  `parquet:"map_id"`
	Name     string `parquet:"name"`
	Type     string `parquet:"instance_type"`
	MinLevel int16  `parquet:"min_level"`
	MaxLevel int16  `parquet:"max_level"`
}

type bossRow struct {
	MapID      int32  `parquet:"map_id"`
	Name       string `parquet:"name"`
	CreatureID int32  `parquet:"creature_id"`
}

type objRow struct {
	ID   int64  `parquet:"id"`
	Name string `parquet:"name"`
	Zone string `parquet:"zone_name"`
}

func tbl[T any](t *testing.T, rows []T) store.SizedReaderAt {
	t.Helper()
	var buf bytes.Buffer
	if err := parquet.Write(&buf, rows); err != nil {
		t.Fatal(err)
	}
	return store.SizedReaderAt{ReaderAt: bytes.NewReader(buf.Bytes()), Size: int64(buf.Len())}
}

const (
	shawl, vest, bracers, belt, helm, bar, ore, blade, sword = 1, 2, 3, 4, 5, 10, 11, 6, 7
	mail, cloak                                              = 3, 1
	unsourced                                                = 12
)

func item(id int64, name string, ilvl, req, quality, subclass, slot, bonding int32) itemRow {
	class := int32(4)
	if slot == 17 {
		class = 2
	}
	return itemRow{ID: id, Name: name, ItemLevel: ilvl, RequiredLevel: req, Quality: quality, ClassID: class, SubclassID: subclass, Slot: slot, StatsStatus: "none", Bonding: bonding, SellPrice: 9999}
}

func tier(s string) *string { return &s }

// fixture is a small world around a level 10 warrior who mines and smiths.
func fixture(t *testing.T) store.DataStore {
	var world []lootRow
	for i := int64(0); i < 7; i++ { // the belt: seven creatures, each under 5%
		world = append(world, lootRow{belt, "creature", 700 + i, 100, 3, tier("verified")})
	}
	return store.New(store.ReaderSource{
		"items": tbl(t, []itemRow{
			item(shawl, "Traveler's Shawl", 23, 8, 2, 0, 16, 0),
			item(vest, "Copper Chain Vest", 17, 8, 2, mail, 5, 0),
			item(bracers, "Tough Bracers", 20, 8, 2, mail, 9, 2),
			item(belt, "Wolf Fang Belt", 19, 8, 3, mail, 6, 1),
			item(helm, "Boss Helm", 22, 9, 3, mail, 1, 1),
			item(bar, "Copper Bar", 1, 0, 1, 0, 0, 0),
			item(ore, "Copper Ore", 1, 0, 1, 0, 0, 0),
			item(blade, "Fang Blade", 20, 8, 2, 1, 17, 1),
			item(sword, "Mob's Pride", 21, 8, 2, mail, 8, 1),
			item(unsourced, "Rank Gauntlets", 23, 8, 3, mail, 10, 1),
		}),
		"loot_sources": tbl(t, append(world, []lootRow{
			{shawl, "quest", 500, 0, 0, nil},
			{bracers, "creature", 701, 100, 2, nil},
			{helm, "creature", 600, 100, 20, tier("verified")},
			{ore, "object", 900, 0, 0, nil},
			{blade, "creature", 601, 100, 40, tier("verified")},
			{sword, "creature", 602, 100, 12, tier("verified")},
		}...)),
		"npcs":            tbl(t, []npcRow{{600, "Hogger Prime", 12, 12, "Dunemark"}, {601, "Rank Wolf", 9, 11, "Dunemark"}, {602, "Far Wolf", 40, 42, "Faraway"}}),
		"quests":          tbl(t, []questRow{{500, "Lost Shawl", 9, "Dunemark"}}),
		"recipes":         tbl(t, []recipeRow{{800, "Copper Chain Vest", "Blacksmithing", 5, vest}}),
		"recipe_reagents": tbl(t, []reagentRow{{800, bar, 6}, {800, ore, 4}}),
		"instances":       tbl(t, []instRow{{30, "Wolf Den", "dungeon", 8, 14}}),
		"instance_bosses": tbl(t, []bossRow{{30, "Hogger Prime", 600}}),
		"objects":         tbl(t, []objRow{{900, "Copper Vein", "Dunemark"}}),
	})
}

func warrior() character.Character {
	lvl, mining, smith := 10, 40, 45
	return character.Character{
		Name: "Testwarrior", Class: "WARRIOR", Level: &lvl,
		Professions: []character.Skill{{Name: "Mining", Rank: &mining}, {Name: "Blacksmithing", Rank: &smith}},
		Bags:        []character.ItemStack{{ItemID: bar, Count: 2, Name: "Copper Bar"}},
		Bank:        []character.ItemStack{{ItemID: bar, Count: 1, Name: "Copper Bar"}},
		Quests:      []character.Quest{{QuestID: 500, Title: "Lost Shawl", Zone: "Dunemark"}},
	}
}

var digits = regexp.MustCompile(`\d{3,}`)

func kinds(r Result) string {
	var k []string
	for _, p := range r.Paths {
		k = append(k, p.Kind)
	}
	return strings.Join(k, ",")
}

func TestEachPathKindFoundAndRanked(t *testing.T) {
	ds, c := fixture(t), warrior()
	cases := []struct {
		name string
		id   int64
		kind string
		want string
	}{
		{"quest", shawl, Quest, "Finish Lost Shawl for it (it's in your quest log, in Dunemark)"},
		{"craft", vest, Craft, "Learn it from a Blacksmithing trainer (if you haven't); have 3/6 Copper Bar"},
		{"auction", bracers, Auction, "Common world drop. Usually cheap on the Auction House."},
		{"dungeon", helm, Dungeon, "Drops often from Hogger Prime in Wolf Den"},
		{"creature", blade, Creature, "Drops often from Rank Wolf in Dunemark"},
	}
	for _, tc := range cases {
		r := For(c, ds, tc.id, Options{})
		if len(r.Paths) == 0 || r.Paths[0].Kind != tc.kind && !(tc.kind == Creature && kinds(r) == "creature") {
			t.Errorf("%s: kinds %q", tc.name, kinds(r))
			continue
		}
		if !strings.Contains(r.Paths[0].Line, tc.want) {
			t.Errorf("%s: line %q lacks %q", tc.name, r.Paths[0].Line, tc.want)
		}
	}
}

func TestQuestOutranksOthers(t *testing.T) {
	ds, c := fixture(t), warrior()
	// The shawl has a quest and is not a world drop or Bind on Equip: the quest is its only way.
	if r := For(c, ds, shawl, Options{}); kinds(r) != "quest" {
		t.Errorf("shawl kinds = %q", kinds(r))
	}
	// A bind-on-pickup boss drop at 20%: the dungeon, then the creature itself.
	if r := For(c, ds, helm, Options{}); kinds(r) != "dungeon,creature" {
		t.Errorf("BoP boss item kinds = %q", kinds(r))
	}
}

// An item the dataset lists no source for is not called a random drop: the answer says the data cannot place it.
func TestUnsourcedItemIsNotCalledNotWorthChasing(t *testing.T) {
	r := For(warrior(), fixture(t), unsourced, Options{})
	if r.Dependable() || !strings.Contains(r.Reason, "lists no source") || strings.Contains(r.Reason, "worth chasing") {
		t.Errorf("unsourced item: %+v", r)
	}
}

func TestWorldDropsAreNotDependable(t *testing.T) {
	ds, c := fixture(t), warrior()
	r := For(c, ds, belt, Options{}) // BoP, seven creatures, all under 5%
	if r.Dependable() || !strings.Contains(r.Reason, "not worth chasing") {
		t.Errorf("bind-on-pickup world drop: %+v", r)
	}
	if r := For(c, ds, sword, Options{}); r.Dependable() { // one creature, but far above the level range
		t.Errorf("out-of-range creature: %q", kinds(r))
	}
}

func TestCraftHaveNeedFromBagsAndBank(t *testing.T) {
	r := For(warrior(), fixture(t), vest, Options{})
	p := r.Paths[0]
	if p.Kind != Craft || len(p.Reagents) != 2 {
		t.Fatalf("craft path: %+v", p)
	}
	byName := map[string]Reagent{}
	for _, g := range p.Reagents {
		byName[g.Name] = g
	}
	if g := byName["Copper Bar"]; g.Need != 6 || g.Have != 3 || g.InBags != 2 || g.InBank != 1 || g.Missing != 3 || !strings.Contains(g.How, "Auction House") {
		t.Errorf("Copper Bar: %+v", g)
	}
	if g := byName["Copper Ore"]; g.Need != 4 || g.Have != 0 || !strings.Contains(g.How, "gather it with your Mining: Copper Vein in Dunemark") {
		t.Errorf("Copper Ore: %+v", g)
	}
	// A character without the profession gets no craft path.
	c := warrior()
	c.Professions = c.Professions[:1]
	if r := For(c, fixture(t), vest, Options{}); r.Dependable() {
		t.Errorf("craft without the profession counts as a way: %q", kinds(r))
	}
}

func TestAuctionHouseOnlyForTradeableAndNeverVendorPrice(t *testing.T) {
	ds, c := fixture(t), warrior()
	for _, id := range []int64{belt, helm, blade} { // bind on pickup
		for _, p := range For(c, ds, id, Options{}).Paths {
			if p.Kind == Auction {
				t.Errorf("item %d is Bind on Pickup but has an Auction House path", id)
			}
		}
	}
	// Every fixture item has a vendor sell price of 9999 copper: it is never shown as a price.
	r := For(c, ds, bracers, Options{})
	if got := r.Paths[0].Line; strings.Contains(got, "about") || strings.Contains(got, "99") {
		t.Errorf("vendor price shown: %q", got)
	}
	if Tradeable(1, 3) || Tradeable(4, 1) || Tradeable(5, 1) || !Tradeable(0, 0) || !Tradeable(2, 1) {
		t.Error("Tradeable")
	}
}

func TestNoIdsInPlayerWords(t *testing.T) {
	ds, c := fixture(t), warrior()
	for _, id := range []int64{shawl, vest, bracers, belt, helm, blade} {
		r := For(c, ds, id, Options{})
		if m := digits.FindString(r.Text()); m != "" {
			t.Errorf("item %d text holds digits %q: %s", id, m, r.Text())
		}
	}
}

func TestInSlotSkipsItemsWithoutAWay(t *testing.T) {
	got, err := InSlot(warrior(), fixture(t), "", "waist", Options{}, 3)
	if err != nil || len(got) != 0 {
		t.Errorf("waist (only the world-drop belt): %v %v", got, err)
	}
	got, err = InSlot(warrior(), fixture(t), "", "back", Options{}, 3)
	if err != nil || len(got) != 1 || got[0].Result.Item != "Traveler's Shawl" {
		t.Errorf("back: %v %v", got, err)
	}
}

// A reward of a quest the player has not picked up is a way to get the item, and the answer names the
// quest, taking its name from QuestieDB when the dataset has no row for it.
func TestQuestRewardNamesItsQuestFromQuestie(t *testing.T) {
	const shield, mace = 40, 41
	ds := store.New(store.ReaderSource{
		"items": tbl(t, []itemRow{
			item(shield, "Oak Bulwark", 14, 9, 3, 6, 14, 0),
			item(mace, "Oak Skybreaker Mace", 14, 9, 3, 4, 13, 0),
		}),
		"loot_sources": tbl(t, []lootRow{{shield, "quest", 777, 0, 0, nil}, {mace, "quest", 777, 0, 0, nil}}),
	})
	qx := &questie.Index{Quests: map[int]*questie.Quest{777: {ID: 777, Name: "The Oak Skybreaker Quest"}}}
	lvl := 10
	fury := character.Character{Name: "T", Class: "WARRIOR", Level: &lvl, Profile: &character.Profile{Role: "damage", Focus: "leveling", Spec: "Fury"}}
	r := For(fury, ds, mace, Options{Questie: qx})
	if len(r.Paths) == 0 || r.Paths[0].Kind != Quest || !strings.Contains(r.Paths[0].Line, "The Oak Skybreaker Quest") {
		t.Errorf("a quest reward is named with its quest: %+v", r)
	}
	if r := For(fury, ds, mace, Options{}); r.Dependable() {
		t.Errorf("without a name for the quest nothing is claimed: %+v", r)
	}
	// A shield is no use to a Fury warrior: it is left out entirely, not marked "not worth chasing".
	if r := For(fury, ds, shield, Options{Questie: qx}); !r.Unfit || len(r.Paths) != 0 || r.Dependable() {
		t.Errorf("a shield for a Fury warrior: %+v", r)
	}
	prot := fury
	prot.Profile = &character.Profile{Role: "tank", Focus: "dungeons", Spec: "Protection"}
	if r := For(prot, ds, shield, Options{Questie: qx}); r.Unfit || !r.Dependable() {
		t.Errorf("a shield for a Protection warrior: %+v", r)
	}
}

type observedRow struct {
	QuestID  int64   `parquet:"quest_id"`
	Title    string  `parquet:"title"`
	Level    *int16  `parquet:"level"`
	Faction  *string `parquet:"faction"`
	Category *string `parquet:"category"`
}

// The client's quests table holds no quest name, so a quest the dataset knows only from quests_observed is still named,
// placed by its zone heading and worded as a choice; a quest of the other side's or far above the level is left out.
func TestQuestRewardNamesItsQuestFromObserved(t *testing.T) {
	const a, b, c, d = 50, 51, 52, 53
	s := func(v string) *string { return &v }
	l := func(v int16) *int16 { return &v }
	ds := store.New(store.ReaderSource{
		"items": tbl(t, []itemRow{
			item(a, "Oak Girdle", 14, 9, 2, 3, 6, 0),
			item(b, "Oak Gloves", 14, 9, 2, 3, 10, 0),
			item(c, "Oak Boots", 14, 9, 2, 3, 8, 0),
			item(d, "Oak Cap", 14, 9, 2, 3, 1, 0),
		}),
		"loot_sources": tbl(t, []lootRow{{a, "quest reward", 801, 0, 0, nil}, {b, "quest reward", 801, 0, 0, nil}, {c, "quest reward", 802, 0, 0, nil}, {d, "quest reward", 803, 0, 0, nil}}),
		"quests_observed": tbl(t, []observedRow{
			{801, "Synthetic Errand", l(10), s("horde"), s("Synthetic Vale")},
			{802, "Synthetic Alliance Errand", l(10), s("alliance"), nil},
			{803, "Synthetic High Errand", l(40), s("horde"), nil},
		}),
	})
	lvl := 10
	horde := character.Character{Name: "T", Class: "WARRIOR", Faction: "Horde", Level: &lvl}
	r := For(horde, ds, a, Options{})
	if len(r.Paths) == 0 || r.Paths[0].Line != "Finish Synthetic Errand and choose it (in Synthetic Vale)" {
		t.Errorf("a quest the dataset knows only from players is named: %+v", r)
	}
	for _, id := range []int64{c, d} {
		if r := For(horde, ds, id, Options{}); r.Dependable() {
			t.Errorf("item %d: a quest of the other side's or far above the level is no way: %+v", id, r)
		}
	}
}

// A node the dataset has no row for is a bare id: with its skill unknown it must not pass for one any skill can take.
func TestPlacedNodesDropBareObjectIDs(t *testing.T) {
	got := placedNodes([]node{{id: 1}, {id: 2, skill: 65}, {id: 3, zone: "Dunemark"}, {id: 4, name: "Tin Vein"}})
	var ids []int64
	for _, n := range got {
		ids = append(ids, n.id)
	}
	if len(ids) != 3 || ids[0] != 2 || ids[1] != 3 || ids[2] != 4 {
		t.Errorf("kept nodes %v, want 2, 3 and 4", ids)
	}
}

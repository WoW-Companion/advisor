package dungeons

import (
	"bytes"
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

type instRow struct {
	MapID      int32  `parquet:"map_id"`
	Name       string `parquet:"name"`
	Type       string `parquet:"instance_type"`
	MaxPlayers *int16 `parquet:"max_players,optional"`
	MinLevel   *int16 `parquet:"min_level,optional"`
	MaxLevel   *int16 `parquet:"max_level,optional"`
	Source     string `parquet:"level_source"`
	Zone       string `parquet:"entrance_zone_name"`
}

type iqRow struct {
	MapID   int32  `parquet:"map_id"`
	QuestID int64  `parquet:"quest_id"`
	Reason  string `parquet:"reason"`
}

type bossRow struct {
	MapID      int32  `parquet:"map_id"`
	Name       string `parquet:"name"`
	CreatureID *int32 `parquet:"creature_id,optional"`
}

type questRow struct {
	ID       int64   `parquet:"id"`
	MapID    *int32  `parquet:"map_id,optional"`
	ZoneName *string `parquet:"zone_name,optional"`
	Name     string  `parquet:"name"`
	Level    int32   `parquet:"level"`
	Giver    string  `parquet:"giver_name"`
	Faction  string  `parquet:"faction"`
}

func table[T any](t *testing.T, rows []T) store.SizedReaderAt {
	t.Helper()
	var buf bytes.Buffer
	if err := parquet.Write(&buf, rows); err != nil {
		t.Fatal(err)
	}
	return store.SizedReaderAt{ReaderAt: bytes.NewReader(buf.Bytes()), Size: int64(buf.Len())}
}

func i16(n int16) *int16 { return &n }
func i32(n int32) *int32 { return &n }
func ip(n int) *int      { return &n }

const (
	cloth, leather = 1, 2
	head, ring     = 1, 11
)

func it(id int64, name string, ilvl, req int32, sub, slot int32, stats ...int16) itemRow {
	return itemRow{id, name, ilvl, req, 2, 4, sub, slot, -1, stats, "known"}
}

const rogueOnly = 1 << (4 - 1)

// synth: Deadmines (15-21, 5 players), Wailing Caverns (17-23), Shadowfang Keep (24-30), Mystery
// Hold (no levels), Far Keep (40-45) and a raid. Creatures: 100 Boss A and 101 Boss B (Deadmines),
// 102 Boss C (Wailing), 103 Boss D (Mystery). Items: a cloth hood and a leather cap (heads), a
// ring for anyone and a rogue-only ring, and a hood too weak to be an upgrade.
func synth(t *testing.T) store.DataStore {
	stab := it(4, "Ring of Stab", 22, 15, 0, ring, 3)
	stab.ClassMask = rogueOnly
	return store.New(store.ReaderSource{
		"items": table(t, []itemRow{
			it(1, "Mind Hood", 25, 15, cloth, head, 5, 7),
			it(2, "Fang Cap", 25, 15, leather, head, 3, 7),
			it(3, "Ring of Mind", 22, 15, 0, ring, 5),
			stab,
			it(5, "Old Hood", 5, 1, cloth, head, 5, 7),
		}),
		"loot_sources": table(t, []lootRow{
			{1, "creature", 100, 100, 20}, {3, "creature", 101, 50, 10}, {4, "creature", 101, 50, 5},
			{3, "creature", 102, 100, 1}, {5, "creature", 100, 10, 5}, {1, "creature", 103, 0, 0},
			{2, "creature", 102, 100, 30}, {1, "object", 100, 0, 0},
		}),
		"instances": table(t, []instRow{
			{1, "Deadmines", "dungeon", i16(5), i16(15), i16(21), "client", "Westfall"},
			{2, "Wailing Caverns", "dungeon", i16(5), i16(17), i16(23), "curated", "The Barrens"},
			{3, "Shadowfang Keep", "dungeon", nil, i16(24), i16(30), "curated", ""},
			{4, "Mystery Hold", "dungeon", nil, nil, nil, "", ""},
			{5, "Far Keep", "dungeon", nil, i16(40), i16(45), "curated", ""},
			{6, "Big Raid", "raid", nil, i16(18), i16(20), "curated", ""},
			{7, "Ember Chasm", "dungeon", i16(5), i16(5), i16(10), "curated", "Orgrimmar"},
			{8, "Stone Stockade", "dungeon", i16(5), i16(30), i16(34), "curated", "Stormwind City"},
		}),
		"instance_quests": table(t, []iqRow{{2, 950, "category"}, {2, 951, "category"}, {2, 952, "category"}, {7, 953, "category"}}),
		"instance_bosses": table(t, []bossRow{
			{1, "Boss A", i32(100)}, {1, "Boss B", i32(101)}, {1, "Lost Boss", nil},
			{2, "Boss C", i32(102)}, {4, "Boss D", i32(103)},
		}),
		"quests": table(t, []questRow{{900, i32(1), nil, "", 0, "", ""}, {901, nil, &[]string{"wailing caverns"}[0], "", 0, "", ""}, {902, i32(9), nil, "", 0, "", ""},
			{950, nil, nil, "Wailing Errand", 18, "Nalpak", ""}, {951, nil, nil, "Horde Errand", 14, "Grol", "Horde"}, {952, nil, nil, "Far Errand", 40, "Gone", ""}, {953, nil, nil, "Ember Errand", 5, "Rask", "Horde"}}),
	})
}

func qst(id int, zone string, done bool) character.Quest {
	return character.Quest{QuestID: id, Title: "Quest " + zone, Zone: zone, Complete: &done}
}

// worn is a head and two rings, all item level 20 and quality 2, so the 25 hoods beat them by 5
// and the 22 rings by 2.
func worn() map[string]character.EquippedItem {
	e := character.EquippedItem{ItemLevel: ip(20), Quality: ip(2)}
	return map[string]character.EquippedItem{"head": e, "finger1": e, "finger2": e}
}

func priest(level int) character.Character {
	return character.Character{Name: "Testpriest", Class: "PRIEST", Level: ip(level), Equipment: worn(), Talents: &character.Talents{Trees: []character.TalentTree{{
		Groups: []character.TalentGroup{{Name: "Shadow Magic", Spent: 5}, {Name: "Discipline"}},
	}}}}
}

func rogue(level int) character.Character {
	return character.Character{Name: "Testrogue", Class: "ROGUE", Level: ip(level), Equipment: worn()}
}

func names(ps []DungeonPick) string {
	var n []string
	for _, p := range ps {
		n = append(n, p.Name)
	}
	return strings.Join(n, ",")
}

func TestPlanPriest(t *testing.T) {
	c := priest(20)
	c.Quests = []character.Quest{qst(900, "", false), qst(901, "", false), qst(902, "", false), qst(903, "Deadmines", false), qst(904, "Deadmines", true)}
	r, err := Plan(c, synth(t))
	if err != nil {
		t.Fatal(err)
	}
	// Deadmines: Mind Hood (Boss A) and Ring of Mind (Boss B); the rogue ring and the weak hood
	// are not upgrades. Wailing: the ring at 1%, the leather cap is not priest gear. Shadowfang is
	// 4 levels away.
	if got := names(r.Picks); got != "Deadmines,Wailing Caverns" {
		t.Fatalf("ranked picks = %s", got)
	}
	dm, wc := r.Picks[0], r.Picks[1]
	if dm.Upgrades != 2 || wc.Upgrades != 1 || dm.Fit != 0 || wc.Fit != 0 {
		t.Errorf("deadmines %+v wailing %+v", dm, wc)
	}
	if len(dm.Drops) != 2 || dm.Drops[0].Item != "Mind Hood" || dm.Drops[0].Boss != "Boss A" || *dm.Drops[0].Chance != 20 || dm.Drops[1].Item != "Ring of Mind" || *dm.Drops[1].Chance != 20 {
		t.Errorf("deadmines drops %+v", dm.Drops)
	}
	if got := strings.Join(dm.UnlinkedBosses, ","); got != "Lost Boss" {
		t.Errorf("unlinked = %q", got)
	}
	// 900 is tied by map, 903 by zone name; 904 is done, 902 is on another map.
	if len(dm.Quests) != 2 || dm.Quests[0].ID != 900 || dm.Quests[1].ID != 903 || len(wc.Quests) != 1 || wc.Quests[0].ID != 901 {
		t.Errorf("quests dm %+v wc %+v", dm.Quests, wc.Quests)
	}
	if got := names(r.Unknown); got != "Mystery Hold" || !r.Unknown[0].LevelUnknown || r.Unknown[0].Upgrades != 1 || r.Unknown[0].Drops[0].Chance != nil {
		t.Errorf("unknown = %+v", r.Unknown)
	}
	text := Render(c, r, 1)
	for _, want := range []string{"1. Deadmines (levels 15\u201321; entrance in Westfall; 5 players)", "Mind Hood (head, +5.0) from Boss A, drops 20% of kills",
		"Drops not known yet: Lost Boss", "In your log: Quest \n", "1 more dungeon(s) fit.", "Level not known yet: Mystery Hold"} {
		if !strings.Contains(text, want) {
			t.Errorf("render missing %q in:\n%s", want, text)
		}
	}
	if strings.Contains(text, "Wailing") {
		t.Errorf("limit 1 listed Wailing:\n%s", text)
	}
}

func TestPlanRogueRanking(t *testing.T) {
	// Level 25: Wailing (17-23, 2 below) beats Shadowfang (24-30, no loot) on upgrades; Deadmines
	// is 4 below and out.
	r, err := Plan(rogue(25), synth(t))
	if err != nil {
		t.Fatal(err)
	}
	if got := names(r.Picks); got != "Wailing Caverns,Shadowfang Keep" {
		t.Fatalf("picks = %s", got)
	}
	if wc := r.Picks[0]; wc.Fit != 2 || wc.Upgrades != 2 || wc.Drops[0].Item != "Fang Cap" {
		t.Errorf("wailing = %+v", wc)
	}
	if sf := r.Picks[1]; sf.Fit != 0 || sf.Upgrades != 0 || len(sf.Drops) != 0 {
		t.Errorf("shadowfang = %+v", sf)
	}
	// Level 22: Deadmines (the two rings) and Wailing (cap and ring) tie on 2 upgrades; Wailing
	// covers 22, Deadmines is 1 below, so level fit puts Wailing first despite its higher map id.
	// Shadowfang opens at 24, so it is not offered yet.
	r, err = Plan(rogue(22), synth(t))
	if err != nil {
		t.Fatal(err)
	}
	if got := names(r.Picks); got != "Wailing Caverns,Deadmines" || r.Picks[1].Fit != 1 {
		t.Errorf("level 22 picks = %s %+v", got, r.Picks)
	}
}

func TestPlanErrors(t *testing.T) {
	if _, err := Plan(priest(20), nil); err == nil {
		t.Error("nil store: want error")
	}
	c := priest(20)
	c.Level = nil
	if _, err := Plan(c, synth(t)); err == nil {
		t.Error("no level: want error")
	}
	if _, err := Plan(priest(20), store.New(store.ReaderSource{"items": table(t, []itemRow{it(1, "H", 20, 10, cloth, head, 5)})})); err == nil || !strings.Contains(err.Error(), "instances") {
		t.Errorf("no instances table: err = %v", err)
	}
	r, err := Plan(priest(60), synth(t))
	if err != nil || len(r.Picks) != 0 || !strings.Contains(Render(priest(60), r, 3), "No dungeon suits") {
		t.Errorf("level 60: %+v, %v", r, err)
	}
}

func warrior(level int, faction string) character.Character {
	return character.Character{Name: "Testwar", Class: "WARRIOR", Faction: faction, Level: ip(level), Equipment: worn()}
}

func TestBelowEveryDungeon(t *testing.T) {
	c := warrior(3, "Horde")
	c.Quests = []character.Quest{qst(1, "Durotar", false)}
	r, err := Plan(c, synth(t))
	if err != nil {
		t.Fatal(err)
	}
	if r.First == nil || r.First.Name != "Ember Chasm" || r.LevelsToGo != 2 || len(r.Picks) != 0 {
		t.Fatalf("result %+v", r)
	}
	text := Render(c, r, 3)
	for _, want := range []string{"Too low for dungeons yet. The first one is Ember Chasm (about levels 5\u201310), in Orgrimmar: 2 more level(s) to go.", "Meanwhile: finish your quest Quest Durotar", "Ember Errand (from Rask)"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
}

func TestInRangePickupsAndDrops(t *testing.T) {
	c := priest(20)
	c.Faction = "Horde"
	r, err := Plan(c, synth(t))
	if err != nil {
		t.Fatal(err)
	}
	var wc DungeonPick
	for _, p := range r.Picks {
		if p.Name == "Wailing Caverns" {
			wc = p
		}
	}
	// 950 is open to anyone and in reach; 951 is Horde; 952 is far above the character.
	if len(wc.Pickups) != 2 || wc.Pickups[0].Title != "Wailing Errand" || wc.Pickups[0].Giver != "Nalpak" || wc.Pickups[1].ID != 951 {
		t.Fatalf("pickups %+v", wc.Pickups)
	}
	text := Render(c, r, 3)
	for _, want := range []string{"Wailing Caverns (about levels 17\u201323; entrance in The Barrens", "Wailing Errand (from Nalpak)", "Ring of Mind (finger1, +2.0) from Boss C"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
	if strings.Contains(text, "Far Errand") || strings.Contains(text, "Too low") {
		t.Errorf("unexpected text:\n%s", text)
	}
}

func TestFactionFiltering(t *testing.T) {
	r, err := Plan(warrior(10, "Alliance"), synth(t))
	if err != nil {
		t.Fatal(err)
	}
	if r.First == nil || r.First.Name != "Deadmines" {
		t.Errorf("alliance first = %+v", r.First)
	}
	r, err = Plan(warrior(10, "Horde"), synth(t))
	if err != nil || names(r.Picks) != "Ember Chasm" || r.First != nil {
		t.Errorf("horde picks = %s (%v)", names(r.Picks), err)
	}
	// Plan keeps the Alliance dungeon out of a Horde plan and the reverse, by entrance capital.
	r, _ = Plan(warrior(32, "Horde"), synth(t))
	if names(r.Picks) != "Shadowfang Keep" {
		t.Errorf("horde 32 picks = %s", names(r.Picks))
	}
}

func TestPlainWording(t *testing.T) {
	forbidden := []string{"data doesn't", "dataset", "general game knowledge", "missing from", "not authoritative", "advisor default", "not linked"}
	for _, c := range []character.Character{warrior(3, "Horde"), warrior(10, "Horde"), priest(20), rogue(25), priest(60)} {
		r, err := Plan(c, synth(t))
		if err != nil {
			t.Fatal(err)
		}
		text := strings.ToLower(Render(c, r, 3))
		for _, f := range forbidden {
			if strings.Contains(text, f) {
				t.Errorf("%s level %d text has %q:\n%s", c.Name, *c.Level, f, text)
			}
		}
	}
}

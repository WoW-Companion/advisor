package dungeons

import (
	"bytes"
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
			{3, "Shadowfang Keep", "dungeon", i16(5), i16(24), i16(30), "curated", ""},
			{4, "Mystery Hold", "dungeon", i16(5), nil, nil, "", ""},
			{5, "Far Keep", "dungeon", i16(5), i16(40), i16(45), "curated", ""},
			{6, "Big Raid", "raid", nil, i16(18), i16(20), "curated", ""},
			{7, "Ember Chasm", "dungeon", i16(5), i16(5), i16(10), "curated", "Orgrimmar"},
			{8, "Stone Stockade", "dungeon", i16(5), i16(30), i16(34), "curated", "Stormwind City"},
		}),
		"instance_quests": table(t, []iqRow{{2, 950, "category"}, {2, 951, "category"}, {2, 952, "category"}, {7, 953, "category"}}),
		"instance_bosses": table(t, []bossRow{
			{1, "Boss A", i32(100)}, {1, "Boss B", i32(101)}, {1, "Lost Boss", nil},
			{1, "Boss A", i32(100)}, {1, "lost boss", nil}, // one encounter row per difficulty repeats a name
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
		"Drops not known yet: Lost Boss", "In your log: Quest \n", "1 more dungeon fits.", "Level not known yet: Mystery Hold"} {
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
	for _, want := range []string{"Too low for dungeons yet. The first one is Ember Chasm (about levels 5\u201310), in Orgrimmar: 2 more levels to go.", "Meanwhile: finish your quest Quest Durotar", "Ember Errand (from Rask)"} {
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

// questieIndex is a synthetic QuestieDB: Shadowfang Keep (the dataset has no entrance zone for it)
// with a boss that drops the Mind Hood, and Deadmines, whose entrance zone the dataset already has.
func questieIndex() *questie.Index {
	return &questie.Index{
		Areas: map[int]questie.Area{901: {Zone: "Silverpine Test"}, 902: {Zone: "Elsewhere"}},
		Npcs: map[int]questie.Entity{
			500: {Name: "Chief Fang", Rank: 3}, 501: {Name: "Fang Guard", Rank: 1}, 502: {Name: "Fang Bat"},
			510: {Name: "Mine Boss", Rank: 3},
		},
		Drops:  map[int][]int{500: {1}, 510: {1, 3}},
		Quests: map[int]*questie.Quest{70: {ID: 70, Name: "Fang Errand", Level: 25, Starters: []questie.Ref{{ID: 600}}}},
		Dungeons: []questie.Dungeon{
			{Area: 910, Name: "Shadowfang Keep", Parent: 901, X: 52.6, Y: 49, Entrance: true, Creatures: []int{500, 501, 502}, Quests: []int{70}},
			{Area: 911, Name: "Deadmines", Parent: 902, X: 1, Y: 2, Entrance: true, Creatures: []int{510}},
		},
	}
}

func TestPlanWithQuestie(t *testing.T) {
	c := priest(24)
	r, err := PlanWith(c, synth(t), questieIndex())
	if err != nil {
		t.Fatal(err)
	}
	var sk, dm *DungeonPick
	for i := range r.Picks {
		switch r.Picks[i].Name {
		case "Shadowfang Keep":
			sk = &r.Picks[i]
		case "Deadmines":
			dm = &r.Picks[i]
		}
	}
	if sk == nil {
		t.Fatalf("no Shadowfang pick: %s", names(r.Picks))
	}
	if sk.EntranceZone != "Silverpine Test" || !sk.HasEntranceXY || sk.EntranceX != 52.6 || sk.EntranceY != 49 || !sk.EntranceFromQuestie {
		t.Errorf("entrance %+v", sk)
	}
	if len(sk.Inside) != 3 || !sk.Inside[0].Boss || !sk.Inside[1].Elite {
		t.Errorf("inside %+v", sk.Inside)
	}
	if len(sk.Drops) != 1 || sk.Drops[0].Item != "Mind Hood" || sk.Drops[0].Boss != "Chief Fang" || !sk.Drops[0].Questie || sk.Upgrades != 1 {
		t.Errorf("drops %+v", sk.Drops)
	}
	if len(sk.Pickups) != 1 || sk.Pickups[0].Title != "Fang Errand" || !sk.Pickups[0].Questie {
		t.Errorf("pickups %+v", sk.Pickups)
	}
	text := Render(c, r, 5)
	for _, want := range []string{"entrance in Silverpine Test (52.6, 49.0)", "Inside (from your Questie database): bosses Chief Fang", "(from your Questie database)", "Fang Errand"} {
		if !strings.Contains(text, want) {
			t.Errorf("render missing %q in:\n%s", want, text)
		}
	}
	// The dataset wins: Deadmines keeps its own zone, no coordinates from another zone, and its
	// Mind Hood stays the dataset's drop from Boss A.
	if dm == nil || dm.EntranceZone != "Westfall" || dm.HasEntranceXY || dm.EntranceFromQuestie {
		t.Fatalf("deadmines %+v", dm)
	}
	for _, d := range dm.Drops {
		if d.Item == "Mind Hood" && (d.Questie || d.Boss != "Boss A") {
			t.Errorf("questie replaced a dataset drop: %+v", d)
		}
	}
}

func TestPlanWithoutQuestieMatchesPlan(t *testing.T) {
	a, _ := Plan(priest(25), synth(t))
	b, _ := PlanWith(priest(25), synth(t), nil)
	if Render(priest(25), a, 5) != Render(priest(25), b, 5) {
		t.Error("nil Questie changed the plan")
	}
}

func TestNamedHasNoLevelGate(t *testing.T) {
	c := warrior(10, "Horde")
	p, togo, err := Named(c, synth(t), questieIndex(), "wailing caverns")
	if err != nil || p == nil || p.Name != "Wailing Caverns" || togo != 7 {
		t.Fatalf("named = %+v, togo %d, %v", p, togo, err)
	}
	if len(p.Bosses) != 1 || p.Bosses[0] != "Boss C" {
		t.Errorf("bosses %v", p.Bosses)
	}
	if len(p.Pickups) == 0 || p.Pickups[0].Title != "Wailing Errand" {
		t.Errorf("pickups %+v", p.Pickups)
	}
	text := RenderNamed(c, *p, togo)
	for _, want := range []string{"You're 10; it opens around 17.", "Boss C", "Wailing Errand", "Key or entry requirement: not known yet"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
	// The other side's dungeon is still described.
	if p, _, _ := Named(c, synth(t), nil, "Stone Stockade"); p == nil {
		t.Error("an Alliance dungeon was hidden from a Horde player")
	}
	if p, _, _ := Named(c, synth(t), nil, "Nowhere"); p != nil {
		t.Errorf("unknown dungeon found: %+v", p)
	}
}

func TestEntranceLessGroupLessRowIsNotADungeon(t *testing.T) {
	row := store.Row{"instance_type": "dungeon", "map_id": int64(2999), "name": "Ruins of Lordaeron", "min_level": int64(27), "max_level": int64(30)}
	if _, ok := pickOf(row, nil); ok {
		t.Error("a row with no entrance and no group size counted as a dungeon")
	}
	row["max_players"] = int64(5)
	if _, ok := pickOf(row, nil); !ok {
		t.Error("a row with a group size was dropped")
	}
}

func TestABossIsNamedOnce(t *testing.T) {
	p, _, err := Named(warrior(20, "Horde"), synth(t), nil, "Deadmines")
	if err != nil || p == nil {
		t.Fatalf("%v %v", p, err)
	}
	if got := strings.Join(p.Bosses, ","); got != "Boss A,Boss B,Lost Boss" {
		t.Errorf("bosses %q", got)
	}
	if len(p.UnlinkedBosses) != 1 || p.UnlinkedBosses[0] != "Lost Boss" {
		t.Errorf("unlinked %v", p.UnlinkedBosses)
	}
}

type bossModeRow struct {
	MapID      int32  `parquet:"map_id"`
	Name       string `parquet:"name"`
	CreatureID *int32 `parquet:"creature_id,optional"`
	Difficulty int32  `parquet:"difficulty_id"`
}

// raidAndModes: a dungeon whose bosses are kept under the base difficulties and an alternate mode (id 198), and a raid.
func raidAndModes(t *testing.T) store.DataStore {
	return store.New(store.ReaderSource{
		"items": table(t, []itemRow{it(1, "Mind Hood", 25, 15, cloth, head, 5, 7), it(2, "Mode Cap", 25, 15, cloth, head, 5, 7)}),
		"loot_sources": table(t, []lootRow{
			{1, "creature", 100, 1000, 3}, {1, "creature", 101, 1000, 0}, {2, "creature", 102, 10, 5},
		}),
		"instances": table(t, []instRow{
			{1, "Mode Keep", "dungeon", i16(5), i16(15), i16(21), "curated", "Westfall"},
			{2, "Ash Citadel", "raid", i16(40), i16(60), i16(60), "curated", "Silithus"},
			{3, "Wyrm's Lair", "raid", i16(40), i16(60), i16(60), "curated", "Dustwallow Marsh"},
		}),
		"instance_bosses": table(t, []bossModeRow{
			{1, "First Boss", i32(100), 0}, {1, "Second Boss", nil, 1}, {1, "Alternate Boss", i32(102), 198}, {1, "First Boss", nil, 198},
			{2, "Raid Boss", i32(101), 0},
		}),
	})
}

func TestAlternateModeBossesAreNotListed(t *testing.T) {
	p, _, err := Named(warrior(20, "Horde"), raidAndModes(t), nil, "Mode Keep")
	if err != nil || p == nil {
		t.Fatalf("%v %v", p, err)
	}
	if got := strings.Join(p.Bosses, ","); got != "First Boss,Second Boss" {
		t.Errorf("bosses %q", got)
	}
	for _, d := range p.Drops {
		if d.Boss == "Alternate Boss" {
			t.Errorf("a drop of an alternate-mode boss: %+v", d)
		}
	}
	if got := standardEncounters([]store.Row{{"difficulty_id": int64(198)}}); len(got) != 1 {
		t.Errorf("a map with only an alternate mode lost its bosses: %v", got)
	}
}

func TestARaidIsDescribedByNameNotPlanned(t *testing.T) {
	c := warrior(60, "Horde")
	ds := raidAndModes(t)
	p, _, err := Named(c, ds, nil, "Ash Citadel")
	if err != nil || p == nil || !p.Raid {
		t.Fatalf("%v %v", p, err)
	}
	text := RenderNamed(c, *p, 0)
	for _, want := range []string{"Ash Citadel (level 60; entrance in Silithus; 40 players)", "All bosses: Raid Boss", "Lockout and attunement: not known yet"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
	if strings.Contains(text, "Key or entry requirement") {
		t.Errorf("a raid was given the dungeon key line:\n%s", text)
	}
	if d := Mentioned(ds, nil, "what drops in Ash Citadel?"); d != "Ash Citadel" {
		t.Errorf("mentioned %q", d)
	}
	res, err := Plan(c, ds)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range append(res.Picks, res.Unknown...) {
		if p.Raid {
			t.Errorf("a raid was planned: %s", p.Name)
		}
	}
	if d, _, _ := Named(c, ds, nil, "Mode Keep"); d == nil || strings.Contains(RenderNamed(c, *d, 0), "Lockout") {
		t.Error("a dungeon was given a raid lockout line")
	}
}

func TestPercentOfARareDrop(t *testing.T) {
	for _, tc := range []struct {
		ch   float64
		want string
	}{{100, "100%"}, {33.3, "33%"}, {1, "1%"}, {0.5, "0.5%"}, {0.3, "0.3%"}, {0.04, "under 0.1%"}} {
		if got := percent(tc.ch); got != tc.want {
			t.Errorf("percent(%v) = %q, want %q", tc.ch, got, tc.want)
		}
	}
}

func TestANameWithAnApostropheIsMentioned(t *testing.T) {
	ds := raidAndModes(t)
	for _, q := range []string{"what drops in Wyrm's Lair?", "what drops in Wyrms Lair?", "wyrm\u2019s lair loot"} {
		if got := Mentioned(ds, nil, q); got != "Wyrm's Lair" {
			t.Errorf("%q: mentioned %q", q, got)
		}
	}
}

// namesStore holds dungeons whose names share a word and ones players shorten.
func namesStore(t *testing.T) store.DataStore {
	return store.New(store.ReaderSource{
		"instances": table(t, []instRow{
			{1, "Blackrock Depths", "dungeon", i16(5), i16(52), i16(60), "curated", "Searing Gorge"},
			{2, "Blackrock Spire", "dungeon", i16(10), i16(55), i16(60), "curated", "Burning Steppes"},
			{3, "Stormwind Stockade", "dungeon", i16(5), i16(22), i16(30), "curated", "Stormwind City"},
			{4, "Onyxia's Lair", "raid", i16(40), i16(60), i16(60), "curated", "Dustwallow Marsh"},
			{5, "Sunken Temple", "dungeon", i16(5), i16(50), i16(60), "curated", "Swamp of Sorrows"},
		}),
	})
}

func TestAPartOfANameThatFitsSeveralDungeonsIsNotGuessed(t *testing.T) {
	ds := namesStore(t)
	c := warrior(60, "Horde")
	p, _, err := Named(c, ds, nil, "Blackrock")
	if p != nil || err == nil || !strings.Contains(err.Error(), "Blackrock Depths") || !strings.Contains(err.Error(), "Blackrock Spire") {
		t.Fatalf("a partial name picked %v (%v)", p, err)
	}
	for _, name := range []string{"Blackrock Depths", "lower blackrock spire"} {
		if p, _, err := Named(c, ds, nil, name); err != nil || p == nil || !strings.HasPrefix(p.Name, "Blackrock") {
			t.Errorf("%q: %v %v", name, p, err)
		}
	}
	if d := Mentioned(ds, nil, "tell me about blackrock"); d != "" {
		t.Errorf("mentioned %q", d)
	}
}

func TestShortNamesPlayersUse(t *testing.T) {
	ds := namesStore(t)
	c := warrior(60, "Horde")
	for _, tc := range []struct{ text, want string }{
		{"what level is the stockade?", "Stormwind Stockade"}, {"how do I get into BRD", "Blackrock Depths"}, {"Onyxia attunement", "Onyxia's Lair"},
		{"loot in the Temple of Atal'Hakkar", "Sunken Temple"}, {"is UBRS worth it", "Blackrock Spire"}, {"what is the best strat", ""},
	} {
		if got := Mentioned(ds, nil, tc.text); got != tc.want {
			t.Errorf("%q: mentioned %q, want %q", tc.text, got, tc.want)
		}
		if tc.want == "" {
			continue
		}
		if p, _, err := Named(c, ds, nil, tc.want); err != nil || p == nil || p.Name != tc.want {
			t.Errorf("%q: named %v %v", tc.want, p, err)
		}
	}
	for alias, want := range map[string]string{"the stockade": "Stormwind Stockade", "Temple of Atal'Hakkar": "Sunken Temple", "ony": "Onyxia's Lair", "brd": "Blackrock Depths"} {
		if p, _, err := Named(c, ds, nil, alias); err != nil || p == nil || p.Name != want {
			t.Errorf("%q: named %v %v", alias, p, err)
		}
	}
}

func TestACountIsWordedInTheSingular(t *testing.T) {
	if got := Render(warrior(4, "Horde"), Result{First: &DungeonPick{Name: "Ember Chasm", MinLevel: ip(5), MaxLevel: ip(10)}, LevelsToGo: 1}, 0); !strings.Contains(got, "1 more level to go.") {
		t.Errorf("one level: %s", got)
	}
	r := Result{Picks: []DungeonPick{{Name: "A"}, {Name: "B"}, {Name: "C"}}}
	for limit, want := range map[int]string{2: "1 more dungeon fits.", 1: "2 more dungeons fit."} {
		if got := Render(warrior(20, "Horde"), r, limit); !strings.Contains(got, want) {
			t.Errorf("limit %d: %s", limit, got)
		}
	}
	nameless := character.Character{Class: "WARRIOR", Level: ip(20)}
	if got := Render(nameless, r, 0); !strings.HasPrefix(got, "Dungeons (level 20 warrior):") {
		t.Errorf("a character with no name: %q", got)
	}
}

func TestADropWithNoKillCountSaysItsChanceIsNotKnown(t *testing.T) {
	ch := 12.0
	if got := (Drop{Boss: "Boss A", Item: "Mind Hood", Slot: "head", Delta: 5, Chance: &ch}).String(); !strings.HasSuffix(got, "drops 12% of kills") {
		t.Errorf("a known chance: %q", got)
	}
	if got := (Drop{Boss: "Boss A", Item: "Mind Hood", Slot: "head", Delta: 5}).String(); !strings.HasSuffix(got, "drop chance not known") || strings.Contains(got, "%") {
		t.Errorf("an unknown chance: %q", got)
	}
}

func TestAlternateMode(t *testing.T) {
	ds := raidAndModes(t)
	rows, _ := ds.Filter("instance_bosses", store.Range{Column: "map_id", Min: int64(1), Max: int64(1)})
	for _, r := range rows {
		if want := r["difficulty_id"] == int64(198); AlternateMode(ds, r) != want {
			t.Errorf("%v (difficulty %v): alternate %v, want %v", r["name"], r["difficulty_id"], !want, want)
		}
	}
	raid, _ := ds.Filter("instance_bosses", store.Range{Column: "map_id", Min: int64(2), Max: int64(2)})
	if AlternateMode(ds, raid[0]) {
		t.Error("an ordinary raid boss is an alternate mode")
	}
}

func TestPartySizeIsFiveForADungeon(t *testing.T) {
	for _, tc := range []struct {
		name string
		raid bool
		in   int
		want int
	}{
		{"Deadmines", false, 10, 5}, {"Sunken Temple", false, 20, 5}, {"Scholomance", false, 5, 5}, {"Blackrock Spire", false, 10, 10},
		{"Molten Core", true, 40, 40}, {"Zul'Gurub", true, 20, 20}, {"Ruins of Ahn'Qiraj", true, 20, 20},
	} {
		if got := PartySize(tc.name, tc.raid, tc.in); got != tc.want {
			t.Errorf("PartySize(%q, raid %v, %d) = %d, want %d", tc.name, tc.raid, tc.in, got, tc.want)
		}
	}
	ten := 10
	if got := (DungeonPick{Name: "Blackrock Spire", MaxPlayers: &ten}).players(); got != "5 players in Lower Blackrock Spire, 10 in Upper" {
		t.Errorf("Blackrock Spire worded %q", got)
	}
}

func TestMoreShortNames(t *testing.T) {
	for alias, want := range map[string]string{"zf": "Zul'Farrak", "wc": "Wailing Caverns", "sm": "Scarlet Monastery", "uld": "Uldaman", "st": "Sunken Temple",
		"vc": "Deadmines", "dme": "Dire Maul", "dmn": "Dire Maul", "dmw": "Dire Maul", "UBRS": "Blackrock Spire"} {
		if got := aliasOf(alias); got != want {
			t.Errorf("aliasOf(%q) = %q, want %q", alias, got, want)
		}
	}
	for _, alias := range []string{"mc", "dm", "aq"} {
		if got := aliasOf(alias); got != "" {
			t.Errorf("aliasOf(%q) = %q, want none: it fits more than one thing", alias, got)
		}
	}
}

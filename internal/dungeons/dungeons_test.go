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
			{1, "Deadmines", "dungeon", i16(5), i16(15), i16(21)},
			{2, "Wailing Caverns", "dungeon", i16(5), i16(17), i16(23)},
			{3, "Shadowfang Keep", "dungeon", nil, i16(24), i16(30)},
			{4, "Mystery Hold", "dungeon", nil, nil, nil},
			{5, "Far Keep", "dungeon", nil, i16(40), i16(45)},
			{6, "Big Raid", "raid", nil, i16(18), i16(20)},
		}),
		"instance_bosses": table(t, []bossRow{
			{1, "Boss A", i32(100)}, {1, "Boss B", i32(101)}, {1, "Lost Boss", nil},
			{2, "Boss C", i32(102)}, {4, "Boss D", i32(103)},
		}),
		"quests": table(t, []questRow{{900, i32(1), nil}, {901, nil, &[]string{"wailing caverns"}[0]}, {902, i32(9), nil}}),
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
	for _, want := range []string{"1. Deadmines (level 15-21, 5 players): 2 upgrade(s)", "Mind Hood (head, +5.0) from Boss A, drops 20.0% of kills",
		"boss drops not linked in this dataset: Lost Boss", "open quest: Quest  (900)", "1 more dungeon(s) fit.", "Level unknown (unranked", "Mystery Hold (level unknown)", "drop chance unknown"} {
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
	// Shadowfang, 2 above with none, is last.
	r, err = Plan(rogue(22), synth(t))
	if err != nil {
		t.Fatal(err)
	}
	if got := names(r.Picks); got != "Wailing Caverns,Deadmines,Shadowfang Keep" || r.Picks[1].Fit != 1 || r.Picks[2].Fit != 2 {
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
	if err != nil || len(r.Picks) != 0 || !strings.Contains(Render(priest(60), r, 3), "No dungeon in the dataset") {
		t.Errorf("level 60: %+v, %v", r, err)
	}
}

package paths_test

import (
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/advice"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/dungeons"
	"github.com/WoW-Companion/advisor/internal/farm"
	"github.com/WoW-Companion/advisor/internal/names"
	"github.com/WoW-Companion/advisor/internal/paths"
	"github.com/WoW-Companion/advisor/internal/questie"
	"github.com/WoW-Companion/advisor/internal/quests"
	"github.com/WoW-Companion/advisor/internal/route"
	"github.com/WoW-Companion/advisor/internal/session"
	"github.com/WoW-Companion/advisor/internal/store"
)

// The offline boss-loot, rare-spawn, elite-quest and dungeon-order benchmark: what the advisor says of what a creature or a boss
// drops, of where a rare stands, of whether a quest needs a group, and of the order in which dungeons suit a level, beside the
// dataset's rows and the guides, with no network. It reads the snapshots the other offline benchmarks read (places: instances,
// instance_bosses, npcs, quests_observed; sources and gating: items and loot_sources). The dataset links no boss to a creature, holds
// no creature that is a rare or an elite and no spawn place or timer, so the cases it cannot answer check that the advisor says so and
// states none; the rare-creature ranks of a player's QuestieDB are synthetic fixtures. A difference the dataset's own rows cause is
// marked "(data)". make offline-benchmark runs it; WOWC_BENCH_REPORT receives the tables.

const (
	encLootSrc   = "dataset loot_sources (source_kind creature: kills, drops), items; the advisor's farm spots"
	encNoLootSrc = "dataset loot_sources (no row for the item), items; Warcraft Wiki item pages (the dungeon boss that drops it)"
	encLastSrc   = "dataset instance_bosses (name, order_index); Warcraft Wiki dungeon pages (the last boss of each dungeon)"
	encRareSrc   = "dataset npcs (classification, zone_name: none for a rare); Warcraft Wiki creature pages (rare spawn and its zone)"
	encRankSrc   = "Questie npc rank (0 normal, 1 elite, 2 rare elite, 3 boss, 4 rare); Warcraft Wiki (a rare is soloable, a rare elite is not)"
	encQuestSrc  = "dataset quests_observed (tag group or dungeon); Warcraft Wiki quest pages (Group quest, Dungeon quest)"
	encOrderSrc  = "dataset instances (min_level, max_level; level_source curated); Warcraft Wiki dungeon level ranges listed lowest first"
	encNoneSrc   = "the dataset has no such row, column or table"
)

// ---- boss loot ----

// encDrops are recipe items that creatures drop: the chance the advisor gives beside the dataset's kills and drops.
var encDrops = []int64{9298, 3873, 8390, 11203, 255730, 273103, 7450}

// TestOfflineBenchmarkBossLootChance: the chance a creature drops an item, as the farm spots give it, beside the dataset's loot_sources rows.
func TestOfflineBenchmarkBossLootChance(t *testing.T) {
	ds := srcStore()
	var out []benchRow
	for _, id := range encDrops {
		it, err := ds.Get("items", id)
		if err != nil {
			t.Fatal(err)
		}
		name := text(it["name"])
		rows, _ := ds.Filter("loot_sources", store.Range{Column: "item_id", Min: id, Max: id})
		var kills, drops int64
		for _, r := range rows {
			if text(r["source_kind"]) == "creature" {
				kills += num(r["kills"])
				drops += num(r["drops"])
			}
		}
		spots := farm.Spots(horde(40), ds, id)
		render := farm.Render(name, spots, false, 9)
		var gk, gd int64
		ok := len(spots) > 0
		for _, s := range spots {
			gk += s.Kills
			gd += s.Drops
			ok = ok && s.Chance != nil && strings.Contains(render, fmt.Sprintf("%.1f%% of %d loot windows", *s.Chance, s.Kills))
		}
		ok = ok && gk == kills && gd == drops
		out = append(out, benchRow{name + ": the chance it drops", fmt.Sprintf("%d creatures, %d kills, %d drops", len(spots), gk, gd), fmt.Sprintf("loot_sources: %d kills, %d drops, %.1f%% over all creatures", kills, drops, 100*float64(drops)/float64(kills)), encLootSrc,
			resultIf(ok, "another count or chance than the dataset's, or a creature was added")})
		out = append(out, benchRow{name + ": the boss that drops it", fmt.Sprintf("creature %d, zone unknown", spots[0].ID), "a named creature (Warcraft Wiki)", encLootSrc,
			"differ (data): the dataset names no creature or zone for the drop"})
	}
	finish(t, "advice-boss-loot-chance", "Offline boss and creature loot: the chance of a drop", out)
}

// encMolten are Molten Core items; the dataset holds the item and no creature that drops it.
var encMolten = []struct{ name, boss string }{
	{"Fiery Core", "Molten Core (Ragnaros's trash; Warcraft Wiki)"}, {"Lava Core", "Molten Core (Lava Annihilator; Warcraft Wiki)"},
	{"Core Leather", "Molten Core (any boss; Warcraft Wiki)"},
}

// TestOfflineBenchmarkBossLootNoSource: an item a boss drops that the dataset holds no loot row for: the advisor says the data lists no source, names no boss or
// dungeon, and offers no farm spot.
func TestOfflineBenchmarkBossLootNoSource(t *testing.T) {
	ds := gateStore()
	var out []benchRow
	for _, m := range encMolten {
		row := rowByName(t, ds, "items", m.name)
		id := num(row["id"])
		res := paths.For(horde(60), ds, id, paths.Options{})
		txt := res.Text()
		spots := farm.Spots(horde(60), ds, id)
		ok := strings.Contains(txt, "lists no source") && len(spots) == 0 && !strings.Contains(txt, "Molten Core") && !res.Dependable()
		out = append(out, benchRow{m.name + ": which boss drops it", firstLine(txt), m.boss, encNoLootSrc, resultIf(ok, "a boss, a dungeon or a spot was named with no source")})
		out = append(out, benchRow{m.name + ": the dungeon boss", "none named", m.boss, encNoLootSrc, "differ (data): the dataset links no item to a boss or dungeon"})
	}
	finish(t, "advice-boss-loot-no-source", "Offline boss loot: an item with no loot row", out)
}

// encLast are dungeons and the boss that ends each, from the guides.
var encLast = []struct{ dungeon, last string }{
	{"Deadmines", "Edwin VanCleef"}, {"Wailing Caverns", "Mutanus the Devourer"}, {"Shadowfang Keep", "Archmage Arugal"}, {"Uldaman", "Archaedas"},
	{"Scholomance", "Darkmaster Gandling"}, {"Zul'Farrak", "Chief Ukorz Sandscalp"}, {"Gnomeregan", "Mekgineer Thermaplugg"}, {"Blackrock Depths", "Emperor Dagran Thaurissan"},
}

// TestOfflineBenchmarkBossLootLastBoss: the bosses of a dungeon come in the order of the run, so the last name is the boss that ends it, and the advisor says
// the drops are not known for them.
func TestOfflineBenchmarkBossLootLastBoss(t *testing.T) {
	ds := instStore()
	c := character.Character{Faction: "Horde", Class: "WARRIOR"}
	var out []benchRow
	for _, tc := range encLast {
		p, _, err := dungeons.Named(c, ds, nil, tc.dungeon)
		if err != nil || p == nil || len(p.Bosses) == 0 {
			t.Fatalf("%s: %v", tc.dungeon, err)
		}
		txt := dungeons.RenderNamed(c, *p, 0)
		last := p.Bosses[len(p.Bosses)-1]
		ok := last == tc.last && strings.HasSuffix(strings.TrimSpace(strings.SplitN(strings.SplitN(txt, "All bosses: ", 2)[1], "\n", 2)[0]), tc.last) && strings.Contains(txt, "Drops not known yet")
		out = append(out, benchRow{tc.dungeon + ": the boss that ends it, and its drops", last + "; drops not known yet", tc.last + "; drops from the loot table (Warcraft Wiki)", encLastSrc, resultIf(ok, "another last boss, or a drop was stated")})
	}
	finish(t, "advice-boss-loot-last-boss", "Offline boss loot: the last boss of a dungeon", out)
}

// ---- rare spawns ----

var encRares = []struct{ name, where string }{
	{"Mother Fang", "Elwynn Forest"}, {"Vultros", "Elwynn Forest"}, {"Gruff Swiftbite", "Westfall"}, {"Lord Condar", "Loch Modan"},
	{"Mor'Ladim", "Duskwood"}, {"Thuros Lightfingers", "Westfall"}, {"Narg the Taskmaster", "Elwynn Forest"},
}

// TestOfflineBenchmarkRareSpawnByName: a rare asked for by name: the dataset holds no such creature, no zone for any, so the advisor finds none and states no zone.
func TestOfflineBenchmarkRareSpawnByName(t *testing.T) {
	ds := srcStore()
	var out []benchRow
	for _, r := range encRares {
		hits, _, err := names.Find(ds, names.Npcs, r.name, 5)
		id, name, cands := farm.Resolve(ds, r.name)
		ok := len(hits) == 0 && err == nil && id == 0 && name == "" && len(cands) == 0
		out = append(out, benchRow{r.name + ": found, and where it stands", fmt.Sprintf("%d creatures, no zone", len(hits)), "a rare in " + r.where + " (Warcraft Wiki)", encRareSrc, resultIf(ok, "a creature or zone was invented")})
	}
	out = append(out, benchRow{"the rare creatures of the dataset", "none stated", "npcs.classification holds only 0 or null; npcs.zone_name is null for every row", encRareSrc, "differ (data): the dataset holds no rare, no spawn place and no respawn timer"})
	finish(t, "advice-rare-spawn-by-name", "Offline rare spawns: a rare asked for by name", out)
}

var encRanks = []struct {
	name       string
	rank       int
	inDungeon  bool
	wantGroup  bool
	guideWords string
}{
	{"a rare in the open", 4, false, false, "a rare is soloable"}, {"a rare inside a dungeon", 4, true, true, "a dungeon is group content"},
	{"a rare elite in the open", 2, false, true, "a rare elite needs a group"}, {"an elite in the open", 1, false, true, "an elite needs a group"},
	{"a boss in the open", 3, false, true, "a boss needs a group"}, {"a normal creature inside a dungeon", 0, true, true, "a dungeon is group content"},
	{"a normal creature in the open", 0, false, false, "a normal creature is soloable"},
}

// TestOfflineBenchmarkRareSpawnGroupFlag: whether the quest a creature of each rank is the objective of is flagged as needing a group, in the open and in a dungeon.
func TestOfflineBenchmarkRareSpawnGroupFlag(t *testing.T) {
	var out []benchRow
	for _, tc := range encRanks {
		spawn := []questie.Spawn{{Area: 100, X: 50, Y: 50}}
		if tc.inDungeon {
			spawn = []questie.Spawn{{Area: 910, X: 50, Y: 50}}
		}
		qx := &questie.Index{
			Quests:   map[int]*questie.Quest{1: {ID: 1, Name: "Synthetic Hunt", Level: 20, Creatures: []questie.Ref{{ID: 10}}}},
			Npcs:     map[int]questie.Entity{10: {Name: "Synthetic Beast", Level: 20, Rank: tc.rank, Spawns: spawn}},
			Dungeons: []questie.Dungeon{{Area: 910, Name: "Synthetic Keep"}},
		}
		level := 20
		c := character.Character{Name: "Benchtest", Faction: "Horde", Class: "WARRIOR", Level: &level, Quests: []character.Quest{{QuestID: 1, Title: "Synthetic Hunt", Level: &level}}}
		in := session.Live(c, store.New(store.ReaderSource{}), qx, route.Plan{Start: &route.Point{Map: 1, X: 50, Y: 50}}, nil)
		got := in.Info.Group != nil && in.Info.Group(1)
		out = append(out, benchRow{tc.name + ": group flag on its quest", fmt.Sprint(got), fmt.Sprintf("%v (%s)", tc.wantGroup, tc.guideWords), encRankSrc, resultIf(got == tc.wantGroup, "a group quest was not flagged, or a solo one was")})
	}
	finish(t, "advice-rare-spawn-group-flag", "Offline rare spawns: the group flag by rank", out)
}

// TestOfflineBenchmarkRareSpawnInDungeon: a rare and a rare elite inside a dungeon, as a player's QuestieDB lists them: the rare elite is named an elite, the rare is
// not, and the boss is named a boss.
func TestOfflineBenchmarkRareSpawnInDungeon(t *testing.T) {
	ds := instStore()
	qx := &questie.Index{
		Areas: map[int]questie.Area{900: {Zone: "Westfall"}},
		Npcs: map[int]questie.Entity{
			20: {Name: "Synthetic Rare Hound", Rank: 4}, 21: {Name: "Synthetic Rare Elite Ogre", Rank: 2}, 22: {Name: "Synthetic Dungeon Boss", Rank: 3}, 23: {Name: "Synthetic Guard", Rank: 1},
		},
		Dungeons: []questie.Dungeon{{Area: 910, Name: "Deadmines", Parent: 900, Creatures: []int{22, 21, 23, 20}}},
	}
	c := character.Character{Faction: "Horde", Class: "WARRIOR"}
	p, _, err := dungeons.Named(c, ds, qx, "Deadmines")
	if err != nil || p == nil {
		t.Fatalf("%v", err)
	}
	txt := dungeons.RenderNamed(c, *p, 0)
	line := func(prefix string) string {
		for _, l := range strings.Split(txt, "\n") {
			if s := strings.TrimSpace(l); strings.HasPrefix(s, prefix) {
				return s
			}
		}
		return ""
	}
	elites, other, boss := line("Elites:"), line("Other creatures:"), line("Bosses (Questie):")
	var out []benchRow
	out = append(out, benchRow{"a rare elite in a dungeon", elites, "listed as an elite (rank 2)", encRankSrc, resultIf(strings.Contains(elites, "Synthetic Rare Elite Ogre") && strings.Contains(elites, "Synthetic Guard"), "a rare elite is not listed as an elite")})
	out = append(out, benchRow{"a rare in a dungeon", other, "not an elite, not a boss (rank 4)", encRankSrc, resultIf(strings.Contains(other, "Synthetic Rare Hound") && !strings.Contains(elites, "Rare Hound") && !strings.Contains(boss, "Rare Hound"), "a rare is listed as an elite or a boss")})
	out = append(out, benchRow{"a boss in a dungeon", boss, "listed as a boss (rank 3)", encRankSrc, resultIf(strings.Contains(boss, "Synthetic Dungeon Boss") && !strings.Contains(elites, "Dungeon Boss"), "a boss is not listed as a boss")})
	finish(t, "advice-rare-spawn-in-dungeon", "Offline rare spawns: a rare and a rare elite inside a dungeon", out)
}

// ---- elite and group quests ----

var encGroupQuests = []struct {
	id     int64
	tag    string
	source string
}{
	{1486, "dungeon", "Wailing Caverns"}, {914, "dungeon", "Wailing Caverns"}, {6562, "dungeon", "Blackfathom Deeps"}, {2922, "dungeon", "Gnomeregan"},
	{1014, "dungeon", "Shadowfang Keep"}, {6626, "dungeon", "Razorfen Downs"}, {5088, "group", "Thousand Needles"}, {547, "group", "Hillsbrad Foothills"},
	{652, "group", "Arathi Highlands"},
}

// encSoloQuests are quests players gave no group or dungeon tag.
var encSoloQuests = []int64{189, 575, 584, 1362, 5, 89}

func encQuestLine(ds store.DataStore, id int64) (store.Row, character.Character) {
	rows, _ := ds.Filter("quests_observed", store.Range{Column: "quest_id", Min: id, Max: id})
	row := rows[0]
	lvl := int(num(row["level"]))
	done := true
	return row, character.Character{Name: "Benchtest", Faction: "Horde", Class: "WARRIOR", Level: &lvl, Quests: []character.Quest{{QuestID: int(id), Title: text(row["title"]), Level: &lvl, Complete: &done}}}
}

// TestOfflineBenchmarkGroupQuestFlag: a quest in the log that players tagged group or dungeon is shown as an elite or group quest, and a quest
// that is asked about by name says it is a group or dungeon quest; a quest with no tag is neither.
func TestOfflineBenchmarkGroupQuestFlag(t *testing.T) {
	ds := qbStore()
	var out []benchRow
	for _, tc := range encGroupQuests {
		row, c := encQuestLine(ds, tc.id)
		steps := advice.WhatsNext(c, ds)
		flagged := len(steps) > 0 && steps[0].Elite
		out = append(out, benchRow{fmt.Sprintf("%s (%s): flagged as an elite or group quest", text(row["title"]), tc.source), fmt.Sprint(flagged), fmt.Sprintf("true (tag %s)", text(row["tag"])), encQuestSrc,
			resultIf(flagged && text(row["tag"]) == tc.tag, "a quest players tagged as a group quest is not flagged")})
		id, name, _ := quests.Resolve(ds, text(row["title"]))
		words := quests.Render(quests.OfWith(ds, nil, id), name)
		want := map[string]string{"dungeon": "a dungeon quest", "group": "a group quest"}[tc.tag]
		out = append(out, benchRow{fmt.Sprintf("%s: the quest lookup says it needs a group", text(row["title"])), firstLine(words), want + " (" + tc.source + ")", encQuestSrc,
			resultIf(id == tc.id && strings.Contains(words, want), "the quest lookup does not say it is a group or dungeon quest")})
	}
	finish(t, "advice-group-quest-flag", "Offline elite and group quests: the flag and the lookup", out)
}

// TestOfflineBenchmarkSoloQuestFlag: a quest players did not tag is not shown as an elite or group quest.
func TestOfflineBenchmarkSoloQuestFlag(t *testing.T) {
	ds := qbStore()
	var out []benchRow
	for _, id := range encSoloQuests {
		row, c := encQuestLine(ds, id)
		steps := advice.WhatsNext(c, ds)
		flagged := len(steps) > 0 && steps[0].Elite
		_, name, _ := quests.Resolve(ds, text(row["title"]))
		words := quests.Render(quests.OfWith(ds, nil, id), name)
		out = append(out, benchRow{fmt.Sprintf("%s: not an elite or group quest", text(row["title"])), fmt.Sprint(flagged), "false (no tag)", encQuestSrc,
			resultIf(!flagged && text(row["tag"]) == "" && !strings.Contains(words, "group quest") && !strings.Contains(words, "dungeon quest"), "a solo quest was called a group or dungeon quest")})
	}
	finish(t, "advice-solo-quest-flag", "Offline elite and group quests: a quest with no tag", out)
}

// ---- dungeon order by level ----

// encLadder is the dungeons a character of a side may be offered at a level, lowest range first, from the dataset's instances rows.
func encLadder(t *testing.T, ds store.DataStore, side string, level int) []string {
	t.Helper()
	rows, _ := ds.Filter("instances", store.Range{Column: "map_id", Min: int64(0)})
	other := map[string]string{"Horde": "Stormwind Stockade", "Alliance": "Ragefire Chasm"}[side]
	type e struct {
		name        string
		fit, lo, hi int
		map_        int64
	}
	var es []e
	for _, r := range rows {
		lo, hi := int(num(r["min_level"])), int(num(r["max_level"]))
		if text(r["instance_type"]) != "dungeon" || lo == 0 || text(r["name"]) == other || text(r["entrance_zone_name"]) == "" {
			continue
		}
		if hi < level-dungeons.LevelWindow || lo > level {
			continue
		}
		es = append(es, e{name: text(r["name"]), fit: max(level-hi, 0), lo: lo, hi: hi, map_: num(r["map_id"])})
	}
	sort.SliceStable(es, func(i, j int) bool {
		a, b := es[i], es[j]
		if a.fit != b.fit {
			return a.fit < b.fit
		}
		if a.lo != b.lo {
			return a.lo < b.lo
		}
		if a.hi != b.hi {
			return a.hi < b.hi
		}
		return a.map_ < b.map_
	})
	var out []string
	for _, x := range es {
		out = append(out, x.name)
	}
	return out
}

var encLevels = []struct {
	side  string
	level int
}{{"Horde", 20}, {"Horde", 30}, {"Horde", 40}, {"Horde", 50}, {"Horde", 58}, {"Alliance", 25}, {"Alliance", 45}}

func TestOfflineBenchmarkDungeonOrderA(t *testing.T) { dungeonOrder(t, "a", encLevels[:4]) }
func TestOfflineBenchmarkDungeonOrderB(t *testing.T) { dungeonOrder(t, "b", encLevels[4:]) }

// dungeonOrder: the dungeons offered to a character at a level come lowest level range first (the order a guide lists them in, the one that is
// about to be outgrown first), with a dungeon already above its range after those that fit.
func dungeonOrder(t *testing.T, part string, cases []struct {
	side  string
	level int
}) {
	ds := instStore()
	var out []benchRow
	for _, tc := range cases {
		l := tc.level
		c := character.Character{Faction: tc.side, Class: "WARRIOR", Level: &l}
		r, err := dungeons.Plan(c, ds)
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, p := range r.Picks {
			got = append(got, p.Name)
		}
		want := encLadder(t, ds, tc.side, tc.level)
		out = append(out, benchRow{fmt.Sprintf("%s level %d: the dungeons in order", tc.side, tc.level), strings.Join(got, " > "), strings.Join(want, " > "), encOrderSrc,
			resultIf(slices.Equal(got, want) && len(got) > 0, "the order is not lowest range first")})
	}
	finish(t, "advice-dungeon-order-"+part, "Offline dungeon order: the dungeons for a level ("+part+")", out)
}

var encFirst = []struct {
	side, name string
	level      int
}{{"Horde", "Ragefire Chasm", 10}, {"Alliance", "Deadmines", 10}, {"Horde", "Ragefire Chasm", 5}}

// TestOfflineBenchmarkDungeonOrderFirst: a character below every dungeon is told the first one that opens for them and how many levels to go, not one of the other side.
func TestOfflineBenchmarkDungeonOrderFirst(t *testing.T) {
	ds := instStore()
	var out []benchRow
	for _, tc := range encFirst {
		l := tc.level
		c := character.Character{Faction: tc.side, Class: "WARRIOR", Level: &l}
		r, err := dungeons.Plan(c, ds)
		if err != nil {
			t.Fatal(err)
		}
		row := rowByName(t, ds, "instances", tc.name)
		got := "no dungeon named"
		if r.First != nil {
			got = fmt.Sprintf("%s, %d levels to go", r.First.Name, r.LevelsToGo)
		}
		want := fmt.Sprintf("%s, %d levels to go", tc.name, num(row["min_level"])-int64(tc.level))
		out = append(out, benchRow{fmt.Sprintf("%s level %d: the first dungeon to open", tc.side, tc.level), got, want, encOrderSrc, resultIf(got == want && len(r.Picks) == 0, "another first dungeon, or levels to go")})
	}
	finish(t, "advice-dungeon-order-first", "Offline dungeon order: the first dungeon to open", out)
}

// ---- no answer ----

var groupSize = regexp.MustCompile(`(?i)\b(\d+|five|ten|two|three|four)[- ](player|man|person|people)s?\b|\bgroup of \d+`)

// TestOfflineBenchmarkEncounterNoAnswer: what the dataset cannot say about a boss, a rare, a group quest and a dungeon order: the advisor says so and states none.
func TestOfflineBenchmarkEncounterNoAnswer(t *testing.T) {
	ds := srcStore()
	var out []benchRow
	// a boss that is no creature of the dataset
	for _, n := range []string{"Ragnaros", "Edwin VanCleef", "Archaedas"} {
		hits, _, err := names.Find(ds, names.Npcs, n, 5)
		spots := farm.Spots(horde(60), ds, 17010)
		out = append(out, benchRow{n + ": the boss as a creature with a loot table", fmt.Sprintf("%d creatures, %d spots for its drops", len(hits), len(spots)), "a boss creature with a loot table (Warcraft Wiki)", encNoneSrc,
			resultIf(len(hits) == 0 && err == nil && len(spots) == 0, "a creature or a drop was invented")})
	}
	// the drops of the bosses of three dungeons are not known
	c := character.Character{Faction: "Horde", Class: "WARRIOR"}
	for _, d := range []string{"Deadmines", "Shadowfang Keep", "Uldaman"} {
		p, _, _ := dungeons.Named(c, instStore(), nil, d)
		txt := dungeons.RenderNamed(c, *p, 0)
		out = append(out, benchRow{d + ": what its bosses drop", firstLine(strings.SplitN(txt, "Drops not known yet", 2)[1]), "the boss loot tables (Warcraft Wiki)", encNoneSrc,
			resultIf(len(p.Drops) == 0 && len(p.UnlinkedBosses) == len(p.Bosses) && strings.Contains(txt, "Drops not known yet"), "a drop was stated for a boss the dataset cannot link")})
	}
	// no respawn timer, spawn place or group size column
	npcs, _ := ds.Filter("npcs", store.Range{Column: "id", Min: int64(0)})
	cols := map[string]bool{}
	for _, r := range npcs {
		for k := range r {
			cols[k] = true
		}
	}
	out = append(out, benchRow{"npcs: a respawn timer or a spawn place column", fmt.Sprintf("%d columns, none for either", len(cols)), "no respawn, spawn_x or spawn_y column", encNoneSrc,
		resultIf(!cols["respawn"] && !cols["respawn_time"] && !cols["spawn_x"] && !cols["spawn_y"], "a column was invented")})
	qo, _ := qbStore().Filter("quests_observed", store.Range{Column: "quest_id", Min: int64(0)})
	size := 0
	for _, r := range qo {
		if num(r["suggested_group"]) != 0 {
			size++
		}
	}
	_, name, _ := quests.Resolve(qbStore(), "Arikara")
	words := quests.Render(quests.OfWith(qbStore(), nil, 5088), name)
	out = append(out, benchRow{"Arikara: how many players it needs", firstLine(words), "no group size in the dataset (suggested_group is null on every row)", encNoneSrc,
		resultIf(size == 0 && !groupSize.MatchString(words), "a group size was stated")})
	// a dungeon with no level range is not ranked among the others
	l := 40
	r, _ := dungeons.Plan(character.Character{Faction: "Horde", Class: "WARRIOR", Level: &l}, instStore())
	unranked := true
	for _, p := range r.Picks {
		if p.LevelUnknown || p.MinLevel == nil {
			unranked = false
		}
	}
	out = append(out, benchRow{"a dungeon with no level range in the order", fmt.Sprintf("%d unranked, none among the picks", len(r.Unknown)), "no min_level or max_level row", encNoneSrc, resultIf(unranked && len(r.Unknown) > 0, "a dungeon with no range was ranked")})
	finish(t, "advice-encounter-no-answer", "Offline boss loot, rares, group quests and dungeon order: no answer in the dataset", out)
}

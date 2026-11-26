package paths_test

import (
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/WoW-Companion/advisor/internal/advice"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/dungeons"
	"github.com/WoW-Companion/advisor/internal/farm"
	"github.com/WoW-Companion/advisor/internal/learn"
	"github.com/WoW-Companion/advisor/internal/names"
	"github.com/WoW-Companion/advisor/internal/paths"
	"github.com/WoW-Companion/advisor/internal/questie"
	"github.com/WoW-Companion/advisor/internal/quests"
	"github.com/WoW-Companion/advisor/internal/route"
	"github.com/WoW-Companion/advisor/internal/session"
	"github.com/WoW-Companion/advisor/internal/store"
	"github.com/WoW-Companion/advisor/internal/wording"
)

// The offline class-quest-chain, rare-spawn, elite-location and recipe-source benchmark: what the advisor answers about
// the order of a class's quests, about a creature that is a rare, an elite or a boss and where it stands, and about
// where a recipe is obtained (a drop, a vendor, a quest or a trainer), beside the dataset's rows, with no network. It
// reads testdata/places_snapshot (quests_observed, npcs, instances, instance_bosses, trainers), testdata/advice_snapshot
// (recipes, items, vendor_sources, loot_sources) and testdata/sources_snapshot (the recipes whose recipe item drops,
// with their loot rows and the creatures that drop them; rebuild it with testdata/sources_snapshot/build.sh).
// The dataset holds no class quest line, no creature that is a rare or an elite (every named creature is a critter or
// a name Everloot gave, with no classification, level or zone), no recipe vendor name and no quest that rewards a
// recipe, so those cases check that the advisor says so and states none. The class-quest order and the creature
// ranks of a player's QuestieDB are synthetic fixtures built over the dataset's own quest rows. A difference the
// dataset's own rows cause is marked "(data)". make offline-benchmark runs it; WOWC_BENCH_REPORT receives the tables.

const (
	srcDir          = "../../testdata/sources_snapshot"
	chainSrc        = "dataset quests_observed (title, level, faction, category); a synthetic QuestieDB over those rows gives the order (the dataset has no class quest line)"
	rankSrc         = "dataset npcs.classification codes (0 normal, 1 elite, 2 rare elite, 3 boss, 4 rare); the dataset's snapshot holds none above 0"
	eliteQuestSrc   = "dataset quests_observed (tag group or dungeon, level)"
	creatureSrc     = "dataset npcs (name, min_level, max_level, zone_name), loot_sources (kills, drops)"
	bossSrc         = "dataset instance_bosses (name), instances (entrance_zone_name, min_level, max_level)"
	recipeOriginSrc = "dataset recipes (learn_skill, learned_from), recipe_items, loot_sources (creature, kills, drops), vendor_sources, trainer_spells, npcs"
	noSourceSrc     = "the dataset has no such row, column or table"
)

// srcMix serves the sources snapshot's tables first and the places and advice snapshots' for the rest.
type srcMix struct{ first, then store.Source }

func (m srcMix) Open(table string) (io.ReaderAt, int64, error) {
	if r, n, err := m.first.Open(table); err == nil {
		return r, n, nil
	}
	return m.then.Open(table)
}

var (
	srcOnce  sync.Once
	srcCache store.DataStore
)

// srcStore reads the snapshots once for the package: the tables are decoded on first use and the tests share them.
func srcStore() store.DataStore {
	srcOnce.Do(func() {
		srcCache = store.New(srcMix{first: store.DirSource(srcDir), then: qbMix{first: store.DirSource(qbPlacesDir), then: store.DirSource(qbAdviceDir)}})
	})
	return srcCache
}

// ---- class quest chains ----

// classChains are the class quests of the dataset in the order a player does them (the level order of
// quests_observed, ties by number): the Warlock quests of the Dogran and Cabal lines and the Paladin starter quests.
var classChains = []struct {
	class string
	ids   []int64
}{
	{"Warlock", []int64{1507, 1508, 1509, 1740, 1801}},
	{"Paladin", []int64{98601, 90902, 91208}},
}

var classBit = map[string]uint64{"Paladin": 1 << 1, "Warlock": 1 << 8}

// chainIndex is a QuestieDB whose class quests are the dataset's own rows (title, level), each limited to its
// class and each one following the one before it in its chain.
func chainIndex(t *testing.T, ds store.DataStore) *questie.Index {
	t.Helper()
	ix := &questie.Index{Quests: map[int]*questie.Quest{}}
	for _, ch := range classChains {
		for i, id := range ch.ids {
			row := qbQuest(t, id)
			q := &questie.Quest{ID: int(id), Name: text(row["title"]), Level: int(num(row["level"])), Classes: classBit[ch.class]}
			if i > 0 {
				q.PreSingle = []int{int(ch.ids[i-1])}
			}
			if i+1 < len(ch.ids) {
				q.Next = int(ch.ids[i+1])
			}
			ix.Quests[int(id)] = q
		}
	}
	return ix
}

func TestOfflineBenchmarkClassQuestChainWarlock(t *testing.T) { classChain(t, "warlock", 0) }
func TestOfflineBenchmarkClassQuestChainPaladin(t *testing.T) { classChain(t, "paladin", 1) }

// classChain: each class quest in its chain shows the quest the dataset names for its id (title, level, side, class
// heading), what it comes after and what it leads to, with the title and level of those quests as the dataset has
// them, and no other class.
func classChain(t *testing.T, part string, chain int) {
	ds := qbStore()
	ix := chainIndex(t, ds)
	ch := classChains[chain]
	var out []benchRow
	for i, id := range ch.ids {
		row := qbQuest(t, id)
		c := quests.OfWith(ds, ix, id)
		txt := quests.Render(c, "")
		var prev, next []string
		if i > 0 {
			p := qbQuest(t, ch.ids[i-1])
			prev = []string{fmt.Sprintf("%s (quest %d), level %d", text(p["title"]), ch.ids[i-1], num(p["level"]))}
		}
		if i+1 < len(ch.ids) {
			n := qbQuest(t, ch.ids[i+1])
			next = []string{fmt.Sprintf("%s (quest %d), level %d", text(n["title"]), ch.ids[i+1], num(n["level"]))}
		}
		var gotPrev, gotNext []string
		for _, f := range c.Before {
			gotPrev = append(gotPrev, fmt.Sprintf("%s (quest %d), level %d", f.Name, f.QuestID, f.Level))
		}
		for _, f := range c.After {
			gotNext = append(gotNext, fmt.Sprintf("%s (quest %d), level %d", f.Name, f.QuestID, f.Level))
		}
		head := fmt.Sprintf("%s (quest %d), level %d", text(row["title"]), id, num(row["level"]))
		ok := c.Found && c.Quest != nil && strings.Contains(firstLine(txt), head) && c.Quest.Classes == ch.class && c.Quest.Category == text(row["category"]) &&
			slices.Equal(gotPrev, prev) && slices.Equal(gotNext, next) && (i == 0 || strings.Contains(txt, "Comes after:")) && (i+1 == len(ch.ids) || strings.Contains(txt, "Leads to:")) &&
			(i+1 < len(ch.ids) || strings.Contains(txt, "No follow-up quest is listed"))
		got := fmt.Sprintf("%s; after %v; leads to %v", firstLine(txt), gotPrev, gotNext)
		want := fmt.Sprintf("%s, %s only; after %v; leads to %v", head, ch.class, prev, next)
		out = append(out, benchRow{fmt.Sprintf("%s quest %d: place in the %s chain", text(row["title"]), id, ch.class), got, want, chainSrc,
			resultIf(ok, "the title, level, class, the quest before or the quest after")})
	}
	finish(t, "advice-class-chain-"+part, "Offline class-quest chains: "+ch.class, out)
}

// TestOfflineBenchmarkClassQuestChainNoAnswer: with no QuestieDB the dataset names no order for a class's quests, so the
// advisor lists no quest before or after, and a class quest the dataset lacks is not invented. Two quests of one title
// stay two quests.
func TestOfflineBenchmarkClassQuestChainNoAnswer(t *testing.T) {
	ds := qbStore()
	var out []benchRow
	for _, id := range []int64{1508, 1740, 1801, 91208} {
		c := quests.OfWith(ds, nil, id)
		txt := quests.Render(c, "")
		out = append(out, benchRow{fmt.Sprintf("quest %d with no QuestieDB: before and after", id), fmt.Sprintf("%d before, %d after, next %d", len(c.Before), len(c.After), c.NextQuestID), "no quest_lines row and no precursor column in the dataset", noSourceSrc,
			resultIf(len(c.Before) == 0 && len(c.After) == 0 && c.NextQuestID == 0 && !strings.Contains(txt, "Comes after") && !strings.Contains(txt, "Leads to"), "an order was invented")})
	}
	// two quests with one title: asked by title the advisor lists both numbers rather than picking one
	id, _, cands := quests.ResolveWith(ds, nil, "Tome of the Cabal")
	out = append(out, benchRow{"Tome of the Cabal asked by its title", fmt.Sprintf("id %d, %v", id, cands), "quests_observed holds three quests of that title (1801, 1803, 1805)", noSourceSrc,
		resultIf(id == 0 && slices.Equal(cands, []string{"Tome of the Cabal (1801)", "Tome of the Cabal (1803)", "Tome of the Cabal (1805)"}), "one of the three was picked")})
	c := quests.OfWith(ds, chainIndex(t, ds), 999999)
	out = append(out, benchRow{"a class quest the dataset and the QuestieDB lack", quests.Render(c, ""), "no row in quests_observed or the QuestieDB", noSourceSrc, resultIf(!c.Found, "a quest was invented")})
	finish(t, "advice-class-chain-no-answer", "Offline class-quest chains: no answer in the dataset", out)
}

// ---- elites, rares and bosses ----

// rankCases are what a quest objective's creature classification means for a player: an elite, a rare elite and a boss
// are group content to be flagged, a plain rare and a normal creature are soloable.
var rankCases = []struct {
	name  string
	rank  int
	group bool
}{
	{"a normal creature", 0, false}, {"an elite", 1, true}, {"a rare elite", 2, true}, {"a boss", 3, true}, {"a rare", 4, false},
}

// TestOfflineBenchmarkEliteObjectiveFlag: the group flag the session gives a quest whose objective creature has each
// classification, outside any dungeon.
func TestOfflineBenchmarkEliteObjectiveFlag(t *testing.T) {
	var out []benchRow
	for _, tc := range rankCases {
		qx := &questie.Index{
			Quests: map[int]*questie.Quest{1: {ID: 1, Name: "Synthetic Hunt", Level: 20, Creatures: []questie.Ref{{ID: 10}}}},
			Npcs:   map[int]questie.Entity{10: {Name: "Synthetic Beast", Level: 20, Rank: tc.rank}},
		}
		level := 20
		c := character.Character{Name: "Benchtest", Faction: "Horde", Class: "WARRIOR", Level: &level, Quests: []character.Quest{{QuestID: 1, Title: "Synthetic Hunt", Level: &level}}}
		plan := route.Plan{Start: &route.Point{Map: 1, X: 50, Y: 50}}
		in := session.Live(c, store.New(store.ReaderSource{}), qx, plan, nil)
		got := in.Info.Group != nil && in.Info.Group(1)
		out = append(out, benchRow{fmt.Sprintf("objective is %s (classification %d): group flag", tc.name, tc.rank), fmt.Sprint(got), fmt.Sprint(tc.group), rankSrc, resultIf(got == tc.group, "an elite or boss was not flagged, or a rare was")})
	}
	finish(t, "advice-elite-objective-flag", "Offline elite objectives: the group flag", out)
}

// eliteQuests are quests players tagged as group or dungeon quests in quests_observed, and two with no tag.
var eliteQuests = []struct {
	id   int64
	want bool
}{{19, true}, {474, true}, {628, true}, {377, true}, {709, true}, {5, false}, {89, false}}

// TestOfflineBenchmarkEliteQuestTag: a quest in the log that players tagged group or dungeon is shown as an elite or
// group quest; an untagged quest is not.
func TestOfflineBenchmarkEliteQuestTag(t *testing.T) {
	ds := qbStore()
	var out []benchRow
	for _, tc := range eliteQuests {
		row := qbQuest(t, tc.id)
		lvl := int(num(row["level"]))
		done := true
		c := character.Character{Name: "Benchtest", Faction: "Horde", Class: "WARRIOR", Level: &lvl, Quests: []character.Quest{{QuestID: int(tc.id), Title: text(row["title"]), Level: &lvl, Complete: &done}}}
		steps := advice.WhatsNext(c, ds)
		got := len(steps) > 0 && steps[0].Elite
		tag := text(row["tag"])
		if tag == "" {
			tag = "no tag"
		}
		out = append(out, benchRow{fmt.Sprintf("%s (quest %d): elite or group quest", text(row["title"]), tc.id), fmt.Sprint(got), fmt.Sprintf("%v (tag: %s)", tc.want, tag), eliteQuestSrc,
			resultIf(got == tc.want, "a group or dungeon quest is not flagged, or an untagged one is")})
	}
	finish(t, "advice-elite-quest-tag", "Offline elite quests: the group tag", out)
}

// creatureCases are creatures the dataset names only through Everloot's drop rows: no classification, no level, no zone.
var creatureCases = []struct {
	item int64
	id   int64
	name string
}{{11098, 2266, "Hillsbrad Farmer"}, {4352, 7874, "Razorfen Thornweaver"}}

// TestOfflineBenchmarkCreatureLocation: where a creature that drops an item stands and what it is: the dataset names
// no zone, level or rank for it, so the advisor states none and calls it neither a rare nor an elite.
func TestOfflineBenchmarkCreatureLocation(t *testing.T) {
	ds := srcStore()
	words := regexp.MustCompile(`(?i)\b(rare|elite|spawn|spawns|patrol|boss)\b`)
	var out []benchRow
	for _, tc := range creatureCases {
		lvl := 40
		spots := farm.Spots(character.Character{Class: "MAGE", Level: &lvl}, ds, tc.item)
		npc, err := ds.Get("npcs", tc.id)
		if err != nil {
			t.Fatal(err)
		}
		var spot *farm.Spot
		for i := range spots {
			if spots[i].ID == tc.id {
				spot = &spots[i]
			}
		}
		if spot == nil {
			t.Fatalf("%s: no spot", tc.name)
		}
		ref := fmt.Sprintf("npcs: zone %q, levels %v-%v, classification %v", text(npc["zone_name"]), npc["min_level"], npc["max_level"], npc["classification"])
		got := fmt.Sprintf("zone %q, levels %v-%v, in range %v", spot.Zone, spot.MinLevel, spot.MaxLevel, spot.InRange)
		ok := spot.Name == tc.name && spot.Zone == "" && spot.MinLevel == nil && spot.MaxLevel == nil && !spot.InRange
		out = append(out, benchRow{tc.name + ": zone, level range and range check", got, ref, creatureSrc, resultIf(ok, "a zone or level was stated for a creature the dataset places nowhere")})
		txt := paths.For(horde(40), ds, tc.item, paths.Options{}).Text()
		out = append(out, benchRow{tc.name + ": no rare, elite or spawn claim", firstLine(strings.TrimSpace(strings.TrimPrefix(txt, "How to get"))), "no classification, spawn or timer column in npcs", noSourceSrc,
			resultIf(!words.MatchString(txt), "the advice calls the creature a rare, an elite or a spawn")})
	}
	out = append(out, benchRow{"zone of the creatures", "unknown for both", "npcs.zone_name is null for every row", creatureSrc, "differ (data): the dataset places no creature in a zone"})
	out = append(out, benchRow{"rare or elite status of the creatures", "none stated", "npcs.classification is null for an Everloot-named creature", rankSrc, "differ (data): the dataset has no rare or elite creature"})
	finish(t, "advice-creature-location", "Offline creature location: zone, level and rank", out)
}

var bossDungeons = []string{"Deadmines", "Wailing Caverns", "Shadowfang Keep", "Razorfen Kraul", "Uldaman", "Blackrock Depths"}

// TestOfflineBenchmarkDungeonBosses: the bosses of a dungeon and where its entrance stands, as the advisor words them
// beside instance_bosses and instances.
func TestOfflineBenchmarkDungeonBosses(t *testing.T) {
	ds := qbStore()
	var out []benchRow
	for _, name := range bossDungeons {
		inst := rowByName(t, ds, "instances", name)
		mid := num(inst["map_id"])
		rows, _ := ds.Filter("instance_bosses", store.Range{Column: "map_id", Min: mid, Max: mid})
		var want []string
		for _, r := range rows {
			if n := text(r["name"]); n != "" && !slices.Contains(want, n) {
				want = append(want, n)
			}
		}
		c := character.Character{Faction: "Horde", Class: "MAGE"} // no level: no gear is scored, only the dungeon's own rows are read
		p, _, err := dungeons.Named(c, ds, nil, name)
		if err != nil || p == nil {
			t.Fatalf("%s: %v", name, err)
		}
		txt := dungeons.RenderNamed(c, *p, 0)
		ok := slices.Equal(p.Bosses, want) && p.EntranceZone == text(inst["entrance_zone_name"]) && strings.Contains(txt, "entrance in "+text(inst["entrance_zone_name"])) && strings.Contains(txt, "All bosses: "+strings.Join(want, ", "))
		out = append(out, benchRow{name + ": bosses and entrance", fmt.Sprintf("%d bosses; entrance in %s", len(p.Bosses), p.EntranceZone), fmt.Sprintf("%d bosses; entrance in %s", len(want), text(inst["entrance_zone_name"])), bossSrc,
			resultIf(ok, "a boss or the entrance zone differs")})
		out = append(out, benchRow{name + ": no boss named an elite or a rare", firstLine(txt), "instance_bosses has no creature id or classification", noSourceSrc,
			resultIf(!strings.Contains(txt, "Elites:") && !strings.Contains(txt, "Bosses (Questie)"), "a creature was ranked with no source")})
	}
	finish(t, "advice-dungeon-bosses", "Offline dungeon bosses: names and entrance", out)
}

// TestOfflineBenchmarkRareNoAnswer: rares and elites the dataset does not hold. The advisor finds no creature by that
// name, names no zone for a creature it does not know, and states no rank for a creature with none.
func TestOfflineBenchmarkRareNoAnswer(t *testing.T) {
	ds := srcStore()
	var out []benchRow
	for _, name := range []string{"Synthetic Rare Elite Of The Pale Oak", "Synthetic Wandering Rare", "Synthetic Dungeon Elite Captain"} {
		hits, _, err := names.Find(ds, names.Npcs, name, 5)
		out = append(out, benchRow{name + ": found by name", fmt.Sprintf("%d creatures, %v", len(hits), err), "no npcs row of that name", noSourceSrc, resultIf(len(hits) == 0 && err == nil, "a creature was invented")})
	}
	// a creature id the dataset does not hold has no spot and no name
	spots := farm.Spots(character.Character{Class: "MAGE"}, ds, 999999999)
	out = append(out, benchRow{"an item no creature drops", fmt.Sprintf("%d spots", len(spots)), "no loot_sources row", noSourceSrc, resultIf(len(spots) == 0, "a creature was invented")})
	res, _, err := dungeons.Named(horde(30), ds, nil, "The Synthetic Vault Of Rares")
	out = append(out, benchRow{"a dungeon the dataset lacks", fmt.Sprint(res, err), "no instances row", noSourceSrc, resultIf(res == nil && err == nil, "a dungeon was invented")})
	finish(t, "advice-rare-no-answer", "Offline rares and elites: no answer in the dataset", out)
}

// ---- recipe sources ----

type recipeCase struct {
	recipe, kind string // a drop, a vendor, a trainer, both a trainer and an item, or an item with no place
	item         int64
	faction      string
	creature     string // the named creature of a drop
}

var dropRecipes = []recipeCase{
	{"Boots of the Enchanter", "drop", 4352, "Horde", "Razorfen Thornweaver"},
	{"Enchant Cloak - Lesser Shadow Resistance", "drop", 11098, "Horde", "Hillsbrad Farmer"},
	{"Golden Scale Cuirass", "unnamed drop", 3873, "Horde", ""},
	{"Elixir of Greater Strength", "unnamed drop", 9298, "Alliance", ""},
	{"Enchant Gloves - Advanced Mining", "unnamed drop", 11203, "Horde", ""},
	{"Surgical Tourniquet", "unnamed drop", 255730, "Alliance", ""},
}

// TestOfflineBenchmarkRecipeDropSource: a recipe whose recipe item drops: the creature the loot rows name is named when
// npcs holds its name, and a creature with no name is not named or guessed at; the skill needed is the recipe's.
func TestOfflineBenchmarkRecipeDropSource(t *testing.T) {
	ds := srcStore()
	var out []benchRow
	for _, tc := range dropRecipes {
		res, err := learn.Find(character.Character{Faction: tc.faction, Class: "MAGE"}, ds, nil, tc.recipe)
		if err != nil {
			t.Fatalf("%s: %v", tc.recipe, err)
		}
		itemRow, _ := ds.Get("items", tc.item)
		recs, _ := ds.SearchName("recipes", tc.recipe, 5)
		loot, _ := ds.Filter("loot_sources", store.Range{Column: "item_id", Min: tc.item, Max: tc.item})
		var named []string
		for _, l := range loot {
			if text(l["source_kind"]) != "creature" {
				continue
			}
			if n, err := ds.Get("npcs", num(l["source_id"])); err == nil && text(n["name"]) != "" {
				named = append(named, text(n["name"]))
			}
		}
		var got []string
		for _, w := range res.Ways {
			got = append(got, w.Kind+" "+w.Name)
		}
		skill := text(recs[0]["skill_line_name"])
		need := num(recs[0]["learn_skill"])
		ok := res.Gate.Skill == int(need) && strings.Contains(res.Text, fmt.Sprintf("It needs %s %d.", skill, need))
		if len(named) > 0 {
			ok = ok && len(res.Ways) == len(named) && res.Ways[0].Kind == learn.Drop && res.Ways[0].Name == named[0] && strings.HasPrefix(res.Text, "The recipe "+text(itemRow["name"])+" drops from "+named[0])
		} else {
			ok = ok && len(res.Ways) == 0 && strings.HasPrefix(res.Text, "It is learned from the recipe item "+text(itemRow["name"])) && !strings.Contains(res.Text, "trainer")
		}
		ref := fmt.Sprintf("learned from %s; %d creature rows, %d named; needs %s %d", text(itemRow["name"]), len(loot), len(named), skill, need)
		out = append(out, benchRow{fmt.Sprintf("%s (%s): where the recipe drops", tc.recipe, tc.faction), firstLine(res.Text) + fmt.Sprintf(" %v", got), ref, recipeOriginSrc,
			resultIf(ok, "a creature the dataset does not name, a trainer for an item recipe, or the wrong skill")})
		if len(named) == 0 && len(loot) > 0 {
			out = append(out, benchRow{tc.recipe + ": creature name", "no creature named", fmt.Sprintf("%d creatures drop it", len(loot)), recipeOriginSrc, "differ (data): npcs holds no name for the creature that drops the recipe item"})
		}
	}
	finish(t, "advice-recipe-drop", "Offline recipe source: drops", out)
}

var bothRecipes = []struct{ recipe, faction string }{
	{"Moonglow Boots", "Horde"}, {"Murloc Scale Shoes", "Alliance"}, {"Novice Arcanist's Sash", "Horde"}, {"Enchant Cloak - Minor Protection", "Alliance"},
}

// TestOfflineBenchmarkRecipeBothSource: a recipe a trainer teaches and a recipe item also teaches: the advisor offers the
// trainers the dataset names for the side first (the gate the trainer asks), then the recipe item, and never invents
// a vendor.
func TestOfflineBenchmarkRecipeBothSource(t *testing.T) {
	ds := qbStore()
	var out []benchRow
	for _, tc := range bothRecipes {
		res, err := learn.Find(character.Character{Faction: tc.faction, Class: "MAGE"}, ds, nil, tc.recipe)
		if err != nil {
			t.Fatalf("%s: %v", tc.recipe, err)
		}
		recs, _ := ds.SearchName("recipes", tc.recipe, 5)
		spell := num(recs[0]["spell_id"])
		ts, _ := ds.Filter("trainer_spells", store.Range{Column: "spell_id", Min: spell, Max: spell})
		named := 0
		for _, r := range ts {
			if row, err := ds.Get("trainers", num(r["npc_id"])); err == nil && text(row["name"]) != "" {
				named++
			}
		}
		ris, _ := ds.Filter("recipe_items", store.Range{Column: "teaches_spell_id", Min: spell, Max: spell})
		vend := 0
		for _, ri := range ris {
			vs, _ := ds.Filter("vendor_sources", store.Range{Column: "item_id", Min: num(ri["item_id"]), Max: num(ri["item_id"])})
			vend += len(vs)
		}
		kinds := map[string]bool{}
		for _, w := range res.Ways {
			kinds[w.Kind] = true
		}
		ok := recs[0]["learned_from"] == "both" && len(ris) > 0 && (vend > 0 || !strings.Contains(res.Text, "Buy the recipe"))
		if named > 0 {
			ok = ok && len(res.Ways) > 0 && res.Ways[0].Kind == learn.Trainer && kinds[learn.Trainer]
		} else {
			it, _ := ds.Get("items", num(ris[0]["item_id"]))
			ok = ok && len(res.Ways) == 0 && strings.HasPrefix(res.Text, "Ask "+wording.A(text(recs[0]["skill_line_name"]))+" trainer") && strings.Contains(res.Text, "also taught by the recipe item "+text(it["name"]))
		}
		ref := fmt.Sprintf("trainer and recipe item; %d trainers, %d named; %d vendor rows", len(ts), named, vend)
		out = append(out, benchRow{fmt.Sprintf("%s (%s): trainer and item source", tc.recipe, tc.faction), firstLine(res.Text), ref, recipeOriginSrc, resultIf(ok, "no trainer first, or a vendor the dataset does not name")})
		if len(ts) > 0 && named == 0 {
			out = append(out, benchRow{tc.recipe + ": trainer name", "no trainer named", fmt.Sprintf("%d trainers teach it", len(ts)), recipeOriginSrc, "differ (data): trainers.name is empty for every trainer of the recipe"})
		}
	}
	finish(t, "advice-recipe-both", "Offline recipe source: trainer and recipe item", out)
}

// TestOfflineBenchmarkRecipeNoSource: recipe sources the dataset does not hold. A quest that rewards a recipe is in no
// table, a vendor has no name, and a recipe the dataset lacks matches nothing: the advisor names no place for any.
func TestOfflineBenchmarkRecipeNoSource(t *testing.T) {
	ds := srcStore()
	var out []benchRow
	for _, name := range []string{"Boots of the Enchanter", "Surgical Tourniquet"} {
		res, err := learn.Find(character.Character{Faction: "Horde", Class: "MAGE"}, ds, nil, name)
		if err != nil {
			t.Fatal(err)
		}
		isQuest := false
		for _, w := range res.Ways {
			isQuest = isQuest || w.Kind == learn.Quest || w.Kind == learn.Vendor
		}
		out = append(out, benchRow{name + ": a quest or a vendor reward", firstLine(res.Text), "no quest reward row and no vendor row for the recipe item", noSourceSrc, resultIf(!isQuest, "a quest or vendor was named")})
	}
	for _, name := range []string{"Elixir Of The Unwritten Moon", "Pattern: Synthetic Cloak Of The Pale Oak", "Plans: Synthetic Sunken Blade"} {
		res, err := learn.Find(character.Character{Faction: "Horde", Class: "MAGE"}, ds, nil, name)
		out = append(out, benchRow{name + ": a recipe the dataset lacks", fmt.Sprintf("%d ways, %v", len(res.Ways), err), "no recipes row", noSourceSrc, resultIf(err != nil && len(res.Ways) == 0, "a recipe was invented")})
	}
	finish(t, "advice-recipe-no-source", "Offline recipe source: no answer in the dataset", out)
}

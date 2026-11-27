package paths_test

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/ask"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/dungeons"
	"github.com/WoW-Companion/advisor/internal/paths"
	"github.com/WoW-Companion/advisor/internal/store"
)

// The offline dungeon-advice benchmark: the level range the advisor states for a dungeon, the dungeons it recommends for a level
// and a side, the dungeon it understands from a player's own words, the boss it names for an item, the drop chance it words, and
// the key, attunement or quest it says a dungeon or raid needs, beside the dataset's rows and the guides, with no network. It
// reads the snapshots the other offline benchmarks read (places: instances, instance_bosses; usability: items) and the
// composed dungeon of offline_instances_benchmark_test.go: the dataset links no boss to a creature, so the cases of a boss's
// drops run on composed rows over real items. The dataset holds no key, attunement or lockout row, so a case that asks for one
// checks that the advisor says it is not known and names none. A difference the dataset's own rows cause is marked "(data)".
// make offline-benchmark runs it; WOWC_BENCH_REPORT receives the tables.

const (
	daRangeSrc = "dataset instances (min_level, max_level; level_source curated); Wowhead Classic dungeon level list (public; guides differ by a few levels)"
	daForSrc   = "dataset instances (levels, entrance zone); Wowhead Classic dungeon level list; the game's side of each entrance city"
	daNameSrc  = "dataset instances (name, instance_type); the names players use for a dungeon or raid (Warcraft Wiki, Wowhead)"
	daLootSrc  = "dataset loot_sources (kills, drops), instance_bosses (creature_id, difficulty_id); composed rows over real items"
	daGateSrc  = "dataset instances (no key, attunement or lockout column); Warcraft Wiki dungeon and raid entry pages"
	daNoneSrc  = "dataset instances, instance_bosses (no row answers it)"
)

// daGuide are dungeons with the level range of the Wowhead Classic list.
var daGuide = []struct {
	name   string
	lo, hi int64
}{
	{"Blackfathom Deeps", 24, 32}, {"Stormwind Stockade", 24, 32}, {"Gnomeregan", 29, 38}, {"Razorfen Kraul", 30, 40},
	{"Razorfen Downs", 40, 50}, {"Uldaman", 42, 52}, {"Zul'Farrak", 44, 54}, {"Maraudon", 46, 55},
	{"Sunken Temple", 50, 60}, {"Blackrock Depths", 52, 60}, {"Scholomance", 58, 60}, {"Stratholme", 58, 60},
}

func TestOfflineBenchmarkDungeonRangeGuideA(t *testing.T) { dungeonRangeGuide(t, "a", daGuide[:6]) }
func TestOfflineBenchmarkDungeonRangeGuideB(t *testing.T) { dungeonRangeGuide(t, "b", daGuide[6:]) }

// dungeonRangeGuide: the range the advisor words for a dungeon asked by name is the dataset's; beside the guide's it is a dataset-side
// difference when they differ.
func dungeonRangeGuide(t *testing.T, part string, cases []struct {
	name   string
	lo, hi int64
}) {
	ds := instStore()
	c := character.Character{Faction: "Horde", Class: "WARRIOR"}
	var out []benchRow
	for _, g := range cases {
		p, togo, err := dungeons.Named(c, ds, nil, g.name)
		if err != nil || p == nil || p.MinLevel == nil || p.MaxLevel == nil {
			t.Fatalf("%s: %v %v", g.name, p, err)
		}
		row := rowByName(t, ds, "instances", g.name)
		lo, hi := num(row["min_level"]), num(row["max_level"])
		want := fmt.Sprintf("about levels %d–%d", lo, hi)
		txt := dungeons.RenderNamed(c, *p, togo)
		out = append(out, benchRow{g.name + ": the level range", firstLine(txt), fmt.Sprintf("instances level %d-%d (%s)", lo, hi, text(row["level_source"])), daRangeSrc,
			resultIf(int64(*p.MinLevel) == lo && int64(*p.MaxLevel) == hi && strings.Contains(txt, want), "another range than the row")})
		res := "match"
		if lo != g.lo || hi != g.hi {
			res = "differ (data): the curated range is not the guide's range"
		}
		out = append(out, benchRow{g.name + ": the range in the guide", fmt.Sprintf("dataset %d-%d", lo, hi), fmt.Sprintf("Wowhead Classic: %d-%d", g.lo, g.hi), daRangeSrc, res})
	}
	finish(t, "advice-dungeon-range-guide-"+part, "Offline dungeon level ranges against the guide ("+part+")", out)
}

func TestOfflineBenchmarkDungeonRecommendationA(t *testing.T) {
	dungeonRecommendation(t, "a", []int{24, 33, 41})
}
func TestOfflineBenchmarkDungeonRecommendationB(t *testing.T) {
	dungeonRecommendation(t, "b", []int{49, 56})
}

// dungeonRecommendation: the dungeons recommended to an Alliance character of a level are the dataset's whose range fits it; beside
// the guide's list for the level it is a dataset-side difference when they differ.
func dungeonRecommendation(t *testing.T, part string, levels []int) {
	ds := instStore()
	var out []benchRow
	for _, level := range levels {
		c := character.Character{Faction: "Alliance", Class: "PRIEST", Level: &level}
		res, err := dungeons.Plan(c, ds)
		if err != nil {
			t.Fatal(err)
		}
		got := map[string]bool{}
		for _, p := range res.Picks {
			got[p.Name] = true
		}
		var rule, guide, advisor []string
		for _, g := range daGuide {
			row := rowByName(t, ds, "instances", g.name)
			lo, hi := num(row["min_level"]), num(row["max_level"])
			if int64(level) >= lo && int64(level-dungeons.LevelWindow) <= hi {
				rule = append(rule, g.name)
			}
			if int64(level) >= g.lo && int64(level-dungeons.LevelWindow) <= g.hi {
				guide = append(guide, g.name)
			}
			if got[g.name] {
				advisor = append(advisor, g.name)
			}
		}
		out = append(out, benchRow{fmt.Sprintf("level %d Alliance: the dungeons recommended", level), strings.Join(advisor, ", "), "instances rows whose range fits: " + strings.Join(rule, ", "), daForSrc,
			resultIf(strings.Join(advisor, ",") == strings.Join(rule, ","), "another set than the rows' ranges")})
		res2 := "match"
		if strings.Join(rule, ",") != strings.Join(guide, ",") {
			res2 = "differ (data): the curated ranges fit another set than the guide's"
		}
		out = append(out, benchRow{fmt.Sprintf("level %d Alliance: the guide's dungeons", level), strings.Join(rule, ", "), "Wowhead Classic: " + strings.Join(guide, ", "), daForSrc, res2})
	}
	finish(t, "advice-dungeon-recommendation-"+part, "Offline recommended dungeons for a level, against the guide ("+part+")", out)
}

type daSide struct {
	faction, dungeon string
	level            int
	offered          bool
}

func TestOfflineBenchmarkDungeonRecommendedSideA(t *testing.T) {
	dungeonRecommendedSide(t, "a", []daSide{{"Alliance", "Stormwind Stockade", 26, true}, {"Horde", "Stormwind Stockade", 26, false}})
}
func TestOfflineBenchmarkDungeonRecommendedSideB(t *testing.T) {
	dungeonRecommendedSide(t, "b", []daSide{{"Horde", "Ragefire Chasm", 15, true}, {"Alliance", "Ragefire Chasm", 15, false}})
}

// dungeonRecommendedSide: a side is offered the dungeons of its own entrance city and not the other side's.
func dungeonRecommendedSide(t *testing.T, part string, cases []daSide) {
	ds := instStore()
	var out []benchRow
	for _, tc := range cases {
		level := tc.level
		res, err := dungeons.Plan(character.Character{Faction: tc.faction, Class: "ROGUE", Level: &level}, ds)
		if err != nil {
			t.Fatal(err)
		}
		offered := false
		for _, p := range res.Picks {
			offered = offered || p.Name == tc.dungeon
		}
		out = append(out, benchRow{fmt.Sprintf("level %d %s: %s", tc.level, tc.faction, tc.dungeon), fmt.Sprintf("offered %v", offered), fmt.Sprintf("offered %v (the entrance is in a %s city)", tc.offered, map[bool]string{true: "friendly", false: "hostile"}[tc.offered]), daForSrc,
			resultIf(offered == tc.offered, "a dungeon of the other side was offered, or its own left out")})
	}
	finish(t, "advice-dungeon-recommended-side-"+part, "Offline recommended dungeons: each side's own ("+part+")", out)
}

// daAsks are a player's words and the dungeon or raid they name.
var daAsks = []struct{ question, want string }{
	{"What level is the stockade?", "Stormwind Stockade"},
	{"How do I get into BRD?", "Blackrock Depths"},
	{"Do I need an attunement for Onyxia?", "Onyxia's Lair"},
	{"What drops in the Temple of Atal'Hakkar?", "Sunken Temple"},
	{"Is UBRS worth running at 58?", "Blackrock Spire"},
	{"Which bosses are in Scarlet Monastery?", "Scarlet Monastery"},
	{"What is the level range for SFK?", "Shadowfang Keep"},
	{"Best loot from Lower Blackrock Spire?", "Blackrock Spire"},
	{"Is Maraudon worth doing at 45?", "Maraudon"},
}

// TestOfflineBenchmarkDungeonAskedByName: the dungeon or raid a player means by the name they use for it is found, and the range the
// advisor then gives is the dataset's.
func TestOfflineBenchmarkDungeonAskedByName(t *testing.T) {
	ds := instStore()
	c := character.Character{Faction: "Horde", Class: "WARRIOR"}
	var out []benchRow
	for _, a := range daAsks {
		cl := ask.Classify(a.question, ds, &c)
		row := rowByName(t, ds, "instances", a.want)
		ok := cl.Topic == ask.TopicDungeon && cl.Subject == a.want
		var txt string
		if ok {
			if p, togo, err := dungeons.Named(c, ds, nil, cl.Subject); err == nil && p != nil {
				txt = firstLine(dungeons.RenderNamed(c, *p, togo))
				ok = p.MinLevel != nil && int64(*p.MinLevel) == num(row["min_level"]) && int64(*p.MaxLevel) == num(row["max_level"])
			} else {
				ok = false
			}
		}
		out = append(out, benchRow{a.question, fmt.Sprintf("%s: %s", cl.Subject, txt), fmt.Sprintf("%s, instances level %d-%d", a.want, num(row["min_level"]), num(row["max_level"])), daNameSrc, resultIf(ok, "another dungeon, or none, was understood")})
	}
	finish(t, "advice-dungeon-asked-by-name", "Offline dungeon names: what a player's words mean", out)
}

// ---- boss loot ----

// daExpect is the way of the boss path for a chance: often above 15%, sometimes from 5% up to it, none below.
func daExpect(ch float64) string {
	switch {
	case ch > paths.Often:
		return "often"
	case ch >= paths.Sometimes:
		return "sometimes"
	}
	return ""
}

var daBossLine = regexp.MustCompile(`Drops (often|sometimes) from (.+) in Synthetic Hollow Keep`)

// TestOfflineBenchmarkBossForItem: the boss the advisor names for an item, and how often it says it drops, are the dataset's loot rows
// (drops of the loot windows) read by the thresholds the advisor documents; a rare drop is not offered.
func TestOfflineBenchmarkBossForItem(t *testing.T) {
	ds := synthStore()
	var out []benchRow
	for _, d := range synthDrops {
		txt := paths.For(horde(40), ds, d.itemID, paths.Options{}).Text()
		ch := 100 * float64(d.drop) / float64(d.kills)
		want := daExpect(ch)
		m := daBossLine.FindStringSubmatch(txt)
		ok := (want == "" && m == nil) || (m != nil && m[1] == want && m[2] == d.boss)
		got := "no dungeon path"
		if m != nil {
			got = m[0]
		}
		ref := fmt.Sprintf("%s drops %d in %d loot windows = %.1f%%: %s", d.boss, d.drop, d.kills, ch, map[bool]string{true: "named, " + want, false: "not offered (under 5%)"}[want != ""])
		out = append(out, benchRow{d.item + ": the boss that drops it", got, ref, daLootSrc, resultIf(ok, "another boss or chance word, or a rare drop offered")})
	}
	finish(t, "advice-dungeon-boss-for-item", "Offline boss loot: the boss named for an item and its chance word", out)
}

// TestOfflineBenchmarkBossForItemWindow: the boss path is offered only to a character within three levels of the dungeon's range.
func TestOfflineBenchmarkBossForItemWindow(t *testing.T) {
	ds := synthStore()
	var out []benchRow
	for _, tc := range []struct {
		level   int
		offered bool
	}{{34, false}, {35, true}, {44, true}, {51, true}, {52, false}} {
		txt := paths.For(horde(tc.level), ds, iSteelHelm, paths.Options{}).Text()
		got := daBossLine.MatchString(txt)
		out = append(out, benchRow{fmt.Sprintf("level %d: the boss path for a 25%% drop", tc.level), fmt.Sprintf("offered %v", got), fmt.Sprintf("offered %v: the dungeon is levels 38-48, a window of 3", tc.offered), daLootSrc,
			resultIf(got == tc.offered, "the path ignored the dungeon's range")})
	}
	finish(t, "advice-dungeon-boss-for-item-window", "Offline boss loot: the level window of the boss path", out)
}

// TestOfflineBenchmarkBossForItemAlternate: a boss of an alternate mode of the dungeon is not offered as the dungeon's boss, the same
// as it is not in the dungeon's own boss list.
func TestOfflineBenchmarkBossForItemAlternate(t *testing.T) {
	ds := synthStore()
	txt := paths.For(horde(40), ds, iMithrilShoulder, paths.Options{}).Text()
	p := synthKeep(t)
	listed := false
	for _, b := range p.Bosses {
		listed = listed || b == "Synthetic Alternate Boss"
	}
	out := []benchRow{
		{"Heavy Mithril Shoulder: an alternate-mode boss's drop", firstLine(txt) + "; boss path " + fmt.Sprint(strings.Contains(txt, "Synthetic Alternate Boss")), "not a boss of the dungeon's own list (difficulty 198)", daLootSrc,
			resultIf(!strings.Contains(txt, "Synthetic Alternate Boss"), "a drop of an alternate mode was named as the dungeon's")},
		{"Synthetic Hollow Keep: the alternate-mode boss in the boss list", fmt.Sprintf("listed %v", listed), "not listed", daLootSrc, resultIf(!listed, "an alternate-mode boss is listed")},
	}
	finish(t, "advice-dungeon-boss-alternate", "Offline boss loot: an alternate mode's boss", out)
}

// TestOfflineBenchmarkBossChanceUnknown: a drop the dataset has no kill count for is worded with no chance, and says the chance is not known; a
// known chance is worded as the dataset's.
func TestOfflineBenchmarkBossChanceUnknown(t *testing.T) {
	const gloves = 7927 // Ornate Mithril Gloves
	row := func(kv ...any) store.Row {
		r := store.Row{}
		for i := 0; i < len(kv); i += 2 {
			r[kv[i].(string)] = kv[i+1]
		}
		return r
	}
	ds := instOverlay{synthStore(), map[string][]store.Row{"loot_sources": {
		row("item_id", int64(gloves), "source_kind", "creature", "source_id", synthCreature["Synthetic Warden Alpha"], "kills", int64(0), "drops", int64(0), "reporters", int64(1), "tier", "unverified"),
	}}}
	level := 40
	c := character.Character{Faction: "Horde", Class: "WARRIOR", Level: &level}
	p, _, err := dungeons.Named(c, ds, nil, "Synthetic Hollow Keep")
	if err != nil || p == nil {
		t.Fatalf("%v %v", p, err)
	}
	var out []benchRow
	for _, d := range p.Drops {
		if d.ItemID != gloves && d.ItemID != iSteelHelm {
			continue
		}
		line := d.String()
		if d.ItemID == gloves {
			out = append(out, benchRow{"Ornate Mithril Gloves: the chance with no kills recorded", line, "kills 0: no chance is computable", daLootSrc, resultIf(d.Chance == nil && strings.HasSuffix(line, "drop chance not known") && !strings.Contains(line, "%"), "a chance was worded or the gap not said")})
		} else {
			out = append(out, benchRow{"Steel Plate Helm: the chance with kills recorded", line, "10 drops in 40 loot windows = 25%", daLootSrc, resultIf(strings.HasSuffix(line, "drops 25% of kills"), "another chance than the rows'")})
		}
	}
	if len(out) != 2 {
		t.Fatalf("drops %d", len(p.Drops))
	}
	finish(t, "advice-dungeon-boss-chance-unknown", "Offline boss loot: a drop chance the dataset cannot give", out)
}

// ---- keys, attunements and quests that gate entry ----

var daGates = []struct{ question, dungeon, needs string }{
	{"Do I need an attunement for BWL?", "Blackwing Lair", "the Blackhand's Command quest chain (Warcraft Wiki)"},
	{"How do I get into Onyxia's Lair?", "Onyxia's Lair", "the Drakkisath's Brand / Onyxia attunement quest chain (Warcraft Wiki)"},
	{"How do I get attuned for Molten Core?", "Molten Core", "the Attunement to the Core quest (Warcraft Wiki)"},
	{"Do I need a key for UBRS?", "Blackrock Spire", "the Seal of Ascension for the upper spire (Warcraft Wiki)"},
	{"What key do I need for Scholo?", "Scholomance", "the Skeleton Key, or a rogue's lockpicking (Warcraft Wiki)"},
	{"What do I need to enter the Stockade?", "Stormwind Stockade", "no key or quest; an Alliance city entrance (Warcraft Wiki)"},
}

var daNamed = regexp.MustCompile(`(?i)blackhand's command|drakkisath's brand|drakefire|lothos|attunement to the core|seal of ascension|skeleton key|shadowforge|scarlet key|crescent key|lockpick`)

// TestOfflineBenchmarkEntryAsked: a player asks what a dungeon or raid needs to enter: the advisor understands the name, says the key,
// attunement or quest is not known, and names none; the requirement itself is a dataset-side gap.
func TestOfflineBenchmarkEntryAsked(t *testing.T) {
	ds := instStore()
	c := character.Character{Faction: "Horde", Class: "WARRIOR"}
	var out []benchRow
	for _, g := range daGates {
		cl := ask.Classify(g.question, ds, &c)
		var txt string
		ok := cl.Topic == ask.TopicDungeon && cl.Subject == g.dungeon
		if ok {
			p, togo, err := dungeons.Named(c, ds, nil, cl.Subject)
			ok = err == nil && p != nil
			if ok {
				txt = dungeons.RenderNamed(c, *p, togo)
			}
		}
		line := ""
		for _, l := range strings.Split(txt, "\n") {
			if strings.Contains(l, "not known yet") && (strings.Contains(l, "Key or entry requirement") || strings.Contains(l, "Lockout and attunement")) {
				line = strings.TrimSpace(l)
			}
		}
		out = append(out, benchRow{g.question, orNone(line), "the entry is a gap in the dataset: say it is not known, name none", daGateSrc, resultIf(ok && line != "" && !daNamed.MatchString(txt), "the name was not understood, or a requirement was named")})
		out = append(out, benchRow{g.dungeon + ": the key, attunement or quest", "none named", g.needs, daGateSrc, "differ (data): the dataset links no key, attunement or quest to a dungeon or raid"})
	}
	finish(t, "advice-dungeon-entry-asked", "Offline dungeon and raid entry: asked in a player's words", out)
}

// ---- no answer ----

// TestOfflineBenchmarkDungeonNameNoAnswer: a part of a name that fits several dungeons, and a dungeon the dataset does not hold, get no
// dungeon: the advisor names the candidates or says none, and never picks one.
func TestOfflineBenchmarkDungeonNameNoAnswer(t *testing.T) {
	ds := instStore()
	c := character.Character{Faction: "Horde", Class: "WARRIOR"}
	var out []benchRow
	for _, tc := range []struct {
		name  string
		among []string
	}{
		{"Blackrock", []string{"Blackrock Depths", "Blackrock Spire"}},
		{"Zul", []string{"Zul'Farrak", "Zul'Gurub"}},
		{"Razorfen", []string{"Razorfen Kraul", "Razorfen Downs"}},
		{"Ahn'Qiraj", []string{"Ahn'Qiraj Temple", "Ruins of Ahn'Qiraj"}},
	} {
		p, _, err := dungeons.Named(c, ds, nil, tc.name)
		ok := p == nil && err != nil
		for _, a := range tc.among {
			ok = ok && strings.Contains(err.Error(), a)
		}
		cl := ask.Classify("what drops in "+tc.name+"?", ds, &c)
		out = append(out, benchRow{tc.name + ": which dungeon", fmt.Sprintf("picked %v; asked topic %q; %v", p != nil, cl.Subject, err), "fits " + strings.Join(tc.among, " and ") + ": none is guessed", daNameSrc,
			resultIf(ok && cl.Subject == "", "a dungeon was picked for a name that fits several")})
	}
	for _, name := range []string{"Karazhan", "Black Temple", "Ruins of Lordaeron"} {
		p, _, err := dungeons.Named(c, ds, nil, name)
		out = append(out, benchRow{name + ": described", fmt.Sprintf("%v, %v", p != nil, err), "no row the advisor can describe: no entrance, group size or level", daNoneSrc, resultIf(p == nil && err == nil, "a dungeon was described from no row")})
	}
	finish(t, "advice-dungeon-name-no-answer", "Offline dungeon names: no answer, and none guessed", out)
}

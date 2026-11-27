package paths_test

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/ask"
	"github.com/WoW-Companion/advisor/internal/farm"
	"github.com/WoW-Companion/advisor/internal/names"
	"github.com/WoW-Companion/advisor/internal/paths"
	"github.com/WoW-Companion/advisor/internal/store"
)

// The offline rare and elite mob benchmark: where a rare or an elite stands, its level and the group it needs, what it drops and from which creature, and what makes it spawn or when it
// respawns, as the advisor words it for a question in the player's own words, beside the dataset's npcs and loot_sources rows and the game's guides, with no network. It reads
// testdata/drops_snapshot (npcs, loot_sources, items, names: the creatures players saw drop items). The dataset holds no creature marked a rare or an elite (npcs.classification is 0 or empty), no zone,
// spawn point, respawn timer or spawn condition for any creature, and a level for a handful only, so a case it cannot answer checks that the advisor says "the data holds none" and names no zone, level,
// group size or timer from memory; the zone, level and timer of the game's rares are listed beside it as "(data)". A creature the dataset names is described from its row: what it holds, "not held" for
// the rest, and the items players saw it drop with the share of kills. Before ADV-311 only questions that put "rare spawn", "rare elite" or "respawn" in the words got the no-data line: a question
// that asked "what zone is the rare X in", "how do I find the rare elite X", "how many people do I need for the elite X", "what triggers the spawn of X" or "is the rare always up" got none, and a creature
// asked for by name ("where is Black Ooze", "what does Razorfen Thornweaver drop") got no row at all. A rare item, a rare herb and an elite quest are no creature and get no such line. make offline-benchmark
// runs it; WOWC_BENCH_REPORT receives the tables.

const (
	reLocSrc   = "dataset npcs (zone_name, zone_id null for every creature of the snapshot), loot_sources (names only); Warcraft Wiki creature pages (a rare's zone)"
	reLevelSrc = "dataset npcs (min_level, max_level, classification: null or 0 for a named elite); Warcraft Wiki creature pages (level and classification: Elite, Rare Elite, Rare)"
	reDropSrc  = "dataset loot_sources (source_kind creature: drops, kills), npcs (name), items (name); Warcraft Wiki, Loot tables"
	reSpawnSrc = "dataset npcs (no respawn, spawn point or spawn condition column); Warcraft Wiki creature pages (respawn timer, time of day, placeholder, trigger)"
	reGuardSrc = "dataset items (quality), quests; the quality or quest named, not a creature"
	reDataGap  = "differ (data): the dataset has none, so the advisor states none"
)

const reRareLine = "Rare spawns and named elites:"

// reLimit is the no-data line for a question about a rare or an elite: present, and it says "say so" and "from memory".
func reLimit(q string) (string, bool) {
	f := ask.LimitFacts(q)
	return firstLineMatching(f, reRareLine), strings.Contains(f, reRareLine) && strings.Contains(f, "Say so") && strings.Contains(f, "from memory")
}

// reStated is a place, level, group size or timer in an answer's words: a number of minutes or hours, a level, a group of players, or the name of a zone of the game.
var reStated = regexp.MustCompile(`(?i)\b(?:elwynn|westfall|duskwood|loch modan|redridge|barrens|stranglethorn|hillsbrad|stonetalon)\b|\blevel \d+\b|\b\d+ ?(?:min(?:ute)?s?|hours?|h)\b|\b\d+[- ](?:man|player|people)\b`)

// reCreature is the dataset's row of the named creature.
func reCreature(t *testing.T, id int64, name string) store.Row {
	t.Helper()
	return rdNpc(t, rdCreature{id, name})
}

// reDrops is the items players saw the creature drop that the items table names, best share first: the same reading CreatureFacts makes of loot_sources.
func reDrops(t *testing.T, id int64) []string {
	t.Helper()
	rows, err := rdStore().Filter("loot_sources", store.Range{Column: "source_id", Min: id, Max: id})
	if err != nil {
		t.Fatal(err)
	}
	type d struct {
		item         string
		drops, kills int64
	}
	var all []d
	for _, r := range rows {
		it, err := rdStore().Get("items", num(r["item_id"]))
		if text(r["source_kind"]) != "creature" || num(r["kills"]) == 0 || err != nil || it == nil || text(it["name"]) == "" {
			continue
		}
		all = append(all, d{text(it["name"]), num(r["drops"]), num(r["kills"])})
	}
	sort.SliceStable(all, func(i, j int) bool {
		if x, y := all[i].drops*all[j].kills, all[j].drops*all[i].kills; x != y {
			return x > y
		}
		return all[i].item < all[j].item
	})
	var out []string
	for _, x := range all {
		out = append(out, fmt.Sprintf("%s (about %.0f%% of %d kills reported)", x.item, 100*float64(x.drops)/float64(x.kills), x.kills))
	}
	return out
}

var reLocations = []struct{ q, game string }{
	{"what zone is the rare Gruff Swiftbite in?", "Gruff Swiftbite: a rare in Westfall"},
	{"where do I find rares in Westfall?", "Westfall's rares: Gruff Swiftbite, Thuros Lightfingers"},
	{"how do I find the rare elite Narg the Taskmaster?", "Narg the Taskmaster: a rare in Elwynn Forest"},
	{"where can I find the rare Mother Fang?", "Mother Fang: a rare in Elwynn Forest"},
	{"which area is the rare Lord Condar located in?", "Lord Condar: a rare in Loch Modan"},
	{"where is the rare elite Mor'Ladim found?", "Mor'Ladim: a rare in Duskwood"},
	{"how do I camp a rare for its loot?", "a rare stands at fixed spots and respawns on a long timer"},
}

// TestOfflineBenchmarkRareLocationAsked: where a rare stands, asked in the player's words ("what zone is the rare X in", "where do I find rares in Westfall", "how do I find the rare elite X",
// "which area is the rare X located in"), reaches the line that the dataset holds no zone or spawn point for any creature, and no zone is named from memory. Before the fix only the wordings that
// said "rare spawn", "rare elite" or "where does X spawn" did.
func TestOfflineBenchmarkRareLocationAsked(t *testing.T) {
	var out []benchRow
	for _, tc := range reLocations {
		line, ok := reLimit(tc.q)
		out = append(out, benchRow{fmt.Sprintf("%q: where the rare stands", tc.q), firstLine(line), "game: " + tc.game + "; dataset: no zone for any creature", reLocSrc,
			resultIf(ok && reStated.FindString(line) == "", "no line that says the data holds no zone or spawn point, or a zone was named")})
	}
	out = append(out, benchRow{"the zone of each rare above", "none stated", "game: each of these rares stands in one zone", reLocSrc, reDataGap})
	finish(t, "advice-rare-location-asked", "Offline rare mobs: where a rare stands, asked in the player's words", out)
}

var reNamed = []struct {
	q    string
	c    rdCreature
	game string
}{
	{"where is Black Ooze?", rdCreature{1032, "Black Ooze"}, "an ooze of the Barrens' caves (Warcraft Wiki creature page)"},
	{"where can I find Death's Head Geomancer?", rdCreature{7335, "Death's Head Geomancer"}, "a Razorfen Kraul creature (Warcraft Wiki creature page)"},
	{"which faction is Galak Wrangler?", rdCreature{4093, "Galak Wrangler"}, "a centaur of the Galak clan, hostile to the Horde (Warcraft Wiki creature page)"},
	{"what zone is Saltstone Crystalhide in?", rdCreature{4151, "Saltstone Crystalhide"}, "a creature of the open world (Warcraft Wiki creature page)"},
	{"where does Highperch Consort spawn?", rdCreature{4109, "Highperch Consort"}, "a harpy of Thousand Needles (Warcraft Wiki creature page)"},
}

// TestOfflineBenchmarkNamedCreatureFacts: a creature asked for by name is described from the dataset's row: the zone, level, faction and rank the row holds, "not held" for the rest, and nothing from memory.
// Before the fix the advisor had no row for it and the model was left to place it from memory.
func TestOfflineBenchmarkNamedCreatureFacts(t *testing.T) {
	var out []benchRow
	for _, tc := range reNamed {
		r := reCreature(t, tc.c.id, tc.c.name)
		got := ask.CreatureFacts(rdStore(), tc.q)
		ok := strings.Contains(got, fmt.Sprintf("%s (creature %d): level not held; zone not held; faction not held; rank (rare or elite) not held", tc.c.name, tc.c.id)) &&
			r["zone_name"] == nil && r["min_level"] == nil && r["faction_template_id"] == nil && reStated.FindString(got) == ""
		out = append(out, benchRow{fmt.Sprintf("%q: the creature's row", tc.q), firstLineMatching(got, tc.c.name), fmt.Sprintf("dataset npcs %d: zone %v, level %v, faction %v, rank %v; game: %s", tc.c.id, r["zone_name"], r["min_level"], r["faction_template_id"], r["classification"], tc.game), reLocSrc,
			resultIf(ok, "the creature's row is not described, or a zone, level or faction was stated that the row does not hold")})
	}
	// a creature the dataset holds a level and a faction for is described with them
	r := reCreature(t, 14878, "Jubling")
	got := ask.CreatureFacts(rdStore(), "what level is the Jubling?")
	out = append(out, benchRow{"\"what level is the Jubling?\": a creature with a level", firstLineMatching(got, "Jubling"), fmt.Sprintf("dataset npcs 14878: level %d-%d, faction template %d", num(r["min_level"]), num(r["max_level"]), num(r["faction_template_id"])), reLevelSrc,
		resultIf(strings.Contains(got, fmt.Sprintf("Jubling (creature 14878): level %d; zone not held; faction template %d;", num(r["min_level"]), num(r["faction_template_id"]))), "the level or faction of the row was not stated")})
	finish(t, "advice-rare-named-creature", "Offline rare and named creatures: a creature asked for by name", out)
}

var reLevels = []struct{ q, game string }{
	{"what level is the rare Mother Fang?", "Mother Fang: a rare at the level of its zone"},
	{"how strong is the rare Vultros?", "Vultros: a rare, strong for its zone"},
	{"what level is the boss Edwin VanCleef?", "Edwin VanCleef: the last boss of the Deadmines, an elite"},
	{"how many people do I need for the elite Hogger?", "Hogger: an elite of Elwynn Forest, a party's fight"},
	{"do I need a group to kill the elite in Stonetalon?", "an elite needs a group; a rare elite needs one too"},
	{"can I solo a rare elite at my level?", "a rare elite is an elite: a group's fight"},
	{"which elites should I avoid at level 40?", "elites of every zone: a group's fight"},
}

// TestOfflineBenchmarkEliteLevelAndGroup: the level of an elite or a boss and the group it needs, asked in the player's words ("what level is the boss X", "how many people do I need for the elite X",
// "do I need a group to kill the elite in Stonetalon"), reaches the line that the dataset marks no creature an elite and holds no level or group size for one, and no level or number of players is stated.
func TestOfflineBenchmarkEliteLevelAndGroup(t *testing.T) {
	var out []benchRow
	for _, tc := range reLevels {
		line, ok := reLimit(tc.q)
		out = append(out, benchRow{fmt.Sprintf("%q: the elite's level and group", tc.q), firstLine(line), "game: " + tc.game + "; dataset: no elite flag, no level, no group size for a creature", reLevelSrc,
			resultIf(ok && reStated.FindString(line) == "", "no line that says the data holds no elite level or group size, or one was stated")})
	}
	out = append(out, benchRow{"the level and group size of each elite above", "none stated", "game: one level and a group of five to ten for each elite", reLevelSrc, reDataGap})
	finish(t, "advice-elite-level-group", "Offline elite mobs: level and group needed, asked in the player's words", out)
}

// TestOfflineBenchmarkEliteDungeonGroup: the elites of a dungeon are a party's fight: the dungeon's facts give the group size the dataset holds (5 players) and the level range, not a level for each elite.
func TestOfflineBenchmarkEliteDungeonGroup(t *testing.T) {
	var out []benchRow
	for _, tc := range []struct {
		q, name, want string
		lo, hi        int
	}{
		{"how many players for Razorfen Downs?", "Razorfen Downs", "5 players", 35, 45},
		{"what level is Wailing Caverns?", "Wailing Caverns", "5 players", 15, 25},
		{"how many people do I need for Zul'Farrak?", "Zul'Farrak", "5 players", 44, 54},
	} {
		cl, txt, p := ieGround(t, ieChar(60), tc.q)
		ok := ieShape(cl, tc.name) && p != nil && strings.Contains(firstLineMatching(txt, "players"), tc.want) && p.MinLevel != nil && *p.MinLevel == tc.lo && *p.MaxLevel == tc.hi
		out = append(out, benchRow{fmt.Sprintf("%q: the group a dungeon's elites need", tc.q), ieDesc(cl) + "; " + tc.want, fmt.Sprintf("game: %s, %d-%d, a party of five; dataset instances level %d-%d", tc.name, tc.lo, tc.hi, tc.lo, tc.hi), ieSizeSrc,
			resultIf(ok, "the dungeon was not named, or its group size or level range is not the game's")})
	}
	finish(t, "advice-elite-dungeon-group", "Offline elite mobs: the group a dungeon's elites need", out)
}

// TestOfflineBenchmarkRareEliteDropCreature: what a creature drops, asked by the creature's name: the items players saw it drop and the share of kills, from loot_sources, as the dataset holds them.
func TestOfflineBenchmarkRareEliteDropCreature(t *testing.T) {
	var out []benchRow
	for _, tc := range []struct {
		q string
		c rdCreature
	}{
		{"What does Razorfen Thornweaver drop?", rdCreature{7874, "Razorfen Thornweaver"}},
		{"what does the elite Razorfen Battleguard drop?", rdCreature{7873, "Razorfen Battleguard"}},
		{"which items does Galak Wrangler drop?", rdCreature{4093, "Galak Wrangler"}},
		{"what loot does Death's Head Geomancer give?", rdCreature{7335, "Death's Head Geomancer"}},
	} {
		reCreature(t, tc.c.id, tc.c.name)
		want := reDrops(t, tc.c.id)
		got := ask.CreatureFacts(rdStore(), tc.q)
		ok := len(want) > 0
		for i, w := range want {
			if i < 4 {
				ok = ok && strings.Contains(got, w)
			}
		}
		out = append(out, benchRow{fmt.Sprintf("%q: what the creature drops", tc.q), firstLineMatching(got, "players saw it drop"), fmt.Sprintf("dataset loot_sources: %s", strings.Join(want[:min(4, len(want))], ", ")), reDropSrc,
			resultIf(ok, "the creature's drops or their shares are not the dataset's")})
	}
	finish(t, "advice-rare-elite-drop-creature", "Offline rare and elite drops: what a creature drops", out)
}

// TestOfflineBenchmarkRareEliteDropItem: where an item that a few named creatures drop is recommended: the creature the dataset names and its share of kills, no zone or level, and the farm list says "zone unknown".
func TestOfflineBenchmarkRareEliteDropItem(t *testing.T) {
	var out []benchRow
	for _, id := range []int64{7450, 4633, 276160} {
		it := itemsRow(t, rdStore(), id)
		c := levChar("WARRIOR", 40)
		res := paths.For(c, rdStore(), id, paths.Options{})
		best, share := rdBestCreature(t, id)
		spots := farm.Spots(c, rdStore(), id)
		rendered := farm.Render(text(it["name"]), spots, farm.Crafted(rdStore(), id), 5)
		ok := !res.Dependable() && strings.Contains(res.Reason, fmt.Sprintf("about %.0f%%", share)) && strings.Contains(res.Reason, "no level or zone") && len(spots) > 0 && strings.Contains(rendered, "level unknown, zone unknown")
		out = append(out, benchRow{fmt.Sprintf("%s: which creature drops it and where, level 40", text(it["name"])), res.Reason, fmt.Sprintf("dataset: %q is the best source at %.0f%% of its kills; no level or zone for it", best, share), reDropSrc,
			resultIf(ok, "the share is not the dataset's, or a zone or level was named for a creature the data does not place")})
	}
	for _, id := range []int64{5637} {
		it := itemsRow(t, rdStore(), id)
		res := paths.For(levChar("WARRIOR", 40), rdStore(), id, paths.Options{})
		rows, _ := rdStore().Filter("loot_sources", store.Range{Column: "item_id", Min: id, Max: id})
		crs := map[int64]bool{}
		for _, r := range rows {
			crs[num(r["source_id"])] = true
		}
		out = append(out, benchRow{fmt.Sprintf("%s: how to get it, level 40", text(it["name"])), res.Reason, fmt.Sprintf("dataset: %d creatures carry it (more than %d): a random drop", len(crs), paths.WorldDropMany), reDropSrc,
			resultIf(len(crs) > paths.WorldDropMany && !res.Dependable() && strings.Contains(res.Reason, "random drop"), "an item many creatures carry was sent to one creature")})
	}
	// the names of the creatures come from players' drop reports, some in another language: the advisor gives them as the dataset does
	it := itemsRow(t, rdStore(), 3916)
	res := paths.For(levChar("WARRIOR", 40), rdStore(), 3916, paths.Options{})
	out = append(out, benchRow{fmt.Sprintf("%s: the creature's name", text(it["name"])), res.Reason, "game: the Skullsplitter trolls, named in English in the game's English client", reDropSrc,
		"differ (data): the dataset's names for these creatures come from drop reports in French (Everloot sourceName), and the advisor gives the dataset's name"})
	finish(t, "advice-rare-elite-drop-item", "Offline rare and elite drops: where an item is recommended", out)
}

var reSpawn = []struct{ q, game string }{
	{"how long until Vultros respawns?", "a rare respawns on a long timer after its death"},
	{"what triggers the spawn of Mor'Ladim?", "a rare spawns on its own timer; some need a quest or a kill"},
	{"what is the spawn condition for Mother Fang?", "the creature page names the condition, if any"},
	{"when does Gruff Swiftbite spawn?", "the creature page names its spawn time"},
	{"does the rare spawn only at night?", "some rares stand only at night or in rain"},
	{"is the rare always up?", "a rare is up after its timer, not always"},
	{"can I camp the rare for its drop?", "a rare's respawn timer decides if it is worth camping"},
}

// TestOfflineBenchmarkRareSpawnCondition: when a rare spawns and what makes it, asked in the player's words ("what triggers the spawn of X", "what is the spawn condition for X", "when does X spawn", "is the rare always up",
// "can I camp the rare"), reaches the line that the dataset holds no respawn timer or spawn condition, and no minutes or hours are stated.
func TestOfflineBenchmarkRareSpawnCondition(t *testing.T) {
	var out []benchRow
	for _, tc := range reSpawn {
		line, ok := reLimit(tc.q)
		f := ask.LimitFacts(tc.q)
		ok = ok && strings.Contains(f, "respawn timer") && strings.Contains(f, "spawn condition")
		out = append(out, benchRow{fmt.Sprintf("%q: when and why it spawns", tc.q), firstLine(line), "game: " + tc.game + "; dataset: no respawn timer or spawn condition", reSpawnSrc,
			resultIf(ok && reStated.FindString(line) == "", "no line that says the data holds no respawn timer or spawn condition, or one was stated")})
	}
	out = append(out, benchRow{"the respawn timer of any rare", "none stated", "game: each rare has its own timer", reSpawnSrc, reDataGap})
	out = append(out, benchRow{"the spawn condition of any rare", "none stated", "game: a night-only, quest-triggered or placeholder spawn", reSpawnSrc, reDataGap})
	finish(t, "advice-rare-spawn-condition", "Offline rare mobs: respawn and spawn conditions, asked in the player's words", out)
}

// TestOfflineBenchmarkRareNoAnswerNames: a rare the dataset does not name is not found, and an elite or a rare used in another sense gets no creature line: a rare item, a rare herb, an elite quest, elite gear.
func TestOfflineBenchmarkRareNoAnswerNames(t *testing.T) {
	var out []benchRow
	for _, n := range []string{"Hogger", "Mother Fang", "Vultros", "Gruff Swiftbite"} {
		hits, _, err := names.Find(rdStore(), names.Npcs, n, 3)
		got := ask.CreatureFacts(rdStore(), "where is "+n+"?")
		out = append(out, benchRow{fmt.Sprintf("\"where is %s?\": a creature the dataset does not name", n), fmt.Sprintf("%d creatures, %q", len(hits), got), "game: a creature with a zone (Warcraft Wiki); dataset: no npcs row of that name", reLocSrc,
			resultIf(len(hits) == 0 && err == nil && got == "", "a creature was invented")})
	}
	for _, q := range []string{
		"what is a rare quality item worth?", "where do I find rare herbs?", "what level for the elite quests in Stonetalon?", "is this an elite quest?",
		"where can I find elite gear?", "where do I buy a rare mount?", "what quests are elite for me?", "which rare recipe patterns should I buy?",
	} {
		f := ask.LimitFacts(q)
		out = append(out, benchRow{fmt.Sprintf("%q: no creature meant", q), fmt.Sprintf("rare line: %v", strings.Contains(f, reRareLine)), "no rare or elite creature asked for: the item's quality, a herb, a quest, gear", reGuardSrc,
			resultIf(!strings.Contains(f, reRareLine), "a rare item, herb, quest or gear was met with the rare creature line")})
	}
	finish(t, "advice-rare-no-answer-names", "Offline rare mobs: creatures the dataset does not name and words that name none", out)
}

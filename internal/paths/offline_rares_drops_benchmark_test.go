package paths_test

import (
	"fmt"
	"math"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/WoW-Companion/advisor/internal/ask"
	"github.com/WoW-Companion/advisor/internal/names"
	"github.com/WoW-Companion/advisor/internal/paths"
	"github.com/WoW-Companion/advisor/internal/store"
)

// The offline rare-spawn, named-elite, world-drop and item-set benchmark: what the advisor says about where a rare or a named elite stands and what level and faction it has, where
// an item drops and how often, and which items make up an item set and what a set gives, beside the dataset's rows, with no network. testdata/drops_snapshot
// (testdata/drops_snapshot/build.sh) holds the real loot_sources, npcs and items rows of a spread of drops; the item_sets rows are those of testdata/items_snapshot. The dataset holds no
// creature that is a rare or an elite (npcs.classification is 0 or empty), no zone for any creature, no level or faction for a named creature, no spawn point or respawn timer, no set
// bonus and no world drop flag, so the cases it cannot answer check that the advisor says so and states none of them. A difference the dataset's own rows cause is marked "(data)".
// make offline-benchmark runs it; WOWC_BENCH_REPORT receives the tables.

const (
	rdDir      = "../../testdata/drops_snapshot"
	rdNpcSrc   = "dataset npcs (classification, min_level, max_level, faction_template_id, zone_name: none for a rare or an elite); Warcraft Wiki creature pages (a rare's zone, level and faction)"
	rdDropSrc  = "dataset loot_sources (source_kind creature: drops, kills, tier), npcs (name, min_level, max_level); Warcraft Wiki, Loot (a world drop is carried by many creatures)"
	rdSetSrc   = "dataset item_sets (set_id, name, item_id), items (name)"
	rdNoneSrc  = "the dataset has no such row, column or table (no rare or elite creature, no zone, spawn or respawn column, no set bonus table, no world drop flag)"
	rdNoSetSrc = "the dataset has no item_sets row of that name"
)

var (
	rdOnce  sync.Once
	rdCache store.DataStore
)

// rdStore reads the snapshot once for the package: the tables are decoded on first use and the tests share them.
func rdStore() store.DataStore {
	rdOnce.Do(func() { rdCache = store.NewDir(rdDir) })
	return rdCache
}

// ---- rare spawns and named elites ----

// rdCreature is a creature of the snapshot by id, named as the dataset names it.
type rdCreature struct {
	id   int64
	name string
}

func rdNpc(t *testing.T, c rdCreature) store.Row {
	t.Helper()
	r, err := rdStore().Get("npcs", c.id)
	if err != nil || r == nil || text(r["name"]) != c.name {
		t.Fatalf("creature %d: the snapshot holds %v (%v), the case names %q", c.id, r["name"], err, c.name)
	}
	return r
}

var rdRares = []struct {
	c rdCreature
	q string
}{
	{rdCreature{7873, "Razorfen Battleguard"}, "Where does the elite creature Razorfen Battleguard spawn and what zone is it in?"},
	{rdCreature{4151, "Saltstone Crystalhide"}, "What level is Saltstone Crystalhide, is it a rare elite?"},
	{rdCreature{4093, "Galak Wrangler"}, "Which faction is Galak Wrangler, and where can I find this rare spawn?"},
	{rdCreature{7335, "Death's Head Geomancer"}, "How long until Death's Head Geomancer respawns?"},
	{rdCreature{4109, "Highperch Consort"}, "Where does the rare Highperch Consort spawn?"},
	{rdCreature{1032, "Black Ooze"}, "What is the spawn timer of the named elite Black Ooze, and what level is it?"},
}

// TestOfflineBenchmarkRareNamedEliteFacts: creatures the dataset names as loot sources and nothing else (no rank, level, faction or zone): a question on one's zone, level, faction, rank or respawn
// gets the line that says the data holds none, and the creature's row holds none of them.
func TestOfflineBenchmarkRareNamedEliteFacts(t *testing.T) {
	var out []benchRow
	for _, tc := range rdRares {
		r := rdNpc(t, tc.c)
		held := r["zone_name"] != nil || r["min_level"] != nil || r["faction_template_id"] != nil || num(r["classification"]) > 0
		got := ask.LimitFacts(tc.q)
		out = append(out, benchRow{tc.q, firstLineMatching(got, "Rare spawns and named elites:"), fmt.Sprintf("dataset npcs %d: rank %v, level %v, faction %v, zone %v", tc.c.id, r["classification"], r["min_level"], r["faction_template_id"], r["zone_name"]), rdNpcSrc,
			resultIf(!held && strings.Contains(got, "Rare spawns and named elites:") && strings.Contains(got, "Say so"), "no line that says the data has no rank, level, faction, zone or respawn for the creature, or the dataset holds one")})
	}
	finish(t, "advice-rare-named-elite-facts", "Offline rare spawns and named elites: zone, level, faction, rank and respawn", out)
}

// TestOfflineBenchmarkRareNamedEliteDrops: what the dataset does hold of such a creature is what it drops; the advisor reports that, with the creature's name and the share players saw.
func TestOfflineBenchmarkRareNamedEliteDrops(t *testing.T) {
	var out []benchRow
	for _, id := range []int64{4352, 3916} {
		it := itemsRow(t, rdStore(), id)
		c := levChar("WARRIOR", 40)
		res := paths.For(c, rdStore(), id, paths.Options{})
		best, share := rdBestCreature(t, id)
		out = append(out, benchRow{fmt.Sprintf("%s: which creature drops it and how often, level 40", text(it["name"])), res.Text(), fmt.Sprintf("dataset: %s, %.0f%% of its kills (confirmed), no level, no zone", best, share), rdDropSrc,
			resultIf(strings.Contains(res.Text(), best) && strings.Contains(res.Text(), fmt.Sprintf("about %.0f%%", share)), "the creature or the share is not the dataset's")})
	}
	finish(t, "advice-rare-named-elite-drops", "Offline named creatures: what they drop and how often", out)
}

// rdBestCreature is the creature the dataset's loot_sources rows give the item the best share of kills, and that share in percent.
func rdBestCreature(t *testing.T, item int64) (string, float64) {
	t.Helper()
	rows, err := rdStore().Filter("loot_sources", store.Range{Column: "item_id", Min: item, Max: item})
	if err != nil || len(rows) == 0 {
		t.Fatalf("loot_sources of item %d: %v", item, err)
	}
	drops, kills := map[int64]int64{}, map[int64]int64{}
	for _, r := range rows {
		if text(r["source_kind"]) == "creature" {
			drops[num(r["source_id"])] += num(r["drops"])
			kills[num(r["source_id"])] += num(r["kills"])
		}
	}
	var bestID int64
	best := -1.0
	for id := range drops {
		if s := 100 * float64(drops[id]) / float64(kills[id]); s > best || (s == best && id < bestID) {
			best, bestID = s, id
		}
	}
	n, _ := rdStore().Get("npcs", bestID)
	return text(n["name"]), math.Round(best)
}

// ---- world drops and drop sources ----

// TestOfflineBenchmarkWorldDropMany: an item carried by many creatures is a random world drop: the advisor says so, names no creature to farm and states no fixed chance.
func TestOfflineBenchmarkWorldDropMany(t *testing.T) {
	var out []benchRow
	for _, id := range []int64{4306, 2592, 5805, 1708} {
		it := itemsRow(t, rdStore(), id)
		rows, _ := rdStore().Filter("loot_sources", store.Range{Column: "item_id", Min: id, Max: id})
		crs := map[int64]bool{}
		for _, r := range rows {
			crs[num(r["source_id"])] = true
		}
		res := paths.For(levChar("WARRIOR", 40), rdStore(), id, paths.Options{})
		ok := len(crs) > paths.WorldDropMany && !res.Dependable() && strings.Contains(res.Reason, "random drop")
		out = append(out, benchRow{fmt.Sprintf("%s: how to get it at level 40", text(it["name"])), strings.ReplaceAll(res.Text(), "\n", " / "), fmt.Sprintf("dataset: %d creatures carry it (more than %d): a random drop", len(crs), paths.WorldDropMany), rdDropSrc,
			resultIf(ok && !regexp.MustCompile(`\d%`).MatchString(res.Text()), "not called a random drop, or a creature or a chance was stated")})
	}
	finish(t, "advice-world-drop-many", "Offline world drops: items many creatures carry", out)
}

// TestOfflineBenchmarkWorldDropCommon: a common-quality or better item carried by more creatures than a named drop holds, that nobody can craft or buy, is called a common world drop sold on the Auction House.
func TestOfflineBenchmarkWorldDropCommon(t *testing.T) {
	var out []benchRow
	for _, id := range []int64{1529} {
		it := itemsRow(t, rdStore(), id)
		res := paths.For(levChar("WARRIOR", 40), rdStore(), id, paths.Options{})
		best, ok := res.Best()
		out = append(out, benchRow{fmt.Sprintf("%s: how to get it at level 40", text(it["name"])), strings.ReplaceAll(res.Text(), "\n", " / "), "dataset: seven creatures carry it, quality uncommon, not bound: a world drop that is on the Auction House", rdDropSrc,
			resultIf(ok && best.Kind == paths.Auction && strings.Contains(best.Line, "world drop"), "not a world drop on the Auction House")})
	}
	finish(t, "advice-world-drop-common", "Offline world drops: a world drop for the Auction House", out)
}

// TestOfflineBenchmarkDropRateStated: items a few creatures carry at a fair share of kills are not called a random drop: the advisor names the creatures and the share the dataset holds (drops over kills),
// and says the dataset has no level or zone for them. This is the regression for the answer "a random drop, not worth chasing" on a drop seen on 23% to 26% of kills.
func TestOfflineBenchmarkDropRateStated(t *testing.T) {
	var out []benchRow
	for _, id := range []int64{1524, 3916, 4352, 4614} {
		it := itemsRow(t, rdStore(), id)
		res := paths.For(levChar("WARRIOR", 40), rdStore(), id, paths.Options{})
		best, share := rdBestCreature(t, id)
		want := fmt.Sprintf("about %.0f%%", share)
		ok := strings.Contains(res.Reason, want) && !strings.Contains(res.Reason, "a random drop,") && strings.Contains(res.Reason, "no level or zone")
		if best != "" {
			ok = ok && strings.Contains(res.Reason, best)
		}
		out = append(out, benchRow{fmt.Sprintf("%s: why no way is given at level 40", text(it["name"])), res.Reason, fmt.Sprintf("dataset: %q drops it on %.0f%% of its kills; no creature level or zone", best, share), rdDropSrc,
			resultIf(ok, "called a random drop, or the creature or the share left out")})
	}
	finish(t, "advice-drop-rate-stated", "Offline drop sources: the share of kills the dataset holds", out)
}

// TestOfflineBenchmarkDropLevelGap: the creatures that carry these items have no level in the dataset, so the advisor cannot say whether they suit a character; the dataset-side gap is listed.
func TestOfflineBenchmarkDropLevelGap(t *testing.T) {
	var out []benchRow
	for _, id := range []int64{1524, 3916} {
		it := itemsRow(t, rdStore(), id)
		rows, _ := rdStore().Filter("loot_sources", store.Range{Column: "item_id", Min: id, Max: id})
		withLevel := 0
		for _, r := range rows {
			if n, err := rdStore().Get("npcs", num(r["source_id"])); err == nil && n["min_level"] != nil {
				withLevel++
			}
		}
		out = append(out, benchRow{fmt.Sprintf("%s: the level of the creatures that drop it", text(it["name"])), "none stated", fmt.Sprintf("dataset npcs: %d of %d carriers have a level", withLevel, len(rows)), rdNpcSrc,
			"differ (data): the dataset gives these creatures no level or zone, so no path (farm here at your level) can be given"})
	}
	finish(t, "advice-drop-level-gap", "Offline drop sources: dataset-side gaps", out)
}

// ---- item sets ----

var rdSets = []struct {
	q    string
	name string
	id   int64
}{
	{"which pieces make up the Dreadmist Raiment set?", "Dreadmist Raiment", 183},
	{"what is in the Battlegear of Might set?", "Battlegear of Might", 209},
	{"which items are in the Giantstalker set?", "Giantstalker Armor", 206},
	{"list the pieces of the Champion's Battlegear set", "Champion's Battlegear", 281},
	{"which set does [Stormshroud Pants] belong to?", "Stormshroud Armor", 142},
	{"what pieces does the Volcanic Armor set have?", "Volcanic Armor", 141},
}

// TestOfflineBenchmarkItemSetMembership: the pieces of a set as the advisor lists them are the item_sets rows of the set: the count, the set's id and the names of the pieces the items table holds.
func TestOfflineBenchmarkItemSetMembership(t *testing.T) {
	var out []benchRow
	ds := itemsStore()
	for _, tc := range rdSets {
		rows, err := ds.Filter("item_sets", store.Range{Column: "set_id", Min: tc.id, Max: tc.id})
		if err != nil || len(rows) == 0 || text(rows[0]["name"]) != tc.name {
			t.Fatalf("set %d: the snapshot holds %d rows named %q (%v), the case names %q", tc.id, len(rows), text(rows[0]["name"]), err, tc.name)
		}
		known := 0
		var named []string
		for _, r := range rows {
			if it, err := ds.Get("items", num(r["item_id"])); err == nil && text(it["name"]) != "" {
				known++
				named = append(named, text(it["name"]))
			}
		}
		got := ask.SetFacts(ds, tc.q)
		ok := strings.Contains(got, fmt.Sprintf("%s (set %d): %d pieces in the dataset", tc.name, tc.id, len(rows)))
		for _, n := range named {
			ok = ok && strings.Contains(got, n)
		}
		if known < len(rows) {
			ok = ok && strings.Contains(got, fmt.Sprintf("%d of its items are not in the items table", len(rows)-known))
		}
		out = append(out, benchRow{tc.q, firstLineMatching(got, tc.name), fmt.Sprintf("dataset item_sets %d: %s, %d pieces, %d named in items", tc.id, tc.name, len(rows), known), rdSetSrc, resultIf(ok, "the set's id, piece count or piece names are not the dataset's")})
	}
	finish(t, "advice-item-set-membership", "Offline item sets: the pieces of a set", out)
}

// TestOfflineBenchmarkItemSetSharedName: two sets that share a name (The Elements, ids 187 and 1678) are both listed, each with its own pieces and id, not merged into one.
func TestOfflineBenchmarkItemSetSharedName(t *testing.T) {
	ds := itemsStore()
	got := ask.SetFacts(ds, "which pieces are in The Elements set?")
	var out []benchRow
	var want []string
	for _, id := range []int64{187, 1678} {
		rows, _ := ds.Filter("item_sets", store.Range{Column: "set_id", Min: id, Max: id})
		want = append(want, fmt.Sprintf("The Elements (set %d): %d pieces", id, len(rows)))
	}
	ok := true
	for _, w := range want {
		ok = ok && strings.Contains(got, w)
	}
	out = append(out, benchRow{"which pieces are in The Elements set?", strings.ReplaceAll(got, "\n", " / "), "dataset item_sets: two set ids named The Elements, 8 pieces each", rdSetSrc, resultIf(ok, "one of the two sets of that name is missing")})
	finish(t, "advice-item-set-shared-name", "Offline item sets: two sets with one name", out)
}

// TestOfflineBenchmarkItemSetNoBonus: the dataset holds no set bonus, piece threshold or tier, whichever way the question is put; each gets the line that says so, and the pieces of the set are still listed.
func TestOfflineBenchmarkItemSetNoBonus(t *testing.T) {
	ds := itemsStore()
	var out []benchRow
	for _, q := range []string{
		"what is the set bonus of the Dreadmist Raiment set?",
		"what do I get with 5 pieces of Giantstalker Armor?",
		"which tier set is Battlegear of Might, tier 1?",
		"what does the Volcanic Armor set give with all three pieces?",
		"what bonus do I get for wearing the full Champion's Battlegear set?",
	} {
		got := ask.LimitFacts(q)
		_, errTable := ds.Get("set_bonuses", 1)
		out = append(out, benchRow{q, firstLineMatching(got, "Item set bonuses:"), "dataset: no set bonus, threshold or tier column; no set_bonuses table", rdNoneSrc,
			resultIf(errTable != nil && strings.Contains(got, "Item set bonuses:") && strings.Contains(got, "Say so"), "no line that says the data holds no set bonus, or the dataset holds one")})
	}
	finish(t, "advice-item-set-no-bonus", "Offline item sets: no bonus in the dataset", out)
}

// ---- no answer in the dataset ----

// TestOfflineBenchmarkRareDropNoAnswer: what the dataset does not hold about rares, elites and drops gets the line that says so, whichever way the question is put, and a question the advisor can answer
// from the dataset gets none.
func TestOfflineBenchmarkRareDropNoAnswer(t *testing.T) {
	var out []benchRow
	for _, tc := range []struct{ q, topic string }{
		{"when does the rare spawn respawn after I kill it?", "Rare spawns and named elites:"},
		{"is Silk Cloth a world drop and what is its drop rate?", "Drop chances and world drops:"},
		{"does the elite always drop a guaranteed green item?", "Drop chances and world drops:"},
		{"what is the drop chance of Skullsplitter Tusk?", "Drop chances and world drops:"},
		{"what is the drop table of Hillsbrad Farmer, which world drops can it give?", "Drop chances and world drops:"},
		{"which zone has the most rare elites for a level 30?", "Rare spawns and named elites:"},
	} {
		got := ask.LimitFacts(tc.q)
		out = append(out, benchRow{tc.q, firstLineMatching(got, tc.topic), "dataset: no rank, spawn, respawn, drop table or world drop flag; drops and kills only", rdNoneSrc,
			resultIf(strings.Contains(got, tc.topic) && strings.Contains(got, "Say so"), "no line that says the data has none")})
	}
	for _, q := range []string{"what is a rare quality item worth?", "which quest should I do next?", "where do I buy Silk Cloth?", "what quests are elite for me?"} {
		got := ask.LimitFacts(q)
		unrelated := !strings.Contains(got, "Rare spawns and named elites:") && !strings.Contains(got, "Drop chances and world drops:") && !strings.Contains(got, "Item set bonuses:")
		out = append(out, benchRow{q, "no rare, drop or set line", "no limit line: not a question about a rare creature, a drop chance or a set bonus", rdNoneSrc, resultIf(unrelated, "a no-data line on a question that is not about those")})
	}
	finish(t, "advice-rare-drop-no-answer", "Offline rares, drops and sets: what the dataset does not hold", out)
}

// TestOfflineBenchmarkItemSetNoAnswer: a set the dataset does not hold, and a question that names no set, list no pieces and invent none.
func TestOfflineBenchmarkItemSetNoAnswer(t *testing.T) {
	ds := itemsStore()
	var out []benchRow
	for _, q := range []string{"which pieces make up the Pale Oak Raiment set?", "what is in the Thunderforged Vestments set?", "what pieces does my set have?"} {
		got := ask.SetFacts(ds, q)
		out = append(out, benchRow{q, fmt.Sprintf("%q", got), "no item_sets row of that name: no pieces listed", rdNoSetSrc, resultIf(got == "", "pieces were listed for a set the dataset does not hold")})
	}
	hits, _, _ := names.Find(ds, names.ItemSets, "Pale Oak Raiment", 3)
	out = append(out, benchRow{"Pale Oak Raiment: a set row", fmt.Sprint(len(hits)), "no item_sets row", rdNoSetSrc, resultIf(len(hits) == 0, "a set was invented")})
	finish(t, "advice-item-set-no-answer", "Offline item sets: sets the dataset does not hold", out)
}

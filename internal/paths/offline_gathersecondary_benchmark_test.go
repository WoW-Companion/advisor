package paths_test

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/ask"
	"github.com/WoW-Companion/advisor/internal/store"
)

// The offline gathering and secondary-profession facts benchmark: the facts the advisor puts in front of the model when a player asks
// which skill an ore or herb node needs, what skill a creature needs to be skinned, what a zone's fishing catches are, and what skill a
// Cooking or First Aid recipe needs, beside the dataset's rows and the game's own tables, with no network. It reads
// testdata/tradeskills_snapshot (gathering_nodes, profession_notes, recipes, trainer_spells, recipe_items) and testdata/companions_snapshot
// (fishing_catches, items). The dataset holds no fishing skill, fishing pool, node row for most ores or herbs or skinning table, so
// the no-answer cases check that the advisor says so and states none. A difference the dataset's own rows cause is marked "(data)".
// make offline-benchmark runs it; WOWC_BENCH_REPORT receives the tables.

const (
	gsNodeSrc = "dataset gathering_nodes (skill_required, zone_name); game: Warcraft Wiki Mining and Herbalism skill tables"
	gsSkinSrc = "dataset profession_notes (section rules, subject skinning); game: Warcraft Wiki Skinning (level 11-19: 10 per level above 10; level 20 and up: 5 per level)"
	gsFishSrc = "dataset fishing_catches (zone_name, item_id, casts, rate), items (name)"
	gsFoodSrc = "dataset recipes (learn_skill, learn_skill_source), trainer_spells, recipe_items, profession_notes ranks; game: Warcraft Wiki Cooking and First Aid recipe tables"
	gsNoneSrc = "the dataset has no such row, column or table"
)

// ---- ore and herb nodes ----

var gsNodeCases = []struct{ node, question string }{
	{"Copper Vein", "what mining skill do I need for copper vein?"}, {"Tin Vein", "what skill do I need to mine tin?"},
	{"Silver Vein", "what mining skill is needed for silver?"}, {"Iron Deposit", "which mining skill does an Iron Deposit need and where is it?"},
	{"Peacebloom", "what herbalism skill do I need to pick Peacebloom?"}, {"Earthroot", "which herbalism skill do I need for Earthroot and where does it grow?"},
	{"Briarthorn", "what herbalism skill does Briarthorn need?"},
}

// TestOfflineBenchmarkGatherFactsNodes: the facts for a question that names a node give the lowest skill the dataset has for it and
// only zones the dataset lists, beside the game's skill for the node.
func TestOfflineBenchmarkGatherFactsNodes(t *testing.T) {
	ds := tsStore()
	var out []benchRow
	for _, tc := range gsNodeCases {
		facts := ask.GatherFacts(ds, tc.question)
		line := ""
		for _, l := range strings.Split(facts, "\n") {
			if strings.HasPrefix(l, "- "+tc.node+":") {
				line = l
			}
		}
		var skill int64
		var zones []string
		for _, r := range tsRows(t, "gathering_nodes", "name", tc.node) {
			if s := num(r["skill_required"]); s > 0 && (skill == 0 || s < skill) {
				skill = s
			}
			if z := text(r["zone_name"]); !slices.Contains(zones, z) {
				zones = append(zones, z)
			}
		}
		ok := line != "" && strings.Contains(line, fmt.Sprintf(" %d; found in ", skill)) && strings.Contains(facts, "name no skill or zone from memory")
		for _, z := range zones[:min(len(zones), 3)] {
			ok = ok && (strings.Contains(line, z) || len(zones) > 6)
		}
		res := resultIf(ok, "the node's skill or zones missing from the facts, or no word that other nodes have none")
		if guide := gnGuide[tc.node]; res == "match" && guide != skill {
			res = fmt.Sprintf("differ (data): the dataset gives %d, the game %d", skill, guide)
		}
		out = append(out, benchRow{tc.node + ": skill and zones", line, fmt.Sprintf("dataset %d in %d zone(s); game %d", skill, len(zones), gnGuide[tc.node]), gsNodeSrc, res})
	}
	finish(t, "advice-gather-facts-nodes", "Offline gathering facts: the skill an ore or herb node needs", out)
}

// ---- skinning ----

var gsSkinCases = []struct {
	level      int
	dataset    string // the figure the dataset's rule text states for the level
	gameNeeded int
}{
	{11, "10 for level 11", 10}, {15, "level 15 needs 50", 50}, {19, "level 19 needs 90", 90}, {30, "level 30 creature needs 150", 150}, {45, "five times the creature's level", 225},
}

// TestOfflineBenchmarkGatherFactsSkinning: the skill a creature of a level needs is the dataset's rule worked out for the level, never
// the model's memory; the rule's own examples and the game's table are beside it.
func TestOfflineBenchmarkGatherFactsSkinning(t *testing.T) {
	ds := tsStore()
	var out []benchRow
	var rule string
	for _, r := range tsRows(t, "profession_notes", "section", "rules") {
		if text(r["subject"]) == "skinning" {
			rule = text(r["text"])
		}
	}
	for _, tc := range gsSkinCases {
		facts := ask.GatherFacts(ds, fmt.Sprintf("what skinning skill do I need for a level %d creature?", tc.level))
		want := fmt.Sprintf("A level %d creature needs Skinning %d.", tc.level, tc.gameNeeded)
		ok := strings.Contains(facts, want) && strings.Contains(facts, rule) && (tc.dataset == "" || strings.Contains(rule, tc.dataset))
		out = append(out, benchRow{fmt.Sprintf("skinning a level %d creature", tc.level), firstLineMatching(facts, "A level"), fmt.Sprintf("dataset rule %q; game %d", tc.dataset, tc.gameNeeded), gsSkinSrc, resultIf(ok, "the skill for the level, or the dataset's rule missing")})
	}
	finish(t, "advice-gather-facts-skinning", "Offline gathering facts: the skill a creature needs to be skinned", out)
}

func lastLine(s string) string {
	l := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimPrefix(l[len(l)-1], "- ")
}

func firstLineMatching(s, sub string) string {
	for _, l := range strings.Split(s, "\n") {
		if strings.Contains(l, sub) {
			return strings.TrimPrefix(l, "- ")
		}
	}
	return ""
}

// ---- fishing ----

var gsFishZones = []string{"The Barrens", "Stranglethorn Vale", "Orgrimmar", "Hillsbrad Foothills", "Elwynn Forest", "Dun Morogh"}

// TestOfflineBenchmarkGatherFactsFishing: a zone named in a fishing question gets the dataset's catches for it, best rate first, with
// the casts they come from and the item's name, and the fishing skill line says the data holds none.
func TestOfflineBenchmarkGatherFactsFishing(t *testing.T) {
	ds := compStore()
	var out []benchRow
	for _, z := range gsFishZones {
		facts := ask.GatherFacts(ds, "what can I catch fishing in "+z+"?")
		rows := compRows(t, "fishing_catches", "zone_id", num(rowByCol(t, "fishing_catches", "zone_name", z)["zone_id"]))
		slices.SortStableFunc(rows, func(a, b store.Row) int {
			x, _ := a["rate"].(float64)
			y, _ := b["rate"].(float64)
			switch {
			case x > y:
				return -1
			case x < y:
				return 1
			}
			return int(num(a["item_id"]) - num(b["item_id"]))
		})
		var want []string
		for _, r := range rows[:min(5, len(rows))] {
			it, _ := ds.Get("items", num(r["item_id"]))
			want = append(want, fmt.Sprintf("%s (%.0f%% of %d casts)", text(it["name"]), 100*r["rate"].(float64), num(r["casts"])))
		}
		line := firstLineMatching(facts, "Catches reported in "+z)
		ok := line == fmt.Sprintf("Catches reported in %s, best rate first: %s.", z, strings.Join(want, ", ")) && strings.Contains(facts, "holds no fishing skill")
		out = append(out, benchRow{z + ": what to catch", line, strings.Join(want, ", "), gsFishSrc, resultIf(ok, "the catches, their order or their rates")})
	}
	finish(t, "advice-gather-facts-fishing", "Offline fishing facts: the catches of a zone", out)
}

func rowByCol(t *testing.T, table, col, val string) store.Row {
	t.Helper()
	rows, err := compStore().Filter(table, store.Range{Column: col, Min: val, Max: val})
	if err != nil || len(rows) == 0 {
		t.Fatalf("%s: no row with %s %q (%v)", table, col, val, err)
	}
	return rows[0]
}

// ---- Cooking and First Aid recipe skills ----

var gsRecipeCases = []struct {
	recipe, prof string
	game         int
}{
	{"Spiced Wolf Meat", "Cooking", 10}, {"Scorpid Surprise", "Cooking", 20}, {"Smoked Bear Meat", "Cooking", 40}, {"Crab Cake", "Cooking", 75},
	{"Goblin Deviled Clams", "Cooking", 125}, {"Jungle Stew", "Cooking", 175},
	{"Heavy Linen Bandage", "First Aid", 40}, {"Wool Bandage", "First Aid", 80}, {"Heavy Wool Bandage", "First Aid", 115}, {"Silk Bandage", "First Aid", 150},
	{"Heavy Mageweave Bandage", "First Aid", 240}, {"Runecloth Bandage", "First Aid", 260},
}

// TestOfflineBenchmarkRecipeSkillFacts: the skill a named recipe needs is the dataset's (with "about" when the dataset's own estimate),
// with the rank that skill belongs to, beside the game's table. The three higher bandages are estimates the dataset puts 40 to 60 under
// the game's skill: a difference the dataset causes.
func TestOfflineBenchmarkRecipeSkillFacts(t *testing.T) {
	ds := consVendors{tsStore()}
	var out []benchRow
	for _, tc := range gsRecipeCases {
		facts := ask.RecipeSkillFacts(ds, "what skill do I need to learn "+tc.recipe+"?")
		line := firstLineMatching(facts, tc.recipe+" ("+tc.prof+")")
		rec := ccRecipe(t, ds, lgItemID(t, ds, tc.recipe), tc.prof)
		need := num(rec["learn_skill"])
		about := map[bool]string{true: "about ", false: ""}[text(rec["learn_skill_source"]) == "estimate"]
		ok := strings.Contains(line, fmt.Sprintf("It needs %s%s %d.", about, tc.prof, need)) && strings.Count(facts, "\n- ") == 1
		if name, floor, level := rlRank(t, ds, need); name != "" {
			ok = ok && strings.Contains(line, fmt.Sprintf("Skill above %d needs the %s rank, trained from character level %d.", floor, name, level))
		}
		res := resultIf(ok, "the skill, its estimate wording or the rank")
		if res == "match" && need != int64(tc.game) {
			res = fmt.Sprintf("differ (data): the dataset gives %s%d (%s), the game %d", about, need, text(rec["learn_skill_source"]), tc.game)
		}
		out = append(out, benchRow{tc.recipe + " (" + tc.prof + "): the skill to learn it", line, fmt.Sprintf("dataset %d (%s); game %d", need, text(rec["learn_skill_source"]), tc.game), gsFoodSrc, res})
	}
	finish(t, "advice-recipe-skill-facts", "Offline Cooking and First Aid facts: the skill a recipe needs", out)
}

// ---- no answer ----

// TestOfflineBenchmarkGatherFactsNoAnswer: what the dataset does not hold is said and not guessed: a fishing skill for a zone, a fishing
// pool, an ore the dataset lists no node for (Thorium, and Mithril which it names only in French), the skill for a creature too low
// for the skinning rule, and a recipe the dataset has no row for.
func TestOfflineBenchmarkGatherFactsNoAnswer(t *testing.T) {
	ts, comp := tsStore(), compStore()
	type nc struct {
		q     string
		facts string
		must  []string // phrases the facts must carry
		never *regexp.Regexp
		ref   string
	}
	cases := []nc{
		{"what fishing skill do I need to fish in Stranglethorn Vale?", ask.GatherFacts(comp, "what fishing skill do I need to fish in Stranglethorn Vale?"), []string{"holds no fishing skill", "name no skill number"}, regexp.MustCompile(`(?i)fishing (skill )?\d`), "no fishing skill column"},
		{"which fishing pool has Oily Blackmouth?", ask.GatherFacts(comp, "which fishing pool has Oily Blackmouth?"), []string{"no fishing pool locations"}, regexp.MustCompile(`(?i)pool at`), "no fishing pool table"},
		{"what mining skill does Thorium Vein need?", ask.GatherFacts(ts, "what mining skill does Thorium Vein need?"), []string{"say the data has none"}, regexp.MustCompile(`(?i)thorium vein: needs`), "no Thorium Vein row"},
		{"what mining skill does a Mithril Deposit need?", ask.GatherFacts(ts, "what mining skill does a Mithril Deposit need?"), []string{"say the data has none"}, regexp.MustCompile(`(?i)mithril deposit: needs`), "Mithril Deposit only in French, no skill"},
		{"what skinning skill do I need for a level 8 creature?", ask.GatherFacts(ts, "what skinning skill do I need for a level 8 creature?"), []string{"the rule gives no skill below level 11, so state none"}, regexp.MustCompile(`level 8 creature needs`), "rule starts at level 11"},
		{"what skill do I need to cook [Gnoll Burger Supreme]?", ask.RecipeSkillFacts(consVendors{ts}, "what skill do I need to cook [Gnoll Burger Supreme]?"), []string{"no Cooking or First Aid recipe row", "state none"}, regexp.MustCompile(`It needs`), "no recipe row for a recipe that is not in the game"},
	}
	var out []benchRow
	for _, c := range cases {
		ok := c.facts != "" && (c.never == nil || !c.never.MatchString(c.facts))
		for _, m := range c.must {
			ok = ok && strings.Contains(c.facts, m)
		}
		out = append(out, benchRow{"\"" + c.q + "\"", lastLine(c.facts), "states none: " + c.ref, gsNoneSrc, resultIf(ok, "no word that the data has none, or a skill or place stated")})
	}
	finish(t, "advice-gather-facts-no-answer", "Offline gathering, fishing and recipes: no answer in the dataset", out)
}

// TestOfflineBenchmarkGatherFactsQuiet: a question about none of these gets no gathering or recipe facts.
func TestOfflineBenchmarkGatherFactsQuiet(t *testing.T) {
	for _, q := range []string{"what should I do next?", "is the mine in Elwynn worth it?", "how do I get a dungeon group?"} {
		if got := ask.GatherFacts(tsStore(), q) + ask.RecipeSkillFacts(tsStore(), q); got != "" {
			t.Errorf("%q: facts %q", q, got)
		}
	}
}

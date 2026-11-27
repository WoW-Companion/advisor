package paths_test

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/learn"
	"github.com/WoW-Companion/advisor/internal/paths"
	"github.com/WoW-Companion/advisor/internal/store"
)

// The offline crafting benchmark: what a recipe needs (the materials and how many of each), where a recipe is learned (trainer,
// vendor, drop, quest), what the answer is for a recipe the character already knows or cannot learn yet, and what it says when
// the dataset has no answer, beside the dataset's rows, with no network. Materials and skills read testdata/tradeskills_snapshot
// (recipes, recipe_reagents, items); recipe items, vendors, creatures and quests read the places and advice snapshots. The dataset
// holds no drop row or quest reward row for any recipe item, so the drop and quest cases compose one row of the dataset's
// loot_sources shape for a real recipe item, a creature the snapshot names and a quest players observed (the same device the quest
// benchmark uses); every other row is the dataset's. A multi-item total is checked in the professions package. make
// offline-benchmark runs it; WOWC_BENCH_REPORT receives the tables.

const (
	ccMatSrc    = "dataset recipes (creates_item_id, skill_line_name), recipe_reagents (reagent_item_id, count), items (name)"
	ccSourceSrc = "dataset recipes (learned_from), recipe_items, vendor_sources, loot_sources (composed creature and quest rows), npcs, quests_observed"
	ccKnownSrc  = "dataset recipes (learn_skill, learn_skill_source, learned_from); the capture's known spells and skills"
	ccNoneSrc   = "the dataset has no such row"
)

// ---- the materials a recipe needs ----

// ccRecipe is the dataset's recipe of a profession that makes an item: the one the advisor takes first, the lowest spell id.
func ccRecipe(t *testing.T, ds store.DataStore, item int64, prof string) store.Row {
	t.Helper()
	rows, err := ds.Filter("recipes", store.Range{Column: "creates_item_id", Min: item, Max: item})
	if err != nil {
		t.Fatal(err)
	}
	var best store.Row
	for _, r := range rows {
		if strings.EqualFold(text(r["skill_line_name"]), prof) && (best == nil || num(r["spell_id"]) < num(best["spell_id"])) {
			best = r
		}
	}
	if best == nil {
		t.Fatalf("item %d: no %s recipe in the snapshot", item, prof)
	}
	return best
}

// ccMaterials are the dataset's materials of a recipe as "N Name", sorted.
func ccMaterials(t *testing.T, ds store.DataStore, spell int64) []string {
	t.Helper()
	rows, err := ds.Filter("recipe_reagents", store.Range{Column: "spell_id", Min: spell, Max: spell})
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, r := range rows {
		it, _ := ds.Get("items", num(r["reagent_item_id"]))
		out = append(out, fmt.Sprintf("%d %s", max(num(r["count"]), 1), text(it["name"])))
	}
	sort.Strings(out)
	return out
}

// ccCraft is the craft way of an answer, whatever its rank among the ways.
func ccCraft(res paths.Result) (paths.Path, bool) {
	for _, p := range res.Paths {
		if p.Kind == paths.Craft {
			return p, true
		}
	}
	return paths.Path{}, false
}

func ccGot(p paths.Path) []string {
	var out []string
	for _, g := range p.Reagents {
		out = append(out, fmt.Sprintf("%d %s", g.Need, g.Name))
	}
	sort.Strings(out)
	return out
}

var ccMaterialCases = []struct {
	item, prof string
}{
	{"Heavy Linen Bandage", "First Aid"}, {"Elixir of Minor Defense", "Alchemy"}, {"Copper Bracers", "Blacksmithing"},
	{"Handstitched Leather Belt", "Leatherworking"}, {"Bolt of Woolen Cloth", "Tailoring"}, {"Rough Blasting Powder", "Engineering"},
	{"Swiftness Potion", "Alchemy"}, {"Silk Bandage", "First Aid"},
}

// TestOfflineBenchmarkCraftMaterialLists: the materials and the count of each that the craft answer lists for a crafted item, beside the
// recipe's rows: every material, with its count, and none the recipe does not hold.
func TestOfflineBenchmarkCraftMaterialLists(t *testing.T) {
	ds := consVendors{tsStore()}
	var out []benchRow
	for _, tc := range ccMaterialCases {
		id := lgItemID(t, ds, tc.item)
		rec := ccRecipe(t, ds, id, tc.prof)
		want := ccMaterials(t, ds, num(rec["spell_id"]))
		res := paths.For(tsChar("Horde", 60, tsSkill{tc.prof, 300, 300}), ds, id, paths.Options{})
		p, found := ccCraft(res)
		if !found {
			out = append(out, benchRow{tc.item + ": materials", "no way", strings.Join(want, ", "), ccMatSrc, "differ: no craft way"})
			continue
		}
		got := ccGot(p)
		ok := strings.Join(got, ", ") == strings.Join(want, ", ") && p.Kind == paths.Craft
		for _, g := range p.Reagents {
			ok = ok && strings.Contains(p.Line, fmt.Sprintf("0/%d %s", g.Need, g.Name))
		}
		out = append(out, benchRow{tc.item + " (" + tc.prof + "): the materials and counts", strings.Join(got, ", "), strings.Join(want, ", "), ccMatSrc,
			resultIf(ok, "a material or a count other than the recipe's, or a line that does not state each count")})
	}
	finish(t, "advice-craft-materials", "Offline crafting: the materials and counts of a recipe", out)
}

// ---- where a recipe is learned ----

// ccQuest is a quest players observed (quests_observed), its title the dataset's.
func ccQuest(t *testing.T, id int64) string {
	t.Helper()
	return text(qbQuest(t, id)["title"])
}

func ccCreature(t *testing.T, id int64) string {
	t.Helper()
	r, err := qbStore().Get("npcs", id)
	if err != nil || text(r["name"]) == "" {
		t.Fatalf("npc %d: no name in the snapshot (%v)", id, err)
	}
	return text(r["name"])
}

func ccDrop(item, creature int64) store.Row {
	return store.Row{"item_id": item, "source_kind": "creature", "source_id": creature, "tier": "settled", "rate": 0.5}
}

const (
	ccWolf      = 565 // Rabid Dire Wolf, named in npcs
	ccFlesh     = 3   // Flesh Eater
	ccQuestRiv  = 890 // a Horde quest players observed
	ccQuestBetr = 879
)

// ccPatternCases: recipe items (real rows of recipe_items) and where each is learned. rows are composed where the dataset holds none.
type ccSourceCase struct {
	label, craft, prof string
	faction            string
	recipeItem         int64
	rows               []store.Row
	kind               string // trainer, vendor, drop, quest
	creature, quest    int64
}

var ccSourceCases = []ccSourceCase{
	{"Elixir of Minor Defense", "Elixir of Minor Defense", "Alchemy", "Horde", 0, nil, "trainer", 0, 0},
	{"Heavy Linen Bandage", "Heavy Linen Bandage", "First Aid", "Alliance", 0, nil, "trainer", 0, 0},
	{"Red Linen Bag", "Red Linen Bag", "Tailoring", "Horde", 5771, nil, "vendor", 0, 0},
	{"Strider Stew", "Strider Stew", "Cooking", "Alliance", 5486, nil, "vendor", 0, 0},
	{"Fine Leather Boots", "Fine Leather Boots", "Leatherworking", "Horde", 2406, []store.Row{ccDrop(2406, ccWolf)}, "drop", ccWolf, 0},
	{"Goretusk Liver Pie", "Goretusk Liver Pie", "Cooking", "Horde", 2697, []store.Row{qbReward(2697, ccQuestRiv)}, "quest", 0, ccQuestRiv},
}

func TestOfflineBenchmarkCraftRecipeSourcesA(t *testing.T) {
	ccRecipeSources(t, "a", ccSourceCases[:3])
}
func TestOfflineBenchmarkCraftRecipeSourcesB(t *testing.T) {
	ccRecipeSources(t, "b", ccSourceCases[3:])
}

func ccRecipeSources(t *testing.T, part string, cases []ccSourceCase) {
	var out []benchRow
	for _, tc := range cases {
		ds := qbRewards{DataStore: consVendors{qbStore()}, rows: tc.rows}
		id := lgItemID(t, ds, tc.craft)
		rec := ccRecipe(t, ds, id, tc.prof)
		c := tsChar(tc.faction, 60, tsSkill{tc.prof, 300, 300})
		best, _ := ccCraft(paths.For(c, ds, id, paths.Options{}))
		kind, line := "no recipe source", ""
		if best.Recipe != nil {
			kind, line = best.Recipe.Kind, best.Recipe.Line
		}
		found, ferr := learn.Find(c, ds, nil, text(rec["name"]))
		first := "no way"
		if ferr == nil && len(found.Ways) > 0 {
			first = found.Ways[0].Kind + ": " + found.Ways[0].Line
		}
		ref := fmt.Sprintf("%s recipe (learned_from %s)", tc.kind, text(rec["learned_from"]))
		ok := kind == tc.kind && (ferr == nil && len(found.Ways) > 0 && found.Ways[0].Kind == tc.kind || tc.kind == "trainer" && ferr == nil && !strings.Contains(found.Text, "Buy the recipe"))
		switch tc.kind {
		case "vendor":
			ri, _ := qbStore().Filter("recipe_items", store.Range{Column: "item_id", Min: tc.recipeItem, Max: tc.recipeItem})
			named, rows := refVendors(qbStore(), tc.recipeItem, tc.faction)
			ref = fmt.Sprintf("a recipe item (%d row), %d vendor rows, %d named", len(ri), rows, len(named))
			ok = ok && strings.HasPrefix(line, "buy the recipe") && rows > 0
		case "drop":
			ref = "recipe item " + fmt.Sprint(tc.recipeItem) + " drops from " + ccCreature(t, tc.creature) + " (composed loot row)"
			ok = ok && strings.Contains(line, ccCreature(t, tc.creature)) && strings.Contains(first, ccCreature(t, tc.creature))
		case "quest":
			ref = "recipe item " + fmt.Sprint(tc.recipeItem) + " is a reward of quest " + ccQuest(t, tc.quest) + " (composed loot row)"
			ok = ok && strings.Contains(line, ccQuest(t, tc.quest)) && strings.Contains(first, ccQuest(t, tc.quest))
		}
		out = append(out, benchRow{tc.label + " (" + tc.faction + "): where the recipe is learned", kind + ": " + line + " / " + first, ref, ccSourceSrc,
			resultIf(ok, "a source of another kind, or one the dataset's row does not name")})
	}
	finish(t, "advice-craft-recipe-sources-"+part, "Offline crafting: where a recipe is learned ("+part+")", out)
}

// ---- a recipe the character knows, or cannot learn yet ----

var ccKnownCases = []struct {
	label, item, prof string
	rank              int
	known             bool   // the capture lists the recipe as known
	other             bool   // the capture lists other recipes, not this one
	want              string // known, ready, short, far, source
}{
	{"known Elixir of Minor Defense", "Elixir of Minor Defense", "Alchemy", 300, true, false, "known"},
	{"known Heavy Linen Bandage", "Heavy Linen Bandage", "First Aid", 300, true, false, "known"},
	{"another recipe known, not Copper Chain Belt", "Copper Chain Belt", "Blacksmithing", 300, false, true, "learn"},
	{"Heavy Linen Bandage, skill 25 of 40", "Heavy Linen Bandage", "First Aid", 25, false, true, "short"},
	{"Mageweave Bag, skill 100 of 190", "Mageweave Bag", "Tailoring", 100, false, true, "far"},
	{"Silk Bandage, skill 130 of 150", "Silk Bandage", "First Aid", 130, false, true, "short"},
}

// TestOfflineBenchmarkCraftKnownOrNotYet: a recipe the capture lists as known is never sent to a trainer; one it lists as not known is
// said to be learned first; a skill below the recipe's own is said short with the gap of the recipe row, never "you can make it".
func TestOfflineBenchmarkCraftKnownOrNotYetA(t *testing.T) { ccKnownOrNotYet(t, "a", ccKnownCases[:3]) }
func TestOfflineBenchmarkCraftKnownOrNotYetB(t *testing.T) {
	ccKnownOrNotYet(t, "b", ccKnownCases[3:4])
}
func TestOfflineBenchmarkCraftKnownOrNotYetC(t *testing.T) {
	ccKnownOrNotYet(t, "c", ccKnownCases[4:5])
}
func TestOfflineBenchmarkCraftKnownOrNotYetD(t *testing.T) { ccKnownOrNotYet(t, "d", ccKnownCases[5:]) }

func ccKnownOrNotYet(t *testing.T, part string, cases []struct {
	label, item, prof string
	rank              int
	known             bool
	other             bool
	want              string
}) {
	ds := consVendors{tsStore()}
	var out []benchRow
	for _, tc := range cases {
		id := lgItemID(t, ds, tc.item)
		rec := ccRecipe(t, ds, id, tc.prof)
		spell := num(rec["spell_id"])
		need := int(num(rec["learn_skill"]))
		c := tsChar("Horde", 60, tsSkill{tc.prof, tc.rank, 300})
		switch {
		case tc.known:
			c.KnownSpells = map[int64]bool{spell: true}
		case tc.other:
			c.KnownSpells = map[int64]bool{spell + 1: true}
		}
		res := paths.For(c, ds, id, paths.Options{})
		p, found := ccCraft(res)
		if !found {
			out = append(out, benchRow{tc.label, "no way", "", ccKnownSrc, "differ: no way"})
			continue
		}
		gap := need - tc.rank
		ref := fmt.Sprintf("%s needs skill %d (%s); the character has %d", tc.item, need, text(rec["learn_skill_source"]), tc.rank)
		ok := p.Kind == paths.Craft && p.Skill != nil && p.Recipe != nil
		switch tc.want {
		case "known":
			ok = ok && p.Recipe.Kind == "known" && strings.Contains(p.Line, "You already know the recipe") && !strings.Contains(p.Line, "Learn it")
		case "learn":
			ok = ok && p.Recipe.Kind != "known" && strings.Contains(p.Line, "Learn it first") && !strings.Contains(p.Line, "You already know")
		case "short":
			ok = ok && gap > 0 && gap <= paths.ShortMax && p.Skill.Gap == gap && strings.Contains(p.Line, fmt.Sprintf("it needs %d (%d more points)", need, gap)) && !strings.Contains(p.Line, "You can make it now")
		case "far":
			ok = ok && gap > paths.ShortMax && p.Skill.Gap == gap && p.Skill.Status == paths.Far && strings.Contains(p.Line, fmt.Sprintf("it needs %d (%d more points)", need, gap)) && !strings.Contains(p.Line, "You can make it now")
		}
		out = append(out, benchRow{tc.label, firstLine(p.Line), ref, ccKnownSrc, resultIf(ok, "known, learn-first or the skill gap worded other than the recipe row says")})
	}
	finish(t, "advice-craft-known-not-yet-"+part, "Offline crafting: a recipe already known or not learnable yet ("+part+")", out)
}

// ---- no answer in the dataset ----

// TestOfflineBenchmarkCraftNoAnswer: where the dataset has no answer the advisor says so and names nothing: a recipe item no vendor sells and
// no creature drops, a recipe name the dataset lacks, an item no profession makes, and a quest that no table names.
func TestOfflineBenchmarkCraftNoAnswer(t *testing.T) {
	var out []benchRow
	{
		ds := consVendors{qbStore()}
		id := lgItemID(t, ds, "Rugged Leather Pants")
		c := tsChar("Horde", 60, tsSkill{"Leatherworking", 300, 300})
		res := paths.For(c, ds, id, paths.Options{})
		line, kind := "no way", ""
		if p, ok := ccCraft(res); ok && p.Recipe != nil {
			line, kind = p.Recipe.Line, p.Recipe.Kind
		}
		found, _ := learn.Find(c, ds, nil, "Rugged Leather Pants")
		ok := kind == "unknown" && strings.Contains(line, "does not say where it comes from") && len(found.Ways) == 0 && !strings.Contains(found.Text, "trainer")
		out = append(out, benchRow{"Rugged Leather Pants: a recipe item nothing sells or drops", line + " / " + firstLine(found.Text), "recipe item 7288 has no vendor_sources or loot_sources row", ccSourceSrc,
			resultIf(ok, "a place is named, or the recipe is sent to a trainer")})
	}
	{
		ds := consVendors{qbStore()}
		c := tsChar("Horde", 60, tsSkill{"Alchemy", 300, 300})
		found, err := learn.Find(c, ds, nil, "Elixir of Imaginary Wonders")
		ok := err != nil || len(found.Ways) == 0
		got := "an error"
		if err == nil {
			got = firstLine(found.Text)
		}
		out = append(out, benchRow{"a recipe name the dataset lacks", got, "no recipes row of that name", ccNoneSrc, resultIf(ok, "a way is named for a recipe the dataset does not hold")})
	}
	{
		ds := consVendors{tsStore()}
		id := lgItemID(t, ds, "Linen Cloth")
		res := paths.For(tsChar("Horde", 60, tsSkill{"Tailoring", 300, 300}), ds, id, paths.Options{})
		craft := false
		for _, p := range res.Paths {
			craft = craft || p.Kind == paths.Craft
		}
		out = append(out, benchRow{"Linen Cloth: an item no profession makes", firstLine(res.Text()), "no recipes row creates it", ccMatSrc, resultIf(!craft, "a craft is invented for a material")})
	}
	{
		ds := qbRewards{DataStore: consVendors{qbStore()}, rows: []store.Row{qbReward(2697, 99999999)}}
		id := lgItemID(t, ds, "Goretusk Liver Pie")
		res := paths.For(tsChar("Horde", 60, tsSkill{"Cooking", 300, 300}), ds, id, paths.Options{})
		line := "no way"
		if p, ok := ccCraft(res); ok && p.Recipe != nil {
			line = p.Recipe.Line
		}
		ok := !strings.Contains(line, "quest") || strings.Contains(line, "does not say")
		out = append(out, benchRow{"Goretusk Liver Pie: a reward of a quest no table names", line, "no quests or quests_observed row of quest 99999999", ccNoneSrc, resultIf(ok, "a quest is named that no table holds")})
	}
	{
		// the recipe row says an item teaches it and recipe_items names the item, but the items table has no row for it: the recipe is
		// never sent to a trainer
		ds := consVendors{tsStore()}
		for _, tc := range []struct {
			craft int64
			name  string
			prof  string
		}{{733, "Westfall Stew", "Cooking"}, {2312, "Fine Leather Gloves", "Leatherworking"}} {
			c := tsChar("Horde", 60, tsSkill{tc.prof, 300, 300})
			p, _ := ccCraft(paths.For(c, ds, tc.craft, paths.Options{}))
			line, kind := "no recipe source", ""
			if p.Recipe != nil {
				line, kind = p.Recipe.Line, p.Recipe.Kind
			}
			rec := ccRecipe(t, ds, tc.craft, tc.prof)
			ok := kind == "unknown" && strings.Contains(line, "recipe item") && !strings.Contains(line, "trainer")
			out = append(out, benchRow{tc.name + ": taught by a recipe item the items table lacks", line, "recipes.learned_from " + text(rec["learned_from"]) + "; recipe_items names the item; no items, vendor_sources or loot_sources row", ccSourceSrc,
				resultIf(ok, "the recipe is sent to a trainer, though the dataset says an item teaches it")})
		}
	}
	{
		// the dataset holds a recipe for an item its items table does not hold: no guess
		ds := consVendors{tsStore()}
		const ironvine = 22762
		res := paths.For(tsChar("Horde", 60, tsSkill{"Blacksmithing", 300, 300}), ds, ironvine, paths.Options{})
		_, craft := ccCraft(res)
		recs, _ := ds.Filter("recipes", store.Range{Column: "creates_item_id", Min: int64(ironvine), Max: int64(ironvine)})
		ok := !craft && strings.Contains(res.Text(), "does not know this item")
		result := "differ (data): the recipes table names an item the items table does not hold; the advisor says it does not know the item and lists no materials"
		if !ok {
			result = "differ: the advisor invents a craft for an item the dataset does not hold"
		}
		out = append(out, benchRow{"item 22762: a recipe whose product the items table lacks", strings.ReplaceAll(res.Text(), "\n", "; "), fmt.Sprintf("%d recipes row(s) create it; items has no row", len(recs)), ccMatSrc, result})
	}
	finish(t, "advice-craft-no-answer", "Offline crafting: where the dataset has no answer", out)
}

package professions

import (
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/store"
)

type pQuest struct {
	ID      int64  `parquet:"id"`
	Name    string `parquet:"name"`
	Level   int32  `parquet:"level"`
	Faction string `parquet:"faction"`
}

type guideSet struct {
	items    []pItem
	recipes  []pRecipe
	reagents []reagentRow
	vendors  []pVendor
	rItems   []pRecipeItem
	loot     []pLoot
	quests   []pQuest
	de       []pDE
	trainers []pTrainerSpell
	// dungeonBoss, when set, is a creature that is a boss of the one test dungeon.
	dungeonBoss int64
}

type pBoss struct {
	Creature int64 `parquet:"creature_id"`
	Map      int64 `parquet:"map_id"`
}

type pInstance struct {
	Map  int64  `parquet:"map_id"`
	Name string `parquet:"name"`
}

func (g guideSet) build(t *testing.T) store.DataStore {
	t.Helper()
	src := store.ReaderSource{
		"items": table(t, g.items), "recipes": table(t, g.recipes), "recipe_reagents": table(t, g.reagents),
		"vendor_sources": table(t, append(g.vendors, pVendor{99, 9999, 1})), "recipe_items": table(t, append(g.rItems, pRecipeItem{9999, 9999, 0, "x"})),
		"loot_sources": table(t, append(g.loot, pLoot{9999, "creature", 1, 1, 1})), "quests": table(t, append(g.quests, pQuest{1, "x", 1, ""})),
		"disenchant_yields": table(t, append(g.de, pDE{"weapon", "uncommon", 1, 2, "none", 9999})),
		"trainer_spells":    table(t, append(g.trainers, pTrainerSpell{1, 9999, 1})),
		"profession_notes":  table(t, rankTiers()),
	}
	if g.dungeonBoss != 0 {
		src["instance_bosses"] = table(t, []pBoss{{g.dungeonBoss, 429}})
		src["instances"] = table(t, []pInstance{{429, "Dire Maul"}})
	}
	return store.New(src)
}

func benchBands(t *testing.T, g guideSet, o BenchOptions) Benchmark {
	t.Helper()
	b := NewBenchmark(g.build(t), o)
	if len(b.Bands) == 0 {
		t.Fatalf("no bands: %v", b.Notes)
	}
	return b
}

func bandRecipes(b Benchmark) string {
	var out []string
	for _, x := range b.Bands {
		out = append(out, x.Recipe)
	}
	return strings.Join(out, "|")
}

func TestExpansionSurvivesRecipeCycle(t *testing.T) {
	// Fine Leather is made from Rough Leather and back: the expansion must stop, not multiply.
	g := guideSet{
		items: []pItem{{1, "Fine Leather", 1, 1, 0}, {2, "Rough Leather", 1, 1, 0}, {3, "Thread", 1, 1, 0}},
		recipes: []pRecipe{
			pr(10, "Leatherworking", "Vest", nil, 1, 150, 300, "trainer"),
			pr(11, "Leatherworking", "Refine Fine", i32(1), 1, 1, 2, "trainer"),
			pr(12, "Leatherworking", "Refine Rough", i32(2), 1, 1, 2, "trainer"),
		},
		reagents: []reagentRow{{10, 1, 3}, {11, 2, 3}, {12, 1, 3}, {11, 3, 1}},
		vendors:  []pVendor{{1, 3, 10}},
	}
	b := benchBands(t, g, BenchOptions{Profession: "Leatherworking", To: 100})
	if n := b.Efficiency.TotalMaterials; n > 5000 {
		t.Errorf("%d materials: the cycle blew up", n)
	}
}

func TestSanityCapRejectsMaterialHungryBand(t *testing.T) {
	g := guideSet{
		items: []pItem{{1, "Pebble", 1, 1, 0}, {2, "Gold Nugget", 1, 1, 0}},
		recipes: []pRecipe{
			pr(10, "Blacksmithing", "Pebble Pile", nil, 1, 200, 300, "trainer"),
			pr(11, "Blacksmithing", "Nugget Ring", nil, 1, 200, 300, "trainer"),
		},
		reagents: []reagentRow{{10, 1, 40}, {11, 2, 1}},
		vendors:  []pVendor{{1, 1, 1}, {1, 2, 100}},
	}
	b := benchBands(t, g, BenchOptions{Profession: "Blacksmithing", To: 60})
	if got := b.Bands[0].Recipe; got != "Nugget Ring" {
		t.Errorf("picked %s, want Nugget Ring: 40 materials per point is over the cap", got)
	}
	// With no alternative the hungry recipe is still used.
	g.recipes = g.recipes[:1]
	if b := benchBands(t, g, BenchOptions{Profession: "Blacksmithing", To: 60}); b.Bands[0].Recipe != "Pebble Pile" {
		t.Errorf("sole recipe not used: %s", bandRecipes(b))
	}
}

func TestTierCostPrefersCommonOverGem(t *testing.T) {
	g := guideSet{
		items: []pItem{{1, "Copper Bar", 1, 1, 0}, {2, "Star Ruby", 1, 1, 0}},
		recipes: []pRecipe{
			pr(10, "Blacksmithing", "Copper Mace", nil, 1, 150, 300, "trainer"),
			pr(11, "Blacksmithing", "Ruby Ring", nil, 1, 150, 300, "trainer"),
		},
		reagents: []reagentRow{{10, 1, 6}, {11, 2, 1}},
	}
	b := benchBands(t, g, BenchOptions{Profession: "Blacksmithing", To: 50})
	if got := b.Bands[0].Recipe; got != "Copper Mace" {
		t.Errorf("picked %s, want the copper recipe over the gem", got)
	}
}

func TestVendorPatternUsedAndSpecialisationSkipped(t *testing.T) {
	g := guideSet{
		items: []pItem{{1, "Heavy Leather", 1, 1, 0}, {7, "Pattern: Wicked Leather Gauntlets", 1, 1, 0}},
		recipes: []pRecipe{
			pr(10, "Leatherworking", "Nightscape Pants", nil, 1, 150, 235, "trainer"),
			pr(11, "Leatherworking", "Green Dragonscale Gauntlets", nil, 235, 245, 265, "trainer"),
			pr(12, "Leatherworking", "Wicked Leather Gauntlets", nil, 235, 245, 265, "item"),
		},
		reagents: []reagentRow{{10, 1, 2}, {11, 1, 1}, {12, 1, 9}},
		vendors:  []pVendor{{1, 1, 30}, {2, 7, 5000}},
		rItems:   []pRecipeItem{{7, 12, 235, "Leatherworking"}},
	}
	b := benchBands(t, g, BenchOptions{Profession: "Leatherworking", From: 200, To: 265})
	r := bandRecipes(b)
	if strings.Contains(r, "Dragonscale") || !strings.Contains(r, "Wicked Leather Gauntlets") {
		t.Fatalf("bands %s", r)
	}
	if !strings.Contains(b.GuideCheck, "vendor pattern Pattern: Wicked Leather Gauntlets") {
		t.Errorf("guide check: %s", b.GuideCheck)
	}
}

func TestCooldownMaterialAvoided(t *testing.T) {
	g := guideSet{
		items: []pItem{{1, "Mooncloth", 1, 1, 0}, {2, "Runecloth", 1, 1, 0}},
		recipes: []pRecipe{
			pr(10, "Tailoring", "Mooncloth Boots", nil, 1, 150, 300, "trainer"),
			pr(11, "Tailoring", "Runecloth Belt", nil, 1, 150, 300, "trainer"),
		},
		reagents: []reagentRow{{10, 1, 1}, {11, 2, 10}},
	}
	b := benchBands(t, g, BenchOptions{Profession: "Tailoring", To: 60})
	if strings.Contains(bandRecipes(b), "Mooncloth") {
		t.Errorf("bands %s", bandRecipes(b))
	}
}

func TestDustFromDisenchanting(t *testing.T) {
	g := guideSet{
		items:    []pItem{{1, "Strange Dust", 1, 1, 0}},
		recipes:  []pRecipe{pr(10, "Enchanting", "Enchant Bracer", nil, 1, 100, 140, "trainer")},
		reagents: []reagentRow{{10, 1, 1}},
		de:       []pDE{{"armor", "uncommon", 11, 25, "rated", 1}},
	}
	b := benchBands(t, g, BenchOptions{Profession: "Enchanting", To: 60, Level: 30})
	got := b.Bands[0].Recipe
	pa := computePath(benchChar("Enchanting", 1, 30), g.build(t), "Enchanting", 60, 1, true, func(p *planner) { p.fresh = true })
	from := pa.Bands[0].Best.Needs[0].From
	if got != "Enchant Bracer" || !strings.Contains(from, "disenchant level-11 to 25 greens or buy") {
		t.Errorf("recipe %s, dust from %q", got, from)
	}
}

func TestGapLineAndQuestRecipe(t *testing.T) {
	g := guideSet{
		items: []pItem{{1, "Mithril Bar", 1, 1, 0}, {7, "Plans: Imperial Plate Bracers", 1, 1, 0}, {8, "Plans: Imperial Plate Leggings", 1, 1, 0}},
		recipes: []pRecipe{
			pr(10, "Blacksmithing", "Mithril Spur", nil, 1, 240, 285, "trainer"),
			pr(11, "Blacksmithing", "Imperial Plate Bracers", nil, 285, 292, 300, "item"),
			pr(12, "Blacksmithing", "Imperial Plate Leggings", nil, 285, 292, 300, "item"),
		},
		reagents: []reagentRow{{10, 1, 2}, {11, 1, 2}, {12, 1, 2}},
		rItems:   []pRecipeItem{{7, 11, 285, "Blacksmithing"}, {8, 12, 285, "Blacksmithing"}},
		loot:     []pLoot{{7, "quest", 500, 0, 0}, {8, "quest", 501, 0, 0}},
		quests:   []pQuest{{500, "Imperial Plate", 50, ""}, {501, "Leggings Quest", 70, ""}},
	}
	b := benchBands(t, g, BenchOptions{Profession: "Blacksmithing", From: 240, To: 300, Level: 60})
	last := b.Bands[len(b.Bands)-1]
	if last.To != 300 || last.Recipe != "Imperial Plate Bracers" || !strings.Contains(b.GuideCheck, "quest recipe Plans: Imperial Plate Bracers from the quest Imperial Plate") {
		t.Errorf("quest recipe not used: %s", b.GuideCheck)
	}
	// Without the quest recipe in reach the band says so, naming a recipe to look for.
	g.quests[0].Level = 70
	b = benchBands(t, g, BenchOptions{Profession: "Blacksmithing", From: 240, To: 300, Level: 60})
	gap := b.Bands[len(b.Bands)-1]
	if gap.Gap == "" || gap.To != 300 || !strings.Contains(gap.Gap, "No trainer or vendor recipe covers 285–300") || !strings.Contains(gap.Gap, "Imperial Plate") {
		t.Errorf("last band %+v", gap)
	}
	if !strings.Contains(RenderBenchmark(b), "No trainer or vendor recipe covers 285–300") {
		t.Error("gap missing from the rendering")
	}
}

func TestAlchemyMakesBandsDespiteTransmuteCycle(t *testing.T) {
	g := guideSet{
		items: []pItem{{1, "Empty Vial", 1, 1, 0}, {2, "Peacebloom", 1, 1, 0}, {3, "Iron Bar", 1, 1, 0}, {4, "Gold Bar", 1, 1, 0}},
		recipes: []pRecipe{
			pr(10, "Alchemy", "Minor Healing Potion", nil, 1, 55, 100, "trainer"),
			pr(11, "Alchemy", "Lesser Healing Potion", nil, 55, 100, 160, "trainer"),
			pr(12, "Alchemy", "Transmute: Iron to Gold", i32(4), 1, 100, 150, "trainer"),
			pr(13, "Alchemy", "Transmute: Gold to Iron", i32(3), 1, 100, 150, "trainer"),
		},
		reagents: []reagentRow{{10, 1, 1}, {10, 2, 1}, {11, 1, 1}, {11, 2, 2}, {12, 3, 1}, {13, 4, 1}},
		vendors:  []pVendor{{1, 1, 50}},
	}
	b := benchBands(t, g, BenchOptions{Profession: "Alchemy", To: 150})
	if b.Bands[len(b.Bands)-1].To != 150 || strings.Contains(bandRecipes(b), "Transmute") {
		t.Errorf("bands %s, notes %v", bandRecipes(b), b.Notes)
	}
}

func benchChar(profession string, skill, level int) character.Character {
	return character.Character{Name: "Benchmark", Class: "None", Level: &level, Professions: []character.Skill{{Name: profession, Rank: &skill}}}
}

func TestLeatherConversionChainDoesNotBlowUp(t *testing.T) {
	// Medium Leather is made from 4 Light Leather, Light from 3 Scraps, Heavy from 5 Medium and
	// Thick from 6 Heavy: each step multiplies, so the made forms are bought, not expanded.
	g := guideSet{
		items: []pItem{{1, "Ruined Leather Scraps", 1, 1, 0}, {2, "Light Leather", 1, 1, 0}, {3, "Medium Leather", 1, 1, 0},
			{4, "Heavy Leather", 1, 1, 0}, {5, "Thick Leather", 1, 1, 0}},
		recipes: []pRecipe{
			pr(10, "Leatherworking", "Thick Vest", nil, 1, 150, 300, "trainer"),
			pr(11, "Leatherworking", "Refine Light", i32(2), 1, 1, 2, "trainer"),
			pr(12, "Leatherworking", "Refine Medium", i32(3), 1, 1, 2, "trainer"),
			pr(13, "Leatherworking", "Refine Heavy", i32(4), 1, 1, 2, "trainer"),
			pr(14, "Leatherworking", "Refine Thick", i32(5), 1, 1, 2, "trainer"),
		},
		reagents: []reagentRow{{10, 5, 6}, {11, 1, 3}, {12, 2, 4}, {13, 3, 5}, {14, 4, 6}},
		trainers: []pTrainerSpell{{1, 10, 0}, {1, 11, 0}, {1, 12, 0}, {1, 13, 0}, {1, 14, 0}},
		vendors:  []pVendor{{1, 1, 1}},
	}
	b := benchBands(t, g, BenchOptions{Profession: "Leatherworking", To: 100})
	if n := b.Efficiency.TotalMaterials; n > 5000 {
		t.Errorf("%d materials: the conversion chain blew up", n)
	}
}

func TestConfirmedTrainerBeatsUnconfirmed(t *testing.T) {
	g := guideSet{
		items: []pItem{{1, "Linen Cloth", 1, 1, 0}},
		recipes: []pRecipe{
			pr(10, "Tailoring", "Unproven Cape", nil, 1, 100, 200, "trainer"),
			pr(11, "Tailoring", "Proven Cape", nil, 1, 100, 200, "trainer"),
		},
		reagents: []reagentRow{{10, 1, 1}, {11, 1, 1}},
		trainers: []pTrainerSpell{{1, 11, 0}},
	}
	b := benchBands(t, g, BenchOptions{Profession: "Tailoring", To: 50})
	if got := b.Bands[0].Recipe; got != "Proven Cape" {
		t.Errorf("picked %s, want the trainer-confirmed recipe", got)
	}
	if got := b.Bands[0].Source; got != "trainer, confirmed" {
		t.Errorf("source %q", got)
	}
	g.recipes = g.recipes[:1]
	g.trainers = []pTrainerSpell{{1, 99, 0}}
	b = benchBands(t, g, BenchOptions{Profession: "Tailoring", To: 50})
	if got := b.Bands[0].Source; !strings.Contains(got, "ask your trainer") {
		t.Errorf("source %q, want an ask-your-trainer mark", got)
	}
	if !strings.Contains(b.Bands[0].Learn, "probably from your trainer") {
		t.Errorf("learn %q", b.Bands[0].Learn)
	}
}

func TestDungeonOnlyPatternIsNotATrainerRecipe(t *testing.T) {
	g := guideSet{
		items: []pItem{{1, "Linen Cloth", 1, 1, 0}, {7, "Pattern: Dungeon Suit", 1, 1, 0}},
		recipes: []pRecipe{
			pr(10, "Tailoring", "Dungeon Suit", nil, 1, 100, 200, "trainer"),
			pr(11, "Tailoring", "Plain Cape", nil, 1, 100, 200, "trainer"),
		},
		reagents:    []reagentRow{{10, 1, 1}, {11, 1, 3}},
		trainers:    []pTrainerSpell{{1, 11, 0}},
		rItems:      []pRecipeItem{{7, 10, 1, "Tailoring"}},
		loot:        []pLoot{{7, "creature", 500, 1, 1}},
		dungeonBoss: 500,
	}
	b := benchBands(t, g, BenchOptions{Profession: "Tailoring", To: 50})
	if got := b.Bands[0].Recipe; got != "Plain Cape" {
		t.Errorf("picked %s, want the trainer recipe over the dungeon pattern", got)
	}
}

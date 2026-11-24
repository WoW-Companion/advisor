package professions

import (
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/store"
)

type pRecipe struct {
	SpellID       int32   `parquet:"spell_id"`
	SkillLineName string  `parquet:"skill_line_name"`
	Name          *string `parquet:"name,optional"`
	CreatesItemID *int32  `parquet:"creates_item_id,optional"`
	LearnSkill    *int16  `parquet:"learn_skill,optional"`
	YellowSkill   *int16  `parquet:"yellow_skill,optional"`
	GreySkill     *int16  `parquet:"grey_skill,optional"`
	LearnedFrom   string  `parquet:"learned_from"`
}

type pItem struct {
	ID        int64  `parquet:"id"`
	Name      string `parquet:"name"`
	ItemLevel int16  `parquet:"item_level"`
	Quality   int8   `parquet:"quality"`
	ClassID   int8   `parquet:"class_id"`
}

type pVendor struct {
	NpcID int32 `parquet:"npc_id"`
	Item  int32 `parquet:"item_id"`
	Price int64 `parquet:"price_copper"`
}

type pRecipeItem struct {
	Item   int32  `parquet:"item_id"`
	Spell  int32  `parquet:"teaches_spell_id"`
	Skill  int16  `parquet:"required_skill"`
	Profes string `parquet:"profession"`
}

type pFish struct {
	Zone     int32   `parquet:"zone_id"`
	ZoneName string  `parquet:"zone_name"`
	Item     int32   `parquet:"item_id"`
	Rate     float64 `parquet:"rate"`
}

type pDE struct {
	Class   string `parquet:"item_class"`
	Quality string `parquet:"quality"`
	Min     int32  `parquet:"min_item_level"`
	Max     int32  `parquet:"max_item_level"`
	State   string `parquet:"state"`
	Mat     int32  `parquet:"material_item_id"`
}

type pTrainer struct {
	Npc     int32  `parquet:"npc_id"`
	Name    string `parquet:"name"`
	Prof    string `parquet:"profession"`
	Zone    string `parquet:"zone_name"`
	Recipes int32  `parquet:"recipes"`
}

type pTrainerSpell struct {
	Npc   int32 `parquet:"npc_id"`
	Spell int32 `parquet:"spell_id"`
	Cost  int64 `parquet:"cost_copper"`
}

type pNote struct {
	Section  string  `parquet:"section"`
	Subject  string  `parquet:"subject"`
	Related  *string `parquet:"related,optional"`
	SkillMin *int16  `parquet:"skill_min,optional"`
	SkillCap *int16  `parquet:"skill_cap,optional"`
	MinLevel *int16  `parquet:"min_level,optional"`
	Text     string  `parquet:"text"`
}

func pr(spell int32, line, name string, create *int32, learn, yellow, grey int16, from string) pRecipe {
	return pRecipe{spell, line, str(name), create, i16(learn), i16(yellow), i16(grey), from}
}

func tier(name string, lo, hi, lv int16) pNote {
	return pNote{"ranks", name, nil, i16(lo), i16(hi), i16(lv), name + " tier"}
}

func pair(a, b, text string) pNote { return pNote{"pairings", a, str(b), nil, nil, nil, text} }

func pathData(t *testing.T) store.DataStore {
	return store.New(store.ReaderSource{
		"items": table(t, []pItem{
			{1, "Raw Brilliant Smallfish", 1, 1, 0}, {2, "Boar Meat", 1, 1, 0}, {3, "Copper Bar", 1, 1, 0}, {4, "Bronze Bar", 1, 1, 0},
			{5, "Strange Dust", 1, 1, 0}, {6, "Recipe: Goretusk Liver Pie", 1, 1, 0}, {7, "Runed Copper Rod", 1, 1, 0},
			{8, "Raw Slitherskin Mackerel", 1, 1, 0}, {20, "Green Sword", 15, 2, 2}, {21, "Spice Bread", 1, 1, 0}, {22, "Rough Dust", 1, 1, 0},
		}),
		"recipes": table(t, []pRecipe{
			pr(400, "Cooking", "Roasted Boar Meat", nil, 1, 30, 50, "trainer"),
			pr(401, "Cooking", "Brilliant Smallfish", nil, 1, 45, 60, "trainer"),
			pr(402, "Cooking", "Slitherskin Mackerel", nil, 10, 50, 70, "trainer"),
			pr(403, "Cooking", "Goretusk Liver Pie", nil, 20, 50, 70, "item"),
			pr(500, "Blacksmithing", "Copper Mace", nil, 1, 120, 140, "trainer"),
			pr(501, "Blacksmithing", "Bronze Hammer", nil, 90, 125, 150, "trainer"),
			pr(600, "Enchanting", "Runed Copper Rod", i32(7), 1, 5, 20, "trainer"),
			pr(601, "Enchanting", "Enchant Bracer", nil, 1, 70, 90, "trainer"),
		}),
		"recipe_reagents": table(t, []reagentRow{
			{400, 2, 1}, {401, 1, 1}, {402, 8, 1}, {403, 1, 2}, {403, 2, 1}, {500, 3, 6}, {501, 4, 4}, {600, 3, 6}, {601, 5, 1},
		}),
		"vendor_sources": table(t, []pVendor{{7, 6, 1000}, {8, 2, 20}, {8, 3, 80}}),
		"recipe_items":   table(t, []pRecipeItem{{6, 403, 0, "Cooking"}}),
		"fishing_catches": table(t, []pFish{
			{1, "Elwynn Forest", 1, 0.6}, {2, "Westfall", 1, 0.3}, {2, "Westfall", 8, 0.5},
		}),
		"disenchant_yields": table(t, []pDE{{"weapon", "uncommon", 10, 20, "rated", 5}}),
		"trainers":          table(t, []pTrainer{{1, "Cook Cora", "Cooking", "Goldshire", 8}, {2, "Cook Dan", "Cooking", "Darkshire", 12}}),
		"trainer_spells":    table(t, []pTrainerSpell{{1, 401, 100}}),
		"profession_notes": table(t, []pNote{
			tier("Apprentice", 1, 75, 5), tier("Journeyman", 75, 150, 10), tier("Expert", 150, 225, 20),
			{"professions", "Cooking", str("secondary"), nil, nil, nil, "x"},
			{"feeders", "Fishing", str("Cooking"), nil, nil, nil, "Fish while you level Cooking so the catch is not wasted."},
			{"special_cases", "Enchanting", nil, nil, nil, nil, "Needs a rod, and materials come from disenchanting."},
			pair("Mining", "Blacksmithing", "Bars for every recipe."),
			pair("Herbalism", "Alchemy", "Herbs go straight into potions."),
			pair("Tailoring", "Enchanting", "Cloth gear is disenchanted."),
		}),
	})
}

func cook(skill, level int) character.Character {
	return character.Character{Name: "Cook", Class: "MAGE", Level: ip(level),
		Professions: []character.Skill{{Name: "Cooking", Rank: ip(skill), Max: ip(75)}, {Name: "Fishing", Rank: ip(30), Max: ip(75)}}}
}

var gapPhrases = []string{"data doesn't", "dataset", "general game knowledge", "missing from", "not authoritative", "advisor default", "not linked"}

func noGaps(t *testing.T, text string) {
	t.Helper()
	for _, f := range gapPhrases {
		if strings.Contains(strings.ToLower(text), f) {
			t.Errorf("output holds %q:\n%s", f, text)
		}
	}
}

func TestPathCookingFeedersAndVendorRecipe(t *testing.T) {
	ds := pathData(t)
	c := cook(1, 8)
	c.Location = &character.Location{Zone: "Goldshire"}
	pa := ComputePath(c, ds, "Cooking", 0, 1)
	text := RenderPath(pa)
	if pa.Target != 75 || len(pa.Bands) == 0 || pa.Bands[0].From != 1 {
		t.Fatalf("path %+v", pa)
	}
	for _, want := range []string{"Next: make ~", "Fish in Elwynn Forest or Westfall for Raw Brilliant Smallfish (you have Fishing 30)", "Cook Cora in Goldshire", "Journeyman, cap 150, opens at level 10"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
	noGaps(t, text)
}

func TestPathRecipeItemFromVendor(t *testing.T) {
	ds := pathData(t)
	c := cook(45, 10)
	c.Bags = []character.ItemStack{stack(1, 200, "Raw Brilliant Smallfish"), stack(2, 100, "Boar Meat")}
	// Only the pie (learn 20, item recipe) and mackerel, boar are open at 45; hold fish so the pie wins.
	pa := ComputePath(c, ds, "Cooking", 0, 1)
	text := RenderPath(pa)
	if !strings.Contains(text, "Goretusk Liver Pie") || !strings.Contains(text, "Recipe: Goretusk Liver Pie from a vendor, 10s") {
		t.Errorf("vendor recipe item not named:\n%s", text)
	}
}

func TestPathBlacksmithingUsesBankedBars(t *testing.T) {
	ds := pathData(t)
	c := character.Character{Name: "Smith", Level: ip(20), BankCaptured: true,
		Professions: []character.Skill{{Name: "Blacksmithing", Rank: ip(100), Max: ip(150)}},
		Bank:        []character.ItemStack{stack(3, 400, "Copper Bar")}}
	pa := ComputePath(c, ds, "Blacksmithing", 140, 1)
	b := pa.Bands[0]
	if b.Best.Recipe != "Copper Mace" || b.Best.Needs[0].Have == 0 || b.Best.Needs[0].Missing != 0 {
		t.Errorf("band %+v", b)
	}
	if b.From != 100 || b.To != 140 {
		t.Errorf("band runs %d→%d", b.From, b.To)
	}
}

func TestPathEnchantingDisenchants(t *testing.T) {
	ds := pathData(t)
	c := character.Character{Name: "Ench", Level: ip(10),
		Professions: []character.Skill{{Name: "Enchanting", Rank: ip(1), Max: ip(75)}},
		Bags:        []character.ItemStack{stack(20, 1, "Green Sword")}}
	text := RenderPath(ComputePath(c, ds, "Enchanting", 0, 1))
	for _, want := range []string{"Disenchant your Green Sword for Strange Dust", "Make a rod first: Runed Copper Rod", "Needs a rod"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
	noGaps(t, text)
}

func TestPathTierCapGatedByLevel(t *testing.T) {
	ds := pathData(t)
	pa := ComputePath(cook(75, 7), ds, "Cooking", 0, 1)
	if len(pa.Bands) != 0 || pa.Target != 75 {
		t.Fatalf("path %+v", pa)
	}
	if text := RenderPath(pa); !strings.Contains(text, "opens at level 10 (you are level 7)") {
		t.Errorf("gate missing:\n%s", text)
	}
	if pa := ComputePath(cook(75, 12), ds, "Cooking", 0, 1); pa.Target != 150 {
		t.Errorf("target %d at level 12, want 150", pa.Target)
	}
	if pa := ComputePath(cook(60, 7), ds, "Cooking", 200, 1); pa.Target != 75 {
		t.Errorf("a target above the gate gives %d", pa.Target)
	}
}

func TestPathPartsAndNextSkillUp(t *testing.T) {
	ds := pathData(t)
	c := cook(1, 8)
	if got := NextSkillUp(c, ds, "Cooking"); !strings.HasPrefix(got, "next skill-up: make ") {
		t.Errorf("next skill-up %q", got)
	}
	if got := NextSkillUp(c, ds, "Mining"); got != "" {
		t.Errorf("no profession gave %q", got)
	}
}

func TestPairings(t *testing.T) {
	ds := pathData(t)
	c := character.Character{Class: "MAGE", Professions: []character.Skill{{Name: "Herbalism", Rank: ip(10)}},
		Profile: &character.Profile{Role: "damage", Focus: "leveling"}}
	a := Pairings(c, ds)
	if len(a) != 3 || a[0].A != "Herbalism" || !strings.Contains(a[0].Reason, "You already have Herbalism") {
		t.Fatalf("pairings %+v", a)
	}
	if a[1].A != "Tailoring" {
		t.Errorf("a mage should see Tailoring next, got %+v", a[1])
	}
	text := RenderPairings(a)
	if !strings.Contains(text, "any two are fine") {
		t.Errorf("never-insist line missing:\n%s", text)
	}
	noGaps(t, text)
	noGaps(t, RenderPairings(nil))
}

package paths

import (
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/store"
)

type skillRecipe struct {
	SpellID int64  `parquet:"spell_id"`
	Name    string `parquet:"name"`
	Line    string `parquet:"skill_line_name"`
	Learn   int16  `parquet:"learn_skill"`
	Yellow  int16  `parquet:"yellow_skill"`
	Grey    int16  `parquet:"grey_skill"`
	Creates int64  `parquet:"creates_item_id"`
}

const (
	gauntlets, gauntletsBoP, mithril, heavy, plain, stone = 20, 21, 22, 23, 24, 25
	plansVendor, plansDrop                                = 30, 31
)

func recipeItem(id int64, name string) itemRow {
	r := item(id, name, 1, 0, 1, 0, 0, 0)
	r.ClassID = 9
	return r
}

// skillFixture: Blacksmithing recipes around a smith of 48. Runed Gauntlets (BoE) need 60 and are
// taught by a vendor's plans; Heavy Gauntlets need 40 and drop as plans; Plain Bracers need 30
// from the trainer; Mithril Gauntlets need 150; Sharpening Stone (orange at 48 up to 60) is the
// skill-up.
func skillFixture(t *testing.T) store.DataStore {
	return store.New(store.ReaderSource{
		"items": tbl(t, []itemRow{
			item(gauntlets, "Runed Gauntlets", 20, 8, 2, mail, 10, 2),
			item(gauntletsBoP, "Bound Gauntlets", 20, 8, 2, mail, 10, 1),
			item(mithril, "Mithril Gauntlets", 40, 8, 2, mail, 10, 2),
			item(heavy, "Heavy Gauntlets", 20, 8, 2, mail, 10, 2),
			item(plain, "Plain Bracers", 10, 0, 1, mail, 9, 2),
			item(stone, "Rough Sharpening Stone", 1, 0, 1, 0, 0, 0),
			item(bar, "Copper Bar", 1, 0, 1, 0, 0, 0),
			recipeItem(plansVendor, "Plans: Runed Gauntlets"),
			recipeItem(plansDrop, "Plans: Heavy Gauntlets"),
		}),
		"recipes": tbl(t, []skillRecipe{
			{900, "Runed Gauntlets", "Blacksmithing", 60, 80, 100, gauntlets},
			{901, "Bound Gauntlets", "Blacksmithing", 60, 80, 100, gauntletsBoP},
			{902, "Mithril Gauntlets", "Blacksmithing", 150, 170, 190, mithril},
			{903, "Heavy Gauntlets", "Blacksmithing", 40, 50, 70, heavy},
			{904, "Plain Bracers", "Blacksmithing", 30, 50, 70, plain},
			{905, "Rough Sharpening Stone", "Blacksmithing", 1, 60, 80, stone},
		}),
		"recipe_reagents": tbl(t, []reagentRow{{900, bar, 8}, {901, bar, 8}, {902, bar, 8}, {903, bar, 2}, {904, bar, 2}, {905, bar, 1}}),
		"vendor_sources":  tbl(t, []vendorRow{{50, plansVendor, 12500, false}}),
		"loot_sources":    tbl(t, []lootRow{{plansDrop, "creature", 601, 100, 40, tier("verified")}}),
		"npcs":            tbl(t, []npcRow{{50, "Smith Brann", 20, 20, "Dunemark"}, {601, "Rank Wolf", 9, 11, "Wolf Den"}}),
	})
}

func smith(rank int, bars int) character.Character {
	lvl := 20
	c := character.Character{Name: "Smith", Class: "WARRIOR", Level: &lvl,
		Bags: []character.ItemStack{{ItemID: bar, Count: 50, Name: "Copper Bar"}}}
	if rank >= 0 {
		c.Professions = []character.Skill{{Name: "Blacksmithing", Rank: &rank}}
	}
	if bars > 0 {
		c.Bags, c.Bank = nil, []character.ItemStack{{ItemID: bar, Count: bars, Name: "Copper Bar"}}
	}
	return c
}

func craftOf(t *testing.T, r Result) Path {
	t.Helper()
	for _, p := range r.Paths {
		if p.Kind == Craft {
			return p
		}
	}
	t.Fatalf("no craft path: %q", kinds(r))
	return Path{}
}

func TestSkillNotProfessionBoEAndBoP(t *testing.T) {
	ds := skillFixture(t)
	p := craftOf(t, For(smith(-1, 0), ds, gauntlets, Options{}))
	if p.Skill.Status != NotProfession || p.Skill.Need != 60 || !p.Informational ||
		!strings.Contains(p.Line, "Made by Blacksmiths (skill 60). You don't have Blacksmithing: buy it on the Auction House") {
		t.Fatalf("BoE: %+v", p)
	}
	p = craftOf(t, For(smith(-1, 0), ds, gauntletsBoP, Options{}))
	if strings.Contains(p.Line, "Auction House (Bind") || !strings.Contains(p.Line, "not on the Auction House") || !strings.Contains(p.Line, "ask a Blacksmith") {
		t.Fatalf("BoP: %q", p.Line)
	}
	if r := For(smith(-1, 0), ds, gauntletsBoP, Options{}); r.Dependable() {
		t.Fatal("a craft the player cannot make is not a dependable way")
	}
}

func TestSkillCanMakeNamesVendorAndDropRecipes(t *testing.T) {
	ds := skillFixture(t)
	p := craftOf(t, For(smith(70, 0), ds, gauntlets, Options{}))
	if p.Skill.Status != CanMake || !strings.Contains(p.Line, "You can make it if you have the recipe (your Blacksmithing 70, needs 60)") ||
		!strings.Contains(p.Line, "buy the recipe Plans: Runed Gauntlets from Smith Brann in Dunemark for 1g 25s") ||
		p.Recipe.Kind != "vendor" || p.Recipe.Offer == nil || strings.Contains(p.Line, "trainer") {
		t.Fatalf("vendor recipe: %+v", p)
	}
	p = craftOf(t, For(smith(70, 0), ds, heavy, Options{}))
	if p.Recipe.Kind != "drop" || !strings.Contains(p.Line, "the recipe Plans: Heavy Gauntlets drops from Rank Wolf in Wolf Den") || strings.Contains(p.Line, "trainer") {
		t.Fatalf("drop recipe: %+v", p)
	}
	p = craftOf(t, For(smith(70, 0), ds, plain, Options{}))
	if p.Recipe.Kind != "trainer" || !strings.Contains(p.Line, "from a Blacksmithing trainer") {
		t.Fatalf("trainer recipe: %+v", p)
	}
	known := smith(70, 0)
	known.KnownSpells = map[int64]bool{900: true}
	if p = craftOf(t, For(known, ds, gauntlets, Options{})); p.Recipe.Kind != "known" || !strings.Contains(p.Line, "You already know the recipe") || *p.Skill.RecipeKnown != true {
		t.Fatalf("known: %+v", p)
	}
}

func TestSkillShortByTwelveWithBankMaterials(t *testing.T) {
	r := For(smith(48, 40), skillFixture(t), gauntlets, Options{})
	p := craftOf(t, r)
	s := p.Skill
	if s.Status != Short || s.Gap != 12 || s.Have != 48 || s.Need != 60 || len(s.SkillUp) == 0 || !r.Dependable() {
		t.Fatalf("skill: %+v dependable %v", s, r.Dependable())
	}
	st := s.SkillUp[0]
	// Plain Bracers (yellow at 48 to 50, orange below) and the stone are both candidates; the
	// stone is orange for the whole climb: 12 crafts to 60, with the materials in the bank.
	if st.Recipe != "Rough Sharpening Stone" || st.Difficulty != "orange" || st.Crafts != 12 || st.Reaches != 60 || len(st.Missing) != 0 {
		t.Fatalf("step: %+v", st)
	}
	for _, want := range []string{"Your Blacksmithing is 48; it needs 60 (12 more points).", "about 12 crafts to reach 60", "you have the materials", "Then make it"} {
		if !strings.Contains(p.Line, want) {
			t.Errorf("line lacks %q: %s", want, p.Line)
		}
	}
	if len(s.SkillUp) > 3 {
		t.Errorf("more than 3 steps: %d", len(s.SkillUp))
	}
	// With no bars the same step says what is missing.
	lean := smith(48, 0)
	lean.Bags = nil
	if st := craftOf(t, For(lean, skillFixture(t), gauntlets, Options{})).Skill.SkillUp[0]; len(st.Missing) != 1 || st.Missing[0].Missing != 12 {
		t.Errorf("missing: %+v", st)
	}
}

func TestSkillFarShortLeadsWithOtherWays(t *testing.T) {
	r := For(smith(48, 0), skillFixture(t), mithril, Options{})
	p := craftOf(t, r)
	if p.Skill.Status != Far || p.Skill.Gap != 102 || len(p.Skill.SkillUp) != 0 || r.Dependable() ||
		!strings.Contains(p.Line, "far short") || !strings.Contains(p.Line, "Auction House") {
		t.Fatalf("far: %+v", p)
	}
}

func TestSkillItemRecipeNotRecordedNeedsPlans(t *testing.T) {
	ds := skillFixture(t)
	c := smith(70, 0)
	c.KnownSpells = nil // as before the addon listed spells
	c.Recipes = map[string]character.SeenRecipes{"blacksmithing": {Entries: []character.SeenRecipe{{Name: "Rough Sharpening Stone", ID: 905}}}}
	p := craftOf(t, For(c, ds, gauntlets, Options{}))
	if p.Skill.Status != NeedsRecipe || *p.Skill.RecipeKnown || strings.Contains(p.Line, "You can make it now") ||
		!strings.Contains(p.Line, "Needs the Plans: Runed Gauntlets (you don't have them). Your Blacksmithing 70 is enough once you do.") {
		t.Fatalf("not recorded: %+v", p)
	}
}

func TestSkillRecordedRecipeCountsAsKnown(t *testing.T) {
	c := smith(70, 0)
	c.Recipes = map[string]character.SeenRecipes{"blacksmithing": {Entries: []character.SeenRecipe{{Name: "runed gauntlets"}}}}
	p := craftOf(t, For(c, skillFixture(t), gauntlets, Options{}))
	if p.Skill.Status != CanMake || p.Recipe.Kind != "known" || !strings.Contains(p.Line, "You can make it now") {
		t.Fatalf("recorded: %+v", p)
	}
}

func TestSkillNoRecordHedges(t *testing.T) {
	p := craftOf(t, For(smith(70, 0), skillFixture(t), gauntlets, Options{}))
	if p.Skill.RecipeKnown != nil || !strings.Contains(p.Line, "if you have the recipe") || strings.Contains(p.Line, "You can make it now") {
		t.Fatalf("hedge: %+v", p)
	}
}

func TestSkillGapLineNamesProfession(t *testing.T) {
	p := craftOf(t, For(smith(48, 40), skillFixture(t), gauntlets, Options{}))
	if !strings.HasPrefix(p.Line, "Your Blacksmithing is 48;") {
		t.Fatal(p.Line)
	}
}

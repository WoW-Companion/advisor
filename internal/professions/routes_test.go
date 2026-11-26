package professions

import (
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/store"
)

type routeRow struct {
	ID          int64  `parquet:"id"`
	Profession  string `parquet:"profession"`
	Step        int32  `parquet:"step"`
	From        int32  `parquet:"skill_from"`
	To          int32  `parquet:"skill_to"`
	SpellID     *int64 `parquet:"recipe_spell_id,optional"`
	Recipe      string `parquet:"recipe_name"`
	Crafts      int32  `parquet:"crafts"`
	Materials   string `parquet:"materials"`
	LearnedFrom string `parquet:"learned_from"`
	Alt         int64  `parquet:"alternative_of"`
	Notes       string `parquet:"notes"`
}

type enchTrainer struct {
	Npc     int32    `parquet:"npc_id"`
	Name    string   `parquet:"name"`
	Prof    string   `parquet:"profession"`
	Zone    string   `parquet:"zone_name"`
	X       *float64 `parquet:"x,optional"`
	Y       *float64 `parquet:"y,optional"`
	Faction string   `parquet:"faction"`
	Recipes int32    `parquet:"recipes"`
}

func i64p(n int64) *int64 { return &n }

func routeDataset(t *testing.T, withRoute bool) store.DataStore {
	src := store.ReaderSource{
		"items":            table(t, []itemRow{{1, "Strange Dust"}, {2, "Runed Copper Rod"}, {3, "Soul Dust"}}),
		"profession_notes": table(t, rankTiers()),
		"trainers": table(t, []enchTrainer{
			{1, "Carolai Anise", "Enchanting", "Undercity", f64p(70), f64p(58), "Horde", 5},
			{2, "Aelthalyste", "Priest", "Undercity", f64p(49), f64p(17), "Horde", 5},
		}),
	}
	if withRoute {
		src["profession_routes"] = table(t, []routeRow{
			{1, "Enchanting", 1, 20, 70, i64p(7), "Enchant Chest - Inferior Stamina", 50, `[{"item_id":1,"count":50}]`, "trainer", 0, "rod is Blacksmith-made; buy on the AH"},
			{2, "Enchanting", 2, 70, 90, i64p(8), "Enchant Chest - Minor Absorption", 20, `[{"item_id":3,"count":20}]`, "trainer", 0, "needs Journeyman"},
			{3, "Enchanting", 3, 20, 70, i64p(9), "Enchant Bracer - Minor Stamina", 25, `[{"item_id":1,"count":50}]`, "trainer", 1, ""},
		})
	}
	return store.New(src)
}

func f64p(f float64) *float64 { return &f }

func enchanter(rod bool) character.Character {
	rank, max, lvl := 51, 75, 24
	c := character.Character{Name: "Pw", Class: "PRIEST", Faction: "Horde", Level: &lvl,
		Professions: []character.Skill{{Name: "Enchanting", Rank: &rank, Max: &max}},
		Bags:        []character.ItemStack{{ItemID: 1, Count: 60, Name: "Strange Dust"}},
		Bank:        []character.ItemStack{{ItemID: 1, Count: 32, Name: "Strange Dust"}}}
	if rod {
		c.Bags = append(c.Bags, character.ItemStack{ItemID: 2, Count: 1, Name: "Runed Copper Rod"})
	}
	return c
}

func TestRouteFactsFollowTheDatasetRoute(t *testing.T) {
	got := RouteFacts(enchanter(true), routeDataset(t, true), nil, "Enchanting")
	for _, want := range []string{"19 x Enchant Chest - Inferior Stamina", "19 Strange Dust (you have 92: 60 in bags, 32 in the bank, enough)",
		"Then: skill 70 to 90", "Minor Absorption", "Alternative for that step", "Journeyman", "Carolai Anise"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	for _, bad := range []string{"Aelthalyste", "tool (rod"} {
		if strings.Contains(got, bad) {
			t.Errorf("unexpected %q in:\n%s", bad, got)
		}
	}
}

func TestRouteFactsMentionAMissingRodOnly(t *testing.T) {
	got := RouteFacts(enchanter(false), routeDataset(t, true), nil, "Enchanting")
	if !strings.Contains(got, "no Enchanting tool") || !strings.Contains(got, "buy on the AH") {
		t.Errorf("a missing rod is not explained:\n%s", got)
	}
}

func TestNoRouteMeansNoRouteFacts(t *testing.T) {
	if got := RouteFacts(enchanter(true), routeDataset(t, false), nil, "Enchanting"); got != "" {
		t.Errorf("route facts without a route: %s", got)
	}
}

// With no route, the recipe that needs fewer of the same reagent per point wins.
func TestFallbackPrefersTheCheaperOrangeRecipe(t *testing.T) {
	ds := store.New(store.ReaderSource{
		"recipes": table(t, []recipeRow{
			{SpellID: 1, SkillLineID: 1, SkillLineName: "Enchanting", Name: str("Enchant Bracer - Minor Stamina"), LearnSkill: i16(1), YellowSkill: i16(90), GreySkill: i16(110)},
			{SpellID: 2, SkillLineID: 1, SkillLineName: "Enchanting", Name: str("Enchant Chest - Inferior Stamina"), LearnSkill: i16(1), YellowSkill: i16(90), GreySkill: i16(110)},
		}),
		"recipe_reagents": table(t, []reagentRow{{1, 1, 2}, {2, 1, 1}}),
		"items":           table(t, []itemRow{{1, "Strange Dust"}}),
	})
	steps := SkillUp(enchanter(true), ds, "Enchanting", 70, 0)
	if len(steps) != 1 || steps[0].Recipe != "Enchant Chest - Inferior Stamina" {
		t.Fatalf("steps = %+v", steps)
	}
}

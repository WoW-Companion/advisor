package professions

import (
	"fmt"
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
			{1, "Enchanting", 1, 20, 70, i64p(7), "Enchant Chest - Inferior Stamina", 50, `[{"item_id":1,"count":1}]`, "trainer", 0, "rod is Blacksmith-made; buy on the AH"},
			{2, "Enchanting", 2, 70, 90, i64p(8), "Enchant Chest - Minor Absorption", 20, `[{"item_id":3,"count":1}]`, "trainer", 0, "needs Journeyman"},
			{3, "Enchanting", 3, 20, 70, i64p(9), "Enchant Bracer - Minor Stamina", 25, `[{"item_id":1,"count":2}]`, "trainer", 1, ""},
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
	if !strings.Contains(got, "no Runed Copper Rod") || !strings.Contains(got, "buy on the AH") {
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

// The dataset's materials column reaches the store as one flat list: every item id, then every name, then
// every count. A count of 0 is a herb to gather.
func TestParseMaterialsReadsTheDatasetColumns(t *testing.T) {
	for _, c := range []struct {
		name string
		in   any
		want []RouteMaterial
	}{
		{"one", []any{int64(2589), "Linen Cloth", int64(2)}, []RouteMaterial{{2589, 2}}},
		{"two", []any{int64(2996), int64(2320), "Bolt of Linen Cloth", "Coarse Thread", int64(1), int64(3)}, []RouteMaterial{{2996, 1}, {2320, 3}}},
		{"gather", []any{int64(2447), int64(765), "Peacebloom", "Silverleaf", int64(0), int64(0)}, []RouteMaterial{{2447, 0}, {765, 0}}},
		{"empty", []any{}, nil},
		{"ragged", []any{int64(1), "x"}, nil},
		{"json", `[{"item_id":1,"count":2}]`, []RouteMaterial{{1, 2}}},
		{"pairs", "1:2,3:4", []RouteMaterial{{1, 2}, {3, 4}}},
	} {
		if got := parseMaterials(c.in); fmt.Sprint(got) != fmt.Sprint(c.want) {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

// A gathering route (kind gather: no recipe, zones instead) is read and told, with the smelting alternatives
// Mining has beside its gathering steps.
func TestRouteFactsTellGatheringSteps(t *testing.T) {
	ds := snapshot().ds
	r, ok := LoadRoute(ds, "Herbalism")
	if !ok || len(r.Steps) == 0 || !r.Steps[0].Gather || len(r.Steps[0].Where) == 0 || len(r.Steps[0].Materials) == 0 {
		t.Fatalf("Herbalism route = %v, %+v", ok, r.Steps)
	}
	got := RouteFacts(routeChar("Herbalism", 20), ds, nil, "Herbalism")
	if !strings.Contains(got, "Now: skill 20 to 70: gather in Durotar") || !strings.Contains(got, "gather: Peacebloom, Silverleaf, Earthroot") {
		t.Errorf("gathering step not told:\n%s", got)
	}
	got = RouteFacts(routeChar("Mining", 30), ds, nil, "Mining")
	if !strings.Contains(got, "Alternative for that step") || !strings.Contains(got, "Smelt Copper") {
		t.Errorf("the smelting alternative is missing:\n%s", got)
	}
	if s := NextSkillUp(routeChar("Herbalism", 20), ds, "Herbalism"); strings.Contains(s, "make") {
		t.Errorf("a gathering step is no craft: %q", s)
	}
}

// Craft steps carry the materials of the dataset's column: counted for the crafts left.
func TestRouteFactsCountTheDatasetMaterials(t *testing.T) {
	got := RouteFacts(routeChar("Tailoring", 45), snapshot().ds, nil, "Tailoring")
	if !strings.Contains(got, "Now: skill 45 to 55: 12 x Linen Belt; materials: 12 Bolt of Linen Cloth (you have none); 12 Coarse Thread (you have none)") {
		t.Errorf("materials not counted:\n%s", got)
	}
}

func TestToolOfNamesTheProfessionOfATool(t *testing.T) {
	for name, want := range map[string]string{"Mining Pick": "Mining", "Skinning Knife": "Skinning", "Blacksmith Hammer": "Blacksmithing", "Arclight Spanner": "Engineering",
		"Runed Copper Rod": "Enchanting", "Strong Fishing Pole": "Fishing", "Linen Bag": "", "Copper Bar": ""} {
		if got := ToolOf(name); got != want {
			t.Errorf("ToolOf(%q) = %q, want %q", name, got, want)
		}
	}
	if has, needed := HasTool(character.Character{}, "Engineering"); has || !needed {
		t.Errorf("Engineering needs the Arclight Spanner: has %v needed %v", has, needed)
	}
}

func TestACreatureHeldPropIsNoTool(t *testing.T) {
	for _, name := range []string{"Monster - Mining Pick", "Monster - Item, Fishing Pole", "monster - skinning knife"} {
		if got := ToolOf(name); got != "" {
			t.Errorf("ToolOf(%q) = %q, want none", name, got)
		}
	}
	if got := ToolOf("Mining Pick"); got != "Mining" {
		t.Errorf("ToolOf(Mining Pick) = %q", got)
	}
	rank, max := 30, 75
	c := character.Character{Professions: []character.Skill{{Name: "Mining", Rank: &rank, Max: &max}},
		Bags: []character.ItemStack{{Count: 1, Name: "Monster - Mining Pick"}}}
	if has, needed := HasTool(c, "Mining"); has || !needed {
		t.Errorf("HasTool with only a creature's prop = %v, %v; want false, true", has, needed)
	}
}

package professions

import (
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
)

func rodEnchanter() character.Character {
	return character.Character{Name: "Ench", Class: "MAGE", Level: ip(20), Faction: "Alliance",
		Professions: []character.Skill{{Name: "Enchanting", Rank: ip(10), Max: ip(75)}}}
}

// A crafted product in the bags with no captured recipe list is "likely known": never learn it again,
// and the player is told to open the window once.
func TestCarriedProductIsLikelyKnownNotLearnable(t *testing.T) {
	c := rodEnchanter()
	c.Bags = []character.ItemStack{stack(7, 2, "Runed Copper Rod")}
	pa := ComputePath(c, pathData(t), "Enchanting", 40, 1)
	notes := strings.Join(pa.Notes, " ")
	if !strings.Contains(notes, "likely know Runed Copper Rod") || !strings.Contains(notes, character.OpenWindowTip("Enchanting")) {
		t.Errorf("no likely-known note and tip: %q", notes)
	}
	for _, b := range pa.Bands {
		if b.Best.Recipe == "Runed Copper Rod" && b.Best.Learn != "" {
			t.Errorf("an inferred-known recipe is offered to learn: %+v", b.Best)
		}
	}
	if c.KnowsRecipe("Enchanting", 600, "Runed Copper Rod", 7) != character.RecipeLikelyKnown {
		t.Error("KnowsRecipe is not likely known")
	}
	c.Bags = nil
	if c.KnowsRecipe("Enchanting", 600, "Runed Copper Rod", 7) == character.RecipeLikelyKnown {
		t.Error("no product carried, yet likely known")
	}
}

// A captured craft-window list (Poisons, Enchanting, Beast Training) decides what is known and needs no tip.
func TestCapturedCraftWindowListIsUsed(t *testing.T) {
	c := character.Character{Class: "ROGUE", Level: ip(32), Faction: "Horde",
		Recipes: map[string]character.SeenRecipes{"poisons": {SeenAt: 1, Entries: []character.SeenRecipe{{Name: "Instant Poison II", Makes: 6950}}}}}
	c.Bags = []character.ItemStack{stack(6950, 5, "Instant Poison II")}
	if got := c.KnowsRecipe("Poisons", 0, "Instant Poison II", 6950); got != character.RecipeKnown {
		t.Errorf("a captured Poisons recipe is %v, want known", got)
	}
	if got := c.KnowsRecipe("Poisons", 0, "Deadly Poison", 2892); got != character.RecipeNotKnown {
		t.Errorf("a recipe missing from the captured list is %v, want not known", got)
	}
	c = c.WithRecipeProfessions()
	cov := CoverageOf(c, pathData(t))
	if len(cov) != 1 || cov[0].Profession != "Poisons" || cov[0].Captured != 1 {
		t.Errorf("coverage %+v", cov)
	}
	if line := (Coverage{Profession: "Poisons"}).Line(); !strings.Contains(line, "NOT captured") {
		t.Errorf("uncaptured line: %s", line)
	}
}

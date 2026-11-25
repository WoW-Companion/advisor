package character

import "testing"

func TestRecipesAloneMakeAKnownProfession(t *testing.T) {
	c := Character{Recipes: map[string]SeenRecipes{
		"tailoring":  {Entries: []SeenRecipe{{Name: "Linen Bag"}}},
		"enchanting": {Entries: []SeenRecipe{{Name: "Minor Health"}}},
	}}.WithRecipeProfessions()
	if len(c.Professions) != 2 || c.Professions[0].Name != "Enchanting" || !c.Professions[1].FromRecipes || c.Professions[1].Rank != nil {
		t.Errorf("professions = %+v", c.Professions)
	}
	r := 35
	c.Professions[1] = Skill{Name: "Tailoring", Rank: &r}
	if c2 := c.WithRecipeProfessions(); len(c2.Professions) != 2 || c2.Professions[1].Rank == nil {
		t.Errorf("a captured skill was replaced: %+v", c2.Professions)
	}
}

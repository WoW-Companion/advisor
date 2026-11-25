package ask

import (
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/store"
)

type recipeRow struct {
	SpellID       int32   `parquet:"spell_id"`
	SkillLineName string  `parquet:"skill_line_name"`
	Name          *string `parquet:"name,optional"`
	LearnSkill    *int16  `parquet:"learn_skill,optional"`
	YellowSkill   *int16  `parquet:"yellow_skill,optional"`
	GreySkill     *int16  `parquet:"grey_skill,optional"`
	LearnedFrom   string  `parquet:"learned_from"`
}

func profLoaded(t *testing.T) loaded {
	lvl := 24
	n1, n2 := "Bolt of Linen Cloth", "Linen Bag"
	c := character.Character{Name: "Pw", Class: "PRIEST", Faction: "Alliance", Level: &lvl,
		Professions: []character.Skill{{Name: "Tailoring", FromRecipes: true}},
		Recipes: map[string]character.SeenRecipes{"tailoring": {Entries: []character.SeenRecipe{
			{Name: n1, Difficulty: "yellow"}, {Name: n2, Difficulty: "orange"}}}}}
	ds := store.New(store.ReaderSource{"recipes": table(t, []recipeRow{
		{1, "Tailoring", &n1, i16(1), i16(1), i16(30), "trainer"},
		{2, "Tailoring", &n2, i16(10), i16(60), i16(80), "trainer"},
	})})
	return loaded{c: c, ds: ds}
}

func TestProfessionQuestionCarriesThePlanFacts(t *testing.T) {
	cc := profLoaded(t)
	got := professionPlanFacts(cc, "How do I level tailoring?")
	for _, want := range []string{"HAS Tailoring", "leveling plan", "Linen"} {
		if !strings.Contains(got, want) {
			t.Errorf("facts lack %q:\n%s", want, got)
		}
	}
	if got := professionPlanFacts(cc, "What should I do next?"); got != "" {
		t.Errorf("a non-profession question got plan facts: %s", got)
	}
	if got := professionPlanFacts(cc, "How do I level mining?"); got != "" {
		t.Errorf("a profession the character lacks got plan facts: %s", got)
	}
}

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

func TestProfessionWithoutARouteSaysEstimated(t *testing.T) {
	got := professionPlanFacts(profLoaded(t), "How do I level tailoring?")
	if !strings.Contains(got, "estimated") {
		t.Errorf("a profession with no route is not marked estimated:\n%s", got)
	}
}

// A vendor, trainer or flight-path question is answered from find_vendor, never "can't confirm": the
// prompt names the tool and forbids answering without it, and an empty lookup is "not in my data".
func TestPromptCallsVendorLookupFirst(t *testing.T) {
	cfg := Config{Command: Presets["claude"]}
	p := cfg.Prompt(Question{Text: "Is there a poison vendor in Booty Bay?"})
	for _, want := range []string{"find_vendor first", "never say you cannot confirm", "not in my data", "Open your <Profession> window once so I can see your recipes"} {
		if !strings.Contains(p, want) {
			t.Errorf("the prompt lacks %q", want)
		}
	}
}

// A leveling question names a secondary skill however it is spelled or punctuated, by an alias of it, or by "get it to 150".
func TestProfessionAskedSpellings(t *testing.T) {
	c := character.Character{Professions: []character.Skill{{Name: "First Aid"}, {Name: "Fishing"}, {Name: "Cooking"}}}
	for _, tc := range []struct{ q, want string }{
		{"how do I level first aid?", "First Aid"}, {"how do I level first-aid?", "First Aid"}, {"How do I level up my bandaging?", "First Aid"}, {"get my First Aid to 150", "First Aid"},
		{"level my fishing!", "Fishing"}, {"how do I raise my fishing's skill", "Fishing"}, {"how do I raise my angling skill?", "Fishing"},
		{"how can I get cooking to 150?", "Cooking"}, {"how do I level up my cooking: what recipes?", "Cooking"}, {"how do I level cooking\n", "Cooking"}, {"what should I cook to level up?", "Cooking"},
		{"what should I do next?", ""}, {"where can I get fish for cooking?", ""}, {"how do I level mining?", ""}, {"what is a good fishing spot?", ""},
	} {
		if got := professionAsked(c, tc.q); got != tc.want {
			t.Errorf("professionAsked(%q) = %q, want %q", tc.q, got, tc.want)
		}
	}
}

package professions

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/store"
)

// The offline skill-up benchmark: which recipes the advisor lists to level a profession from a skill, in what colour
// and order, and how many crafts it says, beside the dataset's recipe thresholds (learn_skill, yellow_skill,
// grey_skill), with no network. A recipe is orange below its yellow skill, yellow from there to halfway to grey,
// green from there to its grey skill and grey after; a craft gives a skill-up always when orange, three times in
// four when yellow and one in four when green (docs/professions.md; the craft counts of the wow-professions Forever
// guides agree with these chances, which TestTailoringGuideBands and the route benchmark check). make
// offline-benchmark runs it.

const skillSrc = "dataset recipes (learn_skill, yellow_skill, grey_skill), recipe_reagents; wow-professions guides"

// refColour is the game's colour of a recipe at a skill from the three thresholds: "" when the recipe cannot be
// learned yet, "grey" at or past the grey skill.
func refColour(learn, yellow, grey, skill int) string {
	switch {
	case learn > skill:
		return ""
	case skill < yellow:
		return "orange"
	case skill < (yellow+grey)/2:
		return "yellow"
	case skill < grey:
		return "green"
	}
	return "grey"
}

type refRecipe struct {
	name                string
	learn, yellow, grey int
}

// refRecipes are the dataset's recipe rows of a profession that carry all three thresholds, ordered as the table is.
func refRecipes(t *testing.T, ds store.DataStore, prof string) []refRecipe {
	t.Helper()
	rows, err := ds.Filter("recipes", store.Range{Column: "spell_id", Min: int64(0)})
	if err != nil {
		t.Fatal(err)
	}
	var out []refRecipe
	for _, r := range rows {
		if asText(r["skill_line_name"]) != prof || r["learn_skill"] == nil || r["yellow_skill"] == nil || r["grey_skill"] == nil {
			continue
		}
		out = append(out, refRecipe{asText(r["name"]), int(asI64(r["learn_skill"])), int(asI64(r["yellow_skill"])), int(asI64(r["grey_skill"]))})
	}
	return out
}

// refCrafts are the crafts to climb from skill have to the skill reached on a recipe, by the chances in the guide fit.
func refCrafts(r refRecipe, have, reaches int) int {
	mid := (r.yellow + r.grey) / 2
	n := 0.0
	n += float64(max(0, min(reaches, r.yellow)-have))
	n += math.Ceil(float64(max(0, min(reaches, mid)-max(have, r.yellow)))/0.75 - 1e-9)
	n += 4 * float64(max(0, reaches-max(have, mid)))
	return int(n)
}

var skillUpCases = []struct {
	prof  string
	skill int
}{
	{"Tailoring", 1}, {"Tailoring", 20}, {"Tailoring", 40}, {"Tailoring", 30}, {"Leatherworking", 15}, {"Leatherworking", 40},
	{"Leatherworking", 55}, {"Blacksmithing", 30}, {"Blacksmithing", 60}, {"Alchemy", 25}, {"Alchemy", 55}, {"Cooking", 10},
	{"Cooking", 45}, {"Enchanting", 35}, {"Engineering", 50}, {"First Aid", 30}, {"Engineering", 45}, {"First Aid", 45}, {"First Aid", 75},
}

// The skill-up path for characters at a skill; fifteen tests, so each stays inside the CPU cap of a test. For characters at a skill, every recipe the skill-up path lists is one the dataset's
// thresholds make orange, yellow or green for them (never grey, never one they cannot learn yet), is worded with
// its colour, comes after one of a better colour of the same kind, and its crafts to the skill reached match the
// chances of its colour.
func TestOfflineBenchmarkSkillUpColoursA(t *testing.T) { skillUpColours(t, "a", skillUpCases[0:1]) }
func TestOfflineBenchmarkSkillUpColoursB(t *testing.T) { skillUpColours(t, "b", skillUpCases[1:2]) }
func TestOfflineBenchmarkSkillUpColoursC(t *testing.T) { skillUpColours(t, "c", skillUpCases[2:3]) }
func TestOfflineBenchmarkSkillUpColoursD(t *testing.T) { skillUpColours(t, "d", skillUpCases[3:4]) }
func TestOfflineBenchmarkSkillUpColoursE(t *testing.T) { skillUpColours(t, "e", skillUpCases[4:5]) }
func TestOfflineBenchmarkSkillUpColoursF(t *testing.T) { skillUpColours(t, "f", skillUpCases[5:6]) }
func TestOfflineBenchmarkSkillUpColoursG(t *testing.T) { skillUpColours(t, "g", skillUpCases[6:7]) }
func TestOfflineBenchmarkSkillUpColoursH(t *testing.T) { skillUpColours(t, "h", skillUpCases[7:8]) }
func TestOfflineBenchmarkSkillUpColoursI(t *testing.T) { skillUpColours(t, "i", skillUpCases[8:9]) }
func TestOfflineBenchmarkSkillUpColoursJ(t *testing.T) { skillUpColours(t, "j", skillUpCases[9:10]) }
func TestOfflineBenchmarkSkillUpColoursK(t *testing.T) { skillUpColours(t, "k", skillUpCases[10:11]) }
func TestOfflineBenchmarkSkillUpColoursL(t *testing.T) { skillUpColours(t, "l", skillUpCases[11:13]) }
func TestOfflineBenchmarkSkillUpColoursM(t *testing.T) { skillUpColours(t, "m", skillUpCases[13:15]) }
func TestOfflineBenchmarkSkillUpColoursN(t *testing.T) { skillUpColours(t, "n", skillUpCases[15:17]) }
func TestOfflineBenchmarkSkillUpColoursO(t *testing.T) { skillUpColours(t, "o", skillUpCases[17:19]) }

func skillUpColours(t *testing.T, part string, cases []struct {
	prof  string
	skill int
}) {
	ds := snapshot().ds
	var out []benchRow
	for _, tc := range cases {
		refs := refRecipes(t, ds, tc.prof)
		byName := map[string][]refRecipe{}
		for _, r := range refs {
			byName[r.name] = append(byName[r.name], r)
		}
		target := tc.skill + 25
		steps := SkillUp(routeChar(tc.prof, tc.skill), ds, tc.prof, target, 0)
		var got, ref []string
		ok := true
		bad := ""
		for i, s := range steps {
			var r refRecipe
			var col string
			for _, c := range byName[s.Recipe] {
				if k := refColour(c.learn, c.yellow, c.grey, tc.skill); k == "orange" || k == "yellow" || k == "green" {
					r, col = c, k
					break
				}
			}
			got = append(got, fmt.Sprintf("%s %s %d crafts to %d", s.Recipe, s.Difficulty, s.Crafts, s.Reaches))
			if col == "" {
				ok, bad = false, s.Recipe+" is grey, not learnable yet or not in the dataset"
				ref = append(ref, s.Recipe+": none eligible")
				continue
			}
			reaches := min(target, r.grey)
			want := refCrafts(r, tc.skill, reaches)
			ref = append(ref, fmt.Sprintf("%s %s %d crafts to %d", r.name, col, want, reaches))
			if s.Difficulty != col || s.Crafts != want || s.Reaches != reaches {
				ok, bad = false, "the colour or the crafts of "+s.Recipe
			}
			// among recipes of the same availability (materials held, known), a better colour comes first
			if i > 0 {
				p := steps[i-1]
				if (p.MissingReagents == 0) == (s.MissingReagents == 0) && p.Known == s.Known && colourRank(p.Difficulty) > colourRank(s.Difficulty) {
					ok, bad = false, "a "+p.Difficulty+" recipe listed before a "+s.Difficulty+" one"
				}
			}
		}
		// the dataset has recipes in reach, so the path is not empty
		any := false
		for _, r := range refs {
			if k := refColour(r.learn, r.yellow, r.grey, tc.skill); k == "orange" || k == "yellow" || k == "green" {
				any = true
			}
		}
		if any && len(steps) == 0 {
			ok, bad = false, "no recipe listed though the dataset has some in reach"
		}
		if len(steps) > SkillUpMax {
			ok, bad = false, "more recipes than the limit"
		}
		out = append(out, benchRow{fmt.Sprintf("%s skill %d: skill-up recipes", tc.prof, tc.skill), strings.Join(got, "; "), strings.Join(ref, "; "), skillSrc, resultIf(ok, bad)})
	}
	finishOffline(t, "advice-skillup-colours-"+part, "Offline skill-up: recipe colour, order and crafts ("+part+")", out)
}

// TestOfflineBenchmarkSkillUpGreen: the colour of a recipe at a skill and the rate worded for it, from the dataset's
// thresholds: orange one skill-up per craft, yellow three in four, green one in four, grey none. The three words
// differ, and a green recipe is not called yellow.
func TestOfflineBenchmarkSkillUpGreen(t *testing.T) {
	ds := snapshot().ds
	recipes, err := Recipes(ds, "Tailoring")
	if err != nil {
		t.Fatal(err)
	}
	var out []benchRow
	for _, tc := range []struct {
		recipe      string
		skill       int
		colour, per string
	}{
		{"Linen Bag", 20, "orange", "about 1 point per craft"},
		{"Linen Bag", 29, "orange", "about 1 point per craft"},
		{"Linen Bag", 30, "yellow", "about 3 points per 4 crafts"},
		{"Linen Bag", 40, "yellow", "about 3 points per 4 crafts"},
		{"Linen Bag", 46, "yellow", "about 3 points per 4 crafts"},
		{"Linen Bag", 47, "green", "about 1 point per 4 crafts"}, // halfway of 30 and 65, rounded down
		{"Linen Bag", 64, "green", "about 1 point per 4 crafts"},
		{"Linen Bag", 65, "", ""},
		{"Brown Linen Vest", 45, "yellow", "about 3 points per 4 crafts"},
		{"Brown Linen Vest", 58, "green", "about 1 point per 4 crafts"},
	} {
		var r refRecipe
		var rec Recipe
		for _, c := range refRecipes(t, ds, "Tailoring") {
			if c.name == tc.recipe {
				r = c
				break
			}
		}
		for _, c := range recipes {
			if c.Name == tc.recipe {
				rec = c
				break
			}
		}
		col := refColour(r.learn, r.yellow, r.grey, tc.skill)
		if col == "grey" {
			col = ""
		}
		got := colourAt(rec, tc.skill)
		out = append(out, benchRow{fmt.Sprintf("%s at %d (yellow %d, grey %d): colour and rate", tc.recipe, tc.skill, r.yellow, r.grey), got + ", " + SkillPerCraft(got), tc.colour + ", " + tc.per, skillSrc,
			resultIf(col == tc.colour && got == tc.colour && (got == "" || SkillPerCraft(got) == tc.per), "the colour or the rate")})
	}
	finishOffline(t, "advice-skillup-green", "Offline skill-up: colour wording", out)
}

// TestOfflineBenchmarkSkillUpNoAnswer: skill-up questions the dataset cannot answer. The advisor lists no recipe.
func TestOfflineBenchmarkSkillUpNoAnswer(t *testing.T) {
	ds := snapshot().ds
	var out []benchRow
	const none = "the dataset has no recipe that gives skill for it"
	count := func(c character.Character, prof string, target int) int { return len(SkillUp(c, ds, prof, target, 0)) }
	out = append(out, benchRow{"Jewelcrafting: the dataset snapshot holds no recipes", fmt.Sprintf("%d recipes", count(routeChar("Jewelcrafting", 50), "Jewelcrafting", 75)), none, skillSrc,
		resultIf(count(routeChar("Jewelcrafting", 50), "Jewelcrafting", 75) == 0, "a recipe was invented")})
	out = append(out, benchRow{"Fishing: no recipes, the route is the answer", fmt.Sprintf("%d recipes", count(routeChar("Fishing", 50), "Fishing", 75)), none, skillSrc,
		resultIf(count(routeChar("Fishing", 50), "Fishing", 75) == 0, "a recipe was invented")})
	out = append(out, benchRow{"Cooking at 300: every recipe is grey", fmt.Sprintf("%d recipes", count(routeChar("Cooking", 300), "Cooking", 325)), none, skillSrc,
		resultIf(count(routeChar("Cooking", 300), "Cooking", 325) == 0, "a grey recipe was listed")})
	out = append(out, benchRow{"Alchemy with no capture of the profession", fmt.Sprintf("%d recipes", count(character.Character{Class: "MAGE"}, "Alchemy", 50)), none, skillSrc,
		resultIf(count(character.Character{Class: "MAGE"}, "Alchemy", 50) == 0, "a path was made without a skill")})
	lowSkill := routeChar("Tailoring", 1)
	steps := SkillUp(lowSkill, ds, "Tailoring", 1, 0)
	out = append(out, benchRow{"Tailoring 1 to 1: nothing to climb", fmt.Sprintf("%d recipes", len(steps)), none, skillSrc, resultIf(len(steps) == 0, "a recipe was listed for no climb")})
	finishOffline(t, "advice-skillup-no-answer", "Offline skill-up advice, no answer in the dataset", out)
}

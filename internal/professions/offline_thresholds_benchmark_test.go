package professions

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/store"
)

// The offline skill-up threshold benchmark: the colour the advisor gives a recipe at the skill a player stands on the edge of, beside
// the dataset's learn, yellow and grey skills (the game's orange, yellow, green and grey bands: orange from the learn skill, yellow
// from the yellow skill, green halfway to grey, grey from the grey skill), and what the skill-up path says about that recipe alone. It
// reads testdata/tradeskills_snapshot, the whole recipes table. A recipe the dataset has no thresholds for is not coloured: the advisor
// says so and lists none. make offline-benchmark runs it; WOWC_BENCH_REPORT receives the tables.

const (
	thrDir = "../../testdata/tradeskills_snapshot"
	thrSrc = "dataset recipes (learn_skill, yellow_skill, grey_skill); game: Warcraft Wiki Profession skill-up colours (orange, yellow, green, grey)"
)

var (
	thrOnce  sync.Once
	thrCache store.DataStore
)

func thrStore() store.DataStore {
	thrOnce.Do(func() { thrCache = store.NewDir(thrDir) })
	return thrCache
}

// thrOne serves the recipes table as one recipe row, so the skill-up path lists that recipe or nothing.
type thrOne struct {
	store.DataStore
	spell int64
}

func (o thrOne) Filter(table string, r ...store.Range) ([]store.Row, error) {
	rows, err := o.DataStore.Filter(table, r...)
	if table != "recipes" {
		return rows, err
	}
	var out []store.Row
	for _, row := range rows {
		if asI64(row["spell_id"]) == o.spell {
			out = append(out, row)
		}
	}
	return out, err
}

func thrRecipe(t *testing.T, name, prof string) store.Row {
	t.Helper()
	rows, err := thrStore().Filter("recipes", store.Range{Column: "name", Min: name, Max: name})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if asText(r["skill_line_name"]) == prof {
			return r
		}
	}
	t.Fatalf("%s %q: not in the snapshot", prof, name)
	return nil
}

var thrCases = []struct {
	recipe, prof string
	skill        int
	colour       string // the game's colour at the skill; "" for grey or not yet learnable
}{
	{"Heavy Linen Bandage", "First Aid", 49, "orange"}, {"Heavy Linen Bandage", "First Aid", 50, "yellow"}, {"Heavy Linen Bandage", "First Aid", 74, "yellow"},
	{"Heavy Linen Bandage", "First Aid", 75, "green"}, {"Heavy Linen Bandage", "First Aid", 99, "green"}, {"Heavy Linen Bandage", "First Aid", 100, ""},
	{"Silk Bandage", "First Aid", 149, ""}, {"Silk Bandage", "First Aid", 150, "yellow"}, {"Silk Bandage", "First Aid", 179, "yellow"}, {"Silk Bandage", "First Aid", 180, "green"},
	{"Bolt of Woolen Cloth", "Tailoring", 55, "orange"}, {"Bolt of Woolen Cloth", "Tailoring", 69, "orange"}, {"Bolt of Woolen Cloth", "Tailoring", 76, "yellow"},
	{"Bolt of Woolen Cloth", "Tailoring", 77, "green"}, {"Bolt of Woolen Cloth", "Tailoring", 84, "green"}, {"Bolt of Woolen Cloth", "Tailoring", 85, ""},
	{"Mageweave Bag", "Tailoring", 189, ""}, {"Mageweave Bag", "Tailoring", 190, "orange"}, {"Mageweave Bag", "Tailoring", 204, "orange"}, {"Mageweave Bag", "Tailoring", 219, "yellow"},
	{"Mageweave Bag", "Tailoring", 220, "green"}, {"Mageweave Bag", "Tailoring", 234, "green"},
	{"Handstitched Leather Belt", "Leatherworking", 24, ""}, {"Handstitched Leather Belt", "Leatherworking", 25, "orange"}, {"Handstitched Leather Belt", "Leatherworking", 69, "yellow"},
	{"Handstitched Leather Belt", "Leatherworking", 70, "green"}, {"Handstitched Leather Belt", "Leatherworking", 85, ""},
	{"Spiced Wolf Meat", "Cooking", 49, "orange"}, {"Spiced Wolf Meat", "Cooking", 50, "yellow"}, {"Spiced Wolf Meat", "Cooking", 70, "green"}, {"Spiced Wolf Meat", "Cooking", 90, ""},
	{"Mithril Spurs", "Blacksmithing", 209, ""}, {"Mithril Spurs", "Blacksmithing", 229, "orange"}, {"Mithril Spurs", "Blacksmithing", 239, "yellow"}, {"Mithril Spurs", "Blacksmithing", 240, "green"},
	{"Mithril Spurs", "Blacksmithing", 250, ""},
}

// TestOfflineBenchmarkThresholdColours: at each edge skill, the colour the advisor gives, the rate it words for it, and what the
// skill-up path lists for the recipe alone, are the colour the dataset's three skills give: orange below yellow, yellow up to
// halfway to grey (rounded down), green up to grey, and nothing at grey or before the recipe is learnable.
func TestOfflineBenchmarkThresholdColoursA(t *testing.T) { thresholdColours(t, "a", thrCases[:6]) }
func TestOfflineBenchmarkThresholdColoursB(t *testing.T) { thresholdColours(t, "b", thrCases[6:12]) }
func TestOfflineBenchmarkThresholdColoursC(t *testing.T) { thresholdColours(t, "c", thrCases[12:18]) }
func TestOfflineBenchmarkThresholdColoursD(t *testing.T) { thresholdColours(t, "d", thrCases[18:24]) }
func TestOfflineBenchmarkThresholdColoursE(t *testing.T) { thresholdColours(t, "e", thrCases[24:30]) }
func TestOfflineBenchmarkThresholdColoursF(t *testing.T) { thresholdColours(t, "f", thrCases[30:]) }

func thresholdColours(t *testing.T, part string, cases []struct {
	recipe, prof string
	skill        int
	colour       string
}) {
	ds := thrStore()
	var out []benchRow
	for _, tc := range cases {
		row := thrRecipe(t, tc.recipe, tc.prof)
		learn, yellow, grey := int(asI64(row["learn_skill"])), int(asI64(row["yellow_skill"])), int(asI64(row["grey_skill"]))
		ref := refColour(learn, yellow, grey, tc.skill)
		if ref == "grey" {
			ref = ""
		}
		recs, err := Recipes(thrOne{ds, asI64(row["spell_id"])}, tc.prof)
		if err != nil || len(recs) != 1 {
			t.Fatalf("%s: %v", tc.recipe, err)
		}
		got := ""
		if recs[0].LearnSkill != nil && *recs[0].LearnSkill <= tc.skill {
			got = colourAt(recs[0], tc.skill)
		}
		steps := SkillUp(routeChar(tc.prof, tc.skill), thrOne{ds, asI64(row["spell_id"])}, tc.prof, 300, 0)
		listed := ""
		if len(steps) == 1 {
			listed = steps[0].Difficulty
		}
		ok := ref == tc.colour && strings.EqualFold(got, tc.colour) && strings.EqualFold(listed, tc.colour)
		rate := "none"
		if got != "" {
			rate = SkillPerCraft(got)
		}
		if tc.colour != "" {
			ok = ok && SkillPerCraft(tc.colour) == rate
		}
		want := tc.colour
		if want == "" {
			want = "no recipe listed (grey or not learnable yet)"
		}
		out = append(out, benchRow{fmt.Sprintf("%s (%s) at skill %d", tc.recipe, tc.prof, tc.skill), fmt.Sprintf("colour %q, path lists %q, %s", got, listed, rate),
			fmt.Sprintf("%s (learn %d, yellow %d, green %d, grey %d)", want, learn, yellow, (yellow+grey)/2, grey), thrSrc, resultIf(ok, "the colour at the edge of a band")})
	}
	finishOffline(t, "advice-threshold-colours-"+part, "Offline skill-up thresholds: the colour at the edge of a band ("+part+")", out)
}

// TestOfflineBenchmarkThresholdNoAnswer: a recipe the dataset has no thresholds for is not coloured, and the path lists none. A recipe
// that does not exist is not invented.
func TestOfflineBenchmarkThresholdNoAnswer(t *testing.T) {
	ds := thrStore()
	var out []benchRow
	yellow, grey := 50, 100
	learn := 40
	for _, tc := range []struct {
		label string
		r     Recipe
	}{
		{"no yellow or grey skill", Recipe{Name: "Synthetic Recipe", LearnSkill: &learn}},
		{"no grey skill", Recipe{Name: "Synthetic Recipe", LearnSkill: &learn, YellowSkill: &yellow}},
		{"no yellow skill", Recipe{Name: "Synthetic Recipe", LearnSkill: &learn, GreySkill: &grey}},
	} {
		got := colourAt(tc.r, 60)
		steps := len(SkillUp(routeChar("First Aid", 60), thrNone{ds, tc.r}, "First Aid", 100, 0))
		out = append(out, benchRow{"a recipe with " + tc.label, fmt.Sprintf("colour %q, %d listed", got, steps), "the dataset gives no colour: the recipe is not coloured or listed", thrSrc,
			resultIf(got == Unknown && steps == 0, "a colour was guessed")})
	}
	out = append(out, benchRow{"a recipe the dataset does not hold", fmt.Sprintf("%d listed", len(SkillUp(character.Character{}, ds, "Basket Weaving", 100, 0))), "no recipes row for the profession", thrSrc,
		resultIf(len(SkillUp(character.Character{}, ds, "Basket Weaving", 100, 0)) == 0, "a recipe was invented")})
	finishOffline(t, "advice-threshold-no-answer", "Offline skill-up thresholds: no thresholds in the dataset", out)
}

// thrNone serves one recipe row built from a Recipe, with the thresholds it has.
type thrNone struct {
	store.DataStore
	r Recipe
}

func (o thrNone) Filter(table string, r ...store.Range) ([]store.Row, error) {
	if table != "recipes" {
		return o.DataStore.Filter(table, r...)
	}
	row := store.Row{"spell_id": int64(999001), "skill_line_name": "First Aid", "name": o.r.Name, "learn_skill": int64(*o.r.LearnSkill), "learned_from": "trainer"}
	if o.r.YellowSkill != nil {
		row["yellow_skill"] = int64(*o.r.YellowSkill)
	}
	if o.r.GreySkill != nil {
		row["grey_skill"] = int64(*o.r.GreySkill)
	}
	return []store.Row{row}, nil
}

// TestOfflineBenchmarkThresholdEstimates: a recipe whose learn skill the dataset only estimates (learn_skill_source "estimate") is
// listed from no earlier than ten points under its yellow skill, so a path never sends a player to a recipe far below the skill it
// is really taught at. The dataset's estimate sits well under that, and the game teaches such recipes at a skill the dataset does
// not hold, so the gap is the dataset's, not the advisor's.
func TestOfflineBenchmarkThresholdEstimates(t *testing.T) {
	ds := thrStore()
	var out []benchRow
	for _, tc := range []struct{ recipe, prof string }{{"Elixir of Wisdom", "Alchemy"}, {"Lesser Mana Potion", "Alchemy"}, {"Blackmouth Oil", "Alchemy"}} {
		row := thrRecipe(t, tc.recipe, tc.prof)
		learn, yellow := int(asI64(row["learn_skill"])), int(asI64(row["yellow_skill"]))
		one := thrOne{ds, asI64(row["spell_id"])}
		first := 0
		for _, s := range []int{learn, yellow - estimateMargin - 1, yellow - estimateMargin} {
			if first == 0 && len(SkillUp(routeChar(tc.prof, s), one, tc.prof, 300, 0)) == 1 {
				first = s
			}
		}
		res := resultIf(first == learn, "")
		if first != learn {
			res = "differ (data): " + asText(row["learn_skill_source"]) + " learn skill " + fmt.Sprint(learn) + " is a guess; the path starts at yellow minus ten"
		}
		out = append(out, benchRow{fmt.Sprintf("%s (%s): first skill it is listed at", tc.recipe, tc.prof), fmt.Sprintf("skill %d", first), fmt.Sprintf("learn skill %d (%s), yellow %d", learn, asText(row["learn_skill_source"]), yellow), thrSrc, res})
	}
	finishOffline(t, "advice-threshold-estimates", "Offline skill-up thresholds: recipes whose learn skill is an estimate", out)
}

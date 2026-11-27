package professions

import (
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/store"
)

// The offline profession-cooldown benchmark: which crafts a skill-up plan leaves out because the game puts a crafting
// cooldown on them (a transmute, Mooncloth, the Salt Shaker) or on a material they need (Arcanite Bar, Mooncloth,
// Refined Deeprock Salt), beside the recipe rows of the dataset. It reads testdata/tradeskills_snapshot (recipes,
// recipe_reagents, items). The dataset holds no cooldown column, no cooldown length and no record of when a craft was
// last made, so the cases it cannot answer check that the advisor says so and states no figure. make offline-benchmark
// runs it; WOWC_BENCH_REPORT receives the tables.

const (
	cdRecipeSrc = "dataset recipes (spell_id, skill_line_name, name, creates_item_id), recipe_reagents; game: the craft has a cooldown (Wowhead Classic spell page)"
	cdMatSrc    = "dataset recipe_reagents, items; game: the material is made only by a craft with a cooldown"
	cdNoneSrc   = "the dataset has no such column or table"
	cdDir       = "../../testdata/tradeskills_snapshot"
)

var (
	cdOnce  sync.Once
	cdCache store.DataStore
)

func cdStore() store.DataStore {
	cdOnce.Do(func() { cdCache = store.NewDir(cdDir) })
	return cdCache
}

// cdRecipe reads the dataset's recipe row of a profession by name.
func cdRecipe(t *testing.T, line, name string) store.Row {
	t.Helper()
	rows, err := cdStore().Filter("recipes", store.Range{Column: "name", Min: name, Max: name})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if asText(r["skill_line_name"]) == line {
			return r
		}
	}
	t.Fatalf("recipe %s %q: not in the snapshot", line, name)
	return nil
}

func cdPlanner(prof string) *planner {
	return &planner{prof: prof, ds: cdStore(), known: map[int64]bool{}, names: map[int64]string{}}
}

// cdCases: each recipe is left out of a plan (true) or planned (false), and why the game says so.
var cdCases = []struct {
	line, name string
	left       bool
	why        string
}{
	{"Alchemy", "Transmute: Iron to Gold", true, "the craft has a cooldown"},
	{"Alchemy", "Transmute: Mithril to Truesilver", true, "the craft has a cooldown"},
	{"Alchemy", "Transmute: Arcanite", true, "the craft has a cooldown"},
	{"Tailoring", "Mooncloth", true, "the craft has a cooldown (four days)"},
	{"Engineering", "Salt Shaker", true, "the craft has a cooldown on its use"},
	{"Tailoring", "Mooncloth Boots", true, "it needs Mooncloth, a cooldown craft"},
	{"Blacksmithing", "Arcanite Rod", true, "it needs Arcanite Bars, made only by the Transmute: Arcanite cooldown"},
	{"Leatherworking", "Cured Rugged Hide", true, "it needs Refined Deeprock Salt, made only by the Salt Shaker's cooldown"},
	{"Leatherworking", "Cured Thick Hide", false, "it needs Deeprock Salt, sold and dropped, no cooldown"},
	{"Mining", "Smelt Truesilver", false, "an ordinary smelt, no cooldown"},
	{"Tailoring", "Runecloth Belt", false, "an ordinary craft, no cooldown"},
	{"Alchemy", "Mana Potion", false, "an ordinary craft, no cooldown"},
}

// TestOfflineBenchmarkCooldownRecipes: a skill-up plan leaves out a craft with a cooldown, a craft that needs a
// cooldown material, and plans every other one.
func TestOfflineBenchmarkCooldownRecipes(t *testing.T) {
	var out []benchRow
	word := map[bool]string{true: "left out", false: "planned"}
	for _, tc := range cdCases {
		row := cdRecipe(t, tc.line, tc.name)
		rc := recipeOf(row)
		p := cdPlanner(tc.line)
		got := p.blockedRecipe(rc) || p.usesCooldown(rc.SpellID)
		src := cdRecipeSrc
		if strings.Contains(tc.why, "needs") {
			src = cdMatSrc
		}
		out = append(out, benchRow{tc.line + " " + tc.name + ": left out of a plan", word[got], word[tc.left] + ": " + tc.why, src,
			resultIf(got == tc.left, "left out when it should be planned, or planned when it should be left out")})
	}
	finishOffline(t, "advice-cooldown-recipes", "Offline profession cooldowns: crafts a plan leaves out", out)
}

// TestOfflineBenchmarkCooldownNoAnswer: the dataset holds no cooldown column, table or capture field, so the advisor
// has no length of a cooldown or time left to state.
func TestOfflineBenchmarkCooldownNoAnswer(t *testing.T) {
	var out []benchRow
	row := cdRecipe(t, "Alchemy", "Transmute: Arcanite")
	var extra []string
	for col := range row {
		if strings.Contains(strings.ToLower(col), "cooldown") || strings.Contains(strings.ToLower(col), "cd_") {
			extra = append(extra, col)
		}
	}
	out = append(out, benchRow{"a transmute's cooldown length", fmt.Sprintf("cooldown columns on the recipe row: %v", extra), "recipes holds a spell, skill line, name, item and learn skill only", cdNoneSrc,
		resultIf(len(extra) == 0, "the dataset holds a cooldown the answer could state")})
	for _, table := range []string{"recipe_cooldowns", "cooldowns", "craft_cooldowns"} {
		_, err := cdStore().Filter(table, store.Range{Column: "spell_id", Min: int64(0), Max: int64(1)})
		out = append(out, benchRow{"a table of " + strings.ReplaceAll(table, "_", " "), fmt.Sprint(err != nil), "no such table in the dataset", cdNoneSrc, resultIf(err != nil, "a table was invented")})
	}
	for _, typ := range []reflect.Type{reflect.TypeOf(character.Character{}), reflect.TypeOf(character.Skill{})} {
		var held []string
		for i := 0; i < typ.NumField(); i++ {
			if strings.Contains(strings.ToLower(typ.Field(i).Name), "cooldown") {
				held = append(held, typ.Field(i).Name)
			}
		}
		out = append(out, benchRow{"time left on a craft: " + typ.Name() + " capture fields", fmt.Sprint(held), "the capture format holds no craft cooldown or last-crafted time", cdNoneSrc,
			resultIf(len(held) == 0, "a cooldown field the answer could state")})
	}
	finishOffline(t, "advice-cooldown-no-answer", "Offline profession cooldowns: no answer in the dataset", out)
}

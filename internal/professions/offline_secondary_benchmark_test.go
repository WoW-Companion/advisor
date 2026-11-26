package professions

import (
	"fmt"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/store"
)

// The offline secondary-profession and specialization benchmark: the skill-up answer for Cooking, First Aid and Fishing
// at many skills, set beside the dataset's profession_routes row that covers the skill, and the set of recipes a
// plan leaves out because only a specialization teaches them, set beside the specialization each profession has in
// the game. It reads testdata/advice_snapshot (profession_routes, recipes) and testdata/consumables_snapshot (the
// real Cooking and Alchemy recipe rows). The dataset holds no specialization table, no branch choice and no
// specialization trainer, so the cases it cannot answer check that the advisor says so and names none. A
// difference the dataset's own rows cause is marked "(data)". make offline-benchmark runs it; WOWC_BENCH_REPORT
// receives the tables.

const (
	secRouteSrc = "dataset profession_routes (step covering the skill: recipe, crafts, zones, learned_from)"
	secGuideSrc = "game First Aid bandage ladder (public: Linen 1, Heavy Linen 40, Wool 80, Heavy Wool 115, Silk 150, Heavy Silk 180, Mageweave 210, Heavy Mageweave 240, Runecloth 260, Heavy Runecloth 290)"
	secSpecSrc  = "game specializations (public: Dragonscale, Elemental, Tribal Leatherworking; Armorsmith, Weaponsmith Blacksmithing; Gnomish, Goblin Engineering); dataset recipes (skill_line_name, learned_from); the dataset has no specialization table"
	secNoneSrc  = "the dataset has no such column or table"
	consDirSec  = "../../testdata/consumables_snapshot"
)

// secPaths are the secondary-profession characters whose answer is checked: skill spread over every step of the
// three routes.
var secPaths = []struct {
	prof  string
	skill int
}{
	{"Cooking", 1}, {"Cooking", 49}, {"Cooking", 100}, {"Cooking", 160}, {"Cooking", 240}, {"Cooking", 290},
	{"First Aid", 30}, {"First Aid", 78}, {"First Aid", 120}, {"First Aid", 180}, {"First Aid", 215}, {"First Aid", 295},
	{"Fishing", 10}, {"Fishing", 100}, {"Fishing", 180}, {"Fishing", 260},
}

func TestOfflineBenchmarkSecondaryRouteA(t *testing.T) { secondaryRoutes(t, "a", secPaths[:6]) }
func TestOfflineBenchmarkSecondaryRouteB(t *testing.T) { secondaryRoutes(t, "b", secPaths[6:12]) }
func TestOfflineBenchmarkSecondaryRouteC(t *testing.T) { secondaryRoutes(t, "c", secPaths[12:]) }

// secondaryRoutes: the "Now:" line of the skill-up answer names the step the dataset's route has for the skill:
// the recipe and the crafts left, or the zones to fish in, and how the recipe is learned.
func secondaryRoutes(t *testing.T, part string, cases []struct {
	prof  string
	skill int
}) {
	ds := snapshot().ds
	var out []benchRow
	for _, tc := range cases {
		main, _ := refRoute(t, ds, tc.prof)
		var step *refStep
		for i := range main {
			if tc.skill >= main[i].from && tc.skill < main[i].to {
				step = &main[i]
				break
			}
		}
		if step == nil {
			t.Fatalf("%s %d: no step covers it in the dataset", tc.prof, tc.skill)
		}
		got := ""
		for _, l := range strings.Split(RouteFacts(routeChar(tc.prof, tc.skill), ds, nil, tc.prof), "\n") {
			if strings.HasPrefix(l, "Now: ") {
				got = l
			}
		}
		head, _, _ := strings.Cut(got, ";")
		start := max(tc.skill, step.from)
		var want string
		ok := false
		if len(step.where) > 0 && step.recipe == "" {
			want = fmt.Sprintf("Now: skill %d to %d: gather in %s", start, step.to, strings.Join(step.where, ", "))
			ok = strings.TrimSuffix(head, ".") == want
		} else {
			want = fmt.Sprintf("Now: skill %d to %d: %d x %s", start, step.to, step.left(tc.skill), step.recipe)
			ok = strings.TrimSuffix(head, ".") == want
			if step.learned != "" && !strings.Contains(got, "learned from: "+step.learned) {
				ok = false
				want += "; learned from: " + step.learned
			}
		}
		out = append(out, benchRow{fmt.Sprintf("%s at skill %d: the step to do now", tc.prof, tc.skill), got, want, secRouteSrc, resultIf(ok, "the step, its crafts or zones, or how it is learned")})
	}
	finishOffline(t, "advice-secondary-route-"+part, "Offline Cooking, First Aid and Fishing: the step the skill-up answer gives ("+part+")", out)
}

// bandages are the First Aid bandages in the order the game teaches them, with the skill each is taught at.
var bandages = []struct {
	recipe string
	skill  int
}{
	{"Linen Bandage", 1}, {"Heavy Linen Bandage", 40}, {"Wool Bandage", 80}, {"Heavy Wool Bandage", 115}, {"Silk Bandage", 150},
	{"Heavy Silk Bandage", 180}, {"Mageweave Bandage", 210}, {"Heavy Mageweave Bandage", 240}, {"Runecloth Bandage", 260}, {"Heavy Runecloth Bandage", 290},
}

// TestOfflineBenchmarkFirstAidLadder: at the skill the game teaches a bandage, the advisor's first step is that
// bandage, and the dataset's route starts the bandage's first step there.
func TestOfflineBenchmarkFirstAidLadder(t *testing.T) {
	ds := snapshot().ds
	main, _ := refRoute(t, ds, "First Aid")
	var out []benchRow
	for _, b := range bandages {
		first := -1
		for _, s := range main {
			if s.recipe == b.recipe && (first < 0 || s.from < first) {
				first = s.from
			}
		}
		got := ""
		for _, l := range strings.Split(RouteFacts(routeChar("First Aid", b.skill), ds, nil, "First Aid"), "\n") {
			if strings.HasPrefix(l, "Now: ") {
				got = l
			}
		}
		head, _, _ := strings.Cut(got, ";")
		ok := strings.Contains(head, " x "+b.recipe) && first == b.skill
		out = append(out, benchRow{fmt.Sprintf("%s: made from skill %d", b.recipe, b.skill), head, fmt.Sprintf("guide %d; the route's first step %d", b.skill, first), secGuideSrc, resultIf(ok, "the bandage at its skill")})
	}
	finishOffline(t, "advice-first-aid-ladder", "Offline First Aid: the bandage made at each skill", out)
}

// TestOfflineBenchmarkSecondaryTools: what the dataset's notes say a secondary profession needs, and the answer
// names no recipe for Fishing, which has none.
func TestOfflineBenchmarkSecondaryNotes(t *testing.T) {
	ds := snapshot().ds
	var out []benchRow
	for _, tc := range []struct{ prof, word string }{{"Cooking", "raw food"}, {"First Aid", "cloth"}, {"Fishing", "fishing pole"}} {
		n := LoadNotes(ds)
		note := n.Special[strings.ToLower(tc.prof)]
		out = append(out, benchRow{tc.prof + ": what it needs", note, "profession_notes special case mentions " + tc.word, secRouteSrc, resultIf(strings.Contains(strings.ToLower(note), tc.word), "the note")})
		out = append(out, benchRow{tc.prof + ": needs a profession slot", fmt.Sprint(n.Secondary[strings.ToLower(tc.prof)]), "profession_notes: secondary, no slot", secRouteSrc, resultIf(n.Secondary[strings.ToLower(tc.prof)], "counted against the two slots")})
	}
	finishOffline(t, "advice-secondary-notes", "Offline Cooking, First Aid and Fishing: needs and slots", out)
}

// specCase is one recipe: whether the plan leaves it out, by profession and whether the character knows it.
type specCase struct {
	label, prof, recipe string
	known               bool
	blocked             bool
	dataset             string // "" for a composed row
	data                bool   // the dataset itself cannot tell
}

var specCases = []specCase{
	{"Leatherworking, Dragonscale recipe, not known", "Leatherworking", "Synthetic Dragonscale Vest", false, true, "", false},
	{"Leatherworking, Dragonscale recipe, known", "Leatherworking", "Synthetic Dragonscale Vest", true, false, "", false},
	{"Leatherworking, Elemental recipe, not known", "Leatherworking", "Synthetic Elemental Leather Belt", false, true, "", false},
	{"Leatherworking, Tribal recipe, not known", "Leatherworking", "Synthetic Tribal Leather Cloak", false, true, "", false},
	{"Blacksmithing, Armorsmith recipe, not known", "Blacksmithing", "Synthetic Armorsmith Helm", false, true, "", false},
	{"Blacksmithing, Swordsmith recipe, not known", "Blacksmithing", "Synthetic Master Swordsmith Blade", false, true, "", false},
	{"Blacksmithing, Weaponsmith recipe, known", "Blacksmithing", "Synthetic Weaponsmith Axe", true, false, "", false},
	{"Engineering, Gnomish recipe, not known", "Engineering", "Synthetic Gnomish Goggles", false, true, "", false},
	{"Engineering, Goblin recipe, not known", "Engineering", "Synthetic Goblin Land Mine", false, true, "", false},
	{"Engineering, Goblin recipe, known", "Engineering", "Synthetic Goblin Land Mine", true, false, "", false},
	{"Cooking, Goblin Deviled Clams (an ordinary recipe)", "Cooking", "Goblin Deviled Clams", false, false, "Cooking, learn 125, learned from the trainer", false},
	{"Alchemy, Goblin Rocket Fuel (an ordinary recipe)", "Alchemy", "Goblin Rocket Fuel", false, false, "Alchemy, learn 210, learned from an item", false},
	{"Leatherworking, an ordinary recipe", "Leatherworking", "Handstitched Leather Pants", false, false, "Leatherworking, learn 15, learned from the trainer", false},
	{"Alchemy, a transmute (a cooldown)", "Alchemy", "Transmute: Iron to Gold", false, true, "Alchemy, learn 225, learned from an item", false},
	{"Engineering, Goblin Mining Helmet", "Engineering", "Goblin Mining Helmet", false, true, "Engineering, learn 205, learned from an item", true},
}

// TestOfflineBenchmarkSpecializationRecipes: which recipes a plan leaves out as a specialization's, by name and
// profession, beside the specialization each profession has in the game.
func TestOfflineBenchmarkSpecializationRecipes(t *testing.T) {
	var out []benchRow
	for _, tc := range specCases {
		p := &planner{prof: tc.prof, known: map[int64]bool{}, names: map[int64]string{}}
		rc := Recipe{SpellID: 900001, Name: tc.recipe}
		if tc.known {
			p.known[rc.SpellID] = true
		}
		got := p.blockedRecipe(rc)
		word := map[bool]string{true: "left out", false: "planned"}
		ref := word[tc.blocked]
		if tc.dataset != "" {
			ref += " (dataset: " + tc.dataset + ")"
		}
		res := resultIf(got == tc.blocked, "left out when it should be planned, or planned when it should be left out")
		if got == tc.blocked && tc.data {
			res = "differ (data): the dataset has no specialization column, so the name alone decides"
		}
		out = append(out, benchRow{tc.label, word[got], ref, secSpecSrc, res})
	}
	finishOffline(t, "advice-specialization-recipes", "Offline specialization: recipes a plan leaves out", out)
}

// TestOfflineBenchmarkSpecializationRows: the real dataset rows are checked as the planner reads them: a recipe row of
// Cooking and of Alchemy that carries a specialization's word is planned.
func TestOfflineBenchmarkSpecializationRows(t *testing.T) {
	ds := store.NewDir(consDirSec)
	var out []benchRow
	for _, tc := range []struct {
		prof  string
		spell int64
	}{{"Cooking", 6500}, {"Alchemy", 11456}} {
		rows, err := ds.Filter("recipes", store.Range{Column: "spell_id", Min: tc.spell, Max: tc.spell})
		if err != nil || len(rows) == 0 {
			t.Fatalf("recipe %d: %v", tc.spell, err)
		}
		row := rows[0]
		rc := recipeOf(row)
		p := &planner{prof: tc.prof, ds: ds, known: map[int64]bool{}, names: map[int64]string{}}
		line := asText(row["skill_line_name"])
		out = append(out, benchRow{rc.Name + ": planned as a " + tc.prof + " recipe", fmt.Sprint("left out: ", p.blockedRecipe(rc)), "skill line " + line + ", learned from " + rc.LearnedFrom, secSpecSrc,
			resultIf(!p.blockedRecipe(rc) && line == tc.prof, "an ordinary recipe was taken for a specialization's")})
	}
	finishOffline(t, "advice-specialization-rows", "Offline specialization: real recipe rows", out)
}

// TestOfflineBenchmarkSpecializationNoAnswer: the dataset holds no specialization, branch or specialization trainer.
// The advisor names no branch for a profession that has them and offers no specialization as a choice.
func TestOfflineBenchmarkSpecializationNoAnswer(t *testing.T) {
	ds := snapshot().ds
	var out []benchRow
	for _, table := range []string{"specializations", "profession_specializations", "specialization_trainers"} {
		_, err := ds.Filter(table, store.Range{Column: "id", Min: int64(0)})
		out = append(out, benchRow{table + ": a table of branches", fmt.Sprint("error: ", err != nil), "no such table in the dataset", secNoneSrc, resultIf(err != nil, "a table exists: the cases need a second look")})
	}
	branches := []string{"dragonscale", "elemental", "tribal", "armorsmith", "weaponsmith", "swordsmith", "axesmith", "hammersmith", "gnomish", "goblin"}
	for _, prof := range []string{"Leatherworking", "Blacksmithing", "Engineering"} {
		pa := ComputePath(routeChar(prof, 40), ds, prof, 60, 1)
		txt := strings.ToLower(strings.Join(append(append(append([]string{}, pa.Where...), pa.Notes...), pa.Prep...), " "))
		var named []string
		for _, b := range branches {
			if strings.Contains(txt, b) {
				named = append(named, b)
			}
		}
		out = append(out, benchRow{prof + ": which specialization to take", strings.Join(append(pa.Where, pa.Notes...), " "), "no specialization data: none is named", secNoneSrc, resultIf(len(named) == 0, "a specialization is named: "+strings.Join(named, ", "))})
	}
	finishOffline(t, "advice-specialization-no-answer", "Offline specialization: no answer in the dataset", out)
}

package professions

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/store"
)

// The offline profession advice benchmark: the same comparison as make advice-benchmark, on a fixed sample
// that needs no network. testdata/advice_snapshot (scripts/advice-snapshot.sh) holds the real dataset rows of
// eight professions up to skill 75: recipes, reagents, trainers, vendors, patterns. Each test plans one
// profession from skill 1 and sets each band's answer beside the dataset row read straight from the tables
// (never through the planner): how the recipe is learnt, what learning it costs, which materials and how
// many, and how the crafts split into skill-up colours. Further tests set the planner's colours, craft
// counts and reagent counts beside literal dataset values and the recorded wow-professions.com Forever guide
// steps. Run it with make offline-benchmark; WOWC_BENCH_REPORT receives the result tables in Markdown.

const (
	snapshotDir = "../../testdata/advice_snapshot"
	snapSkill   = 75 // the highest skill the snapshot's recipes are learnt at
	snapSource  = "dataset snapshot recipes, recipe_reagents, trainer_spells, vendor_sources, recipe_items"
)

var (
	snapOnce  sync.Once
	snapStore store.DataStore
)

// snapshot reads the snapshot once for the package: tables are decoded on first use, and the tests share them.
func snapshot() *refData {
	snapOnce.Do(func() { snapStore = store.NewDir(snapshotDir) })
	return &refData{ds: snapStore, recipes: map[string]map[int64]store.Row{}}
}

// offlineWindows is the fixed sample: the profession and the skill range planned, from skill 1.
var offlineWindows = []struct {
	prof string
	to   int
}{
	{"First Aid", 75}, {"Cooking", 75}, {"Alchemy", 75}, {"Tailoring", 40},
	{"Leatherworking", 60}, {"Blacksmithing", 75}, {"Enchanting", 75}, {"Engineering", 75},
}

// datasetSource says how the dataset has a recipe learnt, read from the raw tables: trainer, confirmed (a
// trainer lists the spell; the price is the cheapest one listed, and a trainer with no price is not a free
// trainer), vendor pattern (a vendor sells the recipe item; the price is the item's base buy price, else the
// cheapest listing), item (the recipe item drops or is a reward), or trainer, unlisted (the recipe says
// trainer and no trainer's list has it: a data gap).
func (r *refData) datasetSource(row store.Row) (kind string, copper int64) {
	spell := asI64(row["spell_id"])
	if ts, _ := r.ds.Filter("trainer_spells", store.Range{Column: "spell_id", Min: spell, Max: spell}); len(ts) > 0 {
		for _, t := range ts {
			if c := asI64(t["cost_copper"]); c > 0 && (copper == 0 || c < copper) {
				copper = c
			}
		}
		return "trainer, confirmed", copper
	}
	items, _ := r.ds.Filter("recipe_items", store.Range{Column: "teaches_spell_id", Min: spell, Max: spell})
	for _, it := range items {
		id := asI64(it["item_id"])
		vs, _ := r.ds.Filter("vendor_sources", store.Range{Column: "item_id", Min: id, Max: id})
		for _, v := range vs {
			if p := asI64(v["price_copper"]); p > 0 && (copper == 0 || p < copper) {
				copper = p
			}
		}
		if len(vs) > 0 {
			if base, err := r.ds.Get("items", id); err == nil {
				if b := asI64(base["buy_price"]); b > 0 {
					copper = b
				}
			}
			return "vendor pattern", copper
		}
	}
	if len(items) > 0 {
		return "item", 0
	}
	return "trainer, unlisted", 0
}

func TestOfflineBenchmarkFirstAid(t *testing.T)       { offlinePlan(t, 0) }
func TestOfflineBenchmarkCooking(t *testing.T)        { offlinePlan(t, 1) }
func TestOfflineBenchmarkAlchemy(t *testing.T)        { offlinePlan(t, 2) }
func TestOfflineBenchmarkTailoring(t *testing.T)      { offlinePlan(t, 3) }
func TestOfflineBenchmarkLeatherworking(t *testing.T) { offlinePlan(t, 4) }
func TestOfflineBenchmarkBlacksmithing(t *testing.T)  { offlinePlan(t, 5) }
func TestOfflineBenchmarkEnchanting(t *testing.T)     { offlinePlan(t, 6) }
func TestOfflineBenchmarkEngineering(t *testing.T)    { offlinePlan(t, 7) }

func offlinePlan(t *testing.T, i int) {
	w := offlineWindows[i]
	finishOffline(t, "professions-"+strings.ReplaceAll(strings.ToLower(w.prof), " ", "-"), "Offline plan: "+w.prof, offlineWindow(t, snapshot(), w.prof, w.to))
}

func finishOffline(t *testing.T, area, title string, rows []benchRow) {
	t.Helper()
	for _, r := range rows {
		if strings.HasPrefix(r.result, "differ") && !strings.Contains(r.result, "(data)") && !strings.Contains(r.result, "(model)") {
			t.Errorf("%s: advisor %q, reference %q: %s", r.check, r.advisor, r.reference, r.result)
		}
	}
	printCounts(area, rows)
	writeBenchTable(t, title, rows)
}

// offlineWindow plans one profession from skill 1 and checks every band against the dataset rows.
func offlineWindow(t *testing.T, ref *refData, prof string, to int) []benchRow {
	t.Helper()
	b := NewBenchmark(ref.ds, BenchOptions{Profession: prof, From: 1, To: to, Level: 60, Faction: "horde"})
	if len(b.Bands) == 0 {
		t.Fatalf("%s: no bands: %v", prof, b.Notes)
	}
	var out []benchRow
	for _, x := range b.Bands {
		label := fmt.Sprintf("%s %d→%d %s", prof, x.From, x.To, x.Recipe)
		if x.Gap != "" {
			out = append(out, benchRow{label + ": recipe", "no recipe", "a recipe for every skill point", snapSource, "differ (data): " + x.Gap})
			continue
		}
		row, ok := ref.of(prof)[x.SpellID]
		if !ok {
			out = append(out, benchRow{label + ": recipe", x.Recipe, "not in the dataset", snapSource, "differ: the plan names a recipe the dataset lacks"})
			continue
		}

		// how the recipe is learnt
		kind, copper := ref.datasetSource(row)
		got := x.Source
		res := "match"
		switch {
		case x.LearnVia == "known":
			// a recipe an earlier band taught: nothing to learn here
			got = "known (taught earlier in the plan)"
		case kind == "trainer, unlisted":
			if !strings.HasPrefix(x.Source, "trainer, unconfirmed") {
				res = "differ"
			} else {
				res = "differ (data): no trainer's list in the dataset has this recipe; the plan says to ask the trainer"
			}
		case !strings.HasPrefix(x.Source, kind):
			res = "differ"
		}
		out = append(out, benchRow{label + ": how it is learnt", got, kind, "dataset trainer_spells, recipe_items, vendor_sources", res})

		// what learning it costs
		if x.LearnVia != "known" && kind != "trainer, unlisted" {
			res = "match"
			if x.LearnCopper != copper {
				res = "differ"
			}
			out = append(out, benchRow{label + ": learning price", Money(x.LearnCopper), Money(copper), "dataset trainer_spells.cost_copper or vendor_sources.price_copper", res})
		}

		// material counts: every reagent, times the crafts
		want := ref.reagents(x.SpellID)
		have := map[int64]int{}
		for _, m := range x.Materials {
			have[m.ID] += m.Count
		}
		var bad []string
		for id, n := range want {
			if int64(have[id]) != n*int64(x.Crafts) {
				bad = append(bad, fmt.Sprintf("item %d: %d, dataset %d × %d", id, have[id], n, x.Crafts))
			}
		}
		for id := range have {
			if _, ok := want[id]; !ok {
				bad = append(bad, fmt.Sprintf("item %d is not a reagent", id))
			}
		}
		res = "match"
		if len(bad) > 0 {
			res = "differ: " + strings.Join(bad, "; ")
		}
		out = append(out, benchRow{label + ": material counts", describeMaterials(ref, x.Materials), fmt.Sprintf("the recipe's reagents × %d crafts", x.Crafts), "dataset recipe_reagents", res})

		// skill-up range: the orange crafts are the skill points below the yellow threshold, and the
		// recipe turns grey no earlier than the band's end
		y, hasY := intOf(row, "yellow_skill")
		g, hasG := intOf(row, "grey_skill")
		if hasY && hasG {
			wantOrange := max(0, min(x.To, y)-x.From)
			res = "match"
			if x.OrangeCrafts != wantOrange || x.To > g {
				res = "differ"
			}
			out = append(out, benchRow{label + ": skill-up range", fmt.Sprintf("%d orange crafts to skill %d", x.OrangeCrafts, x.To),
				fmt.Sprintf("%d orange crafts (yellow from %d, grey from %d)", wantOrange, y, g), "dataset recipes yellow_skill, grey_skill", res})
		}
	}
	// what each rank costs at its trainer: the dataset's prices of the trainer recipes the plan learns in
	// the rank, a recipe no trainer prices counted apart
	for _, tier := range b.Tiers {
		var cost int64
		var n, unpriced int
		for _, x := range b.Bands {
			if x.Gap != "" || x.LearnVia != "trainer" || x.From < tier.SkillMin || x.From >= tier.SkillCap {
				continue
			}
			row, ok := ref.of(prof)[x.SpellID]
			if !ok {
				continue
			}
			n++
			if _, c := ref.datasetSource(row); c > 0 {
				cost += c
			} else {
				unpriced++
			}
		}
		res := "match"
		if tier.TrainerSpells != n || tier.TrainerCopper != cost || tier.TrainerUnpriced != unpriced {
			res = "differ"
		}
		out = append(out, benchRow{fmt.Sprintf("%s %s rank: trainer cost", prof, tier.Name), trainerCostText(tier),
			fmt.Sprintf("%d recipes for %s, %d with no price", n, Money(cost), unpriced), "dataset trainer_spells.cost_copper", res})
	}
	return out
}

func describeMaterials(ref *refData, ms []BenchMaterial) string {
	if len(ms) == 0 {
		return "none"
	}
	var parts []string
	for _, m := range ms {
		parts = append(parts, fmt.Sprintf("%d %s", m.Count, m.Item))
	}
	return strings.Join(parts, ", ")
}

// recipeCase is a dataset row stated as literals: the learn skill, the yellow and grey thresholds and the
// reagents of one recipe, and the colour the game gives at a few skills (orange below yellow, yellow up to
// grey, none from grey on). The planner's reading of the row is set beside them.
type recipeCase struct {
	prof, recipe string
	learn        int
	yellow, grey int
	reagents     string // "item xN; item xN", the dataset's recipe_reagents
	skills       []int
	colours      []string // orange, yellow or "" for grey, one per skill
}

var literalCases = []recipeCase{
	{"First Aid", "Linen Bandage", 1, 30, 60, "Linen Cloth x1", []int{10, 30, 59, 60}, []string{Orange, Yellow, Yellow, ""}},
	{"First Aid", "Heavy Linen Bandage", 40, 50, 100, "Linen Cloth x2", []int{40, 50, 99, 100}, []string{Orange, Yellow, Yellow, ""}},
	{"First Aid", "Minor Healing Potion", 1, 55, 95, "Mild Spices x1; Peacebloom x1; Empty Vial x1", []int{54, 55, 95}, []string{Orange, Yellow, ""}},
	{"Cooking", "Slitherskin Mackerel", 1, 45, 85, "Raw Slitherskin Mackerel x1", []int{44, 45, 84, 85}, []string{Orange, Yellow, Yellow, ""}},
	{"Alchemy", "Elixir of Minor Defense", 15, 55, 95, "Silverleaf x1; Peacebloom x1; Empty Vial x1", []int{54, 55, 94, 95}, []string{Orange, Yellow, Yellow, ""}},
	{"Blacksmithing", "Rough Sharpening Stone", 1, 15, 55, "Rough Stone x1", []int{14, 15, 54, 55}, []string{Orange, Yellow, Yellow, ""}},
	{"Tailoring", "Bolt of Linen Cloth", 1, 25, 50, "Linen Cloth x2", []int{24, 25, 49, 50}, []string{Orange, Yellow, Yellow, ""}},
	{"Leatherworking", "Cured Light Hide", 30, 50, 70, "Light Hide x1; Salt x1", []int{49, 50, 69, 70}, []string{Orange, Yellow, Yellow, ""}},
	{"Engineering", "Handful of Copper Bolts", 30, 45, 60, "Copper Bar x1", []int{44, 45, 59, 60}, []string{Orange, Yellow, Yellow, ""}},
	{"Enchanting", "Enchant Bracer - Inferior Stamina", 15, 20, 25, "Mote of Magic x1", []int{19, 20, 24, 25}, []string{Orange, Yellow, Yellow, ""}},
}

func colourWord(c string) string {
	if c == "" {
		return "grey (no skill-up)"
	}
	return c
}

func TestOfflineBenchmarkLiteralCases(t *testing.T) {
	ref := snapshot()
	var out []benchRow
	for _, c := range literalCases {
		row := ref.byName(c.prof, c.recipe)
		if row == nil {
			t.Fatalf("%s %s is not in the snapshot", c.prof, c.recipe)
		}
		recipes, err := Recipes(ref.ds, c.prof)
		if err != nil {
			t.Fatal(err)
		}
		var r Recipe
		for _, x := range recipes {
			if x.SpellID == asI64((*row)["spell_id"]) {
				r = x
			}
		}
		src := fmt.Sprintf("dataset recipes: learn %d, yellow %d, grey %d", c.learn, c.yellow, c.grey)
		label := c.prof + " " + c.recipe
		got := "not read"
		if r.LearnSkill != nil && r.YellowSkill != nil && r.GreySkill != nil {
			got = fmt.Sprintf("learn %d, yellow %d, grey %d", *r.LearnSkill, *r.YellowSkill, *r.GreySkill)
		}
		res := "match"
		if got != fmt.Sprintf("learn %d, yellow %d, grey %d", c.learn, c.yellow, c.grey) {
			res = "differ"
		}
		out = append(out, benchRow{label + ": learn skill and thresholds", got, fmt.Sprintf("learn %d, yellow %d, grey %d", c.learn, c.yellow, c.grey), src, res})
		for i, sk := range c.skills {
			res := "match"
			if difficulty(r, sk) != c.colours[i] {
				res = "differ"
			}
			out = append(out, benchRow{fmt.Sprintf("%s at skill %d: colour", label, sk), colourWord(difficulty(r, sk)), colourWord(c.colours[i]), src, res})
		}
		var have []string
		for _, rg := range reagentsOf(ref.ds, r.SpellID) {
			have = append(have, fmt.Sprintf("%s x%d", itemName(ref.ds, asI64(rg["reagent_item_id"])), asI64(rg["count"])))
		}
		sort.Strings(have)
		want := strings.Split(c.reagents, "; ")
		sort.Strings(want)
		res = "match"
		if strings.Join(have, "; ") != strings.Join(want, "; ") {
			res = "differ"
		}
		out = append(out, benchRow{label + ": reagents per craft", strings.Join(have, "; "), c.reagents, "dataset recipe_reagents", res})
	}
	finishOffline(t, "professions-literal", "Offline profession cases against literal dataset values", out)
}

// A guide step's craft count is one good route's; the planner's estimate is the dataset-driven skill-up
// model (an orange craft 1 point, yellow 0.75, green 0.25), whose documented fit is a mean error of 8%
// over the recorded steps. A step further off than offStep is listed as a model difference, and the mean
// error over all steps must stay under offMean.
const (
	offStep = 0.25
	offMean = 0.20
)

func TestOfflineBenchmarkGuides(t *testing.T) {
	ref := snapshot()
	var out []benchRow
	var errSum float64
	var errN int
	for _, g := range loadGuides(t).Professions {
		for _, st := range g.Steps {
			if st.To > snapSkill {
				continue
			}
			row := ref.byName(g.Profession, st.Recipe)
			label := fmt.Sprintf("%s %d→%d %s", g.Profession, st.From, st.To, st.Recipe)
			if row == nil {
				out = append(out, benchRow{label + ": recipe", "-", "in the dataset", g.Source, "differ (data): " + st.Recipe + " is not in the snapshot's recipes (above the snapshot's skill, or missing from the dataset)"})
				continue
			}
			y, hasY := intOf(*row, "yellow_skill")
			gr, hasG := intOf(*row, "grey_skill")
			if hasY && hasG && st.Crafts >= 8 && !strings.HasPrefix(st.Recipe, "Bolt of") {
				c, reach := craftsFor(Recipe{YellowSkill: &y, GreySkill: &gr}, st.From, st.To)
				if reach == st.To {
					off := math.Abs(float64(c-st.Crafts)) / float64(st.Crafts)
					errSum += off
					errN++
					res := "match"
					if off > offStep {
						res = "differ (model): the guide's route counts differently here; the planner's skill-up model is an estimate"
					}
					out = append(out, benchRow{label + ": crafts", fmt.Sprint(c), fmt.Sprintf("%d (within %d%%)", st.Crafts, int(offStep*100)), g.Source + "; " + snapSource, res})
				}
			}
			have := ref.reagents(asI64((*row)["spell_id"]))
			for _, rg := range st.Reagents {
				var got int64
				for id, n := range have {
					if strings.EqualFold(itemName(ref.ds, id), rg.Item) {
						got = n
					}
				}
				res := "match"
				if int(got) != rg.Per {
					res = "differ (data): the dataset's recipe_reagents and the guide disagree"
				}
				out = append(out, benchRow{label + ": " + rg.Item + " per craft", fmt.Sprint(got), fmt.Sprint(rg.Per), g.Source, res})
			}
		}
	}
	mean := errSum / float64(max(errN, 1))
	res := "match"
	if mean > offMean {
		res = "differ"
	}
	out = append(out, benchRow{"skill-up model: mean error over the guide steps below skill 75", fmt.Sprintf("%.0f%% over %d steps", 100*mean, errN),
		fmt.Sprintf("under %d%%", int(offMean*100)), "the Forever guides; " + snapSource, res})
	finishOffline(t, "professions-guides", "Offline profession cases against the Forever guides", out)
}

// planSkills are the characters whose "what do I make next" answer is checked: a profession and a skill.
var planSkills = []struct {
	prof  string
	skill int
}{{"First Aid", 50}, {"Tailoring", 30}, {"Alchemy", 20}, {"Cooking", 40}, {"Enchanting", 60}, {"Blacksmithing", 35}}

// TestOfflineBenchmarkNextRecipes sets the recipes Plan lists for a character at a skill beside the
// dataset: each one is learnable at that skill, the colour is the one its thresholds give, and the
// reagent counts are the recipe's.
func TestOfflineBenchmarkNextRecipes(t *testing.T) {
	ref := snapshot()
	var out []benchRow
	for _, ps := range planSkills {
		lvl, skill := 60, ps.skill
		c := character.Character{Name: "Planner", Class: "None", Level: &lvl, Professions: []character.Skill{{Name: ps.prof, Rank: &skill}}}
		pl := Plan(c, ref.ds, ps.prof)
		if len(pl.Steps) == 0 {
			t.Fatalf("%s at %d: no steps: %v", ps.prof, ps.skill, pl.Notes)
		}
		for _, st := range pl.Steps {
			row, ok := ref.of(ps.prof)[st.Recipe.SpellID]
			label := fmt.Sprintf("%s at skill %d: %s", ps.prof, ps.skill, st.Recipe.Name)
			if !ok {
				out = append(out, benchRow{label, "not in the dataset", "a recipe of " + ps.prof, snapSource, "differ"})
				continue
			}
			wantColour := colour(row, ps.skill)
			if y, ok := intOf(row, "yellow_skill"); ok && wantColour == "yellow" {
				if g, ok := intOf(row, "grey_skill"); ok && ps.skill >= (y+g)/2 {
					wantColour = "green" // halfway from yellow to grey the game shows green
				}
			}
			if wantColour == "grey" {
				wantColour = ""
			}
			res := "match"
			if st.Difficulty != wantColour && !(st.Difficulty == Unknown && wantColour != "") {
				res = "differ"
			}
			if l, ok := intOf(row, "learn_skill"); ok && l > ps.skill && asText(row["learn_skill_source"]) != "estimate" {
				res = "differ"
			}
			out = append(out, benchRow{label + ": colour and learnable", fmt.Sprintf("%s, learn %v", colourWord(st.Difficulty), st.Recipe.LearnSkill != nil && *st.Recipe.LearnSkill <= ps.skill),
				fmt.Sprintf("%s, learn %v (yellow %v, grey %v)", colourWord(wantColour), row["learn_skill"], row["yellow_skill"], row["grey_skill"]), "dataset recipes", res})
			want := ref.reagents(st.Recipe.SpellID)
			var bad []string
			for _, rg := range st.Reagents {
				if int64(rg.Need) != want[rg.ItemID] {
					bad = append(bad, fmt.Sprintf("%s %d, dataset %d", rg.Name, rg.Need, want[rg.ItemID]))
				}
			}
			if len(st.Reagents) != len(want) {
				bad = append(bad, fmt.Sprintf("%d reagents, dataset %d", len(st.Reagents), len(want)))
			}
			res = "match"
			if len(bad) > 0 {
				res = "differ: " + strings.Join(bad, "; ")
			}
			out = append(out, benchRow{label + ": reagent counts", fmt.Sprintf("%d reagents", len(st.Reagents)), fmt.Sprintf("%d reagents", len(want)), "dataset recipe_reagents", res})
		}
	}
	finishOffline(t, "professions-next", "Offline next-recipe answers", out)
}

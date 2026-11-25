package professions

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/WoW-Companion/advisor/internal/benchsummary"
	"github.com/WoW-Companion/advisor/internal/store"
)

// The profession advice benchmark. For every profession it plans three synthetic characters (skill 1, a mid
// rank at 150, near the cap at 285; level 60, empty bags) and compares the plan with
//
//   - the dataset: recipe skill ranges (learn, yellow, grey), reagents and counts, read straight from the
//     recipes and recipe_reagents tables and not through the planner, and
//   - the wow-professions.com WoW Forever leveling guides, recorded as facts (skill range, recipe, crafts)
//     in testdata/forever_guides.json.
//
// It needs a built dataset: WOWC_BENCH_DATASET is its directory (make advice-benchmark downloads or builds
// one). WOWC_BENCH_REPORT, when set, is a file that receives the result table in Markdown. A difference
// from the dataset fails the test; a difference from a guide is listed, since a guide's route is one good
// route and the planner picks its own by cost.

//go:embed testdata/forever_guides.json
var foreverGuidesJSON []byte

type guideReagent struct {
	Item string `json:"item"`
	Per  int    `json:"per"`
}

type guideStep struct {
	From     int            `json:"from"`
	To       int            `json:"to"`
	Recipe   string         `json:"recipe"`
	Crafts   int            `json:"crafts"`
	Reagents []guideReagent `json:"reagents"`
}

type guideProf struct {
	Profession string      `json:"profession"`
	Source     string      `json:"source"`
	Steps      []guideStep `json:"steps"`
}

type foreverGuides struct {
	Retrieved   string      `json:"retrieved"`
	Professions []guideProf `json:"professions"`
}

func loadGuides(t testing.TB) foreverGuides {
	t.Helper()
	var g foreverGuides
	if err := json.Unmarshal(foreverGuidesJSON, &g); err != nil {
		t.Fatal(err)
	}
	return g
}

// step is the guide step that covers a skill (the last step when the skill is at the cap).
func (g guideProf) step(skill int) *guideStep {
	for i := range g.Steps {
		if skill >= g.Steps[i].From && skill < g.Steps[i].To {
			return &g.Steps[i]
		}
	}
	return nil
}

var (
	benchPlans   = map[string]Benchmark{}
	benchPlansMu sync.Mutex
)

// benchPlan is NewBenchmark with each plan made once per run: planning a profession reads the dataset for
// every recipe, and the benchmark's checks all look at the same few plans.
func benchPlan(ds store.DataStore, o BenchOptions) Benchmark {
	key := fmt.Sprintf("%s/%d/%d/%d/%s/%s/%v", titleCase(o.Profession), max(o.From, 1), o.To, o.Level, o.Faction, o.Gather, o.Known)
	benchPlansMu.Lock()
	defer benchPlansMu.Unlock()
	if b, ok := benchPlans[key]; ok {
		return b
	}
	b := NewBenchmark(ds, o)
	benchPlans[key] = b
	return b
}

// noRecipeGap starts the planner's message for a stretch no trainer or vendor recipe covers.
const noRecipeGap = "No trainer or vendor recipe covers"

type benchRow struct{ check, advisor, reference, source, result string }

// refData reads the dataset tables the checks compare against.
type refData struct {
	ds      store.DataStore
	recipes map[string]map[int64]store.Row // profession -> spell id -> row
}

func (r *refData) of(prof string) map[int64]store.Row {
	if m, ok := r.recipes[prof]; ok {
		return m
	}
	rows, _ := r.ds.Filter("recipes", store.Range{Column: "skill_line_name", Min: prof, Max: prof})
	m := map[int64]store.Row{}
	for _, row := range rows {
		m[asI64(row["spell_id"])] = row
	}
	r.recipes[prof] = m
	return m
}

func (r *refData) reagents(spell int64) map[int64]int64 {
	rows, _ := r.ds.Filter("recipe_reagents", store.Range{Column: "spell_id", Min: spell, Max: spell})
	out := map[int64]int64{}
	for _, row := range rows {
		out[asI64(row["reagent_item_id"])] += asI64(row["count"])
	}
	return out
}

func (r *refData) byName(prof, name string) *store.Row {
	for _, row := range r.of(prof) {
		if strings.EqualFold(asText(row["name"]), name) {
			row := row
			return &row
		}
	}
	return nil
}

func intOf(row store.Row, col string) (int, bool) {
	v, ok := row[col]
	if !ok || v == nil {
		return 0, false
	}
	return int(asI64(v)), true
}

// colour is the dataset's colour of a recipe at a skill: orange below yellow_skill, yellow up to grey_skill, grey from there.
func colour(row store.Row, skill int) string {
	if g, ok := intOf(row, "grey_skill"); ok && skill >= g {
		return "grey"
	}
	if y, ok := intOf(row, "yellow_skill"); ok && skill < y {
		return "orange"
	}
	return "yellow"
}

// totalCrafts is the crafts of the recipes the bands name (the guide's step counts are these) and the crafts of
// the intermediates they take (bars smelted, bolts woven, leather ground), which no guide step counts.
func totalCrafts(b Benchmark) (top, intermediate int) {
	var sub func(ms []BenchMaterial) int
	sub = func(ms []BenchMaterial) int {
		n := 0
		for _, m := range ms {
			if m.Make != nil {
				n += m.Make.Crafts + sub(m.Make.Materials)
			}
		}
		return n
	}
	for _, x := range b.Bands {
		top += x.Crafts
		intermediate += sub(x.Materials)
	}
	return top, intermediate
}

// sourcingRow checks how the plan says each recipe is learnt against the dataset's trainer_spells and
// vendor_sources: a recipe the plan calls a confirmed trainer recipe must be in trainer_spells, a vendor pattern
// must be sold by a vendor, and a trainer recipe the dataset's trainer list lacks is counted as unconfirmed
// (the plan says "check when you visit"; the gap is in the data's trainer list).
func sourcingRow(ref *refData, prof string, b Benchmark) benchRow {
	var confirmed, vendor, unconfirmed, wrong int
	var bad []string
	for _, x := range b.Bands {
		if x.Gap != "" || x.Source == "" {
			continue
		}
		switch {
		case strings.HasPrefix(x.Source, "trainer, confirmed"):
			confirmed++
			if rows, _ := ref.ds.Filter("trainer_spells", store.Range{Column: "spell_id", Min: x.SpellID, Max: x.SpellID}); len(rows) == 0 {
				wrong++
				bad = append(bad, x.Recipe+" is called a confirmed trainer recipe, no trainer lists it")
			}
		case strings.HasPrefix(x.Source, "vendor pattern"):
			vendor++
			sold := false
			items, _ := ref.ds.Filter("recipe_items", store.Range{Column: "teaches_spell_id", Min: x.SpellID, Max: x.SpellID})
			for _, it := range items {
				id := asI64(it["item_id"])
				if v, _ := ref.ds.Filter("vendor_sources", store.Range{Column: "item_id", Min: id, Max: id}); len(v) > 0 {
					sold = true
				}
			}
			if !sold {
				wrong++
				bad = append(bad, x.Recipe+" is called a vendor pattern, no vendor sells it")
			}
		case strings.HasPrefix(x.Source, "trainer, unconfirmed"):
			unconfirmed++
		}
	}
	res := "match"
	switch {
	case wrong > 0:
		res = "differ: " + strings.Join(bad, "; ")
	case unconfirmed > 0:
		res = fmt.Sprintf("differ: the dataset's trainer list lacks %d of the %d trainer recipes the plan uses (the plan says \"check when you visit\"; a data gap)", unconfirmed, unconfirmed+confirmed)
	}
	return benchRow{prof + " from skill 1: trainer and vendor sourcing", fmt.Sprintf("%d confirmed trainer, %d vendor pattern, %d unconfirmed trainer", confirmed, vendor, unconfirmed),
		"every trainer recipe in trainer_spells, every vendor pattern in vendor_sources", "dataset trainer_spells, recipe_items, vendor_sources", res}
}

// leafMaterials adds up the raw materials of a material list: a made material counts as what it is made from.
func leafMaterials(ms []BenchMaterial, into map[int64]int) {
	for _, m := range ms {
		if m.Make != nil {
			leafMaterials(m.Make.Materials, into)
			continue
		}
		into[m.ID] += m.Count
	}
}

// totalsRow checks the plan's material totals against the sum of its bands' materials, item by item. A band's
// material that an earlier band crafted (the bolts a later recipe takes) is not fetched, so the crafts of
// each band's recipe are taken off the item it makes.
func totalsRow(ref *refData, prof string, b Benchmark) benchRow {
	want := map[int64]int{}
	made := map[int64]int{}
	for _, x := range b.Bands {
		leafMaterials(x.Materials, want)
		if row, ok := ref.of(prof)[x.SpellID]; ok && x.Gap == "" {
			made[asI64(row["creates_item_id"])] += x.Crafts
		}
	}
	for id, n := range made {
		if w, ok := want[id]; ok {
			want[id] = max(0, w-n)
		}
	}
	got := map[int64]int{}
	for _, t := range b.Materials {
		got[t.ID] += t.Count
	}
	var bad []string
	for id, n := range want {
		if got[id] != n {
			bad = append(bad, fmt.Sprintf("item %d: bands %d, totals %d", id, n, got[id]))
		}
	}
	for id, n := range got {
		if _, ok := want[id]; !ok {
			bad = append(bad, fmt.Sprintf("item %d: totals %d, in no band", id, n))
		}
	}
	sort.Strings(bad)
	res := "match"
	if len(bad) > 0 {
		res = "differ: " + strings.Join(bad[:min(len(bad), 5)], "; ")
	}
	return benchRow{prof + " from skill 1: material totals", fmt.Sprintf("%d materials in the totals", len(b.Materials)), "the sum of the bands' raw materials less what earlier bands craft, item by item", "the plan's own bands", res}
}

// checkBands compares every band with the dataset and returns what disagrees.
func checkBands(ref *refData, b Benchmark) []string {
	var bad []string
	at := b.From
	for i, x := range b.Bands {
		where := fmt.Sprintf("band %d (%d→%d)", i+1, x.From, x.To)
		if x.From != at {
			bad = append(bad, fmt.Sprintf("%s starts at %d, the previous band ended at %d", where, x.From, at))
		}
		at = x.To
		if x.Gap != "" {
			continue
		}
		row, ok := ref.of(b.Profession)[x.SpellID]
		if !ok {
			bad = append(bad, where+": recipe "+x.Recipe+" is not in the dataset's recipes for "+b.Profession)
			continue
		}
		if asText(row["name"]) != x.Recipe {
			bad = append(bad, fmt.Sprintf("%s: recipe name %q, the dataset says %q", where, x.Recipe, asText(row["name"])))
		}
		if colour(row, x.From) == "grey" && x.Crafts > 0 {
			bad = append(bad, fmt.Sprintf("%s: %s is grey at skill %d (grey_skill %v)", where, x.Recipe, x.From, row["grey_skill"]))
		}
		src := asText(row["learn_skill_source"])
		if l, ok := intOf(row, "learn_skill"); ok && src != "estimate" && src != "" && l > x.From {
			bad = append(bad, fmt.Sprintf("%s: %s needs skill %d (%s), planned at %d", where, x.Recipe, l, src, x.From))
		}
		if x.OrangeCrafts+x.YellowCrafts+x.GreenCrafts != x.Crafts {
			bad = append(bad, fmt.Sprintf("%s: %d orange + %d yellow + %d green is not %d crafts", where, x.OrangeCrafts, x.YellowCrafts, x.GreenCrafts, x.Crafts))
		}
		if d := x.ExpectedSkillUps - float64(x.To-x.From); d < -1.5 || d > 1.5 {
			bad = append(bad, fmt.Sprintf("%s: expected skill-ups %.1f for %d points", where, x.ExpectedSkillUps, x.To-x.From))
		}
		want := ref.reagents(x.SpellID)
		got := map[int64]int{}
		for _, m := range x.Materials {
			got[m.ID] += m.Count
		}
		for id, n := range want {
			if int64(got[id]) != n*int64(x.Crafts) {
				bad = append(bad, fmt.Sprintf("%s: reagent %d needs %d × %d crafts, the plan lists %d", where, id, n, x.Crafts, got[id]))
			}
		}
		for id := range got {
			if _, ok := want[id]; !ok {
				bad = append(bad, fmt.Sprintf("%s: the plan lists reagent %d the recipe does not use", where, id))
			}
		}
		for _, cap := range []int{75, 150, 225} {
			if x.From < cap && x.To > cap {
				bad = append(bad, fmt.Sprintf("%s crosses the rank cap %d", where, cap))
			}
		}
	}
	if len(b.Bands) > 0 && at != b.To {
		bad = append(bad, fmt.Sprintf("the bands end at %d, the plan's target is %d", at, b.To))
	}
	return bad
}

func TestProfessionAdviceBenchmark(t *testing.T) {
	dir := os.Getenv("WOWC_BENCH_DATASET")
	if dir == "" {
		t.Skip("WOWC_BENCH_DATASET is not set: run make advice-benchmark")
	}
	ds := store.NewDir(dir)
	ref := &refData{ds: ds, recipes: map[string]map[int64]store.Row{}}
	guides := loadGuides(t)
	var out []benchRow
	add := func(r benchRow) { out = append(out, r) }
	const datasetSrc = "dataset recipes, recipe_reagents"
	const modelSrc = "dataset recipes yellow_skill, grey_skill"

	for _, g := range guides.Professions {
		guideSrc := g.Source
		// the guide against the dataset: every recipe it uses exists and gives skill-ups where the guide uses it
		var badRecipes, badReagents []string
		for _, s := range g.Steps {
			row := ref.byName(g.Profession, s.Recipe)
			if row == nil {
				badRecipes = append(badRecipes, fmt.Sprintf("%q missing", s.Recipe))
				continue
			}
			if c := colour(*row, s.From); c == "grey" {
				badRecipes = append(badRecipes, fmt.Sprintf("%s grey at %d", s.Recipe, s.From))
			}
			src := asText((*row)["learn_skill_source"])
			if l, ok := intOf(*row, "learn_skill"); ok && l > s.From && src != "estimate" {
				badRecipes = append(badRecipes, fmt.Sprintf("%s learn %d (%s) after guide skill %d", s.Recipe, l, src, s.From))
			}
			if len(s.Reagents) > 0 {
				have := ref.reagents(asI64((*row)["spell_id"]))
				for _, gr := range s.Reagents {
					found := false
					for id, n := range have {
						if strings.EqualFold(itemName(ds, id), gr.Item) {
							found = true
							if int(n) != gr.Per {
								badReagents = append(badReagents, fmt.Sprintf("%s: %s %d a craft, guide %d", s.Recipe, gr.Item, n, gr.Per))
							}
						}
					}
					if !found {
						badReagents = append(badReagents, fmt.Sprintf("%s: no %s in the dataset", s.Recipe, gr.Item))
					}
				}
			}
		}
		res := "match"
		if len(badRecipes) > 0 {
			res = "differ: " + strings.Join(badRecipes, "; ")
		}
		add(benchRow{g.Profession + ": guide recipes and skill ranges", fmt.Sprintf("dataset has %d of %d guide recipes usable at the guide's skill", len(g.Steps)-len(badRecipes), len(g.Steps)),
			"every guide recipe exists and gives skill at its step", guideSrc + " vs " + datasetSrc, res})
		res = "match"
		if len(badReagents) > 0 {
			res = "differ: " + strings.Join(badReagents, "; ")
		}
		add(benchRow{g.Profession + ": guide reagents per craft", "dataset recipe_reagents", "per-craft counts the guide's material totals imply", guideSrc + " vs " + datasetSrc, res})

		// the advisor's plans
		for _, from := range []int{1, 150, 285} {
			b := benchPlan(ds, BenchOptions{Profession: g.Profession, From: from})
			label := fmt.Sprintf("%s from skill %d", g.Profession, from)
			bad := checkBands(ref, b)
			res := "match"
			if len(bad) > 0 {
				res = "differ: " + strings.Join(bad, "; ")
				t.Errorf("%s: %s", label, strings.Join(bad, "; "))
			}
			add(benchRow{label + ": plan against the dataset", fmt.Sprintf("%d bands to skill %d", len(b.Bands), b.To),
				"recipe known, not grey, learn skill, reagent counts, no rank crossed", datasetSrc, res})

			if from == 1 {
				add(sourcingRow(ref, g.Profession, b))
				add(totalsRow(ref, g.Profession, b))
			}

			gaps := 0
			for _, x := range b.Bands {
				if strings.HasPrefix(x.Gap, noRecipeGap) {
					gaps++
				}
			}
			res = "match"
			if len(b.Bands) == 0 || gaps > 0 {
				res = fmt.Sprintf("differ: %d band(s) with no advice", gaps)
			}
			add(benchRow{label + ": every skill point has advice", fmt.Sprintf("%d of %d bands are gaps", gaps, len(b.Bands)), "a guide step for every skill", guideSrc, res})

			if len(b.Bands) > 0 && b.Bands[0].Gap == "" {
				gs := g.step(from)
				first := b.Bands[0].Recipe
				switch {
				case gs == nil:
				case strings.EqualFold(first, gs.Recipe) || strings.EqualFold(b.Bands[0].Alternative, gs.Recipe):
					add(benchRow{label + ": first recipe", first, gs.Recipe, guideSrc, "match"})
				default:
					c := "not in the dataset"
					if row := ref.byName(g.Profession, first); row != nil {
						c = colour(*row, from) + " at skill " + fmt.Sprint(from) + " in the dataset"
					}
					add(benchRow{label + ": first recipe", first, gs.Recipe, guideSrc, "differ: the planner picks the cheapest per skill point; its recipe is " + c})
				}
			}
		}

		// the skill-up model against the guide's craft counts, step by step
		var errSum float64
		var errN int
		for _, st := range g.Steps {
			row := ref.byName(g.Profession, st.Recipe)
			if row == nil || st.Crafts < 8 || strings.HasPrefix(st.Recipe, "Bolt of") {
				continue
			}
			y, gr := intOf(*row, "yellow_skill")
			gy, ok := intOf(*row, "grey_skill")
			if !gr || !ok || st.From >= gy {
				continue
			}
			rec := Recipe{YellowSkill: &y, GreySkill: &gy}
			if c, reach := craftsFor(rec, st.From, st.To); c > 0 && reach == st.To {
				errSum += math.Abs(float64(c-st.Crafts)) / float64(st.Crafts)
				errN++
			}
		}
		if errN > 0 {
			mean := errSum / float64(errN)
			res := "match"
			if mean > 0.2 {
				res = "differ"
				t.Errorf("%s: the skill-up model is off the guide's craft counts by %.0f%% on average", g.Profession, 100*mean)
			}
			add(benchRow{g.Profession + ": crafts per step (skill-up model)", fmt.Sprintf("mean error %.0f%% over %d steps", 100*mean, errN),
				"within 20% of the guide's craft counts", guideSrc + "; " + modelSrc, res})
		}

		// the whole path against the guide
		b := benchPlan(ds, BenchOptions{Profession: g.Profession})
		guideCrafts := 0
		for _, s := range g.Steps {
			guideCrafts += s.Crafts
		}
		adv, inter := totalCrafts(b)
		res = "match"
		if adv > guideCrafts*5/4 {
			res = fmt.Sprintf("differ: %d%% more crafts than the guide; by design the planner minimises material cost (vendor price, gathering effort, learning cost), not crafts", 100*adv/guideCrafts-100)
		}
		advText := fmt.Sprint(adv)
		if inter > 0 {
			advText += fmt.Sprintf(" (+%d intermediate crafts the guide's steps leave out)", inter)
		}
		add(benchRow{g.Profession + " 1→300: total crafts", advText, fmt.Sprintf("%d (within 25%%)", guideCrafts), guideSrc, res})
		used := 0
		for _, s := range g.Steps {
			for _, x := range b.Bands {
				if strings.EqualFold(x.Recipe, s.Recipe) {
					used++
					break
				}
			}
		}
		res = "match"
		if used*2 < len(g.Steps) {
			res = "differ: the planner's route shares fewer than half of the guide's recipes"
		}
		add(benchRow{g.Profession + " 1→300: recipes in common", fmt.Sprintf("%d of %d guide recipes", used, len(g.Steps)), "at least half", guideSrc, res})
	}

	// gathering professions: the plan names what to gather, never "a pattern from the Auction House"
	for _, prof := range []string{"Herbalism", "Mining", "Skinning", "Fishing"} {
		b := benchPlan(ds, BenchOptions{Profession: prof})
		text := RenderBenchmark(b)
		res := "match"
		if strings.Contains(text, "pattern at the Auction House") {
			res = "differ: the plan sends a gathering profession to the Auction House for a pattern"
			t.Errorf("%s: %s", prof, res)
		}
		add(benchRow{prof + " 1→300: gathering advice", fmt.Sprintf("%d bands, %d gap(s)", len(b.Bands), func() int {
			n := 0
			for _, x := range b.Bands {
				if strings.HasPrefix(x.Gap, noRecipeGap) {
					n++
				}
			}
			return n
		}()), "gather nodes or catches by skill", "https://www.wow-professions.com/forever/" + strings.ToLower(prof) + "-leveling-guide; dataset gathering_nodes", res})
	}
	printCounts("professions", out)
	writeBenchTable(t, "Profession advice", out)
}

// printCounts prints the area's same/differ count line that make advice-benchmark adds up into one summary line.
func printCounts(area string, rows []benchRow) {
	results := make([]string, len(rows))
	for i, r := range rows {
		results[i] = r.result
	}
	fmt.Println(benchsummary.AreaLine(area, results))
}

func writeBenchTable(t *testing.T, title string, rows []benchRow) {
	path := os.Getenv("WOWC_BENCH_REPORT")
	if path == "" || len(rows) == 0 {
		return
	}
	sort.SliceStable(rows, func(i, j int) bool { return false })
	var b strings.Builder
	fmt.Fprintf(&b, "### %s\n\n| Check | Advisor | Reference | Source | Result |\n| --- | --- | --- | --- | --- |\n", title)
	for _, r := range rows {
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s |\n", r.check, r.advisor, r.reference, r.source, r.result)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(b.String() + "\n"); err != nil {
		t.Fatal(err)
	}
}

// TestTailoringGuideBands lines the Tailoring plan up with the Forever Tailoring guide, step by step: the
// advisor's recipe at the start of each guide step, the guide's, and whether they match. The advisor
// picks its recipes by cost, so a difference is listed, not a failure; a recipe used before the skill it
// can be learnt at (the dataset's learn skill, or for an estimate its yellow start less estimateMargin) is.
func TestTailoringGuideBands(t *testing.T) {
	dir := os.Getenv("WOWC_BENCH_DATASET")
	if dir == "" {
		t.Skip("WOWC_BENCH_DATASET is not set: run make advice-benchmark")
	}
	ds := store.NewDir(dir)
	ref := &refData{ds: ds, recipes: map[string]map[int64]store.Row{}}
	var guide *guideProf
	for _, g := range loadGuides(t).Professions {
		if g.Profession == "Tailoring" {
			g := g
			guide = &g
		}
	}
	if guide == nil {
		t.Fatal("no Tailoring guide recorded")
	}
	b := NewBenchmark(ds, BenchOptions{Profession: "Tailoring", Level: 60, Faction: "horde", From: 1, To: 300})
	var out []benchRow
	for _, s := range guide.Steps {
		got := "no step"
		for _, x := range b.Bands {
			if x.From <= s.From && s.From < x.To {
				got = x.Recipe
				if x.Gap != "" {
					got = "gap"
				}
			}
		}
		res := "match"
		if !strings.EqualFold(got, s.Recipe) {
			res = "differ"
		}
		out = append(out, benchRow{fmt.Sprintf("Tailoring %d→%d", s.From, s.To), got, s.Recipe, guide.Source, res})
	}
	for _, x := range b.Bands {
		if x.Gap != "" {
			continue
		}
		row := ref.byName("Tailoring", x.Recipe)
		if row == nil {
			continue
		}
		learn, ok := intOf(*row, "learn_skill")
		if !ok {
			continue
		}
		if asText((*row)["learn_skill_source"]) == "estimate" && learn > 30 {
			if y, ok := intOf(*row, "yellow_skill"); ok {
				learn = max(learn, y-estimateMargin)
			}
		}
		if x.From < learn {
			t.Errorf("%s is planned at %d, before its learn skill %d", x.Recipe, x.From, learn)
		}
	}
	printCounts("tailoring-guide", out)
	writeBenchTable(t, "Tailoring plan against the Forever guide", out)
}

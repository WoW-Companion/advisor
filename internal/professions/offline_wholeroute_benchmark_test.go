package professions

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/store"
)

// The offline whole-route benchmark: what a profession takes from the first skill point to the cap. Four kinds of case, each set beside
// raw dataset rows (testdata/advice_snapshot) or the recorded Forever guides (testdata/forever_guides.json), never through the advisor's reader:
//   - route: the route the advisor reads for a profession (order of recipes, the skill each step starts and ends at, the crafts in all)
//     beside the guide's steps;
//   - materials: the reagents of a bracket (the three steps the skill-up answer shows) and of the whole route, crafts times the per-craft
//     count in profession_routes;
//   - gold: the vendor price of the materials of a plan (the item's buy price, else the cheapest listing) and what learning its recipes costs;
//   - no answer: a profession, skill or price the dataset holds nothing for, where the advisor must say so and name no recipe or price.
// make offline-benchmark runs it; WOWC_BENCH_REPORT receives the tables.

const (
	wholeSrc = "dataset profession_routes (WoW Forever guides)"
	goldSrc  = "dataset items.buy_price, vendor_sources.price_copper, trainer_spells.cost_copper, recipe_reagents"
)

var wholeRouteProfs = []string{"Alchemy", "Blacksmithing", "Cooking", "Enchanting", "Engineering", "First Aid", "Leatherworking", "Tailoring"}

// guideOf is the recorded guide of a profession.
func guideOf(t *testing.T, prof string) (guideProf, bool) {
	for _, g := range loadGuides(t).Professions {
		if g.Profession == prof {
			return g, true
		}
	}
	return guideProf{}, false
}

// TestOfflineBenchmarkWholeRoute sets each profession's route as the advisor reads it (1 to the cap) beside the rows of the table and the
// recorded guide: the recipes in order, the skill every step starts at and ends at, and the crafts in all.
func TestOfflineBenchmarkWholeRoute(t *testing.T) {
	ds := snapshot().ds
	var out []benchRow
	for _, prof := range wholeRouteProfs {
		main, _ := refRoute(t, ds, prof)
		r, ok := LoadRoute(ds, prof)
		if !ok {
			t.Fatalf("%s: no route", prof)
		}
		var haveSteps, rowSteps []string
		var haveCrafts, rowCrafts int
		for _, s := range r.Steps {
			haveSteps = append(haveSteps, fmt.Sprintf("%d-%d %s", s.From, s.To, s.Recipe))
			haveCrafts += s.Crafts
		}
		for _, s := range main {
			rowSteps = append(rowSteps, fmt.Sprintf("%d-%d %s", s.from, s.to, s.recipe))
			rowCrafts += s.crafts
		}
		res := resultIf(strings.Join(haveSteps, "; ") == strings.Join(rowSteps, "; ") && haveCrafts == rowCrafts, "the route read is not the table's rows")
		out = append(out, benchRow{prof + " 1→300: route read (order, boundaries, crafts)", fmt.Sprintf("%d steps, %d crafts", len(r.Steps), haveCrafts), fmt.Sprintf("%d steps, %d crafts", len(main), rowCrafts), wholeSrc, res})

		// the route is unbroken: each step starts where an earlier one ends, from skill 1 to the cap
		reach, hole := 1, 0
		for _, s := range r.Steps {
			if s.From > reach && hole == 0 {
				hole = reach
			}
			reach = max(reach, s.To)
		}
		out = append(out, benchRow{prof + " 1→300: no skill left uncovered", fmt.Sprintf("covers 1 to %d", reach), "1 to 300, no hole", wholeSrc, resultIf(hole == 0 && reach == 300, fmt.Sprintf("a hole at skill %d, cap %d", hole, reach))})

		// the first three steps the answer gives at skill 1
		got := RouteFacts(routeChar(prof, 1), ds, nil, prof)
		var haveFirst []string
		for _, line := range strings.Split(got, "\n") {
			if strings.HasPrefix(line, "Now: ") || strings.HasPrefix(line, "Then: ") {
				head, _, _ := strings.Cut(line, ";")
				if i := strings.Index(head, " x "); i >= 0 {
					haveFirst = append(haveFirst, strings.TrimSuffix(head[i+3:], "."))
				}
			}
		}
		g, hasGuide := guideOf(t, prof)
		var guideFirst, rowFirst []string
		for i := 0; i < len(g.Steps) && i < 3; i++ {
			guideFirst = append(guideFirst, g.Steps[i].Recipe)
		}
		for i := 0; i < len(main) && i < 3; i++ {
			rowFirst = append(rowFirst, main[i].recipe)
		}
		res = resultIf(strings.Join(haveFirst, ", ") == strings.Join(rowFirst, ", "), "the answer at skill 1 is not the table's first steps")
		if res == "match" && hasGuide && strings.Join(guideFirst, ", ") != strings.Join(haveFirst, ", ") {
			res = "differ (data): the dataset's first steps are not the recorded guide's"
		}
		out = append(out, benchRow{prof + ": the first steps at skill 1", strings.Join(haveFirst, ", "), strings.Join(guideFirst, ", "), wholeSrc + "; " + g.Source, res})

		// the recorded guide's whole route: recipes in order, and the crafts in all
		if hasGuide {
			var guideSteps []string
			var guideCrafts int
			for _, s := range g.Steps {
				guideSteps = append(guideSteps, fmt.Sprintf("%d-%d %s", s.From, s.To, s.Recipe))
				guideCrafts += s.Crafts
			}
			res = "match"
			switch {
			case strings.Join(guideSteps, "; ") != strings.Join(haveSteps, "; "):
				res = fmt.Sprintf("differ (data): the dataset route has %d steps, the recorded guide %d; their boundaries or recipes differ", len(haveSteps), len(guideSteps))
			case guideCrafts != haveCrafts:
				res = "differ (data): the crafts differ"
			}
			out = append(out, benchRow{prof + " 1→300: the guide's order, boundaries and crafts", fmt.Sprintf("%d steps, %d crafts", len(haveSteps), haveCrafts), fmt.Sprintf("%d steps, %d crafts", len(guideSteps), guideCrafts), g.Source, res})
		}
	}
	finishOffline(t, "professions-whole-route", "Offline whole-route cases", out)
}

// namedCount is "N Name" as the skill-up answer writes a material.
var namedCount = regexp.MustCompile(`^(\d+) (.+?)(?: \(.*\))?$`)

// bracketCases are the skills whose three-step answer (a bracket of the route) has its materials added.
var bracketCases = []struct {
	prof  string
	skill int
}{{"Tailoring", 1}, {"Alchemy", 60}, {"Blacksmithing", 100}, {"Engineering", 135}, {"Leatherworking", 100}, {"Enchanting", 110}, {"Cooking", 1}, {"First Aid", 125}}

// TestOfflineBenchmarkBracketMaterials adds the materials of the steps the skill-up answer shows (the step covering the skill and the
// next two, crafts left in the first) and sets each total beside the table's per-craft counts times the crafts.
func TestOfflineBenchmarkBracketMaterials(t *testing.T) {
	ds := snapshot().ds
	var out []benchRow
	for _, c := range bracketCases {
		main, _ := refRoute(t, ds, c.prof)
		i := -1
		for k, s := range main {
			if c.skill >= s.from && c.skill < s.to {
				i = k
				break
			}
		}
		if i < 0 {
			t.Fatalf("%s %d: no step", c.prof, c.skill)
		}
		want := map[string]int{}
		for k := i; k < min(i+3, len(main)); k++ {
			crafts := main[k].crafts
			if k == i {
				crafts = main[k].left(c.skill)
			}
			for _, m := range main[k].mats {
				want[m.name] += m.count * crafts
			}
		}
		have := map[string]int{}
		for _, line := range strings.Split(RouteFacts(routeChar(c.prof, c.skill), ds, nil, c.prof), "\n") {
			if !strings.HasPrefix(line, "Now: ") && !strings.HasPrefix(line, "Then: ") {
				continue
			}
			_, mats, ok := strings.Cut(line, "; materials: ")
			if !ok {
				continue
			}
			mats, _, _ = strings.Cut(mats, "; note:")
			mats = strings.TrimSuffix(mats, ".")
			for _, part := range strings.Split(mats, "; ") {
				// a part is "N Name (you have none)": the name may hold a comma, never a semicolon
				if m := namedCount.FindStringSubmatch(part); m != nil {
					var n int
					fmt.Sscan(m[1], &n)
					have[m[2]] += n
				}
			}
		}
		res := "match"
		var bad []string
		for name, n := range want {
			if have[name] != n {
				bad = append(bad, fmt.Sprintf("%s: %d, dataset %d", name, have[name], n))
			}
		}
		for name := range have {
			if _, ok := want[name]; !ok {
				bad = append(bad, name+" is not in the bracket")
			}
		}
		sort.Strings(bad)
		if len(bad) > 0 {
			res = "differ: " + strings.Join(bad, "; ")
		}
		out = append(out, benchRow{fmt.Sprintf("%s at skill %d: the materials of the three steps shown", c.prof, c.skill), joinCounts(have), joinCounts(want), wholeSrc, res})
	}
	finishOffline(t, "professions-bracket-materials", "Offline bracket material totals", out)
}

// TestOfflineBenchmarkRouteMaterials adds the materials of every step of a route (the whole 1 to 300), as the advisor reads them, beside the
// same sum over the raw rows. A material a step makes for a later one is counted where it is used and where it is made, as the table lists.
func TestOfflineBenchmarkRouteMaterials(t *testing.T) {
	ds := snapshot().ds
	var out []benchRow
	for _, prof := range wholeRouteProfs {
		main, _ := refRoute(t, ds, prof)
		want := map[string]int{}
		for _, s := range main {
			for _, m := range s.mats {
				want[m.name] += m.count * s.crafts
			}
		}
		r, _ := LoadRoute(ds, prof)
		have := map[string]int{}
		for _, s := range r.Steps {
			for _, m := range s.Materials {
				have[itemName(ds, m.ItemID)] += m.Count * s.Crafts
			}
		}
		var bad []string
		for name, n := range want {
			if have[name] != n {
				bad = append(bad, fmt.Sprintf("%s: %d, dataset %d", name, have[name], n))
			}
		}
		sort.Strings(bad)
		var sumHave, sumWant int
		for _, n := range have {
			sumHave += n
		}
		for _, n := range want {
			sumWant += n
		}
		out = append(out, benchRow{prof + " 1→300: every material of the route", fmt.Sprintf("%d materials, %d kinds", sumHave, len(have)), fmt.Sprintf("%d materials, %d kinds", sumWant, len(want)), wholeSrc, resultIf(len(bad) == 0 && len(have) == len(want), strings.Join(bad, "; "))})
	}
	finishOffline(t, "professions-route-materials", "Offline whole-route material totals", out)
}

func joinCounts(m map[string]int) string {
	var parts []string
	for name, n := range m {
		parts = append(parts, fmt.Sprintf("%d %s", n, name))
	}
	sort.Strings(parts)
	return strings.Join(parts, ", ")
}

// refUnit is a vendor's price for one unit as the dataset publishes it: the item's buy price, else the cheapest listing; ok is false when no
// vendor sells the item.
func refUnit(ds store.DataStore, id int64) (copper int64, ok bool) {
	rows, _ := ds.Filter("vendor_sources", store.Range{Column: "item_id", Min: id, Max: id})
	if len(rows) == 0 {
		return 0, false
	}
	if it, err := ds.Get("items", id); err == nil {
		if b := asI64(it["buy_price"]); b > 0 {
			return b, true
		}
	}
	for _, r := range rows {
		if p := asI64(r["price_copper"]); copper == 0 || p < copper {
			copper = p
		}
	}
	return copper, true
}

// goldWindows are the plans whose gold cost is checked: the profession and the skill planned to, from 1. None crafts an intermediate from raw
// materials, so the reagents of the bands are the materials bought.
var goldWindows = []struct {
	prof string
	to   int
}{{"First Aid", 75}, {"Cooking", 75}, {"Alchemy", 75}, {"Tailoring", 40}, {"Enchanting", 75}, {"Blacksmithing", 75}}

var costInText = regexp.MustCompile(`vendor cost ([^,]+),`)

func TestOfflineBenchmarkGoldCostFirstAid(t *testing.T)      { offlineGold(t, 0) }
func TestOfflineBenchmarkGoldCostCooking(t *testing.T)       { offlineGold(t, 1) }
func TestOfflineBenchmarkGoldCostAlchemy(t *testing.T)       { offlineGold(t, 2) }
func TestOfflineBenchmarkGoldCostTailoring(t *testing.T)     { offlineGold(t, 3) }
func TestOfflineBenchmarkGoldCostEnchanting(t *testing.T)    { offlineGold(t, 4) }
func TestOfflineBenchmarkGoldCostBlacksmithing(t *testing.T) { offlineGold(t, 5) }

// offlineGold adds what the vendor materials of a plan cost, and what learning its recipes costs, from the raw tables,
// and sets the plan's totals and the cost line it prints beside them.
func offlineGold(t *testing.T, i int) {
	ref := snapshot()
	var out []benchRow
	for _, w := range goldWindows[i : i+1] {
		b := NewBenchmark(ref.ds, BenchOptions{Profession: w.prof, From: 1, To: w.to, Level: 60, Faction: "horde"})
		if len(b.Bands) == 0 {
			t.Fatalf("%s: no bands: %v", w.prof, b.Notes)
		}
		need := map[int64]int64{}
		made := map[int64]bool{}
		var learn int64
		for _, x := range b.Bands {
			if x.Gap != "" {
				continue
			}
			for id, n := range ref.reagents(x.SpellID) {
				if !made[id] {
					need[id] += n * int64(x.Crafts)
				}
			}
			row := ref.of(w.prof)[x.SpellID]
			made[asI64(row["creates_item_id"])] = true
			if kind, copper := ref.datasetSource(row); kind == "trainer, confirmed" || kind == "vendor pattern" {
				learn += copper
			}
		}
		var want int64
		var priced []string
		for id, n := range need {
			if unit, ok := refUnit(ref.ds, id); ok {
				want += unit * n
				priced = append(priced, fmt.Sprintf("%d %s at %s", n, itemName(ref.ds, id), Money(unit)))
			}
		}
		sort.Strings(priced)
		label := fmt.Sprintf("%s 1→%d", w.prof, w.to)
		out = append(out, benchRow{label + ": vendor materials, total price", Money(b.Efficiency.VendorCopper), Money(want) + " (" + strings.Join(priced, ", ") + ")", goldSrc, resultIf(b.Efficiency.VendorCopper == want, "the plan's vendor total is not the dataset's prices times the counts")})

		shown := "no cost line"
		if m := costInText.FindStringSubmatch(RenderBenchmark(b)); m != nil {
			shown = m[1]
		}
		out = append(out, benchRow{label + ": the cost the plan prints", shown, Money(want), goldSrc, resultIf(shown == Money(want), "the printed cost is not the total")})

		var haveLearn int64
		for _, x := range b.Bands {
			haveLearn += x.LearnCopper
		}
		out = append(out, benchRow{label + ": learning the recipes, total price", Money(haveLearn), Money(learn), goldSrc, resultIf(haveLearn == learn, "the learn prices do not add up to the dataset's")})
	}
	finishOffline(t, "professions-gold-cost-"+strings.ReplaceAll(strings.ToLower(goldWindows[i].prof), " ", "-"), "Offline gold cost: "+goldWindows[i].prof, out)
}

// TestOfflineBenchmarkLimitedStock sets a material only a limited-stock vendor sells beside the dataset's vendor rows: the plan must say the
// stock is limited, as the vendor line already does, since a plan needs 50 of it.
func TestOfflineBenchmarkLimitedStock(t *testing.T) {
	ref := snapshot()
	b := NewBenchmark(ref.ds, BenchOptions{Profession: "Enchanting", From: 1, To: 75, Level: 60, Faction: "horde"})
	var out []benchRow
	for _, m := range b.Materials {
		if m.Kind != "vendor" {
			continue
		}
		rows, _ := ref.ds.Filter("vendor_sources", store.Range{Column: "item_id", Min: m.ID, Max: m.ID})
		all := len(rows) > 0
		for _, r := range rows {
			if l, _ := r["limited"].(bool); !l {
				all = false
			}
		}
		if !all {
			continue
		}
		res := resultIf(strings.Contains(m.Source, "limited stock"), "the plan does not say the stock is limited")
		out = append(out, benchRow{fmt.Sprintf("Enchanting 1→75: %d %s, every vendor limited", m.Count, m.Item), m.Source, "limited stock", "dataset vendor_sources.limited", res})
	}
	if len(out) == 0 {
		t.Fatal("no limited-stock vendor material in the plan")
	}
	finishOffline(t, "professions-limited-stock", "Offline limited-stock vendor materials", out)
}

// TestOfflineBenchmarkWholeRouteNoAnswer: asks the dataset has no answer for. The advisor says so and names no recipe, price or step.
func TestOfflineBenchmarkWholeRouteNoAnswer(t *testing.T) {
	ref := snapshot()
	var out []benchRow

	// a profession with no route and no recipe rows: the route answer is empty and the plan names no recipe
	got := RouteFacts(routeChar("Jewelcrafting", 50), ref.ds, nil, "Jewelcrafting")
	b := NewBenchmark(ref.ds, BenchOptions{Profession: "Jewelcrafting", From: 1, To: 75, Level: 60, Faction: "horde"})
	named := 0
	for _, x := range b.Bands {
		if x.Gap == "" {
			named++
		}
	}
	out = append(out, benchRow{"Jewelcrafting 1→75: no route and no recipes in the dataset", fmt.Sprintf("route answer %q, %d recipes named", got, named), "no route, no recipe named", wholeSrc, resultIf(got == "" && named == 0, "the advisor names a route or a recipe the dataset does not hold")})

	// a character at the cap has no step left
	got = RouteFacts(routeChar("Tailoring", 300), ref.ds, nil, "Tailoring")
	out = append(out, benchRow{"Tailoring at skill 300: the route is over", fmt.Sprintf("route answer %q", got), "no step left", wholeSrc, resultIf(got == "", "the advisor gives a step past the cap")})

	// a stretch of skill no recipe of the snapshot covers: a gap, with no recipe made up
	b = NewBenchmark(ref.ds, BenchOptions{Profession: "Alchemy", From: 150, To: 200, Level: 60, Faction: "horde"})
	named, gaps := 0, 0
	for _, x := range b.Bands {
		if x.Gap != "" {
			gaps++
		} else {
			named++
		}
	}
	out = append(out, benchRow{"Alchemy 150→200 beyond the dataset's recipes: no recipe", fmt.Sprintf("%d gap band(s), %d recipe(s) named", gaps, named), "a gap, no recipe", snapSource, resultIf(gaps > 0 && named == 0, "the plan names a recipe the dataset does not hold")})

	// a material no vendor sells and no source lists: no price is made up
	b = NewBenchmark(ref.ds, BenchOptions{Profession: "Blacksmithing", From: 1, To: 75, Level: 60, Faction: "horde"})
	var priced, unpriced []string
	for _, m := range b.Materials {
		if m.Kind == "vendor" {
			continue
		}
		if m.CostCopper != 0 || m.UnitCopper != 0 {
			priced = append(priced, m.Item)
		} else {
			unpriced = append(unpriced, m.Item)
		}
	}
	text := RenderBenchmark(b)
	shownPrice := 0
	for _, l := range strings.Split(text, "\n") {
		for _, name := range unpriced {
			if strings.HasPrefix(l, "- ") && strings.Contains(l, " × "+name+":") && strings.Contains(l, " = ") {
				shownPrice++
			}
		}
	}
	out = append(out, benchRow{"Blacksmithing 1→75: materials no vendor sells have no price", fmt.Sprintf("%d unpriced (%s), %d priced, %d price lines", len(unpriced), strings.Join(unpriced, ", "), len(priced), shownPrice), "no price stated", goldSrc, resultIf(len(priced) == 0 && shownPrice == 0 && len(unpriced) > 0, "a price is stated for a material no vendor sells")})

	// a trainer recipe the dataset has no price for: counted apart, never shown as free
	free := ""
	for _, tr := range b.Tiers {
		if tr.TrainerSpells > 0 && tr.TrainerCopper == 0 && tr.TrainerSpells != tr.TrainerUnpriced && strings.Contains(text, fmt.Sprintf("trainer recipes on this path: %d for 0c", tr.TrainerSpells)) {
			free = tr.Name
		}
	}
	line := ""
	for _, l := range strings.Split(text, "\n") {
		if strings.Contains(l, "trainer recipes on this path") {
			line = strings.TrimSpace(l)
		}
	}
	out = append(out, benchRow{"Blacksmithing 1→75: trainer recipes with no price in the dataset", line, "price not in the dataset, never 0c", "dataset trainer_spells.cost_copper", resultIf(free == "" && strings.Contains(line, "price not in the dataset") && !strings.Contains(line, "for 0c"), "unpriced recipes are shown as free")})
	finishOffline(t, "professions-whole-route-no-answer", "Offline whole-route cases the dataset has no answer for", out)
}

func TestLimitedNote(t *testing.T) {
	lim, open := store.Row{"limited": true}, store.Row{"limited": false}
	for _, c := range []struct {
		name string
		rows []store.Row
		want string
	}{
		{"none", nil, ""},
		{"all limited", []store.Row{lim, lim}, " (limited stock)"},
		{"one open vendor", []store.Row{lim, open}, ""},
		{"no flag", []store.Row{{"price_copper": int64(5)}}, ""},
		{"numeric flag", []store.Row{{"limited": int64(1)}}, " (limited stock)"},
	} {
		if got := limitedNote(c.rows); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

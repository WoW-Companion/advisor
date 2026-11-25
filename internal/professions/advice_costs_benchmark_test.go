package professions

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/store"
)

// The costs and places half of the profession advice benchmark (make advice-benchmark runs it with the
// recipe half in advice_benchmark_test.go). For every profession it plans a character at five skills
// (1, 75, 150, 225, 285) and compares what the plan says about money and places with the dataset:
//
//   - trainer recipes: the price against trainer_spells, never before the skill the trainer asks for, and
//     the trainer gate's total never "0c" for a price the dataset does not list;
//   - trainer places: a trainer of the profession, named, in a zone, on the character's own side;
//   - vendor materials and patterns: the unit price against the item's base price in items.buy_price
//     (what a character with no reputation pays), the totals against the unit prices;
//   - gathering: the nodes a plan names need a skill the character has, with the skill the nodes table lists.
//
// A difference in the advice fails the test; a difference whose cause is a hole in the dataset (a trainer
// with no name or zone, a trainer recipe no trainer lists) is listed as "differ (data)" and fails nothing.

var benchProfessions = []string{"Alchemy", "Blacksmithing", "Enchanting", "Engineering", "Leatherworking", "Tailoring", "Cooking", "First Aid"}

var benchSkills = []int{1, 75, 150, 225, 285}

const (
	trainerSrc = "dataset trainer_spells, trainers"
	vendorSrc  = "dataset vendor_sources, items.buy_price"
)

// trainerOffer is the cheapest price among the trainer_spells rows of a spell and the lowest skill a trainer asks;
// listed says some trainer teaches it.
func trainerOffer(ds store.DataStore, spell int64) (cost int64, skill int, listed bool) {
	rows, _ := ds.Filter("trainer_spells", store.Range{Column: "spell_id", Min: spell, Max: spell})
	cost = -1
	skill = -1
	for _, r := range rows {
		listed = true
		if c := asI64(r["cost_copper"]); c > 0 && (cost < 0 || c < cost) {
			cost = c
		}
		if s := int(asI64(r["required_skill"])); s > 1 && (skill < 0 || s < skill) {
			skill = s
		}
	}
	return cost, skill, listed
}

// trainerRows are the dataset's trainers of a profession.
func trainerRows(ds store.DataStore, prof string) []store.Row {
	rows, _ := ds.Filter("trainers", store.Range{Column: "profession", Min: prof, Max: prof})
	return rows
}

func joinMax(bad []string, n int) string {
	sort.Strings(bad)
	if len(bad) > n {
		return strings.Join(bad[:n], "; ") + fmt.Sprintf("; and %d more", len(bad)-n)
	}
	return strings.Join(bad, "; ")
}

var needsSkillRE = regexp.MustCompile(`(?:Gather |; )([^;()]+?) \(needs skill (\d+)`)

func TestProfessionCostsBenchmark(t *testing.T) {
	dir := os.Getenv("WOWC_BENCH_DATASET")
	if dir == "" {
		t.Skip("WOWC_BENCH_DATASET is not set: run make advice-benchmark")
	}
	ds := store.NewDir(dir)
	var out []benchRow
	add := func(r benchRow) { out = append(out, r) }
	fail := func(row benchRow) {
		if strings.HasPrefix(row.result, "differ") && !strings.HasPrefix(row.result, "differ (data)") {
			t.Errorf("%s: %s", row.check, row.result)
		}
	}

	for _, prof := range benchProfessions {
		var costBad, gateBad, vendorBad, patternBad, totalBad []string
		var costN, gateN, vendorN, patternN int
		var tierFree []string
		for _, from := range benchSkills {
			b := benchPlan(ds, BenchOptions{Profession: prof, From: from})
			where := fmt.Sprintf("from %d", from)
			for _, x := range b.Bands {
				if x.Gap != "" || x.LearnVia != "trainer" {
					continue
				}
				cost, skill, listed := trainerOffer(ds, x.SpellID)
				if !listed {
					continue // a trainer recipe no trainer lists: the sourcing row counts it
				}
				costN++
				if cost > 0 && x.LearnCopper != cost {
					costBad = append(costBad, fmt.Sprintf("%s %s: %s, trainer_spells %s", where, x.Recipe, Money(x.LearnCopper), Money(cost)))
				}
				gateN++
				if skill > 0 && x.From < skill {
					gateBad = append(gateBad, fmt.Sprintf("%s %s planned at %d, a trainer asks skill %d", where, x.Recipe, x.From, skill))
				}
			}
			text := RenderBenchmark(b)
			for _, tr := range b.Tiers {
				if tr.TrainerSpells > 0 && tr.TrainerCopper == 0 && strings.Contains(text, fmt.Sprintf("trainer recipes on this path: %d for 0c", tr.TrainerSpells)) {
					tierFree = append(tierFree, fmt.Sprintf("%s %s: %d recipe(s) shown as 0c", where, tr.Name, tr.TrainerSpells))
				}
			}
			for _, m := range b.Materials {
				if m.Kind != "vendor" {
					continue
				}
				vendorN++
				if it, err := ds.Get("items", m.ID); err == nil {
					if base := asI64(it["buy_price"]); base > 0 && m.UnitCopper != base {
						vendorBad = append(vendorBad, fmt.Sprintf("%s %s: plan %s, base price %s", where, m.Item, Money(m.UnitCopper), Money(base)))
					}
				}
				if m.CostCopper != m.UnitCopper*int64(m.Count) {
					totalBad = append(totalBad, fmt.Sprintf("%s %s: %d × %d is not %d", where, m.Item, m.Count, m.UnitCopper, m.CostCopper))
				}
			}
			var sum int64
			for _, m := range b.Materials {
				sum += m.CostCopper
			}
			if sum != b.Efficiency.VendorCopper {
				totalBad = append(totalBad, fmt.Sprintf("%s: vendor cost %d, the materials add up to %d", where, b.Efficiency.VendorCopper, sum))
			}
			for _, ri := range b.RecipeItems {
				if ri.Source != "vendor" {
					continue
				}
				patternN++
				if it, err := ds.Get("items", ri.ID); err == nil {
					if base := asI64(it["buy_price"]); base > 0 && ri.Copper != base {
						patternBad = append(patternBad, fmt.Sprintf("%s %s: plan %s, base price %s", where, ri.Item, Money(ri.Copper), Money(base)))
					}
				}
			}
		}

		res := resultOf(costBad)
		row := benchRow{prof + " (skills 1, 75, 150, 225, 285): trainer prices", fmt.Sprintf("%d trainer recipes planned", costN), "the cheapest trainer_spells cost_copper of each recipe", trainerSrc, res}
		add(row)
		fail(row)
		row = benchRow{prof + " (skills 1, 75, 150, 225, 285): trainer skill gate", fmt.Sprintf("%d trainer recipes planned", gateN), "never before the skill a trainer asks (required_skill)", trainerSrc, resultOf(gateBad)}
		add(row)
		fail(row)
		row = benchRow{prof + " (skills 1, 75, 150, 225, 285): trainer gate totals", "the Gates line of every plan", "no trainer total of 0c for a price the dataset does not list", trainerSrc, resultOf(tierFree)}
		add(row)
		fail(row)
		row = benchRow{prof + " (skills 1, 75, 150, 225, 285): vendor material prices", fmt.Sprintf("%d vendor materials priced", vendorN), "the item's base price, items.buy_price (no reputation discount)", vendorSrc, resultOf(vendorBad)}
		add(row)
		fail(row)
		row = benchRow{prof + " (skills 1, 75, 150, 225, 285): vendor pattern prices", fmt.Sprintf("%d vendor patterns priced", patternN), "the item's base price, items.buy_price", vendorSrc, resultOf(patternBad)}
		add(row)
		fail(row)
		row = benchRow{prof + " (skills 1, 75, 150, 225, 285): vendor cost totals", "unit price × count, summed", "the plan's own materials", "the plan's own totals", resultOf(totalBad)}
		add(row)
		fail(row)

		// the trainer's place, for each side
		var placeBad, placeData []string
		for _, side := range []string{"horde", "alliance"} {
			p := &planner{c: character.Character{Faction: titleCase(side)}, ds: ds, prof: prof}
			named := p.trainer()
			switch {
			case named == "":
				placeData = append(placeData, side+": no trainer named, the dataset lists no "+prof+" trainer")
				continue
			case strings.Contains(named, "name not in the dataset"):
				placeData = append(placeData, side+": "+named)
			}
			zone := ""
			if i := strings.LastIndex(named, " in "); i >= 0 {
				zone = named[i+4:]
			}
			if zone == "" {
				placeData = append(placeData, side+": "+named+" has no zone in the dataset")
			}
			if s := sideOfZone(zone); s != "" && s != side {
				placeBad = append(placeBad, fmt.Sprintf("%s character sent to %s", side, named))
			}
		}
		res = resultOf(placeBad)
		if res == "match" && len(placeData) > 0 {
			res = "differ (data): " + joinMax(placeData, 4)
		}
		advice := "trainer named for both sides"
		if len(placeBad) == 0 && len(placeData) > 0 {
			advice = "trainer named, place incomplete"
		}
		row = benchRow{prof + ": trainer place", advice, "a " + prof + " trainer with a name and a zone on the character's own side", "dataset trainers", res}
		add(row)
		fail(row)
	}

	// gathering professions: the nodes a plan names are reachable within the band and need the skill the nodes table lists
	for _, prof := range []string{"Herbalism", "Mining"} {
		var bad []string
		n := 0
		nodeSkill := map[string]int{}
		rows, _ := ds.Filter("gathering_nodes", store.Range{Column: "profession", Min: prof, Max: prof})
		for _, r := range rows {
			if s := int(asI64(r["skill_required"])); s > 0 {
				nodeSkill[asText(r["name"])] = s
			}
		}
		for _, from := range benchSkills {
			b := benchPlan(ds, BenchOptions{Profession: prof, From: from})
			for _, x := range b.Bands {
				if !strings.Contains(x.Gap, "levels by gathering") {
					continue
				}
				for _, m := range needsSkillRE.FindAllStringSubmatch(x.Gap, -1) {
					n++
					var need int
					fmt.Sscan(m[2], &need)
					switch {
					case need > x.To:
						bad = append(bad, fmt.Sprintf("from %d band %d→%d names %s, which needs skill %d", from, x.From, x.To, m[1], need))
					case nodeSkill[m[1]] != need:
						bad = append(bad, fmt.Sprintf("from %d band %d→%d: %s needs skill %d, gathering_nodes says %d", from, x.From, x.To, m[1], need, nodeSkill[m[1]]))
					}
				}
			}
		}
		row := benchRow{prof + " (skills 1, 75, 150, 225, 285): gathering nodes", fmt.Sprintf("%d nodes named", n), "every named node is reachable within its band and needs the skill gathering_nodes lists", "dataset gathering_nodes", resultOf(bad)}
		add(row)
		fail(row)
	}
	// the guide-check line each plan ends its bands with is readable for every profession, Fishing included
	for _, prof := range append(append([]string{}, benchProfessions...), "Herbalism", "Mining", "Skinning", "Fishing") {
		b := benchPlan(ds, BenchOptions{Profession: prof})
		res := "match"
		if strings.Contains(b.GuideCheck, "()") || b.GuideCheck == "" {
			res = "differ: the guide-check line is empty or has an empty source: " + b.GuideCheck
		}
		row := benchRow{prof + " 1→300: guide-check line", fmt.Sprintf("%d bands, %d characters", len(b.Bands), len(b.GuideCheck)), "each band's recipe and how it is learnt, no empty brackets", "the plan's own bands", res}
		add(row)
		fail(row)
	}
	printCounts("costs", out)
	writeBenchTable(t, "Profession costs and places", out)
}

func resultOf(bad []string) string {
	if len(bad) == 0 {
		return "match"
	}
	return "differ: " + joinMax(bad, 5)
}

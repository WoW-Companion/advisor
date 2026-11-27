package professions

import (
	"fmt"
	"sort"
	"strings"
	"testing"
)

// The offline multi-item material total benchmark: the materials a whole run of crafts needs, over every recipe of a profession's plan,
// beside the dataset's recipe_reagents. The reference is each recipe's reagents times its crafts, added over the bands, leaving out a
// reagent an earlier band of the plan crafted (it comes from that band's output); a material two bands share is added once. The windows
// chosen craft no intermediate from raw materials beyond that. make offline-benchmark runs it; WOWC_BENCH_REPORT receives the tables.

const totalsSrc = "dataset recipe_reagents (reagent_item_id, count) times each band's crafts, recipes (creates_item_id)"

func TestOfflineBenchmarkTotalsFirstAid(t *testing.T)      { offlineTotals(t, "First Aid", 75) }
func TestOfflineBenchmarkTotalsCooking(t *testing.T)       { offlineTotals(t, "Cooking", 45) }
func TestOfflineBenchmarkTotalsAlchemy(t *testing.T)       { offlineTotals(t, "Alchemy", 75) }
func TestOfflineBenchmarkTotalsBlacksmithing(t *testing.T) { offlineTotals(t, "Blacksmithing", 65) }

func offlineTotals(t *testing.T, prof string, to int) {
	t.Helper()
	ref := snapshot()
	b := NewBenchmark(ref.ds, BenchOptions{Profession: prof, From: 1, To: to, Level: 60, Faction: "horde"})
	if len(b.Bands) == 0 {
		t.Fatalf("%s: no bands: %v", prof, b.Notes)
	}
	want := map[int64]int64{}
	made := map[int64]bool{}
	var recipes []string
	for _, x := range b.Bands {
		if x.Gap != "" {
			continue
		}
		recipes = append(recipes, fmt.Sprintf("%s ×%d", x.Recipe, x.Crafts))
		for id, n := range ref.reagents(x.SpellID) {
			if !made[id] {
				want[id] += n * int64(x.Crafts)
			}
		}
		if row, ok := ref.of(prof)[x.SpellID]; ok {
			made[asI64(row["creates_item_id"])] = true
		}
	}
	have := map[int64]int{}
	sum := 0
	for _, m := range b.Materials {
		have[m.ID] += m.Count
		sum += m.Count
	}
	var gotParts, wantParts, bad []string
	for id, n := range want {
		wantParts = append(wantParts, fmt.Sprintf("%d %s", n, itemName(ref.ds, id)))
		if int64(have[id]) != n {
			bad = append(bad, fmt.Sprintf("%s: %d, dataset %d", itemName(ref.ds, id), have[id], n))
		}
	}
	for _, m := range b.Materials {
		gotParts = append(gotParts, fmt.Sprintf("%d %s", m.Count, m.Item))
		if _, ok := want[m.ID]; !ok {
			bad = append(bad, fmt.Sprintf("%s is not a reagent of the plan", m.Item))
		}
	}
	sort.Strings(gotParts)
	sort.Strings(wantParts)
	var total int64
	for _, n := range want {
		total += n
	}
	label := fmt.Sprintf("%s 1→%d (%s)", prof, to, strings.Join(recipes, ", "))
	res := "match"
	if len(bad) > 0 {
		res = "differ: " + strings.Join(bad, "; ")
	}
	out := []benchRow{{label + ": the total of each material", strings.Join(gotParts, ", "), strings.Join(wantParts, ", "), totalsSrc, res}}
	res = resultIf(int64(sum) == total && int64(b.Efficiency.TotalMaterials) == total, "the plan's count of all materials is not the sum of the recipes'")
	out = append(out, benchRow{label + ": the count of all materials", fmt.Sprint(b.Efficiency.TotalMaterials), fmt.Sprint(total), totalsSrc, res})
	finishOffline(t, "totals-"+strings.ReplaceAll(strings.ToLower(prof), " ", "-"), "Offline material totals: "+prof, out)
}

package professions

import (
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/store"
)

// clothData is a Tailoring world where the cheapest recipe by far is a Silk Cloth vest that would take
// 700 Silk Cloth to 140; a Runecloth hat and a pair of boots that take Mithril Bars are the other ways.
func clothData(t *testing.T, hat, boots bool) store.DataStore {
	recipes := []pRecipeSrc{rs(400, "Tailoring", "Silk Vest", i32(20), 1, "trainer", 150, 170, "trainer")}
	reagents := []reagentRow{{400, 1, 5}}
	if hat {
		recipes = append(recipes, rs(401, "Tailoring", "Runecloth Hat", i32(21), 1, "trainer", 150, 170, "trainer"))
		reagents = append(reagents, reagentRow{401, 3, 4})
	}
	if boots {
		recipes = append(recipes, rs(402, "Tailoring", "Bar Boots", i32(22), 1, "trainer", 150, 170, "trainer"))
		recipes = append(recipes, rs(200, "Mining", "Smelt Mithril", i32(2), 175, "trainer", 200, 220, "trainer"))
		reagents = append(reagents, reagentRow{402, 2, 2}, reagentRow{402, 1, 1}, reagentRow{200, 4, 1})
	}
	return store.New(store.ReaderSource{
		"items": table(t, []pItem{{1, "Silk Cloth", 1, 1, 7}, {2, "Mithril Bar", 1, 1, 7}, {3, "Runecloth", 1, 1, 7}, {4, "Mithril Ore", 1, 1, 7},
			{20, "Silk Vest", 1, 1, 4}, {21, "Runecloth Hat", 1, 1, 4}, {22, "Bar Boots", 1, 1, 4}}),
		"recipes":          table(t, recipes),
		"recipe_reagents":  table(t, reagents),
		"profession_notes": table(t, rankTiers()),
	})
}

// largest is the most units of any one material a benchmark asks for.
func largest(b Benchmark) (string, int) {
	item, n := "", 0
	for _, m := range b.Materials {
		if m.Count > n {
			item, n = m.Item, m.Count
		}
	}
	return item, n
}

func TestCheapestRecipeStaysUnderUnitCap(t *testing.T) {
	b := NewBenchmark(clothData(t, true, false), BenchOptions{Profession: "Tailoring", To: 140})
	if b.To != 140 || b.Bands[len(b.Bands)-1].To != 140 {
		t.Fatalf("path does not reach 140:\n%s", RenderBenchmark(b))
	}
	if item, n := largest(b); n > maxUnitsPerMaterial {
		t.Errorf("%d × %s, over the %d cap:\n%s", n, item, maxUnitsPerMaterial, RenderBenchmark(b))
	}
	recipes := map[string]bool{}
	for _, x := range b.Bands {
		recipes[x.Recipe] = true
	}
	if !recipes["Silk Vest"] || !recipes["Runecloth Hat"] {
		t.Errorf("want the vest cut short and the hat after it, got %v", recipes)
	}
}

func TestNoRecipeUnderCapKeepsCheapestAndSaysSo(t *testing.T) {
	b := NewBenchmark(clothData(t, false, false), BenchOptions{Profession: "Tailoring", To: 140})
	if _, n := largest(b); n <= maxUnitsPerMaterial {
		t.Fatalf("only one recipe, but the plan stays under the cap:\n%s", RenderBenchmark(b))
	}
	found := false
	for _, n := range b.Notes {
		found = found || strings.Contains(n, "Silk Cloth")
	}
	if !found {
		t.Errorf("the shortfall is not stated: %v", b.Notes)
	}
}

func TestNonMinerBuysBarsInsteadOfCloth(t *testing.T) {
	b := NewBenchmark(clothData(t, false, true), BenchOptions{Profession: "Tailoring", To: 140})
	for _, x := range b.Bands {
		if x.Recipe != "Bar Boots" {
			t.Errorf("band %d→%d is %q: a few bought bars should beat hundreds of cloth", x.From, x.To, x.Recipe)
		}
	}
	if item, n := largest(b); n > maxUnitsPerMaterial {
		t.Errorf("%d × %s over the cap", n, item)
	}
	for _, m := range b.Materials {
		if m.Item == "Mithril Bar" && m.Rare {
			t.Error("bought bars are scored as rare")
		}
	}
}

package professions

import (
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/store"
)

type pName struct {
	Norm  string `parquet:"norm"`
	Name  string `parquet:"name"`
	Table string `parquet:"table"`
	ID    int64  `parquet:"id"`
}

// smeltData is a Blacksmithing and Mining world with a two-input smelt (Bronze Bar = Copper Bar +
// Tin Bar), a plate made as a step on the way (Silver Plate, from Bronze Bars) and a gem nobody sells.
func smeltData(t *testing.T) store.DataStore {
	return store.New(store.ReaderSource{
		"items": table(t, []pItem{
			{1, "Copper Ore", 1, 1, 7}, {2, "Copper Bar", 1, 1, 7}, {10, "Tin Bar", 1, 1, 7}, {11, "Bronze Bar", 1, 1, 7},
			{12, "Silver Plate", 1, 1, 4}, {13, "Odd Gem", 1, 1, 7}, {14, "Plated Sword", 1, 1, 2}, {15, "Gem Ring", 1, 1, 4},
		}),
		"recipes": table(t, []pRecipeSrc{
			rs(200, "Mining", "Smelt Copper", i32(2), 1, "trainer", 25, 47, "trainer"),
			rs(201, "Mining", "Smelt Bronze", i32(11), 15, "trainer", 40, 65, "trainer"),
			rs(106, "Blacksmithing", "Silver Plate", i32(12), 1, "trainer", 40, 70, "trainer"),
			rs(107, "Blacksmithing", "Plated Sword", i32(14), 1, "trainer", 40, 70, "trainer"),
		}),
		"recipe_reagents": table(t, []reagentRow{
			{200, 1, 1}, {201, 2, 1}, {201, 10, 1}, {106, 11, 6}, {107, 12, 1}, {107, 11, 2},
		}),
		"vendor_sources":   table(t, []pVendor{{1, 10, 25}}),
		"gathering_yields": table(t, []pYield{{50, 1}}),
		"gathering_nodes":  table(t, []pNode{{50, "Mining", "Copper Vein", "Elwynn Forest", 1}}),
		"names":            table(t, []pName{{"fancy ring", "Fancy Ring", "items", 78}}),
		"profession_notes": table(t, rankTiers()),
	})
}

func findMake(b Benchmark, item string) *BenchMake {
	for _, x := range b.Bands {
		for _, m := range x.Materials {
			if m.Item == item {
				return m.Make
			}
		}
	}
	return nil
}

func TestMultiInputSmeltExpands(t *testing.T) {
	b := NewBenchmark(smeltData(t), BenchOptions{Profession: "Blacksmithing", From: 20, To: 60, Gather: "mining"})
	bar := findMake(b, "Bronze Bar")
	if bar == nil || bar.Verb != "smelt" || bar.Profession != "Mining" || bar.LearnSkill == nil || *bar.LearnSkill != 15 {
		t.Fatalf("bronze not smelted:\n%s", RenderBenchmark(b))
	}
	var copper *BenchMake
	var tin string
	for _, m := range bar.Materials {
		if m.Item == "Copper Bar" {
			copper = m.Make
		}
		if m.Item == "Tin Bar" {
			tin = m.Item
		}
	}
	if copper == nil || copper.Verb != "smelt" || copper.Materials[0].Item != "Copper Ore" || tin == "" {
		t.Errorf("bronze inputs not expanded: %+v", bar.Materials)
	}
	if text := RenderBenchmark(b); !strings.Contains(text, "needs Mining skill") {
		t.Errorf("Mining skill not stated:\n%s", text)
	}
}

func TestBarsBoughtWithoutMining(t *testing.T) {
	b := NewBenchmark(smeltData(t), BenchOptions{Profession: "Blacksmithing", From: 20, To: 60})
	if findMake(b, "Bronze Bar") != nil {
		t.Errorf("bar smelted without Mining:\n%s", RenderBenchmark(b))
	}
	for _, m := range b.Materials {
		if m.Item == "Bronze Bar" && (m.Rare || m.Kind != "auction") {
			t.Errorf("bought bar is %+v, want a plain Auction House buy", m)
		}
	}
	if b.Efficiency.AuctionUnits == 0 {
		t.Error("no bought units counted")
	}
}

func TestIntermediateGearCrafted(t *testing.T) {
	b := NewBenchmark(smeltData(t), BenchOptions{Profession: "Blacksmithing", From: 20, To: 60, Gather: "mining"})
	plate := findMake(b, "Silver Plate")
	if plate == nil || plate.Verb != "craft" || plate.Profession != "Blacksmithing" || plate.ExpectedSkillUps <= 0 {
		t.Fatalf("plate not crafted with skill-ups:\n%s", RenderBenchmark(b))
	}
}

func TestRareScoredPerUnit(t *testing.T) {
	score := func(n int) int64 {
		ds := store.New(store.ReaderSource{
			"items":            table(t, []pItem{{13, "Odd Gem", 1, 1, 7}, {15, "Gem Ring", 1, 1, 4}}),
			"recipes":          table(t, []pRecipeSrc{rs(300, "Blacksmithing", "Gem Ring", i32(15), 1, "trainer", 500, 600, "trainer")}),
			"recipe_reagents":  table(t, []reagentRow{{300, 13, int16(n)}}),
			"profession_notes": table(t, rankTiers()),
		})
		b := NewBenchmark(ds, BenchOptions{Profession: "Blacksmithing", From: 1, To: 10})
		if b.Efficiency.RareMaterials != 1 {
			t.Fatalf("rare types %d:\n%s", b.Efficiency.RareMaterials, RenderBenchmark(b))
		}
		return b.Efficiency.Score
	}
	one, three := score(1), score(3)
	if three <= one+2*scoreRareUnit {
		t.Errorf("three gems per craft scored %d against %d for one: rare units are not counted one by one", three, one)
	}
}

func TestUnnamedItemFallback(t *testing.T) {
	ds := smeltData(t)
	p := &planner{ds: ds, names: map[int64]string{}}
	if got := p.name(78); got != "Fancy Ring" {
		t.Errorf("name from the names table = %q", got)
	}
	if got := p.name(77); got != "unnamed item 77" {
		t.Errorf("fallback = %q", got)
	}
	if got := p.name(13); got != "Odd Gem" {
		t.Errorf("name from items = %q", got)
	}
}

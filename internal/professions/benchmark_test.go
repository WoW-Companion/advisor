package professions

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/store"
)

type pLoot struct {
	Item  int32  `parquet:"item_id"`
	Kind  string `parquet:"source_kind"`
	ID    int64  `parquet:"source_id"`
	Kills int32  `parquet:"kills"`
	Drops int32  `parquet:"drops"`
}

type pNpc struct {
	ID   int64  `parquet:"id"`
	Name string `parquet:"name"`
}

func rankTiers() []pNote {
	return []pNote{tier("Apprentice", 1, 75, 5), tier("Journeyman", 75, 150, 10), tier("Expert", 150, 225, 20), tier("Artisan", 225, 300, 35)}
}

// benchData is a small synthetic dataset: Leatherworking from 1 to 300 and Enchanting with disenchant materials.
func benchData(t *testing.T) store.DataStore {
	return store.New(store.ReaderSource{
		"items": table(t, []pItem{
			{1, "Light Leather", 1, 1, 0}, {2, "Coarse Thread", 1, 1, 0}, {3, "Medium Leather", 1, 1, 0}, {4, "Heavy Leather", 1, 1, 0},
			{5, "Rugged Leather", 1, 1, 0}, {6, "Rune Thread", 1, 1, 0}, {7, "Pattern: Dark Tunic", 1, 1, 0}, {8, "Core Hide", 1, 1, 0},
			{9, "Wolf Pelt", 1, 1, 0}, {20, "Strange Dust", 1, 1, 0}, {21, "Lesser Magic Essence", 1, 1, 0}, {22, "Runed Rod", 1, 1, 0},
		}),
		"recipes": table(t, []pRecipe{
			pr(1000, "Leatherworking", "Light Armor Kit", nil, 1, 30, 45, "trainer"),
			pr(1001, "Leatherworking", "Leather Boots", nil, 1, 55, 75, "trainer"),
			pr(1002, "Leatherworking", "Embossed Vest", nil, 40, 90, 110, "trainer"),
			pr(1003, "Leatherworking", "Fine Belt", nil, 75, 120, 140, "trainer"),
			pr(1004, "Leatherworking", "Dark Tunic", nil, 100, 145, 165, "item"),
			pr(1005, "Leatherworking", "Heavy Armor Kit", nil, 150, 185, 205, "trainer"),
			pr(1006, "Leatherworking", "Rugged Gloves", nil, 175, 210, 230, "trainer"),
			pr(1007, "Leatherworking", "Hide Cloak", nil, 225, 250, 270, "trainer"),
			pr(1008, "Leatherworking", "Core Cuirass", nil, 250, 280, 300, "trainer"),
			pr(2000, "Enchanting", "Runed Rod", i32(22), 1, 5, 20, "trainer"),
			pr(2001, "Enchanting", "Enchant Bracer", nil, 1, 60, 80, "trainer"),
			pr(2002, "Enchanting", "Enchant Shield", nil, 60, 100, 120, "trainer"),
		}),
		"recipe_reagents": table(t, []reagentRow{
			{1000, 1, 1}, {1001, 1, 2}, {1001, 2, 1}, {1002, 3, 4}, {1002, 2, 2}, {1003, 3, 6}, {1004, 3, 8}, {1004, 2, 2},
			{1005, 4, 5}, {1006, 5, 6}, {1006, 6, 2}, {1007, 5, 8}, {1007, 6, 1}, {1008, 8, 4}, {1008, 6, 3},
			{2000, 20, 2}, {2001, 20, 1}, {2002, 20, 2}, {2002, 21, 1},
		}),
		"vendor_sources":    table(t, []pVendor{{1, 1, 25}, {1, 2, 40}, {1, 3, 150}, {1, 6, 900}, {1, 7, 6000}}),
		"recipe_items":      table(t, []pRecipeItem{{7, 1004, 0, "Leatherworking"}}),
		"gathering_yields":  table(t, []pYield{{50, 4}, {51, 5}}),
		"gathering_nodes":   table(t, []pNode{{50, "Skinning", "Boar", "Elwynn Forest", 1}, {51, "Skinning", "Tiger", "Stranglethorn", 200}}),
		"loot_sources":      table(t, []pLoot{{8, "creature", 900, 100, 2}, {9, "creature", 901, 100, 40}}),
		"npcs":              table(t, []pNpc{{900, "Core Guardian"}, {901, "Timber Wolf"}}),
		"disenchant_yields": table(t, []pDE{{"weapon", "uncommon", 10, 20, "rated", 20}, {"armor", "uncommon", 10, 30, "rated", 21}}),
		"trainers":          table(t, []pTrainer{{1, "Tanner Tess", "Leatherworking", "Goldshire", 8}, {2, "Ench Eli", "Enchanting", "Darkshire", 4}}),
		"trainer_spells": table(t, []pTrainerSpell{
			{1, 1002, 200}, {1, 1003, 1500}, {1, 1005, 9000}, {1, 1006, 20000}, {1, 1007, 40000}, {1, 1008, 80000}, {2, 2002, 700},
		}),
		"profession_notes": table(t, rankTiers()),
	})
}

type pYield struct {
	Node int64 `parquet:"node_id"`
	Item int32 `parquet:"item_id"`
}

type pNode struct {
	Node  int64  `parquet:"node_id"`
	Prof  string `parquet:"profession"`
	Name  string `parquet:"name"`
	Zone  string `parquet:"zone_name"`
	Skill int16  `parquet:"skill_required"`
}

func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != got {
		t.Errorf("output differs from %s; rerun with UPDATE_GOLDEN=1 to review.\n--- got ---\n%s", path, got)
	}
}

func TestBenchmarkLeatherworkingGolden(t *testing.T) {
	b := NewBenchmark(benchData(t), BenchOptions{Profession: "leatherworking", Faction: "horde"})
	if b.From != 1 || b.To != 300 || b.Bands[len(b.Bands)-1].To != 300 {
		t.Fatalf("path %d→%d, %d bands", b.From, b.To, len(b.Bands))
	}
	golden(t, "benchmark_leatherworking.golden", RenderBenchmark(b))
}

func TestBenchmarkEnchantingGolden(t *testing.T) {
	b := NewBenchmark(benchData(t), BenchOptions{Profession: "Enchanting", To: 120, Level: 20})
	golden(t, "benchmark_enchanting.golden", RenderBenchmark(b))
}

func TestBenchmarkTotalsAndLevelCap(t *testing.T) {
	ds := benchData(t)
	b := NewBenchmark(ds, BenchOptions{Profession: "Leatherworking", Level: 12})
	if b.To != 150 || len(b.Notes) == 0 {
		t.Errorf("level 12 should stop at 150 with a note, got %d %v", b.To, b.Notes)
	}
	full := NewBenchmark(ds, BenchOptions{Profession: "Leatherworking"})
	sum, vendor, rare := 0, int64(0), 0
	for _, m := range full.Materials {
		sum += m.Count
		vendor += m.CostCopper
		if m.Rare {
			rare++
		}
	}
	e := full.Efficiency
	if e.TotalMaterials != sum || e.VendorCopper != vendor || e.RareMaterials != rare || e.DistinctMaterials != len(full.Materials) {
		t.Errorf("efficiency %+v disagrees with materials", e)
	}
	kinds := map[string]string{}
	for _, m := range full.Materials {
		kinds[m.Item] = m.Kind
	}
	if kinds["Light Leather"] != "vendor" || kinds["Heavy Leather"] != "gathering" || kinds["Core Hide"] != "rare" {
		t.Errorf("kinds %v", kinds)
	}
	if len(full.RecipeItems) != 1 || full.RecipeItems[0].Source != "vendor" || full.RecipeItems[0].Copper != 6000 {
		t.Errorf("recipe items %+v", full.RecipeItems)
	}
}

func TestBenchmarkDeterministicAndJSONShape(t *testing.T) {
	ds := benchData(t)
	a := NewBenchmark(ds, BenchOptions{Profession: "Leatherworking"})
	b := NewBenchmark(ds, BenchOptions{Profession: "Leatherworking"})
	if !reflect.DeepEqual(a, b) {
		t.Fatal("two runs differ")
	}
	raw, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	for k, kind := range map[string]string{"profession": "string", "level": "number", "from": "number", "to": "number", "model": "string",
		"bands": "array", "materials": "array", "recipe_items": "array", "tiers": "array", "efficiency": "object"} {
		if jsonKind(m[k]) != kind {
			t.Errorf("%s is %s, want %s", k, jsonKind(m[k]), kind)
		}
	}
	shape := func(v any, fields map[string]string) {
		o, _ := v.(map[string]any)
		for k, kind := range fields {
			if jsonKind(o[k]) != kind {
				t.Errorf("%s is %s, want %s", k, jsonKind(o[k]), kind)
			}
		}
	}
	shape(m["bands"].([]any)[0], map[string]string{"from": "number", "to": "number", "recipe": "string", "spell_id": "number", "crafts": "number",
		"orange_crafts": "number", "yellow_crafts": "number", "expected_skill_ups": "number", "materials": "array"})
	shape(m["materials"].([]any)[0], map[string]string{"item": "string", "item_id": "number", "count": "number", "kind": "string", "source": "string", "rare": "bool"})
	shape(m["tiers"].([]any)[0], map[string]string{"name": "string", "skill_min": "number", "skill_cap": "number", "min_level": "number",
		"trainer_copper": "number", "reachable": "bool"})
	shape(m["efficiency"], map[string]string{"total_materials": "number", "distinct_materials": "number", "vendor_copper": "number",
		"rare_materials": "number", "score": "number", "formula": "string"})
}

func jsonKind(v any) string {
	switch v.(type) {
	case string:
		return "string"
	case float64:
		return "number"
	case bool:
		return "bool"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	}
	return "missing"
}

func TestBenchmarkFishingByZone(t *testing.T) {
	ds := pathData(t)
	b := NewBenchmark(ds, BenchOptions{Profession: "fishing", Level: 20})
	text := RenderBenchmark(b)
	if len(b.Bands) == 0 || len(b.FishZones) != 2 || !strings.Contains(text, "Westfall: Raw Slitherskin Mackerel 50%") {
		t.Errorf("fishing plan:\n%s", text)
	}
}

package gear

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/benchsummary"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/stats"
	"github.com/WoW-Companion/advisor/internal/store"
)

// The offline stat benchmark: item stats and gear comparisons of the advisor set beside the dataset's item
// rows, with no network. testdata/advice_snapshot (scripts/advice-snapshot.sh) holds real rows of the
// published dataset. Each case states the dataset's value by hand (read from the items table's `stats`
// column) and the advisor's answer is computed live. make offline-benchmark runs it with the profession
// cases; WOWC_BENCH_REPORT receives the result table in Markdown.

const (
	snapshotDir = "../../testdata/advice_snapshot"
	itemsSource = "dataset items.stats (type, amount)"
)

func snapshot() store.DataStore { return store.NewDir(snapshotDir) }

type benchRow struct{ check, advisor, reference, source, result string }

func writeBenchTable(t *testing.T, title string, rows []benchRow) {
	path := os.Getenv("WOWC_BENCH_REPORT")
	if path == "" || len(rows) == 0 {
		return
	}
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

func finish(t *testing.T, area, title string, rows []benchRow) {
	t.Helper()
	results := make([]string, len(rows))
	for i, r := range rows {
		results[i] = r.result
		if strings.HasPrefix(r.result, "differ") {
			t.Errorf("%s: advisor %q, reference %q", r.check, r.advisor, r.reference)
		}
	}
	fmt.Println(benchsummary.AreaLine(area, results))
	writeBenchTable(t, title, rows)
}

// statWord names a stat type id as the advisor's stat table does.
func statWord(id int64) string {
	switch id {
	case 3:
		return "Agility"
	case 4:
		return "Strength"
	case 5:
		return "Intellect"
	case 6:
		return "Spirit"
	case 7:
		return "Stamina"
	case 45:
		return "Spell Power"
	}
	return fmt.Sprintf("stat %d", id)
}

func describe(m map[int64]int) string {
	if len(m) == 0 {
		return "no stat amounts"
	}
	ids := make([]int64, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	var parts []string
	for _, id := range ids {
		parts = append(parts, fmt.Sprintf("%s %d", statWord(id), m[id]))
	}
	return strings.Join(parts, ", ")
}

// itemStatCases are items with the amounts the dataset's items table holds for them.
var itemStatCases = []struct {
	id   int64
	name string
	want map[int64]int
}{
	{7065, "Green Silk Armor", map[int64]int{5: 13, 45: 9}},
	{7051, "Earthen Vest", map[int64]int{6: 10, 7: 6}},
	{9515, "Nether-lace Tunic", map[int64]int{7: 6, 5: 9}},
	{7334, "Efflorescent Robe", map[int64]int{7: 9, 5: 2}},
	{250533, "Hard Gold Cuirass", map[int64]int{45: 6, 4: 11}},
	{3750, "Ribbed Breastplate", map[int64]int{4: 5, 7: 5, 6: 5}},
	{7374, "Dusky Leather Armor", map[int64]int{3: 14}},
	{7914, "Barbaric Iron Breastplate", map[int64]int{4: 12}},
	{5770, "Robes of Arcana", map[int64]int{45: 8, 6: 7}},
	{2570, "Linen Cloak", nil},
	{810, "Hammer of the Northern Wind", nil},
}

func TestOfflineBenchmarkItemStats(t *testing.T) {
	ds := snapshot()
	var out []benchRow
	for _, c := range itemStatCases {
		row, err := ds.Get("items", c.id)
		if err != nil {
			t.Fatalf("%s is not in the snapshot: %v", c.name, err)
		}
		got := itemOf(row).Stats
		res := "match"
		if describe(got) != describe(c.want) {
			res = "differ"
		}
		out = append(out, benchRow{c.name + ": stat amounts", describe(got), describe(c.want), itemsSource, res})
	}
	finish(t, "stats-items", "Offline item stats", out)
}

// A gear case: a character, what they wear in the slot, and the item the dataset's stats say is the best
// upgrade, with the reason in the dataset's numbers.
type gearCase struct {
	name, class, spec, role string
	level                   int
	worn                    int // the chest item worn, 0 for none
	slot                    string
	first                   string // the best upgrade, by the reasoning in why
	notFirst                string // an item that must rank below first or not at all
	why                     string
}

var gearCases = []gearCase{
	{"Mage in Azure Silk Vest", "MAGE", "Frost", "damage", 33, 4324, "chest", "Green Silk Armor", "Earthen Vest",
		"Intellect 13 and Spell Power 9 are a mage's stats; Earthen Vest has Spirit and Stamina only"},
	{"Mage in Efflorescent Robe", "MAGE", "Fire", "damage", 32, 7334, "chest", "Green Silk Armor", "Earthen Vest",
		"Intellect 13 and Spell Power 9 against Spirit 10, Stamina 6"},
	{"Priest (shadow) in Azure Silk Vest", "PRIEST", "Shadow", "damage", 33, 4324, "chest", "Green Silk Armor", "Earthen Vest",
		"Intellect 13 and Spell Power 9 against Spirit 10, Stamina 6"},
	{"Rogue in Ribbed Breastplate", "ROGUE", "Combat", "damage", 33, 3750, "chest", "Dusky Leather Armor", "Green Whelp Armor",
		"Agility 14 against Stamina 11"},
	{"Warrior (fury) in Ribbed Breastplate", "WARRIOR", "Fury", "damage", 33, 3750, "chest", "Barbaric Iron Breastplate", "Hard Gold Cuirass",
		"Strength 12 against Strength 11 with 6 Spell Power the class cannot use"},
}

func TestOfflineBenchmarkGearComparison(t *testing.T) {
	ds := snapshot()
	var out []benchRow
	for _, g := range gearCases {
		lvl, id := g.level, g.worn
		c := character.Character{Name: g.name, Class: g.class, Level: &lvl,
			Profile:   &character.Profile{Role: g.role, Focus: "leveling", Spec: g.spec},
			Equipment: map[string]character.EquippedItem{g.slot: {Slot: g.slot, ItemID: &id}}}
		res, err := Find(c, ds, "", Options{Slot: g.slot, NoSources: true})
		if err != nil {
			t.Fatal(err)
		}
		cands := res.Slots[g.slot]
		var names []string
		for _, cd := range cands {
			names = append(names, cd.Item.Name)
		}
		got := "none"
		if len(cands) > 0 {
			got = cands[0].Item.Name
		}
		result := "match"
		if got != g.first {
			result = "differ"
		}
		out = append(out, benchRow{g.name + ": best chest upgrade", got + " (ranked: " + strings.Join(names, ", ") + ")", g.first + ": " + g.why, itemsSource, result})
		// a second row: the item the dataset's stats rank below must not come first
		result = "match"
		for i, n := range names {
			if n == g.notFirst && i == 0 {
				result = "differ"
			}
		}
		out = append(out, benchRow{g.name + ": " + g.notFirst + " is not first", got, "any item but " + g.notFirst, itemsSource, result})
	}
	finish(t, "stats-gear", "Offline gear comparison", out)
}

// rankCases: two items of one slot and the order the dataset's amounts put them in for a build.
var rankCases = []struct {
	name, class, spec string
	level             int
	before, after     string
	why               string
}{
	{"Mage at 28, empty chest", "MAGE", "Frost", 28, "Azure Silk Vest", "Efflorescent Robe",
		"Intellect 9 outweighs Stamina 9 with Intellect 2, though the robe is two item levels higher"},
	{"Mage at 28, empty chest", "MAGE", "Frost", 28, "Robes of Arcana", "Efflorescent Robe",
		"Spell Power 8 and Spirit 7 against Stamina 9 and Intellect 2"},
}

func TestOfflineBenchmarkItemOrder(t *testing.T) {
	ds := snapshot()
	var out []benchRow
	for _, g := range rankCases {
		lvl := g.level
		c := character.Character{Name: g.name, Class: g.class, Level: &lvl, Profile: &character.Profile{Role: "damage", Focus: "leveling", Spec: g.spec}}
		res, err := Find(c, ds, "", Options{Slot: "chest", NoSources: true})
		if err != nil {
			t.Fatal(err)
		}
		pos := map[string]int{}
		var names []string
		for i, cd := range res.Slots["chest"] {
			pos[cd.Item.Name] = i + 1
			names = append(names, cd.Item.Name)
		}
		result := "match"
		if pos[g.before] == 0 || (pos[g.after] != 0 && pos[g.before] > pos[g.after]) {
			result = "differ"
		}
		out = append(out, benchRow{fmt.Sprintf("%s: %s ahead of %s", g.name, g.before, g.after), strings.Join(names, ", "), g.before + " first: " + g.why, itemsSource, result})
	}
	finish(t, "stats-order", "Offline gear order by stat amounts", out)
}

// totalsCases: the gear-only stat totals the stat check reads, set beside the sum of the worn items' amounts.
func TestOfflineBenchmarkWornTotals(t *testing.T) {
	ds := snapshot()
	lvl := 33
	chest, id := "chest", 7065 // Green Silk Armor: Intellect 13, Spell Power 9
	c := character.Character{Name: "Totals", Class: "MAGE", Level: &lvl, Profile: &character.Profile{Role: "damage", Focus: "leveling", Spec: "Frost"},
		Equipment: map[string]character.EquippedItem{chest: {Slot: chest, ItemID: &id, Name: "Green Silk Armor"}}}
	id2 := 7051 // Earthen Vest: Spirit 10, Stamina 6
	c.Equipment["legs"] = character.EquippedItem{Slot: "legs", ItemID: &id2, Name: "Earthen Vest"}
	r := stats.Check(c, ds, "")
	var out []benchRow
	for _, w := range []struct {
		stat string
		want float64
	}{{stats.Intellect, 13}, {stats.Spirit, 10}, {stats.Stamina, 6}, {stats.Strength, 0}} {
		res := "match"
		if r.Totals[w.stat] != w.want {
			res = "differ"
		}
		out = append(out, benchRow{"Mage in Green Silk Armor and Earthen Vest: " + w.stat + " from gear", fmt.Sprint(r.Totals[w.stat]), fmt.Sprint(w.want), itemsSource, res})
	}
	res := "match"
	if r.Source != stats.SourceGear {
		res = "differ"
	}
	out = append(out, benchRow{"Mage worn totals: source", r.Source, stats.SourceGear, "no recorded totals: the dataset's items", res})
	finish(t, "stats-totals", "Offline worn stat totals", out)
}

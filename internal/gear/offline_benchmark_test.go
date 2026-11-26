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
	case 41:
		return "Spell Healing"
	case 42:
		return "Spell Damage"
	case 50:
		return "Extra Armor"
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

// armorCases are items with the stats and armor the dataset's items table holds for them: DATA-042's stat
// amounts, and armor computed from item level, armor type, quality and slot (null for no armor).
var armorCases = []struct {
	id    int64
	name  string
	want  map[int64]int
	armor int
}{
	{252434, "Defender's Leather Armor", map[int64]int{3: 3, 7: 5, 4: 7, 50: 33}, 85},
	{252493, "Wisdom's Leather Armor", map[int64]int{5: 5, 6: 3, 7: 3, 41: 15, 42: 5}, 85},
	{7930, "Heavy Mithril Breastplate", map[int64]int{7: 15, 50: 90}, 446},
	{4820, "Guardian Buckler", map[int64]int{7: 3, 6: 3}, 495},
	{250487, "Sterling Silver Breastplate", map[int64]int{45: 7, 7: 6, 4: 7}, 198},
	{4324, "Azure Silk Vest", map[int64]int{5: 9, 87: 7}, 43},
	{2568, "Brown Linen Vest", nil, 15},
	{14158, "Pagan Vest", nil, 40},
	{1193, "Banded Buckler", nil, 336},
	{3750, "Ribbed Breastplate", map[int64]int{4: 5, 7: 5, 6: 5}, 94},
}

func TestOfflineBenchmarkItemArmor(t *testing.T) {
	ds := snapshot()
	var out []benchRow
	for _, c := range armorCases {
		row, err := ds.Get("items", c.id)
		if err != nil {
			t.Fatalf("%s is not in the snapshot: %v", c.name, err)
		}
		it := itemOf(row)
		res := "match"
		if describe(it.Stats) != describe(c.want) {
			res = "differ"
		}
		out = append(out, benchRow{c.name + ": stat amounts", describe(it.Stats), describe(c.want), itemsSource, res})
		res = "match"
		if it.Armor != c.armor {
			res = "differ"
		}
		out = append(out, benchRow{c.name + ": armor", fmt.Sprint(it.Armor), fmt.Sprint(c.armor), "dataset items.armor", res})
	}
	finish(t, "stats-armor", "Offline item armor and older stats", out)
}

func firstOf(t *testing.T, ds store.DataStore, c character.Character, slot string) (string, []string) {
	t.Helper()
	res, err := Find(c, ds, "", Options{Slot: slot, NoSources: true})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, cd := range res.Slots[slot] {
		names = append(names, cd.Item.Name)
	}
	if len(names) == 0 {
		return "none", nil
	}
	return names[0], names
}

func indexOf(names []string, n string) int {
	for i, x := range names {
		if x == n {
			return i
		}
	}
	return -1
}

// TestOfflineBenchmarkOlderStats: a healer's bonus healing, a caster's bonus damage and a tank's extra armor count as
// the stat they stand for; a stat the build cannot use does not.
func TestOfflineBenchmarkOlderStats(t *testing.T) {
	ds := snapshot()
	var out []benchRow
	for _, g := range []struct {
		name, class, spec, role string
		level                   int
		ahead, behind, why      string
	}{
		{"Holy priest, empty chest", "PRIEST", "Holy", "healer", 15, "Filigreed Pristine Gown", "Filigreed Flame Gown",
			"Spell Healing 13 is the healer's stat; the Flame Gown's Fire power (stat 85) is not"},
		{"Feral druid tank, empty chest", "DRUID", "Feral Combat", "tank", 15, "Defender's Leather Armor", "Trapper's Leather Armor",
			"Extra Armor 33 and Strength 7 serve a tank; Trapper's has Intellect and Spirit"},
	} {
		lvl := g.level
		c := character.Character{Name: g.name, Class: g.class, Level: &lvl, Profile: &character.Profile{Role: g.role, Focus: "leveling", Spec: g.spec}}
		got, names := firstOf(t, ds, c, "chest")
		res := "match"
		if indexOf(names, g.ahead) < 0 || indexOf(names, g.behind) >= 0 && indexOf(names, g.ahead) > indexOf(names, g.behind) {
			res = "differ"
		}
		out = append(out, benchRow{g.name + ": " + g.ahead + " ahead of " + g.behind, fmt.Sprintf("%s first; %s at %d, %s at %d", got, g.ahead, indexOf(names, g.ahead)+1, g.behind, indexOf(names, g.behind)+1),
			g.ahead + " ahead: " + g.why, itemsSource, res})
	}
	finish(t, "stats-older", "Offline older stats in gear comparisons", out)
}

// TestOfflineBenchmarkCapturedStats: the same worn and bag items weighed from the dataset's amounts and from the
// addon's captured stats come to the same answer; armor counts on both sides when only one side is captured.
func TestOfflineBenchmarkCapturedStats(t *testing.T) {
	ds := snapshot()
	var out []benchRow
	lvl := 38
	worn, bag := 250533, 250487 // Hard Gold Cuirass (armor 210), Sterling Silver Breastplate (armor 198, a lower item level)
	mk := func(stats map[string]float64) character.Character {
		c := character.Character{Name: "Tank", Class: "WARRIOR", Level: &lvl, Profile: &character.Profile{Role: "tank", Focus: "leveling", Spec: "Protection"},
			Equipment: map[string]character.EquippedItem{"chest": {Slot: "chest", ItemID: &worn}},
			Bags:      []character.ItemStack{{ItemID: bag, Count: 1, Name: "Sterling Silver Breastplate", Stats: stats}}}
		return c
	}
	for _, c := range []struct {
		name  string
		stats map[string]float64
	}{
		{"bag item with captured stats (armor 198)", map[string]float64{"str": 7, "sta": 6, "armor": 198}},
		{"bag item without captured stats", nil},
	} {
		got := BagUpgrades(mk(c.stats), ds, "Protection")
		res := "match"
		if len(got) != 0 {
			res = "differ"
		}
		var names []string
		for _, u := range got {
			names = append(names, u.Name)
		}
		answer := "no upgrade"
		if len(names) > 0 {
			answer = "upgrade: " + strings.Join(names, ", ")
		}
		out = append(out, benchRow{"Protection warrior, Hard Gold Cuirass worn, " + c.name, answer,
			"no upgrade: armor 210 worn against 198 in the bag, strength 11 against 7, stamina 0 against 6", "dataset items.armor, items.stats; the capture's stats", res})
	}
	// the capture's total beats the dataset's guess for the same item: a captured item with more armor than worn is an upgrade
	got := BagUpgrades(mk(map[string]float64{"str": 11, "sta": 9, "armor": 260}), ds, "Protection")
	res := "match"
	if len(got) != 1 {
		res = "differ"
	}
	out = append(out, benchRow{"Protection warrior, bag item captured with armor 260 and stamina 9", fmt.Sprintf("%d upgrades", len(got)), "1 upgrade: armor 260 against 210, stamina 9 against 0", "the capture's stats against dataset items.armor", res})
	finish(t, "stats-captured", "Offline gear comparison with and without captured stats", out)
}

// TestOfflineBenchmarkUnknownStats: an item the dataset lists no stats for (status none or unknown) is
// ranked by the level needed to wear it, and the answer says so.
func TestOfflineBenchmarkUnknownStats(t *testing.T) {
	ds := snapshot()
	lvl := 25
	worn := 14158 // Pagan Vest: stats unknown
	c := character.Character{Name: "Mage", Class: "MAGE", Level: &lvl, Profile: &character.Profile{Role: "damage", Focus: "leveling", Spec: "Frost"},
		Equipment: map[string]character.EquippedItem{"chest": {Slot: "chest", ItemID: &worn}}}
	res, err := Find(c, ds, "", Options{Slot: "chest", NoSources: true})
	if err != nil {
		t.Fatal(err)
	}
	var out []benchRow
	basis := "none"
	for _, cd := range res.Slots["chest"] {
		basis = cd.Basis
		break
	}
	r := "match"
	if basis != BasisItemLevel {
		r = "differ"
	}
	out = append(out, benchRow{"Mage wearing Pagan Vest (stats unknown): basis of the ranking", basis, BasisItemLevel + ": the worn item's stats are unknown in the dataset", "dataset items.stats_status", r})
	finish(t, "stats-unknown", "Offline gear comparison when stats are unknown", out)
}

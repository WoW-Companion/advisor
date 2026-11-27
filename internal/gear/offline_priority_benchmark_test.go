package gear

import (
	"fmt"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/stats"
)

// The offline stat priority and item comparison benchmark: the stat order the advisor gives a class, spec and role, and the answer to
// "which of these two items is better for my class", beside the dataset's item rows (testdata/advice_snapshot: items stats,
// allowable_class_mask, armor type), with no network. The dataset holds no stat priority table, so the order of a spec is the
// public Classic convention stated by hand in each case (the stat the build turns into power, then the next two); the item side is
// read from the items table. An item a class cannot wear or cannot use is never offered, and what the dataset lacks (a class it has
// no rules for, a worn item it does not hold, stats it has not seen) is said, not guessed. make offline-benchmark runs it;
// WOWC_BENCH_REPORT receives the tables.

const (
	prioSrc = "game stat convention (public Classic guides: primary stat per spec); the dataset has no stat priority table"
	cmpSrc  = "dataset items (stats, item_level, armor type, allowable_class_mask)"
	noneSrc = "the dataset has no such rows"
)

// resultIf is "match", or "differ: " and what differs.
func resultIf(ok bool, bad string) string {
	if ok {
		return "match"
	}
	return "differ: " + bad
}

func charOf(class, spec, role string, level int, worn map[string]int) character.Character {
	lv := level
	c := character.Character{Name: "Bench", Class: class, Level: &lv, Profile: &character.Profile{Role: role, Focus: "leveling", Spec: spec}}
	for slot, id := range worn {
		i := id
		if c.Equipment == nil {
			c.Equipment = map[string]character.EquippedItem{}
		}
		c.Equipment[slot] = character.EquippedItem{Slot: slot, ItemID: &i}
	}
	return c
}

var prioCases = []struct {
	label, class, spec, role string
	order                    []string // the stats the guides put first, in order
}{
	{"Fury warrior", "WARRIOR", "Fury", "damage", []string{"strength", "agility", "stamina"}},
	{"Protection warrior", "WARRIOR", "Protection", "tank", []string{"stamina"}},
	{"Combat rogue", "ROGUE", "Combat", "damage", []string{"agility", "strength", "stamina"}},
	{"Beast Mastery hunter", "HUNTER", "Beast Mastery", "damage", []string{"agility", "stamina", "intellect"}},
	{"Frost mage", "MAGE", "Frost", "damage", []string{"intellect"}},
	{"Holy priest", "PRIEST", "Holy", "healer", []string{"intellect", "spirit", "stamina"}},
	{"Restoration druid", "DRUID", "Restoration", "healer", []string{"intellect", "spirit", "stamina"}},
	{"Holy paladin", "PALADIN", "Holy", "healer", []string{"intellect"}},
	{"Affliction warlock", "WARLOCK", "Affliction", "damage", []string{"intellect", "stamina"}},
}

func TestOfflineBenchmarkStatPriorityA(t *testing.T) { statPriority(t, "a", prioCases[:5]) }
func TestOfflineBenchmarkStatPriorityB(t *testing.T) { statPriority(t, "b", prioCases[5:]) }

// statPriority: the advisor's order starts with the stats the guides put first, and its lead line names the first.
func statPriority(t *testing.T, part string, cases []struct {
	label, class, spec, role string
	order                    []string
}) {
	var out []benchRow
	for _, tc := range cases {
		r := stats.Check(charOf(tc.class, tc.spec, tc.role, 40, nil), nil, "")
		got := strings.Join(r.Priority, " > ")
		n := min(len(tc.order), len(r.Priority))
		ok := len(r.Priority) >= len(tc.order) && strings.Join(r.Priority[:n], ",") == strings.Join(tc.order, ",")
		out = append(out, benchRow{tc.label + ": the stat order", got, strings.Join(tc.order, " > ") + " first", prioSrc, resultIf(ok, "another stat order than the guides'")})
		lead := ""
		if len(r.Insights) > 0 {
			lead = r.Insights[0]
		}
		want := fmt.Sprintf("%s: %s first", tc.spec, stats.Name(tc.order[0]))
		out = append(out, benchRow{tc.label + ": the lead line", lead, want, prioSrc, resultIf(strings.HasPrefix(lead, want), "the lead line names another first stat")})
	}
	finish(t, "stats-priority-"+part, "Offline stat priority ("+part+")", out)
}

// TestOfflineBenchmarkStatCheckGear: the stat check reads the worn items' amounts from the dataset and calls out the stat a class
// barely uses.
func TestOfflineBenchmarkStatCheckGear(t *testing.T) {
	ds := snapshot()
	var out []benchRow
	for _, tc := range []struct {
		label, class, spec, role string
		worn                     map[string]int
		stat                     string // the stat the class wastes
		item                     string
		statID                   int64
	}{
		{"Fury warrior in Earthen Vest (Spirit 10, Stamina 6)", "WARRIOR", "Fury", "damage", map[string]int{"chest": 7051}, "Spirit", "Earthen Vest", 6},
		{"Frost mage in Dusky Leather Armor (Agility 14)", "MAGE", "Frost", "damage", map[string]int{"chest": 7374}, "Agility", "Dusky Leather Armor", 3},
	} {
		c := charOf(tc.class, tc.spec, tc.role, 40, tc.worn)
		eq := c.Equipment["chest"]
		eq.Name = tc.item
		c.Equipment["chest"] = eq
		r := stats.Check(c, ds, "")
		row, err := ds.Get("items", int64(tc.worn["chest"]))
		if err != nil {
			t.Fatal(err)
		}
		amount := itemOf(row).Stats[tc.statID]
		line := strings.Join(r.Insights, " ")
		ok := strings.Contains(line, tc.stat+", which a "+strings.ToLower(tc.class)+" barely uses") && strings.Contains(line, tc.item) && r.Source == stats.SourceGear
		out = append(out, benchRow{tc.label + ": the wasted stat", line, fmt.Sprintf("%s %d on %s is a stat a %s barely uses", tc.stat, amount, tc.item, strings.ToLower(tc.class)), cmpSrc + "; " + prioSrc, resultIf(ok, "the wasted stat or the item is not named")})
	}
	// a capture that names no worn item still counts it: "1 item gives Spirit", never "0 items"
	c := charOf("WARRIOR", "Fury", "damage", 40, map[string]int{"chest": 7051})
	line := strings.Join(stats.Check(c, ds, "").Insights, " ")
	out = append(out, benchRow{"Fury warrior in Earthen Vest, the capture gives no item name: the wasted stat", line, "one item carries the Spirit, so \"1 item gives Spirit\"", cmpSrc,
		resultIf(strings.Contains(line, "1 item gives Spirit"), "the item is not counted when it has no name")})
	finish(t, "stats-check-gear", "Offline stat check on worn items", out)
}

// ---- two items, one class ----

// A comparison: the character wears one item (or none) and the dataset has another; ranked names the order the dataset's amounts put
// them in for the class, and unusable is an item the class may not wear.
type cmpCase struct {
	label, class, spec, role string
	level                    int
	worn                     int // 0 for an empty chest
	better, worse            string
	why                      string
}

var cmpCases = []cmpCase{
	{"Mage in Earthen Vest, Green Silk Armor in the bags", "MAGE", "Frost", "damage", 33, 7051, "Green Silk Armor", "Earthen Vest",
		"Intellect 13 and Spell Power 9 against Spirit 10 and Stamina 6, though Earthen Vest is the higher item level (34 against 33)"},
	{"Rogue in Green Whelp Armor, Dusky Leather Armor", "ROGUE", "Combat", "damage", 33, 7375, "Dusky Leather Armor", "Green Whelp Armor",
		"Agility 14 against Stamina 11, at the same item level 35"},
	{"Druid (feral) in Ribbed Breastplate", "DRUID", "Feral Combat", "damage", 33, 3750, "Dusky Leather Armor", "Green Whelp Armor",
		"Agility 14 against Stamina 11 on the leather the druid wears"},
	{"Warrior (arms) in Sterling Silver Breastplate", "WARRIOR", "Arms", "damage", 33, 250487, "Barbaric Iron Breastplate", "Hard Gold Cuirass",
		"Strength 12 against Strength 11 with 6 Spell Power, which does nothing for a warrior"},
	{"Warlock, empty chest", "WARLOCK", "Affliction", "damage", 33, 0, "Green Silk Armor", "Nether-lace Tunic",
		"Intellect 13 and Spell Power 9; Nether-lace Tunic is a mage-only item (allowable_class_mask 128) the warlock cannot use"},
	{"Priest (holy) in Azure Silk Vest", "PRIEST", "Holy", "healer", 33, 4324, "Green Silk Armor", "Astral Knot Blouse",
		"Intellect 13 and Spell Power 9; Astral Knot Blouse is a mage-only item (allowable_class_mask 128) the priest cannot use"},
}

func TestOfflineBenchmarkItemComparison(t *testing.T) {
	ds := snapshot()
	var out []benchRow
	for _, tc := range cmpCases {
		var worn map[string]int
		if tc.worn != 0 {
			worn = map[string]int{"chest": tc.worn}
		}
		first, names := firstOf(t, ds, charOf(tc.class, tc.spec, tc.role, tc.level, worn), "chest")
		list := strings.Join(names, ", ")
		out = append(out, benchRow{tc.label + ": the better item", first + " (ranked: " + list + ")", tc.better + ": " + tc.why, cmpSrc, resultIf(first == tc.better, "another item is first")})
		usable := indexOf(names, tc.worse) < 0 || indexOf(names, tc.worse) > indexOf(names, tc.better)
		if usable && tc.worse != "" && strings.Contains(tc.why, "allowable_class_mask") {
			usable = indexOf(names, tc.worse) < 0
		}
		out = append(out, benchRow{tc.label + ": " + tc.worse + " after it", list, tc.worse + " below " + tc.better + " or not offered", cmpSrc, resultIf(usable, tc.worse+" is offered before the better item or to a class that cannot use it")})
	}
	finish(t, "stats-compare", "Offline two-item stat comparison", out)
}

// TestOfflineBenchmarkItemComparisonKeepsWorn: the dataset's amounts decide between two items, not the item level alone: a worn
// item of a higher item level does not hide an item that carries the stats the class uses, and an item that carries fewer of them
// is not called an upgrade.
func TestOfflineBenchmarkItemComparisonKeepsWorn(t *testing.T) {
	ds := snapshot()
	var out []benchRow
	for _, tc := range []struct {
		label, class, spec, role string
		worn                     int
		never                    string
	}{
		{"Mage in Green Silk Armor", "MAGE", "Frost", "damage", 7065, "Earthen Vest"},
		{"Rogue in Dusky Leather Armor", "ROGUE", "Combat", "damage", 7374, "Green Whelp Armor"},
	} {
		_, names := firstOf(t, ds, charOf(tc.class, tc.spec, tc.role, 33, map[string]int{"chest": tc.worn}), "chest")
		out = append(out, benchRow{tc.label + ": a worse item is not an upgrade", strings.Join(names, ", "), tc.never + " is not offered", cmpSrc, resultIf(indexOf(names, tc.never) < 0, tc.never+" is offered over a better worn item")})
	}
	finish(t, "stats-compare-worn", "Offline two-item comparison: the worn item stays", out)
}

// ---- no answer in the dataset ----

// TestOfflineBenchmarkStatNoAnswer: what the dataset or the tables lack is said and nothing is guessed: a class with no stat rules, a
// character with no play style, stats the advisor cannot see, and a worn item the dataset does not hold.
func TestOfflineBenchmarkStatNoAnswer(t *testing.T) {
	ds := snapshot()
	var out []benchRow
	dk := stats.Check(charOf("DEATHKNIGHT", "Blood", "tank", 58, nil), ds, "")
	out = append(out, benchRow{"Death Knight: a stat priority", strings.Join(dk.Insights, " "), "no stat rules for the class: none is given", noneSrc,
		resultIf(len(dk.Priority) == 0 && strings.Contains(strings.Join(dk.Insights, " "), "no stat priorities for this class"), "a priority was given for a class with none")})
	bare := charOf("MAGE", "", "", 30, nil)
	bare.Profile = nil
	np := stats.Check(bare, ds, "")
	out = append(out, benchRow{"Mage with no play style: a stat priority", np.Needs, "no play style: it is asked for, no order is guessed", noneSrc,
		resultIf(np.Needs == stats.NeedsPlayStyle && len(np.Priority) == 0, "a priority was given without a play style")})
	naked := stats.Check(charOf("WARRIOR", "Fury", "damage", 30, nil), ds, "")
	out = append(out, benchRow{"Fury warrior with nothing worn: what to raise", strings.Join(naked.Insights, " "), "no gear and no totals: the advisor cannot tell", noneSrc,
		resultIf(strings.Contains(strings.Join(naked.Insights, " "), "can't see your stats yet") && len(naked.Totals) == 0, "a stat to raise was named without seeing any")})
	c := charOf("MAGE", "Frost", "damage", 33, nil)
	id, name := 99999999, "Mystery Robe"
	c.Equipment = map[string]character.EquippedItem{"chest": {Slot: "chest", ItemID: &id, Name: name}}
	res, err := Find(c, ds, "", Options{Slot: "chest", NoSources: true})
	if err != nil {
		t.Fatal(err)
	}
	text := Render(c, res)
	out = append(out, benchRow{"Mage in an item the dataset does not hold: upgrades for it", fmt.Sprintf("%d upgrades; %s", len(res.Slots["chest"]), res.Unknown["chest"]), "no row for the worn item and no item level: nothing is compared", noneSrc,
		resultIf(len(res.Slots["chest"]) == 0 && res.Unknown["chest"] == name && strings.Contains(text, "nothing is compared there"), "items were offered as better than an item nobody knows")})
	finish(t, "stats-no-answer", "Offline stat priority and item comparison: no answer in the dataset", out)
}

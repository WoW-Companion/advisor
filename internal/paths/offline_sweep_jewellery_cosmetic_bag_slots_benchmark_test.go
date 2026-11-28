package paths_test

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/ask"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/gear"
	"github.com/WoW-Companion/advisor/internal/inventory"
)

// The offline neck, ring, shirt, tabard and bag-slot sweep: what the gear finder offers a synthetic character for the neck and the two finger
// slots (all nine classes, damage, tank and healer builds, levels across 1 to 60, both factions, empty slots so every usable item is a
// candidate), that it never offers a shirt, a tabard or a bag, what the equip-from-bags list and the bag review say about necks, rings, shirts,
// tabards, bags, quivers and ammo pouches in the bags, and how everyday phrasings of these questions are answered, beside the dataset's item
// rows, with no network. testdata/usability_snapshot holds the real rows (necks, rings, shirts, tabards, plain and class bags, quivers and
// ammo pouches). The reference is written apart from the finder: the item's class mask, quality, bonding and required level. The dataset holds
// no unique-equipped flag, no bag size, no ammo speed and no race or faction column, so a ring worn in one finger slot is never offered for the
// other, a bag is never ranked and a tabard is never called faction-locked. make offline-benchmark runs it; WOWC_BENCH_REPORT receives the tables.

const (
	jcSrc      = "dataset items (slot 2 neck, 11 finger, 4 shirt, 19 tabard, 18 bag; allowable_class_mask, required_level, quality, bonding)"
	jcBagSrc   = "dataset items (slot, item_level, stats_status); game: one copy of a ring fills one finger slot, a shirt and a tabard carry no stats, a bag is a container"
	jcRevSrc   = "dataset items (class_id 1 bag, 11 quiver (subclass 2, arrows) or ammo pouch (subclass 3, bullets), slot 18, required_level, allowable_class_mask); game: Warcraft Wiki Quiver, Ammo Pouch"
	jcPhraseSr = "dataset items (slots 2, 4, 11, 18, 19; no unique flag, no bag size, no race column); Icy Veins WoW Forever gear guides (rings and necks are for every class)"
)

var jcLevels = []int{1, 9, 18, 28, 37, 47, 58}

// jcFindSlots: for every class, build, level and faction, the finder offers the neck the rows allow, the rows' rings between the two finger slots
// with no ring in both, and no shirt, tabard or bag.
func jcFindSlots(t *testing.T, area, title string, classes []string) {
	var out []benchRow
	ds := usabStore()
	for _, class := range classes {
		classID := grClassOf(class)
		for _, b := range wsBuilds[class] {
			for i, lv := range jcLevels {
				faction := []string{"Horde", "Alliance"}[i%2]
				level := lv
				c := character.Character{Name: "Sweep", Class: class, Faction: faction, Level: &level, Profile: &character.Profile{Role: b.role, Focus: "leveling", Spec: b.spec}}
				res, err := gear.Find(c, ds, "", gear.Options{NoSources: true})
				if err != nil {
					t.Fatal(err)
				}
				ref := wsReference(t, class, classID, level, faction, wsFree(b.role, b.spec))
				ids := func(slot string) map[string]bool {
					m := map[string]bool{}
					for _, cd := range res.Slots[slot] {
						m[fmt.Sprintf("%d", cd.Item.ID)] = true
					}
					return m
				}
				allowed := func(slot string) map[string]bool {
					m := map[string]bool{}
					for k := range ref {
						if strings.HasPrefix(k, slot+"/") {
							m[strings.TrimPrefix(k, slot+"/")] = true
						}
					}
					return m
				}
				diff := func(a, b map[string]bool) []string {
					var d []string
					for k := range a {
						if !b[k] {
							d = append(d, k)
						}
					}
					sort.Strings(d)
					return d
				}
				who := fmt.Sprintf("%s %s level %d, %s", class, b.spec, lv, faction)
				// the neck
				neck, wantNeck := ids("neck"), allowed("neck")
				bad := ""
				if e := diff(neck, wantNeck); len(e) > 0 {
					bad += fmt.Sprintf(" offers %v the rows do not allow;", e)
				}
				if m := diff(wantNeck, neck); len(m) > 0 {
					bad += fmt.Sprintf(" leaves out %v the rows allow;", m)
				}
				out = append(out, benchRow{who + ": what fits the neck", fmt.Sprintf("%d offered", len(neck)), fmt.Sprintf("%d allowed", len(wantNeck)), jcSrc, resultIf(bad == "", strings.TrimSuffix(strings.TrimSpace(bad), ";"))})
				// the two finger slots
				f1, f2, wantRing := ids("finger1"), ids("finger2"), allowed("finger")
				union := map[string]bool{}
				var both []string
				for k := range f1 {
					union[k] = true
					if f2[k] {
						both = append(both, k)
					}
				}
				for k := range f2 {
					union[k] = true
				}
				bad = ""
				if len(both) > 0 {
					bad += fmt.Sprintf(" offers %v for both finger slots;", both)
				}
				if e := diff(union, wantRing); len(e) > 0 {
					bad += fmt.Sprintf(" offers %v the rows do not allow;", e)
				}
				if m := diff(wantRing, union); len(m) > 0 {
					bad += fmt.Sprintf(" leaves out %v the rows allow;", m)
				}
				out = append(out, benchRow{who + ": what fits the two ring slots", fmt.Sprintf("%d in finger 1, %d in finger 2", len(f1), len(f2)), fmt.Sprintf("%d allowed, none in both", len(wantRing)), jcSrc, resultIf(bad == "", strings.TrimSuffix(strings.TrimSpace(bad), ";"))})
				// the shirt, the tabard and the bags carry no stats, so none is an upgrade
				for _, slot := range []string{"shirt", "tabard", "bag"} {
					n := len(res.Slots[slot])
					out = append(out, benchRow{who + ": what fits the " + slot + " slot", fmt.Sprintf("%d offered", n), "none: no stats or bag size to rank by", jcSrc, resultIf(n == 0, "an item offered for the "+slot+" slot")})
				}
			}
		}
	}
	finish(t, area, title, out)
}

func TestOfflineBenchmarkSweepJewelleryCosmeticBagSlotsWarrior(t *testing.T) {
	jcFindSlots(t, "gear-sweep-jewellery-slots-warrior", "Offline neck, ring, shirt, tabard and bag sweep: warrior", []string{"WARRIOR"})
}

func TestOfflineBenchmarkSweepJewelleryCosmeticBagSlotsPaladin(t *testing.T) {
	jcFindSlots(t, "gear-sweep-jewellery-slots-paladin", "Offline neck, ring, shirt, tabard and bag sweep: paladin", []string{"PALADIN"})
}

func TestOfflineBenchmarkSweepJewelleryCosmeticBagSlotsHunter(t *testing.T) {
	jcFindSlots(t, "gear-sweep-jewellery-slots-hunter", "Offline neck, ring, shirt, tabard and bag sweep: hunter", []string{"HUNTER"})
}

func TestOfflineBenchmarkSweepJewelleryCosmeticBagSlotsRogue(t *testing.T) {
	jcFindSlots(t, "gear-sweep-jewellery-slots-rogue", "Offline neck, ring, shirt, tabard and bag sweep: rogue", []string{"ROGUE"})
}

func TestOfflineBenchmarkSweepJewelleryCosmeticBagSlotsPriest(t *testing.T) {
	jcFindSlots(t, "gear-sweep-jewellery-slots-priest", "Offline neck, ring, shirt, tabard and bag sweep: priest", []string{"PRIEST"})
}

func TestOfflineBenchmarkSweepJewelleryCosmeticBagSlotsShaman(t *testing.T) {
	jcFindSlots(t, "gear-sweep-jewellery-slots-shaman", "Offline neck, ring, shirt, tabard and bag sweep: shaman", []string{"SHAMAN"})
}

func TestOfflineBenchmarkSweepJewelleryCosmeticBagSlotsMage(t *testing.T) {
	jcFindSlots(t, "gear-sweep-jewellery-slots-mage", "Offline neck, ring, shirt, tabard and bag sweep: mage", []string{"MAGE"})
}

func TestOfflineBenchmarkSweepJewelleryCosmeticBagSlotsWarlock(t *testing.T) {
	jcFindSlots(t, "gear-sweep-jewellery-slots-warlock", "Offline neck, ring, shirt, tabard and bag sweep: warlock", []string{"WARLOCK"})
}

func TestOfflineBenchmarkSweepJewelleryCosmeticBagSlotsDruid(t *testing.T) {
	jcFindSlots(t, "gear-sweep-jewellery-slots-druid", "Offline neck, ring, shirt, tabard and bag sweep: druid", []string{"DRUID"})
}

// TestOfflineBenchmarkSweepJewelleryAskedAlone: asking the finder for the neck, one finger slot, the shirt or the tabard alone gives the same neck
// and ring candidates as the whole review, and the shirt and tabard answer says none is an upgrade rather than "no upgrades found".
func TestOfflineBenchmarkSweepJewelleryAskedAloneWarriorRogue(t *testing.T) {
	jcAskedAlone(t, "gear-sweep-jewellery-slots-alone-a", []string{"WARRIOR", "ROGUE"})
}

func TestOfflineBenchmarkSweepJewelleryAskedAlonePriestShaman(t *testing.T) {
	jcAskedAlone(t, "gear-sweep-jewellery-slots-alone-b", []string{"PRIEST", "SHAMAN"})
}

func jcAskedAlone(t *testing.T, area string, classes []string) {
	var out []benchRow
	ds := usabStore()
	for _, class := range classes {
		for _, lv := range []int{20, 45} {
			level := lv
			c := character.Character{Name: "Sweep", Class: class, Faction: "Horde", Level: &level}
			all, err := gear.Find(c, ds, "", gear.Options{NoSources: true})
			if err != nil {
				t.Fatal(err)
			}
			for _, slot := range []string{"neck", "finger1", "finger2"} {
				one, err := gear.Find(c, ds, "", gear.Options{Slot: slot, NoSources: true})
				if err != nil {
					t.Fatal(err)
				}
				a, b := candNames(all.Slots[slot]), candNames(one.Slots[slot])
				out = append(out, benchRow{fmt.Sprintf("%s level %d: asked for the %s alone", class, lv, slot), b, a, jcSrc, resultIf(a == b, "a different list than the whole review gives")})
			}
			// the upgrade finder's default of three per slot: the two finger slots split the ranking, so no ring is in both and the rows' rings fill up to six places
			l1, err := gear.Find(c, ds, "", gear.Options{Slot: "finger1", Limit: 3, NoSources: true})
			if err != nil {
				t.Fatal(err)
			}
			l2, _ := gear.Find(c, ds, "", gear.Options{Slot: "finger2", Limit: 3, NoSources: true})
			n := len(strings.Split(candNames(all.Slots["finger1"]), ","))
			if candNames(all.Slots["finger1"]) == "" {
				n = 0
			}
			a1, a2 := candNames(l1.Slots["finger1"]), candNames(l2.Slots["finger2"])
			overlap := false
			for _, x := range strings.Split(a1, ",") {
				for _, y := range strings.Split(a2, ",") {
					overlap = overlap || x != "" && x == y
				}
			}
			placed := len(l1.Slots["finger1"]) + len(l2.Slots["finger2"])
			out = append(out, benchRow{fmt.Sprintf("%s level %d: the two ring slots with three offered per slot", class, lv), fmt.Sprintf("finger1 %s; finger2 %s", a1, a2), fmt.Sprintf("%d rings allowed: %d places, none in both", n, min(n, 6)), jcSrc, resultIf(!overlap && placed == min(n, 6), "a ring offered for both slots, or fewer places than the rings allow")})
			for _, slot := range []string{"shirt", "tabard"} {
				one, _ := gear.Find(c, ds, "", gear.Options{Slot: slot, NoSources: true})
				text := gear.Render(c, one)
				ok := strings.Contains(text, "has no stats") && !strings.Contains(text, "No upgrades found")
				out = append(out, benchRow{fmt.Sprintf("%s level %d: asked for the %s alone", class, lv, slot), firstLine(strings.TrimPrefix(text, "Gear upgrades for Sweep (level "+fmt.Sprint(lv)+" "+strings.ToLower(class)+"):\n")), "a shirt or tabard has no stats: none is an upgrade", jcSrc, resultIf(ok, "no upgrades found, as if the slot had a list to search")})
			}
		}
	}
	finish(t, area, "Offline neck, ring, shirt and tabard sweep: one slot asked alone ("+strings.ToLower(strings.Join(classes, " and "))+")", out)
}

func candNames(cs []gear.Candidate) string {
	var n []string
	for _, c := range cs {
		n = append(n, fmt.Sprintf("%d", c.Item.ID))
	}
	return strings.Join(n, ",")
}

var jcBagCases = []wsBagCase{
	// the neck
	{"a neck for the empty neck slot", "MAGE", 40, "Frost", "damage", nil, []string{"Choker of the High Shaman"}, map[string]string{"neck": "Choker of the High Shaman"}, "neck"},
	{"a neck whose item level suggests a required level above 30, at level 30", "MAGE", 30, "Frost", "damage", nil, []string{"Choker of the High Shaman"}, nil, "neck"},
	{"a neck needing level 30, at level 29", "PRIEST", 29, "Holy", "healer", nil, []string{"Pendant of Myzrael"}, nil, "neck"},
	{"a neck needing level 30, at level 30", "PRIEST", 30, "Holy", "healer", nil, []string{"Pendant of Myzrael"}, map[string]string{"neck": "Pendant of Myzrael"}, "neck"},
	{"the neck already worn, a copy in the bags", "WARLOCK", 40, "Affliction", "damage", map[string]string{"neck": "Pendant of Myzrael"}, []string{"Pendant of Myzrael"}, nil, "neck"},
	{"a neck for a class in plate (a neck has no armor type)", "WARRIOR", 35, "Protection", "tank", nil, []string{"Pendant of Myzrael"}, map[string]string{"neck": "Pendant of Myzrael"}, "neck"},
	{"a neck for a class in cloth (a neck has no armor type)", "WARLOCK", 35, "Affliction", "damage", nil, []string{"Pendant of Myzrael"}, map[string]string{"neck": "Pendant of Myzrael"}, "neck"},
	{"a test item the game never gave out (the dataset's 'Deprecated' neck)", "WARRIOR", 50, "Fury", "damage", nil, []string{"Deprecated Blood Totem"}, nil, "neck"},
	// the rings
	{"a ring for each empty finger slot", "ROGUE", 33, "Combat", "damage", nil, []string{"Defias Mage Ring", "Ring of Pure Silver"}, map[string]string{"finger1": "Ring of Pure Silver", "finger2": "Defias Mage Ring"}, "ring"},
	{"two copies of one ring in one stack, both finger slots empty", "ROGUE", 33, "Combat", "damage", nil, []string{"Ring of Pure Silver", "Ring of Pure Silver"}, map[string]string{"finger1": "Ring of Pure Silver"}, "ring"},
	{"the ring worn in the second slot, a copy in the bags", "ROGUE", 33, "Combat", "damage", map[string]string{"finger2": "Ring of Pure Silver"}, []string{"Ring of Pure Silver"}, nil, "ring"},
	{"a different ring for the empty second slot", "ROGUE", 33, "Combat", "damage", map[string]string{"finger1": "Ring of Pure Silver"}, []string{"Defias Mage Ring"}, map[string]string{"finger2": "Defias Mage Ring"}, "ring"},
	{"a ring that needs level 60, at level 59", "MAGE", 59, "Frost", "damage", nil, []string{"Don Rodrigo's Band"}, nil, "ring"},
	{"a ring that needs level 60, at level 60", "MAGE", 60, "Frost", "damage", nil, []string{"Don Rodrigo's Band"}, map[string]string{"finger1": "Don Rodrigo's Band"}, "ring"},
	{"a ring for a class in plate (a ring has no armor type)", "PALADIN", 35, "Retribution", "damage", nil, []string{"Defias Mage Ring"}, map[string]string{"finger1": "Defias Mage Ring"}, "ring"},
	// a shirt, a tabard, a bag and a quiver are no gear to weigh
	{"a shirt for the empty shirt slot", "MAGE", 30, "Frost", "damage", nil, []string{"White Tuxedo Shirt"}, nil, "shirt"},
	{"a shirt for a warrior", "WARRIOR", 30, "Fury", "damage", nil, []string{"Recruit's Shirt"}, nil, "shirt"},
	{"a tabard for the empty tabard slot", "PRIEST", 30, "Holy", "healer", nil, []string{"Guild Tabard"}, nil, "tabard"},
	{"a faction tabard", "WARRIOR", 30, "Fury", "damage", nil, []string{"Tabard of Stormwind"}, nil, "tabard"},
	{"a bag", "WARRIOR", 30, "Fury", "damage", nil, []string{"Mageweave Bag"}, nil, "bag"},
	{"a quiver for a hunter", "HUNTER", 40, "Beast Mastery", "damage", nil, []string{"Heavy Quiver"}, nil, "bag"},
	{"an ammo pouch for a hunter", "HUNTER", 40, "Beast Mastery", "damage", nil, []string{"Thick Leather Ammo Pouch"}, nil, "bag"},
	{"a soul bag for a warlock", "WARLOCK", 40, "Affliction", "damage", nil, []string{"Soul Pouch"}, nil, "bag"},
}

// TestOfflineBenchmarkSweepJewelleryCosmeticBagSlotsFromBags: the equip-from-bags list for a neck, a ring, a shirt, a tabard, a bag, a quiver or an ammo
// pouch in the bags, beside the dataset's rows and the game's rules for the slot.
func TestOfflineBenchmarkSweepJewelleryCosmeticBagSlotsFromBags(t *testing.T) {
	ds := usabStore()
	var out []benchRow
	for _, tc := range jcBagCases {
		c := character.Character{Name: "Sweep", Faction: "Horde", Class: tc.class, Level: &tc.level, Profile: &character.Profile{Role: tc.role, Focus: "leveling", Spec: tc.spec}, Equipment: map[string]character.EquippedItem{}}
		for slot, name := range tc.worn {
			r := usabItem(t, name)
			id, il := int(num(r["id"])), int(num(r["item_level"]))
			c.Equipment[slot] = character.EquippedItem{Slot: slot, ItemID: &id, ItemLevel: &il, Name: name}
		}
		for _, name := range tc.bags {
			r := usabItem(t, name)
			n := 1
			if len(c.Bags) == 0 && len(tc.bags) == 2 && tc.bags[0] == tc.bags[1] {
				n = 2 // both copies are one stack
			}
			if len(c.Bags) > 0 && c.Bags[0].Name == name && tc.bags[0] == tc.bags[1] {
				continue
			}
			c.Bags = append(c.Bags, character.ItemStack{ItemID: int(num(r["id"])), Name: name, Count: n})
		}
		got := map[string]string{}
		var parts, want []string
		for _, u := range gear.BagUpgrades(c, ds, tc.spec) {
			got[u.Slot] = u.Name
			parts = append(parts, u.Slot+" <- "+u.Name)
		}
		for s, n := range tc.want {
			want = append(want, s+" <- "+n)
		}
		sort.Strings(parts)
		sort.Strings(want)
		ok := len(got) == len(tc.want)
		for s, n := range tc.want {
			ok = ok && got[s] != "" && (got[s] == n || tc.slotQuestion == "ring" && len(tc.want) == 2 && (got["finger1"] == tc.want["finger2"] || got["finger1"] == tc.want["finger1"]))
		}
		a, w := strings.Join(parts, "; "), strings.Join(want, "; ")
		if a == "" {
			a = "nothing to equip"
		}
		if w == "" {
			w = "nothing to equip"
		}
		out = append(out, benchRow{fmt.Sprintf("%s level %d %s: %s", tc.class, tc.level, tc.spec, tc.what), a, w, jcBagSrc + "; " + jcSrc, resultIf(ok, "the equip list for the "+tc.slotQuestion+" slot")})
	}
	finish(t, "gear-sweep-jewellery-slots-bags", "Offline neck, ring, shirt, tabard and bag sweep: the equip-from-bags list", out)
}

// jcContainerCase is a bag, quiver or ammo pouch in a character's bags, the ranged weapon worn, and what the review must say.
type jcContainerCase struct {
	class   string
	level   int
	ranged  string
	item    string
	want    inventory.Action
	says    []string
	without []string
	ref     string
}

var jcContainerCases = []jcContainerCase{
	{"HUNTER", 40, "", "Heavy Quiver", inventory.Unsure, []string{"no bag size"}, []string{"from level", "fires"}, "a quiver of required level 30 the hunter has reached"},
	{"HUNTER", 20, "", "Heavy Quiver", inventory.Unsure, []string{"you can use it from level 30", "no bag size"}, nil, "a quiver of required level 30, ten levels ahead"},
	{"HUNTER", 29, "", "Heavy Leather Ammo Pouch", inventory.Unsure, []string{"you can use it from level 30", "no bag size"}, nil, "an ammo pouch of required level 30, a level ahead"},
	{"HUNTER", 30, "", "Heavy Leather Ammo Pouch", inventory.Unsure, []string{"no bag size"}, []string{"from level"}, "an ammo pouch of required level 30 at 30"},
	{"HUNTER", 35, "", "Quickdraw Quiver", inventory.Unsure, []string{"you can use it from level 40", "no bag size"}, nil, "a quiver of required level 40"},
	{"HUNTER", 40, "Worn Shortbow", "Heavy Quiver", inventory.Unsure, []string{"no bag size"}, []string{"fires"}, "a quiver for the arrows a bow fires"},
	{"HUNTER", 40, "Solid Blunderbuss", "Heavy Quiver", inventory.Unsure, []string{"fires bullets", "quiver holds arrows"}, nil, "a quiver (arrows) beside a gun (bullets)"},
	{"HUNTER", 40, "Worn Shortbow", "Thick Leather Ammo Pouch", inventory.Unsure, []string{"fires arrows", "ammo pouch holds bullets"}, nil, "an ammo pouch (bullets) beside a bow (arrows)"},
	{"HUNTER", 40, "Solid Blunderbuss", "Thick Leather Ammo Pouch", inventory.Unsure, []string{"no bag size"}, []string{"fires"}, "an ammo pouch for the bullets a gun fires"},
	{"ROGUE", 40, "Light Crossbow", "Heavy Quiver", inventory.Unsure, []string{"no bag size"}, []string{"fires"}, "a quiver for the arrows a crossbow fires"},
	{"WARRIOR", 40, "Solid Blunderbuss", "Heavy Quiver", inventory.Unsure, []string{"fires bullets", "quiver holds arrows"}, nil, "a warrior's gun beside a quiver"},
	{"PRIEST", 40, "", "Heavy Quiver", inventory.Sell, []string{"cannot use ammo"}, nil, "a class with no ranged ammo weapon"},
	{"MAGE", 40, "", "Thick Leather Ammo Pouch", inventory.Sell, []string{"cannot use ammo"}, nil, "a class with no ranged ammo weapon"},
	{"WARRIOR", 30, "", "Mageweave Bag", inventory.Unsure, []string{"no bag size"}, []string{"from level"}, "a plain bag has no required level; its item level is no level to wear it"},
	{"MAGE", 5, "", "Mageweave Bag", inventory.Unsure, []string{"no bag size"}, []string{"from level"}, "a bag of item level 35 for a level 5 character: no required level"},
	{"WARLOCK", 30, "", "Soul Pouch", inventory.Unsure, []string{"no bag size"}, []string{"from level"}, "a warlock's own class bag"},
	{"MAGE", 30, "", "Soul Pouch", inventory.Sell, []string{"class restriction leaves your class out"}, nil, "a bag limited to warlocks"},
	{"DRUID", 30, "", "Cenarion Herb Bag", inventory.Unsure, []string{"no bag size"}, []string{"from level"}, "a herb bag with no class limit"},
	{"MAGE", 30, "", "White Tuxedo Shirt", inventory.Keep_, []string{"not far below"}, nil, "a shirt: every class, no level to fall behind"},
	{"WARRIOR", 60, "", "Recruit's Shirt", inventory.Keep_, []string{"not far below"}, nil, "a shirt of item level 1 at level 60: no level to fall behind"},
	{"PRIEST", 50, "", "Guild Tabard", inventory.Keep_, []string{"not far below"}, nil, "a tabard: every class"},
	{"HUNTER", 10, "", "Tabard of Stormwind", inventory.Keep_, []string{"not far below"}, nil, "a tabard whose faction the data does not name (no race column)"},
}

// TestOfflineBenchmarkSweepJewelleryCosmeticBagSlotsReview: the bag review for a bag, quiver, ammo pouch, shirt or tabard: a container is never ranked (the
// data holds no bag size), says from which level its row lets the holder carry it and which ammo it holds against the ranged weapon worn, is sold by a
// class that cannot use it, and a shirt or tabard is kept by every class at every level.
func TestOfflineBenchmarkSweepJewelleryCosmeticBagSlotsReview(t *testing.T) {
	ds := usabStore()
	var out []benchRow
	for _, tc := range jcContainerCases {
		r := usabItem(t, tc.item)
		c := usabChar(tc.class, tc.level, "Horde")
		c.Name, c.BagsCaptured, c.BankCaptured, c.Equipment = "Benchtest", true, true, map[string]character.EquippedItem{}
		if tc.ranged != "" {
			w := usabItem(t, tc.ranged)
			id := int(num(w["id"]))
			c.Equipment["ranged"] = character.EquippedItem{Slot: "ranged", ItemID: &id, Name: tc.ranged}
		}
		c.Bags = []character.ItemStack{{ItemID: int(num(r["id"])), Count: 1, Name: tc.item}}
		res := inventory.ReviewAll([]character.Character{c}, lgItemsOnly{ds})
		got, ok := "no line", false
		if len(res.Lines) == 1 {
			l := res.Lines[0]
			got = fmt.Sprintf("%s: %s", l.Action, l.Reason)
			ok = l.Action == tc.want
			for _, s := range tc.says {
				ok = ok && strings.Contains(l.Reason, s)
			}
			for _, s := range tc.without {
				ok = ok && !strings.Contains(l.Reason, s)
			}
		}
		weapon := ""
		if tc.ranged != "" {
			weapon = ", wielding " + tc.ranged
		}
		out = append(out, benchRow{fmt.Sprintf("%s (%s, level %d%s): bag review", tc.item, strings.ToLower(tc.class), tc.level, weapon), got,
			fmt.Sprintf("%s (%s; class %d, subclass %d, class mask %d, required level %d)", tc.want, tc.ref, num(r["class_id"]), num(r["subclass_id"]), num(r["allowable_class_mask"]), num(r["required_level"])), jcRevSrc, resultIf(ok, "the action or the reason for this bag, quiver, pouch, shirt or tabard")})
	}
	finish(t, "gear-sweep-jewellery-slots-review", "Offline neck, ring, shirt, tabard and bag sweep: the bag review for containers, shirts and tabards", out)
}

// jcPhrase is one everyday way to ask about a neck, ring, shirt, tabard or bag slot, and the facts the advisor adds to the question.
type jcPhrase struct {
	q    string
	slot string
	want []string // each must appear in the facts of the question
}

var jcPhrases = []jcPhrase{
	// rings, and the same ring twice
	{"can I wear two of the same ring?", "ring", []string{"Unique items:", "no unique or unique-equipped flag"}},
	{"can I equip two copies of the same ring", "ring", []string{"Unique items:"}},
	{"can I wear the same ring on both fingers", "ring", []string{"Unique items:"}},
	{"are rings unique-equipped?", "ring", []string{"Unique items:"}},
	{"can I wear two of the same trinket?", "ring", []string{"Unique items:"}},
	{"is the Defias Mage Ring unique?", "ring", []string{"Unique items:"}},
	{"can my mage wear a ring?", "ring", []string{"Mage:", "any class may wear them"}},
	{"can a warrior wear a necklace", "neck", []string{"Warrior:", "any class may wear them"}},
	{"can my priest use an amulet?", "neck", []string{"Priest:", "any class may wear them"}},
	{"are necklaces restricted by armor type?", "neck", []string{"any class may wear them"}},
	// shirts and tabards
	{"do shirts give stats?", "shirt", []string{"Shirts and tabards:", "no stats"}},
	{"can any class wear a shirt?", "shirt", []string{"Shirts and tabards:", "no class limit"}},
	{"what does a shirt do", "shirt", []string{"Shirts and tabards:"}},
	{"can a warrior wear a shirt under plate?", "shirt", []string{"Shirts and tabards:"}},
	{"can I put on a shirt and a chestpiece together?", "shirt", []string{"Shirts and tabards:", "never replaces it"}},
	{"does a tabard replace my chest armor?", "tabard", []string{"Shirts and tabards:", "never replaces it"}},
	{"do tabards give stats", "tabard", []string{"Shirts and tabards:", "no stats"}},
	{"which tabard should I wear?", "tabard", []string{"Shirts and tabards:", "is an upgrade over another"}},
	{"can my mage wear a tabard?", "tabard", []string{"Shirts and tabards:", "any class may wear one"}},
	{"can a Horde character wear the Tabard of Stormwind?", "tabard", []string{"do not call a tabard faction-locked"}},
	// bags, quivers and ammo pouches
	{"which bag should I use?", "bag", []string{"Bag and bank size:", "no bag slot count"}},
	{"what is the best bag for a hunter?", "bag", []string{"Bag and bank size:"}},
	{"how many bag slots do I have?", "bag", []string{"Bag and bank size:"}},
	{"can I equip a bigger bag?", "bag", []string{"Bag and bank size:"}},
	{"what is the biggest quiver?", "bag", []string{"Bag and bank size:", "quiver"}},
	{"can a priest use an ammo pouch?", "bag", []string{"Quivers, ammo pouches and class bags:", "only a hunter, rogue or warrior fires ammo"}},
	{"which class can use a quiver", "bag", []string{"Quivers, ammo pouches and class bags:", "0 of the"}},
	{"do I need a quiver for my bow?", "bag", []string{"Quivers, ammo pouches and class bags:", "arrows"}},
	{"does a gun use a quiver or an ammo pouch?", "bag", []string{"holds bullets"}},
	{"can a mage use a soul bag?", "bag", []string{"Quivers, ammo pouches and class bags:", "Soul Pouch (warlocks only)"}},
	{"what level do I need for the Heavy Quiver?", "bag", []string{"Quivers, ammo pouches and class bags:", "required level"}},
	{"can a druid use a herb bag?", "bag", []string{"Quivers, ammo pouches and class bags:"}},
}

// jcNotPhrases are questions about a ring, a neck, a shirt or a bag that need none of these lines.
var jcNotPhrases = []string{
	"which ring should I wear?", "what is the best neck for me?", "where do I buy a ring of protection?", "I wear the same boots as my brother",
	"where can I buy a shirt?", "how do I get to Stormwind?",
}

// TestOfflineBenchmarkSweepJewelleryCosmeticBagSlotPhrasings: everyday ways to ask about the same ring twice, a shirt, a tabard, a bag, a quiver or an
// ammo pouch reach the lines that say what the dataset holds (no unique flag, no stats on a shirt or tabard, no bag size, the class limit of each
// bag row), and a question that needs none gets none.
func TestOfflineBenchmarkSweepJewelleryCosmeticBagSlotPhrasings(t *testing.T) {
	ds := usabStore()
	c := usabChar("MAGE", 30, "Horde")
	facts := func(q string) string {
		return ask.LimitFacts(q) + "\n" + ask.ProficiencyFacts(c, ds, q) + "\n" + ask.AmmoBagFacts(ds, q)
	}
	var out []benchRow
	for _, tc := range jcPhrases {
		f := facts(tc.q)
		var miss []string
		for _, w := range tc.want {
			if !strings.Contains(f, w) {
				miss = append(miss, w)
			}
		}
		got := "the facts hold the rule asked"
		if len(miss) > 0 {
			got = fmt.Sprintf("facts lack %q", miss)
		}
		out = append(out, benchRow{fmt.Sprintf("%q: %s", tc.q, tc.slot), got, strings.Join(tc.want, ", "), jcPhraseSr, resultIf(len(miss) == 0, got)})
	}
	for _, q := range jcNotPhrases {
		f := ask.LimitFacts(q)
		bad := strings.Contains(f, "Unique items:") || strings.Contains(f, "Bag and bank size:") || ask.AmmoBagFacts(ds, q) != ""
		if q == "where can I buy a shirt?" {
			bad = strings.Contains(f, "Unique items:") || strings.Contains(f, "Bag and bank size:") || ask.AmmoBagFacts(ds, q) != ""
		}
		out = append(out, benchRow{fmt.Sprintf("%q: needs no unique, bag or ammo line", q), fmt.Sprintf("lines added: %v", bad), "no unique, bag size or ammo bag line", jcPhraseSr, resultIf(!bad, "a rule line for a question that is none")})
	}
	finish(t, "gear-sweep-jewellery-slots-phrasings", "Offline neck, ring, shirt, tabard and bag sweep: questions in a player's words", out)
}

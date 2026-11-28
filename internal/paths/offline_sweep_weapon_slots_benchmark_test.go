package paths_test

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/WoW-Companion/advisor/internal/ask"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/gear"
	"github.com/WoW-Companion/advisor/internal/inventory"
	itemdata "github.com/WoW-Companion/advisor/internal/items"
	"github.com/WoW-Companion/advisor/internal/store"
)

// The offline weapon-slot sweep: what the gear finder offers a synthetic character for the main hand, the off hand, a two-hander, the
// ranged slot, the trinkets, the rings and the neck (all nine classes, damage and shield or healer builds, levels across 1 to 60, both
// factions, empty slots so every usable item is a candidate), and what the bag review and the equip-from-bags list say about weapons,
// off-hand items, trinkets and rings in the bags, beside the dataset's item rows, with no network. testdata/usability_snapshot holds the
// real rows (weapons of every subclass, shields, held items, trinkets, rings, necks, relics) and the weapon_skills the dataset gives each
// class. The reference is written apart from the finder: the weapon skills the dataset lets the class learn, the item's class mask, quality,
// bonding and required level, the shield classes and the levels Dual Wield opens the off hand (rogue 10, warrior and hunter 20, shaman 40).
// The dataset holds no unique-equipped flag and no relic effect, so a ring or trinket worn in one slot is never offered for its pair and a
// relic is never ranked. make offline-benchmark runs it; WOWC_BENCH_REPORT receives the tables.

const (
	wsSrc      = "dataset items (class_id, subclass_id, slot, allowable_class_mask, required_level, quality, bonding), weapon_skills (learnable)"
	wsBagSrc   = "dataset items (slot, item_level, stats_status); game: a two-hander fills both hands, one copy of a ring or trinket fills one slot"
	wsPhraseSr = "dataset weapon_skills, items (slot 14 shield, 22 off hand, 23 held, 28 relic); Icy Veins WoW Forever class guides (weapon and armor proficiency)"
)

// wsSlots are the questions the sweep asks of the finder: the slot it names and the capture slots that answer it.
var wsSlots = []struct{ name, slot string }{
	{"main hand", "mainhand"}, {"off hand", "offhand"}, {"ranged", "ranged"}, {"trinket", "trinket"}, {"ring", "finger"}, {"neck", "neck"},
}

// wsFree is whether a build keeps no shield: a two-hand or dual-wield damage spec (the guides' Arms, Fury, Combat, Enhancement,
// Retribution, feral).
func wsFree(role, spec string) bool {
	if role != "damage" {
		return false
	}
	spec = strings.ToLower(spec)
	for _, k := range []string{"arms", "fury", "enhancement", "retribution", "combat", "assassination", "subtlety", "feral"} {
		if strings.Contains(spec, k) {
			return true
		}
	}
	return false
}

// wsReference is the (capture slot, item id) pairs the dataset's rows allow the class at the level in the six slots, with the trinket and
// ring pairs joined into one.
func wsReference(t *testing.T, class string, classID int64, level int, faction string, free bool) map[string]bool {
	t.Helper()
	rows, err := usabStore().Filter("items", store.Range{Column: "id", Min: 0, Max: 1 << 40})
	if err != nil {
		t.Fatal(err)
	}
	dual := false
	if need, ok := grDualLevel[class]; ok && level >= need {
		dual = true
	}
	out := map[string]bool{}
	for _, r := range rows {
		slot, cls, sub := num(r["slot"]), num(r["class_id"]), num(r["subclass_id"])
		if slot == 0 || (cls != 2 && cls != 4) || slot == 28 {
			continue
		}
		name := text(r["name"])
		il, q, bond := int(num(r["item_level"])), num(r["quality"]), num(r["bonding"])
		if itemdata.Unobtainable(name) || itemdata.WrongFaction(name, faction) || il <= 1 || q <= 0 || q == 6 || bond == 4 || bond == 5 {
			continue
		}
		req, _ := gear.ReqLevelOf(int(num(r["required_level"])), il)
		if req > level+2 || !maskAllows(num(r["allowable_class_mask"]), classID) {
			continue
		}
		if cls == 2 && !wsLearns(t, classID, weaponSkillOf[sub]) {
			continue
		}
		var slots []string
		switch slot {
		case 2:
			slots = []string{"neck"}
		case 11:
			slots = []string{"finger"}
		case 12:
			slots = []string{"trinket"}
		case 13:
			slots = []string{"mainhand"}
			if dual {
				slots = append(slots, "offhand")
			}
		case 17, 21:
			slots = []string{"mainhand"}
		case 22:
			if dual {
				slots = []string{"offhand"}
			}
		case 14:
			if grShieldClasses[class] && !free {
				slots = []string{"offhand"}
			}
		case 23:
			slots = []string{"offhand"}
		case 15, 25, 26:
			slots = []string{"ranged"}
		}
		for _, s := range slots {
			out[fmt.Sprintf("%s/%d", s, num(r["id"]))] = true
		}
	}
	return out
}

var (
	wsLearnOnce  sync.Once
	wsLearnTable map[[2]int64]bool
)

// wsLearns is whether the dataset's weapon_skills let the class learn the skill line (the table read once for the package).
func wsLearns(t *testing.T, classID, skill int64) bool {
	t.Helper()
	wsLearnOnce.Do(func() {
		rows, err := usabStore().Filter("weapon_skills", store.Range{Column: "class_id", Min: int64(0), Max: int64(99)})
		if err != nil {
			t.Fatal(err)
		}
		wsLearnTable = map[[2]int64]bool{}
		for _, r := range rows {
			if b, _ := r["learnable"].(bool); b {
				wsLearnTable[[2]int64{num(r["class_id"]), num(r["skill_line_id"])}] = true
			}
		}
	})
	return wsLearnTable[[2]int64{classID, skill}]
}

func wsSlotKey(slot string) string {
	if strings.HasPrefix(slot, "finger") {
		return "finger"
	}
	if strings.HasPrefix(slot, "trinket") {
		return "trinket"
	}
	return slot
}

type wsBuild struct{ spec, role string }

var wsBuilds = map[string][]wsBuild{
	"WARRIOR": {{"Fury", "damage"}, {"Protection", "tank"}}, "PALADIN": {{"Retribution", "damage"}, {"Protection", "tank"}},
	"HUNTER": {{"Beast Mastery", "damage"}}, "ROGUE": {{"Combat", "damage"}}, "PRIEST": {{"Holy", "healer"}},
	"SHAMAN": {{"Enhancement", "damage"}, {"Restoration", "healer"}}, "MAGE": {{"Frost", "damage"}}, "WARLOCK": {{"Affliction", "damage"}},
	"DRUID": {{"Feral Combat", "damage"}, {"Restoration", "healer"}},
}

var wsLevels = []int{4, 12, 22, 33, 40, 51, 60}

// wsFindSlots: for every class, build, level and faction, the items the finder offers in each of the six slots are the items the rows allow.
func wsFindSlots(t *testing.T, area, title string, classes []string) {
	var out []benchRow
	ds := usabStore()
	asked := map[string]int{}
	for _, class := range classes {
		classID := grClassOf(class)
		for _, b := range wsBuilds[class] {
			for i, lv := range wsLevels {
				faction := []string{"Alliance", "Horde"}[i%2]
				level := lv
				c := character.Character{Name: "Sweep", Class: class, Faction: faction, Level: &level, Profile: &character.Profile{Role: b.role, Focus: "leveling", Spec: b.spec}}
				res, err := gear.Find(c, ds, "", gear.Options{NoSources: true})
				if err != nil {
					t.Fatal(err)
				}
				app := map[string]bool{}
				for slot, cands := range res.Slots {
					for _, cd := range cands {
						app[fmt.Sprintf("%s/%d", wsSlotKey(slot), cd.Item.ID)] = true
					}
				}
				ref := wsReference(t, class, classID, level, faction, wsFree(b.role, b.spec))
				for _, sl := range wsSlots {
					var extra, missing []string
					for k := range app {
						if strings.HasPrefix(k, sl.slot+"/") && !ref[k] {
							extra = append(extra, k)
						}
					}
					n := 0
					for k := range ref {
						if strings.HasPrefix(k, sl.slot+"/") {
							n++
							if !app[k] {
								missing = append(missing, k)
							}
						}
					}
					sort.Strings(extra)
					sort.Strings(missing)
					bad := ""
					if len(extra) > 0 {
						bad += fmt.Sprintf(" offers %v the rows do not allow;", extra)
					}
					if len(missing) > 0 {
						bad += fmt.Sprintf(" leaves out %v the rows allow;", missing)
					}
					asked[sl.name]++
					out = append(out, benchRow{fmt.Sprintf("%s %s level %d, %s: what fits the %s", class, b.spec, lv, faction, sl.name),
						fmt.Sprintf("%d offered", len(keysWith(app, sl.slot+"/"))), fmt.Sprintf("%d allowed", n), wsSrc,
						resultIf(bad == "", strings.TrimSuffix(strings.TrimSpace(bad), ";"))})
				}
			}
		}
	}
	finish(t, area, title, out)
}

func keysWith(m map[string]bool, prefix string) []string {
	var out []string
	for k := range m {
		if strings.HasPrefix(k, prefix) {
			out = append(out, k)
		}
	}
	return out
}

func TestOfflineBenchmarkSweepWeaponSlotsWarrior(t *testing.T) {
	wsFindSlots(t, "gear-sweep-weapon-slots-warrior", "Offline weapon-slot sweep: warrior", []string{"WARRIOR"})
}

func TestOfflineBenchmarkSweepWeaponSlotsPaladin(t *testing.T) {
	wsFindSlots(t, "gear-sweep-weapon-slots-paladin", "Offline weapon-slot sweep: paladin", []string{"PALADIN"})
}

func TestOfflineBenchmarkSweepWeaponSlotsHunter(t *testing.T) {
	wsFindSlots(t, "gear-sweep-weapon-slots-hunter", "Offline weapon-slot sweep: hunter", []string{"HUNTER"})
}

func TestOfflineBenchmarkSweepWeaponSlotsRogue(t *testing.T) {
	wsFindSlots(t, "gear-sweep-weapon-slots-rogue", "Offline weapon-slot sweep: rogue", []string{"ROGUE"})
}

func TestOfflineBenchmarkSweepWeaponSlotsPriest(t *testing.T) {
	wsFindSlots(t, "gear-sweep-weapon-slots-priest", "Offline weapon-slot sweep: priest", []string{"PRIEST"})
}

func TestOfflineBenchmarkSweepWeaponSlotsShaman(t *testing.T) {
	wsFindSlots(t, "gear-sweep-weapon-slots-shaman", "Offline weapon-slot sweep: shaman", []string{"SHAMAN"})
}

func TestOfflineBenchmarkSweepWeaponSlotsMage(t *testing.T) {
	wsFindSlots(t, "gear-sweep-weapon-slots-mage", "Offline weapon-slot sweep: mage", []string{"MAGE"})
}

func TestOfflineBenchmarkSweepWeaponSlotsWarlock(t *testing.T) {
	wsFindSlots(t, "gear-sweep-weapon-slots-warlock", "Offline weapon-slot sweep: warlock", []string{"WARLOCK"})
}

func TestOfflineBenchmarkSweepWeaponSlotsDruid(t *testing.T) {
	wsFindSlots(t, "gear-sweep-weapon-slots-druid", "Offline weapon-slot sweep: druid", []string{"DRUID"})
}

// wsBagCase is one character's worn gear and bags and the equip-from-bags list the game's rules give: slot to the item name, "" for none.
type wsBagCase struct {
	what         string
	class        string
	level        int
	spec, role   string
	worn         map[string]string
	bags         []string
	want         map[string]string
	slotQuestion string
}

var wsBagCases = []wsBagCase{
	// a ring or trinket worn in one slot is not offered again for the other
	{"the trinket worn in one slot, a copy in the bags", "ROGUE", 50, "Combat", "damage", map[string]string{"trinket1": "Thunderbrew's Boot Flask"}, []string{"Thunderbrew's Boot Flask"}, nil, "trinket"},
	{"the trinket worn in the other slot, a copy in the bags", "ROGUE", 50, "Combat", "damage", map[string]string{"trinket2": "Thunderbrew's Boot Flask"}, []string{"Thunderbrew's Boot Flask"}, nil, "trinket"},
	{"the ring worn in one slot, a copy in the bags", "ROGUE", 30, "Combat", "damage", map[string]string{"finger1": "Defias Mage Ring"}, []string{"Defias Mage Ring"}, nil, "ring"},
	{"a second ring for the empty ring slot", "ROGUE", 33, "Combat", "damage", map[string]string{"finger1": "Defias Mage Ring"}, []string{"Ring of Pure Silver"}, map[string]string{"finger2": "Ring of Pure Silver"}, "ring"},
	{"a trinket for the empty trinket slots", "WARRIOR", 50, "Fury", "damage", nil, []string{"Thunderbrew's Boot Flask"}, map[string]string{"trinket1": "Thunderbrew's Boot Flask"}, "trinket"},
	{"a neck for the empty neck slot", "WARRIOR", 35, "Fury", "damage", nil, []string{"Pendant of Myzrael"}, map[string]string{"neck": "Pendant of Myzrael"}, "neck"},
	{"a neck that needs level 30, at level 25", "WARRIOR", 25, "Fury", "damage", nil, []string{"Pendant of Myzrael"}, nil, "neck"},
	// a two-hander takes both hands
	{"a two-hander of item level 26 over a dual-wield pair of 16 and 14", "WARRIOR", 30, "Fury", "damage", map[string]string{"mainhand": "Hatchet", "offhand": "Mace"}, []string{"Dacian Falx"}, nil, "two-hand"},
	{"a two-hander of item level 26 over a worn main hand of 2 and an empty off hand", "WARRIOR", 30, "Fury", "damage", map[string]string{"mainhand": "Worn Axe"}, []string{"Dacian Falx"}, map[string]string{"mainhand": "Dacian Falx"}, "two-hand"},
	{"a two-hander for a worn one-hand and shield (a tank keeps the shield)", "WARRIOR", 30, "Protection", "tank", map[string]string{"mainhand": "Hatchet", "offhand": "Dented Buckler"}, []string{"Dacian Falx"}, nil, "two-hand"},
	// the off hand opens when Dual Wield is learned
	{"an off-hand weapon one level before a rogue learns Dual Wield", "ROGUE", 9, "Combat", "damage", map[string]string{"mainhand": "Worn Dagger"}, []string{"Left-Handed Brass Knuckles"}, nil, "off hand"},
	{"an off-hand weapon at the level a rogue learns Dual Wield", "ROGUE", 10, "Combat", "damage", map[string]string{"mainhand": "Worn Dagger"}, []string{"Left-Handed Brass Knuckles"}, map[string]string{"offhand": "Left-Handed Brass Knuckles"}, "off hand"},
	{"a one-hand weapon for a shaman's off hand at 39", "SHAMAN", 39, "Enhancement", "damage", map[string]string{"mainhand": "Hatchet"}, []string{"Mace"}, nil, "off hand"},
	{"a one-hand weapon for a shaman's off hand at 40", "SHAMAN", 40, "Enhancement", "damage", map[string]string{"mainhand": "Hatchet"}, []string{"Mace"}, map[string]string{"offhand": "Mace"}, "off hand"},
	{"an off-hand weapon for a paladin", "PALADIN", 60, "Retribution", "damage", map[string]string{"mainhand": "Hatchet"}, []string{"Left-Handed Claw"}, nil, "off hand"},
	{"a shield for a priest", "PRIEST", 30, "Holy", "healer", nil, []string{"Banded Buckler"}, nil, "off hand"},
	{"a held item for a priest's empty off hand", "PRIEST", 30, "Holy", "healer", nil, []string{"Grayson's Torch"}, map[string]string{"offhand": "Grayson's Torch"}, "off hand"},
	{"a shield for a fury warrior (dual wield keeps none)", "WARRIOR", 30, "Fury", "damage", map[string]string{"mainhand": "Hatchet", "offhand": "Mace"}, []string{"Banded Buckler"}, nil, "off hand"},
	// the ranged slot is for the classes with a ranged weapon skill
	{"a wand for a mage", "MAGE", 20, "Frost", "damage", nil, []string{"Charred Razormane Wand"}, map[string]string{"ranged": "Charred Razormane Wand"}, "ranged"},
	{"a wand for a warrior", "WARRIOR", 20, "Fury", "damage", nil, []string{"Charred Razormane Wand"}, nil, "ranged"},
	{"a bow for a paladin", "PALADIN", 20, "Retribution", "damage", nil, []string{"Worn Shortbow"}, nil, "ranged"},
	{"a crossbow over a worn bow for a hunter", "HUNTER", 20, "Beast Mastery", "damage", map[string]string{"ranged": "Worn Shortbow"}, []string{"Fine Light Crossbow"}, map[string]string{"ranged": "Fine Light Crossbow"}, "ranged"},
	{"a thrown weapon for a rogue", "ROGUE", 12, "Combat", "damage", nil, []string{"Balanced Throwing Dagger"}, map[string]string{"ranged": "Balanced Throwing Dagger"}, "ranged"},
	{"a gun for a mage", "MAGE", 20, "Frost", "damage", nil, []string{"Ornate Blunderbuss"}, nil, "ranged"},
	// the main hand follows the weapon skills the dataset gives the class
	{"a staff for a priest", "PRIEST", 20, "Holy", "healer", nil, []string{"Quarter Staff"}, map[string]string{"mainhand": "Quarter Staff"}, "main hand"},
	{"a sword for a priest", "PRIEST", 20, "Holy", "healer", nil, []string{"Cutlass"}, nil, "main hand"},
	{"a two-hand mace for a druid", "DRUID", 30, "Feral Combat", "damage", map[string]string{"mainhand": "Mace"}, []string{"Maul"}, map[string]string{"mainhand": "Maul"}, "main hand"},
}

// TestOfflineBenchmarkSweepWeaponSlotsFromBags: the equip-from-bags list for a weapon, an off-hand item, a trinket, a ring, a neck or a ranged
// weapon in the bags, beside the dataset's rows and the game's rules for the slot.
func TestOfflineBenchmarkSweepWeaponSlotsFromBags(t *testing.T) {
	ds := usabStore()
	var out []benchRow
	for _, tc := range wsBagCases {
		c := character.Character{Name: "Sweep", Faction: "Horde", Class: tc.class, Level: &tc.level, Profile: &character.Profile{Role: tc.role, Focus: "leveling", Spec: tc.spec}, Equipment: map[string]character.EquippedItem{}}
		item := func(name string) (int, int) {
			r := usabItem(t, name)
			return int(num(r["id"])), int(num(r["item_level"]))
		}
		for slot, name := range tc.worn {
			id, il := item(name)
			c.Equipment[slot] = character.EquippedItem{Slot: slot, ItemID: &id, ItemLevel: &il, Name: name}
		}
		for _, name := range tc.bags {
			id, _ := item(name)
			c.Bags = append(c.Bags, character.ItemStack{ItemID: id, Name: name, Count: 1})
		}
		got := map[string]string{}
		var parts []string
		for _, u := range gear.BagUpgrades(c, ds, tc.spec) {
			got[u.Slot] = u.Name
			parts = append(parts, u.Slot+" <- "+u.Name)
		}
		sort.Strings(parts)
		var want []string
		for s, n := range tc.want {
			want = append(want, s+" <- "+n)
		}
		sort.Strings(want)
		ok := len(got) == len(tc.want)
		for s, n := range tc.want {
			ok = ok && got[s] == n
		}
		a, w := strings.Join(parts, "; "), strings.Join(want, "; ")
		if a == "" {
			a = "nothing to equip"
		}
		if w == "" {
			w = "nothing to equip"
		}
		out = append(out, benchRow{fmt.Sprintf("%s level %d %s: %s", tc.class, tc.level, tc.spec, tc.what), a, w, wsBagSrc + "; " + wsSrc, resultIf(ok, "the equip list for the "+tc.slotQuestion+" slot")})
	}
	finish(t, "gear-sweep-weapon-slots-bags", "Offline weapon-slot sweep: the equip-from-bags list", out)
}

// wsPhrase is one everyday way to ask about a weapon, off-hand, shield, ranged, relic or jewellery slot, and what the facts the advisor adds
// to the question must say.
type wsPhrase struct {
	class, q string
	want     []string // each must appear in the weapon or proficiency facts
	slot     string
}

var wsPhrases = []wsPhrase{
	{"MAGE", "can a paladin use a shield?", []string{"Paladin:", "Can carry a shield."}, "shield"},
	{"MAGE", "can priests use shields", []string{"Priest:", "Cannot use a shield."}, "shield"},
	{"ROGUE", "does my warrior get a shield?", []string{"Warrior:", "Can carry a shield."}, "shield"},
	{"SHAMAN", "can I use a shield?", []string{"Shaman:", "Can carry a shield."}, "shield"},
	{"WARRIOR", "should my warrior carry a shield or a two-hander?", []string{"Warrior:", "Can carry a shield."}, "shield"},
	{"MAGE", "which classes can use shields", []string{"Warrior:", "Paladin:", "Shaman:", "Priest:", "Druid:"}, "shield"},
	{"ROGUE", "can my rogue use an off hand?", []string{"learns it at level 10"}, "off hand"},
	{"HUNTER", "when do I get an offhand?", []string{"learns it at level 20"}, "off hand"},
	{"PRIEST", "can my priest use an off-hand weapon", []string{"cannot dual wield"}, "off hand"},
	{"SHAMAN", "when can I dual wield?", []string{"learns it at level 40"}, "off hand"},
	{"MAGE", "can a warrior use two weapons at once, an off-hand?", []string{"Warrior:", "Dual Wield is a class spell"}, "off hand"},
	{"MAGE", "can a priest use a 2h mace?", []string{"Priest:", "One-Handed Maces"}, "two-hand"},
	{"MAGE", "can my druid use a two-hander?", []string{"Druid:", "Two-Handed Maces"}, "two-hand"},
	{"WARRIOR", "can my shaman use a 2-handed axe", []string{"Shaman:", "Two-Handed Axes"}, "two-hand"},
	{"MAGE", "can a rogue use throwing knives?", []string{"Rogue:", "Thrown"}, "ranged"},
	{"MAGE", "can my hunter use a spear?", []string{"Hunter:", "Polearms"}, "main hand"},
	{"WARRIOR", "can my priest use a wand?", []string{"Priest:", "Wands"}, "ranged"},
	{"WARRIOR", "which ranged weapon can my rogue use", []string{"Rogue:", "Bows", "Guns"}, "ranged"},
	{"MAGE", "what goes in the ranged slot for a paladin?", []string{"Paladin:", "libram"}, "relic"},
	{"MAGE", "can a druid use an idol", []string{"Druid:", "idol"}, "relic"},
	{"WARRIOR", "can my shaman use a relic", []string{"Shaman:", "totem"}, "relic"},
	{"PALADIN", "which relic should I use?", []string{"Paladin:", "libram"}, "relic"},
	{"MAGE", "can a warrior use a libram", []string{"Warrior:", "no relic slot"}, "relic"},
	{"WARRIOR", "can my mage wear a ring?", []string{"Mage:", "any class may wear them"}, "ring"},
	{"WARRIOR", "what trinkets can a priest use", []string{"Priest:", "any class may wear them"}, "trinket"},
	{"MAGE", "can a warrior wear a necklace", []string{"Warrior:", "any class may wear them"}, "neck"},
	{"MAGE", "are rings restricted by armor type?", []string{"any class may wear them"}, "ring"},
}

// wsNotPhrases are questions that name a shield, an off hand or a relic and are none of these.
var wsNotPhrases = []string{
	"when do I learn Power Word: Shield?", "can my warrior use shield slam?", "how does the paladin Divine Shield work?", "is Frost Ward better than Fire Ward?",
	"where do I buy a ring of protection?", "what does the idol of the sun look like in the quest?",
}

// TestOfflineBenchmarkSweepWeaponSlotPhrasings: everyday ways to ask who may use a shield, an off hand, a two-hander, a ranged weapon, a relic, a ring,
// a trinket or a neck reach the rule facts for the class the question names, and the lines say what the dataset gives (the weapon skills the class
// may learn, the Dual Wield level) and no more.
func TestOfflineBenchmarkSweepWeaponSlotPhrasings(t *testing.T) {
	ds := usabStore()
	var out []benchRow
	for _, tc := range wsPhrases {
		c := usabChar(tc.class, 30, "Horde")
		facts := ask.ProficiencyFacts(c, ds, tc.q) + "\n" + ask.WeaponFacts(c, ds, tc.q)
		var miss []string
		for _, w := range tc.want {
			if !strings.Contains(facts, w) {
				miss = append(miss, w)
			}
		}
		got := "the rule facts name every class and rule asked"
		if len(miss) > 0 {
			got = fmt.Sprintf("facts lack %q", miss)
		}
		out = append(out, benchRow{fmt.Sprintf("%q (%s asking): %s", tc.q, strings.ToLower(tc.class), tc.slot), got, strings.Join(tc.want, ", "), wsPhraseSr, resultIf(len(miss) == 0, got)})
	}
	for _, q := range wsNotPhrases {
		f := ask.ProficiencyFacts(usabChar("MAGE", 30, "Horde"), ds, q)
		out = append(out, benchRow{fmt.Sprintf("%q: not a proficiency question", q), fmt.Sprintf("facts added: %v", f != ""), "no class rule lines added", wsPhraseSr, resultIf(f == "", "rule lines for a question that is none")})
	}
	finish(t, "gear-sweep-weapon-slots-phrasings", "Offline weapon-slot sweep: weapon, off-hand, shield, relic and jewellery questions in a player's words", out)
}

// wsReviewCases are a weapon, shield, off-hand weapon, ranged weapon, trinket or ring in the bags and what the bag review tells the player.
var wsReviewCases = []reviewCase{
	{"ROGUE", 30, "Banded Buckler", inventory.Sell, 0}, {"MAGE", 30, "Dacian Falx", inventory.Sell, 0},
	{"PALADIN", 30, "Worn Shortbow", inventory.Sell, 0}, {"WARRIOR", 30, "Charred Razormane Wand", inventory.Sell, 0},
}

// TestOfflineBenchmarkSweepWeaponSlotsReview: a weapon, shield or ranged weapon no skill of the class can use is sold, not kept.
func TestOfflineBenchmarkSweepWeaponSlotsReview(t *testing.T) {
	reviewBatch(t, "gear-sweep-weapon-slots-review", "Offline weapon-slot sweep: the bag review for gear a class cannot use", wsReviewCases)
}

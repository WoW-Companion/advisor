package paths_test

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/WoW-Companion/advisor/internal/brief"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/gear"
	"github.com/WoW-Companion/advisor/internal/inventory"
	"github.com/WoW-Companion/advisor/internal/levelup"
	"github.com/WoW-Companion/advisor/internal/paths"
	"github.com/WoW-Companion/advisor/internal/store"
)

// The offline gear-usability, class-trainer and binding benchmark: who can wear or wield an item, when, what the
// advisor tells a player to do with it, where a class trains and for what, and how an item's binding is worded,
// beside the dataset's rows, with no network. testdata/usability_snapshot (testdata/usability_snapshot/build.sh) holds
// the real rows: a spread of armor, weapons, shirts, tabards, relics and bags, the crafted and dropped items of every
// binding, every class spell, the talent trees, the trainer spells, the weapon skills and the Weapon Masters. The
// dataset holds no armor proficiency, race, unique-equipped or class-trainer table and no class spell cost, so those
// references are the game's own rules written out here and the cases the dataset cannot answer check that the advisor
// says so and states none. A difference whose cause is a missing or wrong dataset row is marked "(data)". make
// offline-benchmark runs it; WOWC_BENCH_REPORT receives the tables.

const (
	usabDir      = "../../testdata/usability_snapshot"
	usabItemSrc  = "dataset items (class_id, subclass_id, slot, allowable_class_mask); weapon_skills; game armor proficiency"
	usabLevelSrc = "dataset items (required_level, item_level)"
	usabBindSrc  = "dataset items (bonding)"
	usabSpellSrc = "dataset class_spells (name, rank, level), talent_nodes"
	usabCostSrc  = "dataset class_spells, trainer_spells (cost_copper)"
	usabCitySrc  = "dataset weapon_masters (city, faction)"
	usabMasterSr = "dataset weapon_masters (cost_copper, min_level), weapon_skills (min_level)"
	usabNoneSrc  = "the dataset has no such column or table"
)

var (
	usabOnce  sync.Once
	usabCache store.DataStore
)

// usabStore reads the snapshot once for the package: the tables are decoded on first use and the tests share them.
func usabStore() store.DataStore {
	usabOnce.Do(func() { usabCache = store.NewDir(usabDir) })
	return usabCache
}

// usabClasses are the nine classes by capture token, with the dataset's class_id.
var usabClasses = []struct {
	token string
	id    int64
}{{"WARRIOR", 1}, {"PALADIN", 2}, {"HUNTER", 3}, {"ROGUE", 4}, {"PRIEST", 5}, {"SHAMAN", 7}, {"MAGE", 8}, {"WARLOCK", 9}, {"DRUID", 11}}

func usabChar(class string, level int, faction string) character.Character {
	return character.Character{Name: "Benchtest", Faction: faction, Class: class, Level: &level}
}

func usabItem(t *testing.T, name string) store.Row {
	t.Helper()
	return rowByName(t, usabStore(), "items", name)
}

// weaponSkillOf is the weapon skill line (SkillLine id) a weapon item subclass needs, from the game's item subclasses.
var weaponSkillOf = map[int64]int64{0: 44, 1: 172, 2: 45, 3: 46, 4: 54, 5: 160, 6: 229, 7: 43, 8: 55, 10: 136, 13: 473, 15: 173, 16: 176, 18: 226, 19: 228}

// dualWielders are the classes that learn Dual Wield, so may hold a weapon in the off hand.
var dualWielders = map[string]bool{"WARRIOR": true, "HUNTER": true, "ROGUE": true, "SHAMAN": true}

func maskAllows(mask, classID int64) bool { return mask <= 0 || mask&(1<<(classID-1)) != 0 }

// learnable reads weapon_skills: whether the class can learn the skill line.
func learnable(t *testing.T, classID, skill int64) bool {
	t.Helper()
	rows, err := usabStore().Filter("weapon_skills", store.Range{Column: "skill_line_id", Min: skill, Max: skill})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if num(r["class_id"]) == classID {
			b, _ := r["learnable"].(bool)
			return b
		}
	}
	return false
}

// armorWorn is the armor subclass the game has a class wear at a level: plate and mail users change type at 40.
func armorWorn(class string, level int) int64 {
	switch class {
	case "WARRIOR", "PALADIN":
		if level >= 40 {
			return 4
		}
		return 3
	case "HUNTER", "SHAMAN":
		if level >= 40 {
			return 3
		}
		return 2
	case "ROGUE", "DRUID":
		return 2
	}
	return 1
}

// refUsable is whether the class wears or wields the item at the level by the game's rules and the dataset's class mask.
func refUsable(t *testing.T, r store.Row, class string, classID int64, level int) bool {
	t.Helper()
	if !maskAllows(num(r["allowable_class_mask"]), classID) {
		return false
	}
	sub, slot := num(r["subclass_id"]), num(r["slot"])
	if num(r["class_id"]) == 2 {
		if slot == 22 && !dualWielders[class] {
			return false
		}
		return learnable(t, classID, weaponSkillOf[sub])
	}
	switch {
	case sub == 0 || sub == 5: // jewellery, trinkets, held items, shirts, tabards: cosmetic armor too
		return true
	case slot == 16: // a cloak is cloth for every class
		return true
	case sub == 6: // shields
		return class == "WARRIOR" || class == "PALADIN" || class == "SHAMAN"
	case sub >= 7 && sub <= 9: // librams, idols, totems
		return sub == 7 && class == "PALADIN" || sub == 8 && class == "DRUID" || sub == 9 && class == "SHAMAN"
	}
	return sub == armorWorn(class, level)
}

func whoCan(r store.Row, level int, ref func(class string, id int64) bool) (advisor, reference string) {
	var a, b []string
	for _, c := range usabClasses {
		if ok, known := gear.CanUse(c.token, level, r); ok && known {
			a = append(a, strings.ToLower(c.token[:3]))
		}
		if ref(c.token, c.id) {
			b = append(b, strings.ToLower(c.token[:3]))
		}
	}
	return strings.Join(a, " "), strings.Join(b, " ")
}

var weaponCases = []string{
	"Worn Dagger", "Hatchet", "Battle Axe", "Worn Shortbow", "Solid Blunderbuss", "Mace", "Maul", "Cutlass", "Bastard Sword",
	"Quarter Staff", "Iron Fists", "Balanced Throwing Dagger", "Light Crossbow", "Charred Razormane Wand", "Blight",
	"Left-Handed Claw", "Elunite Axe", "Blade of Cunning", "Diabolist's Blade",
}

// TestOfflineBenchmarkWeaponUsability: which classes the advisor says can wield a weapon, beside the weapon skills the
// dataset says each class can learn, the item's class mask, and the classes that learn Dual Wield for an off-hand weapon.
func TestOfflineBenchmarkWeaponUsability(t *testing.T) {
	var out []benchRow
	for _, name := range weaponCases {
		r := usabItem(t, name)
		got, want := whoCan(r, 60, func(class string, id int64) bool { return refUsable(t, r, class, id, 60) })
		out = append(out, benchRow{name + ": who can wield it", got, want + fmt.Sprintf(" (subclass %d, slot %d, class mask %d)", num(r["subclass_id"]), num(r["slot"]), num(r["allowable_class_mask"])),
			usabItemSrc, resultIf(got == want, "the classes that can wield it")})
	}
	finish(t, "advice-weapon-usability", "Offline gear: who can wield a weapon", out)
}

var armorCases = []string{
	"Tattered Cloth Vest", "Dirty Leather Vest", "Scalemail Vest", "Heavy Mithril Breastplate", "Worn Wooden Buckler", "Rugged Cape",
	"Ring of Pure Silver", "Thunderbrew's Boot Flask", "Bastion of Stormwind", "Dread Mage Hat", "Manaweave Robe", "Girdle of Thero-shan",
	"Steel Plate Helm", "Studded Hat", "Chainmail Belt",
}

// TestOfflineBenchmarkArmorUsability: which classes the advisor says can wear armor at level 30 and 50, beside the
// game's armor proficiency (cloth for casters, leather, mail and plate by class, the type changing at 40), the dataset's
// class mask, shields and jewellery.
func TestOfflineBenchmarkArmorUsability(t *testing.T) {
	var out []benchRow
	for _, name := range armorCases {
		r := usabItem(t, name)
		for _, level := range []int{30, 50} {
			got, want := whoCan(r, level, func(class string, id int64) bool { return refUsable(t, r, class, id, level) })
			out = append(out, benchRow{fmt.Sprintf("%s: who can wear it at level %d", name, level), got, want + fmt.Sprintf(" (subclass %d, slot %d, class mask %d)", num(r["subclass_id"]), num(r["slot"]), num(r["allowable_class_mask"])),
				usabItemSrc, resultIf(got == want, "the classes that can wear it")})
		}
	}
	finish(t, "advice-armor-usability", "Offline gear: who can wear armor", out)
}

var oddCases = []string{
	"Libram of Light", "Idol of Health", "Totem of Life", "Bloodsail Admiral's Hat", "Festival Suit", "Guild Tabard", "Recruit's Shirt",
	"Veteran Adventurer's Bracer",
}

// TestOfflineBenchmarkOddGearUsability: a libram, idol and totem go to the paladin, druid and shaman, and cosmetic armor,
// a shirt and a tabard to every class, where the dataset's class mask says all classes.
func TestOfflineBenchmarkOddGearUsability(t *testing.T) {
	var out []benchRow
	for _, name := range oddCases {
		r := usabItem(t, name)
		got, want := whoCan(r, 60, func(class string, id int64) bool { return refUsable(t, r, class, id, 60) })
		out = append(out, benchRow{name + ": who can wear it", got, want + fmt.Sprintf(" (subclass %d, slot %d, class mask %d)", num(r["subclass_id"]), num(r["slot"]), num(r["allowable_class_mask"])),
			usabItemSrc, resultIf(got == want, "the classes that can wear it")})
	}
	for _, name := range []string{"Large Blue Sack", "Light Quiver"} {
		r := usabItem(t, name)
		ok, known := gear.CanUse("WARRIOR", 60, r)
		out = append(out, benchRow{name + ": a bag is not worn gear", fmt.Sprintf("usable %v, rule known %v", ok, known), fmt.Sprintf("item class %d: no armor or weapon rule", num(r["class_id"])),
			usabItemSrc, resultIf(!known, "a verdict on an item that is not worn gear")})
	}
	finish(t, "advice-odd-gear-usability", "Offline gear: relics, cosmetic armor, shirts, tabards and bags", out)
}

// levelCases are a character and an item they could take: the dataset's required level decides when.
var levelCases = []struct {
	class string
	level int
	item  string
}{
	{"ROGUE", 30, "Iron Fists"}, {"ROGUE", 30, "Studded Hat"}, {"WARRIOR", 10, "Battle Axe"}, {"WARRIOR", 30, "Dacian Falx"},
	{"ROGUE", 30, "Left-Handed Blades"}, {"ROGUE", 20, "Defias Mage Ring"}, {"WARRIOR", 30, "Chainmail Belt"}, {"WARRIOR", 38, "Steel Plate Helm"},
	{"WARRIOR", 30, "Elunite Axe"}, {"ROGUE", 30, "Rugged Cape"}, {"ROGUE", 30, "Ring of Pure Silver"},
}

// found is the candidate of the item in a gear search, if any.
func found(res gear.Result, id int64) (gear.Candidate, bool) {
	for _, cs := range res.Slots {
		for _, c := range cs {
			if c.Item.ID == id {
				return c, true
			}
		}
	}
	return gear.Candidate{}, false
}

// TestOfflineBenchmarkRequiredLevel: the level an item needs, as the gear search words it, beside the dataset's
// required_level: "requires level N" from then on, "at level N" while the character is up to two levels short, nothing
// further off; an item with no required level in the dataset gets an estimate from its item level and is said to be one.
func TestOfflineBenchmarkRequiredLevel(t *testing.T) {
	ds := usabStore()
	var out []benchRow
	for _, tc := range levelCases {
		r := usabItem(t, tc.item)
		res, err := gear.Find(usabChar(tc.class, tc.level, "Horde"), ds, "", gear.Options{NoSources: true})
		if err != nil {
			t.Fatal(err)
		}
		cd, offered := found(res, num(r["id"]))
		rl, il := num(r["required_level"]), num(r["item_level"])
		got, want, ok := "not offered", "", false
		if offered {
			got = cd.LevelText()
		}
		switch {
		case rl == 0:
			want = fmt.Sprintf("no required level; estimated %d from item level %d", max(1, il-5), il)
			ok = offered && strings.HasSuffix(got, "(estimated)") && strings.Contains(got, fmt.Sprint(max(1, il-5)))
		case rl > int64(tc.level)+2:
			want = fmt.Sprintf("required_level %d: too far above level %d to offer", rl, tc.level)
			ok = !offered
		case rl > int64(tc.level):
			want = fmt.Sprintf("required_level %d: at level %d", rl, rl)
			ok = got == fmt.Sprintf("at level %d", rl)
		default:
			want = fmt.Sprintf("required_level %d: requires level %d", rl, rl)
			ok = got == fmt.Sprintf("requires level %d", rl)
		}
		out = append(out, benchRow{fmt.Sprintf("%s (%s, level %d): level needed", tc.item, strings.ToLower(tc.class), tc.level), got, want, usabLevelSrc, resultIf(ok, "the level the item needs")})
	}
	finish(t, "advice-required-level", "Offline gear: the level an item needs", out)
}

// reviewCases are items in one character's bags and what a player should be told: keep what they can wear (from the level
// the dataset gives), sell what no class of theirs can wear.
type reviewCase struct {
	class string
	level int
	item  string
	want  inventory.Action
	from  int64 // the level the answer must name, 0 for none
}

var reviewCases = []reviewCase{
	{"PALADIN", 30, "Libram of Light", inventory.Keep_, 60}, {"WARRIOR", 30, "Libram of Light", inventory.Sell, 0},
	{"DRUID", 30, "Idol of Health", inventory.Keep_, 60}, {"SHAMAN", 30, "Totem of Life", inventory.Keep_, 60},
	{"MAGE", 30, "Guild Tabard", inventory.Keep_, 0}, {"PRIEST", 50, "Recruit's Shirt", inventory.Keep_, 0},
	{"ROGUE", 50, "Bloodsail Admiral's Hat", inventory.Keep_, 0}, {"WARRIOR", 30, "Steel Plate Helm", inventory.Keep_, 40},
	{"MAGE", 30, "Steel Plate Helm", inventory.Sell, 0}, {"WARRIOR", 50, "Heavy Mithril Breastplate", inventory.Keep_, 0},
	{"WARRIOR", 30, "Large Blue Sack", inventory.Unsure, 0},
}

// reviewBatch runs the bag review for each case: one character with one item in the bags.
func reviewBatch(t *testing.T, area, title string, cases []reviewCase) {
	ds := usabStore()
	var out []benchRow
	for _, tc := range cases {
		r := usabItem(t, tc.item)
		c := usabChar(tc.class, tc.level, "Alliance")
		c.Name, c.BagsCaptured, c.BankCaptured = "Benchtest", true, true
		c.Bags = []character.ItemStack{{ItemID: int(num(r["id"])), Count: 1, Name: tc.item}}
		res := inventory.ReviewAll([]character.Character{c}, ds)
		got := "no line"
		action := inventory.Action("")
		if len(res.Lines) == 1 {
			action = res.Lines[0].Action
			got = string(action) + ": " + res.Lines[0].Reason
		}
		want := fmt.Sprintf("%s (required_level %d, class mask %d)", tc.want, num(r["required_level"]), num(r["allowable_class_mask"]))
		ok := action == tc.want && (tc.from == 0 || strings.Contains(got, "level "+strconv.FormatInt(tc.from, 10)))
		if tc.from > 0 && num(r["required_level"]) != tc.from {
			t.Fatalf("%s: required_level is %d, the case expects %d", tc.item, num(r["required_level"]), tc.from)
		}
		out = append(out, benchRow{fmt.Sprintf("%s (%s, level %d): bag review", tc.item, strings.ToLower(tc.class), tc.level), got, want, usabItemSrc + "; " + usabLevelSrc, resultIf(ok, "the action or the level it can be worn from")})
	}
	finish(t, area, title, out)
}

// TestOfflineBenchmarkGearReviewRelics: a libram, idol or totem is kept by the class that holds it, from the level the
// dataset gives, and sold by a class that cannot.
func TestOfflineBenchmarkGearReviewRelics(t *testing.T) {
	reviewBatch(t, "advice-gear-review-relics", "Offline gear: what the bag review says to do with a relic", reviewCases[:4])
}

// TestOfflineBenchmarkGearReviewWorn: shirts, tabards and cosmetic armor are kept by every class and never far below.
func TestOfflineBenchmarkGearReviewWorn(t *testing.T) {
	reviewBatch(t, "advice-gear-review-worn", "Offline gear: what the bag review says to do with a shirt, tabard or cosmetic piece", reviewCases[4:7])
}

// TestOfflineBenchmarkGearReviewArmor: plate that needs level 40 is kept by a warrior from then, sold by a mage; a bag
// is no verdict on worn gear.
func TestOfflineBenchmarkGearReviewArmor(t *testing.T) {
	reviewBatch(t, "advice-gear-review-armor", "Offline gear: what the bag review says to do with armor and a bag", reviewCases[7:])
}

var bindCases = []struct {
	item     string
	equip    bool // the answer says Bind on Equip
	pickup   bool // the answer says it binds when picked up
	describe string
}{
	{"Embossed Leather Vest", true, false, "crafted and dropped, bind on equip"}, {"Rough Bronze Shoulders", true, false, "crafted, bind on equip"},
	{"Robe of Power", false, true, "crafted, bind on pickup"}, {"Goblin Mining Helmet", false, true, "crafted, bind on pickup"},
	{"Pendant of Myzrael", false, true, "dropped, bind on pickup"}, {"Minor Recombobulator", false, false, "crafted, no binding"},
	{"Dimensional Ripper - Everlook", false, false, "crafted, bind on use"}, {"EZ-Thro Field Transporter: Gadgetzan", false, false, "crafted, bind on use"},
	{"Ryedol's Lucky Pick", false, false, "a quest item"},
}

var (
	bindEquip  = regexp.MustCompile(`(?i)bind on equip`)
	bindPickup = regexp.MustCompile(`(?i)bind on pickup|bound when picked up`)
	uniqueWord = regexp.MustCompile(`(?i)\bunique\b`)
)

// TestOfflineBenchmarkBindWording: the binding the answer for an item names, beside the dataset's bonding: Bind on Equip
// only for bonding 2, binds when picked up only for bonding 1, no binding named for bonding 0, 3 (on use) and 4 or 5 (quest).
func TestOfflineBenchmarkBindWording(t *testing.T) {
	ds := usabStore()
	var out []benchRow
	for _, tc := range bindCases {
		r := usabItem(t, tc.item)
		res := paths.For(horde(60), ds, num(r["id"]), paths.Options{})
		txt := res.Text()
		var got []string
		if bindEquip.MatchString(txt) {
			got = append(got, "Bind on Equip")
		}
		if bindPickup.MatchString(txt) {
			got = append(got, "Bind on Pickup")
		}
		if len(got) == 0 {
			got = []string{"no binding named"}
		}
		b := num(r["bonding"])
		ref := map[int64]string{0: "none", 1: "on pickup", 2: "on equip", 3: "on use", 4: "quest item", 5: "quest item"}[b]
		ok := bindEquip.MatchString(txt) == tc.equip && bindPickup.MatchString(txt) == tc.pickup && tc.equip == (b == 2) && tc.pickup == (b == 1)
		out = append(out, benchRow{tc.item + ": binding (" + tc.describe + ")", strings.Join(got, ", "), fmt.Sprintf("bonding %d (%s)", b, ref), usabBindSrc, resultIf(ok, "a binding the item does not have, or none for the one it has")})
	}
	finish(t, "advice-bind-wording", "Offline items: the binding named for an item", out)
}

// TestOfflineBenchmarkUniqueWording: the dataset has no unique-equipped flag, so the answer for a ring or trinket never
// calls one unique; and a ring or trinket worn in one slot is never offered for the other (one copy cannot fill both).
func TestOfflineBenchmarkUniqueWording(t *testing.T) {
	ds := usabStore()
	var out []benchRow
	for _, name := range []string{"Ring of Pure Silver", "Don Rodrigo's Band", "Thunderbrew's Boot Flask"} {
		r := usabItem(t, name)
		txt := paths.For(horde(60), ds, num(r["id"]), paths.Options{}).Text()
		out = append(out, benchRow{name + ": unique wording", firstLine(txt), "no unique flag in the dataset's items columns", usabNoneSrc, resultIf(!uniqueWord.MatchString(txt), "an item called unique with no unique flag in the data")})
	}
	for _, tc := range []struct {
		worn, other, item string
		level             int
	}{{"finger1", "finger2", "Defias Mage Ring", 30}, {"finger2", "finger1", "Defias Mage Ring", 30}, {"trinket1", "trinket2", "Thunderbrew's Boot Flask", 50}, {"trinket2", "trinket1", "Thunderbrew's Boot Flask", 50}} {
		r := usabItem(t, tc.item)
		id, il := int(num(r["id"])), int(num(r["item_level"]))
		c := usabChar("ROGUE", tc.level, "Horde")
		c.Equipment = map[string]character.EquippedItem{tc.worn: {Slot: tc.worn, ItemID: &id, ItemLevel: &il, Name: tc.item}}
		res, err := gear.Find(c, ds, "", gear.Options{NoSources: true})
		if err != nil {
			t.Fatal(err)
		}
		offered := false
		for _, cd := range res.Slots[tc.other] {
			offered = offered || cd.Item.ID == int64(id)
		}
		out = append(out, benchRow{fmt.Sprintf("%s worn in %s: offered for %s", tc.item, tc.worn, tc.other), fmt.Sprint(offered), "false: one copy fills one slot", usabItemSrc, resultIf(!offered, "the worn item offered for the other slot")})
	}
	finish(t, "advice-unique-wording", "Offline items: unique and two-slot wording", out)
}

var spellLine = regexp.MustCompile(`(?m)^- Level (\d+): (.+?) \((?:new spell|new rank of a spell you have)`)
var rankSuffix = regexp.MustCompile(`^(.*) \(Rank (\d+)\)$`)

var (
	talentOnce  sync.Once
	talentCache map[int64]map[string]bool
)

// talentSet is the lower-cased talent names of a class's trees, read straight from the dataset's talent_trees and
// talent_nodes (read once for the package).
func talentSet(t *testing.T, classID int64) map[string]bool {
	t.Helper()
	talentOnce.Do(func() {
		ds := usabStore()
		trees, err1 := ds.Filter("talent_trees", store.Range{Column: "tree_id", Min: int64(0)})
		nodes, err2 := ds.Filter("talent_nodes", store.Range{Column: "node_id", Min: int64(0)})
		if err1 != nil || err2 != nil {
			return
		}
		classOf := map[int64]int64{}
		for _, tr := range trees {
			classOf[num(tr["tree_id"])] = num(tr["class_id"])
		}
		talentCache = map[int64]map[string]bool{}
		for _, n := range nodes {
			c := classOf[num(n["tree_id"])]
			if talentCache[c] == nil {
				talentCache[c] = map[string]bool{}
			}
			talentCache[c][strings.ToLower(text(n["name"]))] = true
		}
	})
	if len(talentCache[classID]) == 0 {
		t.Fatalf("class %d: no talent names in the snapshot", classID)
	}
	return talentCache[classID]
}

func classRows(t *testing.T, classID int64, from, to int) []store.Row {
	t.Helper()
	rows, err := usabStore().Filter("class_spells", store.Range{Column: "class_id", Min: classID, Max: classID}, store.Range{Column: "level", Min: int64(from), Max: int64(to)})
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

// trainCases are a class, a faction and a level: the spells the advisor says to train over that level and the next.
var trainCases = []struct {
	class, faction string
	level          int
}{
	{"MAGE", "Horde", 10}, {"WARRIOR", "Alliance", 20}, {"PALADIN", "Alliance", 30}, {"PRIEST", "Horde", 14}, {"ROGUE", "Alliance", 16},
	{"HUNTER", "Horde", 24}, {"SHAMAN", "Horde", 30}, {"WARLOCK", "Alliance", 20}, {"DRUID", "Horde", 40}, {"MAGE", "Alliance", 40},
	{"WARRIOR", "Horde", 50}, {"ROGUE", "Horde", 60},
}

func classID(token string) int64 {
	for _, c := range usabClasses {
		if c.token == token {
			return c.id
		}
	}
	return 0
}

// TestOfflineBenchmarkClassSpellLevels: each spell the advisor says a class trains at a level, with its rank, beside the
// dataset's class_spells row: the level must be the row's, and a talent or a talent's effect is never called trainable.
func TestOfflineBenchmarkClassSpellLevels(t *testing.T) {
	ds := usabStore()
	var out []benchRow
	for _, tc := range trainCases {
		cid := classID(tc.class)
		facts, _, _ := levelup.Facts(usabChar(tc.class, tc.level, tc.faction), ds, nil, tc.level, tc.level+1)
		rows := classRows(t, cid, tc.level, tc.level+1)
		talents := talentSet(t, cid)
		var bad, listed []string
		for _, m := range spellLine.FindAllStringSubmatch(facts, -1) {
			lv, _ := strconv.Atoi(m[1])
			name, rank := m[2], int64(0)
			if rm := rankSuffix.FindStringSubmatch(name); rm != nil {
				name = rm[1]
				rank, _ = strconv.ParseInt(rm[2], 10, 64)
			}
			listed = append(listed, fmt.Sprintf("%s %d@%d", name, rank, lv))
			match := false
			for _, r := range rows {
				if text(r["name"]) == name && num(r["rank"]) == rank && num(r["level"]) == int64(lv) {
					match = true
					if rank == 0 && talents[strings.ToLower(name)] {
						bad = append(bad, name+" is a talent")
					}
				}
			}
			if !match {
				bad = append(bad, fmt.Sprintf("%s rank %d level %d is no row", name, rank, lv))
			}
		}
		ref := fmt.Sprintf("%d class_spells rows at levels %d to %d", len(rows), tc.level, tc.level+1)
		out = append(out, benchRow{fmt.Sprintf("%s (%s, level %d): spell levels", strings.ToLower(tc.class), tc.faction, tc.level), fmt.Sprintf("%d listed: %s", len(listed), strings.Join(listed, ", ")), ref, usabSpellSrc,
			resultIf(len(listed) > 0 && len(bad) == 0, strings.Join(bad, "; "))})
	}
	finish(t, "advice-class-spell-levels", "Offline class trainer: the level of each spell", out)
}

// TestOfflineBenchmarkClassTrainerCosts: the dataset has no cost for a class spell (trainer_spells holds recipes only), so
// the answer names none and says so.
func TestOfflineBenchmarkClassTrainerCosts(t *testing.T) {
	ds := usabStore()
	var out []benchRow
	for _, tc := range trainCases[:8] {
		rows := classRows(t, classID(tc.class), tc.level, tc.level+1)
		priced := 0
		for _, r := range rows {
			ts, _ := ds.Filter("trainer_spells", store.Range{Column: "spell_id", Min: num(r["spell_id"]), Max: num(r["spell_id"])})
			priced += len(ts)
		}
		facts, _, _ := levelup.Facts(usabChar(tc.class, tc.level, tc.faction), ds, nil, tc.level, tc.level+1)
		says := strings.Contains(facts, "do not give a price") && !regexp.MustCompile(`costs \d|\d+[gsc]\b`).MatchString(facts)
		out = append(out, benchRow{fmt.Sprintf("%s (level %d): training cost", strings.ToLower(tc.class), tc.level), firstLine(facts[strings.Index(facts, "Training costs"):]),
			fmt.Sprintf("%d trainer_spells rows for %d class spells: no cost in the dataset", priced, len(rows)), usabCostSrc, resultIf(priced == 0 && says, "a price, or no statement that none is known")})
	}
	finish(t, "advice-class-trainer-cost", "Offline class trainer: what training costs", out)
}

// capitals are the cities the dataset's weapon_masters name for a faction.
func capitals(t *testing.T, faction string) []string {
	t.Helper()
	rows, err := usabStore().Filter("weapon_masters", store.Range{Column: "npc_id", Min: int64(0), Max: int64(1 << 40)})
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, r := range rows {
		if text(r["faction"]) == faction && !slices.Contains(out, text(r["city"])) {
			out = append(out, text(r["city"]))
		}
	}
	slices.Sort(out)
	return out
}

var coordinates = regexp.MustCompile(`\d`)

// TestOfflineBenchmarkClassTrainerCity: the city the advisor sends each class of each side to, beside the cities the
// dataset's Weapon Masters stand in for that side, and no trainer name or position: the dataset has no class trainer.
func TestOfflineBenchmarkClassTrainerCity(t *testing.T) {
	var out []benchRow
	for _, faction := range []string{"Horde", "Alliance"} {
		caps := capitals(t, faction)
		for _, c := range usabClasses {
			pl, ok := levelup.TrainAt(usabChar(c.token, 30, faction), nil)
			if !ok {
				t.Fatalf("%s %s: no class city", faction, c.token)
			}
			line := pl.Line(c.token)
			named := pl.Trainer != nil || coordinates.MatchString(line)
			res := resultIf(slices.Contains(caps, pl.City) && !named, "a city the dataset does not hold, or a trainer or position named")
			if !slices.Contains(caps, pl.City) && !named {
				res = "differ (data): no dataset row places this side in " + pl.City
			}
			out = append(out, benchRow{fmt.Sprintf("%s %s: where to train", faction, strings.ToLower(c.token)), line, "side's cities: " + strings.Join(caps, ", "), usabCitySrc, res})
		}
	}
	finish(t, "advice-class-trainer-city", "Offline class trainer: the city", out)
}

var masterCases = []struct {
	faction, class string
	subclass       int
}{
	{"Horde", "WARRIOR", 6}, {"Horde", "SHAMAN", 13}, {"Horde", "HUNTER", 3}, {"Horde", "ROGUE", 18}, {"Horde", "DRUID", 5},
	{"Alliance", "PALADIN", 8}, {"Alliance", "MAGE", 10}, {"Alliance", "WARLOCK", 15}, {"Alliance", "HUNTER", 2}, {"Alliance", "ROGUE", 7},
}

// TestOfflineBenchmarkWeaponTrainerCost: a weapon skill a class can learn and lacks, the Weapon Master and the price and level
// the advisor names, beside the dataset's weapon_masters row and the skill's minimum level.
func TestOfflineBenchmarkWeaponTrainerCost(t *testing.T) {
	ds := usabStore()
	masters, err := ds.Filter("weapon_masters", store.Range{Column: "npc_id", Min: int64(0), Max: int64(1 << 40)})
	if err != nil {
		t.Fatal(err)
	}
	var out []benchRow
	for _, tc := range masterCases {
		c := usabChar(tc.class, 40, tc.faction)
		c.SkillsCaptured = true
		f := gear.LoadWeaponSkills(c, ds)
		_, need := f.ItemSkill(c, 2, tc.subclass)
		skill := weaponSkillOf[int64(tc.subclass)]
		wsr, _ := ds.Filter("weapon_skills", store.Range{Column: "skill_line_id", Min: skill, Max: skill})
		skillMin := int64(0)
		for _, r := range wsr {
			if num(r["class_id"]) == classID(tc.class) {
				skillMin = num(r["min_level"])
			}
		}
		var refs []string
		ok := need != nil
		got := "no skill needed"
		if need != nil {
			got = need.Line()
			if need.MinLevel > 1 {
				got += fmt.Sprintf(", from level %d", need.MinLevel)
			}
			found := false
			for _, m := range masters {
				if text(m["faction"]) != tc.faction {
					continue
				}
				for i, s := range ints64(m["skills"]) {
					if s != skill {
						continue
					}
					cost, lvl := ints64(m["cost_copper"])[i], max(ints64(m["min_level"])[i], skillMin)
					refs = append(refs, fmt.Sprintf("%s in %s %s, level %d", text(m["name"]), text(m["city"]), gear.CostText(cost), lvl))
					if text(m["name"]) == need.Master && need.Cost == cost && int64(need.MinLevel) == lvl {
						found = true
					}
				}
			}
			ok = found
		}
		out = append(out, benchRow{fmt.Sprintf("%s %s: train weapon skill %d", tc.faction, strings.ToLower(tc.class), skill), got, strings.Join(refs, "; "), usabMasterSr, resultIf(ok, "the master, cost or level")})
	}
	finish(t, "advice-weapon-trainer-cost", "Offline class trainer: weapon masters, price and level", out)
}

func ints64(v any) []int64 {
	l, _ := v.([]any)
	var out []int64
	for _, x := range l {
		out = append(out, num(x))
	}
	return out
}

// TestOfflineBenchmarkUsabilityNoAnswer: what the dataset cannot answer, the advisor says so and names none: a required level
// the dataset leaves 0, a class trainer's name or position, a spell's training price, a race restriction, a unique flag,
// a weapon skill no master teaches and the rested bonus.
func TestOfflineBenchmarkUsabilityNoAnswer(t *testing.T) {
	ds := usabStore()
	var out []benchRow
	for _, tc := range levelCases[8:] {
		r := usabItem(t, tc.item)
		res, _ := gear.Find(usabChar(tc.class, tc.level, "Horde"), ds, "", gear.Options{NoSources: true})
		cd, offered := found(res, num(r["id"]))
		out = append(out, benchRow{tc.item + ": level needed, none in the dataset", cd.LevelText(), fmt.Sprintf("required_level %d", num(r["required_level"])), usabLevelSrc, resultIf(offered && num(r["required_level"]) == 0 && strings.HasSuffix(cd.LevelText(), "(estimated)"), "an estimate not marked as one")})
	}
	for _, class := range []string{"MAGE", "ROGUE", "DRUID"} {
		pl, _ := levelup.TrainAt(usabChar(class, 30, "Horde"), nil)
		out = append(out, benchRow{strings.ToLower(class) + ": class trainer's name and position", pl.Line(class), "no class trainer in the dataset", usabNoneSrc, resultIf(pl.Trainer == nil && !coordinates.MatchString(pl.Line(class)), "an NPC or a position was named")})
	}
	facts, _, _ := levelup.Facts(usabChar("MAGE", 20, "Horde"), ds, nil, 20, 21)
	out = append(out, benchRow{"mage: price of training", firstLine(facts[strings.Index(facts, "Training costs"):]), "no class spell cost in the dataset", usabNoneSrc, resultIf(strings.Contains(facts, "do not give a price"), "a price was given")})
	races := regexp.MustCompile(`(?i)\b(human|orc|dwarf|gnome|troll|tauren|undead|night elf|race|racial)\b`)
	for _, name := range []string{"Hatchet", "Defias Mage Ring"} {
		r := usabItem(t, name)
		res, _ := gear.Find(usabChar("ROGUE", 30, "Horde"), ds, "", gear.Options{NoSources: true})
		cd, _ := found(res, num(r["id"]))
		txt := cd.Describe() + " " + cd.LevelText() + " " + paths.For(horde(60), ds, num(r["id"]), paths.Options{}).Text()
		out = append(out, benchRow{name + ": race restriction", firstLine(txt), "the dataset has no race mask", usabNoneSrc, resultIf(!races.MatchString(txt), "a race restriction was stated")})
	}
	for _, class := range []string{"PRIEST", "MAGE"} {
		c := usabChar(class, 30, "Horde")
		wand := usabItem(t, "Charred Razormane Wand")
		f := gear.LoadWeaponSkills(c, ds)
		_ = wand
		c.SkillsCaptured = true
		_, need := f.ItemSkill(c, 2, 19)
		line := "no skill needed"
		if need != nil {
			line = need.Line()
		}
		out = append(out, benchRow{class + ": who teaches Wands", line, "no weapon_masters row lists skill 228", usabNoneSrc, resultIf(need == nil || need.Master == "" && strings.Contains(line, "a Weapon Master"), "a master was named")})
	}
	xp, xpMax, rested := 1000, 5000, 2800
	c := usabChar("MAGE", 30, "Horde")
	c.XP, c.XPMax = &xp, &xpMax
	for _, withRested := range []bool{true, false} {
		c.RestedXP = nil
		if withRested {
			c.RestedXP = &rested
		}
		line := ""
		for _, l := range strings.Split(brief.Character(c, character.Client{}, time.Unix(0, 0)), "\n") {
			if strings.Contains(l, "rested") {
				line = l
			}
		}
		claims := regexp.MustCompile(`%|bonus|double|hours|inn\b`).MatchString(line)
		ok := !claims && (withRested && strings.Contains(line, "rested 2800 XP") || !withRested && strings.Contains(line, "rested XP not captured"))
		out = append(out, benchRow{fmt.Sprintf("rested XP (captured %v)", withRested), line, "the capture's own value; the dataset holds no rested rate", usabNoneSrc, resultIf(ok, "a rested bonus or rate was claimed or invented")})
	}
	finish(t, "advice-usability-no-answer", "Offline gear and class training: no answer in the dataset", out)
}

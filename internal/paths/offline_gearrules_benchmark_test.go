package paths_test

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/gear"
	"github.com/WoW-Companion/advisor/internal/inventory"
	"github.com/WoW-Companion/advisor/internal/learn"
	"github.com/WoW-Companion/advisor/internal/levelup"
	"github.com/WoW-Companion/advisor/internal/paths"
	"github.com/WoW-Companion/advisor/internal/store"
)

// The offline gear-rules benchmark: the level a class changes armor type (plate and mail at 40), the shields, off-hand
// weapons and held items a class can use and the level Dual Wield opens the off hand, the Enchanting recipes' skill and
// item type, and what the review says to do with ammo, quivers, ammo pouches and class bags, beside the dataset's rows,
// with no network. testdata/usability_snapshot holds the real rows (ammo, quivers, pouches and bags included, see its
// build.sh); the enchant rows come from testdata/tradeskills_snapshot. The dataset holds no armor proficiency spell, no
// bag size, no ammo-to-weapon link and no enchant item type or stat, so those references are the game's own rules written
// out here and the cases the dataset cannot answer check that the advisor says so and states none. A difference whose
// cause is a missing or wrong dataset row is marked "(data)". make offline-benchmark runs it; WOWC_BENCH_REPORT
// receives the tables.

const (
	grArmorSrc   = "dataset items (class_id 4, subclass_id, slot, required_level); game armor proficiency (plate and mail from 40); Warcraft Wiki armor pages"
	grHandSrc    = "dataset items (subclass_id, slot, allowable_class_mask, required_level); game Dual Wield levels (rogue 10, warrior and hunter 20, shaman 40); Warcraft Wiki Dual Wield, Shield and Held in Off-hand pages"
	grAmmoSrc    = "dataset items (class_id 6 ammo, subclass_id 2 arrow, 3 bullet, required_level); game: hunters, rogues and warriors use bows, crossbows and guns; Warcraft Wiki Ammunition page"
	grQuiverSrc  = "dataset items (class_id 11 quiver and ammo pouch, class_id 1 bags, allowable_class_mask); Warcraft Wiki Quiver, Ammo Pouch and Soul Bag pages"
	grEnchantSrc = "dataset recipes (skill_line_name Enchanting, learn_skill), profession_notes ranks; the recipe name gives the item type (Warcraft Wiki Enchanting pages)"
)

// grClassOf is a class token's dataset class_id.
func grClassOf(token string) int64 {
	for _, c := range usabClasses {
		if c.token == token {
			return c.id
		}
	}
	return 0
}

// ---- armor type by level ----

var grArmorCases = []struct {
	class, item string
	lo, hi      int // the levels either side of the change: 39 and 40, where the class gains plate or mail
}{
	{"WARRIOR", "Steel Plate Helm", 39, 40}, {"PALADIN", "Heavy Mithril Pants", 39, 40}, {"WARRIOR", "Scalemail Boots", 39, 40},
	{"HUNTER", "Scalemail Vest", 39, 40}, {"SHAMAN", "Chainmail Armor", 39, 40}, {"HUNTER", "Cured Leather Gloves", 39, 40},
	{"SHAMAN", "Cured Leather Boots", 39, 40}, {"ROGUE", "Scalemail Vest", 39, 40}, {"DRUID", "Cured Leather Gloves", 39, 40},
	{"PRIEST", "Thick Cloth Vest", 39, 40}, {"MAGE", "Thick Cloth Gloves", 39, 40}, {"WARLOCK", "Scalemail Boots", 39, 40},
}

// TestOfflineBenchmarkArmorTypeLevel: whether a class may wear an armor piece at the level before and at the level its armor
// type changes, beside the dataset's armor subclass and the game's rule: warriors and paladins wear mail then plate, hunters
// and shamans leather then mail, both from level 40; rogues and druids leather, priests, mages and warlocks cloth, always.
func TestOfflineBenchmarkArmorTypeLevel(t *testing.T) {
	var out []benchRow
	for _, tc := range grArmorCases {
		r := usabItem(t, tc.item)
		sub := num(r["subclass_id"])
		var got, want []string
		ok := true
		for _, lv := range []int{tc.lo, tc.hi} {
			a, known := gear.CanUse(tc.class, lv, r)
			w := sub == armorWorn(tc.class, lv)
			ok = ok && known && a == w
			got = append(got, fmt.Sprintf("level %d: %v", lv, a))
			want = append(want, fmt.Sprintf("level %d: %v", lv, w))
		}
		out = append(out, benchRow{fmt.Sprintf("%s (%s, armor subclass %d, required level %d): worn at %d and %d", tc.item, strings.ToLower(tc.class), sub, num(r["required_level"]), tc.lo, tc.hi),
			strings.Join(got, ", "), strings.Join(want, ", "), grArmorSrc, resultIf(ok, "the level the armor type changes")})
	}
	finish(t, "advice-armor-type-level", "Offline gear: the level a class changes armor type", out)
}

// TestOfflineBenchmarkArmorUpgradeReviewPlate: the bag review says to keep armor a class will wear from the level the dataset gives
// ("you can wear it from level N", judged at the level the item needs, so plate that needs 40 is a warrior's from 40).
func TestOfflineBenchmarkArmorUpgradeReviewPlate(t *testing.T) {
	reviewBatch(t, "advice-armor-upgrade-review-plate", "Offline gear: plate kept for the level it can be worn", []reviewCase{
		{"WARRIOR", 35, "Heavy Mithril Breastplate", inventory.Keep_, 41}, {"PALADIN", 33, "Ornate Mithril Pants", inventory.Keep_, 40},
	})
}

func TestOfflineBenchmarkArmorUpgradeReviewMail(t *testing.T) {
	reviewBatch(t, "advice-armor-upgrade-review-mail", "Offline gear: mail kept for the level it can be worn", []reviewCase{
		{"HUNTER", 45, "Knight-Captain's Chain Armguards", inventory.Keep_, 55},
	})
}

func TestOfflineBenchmarkArmorUpgradeReviewLeather(t *testing.T) {
	reviewBatch(t, "advice-armor-upgrade-review-leather", "Offline gear: leather kept for the level it can be worn", []reviewCase{
		{"DRUID", 36, "Wolfshead Helm", inventory.Keep_, 40}, {"ROGUE", 28, "Studded Hat", inventory.Keep_, 32},
	})
}

// TestOfflineBenchmarkArmorOtherTypeReview: the armor type a class never wears at its level is sold, not kept for later.
func TestOfflineBenchmarkArmorOtherTypeReview(t *testing.T) {
	reviewBatch(t, "advice-armor-other-type-review", "Offline gear: armor of a type the class does not wear", []reviewCase{
		{"SHAMAN", 36, "Chainmail Belt", inventory.Sell, 0}, {"HUNTER", 36, "Scalemail Vest", inventory.Sell, 0},
	})
}

func TestOfflineBenchmarkArmorOtherTypeReviewAlt(t *testing.T) {
	reviewBatch(t, "advice-armor-other-type-review-alt", "Offline gear: armor restricted to another class", []reviewCase{
		{"ROGUE", 50, "Wolfshead Helm", inventory.Sell, 0},
	})
}

// grShopCases: the Auction House search names the armor material the class wears at its level, though no example item sits in the level range.
var grShopCases = []struct {
	class string
	level int
	slot  string
	want  string
}{
	{"HUNTER", 20, "chest", "leather"}, {"ROGUE", 20, "legs", "leather"}, {"SHAMAN", 20, "chest", "leather"}, {"DRUID", 20, "chest", "leather"},
	{"WARRIOR", 42, "chest", "plate"}, {"PALADIN", 42, "legs", "plate"},
}

// TestOfflineBenchmarkArmorShopping: the Auction House search for an armor slot names the material the class wears at its level (the
// dataset's armor subclass of the base items, the game's armor rule where no base item is near), never a bare "chest".
func TestOfflineBenchmarkArmorShopping(t *testing.T) {
	ds := usabStore()
	var out []benchRow
	for _, tc := range grShopCases {
		s, ok := paths.ShoppingList(usabChar(tc.class, tc.level, "Horde"), ds, "", tc.slot, paths.Options{})
		got := "no list"
		if ok {
			got = s.Type + "; " + strings.Join(s.Steps[:1], " ")
		}
		out = append(out, benchRow{fmt.Sprintf("%s level %d: Auction House search for the %s slot", strings.ToLower(tc.class), tc.level, tc.slot), got,
			fmt.Sprintf("%s armor at level %d", tc.want, tc.level), grArmorSrc, resultIf(ok && s.Armor == tc.want && strings.HasPrefix(s.Type, tc.want+" ") && strings.Contains(s.Steps[0], strings.ToUpper(tc.want[:1])+tc.want[1:]), "the armor material left out of the search")})
	}
	finish(t, "advice-armor-shopping", "Offline gear: the armor material of an Auction House search", out)
}

// ---- weapons, shields and off-hand items ----

// grDualLevel is the level a class learns Dual Wield in the game; the dataset's class_spells holds no row for it.
var grDualLevel = map[string]int{"ROGUE": 10, "WARRIOR": 20, "HUNTER": 20, "SHAMAN": 40}

var grShieldClasses = map[string]bool{"WARRIOR": true, "PALADIN": true, "SHAMAN": true}

// grOffhand is whether the class may take the item in the off hand at the level: a shield for the shield classes, a weapon
// made for the off hand or a one-hander from the level Dual Wield is learned, a held-in-off-hand item for any class, and the
// item's own class mask and required level (the gear search looks two levels ahead).
func grOffhand(t *testing.T, r store.Row, class string, level int) bool {
	t.Helper()
	if !maskAllows(num(r["allowable_class_mask"]), grClassOf(class)) {
		return false
	}
	if num(r["required_level"]) > int64(level+2) {
		return false
	}
	switch {
	case num(r["class_id"]) == 4 && num(r["subclass_id"]) == 6:
		return grShieldClasses[class]
	case num(r["class_id"]) == 2:
		dw, ok := grDualLevel[class]
		return ok && level >= dw && learnable(t, grClassOf(class), weaponSkillOf[num(r["subclass_id"])])
	}
	return true
}

// grOffered is whether the gear search puts the item in a slot for the character.
func grOffered(t *testing.T, class string, level int, slot string, id int64) bool {
	t.Helper()
	res, err := gear.Find(usabChar(class, level, "Horde"), usabStore(), "", gear.Options{Slot: slot, NoSources: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range res.Slots[slot] {
		if c.Item.ID == id {
			return true
		}
	}
	return false
}

type grHandCase struct {
	class string
	level int
	item  string
}

func grHandBatch(t *testing.T, area, title, what string, cases []grHandCase) {
	var out []benchRow
	for _, tc := range cases {
		r := usabItem(t, tc.item)
		got := grOffered(t, tc.class, tc.level, "offhand", num(r["id"]))
		want := grOffhand(t, r, tc.class, tc.level)
		out = append(out, benchRow{fmt.Sprintf("%s level %d: %s in the off hand", strings.ToLower(tc.class), tc.level, tc.item), fmt.Sprintf("offered %v", got),
			fmt.Sprintf("offered %v (%s, slot %d, class mask %d, required level %d)", want, what, num(r["slot"]), num(r["allowable_class_mask"]), num(r["required_level"])), grHandSrc,
			resultIf(got == want, "who may hold it in the off hand, or from which level")})
	}
	finish(t, area, title, out)
}

// TestOfflineBenchmarkDualWieldLevel: a one-hand or off-hand weapon is offered for the off hand from the level the class learns
// Dual Wield (rogue 10, warrior and hunter 20, shaman 40) and never to a class that cannot dual wield.
func TestOfflineBenchmarkDualWieldLevel(t *testing.T) {
	grHandBatch(t, "advice-dual-wield-level", "Offline gear: the level the off hand opens for a weapon", "an off-hand weapon", []grHandCase{
		{"ROGUE", 9, "Left-Handed Brass Knuckles"}, {"ROGUE", 10, "Left-Handed Brass Knuckles"}, {"WARRIOR", 19, "Left-Handed Brass Knuckles"},
		{"WARRIOR", 20, "Left-Handed Brass Knuckles"}, {"HUNTER", 19, "Left-Handed Claw"}, {"HUNTER", 20, "Left-Handed Claw"},
		{"SHAMAN", 39, "Left-Handed Claw"}, {"SHAMAN", 40, "Left-Handed Claw"}, {"PALADIN", 60, "Left-Handed Claw"}, {"DRUID", 60, "Left-Handed Claw"},
		{"PRIEST", 60, "Worn Dagger"}, {"MAGE", 60, "Cutlass"},
	})
}

// TestOfflineBenchmarkShieldClasses: a shield is offered to the warrior, paladin and shaman and to no other class, and a shield
// the dataset limits to the paladin by its class mask to the paladin alone.
func TestOfflineBenchmarkShieldClasses(t *testing.T) {
	grHandBatch(t, "advice-shield-classes", "Offline gear: which classes carry a shield", "a shield", []grHandCase{
		{"WARRIOR", 30, "Banded Buckler"}, {"PALADIN", 30, "Banded Buckler"}, {"SHAMAN", 30, "Banded Buckler"}, {"ROGUE", 30, "Banded Buckler"},
		{"HUNTER", 30, "Banded Buckler"}, {"DRUID", 30, "Banded Buckler"}, {"PRIEST", 30, "Banded Buckler"}, {"WARLOCK", 30, "Banded Buckler"},
		{"PALADIN", 30, "Bastion of Stormwind"}, {"WARRIOR", 30, "Bastion of Stormwind"},
	})
}

// TestOfflineBenchmarkHeldInOffHand: an item held in the off hand goes to the classes its class mask names (an orb to the mage or
// warlock, a medicine bag to the druid) and to every class when the mask names none.
func TestOfflineBenchmarkHeldInOffHand(t *testing.T) {
	grHandBatch(t, "advice-held-off-hand", "Offline gear: items held in the off hand", "a held item", []grHandCase{
		{"MAGE", 40, "Celestial Orb"}, {"PRIEST", 40, "Celestial Orb"}, {"WARLOCK", 40, "Orb of Soran'ruk"}, {"MAGE", 40, "Orb of Soran'ruk"},
		{"DRUID", 40, "Veildust Medicine Bag"}, {"PRIEST", 40, "Veildust Medicine Bag"}, {"WARRIOR", 30, "Grayson's Torch"}, {"ROGUE", 30, "Grayson's Torch"},
	})
}

var grMainCases = []grHandCase{
	{"ROGUE", 40, "Maul"}, {"PALADIN", 40, "Maul"}, {"DRUID", 40, "Maul"}, {"PRIEST", 40, "Maul"}, {"DRUID", 40, "Hatchet"}, {"SHAMAN", 40, "Hatchet"},
	{"HUNTER", 40, "Bastard Sword"}, {"SHAMAN", 40, "Bastard Sword"}, {"MAGE", 40, "Worn Dagger"}, {"PALADIN", 40, "Worn Dagger"},
}

// TestOfflineBenchmarkMainHandWeapons: the main hand and two-hand weapons the gear search offers a class, beside the weapon skills
// the dataset says the class can learn (a two-hander is a main-hand weapon only).
func TestOfflineBenchmarkMainHandWeapons(t *testing.T) {
	var out []benchRow
	for _, tc := range grMainCases {
		r := usabItem(t, tc.item)
		got := grOffered(t, tc.class, tc.level, "mainhand", num(r["id"]))
		want := refUsable(t, r, tc.class, grClassOf(tc.class), tc.level)
		out = append(out, benchRow{fmt.Sprintf("%s level %d: %s in the main hand", strings.ToLower(tc.class), tc.level, tc.item), fmt.Sprintf("offered %v", got),
			fmt.Sprintf("offered %v (weapon skill %d, slot %d)", want, weaponSkillOf[num(r["subclass_id"])], num(r["slot"])), usabItemSrc, resultIf(got == want, "who can wield it")})
	}
	finish(t, "advice-main-hand-weapons", "Offline gear: main hand and two-hand weapons by class", out)
}

// ---- enchants ----

var grEnchantCases = []struct {
	recipe, slot string
	level, skill int
}{
	{"Enchant Bracer - Minor Stamina", "wrist", 10, 40}, {"Enchant Chest - Minor Intellect", "chest", 10, 15}, {"Enchant Cloak - Minor Resistance", "back", 12, 30},
	{"Enchant Shield - Minor Stamina", "shield", 8, 60}, {"Enchant 2H Weapon - Lesser Intellect", "two-hand weapon", 12, 95}, {"Enchant Weapon - Minor Striking", "weapon", 12, 95},
}

// grSlotWord names an item type an answer would claim.
var grSlotWord = regexp.MustCompile(`(?i)\b(bracers?|wrists?|chest|cloaks?|back|shields?|weapons?|boots|feet|gloves|hands)\b`)

// TestOfflineBenchmarkEnchantRequirements: the skill an enchant needs, the rank and the character level that skill is trained from,
// said to a character who is short of the skill (never "not yet" to one who has it), and the item type: the recipe's name gives it, the
// dataset has no column for it, so the answer must not name an item type of its own.
func TestOfflineBenchmarkEnchantRequirements(t *testing.T) {
	ds := consVendors{tsStore()}
	var out []benchRow
	for _, tc := range grEnchantCases {
		rec := rowByName(t, ds, "recipes", tc.recipe)
		need := num(rec["learn_skill"])
		c := tsChar("Horde", tc.level, tsSkill{"Enchanting", tc.skill, 150})
		res, err := learn.Find(c, ds, nil, tc.recipe)
		name, floor, level := rlRank(t, ds, need)
		ok := err == nil && int64(res.Gate.Skill) == need && res.Gate.Rank == name && int64(res.Gate.Level) == level &&
			strings.Contains(res.Text, fmt.Sprintf("It needs Enchanting %d.", need))
		if name != "" {
			notYet := int64(tc.level) < level && int64(tc.skill) < need
			ok = ok && strings.Contains(res.Text, "so not yet.") == notYet
		}
		out = append(out, benchRow{fmt.Sprintf("%s (level %d, Enchanting %d): skill and gate", tc.recipe, tc.level, tc.skill), fmt.Sprintf("skill %d, %s", res.Gate.Skill, firstLineOf(res.Gate.Line, "no rank")),
			fmt.Sprintf("learn_skill %d; %s", need, rlWant(name, floor, level)), grEnchantSrc, resultIf(ok, "the skill, the rank or a \"not yet\"")})
		out = append(out, benchRow{tc.recipe + ": the item it goes on", fmt.Sprintf("names an item type: %v", grSlotWord.MatchString(res.Text)), "the recipe name: " + tc.slot + "; no enchant slot column in the dataset",
			grEnchantSrc, resultIf(!grSlotWord.MatchString(res.Text), "an item type was claimed")})
	}
	finish(t, "advice-enchant-requirements", "Offline enchants: skill, rank and item type", out)
}

func firstLineOf(s, none string) string {
	if i := strings.Index(s, "Rank:"); i >= 0 && strings.TrimSpace(s[i+5:]) != "" {
		return strings.TrimSpace(s[i+5:])
	}
	return none
}

// ---- ammo, quivers and bags ----

type grBagCase struct {
	holder string // the character holding the item: class token and level
	level  int
	alts   []string // other characters, by class token
	item   string
	want   inventory.Action
	to     string // the class token of the alt a mail goes to
	says   string // a phrase the reason holds
	ref    string
}

func grBagChars(tc grBagCase, id int64) []character.Character {
	level := tc.level
	mk := func(class string, lv int) character.Character {
		c := usabChar(class, lv, "Alliance")
		c.Name, c.BagsCaptured, c.BankCaptured = strings.ToUpper(class[:1])+strings.ToLower(class[1:]), true, true
		return c
	}
	h := mk(tc.holder, level)
	h.Bags = []character.ItemStack{{ItemID: int(id), Count: 1, Name: tc.item}}
	chars := []character.Character{h}
	for _, a := range tc.alts {
		chars = append(chars, mk(a, 60))
	}
	return chars
}

func grBagBatch(t *testing.T, area, title, src string, cases []grBagCase) {
	var out []benchRow
	for _, tc := range cases {
		r := usabItem(t, tc.item)
		res := inventory.ReviewAll(grBagChars(tc, num(r["id"])), lgItemsOnly{usabStore()})
		got, action, to, reason := "no line", inventory.Action(""), "", ""
		for _, l := range res.Lines {
			if l.Character == strings.ToUpper(tc.holder[:1])+strings.ToLower(tc.holder[1:]) {
				action, to, reason = l.Action, l.To, l.Reason
				got = fmt.Sprintf("%s%s: %s", action, map[bool]string{true: " to " + to}[to != ""], reason)
			}
		}
		wantTo := ""
		if tc.to != "" {
			wantTo = strings.ToUpper(tc.to[:1]) + strings.ToLower(tc.to[1:])
		}
		ok := action == tc.want && to == wantTo && strings.Contains(reason, tc.says)
		label := fmt.Sprintf("%s (%s, level %d", tc.item, strings.ToLower(tc.holder), tc.level)
		if len(tc.alts) > 0 {
			label += ", alt " + strings.ToLower(strings.Join(tc.alts, " "))
		}
		out = append(out, benchRow{label + "): bag review", got, fmt.Sprintf("%s (%s; class %d, subclass %d, class mask %d, required level %d)", tc.want, tc.ref, num(r["class_id"]), num(r["subclass_id"]),
			num(r["allowable_class_mask"]), num(r["required_level"])), src, resultIf(ok, "the action, the alt it goes to or the reason")})
	}
	finish(t, area, title, out)
}

// TestOfflineBenchmarkAmmoReview: ammo is kept by the classes that use it (hunters, and warriors and rogues who carry a bow, crossbow
// or gun), from the level the dataset gives, and a class that cannot use ammo sends it to an alt that can, else sells it.
func TestOfflineBenchmarkAmmoReview(t *testing.T) {
	grBagBatch(t, "advice-ammo-review", "Offline ammo: who keeps it, from which level, who passes it on", grAmmoSrc, []grBagCase{
		{"HUNTER", 30, nil, "Razor Arrow", inventory.Keep_, "", "the best arrows", "an arrow, required level 25"},
		{"HUNTER", 20, nil, "Razor Arrow", inventory.Keep_, "", "from level 25", "an arrow, required level 25"},
		{"HUNTER", 60, nil, "Thorium Shells", inventory.Keep_, "", "the best bullets", "a bullet, required level 52"},
		{"HUNTER", 40, nil, "Thorium Shells", inventory.Keep_, "", "from level 52", "a bullet, required level 52"},
		{"WARRIOR", 40, nil, "Crafted Heavy Shot", inventory.Keep_, "", "the best bullets", "a bullet; warriors use guns"},
		{"ROGUE", 40, nil, "Explosive Arrow", inventory.Keep_, "", "the best arrows", "an arrow; rogues use bows and crossbows"},
		{"MAGE", 40, nil, "Razor Arrow", inventory.Sell, "", "cannot use ammo", "a mage has no ranged weapon that fires ammo"},
		{"PRIEST", 60, nil, "Hi-Impact Mithril Slugs", inventory.Sell, "", "cannot use ammo", "a priest has no ranged weapon that fires ammo"},
		{"MAGE", 40, []string{"HUNTER"}, "Razor Arrow", inventory.Mail, "HUNTER", "can use arrows", "an alt hunter uses it"},
		{"DRUID", 60, []string{"PALADIN", "WARRIOR"}, "Crafted Heavy Shot", inventory.Mail, "WARRIOR", "can use bullets", "an alt warrior uses it"},
	})
}

// TestOfflineBenchmarkQuiverReview: a quiver or ammo pouch is for a class that uses ammo; a class that cannot use ammo mails it to an
// alt that can, else sells it. The dataset holds no bag size, so one the holder can use is left unplaced.
func TestOfflineBenchmarkQuiverReview(t *testing.T) {
	grBagBatch(t, "advice-quiver-review", "Offline quivers and ammo pouches: who can use one", grQuiverSrc, []grBagCase{
		{"HUNTER", 40, nil, "Heavy Quiver", inventory.Unsure, "", "no bag size", "a quiver the hunter can use; no slot count"},
		{"HUNTER", 40, nil, "Thick Leather Ammo Pouch", inventory.Unsure, "", "no bag size", "an ammo pouch the hunter can use; no slot count"},
		{"PRIEST", 50, nil, "Heavy Quiver", inventory.Sell, "", "cannot use ammo", "a priest has no ammo to put in it"},
		{"MAGE", 50, nil, "Heavy Leather Ammo Pouch", inventory.Sell, "", "cannot use ammo", "a mage has no ammo to put in it"},
		{"PRIEST", 50, []string{"HUNTER"}, "Heavy Quiver", inventory.Mail, "HUNTER", "can use it", "an alt hunter uses it"},
		{"SHAMAN", 50, []string{"MAGE", "ROGUE"}, "Quickdraw Quiver", inventory.Mail, "ROGUE", "can use it", "an alt rogue can use ammo"},
	})
}

// TestOfflineBenchmarkClassBagReview: a bag the dataset limits to a class (a Soul Pouch or Felcloth Bag is the warlock's) is kept by
// that class, mailed to an alt of that class by any other, and otherwise sold; a bag with no class limit is left unplaced.
func TestOfflineBenchmarkClassBagReview(t *testing.T) {
	grBagBatch(t, "advice-class-bag-review", "Offline bags: a bag limited to a class", grQuiverSrc, []grBagCase{
		{"WARLOCK", 50, nil, "Soul Pouch", inventory.Unsure, "", "no bag size", "class mask 256: the warlock"},
		{"HUNTER", 50, nil, "Soul Pouch", inventory.Sell, "", "class restriction", "class mask 256 leaves out the hunter"},
		{"PRIEST", 60, []string{"WARLOCK"}, "Felcloth Bag", inventory.Mail, "WARLOCK", "can use it", "class mask 256: an alt warlock"},
		{"MAGE", 40, nil, "Linen Bag", inventory.Unsure, "", "no bag size", "a plain bag has no class limit"},
		{"DRUID", 60, nil, "Enchanted Mageweave Pouch", inventory.Unsure, "", "no bag size", "no class mask in the dataset"},
		{"WARRIOR", 60, nil, "Cenarion Herb Bag", inventory.Unsure, "", "no bag size", "no class mask in the dataset"},
	})
}

// ---- no answer in the dataset ----

var grNumbered = regexp.MustCompile(`(?i)\b\d+[- ]?(slots?|bags?)\b|\b(speed|haste)\b|\d+%|\bdps\b|\bdamage\b`)

// TestOfflineBenchmarkGearRulesNoAnswer: what the dataset cannot answer, the advisor says so and states none: a bag's slot count,
// a quiver's ammo speed, the damage of ammo, an enchant's stats, and the training that opens plate or mail.
func TestOfflineBenchmarkGearRulesNoAnswer(t *testing.T) {
	var out []benchRow
	review := func(class, item string) string {
		r := usabItem(t, item)
		c := usabChar(class, 50, "Alliance")
		c.Name, c.BagsCaptured, c.BankCaptured = "Benchtest", true, true
		c.Bags = []character.ItemStack{{ItemID: int(num(r["id"])), Count: 1, Name: item}}
		res := inventory.ReviewAll([]character.Character{c}, lgItemsOnly{usabStore()})
		if len(res.Lines) != 1 {
			return "no line"
		}
		return string(res.Lines[0].Action) + ": " + res.Lines[0].Reason
	}
	for _, name := range []string{"Mageweave Bag", "Runecloth Bag"} {
		got := review("MAGE", name)
		out = append(out, benchRow{name + ": how many slots", got, "no bag size column in items", usabNoneSrc, resultIf(strings.Contains(got, "no bag size") && !grNumbered.MatchString(got), "a slot count was given")})
	}
	got := review("HUNTER", "Quickdraw Quiver")
	out = append(out, benchRow{"Quickdraw Quiver: its ammo speed bonus and slots", got, "no bag size and no speed column in items", usabNoneSrc, resultIf(strings.Contains(got, "no bag size") && !grNumbered.MatchString(got), "a speed bonus or slot count was given")})
	got = review("HUNTER", "Thorium Shells")
	out = append(out, benchRow{"Thorium Shells: the damage it adds", got, "no damage on ammo rows (dmg_min and dmg_max are empty)", usabNoneSrc, resultIf(!grNumbered.MatchString(got), "ammo damage was given")})
	got = review("HUNTER", "Linen Bag")
	out = append(out, benchRow{"Linen Bag: the level to carry it (required level 0)", got, "required_level 0 in the dataset", usabLevelSrc, resultIf(!regexp.MustCompile(`(?i)level \d+`).MatchString(got), "a level was invented")})
	res, err := learn.Find(tsChar("Horde", 30), consVendors{tsStore()}, nil, "Enchant Bracer - Minor Stamina")
	out = append(out, benchRow{"Enchant Bracer - Minor Stamina: how much Stamina it gives", firstLine(res.Text), "no enchant stat or amount in the dataset", usabNoneSrc, resultIf(err == nil && !regexp.MustCompile(`\+\d+|\bstamina\b`).MatchString(strings.ToLower(res.Text)), "an enchant bonus was stated")})
	for _, class := range []string{"WARRIOR", "HUNTER"} {
		facts, _, _ := levelup.Facts(usabChar(class, 40, "Horde"), usabStore(), nil, 39, 41)
		out = append(out, benchRow{strings.ToLower(class) + " levels 39 to 41: the training that opens plate or mail", firstLine(facts), "class_spells holds no Plate Mail, Mail or Leather proficiency (the 40 rule is the game's)", usabNoneSrc,
			resultIf(!slices.ContainsFunc([]string{"plate mail", "proficiency"}, func(w string) bool { return strings.Contains(strings.ToLower(facts), w) }), "a proficiency was named")})
	}
	finish(t, "advice-gear-rules-no-answer", "Offline gear rules: no answer in the dataset", out)
}

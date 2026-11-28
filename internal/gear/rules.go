package gear

import (
	"strings"

	"github.com/WoW-Companion/advisor/internal/store"
)

// Item-table values (see the data repo's items table: class_id, subclass_id, slot).
const (
	classWeapon = 2
	classArmor  = 4
)

// Armor subclass ids.
const (
	armorCloth   = 1
	armorLeather = 2
	armorMail    = 3
	armorPlate   = 4
	armorShield  = 6
)

// Weapon subclass ids.
const (
	axe1H    = 0
	axe2H    = 1
	bow      = 2
	gun      = 3
	mace1H   = 4
	mace2H   = 5
	polearm  = 6
	sword1H  = 7
	sword2H  = 8
	staff    = 10
	fist     = 13
	dagger   = 15
	thrown   = 16
	crossbow = 18
	wand     = 19
)

// classRules says what a class may equip. The rules, kept deliberately small:
//
//   - Armor in the slots head, shoulder, chest, waist, legs, feet, wrist and hands must be of the
//     class's one armor type (the type it gets its armor bonus from); lighter types are not
//     suggested even where the game lets the class wear them. Warriors and paladins wear mail, and
//     hunters and shamans leather, until level ArmorLevel, when the class gains its own type.
//   - Cloaks, necks, rings and trinkets are never restricted by armor type.
//   - Shields only for classes with Shield set.
//   - Weapons by subclass from Weapons. A one-hand weapon goes in the off hand only for classes
//     with DualWield. Two-handers are compared against the main hand only.
//   - Shirts, tabards and bags are not considered.
type classRules struct {
	ID        int // the game's class id, matched against an item's allowable_class_mask
	Armor     int
	Shield    bool
	DualWield bool
	Weapons   []int
	// LowArmor is the armor type worn before level ArmorLevel; 0 when Armor is worn from level 1.
	LowArmor   int
	ArmorLevel int
}

// armorGainLevel is the level at which the plate and mail classes gain their armor type: Classic Era
// trainer spells Plate Mail (warrior, paladin) and Mail (hunter, shaman), both learned at 40. Not
// verifiable from the dataset, which holds no spell or trainer data; see docs/gear.md.
const armorGainLevel = 40

// armorAt is the armor type the class is assumed to wear at level.
func (r classRules) armorAt(level int) int {
	if r.LowArmor != 0 && level < r.ArmorLevel {
		return r.LowArmor
	}
	return r.Armor
}

// rules is keyed by the class token a ForeverScribe capture writes. Classes without an entry are
// reported as unsupported rather than guessed.
var rules = map[string]classRules{
	"WARRIOR": {1, armorPlate, true, true, []int{axe1H, axe2H, bow, crossbow, dagger, fist, gun, mace1H, mace2H, polearm, staff, sword1H, sword2H, thrown}, armorMail, armorGainLevel},
	"PALADIN": {2, armorPlate, true, false, []int{axe1H, axe2H, mace1H, mace2H, polearm, sword1H, sword2H}, armorMail, armorGainLevel},
	"HUNTER":  {3, armorMail, false, true, []int{axe1H, axe2H, bow, crossbow, dagger, fist, gun, polearm, staff, sword1H, sword2H, thrown}, armorLeather, armorGainLevel},
	"ROGUE":   {4, armorLeather, false, true, []int{axe1H, bow, crossbow, dagger, fist, gun, mace1H, sword1H, thrown}, 0, 0},
	"PRIEST":  {5, armorCloth, false, false, []int{dagger, mace1H, staff, wand}, 0, 0},
	"SHAMAN":  {7, armorMail, true, true, []int{axe1H, axe2H, dagger, fist, mace1H, mace2H, staff}, armorLeather, armorGainLevel},
	"MAGE":    {8, armorCloth, false, false, []int{dagger, staff, sword1H, wand}, 0, 0},
	"WARLOCK": {9, armorCloth, false, false, []int{dagger, staff, sword1H, wand}, 0, 0},
	"DRUID":   {11, armorLeather, false, false, []int{dagger, fist, mace1H, mace2H, polearm, staff}, 0, 0},
}

// Item slots (inventory types) by the capture slots they fit.
var slotsOf = map[int8][]string{
	1: {"head"}, 2: {"neck"}, 3: {"shoulder"}, 5: {"chest"}, 20: {"chest"}, 6: {"waist"}, 7: {"legs"},
	8: {"feet"}, 9: {"wrist"}, 10: {"hands"}, 11: {"finger1", "finger2"}, 12: {"trinket1", "trinket2"},
	16: {"back"}, 13: {"mainhand", "offhand"}, 21: {"mainhand"}, 17: {"mainhand"}, 14: {"offhand"},
	22: {"offhand"}, 23: {"offhand"}, 15: {"ranged"}, 25: {"ranged"}, 26: {"ranged"},
}

// Slots whose armor is not restricted by armor type.
var anyArmorSlot = map[int8]bool{2: true, 11: true, 12: true, 16: true, 23: true}

// fitsSlot lists the capture slots an item of inventory type it may fill for this class.
func (r classRules) fitsSlots(slot int8) []string {
	out := slotsOf[slot]
	if slot == 13 && !r.DualWield {
		return []string{"mainhand"}
	}
	return out
}

// canUse reports whether a class at level may equip an item of the given class, subclass and slot.
func (r classRules) canUse(classID, subclass int8, slot int8, level int) bool {
	switch classID {
	case classArmor:
		if slot == 14 || int(subclass) == armorShield {
			return r.Shield
		}
		return anyArmorSlot[slot] || int(subclass) == r.armorAt(level)
	case classWeapon:
		if slot == 22 && !r.DualWield {
			return false
		}
		for _, w := range r.Weapons {
			if int(subclass) == w {
				return true
			}
		}
	}
	return false
}

// allowsClass reads an item's allowable_class_mask: 0 and -1 mean every class.
func allowsClass(mask int32, classID int) bool {
	return mask <= 0 || mask&(1<<(classID-1)) != 0
}

// maskClasses is the game's classes by the bit an allowable_class_mask uses (1 << (id-1)), as a plural for a sentence.
var maskClasses = []struct {
	id   uint
	name string
}{{1, "warriors"}, {2, "paladins"}, {3, "hunters"}, {4, "rogues"}, {5, "priests"}, {7, "shamans"}, {8, "mages"}, {9, "warlocks"}, {11, "druids"}}

// ClassRestricted is true for an item whose allowable_class_mask leaves a class out (a Rage Potion, a poison): 0 and -1
// mean every class.
func ClassRestricted(row store.Row) bool {
	mask := i64(row["allowable_class_mask"])
	if mask <= 0 {
		return false
	}
	for _, c := range maskClasses {
		if mask&(1<<(c.id-1)) == 0 {
			return true
		}
	}
	return false
}

// ClassList names the classes an item's allowable_class_mask allows: "warriors and druids".
func ClassList(row store.Row) string {
	mask := i64(row["allowable_class_mask"])
	var names []string
	for _, c := range maskClasses {
		if mask <= 0 || mask&(1<<(c.id-1)) != 0 {
			names = append(names, c.name)
		}
	}
	switch len(names) {
	case 0:
		return "other classes"
	case 1:
		return names[0]
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}

// ArmorTypeWord is an armor subclass as a player says it ("mail"); "" for anything else, a weapon or
// a shield included.
func ArmorTypeWord(classID, subclass int) string {
	if classID != classArmor {
		return ""
	}
	switch subclass {
	case armorCloth:
		return "cloth"
	case armorLeather:
		return "leather"
	case armorMail:
		return "mail"
	case armorPlate:
		return "plate"
	}
	return ""
}

// ArmorFor is the armor type word ("mail") the class wears at level by the rules above; "" for a class without rules.
func ArmorFor(class string, level int) string {
	rl, ok := rules[strings.ToUpper(class)]
	if !ok {
		return ""
	}
	return ArmorTypeWord(classArmor, rl.armorAt(level))
}

// AllowsClass reads an item row's allowable_class_mask for the class token: allowed is false when the mask leaves the
// class out, and known is false for a class without rules or a row of nothing.
func AllowsClass(class string, row store.Row) (allowed, known bool) {
	rl, ok := rules[strings.ToUpper(class)]
	if !ok || row == nil {
		return false, false
	}
	return allowsClass(int32(i64(row["allowable_class_mask"])), rl.ID), true
}

// ArmorLadder is the armor the class wears by the rules above: the type worn from level 1 (low), the level it changes
// at (0 when it never does) and the type worn from then (high), and whether the class carries a shield. ok is false for
// a class without rules.
func ArmorLadder(class string) (low string, gainLevel int, high string, shield, ok bool) {
	rl, found := rules[strings.ToUpper(class)]
	if !found {
		return "", 0, "", false, false
	}
	high = ArmorTypeWord(classArmor, rl.Armor)
	low = high
	if rl.LowArmor != 0 {
		low, gainLevel = ArmorTypeWord(classArmor, rl.LowArmor), rl.ArmorLevel
	}
	return low, gainLevel, high, rl.Shield, true
}

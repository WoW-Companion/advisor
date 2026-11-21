package gear

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
//     suggested even where the game lets the class wear them.
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
}

// rules is keyed by the class token a ForeverScribe capture writes. Classes without an entry are
// reported as unsupported rather than guessed.
var rules = map[string]classRules{
	"WARRIOR": {1, armorPlate, true, true, []int{axe1H, axe2H, bow, crossbow, dagger, fist, gun, mace1H, mace2H, polearm, staff, sword1H, sword2H, thrown}},
	"PALADIN": {2, armorPlate, true, false, []int{axe1H, axe2H, mace1H, mace2H, polearm, sword1H, sword2H}},
	"HUNTER":  {3, armorMail, false, true, []int{axe1H, axe2H, bow, crossbow, dagger, fist, gun, polearm, staff, sword1H, sword2H, thrown}},
	"ROGUE":   {4, armorLeather, false, true, []int{axe1H, bow, crossbow, dagger, fist, gun, mace1H, sword1H, thrown}},
	"PRIEST":  {5, armorCloth, false, false, []int{dagger, mace1H, staff, wand}},
	"SHAMAN":  {7, armorMail, true, true, []int{axe1H, axe2H, dagger, fist, mace1H, mace2H, staff}},
	"MAGE":    {8, armorCloth, false, false, []int{dagger, staff, sword1H, wand}},
	"WARLOCK": {9, armorCloth, false, false, []int{dagger, staff, sword1H, wand}},
	"DRUID":   {11, armorLeather, false, false, []int{dagger, fist, mace1H, mace2H, polearm, staff}},
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

// canUse reports whether the class may equip an item of the given class, subclass and slot.
func (r classRules) canUse(classID, subclass int8, slot int8) bool {
	switch classID {
	case classArmor:
		if slot == 14 || int(subclass) == armorShield {
			return r.Shield
		}
		return anyArmorSlot[slot] || int(subclass) == r.Armor
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

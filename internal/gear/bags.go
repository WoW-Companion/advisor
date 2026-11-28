package gear

import (
	"sort"
	"strings"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/store"
)

// BagUpgrade is an item already in the character's bags that beats what they wear in a slot: free,
// and usable now.
type BagUpgrade struct {
	Slot string
	Item Item
	// Name is the item's name with its random suffix ("of the Tiger") when the capture has one.
	Name string
	// Likely is true when the item is better only by item level (quality breaks a tie), not by
	// weighed stats: the advice says "likely better (item level N vs M)" and gives no number.
	Likely bool
	Empty  bool // the slot is empty
	Delta  float64
	// ReplacesILvl is the worn item's item level; 0 for an empty slot.
	ReplacesILvl int
	// Notes are debug lines, such as an enchant left out of the weighing.
	Notes []string `json:"-"`
	// Replaces is the worn item's name; empty for an empty slot.
	Replaces string
	// Need is the weapon skill the item needs and the character can learn but has not; nil otherwise.
	Need *SkillNeed
}

// equipLocs maps the addon's equipLoc strings to inventory types.
var equipLocs = map[string]int8{
	"INVTYPE_HEAD": 1, "INVTYPE_NECK": 2, "INVTYPE_SHOULDER": 3, "INVTYPE_CHEST": 5, "INVTYPE_ROBE": 20,
	"INVTYPE_WAIST": 6, "INVTYPE_LEGS": 7, "INVTYPE_FEET": 8, "INVTYPE_WRIST": 9, "INVTYPE_HAND": 10,
	"INVTYPE_FINGER": 11, "INVTYPE_TRINKET": 12, "INVTYPE_CLOAK": 16,
}

// bagItem is a bag stack resolved to an Item, from the dataset or from the addon's own fields.
func bagItem(st character.ItemStack, ds store.DataStore, rl classRules, level int) (Item, bool) {
	if ds != nil {
		if row, err := ds.Get("items", int64(st.ItemID)); err == nil && row != nil {
			it := itemOf(row)
			if it.ItemLevel <= 0 || it.Quality <= 0 || it.Quality == qualityArtifact || unequippableBonding(it.Bonding) {
				return Item{}, false
			}
			if !rl.canUse(int8(i64(row["class_id"])), int8(i64(row["subclass_id"])), int8(it.Slot), level) ||
				!allowsClass(int32(i64(row["allowable_class_mask"])), rl.ID) {
				return Item{}, false
			}
			it.Captured, it.Enchant = capturedStats(st.Stats), derefInt(st.Enchant)
			it.ReqLevel, it.Estimated = ReqLevelOf(it.RequiredLevel, it.ItemLevel)
			return it, true
		}
	}
	// Not in the dataset: only what the addon recorded, and only with every field present. The
	// addon gives no armor or weapon type, so only the slots that no armor type restricts count.
	if st.Quality == nil || st.ItemLevel == nil || st.ReqLevel == nil || st.EquipLoc == "" {
		return Item{}, false
	}
	slot, ok := equipLocs[strings.ToUpper(st.EquipLoc)]
	if !ok || !anyArmorSlot[slot] || *st.Quality <= 0 || *st.Quality == qualityArtifact || *st.ItemLevel <= 0 {
		return Item{}, false
	}
	return Item{ID: int64(st.ItemID), Name: st.Name, ItemLevel: *st.ItemLevel, RequiredLevel: *st.ReqLevel,
		ReqLevel: *st.ReqLevel, Quality: *st.Quality, Slot: int(slot),
		Captured: capturedStats(st.Stats), Enchant: derefInt(st.Enchant)}, true
}

func derefInt(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

// BagUpgrades lists the items in the character's bags that beat what they wear, best first by slot
// order. group is the top talent group name; empty when unknown. Items the dataset does not know
// count only when the addon recorded their quality, level, required level and slot. A character
// whose bags were not captured, or a class without rules, has none.
func BagUpgrades(c character.Character, ds store.DataStore, group string) []BagUpgrade {
	if c.Level == nil || len(c.Bags) == 0 {
		return nil
	}
	rl, ok := rules[strings.ToUpper(c.Class)]
	if !ok {
		return nil
	}
	level := *c.Level
	if ds != nil {
		rl.DualWield = rl.DualWield && knowsDualWield(c, ds, strings.ToUpper(c.Class))
	} else {
		rl.DualWield = false
	}
	w := namedWeights(c, group)
	wsk := LoadWeaponSkills(c, ds)
	var lo loadout
	if ds != nil {
		lo = loadoutOf(c, ds, group)
	}

	type option struct {
		up   BagUpgrade
		gain float64
	}
	var opts []option
	seen := map[int64]bool{}
	for _, st := range c.Bags {
		if seen[int64(st.ItemID)] {
			continue
		}
		seen[int64(st.ItemID)] = true
		it, ok := bagItem(st, ds, rl, level)
		if !ok || it.ReqLevel > level {
			continue
		}
		usable, need := wsk.ItemSkill(c, it.Class, it.Subclass)
		if !usable {
			continue // the class can never learn the weapon's skill
		}
		name := it.Name
		if name == "" {
			name = st.Name
		}
		if name == "" {
			continue
		}
		if st.Suffix != "" && !strings.Contains(name, st.Suffix) {
			name += " " + st.Suffix
		}
		for _, slot := range rl.fitsSlots(int8(it.Slot)) {
			if ds != nil && !lo.allows(slot, it) {
				continue
			}
			cur, worn, known := wornFor(c, ds, slot)
			if worn && !known {
				continue // something is worn there that we know nothing about
			}
			if worn && cur.ID == it.ID {
				continue
			}
			// The dataset holds no unique-equipped flag, so an item worn in one slot of a pair (rings, trinkets) is not
			// offered for the other, as the finder never offers it: one copy fills one slot.
			if p := pairOf(slot); p != "" && wornID(c, p) == it.ID {
				continue
			}
			up := BagUpgrade{Slot: slot, Item: it, Name: name, Empty: !worn, Likely: true, Need: need}
			if worn {
				up.Replaces = c.Equipment[slot].Name
				if up.Replaces == "" {
					up.Replaces = cur.Name
				}
			}
			// A two-hander also takes the off hand, so it is weighed against both hands together, as the finder does.
			var off *Item
			if slot == "mainhand" && it.Slot == invTwoHand && lo.off != nil {
				off = lo.off
			}
			var gain float64
			switch {
			case !worn:
				if off != nil {
					continue
				}
				gain = 1e12 + fallbackValue(it)
			case w != nil && it.statMap() != nil && cur.statMap() != nil && (off == nil || off.statMap() != nil):
				gain = statSum(it.statMap(), w) - statSum(cur.statMap(), w)
				if off != nil {
					gain -= statSum(off.statMap(), w)
				}
				up.Likely = false
				up.Notes = append(enchantNote(it, "bag item"), enchantNote(cur, "worn item")...)
			case off != nil:
				// No stats to weigh: item level decides, against what both hands hold.
				gain = fallbackValue(it) - fallbackValue(cur) - fallbackValue(*off)
			case it.ItemLevel > cur.ItemLevel || it.ItemLevel == cur.ItemLevel && it.Quality > cur.Quality:
				// No stats to weigh: item level decides, and quality only breaks a tie.
				gain = fallbackValue(it) - fallbackValue(cur)
			default:
				continue
			}
			if worn && gain <= 0 {
				continue
			}
			up.Delta, up.ReplacesILvl = gain, cur.ItemLevel
			opts = append(opts, option{up, gain})
		}
	}
	sort.SliceStable(opts, func(i, j int) bool {
		if opts[i].gain != opts[j].gain {
			return opts[i].gain > opts[j].gain
		}
		return opts[i].up.Item.ID < opts[j].up.Item.ID
	})
	// Each item and each slot is used once, best first.
	usedItem, usedSlot := map[int64]bool{}, map[string]bool{}
	var out []BagUpgrade
	for _, o := range opts {
		if usedItem[o.up.Item.ID] || usedSlot[o.up.Slot] {
			continue
		}
		usedItem[o.up.Item.ID], usedSlot[o.up.Slot] = true, true
		out = append(out, o.up)
	}
	order := map[string]int{}
	for i, s := range slotOrder {
		order[s] = i + 1
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := order[out[i].Slot], order[out[j].Slot]
		if a != b {
			if a == 0 || b == 0 {
				return a != 0
			}
			return a < b
		}
		return out[i].Slot < out[j].Slot
	})
	return out
}

// slotOrder is the capture's slot order, as the panel lists slots.
var slotOrder = []string{"head", "neck", "shoulder", "back", "chest", "wrist", "hands", "waist", "legs", "feet",
	"finger1", "finger2", "trinket1", "trinket2", "mainhand", "offhand", "ranged"}

// pairOf is the other slot of a pair of slots that hold the same kind of item (rings, trinkets); "" for any other slot.
func pairOf(slot string) string {
	for _, p := range pairedSlots {
		switch slot {
		case p[0]:
			return p[1]
		case p[1]:
			return p[0]
		}
	}
	return ""
}

// wornID is the id of the item the capture records in slot; 0 for an empty slot.
func wornID(c character.Character, slot string) int64 {
	if eq, ok := c.Equipment[slot]; ok && eq.ItemID != nil {
		return int64(*eq.ItemID)
	}
	return 0
}

// wornFor is the item worn in slot: worn is false for an empty slot; known is false when something is
// worn but neither the dataset nor the capture says enough about it.
func wornFor(c character.Character, ds store.DataStore, slot string) (it Item, worn, known bool) {
	eq, has := c.Equipment[slot]
	if !has || (eq.ItemID == nil || *eq.ItemID == 0) && eq.ItemLevel == nil {
		return Item{}, false, true
	}
	if ds != nil && eq.ItemID != nil && *eq.ItemID != 0 {
		if row, err := ds.Get("items", int64(*eq.ItemID)); err == nil {
			it = itemOf(row)
		}
	}
	if eq.ItemID != nil {
		it.ID = int64(*eq.ItemID)
	}
	if eq.ItemLevel != nil {
		it.ItemLevel = *eq.ItemLevel
	}
	if eq.Quality != nil {
		it.Quality = *eq.Quality
	}
	if it.Name == "" {
		it.Name = eq.Name
	}
	it.ReqLevel = it.RequiredLevel
	it.Captured, it.Enchant = capturedStats(eq.Stats), derefInt(eq.Enchant)
	return it, true, it.ItemLevel > 0
}

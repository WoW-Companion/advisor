package gear

import (
	"strings"

	"github.com/WoW-Companion/advisor/internal/brief"
	"github.com/WoW-Companion/advisor/internal/character"
	itemdata "github.com/WoW-Companion/advisor/internal/items"
	"github.com/WoW-Companion/advisor/internal/store"
)

// Inventory types of weapons and off-hand items.
const (
	invOneHand  = 13
	invShield   = 14
	invTwoHand  = 17
	invMainHand = 21
	invOffHand  = 22
	invHeld     = 23
)

// dualWieldLevel is the level a class learns Dual Wield when the dataset's class_spells has no row for
// it (Classic Era trainers). Classes not listed never dual wield.
var dualWieldLevel = map[string]int{"WARRIOR": 20, "HUNTER": 20, "ROGUE": 10, "SHAMAN": 40}

// knowsDualWield reports whether the character has learned Dual Wield: from the dataset's class_spells
// level for it when there is one, else the table above. The capture holds no known spells, so the
// class and level are what there is; a level not reached means not learned.
func knowsDualWield(c character.Character, ds store.DataStore, class string) bool {
	if c.Level == nil {
		return false
	}
	need, ok := dualWieldLevel[class]
	if !ok {
		return false
	}
	if cid, found := classIDs[class]; found {
		if rows, err := ds.Filter("class_spells", store.Range{Column: "class_id", Min: cid, Max: cid}); err == nil {
			for _, r := range rows {
				if n, _ := r["name"].(string); n == "Dual Wield" {
					if lv, ok := r["level"].(int64); ok && lv > 0 {
						need = int(lv)
					}
					break
				}
			}
		}
	}
	return *c.Level >= need
}

var classIDs = map[string]int64{"WARRIOR": 1, "PALADIN": 2, "HUNTER": 3, "ROGUE": 4, "PRIEST": 5, "SHAMAN": 7, "MAGE": 8, "WARLOCK": 9, "DRUID": 11}

// loadout is what the character wields now and whether their play style lets the weapon style change.
type loadout struct {
	main, off *Item
	// free is true when the stored play style is a two-hand or dual-wield damage one: the weapon style
	// may change. Otherwise the current style (one-hand and shield, dual wield, two-hand) is kept.
	free bool
}

// freeSpecs are the specs that fight with two hands or two weapons.
var freeSpecs = map[string]bool{"arms": true, "fury": true, "enhancement": true, "retribution": true, "combat": true, "assassination": true, "subtlety": true, "feral": true}

func loadoutOf(c character.Character, ds store.DataStore, group string) loadout {
	var lo loadout
	if m, ok := wornItem(c, ds, "mainhand"); ok {
		lo.main = &m
	}
	if o, ok := wornItem(c, ds, "offhand"); ok {
		lo.off = &o
	}
	if p := c.Profile; p != nil && strings.EqualFold(p.Role, "damage") {
		spec := strings.ToLower(p.Spec)
		if spec == "" {
			spec = strings.ToLower(group)
		}
		for name := range freeSpecs {
			if spec != "" && strings.Contains(spec, name) {
				lo.free = true
			}
		}
		if spec == "" && strings.EqualFold(c.Class, "WARRIOR") {
			lo.free = true
		}
	}
	return lo
}

// wornItem is the item worn in a slot, from the dataset; ok is false when empty or not in the dataset.
func wornItem(c character.Character, ds store.DataStore, slot string) (Item, bool) {
	eq, has := c.Equipment[slot]
	if !has || eq.ItemID == nil || *eq.ItemID == 0 {
		return Item{}, false
	}
	row, err := ds.Get("items", int64(*eq.ItemID))
	if err != nil {
		return Item{}, false
	}
	it := itemOf(row)
	if eq.ItemLevel != nil {
		it.ItemLevel = *eq.ItemLevel
	}
	if eq.Quality != nil {
		it.Quality = *eq.Quality
	}
	return it, true
}

// allows reports whether the weapon style lets a candidate into a slot: no two-hander over a one-hand
// and off-hand, no weapon for a shield, no off hand beside a two-hander, unless the play style is free.
func (lo loadout) allows(slot string, it Item) bool {
	switch slot {
	case "mainhand":
		if it.Slot == invTwoHand {
			return lo.free || lo.off == nil
		}
		return lo.free || lo.main == nil || lo.main.Slot != invTwoHand
	case "offhand":
		if lo.free && it.Slot == invShield {
			return false // a two-hand or dual-wield damage build has no use for a shield
		}
		if lo.main != nil && lo.main.Slot == invTwoHand {
			return false // a two-hander leaves no off hand
		}
		if lo.free || lo.off == nil {
			return true
		}
		if lo.off.Slot == invShield {
			return it.Slot == invShield // a shield is replaced by a better shield
		}
		return it.Slot != invShield // a weapon or held item keeps the style
	}
	return true
}

// Fits reports whether the item suits the character's build at all: a shield never does for a
// two-hand or dual-wield damage build, and the other faction's PvP rank gear never does. An item that does not fit is left out of
// the advice entirely.
func Fits(c character.Character, ds store.DataStore, itemID int64) bool {
	row, err := ds.Get("items", itemID)
	if err != nil {
		return true
	}
	it := itemOf(row)
	if itemdata.WrongFaction(it.Name, c.Faction) {
		return false
	}
	group := ""
	if names, _ := brief.TopGroups(c.Talents); len(names) == 1 {
		group = names[0]
	}
	lo := loadoutOf(c, ds, group)
	return it.Slot != invShield || lo.allows("offhand", it)
}

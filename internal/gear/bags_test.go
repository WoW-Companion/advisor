package gear

import (
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/store"
)

const feetSlot, handsSlot, cloakSlot, mail = 8, 10, 16, 3

func bagStore(t *testing.T) store.DataStore {
	t.Helper()
	return store.New(store.ReaderSource{"items": table(t, []itemRow{
		it(1, "Plain Boots", 8, 1, 1, 4, cloth, feetSlot),
		it(2, "Test Mail Boots", 10, 4, 2, 4, mail, feetSlot),
		it(3, "Test Mail Gauntlets", 12, 7, 2, 4, mail, handsSlot),
		it(4, "Plain Gauntlets", 10, 1, 1, 4, mail, handsSlot),
		it(5, "Silk Slippers", 30, 5, 3, 4, cloth, feetSlot),
		it(6, "Worn Mail Boots", 5, 1, 1, 4, mail, feetSlot),
		it(7, "Test Digging Pick", 15, 1, 2, 2, 20, 17),
		it(8, "Silk Cloak", 12, 5, 2, 4, 0, cloakSlot),
		it(9, "Paladin Boots", 20, 4, 3, 4, mail, feetSlot),
		it(10, "Future Boots", 14, 14, 2, 4, mail, feetSlot),
	})})
}

func bagWarrior(bags ...character.ItemStack) character.Character {
	c := char("WARRIOR", 10, map[string]character.EquippedItem{
		"feet":  {Slot: "feet", ItemID: ip(1)},
		"hands": {Slot: "hands", ItemID: ip(4)},
	})
	c.Bags = bags
	return c
}

func stack(id int) character.ItemStack { return character.ItemStack{ItemID: id, Count: 1} }

func bagSlots(us []BagUpgrade) string {
	var out []string
	for _, u := range us {
		out = append(out, u.Slot+"="+u.Name)
	}
	return strings.Join(out, ",")
}

func TestBagUpgradesLevelTenWarrior(t *testing.T) {
	ds := bagStore(t)
	got := BagUpgrades(bagWarrior(stack(2), stack(3)), ds, "")
	if want := "hands=Test Mail Gauntlets,feet=Test Mail Boots"; bagSlots(got) != want {
		// slot order is the panel's: hands before feet
		t.Fatalf("got %s, want %s", bagSlots(got), want)
	}
	for _, u := range got {
		if !u.Likely || u.Replaces == "" && !u.Empty {
			t.Errorf("%s: want a likely-better item that names what it replaces: %+v", u.Slot, u)
		}
	}
}

func TestBagUpgradesIgnoreWorseUnusableAndTools(t *testing.T) {
	ds := bagStore(t)
	bags := []character.ItemStack{stack(6), stack(5), stack(7), stack(10), stack(1)}
	if got := BagUpgrades(bagWarrior(bags...), ds, ""); len(got) != 0 {
		t.Fatalf("worse, cloth, tool, too-high and already-worn items must not be offered: %s", bagSlots(got))
	}
}

func TestBagUpgradesCloakIsAnyArmor(t *testing.T) {
	got := BagUpgrades(bagWarrior(stack(8)), bagStore(t), "")
	if bagSlots(got) != "back=Silk Cloak" || !got[0].Empty {
		t.Fatalf("a cloth cloak fits an empty cloak slot for a mail wearer: %s", bagSlots(got))
	}
}

func TestBagUpgradesFallbackOrder(t *testing.T) {
	// Null stats: quality beats item level, then item level beats required level.
	ds := store.New(store.ReaderSource{"items": table(t, []itemRow{
		it(1, "Worn Ring", 20, 5, 1, 4, 0, ringSlot),
		it(2, "Green Ring", 10, 5, 2, 4, 0, ringSlot),
		it(3, "White High Ring", 30, 5, 1, 4, 0, ringSlot),
	})})
	c := char("WARRIOR", 10, map[string]character.EquippedItem{
		"finger1": {Slot: "finger1", ItemID: ip(1)}, "finger2": {Slot: "finger2", ItemID: ip(1)}})
	c.Bags = []character.ItemStack{stack(2), stack(3)}
	got := BagUpgrades(c, ds, "")
	if len(got) != 2 || got[0].Item.Name != "Green Ring" || got[1].Item.Name != "White High Ring" {
		t.Fatalf("got %s", bagSlots(got))
	}
	for _, u := range got {
		if !u.Likely {
			t.Errorf("%s: no stats to weigh, so only likely better", u.Name)
		}
	}
}

func TestBagUpgradesItemMissingFromDataset(t *testing.T) {
	ds := bagStore(t)
	fields := character.ItemStack{ItemID: 900, Count: 1, Name: "Test Charm", Quality: ip(2), ItemLevel: ip(11), ReqLevel: ip(6),
		EquipLoc: "INVTYPE_FINGER", Suffix: "of the Test"}
	got := BagUpgrades(bagWarrior(fields), ds, "")
	if len(got) == 0 || got[0].Name != "Test Charm of the Test" {
		t.Fatalf("an item with the addon's fields should count: %s", bagSlots(got))
	}
	bare := character.ItemStack{ItemID: 901, Count: 1, Name: "Test Charm"}
	if got := BagUpgrades(bagWarrior(bare), ds, ""); len(got) != 0 {
		t.Fatalf("an item with no dataset row and no fields is skipped: %s", bagSlots(got))
	}
	partial := fields
	partial.ReqLevel = nil
	if got := BagUpgrades(bagWarrior(partial), ds, ""); len(got) != 0 {
		t.Fatalf("a missing required level is skipped: %s", bagSlots(got))
	}
}

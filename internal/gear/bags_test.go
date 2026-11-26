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
	// Null stats: item level decides; a higher-quality lower-level ring is no upgrade.
	ds := store.New(store.ReaderSource{"items": table(t, []itemRow{
		it(1, "Worn Ring", 20, 5, 1, 4, 0, ringSlot),
		it(2, "Green Ring", 10, 5, 2, 4, 0, ringSlot),
		it(3, "White High Ring", 30, 5, 1, 4, 0, ringSlot),
	})})
	c := char("WARRIOR", 10, map[string]character.EquippedItem{
		"finger1": {Slot: "finger1", ItemID: ip(1)}, "finger2": {Slot: "finger2", ItemID: ip(1)}})
	c.Bags = []character.ItemStack{stack(2), stack(3)}
	got := BagUpgrades(c, ds, "")
	if len(got) != 1 || got[0].Item.Name != "White High Ring" || !got[0].Likely {
		t.Fatalf("got %s", bagSlots(got))
	}
	if w := got[0].LikelyWording(); w != "likely better (item level 30 vs 20)" {
		t.Errorf("wording %q", w)
	}
}

// bootStore holds leather boots whose stats the dataset does not know.
func bootStore(t *testing.T) store.DataStore {
	t.Helper()
	return store.New(store.ReaderSource{"items": table(t, []itemRow{
		it(9001, "Test Worn Boots", 36, 10, 2, 4, leather, feetSlot),
		it(9002, "Test Blue Boots", 15, 10, 3, 4, leather, feetSlot),
		it(9003, "Test Agile Boots", 20, 10, 2, 4, leather, feetSlot),
		it(9004, "Test Green Boots", 40, 25, 2, 4, leather, feetSlot),
	})})
}

func rogueBoots(worn, bag character.ItemStack, enchant int) character.Character {
	c := char("ROGUE", 32, map[string]character.EquippedItem{"feet": {Slot: "feet", ItemID: ip(worn.ItemID),
		ItemLevel: worn.ItemLevel, Quality: worn.Quality, Name: worn.Name, Stats: worn.Stats}})
	c.Profile = &character.Profile{Role: "damage", Spec: "Combat"}
	bag.Enchant = &enchant
	c.Bags = []character.ItemStack{bag}
	return c
}

func boot(id, ilvl, q int, name string, st map[string]float64) character.ItemStack {
	return character.ItemStack{ItemID: id, Count: 1, Name: name, Quality: ip(q), ItemLevel: ip(ilvl), ReqLevel: ip(10),
		EquipLoc: "INVTYPE_FEET", Stats: st}
}

func TestBagUpgradesCapturedStats(t *testing.T) {
	ds := bootStore(t)
	worn := boot(9001, 36, 2, "Test Worn Boots", map[string]float64{"armor": 71, "agi": 5, "sta": 7, "spi": 5})
	worse := boot(9002, 15, 3, "Test Blue Boots", map[string]float64{"armor": 53, "agi": 4, "sta": 4})
	if got := BagUpgrades(rogueBoots(worn, worse, 8484), ds, ""); len(got) != 0 {
		t.Fatalf("a downgrade in stats must not be offered: %s", bagSlots(got))
	}
	better := boot(9003, 20, 2, "Test Agile Boots", map[string]float64{"armor": 60, "agi": 9, "sta": 7})
	got := BagUpgrades(rogueBoots(worn, better, 8484), ds, "")
	if len(got) != 1 || got[0].Likely || !strings.Contains(got[0].Line(), "better") || strings.Contains(got[0].Line(), "likely") {
		t.Fatalf("a real stat upgrade is plainly better: %+v", got)
	}
	if len(got[0].Notes) == 0 || !strings.Contains(got[0].Notes[0], "8484") {
		t.Errorf("the ignored enchant is noted: %v", got[0].Notes)
	}
}

func TestBagUpgradesNoStatsLowerItemLevel(t *testing.T) {
	ds := bootStore(t)
	worn := boot(9001, 36, 2, "Test Worn Boots", nil)
	low := boot(9002, 15, 3, "Test Blue Boots", nil)
	if got := BagUpgrades(rogueBoots(worn, low, 0), ds, ""); len(got) != 0 {
		t.Fatalf("a lower item level is never an upgrade by quality: %s", bagSlots(got))
	}
	high := boot(9004, 40, 2, "Test Green Boots", nil)
	low.ItemLevel = ip(15)
	worn = boot(9001, 36, 3, "Test Worn Boots", nil)
	got := BagUpgrades(rogueBoots(worn, high, 0), ds, "")
	if len(got) != 1 || !got[0].Likely || got[0].LikelyWording() != "likely better (item level 40 vs 36)" {
		t.Fatalf("a higher item level is likely better: %+v", got)
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

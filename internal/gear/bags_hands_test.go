package gear

import (
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/store"
)

const (
	ringSlotInv    = 11
	trinketSlotInv = 12
	axe1h, mace1h  = 0, 4
	sword2h        = 8
)

func handsStore(t *testing.T) store.DataStore {
	t.Helper()
	return store.New(store.ReaderSource{"items": table(t, []itemRow{
		it(1, "Test Hatchet", 16, 11, 1, classWeapon, axe1h, invOneHand),
		it(2, "Test Mace", 14, 9, 1, classWeapon, mace1h, invOneHand),
		it(3, "Test Falx", 26, 21, 1, classWeapon, sword2h, invTwoHand),
		it(4, "Test Great Falx", 40, 25, 1, classWeapon, sword2h, invTwoHand),
		it(5, "Test Band", 20, 15, 2, classArmor, 0, ringSlotInv),
		it(6, "Test Other Band", 18, 15, 2, classArmor, 0, ringSlotInv),
		it(7, "Test Charm", 20, 15, 2, classArmor, 0, trinketSlotInv),
		it(8, "Test Other Charm", 18, 15, 2, classArmor, 0, trinketSlotInv),
	})})
}

func worn(eq map[string]int) map[string]character.EquippedItem {
	il := map[int]int{1: 16, 2: 14, 5: 20, 6: 18, 7: 20, 8: 18}
	out := map[string]character.EquippedItem{}
	for slot, id := range eq {
		out[slot] = character.EquippedItem{Slot: slot, ItemID: ip(id), ItemLevel: ip(il[id])}
	}
	return out
}

func furyWarrior(level int, eq map[string]int, bag ...int) character.Character {
	c := char("WARRIOR", level, worn(eq))
	c.Profile = &character.Profile{Role: "damage", Spec: "Fury"}
	for _, id := range bag {
		c.Bags = append(c.Bags, stack(id))
	}
	return c
}

// A two-hander takes both hands, so a bag one is an upgrade only over the main hand and the off hand together, as the finder weighs it.
func TestBagUpgradesTwoHanderWeighsBothHands(t *testing.T) {
	ds := handsStore(t)
	for _, tc := range []struct {
		name string
		eq   map[string]int
		bag  int
		want string
	}{
		{"item level 26 against a dual-wield pair of 16 and 14", map[string]int{"mainhand": 1, "offhand": 2}, 3, ""},
		{"item level 40 against the same pair", map[string]int{"mainhand": 1, "offhand": 2}, 4, "mainhand=Test Great Falx"},
		{"item level 26 against a main hand of 16 and an empty off hand", map[string]int{"mainhand": 1}, 3, "mainhand=Test Falx"},
	} {
		if got := bagSlots(BagUpgrades(furyWarrior(30, tc.eq, tc.bag), ds, "Fury")); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

// A ring or trinket worn in one slot is not offered for the pair's other slot (the dataset has no unique-equipped flag, and the finder never
// offers it), while a different one fills the other slot.
func TestBagUpgradesWornCopyNotOfferedForThePairedSlot(t *testing.T) {
	ds := handsStore(t)
	for _, tc := range []struct {
		name string
		eq   map[string]int
		bag  int
		want string
	}{
		{"ring worn in the first slot", map[string]int{"finger1": 5}, 5, ""},
		{"ring worn in the second slot", map[string]int{"finger2": 5}, 5, ""},
		{"another ring for the empty second slot", map[string]int{"finger1": 5}, 6, "finger2=Test Other Band"},
		{"trinket worn in the first slot", map[string]int{"trinket1": 7}, 7, ""},
		{"trinket worn in the second slot", map[string]int{"trinket2": 7}, 7, ""},
		{"another trinket for the empty first slot", map[string]int{"trinket2": 7}, 8, "trinket1=Test Other Charm"},
	} {
		if got := bagSlots(BagUpgrades(furyWarrior(30, tc.eq, tc.bag), ds, "Fury")); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestPairOf(t *testing.T) {
	for slot, want := range map[string]string{"finger1": "finger2", "finger2": "finger1", "trinket1": "trinket2", "trinket2": "trinket1", "neck": "", "mainhand": ""} {
		if got := pairOf(slot); got != want {
			t.Errorf("pairOf(%q) = %q, want %q", slot, got, want)
		}
	}
}

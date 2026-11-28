package inventory

import (
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/store"
)

// consDS: food and drinks of three levels, a class potion for warriors and druids, a rogue's bound poison, a rogue and
// druid tea and a bound drink. Ids: 1 Plain Water, 2 Sweet Nectar, 3 Cured Ham Steak, 4 Morning Glory Dew, 5 Rage Elixir,
// 6 Venom Vial (bound), 7 Thistle Brew, 8 Bound Cider.
func consDS(t *testing.T) store.DataStore {
	row := func(id int64, name string, rl, mask, bonding int32) xItem {
		x := xi(id, name, 1, 0, 5, nil)
		x.RequiredLevel, x.ClassMask, x.Bonding = rl, mask, bonding
		return x
	}
	return store.New(store.ReaderSource{"items": table(t, []xItem{
		row(1, "Plain Water", 1, -1, 0), row(2, "Sweet Nectar", 25, -1, 0), row(3, "Cured Ham Steak", 35, -1, 0),
		row(4, "Morning Glory Dew", 45, -1, 0), row(5, "Rage Elixir", 25, 1025, 0), row(6, "Venom Vial", 44, 8, 1),
		row(7, "Thistle Brew", 5, 1032, 0), row(8, "Bound Cider", 5, -1, 1),
	})})
}

func stack(id int, name string) character.ItemStack {
	return character.ItemStack{ItemID: id, Count: 3, Name: name}
}

func review(t *testing.T, chars ...character.Character) CrossResult {
	t.Helper()
	return ReviewAll(chars, consDS(t))
}

func holder(name, class string, level int, bags ...character.ItemStack) character.Character {
	return character.Character{Name: name, Class: class, Level: &level, BagsCaptured: true, BankCaptured: true, Bags: bags}
}

func TestNectarAndDewAreDrinksNotFood(t *testing.T) {
	// a level 50 mage holds a level 35 food and a level 25 drink: the food is no better drink, so the nectar stays
	r := review(t, holder("Mage", "MAGE", 50, stack(1, "Plain Water"), stack(2, "Sweet Nectar"), stack(3, "Cured Ham Steak")))
	if l := find(r.Lines, "Mage", "Sweet Nectar"); l.Action != Keep_ || !strings.Contains(l.Reason, "best drink") {
		t.Fatalf("Sweet Nectar = %s: %s", l.Action, l.Reason)
	}
	if l := find(r.Lines, "Mage", "Plain Water"); l.Action != Sell || !strings.Contains(l.Reason, "better drink") {
		t.Fatalf("Plain Water = %s: %s", l.Action, l.Reason)
	}
	if l := find(r.Lines, "Mage", "Cured Ham Steak"); l.Action != Keep_ || !strings.Contains(l.Reason, "best food") {
		t.Fatalf("Cured Ham Steak = %s: %s", l.Action, l.Reason)
	}
	// the dew a level 50 priest holds beside the nectar is the better drink
	r = review(t, holder("Priest", "PRIEST", 50, stack(2, "Sweet Nectar"), stack(4, "Morning Glory Dew")))
	if l := find(r.Lines, "Priest", "Sweet Nectar"); l.Action != Sell {
		t.Fatalf("Sweet Nectar beside the dew = %s: %s", l.Action, l.Reason)
	}
}

func TestDrinkIsNoUseToAClassWithNoMana(t *testing.T) {
	for _, class := range []string{"WARRIOR", "ROGUE"} {
		r := review(t, holder("Solo", class, 50, stack(2, "Sweet Nectar")))
		if l := find(r.Lines, "Solo", "Sweet Nectar"); l.Action != Sell || !strings.Contains(l.Reason, "no mana") {
			t.Fatalf("%s alone: %s: %s", class, l.Action, l.Reason)
		}
	}
	// a mage alt gets it, a bound one cannot be mailed, and a warrior's food is kept
	r := review(t, holder("Solo", "WARRIOR", 50, stack(2, "Sweet Nectar"), stack(8, "Bound Cider"), stack(3, "Cured Ham Steak")), holder("Alt", "MAGE", 50))
	if l := find(r.Lines, "Solo", "Sweet Nectar"); l.Action != Mail || l.To != "Alt" {
		t.Fatalf("with a mage alt: %s to %q: %s", l.Action, l.To, l.Reason)
	}
	if l := find(r.Lines, "Solo", "Bound Cider"); l.Action != Sell {
		t.Fatalf("a bound drink = %s to %q: %s", l.Action, l.To, l.Reason)
	}
	if l := find(r.Lines, "Solo", "Cured Ham Steak"); l.Action != Keep_ {
		t.Fatalf("a warrior's food = %s: %s", l.Action, l.Reason)
	}
	// another warrior is no home for a drink
	r = review(t, holder("Solo", "WARRIOR", 50, stack(2, "Sweet Nectar")), holder("Alt", "ROGUE", 50))
	if l := find(r.Lines, "Solo", "Sweet Nectar"); l.Action != Sell {
		t.Fatalf("with a rogue alt: %s to %q", l.Action, l.To)
	}
}

func TestClassPotionIsForItsClasses(t *testing.T) {
	r := review(t, holder("Mage", "MAGE", 50, stack(5, "Rage Elixir")))
	if l := find(r.Lines, "Mage", "Rage Elixir"); l.Action != Sell || !strings.Contains(l.Reason, "warriors and druids") {
		t.Fatalf("mage alone: %s: %s", l.Action, l.Reason)
	}
	r = review(t, holder("Mage", "MAGE", 50, stack(5, "Rage Elixir")), holder("Bear", "DRUID", 10))
	if l := find(r.Lines, "Mage", "Rage Elixir"); l.Action != Mail || l.To != "Bear" {
		t.Fatalf("with a druid alt: %s to %q: %s", l.Action, l.To, l.Reason)
	}
	// bound to its holder, a poison cannot be mailed to the rogue
	r = review(t, holder("Mage", "MAGE", 60, stack(6, "Venom Vial")), holder("Stab", "ROGUE", 60))
	if l := find(r.Lines, "Mage", "Venom Vial"); l.Action != Sell {
		t.Fatalf("a bound poison: %s to %q: %s", l.Action, l.To, l.Reason)
	}
	// its own class is never told to sell it
	for _, class := range []string{"WARRIOR", "DRUID"} {
		r = review(t, holder("Own", class, 60, stack(5, "Rage Elixir")))
		if l := find(r.Lines, "Own", "Rage Elixir"); l.Action == Sell || l.Action == Mail {
			t.Fatalf("%s: %s: %s", class, l.Action, l.Reason)
		}
	}
}

func TestClassTeaIsNoDrinkToRankAgainstWater(t *testing.T) {
	// a rogue's tea is not a drink to rank: it is neither sold for having no mana nor the cause of the water being sold
	r := review(t, holder("Stab", "ROGUE", 50, stack(7, "Thistle Brew")))
	if l := find(r.Lines, "Stab", "Thistle Brew"); l.Action == Sell || l.Action == Mail {
		t.Fatalf("rogue's tea: %s: %s", l.Action, l.Reason)
	}
	r = review(t, holder("Bear", "DRUID", 50, stack(7, "Thistle Brew"), stack(1, "Plain Water")))
	if l := find(r.Lines, "Bear", "Plain Water"); l.Action != Keep_ {
		t.Fatalf("water beside a class tea = %s: %s", l.Action, l.Reason)
	}
}

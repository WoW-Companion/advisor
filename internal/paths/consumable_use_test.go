package paths

import (
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
)

const (
	rageElixir = 40 // a warrior and druid potion needing level 25
	plainTonic = 41 // any class, no level gate
	gatedFood  = 42 // any class, needs level 35
)

func consumable(id int64, name string, req, mask int32) itemRow {
	r := item(id, name, 20, req, 1, 1, 0, 0)
	r.ClassID, r.ClassMask = 0, mask
	return r
}

func useText(t *testing.T, class string, level int, id int64) Result {
	t.Helper()
	ds := fixtureWith(t, consumable(rageElixir, "Rage Elixir", 25, 1025), consumable(plainTonic, "Plain Tonic", 1, -1), consumable(gatedFood, "Heavy Stew", 35, -1))
	c := character.Character{Class: class, Level: &level}
	return For(c, ds, id, Options{})
}

func TestConsumableAnswerNamesTheLevelItNeeds(t *testing.T) {
	r := useText(t, "WARRIOR", 10, rageElixir)
	if !strings.Contains(r.Text(), "It needs level 25 to use; you are level 10.") {
		t.Fatalf("a level 10 warrior was not told the level: %q", r.Text())
	}
	if r.Use == "" {
		t.Fatal("Use is empty")
	}
	for _, tc := range []struct {
		class string
		level int
		id    int64
	}{{"WARRIOR", 25, rageElixir}, {"MAGE", 60, plainTonic}, {"MAGE", 10, plainTonic}} {
		if got := useText(t, tc.class, tc.level, tc.id); got.Use != "" || strings.Contains(got.Text(), "needs level") {
			t.Fatalf("%s %d item %d: says %q, but the character can use it", tc.class, tc.level, tc.id, got.Use)
		}
	}
}

func TestConsumableAnswerNamesTheClassesThatCanUseIt(t *testing.T) {
	r := useText(t, "MAGE", 60, rageElixir)
	if !strings.Contains(r.Text(), "Only warriors and druids can use it, not your class.") {
		t.Fatalf("a mage was not told the class limit: %q", r.Text())
	}
	both := useText(t, "MAGE", 10, rageElixir).Use
	if !strings.Contains(both, "Only warriors and druids") || !strings.Contains(both, "needs level 25") {
		t.Fatalf("class and level should both be said: %q", both)
	}
	if got := useText(t, "DRUID", 60, rageElixir).Use; got != "" {
		t.Fatalf("a druid can use it: %q", got)
	}
}

func TestOnlyConsumablesGetAUseLine(t *testing.T) {
	// a level 35 food for a level 10 character, and a piece of gear at the same level: only the consumable carries the line
	if got := useText(t, "MAGE", 10, gatedFood).Use; !strings.Contains(got, "level 35") {
		t.Fatalf("food use line = %q", got)
	}
	if got := useText(t, "MAGE", 5, vest).Use; got != "" {
		t.Fatalf("gear got a use line: %q", got)
	}
}

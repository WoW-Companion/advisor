package gear

import (
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/store"
)

// plain is an item the dataset says carries no stats.
func plain(id int64, name string, ilvl, req, quality, class, sub, slot int32) itemRow {
	r := it(id, name, ilvl, req, quality, class, sub, slot)
	r.StatsStatus = "none"
	return r
}

func headTop(t *testing.T, c character.Character, ds store.DataStore, group string) Result {
	t.Helper()
	res, err := Find(c, ds, group, Options{Slot: "head"})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// A plain white helm two item levels above a green with Agility is not the rogue's first pick: the plain piece counts half its
// item level, as any piece with no stat the build weighs does.
func TestPlainItemDoesNotOutrankAStatItem(t *testing.T) {
	ds := store.New(store.ReaderSource{"items": table(t, []itemRow{
		plain(1, "Plain Cap", 22, 15, 1, 4, leather, headSlot),
		it(2, "Agile Cap", 20, 15, 2, 4, leather, headSlot, 3),
	})})
	c := char("ROGUE", 20, nil)
	c.Profile = &character.Profile{Role: "damage", Focus: "leveling", Spec: "Combat"}
	res := headTop(t, c, ds, "")
	if got := names(res.Slots["head"]); got != "Agile Cap,Plain Cap" {
		t.Errorf("order %s", got)
	}
	if res.ItemLevelOnly != false {
		t.Error("ranked by stats")
	}
}

// A plain item above a plain item still ranks by item level, and a plain item beats a stat item of under half its item level.
func TestPlainItemsStillGoByItemLevel(t *testing.T) {
	ds := store.New(store.ReaderSource{"items": table(t, []itemRow{
		plain(1, "Plain Cap", 22, 15, 1, 4, leather, headSlot),
		plain(2, "Old Plain Cap", 12, 5, 1, 4, leather, headSlot),
		it(3, "Weak Agile Cap", 5, 1, 2, 4, leather, headSlot, 3),
	})})
	c := char("ROGUE", 20, nil)
	c.Profile = &character.Profile{Role: "damage", Focus: "leveling", Spec: "Combat"}
	if got := names(headTop(t, c, ds, "").Slots["head"]); got != "Plain Cap,Old Plain Cap,Weak Agile Cap" {
		t.Errorf("order %s", got)
	}
}

// A worn plain helm is beaten by a lower-level green that carries the stat the build wants.
func TestWornPlainItemYieldsToAStatItem(t *testing.T) {
	ds := store.New(store.ReaderSource{"items": table(t, []itemRow{
		plain(1, "Plain Cap", 22, 15, 1, 4, leather, headSlot),
		it(2, "Agile Cap", 20, 15, 2, 4, leather, headSlot, 3),
	})})
	worn := 1
	c := char("ROGUE", 20, map[string]character.EquippedItem{"head": {Slot: "head", ItemID: &worn}})
	c.Profile = &character.Profile{Role: "damage", Focus: "leveling", Spec: "Combat"}
	if got := names(headTop(t, c, ds, "").Slots["head"]); got != "Agile Cap" {
		t.Errorf("upgrades %q", got)
	}
}

// An item whose stats are unknown (not none) keeps the item-level basis: nothing is claimed about it.
func TestUnknownStatsKeepItemLevelBasis(t *testing.T) {
	ds := store.New(store.ReaderSource{"items": table(t, []itemRow{it(1, "Mystery Cap", 22, 15, 2, 4, leather, headSlot)})})
	c := char("ROGUE", 20, nil)
	c.Profile = &character.Profile{Role: "damage", Focus: "leveling", Spec: "Combat"}
	res := headTop(t, c, ds, "")
	if !res.ItemLevelOnly || len(res.Slots["head"]) != 1 || res.Slots["head"][0].Basis != BasisItemLevel {
		t.Errorf("%+v", res)
	}
}

// A character with no saved play style is weighed by the stat table's profile of the talent group they hold, for a class the gear
// weights file does not list; a group that names no spec leaves the ranking by item level, and says why.
func TestGroupWithoutPlayStyleUsesStatTable(t *testing.T) {
	ds := store.New(store.ReaderSource{"items": table(t, []itemRow{
		it(1, "Brute Helm", 20, 8, 2, 4, 3, headSlot, 4),
		it(2, "Sturdy Helm", 21, 8, 2, 4, 3, headSlot, 7),
	})})
	c := char("WARRIOR", 10, nil)
	res := headTop(t, c, ds, "Fury")
	if res.Spec != "Fury" || res.ItemLevelOnly || names(res.Slots["head"]) != "Brute Helm,Sturdy Helm" {
		t.Errorf("Fury: spec %q %s ilvl-only %v", res.Spec, names(res.Slots["head"]), res.ItemLevelOnly)
	}
	res = headTop(t, c, ds, "Protection")
	if res.Spec != "Protection" || names(res.Slots["head"]) != "Sturdy Helm,Brute Helm" {
		t.Errorf("Protection: spec %q %s", res.Spec, names(res.Slots["head"]))
	}
	res = headTop(t, c, ds, "Surviving")
	if res.Spec != "" || !res.ItemLevelOnly || !strings.Contains(res.Reason, "no stat weights") {
		t.Errorf("unnamed group: %+v", res)
	}
	if res = headTop(t, c, ds, ""); !strings.Contains(res.Reason, "spec unknown") {
		t.Errorf("no group: %q", res.Reason)
	}
}

// The group's weights reach the bag comparison too.
func TestGroupWithoutPlayStyleWeighsBags(t *testing.T) {
	if w := namedWeights(char("WARRIOR", 10, nil), "Protection"); w == nil || w["stamina"] <= w["strength"] {
		t.Errorf("weights %v", w)
	}
	if w := namedWeights(char("WARRIOR", 10, nil), ""); w != nil {
		t.Errorf("no group: %v", w)
	}
}

func rollRow(bonding, class, sub, slot, ilvl, req int64) store.Row {
	return store.Row{"bonding": bonding, "class_id": class, "subclass_id": sub, "slot": slot, "item_level": ilvl, "required_level": req, "allowable_class_mask": int64(-1)}
}

// A plate piece that needs level 40 is Need for a level 38 warrior (plate is theirs from 40), not Greed for "cannot wear"; a mage still
// cannot, and a mail piece is theirs now.
func TestLootRollJudgesArmorAtTheLevelTheItemNeeds(t *testing.T) {
	for _, tc := range []struct {
		name, class string
		level       int
		row         store.Row
		roll        Roll
		from        string
	}{
		{"plate at 40, warrior 38", "WARRIOR", 38, rollRow(1, 4, plate, 5, 45, 40), RollNeed, "from level 40"},
		{"plate at 40, paladin 25", "PALADIN", 25, rollRow(2, 4, plate, 5, 45, 40), RollNeed, "from level 40"},
		{"plate at 40, mage 38", "MAGE", 38, rollRow(1, 4, plate, 5, 45, 40), RollGreed, ""},
		{"plate at 40, rogue 38", "ROGUE", 38, rollRow(1, 4, plate, 5, 45, 40), RollGreed, ""},
		{"mail at 30, warrior 28", "WARRIOR", 28, rollRow(1, 4, 3, 5, 35, 30), RollNeed, "from level 30"},
		{"mail at 45, hunter 38", "HUNTER", 38, rollRow(1, 4, 3, 5, 50, 45), RollNeed, "from level 45"},
		{"leather at 45, hunter 38 (mail since 40)", "HUNTER", 38, rollRow(1, 4, leather, 5, 50, 45), RollGreed, ""},
		{"no required level, item level 55 reads as 50", "WARRIOR", 45, rollRow(1, 4, plate, 5, 55, 0), RollNeed, "from level 50"},
		{"plate for a level 60 warrior", "WARRIOR", 60, rollRow(1, 4, plate, 5, 62, 55), RollNeed, ""},
		{"plate below 40 and needed below 40", "WARRIOR", 30, rollRow(1, 4, plate, 5, 35, 30), RollGreed, ""},
	} {
		got := LootRoll(tc.class, tc.level, tc.row)
		if got.Roll != tc.roll || tc.from != "" && !strings.Contains(got.Why, tc.from) || tc.from == "" && strings.Contains(got.Why, "from level") {
			t.Errorf("%s: %s", tc.name, got.Text())
		}
	}
}

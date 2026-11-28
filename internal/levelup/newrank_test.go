package levelup

import (
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/store"
)

type nodeRow struct {
	TreeID  int32  `parquet:"tree_id"`
	SpellID int32  `parquet:"spell_id"`
	Name    string `parquet:"name"`
}

func mageSpells(t *testing.T) store.DataStore {
	return store.New(store.ReaderSource{
		"class_spells": tbl(t, []spellRow{
			{8, 133, "Fireball", i32(1), 1, true, nil},
			{8, 143, "Fireball", i32(2), 6, true, nil},
			{8, 145, "Fireball", i32(3), 12, true, nil},
			{8, 116, "Frostbolt", i32(1), 4, true, nil},
			{8, 11222, "Arcane Focus", nil, 1, true, nil},
			{8, 2000, "Feral Charge (Bear)", nil, 20, true, nil},
		}),
		// the tree has no class: the dataset leaves class_id empty on it
		"talent_nodes": tbl(t, []nodeRow{{1058, 11222, "Arcane Focus"}, {1058, 2000, "Feral Charge"}}),
	})
}

func newOf(sp []Spell, name string) (Spell, bool) {
	for _, s := range sp {
		if s.Name == name {
			return s, true
		}
	}
	return Spell{}, false
}

func TestHigherRankIsNewSpellWhenNoLowerRankIsKnown(t *testing.T) {
	d := mageSpells(t)
	lvl := 12
	for _, tc := range []struct {
		label    string
		known    map[int64]bool
		from     int
		wantNew  bool
		wantRank int
	}{
		{"a capture that lists no spell: nothing known", map[int64]bool{}, 1, true, 3},
		{"the lower ranks known", map[int64]bool{133: true, 143: true}, 12, false, 3},
		{"a capture that lists the known spells but not Fireball", map[int64]bool{116: true}, 12, true, 3},
		{"no capture of the known spells, the window holds rank 1 too", nil, 1, true, 3},
		{"no capture of the known spells, only the rank in the window", nil, 12, false, 3},
	} {
		c := character.Character{Class: "MAGE", Faction: "Horde", Level: &lvl, KnownSpells: tc.known}
		s, ok := newOf(Spells(c, d, tc.from, 12), "Fireball")
		if !ok || s.Rank != tc.wantRank || s.New != tc.wantNew {
			t.Errorf("%s: %+v, want rank %d new %v", tc.label, s, tc.wantRank, tc.wantNew)
		}
	}
}

func TestTalentOfATreeWithNoClassIsNotATrainerSpell(t *testing.T) {
	lvl := 20
	c := character.Character{Class: "MAGE", Faction: "Horde", Level: &lvl}
	got := names(Spells(c, mageSpells(t), 1, 20))
	for _, talent := range []string{"Arcane Focus", "Feral Charge (Bear)"} {
		if _, ok := newOf(Spells(c, mageSpells(t), 1, 20), talent); ok {
			t.Errorf("%s listed as a trainer spell: %s", talent, got)
		}
	}
	if _, ok := newOf(Spells(c, mageSpells(t), 1, 20), "Frostbolt"); !ok {
		t.Errorf("Frostbolt left out: %s", got)
	}
}

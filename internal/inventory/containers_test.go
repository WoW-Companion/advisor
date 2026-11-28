package inventory

import (
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/store"
)

// A quiver or ammo pouch the holder can use stays unplaced (the data holds no size), and the note adds what the rows do say: the level it needs,
// and the other kind of ammo the holder's ranged weapon fires.
func TestContainerNoteNamesLevelAndAmmoMismatch(t *testing.T) {
	ds := store.New(store.ReaderSource{"items": table(t, []itemRow{
		{ID: 30, Name: "Test Quiver", ItemLevel: 35, RequiredLevel: 30, Quality: 2, ClassID: 11, SubclassID: 2, Slot: 18, ClassMask: -1, StatsStatus: "none"},
		{ID: 31, Name: "Test Pouch", ItemLevel: 35, RequiredLevel: 30, Quality: 2, ClassID: 11, SubclassID: 3, Slot: 18, ClassMask: -1, StatsStatus: "none"},
		{ID: 32, Name: "Test Sack", ItemLevel: 40, RequiredLevel: 0, Quality: 1, ClassID: 1, Slot: 18, ClassMask: -1, StatsStatus: "none"},
		{ID: 33, Name: "Test Bow", ItemLevel: 10, Quality: 1, ClassID: 2, SubclassID: 2, Slot: 15, ClassMask: -1, StatsStatus: "none"},
		{ID: 34, Name: "Test Gun", ItemLevel: 10, Quality: 1, ClassID: 2, SubclassID: 3, Slot: 26, ClassMask: -1, StatsStatus: "none"},
	})})
	worn := func(id int, name string) map[string]character.EquippedItem {
		return map[string]character.EquippedItem{"ranged": {Slot: "ranged", ItemID: &id, Name: name}}
	}
	for _, tc := range []struct {
		what    string
		level   int
		ranged  map[string]character.EquippedItem
		item    int
		name    string
		want    []string
		without []string
	}{
		{"a quiver above the level", 20, nil, 30, "Test Quiver", []string{"from level 30", "no bag size"}, nil},
		{"a quiver at its level, no ranged weapon", 30, nil, 30, "Test Quiver", []string{"no bag size"}, []string{"from level", "fires"}},
		{"a quiver beside a bow", 40, worn(33, "Test Bow"), 30, "Test Quiver", []string{"no bag size"}, []string{"fires"}},
		{"a quiver beside a gun", 40, worn(34, "Test Gun"), 30, "Test Quiver", []string{"fires bullets", "quiver holds arrows", "no bag size"}, nil},
		{"an ammo pouch beside a bow", 40, worn(33, "Test Bow"), 31, "Test Pouch", []string{"fires arrows", "ammo pouch holds bullets"}, nil},
		{"an ammo pouch beside a gun", 40, worn(34, "Test Gun"), 31, "Test Pouch", []string{"no bag size"}, []string{"fires"}},
		{"a bag with no level is never given an estimated one", 5, nil, 32, "Test Sack", []string{BagReason}, []string{"from level"}},
	} {
		c := character.Character{Name: "Hunt", Class: "HUNTER", Level: ip(tc.level), BagsCaptured: true, Equipment: tc.ranged,
			Bags: []character.ItemStack{{ItemID: tc.item, Count: 1, Name: tc.name}}}
		res := ReviewAll([]character.Character{c}, ds)
		if len(res.Lines) != 1 || res.Lines[0].Action != Unsure {
			t.Errorf("%s: %+v", tc.what, res.Lines)
			continue
		}
		got := res.Lines[0].Reason
		for _, w := range tc.want {
			if !strings.Contains(got, w) {
				t.Errorf("%s: %q lacks %q", tc.what, got, w)
			}
		}
		for _, w := range tc.without {
			if strings.Contains(got, w) {
				t.Errorf("%s: %q holds %q", tc.what, got, w)
			}
		}
	}
}

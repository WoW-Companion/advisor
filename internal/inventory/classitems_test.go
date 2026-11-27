package inventory

import (
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/store"
)

func classItemStore(t *testing.T) store.DataStore {
	totem := itemRow{ID: 21, Name: "Totem of Rain", ItemLevel: 20, RequiredLevel: 20, Quality: 2, ClassID: 4, SubclassID: 9, Slot: 28, ClassMask: -1, StatsStatus: "known"}
	return store.New(store.ReaderSource{
		"items": table(t, []itemRow{
			{ID: 20, Name: "Maple Seed", Quality: 1, ClassID: 15, SubclassID: 1, ClassMask: -1, StatsStatus: "known"},
			totem,
		}),
	})
}

func bagsOf(class string, level int, ids ...int64) character.Character {
	c := character.Character{Name: "A", Class: class, Level: &level, BagsCaptured: true, BankCaptured: true}
	names := map[int64]string{20: "Maple Seed", 21: "Totem of Rain", 22: "Soul Shard", 23: "Major Healthstone"}
	for _, id := range ids {
		c.Bags = append(c.Bags, character.ItemStack{ItemID: int(id), Count: 3, Name: names[id]})
	}
	return c
}

func groupOf(r Result, name string) (Group, string) {
	for g, es := range r.Groups {
		for _, e := range es {
			if e.Name == name {
				return g, e.Reason
			}
		}
	}
	return "", ""
}

func TestClassReagentKept(t *testing.T) {
	ds := classItemStore(t)
	for _, tc := range []struct {
		class string
		ids   []int64
		item  string
		want  Group
	}{
		{"DRUID", []int64{20}, "Maple Seed", Keep},
		{"MAGE", []int64{20}, "Maple Seed", Unknown},
		{"WARLOCK", []int64{22}, "Soul Shard", Keep}, // no row in the dataset: still the warlock's
		{"MAGE", []int64{22}, "Soul Shard", Unknown},
		{"WARLOCK", []int64{23}, "Major Healthstone", Keep},
		{"PRIEST", []int64{23}, "Major Healthstone", Unknown},
	} {
		g, why := groupOf(Review(bagsOf(tc.class, 40, tc.ids...), ds), tc.item)
		if g != tc.want {
			t.Errorf("%s with %s: %s (%s), want %s", tc.class, tc.item, g, why, tc.want)
		}
	}
}

func TestClassReagentMailedToAlt(t *testing.T) {
	ds := classItemStore(t)
	res := ReviewAll([]character.Character{bagsOf("MAGE", 40, 20), func() character.Character { c := bagsOf("DRUID", 40); c.Name = "Dru"; return c }()}, ds)
	if len(res.Lines) != 1 || res.Lines[0].Action != Mail || res.Lines[0].To != "Dru" {
		t.Errorf("a mage's Maple Seed with a druid alt: %+v, want mail to Dru", res.Lines)
	}
	res = ReviewAll([]character.Character{bagsOf("DRUID", 40, 20)}, ds)
	if len(res.Lines) != 1 || res.Lines[0].Action != Keep_ || !strings.Contains(res.Lines[0].Reason, "druid") {
		t.Errorf("a druid's Maple Seed: %+v, want keep for the druid", res.Lines)
	}
}

func TestRelicKeptByItsClass(t *testing.T) {
	ds := classItemStore(t)
	for _, tc := range []struct {
		class string
		level int
		want  Group
	}{{"SHAMAN", 40, Equip}, {"SHAMAN", 10, Keep}, {"PALADIN", 40, Bank}, {"WARRIOR", 40, Bank}} {
		if g, why := groupOf(Review(bagsOf(tc.class, tc.level, 21), ds), "Totem of Rain"); g != tc.want {
			t.Errorf("%s level %d: a totem is %s (%s), want %s", tc.class, tc.level, g, why, tc.want)
		}
	}
	worn := bagsOf("SHAMAN", 40, 21)
	id := 99
	worn.Equipment = map[string]character.EquippedItem{"ranged": {ItemID: &id}}
	if g, _ := groupOf(Review(worn, ds), "Totem of Rain"); g != Keep {
		t.Errorf("a totem beside a worn totem is %s, want keep", g)
	}
}

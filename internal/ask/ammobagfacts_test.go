package ask

import (
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/store"
)

type bagItemRow struct {
	ID          int64  `parquet:"id"`
	Name        string `parquet:"name"`
	ClassID     int32  `parquet:"class_id"`
	SubclassID  int32  `parquet:"subclass_id"`
	Slot        int32  `parquet:"slot"`
	ClassMask   int32  `parquet:"allowable_class_mask"`
	StatsStatus string `parquet:"stats_status"`
}

func bagDS(t *testing.T) store.DataStore {
	return store.New(store.ReaderSource{"items": parquetOf(t, []bagItemRow{
		{1, "Test Quiver", 11, 2, 18, -1, "none"}, {2, "Test Pouch", 11, 3, 18, -1, "none"},
		{3, "Test Soul Bag", 1, 1, 18, 256, "none"}, {4, "Test Herb Bag", 1, 2, 18, -1, "none"},
		{5, "Test Sash", 4, 0, 6, -1, "none"},
	})})
}

func TestAmmoBagFactsNameTheRowsLimits(t *testing.T) {
	ds := bagDS(t)
	for _, q := range []string{"can a priest use an ammo pouch?", "which class can use a quiver", "can my mage use a soul bag?", "can I use a herb bag as a hunter?", "do I need a quiver for my bow?"} {
		got := AmmoBagFacts(ds, q)
		for _, w := range []string{"holds arrows", "holds bullets", "0 of the 2 quivers and pouches", "Test Soul Bag (warlocks only)", "no slot count"} {
			if !strings.Contains(got, w) {
				t.Errorf("%q: facts lack %q:\n%s", q, w, got)
			}
		}
		if strings.Contains(got, "Test Herb Bag") {
			t.Errorf("%q: a bag with no class limit is named as limited: %s", q, got)
		}
	}
	for _, q := range []string{"which bag should I use?", "what helm should I wear?", "where do I buy arrows?"} {
		if got := AmmoBagFacts(ds, q); got != "" {
			t.Errorf("%q: facts for a question that names no quiver or class bag: %s", q, got)
		}
	}
	if got := AmmoBagFacts(nil, "can a priest use a quiver?"); got != "" {
		t.Errorf("no dataset: %s", got)
	}
}

func TestLimitFactsUniqueAndShirtAndBagChoice(t *testing.T) {
	for _, tc := range []struct{ q, want string }{
		{"can I wear two of the same ring?", "Unique items:"},
		{"can I equip two copies of the same ring", "Unique items:"},
		{"can I wear the same ring on both fingers", "Unique items:"},
		{"can I use two of the same trinket at once?", "Unique items:"},
		{"are rings unique-equipped?", "Unique items:"},
		{"do shirts give stats?", "Shirts and tabards:"},
		{"can any class wear a shirt?", "Shirts and tabards:"},
		{"does a tabard replace my chest armor?", "Shirts and tabards:"},
		{"which tabard is best?", "Shirts and tabards:"},
		{"which bag should I use?", "Bag and bank size:"},
		{"what is the best bag for a hunter?", "Bag and bank size:"},
		{"what is the best quiver?", "Bag and bank size:"},
	} {
		if got := LimitFacts(tc.q); !strings.Contains(got, tc.want) {
			t.Errorf("LimitFacts(%q) = %q, want %q", tc.q, got, tc.want)
		}
	}
	for _, q := range []string{"which ring should I wear?", "can my mage wear a ring?", "what is the best neck for me?", "I wear the same boots"} {
		got := LimitFacts(q)
		for _, f := range []string{"Unique items:", "Bag and bank size:"} {
			if strings.Contains(got, f) {
				t.Errorf("LimitFacts(%q) holds %s", q, f)
			}
		}
	}
}

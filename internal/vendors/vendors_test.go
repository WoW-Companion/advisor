package vendors

import (
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
)

func TestMoneyAndCheap(t *testing.T) {
	for _, tc := range []struct {
		c    int64
		want string
	}{{0, ""}, {5, "5c"}, {400, "4s"}, {25050, "2g 50s 50c"}} {
		if got := Money(tc.c); got != tc.want {
			t.Errorf("Money(%d) = %q, want %q", tc.c, got, tc.want)
		}
	}
	for _, tc := range []struct {
		price int64
		level int
		cheap bool
	}{{10000, 5, true}, {10001, 5, false}, {20000, 10, true}, {70000, 60, true}, {0, 10, false}} {
		if got := (Offer{PriceCopper: tc.price}).Cheap(tc.level); got != tc.cheap {
			t.Errorf("Cheap(%d, level %d) = %v", tc.price, tc.level, got)
		}
	}
}

func TestLineLimited(t *testing.T) {
	o := Offer{Vendor: "Dree", Zone: "Dunemark", Price: "4s", Limited: true}
	if got := o.Line(); got != "Buy from Dree in Dunemark for 4s (limited stock)" {
		t.Fatal(got)
	}
}

func TestSeenVendorBeatsTheDatasetAndIsMarked(t *testing.T) {
	c := character.Character{Faction: "Alliance", Seen: &character.Observed{Vendors: []character.SeenVendor{
		{NPC: 1, Name: "Dear", Faction: "Alliance", Items: []character.SeenStock{{ItemID: 5, Price: 9000}}},
		{NPC: 2, Name: "Cheap", Faction: "Alliance", Items: []character.SeenStock{{ItemID: 5, Price: 500}}},
		{NPC: 3, Name: "Enemy", Faction: "Horde", Items: []character.SeenStock{{ItemID: 5, Price: 1}}},
	}}}
	o, ok := BestFor(c, nil, 5)
	if !ok || o.Vendor != "Cheap" || o.PriceCopper != 500 || !o.Seen() || o.Line() != "Buy from Cheap for 5s (seen by you)" {
		t.Errorf("%+v %q", o, o.Line())
	}
	if _, ok := BestFor(character.Character{}, nil, 5); ok {
		t.Error("an offer from nowhere")
	}
}

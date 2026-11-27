package vendors

import (
	"bytes"
	"testing"

	"github.com/parquet-go/parquet-go"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/store"
)

type itemRow struct {
	ID       int32 `parquet:"id"`
	BuyPrice int32 `parquet:"buy_price"`
}

func itemsTable(t *testing.T, rows []itemRow) store.SizedReaderAt {
	t.Helper()
	var buf bytes.Buffer
	if err := parquet.Write(&buf, rows); err != nil {
		t.Fatal(err)
	}
	return store.SizedReaderAt{ReaderAt: bytes.NewReader(buf.Bytes()), Size: int64(buf.Len())}
}

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

func TestLineForSaysEachForMoreThanOne(t *testing.T) {
	o := Offer{Vendor: "Dree", Zone: "Dunemark", Price: "10c", Limited: true}
	for n, want := range map[int]string{0: "Buy from Dree in Dunemark for 10c (limited stock)", 1: "Buy from Dree in Dunemark for 10c (limited stock)", 2: "Buy from Dree in Dunemark for 10c each (limited stock)"} {
		if got := o.LineFor(n); got != want {
			t.Errorf("LineFor(%d) = %q, want %q", n, got, want)
		}
	}
	if got := (Offer{Vendor: "Dree"}).LineFor(3); got != "Buy from Dree" {
		t.Errorf("an unknown price says no each: %q", got)
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

// A vendor row's price can carry the reporter's reputation discount: a player with no standing pays the item's buy price.
func TestBasePriceRaisesADiscountedPriceToTheBuyPrice(t *testing.T) {
	ds := store.New(store.ReaderSource{"items": itemsTable(t, []itemRow{{7, 300}, {8, 100}})})
	for _, tc := range []struct{ item, listed, want int64 }{
		{7, 270, 300}, // a tenth off at Honored: the base price is 300
		{7, 300, 300},
		{7, 0, 0}, // an unknown price stays unknown
		{8, 120, 120},
		{9, 50, 50}, // the item has no row
	} {
		if got := BasePrice(ds, tc.item, tc.listed); got != tc.want {
			t.Errorf("BasePrice(item %d, listed %d) = %d, want %d", tc.item, tc.listed, got, tc.want)
		}
	}
	if got := BasePrice(nil, 7, 270); got != 270 {
		t.Errorf("without a dataset BasePrice = %d, want 270", got)
	}
}

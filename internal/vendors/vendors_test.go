package vendors

import "testing"

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

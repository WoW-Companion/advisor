package paths

import (
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/store"
)

type vendorRow struct {
	NPCID   int64 `parquet:"npc_id"`
	ItemID  int64 `parquet:"item_id"`
	Price   int64 `parquet:"price_copper"`
	Limited bool  `parquet:"limited"`
}

// vendorFixture adds Everloot vendor offers to the base world: the bracers (a world drop) are sold
// cheaply, limited; the shawl by a dear vendor; the copper ore (a craft reagent) by a vendor.
func vendorFixture(t *testing.T, offers []vendorRow) store.DataStore {
	return store.New(store.ReaderSource{
		"items": tbl(t, []itemRow{
			item(bracers, "Tough Bracers", 20, 8, 2, mail, 9, 2),
			item(vest, "Copper Chain Vest", 17, 8, 2, mail, 5, 0),
			item(bar, "Copper Bar", 1, 0, 1, 0, 0, 0),
			item(ore, "Copper Ore", 1, 0, 1, 0, 0, 0),
		}),
		"loot_sources":    tbl(t, []lootRow{{bracers, "creature", 701, 100, 2, nil}}),
		"npcs":            tbl(t, []npcRow{{50, "Quartermaster Dree", 20, 20, "Dunemark"}, {51, "Oreseller Bo", 20, 20, "Dunemark"}}),
		"recipes":         tbl(t, []recipeRow{{800, "Copper Chain Vest", "Blacksmithing", 5, vest}}),
		"recipe_reagents": tbl(t, []reagentRow{{800, bar, 6}, {800, ore, 4}}),
		"vendor_sources":  tbl(t, offers),
	})
}

func TestVendorPathOnUpgradeLimitedAndRanked(t *testing.T) {
	ds := vendorFixture(t, []vendorRow{{50, bracers, 25050, true}, {51, bracers, 90000, false}})
	r := For(warrior(), ds, bracers, Options{})
	if kinds(r) != "auction,vendor" { // 2g 50s 50c is over the 2g threshold at level 10
		t.Fatalf("kinds %q", kinds(r))
	}
	p := r.Paths[1]
	if !strings.Contains(p.Line, "Buy from Quartermaster Dree in Dunemark for 2g 50s 50c (limited stock)") {
		t.Fatalf("line %q", p.Line)
	}
	if p.Source != "everloot" || p.Offer == nil || !p.Offer.Limited || p.Offer.PriceCopper != 25050 {
		t.Fatalf("tags %+v", p)
	}
	if txt := r.Text(); !strings.Contains(txt, "Sources: Everloot (CC BY-SA)") {
		t.Fatalf("no credit: %s", txt)
	}
}

func TestVendorRankCheapBesideAuctionDearBelow(t *testing.T) {
	cheap := For(warrior(), vendorFixture(t, []vendorRow{{50, bracers, 15000, false}}), bracers, Options{})
	if kinds(cheap) != "vendor,auction" || cheap.Paths[0].Offer.Limited {
		t.Fatalf("cheap: %q", kinds(cheap))
	}
	dear := For(warrior(), vendorFixture(t, []vendorRow{{50, bracers, 500000, false}}), bracers, Options{})
	if kinds(dear) != "auction,vendor" {
		t.Fatalf("dear: %q", kinds(dear))
	}
}

func TestVendorPathOnCraftReagent(t *testing.T) {
	ds := vendorFixture(t, []vendorRow{{51, ore, 400, false}})
	r := For(warrior(), ds, vest, Options{})
	var how string
	for _, p := range r.Paths {
		for _, g := range p.Reagents {
			if g.Name == "Copper Ore" {
				how = g.How
				if g.Source != "everloot" || g.Offer == nil {
					t.Fatalf("reagent tags: %+v", g)
				}
			}
		}
	}
	// The warrior mines, which stays first; the vendor is named with its price (each: the recipe needs more than one) as the other way.
	if !strings.HasSuffix(how, ", or buy from Oreseller Bo in Dunemark for 4s each") || !strings.HasPrefix(how, "gather it") {
		t.Fatalf("how %q", how)
	}
}

func TestNoVendorRowNoVendorPath(t *testing.T) {
	ds := vendorFixture(t, []vendorRow{{51, ore, 400, false}})
	if r := For(warrior(), ds, bracers, Options{}); strings.Contains(kinds(r), "vendor") || strings.Contains(r.Text(), "Everloot") {
		t.Fatalf("kinds %q: %s", kinds(r), r.Text())
	}
	// A vendor the dataset does not name is never offered.
	ds = vendorFixture(t, []vendorRow{{999, bracers, 100, false}})
	if r := For(warrior(), ds, bracers, Options{}); strings.Contains(kinds(r), "vendor") {
		t.Fatalf("unnamed vendor offered: %q", kinds(r))
	}
}

package professions

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/learn"
	"github.com/WoW-Companion/advisor/internal/store"
	"github.com/WoW-Companion/advisor/internal/vendors"
)

// The offline reputation benchmark: what a player pays and is told when a vendor's goods depend on standing,
// beside the dataset's vendor rows and items' buy prices, with no network. The dataset holds no standing, faction
// reputation or "requires Honored" column: vendor_sources has the price a reporting player paid, which a reputation
// discount (a tenth off at Honored) lowers below the item's buy price. So each case sets the price the advisor
// quotes against the buy price a player with no standing pays, and the cases the dataset cannot answer (which
// standing unlocks an item, whether a vendor is open to the player) check that the advisor claims nothing.
// The snapshot names no vendor, so a creature with no name is given a synthetic one; the rows stay the dataset's.
// make offline-benchmark runs it.

const (
	repSrc     = "dataset vendor_sources (price_copper), items (buy_price)"
	repNoReply = "the dataset has no standing or reputation column"
)

// namedVendors gives a vendor creature the snapshot lacks a name for a synthetic one, so the price logic can run.
type namedVendors struct{ store.DataStore }

func (n namedVendors) Get(table string, id int64) (store.Row, error) {
	r, err := n.DataStore.Get(table, id)
	if table == "npcs" && (err != nil || asText(r["name"]) == "") {
		return store.Row{"id": id, "name": fmt.Sprintf("Synthetic Vendor %d", id), "zone_name": "Synthetic Vale"}, nil
	}
	return r, err
}

// standingWords are what an answer would say if it claimed to know a standing.
var standingWords = regexp.MustCompile(`(?i)\b(friendly|honored|revered|exalted|reputation|standing|faction)\b`)

// refBuy is the dataset's price for a player with no standing: items.buy_price, and the lowest and highest price
// a vendor row reports for the item.
func refBuy(ds store.DataStore, item int64) (buy, lo, hi int64) {
	it, _ := ds.Get("items", item)
	buy = asI64(it["buy_price"])
	rows, _ := ds.Filter("vendor_sources", store.Range{Column: "item_id", Min: item, Max: item})
	for _, r := range rows {
		p := asI64(r["price_copper"])
		if p <= 0 {
			continue
		}
		if lo == 0 || p < lo {
			lo = p
		}
		hi = max(hi, p)
	}
	return buy, lo, hi
}

// repItems are items the dataset's vendor rows list below the buy price (a reporter's discount), the first ten on
// one vendor row only, then items of several vendor rows, and last items sold at the buy price.
var repItems = []int64{5786, 5787, 6328, 6329, 6342, 20758, 17188, 6272, 6274, 6325, 6368, 253665, 253668, 5771, 6270, 6474, 5486}

// TestOfflineBenchmarkReputationPrice: the price a vendor offer shows for an item, beside the dataset's buy price.
func TestOfflineBenchmarkReputationPrice(t *testing.T) {
	ds := namedVendors{placesStore()}
	var out []benchRow
	for _, id := range repItems {
		buy, lo, hi := refBuy(ds, id)
		it, _ := ds.Get("items", id)
		o, ok := vendors.Best(ds, id)
		ref := fmt.Sprintf("buy price %s (rows %s to %s)", vendors.Money(buy), vendors.Money(lo), vendors.Money(hi))
		got := "no offer"
		if ok {
			got = o.Price
		}
		out = append(out, benchRow{fmt.Sprintf("%s: vendor price", asText(it["name"])), got, ref, repSrc, resultIf(ok && o.PriceCopper == buy, "a discounted price quoted as the price")})
	}
	finishOffline(t, "advice-reputation-price", "Offline reputation: vendor price against the buy price", out)
}

// TestOfflineBenchmarkReputationRecipe: the recipe-vendor line the recipe finder gives, beside the same buy price.
func TestOfflineBenchmarkReputationRecipe(t *testing.T) {
	ds := namedVendors{placesStore()}
	var out []benchRow
	for _, tc := range vendorRecipeCases {
		res, err := learn.Find(character.Character{Faction: tc.faction, Class: "MAGE"}, ds, nil, tc.recipe)
		if err != nil {
			t.Fatalf("%s: %v", tc.recipe, err)
		}
		buy, _, _ := refBuy(ds, tc.item)
		prices := []string{}
		ok := len(res.Ways) > 0
		for _, w := range res.Ways {
			prices = append(prices, w.Price)
			ok = ok && (buy == 0 || w.Price == vendors.Money(buy))
		}
		out = append(out, benchRow{fmt.Sprintf("%s (%s): recipe price", tc.recipe, tc.faction), strings.Join(prices, ", "), "buy price " + vendors.Money(buy), repSrc,
			resultIf(ok, "a discounted price quoted as the price")})
	}
	finishOffline(t, "advice-reputation-recipe", "Offline reputation: recipe vendor price", out)
}

// TestOfflineBenchmarkReputationNoAnswer: what standing unlocks an item, and whether a vendor is open to the
// player, are in no dataset table. The advisor names no standing and a character's captured reputation changes no answer.
func TestOfflineBenchmarkReputationNoAnswer(t *testing.T) {
	ds := namedVendors{placesStore()}
	var out []benchRow
	exalted, honored := 42000, 9000
	rich := character.Character{Faction: "Horde", Class: "MAGE", Reputations: []character.Reputation{
		{Name: "Synthetic Faction", Standing: levelOf(8), Value: &exalted}, {Name: "Another Synthetic Faction", Standing: levelOf(6), Value: &honored}}}
	plain := character.Character{Faction: "Horde", Class: "MAGE"}
	for _, id := range []int64{6272, 6342, 5787} {
		it, _ := ds.Get("items", id)
		a, okA := vendors.BestFor(rich, ds, id)
		b, okB := vendors.BestFor(plain, ds, id)
		ok := okA && okB && a.Line() == b.Line() && !standingWords.MatchString(a.Line())
		out = append(out, benchRow{asText(it["name"]) + ": which standing unlocks it", a.Line(), repNoReply, repSrc, resultIf(ok, "a standing was named, or a captured reputation changed the offer")})
	}
	for _, tc := range []struct{ recipe, faction string }{{"Blue Linen Robe", "Horde"}, {"Enchant Chest - Minor Intellect", "Alliance"}, {"Rainbow Fin Albacore", "Alliance"}} {
		res, err := learn.Find(character.Character{Faction: tc.faction, Class: "MAGE"}, ds, nil, tc.recipe)
		if err != nil {
			t.Fatalf("%s: %v", tc.recipe, err)
		}
		out = append(out, benchRow{tc.recipe + ": standing to learn it", firstLine(res.Text), repNoReply, recipeSrc, resultIf(!standingWords.MatchString(res.Text), "a standing was named")})
	}
	// a vendor row whose price the dataset records as 0: the price is unknown and no price is quoted
	zero := int64(249885)
	o, ok := vendors.Best(ds, zero)
	out = append(out, benchRow{"Recipe: Venomous Smoothie: vendor price", fmt.Sprintf("%q", o.Price), "price_copper 0: no price", repSrc, resultIf(!ok || o.Price == "" && o.PriceCopper == 0, "a price was invented")})
	finishOffline(t, "advice-reputation-no-answer", "Offline reputation advice, no answer in the dataset", out)
}

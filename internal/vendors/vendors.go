// Package vendors finds who sells an item, from the dataset's vendor_sources table (offers players
// saw in game and reported to Everloot, CC BY-SA 4.0). The rules are in docs/upgrade-paths.md.
package vendors

import (
	"fmt"
	"strings"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/store"
)

// Source tags the facts as Everloot's; Credit is the attribution line for an answer's sources.
const (
	Source = "everloot"
	Credit = "Everloot (CC BY-SA)"
)

// CheapPerBand is the price, in copper, a vendor path may cost per ten levels of the character's
// level band (level/10 + 1 bands) to count as cheap: 1 gold at levels 1-9, 2 gold at 10-19, and so on.
const CheapPerBand = 10000

// Offer is one vendor selling one item.
type Offer struct {
	NPCID       int64  `json:"-"`
	Vendor      string `json:"vendor"`
	Zone        string `json:"zone,omitempty"`
	PriceCopper int64  `json:"price_copper"`
	Price       string `json:"price,omitempty" jsonschema:"the price in gold, silver and copper; empty when the price is unknown"`
	Limited     bool   `json:"limited,omitempty" jsonschema:"the vendor has limited stock"`
	Source      string `json:"source" jsonschema:"everloot, or seen by you when the player saw this vendor in game"`
}

// Seen reports whether the offer is one the player saw in game.
func (o Offer) Seen() bool { return o.Source == character.SeenByYou }

// Cheap reports whether the price is modest for a character of the level: at or under CheapPerBand
// copper for each band of ten levels. An unknown price is never cheap.
func (o Offer) Cheap(level int) bool {
	if o.PriceCopper <= 0 {
		return false
	}
	if level < 1 {
		level = 1
	}
	return o.PriceCopper <= int64(level/10+1)*CheapPerBand
}

// Line is "Buy from <vendor> in <zone> for <price>", with "limited stock" when it is limited.
func (o Offer) Line() string {
	s := "Buy from " + o.Vendor
	if o.Zone != "" {
		s += " in " + o.Zone
	}
	if o.Price != "" {
		s += " for " + o.Price
	}
	if o.Limited {
		s += " (limited stock)"
	}
	if o.Seen() {
		s += " (" + character.SeenByYou + ")"
	}
	return s
}

// Money writes copper as gold, silver and copper; "" for a price of zero or less.
func Money(c int64) string {
	if c <= 0 {
		return ""
	}
	var p []string
	if g := c / 10000; g > 0 {
		p = append(p, fmt.Sprintf("%dg", g))
	}
	if s := c % 10000 / 100; s > 0 {
		p = append(p, fmt.Sprintf("%ds", s))
	}
	if k := c % 100; k > 0 {
		p = append(p, fmt.Sprintf("%dc", k))
	}
	return strings.Join(p, " ")
}

// BasePrice is the price a player with no standing pays for an item whose vendor row lists the price. A
// reporter's price can be lower than the item's buy price because of a reputation discount the reporter had
// (a tenth off at Honored), so the buy price is used when it is higher. An unknown listed price (0 or less)
// stays unknown.
func BasePrice(ds store.DataStore, itemID, listed int64) int64 {
	if listed <= 0 || ds == nil {
		return listed
	}
	if it, err := ds.Get("items", itemID); err == nil {
		if base := i64(it["buy_price"]); base > listed {
			return base
		}
	}
	return listed
}

func i64(v any) int64 { n, _ := v.(int64); return n }

func str(v any) string { s, _ := v.(string); return s }

// Best is the cheapest offer for the item (a known price before an unknown one, then the lowest
// NPC id), or false when the table is missing or has no vendor for it. A vendor the dataset does
// not name is skipped: advice never points at "vendor 1234".
func Best(ds store.DataStore, itemID int64) (Offer, bool) {
	if ds == nil || itemID <= 0 {
		return Offer{}, false
	}
	rows, err := ds.Filter("vendor_sources", store.Range{Column: "item_id", Min: itemID, Max: itemID})
	if err != nil {
		return Offer{}, false
	}
	var best Offer
	found := false
	for _, r := range rows {
		o := Offer{NPCID: i64(r["npc_id"]), PriceCopper: BasePrice(ds, itemID, i64(r["price_copper"])), Source: Source}
		if b, ok := r["limited"].(bool); ok {
			o.Limited = b
		} else {
			o.Limited = i64(r["limited"]) > 0
		}
		n, err := ds.Get("npcs", o.NPCID)
		if err != nil || str(n["name"]) == "" {
			continue
		}
		o.Vendor = str(n["name"])
		if o.Zone = str(n["zone_name"]); o.Zone == "" {
			o.Zone = str(n["located_in"])
		}
		o.Price = Money(o.PriceCopper)
		if !found || better(o, best) {
			best, found = o, true
		}
	}
	return best, found
}

// BestFor is Best for a character: a vendor the player opened and saw sell the item (open to their
// faction) comes first, at the price it showed, ahead of the dataset's offers.
func BestFor(c character.Character, ds store.DataStore, itemID int64) (Offer, bool) {
	vs, stock := c.Seen.VendorsOf(itemID, c.Faction)
	best, found := Offer{}, false
	for i, v := range vs {
		o := Offer{NPCID: v.NPC, Vendor: v.Name, PriceCopper: stock[i].Price, Price: Money(stock[i].Price), Source: character.SeenByYou,
			Zone: c.ZoneOfMap(v.Map)}
		if stock[i].Ext {
			o.PriceCopper, o.Price = 0, "" // it also costs tokens or items: no plain price to compare
		}
		if !found || better(o, best) {
			best, found = o, true
		}
	}
	if found {
		return best, true
	}
	return Best(ds, itemID)
}

func better(a, b Offer) bool {
	ak, bk := a.PriceCopper > 0, b.PriceCopper > 0
	if ak != bk {
		return ak
	}
	if a.PriceCopper != b.PriceCopper {
		return a.PriceCopper < b.PriceCopper
	}
	return a.NPCID < b.NPCID
}

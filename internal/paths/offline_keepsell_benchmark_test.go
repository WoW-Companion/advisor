package paths_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/ask"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/inventory"
	"github.com/WoW-Companion/advisor/internal/paths"
	"github.com/WoW-Companion/advisor/internal/store"
)

// The offline item-binding and keep-or-sell benchmark: what the bag and bank review says to do with a stack by how the item
// binds (Bind on Pickup cannot be mailed, traded or put on the Auction House; Bind on Equip can), which items the item paths
// treat as tradeable, and what the advisor says when the dataset holds no bag size, price or item, beside the dataset's items rows,
// with no network. It reads testdata/usability_snapshot (items: bonding, quality, class, slot, required_level, sell_price). The
// dataset holds no bag or bank slot count, no bank slot price and no Auction House price, and the capture holds no cooldown, so
// the cases it cannot answer check that the advisor says so and states none. make offline-benchmark runs it; WOWC_BENCH_REPORT
// receives the tables.

const (
	bindSrc     = "dataset items (bonding, quality, class_id, slot, required_level, sell_price); game: Bind on Pickup items cannot be mailed, traded or auctioned"
	bindTradeSr = "dataset items (bonding 0 none, 1 Bind on Pickup, 2 Bind on Equip, 3 Bind on Use, 4 quest); game binding rules"
	bindNoneSrc = "the dataset has no such column, row or table"
)

// bindChar is a captured character holding the named items in the bags and the bank seen.
func bindChar(t *testing.T, name, class string, level int, bag ...string) character.Character {
	t.Helper()
	c := usabChar(class, level, "Horde")
	c.Name = name
	c.BagsCaptured, c.BankCaptured, c.Equipment = true, true, map[string]character.EquippedItem{}
	for _, b := range bag {
		r := usabItem(t, b)
		c.Bags = append(c.Bags, character.ItemStack{ItemID: int(num(r["id"])), Count: 1, Name: b})
	}
	return c
}

func bindLine(l []inventory.Line, who, item string) inventory.Line {
	for _, x := range l {
		if x.Character == who && x.Item == item {
			return x
		}
	}
	return inventory.Line{}
}

type bindingCase struct {
	label     string
	chars     func(*testing.T) []character.Character
	who, item string
	action    inventory.Action
	to        string
	ah        bool
	ref       string
}

var bindingCases = []bindingCase{
	{"a Bind on Pickup robe a warrior holds, a mage alt could wear: never mailed", func(t *testing.T) []character.Character {
		return []character.Character{bindChar(t, "Wara", "WARRIOR", 40, "Robe of Power"), bindChar(t, "Magea", "MAGE", 40)}
	}, "Wara", "Robe of Power", inventory.Sell, "", false, "bonding 1: bound to the holder, cannot be mailed or auctioned, so the vendor"},
	{"a Bind on Pickup plate piece no alt can wear: the vendor, not the Auction House", func(t *testing.T) []character.Character {
		return []character.Character{bindChar(t, "Magea", "MAGE", 40, "Sergeant Major's Plate Wristguards")}
	}, "Magea", "Sergeant Major's Plate Wristguards", inventory.Sell, "", false, "bonding 1: cannot be auctioned, so the vendor"},
	{"a Bind on Pickup plate piece a warrior alt could wear, held by a mage", func(t *testing.T) []character.Character {
		return []character.Character{bindChar(t, "Magea", "MAGE", 50, "Sergeant Major's Plate Wristguards"), bindChar(t, "Wara", "WARRIOR", 50)}
	}, "Magea", "Sergeant Major's Plate Wristguards", inventory.Sell, "", false, "bonding 1: cannot be mailed to the warrior"},
	{"a Bind on Equip rare an alt can wear: mailed", func(t *testing.T) []character.Character {
		return []character.Character{bindChar(t, "Wara", "WARRIOR", 40, "Feathered Mantle"), bindChar(t, "Rogua", "ROGUE", 30)}
	}, "Wara", "Feathered Mantle", inventory.Mail, "Rogua", false, "bonding 2: can be mailed to an alt who can wear it"},
	{"a Bind on Equip green nobody can use: the Auction House", func(t *testing.T) []character.Character {
		return []character.Character{bindChar(t, "Magea", "MAGE", 40, "Heavy Mithril Shoulder")}
	}, "Magea", "Heavy Mithril Shoulder", inventory.Sell, "", true, "bonding 2, uncommon: tradeable, so the Auction House"},
	{"a Bind on Pickup neck piece that fills an empty slot: kept", func(t *testing.T) []character.Character {
		return []character.Character{bindChar(t, "Magea", "MAGE", 40, "Pendant of Myzrael")}
	}, "Magea", "Pendant of Myzrael", inventory.Keep_, "", false, "an empty neck slot takes it"},
	{"a quest item no log asks for: tossed, never mailed or sold", func(t *testing.T) []character.Character {
		return []character.Character{bindChar(t, "Magea", "MAGE", 40, "Torch of Retribution")}
	}, "Magea", "Torch of Retribution", inventory.Toss, "", false, "bonding 4: a quest item, no quest needs it"},
}

// TestOfflineBenchmarkBindingKeepOrSellA and B: what the review says to do with a stack by its binding.
func TestOfflineBenchmarkBindingKeepOrSellA(t *testing.T) { bindingBatch(t, "a", bindingCases[:4]) }
func TestOfflineBenchmarkBindingKeepOrSellB(t *testing.T) { bindingBatch(t, "b", bindingCases[4:]) }

func bindingBatch(t *testing.T, part string, cases []bindingCase) {
	ds := usabStore()
	var out []benchRow
	for _, tc := range cases {
		r := inventory.ReviewAll(tc.chars(t), ds)
		l := bindLine(r.Lines, tc.who, tc.item)
		row := usabItem(t, tc.item)
		got := fmt.Sprintf("%s %s ah=%v: %s", l.Action, l.To, l.AH, l.Reason)
		ref := fmt.Sprintf("%s %s ah=%v (bonding %d): %s", tc.action, tc.to, tc.ah, num(row["bonding"]), tc.ref)
		out = append(out, benchRow{tc.label, got, ref, bindSrc,
			resultIf(l.Action == tc.action && l.To == tc.to && l.AH == tc.ah, "mailed or listed an item that cannot be, or the wrong advice for its binding")})
	}
	finish(t, "advice-binding-keep-or-sell-"+part, "Offline item binding: keep, sell or mail ("+part+")", out)
}

// TestOfflineBenchmarkBindingTradeable: which items the item paths treat as tradeable and as auctionable, by their dataset binding and quality.
func TestOfflineBenchmarkBindingTradeable(t *testing.T) {
	var out []benchRow
	for _, tc := range []struct {
		item                string
		tradeable, auctions bool
	}{
		{"Robe of Power", false, false},                      // Bind on Pickup, rare
		{"Pendant of Myzrael", false, false},                 // Bind on Pickup, uncommon
		{"Heavy Mithril Shoulder", true, true},               // Bind on Equip, uncommon
		{"Feathered Mantle", true, true},                     // Bind on Equip, rare
		{"Deprecated Cat's Paw", true, false},                // Bind on Use, common
		{"Ryedol's Lucky Pick", false, false},                // quest item
		{"Sergeant Major's Plate Wristguards", false, false}, // Bind on Pickup, rare
	} {
		r := usabItem(t, tc.item)
		b, q := int(num(r["bonding"])), int(num(r["quality"]))
		tr, au := paths.Tradeable(b, q), paths.AuctionTradeable(b, q)
		out = append(out, benchRow{fmt.Sprintf("%s (bonding %d, quality %d): tradeable, on the Auction House", tc.item, b, q), fmt.Sprintf("tradeable %v, auction %v", tr, au),
			fmt.Sprintf("tradeable %v, auction %v", tc.tradeable, tc.auctions), bindTradeSr, resultIf(tr == tc.tradeable && au == tc.auctions, "tradeable when bound, or bound when tradeable")})
	}
	finish(t, "advice-binding-tradeable", "Offline item binding: tradeable and auctionable items", out)
}

// TestOfflineBenchmarkBindingNoAnswer: no bag or bank size, no Auction House price, no sell price for an item the dataset prices
// nothing for, no item it does not hold, no bank the capture did not see and no craft cooldown: the advisor says so and states none.
func TestOfflineBenchmarkBindingNoAnswer(t *testing.T) {
	ds := usabStore()
	var out []benchRow
	robe := num(usabItem(t, "Robe of Power")["id"])
	items, err := ds.Filter("items", store.Range{Column: "id", Min: robe, Max: robe})
	var cols []string
	if err == nil && len(items) == 1 {
		for col := range items[0] {
			l := strings.ToLower(col)
			if strings.Contains(l, "slots") || strings.Contains(l, "container") || strings.Contains(l, "capacity") || strings.Contains(l, "auction") {
				cols = append(cols, col)
			}
		}
	}
	out = append(out, benchRow{"how many slots a bag holds", fmt.Sprintf("columns held: %v", cols), "items holds a name, level, quality, class, slot, stats and prices only", bindNoneSrc,
		resultIf(err == nil && len(items) == 1 && len(cols) == 0, "the dataset holds a bag size the answer could state")})
	for _, table := range []string{"bag_slots", "bank_slots", "auction_prices"} {
		_, err := ds.Filter(table, store.Range{Column: "id", Min: int64(0), Max: int64(1)})
		out = append(out, benchRow{"a table of " + strings.ReplaceAll(table, "_", " "), fmt.Sprint(err != nil), "no such table in the dataset", bindNoneSrc, resultIf(err != nil, "a table was invented")})
	}
	// an item the dataset does not hold is never judged
	ghost := bindChar(t, "Magea", "MAGE", 40)
	ghost.Bags = []character.ItemStack{{ItemID: 99999999, Count: 1, Name: "Mystery Trinket"}}
	l := bindLine(inventory.ReviewAll([]character.Character{ghost}, ds).Lines, "Magea", "Mystery Trinket")
	out = append(out, benchRow{"an item the dataset does not hold: keep, sell or mail", fmt.Sprintf("%s: %s", l.Action, l.Reason), "no items row, so no binding, quality or price", bindNoneSrc,
		resultIf(l.Action == inventory.Unsure && strings.Contains(l.Reason, "does not know"), "a verdict was guessed")})
	// a Bind on Pickup piece the dataset gives no price: the vendor price stays unknown
	r := inventory.ReviewAll([]character.Character{bindChar(t, "Magea", "MAGE", 40, "Pendant of Myzrael")}, ds)
	l = bindLine(r.Lines, "Magea", "Pendant of Myzrael")
	out = append(out, benchRow{"Pendant of Myzrael: its vendor price", fmt.Sprint(l.Price), "sell_price is empty in the dataset", bindNoneSrc, resultIf(l.Price == nil, "a price was made up")})
	// an Auction House line carries no price
	r = inventory.ReviewAll([]character.Character{bindChar(t, "Magea", "MAGE", 40, "Heavy Mithril Shoulder")}, ds)
	txt := inventory.RenderAll(r, 1)
	out = append(out, benchRow{"Heavy Mithril Shoulder: what it sells for on the Auction House", firstMatch(txt, "Heavy Mithril Shoulder"), "the dataset holds no Auction House price", bindNoneSrc,
		resultIf(strings.Contains(txt, "value on the Auction House") && r.SellValue == 0, "an Auction House price was stated")})
	// a bank the capture never saw is named, not guessed
	unseen := bindChar(t, "Magea", "MAGE", 40, "Pendant of Myzrael")
	unseen.BankCaptured = false
	r = inventory.ReviewAll([]character.Character{unseen}, ds)
	out = append(out, benchRow{"a bank the capture never saw", strings.Join(r.Notes, " "), "no bank rows were captured", bindNoneSrc, resultIf(strings.Contains(strings.Join(r.Notes, " "), "bank not seen yet"), "a bank was assumed empty")})
	// a cooldown question has no figure in the facts the answer is framed with
	for _, q := range []string{"when can i transmute arcanite again?", "how long is the mooncloth cooldown?"} {
		cl := ask.Classify(q, ds, nil)
		out = append(out, benchRow{"the question \"" + q + "\"", fmt.Sprintf("kind %d, topic %q, subject %q", cl.Kind, cl.Topic, cl.Subject), "a know-how question with no dataset fact behind it: no cooldown figure in the facts", bindNoneSrc,
			resultIf(cl.Kind == ask.KindKnowHow && cl.Topic == ask.TopicNone, "a cooldown question answered as the player's own state")})
	}
	finish(t, "advice-binding-no-answer", "Offline bags, banks and cooldowns: no answer in the dataset", out)
}

// firstMatch is the first line of the text that holds the word.
func firstMatch(text, word string) string {
	for _, l := range strings.Split(text, "\n") {
		if strings.Contains(l, word) {
			return strings.TrimSpace(l)
		}
	}
	return ""
}

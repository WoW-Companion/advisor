package paths_test

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/ask"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/inventory"
	"github.com/WoW-Companion/advisor/internal/paths"
	"github.com/WoW-Companion/advisor/internal/store"
	"github.com/WoW-Companion/advisor/internal/vendors"
)

// The offline economy benchmark: what the advisor says a vendor pays for an item, whether an item goes to a vendor or the Auction House,
// how many bag slots a sale frees and what it says of bag size, what a craft or a farm earns, and what it says when the dataset has no
// answer (the Auction House, profit, gold per hour, the gold cap, repairs, respecs), beside the dataset's rows, with no network. It reads
// testdata/usability_snapshot (items: sell_price, bonding, quality, slot) and testdata/places_snapshot (recipes, vendor rows). The dataset
// holds vendor prices only, so the cases it cannot answer check that the advisor says so and states none. make offline-benchmark runs it;
// WOWC_BENCH_REPORT receives the tables.

const (
	ecoSellSrc  = "dataset items (sell_price, copper); the game's tooltip money units"
	ecoRouteSrc = "dataset items (bonding, quality, sell_price); game: Bind on Pickup items cannot be auctioned, a Bind on Equip green or better is worth more on the Auction House than to a vendor"
	ecoSlotSrc  = "dataset items (class_id, sell_price); game: one stack fills one bag slot, whatever its count"
	ecoGoldSrc  = "dataset recipes, recipe_reagents, items (sell_price): vendor prices only; no Auction House price, demand or time per kill"
	ecoNoneSrc  = "the dataset has no Auction House price, gold cap, repair cost, respec cost, guild bank rule, bag size or profit table or column"
)

var ecoSells = regexp.MustCompile(`sells to a vendor for (.+?) each \((\d+) copper\)`)

// ecoTip is the tooltip line the advisor gives for a vendor-price question about one item.
func ecoTip(ds store.DataStore, name string) string {
	f := ask.ItemTooltipFacts(ds, "how much does ["+name+"] sell for to a vendor?")
	for _, l := range strings.Split(f, "\n") {
		if strings.HasPrefix(l, "- "+name+":") {
			return l
		}
	}
	return ""
}

// TestOfflineBenchmarkEconVendorPrice: what a vendor pays for an item, as the answer's facts state it: the dataset's sell_price in the game's money units
// to the copper, the copper beside it, for a question that says "sell" and links the item.
func TestOfflineBenchmarkEconVendorPrice(t *testing.T) {
	ds := usabStore()
	var out []benchRow
	for _, name := range []string{"Heavy Mithril Shoulder", "Thick Cloth Vest", "Copper Bar", "Bolt of Woolen Cloth", "Mageweave Bag", "Robe of Power", "Linen Bag", "Deprecated Cat's Paw"} {
		want := num(usabItem(t, name)["sell_price"])
		line := ecoTip(ds, name)
		m := ecoSells.FindStringSubmatch(line)
		got := "no vendor price stated"
		ok := false
		if m != nil {
			got = m[1] + " (" + m[2] + " copper)"
			ok = copperOf(m[1]) == want && m[2] == fmt.Sprint(want)
		}
		out = append(out, benchRow{name + ": what a vendor pays", got, fmt.Sprintf("%s (%d copper)", vendors.Money(want), want), ecoSellSrc, resultIf(ok, "the price, or none stated, or a bare copper count")})
	}
	finish(t, "advice-economy-vendor-price", "Offline economy: what a vendor pays for an item", out)
}

// TestOfflineBenchmarkEconNoVendorPrice: an item the dataset holds no sell_price for: the tooltip states no vendor price, and none is made up.
func TestOfflineBenchmarkEconNoVendorPrice(t *testing.T) {
	ds := usabStore()
	var out []benchRow
	rows, err := ds.Filter("items", store.Range{Column: "id", Min: int64(0)})
	if err != nil {
		t.Fatal(err)
	}
	nulls := 0
	var first store.Row
	for _, r := range rows {
		if _, ok := r["sell_price"].(int64); !ok {
			nulls++
			if first == nil || text(r["name"]) < text(first["name"]) && text(r["name"]) != "" {
				first = r
			}
		}
	}
	if first == nil {
		t.Fatal("no item without a sell price in the snapshot")
	}
	name := text(first["name"])
	f := ask.ItemTooltipFacts(ds, "how much does ["+name+"] sell for?")
	ok := !strings.Contains(f, "sells to a vendor for") && !strings.Contains(f, "no vendor buys it")
	res := "differ (data): the dataset holds no sell_price for it, so no vendor price is given"
	if !ok {
		res = "differ: a vendor price was stated for an item the dataset does not price"
	}
	out = append(out, benchRow{fmt.Sprintf("%s (1 of %d items with no sell_price): what a vendor pays", name, nulls), firstLine(strings.TrimPrefix(f, "Item tooltips (from the item rows; answer from these, not from memory):\n")), "a vendor price in the game; sell_price is null in the dataset", ecoSellSrc, res})
	finish(t, "advice-economy-vendor-price-none", "Offline economy: an item with no sell price", out)
}

func ecoReview(t *testing.T, ds store.DataStore, names ...string) (inventory.CrossResult, string) {
	t.Helper()
	r := inventory.ReviewAll([]character.Character{bindChar(t, "Magea", "MAGE", 40, names...)}, ds)
	return r, inventory.RenderAll(r, 1)
}

func ecoHead(txt, prefix string) string {
	for _, l := range strings.Split(txt, "\n") {
		if strings.HasPrefix(l, prefix) {
			return l
		}
	}
	return ""
}

// TestOfflineBenchmarkEconSellVersusAuction: where an item goes, to a vendor or to the Auction House, and what the Sell heading says it is worth: a vendor
// stack is worth its sell_price, an Auction House stack has no price in the dataset and is left out of the total, and a heading that is all Auction House
// stacks says so instead of a zero.
func TestOfflineBenchmarkEconSellVersusAuction(t *testing.T) {
	ds := usabStore()
	var out []benchRow
	shoulder := num(usabItem(t, "Heavy Mithril Shoulder")["sell_price"])
	hat := num(usabItem(t, "Dread Mage Hat")["sell_price"])
	wrist := num(usabItem(t, "Sergeant Major's Plate Wristguards")["sell_price"])

	r, txt := ecoReview(t, ds, "Heavy Mithril Shoulder")
	h := ecoHead(txt, "Sell (")
	out = append(out, benchRow{"a Bind on Equip green nobody can use: the Sell heading", h, "Auction House, no price in the data: not a vendor value of 0", ecoRouteSrc,
		resultIf(strings.Contains(h, "Auction House") && !strings.Contains(h, "0g 0s 0c") && r.SellValue == 0 && r.SellAuction == 1, "a zero vendor value stood for an Auction House sale")})

	r, txt = ecoReview(t, ds, "Sergeant Major's Plate Wristguards")
	h = ecoHead(txt, "Sell (")
	out = append(out, benchRow{"a Bind on Pickup rare nobody can use: the Sell heading", h, fmt.Sprintf("vendor, %s (%d copper): it cannot be auctioned", vendors.Money(wrist), wrist), ecoRouteSrc,
		resultIf(copperOf(h) == wrist && !strings.Contains(h, "Auction House") && r.SellAuction == 0, "the vendor value or an Auction House sale")})

	r, txt = ecoReview(t, mbGrey(t, "Dread Mage Hat"), "Dread Mage Hat", "Heavy Mithril Shoulder")
	h = ecoHead(txt, "Sell (")
	out = append(out, benchRow{"a grey hat and a Bind on Equip green: the Sell heading", h, fmt.Sprintf("%s at a vendor for the hat, the green left to the Auction House with no price", vendors.Money(hat)), ecoRouteSrc,
		resultIf(copperOf(strings.SplitN(h, "at a vendor", 2)[0]) == hat && strings.Contains(h, "1 stack(s) for the Auction House") && r.SellValue == hat, "the vendor value or the Auction House stack")})

	r, txt = ecoReview(t, ds, "Copper Bar")
	l := bindLine(r.Lines, "Magea", "Copper Bar")
	out = append(out, benchRow{"Copper Bar, no character has a profession for it: sell where", fmt.Sprintf("%s: %s; AH %v", l.Action, l.Reason, l.AH), "the Auction House: a white trade good that binds to no one", ecoRouteSrc,
		resultIf(l.Action == inventory.Sell && l.AH && strings.Contains(ecoHead(txt, "Sell ("), "Auction House"), "the route or the heading")})

	f := ask.ItemTooltipFacts(ds, "should I vendor or auction [Robe of Power]?")
	out = append(out, benchRow{"a Bind on Pickup robe: vendor or Auction House", firstMatch(f, "Robe of Power"), "vendor only: a soulbound item cannot be put on the Auction House", ecoRouteSrc,
		resultIf(strings.Contains(f, "cannot be traded, mailed or put on the Auction House, only sold to a vendor"), "the rule")})

	f = ask.ItemTooltipFacts(ds, "should I vendor or auction [Heavy Mithril Shoulder]?")
	out = append(out, benchRow{"a Bind on Equip green: vendor or Auction House", firstMatch(f, "Heavy Mithril Shoulder"), "both are open until it is worn: the facts carry the vendor price and no Auction House price", ecoRouteSrc,
		resultIf(strings.Contains(f, "put on the Auction House") && strings.Contains(f, vendors.Money(shoulder)) && strings.Contains(f, "no Auction House price"), "the rule or the price")})
	finish(t, "advice-economy-sell-versus-auction", "Offline economy: vendor or Auction House", out)
}

// TestOfflineBenchmarkEconBagSpace: the bag slots a sale frees, one per stack in the bags and none from the bank, an Auction House stack counted like a vendor
// stack, and what the advisor says of a bag's size and a bank slot: the dataset holds none.
func TestOfflineBenchmarkEconBagSpace(t *testing.T) {
	ds := usabStore()
	var out []benchRow
	r, _ := ecoReview(t, ds, "Heavy Mithril Shoulder", "Copper Bar", "Bolt of Woolen Cloth")
	out = append(out, benchRow{"three trade stacks sold on the Auction House from the bags: slots freed", fmt.Sprint(r.SlotsFreed), "3: one slot a stack", ecoSlotSrc, resultIf(r.SlotsFreed == 3, "the slots freed")})

	c := bindChar(t, "Magea", "MAGE", 40)
	c.Bank = append(c.Bank, mbStack(t, "Copper Bar", 20))
	r = inventory.ReviewAll([]character.Character{c}, ds)
	out = append(out, benchRow{"a stack of 20 Copper Bar in the bank, sold: slots freed in the bags", fmt.Sprint(r.SlotsFreed), "0: the bank stack fills no bag slot", ecoSlotSrc, resultIf(r.SlotsFreed == 0, "a bank stack freed a bag slot")})

	_, txt := ecoReview(t, ds, "Mageweave Bag", "Linen Bag")
	out = append(out, benchRow{"two bags in the bags: kept or sold", firstMatch(txt, "Mageweave Bag"), "neither: the data holds no bag size, so it cannot say which is bigger; no slot freed", ecoSlotSrc,
		resultIf(strings.Contains(txt, "Bag slots freed: 0") && strings.Contains(txt, "no bag size") && !strings.Contains(txt, "Sell ("), "a bag was sold or ranked")})

	for _, q := range []string{"should I buy a bigger bag?", "what is the biggest bag I can craft?", "which bag is the largest?", "how many slots does the biggest bag have?"} {
		f := ask.LimitFacts(q)
		out = append(out, benchRow{"\"" + q + "\": the facts", firstLine(f), "the data holds no bag size or bank slot price: say so, name no slot count", ecoNoneSrc,
			resultIf(strings.Contains(f, "Bag and bank size") && strings.Contains(f, "Say so"), "no statement that the data holds none")})
	}
	finish(t, "advice-economy-bag-space", "Offline economy: bag slots a sale frees and bag size", out)
}

var ecoMoneyFigure = regexp.MustCompile(`\d+ ?(g|gold|s|silver|c|copper)\b`)

// TestOfflineBenchmarkEconGoldMaking: gold-making questions get the line that the dataset holds vendor prices only, so no market price, profit or gold per hour
// is named and no profession is ranked by gold.
func TestOfflineBenchmarkEconGoldMaking(t *testing.T) {
	var out []benchRow
	for _, q := range []string{
		"what is the best way to make gold at level 40?",
		"which profession makes the most gold?",
		"is crafting Elixir of Minor Fortitude profitable?",
		"how much gold per hour can I make farming Wool Cloth?",
		"what sells best on the auction house?",
		"is it worth crafting Linen Bandages to sell?",
		"is enchanting or mining better for making money?",
	} {
		f := ask.LimitFacts(q)
		out = append(out, benchRow{"\"" + q + "\": the facts", firstLine(f), "the data holds vendor prices only: say so, name no market price, profit or gold per hour", ecoGoldSrc,
			resultIf(strings.Contains(f, "Gold making:") && strings.Contains(f, "Say so") && !ecoMoneyFigure.MatchString(f), "no statement that the data holds no market price")})
	}
	finish(t, "advice-economy-gold-making", "Offline economy: gold-making questions", out)
}

// TestOfflineBenchmarkEconCraftForProfit: for a craft, the vendor value of what it makes beside the vendor price of what it takes, from the dataset's rows, and the
// answer: it states the materials' price and the item, never a profit, margin or Auction House price.
func TestOfflineBenchmarkEconCraftForProfit(t *testing.T) {
	ds := mbNamed()
	profit := regexp.MustCompile(`(?i)\b(profit|margin|worth it|gold per hour|per hour)\b`)
	var out []benchRow
	for _, tc := range []struct {
		item int64
		prof string
	}{{858, "First Aid"}, {2311, "Leatherworking"}, {3473, "Blacksmithing"}, {2848, "Blacksmithing"}} {
		res := paths.For(mbChar(40, "Horde", map[string]int{tc.prof: 300}), ds, tc.item, paths.Options{})
		p, ok := moneyCraft(res)
		if !ok {
			t.Fatalf("%d: no craft path: %s", tc.item, res.Text())
		}
		var cost int64
		priced := true
		for _, g := range p.Reagents {
			u := moneyUnit(t, ds, g.ItemID)
			if u == 0 {
				priced = false
			}
			cost += u * int64(g.Need)
		}
		it, err := ds.Get("items", tc.item)
		if err != nil {
			t.Fatal(err)
		}
		sell := num(it["sell_price"])
		ref := fmt.Sprintf("makes an item a vendor pays %s for; its materials cost %s at a vendor (every one priced: %v); the Auction House price is not in the data", moneyWords(sell), moneyWords(cost), priced)
		out = append(out, benchRow{res.Item + ": the profit of crafting it", firstLine(strings.SplitN(res.Text(), "\n", 3)[1]), ref, ecoGoldSrc,
			resultIf(!profit.MatchString(res.Text()) && !ecoAHFigure(res.Text()), "a profit or an Auction House price was stated")})
	}
	finish(t, "advice-economy-craft-for-profit", "Offline economy: crafting for profit", out)
}

// ecoAHFigure is true for a text that names a price beside the Auction House.
func ecoAHFigure(s string) bool {
	return regexp.MustCompile(`(?i)auction house[^.\n]*\d+ ?(g|s|c|gold|silver|copper)\b`).MatchString(s)
}

// TestOfflineBenchmarkEconNoAnswer: what the dataset does not hold about money: the Auction House's price, fee and listing time, the gold cap, a repair bill, a respec and a
// guild bank. The advisor says so and names no figure; a vendor price question is not answered with a no.
func TestOfflineBenchmarkEconNoAnswer(t *testing.T) {
	var out []benchRow
	for _, tc := range []struct{ q, word string }{
		{"how much is Linen Cloth worth on the AH?", "Auction House:"},
		{"how long can I list an item on the auction house for?", "Auction House:"},
		{"how much does Thorium Ore sell for at the auction house?", "Auction House:"},
		{"can I sell soulbound items on the auction house?", "Auction House:"},
		{"what is the gold cap?", "Gold and fees:"},
		{"how much does it cost to repair my gear?", "Gold and fees:"},
		{"how much does it cost to respec my talents?", "Gold and fees:"},
		{"can I put gold in the guild bank?", "Gold and fees:"},
	} {
		f := ask.LimitFacts(tc.q)
		out = append(out, benchRow{"\"" + tc.q + "\": the facts", firstLine(f), "the data holds none: say so, name no figure", ecoNoneSrc,
			resultIf(strings.Contains(f, tc.word) && strings.Contains(f, "Say so") && !ecoMoneyFigure.MatchString(f), "no statement that the data holds none")})
	}
	for _, q := range []string{"how much does a Mageweave Bag sell for to a vendor?", "what does a vendor pay for Linen Cloth?", "where do I sell my grey items?"} {
		f := ask.LimitFacts(q)
		out = append(out, benchRow{"\"" + q + "\": a question the data answers", f, "no limit facts: the dataset holds the vendor price", ecoSellSrc, resultIf(f == "", "a vendor price question was met with a statement that the data holds none")})
	}
	out = append(out,
		benchRow{"an Auction House price for any item", "none stated", "a price in gold (Auction House listings)", ecoNoneSrc, "differ (data): the dataset holds no Auction House price"},
		benchRow{"profit or gold per hour for a craft, a farm or a profession", "none stated", "gold per hour (gold guides)", ecoNoneSrc, "differ (data): the dataset holds no demand, Auction House price or time per kill"})
	finish(t, "advice-economy-no-answer", "Offline economy: no answer in the dataset", out)
}

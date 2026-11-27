package paths_test

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/farm"
	"github.com/WoW-Companion/advisor/internal/inventory"
	"github.com/WoW-Companion/advisor/internal/paths"
	"github.com/WoW-Companion/advisor/internal/store"
	"github.com/WoW-Companion/advisor/internal/vendors"
)

// The offline crafting-cost, vendor-price, craft-versus-buy, gold-farming and no-price benchmark: what the advisor says a craft's
// materials cost, what a vendor charges and pays, whether to make or buy, and what farming a drop is worth, beside the dataset's
// rows, with no network. It reads the places and advice snapshots (vendor_sources, npcs, items, recipes, recipe_reagents,
// loot_sources). The dataset holds only vendor prices (items.buy_price, items.sell_price and the vendor rows' price_copper): no
// Auction House price, no time per kill, no profit, so the cases it cannot answer check that the advisor says so and states none.
// make offline-benchmark runs it; WOWC_BENCH_REPORT receives the tables.

const (
	moneyCostSrc  = "dataset recipe_reagents (count), vendor_sources (price_copper), items (buy_price): the dearer of the lowest row and the buy price"
	moneyBuySrc   = "dataset vendor_sources (price_copper), items (buy_price); the game's tooltip money units"
	moneySellSrc  = "dataset items (sell_price, quality 0 stands for a grey); the game's tooltip money units"
	moneyVsSrc    = "dataset vendor_sources, items (buy_price), recipe_reagents; guides: make it when the materials are in your bags, else buy it when it is cheap"
	moneyFarmSrc  = "dataset loot_sources (kills, drops), items (sell_price); guides: farm what drops often and sells well"
	moneyNoneSrc  = "the dataset has no Auction House price, profit, time per kill or gold per hour"
	moneyEachWord = "each"
)

// moneyFor is the copper the first "for <money>" of a reagent line states, and whether it says "each".
func moneyFor(how string) (copper int64, each bool) {
	i := strings.Index(how, " for ")
	if i < 0 {
		return 0, false
	}
	rest := how[i+len(" for "):]
	if j := strings.IndexAny(rest, "(,"); j >= 0 {
		rest = rest[:j]
	}
	return copperOf(rest), strings.Contains(rest, moneyEachWord)
}

// moneyUnit is the price of one unit of an item the dataset sells through a vendor: the dearer of the lowest vendor row
// and the item's buy price, as the advisor prices it; 0 when no vendor row carries a price.
func moneyUnit(t *testing.T, ds store.DataStore, id int64) int64 {
	t.Helper()
	low := int64(0)
	for _, r := range mbRows(t, ds, "vendor_sources", "item_id", id) {
		if p := num(r["price_copper"]); p > 0 && (low == 0 || p < low) {
			low = p
		}
	}
	if low == 0 {
		return 0
	}
	it, err := ds.Get("items", id)
	if err != nil {
		t.Fatal(err)
	}
	if b := num(it["buy_price"]); b > low {
		return b
	}
	return low
}

func moneyCraft(res paths.Result) (paths.Path, bool) {
	for _, p := range res.Paths {
		if p.Kind == paths.Craft && !p.Informational {
			return p, true
		}
	}
	return paths.Path{}, false
}

// ---- crafting reagent cost ----

var moneyCrafts = []struct {
	item int64
	prof string
}{
	{858, "First Aid"}, {118, "First Aid"}, {2311, "Leatherworking"}, {2315, "Leatherworking"}, {3473, "Blacksmithing"}, {2848, "Blacksmithing"}, {250621, "Blacksmithing"},
}

// TestOfflineBenchmarkCraftReagentCostA and B: what the materials a vendor sells cost for a craft: the price a reagent line states, read
// as a reader does (a price with "each" is per unit, one without is for the line), beside the unit price times the count the
// recipe lists. A material no vendor sells carries no price.
func TestOfflineBenchmarkCraftReagentCostA(t *testing.T) { craftReagentCost(t, "a", moneyCrafts[:4]) }
func TestOfflineBenchmarkCraftReagentCostB(t *testing.T) { craftReagentCost(t, "b", moneyCrafts[4:]) }

func craftReagentCost(t *testing.T, part string, cases []struct {
	item int64
	prof string
}) {
	ds := mbNamed()
	var out []benchRow
	for _, tc := range cases {
		res := paths.For(mbChar(40, "Horde", map[string]int{tc.prof: 300}), ds, tc.item, paths.Options{})
		p, ok := moneyCraft(res)
		if !ok {
			t.Fatalf("%d: no craft path: %s", tc.item, res.Text())
		}
		var got, want int64
		var parts []string
		for _, g := range p.Reagents {
			unit := moneyUnit(t, ds, g.ItemID)
			want += unit * int64(g.Need)
			c, each := moneyFor(g.How)
			if each {
				c *= int64(g.Need)
			}
			if unit == 0 && c != 0 {
				c = -1 << 30 // a price with no vendor row behind it
			}
			got += c
			parts = append(parts, fmt.Sprintf("%dx %s %s", g.Need, g.Name, moneyWords(unit*int64(g.Need))))
		}
		out = append(out, benchRow{res.Item + ": what the vendor-sold materials cost", fmt.Sprintf("%s (%s)", moneyWords(got), strings.Join(parts, ", ")),
			moneyWords(want) + " from recipe_reagents and the vendor rows", moneyCostSrc, resultIf(got == want, "the materials' price, or a unit price read as a total")})
	}
	finish(t, "advice-craft-reagent-cost-"+part, "Offline crafting cost: the price of the materials a vendor sells", out)
}

func moneyWords(c int64) string {
	if c < 0 {
		return "a price with no vendor row"
	}
	if c == 0 {
		return "no vendor price"
	}
	return vendors.Money(c)
}

// ---- vendor buy price ----

var moneyBuys = []int64{2320, 2321, 2324, 4291, 4340, 2325, 2880}

// TestOfflineBenchmarkVendorBuyPrice: the price the vendor path states for a material a vendor sells, beside the dataset's price to the copper.
func TestOfflineBenchmarkVendorBuyPrice(t *testing.T) {
	ds := mbNamed()
	var out []benchRow
	for _, id := range moneyBuys {
		want := moneyUnit(t, ds, id)
		res := paths.For(mbChar(40, "Horde", nil), ds, id, paths.Options{})
		got, line := int64(0), "no vendor path"
		for _, p := range res.Paths {
			if p.Kind == paths.Vendor {
				c, _ := moneyFor(p.Line)
				got, line = c, p.Line
			}
		}
		out = append(out, benchRow{res.Item + ": the vendor's price", line, fmt.Sprintf("%s (%d copper)", vendors.Money(want), want), moneyBuySrc, resultIf(want > 0 && got == want, "the price")})
	}
	finish(t, "advice-vendor-buy-price", "Offline vendor price: what a vendor charges", out)
}

// ---- vendor sell price ----

// moneyGrey makes the named items grey, as a grey item of the same price would be: every other column is the dataset's.
type moneyGrey struct {
	store.DataStore
	ids map[int64]bool
}

func (m moneyGrey) Get(table string, id int64) (store.Row, error) {
	r, err := m.DataStore.Get(table, id)
	if table == "items" && err == nil && m.ids[id] {
		c := store.Row{}
		for k, v := range r {
			c[k] = v
		}
		c["quality"] = int64(0)
		return c, nil
	}
	return r, err
}

var moneySells = []struct {
	item  int64
	count int
}{{2589, 20}, {2324, 7}, {4340, 3}, {2997, 5}, {2325, 50}, {4291, 30}}

// TestOfflineBenchmarkVendorSellPrice: what the bag review says a stack of grey items sells for, beside count times the item's
// sell_price, to the copper, and the vendor value of all of them together.
func TestOfflineBenchmarkVendorSellPrice(t *testing.T) {
	ids := map[int64]bool{}
	level := 30
	c := character.Character{Name: "Benchtest", Faction: "Horde", Class: "MAGE", Level: &level, BagsCaptured: true, BankCaptured: true}
	for _, tc := range moneySells {
		ids[tc.item] = true
		c.Bags = append(c.Bags, character.ItemStack{ItemID: int(tc.item), Count: tc.count})
	}
	ds := moneyGrey{DataStore: qbStore(), ids: ids}
	rev := inventory.Review(c, ds)
	review := inventory.Render(rev)
	var out []benchRow
	var total int64
	for _, tc := range moneySells {
		it, err := ds.Get("items", tc.item)
		if err != nil {
			t.Fatal(err)
		}
		want := num(it["sell_price"]) * int64(tc.count)
		total += want
		got := ""
		for _, l := range strings.Split(review, "\n") {
			if i := strings.Index(l, "sells for "); i >= 0 && strings.HasPrefix(l, "- "+text(it["name"])+" x") {
				got = l[i+len("sells for "):]
			}
		}
		out = append(out, benchRow{fmt.Sprintf("%d %s: sells for", tc.count, text(it["name"])), got, fmt.Sprintf("%s (%d copper: %d each)", vendors.Money(want), want, num(it["sell_price"])), moneySellSrc, resultIf(want > 0 && copperOf(got) == want, "the amount in copper")})
	}
	line := ""
	for _, l := range strings.Split(review, "\n") {
		if strings.HasPrefix(l, "Vendor value: ") {
			line = l
		}
	}
	out = append(out, benchRow{"all six stacks: the vendor value", line, fmt.Sprintf("%s (%d copper)", vendors.Money(total), total), moneySellSrc, resultIf(copperOf(line) == total, "the total in copper")})
	finish(t, "advice-vendor-sell-price", "Offline vendor price: what a vendor pays", out)
}

// ---- craft versus buy ----

var moneyVs = []struct {
	label string
	item  int64
	prof  string
	bags  map[int]int // materials in the bags
	want  string      // the first way
	why   string
}{
	{"Lesser Healing Potion, no materials", 858, "First Aid", nil, paths.Vendor, "the vendor's price is under the cheap line and the herb is not in the bags"},
	{"Lesser Healing Potion, every material in the bags", 858, "First Aid", map[int]int{2450: 1, 2678: 2, 3371: 1}, paths.Craft, "the materials are in the bags: making it costs nothing more"},
	{"Copper Rod, no materials", 6217, "Blacksmithing", nil, paths.Vendor, "the vendor's price is under the cheap line"},
	{"Copper Rod, the bar in the bags", 6217, "Blacksmithing", map[int]int{2840: 1}, paths.Craft, "the one material is in the bags"},
	{"Coarse Blasting Powder, no materials", 4364, "Engineering", nil, paths.Vendor, "the vendor's price is under the cheap line"},
	{"Elixir of Minor Fortitude, every material in the bags", 2458, "Alchemy", map[int]int{2447: 1, 2449: 2, 3371: 1}, paths.Craft, "the materials are in the bags: no need to buy it"},
}

// TestOfflineBenchmarkCraftVersusBuy: the way the advisor puts first for an item a craft makes: a craft whose materials are all in the bags comes before
// buying it, a cheap vendor price before a craft that still needs materials, and the answer states no profit, which the dataset cannot know.
func TestOfflineBenchmarkCraftVersusBuy(t *testing.T) {
	ds := mbNamed()
	profit := regexp.MustCompile(`(?i)\b(profit|margin|worth it|gold per hour|per hour)\b`)
	var out []benchRow
	for _, tc := range moneyVs {
		c := mbChar(30, "Horde", map[string]int{tc.prof: 100})
		for id, n := range tc.bags {
			c.Bags = append(c.Bags, character.ItemStack{ItemID: id, Count: n})
		}
		res := paths.For(c, ds, tc.item, paths.Options{})
		first := ""
		if len(res.Paths) > 0 {
			first = res.Paths[0].Kind
		}
		ok := first == tc.want && !profit.MatchString(res.Text())
		out = append(out, benchRow{tc.label + ": the first way", first + ": " + firstLine(strings.SplitN(res.Text(), "\n", 3)[1]), tc.want + ": " + tc.why, moneyVsSrc, resultIf(ok, "the order of the ways, or a profit was stated")})
	}
	finish(t, "advice-craft-versus-buy", "Offline craft versus buy: the way put first", out)
}

// ---- gold farming ----

var moneyFarms = []int64{2592, 4306, 4338, 2319, 2771, 3355}

// TestOfflineBenchmarkGoldFarming: a farming answer for a drop that sells well: the best chance the farm spots give beside the dataset's kills and drops,
// and what a vendor pays for the drop, which the dataset holds; no gold per hour, which it does not.
func TestOfflineBenchmarkGoldFarming(t *testing.T) {
	ds := qbStore()
	var out []benchRow
	for _, id := range moneyFarms {
		it, err := ds.Get("items", id)
		if err != nil {
			t.Fatal(err)
		}
		name := text(it["name"])
		spots := farm.Spots(horde(30), ds, id)
		if len(spots) == 0 {
			t.Fatalf("%s: no farm spot", name)
		}
		render := farm.Render(name, spots, false, 5) + farm.Value(ds, id)
		best := -1.0
		for _, r := range mbRows(t, ds, "loot_sources", "item_id", id) {
			if k := num(r["kills"]); k > 0 && text(r["source_kind"]) != "crafted" {
				if c := 100 * float64(num(r["drops"])) / float64(k); c > best {
					best = c
				}
			}
		}
		sell := num(it["sell_price"])
		top := spots[0]
		ok := top.Chance != nil && *top.Chance >= best-0.05 && strings.Contains(render, fmt.Sprintf("%.1f%% of %d loot windows", *top.Chance, top.Kills))
		ok = ok && sell > 0 && strings.Contains(render, "A vendor pays "+vendors.Money(sell)+" for each") && !regexp.MustCompile(`\d+ ?(g|gold)? per hour`).MatchString(render)
		out = append(out, benchRow{name + ": farm it, and what it sells for", fmt.Sprintf("best chance %.1f%%; %s", *top.Chance, vendorLine(render)), fmt.Sprintf("best chance %.1f%% in loot_sources; %s each to a vendor", best, vendors.Money(sell)), moneyFarmSrc, resultIf(ok, "the chance, or the sell price, or a gold-per-hour figure")})
	}
	finish(t, "advice-gold-farming", "Offline gold farming: the best spot and what the drop sells for", out)
}

func vendorLine(render string) string {
	for _, l := range strings.Split(render, "\n") {
		if strings.HasPrefix(l, "A vendor pays") {
			return strings.SplitN(l, ". ", 2)[0]
		}
	}
	return "no value stated"
}

// ---- no answer in the dataset ----

// TestOfflineBenchmarkMoneyNoAnswer: what the dataset cannot price: an Auction House price, a profit, a gold-per-hour figure, a vendor price for an item no vendor sells,
// and a grey item with no sell price. The advisor says so and states none.
func TestOfflineBenchmarkMoneyNoAnswer(t *testing.T) {
	ds := mbNamed()
	figure := regexp.MustCompile(`\d+ ?[gsc]\b`)
	var out []benchRow
	c := mbChar(30, "Horde", map[string]int{"Alchemy": 100})
	res := paths.For(c, ds, 2458, paths.Options{})
	ah := ""
	for _, p := range res.Paths {
		if p.Kind == paths.Auction {
			ah = p.Line
		}
	}
	out = append(out, benchRow{"Elixir of Minor Fortitude: the Auction House price", ah, "none: " + moneyNoneSrc, moneyNoneSrc, resultIf(ah != "" && !figure.MatchString(ah), "a price was stated, or no Auction House way")})
	g, _ := mbReagent(t, c, ds, 2458, "Earthroot")
	out = append(out, benchRow{"Earthroot, bought on the Auction House for the elixir: its price", g.How, "none: " + moneyNoneSrc, moneyNoneSrc, resultIf(mbKind(g.How) == "auction" && !figure.MatchString(g.How), "a price was stated")})
	profit := regexp.MustCompile(`(?i)\b(profit|margin|gold per hour|worth it)\b`)
	out = append(out, benchRow{"Elixir of Minor Fortitude: the profit of crafting it", firstLine(res.Text()), "none: " + moneyNoneSrc, moneyNoneSrc, resultIf(!profit.MatchString(res.Text()), "a profit was stated")})
	value := farm.Value(ds, 2592)
	out = append(out, benchRow{"Wool Cloth: gold per hour of farming", strings.TrimSpace(value), "none: no time per kill or Auction House price in the dataset", moneyNoneSrc, resultIf(strings.Contains(value, "no gold per hour is given") && !regexp.MustCompile(`\d+ ?(g|gold)? per hour`).MatchString(value), "a gold-per-hour figure was stated")})
	// Peacebloom has a buy price and no vendor row: no vendor sells it
	res = paths.For(mbChar(30, "Horde", nil), ds, 2447, paths.Options{})
	vend := false
	for _, p := range res.Paths {
		vend = vend || p.Kind == paths.Vendor
	}
	out = append(out, benchRow{"Peacebloom: a vendor's price (items.buy_price is 40, no vendor row)", firstLine(res.Text()), "none: no vendor row sells it", moneyBuySrc, resultIf(!vend && !strings.Contains(res.Text(), "Buy from"), "a vendor was named")})
	finish(t, "advice-money-no-answer-price", "Offline money: no answer in the dataset", out)
}

// TestOfflineBenchmarkMoneyNoAnswerData: a grey item the dataset holds no sell price for, and the gaps the dataset itself causes.
func TestOfflineBenchmarkMoneyNoAnswerData(t *testing.T) {
	ds := mbNamed()
	var out []benchRow
	// a grey item the dataset holds no sell price for
	level := 30
	cc := character.Character{Name: "Benchtest", Faction: "Horde", Class: "MAGE", Level: &level, BagsCaptured: true, BankCaptured: true, Bags: []character.ItemStack{{ItemID: 13367, Count: 2}}}
	rev := inventory.Render(inventory.Review(cc, consStore()))
	out = append(out, benchRow{"Wrapped Gift, grey: what it sells for", strings.TrimSpace(vendorValue(rev)), "none: items.sell_price is null", moneySellSrc, resultIf(strings.Contains(rev, "sell price unknown") && strings.Contains(rev, "Vendor value: unknown"), "a price was stated")})
	// the gaps the dataset itself causes: no price to state, and a smelting recipe its snapshot lacks
	out = append(out,
		benchRow{"an Auction House price for any item", "none stated", "a price in gold (Auction House listings)", moneyNoneSrc, "differ (data): the dataset holds no Auction House price"},
		benchRow{"gold per hour for any farm", "none stated", "gold per hour (farming guides)", moneyNoneSrc, "differ (data): the dataset holds no time per kill or Auction House price"})
	bar := paths.For(mbChar(30, "Horde", map[string]int{"Mining": 100}), ds, 2840, paths.Options{})
	recs, _ := ds.Filter("recipes", store.Range{Column: "creates_item_id", Min: int64(2840), Max: int64(2840)})
	loots := mbRows(t, ds, "loot_sources", "item_id", 2840)
	out = append(out, benchRow{"Copper Bar: how a Miner makes it", firstLine(bar.Text()), "smelt Copper Ore with Mining (skill 1)", "dataset loot_sources (a crafted row names the smelt spell), recipes (no row for it)",
		map[bool]string{true: "differ (data): the snapshot's recipes table lacks the smelt spell", false: "differ: a way was invented, or the recipe row is there"}[len(recs) == 0 && len(loots) > 0 && !bar.Dependable()]})
	finish(t, "advice-money-no-answer-data", "Offline money: no answer in the dataset", out)
}

func vendorValue(rev string) string {
	for _, l := range strings.Split(rev, "\n") {
		if strings.Contains(l, "sell price unknown") || strings.HasPrefix(l, "Vendor value") {
			return l
		}
	}
	return rev
}

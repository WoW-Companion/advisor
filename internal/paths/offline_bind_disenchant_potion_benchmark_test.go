package paths_test

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/WoW-Companion/advisor/internal/ask"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/inventory"
	"github.com/WoW-Companion/advisor/internal/store"
	"github.com/WoW-Companion/advisor/internal/vendors"
)

// The offline bind type, disenchant, sell-or-auction and potion benchmark: what the advisor says of how an item binds (bind on equip, bind on
// pickup, bind on use, quest item), what an item disenchants into and which cannot be disenchanted, whether a stack goes to a vendor or the Auction
// House and what the vendor pays, and a potion or elixir's cooldown and battle or guardian stacking, beside the dataset's rows, with no network. It
// reads testdata/usability_snapshot (items: bonding, quality, class, sell_price) and testdata/advice_snapshot (items, disenchant_yields: Everloot's
// bands per item class, quality and item level). The dataset holds no potion cooldown, elixir duration, battle or guardian category, disenchant
// flag for an item the game refuses, Enchanting skill needed or Auction House price, so the cases it cannot answer check that the advisor says so and
// states none. make offline-benchmark runs it; WOWC_BENCH_REPORT receives the tables.

const (
	bdBindSrc = "dataset items (bonding 0 none, 1 Bind on Pickup, 2 Bind on Equip, 3 Bind on Use, 4 quest; class_id 12 quest); game: item tooltip binding"
	bdDESrc   = "dataset disenchant_yields (item_class, quality, min/max_item_level, material_item_id, chance, min/max_count), items (class_id, quality, item_level); game: only uncommon or better armor and weapons disenchant"
	bdSellSrc = "dataset items (sell_price, bonding, quality, class_id); game: Bind on Pickup items cannot be auctioned, an uncommon or better tradeable item is worth more on the Auction House than to a vendor"
	bdPotSrc  = "dataset items (the whole row: no cooldown, duration, category or effect column); game: potion cooldowns, battle and guardian elixirs"
	bdNoneSrc = "the dataset has no such column, row or band"
)

var (
	bdAdviceOnce  sync.Once
	bdAdviceCache store.DataStore
)

func bdAdvice() store.DataStore {
	bdAdviceOnce.Do(func() { bdAdviceCache = store.NewDir(qbAdviceDir) })
	return bdAdviceCache
}

// bdTip is the tooltip line the advisor gives for one linked item.
func bdTip(ds store.DataStore, name string) string {
	f := ask.ItemTooltipFacts(ds, "is ["+name+"] bind on equip or bind on pickup, and can i sell or mail it?")
	for _, l := range strings.Split(f, "\n") {
		if strings.HasPrefix(l, "- "+name+":") {
			return l
		}
	}
	return ""
}

// TestOfflineBenchmarkBindType: how an item binds, as the advisor's tooltip states it, beside the dataset's bonding (and class for a quest item).
func TestOfflineBenchmarkBindType(t *testing.T) {
	ds := usabStore()
	var out []benchRow
	for _, tc := range []struct{ item, kind, trade string }{
		{"Heavy Mithril Shoulder", "Bind on Equip", "binds when equipped"},
		{"Feathered Mantle", "Bind on Equip", "binds when equipped"},
		{"Robe of Power", "Bind on Pickup", "cannot be traded, mailed or put on the Auction House"},
		{"Sergeant Major's Plate Wristguards", "Bind on Pickup", "cannot be traded, mailed or put on the Auction House"},
		{"Deprecated Cat's Paw", "Bind on Use", "binds when used"},
		{"Ryedol's Lucky Pick", "quest item", "cannot be traded, mailed or put on the Auction House"},
		{"Red Power Crystal", "quest item", "cannot be traded, mailed or put on the Auction House"},
		{"Copper Bar", "", "binds to no one"},
	} {
		row := usabItem(t, tc.item)
		b := num(row["bonding"])
		line := bdTip(ds, tc.item)
		want := tc.kind
		if want == "" {
			want = "no binding"
		}
		ok := strings.Contains(line, tc.trade) && (tc.kind == "" && !strings.Contains(line, "Bind on") || tc.kind != "" && strings.Contains(line, tc.kind))
		out = append(out, benchRow{fmt.Sprintf("%s: how it binds", tc.item), bdTipHead(line), fmt.Sprintf("%s (bonding %d, class %d): %s", want, b, num(row["class_id"]), tc.trade), bdBindSrc,
			resultIf(ok, "the wrong binding, or a trade rule that does not follow from it")})
	}
	finish(t, "advice-bind-type", "Offline item binding: bind on equip, bind on pickup, bind on use and quest items", out)
}

func bdTipHead(s string) string {
	if i := strings.Index(s, "; stack size"); i > 0 {
		return s[:i]
	}
	return s
}

// deBands is the dataset's rows for an item's class, quality and item level, the way the game's Enchanting bands are read.
func deExpected(t *testing.T, ds store.DataStore, row store.Row) (names []string) {
	t.Helper()
	bands, err := ds.Filter("disenchant_yields", store.Range{Column: "band_id", Min: int64(0)})
	if err != nil {
		t.Fatal(err)
	}
	class := map[int64]string{2: "weapon", 4: "armor"}[num(row["class_id"])]
	quality := map[int64]string{2: "uncommon", 3: "rare", 4: "epic"}[num(row["quality"])]
	for _, b := range bands {
		if text(b["item_class"]) == class && text(b["quality"]) == quality && num(row["item_level"]) >= num(b["min_item_level"]) && num(row["item_level"]) <= num(b["max_item_level"]) && text(b["state"]) != "none" {
			name := fmt.Sprintf("item %d", num(b["material_item_id"]))
			if m, err := ds.Get("items", num(b["material_item_id"])); err == nil && text(m["name"]) != "" {
				name = text(m["name"])
			}
			names = append(names, fmt.Sprintf("%s %.0f%%", name, bdF64(b["chance"])*100))
		}
	}
	return names
}

func bdF64(v any) float64 { f, _ := v.(float64); return f }

// deLine is the advisor's disenchant line for one linked item.
func deLine(ds store.DataStore, name string) string {
	f := ask.DisenchantFacts(ds, "what does ["+name+"] disenchant into?")
	for _, l := range strings.Split(f, "\n") {
		if strings.HasPrefix(l, "- "+name+":") {
			return l
		}
	}
	return ""
}

// TestOfflineBenchmarkDisenchantYields: what a disenchantable item gives, as the advisor states it: every material and its chance in the band, from the dataset.
func TestOfflineBenchmarkDisenchantYields(t *testing.T) {
	ds := bdAdvice()
	var out []benchRow
	for _, name := range []string{"Embossed Leather Vest", "Copper Shortsword", "Murloc Scale Breastplate", "Toughened Leather Armor", "Green Silk Armor", "Dusky Leather Armor", "Copper Battle Axe"} {
		row := rowByName(t, ds, "items", name)
		want := deExpected(t, ds, row)
		line := deLine(ds, name)
		ok := len(want) > 0 && strings.Contains(line, "can be disenchanted")
		for _, w := range want {
			ok = ok && strings.Contains(line, w)
		}
		res := resultIf(ok, "a material or chance missing, or the band's answer not given")
		if strings.Contains(strings.Join(want, ";"), "item ") {
			if ok {
				res = "match"
			}
		}
		out = append(out, benchRow{fmt.Sprintf("%s (%s item level %d): what it disenchants into", name, map[int64]string{2: "weapon", 4: "armor"}[num(row["class_id"])], num(row["item_level"])), line, strings.Join(want, ", "), bdDESrc, res})
	}
	finish(t, "advice-disenchant-yields", "Offline disenchanting: what an item gives", out)
}

// TestOfflineBenchmarkDisenchantCannot: items that cannot be disenchanted: grey and white gear, trade goods and consumables, whatever their price.
func TestOfflineBenchmarkDisenchantCannot(t *testing.T) {
	ds := bdAdvice()
	var out []benchRow
	for _, tc := range []struct{ name, why string }{
		{"Thick Cloth Vest", "common quality armor (quality 1)"},
		{"Copper Bar", "a trade good (class 7)"},
		{"Minor Healing Potion", "a consumable (class 0)"},
		{"Linen Cloth", "a trade good (class 7)"},
	} {
		row := rowByName(t, ds, "items", tc.name)
		line := deLine(ds, tc.name)
		out = append(out, benchRow{fmt.Sprintf("%s: can it be disenchanted", tc.name), line, "no: " + tc.why + " (class " + fmt.Sprint(num(row["class_id"])) + ", quality " + fmt.Sprint(num(row["quality"])) + ")", bdDESrc,
			resultIf(strings.Contains(line, "cannot be disenchanted") && !strings.Contains(line, "giving"), "a disenchant result for an item the game refuses")})
	}
	finish(t, "advice-disenchant-cannot", "Offline disenchanting: items that cannot be disenchanted", out)
}

// bdReview is the bag review of a level 60 warrior holding a stack: a warrior wears no cloth or leather, so a cloth or leather piece is for sale.
func bdReview(t *testing.T, ds store.DataStore, count int, name string) (inventory.CrossResult, inventory.Line) {
	t.Helper()
	c := bindChar(t, "Wara", "WARRIOR", 60, name)
	c.Bags[0].Count = count
	r := inventory.ReviewAll([]character.Character{c}, ds)
	return r, bindLine(r.Lines, "Wara", name)
}

// TestOfflineBenchmarkSellVendorOrAuction: whether a stack goes to a vendor or the Auction House, and what the vendor pays for the stack (the dataset's sell_price
// times the count): a Bind on Pickup piece and a white or grey item go to a vendor, an uncommon or better tradeable item to the Auction House with no price stated.
func TestOfflineBenchmarkSellVendorOrAuction(t *testing.T) {
	var out []benchRow
	for _, tc := range []struct {
		label, item string
		count       int
		ah          bool
		why         string
	}{
		{"a stack of 5 white vests no one wants: vendor, price times the count", "Thick Cloth Vest", 5, false, "quality 1, bonding 0: a vendor"},
		{"a Bind on Pickup epic robe: vendor, never the Auction House", "Robe of the Archmage", 1, false, "bonding 1: cannot be auctioned"},
		{"a Bind on Pickup rare plate piece: vendor", "Sergeant Major's Silk Cuffs", 1, false, "bonding 1, rare: cannot be auctioned"},
		{"a Bind on Equip green leather piece: Auction House", "Ice-covered Bracers", 1, true, "bonding 2, uncommon: tradeable, worth more on the Auction House"},
		{"a Bind on Equip epic cloth glove: Auction House", "Gloves of Spell Mastery", 1, true, "bonding 2, epic: tradeable, worth more on the Auction House"},
		{"a stack of 20 grey cloth vests: vendor, price times the count", "Tattered Cloth Vest", 20, false, "quality 0, bonding 0: a vendor"},
	} {
		ds := store.DataStore(usabStore())
		if tc.item == "Tattered Cloth Vest" {
			ds = mbGrey(t, tc.item)
		}
		r, l := bdReview(t, ds, tc.count, tc.item)
		row := usabItem(t, tc.item)
		price := num(row["sell_price"])
		var got, ref string
		ok := l.Action == inventory.Sell && l.AH == tc.ah
		if tc.ah {
			got = fmt.Sprintf("%s ah=%v, vendor value counted %d", l.Action, l.AH, r.SellValue)
			ref = "Auction House: " + tc.why + "; the dataset holds no Auction House price, so none is stated"
			ok = ok && r.SellValue == 0 && r.SellAuction == 1
		} else {
			got = fmt.Sprintf("%s ah=%v, vendor value %s (%d copper)", l.Action, l.AH, vendors.Money(r.SellValue), r.SellValue)
			ref = fmt.Sprintf("vendor, %s (%d copper = %d x %d): %s", vendors.Money(price*int64(tc.count)), price*int64(tc.count), price, tc.count, tc.why)
			ok = ok && r.SellValue == price*int64(tc.count) && r.SellAuction == 0
		}
		out = append(out, benchRow{tc.label, got, ref, bdSellSrc, resultIf(ok, "the wrong place to sell it, or the wrong vendor value")})
	}
	finish(t, "advice-sell-vendor-or-auction", "Offline sell advice: vendor or Auction House, and the vendor price of a stack", out)
}

// TestOfflineBenchmarkPotionRules: the question about a potion or elixir's cooldown or stacking gets the line that the dataset holds none, and no stack-size line for
// buff stacking; what the dataset does hold of a potion (its required level and vendor price) is stated as it is.
func TestOfflineBenchmarkPotionRules(t *testing.T) {
	ds := bdAdvice()
	var out []benchRow
	row := rowByName(t, ds, "items", "Minor Healing Potion")
	var cols []string
	for c := range row {
		l := strings.ToLower(c)
		if strings.Contains(l, "cooldown") || strings.Contains(l, "duration") || strings.Contains(l, "categor") || strings.Contains(l, "effect") || strings.Contains(l, "spell") {
			cols = append(cols, c)
		}
	}
	for _, tc := range []struct {
		label, q string
		stack    bool
	}{
		{"how long is the cooldown on a healing potion", "how long is the cooldown on a [Minor Healing Potion]?", false},
		{"can a battle elixir and a guardian elixir be stacked", "can i stack a guardian elixir with a battle elixir?", false},
		{"does a flask replace the elixirs", "does a flask replace my elixirs?", false},
		{"can two potions be drunk in a row", "can i potion again right after a mana potion?", false},
		{"how many potions fit in one bag slot (a real stack size question)", "how many healing potions stack in one slot?", true},
	} {
		f := ask.LimitFacts(tc.q)
		hasPot, hasStack := strings.Contains(f, "Potions, elixirs and flasks:"), strings.Contains(f, "Stack size:")
		ok := hasPot == !tc.stack && hasStack == tc.stack && (tc.stack || strings.Contains(f, "no potion cooldown") && strings.Contains(f, "no battle or guardian category") && len(cols) == 0)
		got := "no limit line"
		if hasPot {
			got = "says the data holds no potion cooldown, duration, category or stacking rule"
		}
		if hasStack {
			got = "says the data holds no stack size"
		}
		ref := "no cooldown, duration, category or stacking rule in the data (columns held with those words: none)"
		if tc.stack {
			ref = "no stack size in the data; not a buff rule"
		}
		out = append(out, benchRow{tc.label, got, ref, bdPotSrc, resultIf(ok, "a rule was left unsaid, or a buff question was answered as a stack size")})
	}
	// what the dataset does hold: the required level and the vendor price of an elixir
	name := "Elixir of Minor Agility"
	er := rowByName(t, ds, "items", name)
	line := bdTip(ds, name)
	want := fmt.Sprintf("requires level %d", num(er["required_level"]))
	price := vendors.Money(num(er["sell_price"]))
	out = append(out, benchRow{name + ": required level and vendor price", bdTipHead(line), want + ", sells to a vendor for " + price, bdPotSrc,
		resultIf(strings.Contains(line, want) && strings.Contains(line, "sells to a vendor for "+price+" each"), "a level or price other than the dataset's")})
	finish(t, "advice-potion-rules", "Offline potions and elixirs: cooldown and stacking have no dataset answer", out)
}

// TestOfflineBenchmarkNoAnswerBindDisenchant: the dataset has no answer: an item it does not hold, a disenchant band it does not rate, an item with no sell
// price, an Auction House price. The advisor says so and states none.
func TestOfflineBenchmarkNoAnswerBindDisenchant(t *testing.T) {
	ds := bdAdvice()
	var out []benchRow
	for _, tc := range []struct{ label, item string }{
		{"Heavy Mithril Breastplate (uncommon, item level 46): what it disenchants into", "Heavy Mithril Breastplate"},
		{"Glowing Copper Boots (rare): what it disenchants into", "Glowing Copper Boots"},
		{"Hammer of the Northern Wind (epic): what it disenchants into", "Hammer of the Northern Wind"},
		{"Heavy Copper Broadsword (uncommon weapon, item level 19): what it disenchants into", "Heavy Copper Broadsword"},
	} {
		row := rowByName(t, ds, "items", tc.item)
		line := deLine(ds, tc.item)
		res := "differ: a material was named for an item no band covers"
		if strings.Contains(line, "no rated band") && !strings.Contains(line, "giving") {
			res = "differ (data): the dataset holds no disenchant band for this quality and item level, so the advisor names no material"
		}
		out = append(out, benchRow{tc.label, line, fmt.Sprintf("no band for quality %d, item level %d in disenchant_yields", num(row["quality"]), num(row["item_level"])), bdNoneSrc, res})
	}
	line := deLine(ds, "Mystery Trinket")
	out = append(out, benchRow{"an item the dataset does not hold: can it be disenchanted", line, "no items row", bdNoneSrc,
		resultIf(strings.Contains(line, "no row for this item") && !strings.Contains(line, "giving"), "a disenchant answer for an unknown item")})
	line = bdTip(ds, "Mystery Trinket")
	out = append(out, benchRow{"an item the dataset does not hold: how it binds", line, "no items row", bdNoneSrc,
		resultIf(strings.Contains(line, "no row for this item") && strings.Contains(line, "binding") && !strings.Contains(line, "Bind on"), "a binding was guessed")})
	f := ask.LimitFacts("what will [Silver Bar] sell for on the auction house?")
	out = append(out, benchRow{"the Auction House price of a Silver Bar", firstLine(f), "no Auction House price in the dataset", bdNoneSrc,
		resultIf(strings.Contains(f, "no Auction House prices"), "an Auction House price could be given")})
	pr := rowByName(t, usabStore(), "items", "Razor Arrow")
	line = bdTip(usabStore(), "Razor Arrow")
	out = append(out, benchRow{"Razor Arrow: what a vendor pays", bdTipHead(line), "sell_price is null in the dataset", bdNoneSrc,
		resultIf(pr["sell_price"] == nil && !strings.Contains(line, "sells to a vendor for"), "a vendor price was made up")})
	finish(t, "advice-bind-disenchant-no-answer", "Offline bind, disenchant and potion advice: no answer in the dataset", out)
}

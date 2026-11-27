package paths_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/ask"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/inventory"
	"github.com/WoW-Companion/advisor/internal/paths"
)

// The offline bag and bank slot, tooltip level, trade and mail rule, stack and unique-item benchmark: how many bag slots a review frees, what an item's
// tooltip says of its levels and its binding, what the binding lets a player do with the item (trade, mail, auction), what a stack of N is worth, and what
// the advisor says when the dataset holds no answer (bag and bank sizes, stack sizes, unique limits, mail, Auction House and trade window rules, macros),
// beside the dataset's rows and the public game rules, with no network. It reads testdata/usability_snapshot (items: item_level, required_level, bonding,
// quality, class_id, sell_price). The dataset holds no bag slot count, no stack size, no unique flag and no mail, auction, trade or macro rule, so the cases it
// cannot answer check that the advisor says so and states none. make offline-benchmark runs it; WOWC_BENCH_REPORT receives the tables.

const (
	mbSlotSrc  = "dataset items (quality, class_id, slot, sell_price); game: one stack fills one bag slot, whatever its count"
	mbTipSrc   = "dataset items (item_level, required_level, bonding); game item tooltip"
	mbTradeSrc = "dataset items (bonding 0 none, 1 Bind on Pickup, 2 Bind on Equip, 3 Bind on Use, 4 quest); game: soulbound and quest items cannot be traded, mailed or auctioned"
	mbStackSrc = "dataset items (sell_price) times the stack's count; the capture's own count"
	mbNoneSrc  = "the dataset has no bag size, stack size, unique flag, mail, Auction House, trade window or macro table or column"
)

func mbStack(t *testing.T, name string, count int) character.ItemStack {
	t.Helper()
	return character.ItemStack{ItemID: int(num(usabItem(t, name)["id"])), Count: count, Name: name}
}

func mbGrey(t *testing.T, names ...string) moneyGrey {
	t.Helper()
	ids := map[int64]bool{}
	for _, n := range names {
		ids[num(usabItem(t, n)["id"])] = true
	}
	return moneyGrey{DataStore: usabStore(), ids: ids}
}

// ---- bag and bank slots ----

var slotCases = []struct {
	label       string
	bags, bank  []string // item names; a stack each
	counts      []int    // the bag stacks' counts
	grey        []string // items made grey
	freed       int      // bag slots the game frees
	vendor, bnk int      // stacks the review sends to the vendor and to the bank
}{
	{"three grey stacks of 20, 1 and 5 in the bags: one slot a stack, not one a piece", []string{"Heavy Mithril Shoulder", "Dread Mage Hat", "Sergeant Major's Plate Wristguards"}, nil, []int{20, 1, 5},
		[]string{"Heavy Mithril Shoulder", "Dread Mage Hat", "Sergeant Major's Plate Wristguards"}, 3, 3, 0},
	{"a grey stack in the bank frees no bag slot", nil, []string{"Heavy Mithril Shoulder"}, nil, []string{"Heavy Mithril Shoulder"}, 0, 1, 0},
	{"the same grey item in two bag stacks: two slots", []string{"Heavy Mithril Shoulder", "Heavy Mithril Shoulder"}, nil, []int{200, 3}, []string{"Heavy Mithril Shoulder"}, 2, 2, 0},
	{"a grey stack in the bags and one in the bank: only the bag one frees a slot", []string{"Dread Mage Hat"}, []string{"Dread Mage Hat"}, []int{4}, []string{"Dread Mage Hat"}, 1, 2, 0},
	{"a bag in the bags frees no slot: it is a container, not gear", []string{"Linen Bag", "Mageweave Bag"}, nil, []int{1, 1}, nil, 0, 0, 0},
	{"a quest item no quest needs is not vendor trash and frees no slot", []string{"Ryedol's Lucky Pick"}, nil, []int{2}, []string{"Ryedol's Lucky Pick"}, 0, 0, 0},
	{"a white cloth stack no rule places frees no slot", []string{"Bolt of Woolen Cloth"}, nil, []int{40}, nil, 0, 0, 0},
}

// TestOfflineBenchmarkBagSlotsFreed: the bag slots the review frees, beside the game's one slot per stack: grey stacks and gear sent to the bank in the bags free
// one slot each, whatever their count; stacks already in the bank, bags, quest items and unplaced stacks free none.
func TestOfflineBenchmarkBagSlotsFreed(t *testing.T) {
	var out []benchRow
	for _, tc := range slotCases {
		c := bindChar(t, "Magea", "MAGE", 40)
		for i, n := range tc.bags {
			c.Bags = append(c.Bags, mbStack(t, n, tc.counts[i]))
		}
		for _, n := range tc.bank {
			c.Bank = append(c.Bank, mbStack(t, n, 1))
		}
		r := inventory.Review(c, mbGrey(t, tc.grey...))
		got := fmt.Sprintf("%d slots freed; %d to the vendor", r.SlotsFreed, len(r.Groups[inventory.Vendor]))
		want := fmt.Sprintf("%d slots freed; %d to the vendor", tc.freed, tc.vendor)
		out = append(out, benchRow{tc.label, got, want, mbSlotSrc, resultIf(got == want, "the slots freed")})
	}
	// gear that is not an upgrade goes to the bank and frees the bag slot it filled
	g := bindChar(t, "Magea", "MAGE", 40)
	g.Bags = []character.ItemStack{mbStack(t, "Sergeant Major's Plate Wristguards", 1)}
	r := inventory.Review(g, usabStore())
	got := fmt.Sprintf("%d slots freed; %d to the bank", r.SlotsFreed, len(r.Groups[inventory.Bank]))
	out = append(out, benchRow{"plate wristguards a mage cannot wear, in the bags: banked", got, "1 slots freed; 1 to the bank", mbSlotSrc, resultIf(got == "1 slots freed; 1 to the bank", "the slots freed")})
	// the review says when the bank was never opened: it does not count it empty
	u := bindChar(t, "Magea", "MAGE", 40)
	u.BankCaptured = false
	notes := strings.Join(inventory.Review(u, usabStore()).Notes, " ")
	out = append(out, benchRow{"a bank the capture never saw", notes, "named as not captured, not counted as empty", mbSlotSrc, resultIf(strings.Contains(notes, "Bank not captured"), "a bank was assumed")})
	finish(t, "advice-bag-slots-freed", "Offline bag and bank slots: slots freed by one stack each", out)
}

// ---- item level and required level on the tooltip ----

var tipCases = []struct{ item, binding string }{
	{"Robe of Power", "Bind on Pickup"},
	{"Heavy Mithril Shoulder", "Bind on Equip"},
	{"Thick Cloth Vest", ""},
	{"Deprecated Cat's Paw", "Bind on Use"},
	{"Heavy Quiver", ""},
	{"Mageweave Bag", ""},
	{"Recruit's Shirt", ""},
	{"Thunderbrew's Boot Flask", "Bind on Pickup"},
}

// TestOfflineBenchmarkItemTooltip: the item level, required level and binding the answer's facts give for a linked item, beside the dataset's
// item_level, required_level and bonding; no required level in the data is said, not estimated.
func TestOfflineBenchmarkItemTooltip(t *testing.T) {
	ds := usabStore()
	var out []benchRow
	for _, tc := range tipCases {
		r := usabItem(t, tc.item)
		f := ask.ItemTooltipFacts(ds, "what level does ["+tc.item+"] need and can I trade it?")
		line := ""
		for _, l := range strings.Split(f, "\n") {
			if strings.HasPrefix(l, "- "+tc.item+":") {
				line = l
			}
		}
		il, rl := num(r["item_level"]), num(r["required_level"])
		wantReq := fmt.Sprintf("requires level %d", rl)
		if rl == 0 {
			wantReq = "no required level listed in the data"
		}
		wantBind := tc.binding == "" || strings.Contains(line, tc.binding)
		ok := strings.Contains(line, fmt.Sprintf("item level %d;", il)) && strings.Contains(line, wantReq) && wantBind
		out = append(out, benchRow{tc.item + ": the tooltip levels and binding", line, fmt.Sprintf("item level %d; %s; %s", il, wantReq, noBinding(tc.binding)), mbTipSrc, resultIf(ok, "the levels or the binding")})
	}
	// worn gear of item level 30 or more shows a required level in the game; the dataset lists none for these pieces, so the tooltip says none is listed
	for _, name := range []string{"Dread Mage Hat", "Ring of Pure Silver"} {
		r := usabItem(t, name)
		f := ask.ItemTooltipFacts(ds, "what level does ["+name+"] need?")
		res := "match"
		if !strings.Contains(f, "no required level listed in the data") {
			res = "differ: a level was stated the data does not hold"
		} else if num(r["required_level"]) == 0 && num(r["slot"]) != 0 && num(r["item_level"]) >= 30 {
			res = "differ (data): the dataset lists required_level 0 for worn gear of item level " + fmt.Sprint(num(r["item_level"])) + ", where the game shows a level"
		}
		out = append(out, benchRow{name + ": the required level of worn gear", firstLine(strings.TrimPrefix(f, "Item tooltips (from the item rows; answer from these, not from memory):\n")), "a required level on the tooltip (game); dataset required_level 0", mbTipSrc, res})
	}
	finish(t, "advice-item-tooltip", "Offline item tooltip: item level, required level and binding", out)
}

// ---- trade, mail and the Auction House by binding ----

var tradeCases = []struct {
	item  string
	trade bool   // the game lets the item pass between players
	rule  string // the word the facts give for the rule
}{
	{"Robe of Power", false, "soulbound when picked up"},
	{"Heavy Mithril Shoulder", true, "binds when equipped"},
	{"Deprecated Cat's Paw", true, "binds when used"},
	{"Copper Bar", true, "binds to no one"},
	{"Torch of Retribution", false, "bound to the one who looted it"},
	{"Ryedol's Lucky Pick", false, "bound to the one who looted it"},
}

// TestOfflineBenchmarkTradeMailRule: what the facts say a binding lets a player do with the item, beside the game's rule: Bind on Pickup and quest items
// cannot be traded, mailed or auctioned; Bind on Equip and Bind on Use can until worn or used; an unbound item always can.
func TestOfflineBenchmarkTradeMailRule(t *testing.T) {
	ds := usabStore()
	var out []benchRow
	for _, tc := range tradeCases {
		r := usabItem(t, tc.item)
		f := ask.ItemTooltipFacts(ds, "can I mail or trade ["+tc.item+"] to my alt?")
		can := strings.Contains(f, "can be traded, mailed") || strings.Contains(f, "it can be traded, mailed")
		cannot := strings.Contains(f, "cannot be traded, mailed")
		ok := strings.Contains(f, tc.rule) && can == tc.trade && cannot == !tc.trade && paths.Tradeable(int(num(r["bonding"])), int(num(r["quality"]))) == tc.trade
		out = append(out, benchRow{fmt.Sprintf("%s (bonding %d): traded or mailed", tc.item, num(r["bonding"])), strings.ReplaceAll(strings.TrimPrefix(f, "Item tooltips (from the item rows; answer from these, not from memory):\n"), "\n", " "), fmt.Sprintf("tradeable %v (%s)", tc.trade, tc.rule), mbTradeSrc, resultIf(ok, "the trade or mail rule")})
	}
	// a quest item an alt's log needs is not mailed to the alt: the holder cannot send it
	a := bindChar(t, "Wara", "WARRIOR", 40, "Torch of Retribution")
	b := bindChar(t, "Magea", "MAGE", 40)
	b.Quests = []character.Quest{{QuestID: 1, Title: "Torch", Objectives: []character.Objective{{Text: "Torch of Retribution: 0/1", Type: "item"}}}}
	l := bindLine(inventory.ReviewAll([]character.Character{a, b}, usabStore()).Lines, "Wara", "Torch of Retribution")
	out = append(out, benchRow{"a quest item the alt's quest needs: mailed to the alt", fmt.Sprintf("%s %q: %s", l.Action, l.To, l.Reason), "not mailed: a quest item is bound to its looter and cannot be mailed", mbTradeSrc, resultIf(l.Action != inventory.Mail && l.To == "", "a quest item was mailed")})
	// a grey Bind on Equip piece is as tradeable as a white one
	gds := mbGrey(t, "Heavy Mithril Shoulder")
	txt := paths.For(horde(60), gds, num(usabItem(t, "Heavy Mithril Shoulder")["id"]), paths.Options{}).Text()
	out = append(out, benchRow{"a grey Bind on Equip piece: said to be bound when picked up", firstMatch(txt, "Auction House"), "grey items can be traded and auctioned: never called bound when picked up", mbTradeSrc, resultIf(!strings.Contains(txt, "bound when picked up"), "a tradeable grey item was called bound")})
	finish(t, "advice-trade-mail-rule", "Offline trade and mail: what a binding allows", out)
}

func noBinding(s string) string {
	if s == "" {
		return "no binding"
	}
	return s
}

// ---- stacks and unique items ----

// TestOfflineBenchmarkStackValue: what a stack of grey items sells for, beside the stack's count times the dataset's sell_price, whatever the count.
func TestOfflineBenchmarkStackValue(t *testing.T) {
	var out []benchRow
	for _, tc := range []struct {
		item  string
		count int
	}{{"Heavy Mithril Shoulder", 1}, {"Heavy Mithril Shoulder", 20}, {"Dread Mage Hat", 200}, {"Sergeant Major's Plate Wristguards", 7}} {
		c := bindChar(t, "Magea", "MAGE", 40)
		c.Bags = append(c.Bags, mbStack(t, tc.item, tc.count))
		r := inventory.Review(c, mbGrey(t, tc.item))
		want := num(usabItem(t, tc.item)["sell_price"]) * int64(tc.count)
		out = append(out, benchRow{fmt.Sprintf("a stack of %d grey %s: its vendor value", tc.count, tc.item), fmt.Sprintf("%d copper in %d slot(s) freed", r.VendorValue, r.SlotsFreed), fmt.Sprintf("%d copper (%d x %d) in 1 slot freed", want, tc.count, num(usabItem(t, tc.item)["sell_price"])), mbStackSrc,
			resultIf(r.VendorValue == want && r.SlotsFreed == 1, "the value or the slots")})
	}
	finish(t, "advice-stack-value", "Offline stacks: value by count, one slot each", out)
}

// TestOfflineBenchmarkStackUniqueNoAnswer: the dataset holds no stack size and no unique flag: the tooltip facts say so for any item, a question about either says the
// data holds none, and no item is called unique.
func TestOfflineBenchmarkStackUniqueNoAnswer(t *testing.T) {
	ds := usabStore()
	var out []benchRow
	for _, name := range []string{"Copper Bar", "Ring of Pure Silver", "Thunderbrew's Boot Flask"} {
		f := ask.ItemTooltipFacts(ds, "what is the stack size of ["+name+"], is it unique?")
		out = append(out, benchRow{name + ": stack size and unique limit in the facts", f, "not in the game data: said so, no number named", mbNoneSrc,
			resultIf(strings.Contains(f, "stack size and any unique limit: not in the game data") && !uniqueWord.MatchString(strings.ReplaceAll(f, "any unique limit", "")), "a stack size or unique limit was stated")})
	}
	for _, q := range []string{"how many Linen Cloth fit in one stack?", "what is the max stack of Copper Bar?", "can I carry two of the Ring of Pure Silver, is it unique-equipped?", "is Thunderbrew's Boot Flask unique?"} {
		f := ask.LimitFacts(q)
		out = append(out, benchRow{"\"" + q + "\": the facts", firstLine(f), "the data holds no stack size and no unique flag: say so, name none", mbNoneSrc,
			resultIf(strings.Contains(f, "Stack size: the game data holds no") || strings.Contains(f, "Unique items: the game data holds no"), "no statement that the data holds none")})
	}
	finish(t, "advice-stack-unique-no-answer", "Offline stack size and unique items: no answer in the dataset", out)
}

// TestOfflineBenchmarkMailBankNoAnswer: bag and bank sizes, mail, Auction House and trade window rules, and macros are not in the dataset: the facts say so for a question
// about each, a question about none carries no such facts, and an item the dataset does not hold is not described.
func TestOfflineBenchmarkMailBankNoAnswer(t *testing.T) {
	ds := usabStore()
	var out []benchRow
	for _, tc := range []struct{ q, word string }{
		{"how many slots does a Mageweave Bag have?", "Bag and bank size"},
		{"how much does the next bank slot cost?", "Bag and bank size"},
		{"how many items can I attach to one mail and when does it expire?", "Mail:"},
		{"what is the Auction House deposit for a 24 hour listing?", "Auction House:"},
		{"what is the Auction House cut when an item sells?", "Auction House:"},
		{"how many items can I put in the trade window?", "Trade window:"},
		{"write me a macro to cast Frostbolt and use my trinket", "Macros:"},
	} {
		f := ask.LimitFacts(tc.q)
		out = append(out, benchRow{"\"" + tc.q + "\": the facts", firstLine(f), "the data holds none: say so, name no figure", mbNoneSrc, resultIf(strings.Contains(f, tc.word) && strings.Contains(f, "Say so"), "no statement that the data holds none")})
	}
	for _, q := range []string{"what should I do with my bags?", "what quest should I do next?", "how do I get to Ironforge?"} {
		f := ask.LimitFacts(q)
		out = append(out, benchRow{"\"" + q + "\": a question about none of them", f, "no limit facts", mbNoneSrc, resultIf(f == "", "limit facts on another question")})
	}
	g := ask.ItemTooltipFacts(ds, "can I trade [Blade of Imaginary Dawn]?")
	out = append(out, benchRow{"a linked item the dataset does not hold: the facts", g, "says the dataset has no row, states no level or binding", mbNoneSrc, resultIf(strings.Contains(g, "has no row for this item") && !strings.Contains(g, "item level"), "a tooltip was guessed")})
	n := ask.ItemTooltipFacts(ds, "can I trade my stuff?")
	out = append(out, benchRow{"a trade question that links no item: the facts", n, "no per-item facts", mbNoneSrc, resultIf(n == "", "item facts without an item")})
	finish(t, "advice-mail-bank-no-answer", "Offline bag, bank, mail, auction, trade window and macro: no answer in the dataset", out)
}

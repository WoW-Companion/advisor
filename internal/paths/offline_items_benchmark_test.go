package paths_test

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/WoW-Companion/advisor/internal/brief"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/gear"
	"github.com/WoW-Companion/advisor/internal/inventory"
	"github.com/WoW-Companion/advisor/internal/levelup"
	"github.com/WoW-Companion/advisor/internal/names"
	"github.com/WoW-Companion/advisor/internal/professions"
	"github.com/WoW-Companion/advisor/internal/store"
	"github.com/WoW-Companion/advisor/internal/vendors"
)

// The offline item-level, set-piece, currency and no-answer benchmark: what the advisor says about who can wear a class-bound
// set piece and when, how much better an item of a higher item level is, what a set is called, and what an item sells for
// or costs, beside the dataset's rows, with no network. testdata/items_snapshot (rebuild it with
// testdata/items_snapshot/build.sh) holds the item_sets rows of a spread of sets and the items they name; the gear rows of
// testdata/usability_snapshot give the sell prices. The dataset holds no set-bonus text, no currency, no achievement and no
// key binding table, so the cases it cannot answer check that the advisor states none of them. A difference the dataset's
// own rows cause is marked "(data)". make offline-benchmark runs it; WOWC_BENCH_REPORT receives the tables.

const (
	itemsDir      = "../../testdata/items_snapshot"
	itemsSetSrc   = "dataset item_sets (set_id, name, item_id), items (class_id, subclass_id, slot, allowable_class_mask); game armor proficiency"
	itemsLevelSrc = "dataset items (item_level, quality, stats_status)"
	itemsNameSrc  = "dataset item_sets (set_id, name)"
	itemsMoneySrc = "dataset items (sell_price, buy_price, copper); game money units (100 copper to the silver, 100 silver to the gold); game gold cap (2^31-1 copper)"
	itemsNoneSrc  = "the dataset has no such row, column or table"
)

var (
	itemsOnce  sync.Once
	itemsCache store.DataStore
)

// itemsStore reads the snapshot once for the package: the tables are decoded on first use and the tests share them.
func itemsStore() store.DataStore {
	itemsOnce.Do(func() { itemsCache = store.NewDir(itemsDir) })
	return itemsCache
}

func itemsRow(t *testing.T, ds store.DataStore, id int64) store.Row {
	t.Helper()
	r, err := ds.Get("items", id)
	if err != nil || r == nil {
		t.Fatalf("item %d: not in the snapshot (%v)", id, err)
	}
	return r
}

// ---- armor type and class eligibility of set pieces ----

// setPieces are pieces of class-bound and open sets: plate, mail, leather and cloth, one set to a class.
var setPieces = []int64{272479, 272625, 272725, 272643, 16506, 272490, 272730, 272852, 7950, 15053, 12422, 16505}

// TestOfflineBenchmarkSetPieceEligibility: which classes the advisor says can wear a set piece at level 60, beside the
// piece's class mask and the armor type each class wears.
func TestOfflineBenchmarkSetPieceEligibility(t *testing.T) {
	ds := itemsStore()
	var out []benchRow
	for _, id := range setPieces {
		r := itemsRow(t, ds, id)
		got, want := whoCan(r, 60, func(class string, cid int64) bool { return refUsable(t, r, class, cid, 60) })
		out = append(out, benchRow{fmt.Sprintf("%s: who can wear it at level 60", text(r["name"])), got, want + fmt.Sprintf(" (subclass %d, slot %d, class mask %d)", num(r["subclass_id"]), num(r["slot"]), num(r["allowable_class_mask"])),
			itemsSetSrc, resultIf(got == want, "the classes that can wear it")})
	}
	finish(t, "advice-set-piece-eligibility", "Offline items: who can wear a set piece", out)
}

// TestOfflineBenchmarkSetPieceArmorChange: a plate or mail set piece around level 40, when the warrior, paladin, hunter and
// shaman gain their heavier armor, and a class-bound piece the class may wear in no armor type below it.
func TestOfflineBenchmarkSetPieceArmorChange(t *testing.T) {
	ds := itemsStore()
	var out []benchRow
	for _, tc := range []struct {
		id    int64
		level int
	}{{12422, 39}, {12422, 40}, {272490, 39}, {272490, 40}, {15053, 39}, {15053, 40}, {272852, 39}, {272852, 40}} {
		r := itemsRow(t, ds, tc.id)
		got, want := whoCan(r, tc.level, func(class string, cid int64) bool { return refUsable(t, r, class, cid, tc.level) })
		out = append(out, benchRow{fmt.Sprintf("%s: who can wear it at level %d", text(r["name"]), tc.level), got, want + fmt.Sprintf(" (subclass %d, class mask %d)", num(r["subclass_id"]), num(r["allowable_class_mask"])),
			itemsSetSrc, resultIf(got == want, "the classes that can wear it at this level")})
	}
	finish(t, "advice-set-piece-armor-change", "Offline items: set pieces around the armor change at level 40", out)
}

// ---- item level ----

// itemLevelCase is a warrior at level 30 (mail) with one worn piece and one in the bags.
type itemLevelCase struct {
	slot       string
	worn, bag  int64
	capturedIL int // the item level the capture says the worn piece has, 0 for none
}

var itemLevelCases = []itemLevelCase{
	{"legs", 848, 286, 0}, {"feet", 849, 287, 0}, {"legs", 286, 848, 0}, {"feet", 287, 849, 0}, {"feet", 849, 849, 0}, {"feet", 849, 287, 25}, {"feet", 0, 287, 0},
}

// TestOfflineBenchmarkItemLevelUpgrade: an item in the bags is called an upgrade only above the worn item's level, and the
// advice names the dataset's item levels of both; the capture's own item level of a worn piece comes before the dataset's.
func TestOfflineBenchmarkItemLevelUpgrade(t *testing.T) {
	ds := itemsStore()
	var out []benchRow
	for _, tc := range itemLevelCases {
		bag, wornIL := itemsRow(t, ds, tc.bag), int64(0)
		lvl := 30
		c := character.Character{Name: "Benchtest", Faction: "Horde", Class: "WARRIOR", Level: &lvl, BagsCaptured: true, Equipment: map[string]character.EquippedItem{}}
		c.Bags = []character.ItemStack{{ItemID: int(tc.bag), Count: 1, Name: text(bag["name"])}}
		label := "empty " + tc.slot
		if tc.worn != 0 {
			w := itemsRow(t, ds, tc.worn)
			id := int(tc.worn)
			wornIL = num(w["item_level"])
			eq := character.EquippedItem{Slot: tc.slot, ItemID: &id, Name: text(w["name"])}
			if tc.capturedIL > 0 {
				il := tc.capturedIL
				eq.ItemLevel = &il
				wornIL = int64(tc.capturedIL)
			}
			c.Equipment[tc.slot] = eq
			label = fmt.Sprintf("%s worn, %d captured", text(w["name"]), tc.capturedIL)
		}
		ups := gear.BagUpgrades(c, ds, "")
		got := "no upgrade"
		var up gear.BagUpgrade
		if len(ups) == 1 {
			up = ups[0]
			got = up.Line()
		}
		bagIL, same := num(bag["item_level"]), tc.worn == tc.bag
		wantUp := tc.worn == 0 || !same && bagIL > wornIL
		want := "no upgrade: item level " + fmt.Sprint(bagIL) + " is not above " + fmt.Sprint(wornIL)
		ok := len(ups) == 0 && !wantUp
		if wantUp {
			want = fmt.Sprintf("an upgrade: item level %d against the worn %d", bagIL, wornIL)
			ok = len(ups) == 1 && up.Item.ItemLevel == int(bagIL) && up.ReplacesILvl == int(wornIL) && (tc.worn == 0 && up.Empty || strings.Contains(got, fmt.Sprintf("item level %d vs %d", bagIL, wornIL)))
		}
		out = append(out, benchRow{fmt.Sprintf("%s: bag %s, %s", label, text(bag["name"]), tc.slot), got, want, itemsLevelSrc, resultIf(ok, "the item level comparison")})
	}
	finish(t, "advice-item-level-upgrade", "Offline items: the item level an upgrade rests on", out)
}

// TestOfflineBenchmarkItemLevelSearch: the gear search for a slot lists only items above the worn item's level, each with a
// gain equal to the difference of the dataset's item levels (quality breaks a tie below one level), and never an item that
// needs more than two levels above the character.
func TestOfflineBenchmarkItemLevelSearch(t *testing.T) {
	ds := itemsStore()
	lvl := 30
	var out []benchRow
	for _, tc := range []struct {
		slot string
		worn int64
	}{{"feet", 849}, {"legs", 848}, {"feet", 287}} {
		w := itemsRow(t, ds, tc.worn)
		id := int(tc.worn)
		c := character.Character{Name: "Benchtest", Faction: "Horde", Class: "WARRIOR", Level: &lvl, Equipment: map[string]character.EquippedItem{tc.slot: {Slot: tc.slot, ItemID: &id, Name: text(w["name"])}}}
		res, err := gear.Find(c, ds, "", gear.Options{Slot: tc.slot, NoSources: true})
		if err != nil {
			t.Fatal(err)
		}
		var bad []string
		var listed []string
		for _, cd := range res.Slots[tc.slot] {
			row := itemsRow(t, ds, cd.Item.ID)
			gain := float64(num(row["item_level"]) - num(w["item_level"]))
			listed = append(listed, fmt.Sprintf("%s +%.1f", cd.Item.Name, cd.Delta))
			if cd.Delta < gain-0.5 || cd.Delta > gain+0.5 || cd.Delta <= 0 || cd.Item.ItemLevel != int(num(row["item_level"])) || cd.Item.ReqLevel > lvl+gear.EquipWindow {
				bad = append(bad, cd.Item.Name)
			}
		}
		out = append(out, benchRow{fmt.Sprintf("%s worn (item level %d): %s search", text(w["name"]), num(w["item_level"]), tc.slot), strings.Join(listed, ", "), fmt.Sprintf("every item above item level %d, gain = the item level difference", num(w["item_level"])), itemsLevelSrc,
			resultIf(len(bad) == 0, "a candidate with a wrong gain, level or window: "+strings.Join(bad, ", "))})
	}
	finish(t, "advice-item-level-search", "Offline items: the gear search by item level", out)
}

// ---- set names ----

var setNameCases = []struct {
	query string
	want  []int64 // set ids the dataset gives that name
}{
	{"Champion's Battlegear", []int64{281}}, {"champions battlegear", []int64{281}}, {"Imperial Plate", []int64{321}}, {"Garb of Thero-shan", []int64{221}},
	{"Battlegear of Might", []int64{209}}, {"Dreadmist Raiment", []int64{183}}, {"The Elements", []int64{187, 1678}}, {"Lieutenant Commander's Pursuit", []int64{362, 1766}}, {"Deathbone Guardian", []int64{124, 1786}},
}

// TestOfflineBenchmarkSetNames: the set a name resolves to, beside the dataset's item_sets rows: an apostrophe or capital
// aside, the name finds its set id, and a name two set ids share finds both.
func TestOfflineBenchmarkSetNames(t *testing.T) {
	ds := itemsStore()
	var out []benchRow
	for _, tc := range setNameCases {
		hits, _, err := names.Find(ds, names.ItemSets, tc.query, 10)
		if err != nil {
			t.Fatal(err)
		}
		var got []int64
		for _, h := range hits {
			got = append(got, h.ID)
		}
		slices.Sort(got)
		out = append(out, benchRow{fmt.Sprintf("%q: set ids", tc.query), fmt.Sprint(got), fmt.Sprint(tc.want), itemsNameSrc, resultIf(slices.Equal(got, tc.want), "the set ids of the name")})
	}
	finish(t, "advice-set-names", "Offline items: what a set name resolves to", out)
}

// ---- money: sell prices, costs and the gold cap ----

var moneyUnits = regexp.MustCompile(`(\d+)([gsc])`)

// copperOf is the copper in a "12g 34s 56c" wording; the units may appear in any subset.
func copperOf(s string) int64 {
	var c int64
	for _, m := range moneyUnits.FindAllStringSubmatch(s, -1) {
		n, _ := strconv.ParseInt(m[1], 10, 64)
		c += n * map[string]int64{"g": 10000, "s": 100, "c": 1}[m[2]]
	}
	return c
}

// sellPriceCases are items a mage cannot wear, so the bag review says to sell them, with the copper the dataset pays.
var sellPriceCases = []string{"Black Night Elf Helm", "Brass-studded Bracers", "Chainmail Belt", "Chainmail Gloves", "Scalemail Boots", "Scalemail Pants", "Scalemail Vest", "Platemail Belt"}

// TestOfflineBenchmarkSellPrice: what the bag review says an item sells for, beside the item's sell_price, to the copper.
func TestOfflineBenchmarkSellPrice(t *testing.T) {
	ds := usabStore()
	c := usabChar("MAGE", 30, "Alliance")
	c.Name, c.BagsCaptured, c.BankCaptured = "Benchtest", true, true
	rows := map[string]store.Row{}
	for _, name := range sellPriceCases {
		r := usabItem(t, name)
		rows[name] = r
		c.Bags = append(c.Bags, character.ItemStack{ItemID: int(num(r["id"])), Count: 3, Name: name})
	}
	review := strings.Split(inventory.RenderAll(inventory.ReviewAll([]character.Character{c}, ds), 1), "\n")
	var out []benchRow
	for _, name := range sellPriceCases {
		got := "no line"
		for _, l := range review {
			if i := strings.Index(l, "sells for "); i >= 0 && strings.Contains(l, name+" ") {
				got = strings.TrimSpace(l[i+len("sells for "):])
			}
		}
		want := 3 * num(rows[name]["sell_price"])
		out = append(out, benchRow{name + " x3: sells for", got, fmt.Sprintf("%d copper (sell_price %d each)", want, num(rows[name]["sell_price"])), itemsMoneySrc, resultIf(copperOf(got) == want && want > 0, "the amount in copper")})
	}
	finish(t, "advice-sell-price", "Offline items: sell prices", out)
}

// TestOfflineBenchmarkMoneyWording: the same amount of copper in each place the advisor writes money (a vendor price, the
// bag value, the character's purse, a profession plan's cost and a shortage for training) must be the same amount to the
// copper, and the purse at the game's gold cap must not overflow.
func TestOfflineBenchmarkMoneyWording(t *testing.T) {
	var out []benchRow
	for _, c := range []int64{7, 100, 1050, 12345, 27547, 123456, 10000000} {
		got := map[string]string{"vendor": vendors.Money(c), "bags": inventory.Money(c), "plan": professions.Money(c)}
		var bad []string
		for k, v := range got {
			if copperOf(v) != c {
				bad = append(bad, fmt.Sprintf("%s says %q", k, v))
			}
		}
		words := []string{got["vendor"], got["bags"], got["plan"]}
		out = append(out, benchRow{fmt.Sprintf("%d copper in words", c), "vendor " + words[0] + ", bags " + words[1] + ", plan " + words[2], fmt.Sprintf("%d copper in every place", c), itemsMoneySrc, resultIf(len(bad) == 0, strings.Join(bad, "; "))})
	}
	for _, c := range []int{12345, 2147483647} {
		purse := c
		got := brief.FormatMoney(&purse)
		out = append(out, benchRow{fmt.Sprintf("purse of %d copper", c), got, fmt.Sprintf("%d copper (214748g 36s 47c is the game's cap)", c), itemsMoneySrc, resultIf(copperOf(got) == int64(c), "the purse in copper")})
	}
	for _, tc := range []struct {
		have  int
		total int64
	}{{50, 1050}, {123456, 200000}, {0, 7}} {
		have := tc.have
		ch := character.Character{Money: &have}
		line := levelup.MoneyLine(ch, tc.total)
		short := tc.total - int64(have)
		got := ""
		if i := strings.Index(line, "you are about "); i >= 0 {
			got = strings.SplitN(line[i+len("you are about "):], " short", 2)[0]
		}
		out = append(out, benchRow{fmt.Sprintf("training %d copper, purse %d: shortage", tc.total, have), got, fmt.Sprintf("%d copper short", short), itemsMoneySrc, resultIf(copperOf(got) == short, "the shortage in copper")})
	}
	finish(t, "advice-money-wording", "Offline items: the same amount of money across the advisor", out)
}

// ---- no answer in the dataset ----

var claimWords = regexp.MustCompile(`(?i)set bonus|\(\d\) set|honor points|arena points|badge|currency cap|weekly cap|achievement|key ?bind|hotkey`)

// TestOfflineBenchmarkItemsNoAnswer: what the dataset does not hold, the advisor does not state: a set's bonus, an item's
// currency cost, a currency's cap, an achievement's requirement and a key binding. The set piece's gear lines and the
// price lines carry no such claim, the tables are absent, and a sell price the dataset lacks is "unknown", not zero.
func TestOfflineBenchmarkItemsNoAnswer(t *testing.T) {
	ds := itemsStore()
	var out []benchRow
	for _, tb := range []string{"set_bonuses", "currencies", "achievements", "achievement_criteria", "key_bindings", "item_extended_costs"} {
		_, err := ds.Get(tb, 1)
		out = append(out, benchRow{tb + ": table", fmt.Sprint(err != nil), "the dataset has no such table", itemsNoneSrc, "differ (data): the dataset holds no " + strings.ReplaceAll(tb, "_", " ")})
	}
	// a set piece at the character's level: the gear search and the bag review describe it without a bonus
	lvl := 60
	c := character.Character{Name: "Benchtest", Faction: "Horde", Class: "WARRIOR", Level: &lvl}
	res, _ := gear.Find(c, ds, "", gear.Options{NoSources: true})
	var lines []string
	for _, cs := range res.Slots {
		for _, cd := range cs {
			lines = append(lines, cd.Describe())
		}
	}
	txt := strings.Join(lines, "\n")
	out = append(out, benchRow{"warrior 60 gear search over set pieces: lines", fmt.Sprintf("%d lines", len(lines)), "no set bonus, currency or achievement claim in any line", itemsNoneSrc, resultIf(len(lines) > 0 && !claimWords.MatchString(txt), "a claim about a set bonus, currency or achievement")})
	r := itemsRow(t, ds, 272479)
	bc := character.Character{Name: "Benchtest", Faction: "Horde", Class: "WARRIOR", Level: &lvl, BagsCaptured: true, BankCaptured: true, Bags: []character.ItemStack{{ItemID: 272479, Count: 1, Name: text(r["name"])}}}
	review := inventory.Render(inventory.Review(bc, ds)) + inventory.RenderAll(inventory.ReviewAll([]character.Character{bc}, ds), 1)
	out = append(out, benchRow{"Premier Mortarplate Armor in the bags: review", firstLine(review), "no set bonus or currency claim; no sell price in the dataset", itemsNoneSrc, resultIf(!claimWords.MatchString(review), "a claim about a set bonus or currency")})
	// the price of an item whose sell price the dataset lacks is unknown, not nothing
	mage := character.Character{Name: "Benchtest", Faction: "Horde", Class: "MAGE", Level: &lvl, BagsCaptured: true, BankCaptured: true, Bags: []character.ItemStack{{ItemID: 272479, Count: 1, Name: text(r["name"])}}}
	rep := inventory.RenderAll(inventory.ReviewAll([]character.Character{mage}, ds), 1)
	out = append(out, benchRow{"Premier Mortarplate Armor, sell price null: a mage's review", firstLine(rep), "sell price unknown (items.sell_price is null)", itemsMoneySrc, resultIf(!strings.Contains(rep, "sells for") && !strings.Contains(rep, "0g 0s 0c"), "a price was stated")})
	// items the gold price of which is not what they cost: a listed price of 0 is "no gold price"
	world := worldStore()
	for _, id := range []int64{249479, 285291} {
		it, _ := world.Get("items", id)
		o, ok := vendors.Best(world, id)
		out = append(out, benchRow{text(it["name"]) + ": what it costs", fmt.Sprintf("price %q, line %q", o.Price, o.Line()), "a cost in tokens or items: no currency name or cap in the dataset", itemsNoneSrc, resultIf(ok && o.Price == "" && !claimWords.MatchString(o.Line()), "a gold price or a currency was stated")})
	}
	// a set name the table lacks finds nothing
	hits, _, _ := names.Find(ds, names.ItemSets, "Pale Oak Raiment", 5)
	out = append(out, benchRow{"Pale Oak Raiment: set", fmt.Sprint(len(hits)), "no item_sets row", itemsNoneSrc, resultIf(len(hits) == 0, "a set was invented")})
	// the raid sets the items table lacks the pieces of: the set is named, its pieces are not worn gear the advisor can judge
	var absent int
	for _, set := range []int64{183, 209, 206} {
		rows, _ := ds.Filter("item_sets", store.Range{Column: "set_id", Min: set, Max: set})
		for _, row := range rows {
			if _, err := ds.Get("items", num(row["item_id"])); err != nil {
				absent++
			}
		}
	}
	out = append(out, benchRow{"Dreadmist, Battlegear of Might, Giantstalker: pieces missing from items", fmt.Sprint(absent, " of 24"), "every member of an item set is an items row", itemsNameSrc, "differ (data): the dataset's item_sets name items its items table lacks, so these sets cannot be judged or priced"})
	finish(t, "advice-items-no-answer", "Offline items: no answer in the dataset", out)
}

// TestOfflineBenchmarkAchievementNoAnswer: the dataset holds no achievement, no criterion and no key binding, and the capture
// holds none of the player's: the advisor names no achievement as a requirement and states no binding, whether it is asked
// for an achievement by name or shows the character.
func TestOfflineBenchmarkAchievementNoAnswer(t *testing.T) {
	ds := itemsStore()
	var out []benchRow
	for _, tb := range []string{"achievements", "achievement_criteria"} {
		_, err := ds.Get(tb, 1)
		out = append(out, benchRow{tb + ": table", fmt.Sprint(err != nil), "the dataset has no such table", itemsNoneSrc, "differ (data): the dataset holds no achievement requirement"})
	}
	for _, name := range []string{"Explore Elwynn Forest", "Loremaster of Kalimdor", "Insane in the Membrane"} {
		var found []string
		for _, tb := range []string{names.Items, names.Npcs, names.Instances, names.Recipes, names.ItemSets} {
			hits, _, err := names.Find(ds, tb, name, 3)
			if err != nil {
				continue
			}
			for _, h := range hits {
				found = append(found, tb+":"+h.Name)
			}
		}
		out = append(out, benchRow{name + ": achievement by name", fmt.Sprint(found), "no row of any named table", itemsNoneSrc, resultIf(len(found) == 0, "a row was matched to an achievement")})
	}
	lvl := 60
	c := character.Character{Name: "Benchtest", Faction: "Horde", Class: "WARRIOR", Level: &lvl}
	b := brief.Character(c, character.Client{}, time.Unix(0, 0))
	out = append(out, benchRow{"warrior 60: the character brief", fmt.Sprintf("%d characters, a claim: %v", len(b), claimWords.MatchString(b)), "no achievement, key binding or currency claim: the capture holds none", itemsNoneSrc, resultIf(len(b) > 0 && !claimWords.MatchString(b), "a claim about an achievement, key binding or currency")})
	finish(t, "advice-achievement-no-answer", "Offline items: achievements and key bindings, no answer in the dataset", out)
}

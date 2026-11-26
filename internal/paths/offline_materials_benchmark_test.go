package paths_test

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/WoW-Companion/advisor/internal/ask"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/dungeons"
	"github.com/WoW-Companion/advisor/internal/inventory"
	"github.com/WoW-Companion/advisor/internal/learn"
	"github.com/WoW-Companion/advisor/internal/paths"
	"github.com/WoW-Companion/advisor/internal/questie"
	"github.com/WoW-Companion/advisor/internal/route"
	"github.com/WoW-Companion/advisor/internal/session"
	"github.com/WoW-Companion/advisor/internal/store"
)

// The offline crafting-material, bank and Auction House, vendor-versus-craft cost, travel-time and recurring-activity
// benchmark: what the advisor answers about where a crafting material comes from (gathered, sold, dropped, bought),
// what it does with bank and bag stacks, what a vendor charges beside the cost of the craft, how long a walk takes
// and where a dungeon is, and which holiday events run when, beside the dataset's rows, with no network. It reads
// testdata/places_snapshot (vendor_sources, npcs, instances) first and testdata/advice_snapshot (items, recipes,
// recipe_reagents, loot_sources) for the rest. The dataset holds no price on the Auction House, no daily or weekly
// quest, no raid lockout or reset time, no travel time and no gathering node in the snapshot, so the cases it cannot
// answer check that the advisor says so and states none, and the gathering-node cases compose rows of the dataset's
// loot_sources and gathering_nodes shape under synthetic names. A difference whose cause is a wrong or missing
// dataset row is marked "(data)". make offline-benchmark runs it; WOWC_BENCH_REPORT receives the tables.

const (
	mbMatSrc    = "dataset loot_sources (object, creature, sold by), vendor_sources, items (bonding, quality), recipe_reagents"
	mbNodeSrc   = "dataset loot_sources (gathered: node_name, skill_required) and gathering_nodes (zone_name), composed"
	mbBankSrc   = "dataset recipe_reagents (count); the capture's bags and bank"
	mbReviewSrc = "dataset items (bonding, quality, sell_price, slot), recipes, recipe_reagents"
	mbCostSrc   = "dataset vendor_sources (price_copper), items (buy_price), recipe_reagents (count)"
	mbTravelSrc = "game run speed 7 yd/s, mount +60%, zone widths (public map sizes); dataset instances (entrance_zone_name)"
	mbEventSrc  = "game holiday calendar (public event dates); the dataset has no event or reset table"
)

// mbStore adds composed rows to the places and advice snapshots and names the vendors the snapshot has no name for:
// every other table and row is the dataset's.
type mbStore struct {
	store.DataStore
	loot, nodes []store.Row
	named       bool // give a nameless vendor a synthetic name
	bind        map[int64]int64
}

func mbMatch(row store.Row, ranges []store.Range) bool {
	for _, rg := range ranges {
		v := num(row[rg.Column])
		if lo, ok := rg.Min.(int64); ok && v < lo {
			return false
		}
		if hi, ok := rg.Max.(int64); ok && v > hi {
			return false
		}
	}
	return true
}

func (m mbStore) Filter(table string, ranges ...store.Range) ([]store.Row, error) {
	out, err := m.DataStore.Filter(table, ranges...)
	var add []store.Row
	switch table {
	case "loot_sources":
		add = m.loot
	case "gathering_nodes":
		add = m.nodes
	}
	for _, row := range add {
		if mbMatch(row, ranges) {
			out = append(out, row)
			err = nil // the snapshot lacks the table: the composed rows stand for it
		}
	}
	if table == "items" && len(m.bind) > 0 {
		for i, r := range out {
			if b, ok := m.bind[num(r["id"])]; ok {
				c := store.Row{}
				for k, v := range r {
					c[k] = v
				}
				c["bonding"] = b
				out[i] = c
			}
		}
	}
	return out, err
}

func (m mbStore) Get(table string, id int64) (store.Row, error) {
	r, err := m.DataStore.Get(table, id)
	switch {
	case table == "npcs" && m.named && (err != nil || text(r["name"]) == ""):
		return store.Row{"id": id, "name": fmt.Sprintf("Synthetic Vendor %d", id), "zone_name": "Synthetic Vale"}, nil
	case table == "items" && err == nil:
		if b, ok := m.bind[id]; ok {
			c := store.Row{}
			for k, v := range r {
				c[k] = v
			}
			c["bonding"] = b
			return c, nil
		}
	}
	return r, err
}

func mbRaw() mbStore   { return mbStore{DataStore: qbStore()} }
func mbNamed() mbStore { return mbStore{DataStore: qbStore(), named: true} }

func mbChar(level int, faction string, profs map[string]int) character.Character {
	c := character.Character{Name: "Benchtest", Faction: faction, Class: "MAGE", Level: &level}
	for _, n := range []string{"Alchemy", "Blacksmithing", "Cooking", "Engineering", "First Aid", "Herbalism", "Leatherworking", "Mining", "Skinning", "Tailoring"} {
		if r, ok := profs[n]; ok {
			rank, top := r, 300
			c.Professions = append(c.Professions, character.Skill{Name: n, Rank: &rank, Max: &top})
		}
	}
	return c
}

// mbReagent is the advisor's words for one reagent of one craft, and the craft path's line.
func mbReagent(t *testing.T, c character.Character, ds store.DataStore, craft int64, reagent string) (paths.Reagent, paths.Path) {
	t.Helper()
	res := paths.For(c, ds, craft, paths.Options{})
	for _, p := range res.Paths {
		if p.Kind != paths.Craft {
			continue
		}
		for _, g := range p.Reagents {
			if g.Name == reagent {
				return g, p
			}
		}
	}
	t.Fatalf("craft %d: no reagent %q in %v", craft, reagent, res.Text())
	return paths.Reagent{}, paths.Path{}
}

func mbRows(t *testing.T, ds store.DataStore, table, col string, id int64) []store.Row {
	t.Helper()
	r, err := ds.Filter(table, store.Range{Column: col, Min: id, Max: id})
	if err != nil {
		t.Fatalf("%s: %v", table, err)
	}
	return r
}

func mbKind(how string) string {
	l := strings.ToLower(how)
	switch {
	case strings.HasPrefix(l, "gather"):
		return "gather"
	case strings.Contains(l, "buy from") || strings.Contains(l, "from a vendor"):
		return "vendor"
	case strings.Contains(l, "auction house"):
		return "auction"
	}
	return "none"
}

// mbMaterials: the way to get a missing material, as the craft's reagent line words it, beside what the dataset's
// rows say: a vendor row means a vendor, a node or the item's own name a gatherer, a tradeable drop the Auction House.
var mbMaterials = []struct {
	label, reagent string
	craft          int64
	profs          map[string]int
	want           string
}{
	{"Peacebloom, Herbalism known", "Peacebloom", 2458, map[string]int{"Alchemy": 100, "Herbalism": 100}, "gather"},
	{"Peacebloom, no Herbalism", "Peacebloom", 2458, map[string]int{"Alchemy": 100}, "auction"},
	{"Earthroot, Herbalism known", "Earthroot", 2458, map[string]int{"Alchemy": 100, "Herbalism": 100}, "gather"},
	{"Rough Stone, Mining known", "Rough Stone", 3470, map[string]int{"Blacksmithing": 100, "Mining": 100}, "gather"},
	{"Rough Stone, no Mining", "Rough Stone", 3470, map[string]int{"Blacksmithing": 100}, "auction"},
	{"Light Leather, Skinning known", "Light Leather", 2304, map[string]int{"Leatherworking": 100, "Skinning": 100}, "gather"},
	{"Coarse Thread (sold by vendors)", "Coarse Thread", 2570, map[string]int{"Tailoring": 100}, "vendor"},
	{"Empty Vial (sold by vendors)", "Empty Vial", 2458, map[string]int{"Alchemy": 100}, "vendor"},
	{"Salt (sold by vendors)", "Salt", 4231, map[string]int{"Leatherworking": 100}, "vendor"},
	{"Weak Flux (sold by vendors)", "Weak Flux", 4361, map[string]int{"Engineering": 100}, "vendor"},
	{"Mild Spices (sold by vendors)", "Mild Spices", 724, map[string]int{"Cooking": 100}, "vendor"},
	{"Wool Cloth (creature drops)", "Wool Cloth", 2997, map[string]int{"Tailoring": 100}, "auction"},
	{"Linen Cloth (creature drops)", "Linen Cloth", 1251, map[string]int{"First Aid": 100}, "auction"},
	{"Goretusk Liver (one creature)", "Goretusk Liver", 724, map[string]int{"Cooking": 100}, "auction"},
}

func TestOfflineBenchmarkMaterialSourcesA(t *testing.T) { materialSources(t, "a", mbMaterials[:7]) }
func TestOfflineBenchmarkMaterialSourcesB(t *testing.T) { materialSources(t, "b", mbMaterials[7:]) }

// materialSources: where the advisor sends a player for a missing material, with the vendors
// unnamed as the snapshot has them: the dataset's sold-by rows mean a vendor, never the Auction House.
func materialSources(t *testing.T, part string, cases []struct {
	label, reagent string
	craft          int64
	profs          map[string]int
	want           string
}) {
	ds := mbRaw()
	var out []benchRow
	for _, tc := range cases {
		g, _ := mbReagent(t, mbChar(30, "Horde", tc.profs), ds, tc.craft, tc.reagent)
		it := rowByName(t, ds, "items", tc.reagent)
		id := num(it["id"])
		vend := len(mbRows(t, ds, "vendor_sources", "item_id", id))
		objects, creatures := 0, 0
		for _, r := range mbRows(t, ds, "loot_sources", "item_id", id) {
			switch text(r["source_kind"]) {
			case "object":
				objects++
			case "creature":
				creatures++
			}
		}
		ref := fmt.Sprintf("%s: %d vendor rows, %d node rows, %d creature rows, bonding %d", map[string]string{"gather": "gather it", "vendor": "buy from a vendor", "auction": "buy on the Auction House"}[tc.want], vend, objects, creatures, num(it["bonding"]))
		out = append(out, benchRow{tc.label + ": how to get it", g.How, ref, mbMatSrc, resultIf(mbKind(g.How) == tc.want, "the way to get it: "+mbKind(g.How)+" for "+tc.want)})
	}
	finish(t, "advice-material-source-"+part, "Offline crafting material: how to get a missing one ("+part+")", out)
}

// TestOfflineBenchmarkMaterialVendorNamed: a vendor the dataset names is named with its zone and price.
func TestOfflineBenchmarkMaterialVendorNamed(t *testing.T) {
	ds := mbNamed()
	var out []benchRow
	for _, tc := range []struct {
		reagent string
		craft   int64
		prof    string
	}{{"Coarse Thread", 2570, "Tailoring"}, {"Empty Vial", 2458, "Alchemy"}, {"Salt", 4231, "Leatherworking"}, {"Weak Flux", 4361, "Engineering"}} {
		g, _ := mbReagent(t, mbChar(30, "Horde", map[string]int{tc.prof: 100}), ds, tc.craft, tc.reagent)
		id := num(rowByName(t, ds, "items", tc.reagent)["id"])
		rows := mbRows(t, ds, "vendor_sources", "item_id", id)
		ok := g.Offer != nil && strings.HasPrefix(g.How, "buy from Synthetic Vendor") && strings.Contains(g.How, " for ") && len(rows) > 0
		out = append(out, benchRow{tc.reagent + ": named vendor", g.How, fmt.Sprintf("a vendor and a price from %d vendor rows", len(rows)), mbMatSrc, resultIf(ok, "the vendor or its price")})
	}
	finish(t, "advice-material-vendor-named", "Offline crafting material: a named vendor", out)
}

// mbNode composes one gathered row and its node row, the dataset's shape, under synthetic names.
func mbNode(item, node int64, name, prof string, skill int64, zone string) (store.Row, store.Row) {
	loot := store.Row{"item_id": item, "source_kind": "gathered", "source_id": node, "node_id": node, "node_name": name, "profession": prof, "skill_required": skill, "tier": "confirmed"}
	nd := store.Row{"id": node, "node_id": node, "name": name, "profession": prof, "skill_required": skill, "zone_name": zone}
	return loot, nd
}

// TestOfflineBenchmarkMaterialGatherNodes: a gathered material names its node and zone from the dataset's rows, and
// a node whose skill the character lacks is not offered as a way to gather it now.
func TestOfflineBenchmarkMaterialGatherNodes(t *testing.T) {
	const mageroyal, peacebloom = 785, 2447
	l1, n1 := mbNode(mageroyal, 2852, "Synthetic Herb Patch", "Herbalism", 50, "Synthetic Vale")
	l2, n2 := mbNode(peacebloom, 910002, "Synthetic Bloom Bed", "Herbalism", 1, "Synthetic Vale")
	ds := mbStore{DataStore: qbStore(), loot: []store.Row{l1, l2}, nodes: []store.Row{n1, n2}}
	var out []benchRow
	g, _ := mbReagent(t, mbChar(30, "Horde", map[string]int{"Alchemy": 100, "Herbalism": 80}), ds, 2455, "Mageroyal")
	out = append(out, benchRow{"Mageroyal at Herbalism 80: node and zone", g.How, "gather it: Synthetic Herb Patch in Synthetic Vale (needs Herbalism 50)", mbNodeSrc,
		resultIf(mbKind(g.How) == "gather" && strings.Contains(g.How, "Synthetic Herb Patch") && strings.Contains(g.How, "Synthetic Vale"), "the node or its zone")})
	g, _ = mbReagent(t, mbChar(30, "Horde", map[string]int{"Alchemy": 100, "Herbalism": 30}), ds, 2455, "Mageroyal")
	out = append(out, benchRow{"Mageroyal at Herbalism 30: skill short of the node", g.How, "not gatherable yet: the node needs Herbalism 50", mbNodeSrc,
		resultIf(mbKind(g.How) != "gather" && strings.Contains(g.How, "50"), "gathering offered with too little skill, or the skill needed not said")})
	g, _ = mbReagent(t, mbChar(30, "Horde", map[string]int{"Alchemy": 100, "Herbalism": 30}), ds, 2458, "Peacebloom")
	out = append(out, benchRow{"Peacebloom at Herbalism 30: node needs 1", g.How, "gather it: Synthetic Bloom Bed in Synthetic Vale (needs Herbalism 1)", mbNodeSrc,
		resultIf(mbKind(g.How) == "gather" && strings.Contains(g.How, "Synthetic Bloom Bed"), "the node")})
	g, _ = mbReagent(t, mbChar(30, "Horde", map[string]int{"Alchemy": 100}), ds, 2455, "Mageroyal")
	out = append(out, benchRow{"Mageroyal with no Herbalism", g.How, "buy it: no gathering profession, a tradeable material", mbNodeSrc,
		resultIf(mbKind(g.How) == "auction" || mbKind(g.How) == "vendor", "gathering offered without the profession")})
	finish(t, "advice-material-gather-nodes", "Offline crafting material: gathering node, zone and skill", out)
}

// TestOfflineBenchmarkMaterialBank: materials in the bank count toward a craft, and the advisor says they are in the
// bank (a player has to fetch them).
func TestOfflineBenchmarkMaterialBank(t *testing.T) {
	ds := mbRaw()
	const stone = 2835
	var out []benchRow
	for _, tc := range []struct {
		label        string
		bags, bank   int
		wantBank     bool
		wantCovered  bool
		needsLine    string
		needsMissing int
	}{
		{"both Rough Stone in the bank", 0, 2, true, true, "2/2 Rough Stone", 0},
		{"one in the bags, one in the bank", 1, 1, true, true, "2/2 Rough Stone", 0},
		{"both in the bags", 2, 0, false, true, "2/2 Rough Stone", 0},
		{"one in the bank, one short", 0, 1, true, false, "1/2 Rough Stone", 1},
	} {
		c := mbChar(30, "Horde", map[string]int{"Blacksmithing": 100})
		if tc.bags > 0 {
			c.Bags = []character.ItemStack{{ItemID: stone, Name: "Rough Stone", Count: tc.bags}}
		}
		if tc.bank > 0 {
			c.Bank = []character.ItemStack{{ItemID: stone, Name: "Rough Stone", Count: tc.bank}}
		}
		g, p := mbReagent(t, c, ds, 3470, "Rough Stone")
		text := p.Line
		for _, r := range p.Reagents {
			text += "\n" + r.Name + ": " + r.How
		}
		bank := strings.Contains(strings.ToLower(paths.Result{Item: "x", Paths: []paths.Path{p}}.Text()), "bank")
		ok := strings.Contains(p.Line, tc.needsLine) && g.Missing == tc.needsMissing && bank == tc.wantBank
		ref := fmt.Sprintf("the recipe needs 2 (recipe_reagents); %d in the bank is counted and said", tc.bank)
		out = append(out, benchRow{tc.label, strings.TrimSpace(p.Line), ref, mbBankSrc, resultIf(ok, "the count, or the bank is not mentioned")})
	}
	finish(t, "advice-material-bank", "Offline bank: materials held in the bank", out)
}

// mbReview reviews the stacks of the characters once and finds one line of it.
func mbReview(chars []character.Character) func(who string, id int, where string) inventory.Line {
	lines := inventory.ReviewAll(chars, qbStore()).Lines
	return func(who string, id int, where string) inventory.Line {
		for _, l := range lines {
			if l.Character == who && l.ItemID == int64(id) && l.Where == where {
				return l
			}
		}
		return inventory.Line{}
	}
}

func mbReviewRow(label string, l inventory.Line, ref string, ok bool) benchRow {
	got := fmt.Sprintf("%s: %s", l.Action, l.Reason)
	if l.AH {
		got += " (Auction House)"
	}
	return benchRow{label, got, ref, mbReviewSrc, resultIf(ok, "the action or the Auction House")}
}

func mbMage(bags, bank []character.ItemStack) character.Character {
	level := 30
	return character.Character{Name: "Benchmage", Faction: "Horde", Class: "MAGE", Level: &level, BankCaptured: true, BagsCaptured: true, Bags: bags, Bank: bank}
}

// TestOfflineBenchmarkBankAuctionGear: what the advisor does with a piece of gear in the bags or the bank, beside the
// item's own row: a Bind on Equip green nobody can wear goes to the Auction House, a Bind on Pickup one cannot.
func TestOfflineBenchmarkBankAuctionGear(t *testing.T) {
	const boeBoots, bopChest = 2307, 3750
	find := mbReview([]character.Character{mbMage(
		[]character.ItemStack{{ItemID: boeBoots, Count: 1}, {ItemID: bopChest, Count: 1}}, []character.ItemStack{{ItemID: boeBoots, Count: 1}})})
	var out []benchRow
	for _, where := range []string{"bags", "bank"} {
		l := find("Benchmage", boeBoots, where)
		out = append(out, mbReviewRow("Bind on Equip green boots nobody can wear, in the "+where, l, "sell on the Auction House: bonding 2, quality 2, a mage wears no leather", l.Action == inventory.Sell && l.AH))
	}
	l := find("Benchmage", bopChest, "bags")
	out = append(out, mbReviewRow("Bind on Pickup mail chest nobody can wear", l, "sell to a vendor: bonding 1, not tradeable, no Auction House", l.Action == inventory.Sell && !l.AH))
	finish(t, "advice-bank-auction-gear", "Offline bank and Auction House: gear nobody can wear", out)
}

// TestOfflineBenchmarkBankAuctionMaterials: a material nobody needs goes to the Auction House when tradeable; a
// material an alt's profession needs is mailed to the alt.
func TestOfflineBenchmarkBankAuctionMaterials(t *testing.T) {
	const malachite, peace = 774, 2447
	level, rank, top := 30, 100, 300
	alch := character.Character{Name: "Benchalch", Faction: "Horde", Class: "WARRIOR", Level: &level, BankCaptured: true, BagsCaptured: true,
		Professions: []character.Skill{{Name: "Alchemy", Rank: &rank, Max: &top}}}
	find := mbReview([]character.Character{mbMage(nil, []character.ItemStack{{ItemID: malachite, Count: 1}, {ItemID: peace, Count: 1}}), alch})
	var out []benchRow
	l := find("Benchmage", malachite, "bank")
	out = append(out, mbReviewRow("Malachite in the bank, no crafter uses it", l, "sell on the Auction House: a tradeable material nobody needs", l.Action == inventory.Sell && l.AH))
	l = find("Benchmage", peace, "bank")
	out = append(out, mbReviewRow("Peacebloom in the bank, an alt has Alchemy", l, "mail it to the Alchemist alt: a recipe reagent", l.Action == inventory.Mail && l.To == "Benchalch"))
	finish(t, "advice-bank-auction-materials", "Offline bank and Auction House: materials", out)
}

// TestOfflineBenchmarkVendorCraftCost: the price a vendor row shows beside the craft's materials, from the dataset's
// own rows. The advisor shows the vendor's price (the buy price when a reputation discount lowered a row), counts the
// recipe's materials from recipe_reagents, and ranks a cheap vendor above a craft that still needs materials.
func TestOfflineBenchmarkVendorCraftCost(t *testing.T) {
	ds := mbNamed()
	var out []benchRow
	type vc struct {
		label string
		item  int64
		craft string // the profession of the craft
		level int
	}
	cases := []vc{
		{"Copper Rod", 6217, "Blacksmithing", 30}, {"Copper Rod at level 5", 6217, "Blacksmithing", 5}, {"Coarse Blasting Powder", 4364, "Engineering", 30},
		{"Lesser Healing Potion", 858, "First Aid", 30}, {"Mote of Magic", 247786, "Enchanting", 30},
	}
	for _, tc := range cases {
		it, err := ds.Get("items", tc.item)
		if err != nil {
			t.Fatal(err)
		}
		rows := mbRows(t, ds, "vendor_sources", "item_id", tc.item)
		low := int64(1 << 40)
		for _, r := range rows {
			if p := num(r["price_copper"]); p > 0 && p < low {
				low = p
			}
		}
		buy := num(it["buy_price"])
		want := low
		if buy > want {
			want = buy
		}
		c := mbChar(tc.level, "Horde", map[string]int{tc.craft: 100})
		res := paths.For(c, ds, tc.item, paths.Options{})
		var vendor paths.Path
		have := false
		for _, p := range res.Paths {
			if p.Kind == paths.Vendor {
				vendor, have = p, true
			}
		}
		got, price := "no vendor path", int64(0)
		if have && vendor.Offer != nil {
			got, price = vendor.Line, vendor.Offer.PriceCopper
		}
		out = append(out, benchRow{tc.label + ": vendor price", got, fmt.Sprintf("%d copper (lowest row %d, items.buy_price %d)", want, low, buy), mbCostSrc, resultIf(have && price == want, "the vendor price")})
		// the ranking: a vendor at or under the cheap line comes before a craft that still needs materials
		cheap := want <= int64(tc.level/10+1)*10000
		first := ""
		if len(res.Paths) > 0 {
			first = res.Paths[0].Kind
		}
		wantFirst := paths.Craft
		if cheap {
			wantFirst = paths.Vendor
		}
		out = append(out, benchRow{tc.label + ": first way", first, fmt.Sprintf("%s: the vendor price is %s the cheap line of %d copper", wantFirst, map[bool]string{true: "under", false: "over"}[cheap], int64(tc.level/10+1)*10000), mbCostSrc, resultIf(first == wantFirst, "the order of the ways")})
	}
	// the craft's own materials: the counts the line states are the recipe's
	for _, tc := range []struct {
		item   int64
		prof   string
		counts map[string]int
	}{
		{4361, "Engineering", map[string]int{"Copper Bar": 2, "Weak Flux": 1}},
		{2997, "Tailoring", map[string]int{"Wool Cloth": 3}},
		{3470, "Blacksmithing", map[string]int{"Rough Stone": 2}},
	} {
		res := paths.For(mbChar(30, "Horde", map[string]int{tc.prof: 100}), ds, tc.item, paths.Options{})
		var got []string
		ok := false
		for _, p := range res.Paths {
			if p.Kind != paths.Craft {
				continue
			}
			ok = len(p.Reagents) == len(tc.counts)
			for _, g := range p.Reagents {
				got = append(got, fmt.Sprintf("%d %s", g.Need, g.Name))
				ok = ok && tc.counts[g.Name] == g.Need
			}
		}
		out = append(out, benchRow{res.Item + ": materials of the craft", strings.Join(got, ", "), fmt.Sprintf("%v from recipe_reagents", tc.counts), mbCostSrc, resultIf(ok, "a material or its count")})
	}
	finish(t, "advice-vendor-craft-cost", "Offline vendor price and craft materials", out)
}

// mbZones are the widths of zone maps in yards, from the game's public map sizes (rounded): the advisor's
// session.MapYards stands for a typical zone.
var mbZones = []struct {
	zone  string
	yards float64
}{{"Elwynn Forest", 3470}, {"Westfall", 3500}, {"Dun Morogh", 4925}, {"Durotar", 5287}, {"Mulgore", 5137}, {"Tirisfal Glades", 4518}}

// TestOfflineBenchmarkTravelTime: the walking and riding time the session plan uses beside the game's speeds and
// zone sizes, the rounding of a total, and what the advisor calls a dungeon's entrance beside the dataset's.
func TestOfflineBenchmarkTravelTime(t *testing.T) {
	var out []benchRow
	level := func(l int) session.In { return session.In{Char: character.Character{Level: &l}} }
	a, b := route.Point{Map: 1}, route.Point{Map: 1, X: 10}
	for _, z := range mbZones {
		perPct := session.MapYards / 100
		ref := z.yards / 100
		out = append(out, benchRow{z.zone + ": yards across one map percent", fmt.Sprintf("%.0f", perPct), fmt.Sprintf("%.0f (zone about %.0f yards wide)", ref, z.yards), mbTravelSrc, resultIf(perPct >= 0.7*ref && perPct <= 1.5*ref, "the zone size")})
	}
	foot := level(20).WalkSecs(a, b)
	out = append(out, benchRow{"on foot, 10 map percent, level 20", fmt.Sprintf("%.0f s", foot), fmt.Sprintf("%.0f s at 7 yd/s over the straight line and the path factor", 10*session.MapYards/100*session.PathFactor/7), mbTravelSrc, resultIf(session.WalkSpeed == 7, "the run speed")})
	mount := level(40).WalkSecs(a, b)
	out = append(out, benchRow{"mounted, 10 map percent, level 40", fmt.Sprintf("%.0f s (%.1f yd/s)", mount, foot/mount*7), "a 60% mount at 11.2 yd/s (the speed at level 40; the 100% mount comes later and costs far more)", mbTravelSrc, resultIf(foot/mount*session.WalkSpeed <= 11.2+0.01, "the mount speed")})
	out = append(out, benchRow{"the level a mount is assumed from", fmt.Sprint(session.MountLevel), "40: the first ground mount", mbTravelSrc, resultIf(session.MountLevel == 40, "the mount level")})
	prev := level(39).WalkSecs(a, b)
	out = append(out, benchRow{"on foot at level 39", fmt.Sprintf("%.0f s", prev), "as at level 20: no mount before 40", mbTravelSrc, resultIf(prev == foot, "a mount before level 40")})
	other := level(20).WalkSecs(route.Point{Map: 1}, route.Point{Map: 2, X: 50})
	out = append(out, benchRow{"between two maps", fmt.Sprintf("%.0f s", other), "no walking time: a stop on another map is a flight, boat or zeppelin, which the plan does not time", mbTravelSrc, resultIf(other == 0, "a walk across maps")})
	out = append(out, benchRow{"a total of 7 minutes", fmt.Sprint(session.RoundMinutes(7 * 60)), "5: totals round to the nearest five minutes", mbTravelSrc, resultIf(session.RoundMinutes(7*60) == 5 && session.RoundMinutes(8*60) == 10 && session.RoundMinutes(30) == 5, "the rounding")})
	// the dungeon entrance named in a travel step
	rows, _ := qbStore().Filter("instances", store.Range{Column: "map_id", Min: int64(0)})
	pl := &route.Places{Instances: map[int]route.Instance{}}
	for _, r := range rows {
		pl.Instances[int(num(r["map_id"]))] = route.Instance{Map: int(num(r["map_id"])), Name: text(r["name"]), Entrance: text(r["entrance_zone_name"])}
	}
	for _, d := range []struct {
		ui     int
		inst   int64
		entry  string
		isInst bool
	}{{310, 33, "Silverpine Forest", true}, {279, 43, "The Barrens", true}, {291, 36, "Westfall", true}, {213, 389, "Orgrimmar", true}} {
		var row store.Row
		for _, r := range rows {
			if num(r["map_id"]) == d.inst {
				row = r
			}
		}
		want := fmt.Sprintf("%s (%s)", text(row["name"]), text(row["entrance_zone_name"]))
		got := pl.Travel(d.ui)
		out = append(out, benchRow{fmt.Sprintf("travel step to dungeon map %d", d.ui), got, want + "; the game puts it in " + d.entry, "dataset instances (name, entrance_zone_name)",
			resultIf(got == want && text(row["entrance_zone_name"]) == d.entry && pl.IsInstance(d.ui), "the dungeon or its entrance zone")})
	}
	finish(t, "advice-travel-time", "Offline travel time and dungeon entrances", out)
}

// mbEvents are the holiday events whose window the game fixes each year, with the window of the game's calendar.
var mbEvents = []struct {
	key        string
	start, end [2]int
}{
	{"midsummer", [2]int{6, 21}, [2]int{7, 5}}, {"hallows-end", [2]int{10, 18}, [2]int{11, 1}}, {"winter-veil", [2]int{12, 15}, [2]int{1, 2}},
	{"childrens-week", [2]int{5, 1}, [2]int{5, 7}}, {"brewfest", [2]int{9, 20}, [2]int{10, 6}}, {"harvest-festival", [2]int{9, 22}, [2]int{9, 29}},
}

// TestOfflineBenchmarkRecurringEvents: which holiday events the advisor says run on a day, at the edges of each
// window and across New Year, beside the game's calendar. The dataset holds no event, daily or weekly reset.
func TestOfflineBenchmarkRecurringEvents(t *testing.T) {
	var out []benchRow
	day := func(m, d int) time.Time { return time.Date(2026, time.Month(m), d, 12, 0, 0, 0, time.UTC) }
	running := func(m, d int, key string) bool {
		for _, e := range questie.RunningEvents(day(m, d)) {
			if e.Key == key {
				return true
			}
		}
		return false
	}
	before := func(m, d int) (int, int) { t := day(m, d).AddDate(0, 0, -1); return int(t.Month()), t.Day() }
	after := func(m, d int) (int, int) { t := day(m, d).AddDate(0, 0, 1); return int(t.Month()), t.Day() }
	for _, e := range mbEvents {
		name := e.key
		bm, bd := before(e.start[0], e.start[1])
		am, ad := after(e.end[0], e.end[1])
		first, last := running(e.start[0], e.start[1], e.key), running(e.end[0], e.end[1], e.key)
		dayBefore, dayAfter := running(bm, bd, e.key), running(am, ad, e.key)
		got := fmt.Sprintf("first day %v, last day %v, day before %v, day after %v", first, last, dayBefore, dayAfter)
		out = append(out, benchRow{name + ": window", got, fmt.Sprintf("%d/%d to %d/%d: true, true, false, false", e.start[0], e.start[1], e.end[0], e.end[1]), mbEventSrc,
			resultIf(first && last && !dayBefore && !dayAfter, "an edge of the window")})
	}
	ev, _ := questie.EventByKey("midsummer")
	out = append(out, benchRow{"Midsummer: the line shown while it runs", ev.Mention(), "names the event and the last day, July 5", mbEventSrc, resultIf(strings.Contains(ev.Mention(), "Midsummer Fire Festival") && strings.Contains(ev.Mention(), "July 5"), "the event or its end")})
	same := len(questie.RunningEvents(time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)))
	out = append(out, benchRow{"September 25: two events at once", fmt.Sprint(same), "2: Brewfest and the Harvest Festival overlap", mbEventSrc, resultIf(same == 2, "the overlap")})
	leap := len(questie.RunningEvents(time.Date(2028, 2, 29, 12, 0, 0, 0, time.UTC)))
	out = append(out, benchRow{"February 29 of a leap year", fmt.Sprint(leap), "0: Love is in the Air ends on February 20", mbEventSrc, resultIf(leap == 0, "an event on February 29")})
	finish(t, "advice-recurring-events", "Offline recurring activity: holiday event windows", out)
}

var mbReset = regexp.MustCompile(`(?i)\b(daily|dailies|weekly|lockout|locked to|resets?|every (day|week)|per (day|week)|cooldown)\b`)

// TestOfflineBenchmarkRecurringNoAnswer: what the dataset does not hold, and the advisor says nothing of: a dungeon's
// or raid's lockout, a quest's repeat, an event the table does not list, travel times between maps, a profit.
func TestOfflineBenchmarkRecurringNoAnswer(t *testing.T) {
	ds := qbStore()
	none := "the dataset has no answer"
	var out []benchRow
	level := 60
	c := character.Character{Faction: "Horde", Class: "WARRIOR", Level: &level}
	for _, name := range []string{"Molten Core", "Blackwing Lair", "Zul'Gurub", "Stratholme", "Scarlet Monastery"} {
		p, togo, err := dungeons.Named(c, ds, nil, name)
		words := ""
		if p != nil {
			words = dungeons.RenderNamed(c, *p, togo)
		}
		// a line that says the lockout is not known is the advisor saying so, not stating one
		var stated []string
		for _, l := range strings.Split(words, "\n") {
			if !strings.HasSuffix(l, ": not known yet") {
				stated = append(stated, l)
			}
		}
		m := mbReset.FindString(strings.Join(stated, "\n"))
		if words == "" {
			words = "no dungeon plan: a raid is not planned"
		}
		out = append(out, benchRow{name + ": lockout or reset", firstLine(words), none + ": instances has no lockout or reset column", "dataset instances (no reset column)", resultIf(err == nil && m == "", "the answer says \""+m+"\"")})
	}
	q := qbQuest(t, qBattleHillsbrad)
	line := questLine(paths.For(c, qbRewards{ds, []store.Row{qbReward(iBelt, qBattleHillsbrad)}}, iBelt, paths.Options{}))
	out = append(out, benchRow{"quest " + text(q["title"]) + ": daily or repeatable", line, none + ": quests_observed has no repeat column", "dataset quests_observed (no repeat column)", resultIf(line != "" && mbReset.FindString(line) == "", "the line claims a repeat")})
	_, found := questie.FindEvent("Darkmoon Faire")
	out = append(out, benchRow{"Darkmoon Faire: when it runs", fmt.Sprint(found), none + ": no event row; the table lists yearly holidays only", mbEventSrc, resultIf(!found, "an event was matched")})
	for _, q := range []string{"when does my raid lockout reset each week?", "which dailies are available today?"} {
		cl := ask.Classify(q, ds, &c)
		out = append(out, benchRow{"the question \"" + q + "\"", fmt.Sprintf("kind %d, topic %q, subject %q", cl.Kind, cl.Topic, cl.Subject), none + ": a know-how question with no subject, so no facts are composed for it", "the question classes of ask.Classify", resultIf(cl.Kind == ask.KindKnowHow && cl.Topic == ask.TopicNone && cl.Subject == "", "a subject was invented")})
	}
	cr := learn.Crossing("Horde", "Eastern Kingdoms", "Kalimdor")
	out = append(out, benchRow{"travel time of the Horde crossing to Kalimdor", cr, none + ": no flight or zeppelin duration", mbTravelSrc, resultIf(cr != "" && !regexp.MustCompile(`\d`).MatchString(cr), "a duration was given")})
	res := paths.For(mbChar(30, "Horde", map[string]int{"Alchemy": 100}), mbNamed(), 2458, paths.Options{})
	profit := regexp.MustCompile(`(?i)\b(profit|worth it|margin|\d+ ?g(old)?\b)`).FindString(res.Text())
	out = append(out, benchRow{"is it worth crafting: a profit or an Auction House price", firstLine(res.Text()), none + ": no Auction House price in the dataset", "dataset items (buy_price, sell_price are vendor prices)", resultIf(profit == "", "the answer says \""+profit+"\"")})
	finish(t, "advice-recurring-no-answer", "Offline recurring activity, travel and price: no answer in the dataset", out)
}

// TestOfflineBenchmarkMaterialNoAnswer: a material the dataset knows no way to get is never given a made-up one.
func TestOfflineBenchmarkMaterialNoAnswer(t *testing.T) {
	var out []benchRow
	const lightHide = 783
	ds := mbStore{DataStore: qbStore(), bind: map[int64]int64{lightHide: 1}}
	c := mbChar(30, "Horde", map[string]int{"Leatherworking": 100})
	res := paths.For(c, ds, 4231, paths.Options{})
	craft := false
	for _, p := range res.Paths {
		if p.Kind == paths.Craft && !p.Informational {
			craft = true
		}
	}
	out = append(out, benchRow{"a Bind on Pickup material with no source row", fmt.Sprintf("a craft path: %v; %s", craft, firstLine(res.Reason)), "no craft path: no row says where to get it and it cannot be bought", "dataset items (bonding), loot_sources (no row)", resultIf(!craft, "a craft path was offered with a material it cannot get")})
	g, _ := mbReagent(t, mbChar(30, "Horde", map[string]int{"Leatherworking": 100}), mbRaw(), 4231, "Light Hide")
	out = append(out, benchRow{"Light Hide: no source row, tradeable", g.How, "the Auction House: the dataset lists no source and the item is tradeable", "dataset items (bonding 0), loot_sources (no row)", resultIf(mbKind(g.How) == "auction", "the way")})
	unnamed := regexp.MustCompile(`vendor \d+|npc \d+|id \d+`)
	g, _ = mbReagent(t, mbChar(30, "Horde", map[string]int{"Tailoring": 100}), mbRaw(), 2570, "Coarse Thread")
	out = append(out, benchRow{"a vendor the dataset gives no name", g.How, "sold by a vendor: no name in npcs, so none is named and no id is shown", "dataset vendor_sources, npcs (no name)", resultIf(mbKind(g.How) == "vendor" && g.Offer == nil && unnamed.FindString(g.How) == "", "a vendor id or the Auction House")})
	g, _ = mbReagent(t, mbChar(30, "Horde", map[string]int{"Alchemy": 100}), mbRaw(), 2458, "Peacebloom")
	out = append(out, benchRow{"the Auction House line shows no price", g.How, "no price: the dataset holds no Auction House price", "dataset items (no auction price)", resultIf(!regexp.MustCompile(`\d`).MatchString(g.How), "a price was shown")})
	out = append(out, benchRow{"an item the dataset does not know", paths.For(mbChar(30, "Horde", nil), mbRaw(), 99999999, paths.Options{}).Reason, "none: it is not in items", "dataset items (no row)", resultIf(paths.For(mbChar(30, "Horde", nil), mbRaw(), 99999999, paths.Options{}).Reason != "" && len(paths.For(mbChar(30, "Horde", nil), mbRaw(), 99999999, paths.Options{}).Paths) == 0, "a way was invented")})
	finish(t, "advice-material-no-answer", "Offline crafting material, no answer in the dataset", out)
}

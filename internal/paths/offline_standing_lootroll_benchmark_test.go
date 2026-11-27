package paths_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/ask"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/dungeons"
	"github.com/WoW-Companion/advisor/internal/gear"
	"github.com/WoW-Companion/advisor/internal/inventory"
	"github.com/WoW-Companion/advisor/internal/store"
)

// The offline reputation standing and grind, loot roll, vendor-trash and instance-entry benchmark: what the advisor says of how far a
// faction standing is from the next one, what a player rolls on a drop and how it binds, which stacks are vendor trash and what they
// sell for, and what a dungeon or raid asks to enter (level, group size, key), beside the dataset's rows and the public game rules, with
// no network. It reads testdata/usability_snapshot (items: bonding, quality, class_id, slot, allowable_class_mask, sell_price) and
// testdata/places_snapshot (instances: min_level, max_level, max_players). The dataset holds no faction, no reputation points, no standing
// a vendor or an item asks for, no grind route, no loot roll rule and no dungeon key, so the cases it cannot answer check that the advisor
// says so and states none. A difference the dataset's own rows cause is marked "(data)". make offline-benchmark runs it;
// WOWC_BENCH_REPORT receives the tables.

const (
	slStandSrc = "game reputation thresholds (Warcraft Wiki, Reputation: Neutral 0, Friendly 3000, Honored 9000, Revered 21000, Exalted 42000 of 43000); the capture's own bar (min, max, value)"
	slNoRepSrc = "the dataset has no faction, reputation or standing table, and no reputation column on an item, vendor or quest"
	slLootSrc  = "dataset items (bonding, class_id, subclass_id, slot, allowable_class_mask); game loot rules: Need for what you will use, Greed for the rest, a quest item is not rolled for"
	slTrashSrc = "dataset items (quality 0 is grey, sell_price in copper, bonding, class_id); game: grey items are only worth their vendor price, a quest item is not vendored"
	slEntrySrc = "dataset instances (min_level, max_level, max_players, instance_type); game dungeon guides (Warcraft Wiki, Wowhead)"
)

// ---- reputation standing ----

func repRow(name string, standing, value int, min, max *int) character.Character {
	return character.Character{Name: "Benchtest", Faction: "Alliance", Class: "WARRIOR",
		Reputations: []character.Reputation{{FactionID: 72, Name: name, Standing: &standing, Value: &value, Min: min, Max: max}}}
}

func ip(n int) *int { return &n }

var standingCases = []struct {
	label, faction string
	standing, val  int
	min, max       *int
	want           string // what the game's thresholds give: the standing and the points to the next
}{
	{"Neutral at 0 of 3000", "Stormwind", 4, 0, ip(0), ip(3000), "Neutral (0 of 3000 points in the standing; 3000 more to Friendly)"},
	{"Friendly at 4200 of 3000-9000", "Stormwind", 5, 4200, ip(3000), ip(9000), "Friendly (1200 of 6000 points in the standing; 4800 more to Honored)"},
	{"Honored at the floor, 9000", "Darnassus", 6, 9000, ip(9000), ip(21000), "Honored (0 of 12000 points in the standing; 12000 more to Revered)"},
	{"Revered one point short, 41999 of 21000-42000", "Ironforge", 7, 41999, ip(21000), ip(42000), "Revered (20999 of 21000 points in the standing; 1 more to Exalted)"},
	{"Exalted at 42500", "Gnomeregan Exiles", 8, 42500, ip(42000), ip(43000), "Exalted (500 of 1000 points: nothing further to earn)"},
	{"Unfriendly at -1500 of -3000-0", "Defias Brotherhood", 3, -1500, ip(-3000), ip(0), "Unfriendly (1500 of 3000 points in the standing; 1500 more to Neutral)"},
	{"Honored, no bar captured: the game's thresholds", "Stormwind", 6, 10000, nil, nil, "Honored (1000 of 12000 points in the standing; 11000 more to Revered)"},
	{"Revered, no bar captured: the game's thresholds", "Stormwind", 7, 40000, nil, nil, "Revered (19000 of 21000 points in the standing; 2000 more to Exalted)"},
}

// TestOfflineBenchmarkReputationStanding: what the advisor says of a faction standing, beside the game's thresholds: the standing, the points
// in it and the points to the next, from the capture's own bar or the thresholds where it holds none.
func TestOfflineBenchmarkReputationStanding(t *testing.T) {
	var out []benchRow
	for _, tc := range standingCases {
		c := repRow(tc.faction, tc.standing, tc.val, tc.min, tc.max)
		got := c.Reputations[0].Progress()
		want := tc.faction + ": " + tc.want
		ok := got == want
		out = append(out, benchRow{tc.label + ": standing and points to the next", got, want, slStandSrc, resultIf(ok, "the standing or the points to the next")})
	}
	finish(t, "advice-standing-points", "Offline reputation standing: points to the next standing", out)
}

// TestOfflineBenchmarkReputationNoAnswer: a reputation grind route, the standing an item asks for and the points a quest gives are not in the
// dataset: the advisor's facts say so and name none, and a question about anything else carries no reputation facts.
func TestOfflineBenchmarkReputationNoAnswer(t *testing.T) {
	ds := usabStore()
	c := repRow("Stormwind", 5, 4200, ip(3000), ip(9000))
	var out []benchRow
	for _, table := range []string{"factions", "reputations", "reputation_rewards"} {
		_, err := ds.Filter(table, store.Range{Column: "id", Min: int64(0), Max: int64(1)})
		out = append(out, benchRow{"a table of " + table, fmt.Sprint(err != nil), "no such table in the dataset", slNoRepSrc, resultIf(err != nil, "a table was invented")})
	}
	for _, q := range []string{
		"what is the fastest way to grind Stormwind reputation to Exalted?",
		"which standing do I need with the Thorium Brotherhood to buy the Dark Iron Boots plan?",
		"how much reputation does the Defias Brotherhood quest give?",
	} {
		f := ask.StandingFacts(c, q)
		out = append(out, benchRow{"\"" + q + "\": the facts", firstLine(f), "the data holds no standing gate, no points per quest and no grind route; say so, name none", slNoRepSrc,
			resultIf(strings.Contains(f, ask.NoStandingGrindFacts) && !strings.Contains(f, "Dark Iron"), "no statement that the data holds none")})
	}
	named := ask.StandingFacts(c, "how far am I from Honored with Stormwind?")
	out = append(out, benchRow{"a named faction: the player's own standing", strings.ReplaceAll(named, "\n", " "), "Stormwind: Friendly, 4800 more to Honored (the capture's own bar)", slStandSrc,
		resultIf(strings.Contains(named, "Stormwind: Friendly (1200 of 6000 points in the standing; 4800 more to Honored)"), "the standing the capture holds")})
	none := ask.StandingFacts(character.Character{Class: "WARRIOR"}, "what is my reputation with Ironforge?")
	out = append(out, benchRow{"no reputation captured: the standing", strings.ReplaceAll(none, "\n", " "), "not captured, so not known", slStandSrc, resultIf(strings.Contains(none, "were not captured"), "a standing was stated")})
	out = append(out, benchRow{"a question that is not about reputation", ask.StandingFacts(c, "what quest should I do next?"), "no reputation facts", slStandSrc, resultIf(ask.StandingFacts(c, "what quest should I do next?") == "", "reputation facts on another question")})
	finish(t, "advice-standing-no-answer", "Offline reputation grind and gates: no answer in the dataset", out)
}

// ---- loot roll and binding ----

var lootCases = []struct {
	label, item, class string
	level              int
	roll               gear.Roll
	binding            string
}{
	{"a Bind on Pickup robe a mage can wear", "Robe of Power", "MAGE", 50, gear.RollNeed, "Bind on Pickup"},
	{"a Bind on Pickup plate piece a mage cannot wear", "Sergeant Major's Plate Wristguards", "MAGE", 60, gear.RollGreed, "Bind on Pickup"},
	{"the same plate piece, a warrior (class mask holds warrior)", "Sergeant Major's Plate Wristguards", "WARRIOR", 60, gear.RollNeed, "Bind on Pickup"},
	{"a Bind on Equip shoulder a warrior can wear", "Heavy Mithril Shoulder", "WARRIOR", 40, gear.RollNeed, "Bind on Equip"},
	{"a Bind on Equip mail shoulder a mage cannot wear", "Heavy Mithril Shoulder", "MAGE", 40, gear.RollGreed, "Bind on Equip"},
	{"a warrior-only axe, a shaman: the class mask rules it out", "Elunite Axe", "SHAMAN", 40, gear.RollGreed, "Bind on Pickup"},
	{"a quest item is not rolled for", "Torch of Retribution", "MAGE", 40, gear.RollPass, "quest item"},
	{"a Bind on Use item that is not worn gear", "Deprecated Cat's Paw", "WARRIOR", 40, gear.RollGreed, "Bind on Use"},
}

// TestOfflineBenchmarkLootRoll: what a class rolls on a drop beside the item's binding, class mask and type in the dataset.
func TestOfflineBenchmarkLootRoll(t *testing.T) {
	var out []benchRow
	for _, tc := range lootCases {
		row := usabItem(t, tc.item)
		r := gear.LootRoll(tc.class, tc.level, row)
		bindWord := map[int64]string{1: "Bind on Pickup", 2: "Bind on Equip", 3: "Bind on Use", 4: "quest item"}[num(row["bonding"])]
		ref := fmt.Sprintf("%s (%s; dataset bonding %d)", tc.roll, tc.binding, num(row["bonding"]))
		out = append(out, benchRow{tc.label + ": the roll", r.Text(), ref, slLootSrc, resultIf(r.Roll == tc.roll && r.Binding == tc.binding && bindWord == tc.binding, "the roll or the binding")})
	}
	// the question names an item: the answer's facts carry its rule
	c := character.Character{Class: "MAGE", Level: ip(50)}
	f := ask.LootRollFacts(c, usabStore(), "should I need or greed on [Robe of Power]?")
	out = append(out, benchRow{"a question that links [Robe of Power]: the facts", strings.ReplaceAll(f, "\n", " "), "Need (Bind on Pickup)", slLootSrc, resultIf(strings.Contains(f, "Robe of Power: Need (Bind on Pickup)"), "the loot rule in the facts")})
	finish(t, "advice-loot-roll", "Offline loot roll: Need, Greed and binding", out)
}

// TestOfflineBenchmarkLootRollNoAnswer: an item the dataset does not hold has no roll or binding; a question that links no item carries no loot facts.
func TestOfflineBenchmarkLootRollNoAnswer(t *testing.T) {
	var out []benchRow
	r := gear.LootRoll("MAGE", 40, nil)
	out = append(out, benchRow{"an item the dataset does not hold", r.Text(), "no row: the binding and the class use are not known", slLootSrc, resultIf(r.Roll == gear.RollUnknown && r.Binding == "", "a roll was guessed")})
	c := character.Character{Class: "MAGE", Level: ip(50)}
	f := ask.LootRollFacts(c, usabStore(), "do I need on [Blade of Imaginary Dawn]?")
	out = append(out, benchRow{"a linked item the dataset does not hold: the facts", strings.ReplaceAll(f, "\n", " "), "says the dataset has no row", slLootSrc, resultIf(strings.Contains(f, "Blade of Imaginary Dawn: Unknown") && strings.Contains(f, "no row"), "a roll was guessed")})
	g := ask.LootRollFacts(c, usabStore(), "how does need before greed work?")
	out = append(out, benchRow{"a roll question that links no item", g, "no per-item loot facts", slLootSrc, resultIf(g == "", "loot facts without an item")})
	finish(t, "advice-loot-roll-no-answer", "Offline loot roll: no answer in the dataset", out)
}

// ---- vendor trash ----

// greyChar holds the items as grey stacks (the dataset's own rows, quality set to 0, the way the vendor price benchmark does).
func greyChar(t *testing.T, class string, count int, items ...string) (character.Character, moneyGrey) {
	c := bindChar(t, "Magea", class, 40)
	ids := map[int64]bool{}
	for _, n := range items {
		r := usabItem(t, n)
		ids[num(r["id"])] = true
		c.Bags = append(c.Bags, character.ItemStack{ItemID: int(num(r["id"])), Count: count, Name: n})
	}
	return c, moneyGrey{DataStore: usabStore(), ids: ids}
}

// TestOfflineBenchmarkVendorTrash: which stacks are vendor trash and what they sell for, beside each item's quality and sell_price.
func TestOfflineBenchmarkVendorTrash(t *testing.T) {
	var out []benchRow
	// a grey stack: the bag review and the all-characters review both say vendor and give count x sell_price
	c, ds := greyChar(t, "MAGE", 3, "Heavy Mithril Shoulder")
	row := usabItem(t, "Heavy Mithril Shoulder")
	want := num(row["sell_price"]) * 3
	rev := inventory.Review(c, ds)
	all := inventory.ReviewAll([]character.Character{c}, ds)
	l := bindLine(all.Lines, "Magea", "Heavy Mithril Shoulder")
	out = append(out, benchRow{"a grey stack of 3: the bag review", fmt.Sprintf("%d in Vendor, %d copper", len(rev.Groups[inventory.Vendor]), rev.VendorValue), fmt.Sprintf("Vendor, %d copper (3 x %d)", want, num(row["sell_price"])), slTrashSrc, resultIf(len(rev.Groups[inventory.Vendor]) == 1 && rev.VendorValue == want, "the group or the value")})
	out = append(out, benchRow{"a grey stack of 3: the all-characters review", fmt.Sprintf("%s %s, %d copper", l.Action, l.Reason, all.SellValue), fmt.Sprintf("Sell, %d copper", want), slTrashSrc, resultIf(l.Action == inventory.Sell && !l.AH && all.SellValue == want, "the action or the value")})
	// a grey bind-on-equip piece is not put on the Auction House: grey is only worth the vendor
	out = append(out, benchRow{"a grey Bind on Equip piece: Auction House or vendor", fmt.Sprintf("ah=%v", l.AH), "vendor only (grey)", slTrashSrc, resultIf(!l.AH, "a grey item was sent to the Auction House")})
	// a white item with no use is not vendor trash
	w, wds := greyChar(t, "MAGE", 5)
	wr := usabItem(t, "Bolt of Woolen Cloth")
	w.Bags = append(w.Bags, character.ItemStack{ItemID: int(num(wr["id"])), Count: 5, Name: "Bolt of Woolen Cloth"})
	wrev := inventory.Review(w, wds)
	out = append(out, benchRow{"a white (common) stack no rule places", fmt.Sprintf("%d in Vendor, %d in Unknown", len(wrev.Groups[inventory.Vendor]), len(wrev.Groups[inventory.Unknown])), "not vendor trash: quality 1, only grey is", slTrashSrc, resultIf(len(wrev.Groups[inventory.Vendor]) == 0 && num(wr["quality"]) == 1, "a white item was called vendor trash")})
	// a grey quest item no log asks for: not for the vendor, the same answer in both reviews
	q, qds := greyChar(t, "MAGE", 2, "Ryedol's Lucky Pick")
	qrev := inventory.Review(q, qds)
	qall := inventory.ReviewAll([]character.Character{q}, qds)
	ql := bindLine(qall.Lines, "Magea", "Ryedol's Lucky Pick")
	out = append(out, benchRow{"a grey quest item no quest needs: the bag review", fmt.Sprintf("%d in Vendor, value %d", len(qrev.Groups[inventory.Vendor]), qrev.VendorValue), "not for the vendor: a quest item (bonding 4) is never vendor trash", slTrashSrc, resultIf(len(qrev.Groups[inventory.Vendor]) == 0 && qrev.VendorValue == 0, "a quest item was put with the vendor trash")})
	out = append(out, benchRow{"a grey quest item no quest needs: the all-characters review", fmt.Sprintf("%s: %s", ql.Action, ql.Reason), "Toss: no quest needs it", slTrashSrc, resultIf(ql.Action == inventory.Toss, "the quest rule did not decide")})
	// two grey stacks: the vendor value is the sum
	m, mds := greyChar(t, "MAGE", 2, "Heavy Mithril Shoulder", "Dread Mage Hat")
	mrev := inventory.Review(m, mds)
	sum := 2*num(usabItem(t, "Heavy Mithril Shoulder")["sell_price"]) + 2*num(usabItem(t, "Dread Mage Hat")["sell_price"])
	out = append(out, benchRow{"two grey stacks: the vendor value", fmt.Sprintf("%d copper (%s)", mrev.VendorValue, inventory.Money(mrev.VendorValue)), fmt.Sprintf("%d copper", sum), slTrashSrc, resultIf(mrev.VendorValue == sum && len(mrev.Groups[inventory.Vendor]) == 2, "the total")})
	finish(t, "advice-vendor-trash", "Offline vendor trash: grey stacks, quest items and what they sell for", out)
}

// ---- instance entry ----

var entryCases = []struct {
	name    string
	raid    bool
	level   int    // the player's level
	want    string // what the guide gives
	players int64  // the game's group size
}{
	{"Dire Maul", false, 54, "opens around 55", 5},
	{"Ragefire Chasm", false, 13, "about levels 13–18", 5},
	{"Blackrock Depths", false, 52, "about levels 52–60", 5},
	{"Naxxramas", true, 60, "level 60", 40},
}

// TestOfflineBenchmarkInstanceEntry: the level a dungeon or raid is entered at and its group size, beside the instances row and the guides.
func TestOfflineBenchmarkInstanceEntry(t *testing.T) {
	ds := instStore()
	var out []benchRow
	for _, tc := range entryCases {
		lvl := tc.level
		c := character.Character{Faction: "Horde", Class: "WARRIOR", Level: &lvl}
		p, togo, err := dungeons.Named(c, ds, nil, tc.name)
		if err != nil || p == nil {
			out = append(out, benchRow{tc.name + ": entry", "no answer", "an instances row", slEntrySrc, "differ: the instance was not found"})
			continue
		}
		txt := dungeons.RenderNamed(c, *p, togo)
		var size int64
		for _, r := range instRows(t, ds, tc.name) {
			if tc.raid == (text(r["instance_type"]) == "raid") {
				size = num(r["max_players"])
			}
		}
		said := int64(dungeons.PartySize(tc.name, tc.raid, int(size))) // a dungeon's party is five, whatever the table's cap
		sizeSaid := p.MaxPlayers != nil && int64(*p.MaxPlayers) == said && strings.Contains(txt, partyWords(tc.name, tc.raid, said))
		out = append(out, benchRow{tc.name + ": the entry level", firstLine(strings.TrimSpace(txt)), tc.want, slEntrySrc, resultIf(strings.Contains(txt, tc.want), "the entry level")})
		res := "match"
		switch {
		case !sizeSaid:
			res = "differ: the group size is not the party's"
		case said != tc.players:
			res = fmt.Sprintf("differ: the advisor words %d players, the game %d", said, tc.players)
		}
		out = append(out, benchRow{tc.name + ": the group size", fmt.Sprintf("%d players", said), fmt.Sprintf("%d players (dataset %d)", tc.players, size), slEntrySrc, res})
	}
	finish(t, "advice-instance-entry", "Offline instance entry: level and group size", out)
}

func instRows(t *testing.T, ds store.DataStore, name string) []store.Row {
	t.Helper()
	rows, err := ds.Filter("instances", store.Range{Column: "name", Min: name, Max: name})
	if err != nil || len(rows) == 0 {
		t.Fatalf("instances %q: not in the snapshot (%v)", name, err)
	}
	return rows
}

// TestOfflineBenchmarkInstanceEntryNoAnswer: a map the dataset sizes but gives no level, and a key no table holds: the advisor says the level or
// the key is not known and names none.
func TestOfflineBenchmarkInstanceEntryNoAnswer(t *testing.T) {
	ds := instStore()
	lvl := 60
	c := character.Character{Faction: "Horde", Class: "WARRIOR", Level: &lvl}
	var out []benchRow
	for _, n := range []string{"Caverns of Time", "Demon Fall Canyon"} {
		r := instRows(t, ds, n)[0]
		p, togo, err := dungeons.Named(c, ds, nil, n)
		if err != nil || p == nil {
			out = append(out, benchRow{n + ": the entry level", "no answer", "an instances row", slEntrySrc, "differ: the instance was not found"})
			continue
		}
		txt := dungeons.RenderNamed(c, *p, togo)
		line := ""
		for _, l := range strings.Split(txt, "\n") {
			if strings.Contains(l, "Entry level") {
				line = strings.TrimSpace(l)
			}
		}
		out = append(out, benchRow{n + ": the entry level", orNone(line), "min_level and max_level are null in the dataset: not known", slEntrySrc,
			resultIf(r["min_level"] == nil && line == "Entry level: not known yet" && !strings.Contains(txt, "levels "), "the level was left out without saying so")})
	}
	for _, n := range []string{"Blackrock Depths"} {
		p, togo, _ := dungeons.Named(c, ds, nil, n)
		txt := ""
		if p != nil {
			txt = dungeons.RenderNamed(c, *p, togo)
		}
		out = append(out, benchRow{n + ": the key", firstMatch(txt, "Key or entry requirement"), "no key in the dataset: not known, none named", slEntrySrc,
			resultIf(strings.Contains(txt, "Key or entry requirement: not known yet") && !strings.Contains(strings.ToLower(txt), "key to"), "a key was named")})
	}
	finish(t, "advice-instance-entry-no-answer", "Offline instance entry: no level or key in the dataset", out)
}

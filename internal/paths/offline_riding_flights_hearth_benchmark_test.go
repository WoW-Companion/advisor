package paths_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/ask"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/inventory"
	"github.com/WoW-Companion/advisor/internal/learn"
	"github.com/WoW-Companion/advisor/internal/levelup"
	"github.com/WoW-Companion/advisor/internal/store"
)

// The offline mount, riding skill, flight path and hearthstone benchmark: how a mount item's level, binding and price are told, how a
// question about learning to ride, a flight's cost, a flight path or a hearthstone is answered (the dataset holds none of these rows, so the
// advisor must say so and name no trainer, node, fare or cooldown), the flight point and the bound hearthstone a class trainer trip names, and
// the crossing between continents, beside the dataset's rows and the game's own lists, with no network. It reads testdata/consumables_snapshot
// (every mount item: required_level, bonding, sell_price). The dataset holds no riding trainer, riding cost, flight path, flight master,
// fare, hearthstone or bind row. A difference caused by the dataset's own rows is marked "(data)". make offline-benchmark runs it;
// WOWC_BENCH_REPORT receives the tables.

const (
	rfMountSrc = "dataset items (class 15, subclass 5: required_level, bonding, sell_price null = the client stores zero); game: mounts are soulbound and have no vendor price"
	rfRideSrc  = "dataset: no riding skill, trainer, cost or race row (docs/schema.md lists none); game: riding is learned from a riding trainer of the race"
	rfFlySrc   = "game flight list (Wowhead taxi guides); the dataset has no flight path, flight master or fare table"
	rfHearSrc  = "the capture's hearth bind (GetBindLocation); the game's hearthstone rule; the dataset has no hearthstone item, bind or cooldown row"
	rfCrossSrc = "game transport guides (Wowhead: zeppelins, boats and the Dark Portal); dataset ui_maps"
	rfNoneSrc  = "the dataset has no such table, row or column"
)

// rfSays is whether the limit facts for a question carry the topic and the instruction to say so, with no figure in them.
func rfSays(q, topic string) (string, bool) {
	f := ask.LimitFacts(q)
	return firstLine(f), strings.Contains(f, topic) && strings.Contains(f, "Say so") && !trDigit.MatchString(f)
}

// TestOfflineBenchmarkMountTooltipBinding: a mount item's tooltip: the dataset's required level and binding, and no vendor to sell it to when
// the dataset lists no sell price (a soulbound mount with a null price is not "only sold to a vendor").
func TestOfflineBenchmarkMountTooltipBinding(t *testing.T) {
	ds := consStore()
	var out []benchRow
	for _, tc := range []struct{ name, gameBind string }{
		{"Black Ram", "Bind on Pickup"}, {"Great White Kodo", "Bind on Pickup"}, {"Brown Horse Bridle", "Bind on Pickup"}, {"Reins of the Frostsaber", "Bind on Pickup"},
	} {
		row := rowByName(t, ds, "items", tc.name)
		line := firstLine(strings.TrimPrefix(ask.ItemTooltipFacts(ds, "what level do I need for ["+tc.name+"] and can i sell it?"), "Item tooltips (from the item rows; answer from these, not from memory):\n"))
		ok := strings.Contains(line, fmt.Sprintf("requires level %d;", num(row["required_level"]))) && strings.Contains(line, tc.gameBind) &&
			row["sell_price"] == nil && strings.Contains(line, "no vendor buys it") && !strings.Contains(line, "only sold to a vendor")
		out = append(out, benchRow{tc.name + ": level, binding and where it sells", line,
			fmt.Sprintf("required_level %d, bonding %d (%s), sell_price null: no vendor buys it", num(row["required_level"]), num(row["bonding"]), tc.gameBind), rfMountSrc,
			resultIf(ok, "the level or binding, or a vendor sale told for an item with no vendor price")})
	}
	// a mount the dataset does price: the price is the dataset's, with its copper
	row := rowByName(t, ds, "items", "Deathcharger's Reins")
	line := firstLine(strings.TrimPrefix(ask.ItemTooltipFacts(ds, "can i sell [Deathcharger's Reins]?"), "Item tooltips (from the item rows; answer from these, not from memory):\n"))
	out = append(out, benchRow{"Deathcharger's Reins: the vendor price", line, fmt.Sprintf("sell_price %d copper", num(row["sell_price"])), rfMountSrc,
		resultIf(strings.Contains(line, "sells to a vendor for 25g each (250000 copper)") && !strings.Contains(line, "no vendor buys it"), "the dataset's price was not told, or a price was denied")})
	finish(t, "advice-mount-tooltip", "Offline mounts: the tooltip's level, binding and vendor price", out)
}

// TestOfflineBenchmarkMountBagReview: a mount in the bags is never sent to a vendor or thrown away by the bag review: no rule places it, so it is left as "not sure".
func TestOfflineBenchmarkMountBagReview(t *testing.T) {
	ds := consStore()
	names := []string{"Black Ram", "Brown Horse Bridle", "Crumbling Hearthstone"}
	lvl := 60
	c := character.Character{Name: "Wara", Class: "WARRIOR", Faction: "Horde", Level: &lvl, BagsCaptured: true, BankCaptured: true, Equipment: map[string]character.EquippedItem{}}
	for _, n := range names {
		c.Bags = append(c.Bags, character.ItemStack{ItemID: int(num(rowByName(t, ds, "items", n)["id"])), Count: 1, Name: n})
	}
	r := inventory.ReviewAll([]character.Character{c}, ds)
	var out []benchRow
	for _, n := range names {
		l := bindLine(r.Lines, "Wara", n)
		out = append(out, benchRow{n + " in the bags: sell or toss it?", fmt.Sprintf("%s (%s)", l.Action, l.Reason), "not sold or tossed: a mount or hearthstone is kept by the player", rfMountSrc,
			resultIf(l.Action == inventory.Unsure && r.SellValue == 0, "a mount or hearthstone was sent to a vendor or thrown away")})
	}
	finish(t, "advice-mount-bag-review", "Offline mounts and the hearthstone in the bags: never sold or tossed", out)
}

// TestOfflineBenchmarkRidingPhrasings: a question about learning to ride, in the words a player uses, gets the line that the dataset holds no riding trainer, level or cost.
func TestOfflineBenchmarkRidingPhrasings(t *testing.T) {
	var out []benchRow
	for _, q := range []string{
		"where do I learn to ride as a gnome?", "how much does it cost to learn to ride?", "what level do I need to ride a horse?",
		"which races can ride a kodo?", "can my troll ride a raptor yet?", "where can I go train to ride?",
	} {
		line, ok := rfSays(q, "Riding and mounts:")
		out = append(out, benchRow{"\"" + q + "\"", line, "no riding trainer, level, cost or race row: say so, name none", rfRideSrc, resultIf(ok, "no statement that the data holds no riding row, or a figure in it")})
	}
	for _, q := range []string{"how do I ride the zeppelin to Undercity?", "how long is the boat ride to Auberdine?"} {
		out = append(out, benchRow{"\"" + q + "\"", ask.LimitFacts(q) + "(none)", "a boat or zeppelin ride is not the riding skill: no riding facts", rfRideSrc,
			resultIf(!strings.Contains(ask.LimitFacts(q), "Riding and mounts:"), "riding facts on a question about a boat or a zeppelin")})
	}
	finish(t, "advice-riding-phrasings", "Offline riding skill: the words a player uses to ask", out)
}

// TestOfflineBenchmarkFlightFares: what a flight costs, in the ways it is asked, gets the line that the dataset holds no fare or flight time.
func TestOfflineBenchmarkFlightFares(t *testing.T) {
	var out []benchRow
	for _, q := range []string{
		"what does it cost to fly from Stormwind to Ironforge?", "how much does it cost to fly to Booty Bay?", "how much is the tram from Ironforge to Stormwind?",
		"how much is a flight to Ironforge?", "how long is the flight from Menethil to Auberdine?",
	} {
		line, ok := rfSays(q, "Travel times:")
		out = append(out, benchRow{"\"" + q + "\"", line, "no fare, flight time or tram schedule in the data: say so, name none", rfFlySrc, resultIf(ok, "a fare or a time was left unsaid, or one was named")})
	}
	finish(t, "advice-flight-fares", "Offline flight fares and times: no answer in the dataset", out)
}

// TestOfflineBenchmarkFlightPathQuestions: which flight path to discover first, where a flight master stands and which nodes connect: the dataset holds none of it.
func TestOfflineBenchmarkFlightPathQuestions(t *testing.T) {
	var out []benchRow
	for _, q := range []string{
		"which flight path should I discover first in Westfall?", "where is the flight master in Stormwind?", "which flight paths connect Southshore and Tarren Mill?",
		"how do I unlock the Thelsamar flight path?", "where is the gryphon master in Ironforge?",
	} {
		line, ok := rfSays(q, "Flight paths:")
		out = append(out, benchRow{"\"" + q + "\"", line, "no flight path, flight master or link row: say so, name no node or order", rfFlySrc, resultIf(ok, "no statement that the data holds no flight path row, or a figure in it")})
	}
	ds := bdAdvice()
	for _, table := range []string{"flight_paths", "taxi_nodes", "flight_masters"} {
		_, err := ds.Filter(table, store.Range{Column: "id", Min: int64(0)})
		res := "match"
		if err == nil {
			res = "differ: the dataset now holds a " + table + " table: the cases here need a second look"
		}
		out = append(out, benchRow{"dataset table " + table, fmt.Sprintf("error: %v", err != nil), "absent: no flight row to answer from", rfNoneSrc, res})
	}
	finish(t, "advice-flight-path-questions", "Offline flight paths: which to discover, where the master stands, what connects", out)
}

// TestOfflineBenchmarkFlightPointsToCity: the flight point a class trainer trip names, for zones and classes beside the flight point benchmark's.
func TestOfflineBenchmarkFlightPointsToCity(t *testing.T) {
	var out []benchRow
	lvl := 30
	for _, tc := range []struct{ side, class, zone, city, point string }{
		{"Alliance", "MAGE", "Loch Modan", "Ironforge", "Thelsamar"},
		{"Alliance", "WARLOCK", "Westfall", "Stormwind City", "Sentinel Hill"},
		{"Horde", "WARRIOR", "The Barrens", "Orgrimmar", "the Crossroads"},
		{"Alliance", "PRIEST", "Loch Modan", "Ironforge", "Thelsamar"},
		{"Horde", "ROGUE", "Hillsbrad Foothills", "Undercity", "Tarren Mill"},
	} {
		c := character.Character{Class: tc.class, Faction: tc.side, Level: &lvl, Location: &character.Location{Zone: tc.zone}}
		pl, ok := levelup.TrainAt(c, nil)
		got := fmt.Sprintf("%s, fly from %q", pl.City, pl.From)
		ref := fmt.Sprintf("%s, fly from %q", tc.city, tc.point)
		out = append(out, benchRow{fmt.Sprintf("%s %s in %q: flight to the class trainer", tc.side, tc.class, tc.zone), got, ref, rfFlySrc,
			resultIf(ok && got == ref && strings.Contains(pl.Line(tc.class), "(fly from "+tc.point+")"), "the city or the flight point")})
	}
	finish(t, "advice-flight-points-city", "Offline flight points: more class trainer trips", out)
}

// TestOfflineBenchmarkHearthstoneCooldown: how often the hearthstone can be used, in the ways it is asked, gets the line that the dataset holds no cooldown.
func TestOfflineBenchmarkHearthstoneCooldown(t *testing.T) {
	var out []benchRow
	for _, q := range []string{
		"how often can I use my hearthstone?", "what is my hearthstone's CD?", "how soon can I hearth again?", "how long until my hearthstone is ready?",
	} {
		line, ok := rfSays(q, "Travel times:")
		out = append(out, benchRow{"\"" + q + "\"", line, "no hearthstone cooldown in the data: say so, name none", rfHearSrc, resultIf(ok, "the cooldown was left unsaid, or one was named")})
	}
	finish(t, "advice-hearth-cooldown", "Offline hearthstone cooldown: no answer in the dataset", out)
}

// TestOfflineBenchmarkHearthstoneRules: where it binds, how to change it, getting a new one and using it in combat: the dataset holds no hearthstone row.
func TestOfflineBenchmarkHearthstoneRules(t *testing.T) {
	var out []benchRow
	for _, q := range []string{
		"how do I change where my hearthstone is bound?", "where do I get a new hearthstone if I lost mine?", "can I use the hearthstone in combat?",
	} {
		line, ok := rfSays(q, "Hearthstone:")
		out = append(out, benchRow{"\"" + q + "\"", line, "no hearthstone item, bind or combat rule in the data: say so, name none", rfHearSrc, resultIf(ok, "no statement that the data holds no hearthstone rule, or a figure in it")})
	}
	ds := consStore()
	line := bdTip(ds, "Hearthstone")
	out = append(out, benchRow{"[Hearthstone]: the item row", line, "no items row named Hearthstone", rfNoneSrc,
		resultIf(strings.Contains(line, "no row for this item") && !strings.Contains(line, "requires level"), "a level or binding was given for an item the dataset does not hold")})
	finish(t, "advice-hearth-rules", "Offline hearthstone rules: no answer in the dataset", out)
}

// TestOfflineBenchmarkHearthBoundTrip: a class trainer trip says the hearthstone is bound in the city only when the capture says so, beside the flight point.
func TestOfflineBenchmarkHearthBoundTrip(t *testing.T) {
	var out []benchRow
	lvl := 30
	for _, tc := range []struct {
		side, class, zone, bind, want string
		bound                         bool
	}{
		{"Alliance", "PRIEST", "Redridge Mountains", "Stormwind City", "(fly from Lakeshire; your hearthstone is bound there)", true},
		{"Alliance", "PRIEST", "Redridge Mountains", "Goldshire", "(fly from Lakeshire)", false},
		{"Horde", "HUNTER", "Mulgore", "Thunder Bluff", "your hearthstone is bound there", true},
		{"Alliance", "MAGE", "Loch Modan", " ironforge ", "(fly from Thelsamar; your hearthstone is bound there)", true},
	} {
		c := character.Character{Class: tc.class, Faction: tc.side, Level: &lvl, Hearth: tc.bind, Location: &character.Location{Zone: tc.zone}}
		pl, _ := levelup.TrainAt(c, nil)
		line := pl.Line(tc.class)
		ok := strings.Contains(line, tc.want) && strings.Contains(line, "hearthstone") == tc.bound
		ref := "the line says: " + tc.want
		if !tc.bound {
			ref = "no hearthstone said: the bind is not in " + pl.City
		}
		out = append(out, benchRow{fmt.Sprintf("%s %s in %s, bound at %q", tc.side, tc.class, tc.zone, strings.TrimSpace(tc.bind)), line, ref, rfHearSrc, resultIf(ok, "a bind said for a city it is not in, or left out for the city it is in")})
	}
	finish(t, "advice-hearth-bound-trip", "Offline hearthstone bind on a class trainer trip", out)
}

// TestOfflineBenchmarkFixedDestinationCrossings: the fixed transport between continents (zeppelin, boat, Dark Portal) a trip names, beside the transport guides.
func TestOfflineBenchmarkFixedDestinationCrossings(t *testing.T) {
	const ek, kal, outl = "Eastern Kingdoms", "Kalimdor", "Outland"
	var out []benchRow
	for _, tc := range []struct {
		side, from, to string
		must           []string
	}{
		{"Alliance", kal, outl, []string{"Menethil Harbor", "Dark Portal", "Blasted Lands"}},
		{"Horde", ek, outl, []string{"Dark Portal", "Blasted Lands"}},
		{"Horde", outl, kal, []string{"Dark Portal", "Zeppelin", "Orgrimmar"}},
		{"Alliance", ek, kal, []string{"Boat", "Auberdine"}},
	} {
		got := learn.Crossing(tc.side, tc.from, tc.to)
		ok := got != ""
		for _, m := range tc.must {
			ok = ok && strings.Contains(got, m)
		}
		out = append(out, benchRow{fmt.Sprintf("%s: %s to %s", tc.side, tc.from, tc.to), fmt.Sprintf("%q", got), "mentions " + strings.Join(tc.must, ", "), rfCrossSrc, resultIf(ok, "the fixed route between the continents")})
	}
	finish(t, "advice-fixed-crossings", "Offline fixed-destination travel between continents", out)
}

// TestOfflineBenchmarkRidingFlightNoAnswer: asks the dataset cannot answer: a riding table, a mount trainer, a flight master's name. The advisor says so and names none.
func TestOfflineBenchmarkRidingFlightNoAnswer(t *testing.T) {
	var out []benchRow
	ds := consStore()
	for _, table := range []string{"riding_trainers", "mount_sources", "hearthstone_binds"} {
		_, err := ds.Filter(table, store.Range{Column: "id", Min: int64(0)})
		res := "match"
		if err == nil {
			res = "differ: the dataset now holds a " + table + " table: the cases here need a second look"
		}
		out = append(out, benchRow{"dataset table " + table, fmt.Sprintf("error: %v", err != nil), "absent: no row to answer from", rfNoneSrc, res})
	}
	for _, q := range []string{"which riding trainer teaches gnomes?", "how much gold does Journeyman Riding cost?"} {
		line, ok := rfSays(q, "Riding and mounts:")
		out = append(out, benchRow{"\"" + q + "\"", line, "no riding trainer or cost row: say so, name none", rfNoneSrc, resultIf(ok, "a trainer or a cost was not denied")})
	}
	pl, _ := levelup.TrainAt(character.Character{Class: "DRUID", Faction: "Horde", Location: &character.Location{Zone: "The Barrens"}}, nil)
	out = append(out, benchRow{"Horde druid in the Barrens: a flight to Thunder Bluff", pl.Line("DRUID"), "no flight point named: none is listed for Thunder Bluff", rfNoneSrc,
		resultIf(!strings.Contains(pl.Line("DRUID"), "fly from"), "a flight point was invented")})
	finish(t, "advice-riding-flight-no-answer", "Offline riding and flight advice: no answer in the dataset", out)
}

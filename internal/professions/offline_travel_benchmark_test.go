package professions

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/learn"
	"github.com/WoW-Companion/advisor/internal/levelup"
	"github.com/WoW-Companion/advisor/internal/store"
)

// The offline travel benchmark: how the advisor says to get somewhere, beside the dataset's place names and the
// zone and flight lists of the game, with no network. The dataset holds no flight path, flight master or zone
// connection table, so the continent of a zone, the crossing between continents and the flight point that reaches
// a class city are set beside the zones the dataset's npcs, trainers, instances and profession_tiers rows
// name, and the game's own zone and flight lists written out here. A case with no answer in the dataset passes
// when the advisor says so and names no place. make offline-benchmark runs it.

const (
	zonesSrc   = "dataset npcs, trainers, instances, profession_tiers (zone names); game zone list"
	flightsSrc = "game flight path list; dataset zone names"
)

// gameContinent is the continent of each zone name the dataset's rows hold, from the game's zone lists. An empty
// continent is a place no zone list holds.
var gameContinent = map[string]string{
	"Arathi Highlands": "Eastern Kingdoms", "Badlands": "Eastern Kingdoms", "Burning Steppes": "Eastern Kingdoms",
	"Dun Morogh": "Eastern Kingdoms", "Duskwood": "Eastern Kingdoms", "Eastern Plaguelands": "Eastern Kingdoms",
	"Elwynn Forest": "Eastern Kingdoms", "Hillsbrad Foothills": "Eastern Kingdoms", "Ironforge": "Eastern Kingdoms",
	"Searing Gorge": "Eastern Kingdoms", "Silverpine Forest": "Eastern Kingdoms", "Stormwind City": "Eastern Kingdoms",
	"Stranglethorn Vale": "Eastern Kingdoms", "Swamp of Sorrows": "Eastern Kingdoms", "The Hinterlands": "Eastern Kingdoms",
	"Tirisfal Glades": "Eastern Kingdoms", "Uldaman": "Eastern Kingdoms", "Undercity": "Eastern Kingdoms",
	"Western Plaguelands": "Eastern Kingdoms", "Westfall": "Eastern Kingdoms", "Eastern Kingdoms": "Eastern Kingdoms",
	"Ashenvale": "Kalimdor", "Darnassus": "Kalimdor", "Desolace": "Kalimdor", "Durotar": "Kalimdor",
	"Dustwallow Marsh": "Kalimdor", "Felwood": "Kalimdor", "Feralas": "Kalimdor", "Kalimdor": "Kalimdor",
	"Mount Hyjal": "Kalimdor", "Mulgore": "Kalimdor", "Orgrimmar": "Kalimdor", "Silithus": "Kalimdor",
	"Stonetalon Mountains": "Kalimdor", "Tanaris": "Kalimdor", "The Barrens": "Kalimdor", "Thunder Bluff": "Kalimdor",
	"Zephras Isle": "",
}

// datasetZones are the distinct zone names the snapshot's rows hold: creature and trainer zones, a dungeon's
// entrance zone, and the zone a profession tier's place ends in ("Booty Bay, Stranglethorn Vale").
func datasetZones(t *testing.T, ds store.DataStore) []string {
	t.Helper()
	set := map[string]bool{}
	add := func(table, col string, tail bool) {
		rows, err := ds.Filter(table, store.Range{Column: map[string]string{"npcs": "id", "trainers": "npc_id", "instances": "map_id", "profession_tiers": "skill_required"}[table], Min: int64(0)})
		if err != nil {
			t.Fatalf("%s: %v", table, err)
		}
		for _, r := range rows {
			z := asText(r[col])
			if tail {
				if i := strings.LastIndex(z, ", "); i >= 0 {
					z = z[i+2:]
				}
			}
			if z != "" {
				set[z] = true
			}
		}
	}
	add("npcs", "zone_name", false)
	add("trainers", "zone_name", false)
	add("instances", "entrance_zone_name", false)
	add("profession_tiers", "place", true)
	var out []string
	for z := range set {
		out = append(out, z)
	}
	sort.Strings(out)
	return out
}

// TestOfflineBenchmarkTravelZones: the continent the advisor puts each zone of the dataset on, beside the game's zone
// list. It decides whether a trainer, vendor or quest giver counts as on the player's continent, and whether
// the route adds a zeppelin or boat.
func TestOfflineBenchmarkTravelZones(t *testing.T) {
	var out []benchRow
	for _, z := range datasetZones(t, placesStore()) {
		want, known := gameContinent[z]
		if !known {
			t.Fatalf("zone %q of the snapshot is missing from gameContinent", z)
		}
		got := learn.ContinentOf(z)
		switch {
		case want == "":
			out = append(out, benchRow{"zone " + z + ": continent", fmt.Sprintf("%q", got), "no zone list holds it", zonesSrc, "differ (data): the dataset names a place the game has no zone for"})
		default:
			out = append(out, benchRow{"zone " + z + ": continent", got, want, zonesSrc, resultIf(got == want, "the continent")})
		}
	}
	if len(out) < 10 {
		t.Fatalf("only %d zones in the snapshot", len(out))
	}
	finishOffline(t, "advice-travel-zones", "Offline travel: continent of a zone", out)
}

// TestOfflineBenchmarkTravelCrossings: the zeppelin, boat or portal a route says between continents, by side.
func TestOfflineBenchmarkTravelCrossings(t *testing.T) {
	const (
		ek, kal, out0 = "Eastern Kingdoms", "Kalimdor", "Outland"
	)
	var out []benchRow
	for _, tc := range []struct {
		side, from, to string
		must           []string // each must be in the answer
		none           bool
	}{
		{"Horde", ek, kal, []string{"Zeppelin", "Orgrimmar"}, false},
		{"Horde", kal, ek, []string{"Zeppelin", "Undercity"}, false},
		{"Alliance", ek, kal, []string{"Boat", "Auberdine"}, false},
		{"Alliance", kal, ek, []string{"Boat", "Menethil Harbor"}, false},
		{"Horde", ek, ek, nil, true},
		{"Alliance", kal, kal, nil, true},
		{"Horde", ek, out0, []string{"Dark Portal", "Blasted Lands"}, false},
		{"Alliance", ek, out0, []string{"Dark Portal", "Blasted Lands"}, false},
		{"Horde", kal, out0, []string{"Zeppelin", "Undercity", "Dark Portal", "Blasted Lands"}, false},
		{"Alliance", kal, out0, []string{"Boat", "Menethil Harbor", "Dark Portal", "Blasted Lands"}, false},
		{"Horde", out0, ek, []string{"Dark Portal"}, false},
		{"Alliance", out0, kal, []string{"Dark Portal", "Boat", "Auberdine"}, false},
	} {
		got := learn.Crossing(tc.side, tc.from, tc.to)
		ref := "no crossing: the player walks or rides"
		ok := got == ""
		if !tc.none {
			ref = "mentions " + strings.Join(tc.must, ", ")
			ok = got != ""
			for _, m := range tc.must {
				ok = ok && strings.Contains(got, m)
			}
		}
		out = append(out, benchRow{fmt.Sprintf("%s: %s to %s", tc.side, tc.from, tc.to), fmt.Sprintf("%q", got), ref, flightsSrc, resultIf(ok, "the crossing the game uses")})
	}
	finishOffline(t, "advice-travel-crossings", "Offline travel: crossing between continents", out)
}

// guideFlights are the flight points with a direct flight to a class city, from the game's flight path list.
var guideFlights = []struct{ side, class, city, zone, point string }{
	{"Horde", "MAGE", "Undercity", "Hillsbrad Foothills", "Tarren Mill"}, {"Horde", "MAGE", "Undercity", "Silverpine Forest", "The Sepulcher"},
	{"Horde", "HUNTER", "Orgrimmar", "The Barrens", "the Crossroads"},
	{"Alliance", "MAGE", "Stormwind City", "Westfall", "Sentinel Hill"}, {"Alliance", "MAGE", "Stormwind City", "Redridge Mountains", "Lakeshire"},
	{"Alliance", "MAGE", "Stormwind City", "Duskwood", "Darkshire"},
	{"Alliance", "HUNTER", "Ironforge", "Loch Modan", "Thelsamar"}, {"Alliance", "HUNTER", "Ironforge", "Wetlands", "Menethil Harbor"},
	{"Horde", "PALADIN", "Silvermoon City", "Ghostlands", "Tranquillien"},
}

// guideCities are the capitals of the class trainers, by side and class, from the game.
var guideCities = []struct{ side, class, city string }{
	{"Horde", "WARRIOR", "Undercity"}, {"Horde", "MAGE", "Undercity"}, {"Horde", "HUNTER", "Orgrimmar"},
	{"Horde", "DRUID", "Thunder Bluff"}, {"Horde", "PALADIN", "Silvermoon City"},
	{"Alliance", "MAGE", "Stormwind City"}, {"Alliance", "HUNTER", "Ironforge"}, {"Alliance", "DRUID", "Darnassus"},
}

// TestOfflineBenchmarkTravelFlights: the "fly from" point the class-trainer line gives a character standing in a zone, and the
// capital a class trains in, beside the game's flight list; and that a flight master the player did not open
// or one of the other side is never named as a known flight.
func TestOfflineBenchmarkTravelFlights(t *testing.T) {
	ds := placesStore()
	zones := map[string]bool{}
	for _, z := range datasetZones(t, ds) {
		zones[z] = true
	}
	var out []benchRow
	for _, tc := range guideFlights {
		lvl := 20
		c := character.Character{Class: tc.class, Faction: tc.side, Level: &lvl, Location: &character.Location{Zone: tc.zone}}
		pl, ok := levelup.TrainAt(c, nil)
		got := fmt.Sprintf("%s, fly from %q", pl.City, pl.From)
		ref := fmt.Sprintf("%s, fly from %q", tc.city, tc.point)
		res := resultIf(ok && got == ref && strings.Contains(pl.Line(tc.class), "fly from "+tc.point) &&
			learn.ContinentOf(tc.zone) == learn.ContinentOf(tc.city), "the flight point or the continent")
		out = append(out, benchRow{fmt.Sprintf("%s %s in %s: flight to %s", tc.side, tc.class, tc.zone, tc.city), got, ref, flightsSrc, res})
		if !zones[tc.zone] || !zones[tc.city] {
			out = append(out, benchRow{fmt.Sprintf("%s to %s: zones named in the dataset", tc.zone, tc.city), "named in the advisor's list", "a zone row for both", flightsSrc, "differ (data): the snapshot's rows name no creature or trainer in one of them"})
		}
	}
	for _, tc := range guideCities {
		lvl := 20
		c := character.Character{Class: tc.class, Faction: tc.side, Level: &lvl}
		got := levelup.ClassCity(c.Faction, c.Class)
		out = append(out, benchRow{fmt.Sprintf("%s %s: capital with the class trainer", tc.side, tc.class), got, tc.city, flightsSrc, resultIf(got == tc.city, "the city")})
	}
	// flight masters the player opened: the other side's are not offered, the newest comes first
	obs := &character.Observed{Flights: []character.SeenFlight{
		{NPC: 1, Name: "Synthetic Horde Master", Map: 10, Faction: "Horde", T: 100},
		{NPC: 2, Name: "Synthetic Alliance Master", Map: 11, Faction: "Alliance", T: 300},
		{NPC: 3, Name: "Synthetic Newer Horde Master", Map: 12, Faction: "Horde", T: 200},
	}}
	got := []string{}
	for _, f := range obs.FlightsFor("Horde") {
		got = append(got, f.Name)
	}
	out = append(out, benchRow{"Horde: flight masters opened", strings.Join(got, ", "), "Synthetic Newer Horde Master, Synthetic Horde Master", "the capture's flightmasters", resultIf(strings.Join(got, ", ") == "Synthetic Newer Horde Master, Synthetic Horde Master", "the side or the order")})
	finishOffline(t, "advice-travel-flights", "Offline travel: flight points", out)
}

// TestOfflineBenchmarkTravelNoAnswer: travel questions the dataset cannot answer. The advisor says so.
func TestOfflineBenchmarkTravelNoAnswer(t *testing.T) {
	var out []benchRow
	lvl := 20
	noFlight := func(zone, side, class string) (string, bool) {
		pl, ok := levelup.TrainAt(character.Character{Class: class, Faction: side, Level: &lvl, Location: &character.Location{Zone: zone}}, nil)
		return pl.Line(class), ok && pl.From == "" && !strings.Contains(pl.Line(class), "fly from")
	}
	const none = "the dataset holds no flight path or connection for it"
	for _, tc := range []struct{ zone, side, class string }{
		{"Stranglethorn Vale", "Alliance", "MAGE"}, {"Tanaris", "Horde", "MAGE"}, {"Dun Morogh", "Alliance", "HUNTER"},
		{"The Hinterlands", "Horde", "WARRIOR"}, {"Elwynn Forest", "Alliance", "DRUID"},
	} {
		line, ok := noFlight(tc.zone, tc.side, tc.class)
		out = append(out, benchRow{fmt.Sprintf("%s %s in %s: which flight point", tc.side, tc.class, tc.zone), line, none, flightsSrc, resultIf(ok, "a flight point was named")})
	}
	if from := levelup.FlightFrom("Stormwind City", "Stranglethorn Vale"); from != "" {
		out = append(out, benchRow{"flight point to Stormwind City from Stranglethorn Vale", from, none, flightsSrc, "differ: a flight point was invented"})
	} else {
		out = append(out, benchRow{"flight point to Stormwind City from Stranglethorn Vale", "none", none, flightsSrc, "match"})
	}
	c := learn.Crossing("Horde", "", "Kalimdor")
	out = append(out, benchRow{"crossing from a zone with no known continent", fmt.Sprintf("%q", c), none, zonesSrc, resultIf(c == "", "a crossing was invented")})
	c = learn.Crossing("", "Eastern Kingdoms", "Kalimdor")
	out = append(out, benchRow{"crossing for a player with no known side", fmt.Sprintf("%q", c), "no side: no zeppelin or boat chosen", zonesSrc, resultIf(c == "", "a side was assumed")})
	var none2 *character.Observed
	out = append(out, benchRow{"flight masters the player opened, with no capture", fmt.Sprintf("%d", len(none2.FlightsFor("Horde"))), "0: no flight is suggested", "the capture's flightmasters", resultIf(len(none2.FlightsFor("Horde")) == 0, "a flight was suggested")})
	out = append(out, benchRow{"capital of a class for a side that cannot play it", fmt.Sprintf("%q", levelup.ClassCity("Neutral", "MAGE")), "none", flightsSrc, resultIf(levelup.ClassCity("Neutral", "MAGE") == "", "a city was named")})
	finishOffline(t, "advice-travel-no-answer", "Offline travel advice, no answer in the dataset", out)
}

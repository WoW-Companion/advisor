package paths_test

import (
	"cmp"
	"fmt"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/ask"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/learn"
	"github.com/WoW-Companion/advisor/internal/questie"
	"github.com/WoW-Companion/advisor/internal/route"
	"github.com/WoW-Companion/advisor/internal/store"
)

// The offline flight path, boat and zeppelin benchmark: which flight masters a route flies to, which boats, zeppelins and orbs it names between
// zones and for which faction, what it says when a flight path is not yet discovered, and what it says when the dataset has no answer, beside the
// game's own transport network, with no network. The dataset holds ui_maps (a zone's parent continent) and no flight path, flight master, boat,
// zeppelin or tram row, so a route is set beside the game's transport guides (Warcraft Wiki: Flight path, Boat, Zeppelin, Deeprun Tram, Orb of
// Translocation; Wowhead Classic transport guides), the dataset value being the continent of each end, and the cases the dataset cannot answer check
// that the advisor says so (ask.LimitFacts) and names nothing. A zone the snapshot's ui_maps lacks (Azuremyst Isle, the Exodar, Silvermoon City,
// Hellfire Peninsula) is a "(data)" row: the advisor's answer is the game's, the dataset has no row to show it. make offline-benchmark runs it;
// WOWC_BENCH_REPORT receives the tables.

const (
	fbFlightSrc  = "game: Warcraft Wiki, Flight path (a node is flown to only once discovered, only the player's side and neutral masters; no flight crosses the sea); dataset ui_maps (continent of each end), no flight path table"
	fbBoatSrc    = "game: Warcraft Wiki boat and zeppelin routes (Ratchet-Booty Bay both sides; Auberdine-Menethil Harbor, -Rut'theran Village, -Valaar's Berth Alliance; Orgrimmar, Undercity, Grom'gol Horde); Orb of Translocation (Undercity-Silvermoon City, Horde); dataset ui_maps"
	fbSideSrc    = "game: Warcraft Wiki transport pages (a faction's boats, zeppelins and orb serve its own side only; Ratchet-Booty Bay both); dataset ui_maps, no transport table"
	fbNoDataSrc  = "the dataset has no flight path, flight master, boat, zeppelin or tram row (docs/schema.md of the dataset lists no such table)"
	fbDataGapSrc = "dataset ui_maps snapshot (testdata/world_snapshot) has no row for the zone; game: the zone is on Kalimdor, the Eastern Kingdoms or Outland"
)

// fbZones are the zones of the cases with a map id of the synthetic questie index (the ids are the cases' own).
var fbZones = map[string]int{
	"Stormwind City": 1453, "Darkshore": 1439, "Wetlands": 1437, "Hillsbrad Foothills": 1424, "Azuremyst Isle": 1943, "The Exodar": 1947,
	"Silvermoon City": 1954, "Undercity": 1458, "Orgrimmar": 1454, "The Barrens": 1413, "Durotar": 1411, "Thunder Bluff": 1456,
	"Stranglethorn Vale": 1434, "Hellfire Peninsula": 1944, "Duskwood": 1431, "Silverpine Forest": 1421, "Teldrassil": 1438,
}

func fbNames() map[int]string {
	out := map[int]string{}
	for z, id := range fbZones {
		out[id] = z
	}
	return out
}

// fbDataset is the dataset's value for the ends of a trip: the parent continent of each zone's ui_maps row, or that it has none.
func fbDataset(zones map[string]string, ends ...string) string {
	var parts []string
	for _, z := range ends {
		c := zones[z]
		if c == "" {
			c = "no ui_maps row"
		}
		parts = append(parts, z+": "+c)
	}
	return "ui_maps " + strings.Join(parts, "; ")
}

// fbTravel is the zone step of a route for a character of the side (or the race, when the side is empty) standing on map from with a quest on map to,
// with the flight masters the character opened; flights are opened on the map their Map names.
func fbTravel(t *testing.T, side, race string, from, to string, flights ...character.SeenFlight) route.ZonePlan {
	t.Helper()
	fm, tm := fbZones[from], fbZones[to]
	at := func(area int, x, y float64) []questie.Spawn { return []questie.Spawn{{Area: area, X: x, Y: y}} }
	qx := &questie.Index{Quests: map[int]*questie.Quest{}, Objects: map[int]questie.Entity{}, Areas: map[int]questie.Area{},
		Npcs: map[int]questie.Entity{10: {Name: "Synthetic Giver", Spawns: at(fm, 50, 50)}, 11: {Name: "Synthetic Boar", Spawns: at(tm, 40, 40)}, 12: {Name: "Synthetic Finisher", Spawns: at(tm, 42, 44)}}}
	for id, z := range fbNames() {
		qx.Areas[id] = questie.Area{UIMap: id, Zone: z}
	}
	qx.Quests[900] = &questie.Quest{ID: 900, Name: "Synthetic Errand", Level: 9, Finishers: []questie.Ref{{ID: 12}}, Creatures: []questie.Ref{{ID: 11}}}
	lvl, x, y := 9, 50.0, 50.0
	c := character.Character{Name: "Benchtest", Faction: side, Race: race, QuestsCaptured: true, Level: &lvl,
		Location: &character.Location{MapID: &fm, X: &x, Y: &y},
		Quests:   []character.Quest{{QuestID: 900, Title: "Synthetic Errand", Objectives: []character.Objective{{Text: "Synthetic Boar slain: 0/1"}}}}}
	if len(flights) > 0 {
		c.Seen = &character.Observed{Flights: flights}
		for _, f := range flights {
			qx.Npcs[int(f.NPC)] = questie.Entity{Name: f.Name, Spawns: at(f.Map, f.X, f.Y)}
		}
	}
	for _, z := range route.Build(c, qx).Zones {
		if z.Map == tm {
			return z
		}
	}
	return route.ZonePlan{}
}

// fbMaster is a flight master opened in the named zone.
func fbMaster(npc int64, zone, side string) character.SeenFlight {
	return character.SeenFlight{NPC: npc, Name: fmt.Sprintf("Synthetic Master %d", npc), Map: fbZones[zone], X: 61, Y: 19, Faction: side, T: 1}
}

type fbCase struct {
	check, side, race, from, to string
	flights                     []character.SeenFlight
	want                        string   // the whole step, when the game's route is one wording
	never                       []string // wordings the route must not hold
	prefix                      string   // the step must not start with this ("Fly")
}

func fbRows(t *testing.T, cases []fbCase, src string) []benchRow {
	t.Helper()
	zones := worldZones(t)
	var out []benchRow
	for _, tc := range cases {
		z := fbTravel(t, tc.side, tc.race, tc.from, tc.to, tc.flights...)
		got := z.Travel
		ok := got != ""
		ref := tc.want
		if tc.want != "" {
			ok = ok && got == tc.want
		}
		for _, n := range tc.never {
			ok = ok && !strings.Contains(got, n)
		}
		if len(tc.never) > 0 {
			ref = strings.TrimSuffix(ref+"; never "+strings.Join(tc.never, ", "), "; ")
			ref = strings.TrimPrefix(ref, "; ")
		}
		if tc.prefix != "" {
			ok = ok && !strings.HasPrefix(got, tc.prefix)
			ref += " (no step that starts with \"" + tc.prefix + "\")"
		}
		out = append(out, benchRow{tc.check, got, strings.TrimSpace(ref) + "; " + fbDataset(zones, tc.from, tc.to), src, resultIf(ok, "the route step")})
	}
	return out
}

// ---- flight path connections ----

// TestOfflineBenchmarkFlightConnections: a flight is named only to a flight master the player opened, on the same piece of land, of the player's own side or neutral.
func TestOfflineBenchmarkFlightConnections(t *testing.T) {
	m := fbMaster
	out := fbRows(t, []fbCase{
		{check: "Horde, Durotar to Thunder Bluff, its flight master opened", side: "Horde", from: "Durotar", to: "Thunder Bluff", flights: []character.SeenFlight{m(60, "Thunder Bluff", "Horde")}, want: "Fly to Synthetic Master 60, Thunder Bluff"},
		{check: "Alliance, Stormwind City to Wetlands, Menethil Harbor opened", side: "Alliance", from: "Stormwind City", to: "Wetlands", flights: []character.SeenFlight{m(61, "Wetlands", "Alliance")}, want: "Fly to Synthetic Master 61, Wetlands"},
		{check: "Alliance, Stormwind City to Duskwood, Darkshire opened", side: "Alliance", from: "Stormwind City", to: "Duskwood", flights: []character.SeenFlight{m(62, "Duskwood", "Alliance")}, want: "Fly to Synthetic Master 62, Duskwood"},
		{check: "Horde, Undercity to Silverpine Forest, The Sepulcher opened", side: "Horde", from: "Undercity", to: "Silverpine Forest", flights: []character.SeenFlight{m(63, "Silverpine Forest", "Horde")}, want: "Fly to Synthetic Master 63, Silverpine Forest"},
		{check: "Horde, Undercity to Stranglethorn Vale, a neutral master opened", side: "Horde", from: "Undercity", to: "Stranglethorn Vale", flights: []character.SeenFlight{m(64, "Stranglethorn Vale", "")}, want: "Fly to Synthetic Master 64, Stranglethorn Vale"},
		{check: "Alliance, Stormwind City to Stranglethorn Vale, the same neutral master opened", side: "Alliance", from: "Stormwind City", to: "Stranglethorn Vale", flights: []character.SeenFlight{m(64, "Stranglethorn Vale", "")}, want: "Fly to Synthetic Master 64, Stranglethorn Vale"},
		{check: "Horde, Orgrimmar to Hillsbrad Foothills, its master opened: the zeppelin first", side: "Horde", from: "Orgrimmar", to: "Hillsbrad Foothills", flights: []character.SeenFlight{m(65, "Hillsbrad Foothills", "Horde")}, want: "Zeppelin to Undercity, then fly to Synthetic Master 65"},
	}, fbFlightSrc)
	finish(t, "advice-flight-connections", "Offline flight path connections: the flight master a route flies to", out)
}

// ---- boats, zeppelins and orbs ----

// TestOfflineBenchmarkBoatZeppelinRoutes: the boat, zeppelin and orb steps between zones, each leg named, none that does not run for the side.
func TestOfflineBenchmarkBoatZeppelinRoutes(t *testing.T) {
	out := fbRows(t, []fbCase{
		{check: "Alliance, Stormwind City to The Exodar: two boats", side: "Alliance", from: "Stormwind City", to: "The Exodar", want: "Boat to Auberdine, then boat to Valaar's Berth, then walk or ride to The Exodar"},
		{check: "Alliance, The Exodar to Stormwind City: two boats back", side: "Alliance", from: "The Exodar", to: "Stormwind City", want: "Boat to Auberdine, then boat to Menethil Harbor, then walk or ride to Stormwind City"},
		{check: "Alliance, Darkshore to Azuremyst Isle: the boat to Valaar's Berth", side: "Alliance", from: "Darkshore", to: "Azuremyst Isle", want: "Boat to Valaar's Berth"},
		{check: "Horde, Undercity to Silvermoon City: the Orb of Translocation", side: "Horde", from: "Undercity", to: "Silvermoon City", want: "Orb of Translocation to Silvermoon City"},
		{check: "Horde, The Barrens to Stranglethorn Vale: the boat both sides share", side: "Horde", from: "The Barrens", to: "Stranglethorn Vale", want: "Boat to Booty Bay"},
		{check: "Alliance, The Barrens to Stranglethorn Vale: the same boat", side: "Alliance", from: "The Barrens", to: "Stranglethorn Vale", want: "Boat to Booty Bay"},
		{check: "Alliance, The Barrens to Hellfire Peninsula: the boat out of the Barrens, then the Dark Portal", side: "Alliance", from: "The Barrens", to: "Hellfire Peninsula", want: "Boat to Booty Bay, then the Dark Portal in the Blasted Lands, then walk or ride to Hellfire Peninsula", never: []string{"Menethil"}},
		{check: "Horde, Hellfire Peninsula to The Barrens: the Dark Portal, then the boat that lands in the Barrens", side: "Horde", from: "Hellfire Peninsula", to: "The Barrens", want: "Step through the Dark Portal in Hellfire Peninsula, then boat to Ratchet"},
	}, fbBoatSrc)
	// trip level: the whole way, each leg as the transport names it
	for _, tc := range []struct{ side, from, to, want string }{
		{"Horde", "Orgrimmar", "Silvermoon City", "Zeppelin from Orgrimmar to Undercity, then Orb of Translocation from Undercity to Silvermoon City"},
		{"Horde", "Silvermoon City", "Orgrimmar", "Orb of Translocation from Silvermoon City to Undercity, then zeppelin from Undercity to Orgrimmar"},
	} {
		p := trTrip(tc.side, tc.from, tc.to)
		got := "(no trip)"
		if len(p.Trips) == 1 {
			got = p.Trips[0].Travel
		}
		out = append(out, benchRow{fmt.Sprintf("%s, %s to %s: every leg", tc.side, tc.from, tc.to), got, strings.TrimSpace(tc.want) + ": turn in Synthetic Errand", fbBoatSrc, resultIf(strings.HasPrefix(got, tc.want), "a leg")})
	}
	finish(t, "advice-boat-zeppelin-routes", "Offline boat, zeppelin and orb routes", out)
}

// ---- faction restrictions ----

// TestOfflineBenchmarkTransportFactionRestrictions: a boat, zeppelin or orb of the other side is never offered; a side that is not known is offered only what both sides ride.
func TestOfflineBenchmarkTransportFactionRestrictions(t *testing.T) {
	out := fbRows(t, []fbCase{
		{check: "Horde, Darkshore to Azuremyst Isle: no Alliance boat", side: "Horde", from: "Darkshore", to: "Azuremyst Isle", never: []string{"Valaar", "Boat"}},
		{check: "Alliance, Undercity to Silvermoon City: no Horde orb", side: "Alliance", from: "Undercity", to: "Silvermoon City", never: []string{"Orb"}},
		{check: "Alliance, Orgrimmar to Undercity: no Horde zeppelin", side: "Alliance", from: "Orgrimmar", to: "Undercity", never: []string{"Zeppelin"}},
		{check: "Horde, Darkshore to Teldrassil: no Rut'theran boat", side: "Horde", from: "Darkshore", to: "Teldrassil", never: []string{"Rut'theran", "Auberdine to"}},
		{check: "side not known, Orgrimmar to Undercity: no zeppelin", from: "Orgrimmar", to: "Undercity", never: []string{"Zeppelin", "Boat from Auberdine"}},
		{check: "side not known, Undercity to Silvermoon City: no orb", from: "Undercity", to: "Silvermoon City", never: []string{"Orb"}},
		{check: "side not known, Darkshore to Azuremyst Isle: no boat", from: "Darkshore", to: "Azuremyst Isle", never: []string{"Valaar", "Boat"}},
		{check: "an orc with no faction captured: the Horde's orb", race: "Orc", from: "Undercity", to: "Silvermoon City", want: "Orb of Translocation to Silvermoon City"},
		{check: "a night elf with no faction captured: the Alliance's boat", race: "NightElf", from: "Darkshore", to: "Azuremyst Isle", want: "Boat to Valaar's Berth"},
	}, fbSideSrc)
	// trip level, the same graph
	for _, tc := range []struct {
		side, from, to string
		never          []string
	}{
		{"Alliance", "Undercity", "Silvermoon City", []string{"Orb"}},
		{"Horde", "Wetlands", "Dustwallow Marsh", []string{"Theramore", "Menethil Harbor to"}},
		{"Horde", "Darkshore", "Azuremyst Isle", []string{"Valaar"}},
		{"", "Orgrimmar", "Stormwind City", []string{"Zeppelin from Orgrimmar to Grom'gol", "Zeppelin from Orgrimmar to Undercity"}},
		{"", "Wetlands", "Dustwallow Marsh", []string{"Theramore", "Menethil Harbor to"}},
	} {
		p := trTrip(tc.side, tc.from, tc.to)
		got, ok := "(no trip)", len(p.Trips) == 1
		if ok {
			got = p.Trips[0].Travel
		}
		for _, n := range tc.never {
			ok = ok && !strings.Contains(got, n)
		}
		out = append(out, benchRow{fmt.Sprintf("%s, %s to %s: the trip's whole way", cmp.Or(tc.side, "side not known"), tc.from, tc.to), got, "never " + strings.Join(tc.never, ", "), fbSideSrc, resultIf(ok, "a transport of the other side")})
	}
	finish(t, "advice-transport-faction", "Offline transport faction restrictions", out)
}

// ---- flight paths to discover first ----

// TestOfflineBenchmarkFlightDiscoverFirst: no flight is named to a master the player has not opened, to one of the other side, to one in another zone or across the sea.
func TestOfflineBenchmarkFlightDiscoverFirst(t *testing.T) {
	m := fbMaster
	out := fbRows(t, []fbCase{
		{check: "Horde, Durotar to Thunder Bluff, no master opened: the road", side: "Horde", from: "Durotar", to: "Thunder Bluff", want: "Walk or ride to Thunder Bluff"},
		{check: "Alliance, Stormwind City to Wetlands, no master opened: the road", side: "Alliance", from: "Stormwind City", to: "Wetlands", want: "Walk or ride to Wetlands"},
		{check: "Alliance, a master opened in Hillsbrad Foothills is no flight into Wetlands", side: "Alliance", from: "Stormwind City", to: "Wetlands", flights: []character.SeenFlight{m(70, "Hillsbrad Foothills", "Alliance")}, want: "Walk or ride to Wetlands"},
		{check: "Horde, an Alliance master opened in Wetlands: no flight", side: "Horde", from: "Undercity", to: "Wetlands", flights: []character.SeenFlight{m(71, "Wetlands", "Alliance")}, prefix: "Fly", never: []string{"Synthetic Master"}},
		{check: "an orc with no faction captured, an Alliance master opened: no flight", race: "Orc", from: "Orgrimmar", to: "Hillsbrad Foothills", flights: []character.SeenFlight{m(72, "Hillsbrad Foothills", "Alliance")}, never: []string{"Synthetic Master"}},
		{check: "Horde, Undercity to the Barrens, the Barrens master opened: the sea first, never a bare flight", side: "Horde", from: "Undercity", to: "The Barrens", flights: []character.SeenFlight{m(73, "The Barrens", "Horde")}, prefix: "Fly"},
		{check: "Alliance, Darkshore to Azuremyst Isle, its master opened: the boat first, never a flight over the sea", side: "Alliance", from: "Darkshore", to: "Azuremyst Isle", flights: []character.SeenFlight{m(74, "Azuremyst Isle", "Alliance")}, prefix: "Fly", want: "Boat to Valaar's Berth, then fly to Synthetic Master 74"},
	}, fbFlightSrc)
	finish(t, "advice-flight-discover-first", "Offline flight paths to discover first", out)
}

// ---- no answer in the dataset ----

// TestOfflineBenchmarkFlightTransportNoAnswer: questions on flight paths, boats and zeppelins the dataset cannot answer get the line that says so, and a route question gets none.
func TestOfflineBenchmarkFlightTransportNoAnswer(t *testing.T) {
	var out []benchRow
	for _, tc := range []struct{ q, want string }{
		{"which flight paths connect to Stormwind?", "Flight paths:"},
		{"what order should I discover the flight paths in Kalimdor?", "Flight paths:"},
		{"what faction can use the Menethil Harbor flight master?", "Flight paths:"},
		{"can the Horde take the Auberdine boat?", "Boats and zeppelins:"},
		{"which boat goes to Azuremyst Isle?", "Boats and zeppelins:"},
		{"which zeppelins can the Horde take?", "Boats and zeppelins:"},
		{"does the Alliance have a zeppelin?", "Boats and zeppelins:"},
		{"where does the boat from Ratchet go?", "Boats and zeppelins:"},
	} {
		f := ask.LimitFacts(tc.q)
		ok := strings.Contains(f, tc.want) && strings.Contains(f, "Say so")
		out = append(out, benchRow{"\"" + tc.q + "\"", firstLine(f), "the data holds none: say so, name no node, port, route or faction from memory", fbNoDataSrc, resultIf(ok, "no line that the data holds none")})
	}
	for _, q := range []string{"how do I get to Silvermoon from Undercity?", "where do I take the zeppelin to Undercity?"} {
		f := ask.LimitFacts(q)
		out = append(out, benchRow{"\"" + q + "\"", fmt.Sprintf("limit fact: %q", firstLine(f)), "no limit fact: a route question is answered from the plan's route step", fbNoDataSrc, resultIf(f == "", "a route question was told the data holds none")})
	}
	ds := worldStore()
	for _, table := range []string{"flight_masters", "boats", "zeppelins", "transports"} {
		_, err := ds.Filter(table, store.Range{Column: "id", Min: int64(0)})
		res := "match"
		if err == nil {
			res = "differ: the dataset now holds a " + table + " table: the cases here need a second look"
		}
		out = append(out, benchRow{"dataset table " + table, fmt.Sprintf("error: %v", err != nil), "absent: no transport row to answer from", fbNoDataSrc, res})
	}
	finish(t, "advice-flight-transport-no-answer", "Offline flight path and transport advice, no answer in the dataset", out)
}

// ---- zones the dataset snapshot lacks ----

// TestOfflineBenchmarkTransportDataGaps: the zones the transport routes name that the snapshot's ui_maps has no row for. The advisor knows their continent from its own list.
func TestOfflineBenchmarkTransportDataGaps(t *testing.T) {
	zones := worldZones(t)
	var out []benchRow
	for _, z := range []struct{ name, want string }{
		{"Azuremyst Isle", "Kalimdor"}, {"The Exodar", "Kalimdor"}, {"Silvermoon City", "Eastern Kingdoms"}, {"Hellfire Peninsula", "Outland"},
	} {
		got := learn.ContinentOf(z.name)
		res := resultIf(got == z.want, "the advisor's continent")
		if got == z.want && zones[z.name] == "" {
			res = "differ (data): ui_maps has no row for " + z.name
		}
		out = append(out, benchRow{z.name + ": continent", got, "game: " + z.want + "; dataset: no ui_maps row", fbDataGapSrc, res})
	}
	finish(t, "advice-transport-data-gaps", "Offline transport zones the dataset snapshot lacks", out)
}

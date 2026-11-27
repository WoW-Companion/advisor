package paths_test

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/WoW-Companion/advisor/internal/ask"
	"github.com/WoW-Companion/advisor/internal/brief"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/learn"
	"github.com/WoW-Companion/advisor/internal/levelup"
	"github.com/WoW-Companion/advisor/internal/questie"
	"github.com/WoW-Companion/advisor/internal/route"
)

// The offline travel-route benchmark: the boat, zeppelin, tram and portal legs the route planner words between two zones, the
// hearthstone bind the class-trainer line mentions, and the travel questions the dataset cannot answer, beside the game's own
// transport network, with no network. The dataset holds ui_maps (a zone's parent continent) and no transport, flight, innkeeper,
// bind or schedule table, so each route is set beside the game's transport guides, the dataset value being the continent
// of each end; a difference only the dataset's gap causes is marked "(data)". make offline-benchmark runs it; WOWC_BENCH_REPORT
// receives the tables.

const (
	trZepSrc    = "game: Wowhead Classic / Warcraft Wiki zeppelin routes (Orgrimmar, Undercity and Grom'gol Base Camp); dataset ui_maps (continent of each end)"
	trBoatSrc   = "game: Warcraft Wiki boat routes (Ratchet-Booty Bay, Menethil Harbor-Auberdine and Theramore Isle, Auberdine-Rut'theran Village); dataset ui_maps"
	trCitySrc   = "game: Warcraft Wiki Deeprun Tram, Dark Portal and capital-city transport pages; dataset ui_maps"
	trBindSrc   = "the capture's hearth bind (GetBindLocation); game: a hearthstone returns to its bind; the dataset has no bind or innkeeper row"
	trTimesNone = "the dataset has no hearthstone cooldown, boat or zeppelin schedule, flight fare or flight time column or table"
)

var travelDigit = regexp.MustCompile(`\d`)

// trTrip is the plan of a level 30 character of side standing in zone from, with one ready turn-in in zone to.
func trTrip(side, from, to string) route.Plan {
	qx := &questie.Index{Quests: map[int]*questie.Quest{}, Npcs: map[int]questie.Entity{}, Objects: map[int]questie.Entity{},
		Areas: map[int]questie.Area{900: {UIMap: 5001, Zone: from}, 901: {UIMap: 5002, Zone: to}}}
	qx.Npcs[100] = questie.Entity{Name: "Synthetic Clerk", Spawns: []questie.Spawn{{Area: 901, X: 50, Y: 50}}}
	qx.Quests[100] = &questie.Quest{ID: 100, Name: "Synthetic Errand", Level: 30, Finishers: []questie.Ref{{ID: 100}}}
	lvl, mp, x, y, yes := 30, 5001, 50.0, 50.0, true
	c := character.Character{Name: "Benchtest", Faction: side, Class: "ROGUE", QuestsCaptured: true, Level: &lvl,
		Location: &character.Location{Zone: from, MapID: &mp, X: &x, Y: &y},
		Quests:   []character.Quest{{QuestID: 100, Title: "Synthetic Errand", Complete: &yes}}}
	return route.Build(c, qx)
}

type trCase struct {
	side, from, to string
	must, never    []string
}

// trRows runs each case through the planner and sets the travel step beside the game's route.
func trRows(t *testing.T, cases []trCase, src, bad string) []benchRow {
	t.Helper()
	zones := worldZones(t)
	cont := func(z string) string {
		if c := zones[z]; c != "" {
			return c
		}
		return "no ui_maps row"
	}
	var out []benchRow
	for _, tc := range cases {
		p := trTrip(tc.side, tc.from, tc.to)
		got := "(no trip)"
		ok := len(p.Trips) == 1
		if ok {
			got = p.Trips[0].Travel
			ok = p.Trips[0].ETA > 0
		}
		for _, m := range tc.must {
			ok = ok && strings.Contains(got, m)
		}
		for _, m := range tc.never {
			ok = ok && !strings.Contains(got, m)
		}
		ref := "no other side's transport"
		if len(tc.must) > 0 {
			ref = "mentions " + strings.Join(tc.must, "; ")
		}
		if len(tc.never) > 0 {
			ref += "; never " + strings.Join(tc.never, ", ")
		}
		out = append(out, benchRow{fmt.Sprintf("%s: %s (%s) to %s (%s)", tc.side, tc.from, cont(tc.from), tc.to, cont(tc.to)), got, ref, src, resultIf(ok, bad)})
	}
	return out
}

// TestOfflineBenchmarkTravelBoatsAndZeppelins: the zeppelin or boat leg the route names between two zones, for each of the game's
// routes the planner knows, with the side that may use it.
func TestOfflineBenchmarkTravelBoatsAndZeppelins(t *testing.T) {
	out := trRows(t, []trCase{
		{"Horde", "Undercity", "Stranglethorn Vale", []string{"Zeppelin from Undercity to Grom'gol", "ride to Stranglethorn Vale"}, []string{"Boat"}},
		{"Horde", "Orgrimmar", "Stranglethorn Vale", []string{"Zeppelin from Orgrimmar to Grom'gol"}, []string{"Boat"}},
		{"Horde", "Stranglethorn Vale", "Orgrimmar", []string{"Zeppelin from Grom'gol to Orgrimmar"}, []string{"Boat"}},
		{"Horde", "Stranglethorn Vale", "Tirisfal Glades", []string{"Zeppelin from Grom'gol to Undercity", "Tirisfal Glades"}, []string{"Boat"}},
		{"Horde", "The Barrens", "Stranglethorn Vale", []string{"Boat from Ratchet to Booty Bay"}, []string{"Zeppelin"}},
		{"Alliance", "Stranglethorn Vale", "The Barrens", []string{"Boat from Booty Bay to Ratchet"}, []string{"Zeppelin"}},
		{"Alliance", "Wetlands", "Dustwallow Marsh", []string{"Boat from Menethil Harbor to Theramore Isle"}, []string{"Zeppelin"}},
		{"Alliance", "Teldrassil", "Wetlands", []string{"Boat from Rut'theran Village to Auberdine", "boat from Auberdine to Menethil Harbor"}, []string{"Zeppelin"}},
		{"Alliance", "Wetlands", "Teldrassil", []string{"Boat from Menethil Harbor to Auberdine", "boat from Auberdine to Rut'theran Village"}, []string{"Zeppelin"}},
		{"Horde", "Wetlands", "Dustwallow Marsh", []string{"zeppelin from Undercity to Grom'gol"}, []string{"Theramore", "Menethil Harbor to"}},
		{"Horde", "Darkshore", "Wetlands", nil, []string{"Menethil Harbor", "Auberdine to"}},
	}, trBoatSrc+"; "+trZepSrc, "the boat or zeppelin, or one of the other side's")
	finish(t, "advice-travel-boats-zeppelins", "Offline travel routes: boats and zeppelins", out)
}

// TestOfflineBenchmarkTravelCityLinks: the tram, zeppelin and portal that join two cities or continents, worded as the game names
// them, and never a flight across the sea.
func TestOfflineBenchmarkTravelCityLinks(t *testing.T) {
	out := trRows(t, []trCase{
		{"Alliance", "Ironforge", "Stormwind City", []string{"Deeprun Tram from Ironforge to Stormwind"}, []string{"deeprun", "Fly"}},
		{"Alliance", "Darnassus", "Stormwind City", []string{"then Deeprun Tram from Ironforge to Stormwind", "boat from Auberdine to Menethil Harbor"}, []string{"deeprun", "Fly"}},
		{"Alliance", "Stormwind City", "Darnassus", []string{"Deeprun Tram from Stormwind to Ironforge", "boat from Auberdine to Rut'theran Village"}, []string{"deeprun", "Fly"}},
		{"Horde", "Orgrimmar", "Undercity", []string{"Zeppelin"}, []string{"Fly", "Boat"}},
		{"Horde", "Thunder Bluff", "Undercity", []string{"zeppelin from Grom'gol to Undercity"}, []string{"Fly"}},
		{"Alliance", "Stormwind City", "Hellfire Peninsula", []string{"Dark Portal in the Blasted Lands", "step through"}, []string{"Fly"}},
		{"Horde", "Undercity", "Hellfire Peninsula", []string{"Dark Portal in the Blasted Lands", "step through"}, []string{"Fly"}},
		{"Alliance", "Hellfire Peninsula", "Stormwind City", []string{"Step through the Dark Portal in Hellfire Peninsula"}, []string{"Fly"}},
	}, trCitySrc, "the tram, zeppelin or portal wording, or a flight across the sea")
	finish(t, "advice-travel-city-links", "Offline travel routes: cities, the Deeprun Tram and the Dark Portal", out)
}

// TestOfflineBenchmarkTravelHearthBindLines: the hearthstone mention in the class-trainer line, for classes, zones and binds beside
// the existing hearth cases: it names the bind only when the bind is in the class city, and a bind that is not a city name
// (an inn) is never taken for one.
func TestOfflineBenchmarkTravelHearthBindLines(t *testing.T) {
	lvl := 30
	line := func(side, class, zone, bind string) string {
		pl, _ := levelup.TrainAt(character.Character{Class: class, Faction: side, Level: &lvl, Hearth: bind, Location: &character.Location{Zone: zone}}, nil)
		return pl.Line(class)
	}
	var out []benchRow
	add := func(check, got, ref string, ok bool) {
		out = append(out, benchRow{check, got, ref, trBindSrc, resultIf(ok, "the bind was claimed in the wrong place, or left unsaid")})
	}
	l := line("Horde", "WARRIOR", "Silverpine Forest", "Undercity")
	add("Horde warrior in Silverpine Forest, bound at Undercity", l, "fly from The Sepulcher, and the bind in Undercity", strings.Contains(l, "(fly from The Sepulcher; your hearthstone is bound there)"))
	l = line("Alliance", "PRIEST", "Elwynn Forest", "Goldshire")
	add("Alliance priest in Elwynn Forest, bound at the Goldshire inn", l, "no hearthstone in the line: an inn is not the class city", !strings.Contains(l, "hearth"))
	l = line("Horde", "WARLOCK", "Tirisfal Glades", "Stormwind City")
	add("Horde warlock bound in the Alliance's Stormwind City", l, "no hearthstone in the line: the class city is Undercity", !strings.Contains(l, "hearth"))
	l = line("Alliance", "MAGE", "Wetlands", "Ironforge")
	add("Alliance mage in Wetlands, bound at Ironforge", l, "fly from Menethil Harbor, and the bind in Ironforge", strings.Contains(l, "(fly from Menethil Harbor; your hearthstone is bound there)"))
	b := brief.Character(character.Character{Hearth: "   "}, character.Client{}, time.Unix(1700100000, 0))
	add("a hearth bind of only spaces: the brief", fmt.Sprintf("hearthstone line: %v", strings.Contains(b, "Hearthstone")), "hearthstone line: false", !strings.Contains(b, "Hearthstone"))
	finish(t, "advice-travel-hearth-lines", "Offline travel: hearthstone bind in the class-trainer line", out)
}

// TestOfflineBenchmarkTravelNoAnswer: travel questions and places the dataset cannot answer. The advisor says so and states no time,
// fare, cooldown or route.
func TestOfflineBenchmarkTravelNoAnswer(t *testing.T) {
	var out []benchRow
	for _, q := range []string{
		"what is the hearthstone cooldown?",
		"how often does the zeppelin from Undercity to Orgrimmar run?",
		"what is the boat schedule from Menethil Harbor to Auberdine?",
		"how much does the flight path from Tarren Mill cost?",
	} {
		f := ask.LimitFacts(q)
		ok := strings.Contains(f, "Travel times: the game data holds no hearthstone cooldown") && strings.Contains(f, "Say so") && !travelDigit.MatchString(f)
		out = append(out, benchRow{"\"" + q + "\"", firstLine(f), "the data holds none: say so, give no cooldown, schedule, fare or time", trTimesNone, resultIf(ok, "no statement that the data holds none, or a figure in it")})
	}
	for _, q := range []string{"where do I take the zeppelin to Undercity?", "which flight path should I take to Tarren Mill?"} {
		f := ask.LimitFacts(q)
		out = append(out, benchRow{"\"" + q + "\"", fmt.Sprintf("travel-times fact: %v", strings.Contains(f, "Travel times:")), "no limit fact: the route is answerable from the travel list", trTimesNone, resultIf(!strings.Contains(f, "Travel times:"), "a route question was told the data holds none")})
	}
	for _, tc := range []struct{ side, from, to string }{{"Horde", "Orgrimmar", "Kul Tiras"}, {"Alliance", "Stormwind City", "Zul'Gurub"}} {
		p := trTrip(tc.side, tc.from, tc.to)
		got := "(no trip)"
		if len(p.Trips) == 1 {
			got = p.Trips[0].Travel
		}
		bad := strings.Contains(got, "Zeppelin") || strings.Contains(got, "oat from") || strings.Contains(got, "Portal") || strings.Contains(got, "Tram")
		out = append(out, benchRow{fmt.Sprintf("%s: %s to %s, a zone no list holds", tc.side, tc.from, tc.to), got, "no zeppelin, boat, tram or portal named: the zone has no known route", trTimesNone, resultIf(!bad && got != "(no trip)", "a transport was invented")})
	}
	for _, tc := range []struct{ side, from, to string }{{"Horde", "Zul'Gurub", "Durotar"}, {"Alliance", "Elwynn Forest", "Kul Tiras"}} {
		c := learn.Crossing(tc.side, learn.ContinentOf(tc.from), learn.ContinentOf(tc.to))
		out = append(out, benchRow{fmt.Sprintf("%s: crossing from %q to %q", tc.side, tc.from, tc.to), fmt.Sprintf("%q", c), "no crossing: a zone no list holds has no continent", trTimesNone, resultIf(c == "", "a crossing was invented")})
	}
	finish(t, "advice-travel-no-answer", "Offline travel: no answer in the dataset", out)
}

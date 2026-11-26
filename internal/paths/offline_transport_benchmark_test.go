package paths_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/WoW-Companion/advisor/internal/brief"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/learn"
	"github.com/WoW-Companion/advisor/internal/levelup"
	"github.com/WoW-Companion/advisor/internal/lua"
	"github.com/WoW-Companion/advisor/internal/questie"
	"github.com/WoW-Companion/advisor/internal/route"
	"github.com/WoW-Companion/advisor/internal/store"
)

// The offline transport benchmark: how the advisor says to reach a place (flight points, portals and boats between
// continents, the hearthstone bind and the inn a session ends at, and the travel step of a route), beside the dataset's
// rows and the game's own transport network, with no network. The dataset holds ui_maps (a zone's parent continent) and
// nothing else for travel: no flight path, flight master, portal, boat, zeppelin, innkeeper or bind table, so a case
// about one of those is set beside the game's flight list and transport guides, and a case the dataset cannot answer
// passes when the advisor says so and names no place. A difference caused by the dataset's own rows is marked "(data)".
// make offline-benchmark runs it; WOWC_BENCH_REPORT receives the tables.

const (
	flightGuide  = "game flight path list (Wowhead taxi guides); dataset npcs, trainers zone names"
	portalGuide  = "game transport guides (Wowhead: Undercity-Orgrimmar zeppelin, Menethil Harbor-Auberdine boat, the Dark Portal); dataset ui_maps"
	hearthGuide  = "the capture's hearth bind (GetBindLocation); the game's hearthstone rule; the dataset has no bind or innkeeper row"
	routeGuide   = "game transport guides (Wowhead); questie quest and creature rows in a synthetic index"
	transportNil = "the dataset has no flight path, portal, innkeeper or bind row; docs/schema.md of the dataset lists no such table"
)

// ---- flight paths ----

// TestOfflineBenchmarkFlightPoints: the flight point the class-trainer line names for a character standing in a zone, for
// classes and zones not covered elsewhere, and the zone name as a capture may spell it.
func TestOfflineBenchmarkFlightPoints(t *testing.T) {
	var out []benchRow
	lvl := 30
	for _, tc := range []struct{ side, class, zone, city, point string }{
		{"Horde", "WARLOCK", "Hillsbrad Foothills", "Undercity", "Tarren Mill"},
		{"Alliance", "WARRIOR", "Duskwood", "Stormwind City", "Darkshire"},
		{"Alliance", "PRIEST", "Redridge Mountains", "Stormwind City", "Lakeshire"},
		{"Alliance", "SHAMAN", "Wetlands", "Ironforge", "Menethil Harbor"},
		{"Horde", "ROGUE", "Silverpine Forest", "Undercity", "The Sepulcher"},
		{"Horde", "SHAMAN", "The Barrens", "Orgrimmar", "the Crossroads"},
		{"Horde", "SHAMAN", "THE BARRENS", "Orgrimmar", "the Crossroads"},
		{"Horde", "WARLOCK", " Hillsbrad Foothills ", "Undercity", "Tarren Mill"},
		{"horde", "WARLOCK", "Silverpine Forest", "Undercity", "The Sepulcher"},
	} {
		c := character.Character{Class: tc.class, Faction: tc.side, Level: &lvl, Location: &character.Location{Zone: tc.zone}}
		pl, ok := levelup.TrainAt(c, nil)
		got := fmt.Sprintf("%s, fly from %q", pl.City, pl.From)
		ref := fmt.Sprintf("%s, fly from %q", tc.city, tc.point)
		out = append(out, benchRow{fmt.Sprintf("%s %s in %q: flight to the class trainer", tc.side, tc.class, tc.zone), got, ref, flightGuide,
			resultIf(ok && got == ref && strings.Contains(pl.Line(tc.class), "(fly from "+tc.point+")"), "the flight point, or a zone spelled with other case or spaces")})
	}
	finish(t, "advice-flight-points", "Offline flight points: where to fly from to the class trainer", out)
}

// TestOfflineBenchmarkFlightMasters: the flight masters a player opened, as the brief lists them: newest first, only the
// player's side (a neutral one serves both), each with its zone.
func TestOfflineBenchmarkFlightMasters(t *testing.T) {
	now := time.Unix(1700100000+3600, 0)
	mk := func(side string, fl ...character.SeenFlight) string {
		c := character.Character{Faction: side, Seen: &character.Observed{Flights: fl}}
		return brief.Character(c, character.Client{}, now)
	}
	flights := []character.SeenFlight{
		{NPC: 1, Name: "Synthetic Horde Rider", Map: 10, Faction: "Horde", T: 1700100000 - 7200},
		{NPC: 2, Name: "Synthetic Alliance Rider", Map: 11, Faction: "Alliance", T: 1700100000 - 600},
		{NPC: 3, Name: "Synthetic Neutral Rider", Map: 12, Faction: "", T: 1700100000 - 60},
	}
	var out []benchRow
	for _, tc := range []struct {
		side string
		want []string
		not  []string
	}{
		{"Horde", []string{"Synthetic Neutral Rider; Synthetic Horde Rider"}, []string{"Alliance Rider"}},
		{"Alliance", []string{"Synthetic Neutral Rider; Synthetic Alliance Rider"}, []string{"Horde Rider"}},
	} {
		b := mk(tc.side, flights...)
		line := ""
		for _, l := range strings.Split(b, "\n") {
			if strings.HasPrefix(l, "Flight points you know") {
				line = l
			}
		}
		ok := line != ""
		for _, w := range tc.want {
			ok = ok && strings.Contains(line, w)
		}
		for _, w := range tc.not {
			ok = ok && !strings.Contains(line, w)
		}
		out = append(out, benchRow{tc.side + ": flight points the brief lists", line, "the side's and the neutral flight masters, newest first: " + strings.Join(tc.want, ""), "the capture's flightmasters; the game's flight master faction rule",
			resultIf(ok, "a flight master of the other side is listed, or the order is wrong")})
	}
	b := mk("Horde")
	out = append(out, benchRow{"no flight master opened: the brief names none", fmt.Sprintf("%v", strings.Contains(b, "Flight points you know")), "false: no flight is suggested", "the capture's flightmasters", resultIf(!strings.Contains(b, "Flight points you know"), "a flight point was invented")})
	finish(t, "advice-flight-masters", "Offline flight masters the player opened", out)
}

// ---- portals and zone connections ----

// TestOfflineBenchmarkZoneConnections: the continent the advisor puts a zone on beside the dataset's ui_maps parent, for every
// zone the snapshot holds, and the zones the advisor knows that the dataset does not.
func TestOfflineBenchmarkZoneConnections(t *testing.T) {
	zones := worldZones(t)
	var out []benchRow
	bad := 0
	for _, z := range []string{"Westfall", "Loch Modan", "Teldrassil", "Thousand Needles", "Moonglade", "Blasted Lands", "Deadwind Pass"} {
		want, got := zones[z], learn.ContinentOf(z)
		out = append(out, benchRow{"zone " + z + ": continent", got, want, zoneSrc, resultIf(got == want && want != "", "the continent")})
	}
	for z, want := range zones {
		if learn.ContinentOf(z) != want {
			bad++
		}
	}
	out = append(out, benchRow{fmt.Sprintf("all %d zones of ui_maps: continent", len(zones)), fmt.Sprintf("%d differ", bad), "0 differ", zoneSrc, resultIf(bad == 0, "a zone is on another continent than the dataset's parent")})
	// zones the advisor knows and the snapshot's ui_maps does not hold: the dataset stops at the Classic world
	for _, z := range []string{"Hellfire Peninsula", "Eversong Woods", "Azuremyst Isle", "Shattrath City"} {
		_, held := zones[z]
		res := "match"
		if !held {
			res = "differ (data): ui_maps holds no row for " + z + ", so no parent continent to compare"
		}
		out = append(out, benchRow{"zone " + z + ": continent", learn.ContinentOf(z), "no row in ui_maps", zoneSrc, res})
	}
	finish(t, "advice-zone-connections", "Offline zone connections: the continent of a zone", out)
}

// TestOfflineBenchmarkPortals: the crossing a route says between continents, for the portal, zeppelin and boat each side uses, with
// the faction spelled as a capture or a setting may spell it.
func TestOfflineBenchmarkPortals(t *testing.T) {
	const ek, kal, outl = "Eastern Kingdoms", "Kalimdor", "Outland"
	var out []benchRow
	for _, tc := range []struct {
		side, from, to string
		must, never    []string
	}{
		{"horde", ek, kal, []string{"Zeppelin", "Orgrimmar"}, []string{"Boat"}},
		{"HORDE", kal, ek, []string{"Zeppelin", "Undercity"}, []string{"Boat"}},
		{"alliance", ek, kal, []string{"Boat", "Auberdine"}, []string{"Zeppelin"}},
		{" Alliance ", kal, ek, []string{"Boat", "Menethil Harbor"}, []string{"Zeppelin"}},
		{"horde", kal, outl, []string{"Zeppelin", "Undercity", "Dark Portal", "Blasted Lands"}, []string{"Boat"}},
		{"alliance", outl, ek, []string{"Step through", "Dark Portal", "Hellfire Peninsula"}, []string{"Boat", "Zeppelin"}},
		{"Horde", outl, outl, nil, []string{"Portal", "Zeppelin", "Boat"}},
	} {
		got := learn.Crossing(tc.side, tc.from, tc.to)
		ok := (got != "") == (len(tc.must) > 0)
		for _, m := range tc.must {
			ok = ok && strings.Contains(got, m)
		}
		for _, m := range tc.never {
			ok = ok && !strings.Contains(got, m)
		}
		ref := "mentions " + strings.Join(tc.must, ", ") + "; never the other side's transport"
		if len(tc.must) == 0 {
			ref = "no crossing: the same continent"
		}
		out = append(out, benchRow{fmt.Sprintf("%q: %s to %s", tc.side, tc.from, tc.to), fmt.Sprintf("%q", got), ref, portalGuide, resultIf(ok, "the crossing, or no crossing for a faction spelled in lower case")})
	}
	finish(t, "advice-portals", "Offline portals, zeppelins and boats between continents", out)
}

// ---- hearthstone and inn binds ----

func hearthCapture(t *testing.T, bind string) character.Character {
	t.Helper()
	snap := lua.Table{"location": lua.Table{"zone": "Hillsbrad Foothills"}}
	if bind != "" {
		snap["hearth"] = bind
	}
	return character.LoadCharacter("R:N", lua.Table{"snapshot": snap})
}

// TestOfflineBenchmarkHearthBind: the hearthstone bind the capture holds, loaded, merged between two captures and told to the
// player: where it is bound, and that the class city is one hearth away when the bind is there. The dataset names
// no bind point, so the capture's own value is the reference.
func TestOfflineBenchmarkHearthBind(t *testing.T) {
	var out []benchRow
	add := func(check, got, ref string, ok bool) {
		out = append(out, benchRow{check, got, ref, hearthGuide, resultIf(ok, "the bind was lost, guessed or left unsaid")})
	}
	now := time.Unix(1700100000+60, 0)
	line := func(c character.Character) string {
		for _, l := range strings.Split(brief.Character(c, character.Client{}, now), "\n") {
			if strings.HasPrefix(l, "Hearthstone") {
				return l
			}
		}
		return "(no hearthstone line)"
	}
	c := hearthCapture(t, "Razor Hill")
	add("capture bound at Razor Hill: loaded", c.Hearth, "Razor Hill", c.Hearth == "Razor Hill")
	add("capture bound at Razor Hill: the brief says so", line(c), "Hearthstone bound to: Razor Hill", line(c) == "Hearthstone bound to: Razor Hill")
	add("capture bound at Razor Hill, written with spaces", line(character.Character{Hearth: "  Razor Hill "}), "Hearthstone bound to: Razor Hill", line(character.Character{Hearth: "  Razor Hill "}) == "Hearthstone bound to: Razor Hill")
	none := hearthCapture(t, "")
	add("capture with no bind: the brief names none", line(none), "(no hearthstone line): no bind is guessed", line(none) == "(no hearthstone line)")
	older := character.Character{Name: "A", Hearth: "Orgrimmar", CapturedAt: new(int64)}
	newer := character.Character{Name: "A", Hearth: "Undercity", CapturedAt: new(int64)}
	*older.CapturedAt, *newer.CapturedAt = 100, 200
	add("two captures, the newer rebound: the newer bind", character.MergeCharacters(older, newer).Hearth, "Undercity", character.MergeCharacters(older, newer).Hearth == "Undercity")
	newer.Hearth = ""
	add("two captures, the newer holds no bind: the older bind", character.MergeCharacters(older, newer).Hearth, "Orgrimmar", character.MergeCharacters(older, newer).Hearth == "Orgrimmar")
	// the class city and the bind
	lvl := 30
	mage := func(bind, zone string) levelup.TrainPlace {
		pl, _ := levelup.TrainAt(character.Character{Class: "MAGE", Faction: "Horde", Level: &lvl, Hearth: bind, Location: &character.Location{Zone: zone}}, nil)
		return pl
	}
	pl := mage("Undercity", "Tirisfal Glades")
	add("mage in Tirisfal Glades bound at Undercity: the line", pl.Line("MAGE"), "mentions the hearthstone bound in Undercity", strings.Contains(pl.Line("MAGE"), "your hearthstone is bound there") && !strings.Contains(pl.Line("MAGE"), "fly from"))
	pl = mage("undercity ", "Hillsbrad Foothills")
	add("mage in Hillsbrad bound at Undercity: flight and bind", pl.Line("MAGE"), "fly from Tarren Mill, and the bind in Undercity", strings.Contains(pl.Line("MAGE"), "(fly from Tarren Mill; your hearthstone is bound there)"))
	pl = mage("Orgrimmar", "Tirisfal Glades")
	add("mage in Tirisfal Glades bound at Orgrimmar: no bind claimed", pl.Line("MAGE"), "no hearthstone in the line: the bind is not in Undercity", !strings.Contains(pl.Line("MAGE"), "hearth"))
	pl = mage("", "Tirisfal Glades")
	add("mage in Tirisfal Glades, no bind: no hearthstone said", pl.Line("MAGE"), "no hearthstone in the line", !strings.Contains(pl.Line("MAGE"), "hearth"))
	pl = mage("Undercity", "Undercity")
	add("mage in Undercity bound there: you are here", pl.Line("MAGE"), "(you are here) and no hearthstone", strings.Contains(pl.Line("MAGE"), "(you are here)") && !strings.Contains(pl.Line("MAGE"), "hearth"))
	finish(t, "advice-hearth-bind", "Offline hearthstone bind", out)
}

// TestOfflineBenchmarkInnAnchors: the inn or flight point a short session ends at, for cases beside the inn benchmark's: the
// side an inn serves, the nearest of an inn and a flight point, and an inn the player's side may not use.
func TestOfflineBenchmarkInnAnchors(t *testing.T) {
	var out []benchRow
	add := func(check, got, ref string, ok bool) {
		out = append(out, benchRow{check, got, ref, qbInnSrc, resultIf(ok, "the place the session ends at")})
	}
	only := func(keep ...int) *questie.Index {
		x := qbQx()
		for id := range x.Npcs {
			kept := false
			for _, k := range keep {
				kept = kept || k == id
			}
			if !kept {
				delete(x.Npcs, id)
			}
		}
		return x
	}
	s := qbSession(t, "horde", only(1, 2, 3), 30, nil)
	add("side spelled in lower case (horde): the Horde's inn", s.EndsAt, "Synthetic Innkeeper East, the Horde's inn, nearer than the neutral one", strings.Contains(s.EndsAt, "Synthetic Innkeeper East"))
	s = qbSession(t, "alliance", only(2, 3), 30, nil)
	add("Alliance with the Alliance's and the Horde's inn: the Alliance's", s.EndsAt, "Synthetic Innkeeper West", strings.Contains(s.EndsAt, "Synthetic Innkeeper West"))
	s = qbSession(t, "Alliance", only(3), 30, nil)
	add("Alliance, only a Horde inn on the map", s.EndsAt+"(none)", "no inn the Alliance may use: nowhere is named", s.EndsAt == "")
	s = qbSession(t, "Horde", only(4), 30, nil)
	add("Horde, only a flight point on the map", s.EndsAt, "the flight point: Synthetic Flight Master", strings.Contains(s.EndsAt, "Synthetic Flight Master"))
	s = qbSession(t, "Horde", only(1), 30, nil)
	add("Horde, only the neutral inn", s.EndsAt, "Synthetic Innkeeper North, a neutral inn serves both sides", strings.Contains(s.EndsAt, "Synthetic Innkeeper North"))
	s = qbSession(t, "", only(3), 30, nil)
	add("a character with no known side: an inn of either side", s.EndsAt, "an inn is named: no side is assumed to exclude it", strings.Contains(s.EndsAt, "Synthetic Innkeeper East"))
	s = qbSession(t, "Horde", only(2, 3), 30, nil)
	said := strings.Contains(strings.ToLower(s.Facts()), "hearth")
	add("a session's facts never prescribe a hearthstone", fmt.Sprintf("hearthstone mentioned: %v", said), "hearthstone mentioned: false", !said)
	finish(t, "advice-inn-anchors", "Offline inn and flight point anchors", out)
}

// ---- travel routes ----

// travelPlan is the travel step of a route from one map to another: a character on map from has a quest on map to.
func travelPlan(t *testing.T, side string, from, to int, zones map[int]string, flight *character.SeenFlight) route.ZonePlan {
	t.Helper()
	at := func(area int, x, y float64) []questie.Spawn { return []questie.Spawn{{Area: area, X: x, Y: y}} }
	qx := &questie.Index{
		Quests:  map[int]*questie.Quest{},
		Objects: map[int]questie.Entity{},
		Areas:   map[int]questie.Area{},
		Npcs: map[int]questie.Entity{
			10: {Name: "Synthetic Giver", Spawns: at(from, 50, 50)},
			11: {Name: "Synthetic Boar", Spawns: at(to, 40, 40)},
			12: {Name: "Synthetic Finisher", Spawns: at(to, 42, 44)},
		},
	}
	for m, z := range zones {
		qx.Areas[m] = questie.Area{UIMap: m, Zone: z}
	}
	qx.Quests[900] = &questie.Quest{ID: 900, Name: "Synthetic Errand", Level: 9, Finishers: []questie.Ref{{ID: 12}}, Creatures: []questie.Ref{{ID: 11}}}
	lvl := 9
	x, y := 50.0, 50.0
	c := character.Character{Name: "Benchtest", Faction: side, QuestsCaptured: true, Level: &lvl,
		Location: &character.Location{MapID: &from, X: &x, Y: &y},
		Quests:   []character.Quest{{QuestID: 900, Title: "Synthetic Errand", Objectives: []character.Objective{{Text: "Synthetic Boar slain: 0/1"}}}}}
	if flight != nil {
		c.Seen = &character.Observed{Flights: []character.SeenFlight{*flight}}
		qx.Npcs[int(flight.NPC)] = questie.Entity{Name: flight.Name, Spawns: at(to, flight.X, flight.Y)}
	}
	p := route.Build(c, qx)
	for _, z := range p.Zones {
		if z.Map == to {
			return z
		}
	}
	return route.ZonePlan{}
}

// TestOfflineBenchmarkTravelRoutes: the travel step of a route to a zone the player is not in, beside the transport guides:
// the zeppelin or boat first when the zone is on another continent, then a flight only to a flight master the
// player opened, else a walk.
func TestOfflineBenchmarkTravelRoutes(t *testing.T) {
	const (
		mUC, mOrg, mBarrens, mStone = 1458, 1454, 1413, 1442
		mSW, mDark, mWet, mHill     = 1453, 1439, 1437, 1424
		mBlasted, mHellfire         = 1419, 1944
	)
	names := map[int]string{mUC: "Undercity", mOrg: "Orgrimmar", mBarrens: "The Barrens", mStone: "Stonetalon Mountains", mSW: "Stormwind City",
		mDark: "Darkshore", mWet: "Wetlands", mHill: "Hillsbrad Foothills", mBlasted: "Blasted Lands", mHellfire: "Hellfire Peninsula"}
	var out []benchRow
	add := func(check, got, ref string, ok bool) {
		out = append(out, benchRow{check, got, ref, routeGuide, resultIf(ok, "the travel step")})
	}
	z := travelPlan(t, "Horde", mUC, mBarrens, names, nil)
	add("Horde, Undercity to the Barrens", z.Travel, "Zeppelin to Orgrimmar, then walk or ride to The Barrens", z.Travel == "Zeppelin to Orgrimmar, then walk or ride to The Barrens")
	z = travelPlan(t, "Horde", mOrg, mHill, names, nil)
	add("Horde, Orgrimmar to Hillsbrad Foothills", z.Travel, "Zeppelin to Undercity, then walk or ride to Hillsbrad Foothills", z.Travel == "Zeppelin to Undercity, then walk or ride to Hillsbrad Foothills")
	z = travelPlan(t, "Alliance", mSW, mDark, names, nil)
	add("Alliance, Stormwind City to Darkshore", z.Travel, "Boat to Auberdine, then walk or ride to Darkshore", z.Travel == "Boat to Auberdine, then walk or ride to Darkshore")
	z = travelPlan(t, "Alliance", mDark, mWet, names, nil)
	add("Alliance, Darkshore to Wetlands", z.Travel, "Boat to Menethil Harbor, then walk or ride to Wetlands", z.Travel == "Boat to Menethil Harbor, then walk or ride to Wetlands")
	z = travelPlan(t, "horde", mOrg, mHill, names, nil)
	add("a side spelled in lower case, Orgrimmar to Hillsbrad", z.Travel, "Zeppelin to Undercity, then walk or ride to Hillsbrad Foothills", z.Travel == "Zeppelin to Undercity, then walk or ride to Hillsbrad Foothills")
	z = travelPlan(t, "Horde", mBarrens, mStone, names, nil)
	add("Horde, the Barrens to Stonetalon Mountains: the road, no crossing", z.Travel, "Walk or ride to Stonetalon Mountains from The Barrens", z.Travel == "Walk or ride to Stonetalon Mountains from The Barrens")
	fl := &character.SeenFlight{NPC: 50, Name: "Synthetic Master", Map: mHill, X: 61, Y: 19, Faction: "Horde", T: 1}
	z = travelPlan(t, "Horde", mOrg, mHill, names, fl)
	add("Horde, Orgrimmar to Hillsbrad with its flight master opened", z.Travel, "Zeppelin to Undercity, then fly to Synthetic Master", z.Travel == "Zeppelin to Undercity, then fly to Synthetic Master")
	fl.Faction = "Alliance"
	z = travelPlan(t, "Horde", mOrg, mHill, names, fl)
	add("Horde, the Alliance's flight master opened: no flight", z.Travel, "walk, not a flight of the other side", !strings.Contains(z.Travel, "fly"))
	z = travelPlan(t, "Alliance", mSW, mBlasted, names, nil)
	add("Alliance, Stormwind City to the Blasted Lands: the same continent", z.Travel, "Walk or ride to Blasted Lands", z.Travel == "Walk or ride to Blasted Lands")
	finish(t, "advice-travel-routes", "Offline travel routes: the step that goes to a zone", out)
}

// ---- no answer in the dataset ----

// TestOfflineBenchmarkTransportNoAnswer: travel questions the dataset cannot answer: no flight path, portal, boat, innkeeper or
// bind table. The advisor says so or names nothing.
func TestOfflineBenchmarkTransportNoAnswer(t *testing.T) {
	var out []benchRow
	add := func(check, got, ref string, ok bool) {
		out = append(out, benchRow{check, got, ref, transportNil, resultIf(ok, "a place or a route was invented")})
	}
	ds := worldStore()
	for _, table := range []string{"flight_paths", "taxi_nodes", "portals", "innkeepers", "bind_points"} {
		_, err := ds.Filter(table, store.Range{Column: "id", Min: int64(0)})
		res := "match"
		if err == nil {
			res = "differ: the dataset now holds a " + table + " table: the cases here need a second look"
		}
		out = append(out, benchRow{"dataset table " + table, fmt.Sprintf("error: %v", err != nil), "absent: no travel row to answer from", transportNil, res})
	}
	lvl := 30
	say := func(side, class, zone string) string {
		pl, _ := levelup.TrainAt(character.Character{Class: class, Faction: side, Level: &lvl, Location: &character.Location{Zone: zone}}, nil)
		return pl.Line(class)
	}
	l := say("Horde", "DRUID", "The Barrens")
	add("Horde druid in the Barrens: a flight to Thunder Bluff", l, "no flight point: none is listed for Thunder Bluff", !strings.Contains(l, "fly from"))
	l = say("Alliance", "MAGE", "Ironforge")
	add("Alliance mage in Ironforge: a flight to Stormwind City", l, "no flight point named without a row", !strings.Contains(l, "fly from"))
	l = say("Alliance", "WARRIOR", "Zul'Gurub")
	add("Alliance warrior in a zone no list holds: a flight", l, "no flight point for an unknown zone", !strings.Contains(l, "fly from"))
	l = say("Horde", "MAGE", "Orgrimmar")
	add("Horde mage in Orgrimmar: a mage portal to Undercity", l, "no portal named: the dataset holds none", !strings.Contains(strings.ToLower(l), "portal") && !strings.Contains(strings.ToLower(l), "teleport"))
	for _, tc := range []struct{ side, from, to string }{{"Horde", "Zul'Gurub", "Durotar"}, {"Alliance", "Elwynn Forest", "Zul'Gurub"}, {"Horde", "", "Durotar"}} {
		c := learn.Crossing(tc.side, learn.ContinentOf(tc.from), learn.ContinentOf(tc.to))
		add(fmt.Sprintf("%s: crossing from %q to %q", tc.side, tc.from, tc.to), fmt.Sprintf("%q", c), "no crossing: a zone no list holds has no continent", c == "")
	}
	if f := levelup.FlightFrom("Booty Bay", "Stranglethorn Vale"); f != "" {
		add("flight point to Booty Bay", f, "none: no row", false)
	} else {
		add("flight point to Booty Bay", "none", "none: no row", true)
	}
	// a route to a zone with no known entrance walks with no position
	z := travelPlan(t, "Horde", 1454, 1413, map[int]string{1454: "Orgrimmar", 1413: "The Barrens"}, nil)
	add("route to the Barrens from Orgrimmar: a walk with no known entrance", fmt.Sprintf("%s (position known: %v)", z.Travel, z.Arrival.Known), "a walk, no position invented", z.Arrival.Kind == route.ArriveWalk && !z.Arrival.Known)
	finish(t, "advice-transport-no-answer", "Offline transport advice, no answer in the dataset", out)
}

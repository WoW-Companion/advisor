package paths_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/WoW-Companion/advisor/internal/ask"
	"github.com/WoW-Companion/advisor/internal/brief"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/dungeons"
	"github.com/WoW-Companion/advisor/internal/inventory"
	"github.com/WoW-Companion/advisor/internal/levelup"
	"github.com/WoW-Companion/advisor/internal/route"
	"github.com/WoW-Companion/advisor/internal/session"
	"github.com/WoW-Companion/advisor/internal/store"
	"github.com/WoW-Companion/advisor/internal/wording"
)

// The offline group-play, travel-time, rested-XP and unknown-data benchmark: which dungeons the advisor offers a level and how it
// words a group size, how it says to reach a trainer and how long it counts a walk, a ride and a stated play time to be, what the XP line
// and the quest XP say about rested XP and the top level, and what it says when the dataset holds nothing (a guild bank, a flight time,
// a rested XP rate, the roles a dungeon needs), beside the dataset's rows and the game's own rules, with no network. It reads
// testdata/levelling_snapshot (instances, class spells) and testdata/usability_snapshot (items). The dataset holds no group role, flight
// time, rested XP rate, guild bank table or hearthstone cooldown; those cases check that the advisor says so and states none. A
// difference the dataset's own rows cause is marked "(data)". make offline-benchmark runs it; WOWC_BENCH_REPORT receives the tables.

const (
	gpWindowSrc = "dataset instances (min_level, max_level, entrance_zone_name); the dungeon window of docs/dungeons.md: from the minimum level to three levels past the maximum"
	gpSizeSrc   = "dataset instances (max_players); game: a Classic dungeon is a five-player instance, Upper Blackrock Spire ten (Wowhead, Warcraft Wiki)"
	gpFlightSrc = "game flight path list (Wowhead taxi guides); Warcraft Wiki class trainer lists"
	gpSpeedSrc  = "Warcraft Wiki, Movement speed: a character runs 7 yards a second, a 60% mount 11.2"
	gpTimeSrc   = "the time the player states in the question"
	gpRestSrc   = "Warcraft Wiki, Rest: the rested bonus doubles the XP of kills only; the pool holds at most 150% of a level"
	gpNoneSrc   = "the dataset has no such row, column or table"
)

// ---- dungeon level range, side and group size ----

type gpWindowCase struct {
	name, side string
}

var gpWindowCases = []gpWindowCase{
	{"Deadmines", "Alliance"}, {"Razorfen Downs", "Horde"}, {"Maraudon", "Horde"}, {"Scarlet Monastery", "Alliance"}, {"Zul'Farrak", "Horde"}, {"Gnomeregan", "Alliance"},
}

func gpOffered(t *testing.T, side string, level int, name string) bool {
	t.Helper()
	c := levChar("WARRIOR", level)
	c.Faction = side
	res, err := dungeons.Plan(c, levStore())
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range res.Picks {
		if p.Name == name {
			return true
		}
	}
	return false
}

// TestOfflineBenchmarkGroupDungeonWindow: a dungeon is offered from its minimum level to three levels past its maximum, not a level
// before or after, to the side that can enter it, for six dungeons.
func TestOfflineBenchmarkGroupDungeonWindow(t *testing.T) {
	ds := levStore()
	var out []benchRow
	for _, tc := range gpWindowCases {
		rows, _ := ds.Filter("instances", store.Range{Column: "name", Min: tc.name, Max: tc.name})
		if len(rows) != 1 {
			t.Fatalf("%s: not in the snapshot", tc.name)
		}
		lo, hi := int(num(rows[0]["min_level"])), int(num(rows[0]["max_level"]))
		got := fmt.Sprintf("offered at %d: %v, at %d: %v, at %d: %v, at %d: %v", lo-1, gpOffered(t, tc.side, lo-1, tc.name), lo, gpOffered(t, tc.side, lo, tc.name),
			hi+dungeons.LevelWindow, gpOffered(t, tc.side, hi+dungeons.LevelWindow, tc.name), hi+dungeons.LevelWindow+1, gpOffered(t, tc.side, hi+dungeons.LevelWindow+1, tc.name))
		want := fmt.Sprintf("offered at %d: false, at %d: true, at %d: true, at %d: false", lo-1, lo, hi+dungeons.LevelWindow, hi+dungeons.LevelWindow+1)
		out = append(out, benchRow{tc.name + " for a " + tc.side + " warrior: the levels it is offered at", got, fmt.Sprintf("%s (instances %d-%d)", want, lo, hi), gpWindowSrc, resultIf(got == want, "offered outside its window or not inside it")})
	}
	finish(t, "advice-group-dungeon-window", "Offline dungeon group play: the levels a dungeon is offered at", out)
}

// TestOfflineBenchmarkGroupDungeonSide: a dungeon whose entrance is in the other side's capital is not offered to this side.
func TestOfflineBenchmarkGroupDungeonSide(t *testing.T) {
	var out []benchRow
	for _, tc := range []struct {
		name, side string
		level      int
		want       bool
	}{
		{"Ragefire Chasm", "Alliance", 15, false}, {"Ragefire Chasm", "Horde", 15, true}, {"Stormwind Stockade", "Horde", 26, false}, {"Stormwind Stockade", "Alliance", 26, true},
	} {
		got := gpOffered(t, tc.side, tc.level, tc.name)
		out = append(out, benchRow{fmt.Sprintf("%s at level %d for the %s", tc.name, tc.level, tc.side), fmt.Sprint(got), fmt.Sprint(tc.want),
			"dataset instances (entrance_zone_name Orgrimmar, Stormwind City); the entrance is in a capital of one side", resultIf(got == tc.want, "offered to the side that cannot walk in")})
	}
	finish(t, "advice-group-dungeon-side", "Offline dungeon group play: a dungeon and the side that enters it", out)
}

// gpSizes are dungeons the advisor describes by name that the earlier size cases leave out, with the game's group size.
var gpSizes = []struct {
	name  string
	guide int64
}{{"Deadmines", 5}, {"Ragefire Chasm", 5}, {"Razorfen Kraul", 5}, {"Zul'Farrak", 5}, {"Blackrock Depths", 5}}

// TestOfflineBenchmarkGroupDungeonSize: the group size the advisor words for a dungeon is the dataset's; the dataset's size is beside the
// game's, and a size the dataset gets wrong is a data difference.
func TestOfflineBenchmarkGroupDungeonSize(t *testing.T) {
	ds := levStore()
	var out []benchRow
	for _, tc := range gpSizes {
		rows, _ := ds.Filter("instances", store.Range{Column: "name", Min: tc.name, Max: tc.name})
		c := levChar("WARRIOR", 60)
		p, togo, err := dungeons.Named(c, ds, nil, tc.name)
		if err != nil || p == nil || p.MaxPlayers == nil || len(rows) != 1 {
			t.Fatalf("%s: %v", tc.name, err)
		}
		size := num(rows[0]["max_players"])
		said := int64(dungeons.PartySize(tc.name, false, int(size))) // a dungeon's party is five, whatever the table's cap
		words := dungeons.RenderNamed(c, *p, togo)
		res := "match"
		switch {
		case int64(*p.MaxPlayers) != said || !strings.Contains(words, partyWords(tc.name, false, said)):
			res = "differ: the answer words another size than the party"
		case said != tc.guide:
			res = fmt.Sprintf("differ: the advisor words %d players, the game %d", said, tc.guide)
		}
		out = append(out, benchRow{tc.name + ": group size", fmt.Sprintf("%d players", *p.MaxPlayers), fmt.Sprintf("%d players (dataset %d)", tc.guide, size), gpSizeSrc, res})
	}
	// the dungeon list words the same size
	c := levChar("WARRIOR", 55)
	res, _ := dungeons.Plan(c, ds)
	list := dungeons.Render(c, res, 8)
	rows, _ := ds.Filter("instances", store.Range{Column: "name", Min: "Sunken Temple", Max: "Sunken Temple"})
	size := num(rows[0]["max_players"])
	r := "match"
	if !strings.Contains(list, "Sunken Temple (about levels 50–60; entrance in Swamp of Sorrows; 5 players)") {
		r = "differ: the list words another size or entrance than the game's five-player dungeon"
	}
	out = append(out, benchRow{"Sunken Temple in the dungeon list for a level 55 warrior", firstMatch(list, "Sunken Temple"), fmt.Sprintf("5 players (dataset %d)", size), gpSizeSrc, r})
	finish(t, "advice-group-dungeon-size", "Offline dungeon group play: the group size of a dungeon", out)
}

// ---- flight paths and travel time ----

// TestOfflineBenchmarkTravelFlightPoint: the flight point named for a character in a zone who goes to the class trainer, and none when no direct flight
// to that city is known.
func TestOfflineBenchmarkTravelFlightPoint(t *testing.T) {
	var out []benchRow
	lvl := 30
	for _, tc := range []struct{ side, class, zone, city, point string }{
		{"Alliance", "MAGE", "Westfall", "Stormwind City", "Sentinel Hill"},
		{"Alliance", "ROGUE", "Loch Modan", "Ironforge", "Thelsamar"},
		{"Alliance", "HUNTER", "Wetlands", "Ironforge", "Menethil Harbor"},
		{"Horde", "PALADIN", "Ghostlands", "Silvermoon City", "Tranquillien"},
		{"Horde", "WARLOCK", "Ghostlands", "Undercity", ""}, // Tranquillien flies to Silvermoon City only
		{"Horde", "MAGE", "Undercity", "Undercity", ""},     // already there
	} {
		c := character.Character{Class: tc.class, Faction: tc.side, Level: &lvl, Location: &character.Location{Zone: tc.zone}}
		pl, ok := levelup.TrainAt(c, nil)
		got := fmt.Sprintf("%s, fly from %q", pl.City, pl.From)
		ref := fmt.Sprintf("%s, fly from %q", tc.city, tc.point)
		out = append(out, benchRow{fmt.Sprintf("%s %s in %q: where to fly from", tc.side, tc.class, tc.zone), got, ref, gpFlightSrc, resultIf(ok && got == ref, "the flight point or the city")})
	}
	finish(t, "advice-travel-flight-point", "Offline travel: the flight point to the class trainer", out)
}

func gpWalk(level int, pct float64) float64 {
	c := levChar("MAGE", level)
	in := session.In{Char: c}
	return in.WalkSecs(route.Point{Map: 1, X: 10, Y: 10}, route.Point{Map: 1, X: 10 + pct, Y: 10})
}

// TestOfflineBenchmarkTravelSpeed: how long a walk across a tenth of a map takes on foot and mounted: 7 yards a second to level 39 and the 60% mount at
// 11.2 from level 40, over the straight line and the path factor.
func TestOfflineBenchmarkTravelSpeed(t *testing.T) {
	var out []benchRow
	dist := 10 * session.MapYards / 100 * session.PathFactor
	for _, tc := range []struct {
		level int
		speed float64
		how   string
	}{{39, 7, "on foot"}, {40, 11.2, "on the 60% mount"}, {60, 11.2, "on the 60% mount (the 100% mount is not assumed)"}} {
		got := gpWalk(tc.level, 10)
		want := dist / tc.speed
		out = append(out, benchRow{fmt.Sprintf("a level %d character, 10 map percent (%.0f yards)", tc.level, dist), fmt.Sprintf("%.1f s", got), fmt.Sprintf("%.1f s %s at %.1f yd/s", want, tc.how, tc.speed), gpSpeedSrc,
			resultIf(got > want-0.05 && got < want+0.05, "another speed")})
	}
	// a stop on another map costs no walk: the planner puts no stop on another map in a run
	cross := session.In{Char: levChar("MAGE", 40)}
	z := cross.WalkSecs(route.Point{Map: 1, X: 10, Y: 10}, route.Point{Map: 2, X: 90, Y: 90})
	out = append(out, benchRow{"a stop on another map", fmt.Sprintf("%.0f s", z), "no walk is counted: a run holds the stops of the map the player is on", gpTimeSrc, resultIf(z == 0, "a walk was invented between maps")})
	finish(t, "advice-travel-speed", "Offline travel time: walking and riding", out)
}

// TestOfflineBenchmarkTravelStatedTime: the time a player says they have, said in the ways a player says it, is the time the session plan is built for.
func TestOfflineBenchmarkTravelStatedTime(t *testing.T) {
	var out []benchRow
	for _, tc := range []struct {
		say  string
		want string
	}{
		{"I have an hour and a half to play", "min:90"},
		{"I have 2 hours 30 minutes to play", "min:150"},
		{"I have 1.5 hours to play", "min:90"},
		{"I have three hours to play", "min:180"},
		{"I've got forty five minutes to play", "min:45"},
		{"I have 1 hr 15 min to play", "min:75"},
		{"I've got half an hour", "min:30"},
		{"I have 400 minutes to play", "min:240"},
	} {
		cl := ask.Classify(tc.say, nil, nil)
		out = append(out, benchRow{"\"" + tc.say + "\"", fmt.Sprintf("%s %s", cl.Topic, cl.Subject), "session " + tc.want, gpTimeSrc, resultIf(cl.Topic == ask.TopicSession && cl.Subject == tc.want, "another time than the one said")})
	}
	finish(t, "advice-travel-stated-time", "Offline travel time: the play time a player states", out)
}

// TestOfflineBenchmarkTravelFlightTime: a flight step on the route costs no minutes in a session plan, and the plan states none: the dataset holds no flight time.
func TestOfflineBenchmarkTravelFlightTime(t *testing.T) {
	lvl := 20
	c := levChar("MAGE", lvl)
	mk := func(withFlight bool) session.Session {
		steps := []route.Stop{{N: 1, Kind: route.KindTurnIn, QuestID: 1, Quest: "Report", Label: "Turn in Report", Map: 1, X: 52, Y: 50}}
		if withFlight {
			steps = append(steps, route.Stop{N: 2, Kind: route.KindTravel, Label: "Fly to Stormwind City: turn in Letter", Map: 2, X: 40, Y: 40, QuestID: 2, Quest: "Letter"})
		}
		in := session.In{Char: c, Route: route.Plan{Start: &route.Point{Map: 1, X: 50, Y: 50}, Steps: steps}, Info: session.Info{XP: func(int) (int, bool) { return 500, true }}}
		return session.Plan(in, 30)
	}
	plain, withFlight := mk(false), mk(true)
	facts := withFlight.Facts()
	out := []benchRow{
		{"a route with a flight step: the minutes planned", fmt.Sprintf("%.0f s with, %.0f s without", withFlight.Seconds, plain.Seconds), "the same: no flight time is held, so none is counted", gpNoneSrc,
			resultIf(withFlight.Seconds == plain.Seconds && plain.Seconds > 0, "a flight time was counted")},
		{"the answer's facts name the flight", firstMatch(facts, "Stormwind"), "no flight step in the facts: it is not part of the loop, and no flight minutes are stated", gpNoneSrc,
			resultIf(!strings.Contains(strings.ToLower(facts), "flight") && !strings.Contains(strings.ToLower(facts), "fly"), "a flight was worded in the plan")},
	}
	for _, q := range []string{"how long is the flight from orgrimmar to the crossroads?", "how long does the zeppelin from undercity to orgrimmar take?"} {
		cl := ask.Classify(q, levStore(), nil)
		out = append(out, benchRow{"the question \"" + q + "\"", fmt.Sprintf("kind %d, topic %q", cl.Kind, cl.Topic), "a know-how question with no dataset fact behind it: no flight time in the facts", gpNoneSrc,
			resultIf(cl.Kind == ask.KindKnowHow && cl.Topic == ask.TopicNone, "a flight time question answered from a topic that holds none")})
	}
	for _, table := range []string{"flight_paths", "flight_times"} {
		_, err := levStore().Filter(table, store.Range{Column: "id", Min: int64(0), Max: int64(1)})
		out = append(out, benchRow{"a table of " + strings.ReplaceAll(table, "_", " "), fmt.Sprint(err != nil), "no such table in the dataset", gpNoneSrc, resultIf(err != nil, "a table was invented")})
	}
	finish(t, "advice-travel-flight-time", "Offline travel time: a flight and its minutes", out)
}

// ---- rested XP and level pacing ----

func gpProgress(c character.Character) string {
	for _, l := range strings.Split(brief.Character(c, character.Client{}, time.Unix(0, 0)), "\n") {
		if strings.HasPrefix(l, "Progress: ") {
			return strings.TrimPrefix(l, "Progress: ")
		}
	}
	return ""
}

// TestOfflineBenchmarkRestedProgress: the XP line states the rested pool in points and never as a percent or a rate, says no XP is to be earned at the top
// level (an empty XP bar), and names no rested pool the capture did not hold.
func TestOfflineBenchmarkRestedProgress(t *testing.T) {
	var out []benchRow
	zero, size, pool, over := 0, 2100, 3150, 9000
	for _, tc := range []struct {
		label        string
		lvl          int
		xp, max, rst *int
		want         string
	}{
		{"the pool at its 150% cap", 12, &zero, &size, &pool, "XP 0/2100, rested 3150 XP"},
		{"no rested XP captured", 12, &zero, &size, nil, "XP 0/2100, rested XP not captured"},
		{"the top level: an empty XP bar", 60, &zero, &zero, nil, "no XP to earn at this level, so no rested XP either"},
		{"the top level with a stale rested figure", 60, &zero, &zero, &over, "no XP to earn at this level, so no rested XP either"},
		{"the level's size not captured", 30, &zero, nil, &pool, "XP not captured, rested 3150 XP"},
	} {
		c := levChar("MAGE", tc.lvl)
		c.XP, c.XPMax, c.RestedXP = tc.xp, tc.max, tc.rst
		got := gpProgress(c)
		out = append(out, benchRow{fmt.Sprintf("level %d: %s", tc.lvl, tc.label), got, tc.want, gpRestSrc, resultIf(got == tc.want, "another line than the capture holds")})
	}
	finish(t, "advice-rested-progress", "Offline rested XP: the progress line", out)
}

// TestOfflineBenchmarkRestedQuestXP: the XP a quest gives a character does not change with the rested pool (rested XP doubles kills, not quest rewards),
// and the share of the level a session gives is left out at the top level.
func TestOfflineBenchmarkRestedQuestXP(t *testing.T) {
	var out []benchRow
	pool := 3000
	for _, rested := range []*int{nil, &pool} {
		c := levChar("MAGE", 20)
		c.RestedXP = rested
		in := session.In{Char: c, Info: session.Info{XP: func(int) (int, bool) { return 800, true }}}
		xp, est := in.QuestXP(1, 20)
		out = append(out, benchRow{fmt.Sprintf("a level 20 quest worth 800 XP, rested pool %v", rested != nil), fmt.Sprintf("%d XP, estimated %v", xp, est), "800 XP: the rested bonus is on kills only", gpRestSrc, resultIf(xp == 800 && !est, "the rested pool changed a quest reward")})
	}
	top, zero := 60, 0
	c := character.Character{Name: "Top", Faction: "Horde", Class: "MAGE", Level: &top, XP: &zero, XPMax: &zero}
	in := session.In{Char: c, Route: route.Plan{Start: &route.Point{Map: 1, X: 50, Y: 50}, Steps: []route.Stop{{N: 1, Kind: route.KindTurnIn, QuestID: 1, Quest: "Report", Label: "Turn in Report", Map: 1, X: 52, Y: 50}}},
		Info: session.Info{XP: func(int) (int, bool) { return 500, true }}}
	s := session.Plan(in, 30)
	out = append(out, benchRow{"a session at the top level: the share of the level", fmt.Sprintf("%.1f%%, facts name a share %v", s.SharePct, strings.Contains(s.Facts(), "of this level")), "no share: the level has no size", gpRestSrc,
		resultIf(s.SharePct == 0 && !strings.Contains(s.Facts(), "of this level"), "a share of a level with no size")})
	finish(t, "advice-rested-quest-xp", "Offline rested XP: quest XP and the pool", out)
}

// TestOfflineBenchmarkPacingQuestShare: the share of a quest's XP a character of a level keeps, at the boundaries of the Warcraft Wiki table.
func TestOfflineBenchmarkPacingQuestShare(t *testing.T) {
	var out []benchRow
	for _, tc := range []struct {
		mine, quest int
		share       float64
	}{{15, 10, 1}, {16, 10, 0.8}, {13, 8, 0.8}, {12, 8, 1}, {20, 11, 0.2}, {60, 50, 0.1}, {60, 55, 1}, {60, 54, 0.8}} {
		in := session.In{Char: levChar("MAGE", tc.mine), Info: session.Info{XP: func(int) (int, bool) { return 1000, true }}}
		xp, _ := in.QuestXP(1, tc.quest)
		out = append(out, benchRow{fmt.Sprintf("a level %d character, a level %d quest worth 1000 XP", tc.mine, tc.quest), fmt.Sprintf("%d XP", xp), fmt.Sprintf("%d%%: %d XP (difference %d)", int(tc.share*100), int(tc.share*1000), tc.mine-tc.quest),
			levXPSrc, resultIf(xp == int(tc.share*1000), "another share than the table")})
	}
	finish(t, "advice-pacing-quest-share", "Offline level pacing: the share of a quest's XP", out)
}

// TestOfflineBenchmarkPacingLevelWording: how a dungeon's or quest's level is worded against the player's, across the whole range.
func TestOfflineBenchmarkPacingLevelWording(t *testing.T) {
	var out []benchRow
	for _, tc := range []struct {
		level, mine int
		want        string
	}{{60, 50, "10 levels above you"}, {1, 60, "59 levels below you"}, {35, 34, "A level above you"}, {45, 45, "Right for your level"}, {2, 3, "A level below you"}} {
		got := wording.Fit(tc.level, tc.mine)
		out = append(out, benchRow{fmt.Sprintf("level %d for a level %d character", tc.level, tc.mine), got, tc.want, levDunSrc, resultIf(strings.EqualFold(got, tc.want), "another wording")})
	}
	finish(t, "advice-pacing-level-wording", "Offline level pacing: a level against the player's", out)
}

// ---- no answer in the dataset ----

// TestOfflineBenchmarkUnknownGuildBank: the dataset holds no guild bank; the bag and bank review never sends an item to one and the questions about one
// carry no fact.
func TestOfflineBenchmarkUnknownGuildBank(t *testing.T) {
	var out []benchRow
	for _, table := range []string{"guild_bank", "guild_bank_tabs"} {
		_, err := usabStore().Filter(table, store.Range{Column: "id", Min: int64(0), Max: int64(1)})
		out = append(out, benchRow{"a table of " + strings.ReplaceAll(table, "_", " "), fmt.Sprint(err != nil), "no such table in the dataset", gpNoneSrc, resultIf(err != nil, "a table was invented")})
	}
	ds := usabStore()
	chars := []character.Character{bindChar(t, "Wara", "WARRIOR", 40, "Robe of Power", "Feathered Mantle", "Heavy Mithril Shoulder", "Pendant of Myzrael"), bindChar(t, "Rogua", "ROGUE", 30)}
	r := inventory.ReviewAll(chars, ds)
	var all []string
	for _, l := range r.Lines {
		all = append(all, fmt.Sprintf("%s %s %s", l.Action, l.To, l.Reason))
	}
	txt := strings.ToLower(strings.Join(all, "\n") + "\n" + inventory.RenderAll(r, 1) + strings.Join(r.Notes, " "))
	out = append(out, benchRow{"the bag review of four items and two characters", fmt.Sprintf("%d lines, names a guild bank %v", len(r.Lines), strings.Contains(txt, "guild")), "no guild bank in the capture or the dataset: no item is sent to one", gpNoneSrc,
		resultIf(len(r.Lines) >= 4 && !strings.Contains(txt, "guild"), "an item was sent to a guild bank")})
	for _, q := range []string{"how many tabs does the guild bank have?", "how much does the next guild bank tab cost?"} {
		cl := ask.Classify(q, ds, nil)
		out = append(out, benchRow{"the question \"" + q + "\"", fmt.Sprintf("kind %d, topic %q", cl.Kind, cl.Topic), "a know-how question with no dataset fact behind it", gpNoneSrc,
			resultIf(cl.Kind == ask.KindKnowHow && cl.Topic == ask.TopicNone, "a guild bank question answered from a topic that holds none")})
	}
	finish(t, "advice-unknown-guild-bank", "Offline guild bank: no answer in the dataset", out)
}

// TestOfflineBenchmarkUnknownRestedAndHearth: no rested XP rate, no hearthstone cooldown and no dungeon role in the dataset or in the facts the answer is
// framed with.
func TestOfflineBenchmarkUnknownRestedAndHearth(t *testing.T) {
	ds := levStore()
	var out []benchRow
	// a rested XP rate
	zero, size, rest := 0, 2100, 700
	c := levChar("MAGE", 12)
	c.XP, c.XPMax, c.RestedXP = &zero, &size, &rest
	line := gpProgress(c)
	out = append(out, benchRow{"how fast rested XP builds in an inn", line, "no rate in the dataset or the capture: the line states the pool, never a rate or a time to the cap", gpNoneSrc,
		resultIf(!strings.Contains(line, "%") && !strings.Contains(line, "hour") && !strings.Contains(line, "cap"), "a rested XP rate was stated")})
	cl := ask.Classify("how much rested xp do i get per hour in an inn?", ds, nil)
	out = append(out, benchRow{"the question about the rested XP rate", fmt.Sprintf("kind %d, topic %q", cl.Kind, cl.Topic), "a know-how question with no dataset fact behind it", gpNoneSrc, resultIf(cl.Kind == ask.KindKnowHow && cl.Topic == ask.TopicNone, "answered from a topic that holds a rate")})
	// the hearthstone
	hc := hearthCapture(t, "Orgrimmar")
	hb := brief.Character(hc, character.Client{}, time.Unix(0, 0))
	out = append(out, benchRow{"the hearthstone in the character brief", firstMatch(hb, "Hearthstone"), "the bind place only: the dataset holds no cooldown", gpNoneSrc, resultIf(strings.Contains(hb, "Hearthstone bound to: Orgrimmar") && !strings.Contains(strings.ToLower(firstMatch(hb, "Hearthstone")), "minute"), "a cooldown was stated")})
	cl = ask.Classify("how long is the hearthstone cooldown?", ds, nil)
	out = append(out, benchRow{"the question about the hearthstone cooldown", fmt.Sprintf("kind %d, topic %q", cl.Kind, cl.Topic), "a know-how question with no dataset fact behind it", gpNoneSrc, resultIf(cl.Kind == ask.KindKnowHow && cl.Topic == ask.TopicNone, "answered from a topic that holds a cooldown")})
	// the roles a dungeon needs
	p, togo, err := dungeons.Named(levChar("WARRIOR", 20), ds, nil, "Wailing Caverns")
	words := ""
	if p != nil {
		words = strings.ToLower(dungeons.RenderNamed(levChar("WARRIOR", 20), *p, togo))
	}
	out = append(out, benchRow{"Wailing Caverns: the roles a group needs", firstMatch(words, "wailing caverns"), "instances holds a size and a level range, no tank, healer or damage count", gpNoneSrc,
		resultIf(err == nil && p != nil && !strings.Contains(words, "tank") && !strings.Contains(words, "healer"), "a group composition was stated")})
	cl = ask.Classify("do i need a tank and a healer for wailing caverns?", ds, nil)
	out = append(out, benchRow{"the question about the roles of Wailing Caverns", fmt.Sprintf("kind %d, topic %q, subject %q", cl.Kind, cl.Topic, cl.Subject), "the dungeon's own facts (range, entrance, size), with no role count among them", gpNoneSrc,
		resultIf(cl.Topic == ask.TopicDungeon && cl.Subject == "Wailing Caverns", "the dungeon was not found")})
	rows, _ := ds.Filter("instances", store.Range{Column: "name", Min: "Wailing Caverns", Max: "Wailing Caverns"})
	var cols []string
	if len(rows) == 1 {
		for col := range rows[0] {
			l := strings.ToLower(col)
			if strings.Contains(l, "role") || strings.Contains(l, "tank") || strings.Contains(l, "heal") || strings.Contains(l, "group") {
				cols = append(cols, col)
			}
		}
	}
	out = append(out, benchRow{"instances columns about a group's roles", fmt.Sprintf("%v", cols), "none", gpNoneSrc, resultIf(len(rows) == 1 && len(cols) == 0, "the dataset holds a role column the answer could state")})
	finish(t, "advice-unknown-rested-hearth-roles", "Offline rested XP, hearthstone and dungeon roles: no answer in the dataset", out)
}

package paths_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/ask"
	"github.com/WoW-Companion/advisor/internal/gather"
	"github.com/WoW-Companion/advisor/internal/learn"
	"github.com/WoW-Companion/advisor/internal/store"
)

// The offline zone level, faction territory and capital city service benchmark: what the advisor puts in front of the model for "where should I level at level N", for a
// question on whether a zone is contested, hostile or one side's, and for the services of a capital (bank, auction house, flight master, profession trainer), beside the
// dataset's rows and the game's own brackets, with no network. It reads testdata/places_snapshot (quests_observed, trainers) and testdata/world_snapshot (ui_maps). The dataset
// holds no zone level range, no zone faction or contested flag, no bank, auction house or flight master row; the one place it hints at a zone's level and side is the level and
// faction of the quests players observed under a zone heading, which the cases print beside the game's value. A case the data cannot answer checks that the advisor says so and
// states none. A difference the dataset's own rows cause is marked "(data)". make offline-benchmark runs it; WOWC_BENCH_REPORT receives the tables.

const (
	zlLevelSrc   = "dataset ui_maps (no level column), quests_observed (level, category = the zone heading, faction); Warcraft Wiki zone level brackets (Classic)"
	zlSideSrc    = "dataset ui_maps (no faction column), quests_observed (faction, category), trainers (faction always empty); Warcraft Wiki, Zone: Horde, Alliance and contested zones (Classic)"
	zlCitySrc    = "dataset: no bank, auction house or flight master row; trainers (profession, zone_name); Warcraft Wiki, Capital cities (Orgrimmar, Undercity, Thunder Bluff, Stormwind City, Ironforge, Darnassus)"
	zlNoAnswer   = "the dataset has no such row, column or table"
	zlSideOfZone = "dataset quests_observed (faction, category); the advisor's home-zone list (gather.SideOfZone)"
)

// zlQuestSpan reads the quests players observed under a zone heading for a side: how many, and the lowest and highest quest level. A quest with no faction is open to both.
func zlQuestSpan(t *testing.T, zone, side string) (n, lo, hi int64) {
	t.Helper()
	rows, err := store.NewDir(qbPlacesDir).Filter("quests_observed", store.Range{Column: "quest_id", Min: int64(0), Max: int64(1 << 40)})
	if err != nil {
		t.Fatal(err)
	}
	lo = 1 << 20
	for _, r := range rows {
		f := strings.ToLower(text(r["faction"]))
		if text(r["category"]) != zone || f != "" && side != "" && f != strings.ToLower(side) {
			continue
		}
		n++
		if l, ok := r["level"].(int64); ok && l > 0 {
			lo, hi = min(lo, l), max(hi, l)
		}
	}
	if hi == 0 {
		lo = 0
	}
	return
}

// zlSides counts the quests observed under a zone heading that name a side.
func zlSides(t *testing.T, zone string) (alliance, horde int64) {
	t.Helper()
	rows, err := store.NewDir(qbPlacesDir).Filter("quests_observed", store.Range{Column: "quest_id", Min: int64(0), Max: int64(1 << 40)})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if text(r["category"]) != zone {
			continue
		}
		switch strings.ToLower(text(r["faction"])) {
		case "alliance":
			alliance++
		case "horde":
			horde++
		}
	}
	return
}

func zlSaid(f, topic string) bool {
	return strings.Contains(f, topic) && strings.Contains(f, "Say so") && !evDigit.MatchString(strings.ReplaceAll(f, "ui_maps", ""))
}

var zlLevelCases = []struct{ side, q, zones, bracket string }{
	{"Alliance", "where should I level at level 15?", "Westfall;Redridge Mountains", "Westfall 10-20, Redridge Mountains 15-27"},
	{"Horde", "which zones are good for a level 12 horde character?", "Silverpine Forest;The Barrens", "Silverpine Forest 10-20, The Barrens 10-25"},
	{"Alliance", "what zone should I quest in at level 25?", "Wetlands;Duskwood", "Wetlands 20-30, Duskwood 18-30"},
	{"Horde", "where is a good place to grind at level 22?", "Stonetalon Mountains;Ashenvale", "Stonetalon Mountains 15-27, Ashenvale 18-30"},
	{"Horde", "what is the best zone for level 32 horde?", "Stranglethorn Vale;Thousand Needles", "Stranglethorn Vale 30-45, Thousand Needles 25-35"},
	{"Alliance", "where should I go next, I'm level 18", "Duskwood;Redridge Mountains", "Duskwood 18-30, Redridge Mountains 15-27"},
	{"Horde", "where should I level at level 8?", "Tirisfal Glades;Mulgore", "Tirisfal Glades 1-10, Mulgore 1-10"},
}

// TestOfflineBenchmarkWhereToLevel: "where should I level at level N" in the words a player uses gets the line that the data holds no zone level range, whichever side asks, and
// no zone is named from memory. Before the fix none of these words reached the zone level line.
func TestOfflineBenchmarkWhereToLevel(t *testing.T) {
	cols := evColumns(worldStore(), "ui_maps", "ui_map_id")
	lv := evAnyLike(cols, "level", "min", "max", "bracket")
	var out []benchRow
	for _, tc := range zlLevelCases {
		f := ask.LimitFacts(tc.q)
		var spans []string
		for _, z := range strings.Split(tc.zones, ";") {
			n, lo, hi := zlQuestSpan(t, z, tc.side)
			spans = append(spans, fmt.Sprintf("%s: %d observed quests, levels %d-%d", z, n, lo, hi))
		}
		res := resultIf(zlSaid(f, "Zone levels:") && len(lv) == 0, "no statement that the data holds no zone level range, or a figure in it, or ui_maps has a level column")
		if res == "match" {
			res = "differ (data): the dataset has no zone level range; the game's bracket is " + tc.bracket
		}
		out = append(out, benchRow{fmt.Sprintf("%s: %q", strings.ToLower(tc.side), tc.q), firstLine(f), fmt.Sprintf("ui_maps level columns %v; quests_observed %s; game: %s", lv, strings.Join(spans, "; "), tc.bracket), zlLevelSrc, res})
	}
	finish(t, "advice-where-to-level", "Offline zones: where to level at a level, both sides", out)
}

// TestOfflineBenchmarkWhereToLevelNotProfession: a level question about a profession, a quest list or a route is no zone question and gets no zone line.
func TestOfflineBenchmarkWhereToLevelNotProfession(t *testing.T) {
	var out []benchRow
	for _, q := range []string{"where do I level my mining?", "where do I level up my first aid to 150?", "what quests should I do at level 20?", "where can I mine copper in The Barrens as a horde?"} {
		f := ask.LimitFacts(q)
		out = append(out, benchRow{fmt.Sprintf("%q", q), "no zone level line: " + fmt.Sprint(f == ""), "not a zone bracket question: no zone line", zlLevelSrc, resultIf(!strings.Contains(f, "Zone levels:"), "a zone level line for a question that is none")})
	}
	finish(t, "advice-where-to-level-other", "Offline zones: questions that are not about a zone's level", out)
}

var zlTerritoryCases = []struct{ side, q, zone, game string }{
	{"Horde", "is Ashenvale contested territory for the Horde?", "Ashenvale", "contested: Horde (Astranaar is Alliance-held, Splintertree Post and Zoram'gar Horde)"},
	{"Alliance", "can I quest in Hillsbrad Foothills as Alliance?", "Hillsbrad Foothills", "contested: Southshore Alliance, Tarren Mill Horde"},
	{"Alliance", "is Stranglethorn Vale hostile for me?", "Stranglethorn Vale", "contested: Booty Bay is neutral, Grom'gol Horde, Nesingwary's camp Alliance"},
	{"Horde", "is Redridge Mountains an Alliance zone?", "Redridge Mountains", "Alliance zone: Lakeshire guards attack a Horde character"},
	{"Alliance", "will I get attacked by guards in The Barrens?", "The Barrens", "Horde zone: Crossroads and Camp Taurajo are Horde towns, Ratchet is neutral"},
	{"Horde", "can I go to Duskwood as Horde?", "Duskwood", "Alliance zone: Darkshire guards attack a Horde character"},
}

// TestOfflineBenchmarkZoneTerritory: a question on whether a zone is contested, hostile or one side's gets the line that the data holds no zone control or hostility flag,
// and no side is named from memory. The quests players observed under the zone heading are printed beside the game's value. Before the fix none of these questions reached any line.
func TestOfflineBenchmarkZoneTerritory(t *testing.T) {
	cols := evColumns(worldStore(), "ui_maps", "ui_map_id")
	fac := evAnyLike(cols, "faction", "side", "contested", "hostile")
	var out []benchRow
	for _, tc := range zlTerritoryCases {
		f := ask.LimitFacts(tc.q)
		a, h := zlSides(t, tc.zone)
		res := resultIf(zlSaid(f, "Zone territory:") && len(fac) == 0, "no statement that the data holds no zone control flag, or a figure in it, or ui_maps has a faction column")
		if res == "match" {
			res = "differ (data): the dataset has no zone control flag; the game says " + tc.game
		}
		out = append(out, benchRow{fmt.Sprintf("%s: %q", strings.ToLower(tc.side), tc.q), firstLine(f), fmt.Sprintf("ui_maps faction columns %v; quests_observed %s: %d alliance, %d horde; game: %s", fac, tc.zone, a, h, tc.game), zlSideSrc, res})
	}
	finish(t, "advice-zone-territory", "Offline zones: faction territory, contested and hostile zones", out)
}

// TestOfflineBenchmarkZoneTerritoryNotAsked: a question that names a zone and a side without asking who holds it gets no territory line.
func TestOfflineBenchmarkZoneTerritoryNotAsked(t *testing.T) {
	var out []benchRow
	for _, q := range []string{"where can I mine copper in The Barrens as a horde?", "I am level 30 in Duskwood, what quests are there?", "how do I get to Winterspring?"} {
		f := ask.LimitFacts(q)
		out = append(out, benchRow{fmt.Sprintf("%q", q), "no territory line: " + fmt.Sprint(!strings.Contains(f, "Zone territory:")), "not a territory question", zlSideSrc, resultIf(!strings.Contains(f, "Zone territory:"), "a territory line for a question that is none")})
	}
	finish(t, "advice-zone-territory-other", "Offline zones: questions that are not about who holds a zone", out)
}

// TestOfflineBenchmarkHomeZoneSides: the zones the advisor counts as one side's home (gather.SideOfZone) against the faction of the quests players observed there: a home
// zone with quests of the other side would send that side away from a place it is offered quests in.
func TestOfflineBenchmarkHomeZoneSides(t *testing.T) {
	var out []benchRow
	for _, zone := range []string{"Orgrimmar", "Thunder Bluff", "Undercity", "Tirisfal Glades", "Silverpine Forest", "Mulgore", "Stormwind City", "Ironforge", "Dun Morogh", "Elwynn Forest", "Westfall", "Teldrassil"} {
		side := gather.SideOfZone(zone)
		a, h := zlSides(t, zone)
		other := map[string]int64{"horde": a, "alliance": h}[side]
		own := map[string]int64{"horde": h, "alliance": a}[side]
		out = append(out, benchRow{zone + ": which side's home", side, fmt.Sprintf("quests_observed: %d alliance, %d horde", a, h), zlSideOfZone, resultIf(side != "" && own > 0 && other == 0, "the zone is another side's, or neutral, or has quests of the other side")})
	}
	finish(t, "advice-home-zone-sides", "Offline zones: the advisor's home zones against the observed quests' side", out)
}

var zlServiceCases = []struct{ side, q, topic, game string }{
	{"Horde", "where is the nearest bank?", "Capital city services:", "Orgrimmar (Valley of Strength), Thunder Bluff (Middle Rise), Undercity (Trade Quarter)"},
	{"Alliance", "which city has the auction house for Alliance?", "Capital city services:", "Stormwind City (Trade District), Ironforge (Commons), Darnassus (Craftsmen's Terrace)"},
	{"Horde", "where is the flight master in Orgrimmar?", "Flight paths:", "Orgrimmar: Doras, the wind rider master, atop the Valley of Strength tower"},
	{"Alliance", "where is the closest innkeeper?", "Capital city services:", "an inn in every capital and in most towns"},
	{"Horde", "which city is closest to The Barrens for the Horde?", "Capital city services:", "Orgrimmar, by the Crossroads road (Warcraft Wiki, Orgrimmar)"},
	{"Alliance", "is there a neutral auction house?", "Capital city services:", "Booty Bay, Gadgetzan, Everlook (Warcraft Wiki, Auction House)"},
}

// TestOfflineBenchmarkCapitalServiceAsks: the nearest bank, auction house, flight master or inn, and the nearest capital or a neutral auction house, in a player's words, get the line
// that the data holds none of them and names no NPC or position. Before the fix the nearest-capital and neutral auction house questions reached no line.
func TestOfflineBenchmarkCapitalServiceAsks(t *testing.T) {
	ds := worldStore()
	var out []benchRow
	for _, tc := range zlServiceCases {
		f := ask.LimitFacts(tc.q)
		_, errBank := ds.Filter("banks", store.Range{Column: "zone_id", Min: int64(0)})
		_, errAH := ds.Filter("auction_houses", store.Range{Column: "zone_id", Min: int64(0)})
		res := resultIf(zlSaid(f, tc.topic) && errBank != nil && errAH != nil, "no statement that the data holds none, or a figure in it, or a bank or auction house table exists")
		if res == "match" {
			res = "differ (data): the dataset has no such row; the game has " + tc.game
		}
		out = append(out, benchRow{fmt.Sprintf("%s: %q", strings.ToLower(tc.side), tc.q), firstLine(f), "tables banks and auction_houses: none; game: " + tc.game, zlCitySrc, res})
	}
	finish(t, "advice-capital-service-asks", "Offline capital cities: nearest bank, auction house, flight master, inn and capital", out)
}

var zlTrainerCases = []struct{ faction, city, prof string }{
	{"Alliance", "Stormwind City", "First Aid"}, {"Horde", "Orgrimmar", "Mining"}, {"Alliance", "Ironforge", "Cooking"}, {"Horde", "Thunder Bluff", "Skinning"},
}

// TestOfflineBenchmarkCapitalProfessionTrainerFirst: the profession trainer the advisor names first for a character standing in a capital is the dataset's trainer in that
// capital, and none of the others is a capital of the other side.
func TestOfflineBenchmarkCapitalProfessionTrainerFirst(t *testing.T) {
	ds := rcNamed{store.NewDir(qbPlacesDir)}
	var out []benchRow
	for _, tc := range zlTrainerCases {
		rows, _ := ds.Filter("trainers", store.Range{Column: "profession", Min: tc.prof, Max: tc.prof})
		var want []string
		for _, r := range rows {
			if text(r["zone_name"]) == tc.city {
				want = append(want, fmt.Sprintf("Synthetic Trainer %d", num(r["npc_id"])))
			}
		}
		other := rcHordeCaps
		if tc.faction == "Horde" {
			other = rcAllyCaps
		}
		ways := learn.ProfessionTrainers(rcAt(tc.faction, tc.city), ds, nil, tc.prof)
		var lines, wrong []string
		for _, w := range ways {
			lines = append(lines, w.Line)
			if slices.Contains(other, w.Zone) {
				wrong = append(wrong, w.Zone)
			}
		}
		ok := len(want) > 0 && len(ways) > 0 && ways[0].Zone == tc.city && slices.Contains(want, ways[0].Name) && len(wrong) == 0
		out = append(out, benchRow{fmt.Sprintf("%s in %s: nearest %s trainer", strings.ToLower(tc.faction), tc.city, tc.prof), strings.Join(lines, "; "), fmt.Sprintf("trainers rows in %s: %s", tc.city, strings.Join(want, ", ")), zlCitySrc,
			resultIf(ok, "the capital's trainer is not first, or another side's capital is offered")})
	}
	finish(t, "advice-capital-trainer-first", "Offline capital cities: the nearest profession trainer", out)
}

// TestOfflineBenchmarkZoneCityNoAnswer: what the dataset cannot answer: a zone it has no row for, a zone question with no zone, a barber shop, a territory question about a zone the
// dataset has no quests in, and a profession whose trainer the dataset lacks. The advisor says so and states none.
func TestOfflineBenchmarkZoneCityNoAnswer(t *testing.T) {
	zones := worldZones(t)
	var out []benchRow
	for _, q := range []string{"what level range is the zone Gilneas?", "which zone has the best xp per hour at level 40?"} {
		f := ask.LimitFacts(q)
		out = append(out, benchRow{fmt.Sprintf("%q", q), firstLine(f), "no zone level range, bracket or xp rate row", zlLevelSrc, resultIf(zlSaid(f, "Zone levels:"), "no statement that the data holds no zone level range, or a figure in it")})
	}
	_, ok := zones["Gilneas"]
	out = append(out, benchRow{"Gilneas: a ui_maps row", fmt.Sprint(ok), "none: not a Classic zone", zlNoAnswer, resultIf(!ok, "a zone row was invented")})
	q := "is Alterac Mountains hostile to the Horde?"
	f := ask.LimitFacts(q)
	out = append(out, benchRow{fmt.Sprintf("%q", q), firstLine(f), "no zone control flag; the observed quests are split between the sides", zlSideSrc, resultIf(zlSaid(f, "Zone territory:"), "no statement that the data holds no zone control flag, or a figure in it")})
	q = "where is the barber shop in Stormwind?"
	f = ask.LimitFacts(q)
	out = append(out, benchRow{fmt.Sprintf("%q", q), firstLine(f), "no barber, bank or service row for a capital", zlCitySrc, resultIf(zlSaid(f, "Capital city services:"), "no statement that the data holds none, or a figure in it")})
	ds := rcNamed{store.NewDir(qbPlacesDir)}
	rows, _ := ds.Filter("trainers", store.Range{Column: "profession", Min: "Blacksmithing", Max: "Blacksmithing"})
	ways := learn.ProfessionTrainers(rcAt("Horde", "Orgrimmar"), ds, nil, "Blacksmithing")
	out = append(out, benchRow{"horde in Orgrimmar: nearest Blacksmithing trainer", fmt.Sprintf("%d trainers named", len(ways)), fmt.Sprintf("%d trainers rows for Blacksmithing", len(rows)), zlCitySrc,
		resultIf(len(ways) == 0 && len(rows) == 0, "a trainer was named for a profession the dataset has none for")})
	finish(t, "advice-zone-city-no-answer", "Offline zones and capitals: no answer in the dataset", out)
}

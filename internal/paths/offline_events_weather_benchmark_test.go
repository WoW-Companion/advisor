package paths_test

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/WoW-Companion/advisor/internal/ask"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/digest"
	"github.com/WoW-Companion/advisor/internal/questie"
	"github.com/WoW-Companion/advisor/internal/store"
)

// The offline holiday event, weather, time-of-day and zone-level benchmark: what the advisor answers about which event a player
// names, when an event runs, which quests and items an event offers, and what it says of weather, day and night and a zone's level
// range, beside the dataset's rows and the game's calendar, with no network. It reads testdata/consumables_snapshot (the event
// items), testdata/places_snapshot (creatures and quest titles) and testdata/world_snapshot (ui_maps zones). The dataset holds no
// event table, no event date, no event vendor or reward link, no weather, no day and night cycle, no spawn timer and no zone level
// range, so the cases it cannot answer check that the advisor says so and states none. A difference the dataset's own rows cause is
// marked "(data)". make offline-benchmark runs it; WOWC_BENCH_REPORT receives the tables.

const (
	evCalSrc   = "game holiday calendar (Blizzard news and Wowhead event guides); the dataset has no event table"
	evItemSrc  = "dataset items (name, required_level), vendor_sources, loot_sources: no row ties an item to an event"
	evNoneSrc  = "dataset tables and columns: no event, weather, time-of-day, spawn timer or zone level range"
	evZoneSrc  = "dataset ui_maps (name, type: no level column); Wowhead zone level brackets"
	evNameSrc  = "dataset ui_maps zones, npcs and items: names that share a word with an event are not the event"
	evClockSrc = "the capture's own time (character CapturedAt), which is the player's clock"
)

var evDigit = regexp.MustCompile(`[0-9%]`)

func evAt(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 12, 0, 0, 0, time.UTC) }

func evKeys(t time.Time) string {
	var ks []string
	for _, e := range questie.RunningEvents(t) {
		ks = append(ks, e.Key)
	}
	return strings.Join(ks, ", ")
}

// evColumns is the set of column names of the first row of a table, "" when the table is absent.
func evColumns(ds store.DataStore, table, col string) []string {
	rows, err := ds.Filter(table, store.Range{Column: col, Min: int64(0)})
	if err != nil || len(rows) == 0 {
		return nil
	}
	var out []string
	for k := range rows[0] {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// evAnyLike is the columns whose name carries one of the words.
func evAnyLike(cols []string, words ...string) []string {
	var out []string
	for _, c := range cols {
		for _, w := range words {
			if strings.Contains(c, w) {
				out = append(out, c)
			}
		}
	}
	return out
}

var evPhrases = []struct{ q, key, ref string }{
	{"what does the Feast of Winter Veil give?", "winter-veil", "Feast of Winter Veil"},
	{"is Hallow’s End on?", "hallows-end", "Hallow's End (typographic apostrophe)"},
	{"when is the Fire Festival?", "midsummer", "Midsummer Fire Festival, as players call it"},
	{"how do I do Children's Week?", "childrens-week", "Children's Week"},
	{"where is the Brewfest vendor?", "brewfest", "Brewfest"},
	{"what is love is in the air?", "love-is-in-the-air", "Love is in the Air"},
	{"how long does the lunar festival last?", "lunar-festival", "Lunar Festival"},
	{"noble garden egg locations", "noblegarden", "Noblegarden, spelled as two words"},
}

// TestOfflineBenchmarkEventPlayerWords: the event the advisor names for the words a player puts in a question, beside the game's
// holiday names.
func TestOfflineBenchmarkEventPlayerWords(t *testing.T) {
	var out []benchRow
	for _, tc := range evPhrases {
		e, ok := questie.FindEvent(tc.q)
		out = append(out, benchRow{fmt.Sprintf("%q: the event named", tc.q), e.Key, tc.key + " (" + tc.ref + ")", evCalSrc, resultIf(ok && e.Key == tc.key, "the wrong event or none")})
	}
	finish(t, "advice-event-player-words", "Offline world events: the event a player's words name", out)
}

// TestOfflineBenchmarkEventNotNamed: a zone, a creature or an item that shares a word with an event is not that event: every zone of ui_maps,
// every creature and item name of the snapshots that holds no whole event name names no event, and the creatures and items that do
// hold one name that event.
func TestOfflineBenchmarkEventNotNamed(t *testing.T) {
	var out []benchRow
	zones := worldZones(t)
	var badZones []string
	for z := range zones {
		if _, ok := questie.FindEvent(z); ok {
			badZones = append(badZones, z)
		}
	}
	slices.Sort(badZones)
	_, hasWinterspring := zones["Winterspring"]
	out = append(out, benchRow{fmt.Sprintf("all %d zones of ui_maps: an event named", len(zones)), fmt.Sprintf("%d zones name an event %v", len(badZones), badZones), fmt.Sprintf("none; Winterspring is a zone row: %v", hasWinterspring), evNameSrc, resultIf(len(badZones) == 0 && hasWinterspring, "a zone name was taken for an event")})
	for _, name := range []string{"Winterspring", "Harvest Golem", "Father Winter's Helper", "Winter Reindeer", "Lunar Lantern", "Hallowed Wand - Bat", "Darkmoon Special Reserve"} {
		e, ok := questie.FindEvent(name)
		out = append(out, benchRow{name + ": an event named", fmt.Sprintf("%v %q", ok, e.Key), "none: a zone, creature or item, not an event", evNameSrc, resultIf(!ok, "a name that is no event name was taken for "+e.Key)})
	}
	for _, tc := range []struct{ name, key string }{{"Winter Veil Cookie", "winter-veil"}, {"Winter Veil Roast", "winter-veil"}, {"Hallow's End Pumpkin Treat", "hallows-end"}, {"Lunar Festival Invitation", "lunar-festival"}} {
		e, ok := questie.FindEvent(tc.name)
		out = append(out, benchRow{tc.name + ": an event named", e.Key, tc.key + " (the item's own name holds the event's)", evNameSrc, resultIf(ok && e.Key == tc.key, "the event in the item name was not found")})
	}
	finish(t, "advice-event-not-named", "Offline world events: names that are not an event", out)
}

// TestOfflineBenchmarkEventItems: the event items the dataset holds have a name, a level and no source; the advisor gives the dataset's
// tooltip and the line that the data names no event vendor or reward.
func TestOfflineBenchmarkEventItems(t *testing.T) {
	ds := consStore()
	var out []benchRow
	for _, name := range []string{"Winter Veil Cookie", "Winter Veil Roast", "Hallow's End Pumpkin Treat", "Midsummer Sausage", "Lunar Festival Invitation", "Hallowed Wand - Bat"} {
		row := rowByName(t, ds, "items", name)
		id := num(row["id"])
		vend, _ := ds.Filter("vendor_sources", rangeInt("item_id", id))
		loot, _ := ds.Filter("loot_sources", rangeInt("item_id", id))
		tip := ask.ItemTooltipFacts(ds, "can I trade ["+name+"]?")
		facts := ask.LimitFacts("which holiday event vendor sells " + name + " and what does it cost?")
		said := strings.Contains(facts, "Holiday and world events:") && strings.Contains(facts, "Say so") && !evDigit.MatchString(facts)
		out = append(out, benchRow{name + ": its source", fmt.Sprintf("tooltip %v; says the data names no event source %v", strings.Contains(tip, name), said),
			fmt.Sprintf("dataset: item row, required level %d, %d vendor rows, %d loot rows; the game sells or drops it in its event", num(row["required_level"]), len(vend), len(loot)), evItemSrc,
			resultIf(said && strings.Contains(tip, name), "the advisor did not say that the data names no event source")})
		if len(vend)+len(loot) == 0 {
			out[len(out)-1].result = "differ (data): the dataset names no event vendor or drop for the item (the answer says so)"
			if !said {
				out[len(out)-1].result = "differ: the advisor did not say that the data names no event source"
			}
		}
	}
	finish(t, "advice-event-items", "Offline world events: the items an event offers", out)
}

var evQuestCases = []struct {
	name   string
	now    time.Time
	set    questie.Seasonal
	level  int
	event  string
	want   bool
	reason string
}{
	{"Winter Veil quest, in the window, opted in", evAt(2026, 12, 20), "on", 30, "winter-veil", true, "the event runs and the player opted in"},
	{"Winter Veil quest, in the window, not opted in", evAt(2026, 12, 20), "", 30, "winter-veil", false, "events are opt-in"},
	{"Winter Veil quest in July", evAt(2026, 7, 10), "on", 30, "winter-veil", false, "the event is not running"},
	{"Midsummer quest on its first day", evAt(2026, 6, 21), "on:midsummer", 30, "midsummer", true, "Midsummer runs from June 21"},
	{"Midsummer quest the day before", evAt(2026, 6, 20), "on:midsummer", 30, "midsummer", false, "Midsummer has not begun"},
	{"Midsummer quest opted in to Brewfest only", evAt(2026, 6, 25), "on:brewfest", 30, "midsummer", false, "the opt-in names another event"},
	{"Brewfest quest above the player's level", evAt(2026, 9, 25), "on", 20, "brewfest", false, "a quest above the player's level"},
	{"Brewfest quest during the Harvest Festival overlap", evAt(2026, 9, 25), "on:brewfest", 30, "brewfest", true, "Brewfest and the Harvest Festival run together"},
	{"Winter Veil quest on January 2", evAt(2027, 1, 2), "on", 30, "winter-veil", true, "the window runs over New Year to January 2"},
	{"Winter Veil quest on January 3", evAt(2027, 1, 3), "on", 30, "winter-veil", false, "the window ended"},
}

// TestOfflineBenchmarkEventQuestOffer: whether a holiday quest is offered by date, setting and level, beside the game's rule that an event quest is offered only
// while its event runs.
func TestOfflineBenchmarkEventQuestOffer(t *testing.T) {
	var out []benchRow
	for _, tc := range evQuestCases {
		q := &questie.Quest{ID: 1, Name: "Event quest", Level: 25, Event: tc.event, Starters: []questie.Ref{{ID: 1}}}
		w := questie.Who{Level: tc.level, Done: map[int]bool{}, InLog: map[int]bool{}, Above: 2, Below: 10, Now: tc.now, Seasonal: tc.set}
		got := q.Eligible(w)
		out = append(out, benchRow{tc.name, fmt.Sprint(got), fmt.Sprintf("%v: %s", tc.want, tc.reason), evCalSrc, resultIf(got == tc.want, "the offer does not follow the calendar and the setting")})
	}
	finish(t, "advice-event-quest-offer", "Offline world events: which quests an event offers", out)
}

// TestOfflineBenchmarkEventQuestNames: a quest that carries no sort id is tied to its event by its name; ordinary quests of the dataset, whose names share a word with an event
// ("Love's Gift", "The Sacred Flame"), are not.
func TestOfflineBenchmarkEventQuestNames(t *testing.T) {
	var out []benchRow
	for _, tc := range []struct{ name, key string }{
		{"Flickering Flames in Kalimdor", "midsummer"}, {"Honor the Flame", "midsummer"}, {"Desecrate this Fire!", "midsummer"},
		{"The Feast of Winter Veil", "winter-veil"}, {"Hallow's End Treats", "hallows-end"}, {"Brewfest Prize Tickets", "brewfest"},
	} {
		got := questie.EventOfName(tc.name)
		out = append(out, benchRow{tc.name + ": its event", got, tc.key, evCalSrc, resultIf(got == tc.key, "the quest is not tied to its event")})
	}
	for _, tc := range []struct{ id int64 }{{1195}, {1512}, {93552}} {
		rows, err := qbStore().Filter("quests_observed", rangeInt("quest_id", tc.id))
		title := ""
		if len(rows) > 0 {
			title = text(rows[0]["title"])
		}
		if err != nil || title == "" {
			t.Fatalf("quest %d: not in the snapshot (%v)", tc.id, err)
		}
		got := questie.EventOfName(title)
		out = append(out, benchRow{fmt.Sprintf("%s (quest %d): its event", title, tc.id), fmt.Sprintf("%q", got), "none: an ordinary quest of the dataset", "dataset quests_observed (title)", resultIf(got == "", "an ordinary quest was tied to "+got)})
	}
	finish(t, "advice-event-quest-names", "Offline world events: a quest's event by its name", out)
}

// TestOfflineBenchmarkEventCalendar: which events run on the days around the overlap of the Harvest Festival and Brewfest, over New Year and in the empty months, beside the
// game's calendar, and the clock the advisor reads (the capture's time, not the machine's).
func TestOfflineBenchmarkEventCalendar(t *testing.T) {
	var out []benchRow
	for _, tc := range []struct {
		day  time.Time
		want string
		ref  string
	}{
		{evAt(2026, 9, 25), "harvest-festival, brewfest", "Harvest Festival (September 22 to 29) falls inside Brewfest (September 20 to October 6)"},
		{evAt(2026, 10, 3), "brewfest", "Brewfest after the Harvest Festival ends"},
		{evAt(2026, 10, 20), "hallows-end", "Hallow's End from October 18"},
		{evAt(2026, 11, 15), "", "no holiday event in mid November"},
		{evAt(2027, 1, 1), "winter-veil", "Winter Veil over New Year"},
		{evAt(2026, 8, 15), "", "no holiday event in mid August"},
	} {
		got := evKeys(tc.day)
		out = append(out, benchRow{tc.day.Format("January 2") + ": events running", got, tc.want + " (" + tc.ref + ")", evCalSrc, resultIf(got == tc.want, "the running events differ from the calendar")})
	}
	// the capture's time is the player's clock, whatever the machine's date is
	dec := evAt(2026, 12, 20).Unix()
	c := character.Character{CapturedAt: &dec}
	clock := digest.ClockOf(c, evAt(2026, 7, 1))
	out = append(out, benchRow{"a capture from December 20, read on July 1", evKeys(clock), "winter-veil (the capture's date decides)", evClockSrc, resultIf(evKeys(clock) == "winter-veil", "the machine's date was used")})
	none := digest.ClockOf(character.Character{}, evAt(2026, 7, 1))
	out = append(out, benchRow{"a character with no capture time: the clock", evKeys(none), "midsummer (the machine's date, the only one there is)", evClockSrc, resultIf(evKeys(none) == "midsummer", "no clock fallback")})
	e, _ := questie.EventByKey("brewfest")
	out = append(out, benchRow{"the line shown while Brewfest runs", e.Mention(), "names the event and its last day: Brewfest, October 6", evCalSrc, resultIf(strings.Contains(e.Mention(), "Brewfest is on (until October 6)"), "the mention line")})
	finish(t, "advice-event-calendar", "Offline world events: where and when an event runs", out)
}

var evLimitQs = []string{
	"what does Brewfest give?",
	"which vendor sells the Winter Veil reindeer and what does it cost?",
	"when does the Darkmoon Faire come to Elwynn Forest?",
	"what quests does Hallow's End offer a level 30?",
	"which holiday is on now and where do I go?",
	"how long does the Lunar Festival run and what is the reward?",
}

// TestOfflineBenchmarkEventLimits: a question about an event's dates, vendors or rewards, or the Darkmoon Faire, gets the line that the data holds no
// event table and states no date, price or reward; the dataset has no table or column of one.
func TestOfflineBenchmarkEventLimits(t *testing.T) {
	ds := consStore()
	var out []benchRow
	var tables []string
	for _, tb := range []string{"events", "holidays", "world_events", "holiday_events", "event_vendors", "darkmoon_faire"} {
		if _, err := ds.Filter(tb, store.Range{Column: "id", Min: int64(0)}); err == nil {
			tables = append(tables, tb)
		}
	}
	cols := evColumns(ds, "items", "id")
	like := evAnyLike(cols, "event", "holiday", "season")
	for _, q := range evLimitQs {
		f := ask.LimitFacts(q)
		ok := strings.Contains(f, "Holiday and world events:") && strings.Contains(f, "Say so") && !evDigit.MatchString(f) && len(tables) == 0 && len(like) == 0
		out = append(out, benchRow{fmt.Sprintf("%q", q), firstLine(f), fmt.Sprintf("tables: %v; items columns about an event: %v; no date, vendor or reward is stated", tables, like), evNoneSrc,
			resultIf(ok, "no statement that the data holds no event table, or a figure in it, or the dataset has a table or column")})
	}
	_, found := questie.FindEvent("Darkmoon Faire")
	out = append(out, benchRow{"Darkmoon Faire: an event", fmt.Sprint(found), "none: no event row, and it moves monthly in the game", evCalSrc, resultIf(!found, "an event was matched")})
	finish(t, "advice-event-limits", "Offline world events: dates, vendors and rewards the dataset lacks", out)
}

var evWeatherQs = []string{
	"what is the weather like in Tanaris?",
	"does it rain in Elwynn Forest?",
	"is there a day and night cycle in the game?",
	"which creatures only spawn at night?",
	"what time of day does the zeppelin leave?",
	"does the Silithus sandstorm happen during the day?",
}

// TestOfflineBenchmarkWeatherTimeOfDay: a question about weather, the day and night cycle or a night-only spawn gets the line that the data holds none, with no weather, hour or
// timer in it; the dataset's npcs and ui_maps rows carry no weather, time or spawn column.
func TestOfflineBenchmarkWeatherTimeOfDay(t *testing.T) {
	ds := usabStore()
	var out []benchRow
	npcCols := evColumns(ds, "npcs", "id")
	mapCols := evColumns(worldStore(), "ui_maps", "ui_map_id")
	like := append(evAnyLike(npcCols, "weather", "time", "spawn", "night", "day"), evAnyLike(mapCols, "weather", "time", "spawn", "night", "day")...)
	var tables []string
	for _, tb := range []string{"weather", "day_night", "spawns", "spawn_times", "creature_spawns"} {
		if _, err := ds.Filter(tb, store.Range{Column: "id", Min: int64(0)}); err == nil {
			tables = append(tables, tb)
		}
	}
	for _, q := range evWeatherQs {
		f := ask.LimitFacts(q)
		ok := strings.Contains(f, "Weather and time of day:") && strings.Contains(f, "Say so") && !evDigit.MatchString(f) && len(like) == 0 && len(tables) == 0
		out = append(out, benchRow{fmt.Sprintf("%q", q), firstLine(f), fmt.Sprintf("npcs columns %d, ui_maps columns %d, weather or time columns: %v, tables: %v; none is stated", len(npcCols), len(mapCols), like, tables), evNoneSrc,
			resultIf(ok, "no statement that the data holds none, or a figure in it, or the dataset has a weather or time column")})
	}
	finish(t, "advice-weather-time-of-day", "Offline weather and time of day: no answer in the dataset", out)
}

// TestOfflineBenchmarkNoFalseLimitFacts: a question that only shares a word with an event, weather or zone-level topic gets none of those lines.
func TestOfflineBenchmarkNoFalseLimitFacts(t *testing.T) {
	var out []benchRow
	for _, q := range []string{
		"how do I use Rain of Fire?", "what does a night elf hunter get?", "how do I get to Winterspring?", "where is the Harvest Golem?",
		"I am level 30 in Duskwood, what quests are there?", "what level do I learn Frostbolt?", "where do I get the Darkmoon Card trinket?", "is Blizzard a good mage spell?",
	} {
		f := ask.LimitFacts(q)
		bad := strings.Contains(f, "Holiday and world events:") || strings.Contains(f, "Weather and time of day:") || strings.Contains(f, "Zone levels:")
		out = append(out, benchRow{fmt.Sprintf("%q", q), fmt.Sprintf("%q", firstLine(f)), "no event, weather or zone level line: the words are a spell, a race, a zone, a creature or an item", evNameSrc, resultIf(!bad, "an event, weather or zone level line was added")})
	}
	finish(t, "advice-event-false-limits", "Offline events and weather: questions that only share a word", out)
}

var evZoneCases = []struct {
	q, zone, bracket string
}{
	{"what level is Duskwood for?", "Duskwood", "18 to 30"},
	{"what level range is Westfall?", "Westfall", "10 to 20"},
	{"which level should I be for Stranglethorn Vale?", "Stranglethorn Vale", "30 to 45"},
	{"what is the recommended level for Tanaris?", "Tanaris", "40 to 50"},
	{"what level is the zone Durotar?", "Durotar", "1 to 10"},
}

// TestOfflineBenchmarkZoneLevels: a question about the level a zone is for gets the line that the data holds no zone level range, and none is stated; ui_maps has the zone and no level.
func TestOfflineBenchmarkZoneLevels(t *testing.T) {
	zones := worldZones(t)
	cols := evColumns(worldStore(), "ui_maps", "ui_map_id")
	lv := evAnyLike(cols, "level", "min", "max", "bracket")
	var out []benchRow
	for _, tc := range evZoneCases {
		f := ask.LimitFacts(tc.q)
		_, has := zones[tc.zone]
		said := strings.Contains(f, "Zone levels:") && strings.Contains(f, "Say so") && !regexp.MustCompile(`\b\d`).MatchString(strings.ReplaceAll(f, "ui_maps", ""))
		res := resultIf(said && has && len(lv) == 0, "no statement that the data holds no zone level range, or a figure in it, or the zone row has a level")
		if res == "match" {
			res = fmt.Sprintf("differ (data): the dataset has the zone but no level range; the game's bracket is %s", tc.bracket)
		}
		out = append(out, benchRow{fmt.Sprintf("%q", tc.q), firstLine(f), fmt.Sprintf("ui_maps zone row %v, level columns %v; game: %s", has, lv, tc.bracket), evZoneSrc, res})
	}
	finish(t, "advice-zone-levels", "Offline zones: the level range of a zone", out)
}

// TestOfflineBenchmarkZoneLevelNoAnswer: a zone the dataset has no row for, and a zone question with no zone in it, get no range either.
func TestOfflineBenchmarkZoneLevelNoAnswer(t *testing.T) {
	zones := worldZones(t)
	var out []benchRow
	for _, q := range []string{"what level is the zone Shen'dralas for?", "what level range is the zone Zephras Isle?", "which zone has the highest level range?"} {
		f := ask.LimitFacts(q)
		out = append(out, benchRow{fmt.Sprintf("%q", q), firstLine(f), "the line that the data holds no zone level range, with no figure", evZoneSrc, resultIf(strings.Contains(f, "Zone levels:") && !evDigit.MatchString(f), "no statement that the data holds no zone level range, or a figure in it")})
	}
	_, ok := zones["Nowhere Reach"]
	out = append(out, benchRow{"a zone the dataset lacks: a ui_maps row", fmt.Sprint(ok), "none", evZoneSrc, resultIf(!ok, "a zone row was invented")})
	finish(t, "advice-zone-level-no-answer", "Offline zones: no zone level range in the dataset", out)
}

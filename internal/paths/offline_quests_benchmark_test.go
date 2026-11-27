package paths_test

import (
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/dungeons"
	"github.com/WoW-Companion/advisor/internal/paths"
	"github.com/WoW-Companion/advisor/internal/professions"
	"github.com/WoW-Companion/advisor/internal/questie"
	"github.com/WoW-Companion/advisor/internal/route"
	"github.com/WoW-Companion/advisor/internal/session"
	"github.com/WoW-Companion/advisor/internal/store"
)

// The offline quest-reward, inn, dungeon-range and profession-trainer benchmark: what the advisor answers about the
// reward of a quest, where a session ends (the inn), a dungeon's level range and entrance, and where and when a
// profession rank is trained, beside the dataset's rows, with no network. It reads the snapshots that already hold the
// rows: testdata/places_snapshot (quests, quests_observed, npcs, instances, trainers, profession_tiers) first and
// testdata/advice_snapshot (items, recipes) for the rest. The dataset holds no quest reward row (no loot_sources row of
// kind "quest reward"), no money or reputation reward, no innkeeper or bind point and no quest name outside
// quests_observed, so the reward rows of the quest cases are composed here from a real quest and a real item, and the
// cases the dataset cannot answer check that the advisor says so and states none. A difference whose cause is a wrong
// or missing dataset row is marked "(data)". make offline-benchmark runs it; WOWC_BENCH_REPORT receives the tables.

const (
	qbPlacesDir   = "../../testdata/places_snapshot"
	qbAdviceDir   = "../../testdata/advice_snapshot"
	qbQuestSrc    = "dataset quests_observed (title, level, faction, category), loot_sources (quest reward)"
	qbRewardSrc   = "dataset loot_sources, quests_observed; the dataset has no money or reputation reward column"
	qbInnSrc      = "player's QuestieDB npc flags (canonical innkeeper bit); the dataset has no innkeeper or bind point"
	qbInstanceSrc = "dataset instances (min_level, max_level, entrance_zone_name, max_players)"
	qbTierSrc     = "dataset profession_tiers (skill_required, min_level, trainer_name, place)"
)

// qbMix serves the places snapshot's tables first and the advice snapshot's for the rest.
type qbMix struct{ first, then store.DirSource }

func (m qbMix) Open(table string) (io.ReaderAt, int64, error) {
	if r, n, err := m.first.Open(table); err == nil {
		return r, n, nil
	}
	return m.then.Open(table)
}

var (
	qbOnce  sync.Once
	qbCache store.DataStore
)

func qbStore() store.DataStore {
	qbOnce.Do(func() {
		qbCache = store.New(qbMix{first: store.DirSource(qbPlacesDir), then: store.DirSource(qbAdviceDir)})
	})
	return qbCache
}

// qbRewards adds composed quest reward rows to loot_sources: the dataset has none, so a real quest and a real item are
// paired here. Every other table and row is the dataset's.
type qbRewards struct {
	store.DataStore
	rows []store.Row
}

func (r qbRewards) Filter(table string, ranges ...store.Range) ([]store.Row, error) {
	out, err := r.DataStore.Filter(table, ranges...)
	if table != "loot_sources" {
		return out, err
	}
	for _, row := range r.rows {
		keep := true
		for _, rg := range ranges {
			v := num(row[rg.Column])
			if lo, ok := rg.Min.(int64); ok && v < lo {
				keep = false
			}
			if hi, ok := rg.Max.(int64); ok && v > hi {
				keep = false
			}
		}
		if keep {
			out = append(out, row)
		}
	}
	return out, nil
}

func qbReward(item, quest int64) store.Row {
	return store.Row{"item_id": item, "source_kind": "quest reward", "source_id": quest, "tier": "settled"}
}

func qbQuest(t *testing.T, id int64) store.Row {
	t.Helper()
	r, err := qbStore().Filter("quests_observed", store.Range{Column: "quest_id", Min: id, Max: id})
	if err != nil || len(r) != 1 {
		t.Fatalf("quest %d: not in quests_observed (%v)", id, err)
	}
	return r[0]
}

func qbWarrior(faction string, level int) character.Character {
	return character.Character{Name: "Benchtest", Faction: faction, Class: "WARRIOR", Level: &level}
}

// rewardCase is one quest reward: the item, the quests whose rewards (with the choice) hold it, and the character.
type rewardCase struct {
	label   string
	item    int64
	quest   int64
	choice  []int64 // the reward items of the quest, this one among them
	c       character.Character
	opt     paths.Options
	offered bool // whether the dataset's row of the quest lets the character take it
}

// Real quests (Horde, from quests_observed) and real crafted mail (advice snapshot items): the pairing is composed.
const (
	qBattleHillsbrad = 529  // level 26 horde, Hillsbrad Foothills
	qMissingShipment = 890  // level 14 horde, The Barrens
	qBetrayal        = 879  // level 25 horde, The Barrens
	qBerensPeril     = 516  // level 21 horde, Silverpine Forest
	qElixir          = 496  // level 22 horde, Hillsbrad Foothills
	qDeepmoss        = 1069 // level 20 horde, The Barrens
	qBloodyBone      = 596  // level 37 horde, Stranglethorn Vale
	qDusky           = 93   // level 20 alliance, Duskwood
	qUnknown         = 99999999
)

const (
	iBelt, iPants, iBracers, iBreastplate, iBoots, iVest, iGauntlets, iBronzeBoots, iIronforge, iBarbaric = 2851, 2852, 2854, 2864, 3469, 3471, 3472, 6350, 6730, 7914
)

func qbRewardCases() []rewardCase {
	h30 := qbWarrior("Horde", 30)
	inLog := qbWarrior("Horde", 30)
	inLog.Quests = []character.Quest{{QuestID: qDeepmoss, Title: "Deepmoss Spider Eggs"}}
	done := qbWarrior("Horde", 30)
	done.CompletedQuests = []int{qElixir}
	return []rewardCase{
		{"one reward, Hillsbrad", iBelt, qBattleHillsbrad, []int64{iBelt}, h30, paths.Options{}, true},
		{"one reward, Barrens", iPants, qMissingShipment, []int64{iPants}, h30, paths.Options{}, true},
		{"choice of three (1)", iBreastplate, qBetrayal, []int64{iBreastplate, iVest, iBarbaric}, h30, paths.Options{}, true},
		{"choice of three (2)", iVest, qBetrayal, []int64{iBreastplate, iVest, iBarbaric}, h30, paths.Options{}, true},
		{"choice of three (3)", iBarbaric, qBetrayal, []int64{iBreastplate, iVest, iBarbaric}, h30, paths.Options{}, true},
		{"choice of two (1)", iBoots, qBerensPeril, []int64{iBoots, iGauntlets}, h30, paths.Options{}, true},
		{"choice of two (2)", iGauntlets, qBerensPeril, []int64{iBoots, iGauntlets}, h30, paths.Options{}, true},
		{"quest in the log", iBronzeBoots, qDeepmoss, []int64{iBronzeBoots}, inLog, paths.Options{}, true},
		{"quest in the plan", iBracers, qBattleHillsbrad, []int64{iBracers}, h30, paths.Options{PlanQuests: map[int]bool{qBattleHillsbrad: true}}, true},
		{"quest already done", iBracers, qElixir, []int64{iBracers}, done, paths.Options{}, false},
		{"quest too far above the level", iIronforge, qBloodyBone, []int64{iIronforge}, h30, paths.Options{}, false},
		{"quest of the other side", iBelt, qDusky, []int64{iBelt}, h30, paths.Options{}, false},
		{"quest the dataset does not name", iPants, qUnknown, []int64{iPants}, h30, paths.Options{}, false},
	}
}

// questLine is the quest path of a result, "" when there is none.
func questLine(r paths.Result) string {
	for _, p := range r.Paths {
		if p.Kind == paths.Quest {
			return p.Line
		}
	}
	return ""
}

// TestOfflineBenchmarkQuestRewards: a quest reward path, as the advisor words it, beside the quest's row in
// quests_observed (title, level, side, zone heading) and the number of reward rows the quest has (a choice).
func TestOfflineBenchmarkQuestRewards(t *testing.T) {
	var out []benchRow
	for _, tc := range qbRewardCases() {
		var rows []store.Row
		for _, it := range tc.choice {
			rows = append(rows, qbReward(it, tc.quest))
		}
		ds := qbRewards{qbStore(), rows}
		got := questLine(paths.For(tc.c, ds, tc.item, tc.opt))
		ref, ok := "", false
		if tc.quest == qUnknown {
			ref = "no title for the quest: it is not named by its number, so no path"
			ok = got == ""
		} else {
			q := qbQuest(t, tc.quest)
			title, zone := text(q["title"]), text(q["category"])
			offered := tc.offered
			ref = fmt.Sprintf("level %d %s quest; Finish %s (%s)", num(q["level"]), text(q["faction"]), title, zone)
			if !offered {
				ref = "not offered: " + ref
				ok = got == ""
			} else {
				verb := " for it"
				if len(tc.choice) > 1 {
					verb = " and choose it"
				}
				ok = strings.HasPrefix(got, "Finish "+title+verb) && strings.Contains(got, "in "+zone)
				if tc.label == "quest in the log" {
					ok = ok && strings.Contains(got, "it's in your quest log")
				}
				if tc.label == "quest in the plan" {
					ok = ok && strings.Contains(got, "it's a step of your plan")
				}
			}
		}
		if got == "" {
			got = "no quest path"
		}
		out = append(out, benchRow{tc.label + ": quest reward", got, ref, qbQuestSrc, resultIf(ok, "the quest, its choice wording, its zone or whether it is offered")})
	}
	finish(t, "advice-quest-reward", "Offline quest reward: the path the advisor gives", out)
}

// TestOfflineBenchmarkNoQuestRewardAnswer: what the dataset does not hold of a quest's reward. The advisor names no
// money, reputation or reward it has no row for.
func TestOfflineBenchmarkNoQuestRewardAnswer(t *testing.T) {
	ds := qbStore()
	var out []benchRow
	rows, _ := ds.Filter("loot_sources", store.Range{Column: "source_id", Min: int64(0)})
	n := 0
	for _, r := range rows {
		if strings.Contains(text(r["source_kind"]), "quest") {
			n++
		}
	}
	if n != 0 {
		t.Fatalf("the snapshot now holds %d quest reward rows: the cases composed above need a second look", n)
	}
	money := []string{"gold", "silver", "copper", " xp", "experience", "reputation", "standing", "faction"}
	// a quest with composed rewards: the line states the reward item only
	for _, q := range []int64{qBattleHillsbrad, qBetrayal} {
		items := []int64{iBelt}
		if q == qBetrayal {
			items = []int64{iBreastplate, iVest}
		}
		var comp []store.Row
		for _, it := range items {
			comp = append(comp, qbReward(it, q))
		}
		got := questLine(paths.For(qbWarrior("Horde", 30), qbRewards{ds, comp}, items[0], paths.Options{}))
		said := ""
		for _, w := range money {
			if strings.Contains(strings.ToLower(got), w) {
				said += w + " "
			}
		}
		out = append(out, benchRow{fmt.Sprintf("quest %d: money and reputation reward", q), got, placesNoReply + ": no money or reputation column on quests, quests_observed or loot_sources", qbRewardSrc,
			resultIf(got != "" && said == "", "the line states "+said)})
	}
	// an item the dataset gives no quest reward for is not sent to a quest
	for _, it := range []int64{iBelt, iGauntlets} {
		r := paths.For(qbWarrior("Horde", 30), ds, it, paths.Options{})
		out = append(out, benchRow{fmt.Sprintf("item %d: quest reward with no reward row", it), questLine(r) + "(no quest path)", placesNoReply + ": no loot_sources row of kind quest reward", qbRewardSrc,
			resultIf(questLine(r) == "", "a quest is named for an item no quest row rewards")})
	}
	// a quest the dataset names only by its id: no title
	r := paths.For(qbWarrior("Horde", 30), qbRewards{ds, []store.Row{qbReward(iPants, qUnknown)}}, iPants, paths.Options{})
	out = append(out, benchRow{"a reward of a quest with no name", questLine(r) + "(no quest path)", placesNoReply + ": no title in quests, quests_observed", qbQuestSrc, resultIf(questLine(r) == "" && !strings.Contains(r.Reason+questLine(r), fmt.Sprint(qUnknown)), "the quest is named by its number")})
	finish(t, "advice-no-quest-reward-answer", "Offline quest reward, no answer in the dataset", out)
}

// qbQx is a synthetic QuestieDB: a few inns and a flight point on one synthetic map, each side's own and a neutral one.
func qbQx() *questie.Index {
	const flight, inn = questie.FlagFlightMaster, questie.FlagInnkeeper
	at := func(name, side string, flags int, x, y float64) questie.Entity {
		return questie.Entity{Name: name, Faction: side, Flags: flags, Spawns: []questie.Spawn{{Area: 1, X: x, Y: y}}}
	}
	return &questie.Index{
		Areas: map[int]questie.Area{1: {UIMap: 1, Zone: "Synthetic Vale"}},
		Npcs: map[int]questie.Entity{
			1: at("Synthetic Innkeeper North", "AH", inn, 50, 20),
			2: at("Synthetic Innkeeper West", "A", inn, 31, 52),
			3: at("Synthetic Innkeeper East", "H", inn, 70, 52),
			4: at("Synthetic Flight Master", "AH", flight, 52, 80),
		},
	}
}

func qbPlan(c character.Character, level int) route.Plan {
	stop := route.Stop{N: 1, Kind: route.KindTurnIn, QuestID: 1, QuestIDs: []int{1}, Quest: "Synthetic Errand", Label: "Turn in Synthetic Errand", Map: 1, X: 50, Y: 50}
	return route.Plan{Start: &route.Point{Map: 1, X: 50, Y: 52}, Steps: []route.Stop{stop}}
}

func qbSession(t *testing.T, faction string, qx *questie.Index, minutes int, mutate func(*session.In)) session.Session {
	t.Helper()
	level := 20
	c := character.Character{Name: "Benchtest", Faction: faction, Class: "WARRIOR", Level: &level, XPMax: &level,
		Quests: []character.Quest{{QuestID: 1, Title: "Synthetic Errand", Level: &level, Complete: new(bool)}}}
	*c.Quests[0].Complete = true
	in := session.Live(c, store.New(store.ReaderSource{}), qx, qbPlan(c, level), map[int]int{1: 500})
	if mutate != nil {
		mutate(&in)
	}
	return session.Plan(in, minutes)
}

// TestOfflineBenchmarkInnAndHearth: where a short session ends (the anchors need no table, so the session is built on an empty one). The dataset names no innkeeper, so the inns come from the
// player's QuestieDB; the game's rule is that a character may use only its own side's or a neutral innkeeper, and the
// session ends at the nearest of those on the route's map, never past a 45-minute session. A hearthstone is never
// prescribed: the dataset has no bind point to check it against.
func TestOfflineBenchmarkInnAndHearth(t *testing.T) {
	var out []benchRow
	add := func(check, got, ref string, ok bool) {
		out = append(out, benchRow{check, got, ref, qbInnSrc, resultIf(ok, "the place the session ends at")})
	}
	// the dataset itself: no creature in the snapshot is an innkeeper
	rows, _ := qbStore().Filter("npcs", store.Range{Column: "id", Min: int64(0), Max: int64(1 << 40)})
	inns := 0
	for _, r := range rows {
		if strings.Contains(strings.ToLower(text(r["name"])), "innkeeper") {
			inns++
		}
	}
	qx := qbQx()
	without := func(ids ...int) *questie.Index {
		x := qbQx()
		for _, id := range ids {
			delete(x.Npcs, id)
		}
		return x
	}
	s := qbSession(t, "Horde", qx, 30, nil)
	add("Horde, all inns: an Alliance inn is nearer than the Horde's", s.EndsAt, "Synthetic Innkeeper East, the nearest inn the Horde may use", strings.Contains(s.EndsAt, "Synthetic Innkeeper East"))
	s = qbSession(t, "Alliance", qx, 30, nil)
	add("Alliance, all inns: the Alliance's inn is the nearest", s.EndsAt, "Synthetic Innkeeper West, the nearest inn the Alliance may use", strings.Contains(s.EndsAt, "Synthetic Innkeeper West"))
	s = qbSession(t, "Horde", without(3), 30, nil)
	add("Horde, the Horde's inn gone: the nearer one is the Alliance's", s.EndsAt, "Synthetic Innkeeper North, the neutral inn; West is the Alliance's", strings.Contains(s.EndsAt, "Synthetic Innkeeper North"))
	s = qbSession(t, "Alliance", without(2), 30, nil)
	add("Alliance, the Alliance's inn gone: the nearer one is the Horde's", s.EndsAt, "Synthetic Innkeeper North, the neutral inn; East is the Horde's", strings.Contains(s.EndsAt, "Synthetic Innkeeper North"))
	s = qbSession(t, "Horde", without(1, 3), 30, nil)
	add("Horde, only the Alliance's inn and a flight point", s.EndsAt, "the flight point: Synthetic Flight Master; the Alliance's inn is no anchor", strings.Contains(s.EndsAt, "Synthetic Flight Master"))
	s = qbSession(t, "Horde", qx, 20, nil)
	add("Horde, session of 20 minutes", s.EndsAt, "a short session ends at an inn or flight point", s.EndsAt != "")
	s = qbSession(t, "Horde", qx, 90, nil)
	add("Horde, session of 90 minutes", s.EndsAt+"(none)", "a session over 45 minutes ends nowhere in particular", s.EndsAt == "")
	flightOnly := qbQx()
	for id, e := range flightOnly.Npcs {
		if e.Flags == questie.FlagInnkeeper {
			delete(flightOnly.Npcs, id)
		}
	}
	s = qbSession(t, "Horde", flightOnly, 30, nil)
	add("Horde, no inn on the map, a flight point", s.EndsAt, "the flight point is the anchor: Synthetic Flight Master", strings.Contains(s.EndsAt, "flight point") && strings.Contains(s.EndsAt, "Synthetic Flight Master"))
	s = qbSession(t, "Horde", nil, 30, nil)
	add("Horde, no QuestieDB", s.EndsAt+"(none)", placesNoReply+": no innkeeper in the dataset ("+fmt.Sprint(inns)+" in npcs), so none is named", inns == 0 && s.EndsAt == "")
	far := qbQx()
	for id, e := range far.Npcs {
		e.Spawns = []questie.Spawn{{Area: 9, X: e.Spawns[0].X, Y: e.Spawns[0].Y}}
		far.Npcs[id] = e
	}
	far.Areas[9] = questie.Area{UIMap: 9, Zone: "Synthetic Isle"}
	s = qbSession(t, "Horde", far, 30, nil)
	add("Horde, every inn on another map", s.EndsAt+"(none)", "an inn on another map is out of reach: no place to end at", s.EndsAt == "")
	facts := qbSession(t, "Horde", qx, 30, nil).Facts()
	said := strings.Contains(strings.ToLower(facts), "hearth")
	got := "no hearthstone advice"
	if said {
		got = "tells the player to use or set a hearthstone"
	}
	out = append(out, benchRow{"the hearthstone bind point in a session answer", got, placesNoReply + ": the dataset has no bind point or innkeeper, so no place to set the hearthstone", qbInnSrc, resultIf(!said, "a hearthstone is prescribed with no data behind it")})
	finish(t, "advice-inn-hearth", "Offline inn and hearthstone: where a session ends", out)
}

// qbDungeons are named dungeons the advisor describes: the group size the game gives (a dungeon is a five-player one,
// Upper Blackrock Spire ten) and the level the character is, for the levels still to go.
type qbDungeon struct {
	name    string
	level   int
	guide   int64 // the game's group size
	faction string
}

var qbDungeons = []qbDungeon{
	{"Wailing Caverns", 14, 5, "Horde"}, {"Stormwind Stockade", 25, 5, "Horde"}, {"Scarlet Monastery", 35, 5, "Alliance"},
	{"Gnomeregan", 20, 5, "Alliance"}, {"Maraudon", 50, 5, "Horde"}, {"Sunken Temple", 60, 5, "Horde"}, {"Blackrock Spire", 60, 10, "Alliance"},
	{"Dire Maul", 45, 5, "Horde"}, {"Scholomance", 60, 5, "Alliance"}, {"Stratholme", 56, 5, "Horde"}, {"Uldaman", 30, 5, "Alliance"},
	{"Blackfathom Deeps", 25, 5, "Horde"}, {"Razorfen Downs", 36, 5, "Alliance"}, {"Shadowfang Keep", 18, 5, "Horde"},
}

func TestOfflineBenchmarkDungeonRangeA(t *testing.T) { dungeonRange(t, "a", qbDungeons[0:4]) }
func TestOfflineBenchmarkDungeonRangeB(t *testing.T) { dungeonRange(t, "b", qbDungeons[4:8]) }
func TestOfflineBenchmarkDungeonRangeC(t *testing.T) { dungeonRange(t, "c", qbDungeons[8:11]) }
func TestOfflineBenchmarkDungeonRangeD(t *testing.T) { dungeonRange(t, "d", qbDungeons[11:]) }

// dungeonRange: the level range, entrance zone and levels still to go of a dungeon asked by name, beside
// the instances rows; the group size beside the game's (a dungeon the dataset sizes at ten or twenty is a data difference).
func dungeonRange(t *testing.T, part string, cases []qbDungeon) {
	ds := qbStore()
	var out []benchRow
	for _, tc := range cases {
		rows, _ := ds.Filter("instances", store.Range{Column: "name", Min: tc.name, Max: tc.name})
		if len(rows) == 0 {
			t.Fatalf("%s: not in the snapshot", tc.name)
		}
		r := rows[0]
		c := character.Character{Faction: tc.faction, Class: "WARRIOR", Level: &tc.level}
		p, togo, err := dungeons.Named(c, ds, nil, tc.name)
		if err != nil || p == nil || p.MinLevel == nil || p.MaxLevel == nil {
			t.Fatalf("%s: %v %v", tc.name, p, err)
		}
		lo, hi := int(num(r["min_level"])), int(num(r["max_level"]))
		wantTogo := max(lo-tc.level, 0)
		words := dungeons.RenderNamed(c, *p, togo)
		got := fmt.Sprintf("levels %d-%d, %s, %d to go", *p.MinLevel, *p.MaxLevel, p.EntranceZone, togo)
		ref := fmt.Sprintf("levels %d-%d, %s, %d to go", lo, hi, text(r["entrance_zone_name"]), wantTogo)
		ok := got == ref && strings.Contains(words, fmt.Sprintf("about levels %d–%d", lo, hi)) && strings.Contains(words, "in "+text(r["entrance_zone_name"]))
		out = append(out, benchRow{fmt.Sprintf("%s asked at level %d: range and entrance", tc.name, tc.level), got, ref, qbInstanceSrc, resultIf(ok, "the range, the entrance or the levels to go")})
		size := num(r["max_players"])
		res := "match"
		if size != tc.guide {
			res = fmt.Sprintf("differ (data): the dataset sizes it at %d players, the game at %d", size, tc.guide)
		}
		shown := "no group size worded"
		if p.MaxPlayers != nil {
			shown = fmt.Sprintf("%d players", *p.MaxPlayers)
		}
		out = append(out, benchRow{fmt.Sprintf("%s: group size", tc.name), shown, fmt.Sprintf("%d players (dataset %d)", tc.guide, size), qbInstanceSrc, res})
	}
	finish(t, "advice-dungeon-range-"+part, "Offline dungeon level range, entrance and group size ("+part+")", out)
}

// TestOfflineBenchmarkDungeonNoAnswer: dungeons and raids the dataset holds no level range for, and a name it does not hold.
func TestOfflineBenchmarkDungeonNoAnswer(t *testing.T) {
	ds := qbStore()
	var out []benchRow
	level := 40
	c := character.Character{Faction: "Horde", Class: "WARRIOR", Level: &level}
	for _, name := range []string{"The Drowned City", "Caverns of Time", "Demon Fall Canyon"} {
		p, _, err := dungeons.Named(c, ds, nil, name)
		words := ""
		if p != nil {
			words = dungeons.RenderNamed(c, *p, 0)
		}
		ok := err == nil && p != nil && p.LevelUnknown && !strings.Contains(words, "about levels")
		out = append(out, benchRow{name + ": level range", "no range worded", placesNoReply + ": no min_level or max_level", qbInstanceSrc, resultIf(ok, "a level range is worded")})
	}
	p, _, err := dungeons.Named(c, ds, nil, "Vault of the Forgotten Moon")
	out = append(out, benchRow{"a dungeon the dataset does not hold", fmt.Sprintf("answer %v", p != nil), placesNoReply, qbInstanceSrc, resultIf(err == nil && p == nil, "a dungeon was invented")})
	finish(t, "advice-dungeon-no-answer", "Offline dungeon, no answer in the dataset", out)
}

// qbTierCases are professions at a skill and level, with the tier the next trainer visit is for.
type qbTier struct {
	prof    string
	faction string
	skill   int
	level   int
}

var qbTierCases = []qbTier{
	{"Tailoring", "Horde", 60, 12}, {"Tailoring", "Alliance", 130, 22}, {"Alchemy", "Horde", 45, 10}, {"Alchemy", "Alliance", 120, 22},
	{"Blacksmithing", "Horde", 70, 15}, {"Blacksmithing", "Alliance", 150, 25}, {"Leatherworking", "Horde", 130, 22},
	{"Leatherworking", "Alliance", 70, 15}, {"Engineering", "Horde", 140, 24}, {"Engineering", "Alliance", 60, 12}, {"Enchanting", "Horde", 140, 24},
}

func TestOfflineBenchmarkProfessionRankA(t *testing.T) { professionRank(t, "a", qbTierCases[0:1]) }
func TestOfflineBenchmarkProfessionRankB(t *testing.T) { professionRank(t, "b", qbTierCases[1:3]) }
func TestOfflineBenchmarkProfessionRankC(t *testing.T) { professionRank(t, "c", qbTierCases[3:5]) }
func TestOfflineBenchmarkProfessionRankD(t *testing.T) { professionRank(t, "d", qbTierCases[5:7]) }
func TestOfflineBenchmarkProfessionRankE(t *testing.T) { professionRank(t, "e", qbTierCases[7:9]) }
func TestOfflineBenchmarkProfessionRankF(t *testing.T) { professionRank(t, "f", qbTierCases[9:]) }

// professionRank: where the advisor sends a player to train and what level it says the next rank
// needs, beside the dataset's profession_tiers (trainer, place, skill_required, min_level) for the side.
func professionRank(t *testing.T, part string, cases []qbTier) {
	ds := qbStore()
	var out []benchRow
	for _, tc := range cases {
		level, skill, top := tc.level, tc.skill, tc.skill+25
		c := character.Character{Faction: tc.faction, Class: "MAGE", Level: &level, Professions: []character.Skill{{Name: tc.prof, Rank: &skill, Max: &top}}}
		pa := professions.ComputePath(c, ds, tc.prof, tc.skill+3, 1)
		words := strings.Join(pa.Where, " ")
		tiers, _ := ds.Filter("profession_tiers", store.Range{Column: "profession", Min: tc.prof, Max: tc.prof})
		var named []string
		var next struct {
			name         string
			skill, level int64
		}
		for _, r := range tiers {
			if f := text(r["faction"]); f != "" && f != "Both" && !strings.EqualFold(f, tc.faction) {
				continue
			}
			if text(r["trainer_name"]) != "" {
				named = append(named, text(r["trainer_name"])+" in "+text(r["place"]))
			}
		}
		// the first tier the dataset has the character not yet allowed to train by level
		for _, r := range tiers {
			if num(r["min_level"]) > int64(tc.level) && (next.name == "" || num(r["min_level"]) < next.level) {
				next.name, next.skill, next.level = text(r["tier"]), num(r["skill_required"]), num(r["min_level"])
			}
		}
		trainerOK := len(named) == 0 && strings.Contains(words, "Trainer to visit")
		for _, n := range named {
			if strings.Contains(words, n) {
				trainerOK = true
			}
		}
		out = append(out, benchRow{fmt.Sprintf("%s (%s, skill %d, level %d): trainer", tc.prof, strings.ToLower(tc.faction), tc.skill, tc.level), firstWhere(words, "Trainer to visit"), strings.Join(named, "; "), qbTierSrc, resultIf(trainerOK, "the trainer or town")})
		gate := fmt.Sprintf("level %d", next.level)
		ok := next.name == "" || strings.Contains(words, next.name) && strings.Contains(words, gate)
		out = append(out, benchRow{fmt.Sprintf("%s (skill %d, level %d): next rank gate", tc.prof, tc.skill, tc.level), firstWhere(words, "Next tier"), fmt.Sprintf("%s: skill %d, level %d", next.name, next.skill, next.level), qbTierSrc, resultIf(ok, "the next rank or its level gate")})
	}
	finish(t, "advice-profession-rank-"+part, "Offline profession trainer and rank gate ("+part+")", out)
}

func firstWhere(words, key string) string {
	for _, s := range strings.SplitAfter(words, ". ") {
		if strings.Contains(s, key) {
			return strings.TrimSpace(s)
		}
	}
	return "no such line"
}

// TestOfflineBenchmarkTailoringRankSkill: the skill and character level the Tailoring route says each rank is trained
// at, beside the dataset's skill_required and min_level of the Expert and Artisan rows.
func TestOfflineBenchmarkTailoringRankSkill(t *testing.T) {
	ds := qbStore()
	var out []benchRow
	level, skill, top := 60, 124, 150
	c := character.Character{Faction: "Horde", Class: "MAGE", Level: &level, Professions: []character.Skill{{Name: "Tailoring", Rank: &skill, Max: &top}}}
	pa := professions.ComputePath(c, ds, "Tailoring", 130, 1)
	words := strings.Join(pa.Notes, " ")
	tiers, _ := ds.Filter("profession_tiers", store.Range{Column: "profession", Min: "Tailoring", Max: "Tailoring"})
	seen := map[string]bool{}
	for _, r := range tiers {
		name := text(r["tier"])
		if seen[name] || name == "Journeyman" {
			continue
		}
		seen[name] = true
		sk, lv := num(r["skill_required"]), num(r["min_level"])
		want := fmt.Sprintf("%s: train at skill %d, character level %d", name, sk, lv)
		got := "not worded"
		for _, s := range strings.SplitAfter(words, ". ") {
			if strings.Contains(s, name) {
				got = strings.TrimSpace(s)
				break
			}
		}
		ok := strings.Contains(words, fmt.Sprintf("%s as soon as you have Tailoring %d and character level %d", name, sk, lv))
		out = append(out, benchRow{"Tailoring route: " + name + " rank", got, want, qbTierSrc, resultIf(ok, "the skill or level the rank is trained at")})
	}
	if len(out) != 2 {
		t.Fatalf("tiers: %v", seen)
	}
	finish(t, "advice-tailoring-rank-skill", "Offline Tailoring rank skill requirement", out)
}

// TestOfflineBenchmarkProfessionTrainerNoAnswer: ranks and sides the dataset names no trainer for. The advisor says none
// is known (or names no one), never an id or the other side's trainer.
func TestOfflineBenchmarkProfessionTrainerNoAnswer(t *testing.T) {
	ds := qbStore()
	var out []benchRow
	for _, tc := range []struct{ prof, faction string }{{"Mining", "Horde"}, {"Mining", "Alliance"}, {"Skinning", "Horde"}, {"Herbalism", "Alliance"}} {
		level, skill, top := 30, 60, 75
		c := character.Character{Faction: tc.faction, Class: "MAGE", Level: &level, Professions: []character.Skill{{Name: tc.prof, Rank: &skill, Max: &top}}}
		pa := professions.ComputePath(c, ds, tc.prof, 63, 1)
		line := firstWhere(strings.Join(pa.Where, " "), "rainer")
		tiers, _ := ds.Filter("profession_tiers", store.Range{Column: "profession", Min: tc.prof, Max: tc.prof})
		named := 0
		for _, r := range tiers {
			if text(r["trainer_name"]) != "" {
				named++
			}
		}
		ok := named == 0 && !strings.Contains(line, "Trainer to visit: "+tc.prof+" Trainer") && !slices.ContainsFunc(strings.Fields(line), func(w string) bool { return strings.Trim(w, ".,;()") == "0" })
		out = append(out, benchRow{fmt.Sprintf("%s (%s): trainer", tc.prof, strings.ToLower(tc.faction)), line, fmt.Sprintf("%s: %d guide rows name a trainer", placesNoReply, named), qbTierSrc, resultIf(ok, "a trainer is invented")})
	}
	finish(t, "advice-profession-trainer-no-answer", "Offline profession trainer, none named in the dataset", out)
}

var placesNoReply = "the dataset has no answer"

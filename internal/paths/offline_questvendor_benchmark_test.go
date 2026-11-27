package paths_test

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/levelup"
	"github.com/WoW-Companion/advisor/internal/paths"
	"github.com/WoW-Companion/advisor/internal/questie"
	"github.com/WoW-Companion/advisor/internal/quests"
	"github.com/WoW-Companion/advisor/internal/store"
	"github.com/WoW-Companion/advisor/internal/vendors"
)

// The offline quest-reward rules, quest-prerequisite, vendor-finder and class-trainer travel benchmark: which quest the
// advisor names for a reward and how it words the quest (side, level window, group or dungeon tag, log and plan order),
// how the quest chain says what must be finished first and what a quest needs, who the vendor finder lists for a side,
// and what the line to a class trainer says about the way there, beside the dataset's rows, with no network. It reads
// testdata/places_snapshot (quests_observed, quest_lines) and testdata/advice_snapshot (items, vendor_sources). The
// dataset holds no quest reward row, no prerequisite, level requirement or class column on a quest, no faction, standing
// or discount on a vendor and no flight path row, so the prerequisites come from composed QuestieDB rows (the
// player's own data) and a case the dataset cannot answer checks that the advisor says so and states none. A
// difference caused by the dataset's own rows is marked "(data)". make offline-benchmark runs it; WOWC_BENCH_REPORT
// receives the tables.

const (
	qvQuestSrc  = "dataset quests_observed (title, level, faction, category, tag)"
	qvLineSrc   = "dataset quest_lines (line_id, name, position, quest_id), quests_observed (title)"
	qvQuestieSr = "the player's QuestieDB rows (preQuestSingle: any one; preQuestGroup: all; minLevel; class and race masks), composed here"
	qvVendorSrc = "dataset vendor_sources, items; QuestieDB friendlyToFaction (A, H, AH); the dataset's npcs table has no faction column"
	qvTravelSrc = "game transport guides (Wowhead zeppelin and boat routes, class trainer lists); dataset ui_maps"
	qvNoneSrc   = "the dataset has no such column, row or table"
)

// ---- quest rewards: side, window, tag, order ----

type qvReward struct {
	label string
	item  int64
	quest []int64 // the quests whose rewards hold the item
	c     character.Character
	opt   paths.Options
	want  string // "" when no quest path is expected; else what the line holds, comma-separated
}

func qvRewardCases() []qvReward {
	horde, ally := qbWarrior("Horde", 30), qbWarrior("Alliance", 30)
	failed := true
	retry := qbWarrior("Horde", 30)
	retry.Quests = []character.Quest{{QuestID: qBattleHillsbrad, Title: "Battle of Hillsbrad", Failed: &failed}}
	inLog := qbWarrior("Horde", 30)
	inLog.Quests = []character.Quest{{QuestID: qDeepmoss, Title: "Deepmoss Spider Eggs"}}
	noLevel := character.Character{Name: "Benchtest", Faction: "Horde", Class: "WARRIOR"}
	return []qvReward{
		{"neutral quest, a Horde player", iBelt, []int64{94}, horde, paths.Options{}, "Finish A Watchful Eye for it"},
		{"neutral quest, an Alliance player", iBelt, []int64{94}, ally, paths.Options{}, "Finish A Watchful Eye for it"},
		{"dungeon quest (The Stockade)", iBelt, []int64{386}, ally, paths.Options{}, "Finish What Comes Around... for it,in The Stockade,a dungeon quest"},
		{"group quest (Wetlands)", iBelt, []int64{474}, ally, paths.Options{}, "Finish Defeat Nek'rosh for it,in Wetlands,a group quest"},
		{"quest two levels above the player", iBelt, []int64{qBattleHillsbrad}, qbWarrior("Horde", 24), paths.Options{}, "Finish Battle of Hillsbrad for it"},
		{"quest three levels above the player", iBelt, []int64{qBattleHillsbrad}, qbWarrior("Horde", 23), paths.Options{}, ""},
		{"two quests reward it: the lower number", iBelt, []int64{qMissingShipment, qBattleHillsbrad}, horde, paths.Options{}, "Finish Battle of Hillsbrad for it"},
		{"two quests reward it: the one in the plan", iBelt, []int64{qMissingShipment, qBattleHillsbrad}, horde, paths.Options{PlanQuests: map[int]bool{qMissingShipment: true}}, "Finish The Missing Shipment for it,it's a step of your plan"},
		{"three quests reward it: the one in the log", iBelt, []int64{qMissingShipment, qBattleHillsbrad, qDeepmoss}, inLog, paths.Options{PlanQuests: map[int]bool{qMissingShipment: true}}, "Finish Deepmoss Spider Eggs for it,it's in your quest log"},
		{"a failed quest in the log is offered again", iBelt, []int64{qBattleHillsbrad}, retry, paths.Options{}, "Finish Battle of Hillsbrad for it"},
		{"a player whose level is not known", iBelt, []int64{qBloodyBone}, noLevel, paths.Options{}, "Finish Bloody Bone Necklaces for it"},
	}
}

// TestOfflineBenchmarkQuestRewardRules: the quest path for an item, beside the quest's row in quests_observed: a quest with no
// side belongs to both sides, a quest players tagged for a group or a dungeon says so, the level window is two above, and
// the order is the log, then the plan, then the lower quest number.
func TestOfflineBenchmarkQuestRewardRules(t *testing.T) {
	ds := qbStore()
	var out []benchRow
	for _, tc := range qvRewardCases() {
		var rows []store.Row
		for _, q := range tc.quest {
			rows = append(rows, qbReward(tc.item, q))
		}
		got := questLine(paths.For(tc.c, qbRewards{ds, rows}, tc.item, tc.opt))
		ref := "no quest path: the quest is out of the level window"
		ok := got == ""
		if tc.want != "" {
			ref = tc.want
			ok = strings.HasPrefix(got, strings.SplitN(tc.want, ",", 2)[0])
			for _, w := range strings.Split(tc.want, ",")[1:] {
				ok = ok && strings.Contains(got, w)
			}
			// the quest's own row: the tag the line repeats
			q := qbQuest(t, tc.quest[len(tc.quest)-1])
			if tc.label == "dungeon quest (The Stockade)" || tc.label == "group quest (Wetlands)" {
				ok = ok && (text(q["tag"]) == "dungeon") == strings.Contains(got, "a dungeon quest") && (text(q["tag"]) == "group") == strings.Contains(got, "a group quest")
			}
		}
		if got == "" {
			got = "no quest path"
		}
		out = append(out, benchRow{tc.label + ": quest reward", got, ref, qvQuestSrc, resultIf(ok, "the quest, its wording, the group tag or the order")})
	}
	finish(t, "advice-quest-reward-rules", "Offline quest reward rules: side, level window, group tag and order", out)
}

// ---- quest chains: what comes first ----

func qvQuestie() *questie.Index {
	q := func(id int, name string, mut func(*questie.Quest)) *questie.Quest {
		x := &questie.Quest{ID: id, Name: name, Level: 20}
		if mut != nil {
			mut(x)
		}
		return x
	}
	return &questie.Index{
		Areas: map[int]questie.Area{1: {UIMap: 1, Zone: "Synthetic Vale"}},
		Npcs:  map[int]questie.Entity{},
		Quests: map[int]*questie.Quest{
			9001: q(9001, "First Errand", nil),
			9002: q(9002, "Second Errand", nil),
			9003: q(9003, "Third Errand", func(x *questie.Quest) { x.PreSingle = []int{9001} }),
			9004: q(9004, "Either Errand", func(x *questie.Quest) { x.PreSingle = []int{9001, 9002} }),
			9005: q(9005, "Both Errands", func(x *questie.Quest) { x.PreGroup = []int{9001, 9002} }),
			9006: q(9006, "Orc Warrior Errand", func(x *questie.Quest) { x.MinLevel, x.Classes, x.Races = 18, 1|2, 2 }),
		},
	}
}

// TestOfflineBenchmarkQuestChainRules: what a quest in a line is placed as (step n of m, the line's name and number), a quest
// asked for by its number or by a title many quests share, and what must be finished first: one prerequisite, any one of
// several, or all of a group, as QuestieDB holds them.
func TestOfflineBenchmarkQuestChainRules(t *testing.T) {
	ds := qbStore()
	qx := qvQuestie()
	var out []benchRow
	for _, ln := range []int64{5963, 6026, 6293} {
		l := refLine(t, ds, ln)
		first := l.ids[0]
		c := quests.Of(ds, first)
		head := firstLine(quests.Render(c, quests.Name(ds, first)))
		want := fmt.Sprintf("step 1 of %d in %s (line %d)", len(l.ids), l.name, ln)
		out = append(out, benchRow{fmt.Sprintf("line %d, first quest: its place", ln), head, want, qvLineSrc, resultIf(strings.Contains(head, want), "the step, the length or the line")})
	}
	id, name, cands := quests.Resolve(ds, "92745")
	out = append(out, benchRow{"a quest asked for by its number: its title", fmt.Sprintf("%d %q", id, name), "92745 \"The State of the Mines\"", qvLineSrc, resultIf(id == 92745 && name == "The State of the Mines" && len(cands) == 0, "the title")})
	rows, _ := ds.Filter("quests_observed", store.Range{Column: "quest_id", Min: int64(1)})
	same := 0
	for _, r := range rows {
		if text(r["title"]) == "The Legend of Stalvan" {
			same++
		}
	}
	id, name, cands = quests.Resolve(ds, "The Legend of Stalvan")
	out = append(out, benchRow{"a title five quests share: which quest", fmt.Sprintf("id %d, %d candidates", id, len(cands)), fmt.Sprintf("no single quest; %d quests carry the title", same), qvQuestSrc,
		resultIf(same > 1 && id == 0 && len(cands) == min(same, 8) && strings.Contains(strings.Join(cands, ";"), "(68)"), "a quest was picked, or a candidate left out")})
	for _, tc := range []struct {
		quest   int64
		want    string
		refText string
	}{
		{9003, "Comes after:\n  - First Errand", "one prerequisite: First Errand"},
		{9004, "Comes after (finish any one of these first):", "preQuestSingle 9001, 9002: either one opens it"},
		{9005, "Comes after (finish all of these first):", "preQuestGroup 9001, 9002: both are needed"},
	} {
		txt := quests.Render(quests.OfWith(ds, qx, tc.quest), "")
		out = append(out, benchRow{fmt.Sprintf("quest %d: what must be finished first", tc.quest), strings.ReplaceAll(txt, "\n", " | "), tc.refText, qvQuestieSr, resultIf(strings.Contains(txt, tc.want), "any-one and all-of read the same")})
	}
	txt := quests.Render(quests.OfWith(ds, qx, 9006), "")
	out = append(out, benchRow{"a quest with a level, class and race limit", firstLine(txt), "needs level 18, Warrior, Paladin only, Orc only", qvQuestieSr,
		resultIf(strings.Contains(firstLine(txt), "needs level 18, Warrior, Paladin only, Orc only"), "the level, class or race limit")})
	finish(t, "advice-quest-chain-rules", "Offline quest chain: place in a line, title lookup and what comes first", out)
}

// ---- vendor finder and reputation vendors ----

func qvVendorIndex() *questie.Index {
	at := func(name, side string, x float64) questie.Entity {
		return questie.Entity{Name: name, Sub: "Rune Thread", Faction: side, Flags: 0x80, Spawns: []questie.Spawn{{Area: 1, X: x, Y: 50}}}
	}
	return &questie.Index{
		Areas: map[int]questie.Area{1: {UIMap: 1, Zone: "Synthetic Vale"}, 2: {UIMap: 2, Zone: "Other Vale"}},
		Npcs: map[int]questie.Entity{
			70001: at("Horde Thread Seller", "H", 20),
			70002: at("Alliance Thread Seller", "A", 30),
			70003: at("Neutral Thread Seller", "AH", 40),
			70004: {Name: "Far Thread Seller", Sub: "Rune Thread", Faction: "AH", Flags: 0x80, Spawns: []questie.Spawn{{Area: 2, X: 10, Y: 10}}},
		},
	}
}

func qvFound(r vendors.Result) string {
	var n []string
	for _, f := range r.Found {
		n = append(n, f.Name)
	}
	return strings.Join(n, ", ")
}

// TestOfflineBenchmarkVendorFinder: who the vendor finder lists for a side, beside the dataset's offers and QuestieDB's side
// flag (A, H, AH): a vendor of the other side is never listed, a neutral one is open to both, the zone limits the list, and a
// vendor only the dataset names is never said to be open to both sides, because no row holds its side.
func TestOfflineBenchmarkVendorFinder(t *testing.T) {
	ds := mbNamed()
	qx := qvVendorIndex()
	var out []benchRow
	for _, tc := range []struct {
		label string
		q     vendors.Query
		want  string
		not   []string
	}{
		{"Horde player, Subname Rune Thread", vendors.Query{Text: "Rune Thread", Faction: "Horde", Zone: "Synthetic Vale"}, "Horde Thread Seller", []string{"Alliance Thread Seller"}},
		{"Alliance player, Subname Rune Thread", vendors.Query{Text: "Rune Thread", Faction: "Alliance", Zone: "Synthetic Vale"}, "Alliance Thread Seller", []string{"Horde Thread Seller"}},
		{"player of no known side", vendors.Query{Text: "Rune Thread", Zone: "Synthetic Vale"}, "Horde Thread Seller, Alliance Thread Seller", nil},
		{"zone limit: Other Vale", vendors.Query{Text: "Rune Thread", Faction: "Horde", Zone: "Other Vale"}, "Far Thread Seller", []string{"Horde Thread Seller", "Neutral Thread Seller"}},
	} {
		res := vendors.Search(nil, qx, tc.q)
		got := qvFound(res)
		ok := strings.Contains(got, tc.want)
		for _, n := range tc.not {
			ok = ok && !strings.Contains(got, n)
		}
		out = append(out, benchRow{tc.label + ": who is listed", got, "lists " + tc.want + "; not " + strings.Join(tc.not, ", "), qvVendorSrc, resultIf(ok, "a vendor of the other side, or one missing")})
	}
	res := vendors.Search(nil, qx, vendors.Query{Text: "Neutral Thread", Faction: "Horde"})
	out = append(out, benchRow{"a neutral (AH) vendor: whose it is", res.Text, "open to both factions", qvVendorSrc, resultIf(len(res.Found) == 1 && strings.Contains(res.Text, "open to both factions"), "a neutral vendor was given a side")})
	res = vendors.Search(nil, qx, vendors.Query{Text: "Horde Thread", Faction: "Alliance"})
	out = append(out, benchRow{"a Horde (H) vendor for an Alliance player", res.Text, "no vendor listed", qvVendorSrc, resultIf(len(res.Found) == 0, "a Horde vendor was offered to an Alliance player")})

	// a vendor only the dataset names: the dataset's npcs table has no faction column, so its side is not in the data
	it := num(rowByName(t, ds, "items", "Rune Thread")["id"])
	for _, side := range []string{"Horde", "Alliance"} {
		r := vendors.Search(ds, nil, vendors.Query{Text: "Rune Thread", Faction: side})
		line := ""
		if len(r.Found) > 0 {
			line = firstLine(r.Text)
		}
		ok := len(r.Found) > 0 && !strings.Contains(r.Text, "open to both") && strings.Contains(line, "not in the data") && !r.Found[0].FactionKnown
		out = append(out, benchRow{side + " player, vendor only the dataset names: its side", line, "no answer: no faction on vendor_sources or npcs, so the side is not stated", qvNoneSrc, resultIf(ok, "the vendor was said to be open to both sides")})
	}
	// a limit on how many are listed
	r := vendors.Search(ds, nil, vendors.Query{Text: "Rune Thread"})
	sells := 0
	for _, f := range r.Found {
		sells = max(sells, len(f.Sells))
	}
	out = append(out, benchRow{"a thread sold by many vendors: how many are listed", fmt.Sprintf("%d vendors, up to %d goods each", len(r.Found), sells), fmt.Sprintf("at most %d vendors and %d goods each", vendors.MaxFound, vendors.MaxSells), qvVendorSrc,
		resultIf(len(r.Found) == vendors.MaxFound && sells <= vendors.MaxSells && sells > 0, "the list is longer than the limit or empty")})

	// a vendor the player opened comes before the dataset's, at the price the vendor showed; the other side's is ignored
	c := character.Character{Faction: "Horde", Seen: &character.Observed{Vendors: []character.SeenVendor{
		{NPC: 9100, Name: "Seen Alliance Seller", Faction: "Alliance", Items: []character.SeenStock{{ItemID: it, Price: 5}}},
		{NPC: 9101, Name: "Seen Horde Seller", Faction: "Horde", Items: []character.SeenStock{{ItemID: it, Price: 777}}},
	}}}
	o, found := vendors.BestFor(c, ds, it)
	out = append(out, benchRow{"a vendor the player opened: which one and at what price", fmt.Sprintf("%s at %s", o.Vendor, o.Price), "Seen Horde Seller at 7s 77c: the player's side, the price the vendor showed", "the capture's vendors (seen by you)",
		resultIf(found && o.Vendor == "Seen Horde Seller" && o.PriceCopper == 777 && o.Seen(), "the other side's vendor or the dataset's price")})
	finish(t, "advice-vendor-finder", "Offline vendor finder: side, zone and the vendors a player opened", out)
}

var qvStanding = regexp.MustCompile(`(?i)\b(reputation|standing|friendly|honou?red|revered|exalted|discount)\b`)

// TestOfflineBenchmarkReputationGateNoAnswer: what the dataset does not hold about a vendor's reputation gate, a quest's level
// requirement or the cost of a flight: the advisor says it has no answer and names none.
func TestOfflineBenchmarkReputationGateNoAnswer(t *testing.T) {
	ds := mbNamed()
	var out []benchRow
	for _, q := range []string{"Honored Rune Thread", "Exalted reputation vendor", "Revered discount vendor"} {
		r := vendors.Search(ds, nil, vendors.Query{Text: q})
		out = append(out, benchRow{fmt.Sprintf("%q: a vendor for a standing", q), firstLine(r.Text), "no answer: no standing, faction or discount on vendor_sources", qvNoneSrc,
			resultIf(len(r.Found) == 0 && strings.HasPrefix(r.Text, "Not in my data") && r.Gap != "" && !strings.Contains(r.Text, "Synthetic Vendor"), "a vendor was named for a standing")})
	}
	for _, id := range []int64{qBattleHillsbrad, qBetrayal} {
		txt := quests.Render(quests.OfWith(qbStore(), nil, id), "")
		out = append(out, benchRow{fmt.Sprintf("quest %d: the level the player needs to take it", id), firstLine(txt), "no answer: quests holds no minimum level or level at all", qvNoneSrc,
			resultIf(!strings.Contains(txt, "needs level") && !qvStanding.MatchString(txt), "a requirement was stated")})
	}
	for _, tc := range []struct{ side, class, zone string }{{"Alliance", "DRUID", "Westfall"}, {"Horde", "HUNTER", "Tirisfal Glades"}} {
		lv := 30
		pl, _ := levelup.TrainAt(character.Character{Class: tc.class, Faction: tc.side, Level: &lv, Location: &character.Location{Zone: tc.zone}}, nil)
		line := pl.Line(tc.class)
		out = append(out, benchRow{fmt.Sprintf("%s %s in %s: what the trip costs", tc.side, tc.class, tc.zone), line, "no answer: no flight price, duration or boat time in the dataset", qvNoneSrc,
			resultIf(!regexp.MustCompile(`(?i)\b(\d+ ?(g|s|c|gold|silver|copper|min|minutes|seconds)|cost|price)\b`).MatchString(line), "a price or a time was stated")})
	}
	finish(t, "advice-reputation-gate-no-answer", "Offline reputation gate, quest requirement and travel cost: no answer in the dataset", out)
}

// ---- travel to the class trainer ----

// TestOfflineBenchmarkTrainerTravel: the way to a class trainer the line names, beside the game's guides: the capital beside the
// zone the player stands in, and the zeppelin or boat first when the capital is on the other continent.
func TestOfflineBenchmarkTrainerTravel(t *testing.T) {
	var out []benchRow
	for _, tc := range []struct{ side, class, zone, city, how string }{
		{"Alliance", "DRUID", "Westfall", "Darnassus", "boat to Auberdine"},
		{"Horde", "HUNTER", "Tirisfal Glades", "Orgrimmar", "zeppelin to Orgrimmar"},
		{"Horde", "DRUID", "Hillsbrad Foothills", "Thunder Bluff", "zeppelin to Orgrimmar"},
		{"Alliance", "WARRIOR", "Dun Morogh", "Ironforge", ""},
		{"Alliance", "PRIEST", "Dun Morogh", "Ironforge", ""},
		{"Horde", "WARRIOR", "Mulgore", "Thunder Bluff", ""},
		{"Horde", "HUNTER", "Mulgore", "Thunder Bluff", ""},
		{"Horde", "ROGUE", "Tirisfal Glades", "Undercity", ""},
		{"Alliance", "MAGE", "Elwynn Forest", "Stormwind City", ""},
		{"Horde", "WARRIOR", "Durotar", "Orgrimmar", ""},
	} {
		lv := 20
		c := character.Character{Class: tc.class, Faction: tc.side, Level: &lv, Location: &character.Location{Zone: tc.zone}}
		pl, ok := levelup.TrainAt(c, nil)
		line := pl.Line(tc.class)
		good := ok && pl.City == tc.city
		if tc.how != "" {
			good = good && strings.Contains(strings.ToLower(line), strings.ToLower(tc.how))
		} else {
			good = good && !strings.Contains(strings.ToLower(line), "zeppelin") && !strings.Contains(strings.ToLower(line), "boat")
		}
		ref := tc.city + ", no zeppelin or boat"
		if tc.how != "" {
			ref = tc.city + ", " + tc.how
		}
		out = append(out, benchRow{fmt.Sprintf("%s %s in %s: where to train and how to get there", tc.side, tc.class, tc.zone), line, ref, qvTravelSrc, resultIf(good, "the city beside the zone, or the zeppelin or boat")})
	}
	// in the city already: nothing to travel
	lv := 20
	pl, _ := levelup.TrainAt(character.Character{Class: "PRIEST", Faction: "Alliance", Level: &lv, Location: &character.Location{Zone: "Darnassus"}}, nil)
	out = append(out, benchRow{"Alliance PRIEST in Darnassus: already there", pl.Line("PRIEST"), "Darnassus, you are here, no travel", qvTravelSrc, resultIf(pl.City == "Darnassus" && pl.Here && pl.Cross == "" && pl.From == "", "a trip was named")})
	finish(t, "advice-trainer-travel", "Offline class trainer travel: the city beside the zone and the crossing", out)
}

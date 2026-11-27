package paths_test

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/WoW-Companion/advisor/internal/advice"
	"github.com/WoW-Companion/advisor/internal/ask"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/inventory"
	"github.com/WoW-Companion/advisor/internal/paths"
	"github.com/WoW-Companion/advisor/internal/questie"
	"github.com/WoW-Companion/advisor/internal/quests"
	"github.com/WoW-Companion/advisor/internal/store"
)

// The offline item-started quest, escort and timed quest, and race, class and faction quest restriction benchmark: what the advisor does with an item that begins a quest, what it says of an escort or a timed
// quest, and which quests it offers a character of a race, class and side, beside the dataset's rows and the game's own rules, with no network. It reads testdata/drops_snapshot and
// testdata/consumables_snapshot (items, loot_sources) and testdata/places_snapshot (quests_observed, quests). The dataset holds no link between an item and the quest it starts, no escort flag or
// escort NPC, no time limit, no race or class mask for any quest and no quest giver row; it holds the faction (and, for a class quest, the class) a few players saw a quest under. The item-to-quest pairs
// and the quest givers of the restriction cases are therefore composed here from a real item (or a real quest of quests_observed) and a synthetic QuestieDB, as the quest reward benchmark does, and the
// cases the dataset cannot answer check that the answer is framed with the line that says so (ask.LimitFacts) and that the advisor states nothing from memory. The game's rules: an item marked "This
// Item Begins a Quest" is the only way to that quest, so it is kept until the quest is taken (Warcraft Wiki, Quest); a quest closed to the character's race or class, or given only by the other
// faction's NPCs, cannot be taken; a failed quest is abandoned and taken again from its giver where it is offered again. A difference the dataset's own rows cause is marked "(data)". make
// offline-benchmark runs it; WOWC_BENCH_REPORT receives the tables.

const (
	qrDropsDir    = "../../testdata/drops_snapshot"
	qrConsumeDir  = "../../testdata/consumables_snapshot"
	qrItemSrc     = "dataset items (class_id 12 quest, bonding 4 quest item), loot_sources (creature drops, kills); composed pairing with a quests_observed quest; game: \"This Item Begins a Quest\" (Warcraft Wiki, Quest)"
	qrEscortSrc   = "dataset quests, quests_observed (no escort flag, escort NPC or route); game: Warcraft Wiki, Escort quest"
	qrTimedSrc    = "dataset quests, quests_observed (no time limit or timer column); game: Warcraft Wiki, Quest (a timed quest fails when its timer runs out)"
	qrRestrictSrc = "dataset quests_observed (faction, category = class heading; no race or class mask); player's QuestieDB (requiredRaces, requiredClasses, friendlyToFaction); game: Warcraft Wiki, Quest (race, class and faction requirements)"
	qrNoneSrc     = "the dataset has no such row or column (no item-to-quest link, escort or timer flag, race or class mask)"
)

// qrSources serves the first directory that holds the table.
type qrSources []store.DirSource

func (s qrSources) Open(table string) (io.ReaderAt, int64, error) {
	var err error
	for _, d := range s {
		var r io.ReaderAt
		var n int64
		if r, n, err = d.Open(table); err == nil {
			return r, n, nil
		}
	}
	return nil, 0, err
}

// qrDuo reads the items and loot rows of two snapshots as one table (the drops snapshot's quest items have loot rows, the consumables snapshot's have none); every other table is the first's.
type qrDuo struct{ a, b store.DataStore }

func (d qrDuo) merged(table string) bool { return table == "items" || table == "loot_sources" }

func (d qrDuo) Get(table string, id int64) (store.Row, error) {
	r, err := d.a.Get(table, id)
	if err != nil && d.merged(table) {
		return d.b.Get(table, id)
	}
	return r, err
}

func (d qrDuo) Filter(table string, ranges ...store.Range) ([]store.Row, error) {
	out, err := d.a.Filter(table, ranges...)
	if err != nil || !d.merged(table) {
		return out, err
	}
	more, err := d.b.Filter(table, ranges...)
	return append(out, more...), err
}

func (d qrDuo) SearchName(table, text string, limit int) ([]store.Row, error) {
	out, err := d.a.SearchName(table, text, limit)
	if err != nil || !d.merged(table) || len(out) >= limit {
		return out, err
	}
	more, err := d.b.SearchName(table, text, limit-len(out))
	return append(out, more...), err
}

var (
	qrOnce  sync.Once
	qrCache store.DataStore
)

func qrStore() store.DataStore {
	qrOnce.Do(func() {
		qrCache = qrDuo{store.New(qrSources{store.DirSource(qrDropsDir), store.DirSource(qbPlacesDir)}), store.New(qrSources{store.DirSource(qrConsumeDir), store.DirSource(qbPlacesDir)})}
	})
	return qrCache
}

// qrChar is a character with bags holding the named items.
func qrChar(t *testing.T, name, class, race, faction string, level int, items ...string) character.Character {
	t.Helper()
	c := usabChar(class, level, faction)
	c.Name, c.Race, c.BagsCaptured, c.BankCaptured, c.Equipment = name, race, true, true, map[string]character.EquippedItem{}
	for _, it := range items {
		c.Bags = append(c.Bags, character.ItemStack{ItemID: int(num(rowByName(t, qrStore(), "items", it)["id"])), Count: 1, Name: it})
	}
	return c
}

// ---- items that begin a quest ----

// qrStarters are quest items the dataset holds, each paired with a quest of quests_observed that a Horde character can take. The pair is composed: the dataset has no item-to-quest link.
var qrStarters = []struct {
	item  string
	quest int
}{
	{"Skullsplitter Tusk", 8},
	{"Split Bone Necklace", 193},
	{"Heart of Zeal", 628},
	{"Skin of Shadow", 845},
	{"Frayed Abomination Stitching", 547},
}

// qrIndex is a synthetic QuestieDB whose quests begin from the item ids (no giver), with the race mask the side's dataset faction gives.
func qrIndex(t *testing.T, races uint64, pairs ...[2]int) *questie.Index {
	t.Helper()
	ix := &questie.Index{Quests: map[int]*questie.Quest{}}
	for _, p := range pairs {
		r := qbQuest(t, int64(p[1]))
		ix.Quests[p[1]] = &questie.Quest{ID: p[1], Name: text(r["title"]), Level: int(num(r["level"])), Races: races, StartItems: []int{p[0]}}
	}
	return ix
}

func qrItemID(t *testing.T, name string) int {
	return int(num(rowByName(t, qrStore(), "items", name)["id"]))
}

func qrReview(t *testing.T, c character.Character, ix *questie.Index) (inventory.Group, string) {
	t.Helper()
	res := inventory.ReviewWith(c, qrStore(), ix)
	for g, es := range res.Groups {
		for _, e := range es {
			return g, e.Reason
		}
	}
	return "", ""
}

// TestOfflineBenchmarkItemStartedQuestKept: an item that begins a quest the character can still take is kept, in the bag review and across the character and an alt, and says which quest it starts.
func TestOfflineBenchmarkItemStartedQuestKept(t *testing.T) {
	var out []benchRow
	for _, tc := range qrStarters {
		row := rowByName(t, qrStore(), "items", tc.item)
		q := qbQuest(t, int64(tc.quest))
		ix := qrIndex(t, questie.HordeMask, [2]int{qrItemID(t, tc.item), tc.quest})
		c := qrChar(t, "Gruk", "WARRIOR", "Orc", "Horde", 40, tc.item)
		g, why := qrReview(t, c, ix)
		out = append(out, benchRow{fmt.Sprintf("%s begins %q (quest %d): the bag review of a level 40 orc warrior", tc.item, text(q["title"]), tc.quest), fmt.Sprintf("%s: %s", g, why),
			fmt.Sprintf("keep: item class %d, bonding %d, the quest is not taken (quests_observed faction %v)", num(row["class_id"]), num(row["bonding"]), q["faction"]), qrItemSrc,
			resultIf(g == inventory.Quest && strings.Contains(why, "starts "+text(q["title"])), "the item that begins the quest was not kept for it")})
		alt := qrChar(t, "Bram", "PRIEST", "Orc", "Horde", 12)
		res := inventory.ReviewAllWith([]character.Character{c, alt}, qrStore(), ix)
		l, ok := kaLine(res, "Gruk", tc.item)
		out = append(out, benchRow{tc.item + ": the review across the warrior and an alt priest", fmt.Sprintf("%s %q: %s", l.Action, l.To, l.Reason), "keep: not tossed, sold or mailed to the alt", qrItemSrc,
			resultIf(ok && l.Action == inventory.Keep_ && l.To == "" && res.SlotsFreed == 0, "the quest-starting item was tossed, sold or mailed")})
	}
	finish(t, "advice-item-started-quest-kept", "Offline item-started quests: the item is kept for the quest it begins", out)
}

// TestOfflineBenchmarkItemStartedQuestNotKept: the item is no starter for a character who has the quest in the log, has done it, or cannot take it (the quest is the other side's); it is then a quest item
// no quest needs, as before.
func TestOfflineBenchmarkItemStartedQuestNotKept(t *testing.T) {
	var out []benchRow
	item, id := "Skullsplitter Tusk", 8
	iid := qrItemID(t, item)
	ix := qrIndex(t, questie.HordeMask, [2]int{iid, id})
	for _, tc := range []struct {
		name string
		mut  func(c *character.Character)
		ix   *questie.Index
		ref  string
	}{
		{"the quest is done", func(c *character.Character) { c.CompletedQuests = []int{id} }, ix, "not kept for the quest: it is done"},
		{"the quest is in the log", func(c *character.Character) { c.Quests = []character.Quest{{QuestID: id, Title: "A Rogue's Deal"}} }, ix, "not kept as a starter: the quest is taken"},
		{"the quest is the other side's", func(c *character.Character) { c.Faction, c.Race = "Alliance", "Human" }, ix, "not kept: a Human cannot take a Horde quest"},
	} {
		c := qrChar(t, "Gruk", "WARRIOR", "Orc", "Horde", 40, item)
		tc.mut(&c)
		g, why := qrReview(t, c, tc.ix)
		out = append(out, benchRow{item + " in the bags when " + tc.name, fmt.Sprintf("%s: %s", g, why), tc.ref, qrItemSrc, resultIf(!strings.Contains(why, "starts "), "the item was kept as a starter for a quest the character cannot take")})
	}
	finish(t, "advice-item-started-quest-not-kept", "Offline item-started quests: the item is no starter once the quest is taken, done or closed to the character", out)
}

// TestOfflineBenchmarkItemStartedQuestSource: where the item comes from is the dataset's loot row, not a guess: the creature and the kill share.
func TestOfflineBenchmarkItemStartedQuestSource(t *testing.T) {
	var out []benchRow
	for _, tc := range qrStarters {
		row := rowByName(t, qrStore(), "items", tc.item)
		id := num(row["id"])
		res := paths.For(qrChar(t, "Gruk", "WARRIOR", "Orc", "Horde", 40), qrStore(), id, paths.Options{})
		rows, err := qrStore().Filter("loot_sources", store.Range{Column: "item_id", Min: id, Max: id})
		if err != nil {
			t.Fatalf("loot_sources of %s: %v", tc.item, err)
		}
		if len(rows) > paths.WorldDropMany {
			// more creatures than the world-drop rule allows: the advisor calls it a random drop, as the drops benchmark expects; a quest item of such a source is a note, not a difference
			out = append(out, benchRow{tc.item + ": where it drops", firstLine(res.Text()), fmt.Sprintf("dataset: %d creatures carry it (more than %d): the world-drop rule calls it a random drop", len(rows), paths.WorldDropMany), qrItemSrc,
				resultIf(strings.Contains(res.Text(), "random drop"), "not called a random drop")})
			continue
		}
		if len(rows) > 0 {
			best, share := rdBestCreature(t, id)
			out = append(out, benchRow{tc.item + ": where it drops", firstLine(res.Text()), fmt.Sprintf("dataset: %s, %.0f%% of its kills", best, share), qrItemSrc,
				resultIf(strings.Contains(res.Text(), best), "the creature the dataset names is not in the answer")})
			continue
		}
		out = append(out, benchRow{tc.item + ": where it drops", firstLine(res.Text()), "(data) no loot_sources row: the dataset names no source", qrItemSrc,
			resultIf(!strings.Contains(strings.ToLower(res.Text()), "drops from"), "a source was named that the dataset does not hold")})
	}
	finish(t, "advice-item-started-quest-source", "Offline item-started quests: where the item drops comes from the dataset's loot rows", out)
}

// ---- escort and timed quests ----

func qrLimitBatch(t *testing.T, area, title, src, head string, qs []string, want bool, ref string) {
	t.Helper()
	var out []benchRow
	for _, q := range qs {
		f := ask.LimitFacts(q)
		got := vtSays(f, head)
		out = append(out, benchRow{"\"" + q + "\": the facts", firstLineMatching(f, head), ref, src, resultIf(got == want, map[bool]string{true: "no line saying the data holds none, so one may be made up from memory", false: "the line was added to a question it does not fit"}[want])})
	}
	finish(t, area, title, out)
}

func TestOfflineBenchmarkEscortQuestFacts(t *testing.T) {
	qrLimitBatch(t, "advice-escort-quest-facts", "Offline escort quests: the data holds none, and the answer says so", qrEscortSrc, "Escort quests:", []string{
		"how do I do the escort quest in the Barrens?", "which quests in Duskwood are escort quests?", "what happens if the NPC dies during an escort?", "is there a quest where I have to escort someone through Arathi Highlands?", "can I escort Gilthares alone at level 20?",
	}, true, "no escort flag, NPC or route in the data: say so, call no quest an escort")
}

func TestOfflineBenchmarkEscortQuestScope(t *testing.T) {
	qrLimitBatch(t, "advice-escort-quest-scope", "Offline escort quests: other quest questions get no escort line", qrEscortSrc, "Escort quests:", []string{
		"what quests are in the Barrens?", "where is the quest giver in Goldshire?", "which quest rewards a Thunderfury?",
	}, false, "the question names no escort")
}

func TestOfflineBenchmarkTimedQuestFacts(t *testing.T) {
	qrLimitBatch(t, "advice-timed-quest-facts", "Offline timed quests: the data holds no time limit, and the answer says so", qrTimedSrc, "Timed quests:", []string{
		"which timed quests can I do at level 30?", "how long is the time limit on this quest?", "is there a timer on the quest in Wetlands?", "how long do I have to complete the quest from Fizzle?", "what if the quest times out?",
	}, true, "no time limit or timer in the data: say so, give none")
}

func TestOfflineBenchmarkTimedQuestScope(t *testing.T) {
	qrLimitBatch(t, "advice-timed-quest-scope", "Offline timed quests: other quest questions get no timer line", qrTimedSrc, "Timed quests:", []string{
		"what quests are in Westfall?", "which quest gives the best XP at level 20?", "when does the Darkmoon Faire start?",
	}, false, "the question asks of no time limit")
}

// TestOfflineBenchmarkFailedQuestStep: a failed quest (a timer run out, an escort dead) is shown as one to abandon and, where its giver offers it again, to take again; it is not said to be lost.
func TestOfflineBenchmarkFailedQuestStep(t *testing.T) {
	var out []benchRow
	for _, tc := range []struct {
		id    int
		title string
		level int
	}{{9, "The Killing Fields", 15}, {845, "The Zhevra", 13}} {
		lv, failed := 20, true
		c := character.Character{Name: "Benchtest", Faction: "Horde", Class: "WARRIOR", Level: &lv, Quests: []character.Quest{{QuestID: tc.id, Title: tc.title, Level: &tc.level, Failed: &failed}}}
		var step advice.Step
		for _, s := range advice.WhatsNext(c, qrStore()) {
			if s.QuestID == tc.id {
				step = s
			}
		}
		out = append(out, benchRow{tc.title + " failed in the log", step.Title + ": " + step.Reason, "abandon it, then take it again from its giver where it is offered again", qrTimedSrc,
			resultIf(strings.HasPrefix(step.Title, advice.AbandonPrefix) && strings.Contains(step.Reason, "take it again"), "no way back to the quest was given for a failed quest")})
	}
	finish(t, "advice-failed-quest-step", "Offline escort and timed quests: a failed quest is abandoned and taken again", out)
}

// ---- race, class and faction restrictions ----

// qrVale is a synthetic zone with a Horde-friendly and an Alliance-friendly giver for each real quest id of quests_observed the case names.
func qrVale(t *testing.T, give map[int]string, races map[int]uint64, classes map[int]uint64) *questie.Index {
	t.Helper()
	ix := &questie.Index{Quests: map[int]*questie.Quest{}, Npcs: map[int]questie.Entity{}, Areas: map[int]questie.Area{1: {UIMap: 1, Zone: "Synthetic Vale"}}}
	for id, side := range give {
		r := qbQuest(t, int64(id))
		npc := 1000 + id
		ix.Npcs[npc] = questie.Entity{Name: "Giver of " + text(r["title"]), Faction: side, Spawns: []questie.Spawn{{Area: 1, X: 40, Y: 40}}}
		ix.Quests[id] = &questie.Quest{ID: id, Name: text(r["title"]), Level: 20, Races: races[id], Classes: classes[id], Starters: []questie.Ref{{ID: npc}}}
	}
	return ix
}

func qrOffered(c character.Character, ix *questie.Index) map[string]bool {
	out := map[string]bool{}
	for _, o := range ix.Available(advice.WhoOf(c), questie.Where{Zone: "Synthetic Vale"}, nil) {
		out[o.Quest.Name] = true
	}
	return out
}

// qrCases: the quest (a real one of quests_observed, with the faction or class the dataset gives it), the giver's side or masks the QuestieDB gives it, the character, and whether the game lets it take the quest.
var qrCases = []struct {
	label       string
	quest       int
	side        string
	races, cls  uint64
	race, class string
	faction     string
	can         bool
	ref         string
}{
	{"a Horde quest, Horde-friendly giver, orc warrior", 2, "H", questie.HordeMask, 0, "Orc", "WARRIOR", "Horde", true, "dataset quests_observed faction horde: an orc takes it"},
	{"a Horde quest, Horde-friendly giver, human mage", 2, "H", questie.HordeMask, 0, "Human", "MAGE", "Alliance", false, "dataset quests_observed faction horde: an Alliance character cannot take it"},
	{"an Alliance quest, Alliance giver, human warrior", 5, "A", questie.AllianceMask, 0, "Human", "WARRIOR", "Alliance", true, "dataset quests_observed faction alliance: a human takes it"},
	{"an Alliance quest, Alliance giver, troll hunter", 5, "A", questie.AllianceMask, 0, "Troll", "HUNTER", "Horde", false, "dataset quests_observed faction alliance: a Horde character cannot take it"},
	{"an Alliance quest with a Horde giver only (a neutral quest by race), orc rogue", 9, "H", 0, 0, "Orc", "ROGUE", "Horde", true, "no race mask: the Horde giver is friendly to the orc"},
	{"an Alliance quest whose only giver is Horde-friendly, human rogue", 9, "H", 0, 0, "Human", "ROGUE", "Alliance", false, "the only giver is of the other side: the quest cannot be taken from it"},
	{"a quest masked to orc and troll, tauren druid", 193, "H", 1<<1 | 1<<7, 0, "Tauren", "DRUID", "Horde", false, "race mask orc, troll: a tauren is closed out (the dataset holds no race mask)"},
	{"a quest masked to orc and troll, troll priest", 193, "H", 1<<1 | 1<<7, 0, "Troll", "PRIEST", "Horde", true, "race mask orc, troll: a troll takes it"},
	{"a warlock class quest (quests_observed category Warlock), orc warlock", 1507, "H", questie.HordeMask, 1 << 8, "Orc", "WARLOCK", "Horde", true, "class mask warlock: a warlock takes it"},
	{"a warlock class quest, orc warrior", 1507, "H", questie.HordeMask, 1 << 8, "Orc", "WARRIOR", "Horde", false, "class mask warlock: a warrior cannot take it"},
	{"a paladin class quest (quests_observed category Paladin), human paladin", 98601, "A", questie.AllianceMask, 1 << 1, "Human", "PALADIN", "Alliance", true, "class mask paladin: a paladin takes it"},
	{"a paladin class quest, human priest", 98601, "A", questie.AllianceMask, 1 << 1, "Human", "PRIEST", "Alliance", false, "class mask paladin: a priest cannot take it"},
}

func qrRestrictRows(t *testing.T, from, to int) []benchRow {
	t.Helper()
	var out []benchRow
	for _, tc := range qrCases[from:to] {
		q := qbQuest(t, int64(tc.quest))
		ix := qrVale(t, map[int]string{tc.quest: tc.side}, map[int]uint64{tc.quest: tc.races}, map[int]uint64{tc.quest: tc.cls})
		c := usabChar(tc.class, 20, tc.faction)
		c.Race = tc.race
		offered := qrOffered(c, ix)[text(q["title"])]
		out = append(out, benchRow{fmt.Sprintf("%s: %q offered to a level 20 %s %s", tc.label, text(q["title"]), strings.ToLower(tc.race), strings.ToLower(tc.class)), fmt.Sprintf("offered %v", offered),
			fmt.Sprintf("the game: can take %v (%s; dataset faction %v, category %v)", tc.can, tc.ref, q["faction"], q["category"]), qrRestrictSrc,
			resultIf(offered == tc.can, fmt.Sprintf("offered %v, the game says %v", offered, tc.can))})
	}
	return out
}

func TestOfflineBenchmarkQuestRestrictionA(t *testing.T) {
	finish(t, "advice-quest-restriction-a", "Offline quest restrictions: a quest the character cannot take is not offered (faction and race)", qrRestrictRows(t, 0, 8))
}

func TestOfflineBenchmarkQuestRestrictionB(t *testing.T) {
	finish(t, "advice-quest-restriction-b", "Offline quest restrictions: a quest the character cannot take is not offered (class)", qrRestrictRows(t, 8, len(qrCases)))
}

// TestOfflineBenchmarkQuestRestrictionNoMask: the dataset-only quest finder has no race or class column to check; it says so and makes no claim of having checked.
func TestOfflineBenchmarkQuestRestrictionNoMask(t *testing.T) {
	var out []benchRow
	for _, tc := range []struct{ class, race, faction string }{{"WARRIOR", "Orc", "Horde"}, {"MAGE", "Human", "Alliance"}, {"HUNTER", "Troll", "Horde"}} {
		c := usabChar(tc.class, 30, tc.faction)
		c.Race = tc.race
		av := advice.AvailableQuests(c, qrStore())
		joined := strings.Join(av.Unchecked, " | ")
		out = append(out, benchRow{fmt.Sprintf("level 30 %s %s: the dataset-only pick-up list", strings.ToLower(tc.race), strings.ToLower(tc.class)), fmt.Sprintf("%d quests offered; unchecked: %s", len(av.Quests), joined),
			"quests holds no race or class mask: the list is not a check of either", qrRestrictSrc,
			resultIf(len(av.Quests) == 0 || (strings.Contains(joined, "race not checked") && strings.Contains(joined, "class not checked")), "quests were offered with no note that race and class were not checked")})
	}
	finish(t, "advice-quest-restriction-nomask", "Offline quest restrictions: the dataset-only list says race and class are not checked", out)
}

// ---- what the dataset cannot answer ----

func TestOfflineBenchmarkQuestRestrictionFacts(t *testing.T) {
	qrLimitBatch(t, "advice-quest-restriction-facts", "Offline quest restrictions: the data holds no race or class limit, and the answer says so", qrRestrictSrc, "Race, class and faction limits on quests:", []string{
		"which quests are Horde only in Ashenvale?", "can a troll do the Alliance quests in Westfall?", "are there quests only orcs can take?", "is this quest race-specific?", "which quests are restricted to my race?", "quests only available to the Alliance in Redridge",
	}, true, "no race or class mask in the data: say so, call no quest open or closed to a race from memory")
}

func TestOfflineBenchmarkQuestRestrictionScope(t *testing.T) {
	qrLimitBatch(t, "advice-quest-restriction-scope", "Offline quest restrictions: other questions get no race limit line", qrRestrictSrc, "Race, class and faction limits on quests:", []string{
		"what is the best Horde city for a rogue?", "which Alliance races can be a paladin?", "what quests are in the Barrens?",
	}, false, "the question asks of no quest limit")
}

func TestOfflineBenchmarkItemStartFacts(t *testing.T) {
	qrLimitBatch(t, "advice-item-start-facts", "Offline item-started quests: the data holds no item-to-quest link, and the answer says so", qrItemSrc, "Quests that start from an item:", []string{
		"which quest does Heart of Zeal start?", "what item starts the quest in Thousand Needles?", "this item begins a quest, which one?", "which drops start quests at level 30?", "is there an item-started quest in the Stockade?", "what quest does the Skullsplitter Tusk begin?",
	}, true, "no item-to-quest link in the data: say so, name no pair from memory")
}

func TestOfflineBenchmarkItemStartScope(t *testing.T) {
	qrLimitBatch(t, "advice-item-start-scope", "Offline item-started quests: other questions get no item-start line", qrItemSrc, "Quests that start from an item:", []string{
		"where does the Skullsplitter Tusk drop?", "what quests are in the Barrens?", "should I vendor this grey item?",
	}, false, "the question asks of no quest an item starts")
}

// TestOfflineBenchmarkQuestUnknown: a quest the dataset does not hold has no quest line and no row; the advisor says it does not know and invents none.
func TestOfflineBenchmarkQuestUnknown(t *testing.T) {
	var out []benchRow
	for _, id := range []int64{99999901, 99999902, 99999903, 99999904} {
		c := quests.Of(qrStore(), id)
		r, _ := qrStore().Get("quests_observed", id)
		out = append(out, benchRow{fmt.Sprintf("quest %d, a quest the dataset does not hold: a quest line", id), fmt.Sprintf("found %v, %d steps; name %q", c.Found, len(c.Steps), quests.Name(qrStore(), id)),
			"no quests or quests_observed row: nothing to state", qrNoneSrc, resultIf(!c.Found && len(c.Steps) == 0 && r == nil && quests.Name(qrStore(), id) == "", "a name or a line was made up for a quest the dataset does not hold")})
	}
	finish(t, "advice-quest-unknown", "Offline quests: a quest the dataset does not hold is not invented", out)
}

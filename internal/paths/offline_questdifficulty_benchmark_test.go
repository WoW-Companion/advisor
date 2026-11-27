package paths_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/advice"
	"github.com/WoW-Companion/advisor/internal/ask"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/route"
	"github.com/WoW-Companion/advisor/internal/session"
	"github.com/WoW-Companion/advisor/internal/store"
)

// The offline quest-difficulty benchmark: the colour the advisor gives a quest for a character's level (red, orange, yellow, green, grey), the quests it marks elite or group,
// the XP it says a quest gives, and what it says when the dataset holds nothing, beside the dataset's quests_observed rows (title, level, tag) and the game's own rules, with no network.
// It reads testdata/places_snapshot (quests, quests_observed). The dataset holds each quest's level (quests_observed) and a "group" or "dungeon" tag on some of them; it holds no
// quest XP, no minimum level to take a quest (quests.level and quests.min_level are empty), no suggested group size (quests_observed.suggested_group is empty) and no elite flag, so
// those cases check that the answer is framed with the line that says so (ask.LimitFacts) or that the advisor marks its estimate. The colour thresholds are the game's: red 5 or more
// levels above, orange 3 to 4, yellow within 2, then green down to the gray level and grey below (Warcraft Wiki, Mob difficulty colors and API GetQuestGreenRange: the gray level of a
// quest is that of a creature). A difference the dataset's own rows cause is marked "(data)". make offline-benchmark runs it; WOWC_BENCH_REPORT receives the tables.

const (
	qdColorSrc = "dataset quests_observed (quest_id, title, level); game: Warcraft Wiki Mob difficulty colors (gray level by player level; a quest turns green at the same level as a creature) and API GetQuestGreenRange (5 at level 9, 10 at level 50)"
	qdEliteSrc = "dataset quests_observed (tag \"group\" or \"dungeon\", level); game: Warcraft Wiki, Quests (a group quest is meant for a party)"
	qdXPSrc    = "Warcraft Wiki, Experience points (quest XP by level difference; quests under level 10 give full XP to four levels above); dataset quests: no XP column"
	qdNoneSrc  = "the dataset has no such row or column (quests: level and min_level empty, no XP; quests_observed: suggested_group empty, no elite flag)"
)

// qdQuest is a quest of quests_observed with the level it holds.
type qdQuest struct {
	id    int64
	title string
	level int
}

// qdLevel is the quest's row: its title and level as the dataset holds them, checked against the quest named here.
func qdLevel(t *testing.T, q qdQuest) store.Row {
	t.Helper()
	r := qbQuest(t, q.id)
	if text(r["title"]) != q.title || int(num(r["level"])) != q.level {
		t.Fatalf("quest %d: the snapshot holds %q level %d, the case names %q level %d", q.id, text(r["title"]), num(r["level"]), q.title, q.level)
	}
	return r
}

// qdColors are a character's level, a quest and the colour the game gives it. Green and grey follow the game's gray level (the rows chosen are those the Warcraft Wiki
// table and the formula agree on): at level 10 a quest of level 4 or below is grey, at 20 level 13, at 30 level 22, at 50 level 39 and at 60 level 51.
var qdColors = []struct {
	player int
	quest  qdQuest
	game   string
}{
	{10, qdQuest{380, "Night Web's Hollow", 4}, "grey"},
	{10, qdQuest{8, "A Rogue's Deal", 5}, "green"},
	{20, qdQuest{845, "The Zhevra", 13}, "grey"},
	{20, qdQuest{13, "The People's Militia", 14}, "green"},
	{30, qdQuest{248, "Looking Further", 22}, "grey"},
	{30, qdQuest{91, "Solomon's Law", 23}, "green"},
	{40, qdQuest{185, "Tiger Mastery", 31}, "grey"},
	{50, qdQuest{1108, "Indurium", 39}, "grey"},
	{50, qdQuest{338, "The Green Hills of Stranglethorn", 40}, "green"},
	{55, qdQuest{232, "Errand for Apothecary Zinge", 45}, "green"},
	{60, qdQuest{580, "Whiskey Slim's Lost Grog", 50}, "grey"},
	{20, qdQuest{25, "Stonetalon Standstill", 25}, "red"},
	{20, qdQuest{91, "Solomon's Law", 23}, "orange"},
	{20, qdQuest{248, "Looking Further", 22}, "yellow"},
	{20, qdQuest{5, "Jitters' Growling Gut", 20}, "yellow"},
}

func qdColorRows(t *testing.T, from, to int) []benchRow {
	t.Helper()
	var out []benchRow
	for _, tc := range qdColors[from:to] {
		row := qbQuest(t, tc.quest.id)
		title := text(row["title"])
		lvl := int(num(row["level"]))
		if title != tc.quest.title || lvl != tc.quest.level {
			t.Fatalf("quest %d: the snapshot holds %q level %d, the case names %q level %d", tc.quest.id, title, lvl, tc.quest.title, tc.quest.level)
		}
		got := route.DiffOf(lvl, tc.player)
		out = append(out, benchRow{fmt.Sprintf("%q (level %d) for a level %d character", title, lvl, tc.player), got,
			fmt.Sprintf("dataset level %d; game %s", lvl, tc.game), qdColorSrc, resultIf(got == tc.game, "another colour than the game's")})
	}
	return out
}

func TestOfflineBenchmarkQuestColorA(t *testing.T) {
	finish(t, "advice-questdiff-color-a", "Offline quest colour: grey and green at the gray level", qdColorRows(t, 0, 6))
}

func TestOfflineBenchmarkQuestColorB(t *testing.T) {
	finish(t, "advice-questdiff-color-b", "Offline quest colour: grey and green, higher levels", qdColorRows(t, 6, 11))
}

func TestOfflineBenchmarkQuestColorC(t *testing.T) {
	finish(t, "advice-questdiff-color-c", "Offline quest colour: red, orange and yellow", qdColorRows(t, 11, len(qdColors)))
}

// TestOfflineBenchmarkQuestGreyLevel: the highest level that is grey for a character of a level, beside the game's gray level (Warcraft Wiki table), at the levels where the table
// is certain (the table's rows from 45 to 49 and 57 disagree with its own formula and are left out).
func TestOfflineBenchmarkQuestGreyLevel(t *testing.T) {
	var out []benchRow
	for _, tc := range []struct{ player, game int }{{5, 0}, {6, 1}, {10, 4}, {19, 13}, {20, 13}, {30, 22}, {39, 31}, {40, 31}, {44, 35}, {50, 39}, {55, 43}, {59, 47}, {60, 51}, {70, 61}} {
		got := route.GreyLevel(tc.player)
		out = append(out, benchRow{fmt.Sprintf("the highest grey quest level for a level %d character", tc.player), fmt.Sprint(got), fmt.Sprintf("game %d", tc.game), qdColorSrc, resultIf(got == tc.game, "another gray level")})
	}
	finish(t, "advice-questdiff-grey-level", "Offline quest colour: the gray level by character level", out)
}

// qdStep is the quest's open step in the what's-next list for a character of the level holding the quest, or the turn-in step when it is complete.
func qdStep(t *testing.T, level int, id int64, done bool) (advice.Step, bool) {
	t.Helper()
	c := levChar("WARRIOR", level)
	q := character.Quest{QuestID: int(id)}
	if done {
		yes := true
		q.Complete = &yes
	}
	c.Quests = []character.Quest{q}
	for _, s := range advice.WhatsNext(c, qbStore()) {
		if s.QuestID == int(id) {
			return s, true
		}
	}
	return advice.Step{}, false
}

var qdGroup = []struct {
	q          qdQuest
	tag        string
	player     int
	completed  bool
	wantElite  bool
	wantReason string
}{
	{qdQuest{314, "Protecting the Herd", 12}, "group", 12, false, true, "level 12 quest, you are 12"},
	{qdQuest{19, "Tharil'zun", 25}, "group", 24, false, true, "level 25 quest, you are 24"},
	{qdQuest{547, "Humbert's Sword", 30}, "group", 30, false, true, "level 30 quest, you are 30"},
	{qdQuest{5088, "Arikara", 28}, "group", 28, true, true, "ready to turn in"},
	{qdQuest{1489, "Hamuul Runetotem", 16}, "dungeon", 16, false, true, "level 16 quest, you are 16"},
	{qdQuest{386, "What Comes Around...", 25}, "dungeon", 25, false, true, "level 25 quest, you are 25"},
	{qdQuest{35, "Further Concerns", 10}, "", 10, false, false, "level 10 quest, you are 10"},
	{qdQuest{2, "Sharptalon's Claw", 30}, "", 30, false, false, "level 30 quest, you are 30"},
}

func qdGroupRows(t *testing.T, from, to int) []benchRow {
	t.Helper()
	var out []benchRow
	for _, tc := range qdGroup[from:to] {
		row := qdLevel(t, tc.q)
		tag := text(row["tag"])
		s, ok := qdStep(t, tc.player, tc.q.id, tc.completed)
		res := resultIf(tag == tc.tag && ok && s.Elite == tc.wantElite && s.Level == tc.q.level && strings.Contains(s.Reason, tc.wantReason), "the elite flag, the level or the reason is not the dataset's")
		out = append(out, benchRow{fmt.Sprintf("%q: elite or group for a level %d character (completed %v)", tc.q.title, tc.player, tc.completed),
			fmt.Sprintf("elite %v, level %d, %q", s.Elite, s.Level, s.Reason), fmt.Sprintf("dataset tag %q, level %d: elite %v", tag, tc.q.level, tc.wantElite), qdEliteSrc, res})
	}
	return out
}

func TestOfflineBenchmarkQuestEliteGroup(t *testing.T) {
	finish(t, "advice-questdiff-elite-group", "Offline elite and group quests: players' group tag", qdGroupRows(t, 0, 4))
}

func TestOfflineBenchmarkQuestEliteDungeon(t *testing.T) {
	finish(t, "advice-questdiff-elite-dungeon", "Offline elite and group quests: dungeon tag and none", qdGroupRows(t, 4, len(qdGroup)))
}

// TestOfflineBenchmarkQuestEliteTagCounts: over a spread of quests_observed (the first 25 tagged group or dungeon and the first 25 untagged), the quests the advisor marks elite are
// exactly those a player tagged.
func TestOfflineBenchmarkQuestEliteTagCounts(t *testing.T) {
	rows, err := qbStore().Filter("quests_observed", store.Range{Column: "quest_id", Min: int64(1)})
	if err != nil || len(rows) == 0 {
		t.Fatalf("quests_observed: %v", err)
	}
	tagged, plain, marked, wrong, seen := 0, 0, 0, 0, 0
	for _, r := range rows {
		tag := text(r["tag"])
		want := tag == "group" || tag == "dungeon"
		if (want && tagged == 25) || (!want && plain == 25) {
			continue
		}
		if want {
			tagged++
		} else {
			plain++
		}
		c := levChar("WARRIOR", int(num(r["level"]))) // the level the quest is for: a quest far off the character's level is not listed at all
		c.Quests = []character.Quest{{QuestID: int(num(r["quest_id"]))}}
		got := false
		for _, s := range advice.WhatsNext(c, qbStore()) {
			if s.QuestID == int(num(r["quest_id"])) {
				got, seen = s.Elite, seen+1
			}
		}
		if got {
			marked++
		}
		if got != want {
			wrong++
		}
	}
	out := []benchRow{{fmt.Sprintf("%d quests of quests_observed (%d tagged, %d untagged): those the advisor marks elite or group", tagged+plain, tagged, plain), fmt.Sprintf("%d listed, %d marked, %d unlike the tag", seen, marked, wrong),
		fmt.Sprintf("dataset: %d tagged group or dungeon", tagged), qdEliteSrc, resultIf(seen == tagged+plain && marked == tagged && wrong == 0, "the advisor's elite marks are not the dataset's tags")}}
	finish(t, "advice-questdiff-elite-tags", "Offline elite and group quests: a spread of tagged and untagged quests", out)
}

// TestOfflineBenchmarkQuestLevelUnknown: a quest the dataset does not hold has no level; the advisor says so and guesses none.
func TestOfflineBenchmarkQuestLevelUnknown(t *testing.T) {
	var out []benchRow
	for _, id := range []int64{999991, 999992} {
		s, ok := qdStep(t, 30, id, false)
		out = append(out, benchRow{fmt.Sprintf("quest %d, in no table, for a level 30 character", id), fmt.Sprintf("level %d, %q, elite %v", s.Level, s.Reason, s.Elite), "no level and no elite mark: \"quest level unknown\"", qdNoneSrc,
			resultIf(ok && s.Level == 0 && !s.Elite && strings.Contains(s.Reason, "quest level unknown"), "a level or an elite mark was made up")})
	}
	finish(t, "advice-questdiff-level-unknown", "Offline quest level: a quest with no row", out)
}

// ---- quest XP ----

func qdXPIn(level int, table func(int) (int, bool)) session.In {
	return session.In{Char: levChar("WARRIOR", level), Info: session.Info{XP: table}}
}

// TestOfflineBenchmarkQuestDiffXPShare: the share of a quest's XP a character keeps, at the first cut and on the quests under level 10 whose first cut is a level earlier.
func TestOfflineBenchmarkQuestDiffXPShare(t *testing.T) {
	var out []benchRow
	for _, tc := range []struct {
		mine, quest int
		share       float64
	}{{9, 5, 1}, {10, 5, 0.8}, {14, 9, 0.8}, {15, 10, 1}, {16, 10, 0.8}, {37, 30, 0.6}} {
		xp, est := qdXPIn(tc.mine, func(int) (int, bool) { return 500, true }).QuestXP(1, tc.quest)
		out = append(out, benchRow{fmt.Sprintf("a level %d character, a level %d quest worth 500 XP", tc.mine, tc.quest), fmt.Sprintf("%d XP, estimated %v", xp, est),
			fmt.Sprintf("%d XP (%d%%)", int(tc.share*500), int(tc.share*100)), qdXPSrc, resultIf(xp == int(tc.share*500) && !est, "another share than the game's")})
	}
	finish(t, "advice-questdiff-xp-share", "Offline quest XP: the share a character keeps", out)
}

// TestOfflineBenchmarkQuestDiffXPEstimate: with no XP table the dataset has no quest XP to give; the advisor estimates from the quest's level and marks it an estimate.
func TestOfflineBenchmarkQuestDiffXPEstimate(t *testing.T) {
	var out []benchRow
	for _, tc := range []struct {
		mine int
		q    qdQuest
		want int
	}{{20, qdQuest{5, "Jitters' Growling Gut", 20}, 750}, {30, qdQuest{2, "Sharptalon's Claw", 30}, 1150}, {40, qdQuest{2, "Sharptalon's Claw", 30}, 115}} {
		qdLevel(t, tc.q)
		xp, est := qdXPIn(tc.mine, nil).QuestXP(int(tc.q.id), tc.q.level)
		out = append(out, benchRow{fmt.Sprintf("%q (level %d), a level %d character, no XP table", tc.q.title, tc.q.level, tc.mine), fmt.Sprintf("%d XP, estimated %v", xp, est),
			fmt.Sprintf("an estimate, never a dataset value (the dataset holds no quest XP); about %d XP", tc.want), qdNoneSrc, resultIf(est && xp >= tc.want-60 && xp <= tc.want+60, "no estimate mark, or another figure")})
	}
	finish(t, "advice-questdiff-xp-estimate", "Offline quest XP: the estimate is marked", out)
}

// TestOfflineBenchmarkQuestDiffXPTable: the player's own XP table is used as it is, unmarked, before the share is cut; a quest with no level counts as the character's level and is marked an estimate.
func TestOfflineBenchmarkQuestDiffXPTable(t *testing.T) {
	var out []benchRow
	table := func(id int) (int, bool) { return 1200, id == 91 }
	xp, est := qdXPIn(23, table).QuestXP(91, 23)
	out = append(out, benchRow{"a quest in the player's XP table, 1200 XP at its own level", fmt.Sprintf("%d XP, estimated %v", xp, est), "1200 XP, from the table", qdXPSrc, resultIf(xp == 1200 && !est, "the table figure changed or was marked an estimate")})
	xp, est = qdXPIn(23, table).QuestXP(92, 0)
	out = append(out, benchRow{"a quest missing from the table, no level known, level 23 character", fmt.Sprintf("%d XP, estimated %v", xp, est), "an estimate (870 XP at the character's own level), marked", qdNoneSrc, resultIf(est && xp == session.Estimate(23), "a figure with no mark, or another estimate")})
	finish(t, "advice-questdiff-xp-table", "Offline quest XP: the player's table and the missing level", out)
}

// ---- no answer in the dataset ----

func qdEmpty(t *testing.T, table, column string, ids ...int64) bool {
	t.Helper()
	for _, id := range ids {
		var r store.Row
		var err error
		if table == "quests" {
			r, err = qbStore().Get("quests", id)
		} else {
			rows, e := qbStore().Filter(table, store.Range{Column: "quest_id", Min: id, Max: id})
			if e == nil && len(rows) > 0 {
				r = rows[0]
			}
			err = e
		}
		if err != nil {
			t.Fatalf("%s %d: %v", table, id, err)
		}
		if r != nil && r[column] != nil {
			return false
		}
	}
	return true
}

// TestOfflineBenchmarkQuestNoAnswer: the dataset holds no quest XP, group size, elite flag or minimum level; each question gets the line that says so, whichever way it is put, and no
// question about the quests the advisor can answer (colour, level, what next) gets it.
func TestOfflineBenchmarkQuestNoAnswer(t *testing.T) {
	var out []benchRow
	for _, tc := range []struct {
		q, topic, table, column string
		ids                     []int64
	}{
		{"how much XP does the quest Tharil'zun give?", "Quest XP rewards:", "quests", "xp", []int64{19, 2}},
		{"what XP will that quest reward me at level 30?", "Quest XP rewards:", "quests", "xp_reward", []int64{547}},
		{"how many players does this quest need?", "Elite and group quests:", "quests_observed", "suggested_group", []int64{19, 547, 314}},
		{"is this an elite quest?", "Elite and group quests:", "quests_observed", "elite", []int64{19, 5}},
		{"can I solo the quest Protecting the Herd?", "Elite and group quests:", "quests_observed", "suggested_group", []int64{314}},
		{"what is the minimum level for this quest?", "Quest minimum level:", "quests", "min_level", []int64{19, 5, 2}},
		{"what level do I need to pick up the quest?", "Quest minimum level:", "quests", "level", []int64{5, 2}},
	} {
		got := ask.LimitFacts(tc.q)
		empty := qdEmpty(t, tc.table, tc.column, tc.ids...)
		out = append(out, benchRow{tc.q, firstLineMatching(got, tc.topic), fmt.Sprintf("dataset %s.%s empty: %v; the answer says the data has none", tc.table, tc.column, empty), qdNoneSrc,
			resultIf(empty && strings.Contains(got, tc.topic) && strings.Contains(got, "Say so"), "no line that says the data has none, or the dataset holds one")})
	}
	for _, q := range []string{"what colour is a level 30 quest for my level 20?", "which quests are grey for me?", "what quest should I do next?"} {
		got := ask.LimitFacts(q)
		out = append(out, benchRow{q, fmt.Sprintf("limit line %q", got), "no limit line: the advisor answers it from the quest's level", qdColorSrc, resultIf(got == "", "a no-data line on a question the advisor can answer")})
	}
	finish(t, "advice-questdiff-no-answer", "Offline quest difficulty: what the dataset does not hold", out)
}

// TestOfflineBenchmarkQuestGreyFlag: the "grey" the quest triage puts on a quest ("is it worth it?", "what should I abandon?") is the game's grey, not a quest a few levels
// below that still gives most of its XP: quests of the dataset's levels for a character who has them in the log.
func TestOfflineBenchmarkQuestGreyFlag(t *testing.T) {
	var out []benchRow
	for _, tc := range []struct {
		player int
		quest  qdQuest
		grey   bool
	}{{20, qdQuest{845, "The Zhevra", 13}, true}, {20, qdQuest{13, "The People's Militia", 14}, false}, {50, qdQuest{1108, "Indurium", 39}, true}, {50, qdQuest{338, "The Green Hills of Stranglethorn", 40}, false}} {
		row := qdLevel(t, tc.quest)
		lvl, xpMax, done := tc.quest.level, 10000, false
		c := levChar("WARRIOR", tc.player)
		c.XPMax = &xpMax
		c.Quests = []character.Quest{{QuestID: int(tc.quest.id), Title: text(row["title"]), Level: &lvl, Complete: &done}}
		in := session.In{Char: c, Info: session.Info{XP: func(int) (int, bool) { return 1500, true }}}
		grey := false
		for _, v := range in.Triage() {
			grey = v.Has(session.FlagGrey)
		}
		out = append(out, benchRow{fmt.Sprintf("%q (level %d) in the log of a level %d character: flagged grey", tc.quest.title, tc.quest.level, tc.player), fmt.Sprint(grey),
			fmt.Sprintf("dataset level %d; game grey %v", tc.quest.level, tc.grey), qdColorSrc, resultIf(grey == tc.grey, "grey flag unlike the game's colour")})
	}
	finish(t, "advice-questdiff-grey-flag", "Offline quest colour: the grey flag of the quest triage", out)
}

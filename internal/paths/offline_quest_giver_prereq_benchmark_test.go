package paths_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/advice"
	"github.com/WoW-Companion/advisor/internal/ask"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/questie"
	"github.com/WoW-Companion/advisor/internal/quests"
	"github.com/WoW-Companion/advisor/internal/store"
)

// The offline quest giver, turn-in, prerequisite, minimum level and breadcrumb benchmark: who starts a quest and where, who takes it in and where, what must be done before it and from which level it
// is offered, which quest follows it and what a breadcrumb quest does once the quest it leads to is taken, as the advisor answers beside the dataset's rows and the game's own rules, with no network. It
// reads testdata/places_snapshot (quests_observed, quest_lines, npcs, quests). The dataset names a giver NPC id on a few quests (npcs holds no name or zone for it), no turn-in NPC at all, no minimum level,
// no prerequisite and no breadcrumb row, and only three quest lines, so the giver, turn-in, prerequisite and breadcrumb rows of the cases are composed here from a real quest of quests_observed (its title,
// level, heading and giver id) and a synthetic QuestieDB, as the quest reward and restriction benchmarks do; the cases the dataset cannot answer check that the answer is framed with the line that says so
// (ask.LimitFacts, ask.NamedQuestFacts) and that the advisor states nothing from memory. The game's rules: a quest is offered by its giver at the level it requires (Warcraft Wiki, Quest); a prerequisite
// must be finished first (any one of several, or all of a group); the turn-in is the NPC that ends the quest, which need not be the one that gave it; a breadcrumb quest leads to another quest and is no
// longer offered once that quest is in the log or done (Warcraft Wiki, Breadcrumb quest). A difference the dataset's own rows cause is marked "(data)". make offline-benchmark runs it; WOWC_BENCH_REPORT
// receives the tables.

const (
	qgGiverSrc   = "dataset quests_observed (giver_npc_id, category heading), npcs (no name or zone for the giver); player's QuestieDB (starters, area); Warcraft Wiki, Quest (a quest starts with an NPC or object)"
	qgTurnInSrc  = "dataset quests_observed (turn_in_npc_id null on every row); player's QuestieDB (finishers, area); Warcraft Wiki, Quest (the NPC that ends a quest, often not the giver)"
	qgPrereqSrc  = "dataset quests (min_level, no value), quests_observed (level); player's QuestieDB (requiredLevel, preQuestSingle, preQuestGroup); Warcraft Wiki, Quest (required level and prerequisites)"
	qgFollowSrc  = "dataset quest_lines (line, position); player's QuestieDB (nextQuestInChain, preQuestSingle); Warcraft Wiki, Breadcrumb quest"
	qgNoDataSrc  = "the dataset has no such row or column (no turn-in NPC, NPC position, minimum level, prerequisite or breadcrumb row)"
	qgDataGap    = "differ (data): the dataset has none, so the advisor states none"
	qgSyntheticA = "Synthetic Vale"
)

// qgIndex is a synthetic QuestieDB: one area per zone name, the NPCs and the quests the case lists.
type qgIndex struct{ *questie.Index }

func newQG() qgIndex {
	return qgIndex{&questie.Index{Quests: map[int]*questie.Quest{}, Npcs: map[int]questie.Entity{}, Areas: map[int]questie.Area{}}}
}

// zone is the area id of a zone name, made once.
func (q qgIndex) zone(name string) int {
	for id, a := range q.Areas {
		if a.Zone == name {
			return id
		}
	}
	id := len(q.Areas) + 1
	q.Areas[id] = questie.Area{UIMap: id, Zone: name}
	return id
}

// npc adds an NPC standing in a zone and returns its id.
func (q qgIndex) npc(id int, name, zone, side string) questie.Ref {
	q.Npcs[id] = questie.Entity{Name: name, Faction: side, Spawns: []questie.Spawn{{Area: q.zone(zone), X: 40, Y: 40}}}
	return questie.Ref{ID: id}
}

// quest adds a quest of the dataset (its title and level are the dataset's) with the caller's changes.
func (q qgIndex) quest(t *testing.T, id int, mut func(*questie.Quest)) *questie.Quest {
	t.Helper()
	r := qbQuest(t, int64(id))
	x := &questie.Quest{ID: id, Name: text(r["title"]), Level: int(num(r["level"]))}
	if mut != nil {
		mut(x)
	}
	q.Quests[id] = x
	return x
}

// ---- giver and where it starts ----

// qgGivers are real quests of quests_observed with a giver id: the quest, and the zone heading players saw it under.
var qgGivers = []int{93, 516, 890, 407, 8366, 1218}

// TestOfflineBenchmarkQuestGiverZone: the giver of a quest and the zone it stands in, as the quest tool names them from the player's QuestieDB, with the quest's title and level from the dataset, beside the
// giver id and the zone heading the dataset holds. The dataset names no giver (npcs has no row for it), so the giver's name and zone are the QuestieDB's.
func TestOfflineBenchmarkQuestGiverZone(t *testing.T) {
	ds := qbStore()
	var out []benchRow
	for _, id := range qgGivers {
		r := qbQuest(t, int64(id))
		zone, giver := text(r["category"]), int(num(r["giver_npc_id"]))
		ix := newQG()
		g := ix.npc(giver, fmt.Sprintf("Synthetic giver %d", giver), zone, "AH")
		ix.quest(t, id, func(x *questie.Quest) { x.Starters = []questie.Ref{g} })
		f := quests.FactsOf(ds, ix.Index, int64(id))
		want := fmt.Sprintf("Synthetic giver %d (%s)", giver, zone)
		got := ""
		if f != nil {
			got = f.Giver
		}
		out = append(out, benchRow{fmt.Sprintf("%q (quest %d): who gives it, and where", text(r["title"]), id), got, fmt.Sprintf("dataset giver_npc_id %d, heading %q; QuestieDB: %s", giver, zone, want), qgGiverSrc,
			resultIf(got == want && f != nil && f.Name == text(r["title"]), "the giver or its zone")})
	}
	// a class quest is filed under its class, which is no place: the giver's zone is the QuestieDB's
	r := qbQuest(t, 1508)
	ix := newQG()
	g := ix.npc(int(num(r["giver_npc_id"])), "Synthetic warlock trainer", qgSyntheticA, "H")
	ix.quest(t, 1508, func(x *questie.Quest) { x.Starters = []questie.Ref{g} })
	f := quests.FactsOf(ds, ix.Index, 1508)
	out = append(out, benchRow{"\"Blind Cazul\" (quest 1508), a warlock class quest: where its giver stands", f.Giver, fmt.Sprintf("dataset heading %q is the class, not a zone; QuestieDB: Synthetic warlock trainer (%s)", text(r["category"]), qgSyntheticA), qgGiverSrc,
		resultIf(f.ClassHeading() == "Warlock" && f.Giver == "Synthetic warlock trainer ("+qgSyntheticA+")" && !strings.Contains(f.Giver, "Warlock"), "the class heading was taken for a zone")})
	// the dataset names no giver: the count of quests with a giver id whose NPC the npcs table names
	rows, _ := ds.Filter("quests_observed", store.Range{Column: "quest_id", Min: int64(1)})
	withID, named := 0, 0
	for _, row := range rows {
		if gid := num(row["giver_npc_id"]); gid > 0 {
			withID++
			if n, err := ds.Get("npcs", gid); err == nil && text(n["name"]) != "" {
				named++
			}
		}
	}
	out = append(out, benchRow{"the dataset's giver of a quest, by name", fmt.Sprintf("%d of %d quests with a giver id have an npcs name", named, withID), "game: every quest has a giver with a name and a zone", qgGiverSrc, qgDataGap})
	finish(t, "advice-quest-giver-zone", "Offline quest giver: the giver and its zone, from the QuestieDB and the dataset's quest row", out)
}

// ---- turn-in that differs from the giver ----

var qgTurnIns = []struct{ quest, finish int }{{383, 5}, {407, 6}, {529, 7}, {1111, 8}, {5762, 9}}

// TestOfflineBenchmarkQuestTurnInDiffers: a quest handed in to another NPC than the one that gave it, in another zone: the quest tool names both with their zones, and the place to go once the quest is complete
// is the turn-in's, not the giver's. The dataset holds no turn-in NPC, so the turn-in is the QuestieDB's.
func TestOfflineBenchmarkQuestTurnInDiffers(t *testing.T) {
	ds := qbStore()
	var out []benchRow
	for _, tc := range qgTurnIns {
		r := qbQuest(t, int64(tc.quest))
		zoneA, zoneB := text(r["category"]), "Far Reach "+fmt.Sprint(tc.finish)
		giver := int(num(r["giver_npc_id"]))
		ix := newQG()
		g := ix.npc(giver, fmt.Sprintf("Synthetic giver %d", giver), zoneA, "AH")
		f := ix.npc(900000+tc.finish, fmt.Sprintf("Synthetic finisher %d", tc.finish), zoneB, "AH")
		ix.quest(t, tc.quest, func(x *questie.Quest) { x.Starters, x.Finishers = []questie.Ref{g}, []questie.Ref{f} })
		facts := quests.FactsOf(ds, ix.Index, int64(tc.quest))
		txt := quests.Render(quests.OfWith(ds, ix.Index, int64(tc.quest)), "")
		wantStart := fmt.Sprintf("starts with Synthetic giver %d (%s)", giver, zoneA)
		wantTurn := fmt.Sprintf("turn in to Synthetic finisher %d (%s)", tc.finish, zoneB)
		p, _, ok := ix.Locate(tc.quest, true, questie.Where{Zone: zoneB})
		_, _, farOK := ix.Locate(tc.quest, true, questie.Where{Zone: zoneA})
		out = append(out, benchRow{fmt.Sprintf("%q (quest %d): the turn-in, which is not the giver", text(r["title"]), tc.quest), firstLine(txt) + fmt.Sprintf(" | complete, in %s: %s", zoneB, p.Name),
			fmt.Sprintf("QuestieDB finisher %d in %s, giver %d in %s; dataset turn_in_npc_id %v", 900000+tc.finish, zoneB, giver, zoneA, r["turn_in_npc_id"]), qgTurnInSrc,
			resultIf(facts != nil && facts.TurnIn != facts.Giver && strings.Contains(txt, wantStart) && strings.Contains(txt, wantTurn) && ok && p.Name == fmt.Sprintf("Synthetic finisher %d", tc.finish) && p.Zone == zoneB && !farOK,
				"the turn-in was the giver, in the giver's zone, or the place to go was not the turn-in's")})
	}
	rows, _ := ds.Filter("quests_observed", store.Range{Column: "quest_id", Min: int64(1)})
	withTurn := 0
	for _, row := range rows {
		if num(row["turn_in_npc_id"]) > 0 {
			withTurn++
		}
	}
	out = append(out, benchRow{"the dataset's turn-in NPC of a quest", fmt.Sprintf("%d of %d quests hold a turn-in NPC id", withTurn, len(rows)), "game: every quest has an NPC or object that takes it in", qgTurnInSrc, qgDataGap})
	finish(t, "advice-quest-turnin", "Offline quest turn-in: the NPC and zone that take it in, not the giver's", out)
}

// ---- prerequisite and minimum level ----

// qgOffer is whether the quest is offered to a character of the level who has finished the quests, in the zone the quest's giver stands in.
func qgOffer(ix qgIndex, level int, done []int, quest int) bool {
	c := usabChar("WARRIOR", level, "Horde")
	c.CompletedQuests = done
	for _, o := range ix.Available(advice.WhoOf(c), questie.Where{Zone: qgSyntheticA}, nil) {
		if o.Quest.ID == quest {
			return true
		}
	}
	return false
}

func qgPrereqIndex(t *testing.T) qgIndex {
	ix := newQG()
	g := ix.npc(1, "Synthetic giver", qgSyntheticA, "AH")
	// every quest is level 22 (the cases' level) unless its case says otherwise: the band of levels offered is not what is checked here
	st := func(x *questie.Quest) { x.Starters, x.Level = []questie.Ref{g}, 22 }
	ix.quest(t, 93, st)  // Dusky Crab Cakes: first of its errands
	ix.quest(t, 149, st) // Ghost Hair Thread
	ix.quest(t, 124, func(x *questie.Quest) { st(x); x.MinLevel = 22 })
	ix.quest(t, 246, func(x *questie.Quest) { st(x); x.PreSingle = []int{93} })
	ix.quest(t, 131, func(x *questie.Quest) { st(x); x.PreSingle = []int{93, 149} })
	ix.quest(t, 19, func(x *questie.Quest) { st(x); x.PreGroup = []int{93, 149} })
	ix.quest(t, 198, func(x *questie.Quest) { st(x); x.Level = 32; x.MinLevel = 30 })
	return ix
}

// TestOfflineBenchmarkQuestPrerequisiteLevel: a quest below its minimum level or before its prerequisite is not offered; at the minimum level or after the prerequisite it is; the quest tool states the
// minimum level and what comes first. The dataset holds no minimum level and no prerequisite (quests has no value in min_level), so both are the QuestieDB's.
func TestOfflineBenchmarkQuestPrerequisiteLevel(t *testing.T) {
	ds := qbStore()
	ix := qgPrereqIndex(t)
	var out []benchRow
	for _, tc := range []struct {
		label string
		level int
		done  []int
		quest int
		want  bool
		ref   string
	}{
		{"\"A Baying of Gnolls\" (needs level 22), a level 21 character", 21, nil, 124, false, "requiredLevel 22: not offered at level 21"},
		{"the same quest, a level 22 character", 22, nil, 124, true, "requiredLevel 22: offered at level 22"},
		{"\"Supplies to Private Thorsen\" (level 32, needs level 30), a level 30 character", 30, nil, 198, true, "requiredLevel 30: offered at level 30"},
		{"\"Assessing the Threat\" (after Dusky Crab Cakes), nothing finished", 20, nil, 246, false, "preQuestSingle 93: finish it first"},
		{"the same quest, Dusky Crab Cakes finished", 20, []int{93}, 246, true, "preQuestSingle 93 done: offered"},
		{"\"Delivering Daffodils\" (after either of two), only the second finished", 22, []int{149}, 131, true, "preQuestSingle 93 or 149: either one opens it"},
		{"\"Tharil'zun\" (after both of two), only one finished", 22, []int{93}, 19, false, "preQuestGroup 93 and 149: both are needed"},
		{"the same quest, both finished", 22, []int{93, 149}, 19, true, "preQuestGroup 93 and 149 done: offered"},
	} {
		got := qgOffer(ix, tc.level, tc.done, tc.quest)
		out = append(out, benchRow{tc.label + ": offered", fmt.Sprint(got), fmt.Sprintf("the game: offered %v (%s)", tc.want, tc.ref), qgPrereqSrc, resultIf(got == tc.want, fmt.Sprintf("offered %v, the game says %v", got, tc.want))})
	}
	for _, tc := range []struct {
		quest int64
		want  []string
		ref   string
	}{
		{124, []string{"needs level 22"}, "needs level 22"},
		{246, []string{"Comes after:", "Dusky Crab Cakes"}, "after Dusky Crab Cakes"},
		{131, []string{"Comes after (finish any one of these first):", "Dusky Crab Cakes", "Ghost Hair Thread"}, "after either Dusky Crab Cakes or Ghost Hair Thread"},
		{19, []string{"Comes after (finish all of these first):", "Dusky Crab Cakes", "Ghost Hair Thread"}, "after both Dusky Crab Cakes and Ghost Hair Thread"},
	} {
		txt := quests.Render(quests.OfWith(ds, ix.Index, tc.quest), "")
		ok := true
		for _, w := range tc.want {
			ok = ok && strings.Contains(txt, w)
		}
		out = append(out, benchRow{fmt.Sprintf("quest %d: what the answer says first", tc.quest), strings.ReplaceAll(txt, "\n", " | "), tc.ref, qgPrereqSrc, resultIf(ok, "the level or the prerequisite is missing from the answer")})
	}
	rows, _ := ds.Filter("quests", store.Range{Column: "id", Min: int64(1)})
	withMin := 0
	for _, row := range rows {
		if num(row["min_level"]) > 0 {
			withMin++
		}
	}
	out = append(out, benchRow{"the dataset's minimum level of a quest", fmt.Sprintf("%d of %d quests hold a min_level", withMin, len(rows)), "game: a quest has the level it is offered from", qgPrereqSrc, qgDataGap})
	finish(t, "advice-quest-prereq-level", "Offline quest prerequisite and minimum level: not offered early, stated when asked", out)
}

// TestOfflineBenchmarkQuestInLineKeepsQuestie: a quest that sits in one of the dataset's quest lines still gets the giver, the turn-in and the level it needs from the player's QuestieDB (the line's order wins over the
// QuestieDB's prerequisites); before the fix the answer for such a quest was the line alone, and the giver, turn-in and minimum level the player's own Questie held were dropped.
func TestOfflineBenchmarkQuestInLineKeepsQuestie(t *testing.T) {
	ds := qbStore()
	var out []benchRow
	r := qbQuest(t, 92742)
	ix := newQG()
	g := ix.npc(int(num(r["giver_npc_id"])), "Synthetic giver", text(r["category"]), "AH")
	f := ix.npc(900101, "Synthetic finisher", "Duskwood", "AH")
	ix.quest(t, 92742, func(x *questie.Quest) { x.Starters, x.Finishers = []questie.Ref{g}, []questie.Ref{f} })
	txt := quests.Render(quests.OfWith(ds, ix.Index, 92742), "")
	out = append(out, benchRow{"\"Testing the Wells\" (quest 92742), first of the line Toxic Soil: who gives it and who takes it in", firstLine(strings.SplitN(txt, "From your Questie database", 2)[len(strings.SplitN(txt, "From your Questie database", 2))-1]),
		fmt.Sprintf("dataset giver_npc_id %v, heading %q; QuestieDB: starts with Synthetic giver (%s), turn in to Synthetic finisher (Duskwood)", r["giver_npc_id"], text(r["category"]), text(r["category"])), qgGiverSrc,
		resultIf(strings.Contains(txt, "starts with Synthetic giver ("+text(r["category"])+")") && strings.Contains(txt, "turn in to Synthetic finisher (Duskwood)") && strings.Contains(txt, "step 1 of 11 in Toxic Soil"), "the giver or the turn-in of a quest in a line")})
	ix = newQG()
	g = ix.npc(1, "Synthetic giver", qgSyntheticA, "AH")
	ix.quest(t, 92744, func(x *questie.Quest) { x.Starters = []questie.Ref{g} })
	ix.quest(t, 92745, func(x *questie.Quest) { x.Starters, x.MinLevel, x.PreSingle = []questie.Ref{g}, 12, []int{92744} })
	txt = quests.Render(quests.OfWith(ds, ix.Index, 92745), "")
	out = append(out, benchRow{"\"The State of the Mines\" (quest 92745), third of the line: the level it needs", firstLine(txt) + " | " + firstLine(strings.SplitN(txt, "From your Questie database", 2)[len(strings.SplitN(txt, "From your Questie database", 2))-1]), "QuestieDB requiredLevel 12", qgPrereqSrc,
		resultIf(strings.Contains(txt, "needs level 12") && strings.Contains(txt, "step 3 of 11"), "the minimum level of a quest in a line")})
	out = append(out, benchRow{"the same quest: what must be finished first", strings.ReplaceAll(txt, "\n", " | "), "QuestieDB preQuestSingle 92744 \"Murloc Gills\" is the line's step 2: the line's order is kept and lists it before step 3", qgPrereqSrc,
		resultIf(strings.Contains(txt, "  2. Murloc Gills (quest 92744)") && strings.Contains(txt, "> 3. The State of the Mines"), "the line's order was lost")})
	txt = quests.Render(quests.OfWith(ds, nil, 92745), "")
	out = append(out, benchRow{"the same quest, no QuestieDB: no giver, level or prerequisite is made up", firstLine(txt), "the line alone", qgPrereqSrc,
		resultIf(!strings.Contains(txt, "Questie") && !strings.Contains(txt, "needs level") && !strings.Contains(txt, "Comes after"), "a fact came from nowhere")})
	finish(t, "advice-quest-in-line", "Offline quest in a quest line: the giver, turn-in and level of the player's QuestieDB are kept", out)
}

// ---- breadcrumb and follow-up ----

// TestOfflineBenchmarkQuestBreadcrumbFollowUp: a quest names what follows it (the quest line of the dataset, the QuestieDB's follow-ups), and a breadcrumb quest, which leads to another quest, is no longer
// offered once the quest it leads to is in the log or done. The dataset holds no breadcrumb row.
func TestOfflineBenchmarkQuestBreadcrumbFollowUp(t *testing.T) {
	ds := qbStore()
	var out []benchRow
	l := refLine(t, ds, 6026)
	for _, pos := range []int{0, 5} {
		id := l.ids[pos]
		c := quests.Of(ds, id)
		txt := quests.Render(c, quests.Name(ds, id))
		want := fmt.Sprintf("next is quest %d", l.ids[pos+1])
		out = append(out, benchRow{fmt.Sprintf("quest %d, step %d of the line %q: what follows", id, pos+1, l.name), firstLine(txt), fmt.Sprintf("dataset quest_lines: step %d is quest %d", pos+2, l.ids[pos+1]), qgFollowSrc,
			resultIf(strings.Contains(txt, want) && c.NextQuestID == l.ids[pos+1], "the next quest")})
	}
	ix := newQG()
	g := ix.npc(1, "Synthetic giver", qgSyntheticA, "AH")
	st := func(x *questie.Quest) { x.Starters = []questie.Ref{g} }
	ix.quest(t, 516, func(x *questie.Quest) { st(x); x.Next = 544 }) // Beren's Peril: a breadcrumb that leads to Prison Break In
	ix.quest(t, 544, func(x *questie.Quest) { st(x); x.Level = 21 })
	ix.quest(t, 499, func(x *questie.Quest) { st(x); x.PreSingle = []int{496} })
	ix.quest(t, 496, st)
	txt := quests.Render(quests.OfWith(ds, ix.Index, 496), "")
	out = append(out, benchRow{"quest 496 \"Elixir of Suffering\": what it leads to", strings.ReplaceAll(txt, "\n", " | "), "QuestieDB: 499 has 496 as its prerequisite, so 496 leads to it", qgFollowSrc,
		resultIf(strings.Contains(txt, "Leads to:") && strings.Contains(txt, "Elixir of Suffering (quest 499)"), "the follow-up quest")})
	for _, tc := range []struct {
		label string
		log   []int
		done  []int
		want  bool
		ref   string
	}{
		{"the breadcrumb \"Beren's Peril\", the quest it leads to not taken", nil, nil, true, "a breadcrumb is offered while its quest is not taken"},
		{"the breadcrumb, the quest it leads to in the log", []int{544}, nil, false, "a breadcrumb is no longer offered once its quest is in the log"},
		{"the breadcrumb, the quest it leads to done", nil, []int{544}, false, "a breadcrumb is no longer offered once its quest is done"},
	} {
		c := usabChar("WARRIOR", 21, "Horde")
		c.CompletedQuests = tc.done
		for _, id := range tc.log {
			c.Quests = append(c.Quests, character.Quest{QuestID: id})
		}
		got := false
		for _, o := range ix.Available(advice.WhoOf(c), questie.Where{Zone: qgSyntheticA}, nil) {
			got = got || o.Quest.ID == 516
		}
		out = append(out, benchRow{tc.label + ": offered", fmt.Sprint(got), "the game: offered " + fmt.Sprint(tc.want) + " (" + tc.ref + ")", qgFollowSrc, resultIf(got == tc.want, fmt.Sprintf("offered %v, the game says %v", got, tc.want))})
	}
	out = append(out, benchRow{"which quests of the dataset are breadcrumbs", "none named", "game: a breadcrumb quest points the player to a dungeon or zone quest", qgFollowSrc, qgDataGap})
	finish(t, "advice-quest-breadcrumb", "Offline quest follow-up and breadcrumb: what comes next, a breadcrumb that is not offered twice", out)
}

// ---- what the dataset cannot answer ----

var qgNoDataCases = []struct{ q, head string }{
	{"who gives the quest Tharil'zun?", "Quest givers and where a quest starts:"},
	{"where do I pick up the quest Beren's Peril?", "Quest givers and where a quest starts:"},
	{"where does the quest The Essence of Aku'Mai start?", "Quest givers and where a quest starts:"},
	{"where is the quest giver for the Deathknell quests?", "Quest givers and where a quest starts:"},
	{"where do I turn in Vital Intelligence?", "Quest turn-in:"},
	{"who do I hand in Fields of Grief to?", "Quest turn-in:"},
	{"which NPC takes the quest Deliveries to Sven?", "Quest turn-in:"},
	{"what is the prerequisite quest for Elixir of Suffering?", "Quest prerequisites, follow-ups and breadcrumbs:"},
	{"what do I need to finish before Arugal Must Die?", "Quest prerequisites, follow-ups and breadcrumbs:"},
	{"is there a breadcrumb quest to Razorfen Kraul?", "Quest prerequisites, follow-ups and breadcrumbs:"},
	{"what is the follow-up to Fields of Grief?", "Quest prerequisites, follow-ups and breadcrumbs:"},
	{"what quest leads to Battle of Hillsbrad?", "Quest prerequisites, follow-ups and breadcrumbs:"},
	{"which quest sends me to Wailing Caverns?", "Quest prerequisites, follow-ups and breadcrumbs:"},
	{"what level do I need to start Tharil'zun?", "Quest minimum level:"},
	{"what is the minimum level for Dusky Crab Cakes?", "Quest minimum level:"},
	{"what is the lowest level to accept the quest Apprentice's Duties?", "Quest minimum level:"},
}

// qgFacts is the lines the advisor puts before the model for the question: the limit lines of LimitFacts and of a quest named by its title.
func qgFacts(ds store.DataStore, q string) string {
	return strings.TrimSpace(ask.LimitFacts(q) + "\n" + ask.NamedQuestFacts(ds, q))
}

// qgStated is a giver, a place or a level in a no-data answer's words that the data does not hold.
func qgNames(s string) string {
	for _, w := range []string{"Undertaker", "Brill", "Deathknell", "Lakeshire", "Darkshire", "Hillsbrad", "Orgrimmar", "Stormwind", "Sven", "Mordo"} {
		if strings.Contains(s, w) {
			return w
		}
	}
	return ""
}

// TestOfflineBenchmarkQuestNoDataQuestions: a question for a quest's giver and place, its turn-in, what comes before or after it, a breadcrumb or the level it needs, asked in the player's words, gets the line
// that the dataset holds none, and no giver, place or level is named from memory. Before the fix none of the giver, turn-in, prerequisite and breadcrumb questions got such a line and the title-only
// minimum level questions got none.
func TestOfflineBenchmarkQuestNoDataQuestions(t *testing.T) {
	ds := qbStore()
	var out []benchRow
	for _, tc := range qgNoDataCases {
		got := qgFacts(ds, tc.q)
		line := firstLineMatching(got, tc.head)
		ok := strings.Contains(got, tc.head) && strings.Contains(line, "Say so") && strings.Contains(line, "from memory") && qgNames(got) == ""
		out = append(out, benchRow{fmt.Sprintf("%q", tc.q), firstLine(line), "the data holds no such row: say so, name none from memory", qgNoDataSrc, resultIf(ok, "no line that the data holds none, or a name or place was stated")})
	}
	finish(t, "advice-quest-nodata", "Offline quest giver, turn-in, prerequisite and level: asked in the player's words, no answer from memory", out)
}

// TestOfflineBenchmarkQuestNoDataScope: a question that asks of no quest's giver, turn-in or prerequisite gets none of these lines, and a quest the dataset does not hold has no name, giver or line to state.
func TestOfflineBenchmarkQuestNoDataScope(t *testing.T) {
	ds := qbStore()
	var out []benchRow
	for _, q := range []string{"what quests are in the Barrens?", "what quest should I do next?", "which dungeon quests can I turn in?", "what level is the quest Tharil'zun?", "what comes after Murloc Gills in the Toxic Soil quest line?", "where do I buy Silk Cloth?"} {
		got := qgFacts(ds, q)
		bad := ""
		for _, h := range []string{"Quest givers and where a quest starts:", "Quest turn-in:", "Quest prerequisites, follow-ups and breadcrumbs:", "Quest minimum level:"} {
			if strings.Contains(got, h) {
				bad = h
			}
		}
		out = append(out, benchRow{fmt.Sprintf("%q", q), firstLine(got), "no quest giver, turn-in, prerequisite or level line: the question asks of none", qgNoDataSrc, resultIf(bad == "", "got the line "+bad)})
	}
	for _, id := range []int64{99999911, 99999912} {
		c := quests.OfWith(ds, nil, id)
		txt := quests.Render(c, "")
		out = append(out, benchRow{fmt.Sprintf("quest %d, a quest neither the dataset nor a QuestieDB holds: the answer", id), firstLine(txt), "no quest line, giver, turn-in or prerequisite to state", qgNoDataSrc,
			resultIf(!c.Found && strings.Contains(txt, "No quest line or follow-up quest is listed") && qgNames(txt) == "", "a quest or a giver was made up")})
	}
	// a quest players saw, with a giver id the npcs table does not name: the answer does not name a giver or a turn-in
	for _, id := range []int64{93, 383} {
		txt := quests.Render(quests.OfWith(ds, nil, id), "")
		out = append(out, benchRow{fmt.Sprintf("quest %d, seen by players with a giver id and no NPC name: who gives it and who takes it in", id), strings.ReplaceAll(txt, "\n", " | "), "the data names neither: the answer says so", qgNoDataSrc,
			resultIf(strings.Contains(txt, "doesn't name who gives it or who takes it in") && !strings.Contains(txt, "starts with") && !strings.Contains(txt, "turn in to"), "a giver or a turn-in was named")})
	}
	finish(t, "advice-quest-nodata-scope", "Offline quest giver, turn-in and prerequisite: other questions get no such line, an unknown quest nothing", out)
}

package paths_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/advice"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/quests"
	"github.com/WoW-Companion/advisor/internal/store"
	"github.com/WoW-Companion/advisor/internal/unlock"
)

// The offline quest-order benchmark: what must be done before a quest in its line (the quest before it), the quest it
// unlocks, and what the advisor says when the dataset names no line or no requirement, beside the dataset's quest_lines rows, with no
// network. It reads testdata/places_snapshot (quest_lines, quests, quests_observed). The dataset holds three quest
// lines and no prerequisite, required-reputation, required-class or breadcrumb column, so a quest outside the lines has no
// answer and the cases check that the advisor says so and names none. make offline-benchmark runs it;
// WOWC_BENCH_REPORT receives the tables.

const (
	coLineSrc = "dataset quest_lines (line_id, name, position, quest_id), quests_observed (title)"
	coNoneSrc = "the dataset has no such column, row or table"
)

// reversed serves the rows of quest_lines last to first, so a line is read out of the order of its positions.
type reversed struct{ store.DataStore }

func (r reversed) Filter(table string, ranges ...store.Range) ([]store.Row, error) {
	rows, err := r.DataStore.Filter(table, ranges...)
	if table == "quest_lines" {
		slices.Reverse(rows)
	}
	return rows, err
}

// coPrev is the quest before the given one in its line by the dataset's positions, 0 for the first step; and the one after, 0 for the last.
func coPrev(t *testing.T, ds store.DataStore, line, quest int64) (prev, next int64) {
	t.Helper()
	l := refLine(t, ds, line)
	i := slices.Index(l.ids, quest)
	if i < 0 {
		t.Fatalf("quest %d is not in line %d", quest, line)
	}
	if i > 0 {
		prev = l.ids[i-1]
	}
	if i+1 < len(l.ids) {
		next = l.ids[i+1]
	}
	return prev, next
}

// coAnswerPrev is the step the answer lists just above the asked-for one ("> n. title"), 0 when it is the first.
func coAnswerPrev(c quests.Chain) int64 {
	if c.Position <= 1 || c.Position > len(c.Steps) {
		return 0
	}
	return c.Steps[c.Position-2].QuestID
}

var coOrder = []struct{ line, quest int64 }{
	{6026, 92745}, {6026, 92748}, {6026, 92819}, {6293, 96913}, {6293, 96917}, {6293, 96921}, {6026, 92742}, {6293, 96912}, {5963, 91564},
}

// TestOfflineBenchmarkQuestOrderPrerequisite: the quest to finish first is the one just above it in the line, whatever order the
// rows of the table come in, and the first step of a line has none.
func TestOfflineBenchmarkQuestOrderPrerequisite(t *testing.T) {
	ds := qbStore()
	var out []benchRow
	for _, tc := range coOrder {
		prev, next := coPrev(t, ds, tc.line, tc.quest)
		c := quests.Of(reversed{ds}, tc.quest)
		got := coAnswerPrev(c)
		ref := "no quest before it (step 1)"
		if prev != 0 {
			ref = fmt.Sprintf("quest %d first", prev)
		}
		gotText := "no quest before it"
		if got != 0 {
			gotText = fmt.Sprintf("quest %d first", got)
		}
		out = append(out, benchRow{fmt.Sprintf("line %d, quest %d: what comes first, rows read last to first", tc.line, tc.quest), gotText + fmt.Sprintf("; next %d", c.NextQuestID), ref + fmt.Sprintf("; next %d", next), coLineSrc,
			resultIf(c.Found && got == prev && c.NextQuestID == next, "the quest before or after it, or the order of the rows")})
	}
	finish(t, "advice-quest-order-prerequisite", "Offline quest order: the quest to finish first and the next one", out)
}

// TestOfflineBenchmarkQuestOrderNames: a quest asked for by its title is placed in its line, and the step before and after it are named
// by the titles players reported, never a made-up one.
func TestOfflineBenchmarkQuestOrderNames(t *testing.T) {
	ds := qbStore()
	var out []benchRow
	for _, tc := range []struct {
		title      string
		id         int64
		prev, next string
	}{
		{"Murloc Gills", 92744, "Testing the Wells", "The State of the Mines"},
		{"The State of the Mines", 92745, "Murloc Gills", "quest 92747"},
		{"Testing the Wells", 92742, "", "Murloc Gills"},
	} {
		id, name, cands := quests.Resolve(ds, tc.title)
		c := quests.Of(ds, id)
		txt := quests.Render(c, name)
		okPrev, okNext := true, strings.Contains(txt, tc.next)
		if tc.prev != "" {
			okPrev = strings.Contains(txt, tc.prev+" (quest ")
		}
		out = append(out, benchRow{"\"" + tc.title + "\": the quest before and after it", fmt.Sprintf("id %d, before %q, after %q", id, tc.prev, tc.next), "quests_observed title; quest_lines positions " + fmt.Sprint(c.Position-1, " and ", c.Position+1), coLineSrc,
			resultIf(id == tc.id && len(cands) == 0 && okPrev && okNext, "another quest, or the neighbours not named")})
	}
	finish(t, "advice-quest-order-names", "Offline quest order: a quest asked for by its title", out)
}

// TestOfflineBenchmarkQuestOrderUnlocks: handing in a quest of a line unlocks the next one, and the last one unlocks nothing; the
// plan's link and the what's-next step both say so.
func TestOfflineBenchmarkQuestOrderUnlocks(t *testing.T) {
	ds := qbStore()
	var out []benchRow
	lines := unlock.FromDataset(ds)
	for _, tc := range []struct{ line, quest int64 }{{6026, 92748}, {6026, 92819}, {6293, 96916}, {5963, 91564}} {
		_, next := coPrev(t, ds, tc.line, tc.quest)
		got, line := lines(int(tc.quest))
		out = append(out, benchRow{fmt.Sprintf("line %d, quest %d: the quest handing it in unlocks", tc.line, tc.quest), fmt.Sprintf("%d in %q", got, line), fmt.Sprintf("%d in %q", next, refLine(t, ds, tc.line).name), coLineSrc,
			resultIf(int64(got) == next && line == refLine(t, ds, tc.line).name, "another quest unlocked")})
	}
	yes := true
	for _, tc := range []struct {
		line, quest int64
		reason      string
	}{{6026, 92744, "step 2 of 11 in Toxic Soil, unlocks step 3"}, {6026, 92819, "step 11 of 11 in Toxic Soil"}, {6293, 96916, "step 5 of 10 in PvP Season Journey, unlocks step 6"}} {
		lv := 60
		c := character.Character{Name: "Benchtest", Faction: "Alliance", Class: "MAGE", Level: &lv,
			Quests: []character.Quest{{QuestID: int(tc.quest), Title: fmt.Sprintf("Quest %d", tc.quest), Complete: &yes}}}
		var reason string
		for _, s := range advice.WhatsNext(c, ds) {
			if s.QuestID == int(tc.quest) {
				reason = s.Reason
			}
		}
		last := strings.HasPrefix(tc.reason, "step 11 of 11") // the last step says it is the last, and unlocks nothing
		ok := strings.Contains(reason, tc.reason) && (last == !strings.Contains(reason, "unlocks"))
		out = append(out, benchRow{fmt.Sprintf("quest %d turned in: the what's-next line", tc.quest), reason, tc.reason, coLineSrc, resultIf(ok, "the step, the length or the unlocked step")})
	}
	finish(t, "advice-quest-order-unlocks", "Offline quest order: what handing a quest in unlocks", out)
}

// TestOfflineBenchmarkQuestOrderNoAnswer: the dataset names a requirement for no quest, so a quest outside its three lines gets no
// before or after, a title it does not hold is not guessed, and no reputation, class or breadcrumb requirement is stated.
func TestOfflineBenchmarkQuestOrderNoAnswer(t *testing.T) {
	ds := qbStore()
	var out []benchRow
	for _, id := range []int64{1, 2, 5} {
		c := quests.Of(ds, id)
		txt := quests.Render(c, "")
		out = append(out, benchRow{fmt.Sprintf("quest %d: a line or a follow-up", id), strings.TrimSpace(txt), "quest_lines has no row for it", coNoneSrc,
			resultIf(!c.Found && c.NextQuestID == 0 && c.Position == 0 && strings.Contains(txt, "No quest line or follow-up quest is listed"), "a line was made up")})
	}
	for _, title := range []string{"Nonexistent Quest", "The Final Quest of Everything"} {
		id, name, cands := quests.Resolve(ds, title)
		out = append(out, benchRow{"\"" + title + "\": which quest it is", fmt.Sprintf("id %d, name %q, %d candidates", id, name, len(cands)), "no quest with that title in quests_observed", coNoneSrc,
			resultIf(id == 0 && name == "" && len(cands) == 0, "a quest was guessed")})
	}
	rows, err := ds.Filter("quests", store.Range{Column: "id", Min: int64(92742), Max: int64(92742)})
	var cols []string
	if err == nil && len(rows) == 1 {
		for col := range rows[0] {
			l := strings.ToLower(col)
			if strings.Contains(l, "prereq") || strings.Contains(l, "requires") || strings.Contains(l, "prev") || strings.Contains(l, "breadcrumb") || strings.Contains(l, "reputation") || strings.Contains(l, "class") {
				cols = append(cols, col)
			}
		}
	}
	out = append(out, benchRow{"a quest's prerequisite, reputation or class requirement", fmt.Sprintf("columns held: %v", cols), "quests holds an id, line, level, zone and map only", coNoneSrc,
		resultIf(err == nil && len(rows) == 1 && len(cols) == 0, "the dataset holds a requirement the answer could state")})
	for _, table := range []string{"quest_prerequisites", "quest_requirements"} {
		_, err := ds.Filter(table, store.Range{Column: "quest_id", Min: int64(0), Max: int64(1)})
		out = append(out, benchRow{"a table of " + strings.ReplaceAll(table, "_", " "), fmt.Sprint(err != nil), "no such table in the dataset", coNoneSrc, resultIf(err != nil, "a table was invented")})
	}
	finish(t, "advice-quest-order-no-answer", "Offline quest order: no answer in the dataset", out)
}

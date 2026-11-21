package quests

import (
	"bytes"
	"strings"
	"testing"

	"github.com/parquet-go/parquet-go"

	"github.com/WoW-Companion/advisor/internal/store"
)

type questRow struct {
	ID   int64  `parquet:"id"`
	Name string `parquet:"name"`
}

type lineRow struct {
	LineID   int64  `parquet:"line_id"`
	Name     string `parquet:"name"`
	Position int32  `parquet:"position"`
	QuestID  int64  `parquet:"quest_id"`
}

func table[T any](t *testing.T, rows []T) store.SizedReaderAt {
	t.Helper()
	var buf bytes.Buffer
	if err := parquet.Write(&buf, rows); err != nil {
		t.Fatal(err)
	}
	return store.SizedReaderAt{ReaderAt: bytes.NewReader(buf.Bytes()), Size: int64(buf.Len())}
}

// Line 7 "Wolf Hunt" is stored out of order (quests 12, 10, 11); quest 11 is also in line 9.
func synth(t *testing.T, withLines bool) store.DataStore {
	src := store.ReaderSource{"quests": table(t, []questRow{{10, "Wolf Pelts"}, {11, "Wolf Den"}, {12, "Wolf King"}, {13, "Wolf Pups"}, {20, "Lone Task"}})}
	if withLines {
		src["quest_lines"] = table(t, []lineRow{{7, "Wolf Hunt", 3, 12}, {7, "Wolf Hunt", 1, 10}, {7, "Wolf Hunt", 2, 11}, {9, "Other", 1, 11}, {9, "Other", 2, 13}})
	}
	return store.New(src)
}

func TestOf(t *testing.T) {
	for _, tc := range []struct {
		name            string
		lines           bool
		quest           int64
		found           bool
		pos, total, nxt int
	}{
		{"first step", true, 10, true, 1, 3, 11},
		{"middle, lowest line wins", true, 11, true, 2, 3, 12},
		{"last step", true, 12, true, 3, 3, 0},
		{"in no line", true, 20, false, 0, 0, 0},
		{"no quest_lines table", false, 10, false, 0, 0, 0},
		{"bad id", true, 0, false, 0, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := Of(synth(t, tc.lines), tc.quest)
			if c.Found != tc.found || c.Position != tc.pos || c.Total() != tc.total || int(c.NextQuestID) != tc.nxt || c.Unlocks() != (tc.nxt != 0) {
				t.Errorf("chain = %+v", c)
			}
			if tc.found && (c.Line != "Wolf Hunt" || c.Steps[0].Name != "Wolf Pelts") {
				t.Errorf("chain = %+v", c)
			}
		})
	}
}

func TestResolve(t *testing.T) {
	ds := synth(t, true)
	for _, tc := range []struct {
		in    string
		id    int64
		cands int
	}{
		{"10", 10, 0}, {"wolf den", 11, 0}, {"Wolf Pu", 13, 0}, {"Wolf", 0, 4}, {"zzz", 0, 0},
	} {
		if id, _, c := Resolve(ds, tc.in); id != tc.id || len(c) != tc.cands {
			t.Errorf("Resolve(%q) = %d, %v", tc.in, id, c)
		}
	}
}

func TestRender(t *testing.T) {
	text := Render(Of(synth(t, true), 11), "Wolf Den")
	for _, want := range []string{"Wolf Den (quest 11) is step 2 of 3 in Wolf Hunt (line 7); next is quest 12", "> 2. Wolf Den (quest 11)", "  3. Wolf King (quest 12)"} {
		if !strings.Contains(text, want) {
			t.Errorf("text lacks %q:\n%s", want, text)
		}
	}
	if got := Render(Of(synth(t, false), 10), ""); !strings.Contains(got, "No quest line in the dataset holds quest 10") {
		t.Errorf("no lines = %q", got)
	}
}

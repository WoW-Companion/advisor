package advice

import (
	"bytes"
	"strings"
	"testing"

	"github.com/parquet-go/parquet-go"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/store"
)

type questRow struct {
	ID    int64   `parquet:"id"`
	Level *int32  `parquet:"level,optional"`
	Zone  *string `parquet:"zone,optional"`
}

func i32(n int32) *int32   { return &n }
func str(s string) *string { return &s }
func ip(n int) *int        { return &n }
func bp(b bool) *bool      { return &b }

// synthStore is a tiny quests table: ids 100.. with levels and zones, and 199 with neither.
func synthStore(t *testing.T) store.DataStore {
	t.Helper()
	rows := []questRow{
		{100, i32(24), str("Dunemark")},
		{101, i32(26), str("Dunemark")},
		{102, i32(12), str("Gladehome")},
		{103, i32(30), str("Gladehome")},
		{104, i32(5), nil},
		{199, nil, nil},
	}
	var buf bytes.Buffer
	if err := parquet.Write(&buf, rows); err != nil {
		t.Fatal(err)
	}
	return store.New(store.ReaderSource{"quests": {ReaderAt: bytes.NewReader(buf.Bytes()), Size: int64(buf.Len())}})
}

func titles(steps []Step) string {
	var out []string
	for _, s := range steps {
		out = append(out, s.Kind+":"+s.Title)
	}
	return strings.Join(out, " | ")
}

func TestWhatsNext(t *testing.T) {
	trees := &character.Talents{Unspent: ip(2), Trees: []character.TalentTree{
		{Name: "Discipline", Points: ip(9)}, {Name: "Holy", Points: ip(3)},
	}}
	cases := []struct {
		name string
		c    character.Character
		ds   bool
		want string
	}{
		{"priest 24: turn-ins by zone, talents, near quests, failed last",
			character.Character{Name: "Testpriest", Level: ip(24), Talents: trees, Quests: []character.Quest{
				{QuestID: 1, Title: "B turn", Zone: "Gladehome", Complete: bp(true)},
				{QuestID: 2, Title: "A turn", Zone: "Dunemark", Complete: bp(true)},
				{QuestID: 3, Title: "C turn", Zone: "Dunemark", Complete: bp(true)},
				{QuestID: 4, Title: "No zone turn", Complete: bp(true)},
				{QuestID: 100, Title: "Level from dataset"},
				{QuestID: 101, Title: "Two above"},
				{QuestID: 103, Title: "Six above"},
				{QuestID: 104, Title: "Grey"},
				{QuestID: 5, Title: "Failed one", Level: ip(24), Failed: bp(true)},
				{QuestID: 6, Title: "Level 21", Level: ip(21)},
				{QuestID: 199, Title: "Unknown level"},
			}}, true,
			"talent:Spend 2 talent points | turn_in:Turn in A turn | turn_in:Turn in C turn | turn_in:Turn in B turn | turn_in:Turn in No zone turn | " +
				"quest:Level from dataset | quest:Two above | quest:Level 21 | " +
				"quest:Unknown level | quest:Abandon Failed one"},
		{"rogue 12: grey and too-high skipped, no dataset means unknown levels",
			character.Character{Name: "Testrogue", Level: ip(12), Quests: []character.Quest{
				{QuestID: 102, Title: "From dataset"},
				{QuestID: 7, Title: "Capture level 17", Level: ip(17)},
				{QuestID: 8, Title: "Capture level 15", Level: ip(15)},
				{QuestID: 9, Title: "Capture level 6", Level: ip(6)},
				{QuestID: 10, Title: "Capture level 7", Level: ip(7)},
			}}, false,
			"quest:Capture level 15 | quest:Capture level 7 | quest:From dataset"},
		{"dataset gives level 12", character.Character{Name: "Testrogue", Level: ip(12), Quests: []character.Quest{{QuestID: 102, Title: "From dataset"}}}, true,
			"quest:From dataset"},
		{"nothing captured", character.Character{Name: "Empty"}, true, ""},
		{"no level: nothing filtered", character.Character{Name: "NoLevel", Quests: []character.Quest{{QuestID: 11, Title: "Q", Level: ip(40)}}}, false, "quest:Q"},
	}
	for _, c := range cases {
		var ds store.DataStore
		if c.ds {
			ds = synthStore(t)
		}
		if got := titles(WhatsNext(c.c, ds)); got != c.want {
			t.Errorf("%s:\n got  %s\n want %s", c.name, got, c.want)
		}
	}
}

func TestStepFacts(t *testing.T) {
	c := character.Character{Name: "Testpriest", Level: ip(24), Talents: &character.Talents{Unspent: ip(1), Trees: []character.TalentTree{
		{Name: "Discipline", Points: ip(9)}, {Name: "Holy", Points: ip(9)},
	}}, Quests: []character.Quest{{QuestID: 100, Title: "Q"}, {QuestID: 199, Title: "U"}}}
	steps := WhatsNext(c, synthStore(t))
	want := []struct{ reason, zone, source string }{
		{`milestone; ask "What talents should I get?"; Discipline, Holy are tied at 9 points`, "", "capture: talents.unspent"},
		{"level 24 quest, you are 24", "Dunemark", "dataset: quests id 100"},
		{"quest level unknown", "", "capture: quests[199]"},
	}
	if len(steps) != len(want) {
		t.Fatalf("steps = %s", titles(steps))
	}
	for i, w := range want {
		s := steps[i]
		if s.Reason != w.reason || s.Zone != w.zone || !strings.Contains(strings.Join(s.Sources, ";"), w.source) {
			t.Errorf("step %d = %+v, want %+v", i, s, w)
		}
	}
	if strings.Contains(strings.Join(steps[2].Sources, ";"), "dataset") {
		t.Errorf("unknown level must not cite the dataset: %v", steps[2].Sources)
	}
}

func TestLimitAndRender(t *testing.T) {
	c := character.Character{Name: "Testrogue", Level: ip(12)}
	steps := []Step{{Kind: KindQuest, Title: "A", Reason: "r", Zone: "Z", Sources: []string{"s"}}, {Kind: KindQuest, Title: "B", Reason: "r", Sources: []string{"s"}}}
	if got := len(Limit(steps, 1)); got != 1 {
		t.Errorf("Limit = %d", got)
	}
	if got := len(Limit(steps, 0)); got != 2 {
		t.Errorf("Limit 0 = %d", got)
	}
	text := Render(c, Limit(steps, 1), 2)
	for _, w := range []string{"What's next for Testrogue (level 12)", "1. [quest] A: r (zone: Z) [source: s]", "(1 more not shown)"} {
		if !strings.Contains(text, w) {
			t.Errorf("render lacks %q:\n%s", w, text)
		}
	}
	if !strings.Contains(Render(c, nil, 0), "Nothing to suggest") {
		t.Error("empty render")
	}
}

type lineRow struct {
	LineID   int64  `parquet:"line_id"`
	Name     string `parquet:"name"`
	Position int32  `parquet:"position"`
	QuestID  int64  `parquet:"quest_id"`
}

func encode[T any](t *testing.T, rows []T) store.SizedReaderAt {
	t.Helper()
	var buf bytes.Buffer
	if err := parquet.Write(&buf, rows); err != nil {
		t.Fatal(err)
	}
	return store.SizedReaderAt{ReaderAt: bytes.NewReader(buf.Bytes()), Size: int64(buf.Len())}
}

// chainStore is the quests table with a three-step line "Wolf Hunt" (quests 100, 101, 102), or
// without the quest_lines table.
func chainStore(t *testing.T, lines bool) store.DataStore {
	t.Helper()
	src := store.ReaderSource{"quests": encode(t, []questRow{{100, i32(24), nil}, {101, i32(24), nil}, {102, i32(24), nil}, {103, i32(24), nil}})}
	if lines {
		src["quest_lines"] = encode(t, []lineRow{{7, "Wolf Hunt", 1, 100}, {7, "Wolf Hunt", 2, 101}, {7, "Wolf Hunt", 3, 102}})
	}
	return store.New(src)
}

func TestWhatsNextChain(t *testing.T) {
	done, open := bp(true), (*bool)(nil)
	char := character.Character{Level: ip(24), Quests: []character.Quest{
		{QuestID: 102, Title: "Last", Zone: "Dunemark", Complete: done},   // last step: unlocks nothing
		{QuestID: 100, Title: "Zephyr", Zone: "Dunemark", Complete: done}, // unlocks step 2; sorts after Last by title
		{QuestID: 103, Title: "Loner", Zone: "Dunemark", Complete: done},  // in no line
		{QuestID: 101, Title: "Second", Complete: open},                   // open chain step
	}}
	for _, tc := range []struct {
		name  string
		lines bool
		want  string
	}{
		{"chain marks and unlock boost", true,
			"turn_in:Turn in Zephyr: ready to turn in; step 1 of 3 in Wolf Hunt, unlocks step 2 | turn_in:Turn in Last: ready to turn in; step 3 of 3 in Wolf Hunt | " +
				"turn_in:Turn in Loner: ready to turn in | quest:Second: level 24 quest, you are 24; step 2 of 3 in Wolf Hunt"},
		{"no quest_lines leaves steps unchanged", false,
			"turn_in:Turn in Last: ready to turn in | turn_in:Turn in Loner: ready to turn in | " +
				"turn_in:Turn in Zephyr: ready to turn in | quest:Second: level 24 quest, you are 24"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			for _, s := range WhatsNext(char, chainStore(t, tc.lines)) {
				got = append(got, s.Kind+":"+s.Title+": "+s.Reason)
			}
			if g := strings.Join(got, " | "); g != tc.want {
				t.Errorf("steps =\n%s\nwant\n%s", g, tc.want)
			}
		})
	}
}

func TestOpenQuestsRankNearlyDoneThenHere(t *testing.T) {
	f, err := character.LoadFile("../../testdata/foreverscribe/quest_progress.lua")
	if err != nil {
		t.Fatal(err)
	}
	var titles []string
	for _, s := range WhatsNext(f.Characters[0], nil) {
		titles = append(titles, s.Title)
	}
	// Test Boars has 3 of 4 objectives done (75%), so it is first; then Testzone quests (where the last NPC
	// was met) before Otherzone; then by title.
	want := "Test Boars,Test Bare,Test Claws,Test Far"
	if got := strings.Join(titles, ","); got != want {
		t.Errorf("order = %s, want %s", got, want)
	}
	steps := WhatsNext(f.Characters[0], nil)
	if !strings.Contains(steps[0].Reason, "7/8 Boars slain") {
		t.Errorf("reason lacks progress: %q", steps[0].Reason)
	}
}

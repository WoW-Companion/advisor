package advice

import (
	"bytes"
	"strings"
	"testing"

	"github.com/parquet-go/parquet-go"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/store"
)

type availRow struct {
	ID        int64   `parquet:"id"`
	Name      string  `parquet:"name"`
	Level     *int32  `parquet:"level,optional"`
	Zone      *string `parquet:"zone_name,optional"`
	ClassMask int32   `parquet:"allowable_class_mask"`
}

type availLine struct {
	LineID   int64  `parquet:"line_id"`
	Name     string `parquet:"name"`
	Position int32  `parquet:"position"`
	QuestID  int64  `parquet:"quest_id"`
}

func parq[T any](t *testing.T, rows []T) store.SizedReaderAt {
	t.Helper()
	var buf bytes.Buffer
	if err := parquet.Write(&buf, rows); err != nil {
		t.Fatal(err)
	}
	return store.SizedReaderAt{ReaderAt: bytes.NewReader(buf.Bytes()), Size: int64(buf.Len())}
}

// A level 9 mage in Testisle with quest 1 in the log. Quest 2 is the one to pick up; the rest are
// decoys: 3 wrong class (priest only), 4 follows an unfinished step, 5 completed, 6 DNT, 7 too
// high, 8 in another zone. Quest 9 follows a finished step and is eligible too.
func availFixture(t *testing.T) (character.Character, store.DataStore) {
	t.Helper()
	z, far := "Testisle", "Farland"
	rows := []availRow{
		{1, "Logged Task", i32(9), &z, 0},
		{2, "Gather Kelp", i32(9), &z, 0},
		{3, "Priest Rite", i32(9), &z, 1 << 4},
		{4, "Second Step", i32(9), &z, 0},
		{5, "Done Already", i32(9), &z, 0},
		{6, "DNT Old Quest", i32(9), &z, 0},
		{7, "Too Hard", i32(14), &z, 0},
		{8, "Far Away", i32(9), &far, 0},
		{9, "Next Step", i32(10), &z, 0},
	}
	lines := []availLine{{50, "Tide", 1, 3}, {50, "Tide", 2, 4}, {60, "Reef", 1, 5}, {60, "Reef", 2, 9}}
	ds := store.New(store.ReaderSource{"quests": parq(t, rows), "quest_lines": parq(t, lines)})
	lvl := 9
	return character.Character{Level: &lvl, Class: "MAGE", Quests: []character.Quest{{QuestID: 1, Title: "Logged Task"}}, CompletedQuests: []int{5}}, ds
}

func TestAvailableQuests(t *testing.T) {
	c, ds := availFixture(t)
	got := AvailableQuests(c, ds)
	var ids []int
	for _, s := range got.Quests {
		ids = append(ids, s.QuestID)
	}
	if len(ids) != 2 || ids[0] != 2 || ids[1] != 9 {
		t.Fatalf("available = %v, want [2 9] (level fit first)", ids)
	}
	if s := got.Quests[0]; s.Title != "Pick up: Gather Kelp" || !strings.Contains(s.Reason, "level 9 quest") || !strings.Contains(s.Reason, "in Testisle") || !strings.Contains(s.Reason, "on the way") {
		t.Errorf("step = %+v", s)
	}
	if len(got.Unchecked) == 0 || !strings.Contains(strings.Join(got.Unchecked, ";"), "race") {
		t.Errorf("race is unchecked and must be said so: %v", got.Unchecked)
	}
	if next := WhatsNext(c, ds); !strings.Contains(titles(next), "Pick up: Gather Kelp") {
		t.Errorf("WhatsNext lacks the unpicked quest: %s", titles(next))
	}
}

func TestAvailableQuestsNoLevelData(t *testing.T) {
	lvl := 9
	ds := store.New(store.ReaderSource{"quests": parq(t, []struct {
		ID int64 `parquet:"id"`
	}{{1}})})
	got := AvailableQuests(character.Character{Level: &lvl}, ds)
	if len(got.Quests) != 0 || len(got.Unchecked) == 0 {
		t.Errorf("a dataset without quest levels must say so: %+v", got)
	}
	if got := AvailableQuests(character.Character{}, ds); len(got.Quests) != 0 {
		t.Errorf("no captured level, no suggestion: %+v", got)
	}
}

func TestAvailableQuestsUsesSavedLocation(t *testing.T) {
	c, ds := availFixture(t)
	c.Quests = nil // an empty log: only the last-saved zone says where the player is
	if got := AvailableQuests(c, ds); len(got.Quests) != 0 {
		t.Fatalf("no zone known, nothing may be called nearby: %+v", got)
	}
	c.Location = &character.Location{Zone: "Testisle"}
	got := AvailableQuests(c, ds)
	if len(got.Quests) == 0 || !strings.Contains(got.Quests[0].Reason, "where you were last saved") {
		t.Errorf("available = %+v", got.Quests)
	}
}

type availObs struct {
	QuestID int64  `parquet:"quest_id"`
	Title   string `parquet:"title"`
	Level   int32  `parquet:"level"`
	Giver   int64  `parquet:"giver_npc_id"`
}

type availNPC struct {
	ID   int64  `parquet:"id"`
	Name string `parquet:"name"`
	Zone string `parquet:"zone_name"`
}

// A pick-up step whose giver the client tables lack takes it from quests_observed.
func TestAvailableStepGiverFromObserved(t *testing.T) {
	c, _ := availFixture(t)
	z := "Testisle"
	c.Quests = []character.Quest{{QuestID: 1, Title: "Logged Task", Zone: z}}
	ds := store.New(store.ReaderSource{
		"quests":          parq(t, []availRow{{2, "Gather Kelp", i32(9), &z, 0}}),
		"quests_observed": parq(t, []availObs{{2, "Gather Kelp", 9, 70}}),
		"npcs":            parq(t, []availNPC{{70, "Warden Ila", "Testisle"}}),
	})
	got := AvailableQuests(c, ds)
	if len(got.Quests) != 1 || got.Quests[0].NPC != "Warden Ila" {
		t.Fatalf("steps: %+v", got.Quests)
	}
	if c2, d2 := availFixture(t); len(AvailableQuests(c2, d2).Quests) == 0 || AvailableQuests(c2, d2).Quests[0].NPC != "" {
		t.Error("a step with no known giver must stay without one")
	}
}

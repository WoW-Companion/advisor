package quests

import (
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/questie"
)

// A synthetic QuestieDB index: invented ids and names.
func qix() *questie.Index {
	return &questie.Index{
		Quests: map[int]*questie.Quest{
			95001: {ID: 95001, Name: "Fort Ember", Level: 10, Classes: 1, Starters: []questie.Ref{{ID: 7}}, Finishers: []questie.Ref{{ID: 8}},
				Objective: "Take the fort from Captain Vell at Ash Hollow.", Next: 95002},
			95002: {ID: 95002, Name: "Ember Aftermath", Level: 11, PreSingle: []int{95001}},
			95003: {ID: 95003, Name: "Side Errand", Level: 11, PreGroup: []int{95001}},
			95010: {ID: 95010, Name: "Dead End", Level: 12},
			10:    {ID: 10, Name: "Questie Wolf Pelts", Level: 5, PreSingle: []int{95010}},
		},
		Npcs:  map[int]questie.Entity{7: {Name: "Sergeant Pike", Spawns: []questie.Spawn{{Area: 1}}}, 8: {Name: "Scout Rhea"}},
		Areas: map[int]questie.Area{1: {Zone: "Ash Vale"}},
	}
}

func TestOfWithQuestieChain(t *testing.T) {
	c := OfWith(synth(t, true), qix(), 95001)
	if !c.Found || c.Source != SourceQuestie || len(c.After) != 2 || c.After[0].QuestID != 95002 || c.After[1].QuestID != 95003 {
		t.Fatalf("chain: %+v", c)
	}
	if c.NextQuestID != 95002 || c.Quest.Giver != "Sergeant Pike (Ash Vale)" || c.Quest.TurnIn != "Scout Rhea" || c.Quest.Classes != "Warrior" {
		t.Fatalf("facts: %+v", c.Quest)
	}
	got := Render(c, "Fort Ember")
	for _, want := range []string{"From your Questie database", "Take the fort from Captain Vell", "Leads to", "Ember Aftermath"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %s", want, got)
		}
	}
}

func TestOfWithQuestieBefore(t *testing.T) {
	c := OfWith(synth(t, true), qix(), 95002)
	if len(c.Before) != 1 || c.Before[0].Name != "Fort Ember" || len(c.After) != 0 {
		t.Fatalf("chain: %+v", c)
	}
	if got := Render(c, ""); !strings.Contains(got, "No follow-up quest is listed for it in the game data you have") {
		t.Fatal(got)
	}
}

func TestOfWithNeitherSource(t *testing.T) {
	c := OfWith(synth(t, true), qix(), 99999)
	if c.Found || !strings.Contains(Render(c, ""), "game data you have") {
		t.Fatalf("%+v", c)
	}
	if OfWith(synth(t, true), nil, 95001).Found {
		t.Fatal("no Questie, no chain")
	}
}

func TestOfWithDatasetWins(t *testing.T) {
	// Quest 10 is in the dataset's line 7 and in Questie with another pre-quest.
	c := OfWith(synth(t, true), qix(), 10)
	if c.Source != SourceDataset || c.Line != "Wolf Hunt" || c.Quest != nil || len(c.Before) != 0 {
		t.Fatalf("chain: %+v", c)
	}
}

func TestResolveWithQuestieOnly(t *testing.T) {
	id, name, c := ResolveWith(synth(t, true), qix(), "fort ember")
	if id != 95001 || name != "Fort Ember" || c != nil {
		t.Fatalf("%d %q %v", id, name, c)
	}
	id, name, _ = ResolveWith(synth(t, true), qix(), "95001")
	if id != 95001 || name != "Fort Ember" {
		t.Fatalf("%d %q", id, name)
	}
}

// A quest in one of the dataset's lines keeps the giver, turn-in, minimum level of the player's QuestieDB: the line gives the order only.
func TestOfWithLineKeepsQuestieFacts(t *testing.T) {
	ix := qix()
	ix.Quests[11] = &questie.Quest{ID: 11, Name: "Wolf Den", Level: 11, MinLevel: 9, PreSingle: []int{10}, Starters: []questie.Ref{{ID: 7}}, Finishers: []questie.Ref{{ID: 8}}}
	got := Render(OfWith(synth(t, true), ix, 11), "")
	for _, want := range []string{"step 2 of 3 in Wolf Hunt", "needs level 9", "starts with Sergeant Pike (Ash Vale)", "turn in to Scout Rhea", "> 2. Wolf Den"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
	for _, id := range []int64{10, 12} { // no QuestieDB facts to add: the line alone, nothing made up
		if got := Render(OfWith(synth(t, true), ix, id), ""); strings.Contains(got, "Questie") || strings.Contains(got, "Comes after") {
			t.Errorf("quest %d: %s", id, got)
		}
	}
	if got := Render(OfWith(synth(t, true), nil, 11), ""); strings.Contains(got, "Questie") {
		t.Errorf("no QuestieDB: %s", got)
	}
}

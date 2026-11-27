package inventory

import (
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/questie"
	"github.com/WoW-Companion/advisor/internal/store"
)

func protectDS(t *testing.T) store.DataStore {
	return store.New(store.ReaderSource{"items": table(t, []xItem{
		xi(50, "Kravel's Parts", 0, 12, 0, nil),    // grey quest item
		xi(51, "Glowing Shard", 1, 12, 0, nil),     // quest item
		xi(52, "Old Crate Key", 1, 12, 0, nil),     // leftover of a completed quest
		xi(53, "Ready Source Item", 1, 15, 0, nil), // not quest class: needed only by Questie id
	})})
}

func protectChar() character.Character {
	yes := true
	return character.Character{Name: "Rogue", Class: "ROGUE", Level: ip(32), BagsCaptured: true, BankCaptured: true,
		CompletedQuests: []int{300},
		Quests: []character.Quest{
			{QuestID: 100, Title: "Parts for Kravel", Complete: &yes, Objectives: []character.Objective{{Type: "item", Text: "Kravel's Parts: 1/1", Done: &yes}}},
			{QuestID: 200, Title: "The Glowing Shard", Complete: &yes},
		},
		Bags: []character.ItemStack{{ItemID: 50, Count: 1, Name: "Kravel's Parts"}, {ItemID: 51, Count: 1, Name: "Glowing Shard"},
			{ItemID: 52, Count: 1, Name: "Old Crate Key"}, {ItemID: 53, Count: 1, Name: "Ready Source Item"}}}
}

func protectIndex() *questie.Index {
	return &questie.Index{Quests: map[int]*questie.Quest{
		100: {ID: 100, Name: "Parts for Kravel", Items: []int{50}},
		200: {ID: 200, Name: "The Glowing Shard", Items: []int{51}, SrcItems: []int{53}},
		300: {ID: 300, Name: "Crate Business", Items: []int{52}},
	}}
}

func TestQuestItemsInTheLogAreKept(t *testing.T) {
	r := ReviewAllWith([]character.Character{protectChar()}, protectDS(t), protectIndex())
	for _, item := range []string{"Kravel's Parts", "Glowing Shard", "Ready Source Item"} {
		l := find(r.Lines, "Rogue", item)
		if l.Action != Keep_ || !strings.Contains(l.Reason, "needed for") {
			t.Errorf("%s = %+v, want keep: needed for a quest", item, l)
		}
	}
	if l := find(r.Lines, "Rogue", "Kravel's Parts"); !strings.Contains(l.Reason, "Parts for Kravel") {
		t.Errorf("reason does not name the quest: %q", l.Reason)
	}
	for _, l := range r.Lines {
		if l.Item != "Old Crate Key" && (l.Action == Sell || l.Action == Toss || l.Action == Mail) {
			t.Errorf("%s is %s", l.Item, l.Action)
		}
	}
}

func TestLeftoverQuestItemIsSafeToDestroy(t *testing.T) {
	r := ReviewAllWith([]character.Character{protectChar()}, protectDS(t), protectIndex())
	l := find(r.Lines, "Rogue", "Old Crate Key")
	if l.Action != Toss || !strings.Contains(l.Reason, "safe to destroy") || !strings.Contains(l.Reason, "Crate Business") {
		t.Errorf("leftover = %+v", l)
	}
}

func TestProtectedItemsListsReasons(t *testing.T) {
	ps := ProtectedItems(protectChar(), protectDS(t), protectIndex())
	if len(ps) != 3 {
		t.Fatalf("protected = %+v", ps)
	}
}

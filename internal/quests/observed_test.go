package quests

import (
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/store"
)

type obsRow struct {
	QuestID  int64  `parquet:"quest_id"`
	Title    string `parquet:"title"`
	Level    int32  `parquet:"level"`
	Giver    int64  `parquet:"giver_npc_id"`
	Faction  string `parquet:"faction"`
	Category string `parquet:"category"`
}

type npcRow struct {
	ID   int64  `parquet:"id"`
	Name string `parquet:"name"`
	Zone string `parquet:"zone_name"`
}

type clientRow struct {
	ID    int64  `parquet:"id"`
	Name  string `parquet:"name"`
	Level int32  `parquet:"level"`
}

func obsStore(t *testing.T, clientQuests []clientRow) store.DataStore {
	src := store.ReaderSource{
		"quests_observed": table(t, []obsRow{
			{95001, "Fort Ember (seen)", 12, 70, "Alliance", "Quest line"},
			{95500, "Seen Only", 8, 70, "Horde", "Daily"},
			{95501, "No Giver Yet", 9, 0, "", ""},
		}),
		"npcs": table(t, []npcRow{{70, "Warden Ila", "Ash Vale"}}),
	}
	if clientQuests != nil {
		src["quests"] = table(t, clientQuests)
	}
	return store.New(src)
}

func TestFactsFallbackOrder(t *testing.T) {
	ds := obsStore(t, []clientRow{{ID: 95001, Name: "Client Name", Level: 10}})
	f := FactsOf(ds, qix(), 95001)
	// Level and name come from the client table, the giver from Everloot (before Questie), the
	// turn-in only Questie knows.
	if f.Name != "Client Name" || f.Level != 10 || f.Giver != "Warden Ila (Ash Vale)" || f.TurnIn != "Scout Rhea" {
		t.Fatalf("facts: %+v", f)
	}
	if f.Source != SourceEverloot || f.Credit != Credit || f.Faction != "Alliance" {
		t.Fatalf("tags: %+v", f)
	}
}

func TestFactsEverlootOnlyAndQuestieOnly(t *testing.T) {
	ds := obsStore(t, nil)
	c := OfWith(ds, nil, 95500)
	if !c.Found || c.Source != SourceEverloot || c.Quest.Giver != "Warden Ila (Ash Vale)" || c.Quest.Level != 8 || c.Quest.Category != "Daily" {
		t.Fatalf("chain: %+v", c)
	}
	got := Render(c, "Seen Only")
	for _, want := range []string{"Seen by players on Everloot (CC BY-SA)", "Warden Ila (Ash Vale)", "Horde"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %s", want, got)
		}
	}
	// Not observed: Questie alone answers, tagged questie, with no Everloot credit.
	q := FactsOf(ds, qix(), 95010)
	if q == nil || q.Source != SourceQuestie || q.Credit != "" {
		t.Fatalf("questie-only: %+v", q)
	}
}

func TestFactsNoGiverAndUnknown(t *testing.T) {
	ds := obsStore(t, nil)
	if f := FactsOf(ds, nil, 95501); f == nil || f.Giver != "" || f.Level != 9 {
		t.Fatalf("no giver: %+v", f)
	}
	if FactsOf(ds, nil, 12345) != nil || OfWith(ds, nil, 12345).Found {
		t.Fatal("unknown quest must have no facts")
	}
}

func TestGiverNameSeenFillsOnlyAMissingGiver(t *testing.T) {
	seen := &character.Observed{Givers: []character.SeenGiver{{QuestID: 77, NPC: "Old Giver"}}}
	if n, fromSeen := GiverNameSeen(nil, seen, 77); n != "Old Giver" || !fromSeen {
		t.Errorf("%q %v", n, fromSeen)
	}
	if n, fromSeen := GiverNameSeen(nil, seen, 78); n != "" || fromSeen {
		t.Errorf("an unseen quest got a giver: %q", n)
	}
	if n, _ := GiverNameSeen(nil, nil, 77); n != "" {
		t.Errorf("nothing seen: %q", n)
	}
}

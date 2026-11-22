package unlock

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
)

const observedLua = `WoWCompanionDB = {
  observed = {
    version = 1,
    events = {
      { t = 1000, ev = "turnedin", questID = 30, character = "Hero", map = 7001, x = 50.0, y = 50.0, npc = "Elder Test" },
      { t = 1010, ev = "accepted", questID = 31, character = "Hero", map = 7001, x = 50.5, y = 50.0, npc = "Elder Test" },
      { t = 1500, ev = "accepted", questID = 50, character = "Hero", map = 7001, x = 20.0, y = 20.0, npc = "Passing Scout", offPlan = true },
      { t = 1600, ev = "abandoned", questID = 51, character = "Hero", map = 7001, x = 20.0, y = 20.0 },
      { t = 1700, ev = "shrugged", questID = 52 },
      { t = 1800, ev = "accepted", questID = 0 },
    },
  },
}`

func TestParseEventsKeepsKnownKinds(t *testing.T) {
	evs := ParseEvents(observedLua)
	if len(evs) != 4 || evs[0].Kind != EvTurnedIn || evs[2].QuestID != 50 || !evs[2].OffPlan || evs[2].NPC != "Passing Scout" {
		t.Fatalf("events %+v", evs)
	}
	if ParseEvents("not lua {{{") != nil || ParseEvents("WoWCompanionDB = {}") != nil {
		t.Fatal("a bad or empty file should give no events")
	}
}

func TestApplyAddsAcceptedAndDropsTurnedIn(t *testing.T) {
	captured := int64(900)
	c := character.Character{Name: "Hero", CapturedAt: &captured, Quests: []character.Quest{{QuestID: 30}, {QuestID: 51}}}
	got := Apply(c, ParseEvents(observedLua))
	var inLog []int
	for _, q := range got.Quests {
		inLog = append(inLog, q.QuestID)
	}
	if !reflect.DeepEqual(inLog, []int{31, 50}) {
		t.Fatalf("log after events %v", inLog)
	}
	if !reflect.DeepEqual(got.CompletedQuests, []int{30}) || !got.Quests[1].Observed {
		t.Fatalf("done %v, quests %+v", got.CompletedQuests, got.Quests)
	}
	if len(c.Quests) != 2 || len(c.CompletedQuests) != 0 {
		t.Fatal("Apply changed its input")
	}
	// Events older than the capture, or of another character, change nothing.
	late := int64(5000)
	c.CapturedAt = &late
	if got := Apply(c, ParseEvents(observedLua)); len(got.Quests) != 2 {
		t.Fatalf("old events applied: %+v", got.Quests)
	}
	c.CapturedAt, c.Name = &captured, "Other"
	if got := Apply(c, ParseEvents(observedLua)); len(got.Quests) != 2 {
		t.Fatalf("another character's events applied: %+v", got.Quests)
	}
}

func TestLearnPairsATurnInWithTheNextAcceptance(t *testing.T) {
	got := Learn(ParseEvents(observedLua))
	if len(got) != 1 || got[0].From != 30 || got[0].To != 31 || got[0].NPC != "Elder Test" || got[0].Map != 7001 {
		t.Fatalf("learned %+v", got)
	}
	far := []Event{{T: 1, Kind: EvTurnedIn, QuestID: 1, Map: 1, X: 10, Y: 10}, {T: 5, Kind: EvAccepted, QuestID: 2, Map: 1, X: 80, Y: 80}}
	late := []Event{{T: 1, Kind: EvTurnedIn, QuestID: 1, Map: 1, X: 10, Y: 10}, {T: 999, Kind: EvAccepted, QuestID: 2, Map: 1, X: 10, Y: 10}}
	if len(Learn(far)) != 0 || len(Learn(late)) != 0 {
		t.Fatal("a distant or late acceptance was learned as an unlock")
	}
}

func TestStoreRoundTripAndMerge(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "observed-unlocks.json")
	s := Open(path)
	if len(s.Obs) != 0 || !s.Merge(Learn(ParseEvents(observedLua))) {
		t.Fatal("first merge should add")
	}
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	again := Open(path)
	if !reflect.DeepEqual(again.Obs, s.Obs) {
		t.Fatalf("reopened %+v, saved %+v", again.Obs, s.Obs)
	}
	if again.Merge(Learn(ParseEvents(observedLua))) {
		t.Fatal("the same observation counted twice")
	}
	if !again.Merge([]Obs{{From: 30, To: 31, NPC: "Elder Test", Map: 7001, X: 51, Y: 50, Count: 1, Last: 2000}}) || again.Obs[0].Count != 2 {
		t.Fatalf("a newer sighting should update: %+v", again.Obs)
	}
}

func TestAddonSV(t *testing.T) {
	for in, want := range map[string]string{
		"/g/WTF/Account/A/SavedVariables/ForeverScribe.lua":   "/g/WTF/Account/A/SavedVariables/WoWCompanion.lua",
		`C:\g\WTF\Account\A\SavedVariables\ForeverScribe.lua`: `C:\g\WTF\Account\A\SavedVariables\WoWCompanion.lua`,
		"ForeverScribe.lua": "",
	} {
		if got := AddonSV(in); got != want {
			t.Errorf("AddonSV(%q) = %q, want %q", in, got, want)
		}
	}
}

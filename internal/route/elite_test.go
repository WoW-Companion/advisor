package route

import (
	"slices"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/questie"
)

// eliteIsle is bulwarkIsle with elite quests: 41 is in the log and QuestieDB says elite (the capture
// is silent), 40 is a class quest also marked elite, 43 is elite and not yet accepted, 42 is plain.
func eliteIsle() (c character.Character, run func(prefs *character.PlanPrefs, cap41 *bool) Plan) {
	c = bulwarkChar()
	run = func(prefs *character.PlanPrefs, cap41 *bool) Plan {
		ix := bulwarkIsle(true)
		ix.Quests[40].Elite, ix.Quests[41].Elite = true, true
		ix.Quests[43] = ixQuestCopy(ix, 42, 43)
		ix.Quests[43].Elite = true
		cc := c
		cc.Quests = slices.Clone(c.Quests)
		cc.Quests[1].Elite = cap41
		cc.PlanPrefs = prefs
		return Build(cc, ix)
	}
	return c, run
}

func eliteStopFlags(p Plan, id int) (found, elite bool) {
	for _, s := range slices.Concat(p.Steps, p.Later) {
		if s.QuestID == id {
			found, elite = true, s.Elite
		}
	}
	return found, elite
}

func TestQuestieEliteLogQuestParkedWhenOff(t *testing.T) {
	_, run := eliteIsle()
	p := run(&character.PlanPrefs{LevelReach: 3}, nil)
	if k := parkedOfID(p, 41); k == nil || k.Kind != ParkElite || !k.Elite || slices.Contains(stepQuests(p), 41) {
		t.Errorf("a QuestieDB elite log quest: parked %+v", k)
	}
	if parkedOfID(p, 42) != nil || !slices.Contains(stepQuests(p), 42) {
		t.Errorf("a plain quest should be planned: %v", stepQuests(p))
	}
}

func TestEliteQuestPlannedWithFlagWhenOn(t *testing.T) {
	_, run := eliteIsle()
	p := run(&character.PlanPrefs{LevelReach: 3, IncludeElite: true}, nil)
	if parkedOfID(p, 41) != nil {
		t.Fatalf("an elite quest is planned when included")
	}
	if found, elite := eliteStopFlags(p, 41); !found || !elite {
		t.Errorf("quest 41 steps lack elite: %+v", p.Steps)
	}
	if _, elite := eliteStopFlags(p, 42); elite {
		t.Errorf("a plain quest carries elite")
	}
	lua := Envelope{Plans: []Plan{p}}.Lua()
	if !strings.Contains(lua, "questID = 41, quest = \"Quest 41\"") || !strings.Contains(lua, ", elite = true") {
		t.Errorf("Plan.lua lacks elite = true:\n%s", lua)
	}
}

func TestCaptureSaysNotEliteOverQuestie(t *testing.T) {
	_, run := eliteIsle()
	no := false
	p := run(&character.PlanPrefs{LevelReach: 3}, &no)
	if parkedOfID(p, 41) != nil || !slices.Contains(stepQuests(p), 41) {
		t.Errorf("an explicit elite = false in the capture wins: %v", stepQuests(p))
	}
}

func TestNotAcceptedEliteQuestStaysLaterNotPlanned(t *testing.T) {
	_, run := eliteIsle()
	p := run(&character.PlanPrefs{LevelReach: 3}, nil)
	for _, s := range p.Steps {
		if s.QuestID == 43 {
			t.Fatalf("an unaccepted elite quest is planned: %+v", s)
		}
	}
	if found, elite := eliteStopFlags(p, 43); !found || !elite {
		t.Errorf("an unaccepted elite quest should be visible under later with elite: %+v", p.Later)
	}
	on := run(&character.PlanPrefs{LevelReach: 3, IncludeElite: true}, nil)
	if !slices.Contains(stepQuests(on), 43) {
		t.Errorf("an unaccepted elite quest is planned when included: %v", stepQuests(on))
	}
}

func TestMustDoClassEliteQuestStaysWithFlag(t *testing.T) {
	_, run := eliteIsle()
	p := run(&character.PlanPrefs{LevelReach: 3}, nil)
	if parkedOfID(p, 40) != nil {
		t.Fatalf("a class quest is never parked as elite")
	}
	if found, elite := eliteStopFlags(p, 40); !found || !elite {
		t.Errorf("class quest 40 should be a step with elite: %+v", p.Steps)
	}
}

func TestCaptureGroupSizeMarksQuestElite(t *testing.T) {
	two := 5
	q := character.Quest{Group: &two}
	if e, ok := q.IsElite(); !e || !ok {
		t.Errorf("group 5: elite %v ok %v", e, ok)
	}
	if e, ok := (character.Quest{}).IsElite(); e || ok {
		t.Errorf("silent capture: elite %v ok %v", e, ok)
	}
}

func ixQuestCopy(ix *questie.Index, from, to int) *questie.Quest {
	q := *ix.Quests[from]
	q.ID, q.Name = to, "Quest "+itoa(to)
	return &q
}

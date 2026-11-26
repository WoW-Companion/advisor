package route

import (
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/questie"
)

func TestObjectiveLabel(t *testing.T) {
	for _, tc := range []struct{ verb, in, want string }{
		{"Kill", "Test Boar slain: 0/1", "Kill Test Boar (0/1)"},
		{"Collect", "Linen Cloth: 3/8", "Collect Linen Cloth (3/8)"},
		{"Kill", "Test Wolf", "Kill Test Wolf"},
		{"Kill", "Kill Test Wolf: 2/5", "Kill Test Wolf (2/5)"},
		{"", "Explore the Hollow", "Explore the Hollow"},
		{"Collect", "8/8 Spare Gears", "Collect Spare Gears (8/8)"},
		{"Collect", "Obtain Enchanted Gyrozephyr from Windsong Lake: 0/1", "Obtain Enchanted Gyrozephyr from Windsong Lake (0/1)"},
		{"Collect", "Listen to what Riaani Nightwind has to say", "Listen to what Riaani Nightwind has to say"},
		{"Kill", "Wolf slain: 0/8", "Kill Wolf (0/8)"},
		{"Collect", "Linen: 3/8", "Collect Linen (3/8)"},
		{"Collect", "OBTAIN Air Construct Core: 0/1", "OBTAIN Air Construct Core (0/1)"},
		{"Collect", "Investigate the Camp", "Investigate the Camp"},
		{"Collect", "Obtainable Herbs: 1/2", "Collect Obtainable Herbs (1/2)"},
	} {
		if got := objectiveLabel(tc.verb, tc.in); got != tc.want {
			t.Errorf("objectiveLabel(%q, %q) = %q, want %q", tc.verb, tc.in, got, tc.want)
		}
	}
}

// Plan steps read as actions: no quest name repeated on an objective, no "map units" anywhere a
// player reads.
func TestPlanStepWording(t *testing.T) {
	c := hero()
	c.Quests = []character.Quest{{QuestID: 10, Title: "Quest 10", Objectives: []character.Objective{{Text: "Test Wolf slain: 1/8"}}}}
	ix := hubIsle()
	ix.Quests[10].Creatures = []questie.Ref{{ID: 4}}
	p := BuildWith(c, ix, anyPickup)
	var kill, pick, turn string
	for _, s := range p.Steps {
		switch {
		case s.Kind == KindObjective && s.QuestID == 10:
			kill = s.Label
		case s.Kind == KindPickup && s.QuestID == 11:
			pick = s.Label
		case s.Kind == KindTurnIn && s.QuestID == 10:
			turn = s.Label
		}
	}
	if kill != "Kill Test Wolf (1/8)" || pick != "Pick up Quest 11, Quest 13, Quest 14 from Elder Test" || turn != "Turn in Quest 10, Quest 13 to Elder Test" {
		t.Errorf("kill %q, pick %q, turn %q", kill, pick, turn)
	}
	lines, summary := Lines(p, 20)
	all := append(append(lines, summary, Render(p, 20)), StepText(p.Steps[0]))
	for _, s := range all {
		if strings.Contains(s, "map units") || strings.Contains(s, "(48") {
			t.Errorf("player-facing text %q", s)
		}
	}
	if !strings.HasSuffix(summary, " stops") {
		t.Errorf("summary %q", summary)
	}
}

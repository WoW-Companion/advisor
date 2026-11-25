package route

import (
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/questie"
	"github.com/WoW-Companion/advisor/internal/wording"
)

// vialIsle is hubIsle plus quest 30, a quest with three placed-vial objectives at three posts.
func vialIsle() *questie.Index {
	ix := hubIsle()
	sp := func(x, y float64) []questie.Spawn { return []questie.Spawn{{Area: 901, X: x, Y: y}} }
	ix.Npcs[31] = questie.Entity{Name: "West Post", Spawns: sp(20, 30)}
	ix.Npcs[32] = questie.Entity{Name: "North Post", Spawns: sp(40, 80)}
	ix.Npcs[33] = questie.Entity{Name: "South Post", Spawns: sp(80, 20)}
	ix.Quests[30] = &questie.Quest{ID: 30, Name: "Quest 30", Level: 9, Starters: []questie.Ref{{ID: 1}}, Finishers: []questie.Ref{{ID: 1}},
		Credits: []questie.Credit{{Name: "Western vial", NPCs: []int{31}}, {Name: "Northern vial", NPCs: []int{32}}, {Name: "Southern vial", NPCs: []int{33}}}}
	return ix
}

func vialChar(complete bool, objs ...character.Objective) character.Character {
	c := hero()
	c.Quests = []character.Quest{{QuestID: 30, Title: "Quest 30", Complete: &complete, Objectives: objs}}
	return c
}

func vial(name string, done bool) character.Objective {
	have, need := 0, 1
	if done {
		have = 1
	}
	return character.Objective{Text: name + " vial placed: " + itoa(have) + "/1", Have: &have, Need: &need, Done: &done}
}

func TestUnreadyQuestGetsObjectivesAndNoEarlyTurnIn(t *testing.T) {
	p := Build(vialChar(false, vial("Western", false), vial("Northern", false), vial("Southern", false)), vialIsle())
	ti := stepIndex(p, KindTurnIn, 30)
	objs := 0
	for i, s := range p.Steps {
		if s.Kind == KindObjective && s.QuestID == 30 {
			objs++
			if ti >= 0 && i > ti {
				t.Errorf("objective after the turn-in: %+v", p.Steps)
			}
		}
	}
	if objs != 3 {
		t.Fatalf("want 3 objective steps, got %d: %+v", objs, p.Steps)
	}
	if s := find(p, KindObjective, 30); !strings.HasPrefix(s.Label, "Place ") {
		t.Errorf("label %q", s.Label)
	}
}

// A capture that lists no objectives for an unfinished quest gives no standalone turn-in.
func TestUnreadyQuestWithoutObjectivesHasNoTurnIn(t *testing.T) {
	p := Build(vialChar(false), vialIsle())
	if find(p, KindTurnIn, 30) != nil {
		t.Errorf("turn-in for an unready quest: %+v", p.Steps)
	}
	if len(p.Find) == 0 || !strings.HasPrefix(p.Find[0].Label, "Finish ") {
		t.Errorf("find = %+v", p.Find)
	}
}

func TestReadyQuestGetsTurnIn(t *testing.T) {
	for name, c := range map[string]character.Character{
		"flag":       vialChar(true),
		"objectives": vialChar(false, vial("Western", true), vial("Northern", true), vial("Southern", true)),
	} {
		p := Build(c, vialIsle())
		if find(p, KindTurnIn, 30) == nil || find(p, KindObjective, 30) != nil {
			t.Errorf("%s: %+v", name, p.Steps)
		}
	}
}

func TestEveryStepStartsWithAnAction(t *testing.T) {
	for name, c := range map[string]character.Character{
		"open":  vialChar(false, vial("Western", false), vial("Northern", true), vial("Southern", false)),
		"bare":  vialChar(false),
		"ready": vialChar(true),
		"none":  hero(),
	} {
		p := Build(c, vialIsle())
		for _, s := range p.Steps {
			if !wording.StartsWithAction(s.Label) {
				t.Errorf("%s: step %d %q does not start with an action", name, s.N, s.Label)
			}
		}
		for _, f := range p.Find {
			if !wording.StartsWithAction(f.Label) {
				t.Errorf("%s: find %q does not start with an action", name, f.Label)
			}
		}
	}
}

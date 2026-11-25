package route

import (
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/lua"
)

func guideOf(p Plan, id int) *Guide {
	for i := range p.Guides {
		if p.Guides[i].QuestID == id {
			return &p.Guides[i]
		}
	}
	return nil
}

func TestClassQuestGuidanceWritten(t *testing.T) {
	ix := hubIsle()
	ix.Quests[13].Classes = 1 << 7
	c := hero()
	c.Class = "MAGE"
	p := Build(c, ix)
	g := guideOf(p, 13)
	if g == nil || !g.Class {
		t.Fatalf("no class guidance: %+v", p.Guides)
	}
	if len(g.Objectives) != 1 || g.Objectives[0].Text != "Kill Test Wolf" || g.Objectives[0].Map != 7001 || g.Objectives[0].X != 30 {
		t.Errorf("objectives %+v", g.Objectives)
	}
	if g.TurnIn == nil || g.TurnIn.Text != "Turn in Quest 13 to Elder Test" || g.TurnIn.Y != 50 {
		t.Errorf("turn-in %+v", g.TurnIn)
	}
	if len(g.Steps) == 0 || !strings.HasPrefix(g.After, "After this: ") {
		t.Errorf("steps %v after %q", g.Steps, g.After)
	}
	root := luaRoot(t, p)
	gd := root["characters"].(lua.Table)["Hero"].(lua.Table)["guidance"].(lua.Table)
	if q, ok := gd[int64(13)].(lua.Table); !ok || q["questID"] != int64(13) || q["class"] != true {
		t.Errorf("Plan.lua guidance for 13: %v", gd)
	}
}

func luaRoot(t *testing.T, p Plan) lua.Table {
	t.Helper()
	vars, err := lua.Parse(Envelope{Generated: 1, Plans: []Plan{p}}.Lua())
	if err != nil {
		t.Fatal(err)
	}
	return vars["WoWCompanionPlan"].(lua.Table)
}

func TestGuidanceUnknownPositionFallsBackToZone(t *testing.T) {
	p := Build(hero(), objIsle())
	var g *Guide
	for i := range p.Guides {
		for _, o := range p.Guides[i].Objectives {
			if o.Map == 0 {
				g = &p.Guides[i]
			}
		}
	}
	if g == nil {
		t.Fatalf("no unplaced objective guided: %+v", p.Guides)
	}
	for _, o := range g.Objectives {
		if o.Map == 0 && (o.X != 0 || o.Y != 0 || o.Zone == "" || strings.Contains(o.Text, "location unknown")) {
			t.Errorf("zone-only item %+v", o)
		}
	}
	if g.TurnIn != nil {
		t.Errorf("a withheld turn-in was guided: %+v", g.TurnIn)
	}
}

func TestStepsCarryKindAndQuestID(t *testing.T) {
	p := Build(hero(), hubIsle())
	root := luaRoot(t, p)
	steps := lua.List(root["characters"].(lua.Table)["Hero"].(lua.Table)["steps"])
	kinds := map[string]bool{}
	for _, s := range steps {
		st := s.(lua.Table)
		kinds[st["kind"].(string)] = true
		if st["kind"] != "travel" && st["questID"].(int64) == 0 {
			t.Errorf("step without a quest id: %v", st)
		}
	}
	if !kinds["pickup"] || !kinds["objective"] || !kinds["turnin"] {
		t.Errorf("kinds %v", kinds)
	}
}

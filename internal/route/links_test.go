package route

import (
	"slices"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/lua"
	"github.com/WoW-Companion/advisor/internal/questie"
	"github.com/WoW-Companion/advisor/internal/wording"
)

func stepN(p Plan, kind string, q int) int {
	if i := stepIndex(p, kind, q); i >= 0 {
		return p.Steps[i].N
	}
	return -1
}

func TestStepsCarryTheirDependencies(t *testing.T) {
	p := Build(hero(), chainIsle())
	pick, obj, turn := stepN(p, KindPickup, 30), stepN(p, KindObjective, 30), stepN(p, KindTurnIn, 30)
	if pick < 0 || obj < 0 || turn < 0 {
		t.Fatalf("steps of quest 30 missing: %+v", p.Steps)
	}
	at := func(n int) Stop { return p.Steps[n-1] }
	if got := at(pick).Needs; len(got) != 0 {
		t.Errorf("pick-up needs %v", got)
	}
	if got := at(obj).Needs; !slices.Equal(got, []int{pick}) {
		t.Errorf("objective needs %v, want [%d]", got, pick)
	}
	if got := at(turn).Needs; !slices.Equal(got, []int{min(pick, obj), max(pick, obj)}) {
		t.Errorf("turn-in needs %v, want pick-up %d and objective %d", got, pick, obj)
	}
	if i := stepIndex(p, KindPickup, 31); i >= 0 && p.Steps[i].Locked {
		if got := p.Steps[i].Needs; !slices.Equal(got, []int{turn}) {
			t.Errorf("chain follow-up needs %v, want [%d]", got, turn)
		}
	} else {
		t.Error("no locked follow-up in the plan")
	}
}

func TestPlanLuaCarriesAfterFlightsAndStart(t *testing.T) {
	ix := chainIsle()
	ix.Areas[7001] = questie.Area{UIMap: 7001, Zone: "Testisle", Sub: "Port"}
	ix.Npcs[70] = questie.Entity{Name: "Windrider", Flags: 0x2000, Spawns: []questie.Spawn{{Area: 7001, X: 40, Y: 41}}}
	ix.Npcs[71] = questie.Entity{Name: "Enemy Rider", Flags: 0x2000, Faction: "H", Spawns: []questie.Spawn{{Area: 7001, X: 10, Y: 11}}}
	ix.Npcs[72] = questie.Entity{Name: "Cape Rider", Flags: 0x2000, Spawns: []questie.Spawn{{Area: 7001, X: 60, Y: 61}}}
	c := hero()
	c.Faction = "Alliance"
	at := int64(1234)
	c.CapturedAt = &at
	c.Seen = &character.Observed{Flights: []character.SeenFlight{{NPC: 72, Name: "Cape Rider", Map: 7001, X: 60, Y: 61, T: 1}}}
	p := Build(c, ix)
	vars, err := lua.Parse(Envelope{Generated: 1, Plans: []Plan{p}}.Lua())
	if err != nil {
		t.Fatal(err)
	}
	hero := vars["WoWCompanionPlan"].(lua.Table)["characters"].(lua.Table)["Hero"].(lua.Table)
	start := hero["start"].(lua.Table)
	if start["map"] != int64(7001) || start["at"] != at {
		t.Errorf("start %v", start)
	}
	for _, s := range lua.List(hero["steps"]) {
		if _, ok := s.(lua.Table)["after"]; !ok {
			t.Errorf("step without after: %v", s)
		}
	}
	known := map[string]bool{}
	for _, f := range lua.List(hero["flights"]) {
		ft := f.(lua.Table)
		known[ft["name"].(string)] = ft["known"] == true
		if ft["node"] == "" || ft["map"] != int64(7001) {
			t.Errorf("flight %v", ft)
		}
	}
	if len(known) != 2 || known["Windrider"] || !known["Cape Rider"] {
		t.Errorf("flights %v: want the unknown Windrider, the known Cape Rider, no horde master", known)
	}
}

func TestStartIsAlwaysWritten(t *testing.T) {
	c := hero()
	c.Location = nil
	text := Envelope{Generated: 1, Plans: []Plan{Build(c, chainIsle())}}.Lua()
	if !strings.Contains(text, "      start = { map = 0, x = -1.0, y = -1.0, at = 0 },") {
		t.Errorf("no start in %s", text)
	}
}

func TestSomewhereNamesTheZoneOnce(t *testing.T) {
	for _, tc := range [][2]string{
		{"Locate the hunters' camp", "Locate the hunters' camp (somewhere in Stranglethorn Vale)"},
		{"Locate the hunters' camp in Stranglethorn Vale", "Locate the hunters' camp (somewhere in Stranglethorn Vale)"},
		{"Locate the camp (somewhere in Stranglethorn Vale) in Stranglethorn Vale", "Locate the camp (somewhere in Stranglethorn Vale)"},
	} {
		if got := wording.Somewhere(tc[0], "Stranglethorn Vale"); got != tc[1] {
			t.Errorf("%q: got %q", tc[0], got)
		}
	}
	if got := wording.Action("", "Locate the hunters' camp"); strings.HasPrefix(got, "Complete") {
		t.Errorf("Locate is an action: %q", got)
	}
}

func TestNoLabelRepeatsItsZone(t *testing.T) {
	p := Build(hero(), chainIsle())
	var labels []string
	for _, list := range [][]Stop{p.Steps, p.Later} {
		for _, s := range list {
			labels = append(labels, s.Label)
		}
	}
	for _, f := range p.Find {
		labels = append(labels, f.Label)
	}
	for _, l := range labels {
		if strings.Count(l, p.Zone) > 1 || strings.Contains(l, "(somewhere in "+p.Zone+") in "+p.Zone) {
			t.Errorf("label repeats its zone: %q", l)
		}
	}
}

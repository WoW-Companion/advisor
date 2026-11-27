package route

import (
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/questie"
)

// hearthWorld is a character in Undercity with a ready turn-in in each zone, and an innkeeper in Tarren Mill
// (Hillsbrad Foothills), so a bind named "Tarren Mill" places by the innkeeper.
func hearthWorld(zones ...string) (*questie.Index, character.Character) {
	ix, c := routeWorld(zones...)
	ix.Areas[950] = questie.Area{UIMap: 7200, Zone: "Hillsbrad Foothills", Sub: "Tarren Mill"}
	ix.Npcs[700] = questie.Entity{Name: "Innkeeper Shay", Flags: questie.FlagInnkeeper, Spawns: []questie.Spawn{{Area: 950, X: 60, Y: 20}}}
	c.Hearth = "Tarren Mill"
	return ix, c
}

func hearthLegs(p Plan) (n int) {
	for _, t := range p.Trips {
		if t.Travel != "" && strings.HasPrefix(t.Travel, "Hearth to ") {
			n++
		}
	}
	return n
}

func totalETA(p Plan) (n int) {
	for _, t := range p.Trips {
		n += t.ETA
	}
	return n
}

func TestHearthReadyAndFasterIsUsedOnce(t *testing.T) {
	ix, c := hearthWorld("Hillsbrad Foothills", "Arathi Highlands")
	yes, zero := true, 0
	c.HearthHave, c.HearthReadyIn = &yes, &zero
	p := Build(c, ix)
	if len(p.Trips) != 2 || hearthLegs(p) != 1 {
		t.Fatalf("trips %+v", p.Trips)
	}
	first := p.Trips[0]
	if first.Zone != "Hillsbrad Foothills" || !strings.HasPrefix(first.Travel, "Hearth to Tarren Mill: ") || first.ETA != 1 {
		t.Errorf("first trip %+v", first)
	}
	if p.Exit == nil || p.Exit.Via != "hearth" {
		t.Errorf("exit %+v", p.Exit)
	}
	if strings.Contains(p.Trips[1].Travel, "Hearth") {
		t.Errorf("second use of the hearthstone: %+v", p.Trips[1])
	}
}

func TestHearthThenWalkWordsTheRestOfTheHop(t *testing.T) {
	ix, c := hearthWorld("Alterac Mountains")
	p := Build(c, ix)
	if len(p.Trips) != 1 || !strings.HasPrefix(p.Trips[0].Travel, "Hearth to Tarren Mill, then walk or ride to Alterac Mountains: ") || p.Trips[0].ETA != 9 {
		t.Errorf("trips %+v", p.Trips)
	}
}

func TestHearthSlowerThanTheWalkIsNotUsed(t *testing.T) {
	ix, c := hearthWorld("Tirisfal Glades", "Hillsbrad Foothills")
	c.Hearth = "Tarren Mill"
	p := Build(c, ix)
	if p.Trips[0].Zone != "Tirisfal Glades" || strings.Contains(p.Trips[0].Travel, "Hearth") {
		t.Errorf("trips %+v", p.Trips)
	}
	if p.Trips[0].ETA != 2 {
		t.Errorf("eta %d", p.Trips[0].ETA)
	}
}

func TestHearthOnCooldownLongerThanThePlanIsNotUsed(t *testing.T) {
	ix, c := hearthWorld("Hillsbrad Foothills")
	yes, wait, cd := true, 7200, 3600
	c.HearthHave, c.HearthReadyIn, c.HearthCooldown = &yes, &wait, &cd
	p := Build(c, ix)
	if hearthLegs(p) != 0 || strings.Contains(p.Trips[0].Travel, "Hearth") || strings.Contains(p.Trips[0].Travel, "cooldown") {
		t.Errorf("trips %+v", p.Trips)
	}
	// Aged by the time since the capture, the same wait is over.
	at := int64(1000)
	c.CapturedAt = &at
	p = BuildWith(c, ix, Options{Now: at + 7300})
	if hearthLegs(p) != 1 {
		t.Errorf("aged: trips %+v", p.Trips)
	}
}

func TestHearthReadyMidPlanIsUsedOnALaterLeg(t *testing.T) {
	ix, c := hearthWorld("Tirisfal Glades", "Silverpine Forest", "Arathi Highlands", "Hillsbrad Foothills")
	wait := 600
	c.HearthReadyIn = &wait
	with := Build(c, ix)
	c.HearthReadyIn = nil
	c.Hearth = ""
	without := Build(c, ix)
	if hearthLegs(with) != 1 {
		t.Fatalf("trips %+v", with.Trips)
	}
	if strings.Contains(with.Trips[0].Travel, "Hearth") || with.Trips[0].ETA == 1 {
		t.Errorf("hearthstone used before it was ready: %+v", with.Trips[0])
	}
	if totalETA(with) >= totalETA(without) {
		t.Errorf("hearth did not save time: %d vs %d", totalETA(with), totalETA(without))
	}
	if again := Build(c, ix); totalETA(again) != totalETA(without) {
		t.Errorf("not deterministic")
	}
}

func TestHearthBindNotPlaceableAddsNothingAndSaysSo(t *testing.T) {
	ix, c := hearthWorld("Hillsbrad Foothills")
	c.Hearth = "Nowhere Inn"
	p := Build(c, ix)
	if hearthLegs(p) != 0 || !strings.Contains(p.Basis.Hearth, "Nowhere Inn") || !strings.Contains(p.Basis.Hearth, "could not place") {
		t.Errorf("basis %q trips %+v", p.Basis.Hearth, p.Trips)
	}
}

func TestHearthOlderCaptureCountsAsReady(t *testing.T) {
	ix, c := hearthWorld("Hillsbrad Foothills")
	if c.HearthHave != nil || c.HearthReadyIn != nil {
		t.Fatal("fixture has the fields")
	}
	if p := Build(c, ix); hearthLegs(p) != 1 {
		t.Errorf("trips %+v", p.Trips)
	}
	no := false
	c.HearthHave = &no
	if p := Build(c, ix); hearthLegs(p) != 0 {
		t.Errorf("no stone: trips %+v", p.Trips)
	}
}

func TestPlaceBindByCityOrZone(t *testing.T) {
	g := newGraph(character.Character{Faction: "Horde"})
	for name, want := range map[string]string{"Orgrimmar": "Orgrimmar", "stormwind": "Stormwind City", "Durotar": "Durotar", "Nowhere": ""} {
		got, ok := placeBind(g, nil, name)
		if got != want || ok != (want != "") {
			t.Errorf("%s: %q %v", name, got, ok)
		}
	}
}

func TestHearthWait(t *testing.T) {
	yes, no, wait := true, false, 600
	at := int64(1000)
	for _, tc := range []struct {
		name   string
		c      character.Character
		now    int64
		wait   int
		usable bool
	}{
		{"no bind", character.Character{}, 0, 0, false},
		{"older capture", character.Character{Hearth: "X"}, 5000, 0, true},
		{"no stone", character.Character{Hearth: "X", HearthHave: &no}, 0, 0, false},
		{"as captured", character.Character{Hearth: "X", HearthHave: &yes, HearthReadyIn: &wait}, 0, 600, true},
		{"aged", character.Character{Hearth: "X", HearthReadyIn: &wait, CapturedAt: &at}, 1200, 400, true},
		{"aged past ready", character.Character{Hearth: "X", HearthReadyIn: &wait, CapturedAt: &at}, 9000, 0, true},
	} {
		if w, u := tc.c.HearthWait(tc.now); w != tc.wait || u != tc.usable {
			t.Errorf("%s: %d %v", tc.name, w, u)
		}
	}
}

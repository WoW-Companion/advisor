package character

import (
	"strings"
	"testing"
	"time"
)

func scout(t *testing.T) Character { return fixture(t, "quest_progress.lua").Characters[0] }

func TestProgressParsing(t *testing.T) {
	c := scout(t)
	tests := []struct {
		i    int
		text string
		frac float64
		ok   bool
		near bool
	}{
		{0, "Test Claws: 3/8 Test Claws", 0, true, false},
		{1, "Test Boars: 7/8 Boars slain; 1/1 Test Tusk; 1/1 Test Hide; 1/1 Test Fang", 0.75, true, true},
		{2, "", 0, false, false}, // an objective with no counts or done flag
		{3, "", 0, false, false}, // no objectives at all
	}
	for _, tc := range tests {
		q := c.Quests[tc.i]
		if got := q.ProgressText(); got != tc.text {
			t.Errorf("quest %d text = %q, want %q", tc.i, got, tc.text)
		}
		if f, ok := q.Fraction(); f != tc.frac || ok != tc.ok || q.NearlyDone() != tc.near {
			t.Errorf("quest %d fraction = %v %v near %v", tc.i, f, ok, q.NearlyDone())
		}
	}
}

func TestNPCsIngestedTolerantly(t *testing.T) {
	c := scout(t)
	if len(c.NPCs) != 5 {
		t.Fatalf("npcs = %d", len(c.NPCs))
	}
	var sparse NPC
	for _, n := range c.NPCs {
		if n.ID == 9005 {
			sparse = n
		}
	}
	if sparse.X != nil || sparse.Seen != nil || sparse.Kind != "quest" {
		t.Errorf("sparse npc = %+v", sparse)
	}
}

func TestApproximatePositionFromLastNPC(t *testing.T) {
	c := scout(t)
	pos := c.LastKnownPosition()
	if pos == nil || !pos.Approximate || pos.X != 60 || pos.Y != 20 {
		t.Fatalf("position = %+v", pos)
	}
	if !strings.Contains(pos.Label(), "approximate") {
		t.Errorf("label = %q", pos.Label())
	}
	x, y := 10.0, 12.5
	c.Location = &Location{Zone: "Testzone", X: &x, Y: &y}
	if p := c.LastKnownPosition(); p.Approximate || strings.Contains(p.Label(), "approximate") {
		t.Errorf("saved location should be exact: %+v", p)
	}
}

func TestNearbyGiversNearestFirst(t *testing.T) {
	c := scout(t)
	now := time.Unix(1700100000, 0)
	gs := c.NearbyGivers(now)
	// Gossip, other-map and coordinate-less NPCs are skipped.
	if len(gs) != 2 || gs[0].ID != 9002 || gs[1].ID != 9001 {
		t.Fatalf("givers = %+v", gs)
	}
	if !gs[0].Recent || gs[1].Recent {
		t.Errorf("recent flags = %v %v", gs[0].Recent, gs[1].Recent)
	}
}

func TestNoNPCsTable(t *testing.T) {
	c := fixture(t, "level30_caster.lua").Characters[0]
	if c.NPCs != nil || c.LastKnownPosition() != nil || c.NearbyGivers(time.Now()) != nil {
		t.Errorf("expected no npc data: %+v", c.NPCs)
	}
}

func TestObjectiveProgressPrintsCountOnce(t *testing.T) {
	obj := func(text string, have, need int) Objective { return Objective{Text: text, Have: &have, Need: &need} }
	tests := []struct {
		o    Objective
		want string
	}{
		{obj("0/1 Find Test Scout", 0, 1), "0/1 Find Test Scout"},
		{obj("Test Claws: 3/8", 3, 8), "3/8 Test Claws"},
		{obj("Test Claws", 3, 8), "3/8 Test Claws"},
	}
	for _, tc := range tests {
		q := Quest{Objectives: []Objective{tc.o}}
		if got := q.ObjectiveProgress(); got != tc.want {
			t.Errorf("%q = %q, want %q", tc.o.Text, got, tc.want)
		}
	}
}

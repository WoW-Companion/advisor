package talents

import (
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
)

func bp(b bool) *bool { return &b }

func warrior(unspent int) character.Character {
	return character.Character{Class: "Warrior", Talents: &character.Talents{Unspent: &unspent, Trees: []character.TalentTree{{
		Name: "Test Warrior", Talents: []character.Talent{
			{Name: "Test Rampage", Group: "Test Fury", Rank: 1, MaxRank: ip(1)},
			{Name: "Test Whirl", Group: "Test Fury", Rank: 1, MaxRank: ip(5), SpellID: ip(77)},
			{Name: "Test Slam", Group: "Test Arms", Rank: 0, MaxRank: ip(2), CanBuy: bp(true)},
			{Name: "Test Locked", Group: "Test Arms", Rank: 0, MaxRank: ip(1), CanBuy: bp(false)},
		}}}}}
}

func plan(picks ...character.PlanPick) *character.TalentPlan {
	return &character.TalentPlan{Spec: "Fury", GuideURL: "https://www.icy-veins.com/wow-forever/fury", Date: "2026-11-22", Picks: picks}
}

func TestGuideNext(t *testing.T) {
	pk := func(n string, r int) character.PlanPick { return character.PlanPick{Node: n, Rank: r} }
	cases := []struct {
		name    string
		plan    *character.TalentPlan
		profile *character.Profile
		n       int
		want    string
		missing int
		stale   bool
		done    bool
	}{
		{"rank advances by ranks held", plan(pk("Test Rampage", 1), pk("Test Whirl", 3)), nil, 5, "Test Whirl (ranks 2-3/5) in Test Fury", 0, false, false},
		{"limited to unspent", plan(pk("Test Whirl", 5)), nil, 2, "Test Whirl (ranks 2-3/5) in Test Fury", 0, false, false},
		{"plan order, next node after", plan(pk("Test Slam", 1), pk("Test Whirl", 2)), nil, 2, "Test Slam (rank 1/2) in Test Arms, then Test Whirl (rank 2/5) in Test Fury", 0, false, false},
		{"after spending the point the pick advances", plan(pk("Test Rampage", 1), pk("Test Whirl", 1), pk("Test Slam", 2)), nil, 1, "Test Slam (rank 1/2) in Test Arms", 0, false, false},
		{"node not in the tree", plan(pk("Test Gone", 1), pk("Test Slam", 1)), nil, 3, "Test Slam (rank 1/2) in Test Arms", 1, false, false},
		{"cannot-buy node skipped", plan(pk("Test Locked", 1), pk("Test Slam", 1)), nil, 3, "Test Slam (rank 1/2) in Test Arms", 0, false, false},
		{"stale after a spec change", plan(pk("Test Slam", 1)), &character.Profile{Role: "damage", Focus: "mixed", Spec: "Arms"}, 3, "", 0, true, false},
		{"same spec, any case", plan(pk("Test Slam", 1)), &character.Profile{Role: "damage", Focus: "mixed", Spec: "fury"}, 3, "Test Slam (rank 1/2) in Test Arms", 0, false, false},
		{"all done", plan(pk("Test Rampage", 1), pk("Test Whirl", 1)), nil, 3, "", 0, false, true},
	}
	for _, tc := range cases {
		c := warrior(2)
		c.Plan, c.Profile = tc.plan, tc.profile
		r, ok := GuideNext(c, tc.n)
		if !ok {
			t.Errorf("%s: no result", tc.name)
			continue
		}
		if got := r.Describe(); got != tc.want || len(r.Missing) != tc.missing || r.Stale != tc.stale || r.Done != tc.done {
			t.Errorf("%s: %q missing %v stale %v done %v", tc.name, got, r.Missing, r.Stale, r.Done)
		}
	}
	if _, ok := GuideNext(warrior(2), 3); ok {
		t.Error("a character with no plan has guide picks")
	}
}

func TestGuideNotesAndSource(t *testing.T) {
	c := warrior(1)
	c.Plan = plan(character.PlanPick{Node: "Test Gone", Rank: 1}, character.PlanPick{Node: "Test Locked", Rank: 1}, character.PlanPick{Node: "Test Slam", Rank: 1})
	r, _ := GuideNext(c, 1)
	notes := strings.Join(r.Notes(), "|")
	for _, want := range []string{"not in the captured tree (Test Gone)", "ask the AI once to adapt", "cannot be bought yet"} {
		if !strings.Contains(notes, want) {
			t.Errorf("notes lack %q: %s", want, notes)
		}
	}
	if got := r.Source(); got != "https://www.icy-veins.com/wow-forever/fury Fury build" {
		t.Errorf("Source = %q", got)
	}
	if got := RenderTree(c); !strings.Contains(got, "- Test Whirl [Test Fury] rank 1/5 spell 77") || strings.Contains(got, "Tailoring") {
		t.Errorf("RenderTree:\n%s", got)
	}
}

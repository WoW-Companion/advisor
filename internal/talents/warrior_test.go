package talents

import (
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/store"
)

// warriorData is a Fury (damage) and a Protection (tank) warrior build; node ids 11 Test Rampage,
// 12 Test Bloodthirst, 13 Test Shield Wall.
func warriorData(t *testing.T) store.DataStore {
	return store.New(store.ReaderSource{
		"talent_builds": table(t, append(append(
			pts("Warrior", "Fury", 1, 10, 11, "Test Rampage", 2),
			pts("Warrior", "Fury", 3, 12, 12, "Test Bloodthirst", 3)...),
			pts("Warrior", "Protection", 1, 10, 13, "Test Shield Wall", 1)...)),
		"talent_priorities": table(t, []priorityRow{{"Warrior", "Fury", 1, 11, "Test Rampage", "Rampage is your core damage button."}}),
		"talent_nodes":      table(t, []nodeRow{{11, "Test Rampage"}, {12, "Test Bloodthirst"}, {13, "Test Shield Wall"}}),
	})
}

func warrior(level, spent int, p *character.Profile) character.Character {
	yes, two := true, 2
	return character.Character{Class: "WARRIOR", Level: ip(level), Profile: p, Talents: &character.Talents{Unspent: ip(1), Trees: []character.TalentTree{{
		Groups: []character.TalentGroup{{Name: "Fury", Spent: spent}, {Name: "Protection"}},
		Talents: []character.Talent{
			{Name: "Test Rampage", Rank: spent, NodeID: ip2(11), MaxRank: &two, CanBuy: &yes},
			{Name: "Test Bloodthirst", Rank: 0, NodeID: ip2(12), CanBuy: &yes},
		},
	}}}}
}

func ip2(n int) *int { return &n }

func TestWarriorNextPick(t *testing.T) {
	ds := warriorData(t)
	fury := &character.Profile{Role: "damage", Focus: "leveling", Spec: "Fury"}
	p := Plan(warrior(14, 1, fury), ds, Options{})
	if p.Spec != "Fury" || len(p.Next) == 0 || p.Next[0].Name != "Test Rampage" || p.Next[0].Rank != 2 || p.Next[0].Rationale != "Rampage is your core damage button." {
		t.Fatalf("level 14 Fury with 1 point: %+v", p)
	}
	if got := points(p); got != "Test Rampage=2,Test Bloodthirst=1,Test Bloodthirst=2" {
		t.Errorf("band picks = %s", got)
	}
}

func TestSpecFromRole(t *testing.T) {
	ds := warriorData(t)
	// The spec is unset: the one tank spec matches the stored tank role.
	p := Plan(warrior(10, 0, &character.Profile{Role: "tank", Focus: "dungeons"}), ds, Options{})
	if p.Spec != "Protection" || len(p.Next) != 1 || p.Next[0].Name != "Test Shield Wall" {
		t.Errorf("tank role: %+v", p)
	}
	// The stored spec wins over the role.
	p = Plan(warrior(10, 0, &character.Profile{Role: "tank", Focus: "dungeons", Spec: "Fury"}), ds, Options{})
	if p.Spec != "Fury" {
		t.Errorf("stored spec: %q", p.Spec)
	}
}

func TestNoBuildGivesOptions(t *testing.T) {
	c := warrior(10, 0, &character.Profile{Role: "damage", Focus: "leveling"})
	c.Class = "HUNTER"
	out := Render(Plan(c, warriorData(t), Options{}))
	for _, want := range []string{"no talent builds for HUNTER", "options, not a recommendation", "Test Rampage (rank 1)", "Test Bloodthirst (rank 1)"} {
		if !strings.Contains(out, want) {
			t.Errorf("render lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(strings.ToLower(out), "fetch") {
		t.Errorf("a fetch is mentioned:\n%s", out)
	}
}

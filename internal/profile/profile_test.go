package profile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
)

func ip(n int) *int { return &n }

func tree() character.Character {
	return character.Character{Key: "Realm:Testwar", Class: "Warrior", Talents: &character.Talents{Trees: []character.TalentTree{
		{Name: "Test Warrior", Talents: []character.Talent{
			{Name: "Test Rampage", Group: "Test Fury", MaxRank: ip(1)},
			{Name: "Test Bloodthirst", Group: "Test Fury", MaxRank: ip(3)},
		}},
		{Name: "Test Tailoring", Key: "professions", Talents: []character.Talent{{Name: "Test Stitch", MaxRank: ip(2)}}},
	}}}
}

func TestRoundTripAndAttach(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "profiles.json")
	p, err := Clean(character.Profile{Role: " Damage ", Focus: "LEVELING", Spec: "Fury"})
	if err != nil || p.Role != "damage" || p.Focus != "leveling" {
		t.Fatalf("Clean = %+v, %v", p, err)
	}
	if _, err := Update(path, "Realm:Testwar", func(e *Entry) error { e.Profile = &p; return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := Update(path, "Realm:Other", func(e *Entry) error { e.Profile = &character.Profile{Role: "tank", Focus: "pvp"}; return nil }); err != nil {
		t.Fatal(err)
	}
	c := Attach(path, tree())
	if c.Profile == nil || *c.Profile != p {
		t.Errorf("attached %+v, want %+v", c.Profile, p)
	}
	if got := Attach(path, character.Character{Key: "Realm:Nobody"}); got.Profile != nil || got.Plan != nil {
		t.Errorf("a character with no entry got %+v", got.Profile)
	}
	if fi, err := os.Stat(path); err != nil || runtimeNotWorldReadable(fi) {
		t.Errorf("store mode: %v %v", fi, err)
	}
	// A file that cannot be parsed is reported and never overwritten.
	os.WriteFile(path, []byte("{broken"), 0o600)
	if _, err := Update(path, "k", func(*Entry) error { return nil }); err == nil {
		t.Error("a corrupt store was overwritten")
	}
	if b, _ := os.ReadFile(path); string(b) != "{broken" {
		t.Errorf("store changed to %q", b)
	}
}

func runtimeNotWorldReadable(fi os.FileInfo) bool {
	return fi.Mode().Perm()&0o077 != 0 && os.PathSeparator == '/'
}

func TestClean(t *testing.T) {
	cases := []struct {
		name string
		in   character.Profile
		ok   bool
	}{
		{"good", character.Profile{Role: "tank", Focus: "dungeons"}, true},
		{"guide", character.Profile{Role: "healer", Focus: "mixed", GuideURL: "https://www.icy-veins.com/wow-forever/x"}, true},
		{"bad role", character.Profile{Role: "dps", Focus: "pvp"}, false},
		{"no focus", character.Profile{Role: "tank"}, false},
		{"http guide", character.Profile{Role: "tank", Focus: "pvp", GuideURL: "http://example.com"}, false},
		{"not a url", character.Profile{Role: "tank", Focus: "pvp", GuideURL: "javascript:alert(1)"}, false},
	}
	for _, tc := range cases {
		if _, err := Clean(tc.in); (err == nil) != tc.ok {
			t.Errorf("%s: err = %v", tc.name, err)
		}
	}
}

func TestValidatePlan(t *testing.T) {
	good := character.TalentPlan{Spec: "Fury", GuideURL: "https://www.icy-veins.com/wow-forever/fury", Date: "2026-11-22"}
	cases := []struct {
		name    string
		picks   []character.PlanPick
		want    string // kept picks as node:rank
		dropped int
		fail    bool
	}{
		{"all known, spelling fixed", []character.PlanPick{{Node: "test rampage", Rank: 1}, {Node: "Test Bloodthirst", Rank: 2}}, "Test Rampage:1,Test Bloodthirst:2", 0, false},
		{"unknown node dropped", []character.PlanPick{{Node: "Invented Talent", Rank: 1}, {Node: "Test Rampage", Rank: 1}}, "Test Rampage:1", 1, false},
		{"rank past max dropped", []character.PlanPick{{Node: "Test Rampage", Rank: 2}, {Node: "Test Bloodthirst", Rank: 3}}, "Test Bloodthirst:3", 1, false},
		{"zero rank dropped", []character.PlanPick{{Node: "Test Rampage", Rank: 0}, {Node: "Test Bloodthirst", Rank: 1}}, "Test Bloodthirst:1", 1, false},
		{"repeat keeps first place, highest rank", []character.PlanPick{{Node: "Test Bloodthirst", Rank: 1}, {Node: "Test Rampage", Rank: 1}, {Node: "Test Bloodthirst", Rank: 3}}, "Test Bloodthirst:3,Test Rampage:1", 0, false},
		{"profession node is not a class node", []character.PlanPick{{Node: "Test Stitch", Rank: 1}}, "", 1, true},
		{"nothing known", []character.PlanPick{{Node: "Nope", Rank: 1}}, "", 1, true},
		{"empty", nil, "", 0, true},
	}
	for _, tc := range cases {
		p := good
		p.Picks = tc.picks
		chk, err := ValidatePlan(tree(), p)
		if (err != nil) != tc.fail {
			t.Errorf("%s: err = %v", tc.name, err)
			continue
		}
		var got []string
		for _, pk := range chk.Plan.Picks {
			got = append(got, pk.Node+":"+string(rune('0'+pk.Rank)))
		}
		if !tc.fail && (strings.Join(got, ",") != tc.want || len(chk.Dropped) != tc.dropped) {
			t.Errorf("%s: kept %v dropped %v", tc.name, got, chk.Dropped)
		}
	}
	for name, mut := range map[string]func(*character.TalentPlan){
		"no guide":   func(p *character.TalentPlan) { p.GuideURL = "" },
		"http guide": func(p *character.TalentPlan) { p.GuideURL = "http://www.icy-veins.com/x" },
		"no spec":    func(p *character.TalentPlan) { p.Spec = "" },
	} {
		p := good
		p.Picks = []character.PlanPick{{Node: "Test Rampage", Rank: 1}}
		mut(&p)
		if _, err := ValidatePlan(tree(), p); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// Guide text is never stored and nothing the profile store writes lands in a repo: the store sits in
// the user's config directory, and what it holds is the profile and node names and ranks only.
func TestNothingFromAGuideIsWrittenUnderTheRepo(t *testing.T) {
	t.Setenv(Env, "")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("AppData", t.TempDir())
	wd, _ := os.Getwd()
	repo := wd
	for {
		if _, err := os.Stat(filepath.Join(repo, "go.mod")); err == nil {
			break
		}
		if up := filepath.Dir(repo); up != repo {
			repo = up
			continue
		}
		t.Fatal("no go.mod above the test")
	}
	if p := Path(); p == "" || strings.HasPrefix(p, repo+string(filepath.Separator)) {
		t.Fatalf("the store path %q is inside the repo %q", p, repo)
	}
	path := filepath.Join(t.TempDir(), "profiles.json")
	chk, err := ValidatePlan(tree(), character.TalentPlan{Spec: "Fury", GuideURL: "https://www.icy-veins.com/wow-forever/fury", Date: "2026-11-22",
		Picks: []character.PlanPick{{Node: "Test Rampage", Rank: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	Update(path, "Realm:Testwar", func(e *Entry) error {
		e.Profile = &character.Profile{Role: "damage", Focus: "leveling"}
		e.Plan = &chk.Plan
		return nil
	})
	raw, _ := os.ReadFile(path)
	var doc map[string]map[string]map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	plan, _ := doc["characters"]["Realm:Testwar"]["plan"].(map[string]any)
	for k := range plan {
		if k != "spec" && k != "guide_url" && k != "date" && k != "picks" {
			t.Errorf("the plan stores %q, which is not a node list, URL or date", k)
		}
	}
	for _, pk := range plan["picks"].([]any) {
		if len(pk.(map[string]any)) != 2 {
			t.Errorf("a pick holds more than a node and a rank: %v", pk)
		}
	}
	if before, _ := os.ReadDir(repo); len(before) == 0 {
		t.Skip("repo not readable")
	}
	if _, err := os.Stat(filepath.Join(repo, "profiles.json")); err == nil {
		t.Error("a store file landed at the repo root")
	}
}

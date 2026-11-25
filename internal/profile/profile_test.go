package profile

import (
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
	if got := Attach(path, character.Character{Key: "Realm:Nobody"}); got.Profile != nil {
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
		{"bad role", character.Profile{Role: "dps", Focus: "pvp"}, false},
		{"no focus", character.Profile{Role: "tank"}, true}, // "Not set" is a real choice
		{"neither", character.Profile{}, true},
		{"bad focus", character.Profile{Role: "tank", Focus: "x"}, false},
	}
	for _, tc := range cases {
		if _, err := Clean(tc.in); (err == nil) != tc.ok {
			t.Errorf("%s: err = %v", tc.name, err)
		}
	}
}

// Nothing the profile store writes lands in a repo: the store sits in the user's config directory.
func TestNothingIsWrittenUnderTheRepo(t *testing.T) {
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
	Update(path, "Realm:Testwar", func(e *Entry) error {
		e.Profile = &character.Profile{Role: "damage", Focus: "leveling"}
		return nil
	})
	if before, _ := os.ReadDir(repo); len(before) == 0 {
		t.Skip("repo not readable")
	}
	if _, err := os.Stat(filepath.Join(repo, "profiles.json")); err == nil {
		t.Error("a store file landed at the repo root")
	}
}

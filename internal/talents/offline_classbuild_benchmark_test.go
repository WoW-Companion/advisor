package talents

import (
	"fmt"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/store"
)

// The offline class build benchmark: how the advisor reads the spec a player names or the stored role, which points it
// calls due at a level from 10 to 39, and what it says when the dataset has no answer, beside the rows of the build
// table. The dataset holds no verified Icy Veins build yet (every row is "curated (unverified)"), so the build is the
// snapshot's real nodes and band order written as one row per point in the per-point schema (see
// offline_benchmark_test.go): point i is spent at level 9+i. A difference whose cause is a missing dataset row is
// marked "(data)". make offline-benchmark runs it; WOWC_BENCH_REPORT receives the tables.

var classSpecs = [][2]string{
	{"Priest", "Shadow Magic"}, {"Druid", "Feral Combat"}, {"Warrior", "Protection"}, {"Warrior", "Arms"}, {"Hunter", "Beast Mastery"},
	{"Shaman", "Restoration"}, {"Paladin", "Holy"}, {"Mage", "Fire"},
}

// classStore is built once: the per-point rows of classSpecs over the snapshot.
func classStore(t *testing.T) (ds store.DataStore, built map[string][]point) {
	t.Helper()
	d, b := checkedStore(t, classSpecs)
	return d, b
}

// TestOfflineBenchmarkSpecNames: a spec the player names in a few letters ("shadow", "feral", "prot") is the one
// spec of the class whose name starts with it, in the dataset's build table; the full name and the name with the
// class after it are the same spec.
func TestOfflineBenchmarkSpecNames(t *testing.T) {
	ds, built := classStore(t)
	level := 20
	cases := []struct{ class, typed, want string }{
		{"Priest", "Shadow", "Shadow Magic"}, {"Druid", "Feral", "Feral Combat"}, {"Warrior", "Prot", "Protection"},
		{"Hunter", "Beast", "Beast Mastery"}, {"Shaman", "Resto", "Restoration"}, {"Paladin", "holy", "Holy"},
		{"Warrior", "Arms warrior", "Arms"}, {"Priest", "Shadow Magic", "Shadow Magic"},
	}
	var out []benchRow
	for _, tc := range cases {
		p := Plan(character.Character{Class: classToken(tc.class), Level: &level}, ds, Options{Spec: tc.typed, Limit: 1})
		var first string
		if pts := built[tc.want]; len(pts) > 0 {
			first = pts[0].String()
		}
		got := p.Spec + ": " + points(p)
		if p.NoBuild {
			got = p.Missing
		}
		res := "match"
		if p.Spec != tc.want || len(p.Next) != 1 || p.Next[0].Name+" 1 (level 10)" != first {
			res = "differ"
		}
		out = append(out, benchRow{fmt.Sprintf("%s: spec typed as %q", tc.class, tc.typed), got, tc.want + " build, first point " + first, fixtureLabel, res})
	}
	finishBench(t, "talents-spec-names", "Offline talent builds: the spec a player names", out)
}

// TestOfflineBenchmarkBuildLevels: the points the advisor calls due at a level, with none taken, are every build row at
// or below that level (one point a level from 10), and a character below level 10 is told the build starts at 10.
func TestOfflineBenchmarkBuildLevels(t *testing.T) {
	ds, built := classStore(t)
	var out []benchRow
	cases := []struct {
		class, spec string
		level       int
	}{
		{"Warrior", "Arms", 5}, {"Warrior", "Arms", 9}, {"Warrior", "Arms", 10}, {"Hunter", "Beast Mastery", 15},
		{"Mage", "Fire", 24}, {"Priest", "Shadow Magic", 31}, {"Druid", "Feral Combat", 39}, {"Warrior", "Protection", 12},
	}
	for _, tc := range cases {
		level := tc.level
		p := Plan(character.Character{Class: classToken(tc.class), Level: &level}, ds, Options{Spec: tc.spec, Limit: 3})
		due := 0
		var want []string
		for _, pt := range built[tc.spec] {
			if pt.level <= tc.level {
				due++
				if len(want) < 3 {
					want = append(want, pt.String())
				}
			}
		}
		var got []string
		for _, pt := range p.Next {
			got = append(got, fmt.Sprintf("%s %d (level %d)", pt.Name, pt.Rank, pt.Level))
		}
		res := "match"
		if p.Remaining != due || strings.Join(got, "; ") != strings.Join(want, "; ") {
			res = "differ"
		}
		advisor := fmt.Sprintf("%d due: %s", p.Remaining, strings.Join(got, "; "))
		if due == 0 {
			advisor = "0 due: " + strings.Join(p.Notes, " ")
			first := built[tc.spec][0].level
			if !strings.Contains(advisor, fmt.Sprintf("starts at level %d", first)) {
				res = "differ"
			}
		}
		out = append(out, benchRow{fmt.Sprintf("%s %s at level %d, none taken: points due", tc.class, tc.spec, tc.level), advisor,
			fmt.Sprintf("%d rows at level %d or below (first row level %d)", due, tc.level, built[tc.spec][0].level), fixtureLabel, res})
	}
	finishBench(t, "talents-build-levels", "Offline talent builds: the points due at a level", out)
}

// TestOfflineBenchmarkSpecByRole: the spec the stored role names is the class's spec with that role in the build
// table: the tank and healer specs by name, and the damage spec the class has.
func TestOfflineBenchmarkSpecByRole(t *testing.T) {
	ds, _ := classStore(t)
	level := 20
	cases := []struct{ class, role, want string }{
		{"Warrior", "tank", "Protection"}, {"Warrior", "damage", "Arms"}, {"Shaman", "healer", "Restoration"},
		{"Paladin", "healer", "Holy"}, {"Mage", "damage", "Fire"}, {"Hunter", "damage", "Beast Mastery"},
	}
	var out []benchRow
	for _, tc := range cases {
		c := character.Character{Class: classToken(tc.class), Level: &level, Profile: &character.Profile{Role: tc.role}}
		p := Plan(c, ds, Options{})
		res := "match"
		if p.Spec != tc.want || p.NoBuild {
			res = "differ"
		}
		out = append(out, benchRow{fmt.Sprintf("%s with role %s: the spec", tc.class, tc.role), p.Spec, tc.want + " (the one " + tc.role + " spec of the class in the build table)", fixtureLabel, res})
	}
	finishBench(t, "talents-spec-role", "Offline talent builds: the spec the stored role names", out)
}

// TestOfflineBenchmarkClassBuildNoAnswer: what the dataset cannot answer, the advisor says so and names no talent: a class
// with no rows, a spec with no build, a level not captured, and no dataset at all.
func TestOfflineBenchmarkClassBuildNoAnswer(t *testing.T) {
	ds, _ := classStore(t)
	level := 20
	var out []benchRow
	p := Plan(character.Character{Class: "MONK", Level: &level}, ds, Options{})
	out = append(out, benchRow{"monk: a talent build", p.Missing, "no talent_builds row for the class", talentSource, resultIf(p.Missing != "" && len(p.Next) == 0 && !strings.Contains(p.Missing, "checked build for "), "a pick or a spec was named")})
	p = Plan(character.Character{Class: "WARRIOR", Level: &level}, ds, Options{Spec: "Blood"})
	out = append(out, benchRow{"warrior Blood: a talent build", p.Missing, "no Blood spec in talent_builds (Arms, Fury, Protection)", talentSource, resultIf(p.NoBuild && len(p.Next) == 0 && strings.Contains(p.Missing, "checked build for Blood"), "a pick was named for a spec the dataset lacks")})
	p = Plan(character.Character{Class: "WARRIOR"}, ds, Options{Spec: "Arms"})
	out = append(out, benchRow{"warrior Arms with no level captured: points due", p.Missing, "points are due by level; the capture has none", talentSource, resultIf(strings.Contains(p.Missing, "Level not captured") && len(p.Next) == 0, "a pick was named with no level")})
	p = Plan(character.Character{Class: "WARRIOR", Level: &level}, nil, Options{Spec: "Arms"})
	out = append(out, benchRow{"warrior Arms with no dataset: a talent build", p.Missing, "no talent_builds table", talentSource, resultIf(strings.Contains(p.Missing, "No dataset") && len(p.Next) == 0, "a pick was named with no dataset")})
	finishBench(t, "talents-class-no-answer", "Offline talent builds: no answer in the dataset", out)
}

func resultIf(ok bool, bad string) string {
	if ok {
		return "match"
	}
	return "differ: " + bad
}

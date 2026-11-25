package stats

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/benchsummary"
	"github.com/WoW-Companion/advisor/internal/store"
)

// The stat benchmark: for every class and spec the advice handles, the stat order and the suffixes it
// names are compared with a reference stated apart from the profile table, and every suffix is
// compared with the dataset's random_suffixes. It runs on the real dataset when WOWC_BENCH_DATASET names
// a dataset directory (make advice-benchmark); WOWC_BENCH_REPORT, when set, is a file that receives the
// result table in Markdown. Without a dataset the suffix rows are skipped and the reference rows still run.

// refSuffixes is, per class, spec and role, the suffixes a player of that build is happy to find on
// world-drop gear: the pairs that carry the build's primary stat and a second stat it uses. Source: the
// Classic-era convention for suffix choice (the pair is named by its stats in the dataset's random_suffixes).
var refSuffixes = map[string][]string{
	"WARRIOR Fury":              {"tiger", "bear"},
	"WARRIOR Arms":              {"tiger", "bear"},
	"WARRIOR Protection":        {"bear", "monkey"},
	"PALADIN Retribution":       {"bear", "gorilla"},
	"PALADIN Protection":        {"bear", "eagle"},
	"PALADIN Holy":              {"eagle", "owl"},
	"HUNTER Marksmanship":       {"monkey", "falcon"},
	"HUNTER Beast Mastery":      {"monkey", "falcon"},
	"HUNTER Survival":           {"monkey", "falcon"},
	"ROGUE Combat":              {"tiger", "monkey"},
	"ROGUE Assassination":       {"tiger", "monkey"},
	"ROGUE Subtlety":            {"tiger", "monkey"},
	"PRIEST Holy":               {"owl", "eagle"},
	"PRIEST Discipline":         {"owl", "eagle"},
	"PRIEST Shadow":             {"eagle", "owl"},
	"SHAMAN Enhancement":        {"tiger", "bear"},
	"SHAMAN Elemental":          {"eagle", "owl"},
	"SHAMAN Restoration":        {"eagle", "owl"},
	"MAGE Fire":                 {"eagle", "owl"},
	"MAGE Frost":                {"eagle", "owl"},
	"MAGE Arcane":               {"eagle", "owl"},
	"WARLOCK Affliction":        {"eagle"},
	"WARLOCK Destruction":       {"eagle"},
	"WARLOCK Demonology":        {"eagle"},
	"DRUID Balance":             {"eagle", "owl"},
	"DRUID Feral Combat damage": {"tiger", "monkey", "bear"},
	"DRUID Feral Combat tank":   {"monkey", "bear"},
	"DRUID Restoration":         {"owl", "eagle"},
}

// refLead is the primary stat of each class and role: the stat that turns into the build's power.
func refLead(p Profile) string {
	switch {
	case p.Role == RoleTank:
		return Stamina
	case p.Role == RoleHealer || p.Class == "MAGE" || p.Class == "WARLOCK" || p.Arch == archCaster:
		return Intellect
	case p.Class == "HUNTER" || p.Class == "ROGUE" || p.Arch == archFeral:
		return Agility
	}
	return Strength
}

func refKey(p Profile) string {
	k := p.Class + " " + p.Spec
	if p.Class == "DRUID" && p.Spec == "Feral Combat" {
		k += " " + p.Role
	}
	return k
}

type benchRow struct{ check, advisor, reference, source, result string }

func (r benchRow) line() string {
	return fmt.Sprintf("| %s | %s | %s | %s | %s |", r.check, r.advisor, r.reference, r.source, r.result)
}

func benchDataset(t *testing.T) store.DataStore {
	dir := os.Getenv("WOWC_BENCH_DATASET")
	if dir == "" {
		return nil
	}
	return store.NewDir(dir)
}

func TestStatBenchmark(t *testing.T) {
	ds := benchDataset(t)
	var rows []store.Row
	if ds != nil {
		rows = SuffixRows(ds)
		if len(rows) == 0 {
			t.Fatal("the dataset has no random_suffixes rows")
		}
	}
	var table []benchRow
	fail := func(format string, a ...any) { t.Errorf(format, a...) }

	for _, p := range table0() {
		key := refKey(p)
		pr := Prof{Profile: p, Weights: Adjust(p.Weights, PrefBalanced)}
		order := pr.Order()
		lead := ""
		if len(order) > 0 {
			lead = order[0]
		}
		res := "match"
		if lead != refLead(p) {
			res = "differ"
			fail("%s: stat order leads with %s, the reference is %s", key, lead, refLead(p))
		}
		table = append(table, benchRow{key + ": stat order", strings.Join(order, " > "), refLead(p) + " first",
			"class and role mechanics (primary stat of the build)", res})

		var top []Suffix
		if rows != nil {
			top = pr.VerifiedTop(rows, 2)
		} else {
			top = pr.RankSuffixes()
			if len(top) > 2 {
				top = top[:2]
			}
		}
		var names, keys []string
		for _, s := range top {
			names = append(names, s.Name)
			keys = append(keys, s.Key)
		}
		want := refSuffixes[key]
		if len(want) == 0 {
			fail("%s: no reference suffixes in the benchmark", key)
			continue
		}
		res = "match"
		if len(keys) == 0 || !contains(want, keys[0]) {
			res = "differ"
			fail("%s: top suffix %v, the reference accepts %v", key, keys, want)
		}
		table = append(table, benchRow{key + ": best suffix", strings.Join(names, ", "), strings.Join(want, " or "),
			"suffix convention; stats from random_suffixes", res})

		// every stat of a named suffix must be one the build uses
		used := map[string]bool{}
		for _, s := range order {
			used[s] = true
		}
		res = "match"
		for _, s := range top {
			for _, id := range s.Stats {
				if !used[StatNames[id]] {
					res = "differ"
					fail("%s: suffix %s carries %s, which the build does not use", key, s.Name, StatNames[id])
				}
			}
		}
		table = append(table, benchRow{key + ": suffix stats are used by the build", strings.Join(names, ", "), "every stat in the priority",
			"stats in the profile's order", res})
	}

	if rows != nil {
		for _, s := range Suffixes {
			res := "match"
			if !Verified(s, rows) {
				res = "differ"
				fail("suffix %s (%s) is not in random_suffixes with those stats", s.Name, s.Words)
			}
			table = append(table, benchRow{"suffix " + s.Name + ": stats", s.Words, statWords(rows, s.Name), "dataset random_suffixes (kind suffix)", res})
		}
	}
	results := make([]string, len(table))
	for i, r := range table {
		results[i] = r.result
	}
	fmt.Println(benchsummary.AreaLine("stats", results))
	writeBenchTable(t, "Stat advice", table)
}

func table0() []Profile { return append([]Profile(nil), table...) }

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

// statWords is the stats the dataset gives a suffix name, "Intellect + Spirit" (one entry per distinct set).
func statWords(rows []store.Row, name string) string {
	seen := map[string]bool{}
	var out []string
	for _, set := range suffixSets(rows, name) {
		var parts []string
		for id := range set {
			parts = append(parts, Name(StatNames[id]))
		}
		sort.Strings(parts)
		if w := strings.Join(parts, " + "); !seen[w] {
			seen[w] = true
			out = append(out, w)
		}
	}
	sort.Strings(out)
	return strings.Join(out, " / ")
}

func writeBenchTable(t *testing.T, title string, rows []benchRow) {
	path := os.Getenv("WOWC_BENCH_REPORT")
	if path == "" || len(rows) == 0 {
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "### %s\n\n| Check | Advisor | Reference | Source | Result |\n| --- | --- | --- | --- | --- |\n", title)
	for _, r := range rows {
		b.WriteString(r.line() + "\n")
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(b.String() + "\n"); err != nil {
		t.Fatal(err)
	}
}

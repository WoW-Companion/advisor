package professions

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/store"
)

// The offline route benchmark: the advisor's skill-up answer (RouteFacts) for a character at a profession and
// skill, set beside the dataset's profession_routes rows for the same range, read straight from the snapshot
// table (testdata/advice_snapshot, scripts/advice-snapshot.sh) and never through the advisor's reader. The
// reference reads a row as the dataset publishes it: `materials` holds what one craft uses, as a list of {item_id, name, count} that the
// store returns column by column (all the ids, then all the names, then all the counts). Run it with
// make offline-benchmark; WOWC_BENCH_REPORT receives the result tables in Markdown.

const routesSource = "dataset profession_routes (WoW Forever guides)"

// refStep is a profession_routes row as the dataset holds it.
type refStep struct {
	step, option, from, to, crafts, alt int
	recipe, learned, notes              string
	where                               []string
	mats                                []refMat
}

type refMat struct {
	id    int64
	name  string
	count int
}

func strList(v any) []string {
	var out []string
	l, _ := v.([]any)
	for _, x := range l {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// refMaterials reads the store's column-by-column list: n ids, n names, n counts.
func refMaterials(v any) []refMat {
	l, _ := v.([]any)
	n := len(l) / 3
	var out []refMat
	for i := 0; i < n; i++ {
		id, _ := l[i].(int64)
		name, _ := l[n+i].(string)
		c, _ := l[2*n+i].(int64)
		out = append(out, refMat{id, name, int(c)})
	}
	return out
}

// refRoute reads every row of a profession's route straight from the table, main steps and alternatives.
func refRoute(t *testing.T, ds store.DataStore, prof string) (main []refStep, alts map[int][]refStep) {
	t.Helper()
	rows, err := ds.Filter("profession_routes", store.Range{Column: "profession", Min: prof, Max: prof})
	if err != nil || len(rows) == 0 {
		t.Fatalf("%s: no route rows: %v", prof, err)
	}
	alts = map[int][]refStep{}
	for _, r := range rows {
		s := refStep{step: int(asI64(r["step"])), option: int(asI64(r["option"])), from: int(asI64(r["skill_from"])), to: int(asI64(r["skill_to"])),
			crafts: int(asI64(r["crafts"])), alt: int(asI64(r["alternative_of"])), recipe: asText(r["recipe_name"]), learned: asText(r["learned_from"]),
			notes: asText(r["notes"]), where: strList(r["where"]), mats: refMaterials(r["materials"])}
		if s.option == 0 {
			main = append(main, s)
		} else {
			alts[s.alt] = append(alts[s.alt], s)
		}
	}
	sort.SliceStable(main, func(i, j int) bool { return main[i].from < main[j].from })
	return main, alts
}

// left is the crafts still to make from a skill inside a step: the step's crafts scaled by the skill range
// left, rounded up.
func (s refStep) left(skill int) int {
	if skill <= s.from {
		return s.crafts
	}
	return (s.crafts*(s.to-skill) + (s.to - s.from) - 1) / (s.to - s.from)
}

// routeChar is a character with a profession at a skill.
func routeChar(prof string, skill int) character.Character {
	rank, mx, lvl := skill, 300, 60
	return character.Character{Name: "Route", Class: "MAGE", Faction: "Horde", Level: &lvl,
		Professions: []character.Skill{{Name: prof, Rank: &rank, Max: &mx}}}
}

// routeCases are the characters whose skill-up answer is checked.
var routeCases = []struct {
	prof  string
	skill int
}{
	{"Tailoring", 1}, {"Tailoring", 45}, {"Tailoring", 216}, {"Tailoring", 255}, {"Alchemy", 20}, {"Alchemy", 140},
	{"Blacksmithing", 130}, {"Enchanting", 60}, {"Cooking", 50}, {"First Aid", 150}, {"Engineering", 80}, {"Leatherworking", 200},
	{"Herbalism", 20}, {"Herbalism", 120}, {"Mining", 30}, {"Mining", 90}, {"Skinning", 10}, {"Fishing", 60},
}

func TestOfflineBenchmarkRouteAnswers(t *testing.T) {
	ds := snapshot().ds
	var out []benchRow
	for _, rc := range routeCases {
		main, alts := refRoute(t, ds, rc.prof)
		i := -1
		for k, s := range main {
			if rc.skill >= s.from && rc.skill < s.to {
				i = k
				break
			}
		}
		if i < 0 {
			t.Fatalf("%s %d: no step covers it in the dataset", rc.prof, rc.skill)
		}
		got := RouteFacts(routeChar(rc.prof, rc.skill), ds, nil, rc.prof)
		label := fmt.Sprintf("%s at skill %d", rc.prof, rc.skill)

		// the step covering the skill, and the two after it
		var want, have []string
		for k := i; k < min(i+3, len(main)); k++ {
			s := main[k]
			lead := map[bool]string{true: "Now", false: "Then"}[k == i]
			start := max(rc.skill, s.from)
			if len(s.where) > 0 && s.recipe == "" {
				want = append(want, fmt.Sprintf("%s: skill %d to %d: gather in %s", lead, start, s.to, strings.Join(s.where, ", ")))
			} else {
				want = append(want, fmt.Sprintf("%s: skill %d to %d: %d x %s", lead, start, s.to, s.left(rc.skill), s.recipe))
			}
		}
		for _, line := range strings.Split(got, "\n") {
			if strings.HasPrefix(line, "Now: ") || strings.HasPrefix(line, "Then: ") {
				head, _, _ := strings.Cut(line, ";")
				have = append(have, strings.TrimSuffix(head, "."))
			}
		}
		res := "match"
		if got == "" || strings.Join(have, " | ") != strings.Join(want, " | ") {
			res = "differ"
		}
		out = append(out, benchRow{label + ": steps", strings.Join(have, " | "), strings.Join(want, " | "), routesSource, res})

		// the materials of the step covering the skill, counted for the crafts left
		s := main[i]
		if len(s.mats) > 0 && s.crafts > 0 {
			var wantMats []string
			for _, m := range s.mats {
				wantMats = append(wantMats, fmt.Sprintf("%d %s", m.count*s.left(rc.skill), m.name))
			}
			line := ""
			for _, l := range strings.Split(got, "\n") {
				if strings.HasPrefix(l, "Now: ") {
					line = l
				}
			}
			res = "match"
			var haveMats []string
			for _, w := range wantMats {
				if strings.Contains(line, w+" (") {
					haveMats = append(haveMats, w)
				}
			}
			if len(haveMats) != len(wantMats) {
				res = "differ"
			}
			show := strings.Join(haveMats, ", ")
			if show == "" {
				show = "no materials"
			}
			out = append(out, benchRow{label + ": materials for the crafts left", show, strings.Join(wantMats, ", "), routesSource, res})
		}

		// the alternatives the guide gives for the step
		if n := len(alts[s.step]); n > 0 {
			haveAlt := 0
			for _, a := range alts[s.step] {
				if strings.Contains(got, fmt.Sprintf("Alternative for that step: %d x %s", a.left(rc.skill), a.recipe)) {
					haveAlt++
				}
			}
			res = "match"
			if haveAlt != n {
				res = "differ"
			}
			out = append(out, benchRow{label + ": alternatives for the step", fmt.Sprintf("%d listed", haveAlt), fmt.Sprintf("%d in the guide", n), routesSource, res})
		}
	}
	finishOffline(t, "professions-routes", "Offline skill-up answers against the dataset routes", out)
}

// TestOfflineBenchmarkRouteRead: every profession's route as the advisor reads it, beside the table's main rows.
func TestOfflineBenchmarkRouteRead(t *testing.T) {
	ds := snapshot().ds
	var out []benchRow
	for _, prof := range []string{"Alchemy", "Blacksmithing", "Cooking", "Enchanting", "Engineering", "First Aid", "Fishing", "Herbalism", "Leatherworking", "Mining", "Skinning", "Tailoring"} {
		main, _ := refRoute(t, ds, prof)
		r, _ := LoadRoute(ds, prof)
		var haveRanges, wantRanges []string
		var haveMats, wantMats int
		for _, s := range r.Steps {
			haveRanges = append(haveRanges, fmt.Sprintf("%d-%d", s.From, s.To))
			haveMats += len(s.Materials)
		}
		for _, s := range main {
			wantRanges = append(wantRanges, fmt.Sprintf("%d-%d", s.from, s.to))
			wantMats += len(s.mats)
		}
		res := "match"
		if strings.Join(haveRanges, " ") != strings.Join(wantRanges, " ") {
			res = "differ"
		}
		out = append(out, benchRow{prof + ": main steps read", fmt.Sprintf("%d steps: %s", len(r.Steps), strings.Join(haveRanges, " ")), fmt.Sprintf("%d steps: %s", len(main), strings.Join(wantRanges, " ")), routesSource, res})
		res = "match"
		if haveMats != wantMats {
			res = "differ"
		}
		out = append(out, benchRow{prof + ": materials read", fmt.Sprint(haveMats), fmt.Sprint(wantMats), routesSource, res})
	}
	finishOffline(t, "professions-route-read", "Offline route rows as the advisor reads them", out)
}

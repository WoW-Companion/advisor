package talents

import (
	"fmt"
	"sort"
	"strings"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/store"
)

// Specs lists a class's spec names (for example Arms, Fury and Protection for a warrior) for the
// play-style spec dropdown. They come from the dataset's talent_trees table; without it (or without
// rows for the class) from the curated talent_builds, then from the trees the character's own capture
// holds. The result is sorted and has no repeats; empty when nothing names a spec.
func Specs(ds store.DataStore, c character.Character) []string {
	if ds != nil {
		if names := treeNames(ds, c.Class); len(names) > 0 {
			return names
		}
		if rows, err := classRows(ds, c.Class); err == nil {
			var names []string
			for _, r := range rows {
				names = append(names, r.spec)
			}
			if names = uniqueSorted(names); len(names) > 0 {
				return names
			}
		}
	}
	var names []string
	if c.Talents != nil {
		for _, t := range c.Talents.Trees {
			if !t.IsProfession() {
				names = append(names, t.Name)
			}
		}
	}
	return uniqueSorted(names)
}

// treeNames reads talent_trees rows for class: the class column picks the rows, name is the spec.
func treeNames(ds store.DataStore, class string) []string {
	rows, err := ds.Filter("talent_trees", store.Range{Column: "id", Min: int64(0)})
	if err != nil {
		return nil
	}
	var names []string
	for _, r := range rows {
		if cl := text(r["class"]); cl != "" && !strings.EqualFold(cl, class) {
			continue
		}
		if cl := text(r["class"]); cl == "" && class != "" {
			continue // no class column: cannot tell whose spec it is
		}
		if n := text(r["name"]); n != "" {
			names = append(names, n)
		}
	}
	return uniqueSorted(names)
}

func uniqueSorted(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if s = strings.TrimSpace(s); s != "" && !seen[strings.ToLower(s)] {
			seen[strings.ToLower(s)] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

// MatchSpec is the spec in specs that free text names (case-insensitively), or "" when none does:
// an old free-text play style migrates to the matching spec, else to "Not sure yet".
func MatchSpec(specs []string, free string) string {
	free = strings.TrimSpace(free)
	for _, s := range specs {
		if strings.EqualFold(s, free) {
			return s
		}
	}
	return ""
}

// NormalizeSpec is the spec in specs that free text names: an exact name in any case, the name followed
// by the class ("Fury warrior"), or a prefix that only one spec starts with ("fur"). The error lists
// the valid names. With no specs to check against, the trimmed text is kept.
func NormalizeSpec(specs []string, class, free string) (string, error) {
	free = strings.Join(strings.Fields(free), " ")
	if free == "" || len(specs) == 0 {
		return free, nil
	}
	low := strings.ToLower(free)
	if c := strings.ToLower(strings.TrimSpace(class)); c != "" {
		low = strings.TrimSpace(strings.TrimSuffix(low, " "+c))
	}
	for _, s := range specs {
		if strings.ToLower(s) == low {
			return s, nil
		}
	}
	var hit []string
	for _, s := range specs {
		if strings.HasPrefix(strings.ToLower(s), low) {
			hit = append(hit, s)
		}
	}
	if len(hit) == 1 && low != "" {
		return hit[0], nil
	}
	return "", fmt.Errorf("%q is not one of this class's specs: %s", free, strings.Join(specs, ", "))
}

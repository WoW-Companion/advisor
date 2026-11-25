package professions

import (
	"strings"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/store"
)

// estimateRank reads a skill level off the difficulty colours of the recipes the player's trade skill
// window listed: each colour bounds the skill the window was opened at (orange below the yellow
// threshold, yellow up to the middle, green up to the grey threshold, grey from there). It returns the
// lowest skill all the colours allow, or false when no recipe can be matched to its thresholds.
func estimateRank(c character.Character, ds store.DataStore, profession string) (int, bool) {
	rs, ok := c.Recipes[strings.ToLower(strings.TrimSpace(profession))]
	if !ok || len(rs.Entries) == 0 || ds == nil {
		return 0, false
	}
	recipes, err := Recipes(ds, profession)
	if err != nil {
		return 0, false
	}
	byName := map[string]Recipe{}
	byID := map[int64]Recipe{}
	for _, r := range recipes {
		byName[strings.ToLower(strings.TrimSpace(r.Name))] = r
		byID[r.SpellID] = r
	}
	lo, hi, n := 1, 1<<30, 0
	for _, e := range rs.Entries {
		r, ok := byName[strings.ToLower(e.Name)]
		if !ok {
			r, ok = byID[e.ID]
		}
		if !ok || r.YellowSkill == nil || r.GreySkill == nil {
			continue
		}
		y, g := *r.YellowSkill, *r.GreySkill
		mid := (y + g) / 2
		var a, b int
		switch e.Difficulty {
		case "orange", "optimal":
			a, b = 1, y-1
		case "yellow", "medium":
			a, b = y, mid-1
		case "green", "easy":
			a, b = mid, g-1
		case "grey", "gray", "trivial":
			a, b = g, 1<<30
		default:
			continue
		}
		if b < a { // thresholds too close to hold the colour
			b = a
		}
		n++
		lo, hi = max(lo, a), min(hi, b)
	}
	if n == 0 {
		return 0, false
	}
	return min(lo, max(hi, 1)), true
}

// skillFor is the character's skill in a profession for planning: the captured one, or for a
// profession known only from its recipes the rank the recipe colours give (estimated true).
func skillFor(c character.Character, ds store.DataStore, profession string) (sk character.Skill, estimated, ok bool) {
	sk, ok = skillOf(c, profession)
	if !ok || sk.Rank != nil || !sk.FromRecipes {
		return sk, false, ok
	}
	if n, found := estimateRank(c, ds, sk.Name); found {
		sk.Rank = &n
		return sk, true, true
	}
	return sk, false, true
}

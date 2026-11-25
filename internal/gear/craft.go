package gear

import (
	"fmt"
	"sort"
	"strings"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/store"
)

// Craft statuses of a candidate that a profession makes.
const (
	CraftNow       = "can_make"       // the character knows the recipe, or a trainer teaches it at their skill
	CraftNeedSkill = "needs_skill"    // a trainer teaches it, but only at a higher skill
	CraftNeedItem  = "needs_recipe"   // taught by a recipe item the character does not have
	CraftNoProf    = "not_profession" // the character has no such profession
)

// Craft says whether the character's own skill can make a candidate. Absent when no profession makes it.
type Craft struct {
	Profession string `json:"profession"`
	Need       int    `json:"need_skill"`
	Status     string `json:"status" jsonschema:"can_make, needs_skill, needs_recipe or not_profession"`
}

// craftOf checks the recipes that make itemID against the character's professions and recipes. The
// best status over the recipes wins. ok is false when the dataset knows no recipe for the item.
func craftOf(c character.Character, ds store.DataStore, itemID int64) (Craft, bool) {
	rows, err := ds.Filter("recipes", store.Range{Column: "creates_item_id", Min: itemID, Max: itemID})
	if err != nil || len(rows) == 0 {
		return Craft{}, false
	}
	var best Craft
	rank := map[string]int{CraftNow: 4, CraftNeedSkill: 3, CraftNeedItem: 2, CraftNoProf: 1}
	for _, r := range rows {
		prof, _ := r["skill_line_name"].(string)
		need := int(i64(r["learn_skill"]))
		from, _ := r["learned_from"].(string)
		cr := Craft{Profession: prof, Need: need, Status: CraftNoProf}
		var have *int
		for _, p := range c.Professions {
			if strings.EqualFold(p.Name, prof) && p.Rank != nil {
				have = p.Rank
			}
		}
		rname, _ := r["name"].(string)
		kn := c.KnowsRecipe(prof, i64(r["spell_id"]), rname, itemID)
		switch {
		case kn == character.RecipeKnown:
			cr.Status = CraftNow
		case have == nil:
		case from == "item":
			cr.Status = CraftNeedItem
		case have != nil && *have >= need:
			cr.Status = CraftNow
		default:
			cr.Status = CraftNeedSkill
		}
		if rank[cr.Status] > rank[best.Status] || best.Status == "" {
			best = cr
		}
	}
	return best, true
}

// Line is the craft in words for a candidate's description.
func (c Craft) Line() string {
	switch c.Status {
	case CraftNow:
		return "you can make it now"
	case CraftNeedSkill:
		return fmt.Sprintf("not craftable yet: needs %s %d", c.Profession, c.Need)
	case CraftNeedItem:
		return "not craftable yet: needs a recipe you don't have"
	case CraftNoProf:
		return fmt.Sprintf("not craftable: you don't have %s", c.Profession)
	}
	return ""
}

// beyondReach lists the crafted candidates the character cannot make, as "At Tailoring 125: A, B." and
// "Need a recipe you don't have: C." lines, so the main lists never call them craftable.
func beyondReach(cands []Candidate) []string {
	bySkill := map[string][]string{}
	var skills []string
	var recipe []string
	for _, cd := range cands {
		if cd.Craft == nil {
			continue
		}
		name := cd.Item.Name
		switch cd.Craft.Status {
		case CraftNeedSkill:
			k := fmt.Sprintf("%s %d", cd.Craft.Profession, cd.Craft.Need)
			if _, ok := bySkill[k]; !ok {
				skills = append(skills, k)
			}
			bySkill[k] = append(bySkill[k], name)
		case CraftNeedItem:
			recipe = append(recipe, name)
		}
	}
	sort.Strings(skills)
	var out []string
	for _, k := range skills {
		out = append(out, "At "+k+": "+strings.Join(dedup(bySkill[k]), ", ")+" (a trainer teaches it; not craftable for you yet).")
	}
	if len(recipe) > 0 {
		out = append(out, "Needs a recipe you don't have: "+strings.Join(dedup(recipe), ", ")+" (not craftable for you).")
	}
	return out
}

func dedup(s []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range s {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

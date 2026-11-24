package professions

import (
	"fmt"
	"sort"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/store"
)

// SkillUpMax is how many recipes a skill-up path lists.
const SkillUpMax = 3

// SkillUpStep is one recipe of a skill-up path. Crafts is an estimate: an orange recipe gives about
// one point per craft, a yellow one about one in two.
type SkillUpStep struct {
	Recipe          string
	Creates         string
	Difficulty      string // Orange or Yellow at the character's skill
	Crafts          int    // estimated crafts to reach Reaches
	Reaches         int    // the skill the crafts reach: the target, or where the recipe turns grey
	Known           bool   // the capture says the recipe is known
	LearnFirst      bool   // the capture lists known recipes and this is not one: learn it at a trainer
	Reagents        []Reagent
	MissingReagents int // reagents the character is short of
}

// craftsFor estimates the crafts of a recipe to climb from skill have to target: one per point
// while orange, two per point while yellow. It also returns the skill reached.
func craftsFor(r Recipe, have, target int) (crafts, reaches int) {
	if r.YellowSkill == nil || r.GreySkill == nil {
		return 0, have
	}
	reaches = min(target, *r.GreySkill)
	if reaches <= have {
		return 0, have
	}
	if o := min(reaches, *r.YellowSkill) - have; o > 0 {
		crafts += o
	}
	if y := reaches - max(have, *r.YellowSkill); y > 0 {
		crafts += 2 * y
	}
	return crafts, reaches
}

// splitCrafts is the crafts of a band that are orange (one skill-up each) and yellow (one in two),
// by the same rule as craftsFor.
func splitCrafts(r Recipe, have, reaches int) (orange, yellow int) {
	if r.YellowSkill == nil {
		return 0, 0
	}
	if o := min(reaches, *r.YellowSkill) - have; o > 0 {
		orange = o
	}
	if y := reaches - max(have, *r.YellowSkill); y > 0 {
		yellow = 2 * y
	}
	return
}

// SkillUp is the fastest path from the character's skill in a profession to target: up to
// SkillUpMax recipes they know or can learn now that are orange or yellow at their skill, those
// whose materials they already hold (bags and bank) first, then known ones, then orange before
// yellow, then the most points per craft. exclude is a spell id left out (the recipe the path
// leads to). Nil when the profession is not captured or no recipe qualifies.
func SkillUp(c character.Character, ds store.DataStore, profession string, target int, exclude int64) []SkillUpStep {
	sk, ok := skillOf(c, profession)
	if !ok || sk.Rank == nil {
		return nil
	}
	have := *sk.Rank
	recipes, err := Recipes(ds, sk.Name)
	if err != nil {
		return nil
	}
	bags, bank := held(c)
	listed := len(c.KnownSpells) > 0
	var out []SkillUpStep
	for _, r := range recipes {
		if r.SpellID == exclude || r.LearnSkill == nil || *r.LearnSkill > have {
			continue
		}
		d := difficulty(r, have)
		if d != Orange && d != Yellow {
			continue
		}
		crafts, reaches := craftsFor(r, have, target)
		if crafts == 0 {
			continue
		}
		s := SkillUpStep{Recipe: r.Name, Difficulty: d, Crafts: crafts, Reaches: reaches,
			Known: c.KnownSpells[r.SpellID], LearnFirst: listed && !c.KnownSpells[r.SpellID]}
		if r.CreatesItem != 0 {
			s.Creates = itemName(ds, r.CreatesItem)
		}
		if s.Recipe == "" {
			if s.Recipe = s.Creates; s.Recipe == "" {
				s.Recipe = fmt.Sprintf("spell %d", r.SpellID)
			}
		}
		for _, row := range reagentsOf(ds, r.SpellID) {
			id, _ := row["reagent_item_id"].(int64)
			per := 1
			if n := asInt(row["count"]); n != nil && *n > 0 {
				per = *n
			}
			need := per * crafts
			name := itemName(ds, id)
			if name == "" {
				name = fmt.Sprintf("item %d", id)
			}
			h := bags[id] + bank[id]
			g := Reagent{ItemID: id, Name: name, Need: need, Have: h, Missing: max(0, need-h)}
			s.MissingReagents += min(1, g.Missing)
			s.Reagents = append(s.Reagents, g)
		}
		out = append(out, s)
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if (a.MissingReagents == 0) != (b.MissingReagents == 0) {
			return a.MissingReagents == 0
		}
		if a.Known != b.Known {
			return a.Known
		}
		if a.Difficulty != b.Difficulty {
			return a.Difficulty == Orange
		}
		if a.Crafts != b.Crafts {
			return a.Crafts < b.Crafts
		}
		return a.Recipe < b.Recipe
	})
	if len(out) > SkillUpMax {
		out = out[:SkillUpMax]
	}
	return out
}

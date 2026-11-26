package professions

import (
	"fmt"
	"sort"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/store"
)

// SkillUpMax is how many recipes a skill-up path lists.
const SkillUpMax = 3

// SkillUpStep is one recipe of a skill-up path. Crafts is an estimate: an orange recipe gives one
// point per craft, a yellow one three in four, a green one one in four.
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

	per  map[int64]int // reagent item id -> count per craft
	cost float64       // vendor copper of the reagents per expected skill point
}

// Skill-up chances by colour: a craft gives a point for sure while the recipe is orange, three times in
// four while yellow, one in four while green; a grey one gives nothing. The dataset holds the yellow and
// grey thresholds; green starts halfway between them, as in the game. The three chances fit the craft
// counts of the wow-professions Forever guides (mean error 8%, against 17% for a flat one in two).
const (
	chanceYellow = 0.75
	chanceGreen  = 0.25
)

// greenFrom is the skill at which a recipe turns green: halfway between its yellow and grey skill.
func greenFrom(r Recipe) int { return (*r.YellowSkill + *r.GreySkill) / 2 }

// chanceAt is the chance that a craft of the recipe gives a skill point at the skill: 1 orange, 0.75
// yellow, 0.25 green, 0 grey or unknown.
func chanceAt(r Recipe, skill int) float64 {
	if r.YellowSkill == nil || r.GreySkill == nil {
		return 0
	}
	switch {
	case skill < *r.YellowSkill:
		return 1
	case skill < greenFrom(r):
		return chanceYellow
	case skill < *r.GreySkill:
		return chanceGreen
	}
	return 0
}

// craftsFor estimates the crafts of a recipe to climb from skill have to target: one per point
// while orange, four per three points while yellow, four per point while green. It also returns the
// skill reached.
func craftsFor(r Recipe, have, target int) (crafts, reaches int) {
	if r.YellowSkill == nil || r.GreySkill == nil {
		return 0, have
	}
	reaches = min(target, *r.GreySkill)
	if reaches <= have {
		return 0, have
	}
	o, y, g := splitCrafts(r, have, reaches)
	return o + y + g, reaches
}

// splitCrafts is the crafts of a band that are orange, yellow and green, by the same rule as craftsFor.
func splitCrafts(r Recipe, have, reaches int) (orange, yellow, green int) {
	if r.YellowSkill == nil || r.GreySkill == nil {
		return 0, 0, 0
	}
	mid := greenFrom(r)
	if o := min(reaches, *r.YellowSkill) - have; o > 0 {
		orange = o
	}
	if y := min(reaches, mid) - max(have, *r.YellowSkill); y > 0 {
		yellow = (4*y + 2) / 3 // y / 0.75, rounded up
	}
	if g := reaches - max(have, mid); g > 0 {
		green = 4 * g
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
		s.per = map[int64]int{}
		chance := chanceAt(r, have)
		for _, row := range reagentsOf(ds, r.SpellID) {
			id, _ := row["reagent_item_id"].(int64)
			per := 1
			if n := asInt(row["count"]); n != nil && *n > 0 {
				per = *n
			}
			need := per * crafts
			s.per[id] = per
			if chance > 0 {
				s.cost += float64(per) * float64(basePrice(ds, id, 0)) / chance
			}
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
	out = dropDominated(out)
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
		if a.cost != b.cost {
			return a.cost < b.cost
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

// dropDominated removes an orange recipe that needs more of a reagent per craft than another orange
// recipe that uses the same reagent: every craft of it gives a point, so it only costs more.
func dropDominated(in []SkillUpStep) []SkillUpStep {
	var out []SkillUpStep
	for i, a := range in {
		dominated := false
		if a.Difficulty == Orange {
			for j, b := range in {
				if i == j || b.Difficulty != Orange {
					continue
				}
				for id, pa := range a.per {
					if pb, ok := b.per[id]; ok && pa > pb {
						dominated = true
					}
				}
			}
		}
		if !dominated {
			out = append(out, a)
		}
	}
	return out
}

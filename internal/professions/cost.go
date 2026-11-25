package professions

import (
	"fmt"
	"strings"
)

// The material cost model. Every material has a relative cost per unit, in copper-like units, so
// that "lowest cost per skill point" prefers common, cheap materials over rare ones even when no
// prices are known:
//
//   - a vendor sells it: the vendor price;
//   - the character gathers it: 20 plus twice the node's skill requirement;
//   - it is a disenchant product (dust, essence, shard): a flat 100;
//   - otherwise it is bought or farmed, and is weighted by tier: 60 plus 30 per tier weight
//     (copper < tin < iron < mithril < thorium; linen < wool < silk < mageweave < runecloth;
//     light < medium < heavy < thick < rugged leather), 100 plus twice the node requirement for a
//     gatherable one, and 150 when nothing is known;
//   - gems and elemental, rare-drop or cooldown materials are weighted heavily (heavyEffort).
//
// A recipe's cost is the cost of its materials, a crafted material costing what it takes to make
// it, plus the cost of learning it; the choice minimises cost per expected skill point, counting
// the skill-ups the intermediate crafts give on the way.
const (
	heavyEffort = 3000 // gems, elemental and rare-drop materials, cooldown products

	// maxMatsPerPoint is the most raw materials a band may take per skill point before it is
	// rejected as a choice, when another recipe fits the band.
	maxMatsPerPoint = 25
)

// tierWeights rank materials of one family by how far into the game they come; the longest
// matching word wins.
var tierWeights = []struct {
	word string
	w    int
}{
	{"copper", 1}, {"tin bar", 2}, {"tin ore", 2}, {"bronze", 3}, {"iron", 4}, {"steel", 5}, {"mithril", 8}, {"truesilver", 10}, {"thorium", 14},
	{"linen", 1}, {"wool", 2}, {"silk", 4}, {"mageweave", 7}, {"runecloth", 10},
	{"light leather", 1}, {"medium leather", 2}, {"heavy leather", 3}, {"thick leather", 5}, {"rugged leather", 8},
	{"rough stone", 1}, {"coarse stone", 2}, {"heavy stone", 3}, {"solid stone", 5}, {"dense stone", 8},
}

// heavyWords mark gems and elemental, rare-drop or cooldown materials.
var heavyWords = []string{
	"ruby", "sapphire", "citrine", "aquamarine", "opal", "jade", "emerald", "diamond", "pearl", "malachite", "tigerseye",
	"shadowgem", "moss agate", "moonstone", "essence of", "heart of fire", "globe of water", "breath of wind", "core of earth",
	"elemental", "arcane crystal", "dark iron", "ichor", "mooncloth", "salt shaker", "arcanite bar", "scale of", "black lotus",
}

// cooldownWords mark materials and recipes with a crafting cooldown, left out of a plan by default.
var cooldownWords = []string{"mooncloth", "salt shaker", "arcanite bar", "transmute"}

// specWords mark recipes only a specialisation teaches: Dragonscale, Elemental and Tribal
// Leatherworking, Armorsmith, Weaponsmith (Swordsmith, Axesmith, Hammersmith), and Goblin and
// Gnomish Engineering.
var specWords = []string{"dragonscale", "elemental leather", "tribal leather", "gnomish", "goblin", "master swordsmith",
	"master axesmith", "master hammersmith", "armorsmith", "weaponsmith", "dragonstrike"}

func containsAny(name string, words []string) bool {
	n := strings.ToLower(name)
	for _, w := range words {
		if strings.Contains(n, w) {
			return true
		}
	}
	return false
}

// tierEffort is the weighted cost of a material from its name alone; 0 when the name says nothing.
func tierEffort(name string) int {
	n := strings.ToLower(name)
	best, bestLen := 0, 0
	for _, t := range tierWeights {
		if strings.Contains(n, t.word) && len(t.word) > bestLen {
			best, bestLen = t.w, len(t.word)
		}
	}
	if best == 0 {
		return 0
	}
	return 60 + 30*best
}

// baseEffort is what a material costs when the character neither gathers, buys from a vendor nor
// disenchants it: the Auction House, weighted by tier and rarity.
func (p *planner) baseEffort(id int64) int {
	name := p.name(id)
	if containsAny(name, heavyWords) {
		return heavyEffort
	}
	req := -1
	for _, y := range rowsBy(p.ds, "gathering_yields", "item_id", id) {
		for _, n := range rowsBy(p.ds, "gathering_nodes", "node_id", asI64(y["node_id"])) {
			if r := int(asI64(n["skill_required"])); r > req {
				req = r
			}
		}
	}
	if req >= 0 {
		return 100 + 2*req
	}
	if e := tierEffort(name); e > 0 {
		return e
	}
	return effortAH
}

// blockedRecipe says a recipe is left out by default: a specialisation recipe the character does
// not know, or one that makes or uses a cooldown material.
func (p *planner) blockedRecipe(r Recipe) bool {
	name := r.Name
	if name == "" && r.CreatesItem != 0 {
		name = p.name(r.CreatesItem)
	}
	if containsAny(name, cooldownWords) || (r.CreatesItem != 0 && containsAny(p.name(r.CreatesItem), cooldownWords)) {
		return true
	}
	if p.known[r.SpellID] {
		return false
	}
	return containsAny(name, specWords) || (r.CreatesItem != 0 && containsAny(p.name(r.CreatesItem), specWords))
}

func (p *planner) usesCooldown(spell int64) bool {
	for _, row := range reagentsOf(p.ds, spell) {
		if containsAny(p.name(asI64(row["reagent_item_id"])), cooldownWords) {
			return true
		}
	}
	return false
}

// leaves counts the raw materials of a need: what it is made from, or itself.
func (n Need) leaves() int {
	if n.Make == nil {
		return n.Total
	}
	c := n.Have
	for _, l := range n.Make.Leaves() {
		c += l.Total
	}
	return c
}

// perPoint is the cost per expected skill point of a priced recipe: the skill-ups of the
// intermediate crafts count as points.
func (s scored) perPoint() float64 {
	pts := float64(s.reach-s.from) + s.opt.SubSkillUps
	if pts <= 0 {
		pts = 1
	}
	return float64(s.effort) / pts
}

// matsPerPoint is how many raw materials the band takes per skill point.
func (s scored) matsPerPoint() float64 {
	pts := s.reach - s.from
	if pts <= 0 {
		pts = 1
	}
	return float64(s.mats) / float64(pts)
}

// withinCap drops the options over the materials-per-point cap, unless all of them are over it.
func withinCap(all []scored) []scored {
	var ok []scored
	for _, s := range all {
		if s.matsPerPoint() <= maxMatsPerPoint {
			ok = append(ok, s)
		}
	}
	if len(ok) == 0 {
		return all
	}
	return ok
}

// fitsAt says a recipe could carry a band starting at skill: learnable, orange or yellow, not
// left out, and with a way to learn it.
func (p *planner) fitsAt(r Recipe, skill, stop int) bool {
	if r.LearnSkill != nil && *r.LearnSkill > skill {
		return false
	}
	if d := difficulty(r, skill); d != Orange && d != Yellow {
		return false
	}
	if c, _ := craftsFor(r, skill, stop); c == 0 || p.blockedRecipe(r) || p.usesCooldown(r.SpellID) {
		return false
	}
	_, _, _, ok := p.learnDetail(r, r.LearnedFrom, skill)
	return ok
}

// gapBand builds the band that says no recipe covers from..to, naming a pattern or quest recipe
// to look for when there is one.
func (p *planner) gapBand(recipes []Recipe, from, to, stop int) Band {
	text := fmt.Sprintf("No trainer or vendor recipe covers %d–%d", from, to)
	var names []string
	for _, o := range p.patternAlternatives(recipes, from, stop, "") {
		n := o.Recipe
		if o.Learn != "" {
			n += " (" + o.Learn + ")"
		}
		names = append(names, n)
	}
	if len(names) > 0 {
		text += ": " + strings.Join(names, ", ") + ", or buy a pattern at the Auction House"
	} else {
		text += ": look for a pattern at the Auction House or a quest reward"
	}
	return Band{From: from, To: to, Gap: text + "."}
}

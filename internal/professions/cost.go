package professions

import (
	"fmt"
	"sort"
	"strings"

	"github.com/WoW-Companion/advisor/internal/store"
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
	"elemental", "arcane crystal", "dark iron", "ichor", "mooncloth", "salt shaker", "refined deeprock salt", "arcanite bar", "scale of", "black lotus",
}

// cooldownWords mark materials and recipes with a crafting cooldown, left out of a plan by default. Refined Deeprock Salt
// is what the Salt Shaker makes, one use every three days, so a recipe that takes it (Cured Rugged Hide) is left out too.
var cooldownWords = []string{"mooncloth", "salt shaker", "refined deeprock salt", "arcanite bar", "transmute"}

// specWords mark recipes only a specialisation teaches, by the profession that has the specialisation:
// Dragonscale, Elemental and Tribal Leatherworking, Armorsmith, Weaponsmith (Swordsmith, Axesmith,
// Hammersmith) and Dragonstrike in Blacksmithing, and Gnomish and Goblin Engineering. A word counts
// only in its own profession: Cooking's Goblin Deviled Clams and Alchemy's Goblin Rocket Fuel are
// ordinary recipes.
var specWords = map[string][]string{
	"leatherworking": {"dragonscale", "elemental leather", "tribal leather", "dragonstrike"},
	"blacksmithing":  {"master swordsmith", "master axesmith", "master hammersmith", "armorsmith", "weaponsmith", "dragonstrike"},
	"engineering":    {"gnomish", "goblin"},
}

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
func (p *planner) blockedRecipe(r Recipe) bool { return p.blockedRecipeIn(p.prof, r) }

// blockedRecipeIn is blockedRecipe for a recipe of the named profession (a material's recipe may belong to
// another profession than the one planned).
func (p *planner) blockedRecipeIn(profession string, r Recipe) bool {
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
	words := specWords[strings.ToLower(profession)]
	return containsAny(name, words) || (r.CreatesItem != 0 && containsAny(p.name(r.CreatesItem), words))
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

// maxUnitsPerMaterial is the most units of one bought or dropped material a whole path may take
// before another recipe is chosen instead; a gathered base material (ore, stone, leather, herbs)
// is exempt, because a gatherer finds it as they go.
const maxUnitsPerMaterial = 600

// leafUnits is how many units of each raw material an option takes, the made ones followed down. A
// piece the plan buys because the character cannot craft it yet (a Bolt of Runecloth below the skill
// that makes it) counts as the cloth it is made from, so the cap sees the cloth either way.
func (p *planner) leafUnits(o Option) map[int64]int {
	out := map[int64]int{}
	add := func(id int64, n int) {
		out[id] += n
		for _, row := range rowsBy(p.ds, "recipes", "creates_item_id", id) {
			if !strings.EqualFold(asText(row["skill_line_name"]), p.prof) {
				continue
			}
			for _, rr := range reagentsOf(p.ds, asI64(row["spell_id"])) {
				per := 1
				if c := asInt(rr["count"]); c != nil && *c > 0 {
					per = *c
				}
				out[asI64(rr["reagent_item_id"])] += per * n
			}
			break
		}
	}
	for _, n := range o.Needs {
		if n.Make == nil {
			add(n.ID, n.Missing)
			continue
		}
		for _, l := range n.Make.Leaves() {
			out[l.ID] += l.Total
		}
	}
	return out
}

// gatheredBase says a material is something a gatherer finds, so its count is not capped.
func (p *planner) gatheredBase(id int64) bool {
	return len(gatherProfs(p.ds, id)) > 0 || p.has("Mining") && minedBase(p.ds, id, p.name(id)) ||
		len(rowsBy(p.ds, "fishing_catches", "item_id", id)) > 0
}

// overCap says an option pushes a material the character does not gather past maxUnitsPerMaterial
// over the whole path, given what the bands chosen so far already take.
func (p *planner) overCap(o Option, used map[int64]int) (int64, bool) {
	for id, n := range p.leafUnits(o) {
		if used[id]+n > maxUnitsPerMaterial && !p.gatheredBase(id) {
			return id, true
		}
	}
	return 0, false
}

// minCappedBand is the fewest skill points a band cut short to fit the cap may still carry; a
// shorter one is not worth a band of its own.
const minCappedBand = 5

// withinUnitCap keeps the options that stay under maxUnitsPerMaterial of every material the
// character does not gather, over the whole path. An option that would pass the cap is cut short to
// the last skill point where it still fits, so the next band moves to another recipe. When no option
// fits at all, it returns every option unchanged and the first result says which material overflowed.
func (p *planner) withinUnitCap(all []scored, recipes []Recipe, used map[int64]int, skill int) ([]scored, bool) {
	bySpell := map[int64]Recipe{}
	for _, r := range recipes {
		bySpell[r.SpellID] = r
	}
	var ok []scored
	for _, s := range all {
		_, over := p.overCap(s.opt, used)
		if !over {
			ok = append(ok, s)
			continue
		}
		r, found := bySpell[s.spell]
		if !found {
			continue
		}
		lo, hi := skill+minCappedBand, s.reach-1 // the longest cut that fits, by bisection
		var best *scored
		for lo <= hi {
			mid := (lo + hi) / 2
			c, good := p.price(r, r.LearnedFrom, skill, mid, p.stock)
			if good {
				if _, o := p.overCap(c.opt, used); !o {
					best = &c
					lo = mid + 1
					continue
				}
			}
			hi = mid - 1
		}
		if best != nil && best.reach-skill >= minCappedBand {
			ok = append(ok, *best)
		}
	}
	if len(ok) == 0 {
		return all, true
	}
	return ok, false
}

// noteShortfall says, once per material, that the chosen option goes past the cap because no recipe
// from this skill avoids it.
func (p *planner) noteShortfall(pa *Path, o Option, used map[int64]int, skill int, seen map[int64]bool) {
	var ids []int64
	for id, n := range p.leafUnits(o) {
		if used[id]+n > maxUnitsPerMaterial && !p.gatheredBase(id) && !seen[id] {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		seen[id] = true
		pa.Notes = append(pa.Notes, fmt.Sprintf("No recipe from skill %d avoids going past %d × %s over the plan; this plan takes the cheapest and asks for more.", skill, maxUnitsPerMaterial, p.name(id)))
	}
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
	if g, ok := gatheringProfession(p.prof); ok {
		return Band{From: from, To: to, Gap: p.gatherAdvice(g, from, to)}
	}
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

// minedBase says an item is mined: an ore a Mining smelt takes, or a stone. The dataset lists the
// nodes of some of these (Truesilver Ore, Dense Stone) nowhere, though a miner finds them.
func minedBase(ds store.DataStore, id int64, name string) bool {
	for _, r := range rowsBy(ds, "recipe_reagents", "reagent_item_id", id) {
		if isMiningSpell(ds, asI64(r["spell_id"])) {
			return true
		}
	}
	if strings.HasSuffix(strings.ToLower(name), " stone") {
		if it, err := ds.Get("items", id); err == nil && asI64(it["class_id"]) == 7 && asI64(it["subclass_id"]) == 7 {
			return true
		}
	}
	return false
}

// madeByMining says a Mining smelt makes the item, so it is a bar a miner smelts and anyone else buys.
func madeByMining(ds store.DataStore, id int64) bool {
	for _, row := range rowsBy(ds, "recipes", "creates_item_id", id) {
		if strings.EqualFold(asText(row["skill_line_name"]), "Mining") {
			return true
		}
	}
	return false
}

// minedEffort is what one unit of a mined base material costs: ore by the skill its smelt needs,
// stone by its tier.
func minedEffort(ds store.DataStore, id int64, name string) int {
	for _, r := range rowsBy(ds, "recipe_reagents", "reagent_item_id", id) {
		for _, sp := range rowsBy(ds, "recipes", "spell_id", asI64(r["spell_id"])) {
			if strings.EqualFold(asText(sp["skill_line_name"]), "Mining") {
				return 20 + 2*int(asI64(sp["learn_skill"]))
			}
		}
	}
	if t := tierEffort(name); t > 0 {
		return 20 + 10*((t-60)/30)
	}
	return 100
}

func isMiningSpell(ds store.DataStore, spell int64) bool {
	for _, sp := range rowsBy(ds, "recipes", "spell_id", spell) {
		if strings.EqualFold(asText(sp["skill_line_name"]), "Mining") {
			return true
		}
	}
	return false
}

// gatheringProfession says the profession levels by gathering, not crafting: its skill comes from nodes,
// catches or skinned beasts, so a recipe gap there is not a missing pattern.
func gatheringProfession(name string) (string, bool) {
	for _, g := range []string{"Herbalism", "Mining", "Skinning", "Fishing"} {
		if strings.EqualFold(name, g) {
			return g, true
		}
	}
	return "", false
}

// gatherAdvice is what to do for skill from..to in a gathering profession: the best nodes the dataset
// knows that the skill opens, with where they are, or a plain statement that it knows none.
func (p *planner) gatherAdvice(prof string, from, to int) string {
	if prof == "Fishing" {
		return "Fish in a zone of your level (about one skill-up a catch); the catch list names the zones."
	}
	if prof == "Skinning" {
		return "Skin beasts of about your level (Skinning has no recipes, and the dataset lists no skinning sources yet)."
	}
	rows, _ := p.ds.Filter("gathering_nodes", store.Range{Column: "profession", Min: prof, Max: prof})
	type node struct {
		name  string
		skill int
		zones []string
	}
	byName := map[string]*node{}
	for _, r := range rows {
		if r["skill_required"] == nil {
			continue
		}
		name := asText(r["name"])
		n := byName[name]
		if n == nil {
			n = &node{name: name, skill: int(asI64(r["skill_required"]))}
			byName[name] = n
		}
		if z := asText(r["zone_name"]); z != "" && len(n.zones) < 3 && !containsString(n.zones, z) {
			n.zones = append(n.zones, z)
		}
	}
	var open []*node
	top := 0
	for _, n := range byName {
		top = max(top, n.skill)
		if n.skill <= to {
			open = append(open, n)
		}
	}
	sort.Slice(open, func(i, j int) bool {
		if open[i].skill != open[j].skill {
			return open[i].skill > open[j].skill
		}
		return open[i].name < open[j].name
	})
	if len(open) > 3 {
		open = open[:3]
	}
	if len(open) == 0 {
		return fmt.Sprintf("%s levels by gathering, not crafting, and the dataset lists no node for this skill yet.", prof)
	}
	var parts []string
	for _, n := range open {
		t := fmt.Sprintf("%s (needs skill %d", n.name, n.skill)
		if len(n.zones) > 0 {
			t += "; " + strings.Join(n.zones, ", ")
		}
		parts = append(parts, t+")")
	}
	text := fmt.Sprintf("%s levels by gathering, not crafting. Gather %s.", prof, strings.Join(parts, "; "))
	if top < to {
		text += fmt.Sprintf(" The dataset lists no node above skill %d yet.", top)
	}
	return text
}

func containsString(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

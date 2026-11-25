package professions

import (
	"fmt"
	"sort"
	"strings"
)

// maxChain is how many recipes deep a material is followed back to its own reagents.
const maxChain = 3

// maxLeavesPerUnit is the most raw materials one unit of a made material may take. A recipe that
// grinds more than this into one unit (light leather up to medium, heavy and thick, each several
// times over) is a conversion no plan should count on: the material is bought instead.
const maxLeavesPerUnit = 10

type makeKey struct {
	item  int64
	skill int
}

type reagentUse struct {
	id  int64
	per int
}

// makeChoice is the recipe that makes a material, with its reagents.
type makeChoice struct {
	rec      Recipe
	line     string
	reagents []reagentUse
	leaves   int // raw materials one unit takes, the made reagents followed down
}

type resolved struct {
	src source
	ch  *makeChoice
}

// Make says a material is made by a recipe the character can use (Mining smelting for bars, the
// profession's own recipes for grinding stones, bolts and cured leather) and what that takes.
type Make struct {
	Recipe     string
	Profession string
	Verb       string // smelt or craft
	Crafts     int
	LearnSkill *int
	Around     bool // LearnSkill is an estimate
	Needs      []Need
	SkillUps   float64 // skill-ups these crafts give, when the recipe is in range of the band
}

func (m *Make) skillUps() float64 {
	if m == nil {
		return 0
	}
	t := m.SkillUps
	for _, n := range m.Needs {
		t += n.Make.skillUps()
	}
	return t
}

// Leaves are the materials a Make bottoms out in: everything not made from something else.
func (m *Make) Leaves() []Need {
	var out []Need
	for _, n := range m.Needs {
		if n.Make != nil {
			out = append(out, n.Make.Leaves()...)
		} else {
			out = append(out, n)
		}
	}
	return out
}

func recipeOf(r map[string]any) Recipe {
	rc := Recipe{SpellID: asI64(r["spell_id"]), Name: asText(r["name"]), LearnSkill: asInt(r["learn_skill"]),
		YellowSkill: asInt(r["yellow_skill"]), GreySkill: asInt(r["grey_skill"]), LearnedFrom: asText(r["learned_from"]),
		LearnSource: asText(r["learn_skill_source"])}
	if n := asInt(r["creates_item_id"]); n != nil {
		rc.CreatesItem = int64(*n)
	}
	return rc
}

// skillIn is the character's skill in a profession the recipes of a material may need: the band's
// skill for the planned profession, the captured skill for another, -1 when the character has none.
func (p *planner) skillIn(line string, band int) int {
	if strings.EqualFold(line, p.prof) {
		return band
	}
	if sk, ok := skillOf(p.c, line); ok && sk.Rank != nil {
		return *sk.Rank
	}
	return -1
}

// resolve is the cheapest way to get one unit of a material at a band's skill: a direct source, or
// a recipe that makes it. A recipe is used unless a vendor sells the item for less.
func (p *planner) resolve(id int64, skill, depth int) (source, *makeChoice) {
	direct := p.sourceOf(id)
	if depth >= maxChain {
		return direct, nil
	}
	key := makeKey{id, skill}
	if p.made == nil {
		p.made = map[makeKey]*resolved{}
	}
	if r, ok := p.made[key]; ok {
		return r.src, r.ch
	}
	if p.making == nil {
		p.making = map[int64]bool{}
	}
	p.making[id] = true
	defer delete(p.making, id)
	res := &resolved{src: direct}
	bestUnit := -1
	for _, row := range rowsBy(p.ds, "recipes", "creates_item_id", id) {
		line := asText(row["skill_line_name"])
		rc := recipeOf(row)
		own := strings.EqualFold(line, p.prof)
		if !own && !strings.EqualFold(line, "Mining") && !p.has(line) {
			continue
		}
		have := p.skillIn(line, skill)
		if rc.LearnSkill != nil && have >= 0 && *rc.LearnSkill > have {
			continue
		}
		if p.blockedRecipe(rc) || p.usesCooldown(rc.SpellID) {
			continue
		}
		if _, _, _, ok := p.learnDetail(rc, rc.LearnedFrom, 300); !ok {
			continue
		}
		var rs []reagentUse
		unit, leaves := 0, 0
		for _, rr := range reagentsOf(p.ds, rc.SpellID) {
			rid := asI64(rr["reagent_item_id"])
			per := 1
			if n := asInt(rr["count"]); n != nil && *n > 0 {
				per = *n
			}
			rs = append(rs, reagentUse{rid, per})
			if p.making[rid] {
				unit += per * p.sourceOf(rid).effort
				leaves += per
				continue
			}
			s, sub := p.resolve(rid, skill, depth+1)
			unit += per * s.effort
			if sub != nil {
				leaves += per * sub.leaves
			} else {
				leaves += per
			}
		}
		if len(rs) == 0 || leaves > maxLeavesPerUnit {
			continue
		}
		if bestUnit < 0 || unit < bestUnit {
			bestUnit = unit
			res.ch = &makeChoice{rec: rc, line: line, reagents: rs, leaves: leaves}
		}
	}
	if res.ch != nil {
		if strings.HasPrefix(direct.text, "a vendor") && direct.effort <= bestUnit {
			res.ch = nil // a vendor sells it for less than it costs to make
		} else {
			verb := "craft"
			if strings.EqualFold(res.ch.line, "Mining") {
				verb = "smelt"
			}
			res.src = source{bestUnit, verb + " it (" + res.ch.line + ")"}
		}
	}
	p.made[key] = res
	return res.src, res.ch
}

// expand lays out what making count units of a material takes, following reagents that are made
// themselves, and the skill-ups the crafts give when the recipe is the planned profession's.
func (p *planner) expand(ch *makeChoice, count, skill, depth int) *Make {
	if p.expanding == nil {
		p.expanding = map[int64]bool{}
	}
	p.expanding[ch.rec.SpellID] = true
	defer delete(p.expanding, ch.rec.SpellID)
	name := ch.rec.Name
	if name == "" {
		name = itemName(p.ds, ch.rec.CreatesItem)
	}
	verb := "craft"
	if strings.EqualFold(ch.line, "Mining") {
		verb = "smelt"
	}
	m := &Make{Recipe: name, Profession: ch.line, Verb: verb, Crafts: count, LearnSkill: ch.rec.LearnSkill,
		Around: ch.rec.LearnSource == "estimate"}
	if strings.EqualFold(ch.line, p.prof) {
		switch difficulty(ch.rec, skill) {
		case Orange:
			m.SkillUps = float64(count)
		case Yellow:
			m.SkillUps = float64(count) / 2
		}
	}
	for _, ru := range ch.reagents {
		n := Need{ID: ru.id, Item: p.name(ru.id), Total: ru.per * count, Missing: ru.per * count}
		src, sub := p.resolve(ru.id, skill, depth+1)
		n.From = src.text
		if sub != nil && !p.making[ru.id] && !p.expanding[sub.rec.SpellID] {
			n.Make = p.expand(sub, n.Total, skill, depth+1)
		}
		m.Needs = append(m.Needs, n)
	}
	sort.SliceStable(m.Needs, func(i, j int) bool { return m.Needs[i].ID < m.Needs[j].ID })
	return m
}

// describe is one line for a made material, "smelt 40 × Bronze Bar (Mining) from 40 × Copper Bar, ...".
func (m *Make) describe() string {
	var parts []string
	for _, n := range m.Needs {
		s := fmt.Sprintf("%d × %s", n.Total, n.Item)
		if n.Make != nil {
			s += " (" + n.Make.describe() + ")"
		}
		parts = append(parts, s)
	}
	out := fmt.Sprintf("%s %d × %s, %s", m.Verb, m.Crafts, m.Recipe, m.Profession)
	if m.LearnSkill != nil {
		around := ""
		if m.Around {
			around = "around "
		}
		out += fmt.Sprintf(", %sskill %d", around, *m.LearnSkill)
	}
	if m.SkillUps >= 1 {
		out += fmt.Sprintf(", about %s skill-ups", trimFloat(m.SkillUps))
	}
	return out + ": " + strings.Join(parts, ", ")
}

// materialHint adds where a material not in stock can also be had, for the Auction House line.
func (p *planner) materialHint(id int64) string {
	var hints []string
	if profs := gatherProfs(p.ds, id); len(profs) > 0 {
		hints = append(hints, "gather with "+strings.Join(profs, " or "))
	}
	if it, err := p.ds.Get("items", id); err == nil && asI64(it["class_id"]) == 7 && asI64(it["subclass_id"]) == 5 {
		hints = append(hints, "drops from humanoids")
	}
	if len(hints) == 0 {
		return ""
	}
	return ", or " + strings.Join(hints, " or ")
}

package professions

import (
	"fmt"
	"sort"
	"strings"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/gather"
	"github.com/WoW-Companion/advisor/internal/questie"
	"github.com/WoW-Companion/advisor/internal/store"
)

// where fills the "where you are" lines: skill, tier, the next tier's cap and gate, the trainer.
func (p *planner) where(pa *Path, sk character.Skill, allowed int, allowedTier Tier) {
	line := fmt.Sprintf("%s skill %d, %s tier", sk.Name, p.skill, pa.Tier)
	if sk.Max != nil {
		line += fmt.Sprintf(" (your rank holds up to %d)", *sk.Max)
	}
	pa.Where = append(pa.Where, line+".")
	if next, ok := p.notes.NextTier(allowed); ok {
		lv := 0
		if p.c.Level != nil {
			lv = *p.c.Level
		}
		if lv >= next.MinLevel {
			pa.Where = append(pa.Where, fmt.Sprintf("Next tier: %s, cap %d, level %d needed (you are level %d, so you can train it).", next.Name, next.SkillCap, next.MinLevel, lv))
		} else {
			pa.Where = append(pa.Where, fmt.Sprintf("Next tier: %s, cap %d, opens at level %d (you are level %d).", next.Name, next.SkillCap, next.MinLevel, lv))
		}
	}
	if sk.Max != nil && *sk.Max < pa.Target {
		pa.Where = append(pa.Where, fmt.Sprintf("Your current rank stops at %d: train the next rank when you get there.", *sk.Max))
	}
	if t := p.trainer(); t != "" {
		pa.Where = append(pa.Where, "Trainer to visit: "+t+".")
	}
}

// trainer is the best trainer for the profession: one in the character's zone when there is one,
// else the one teaching the most recipes; a known faction that is not the character's is skipped.
func (p *planner) trainer() string {
	rows, _ := p.ds.Filter("trainers", store.Range{Column: "profession", Min: p.prof, Max: p.prof})
	zone := ""
	if l := p.c.Location; l != nil {
		zone = l.Zone
	}
	type cand struct {
		name, zone string
		here       bool
		recipes    int64
		id         int64
	}
	var cs []cand
	for _, r := range rows {
		if f := asText(r["faction"]); f != "" && p.c.Faction != "" && !strings.EqualFold(f, p.c.Faction) {
			continue
		}
		z := asText(r["zone_name"])
		name := asText(r["name"])
		if name == "" {
			name = fmt.Sprintf("npc %d (name not in the dataset)", asI64(r["npc_id"]))
		}
		cs = append(cs, cand{name, z, zone != "" && strings.EqualFold(z, zone), asI64(r["recipes"]), asI64(r["npc_id"])})
	}
	if len(cs) == 0 {
		return ""
	}
	sort.Slice(cs, func(i, j int) bool {
		a, b := cs[i], cs[j]
		if a.here != b.here {
			return a.here
		}
		if (a.zone != "") != (b.zone != "") {
			return a.zone != ""
		}
		if a.recipes != b.recipes {
			return a.recipes > b.recipes
		}
		return a.id < b.id
	})
	if cs[0].zone != "" {
		return cs[0].name + " in " + cs[0].zone
	}
	return cs[0].name
}

// disenchantable lists the character's own bag and bank items that disenchant into the material.
func (p *planner) disenchantable(material int64) []string {
	bands := rowsBy(p.ds, "disenchant_yields", "material_item_id", material)
	if len(bands) == 0 {
		return nil
	}
	quality := map[int64]string{2: "uncommon", 3: "rare", 4: "epic"}
	class := map[int64]string{2: "weapon", 4: "armor"}
	seen := map[int64]bool{}
	var out []string
	for _, l := range [][]character.ItemStack{p.c.Bags, p.c.Bank} {
		for _, s := range l {
			id := int64(s.ItemID)
			if seen[id] || s.Count == 0 {
				continue
			}
			it, err := p.ds.Get("items", id)
			if err != nil {
				continue
			}
			for _, b := range bands {
				if asText(b["state"]) == "none" || asText(b["item_class"]) != class[asI64(it["class_id"])] || asText(b["quality"]) != quality[asI64(it["quality"])] {
					continue
				}
				if il := asI64(it["item_level"]); il >= asI64(b["min_item_level"]) && il <= asI64(b["max_item_level"]) {
					seen[id] = true
					name := s.Name
					if name == "" {
						name = asText(it["name"])
					}
					out = append(out, name)
					break
				}
			}
		}
	}
	return out
}

// prep fills the feeder and preparation lines from the notes and the gathering, fishing and
// disenchanting tables, for the materials the path still needs.
func (p *planner) prep(pa *Path, bands []Band) {
	missing := map[string]bool{}
	for _, b := range bands {
		for _, n := range b.Best.Needs {
			if n.Missing > 0 {
				missing[n.Item] = true
			}
		}
	}
	var ids []int64
	for id := range p.short {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	add := func(s string) {
		for _, x := range pa.Prep {
			if x == s {
				return
			}
		}
		if len(pa.Prep) < 8 {
			pa.Prep = append(pa.Prep, s)
		}
	}
	for _, f := range p.notes.Feeders {
		if strings.EqualFold(f.To, p.prof) || strings.EqualFold(f.From, p.prof) {
			add(f.From + " → " + f.To + ": " + f.Text)
		}
	}
	if t := p.notes.Special[strings.ToLower(p.prof)]; t != "" {
		add(t)
	}
	reach := pa.Target
	for _, id := range ids {
		name := p.name(id)
		if !missing[name] {
			continue
		}
		if line, ok := p.gatherLine(id, name); ok {
			add(line)
			continue
		}
		if rows := rowsBy(p.ds, "fishing_catches", "item_id", id); len(rows) > 0 {
			sort.SliceStable(rows, func(i, j int) bool { return asFloat(rows[i]["rate"]) > asFloat(rows[j]["rate"]) })
			var zs []string
			for _, r := range rows[:min(2, len(rows))] {
				zs = append(zs, asText(r["zone_name"]))
			}
			how := "Fish in " + strings.Join(zs, " or ") + " for " + name
			if fs, ok := skillOf(p.c, "Fishing"); ok && fs.Rank != nil {
				how += fmt.Sprintf(" (you have Fishing %d)", *fs.Rank)
			} else {
				how += " (learn Fishing first)"
			}
			add(how + ".")
		}
		for _, y := range rowsBy(p.ds, "gathering_yields", "item_id", id) {
			for _, n := range rowsBy(p.ds, "gathering_nodes", "node_id", asI64(y["node_id"])) {
				if rs := asInt(n["skill_required"]); rs != nil && *rs > reach {
					continue
				}
				prof := asText(n["profession"])
				how := fmt.Sprintf("Gather %s: %s in %s", name, asText(n["name"]), asText(n["zone_name"]))
				if rs := asInt(n["skill_required"]); rs != nil {
					how += fmt.Sprintf(" (%s %d)", prof, *rs)
				}
				if !p.has(prof) {
					how += "; needs " + prof
				}
				add(how + ".")
			}
		}
		if mine := p.disenchantable(id); len(mine) > 0 {
			add(fmt.Sprintf("Disenchant your %s for %s.", strings.Join(mine[:min(3, len(mine))], ", "), name))
		} else if len(rowsBy(p.ds, "disenchant_yields", "material_item_id", id)) > 0 {
			add(name + " comes from disenchanting green and better armor and weapons.")
		}
	}
	if strings.EqualFold(p.prof, "Enchanting") {
		p.rod(pa, add)
	}
}

// rod names the rod to make: the best Rod recipe the character can learn by the end of the path.
func (p *planner) rod(pa *Path, add func(string)) {
	recipes, err := Recipes(p.ds, p.prof)
	if err != nil {
		return
	}
	var best *Recipe
	for i, r := range recipes {
		if strings.Contains(strings.ToLower(r.Name), " rod") && r.LearnSkill != nil && *r.LearnSkill <= p.skill+25 {
			if best == nil || *r.LearnSkill > *best.LearnSkill {
				best = &recipes[i]
			}
		}
	}
	if best == nil {
		return
	}
	for _, l := range [][]character.ItemStack{p.c.Bags, p.c.Bank} {
		for _, s := range l {
			if strings.EqualFold(s.Name, best.Name) {
				add("You already hold " + best.Name + ".")
				return
			}
		}
	}
	add(fmt.Sprintf("Make a rod first: %s (learn at %d).", best.Name, *best.LearnSkill))
}

// gatherLine is the "gather here" line from the player's QuestieDB: the zones whose herb or ore
// nodes or fishing schools yield the item, the character's own zone first, with a coordinate. ok is
// false without Questie, or when no node yields it or the character lacks the skill to use it.
func (p *planner) gatherLine(id int64, name string) (string, bool) {
	if p.gather == nil {
		return "", false
	}
	at := questie.Where{}
	if pos := p.c.LastKnownPosition(); pos != nil {
		at = questie.Where{MapID: pos.MapID, Zone: pos.Zone, X: pos.X, Y: pos.Y}
	}
	kind, zones := p.gather.ItemZonesFor(p.c, id, at)
	if len(zones) == 0 {
		return "", false
	}
	skill, how := "Mining", "Mine"
	switch kind {
	case gather.Herb:
		skill, how = "Herbalism", "Pick"
	case gather.Fish:
		skill, how = "Fishing", "Fish"
	}
	line := fmt.Sprintf("%s %s: %s", how, name, gather.Line(zones))
	if gather.AnyQuestie(zones) {
		line += " (" + gather.FromQuestie + ")"
	}
	if !p.has(skill) {
		line += "; needs " + skill
	}
	return line + ".", true
}

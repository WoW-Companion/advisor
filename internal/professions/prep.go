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
	next := "Journeyman"
	if n, ok := p.notes.NextTier(allowed); ok {
		next = n.Name
	}
	pa.Where = append(pa.Where, p.trainerStep(next))
}

// continents are the dataset's whole-continent maps: a trainer placed on one has no place.
var continents = map[string]bool{"Kalimdor": true, "Eastern Kingdoms": true}

// notNamed marks a trainer the dataset lists by id only.
const notNamed = "(name not in the dataset)"

// trainerStep is the "where you are" line that names the trainer to visit. A trainer the trainers table names
// comes first; when the table names none for the character's side, the guide trainers of profession_tiers (a
// name and a place, for the tier about to be trained) come before an id-only row; when the dataset names no
// trainer at all, the line says so and sends the player to a capital city.
func (p *planner) trainerStep(next string) string {
	t := p.trainer()
	if t != "" && !strings.Contains(t, notNamed) {
		return "Trainer to visit: " + t + "."
	}
	if g := p.guideTrainer(next); g != "" {
		return "Trainer to visit: " + g + "."
	}
	if t != "" {
		return "Trainer to visit: " + t + "."
	}
	return "The game data names no " + p.prof + " trainer for your side; ask in a capital city."
}

// guideTrainer is a trainer the dataset's profession_tiers names for the profession and the character's side:
// one of the tier about to be trained, the one in the character's zone first, else the first listed.
func (p *planner) guideTrainer(tier string) string {
	rows, _ := p.ds.Filter("profession_tiers", store.Range{Column: "profession", Min: p.prof, Max: p.prof})
	zone := ""
	if l := p.c.Location; l != nil {
		zone = l.Zone
	}
	best, bestScore := "", -1
	for _, r := range rows {
		name, place := asText(r["trainer_name"]), asText(r["place"])
		if name == "" || asText(r["how"]) != "trainer" {
			continue
		}
		if f := asText(r["faction"]); f != "" && !strings.EqualFold(f, "both") && p.c.Faction != "" && !strings.EqualFold(f, p.c.Faction) {
			continue
		}
		score := 0
		if strings.EqualFold(asText(r["tier"]), tier) {
			score += 2
		}
		if zone != "" && strings.Contains(strings.ToLower(place), strings.ToLower(zone)) {
			score++
		}
		if score > bestScore {
			best, bestScore = name, score
			if place != "" {
				best = name + " in " + place
			}
		}
	}
	return best
}

// trainer is the best trainer for the profession: one in the character's zone when there is one,
// else one in a zone known to be on the character's side, then the one teaching the most recipes; a known
// faction that is not the character's is skipped.
func (p *planner) trainer() string {
	rows, _ := p.ds.Filter("trainers", store.Range{Column: "profession", Min: p.prof, Max: p.prof})
	zone := ""
	if l := p.c.Location; l != nil {
		zone = l.Zone
	}
	type cand struct {
		name, zone string
		here       bool
		ownSide    bool // the zone is known to be on the character's side
		recipes    int64
		id         int64
	}
	var cs []cand
	for _, r := range rows {
		if f := asText(r["faction"]); f != "" && p.c.Faction != "" && !strings.EqualFold(f, p.c.Faction) {
			continue
		}
		z := asText(r["zone_name"])
		if continents[z] {
			z = "" // a continent is not a place to walk to
		}
		// the dataset has no faction for most trainers: a trainer in the other side's home zone is not one the
		// character can use
		if side := sideOfZone(z); side != "" && p.c.Faction != "" && !strings.EqualFold(side, p.c.Faction) {
			continue
		}
		name := asText(r["name"])
		if name == "" {
			name = curatedTrainers[asI64(r["npc_id"])]
		}
		if name == "" {
			name = fmt.Sprintf("npc %d "+notNamed, asI64(r["npc_id"]))
		}
		cs = append(cs, cand{name, z, zone != "" && strings.EqualFold(z, zone), sideOfZone(z) != "", asI64(r["recipes"]), asI64(r["npc_id"])})
	}
	if len(cs) == 0 {
		return ""
	}
	sort.Slice(cs, func(i, j int) bool {
		a, b := cs[i], cs[j]
		if a.here != b.here {
			return a.here
		}
		if a.ownSide != b.ownSide { // a zone known to be on the character's side before one of no known side
			return a.ownSide
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
			for _, r := range rows {
				if z := asText(r["zone_name"]); p.zoneOpen(z) && len(zs) < 2 {
					zs = append(zs, z)
				}
			}
			if len(zs) == 0 {
				continue // every zone the catches were seen in is the other side's home zone
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

// sideOfZone says which faction's home zones a zone is among, "horde" or "alliance", or "" for a neutral or
// unknown zone. A trainer in a capital or starting zone serves that faction only.
func sideOfZone(zone string) string { return gather.SideOfZone(zone) }

// zoneOpen says a zone is not the other faction's capital or starting zone, which a character of a known
// faction cannot fish in at ease. A neutral or unknown zone is open to both.
func (p *planner) zoneOpen(zone string) bool {
	side := sideOfZone(zone)
	return side == "" || p.c.Faction == "" || strings.EqualFold(side, p.c.Faction)
}

// tierAccess says how the dataset's profession_tiers has a rank gained when it is not a trainer's: a "book"
// bought from a vendor or a "quest", with the guide NPC and place of the character's side. how is "" when the
// rank is taught by a trainer or the table has no row for it.
func tierAccess(ds store.DataStore, profession, tier, faction string) (how, text string) {
	if ds == nil {
		return "", ""
	}
	rows, _ := ds.Filter("profession_tiers", store.Range{Column: "profession", Min: profession, Max: profession})
	for _, r := range rows {
		if !strings.EqualFold(asText(r["tier"]), tier) {
			continue
		}
		if f := asText(r["faction"]); f != "" && !strings.EqualFold(f, "both") && faction != "" && !strings.EqualFold(f, faction) {
			continue
		}
		h := asText(r["how"])
		if h != "book" && h != "quest" {
			continue
		}
		who, place := asText(r["trainer_name"]), asText(r["place"])
		switch {
		case who != "" && place != "":
			who += " in " + place
		case who == "":
			who = place
		}
		if h == "book" {
			text = "it is a book"
			if who != "" {
				text += " from " + who
			}
		} else {
			text = "it comes from a quest"
			if who != "" {
				text += ": " + who
			}
		}
		return h, text
	}
	return "", ""
}

// curatedTrainers names the profession trainers the dataset lists by id only, from the WoW: Forever guides:
// the Tailoring trainers in Undercity (Horde) and Stormwind (Alliance).
var curatedTrainers = map[int64]string{
	4576: "Josef Gregorian",
	1346: "Georgio Bolero",
}

// expertEarly is what the Forever Tailoring guide does that a rank cap alone does not say: it trains
// Expert at Tailoring 125 and character level 20, for 50 silver, long before the 150 cap needs it.
func expertEarly(profession, faction string, level int) string {
	if !strings.EqualFold(profession, "Tailoring") {
		return ""
	}
	who := "Josef Gregorian in Undercity (Horde) or Georgio Bolero in Stormwind (Alliance)"
	switch strings.ToLower(faction) {
	case "horde":
		who = "Josef Gregorian in Undercity"
	case "alliance":
		who = "Georgio Bolero in Stormwind"
	}
	s := "Train Journeyman before skill 75, Expert as soon as you have Tailoring 125 and character level 20 (about 50s, " + who + "), and Artisan as soon as you have Tailoring 200 and character level 35."
	if level > 0 && level < 20 {
		s += " Expert needs character level 20, so you are not there yet."
	}
	return s
}

package ask

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/learn"
	"github.com/WoW-Companion/advisor/internal/store"
)

// The facts for a question about gathering (ore and herb nodes, skinning, fishing) and about the skill a Cooking or First Aid recipe
// needs: what the game data holds, and a plain line where it holds nothing, so the answer states no skill from memory.

var (
	// mineWords and herbWords name the gathering professions (and what they gather) in a question about a node's skill or zones.
	mineWords = regexp.MustCompile(`(?i)\b(mining|miner|ore|ores|vein|veins|deposit|deposits)\b`)
	mineVerb  = regexp.MustCompile(`(?i)\b(mine|mines|mined|mining|miner|ore|ores|vein|veins|deposit|deposits)\b`)
	herbWords = regexp.MustCompile(`(?i)\b(herbalism|herbalist|herb|herbs|herbing)\b`)
	// nodeAsk are the words that make a bare profession name a question about its nodes.
	nodeAsk   = regexp.MustCompile(`(?i)\b(skill|skills|need|needs|require|requires|required|node|nodes|zone|zones|where|level|rank)\b`)
	skinWords = regexp.MustCompile(`(?i)\b(skin|skins|skinning|skinned|skinnable)\b`)
	fishWords = regexp.MustCompile(`(?i)\b(fish|fishing|angling|angler)\b`)
	recipeAsk = regexp.MustCompile(`(?i)\b(skill|skills|learn|learned|train|trained|need|needs|require|requires|required|unlock|rank|when|level)\b`)
)

// maxNodeZones caps the zones listed for one node.
const maxNodeZones = 6

// maxCatches caps the catches listed for one zone.
const maxCatches = 5

// maxRecipeSkills caps how many named recipes get a skill line.
const maxRecipeSkills = 3

// gatherNode is one ore or herb node of the dataset, with the lowest skill any row of it gives and its zones.
type gatherNode struct {
	name  string
	prof  string
	skill int64
	zones []string
}

func gatherNodes(ds store.DataStore) []gatherNode {
	rows, _ := ds.Filter("gathering_nodes", store.Range{Column: "node_id"})
	by := map[string]*gatherNode{}
	var order []string
	for _, r := range rows {
		name, prof, skill := str(r["name"]), str(r["profession"]), i64(r["skill_required"])
		if name == "" || skill <= 0 {
			continue
		}
		key := strings.ToLower(prof + "/" + name)
		n := by[key]
		if n == nil {
			n = &gatherNode{name: name, prof: prof, skill: skill}
			by[key] = n
			order = append(order, key)
		}
		n.skill = min(n.skill, skill)
		if z := str(r["zone_name"]); z != "" {
			dup := false
			for _, have := range n.zones {
				dup = dup || have == z
			}
			if !dup {
				n.zones = append(n.zones, z)
			}
		}
	}
	out := make([]gatherNode, 0, len(order))
	for _, k := range order {
		sort.Strings(by[k].zones)
		out = append(out, *by[k])
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].prof != out[j].prof {
			return out[i].prof < out[j].prof
		}
		if out[i].skill != out[j].skill {
			return out[i].skill < out[j].skill
		}
		return out[i].name < out[j].name
	})
	return out
}

// nodeNamed is true when the question names the node: by its full name, or for a vein by its metal ("tin", "copper").
func nodeNamed(t string, n gatherNode) bool {
	name := strings.ToLower(n.name)
	if wordIn(t, name) {
		return true
	}
	for _, suffix := range []string{" vein", " deposit"} {
		if short, ok := strings.CutSuffix(name, suffix); ok && wordIn(t, short) && mineVerb.MatchString(t) {
			return true
		}
	}
	return false
}

// wordIn is true when the lower-case needle is in t as a whole word or phrase.
func wordIn(t, needle string) bool {
	return regexp.MustCompile(`\b` + regexp.QuoteMeta(needle) + `\b`).MatchString(t)
}

func nodeLine(n gatherNode) string {
	zones := n.zones
	more := ""
	if len(zones) > maxNodeZones {
		zones, more = zones[:maxNodeZones], fmt.Sprintf(" and %d more", len(n.zones)-maxNodeZones)
	}
	where := "no zone in the data"
	if len(zones) > 0 {
		where = strings.Join(zones, ", ") + more
	}
	return fmt.Sprintf("- %s: needs %s %d; found in %s", n.name, n.prof, n.skill, where)
}

// nodeFacts is the dataset's skill and zones for the ore or herb nodes the question names (all of a profession's when it names only the
// profession), and what the data does not hold.
func nodeFacts(ds store.DataStore, t string) string {
	nodes := gatherNodes(ds)
	var named []gatherNode
	for _, n := range nodes {
		if nodeNamed(t, n) {
			named = append(named, n)
		}
	}
	profs := map[string]bool{}
	if mineWords.MatchString(t) {
		profs["Mining"] = true
	}
	if herbWords.MatchString(t) {
		profs["Herbalism"] = true
	}
	var lines []string
	switch {
	case len(named) > 0:
		for _, n := range named {
			lines = append(lines, nodeLine(n))
		}
	case len(profs) > 0 && nodeAsk.MatchString(t):
		for _, n := range nodes {
			if profs[n.prof] {
				lines = append(lines, nodeLine(n))
			}
		}
	default:
		return ""
	}
	none := "Gathering nodes: the game data holds a skill and zones for only the nodes listed here. For any other ore or herb (and for a node listed with no skill), " +
		"say the data has none; name no skill or zone from memory."
	if len(lines) == 0 {
		return none
	}
	return "Gathering nodes (the skill each needs and where it is found, from the game data; answer from these):\n" + strings.Join(lines, "\n") + "\n" + none
}

// skinningSkill is the skill a skinner needs for a creature of the level, by the rule the dataset's notes state: 5 per level from level
// 20, and below that 10 at level 11 and 10 more for each level. ok is false for a level the rule does not reach (10 and under).
func skinningSkill(level int) (skill int, ok bool) {
	switch {
	case level >= 20:
		return level * 5, true
	case level >= 11:
		return (level - 10) * 10, true
	}
	return 0, false
}

// skinningFacts is the dataset's skinning rule and the skill it gives for each creature level the question names.
func skinningFacts(ds store.DataStore, text string) string {
	var rule string
	rows, _ := ds.Filter("profession_notes", store.Range{Column: "section", Min: "rules", Max: "rules"})
	for _, r := range rows {
		if strings.EqualFold(str(r["subject"]), "skinning") {
			rule = str(r["text"])
		}
	}
	if rule == "" {
		return "Skinning: the game data holds no rule for the skill a creature needs. Say so; name no skill from memory."
	}
	var b strings.Builder
	b.WriteString("Skinning (the game data's rule; answer from it): " + rule + "\n")
	seen := map[int]bool{}
	for _, m := range atLevel.FindAllStringSubmatch(text, 4) {
		lvl, _ := strconv.Atoi(m[1])
		if seen[lvl] {
			continue
		}
		seen[lvl] = true
		if skill, ok := skinningSkill(lvl); ok {
			fmt.Fprintf(&b, "- A level %d creature needs Skinning %d.\n", lvl, skill)
		} else {
			fmt.Fprintf(&b, "- A level %d creature: the rule gives no skill below level 11, so state none.\n", lvl)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// fishingFacts says the data holds no fishing skill or pool, and lists the catches the data's fishing reports hold for a zone the
// question names.
func fishingFacts(ds store.DataStore, t string) string {
	var b strings.Builder
	b.WriteString("Fishing: the game data holds no fishing skill needed for a zone, a pool or a catch and no fishing pool locations. " +
		"Say so; name no skill number or pool from memory.")
	rows, _ := ds.Filter("fishing_catches", store.Range{Column: "zone_id"})
	byZone := map[string][]store.Row{}
	for _, r := range rows {
		byZone[str(r["zone_name"])] = append(byZone[str(r["zone_name"])], r)
	}
	var zones []string
	for z := range byZone {
		zones = append(zones, z)
	}
	sort.Strings(zones)
	named := false
	for _, z := range zones {
		if z == "" || !wordIn(t, strings.ToLower(z)) {
			continue
		}
		named = true
		rs := byZone[z]
		sort.SliceStable(rs, func(i, j int) bool {
			if a, b := f64(rs[i]["rate"]), f64(rs[j]["rate"]); a != b {
				return a > b
			}
			return i64(rs[i]["item_id"]) < i64(rs[j]["item_id"])
		})
		var parts []string
		for _, r := range rs[:min(maxCatches, len(rs))] {
			name := fmt.Sprintf("item %d", i64(r["item_id"]))
			if it, err := ds.Get("items", i64(r["item_id"])); err == nil && str(it["name"]) != "" {
				name = str(it["name"])
			}
			parts = append(parts, fmt.Sprintf("%s (%.0f%% of %d casts)", name, 100*f64(r["rate"]), i64(r["casts"])))
		}
		fmt.Fprintf(&b, "\nCatches reported in %s, best rate first: %s.", z, strings.Join(parts, ", "))
	}
	if !named && len(zones) > 0 {
		b.WriteString("\nThe data's fishing reports cover only these zones: " + strings.Join(zones, ", ") + ". For any other zone, say there is no report.")
	}
	return b.String()
}

func f64(v any) float64 { f, _ := v.(float64); return f }

func str(v any) string { s, _ := v.(string); return s }

func i64(v any) int64 {
	switch x := v.(type) {
	case int64:
		return x
	case int32:
		return int64(x)
	case int16:
		return int64(x)
	case int:
		return int64(x)
	}
	return 0
}

// GatherFacts is the gathering section of the facts for a question about an ore or herb node's skill or zones, about the skill a
// creature needs to be skinned, or about fishing: what the game data holds, and a line saying it holds nothing for the rest. "" for any
// other question or without a dataset.
func GatherFacts(ds store.DataStore, text string) string {
	if ds == nil {
		return ""
	}
	t := strings.ToLower(text)
	var out []string
	if s := nodeFacts(ds, t); s != "" {
		out = append(out, s)
	}
	if skinWords.MatchString(t) {
		out = append(out, skinningFacts(ds, t))
	}
	if fishWords.MatchString(t) {
		out = append(out, fishingFacts(ds, t))
	}
	return strings.Join(out, "\n")
}

// RecipeSkillFacts are the Cooking and First Aid recipes the question names (by name, or linked as [Name]) with the skill each needs
// and where it is learned, from the recipe rows, for a question that asks what a recipe needs. A linked recipe the data lacks says so.
func RecipeSkillFacts(ds store.DataStore, text string) string {
	if ds == nil || !recipeAsk.MatchString(text) {
		return ""
	}
	t := strings.ToLower(text)
	rows, _ := ds.Filter("recipes", store.Range{Column: "skill_line_name", Min: "Cooking", Max: "Cooking"})
	more, _ := ds.Filter("recipes", store.Range{Column: "skill_line_name", Min: "First Aid", Max: "First Aid"})
	type hit struct{ name, prof string }
	var hits []hit
	for _, r := range append(rows, more...) {
		name := str(r["name"])
		if name != "" && wordIn(t, strings.ToLower(name)) {
			hits = append(hits, hit{name, str(r["skill_line_name"])})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool { return len(hits[i].name) > len(hits[j].name) })
	var kept []hit
	for _, h := range hits {
		inside := false
		for _, k := range kept {
			inside = inside || strings.Contains(strings.ToLower(k.name), strings.ToLower(h.name))
		}
		if !inside && len(kept) < maxRecipeSkills {
			kept = append(kept, h)
		}
	}
	var lines []string
	for _, h := range kept {
		res, err := learn.Find(character.Character{}, ds, nil, h.name)
		if err != nil {
			continue
		}
		lines = append(lines, fmt.Sprintf("- %s (%s): %s", h.name, h.prof, strings.Join(strings.Fields(res.Gate.Line+" "+rankSentence(res.Text)), " ")))
	}
	for _, m := range itemInBrackets.FindAllStringSubmatch(text, maxLootItems) {
		name := strings.TrimSpace(m[1])
		known := false
		for _, h := range kept {
			known = known || strings.EqualFold(h.name, name)
		}
		if !known && recipeTalk.MatchString(text) {
			lines = append(lines, "- "+name+": the game data has no Cooking or First Aid recipe row for this, so the skill it needs is not known; state none")
		}
	}
	if len(lines) == 0 {
		return ""
	}
	return "Recipe skills (what each needs, from the recipe rows; answer from these, not from memory; \"about\" marks the data's own estimate):\n" + strings.Join(lines, "\n")
}

// recipeTalk makes a question's linked name one about a Cooking or First Aid recipe.
var recipeTalk = regexp.MustCompile(`(?i)\b(cook|cooks|cooking|cooked|first aid|bandage|bandages|recipe|recipes|dish|food)\b`)

// rankSentence is the rank sentence of a recipe answer ("Skill above 75 needs the Journeyman rank, trained from character level 10."), or "".
func rankSentence(text string) string {
	for _, s := range strings.Split(text, "\n") {
		if i := strings.Index(s, "Skill above "); i >= 0 {
			return strings.TrimSpace(s[i:])
		}
	}
	return ""
}

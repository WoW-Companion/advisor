package ask

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/gather"
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
	nodeAsk = regexp.MustCompile(`(?i)\b(skill|skills|need|needs|require|requires|required|node|nodes|zone|zones|where|level|rank|pick|picks|gather|gathers|gathering|find|farm|best|should|which)\b`)
	// mineDo is "mine" as a verb ("to mine gold", "can I mine"): a bare "mine" is also the possessive, so it counts only after a word that
	// makes it a verb.
	mineDo = regexp.MustCompile(`(?i)\b(?:to|can|i|should|will|could|would|do|did|and|or|you|we|please) mine\b|\b(?:mines|mined)\b`)
	// shortAsk are the words that make a bare metal ("tin", "iron") a question about its vein, beside the mining verbs.
	shortAsk = regexp.MustCompile(`(?i)\b(?:find|farm|gather|gathering|dig|where)\b`)
	// skillAt reads a gathering skill out of a question: "at herbalism 50", "mining skill of 120", "100 mining", "skill 60".
	skillAt   = regexp.MustCompile(`(?i)\b(?:(mining|herbalism)(?: skill)?(?: level)?(?: of| at| is| =|:)? (\d{1,3})|(\d{1,3}) (mining|herbalism)|\bskill(?: level)?(?: of| at| is| =|:)? (\d{1,3}))\b`)
	skinWords = regexp.MustCompile(`(?i)\b(skin|skins|skinning|skinned|skinnable)\b`)
	fishWords = regexp.MustCompile(`(?i)\b(fish|fishing|angling|angler)\b`)
	recipeAsk = regexp.MustCompile(`(?i)\b(skill|skills|learn|learned|train|trained|need|needs|require|requires|required|unlock|rank|when|level|make|makes|craft|crafts|cook|cooks|high|yet)\b`)
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
		if short, ok := strings.CutSuffix(name, suffix); ok && wordIn(t, short) && (mineVerb.MatchString(t) || mineDo.MatchString(t) || shortAsk.MatchString(t)) {
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

// nodesOf are the nodes a node question lists: the named ones, else every node of the professions it names.
func nodesOf(nodes, named []gatherNode, profs map[string]bool) []gatherNode {
	if len(named) > 0 {
		return named
	}
	var out []gatherNode
	for _, n := range nodes {
		if profs[n.prof] {
			out = append(out, n)
		}
	}
	return out
}

// skillLines work out, for the gathering skill a question states ("what herbs can I pick at herbalism 50?"), which of the listed nodes it
// reaches and which need more, so the answer compares the skill with the dataset's numbers and does not guess. A skill number with no
// profession beside it counts for the one profession the listed nodes belong to; none for a mix.
func skillLines(listed []gatherNode, t string) []string {
	have := map[string]int64{}
	var bare int64
	for _, m := range skillAt.FindAllStringSubmatch(t, -1) {
		switch {
		case m[1] != "":
			have[profOf(m[1])], _ = strconv.ParseInt(m[2], 10, 64)
		case m[4] != "":
			have[profOf(m[4])], _ = strconv.ParseInt(m[3], 10, 64)
		default:
			bare, _ = strconv.ParseInt(m[5], 10, 64)
		}
	}
	profs := map[string]bool{}
	for _, n := range listed {
		profs[n.prof] = true
	}
	if bare > 0 && len(profs) == 1 {
		for p := range profs {
			if _, ok := have[p]; !ok {
				have[p] = bare
			}
		}
	}
	var out []string
	for _, p := range []string{"Herbalism", "Mining"} {
		skill, ok := have[p]
		if !ok || !profs[p] {
			continue
		}
		var can, not []string
		for _, n := range listed {
			switch {
			case n.prof != p:
			case n.skill <= skill:
				can = append(can, n.name)
			default:
				not = append(not, fmt.Sprintf("%s (needs %d)", n.name, n.skill))
			}
		}
		line := fmt.Sprintf("At %s %d: ", p, skill)
		if len(can) > 0 {
			line += "can gather " + strings.Join(can, ", ")
		} else {
			line += "can gather none of the listed nodes"
		}
		if len(not) > 0 {
			line += "; not yet " + strings.Join(not, ", ")
		}
		out = append(out, line+".")
	}
	return out
}

func profOf(word string) string {
	if strings.EqualFold(word, "mining") {
		return "Mining"
	}
	return "Herbalism"
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
	if mineWords.MatchString(t) || mineDo.MatchString(t) {
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
	case len(profs) > 0 && (nodeAsk.MatchString(t) || skillAt.MatchString(t)):
		for _, n := range nodes {
			if profs[n.prof] {
				lines = append(lines, nodeLine(n))
			}
		}
	default:
		return ""
	}
	lines = append(lines, skillLines(nodesOf(nodes, named, profs), t)...)
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
		"It also holds no lure or bait effect, no bite time and no skill gained per cast or catch (a fishing pole is only an item row). " +
		"Say so; name no skill number, pool, lure bonus or bite time from memory.")
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
		if z == "" || !zoneNamed(t, z) {
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
	if fish := caughtFacts(ds, t, rows); fish != "" {
		named = true
		b.WriteString(fish)
	}
	if !named && len(zones) > 0 {
		b.WriteString("\nThe data's fishing reports cover only these zones: " + strings.Join(zones, ", ") + ". For any other zone, say there is no report.")
	}
	return b.String()
}

// zoneTail are the last words of a zone's name that a player leaves off ("Stranglethorn", "Hillsbrad", "Silverpine").
var zoneTail = map[string]bool{"vale": true, "forest": true, "foothills": true, "mountains": true, "highlands": true, "glades": true, "city": true}

// minZoneShort is the shortest short name of a zone (after the leading "the" and the trailing word are left off) a question may name it by.
const minZoneShort = 6

// zoneNamed is true when the question names the zone: by its full name, without a leading "the" ("fish in Barrens" is the zone The
// Barrens) or without a trailing "Vale", "Forest", "Foothills" or "Mountains" ("Stranglethorn", "Hillsbrad").
func zoneNamed(t, zone string) bool {
	z := strings.ToLower(zone)
	if wordIn(t, z) {
		return true
	}
	z = strings.TrimPrefix(z, "the ")
	if wordIn(t, z) {
		return true
	}
	if i := strings.LastIndex(z, " "); i > 0 && zoneTail[z[i+1:]] && i >= minZoneShort {
		return wordIn(t, z[:i])
	}
	return false
}

// homeNote marks a capital or starting zone of one faction, which a character of the other side does not fish in ("Alliance zone, "), and
// is "" for any other zone.
func homeNote(zone string) string {
	switch gather.SideOfZone(zone) {
	case "horde":
		return "Horde home zone, "
	case "alliance":
		return "Alliance home zone, "
	}
	return ""
}

// minFishName is the shortest fish name (after a leading "raw") a question may name a catch by: a shorter one is a common word.
const minFishName = 6

// caughtFacts is where each fish the question names is caught, by the dataset's fishing reports, best rate first: for "where do I catch
// Longjaw Mud Snapper?", with or without "raw" in the name. "" when the question names no catch.
func caughtFacts(ds store.DataStore, t string, rows []store.Row) string {
	ids := map[int64]bool{}
	var order []int64
	for _, r := range rows {
		if id := i64(r["item_id"]); !ids[id] {
			ids[id] = true
			order = append(order, id)
		}
	}
	sort.Slice(order, func(i, j int) bool { return order[i] < order[j] })
	var b strings.Builder
	for _, id := range order {
		it, err := ds.Get("items", id)
		name := strings.ToLower(str(it["name"]))
		if err != nil || name == "" {
			continue
		}
		short := strings.TrimPrefix(name, "raw ")
		if len(short) < minFishName || !wordIn(t, name) && !wordIn(t, short) {
			continue
		}
		var at []store.Row
		for _, r := range rows {
			if i64(r["item_id"]) == id && str(r["zone_name"]) != "" {
				at = append(at, r)
			}
		}
		if len(at) == 0 {
			continue
		}
		sort.SliceStable(at, func(i, j int) bool { return f64(at[i]["rate"]) > f64(at[j]["rate"]) })
		var parts []string
		for _, r := range at[:min(maxNodeZones, len(at))] {
			parts = append(parts, fmt.Sprintf("%s (%s%.0f%% of %d casts)", str(r["zone_name"]), homeNote(str(r["zone_name"])), 100*f64(r["rate"]), i64(r["casts"])))
		}
		fmt.Fprintf(&b, "\n%s is caught in these zones, best rate first: %s. The reports cover only the zones they list; for any other zone, say there is no report.", str(it["name"]), strings.Join(parts, ", "))
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

// fishCaughtFacts is the fishing section for a question that names a catch but has no fishing word ("where do I catch Longjaw Mud Snapper?"):
// the same no-data line as any fishing question, then the zones that catch the fish.
func fishCaughtFacts(ds store.DataStore, t string) string {
	rows, _ := ds.Filter("fishing_catches", store.Range{Column: "zone_id"})
	if len(rows) == 0 || caughtFacts(ds, t, rows) == "" {
		return ""
	}
	return fishingFacts(ds, t)
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
	} else if s := fishCaughtFacts(ds, t); s != "" {
		out = append(out, s)
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

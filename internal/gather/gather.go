// Package gather answers "where do I mine, pick or fish X?" and "where should I gather at my
// level?" from the player's own QuestieDB objects, read at runtime: the zones with the most nodes or
// schools that yield the item, the character's own zone first, with a sample coordinate or two. The
// rules are in docs/professions.md and docs/design/questie-interop.md.
package gather

import (
	"fmt"
	"sort"
	"strings"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/names"
	"github.com/WoW-Companion/advisor/internal/questie"
	"github.com/WoW-Companion/advisor/internal/store"
)

// Kinds of gathering, named for the skill they use.
const (
	Herb = "herb"
	Ore  = "ore"
	Fish = "fishing"
)

// FromQuestie marks every place read from the player's QuestieDB.
const FromQuestie = "from your Questie database"

// MaxZones is how many zones an answer lists.
const MaxZones = 3

// Node is a Questie object that the classification rule accepted as gatherable.
type Node struct {
	questie.Node
	Kind  string
	Skill int // the skill the node needs; 0 when no number is known
}

// Gatherer holds the classified nodes of one QuestieDB. A nil Gatherer knows no nodes.
type Gatherer struct {
	qx    *questie.Index
	ds    store.DataStore
	nodes []Node
}

// notSources are objects that hold loot, never a node.
var notSources = []string{"chest", "crate", "barrel", "footlocker", "cache", "lockbox", "trunk", "strongbox", "bag of", "sack"}

func isContainer(name string) bool {
	n := strings.ToLower(name)
	for _, w := range notSources {
		if strings.Contains(n, w) {
			return true
		}
	}
	return false
}

func str(v any) string { s, _ := v.(string); return s }

func i64(v any) int64 {
	switch x := v.(type) {
	case int64:
		return x
	case int32:
		return int64(x)
	case int:
		return int64(x)
	}
	return 0
}

// reagentSets are the items that Mining recipes use (ore) and the items that other crafts use
// that no Mining recipe does (the herb candidates), from the dataset's recipes and recipe_reagents.
func reagentSets(ds store.DataStore) (ore, herb map[int64]bool) {
	ore, herb = map[int64]bool{}, map[int64]bool{}
	if ds == nil {
		return
	}
	recipes, _ := ds.Filter("recipes", store.Range{Column: "spell_id"})
	line := map[int64]string{}
	for _, r := range recipes {
		line[i64(r["spell_id"])] = strings.ToLower(str(r["skill_line_name"]))
	}
	reag, _ := ds.Filter("recipe_reagents", store.Range{Column: "spell_id"})
	for _, r := range reag {
		id := i64(r["reagent_item_id"])
		switch line[i64(r["spell_id"])] {
		case "mining":
			ore[id] = true
		case "alchemy", "inscription":
			herb[id] = true
		}
	}
	for id := range ore {
		delete(herb, id)
	}
	return
}

// datasetNodes are the dataset's own node names with their profession and skill, when it has any.
func datasetNodes(ds store.DataStore) map[string]store.Row {
	out := map[string]store.Row{}
	if ds == nil {
		return out
	}
	rows, _ := ds.Filter("gathering_nodes", store.Range{Column: "node_id"})
	for _, r := range rows {
		if n := strings.ToLower(str(r["name"])); n != "" {
			if _, dup := out[n]; !dup {
				out[n] = r
			}
		}
	}
	return out
}

// curatedSkills are the node skill levels the curated profession_notes carry, by lower-case node
// name: rows of the "nodes" or "gathering" section with the skill in skill_min.
func curatedSkills(ds store.DataStore) map[string]int {
	out := map[string]int{}
	if ds == nil {
		return out
	}
	rows, _ := ds.Filter("profession_notes", store.Range{Column: "section"})
	for _, r := range rows {
		if s := str(r["section"]); s != "nodes" && s != "gathering" {
			continue
		}
		if n := i64(r["skill_min"]); n > 0 {
			out[strings.ToLower(str(r["subject"]))] = int(n)
		}
	}
	return out
}

func kindOfProfession(p string) string {
	switch strings.ToLower(p) {
	case "herbalism":
		return Herb
	case "mining":
		return Ore
	case "fishing":
		return Fish
	}
	return ""
}

// New classifies the Questie objects that yield items. The rule, in order, for each object that
// is not a container:
//
//  1. the dataset names it as a Herbalism or Mining node: that profession (and its skill);
//  2. its name says vein or deposit (ore), or school, wreckage or similar (fishing);
//  3. it yields an item that a Mining recipe uses as a reagent: ore;
//  4. it yields an item that an Alchemy or Inscription recipe uses and no Mining recipe does: herb.
//
// The node's skill is the curated profession_notes figure for its name, else the dataset's
// skill_required, else the lowest required level of what it yields. A nil index, or one with no
// object rows, gives a Gatherer with no nodes.
func New(qx *questie.Index, ds store.DataStore) *Gatherer {
	g := &Gatherer{qx: qx, ds: ds}
	if qx == nil || len(qx.Nodes) == 0 {
		return g
	}
	ore, herb := reagentSets(ds)
	named, curated := datasetNodes(ds), curatedSkills(ds)
	ids := make([]int, 0, len(qx.Nodes))
	for id := range qx.Nodes {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	for _, id := range ids {
		n := qx.Nodes[id]
		if isContainer(n.Name) {
			continue
		}
		low := strings.ToLower(n.Name)
		kind, skill := "", 0
		if r, ok := named[low]; ok {
			kind = kindOfProfession(str(r["profession"]))
			skill = int(i64(r["skill_required"]))
		}
		if kind == "" {
			switch n.Kind {
			case questie.KindOre:
				kind = Ore
			case questie.KindFish:
				kind = Fish
			}
		}
		for _, y := range n.Yields {
			if kind != "" {
				break
			}
			switch {
			case ore[int64(y.Item)]:
				kind = Ore
			case herb[int64(y.Item)]:
				kind = Herb
			}
		}
		if kind == "" {
			continue
		}
		if c := curated[low]; c > 0 {
			skill = c
		}
		if skill <= 1 {
			skill = 0
			for _, y := range n.Yields {
				if y.Level > 1 && (skill == 0 || y.Level < skill) {
					skill = y.Level
				}
			}
		}
		g.nodes = append(g.nodes, Node{Node: n, Kind: kind, Skill: skill})
	}
	return g
}

// Count is how many gathering nodes were classified.
func (g *Gatherer) Count() int {
	if g == nil {
		return 0
	}
	return len(g.nodes)
}

// Zone is one zone to gather in.
type Zone struct {
	Zone   string         `json:"zone"`
	Spawns int            `json:"spawns" jsonschema:"how many nodes or schools spawn there"`
	Here   bool           `json:"here,omitempty" jsonschema:"the character is in this zone"`
	Spots  []questie.Spot `json:"-"`
	Coords [][2]float64   `json:"coordinates,omitempty" jsonschema:"sample places, map percent"`
	Nodes  []string       `json:"nodes" jsonschema:"the node or school names that spawn there"`
	Seen   bool           `json:"seen_by_you,omitempty" jsonschema:"the player gathered here themselves (the addon's own record)"`
	Skill  int            `json:"skill,omitempty" jsonschema:"the lowest skill a node there needs; 0 when unknown"`
}

// Result is an answer.
type Result struct {
	Query       string `json:"query,omitempty"`
	Kind        string `json:"kind,omitempty"`
	Item        string `json:"item,omitempty"`
	Skill       int    `json:"skill,omitempty" jsonschema:"the character's skill, for a level answer"`
	Zones       []Zone `json:"zones"`
	FromQuestie bool   `json:"from_questie,omitempty"`
	Text        string `json:"text"`
}

func whereOf(c character.Character) questie.Where {
	var w questie.Where
	if p := c.LastKnownPosition(); p != nil {
		w = questie.Where{MapID: p.MapID, Zone: p.Zone, X: p.X, Y: p.Y}
	} else if c.Location != nil {
		w.Zone = c.Location.Zone
	}
	return w
}

func (g *Gatherer) zones(nodes []Node, at questie.Where) []Zone {
	qn := make([]questie.Node, len(nodes))
	skill := map[string]int{}
	for i, n := range nodes {
		qn[i] = n.Node
		if n.Skill > 0 && (skill[n.Name] == 0 || n.Skill < skill[n.Name]) {
			skill[n.Name] = n.Skill
		}
	}
	var out []Zone
	for _, z := range g.qx.NodeZones(qn, at) {
		o := Zone{Zone: z.Zone, Spawns: z.Spawns, Here: z.Here, Nodes: z.Nodes, Spots: z.Spots}
		for _, s := range z.Spots {
			o.Coords = append(o.Coords, [2]float64{round1(s.X), round1(s.Y)})
		}
		for _, n := range z.Nodes {
			if s := skill[n]; s > 0 && (o.Skill == 0 || s < o.Skill) {
				o.Skill = s
			}
		}
		out = append(out, o)
	}
	return out
}

func round1(v float64) float64 { return float64(int(v*10+0.5)) / 10 }

func spots(z Zone) string {
	var s []string
	for _, c := range z.Coords {
		s = append(s, fmt.Sprintf("(%.0f, %.0f)", c[0], c[1]))
	}
	return strings.Join(s, " ")
}

func describe(z Zone) string {
	s := z.Zone
	if len(z.Nodes) > 0 {
		s += ": " + strings.Join(z.Nodes[:min(2, len(z.Nodes))], ", ")
	}
	if sp := spots(z); sp != "" {
		s += " " + sp
	}
	if z.Seen {
		s += " (" + character.SeenByYou + ")"
	}
	return s
}

// ItemZones are the zones where the item with that id can be gathered, for the profession planner:
// the character's zone first, then the busiest. Nil when no classified node yields it.
func (g *Gatherer) ItemZones(item int64, at questie.Where) (kind string, zones []Zone) {
	if g == nil {
		return "", nil
	}
	var hit []Node
	for _, n := range g.nodes {
		for _, y := range n.Yields {
			if int64(y.Item) == item {
				hit = append(hit, n)
				break
			}
		}
	}
	if len(hit) == 0 {
		return "", nil
	}
	zs := g.zones(hit, at)
	return hit[0].Kind, zs[:min(MaxZones, len(zs))]
}

// Line is the planner's "gather here" clause for zones: "Zone: Node (x, y); Zone: ...".
func Line(zones []Zone) string {
	var out []string
	for _, z := range zones[:min(2, len(zones))] {
		out = append(out, describe(z))
	}
	return strings.Join(out, "; ")
}

// verbs map the question's verb to the kinds it can mean.
func verbKinds(verb string) []string {
	switch strings.ToLower(strings.TrimSpace(verb)) {
	case "mine", "mining", "ore":
		return []string{Ore}
	case "pick", "herb", "herbs", "herbalism", "gather herbs":
		return []string{Herb}
	case "fish", "fishing":
		return []string{Fish}
	}
	return nil
}

func inKinds(k string, kinds []string) bool {
	if len(kinds) == 0 {
		return true
	}
	for _, x := range kinds {
		if x == k {
			return true
		}
	}
	return false
}

// Find answers "where can I find/mine/pick/fish X?". verb may be empty. It names up to MaxZones
// zones, with the character's own zone first and then the zones with the most spawns of the nodes
// or schools that yield the item, each with sample coordinates.
func (g *Gatherer) Find(c character.Character, item, verb string) Result {
	res := Result{Query: strings.TrimSpace(item), Zones: []Zone{}}
	if seen, name, kind := g.seenFind(c, res.Query, verb); len(seen) > 0 {
		var rest []Zone
		if g.Count() > 0 {
			rest = g.questieFind(c, res.Query, verb).Zones
		}
		res.Zones = mergeZones(seen, rest)
		res.Item, res.Kind, res.FromQuestie = name, kind, len(res.Zones) > len(seen) || hasQuestie(res.Zones)
		res.Text = zonesText(verbPhrase(kind)+" "+name, res.Zones)
		return res
	}
	found := g.questieFind(c, item, verb)
	if len(found.Zones) == 0 {
		if caught, ok := g.catchZones(c, item, verb); ok {
			return caught
		}
	}
	return found
}

// SideOfZone says which faction's home zones a zone is among, "horde" or "alliance", or "" for a neutral or
// unknown zone. A character of the other faction does not fish or trade in a capital or starting zone.
func SideOfZone(zone string) string {
	switch zone {
	case "Orgrimmar", "Durotar", "Undercity", "Tirisfal Glades", "Silverpine Forest", "Mulgore", "Thunder Bluff":
		return "horde"
	case "Stormwind City", "Elwynn Forest", "Westfall", "Ironforge", "Dun Morogh", "Loch Modan", "Darnassus", "Teldrassil":
		return "alliance"
	}
	return ""
}

// catchZones answers a fishing question from the dataset's fishing_catches when no school or sighting does: the
// zones where installs caught the item, best catch rate first, leaving out a zone that is the other faction's
// home. It needs the item to be one fishing gives; any other question is no answer.
func (g *Gatherer) catchZones(c character.Character, query, verb string) (Result, bool) {
	if g == nil || g.ds == nil || strings.TrimSpace(query) == "" || !inKinds(Fish, verbKinds(verb)) {
		return Result{}, false
	}
	id, name, _ := names.Resolve(g.ds, names.Items, query)
	if id == 0 {
		return Result{}, false
	}
	rows, err := g.ds.Filter("fishing_catches", store.Range{Column: "item_id", Min: id, Max: id})
	if err != nil || len(rows) == 0 {
		return Result{}, false
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if a, b := f64(rows[i]["rate"]), f64(rows[j]["rate"]); a != b {
			return a > b
		}
		return str(rows[i]["zone_name"]) < str(rows[j]["zone_name"])
	})
	res := Result{Query: strings.TrimSpace(query), Item: name, Kind: Fish, Zones: []Zone{}}
	for _, r := range rows {
		z := str(r["zone_name"])
		if side := SideOfZone(z); z == "" || side != "" && c.Faction != "" && !strings.EqualFold(side, c.Faction) {
			continue
		}
		note := fmt.Sprintf("%.0f%% of %d casts", 100*f64(r["rate"]), i64(r["casts"]))
		res.Zones = append(res.Zones, Zone{Zone: z, Nodes: []string{note}})
		if len(res.Zones) == MaxZones {
			break
		}
	}
	if len(res.Zones) == 0 {
		return Result{}, false
	}
	res.Text = zonesText(verbPhrase(Fish)+" "+name, res.Zones)
	return res, true
}

func f64(v any) float64 { f, _ := v.(float64); return f }

func (g *Gatherer) questieFind(c character.Character, item, verb string) Result {
	res := Result{Query: strings.TrimSpace(item), Zones: []Zone{}}
	if g.Count() == 0 {
		res.Text = fmt.Sprintf("No gathering spots are known for %s.", res.Query)
		return res
	}
	kinds := verbKinds(verb)
	q := strings.ToLower(res.Query)
	type cand struct {
		name  string
		nodes []Node
		spawn int
		exact bool
	}
	byItem := map[string]*cand{}
	for _, n := range g.nodes {
		if !inKinds(n.Kind, kinds) {
			continue
		}
		for _, y := range n.Yields {
			low := strings.ToLower(y.Name)
			if q == "" || !strings.Contains(low, q) {
				continue
			}
			cd := byItem[low]
			if cd == nil {
				cd = &cand{name: y.Name, exact: low == q}
				byItem[low] = cd
			}
			cd.nodes = append(cd.nodes, n)
			cd.spawn += len(n.Spawns)
		}
	}
	var best *cand
	for _, cd := range byItem {
		if best == nil || cd.exact && !best.exact || cd.exact == best.exact && (cd.spawn > best.spawn || cd.spawn == best.spawn && cd.name < best.name) {
			best = cd
		}
	}
	if best == nil {
		res.Text = fmt.Sprintf("No node or school that yields %s is known.", res.Query)
		return res
	}
	zs := g.zones(best.nodes, whereOf(c))
	res.Zones = zs[:min(MaxZones, len(zs))]
	res.Item, res.Kind, res.FromQuestie = best.name, best.nodes[0].Kind, len(res.Zones) > 0
	if len(res.Zones) == 0 {
		res.Text = fmt.Sprintf("No place is known for %s.", best.name)
		return res
	}
	res.Text = zonesText(verbPhrase(res.Kind)+" "+best.name, res.Zones)
	return res
}

func verbPhrase(kind string) string {
	switch kind {
	case Ore:
		return "Mine"
	case Herb:
		return "Pick"
	}
	return "Fish for"
}

func skillName(kind string) string {
	switch kind {
	case Ore:
		return "Mining"
	case Herb:
		return "Herbalism"
	}
	return "Fishing"
}

// ForLevel answers "where should I mine/herb/fish at my level?". The character's skill comes from
// their professions. A node suits them when its skill is at most theirs; the best are those within
// 50 points under it (still worth the trip), else the highest they can take. Skills the notes and
// the yielded items do not give count as suiting everyone. Up to MaxZones zones, the character's
// own first, then the most spawns.
func (g *Gatherer) ForLevel(c character.Character, kind string) Result {
	res := Result{Kind: kind, Zones: []Zone{}}
	sk := skillName(kind)
	rank, have := -1, false
	for _, p := range c.Professions {
		if strings.EqualFold(p.Name, sk) && p.Rank != nil {
			rank, have = *p.Rank, true
		}
	}
	if !have {
		res.Text = "Open your " + sk + " window in the game once so I can read your skill, then ask again."
		return res
	}
	res.Skill = rank
	var ok []Node
	top := 0
	for _, n := range g.nodesOf(kind) {
		if n.Skill <= rank {
			ok = append(ok, n)
			top = max(top, n.Skill)
		}
	}
	if len(ok) == 0 && len(seenLevel(c, kind, rank)) == 0 {
		res.Text = fmt.Sprintf("No %s spot suits %s %d yet.", strings.ToLower(sk), sk, rank)
		return res
	}
	var good []Node
	for _, n := range ok {
		if n.Skill >= max(rank-50, 0) && n.Skill > 0 || n.Skill == 0 {
			good = append(good, n)
		}
	}
	if len(good) == 0 {
		for _, n := range ok {
			if n.Skill == top {
				good = append(good, n)
			}
		}
	}
	var zs []Zone
	if len(good) > 0 {
		zs = g.zones(good, whereOf(c))
	}
	res.Zones = mergeZones(seenLevel(c, kind, rank), zs)
	res.FromQuestie = hasQuestie(res.Zones)
	if len(res.Zones) == 0 {
		res.Text = fmt.Sprintf("No %s spot is known.", strings.ToLower(sk))
		return res
	}
	res.Text = zonesText(fmt.Sprintf("At %s %d", sk, rank), res.Zones)
	return res
}

func (g *Gatherer) nodesOf(kind string) []Node {
	var out []Node
	if g == nil {
		return nil
	}
	for _, n := range g.nodes {
		if n.Kind == kind {
			out = append(out, n)
		}
	}
	return out
}

// KindOf maps a profession or verb the player used to a kind; empty when it is none.
func KindOf(s string) string {
	if k := kindOfProfession(s); k != "" {
		return k
	}
	if k := verbKinds(s); len(k) > 0 {
		return k[0]
	}
	return ""
}

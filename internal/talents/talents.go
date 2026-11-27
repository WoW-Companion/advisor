// Package talents says where a character's next talent points go, from the dataset's talent_builds
// (Icy Veins' WoW Forever leveling builds, one row per point) and talent_priorities tables and the
// captured points. The rules are in docs/talents.md.
//
// A build is never invented: with no checked build for the spec the plan says so and lists only
// the unspent points. Points spent off-build are noted, not judged.
package talents

import (
	"fmt"
	"sort"
	"strings"

	"github.com/WoW-Companion/advisor/internal/brief"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/store"
)

// DefaultLimit is how many points a plan lists when none is asked for.
const DefaultLimit = 3

// Options are the caller's choices; the zero value infers the spec and lists DefaultLimit points.
type Options struct {
	Spec  string // overrides the spec inferred from the most points spent
	Limit int
}

// Point is one talent point to spend: Rank is the node's rank once it is spent, Max the most
// ranks the build gives the node.
type Point struct {
	NodeID    int64
	Name      string
	Rank      int
	Max       int
	Level     int // the character level at which the build spends it
	Rationale string
}

// OffBuild is a node with points spent that the build does not give (or not that many).
type OffBuild struct {
	Name  string
	Rank  int
	Build int // ranks the build gives the node; 0 when it is not in it
}

// Priority is one of the guide's key leveling talents, with our own one-line reason.
type Priority struct {
	Rank   int
	NodeID int64
	Name   string
	Reason string
}

// The "no checked build" line is noBuildPre + spec + noBuildPost.
const noBuildPre, noBuildPost = "I don't have a checked build for ", " yet."

// Result is the result of Plan.
type Result struct {
	Class        string
	Spec         string
	SpecInferred bool // the spec was taken from the most points spent
	SpecDefault  bool // no points, or a tie: the class's default leveling spec
	Level        *int
	Unspent      *int
	Done         string // the last build point already taken, "Spirit Tap 5/5"
	Next         []Point
	OffBuild     []OffBuild
	Priorities   []Priority
	Remaining    int      // points the build adds at or below the character's level that are not taken yet
	NoBuild      bool     // Missing is the "no checked build" line: only the unspent points are listed
	Missing      string   // why no plan could be made; empty when there is one
	Options      []string // with Missing: talents open now (rank below max, can be bought), as options, not a recommendation
	Available    []string // specs the dataset has builds for, for Missing
	Notes        []string
}

// buildStep is one talent_builds row: the point that takes node to rankAfter at level.
type buildStep struct {
	spec, name, source string
	level, rankAfter   int
	order, node        int64
	hasLevel           bool
}

func text(v any) string { s, _ := v.(string); return s }

func num(v any) int64 { n, _ := v.(int64); return n }

// classRows reads the builds of a class (every class when empty), by spec and order.
func classRows(ds store.DataStore, class string) ([]buildStep, error) {
	rows, err := ds.Filter("talent_builds", store.Range{Column: "order", Min: 1})
	if err != nil {
		return nil, err
	}
	var out []buildStep
	for _, r := range rows {
		if class != "" && !strings.EqualFold(text(r["class"]), class) {
			continue
		}
		_, hasLevel := r["level"]
		_, hasRank := r["rank_after"]
		out = append(out, buildStep{spec: text(r["spec"]), name: text(r["talent_name"]), source: text(r["source"]),
			level: int(num(r["level"])), rankAfter: int(num(r["rank_after"])), order: num(r["order"]), node: num(r["node_id"]), hasLevel: hasLevel && hasRank})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].spec != out[j].spec {
			return out[i].spec < out[j].spec
		}
		return out[i].order < out[j].order
	})
	return out, nil
}

// checked is true when the steps are a verified build: DATA-045's columns are there and the rows are not
// marked unverified.
func checked(steps []buildStep) bool {
	for _, s := range steps {
		if !s.hasLevel || strings.Contains(strings.ToLower(s.source), "unverified") {
			return false
		}
	}
	return len(steps) > 0
}

// available lists the build specs, in spec order.
func available(steps []buildStep) []string {
	var out []string
	for _, s := range steps {
		if len(out) == 0 || out[len(out)-1] != s.spec {
			out = append(out, s.spec)
		}
	}
	return out
}

// defaultSpec is the class's default leveling spec: the checked spec with the most build points (the
// first by name on a tie).
func defaultSpec(steps []buildStep) string {
	count := map[string]int{}
	for _, s := range steps {
		count[s.spec]++
	}
	best := ""
	for _, sp := range available(steps) {
		if best == "" || count[sp] > count[best] {
			best = sp
		}
	}
	return best
}

// inferSpec is the one group with the most points, or "" when there is none or several tie.
func inferSpec(c character.Character) string {
	names, pts := brief.TopGroups(c.Talents)
	if pts == 0 || len(names) != 1 {
		return ""
	}
	return names[0]
}

// priorities reads the guide's key talents for a class and spec, by rank.
func priorities(ds store.DataStore, class, spec string) []Priority {
	rows, err := ds.Filter("talent_priorities", store.Range{Column: "rank", Min: 1})
	if err != nil {
		return nil
	}
	var out []Priority
	for _, r := range rows {
		if strings.EqualFold(text(r["class"]), class) && strings.EqualFold(text(r["spec"]), spec) {
			out = append(out, Priority{int(num(r["rank"])), num(r["node_id"]), text(r["talent_name"]), text(r["reason"])})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Rank < out[j].Rank })
	return out
}

// matchSpec finds the build spec a group name or an override names, by name, case-insensitively.
func matchSpec(steps []buildStep, name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	best := ""
	for _, s := range steps {
		sp := strings.ToLower(s.spec)
		if sp == name {
			return s.spec
		}
		if name != "" && strings.Contains(name, sp) && len(s.spec) > len(best) {
			best = s.spec
		}
	}
	if best != "" || len(name) < 3 {
		return best
	}
	// A short name a player types ("shadow", "feral", "prot") names the one spec it starts.
	hit := ""
	for _, s := range steps {
		if strings.HasPrefix(strings.ToLower(s.spec), name) && s.spec != hit {
			if hit != "" {
				return ""
			}
			hit = s.spec
		}
	}
	return hit
}

// CantSee is the line for a capture whose spent talents cannot be told apart.
const CantSee = "I can't see which talents you've taken yet."

// nodeSpells maps a talent spell id to its node id (0 when the dataset has no spell ids), and
// the set of node names, both from the dataset's talent_nodes.
func nodeSpells(ds store.DataStore) (map[int64]int64, map[string]bool) {
	spells, names := map[int64]int64{}, map[string]bool{}
	if ds == nil {
		return spells, names
	}
	rows, err := ds.Filter("talent_nodes", store.Range{Column: "node_id", Min: 1})
	if err != nil {
		return spells, names
	}
	for _, r := range rows {
		if sp := num(r["spell_id"]); sp != 0 {
			spells[sp] = num(r["node_id"])
		}
		if n := strings.ToLower(strings.TrimSpace(text(r["name"]))); n != "" {
			names[n] = true
		}
	}
	return spells, names
}

// Unreadable is true when the capture says points are spent but some spent class node cannot be
// told apart: no node id, no name and no spell id the dataset knows. Crediting such a capture would propose rank 1 of a talent that may already be maxed.
func Unreadable(c character.Character, ds store.DataStore) bool {
	if c.Talents == nil {
		return false
	}
	bySpell, _ := nodeSpells(ds)
	for _, tr := range c.Talents.Trees {
		if tr.IsProfession() {
			continue
		}
		for _, tl := range tr.Talents {
			if tl.Rank <= 0 || tl.NodeID != nil || strings.TrimSpace(tl.Name) != "" {
				continue
			}
			if tl.SpellID == nil || bySpell[int64(*tl.SpellID)] == 0 {
				return true
			}
		}
	}
	return false
}

func nodeName(ds store.DataStore, id int64) string {
	rows, err := ds.Filter("talent_nodes", store.Range{Column: "node_id", Min: id, Max: id})
	if err != nil || len(rows) == 0 {
		return ""
	}
	return text(rows[0]["name"])
}

// Plan walks the build for the character's spec in order and lists the first points the character has
// not taken whose level they have reached.
func Plan(c character.Character, ds store.DataStore, opt Options) (p Result) {
	p = Result{Class: c.Class, Level: c.Level}
	defer func() {
		if p.Missing != "" && !p.NoBuild {
			p.Options = openOptions(c, 5)
		}
	}()
	if c.Talents != nil {
		p.Unspent = c.Talents.Unspent
	}
	limit := opt.Limit
	if limit <= 0 {
		limit = DefaultLimit
	}
	if ds == nil {
		p.Missing = "No dataset: there are no talent builds to compare with."
		return p
	}
	all, err := classRows(ds, c.Class)
	if err != nil || len(all) == 0 {
		p.Missing = "The dataset has no talent builds" + map[bool]string{true: " for " + c.Class, false: ""}[c.Class != ""] + "."
		return p
	}
	p.Available = available(all)
	if Unreadable(c, ds) {
		p.Missing = CantSee
		return p
	}

	want := strings.TrimSpace(opt.Spec)
	if want == "" && c.Profile != nil {
		want = strings.TrimSpace(c.Profile.Spec)
	}
	if want == "" && c.Profile != nil {
		want = specForRole(all, c, c.Profile.Role)
	}
	if want == "" {
		if want = inferSpec(c); want != "" {
			p.SpecInferred = true
		} else {
			want, p.SpecDefault = defaultSpec(all), true
		}
	}
	p.Spec = matchSpec(all, want)
	if p.Spec == "" {
		p.Spec, p.NoBuild = want, true
		p.Missing = noBuildPre + want + noBuildPost
		return p
	}
	var build []buildStep
	for _, s := range all {
		if s.spec == p.Spec {
			build = append(build, s)
		}
	}
	if !checked(build) {
		p.NoBuild = true
		p.Missing = noBuildPre + p.Spec + noBuildPost
		return p
	}
	if c.Level == nil {
		p.Missing = "Level not captured: cannot tell which build points are due."
		return p
	}
	p.Priorities = priorities(ds, c.Class, p.Spec)
	why := map[int64]string{}
	for _, pr := range p.Priorities {
		if _, ok := why[pr.NodeID]; !ok {
			why[pr.NodeID] = pr.Reason
		}
	}

	// The captured ranks, by node id, else spell id, else name.
	ranks, byName := map[int64]int{}, map[string]int{}
	bySpell, _ := nodeSpells(ds)
	if c.Talents != nil {
		for _, t := range c.Talents.Trees {
			if t.IsProfession() {
				continue
			}
			for _, tl := range t.Talents {
				switch {
				case tl.NodeID != nil:
					ranks[int64(*tl.NodeID)] += tl.Rank
				case tl.SpellID != nil && bySpell[int64(*tl.SpellID)] != 0:
					ranks[bySpell[int64(*tl.SpellID)]] += tl.Rank
				default:
					byName[strings.ToLower(strings.TrimSpace(tl.Name))] += tl.Rank
				}
			}
		}
	}
	names, maxRank := map[int64]string{}, map[int64]int{}
	for _, s := range build {
		n := s.name
		if n == "" {
			if n = nodeName(ds, s.node); n == "" {
				n = fmt.Sprintf("node %d", s.node)
			}
		}
		names[s.node] = n
		maxRank[s.node] = max(maxRank[s.node], s.rankAfter)
	}
	for id, n := range names {
		if _, ok := ranks[id]; !ok {
			if r, ok := byName[strings.ToLower(n)]; ok {
				ranks[id] = r
			}
		}
	}
	for _, s := range build {
		if ranks[s.node] >= s.rankAfter {
			p.Done = fmt.Sprintf("%s %d/%d", names[s.node], min(ranks[s.node], maxRank[s.node]), maxRank[s.node])
			continue
		}
		if s.level > *c.Level {
			continue
		}
		p.Remaining++
		if len(p.Next) < limit {
			p.Next = append(p.Next, Point{s.node, names[s.node], s.rankAfter, maxRank[s.node], s.level, why[s.node]})
		}
	}
	// Points spent outside the build, once per node.
	if c.Talents != nil {
		byBuildName := map[string]int64{}
		for id, n := range names {
			byBuildName[strings.ToLower(n)] = id
		}
		seen := map[string]bool{}
		for _, t := range c.Talents.Trees {
			if t.IsProfession() {
				continue
			}
			for _, tl := range t.Talents {
				if tl.Rank <= 0 {
					continue
				}
				id, name := int64(0), strings.TrimSpace(tl.Name)
				switch {
				case tl.NodeID != nil:
					id = int64(*tl.NodeID)
				case tl.SpellID != nil && bySpell[int64(*tl.SpellID)] != 0:
					id = bySpell[int64(*tl.SpellID)]
				default:
					id = byBuildName[strings.ToLower(name)]
				}
				if name == "" {
					name = nodeName(ds, id)
				}
				key := fmt.Sprint(id, strings.ToLower(name))
				if have := maxRank[id]; tl.Rank > have && !seen[key] {
					seen[key] = true
					p.OffBuild = append(p.OffBuild, OffBuild{name, tl.Rank, have})
				}
			}
		}
	}
	if p.Remaining == 0 {
		if first := firstLevel(build); p.Done == "" && first > *c.Level {
			p.Notes = append(p.Notes, fmt.Sprintf("The Icy Veins %s build starts at level %d: there is no talent point to spend at level %d.", p.Spec, first, *c.Level))
		} else {
			p.Notes = append(p.Notes, fmt.Sprintf("The Icy Veins %s build has nothing more to take at level %d.", p.Spec, *c.Level))
		}
	}
	if p.Unspent == nil {
		p.Notes = append(p.Notes, "Unspent points not captured: the points listed may not all be affordable yet.")
	}
	return p
}

// firstLevel is the level of the build's first point.
func firstLevel(build []buildStep) int {
	first := 0
	for _, s := range build {
		if first == 0 || s.level < first {
			first = s.level
		}
	}
	return first
}

// roleOf is the role a spec name stands for: the tank, healer and damage specs of every class. The
// dataset's builds carry no role, so the spec's name decides; a name not listed is damage.
func roleOf(spec string) string {
	switch strings.ToLower(spec) {
	case "protection", "blood", "guardian", "brewmaster", "vengeance":
		return "tank"
	case "holy", "discipline", "restoration", "mistweaver", "preservation":
		return "healer"
	}
	return "damage"
}

// specForRole is the build spec of the character's class that matches the stored role, or "" when
// none does or several do and the points spent do not tell them apart.
func specForRole(all []buildStep, c character.Character, role string) string {
	if role == "" || role == "balanced" {
		return ""
	}
	var match []string
	seen := map[string]bool{}
	for _, s := range all {
		if !seen[s.spec] && roleOf(s.spec) == role {
			seen[s.spec] = true
			match = append(match, s.spec)
		}
	}
	if len(match) == 1 {
		return match[0]
	}
	if len(match) > 1 {
		if g := inferSpec(c); g != "" {
			return matchSpec(buildsOf(match), g)
		}
	}
	return ""
}

func buildsOf(specs []string) []buildStep {
	out := make([]buildStep, len(specs))
	for i, s := range specs {
		out[i].spec = s
	}
	return out
}

// openOptions are up to n captured class talents that can be bought now: a rank still open and every
// prerequisite met (the capture says the node can be bought).
func openOptions(c character.Character, n int) []string {
	var out []string
	if c.Talents == nil {
		return nil
	}
	for _, t := range c.Talents.Trees {
		if t.IsProfession() {
			continue
		}
		for _, tl := range t.Talents {
			if len(out) >= n {
				return out
			}
			if tl.CanBuy == nil || !*tl.CanBuy || tl.MaxRank != nil && tl.Rank >= *tl.MaxRank {
				continue
			}
			out = append(out, fmt.Sprintf("%s (rank %d)", tl.Name, tl.Rank+1))
		}
	}
	return out
}

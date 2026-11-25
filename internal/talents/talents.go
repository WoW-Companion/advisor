// Package talents says where a character's next talent points go, from the dataset's curated
// talent_builds table and the captured points. The rules are in docs/talents.md.
//
// A build is never invented: with no build for the spec and level band the plan says so and
// lists the builds the dataset does have. Points spent off-build are listed, not judged.
package talents

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/WoW-Companion/advisor/internal/brief"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/store"
)

// DefaultLimit is how many points a plan lists when none is asked for.
const DefaultLimit = 5

// Options are the caller's choices; the zero value infers the spec and lists DefaultLimit points.
type Options struct {
	Spec  string // overrides the spec inferred from the most points spent
	Limit int
}

// Point is one talent point to spend: Rank is the node's rank once it is spent.
type Point struct {
	NodeID    int64
	Name      string
	Rank      int
	Band      string // the build band that adds it
	Rationale string
}

// OffBuild is a node with points spent that the build does not give (or not that many).
type OffBuild struct {
	Name  string
	Rank  int
	Build int // ranks the build gives the node over the character's bands; 0 when not in it
}

// Result is the result of Plan.
type Result struct {
	Class        string
	Spec         string
	SpecInferred bool
	Level        *int
	Band         string
	Unspent      *int
	Next         []Point
	OffBuild     []OffBuild
	Remaining    int      // points the build still adds, through the character's band
	Missing      string   // why no plan could be made; empty when there is one
	Options      []string // with Missing: talents open now (rank below max, can be bought), as options, not a recommendation
	Available    []string // builds in the dataset, for Missing
	Notes        []string
}

// buildStep is one talent_builds row.
type buildStep struct {
	spec, band, rationale string
	order                 int64
	node                  int64
	points                int
}

func text(v any) string { s, _ := v.(string); return s }

func num(v any) int64 { n, _ := v.(int64); return n }

// parseBand reads "10-19".
func parseBand(b string) (lo, hi int, ok bool) {
	l, h, found := strings.Cut(b, "-")
	lo, err1 := strconv.Atoi(l)
	hi, err2 := strconv.Atoi(h)
	return lo, hi, found && err1 == nil && err2 == nil
}

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
		out = append(out, buildStep{text(r["spec"]), text(r["band"]), text(r["rationale"]), num(r["order"]), num(r["node_id"]), int(num(r["points"]))})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].spec != out[j].spec {
			return out[i].spec < out[j].spec
		}
		return out[i].order < out[j].order
	})
	return out, nil
}

// available lists "Spec 10-19, 20-29" per spec, in spec order.
func available(steps []buildStep) []string {
	var specs []string
	bands := map[string][]string{}
	for _, s := range steps {
		if len(bands[s.spec]) == 0 {
			specs = append(specs, s.spec)
		}
		if l := bands[s.spec]; len(l) == 0 || l[len(l)-1] != s.band {
			bands[s.spec] = append(l, s.band)
		}
	}
	out := make([]string, len(specs))
	for i, s := range specs {
		out[i] = s + " " + strings.Join(bands[s], ", ")
	}
	return out
}

// inferSpec is the one group with the most points, or why there is none.
func inferSpec(c character.Character) (string, string) {
	names, pts := brief.TopGroups(c.Talents)
	switch {
	case pts == 0:
		return "", "No talent points captured: say which spec to plan for."
	case len(names) > 1:
		return "", fmt.Sprintf("%s are tied at %d points: say which spec to plan for.", strings.Join(names, ", "), pts)
	}
	return names[0], ""
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
	return best
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
// told apart: no node id, no name and no spell id the dataset knows, or points with no node list at
// all. Crediting such a capture would propose rank 1 of a talent that may already be maxed.
func Unreadable(c character.Character, ds store.DataStore) bool {
	if c.Talents == nil {
		return false
	}
	bySpell, _ := nodeSpells(ds)
	for _, tr := range c.Talents.Trees {
		if tr.IsProfession() {
			continue
		}
		if len(tr.Talents) == 0 && tr.Points != nil && *tr.Points > 0 {
			return true
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

// Plan compares the build for the character's spec and level band with the captured points and
// lists the next points to spend.
func Plan(c character.Character, ds store.DataStore, opt Options) (p Result) {
	p = Result{Class: c.Class, Level: c.Level}
	defer func() {
		if p.Missing != "" {
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
		p.Missing = "No dataset: there are no curated talent builds to compare with."
		return p
	}
	all, err := classRows(ds, c.Class)
	if err != nil || len(all) == 0 {
		p.Missing = "The dataset has no curated talent builds" + map[bool]string{true: " for " + c.Class, false: ""}[c.Class != ""] + "."
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
		var why string
		if want, why = inferSpec(c); why != "" {
			p.Missing = why
			return p
		}
		p.SpecInferred = true
	}
	p.Spec = matchSpec(all, want)
	if p.Spec == "" {
		p.Spec = want
		p.Missing = "No curated build for " + want + "."
		return p
	}
	if c.Level == nil {
		p.Missing = "Level not captured: cannot pick a level band."
		return p
	}
	var build []buildStep
	band := -1 // index of the character's band among the spec's bands, by low level
	for _, s := range all {
		if s.spec != p.Spec {
			continue
		}
		build = append(build, s)
		if lo, hi, ok := parseBand(s.band); ok && *c.Level >= lo && *c.Level <= hi {
			p.Band = s.band
			band = len(build)
		}
	}
	if p.Band == "" {
		p.Missing = fmt.Sprintf("No curated %s build for level %d.", p.Spec, *c.Level)
		return p
	}

	// Credit the captured ranks against the build's steps, in order; what is left is next.
	ranks, byName := map[int64]int{}, map[string]int{}
	bySpell, _ := nodeSpells(ds)
	if c.Talents != nil {
		for _, t := range c.Talents.Trees {
			for _, tl := range t.Talents {
				switch {
				case tl.NodeID != nil:
					ranks[int64(*tl.NodeID)] += tl.Rank
				case tl.SpellID != nil && bySpell[int64(*tl.SpellID)] != 0:
					ranks[bySpell[int64(*tl.SpellID)]] += tl.Rank
				default:
					byName[strings.ToLower(tl.Name)] += tl.Rank
				}
			}
		}
	}
	names := map[int64]string{}
	name := func(id int64) string {
		if n, ok := names[id]; ok {
			return n
		}
		n := nodeName(ds, id)
		if n == "" {
			n = fmt.Sprintf("node %d", id)
		}
		names[id] = n
		return n
	}
	for _, s := range build {
		if _, ok := ranks[s.node]; !ok {
			if r, ok := byName[strings.ToLower(name(s.node))]; ok {
				ranks[s.node] = r
			}
		}
	}
	credit, total := map[int64]int{}, map[int64]int{}
	for id, r := range ranks {
		credit[id] = r
	}
	running := map[int64]int{}
	for i, s := range build {
		take := min(s.points, credit[s.node])
		credit[s.node] -= take
		running[s.node] += s.points
		total[s.node] += s.points
		if i >= band {
			continue
		}
		for j := take; j < s.points; j++ {
			p.Remaining++
			if len(p.Next) < limit {
				p.Next = append(p.Next, Point{s.node, name(s.node), running[s.node] - s.points + j + 1, s.band, s.rationale})
			}
		}
	}
	// Nodes whose later bands the build still adds are not off-build.
	if c.Talents != nil {
		seen := map[string]bool{}
		for _, t := range c.Talents.Trees {
			for _, tl := range t.Talents {
				if tl.Rank <= 0 {
					continue
				}
				id := int64(0)
				if tl.NodeID != nil {
					id = int64(*tl.NodeID)
				}
				have := total[id]
				if tl.NodeID == nil {
					for nid := range total {
						if strings.EqualFold(name(nid), tl.Name) {
							id, have = nid, total[nid]
						}
					}
				}
				if tl.Rank > have && !seen[fmt.Sprint(id, tl.Name)] {
					seen[fmt.Sprint(id, tl.Name)] = true
					p.OffBuild = append(p.OffBuild, OffBuild{tl.Name, tl.Rank, have})
				}
			}
		}
	}
	if p.Remaining == 0 {
		p.Notes = append(p.Notes, fmt.Sprintf("The %s build has nothing left to add through level band %s.", p.Spec, p.Band))
	}
	if p.Unspent == nil {
		p.Notes = append(p.Notes, "Unspent points not captured: the points listed may not all be affordable yet.")
	}
	return p
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
		if g, why := inferSpec(c); why == "" {
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

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

func nodeName(ds store.DataStore, id int64) string {
	rows, err := ds.Filter("talent_nodes", store.Range{Column: "node_id", Min: id, Max: id})
	if err != nil || len(rows) == 0 {
		return ""
	}
	return text(rows[0]["name"])
}

// Plan compares the build for the character's spec and level band with the captured points and
// lists the next points to spend.
func Plan(c character.Character, ds store.DataStore, opt Options) Result {
	p := Result{Class: c.Class, Level: c.Level}
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

	want := strings.TrimSpace(opt.Spec)
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
	if c.Talents != nil {
		for _, t := range c.Talents.Trees {
			for _, tl := range t.Talents {
				switch {
				case tl.NodeID != nil:
					ranks[int64(*tl.NodeID)] += tl.Rank
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

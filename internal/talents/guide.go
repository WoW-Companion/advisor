package talents

import (
	"fmt"
	"strings"

	"github.com/WoW-Companion/advisor/internal/character"
)

// GuidePick is one point to spend from the player's saved plan: Rank is the node's rank once the
// point is spent.
type GuidePick struct {
	Name    string
	Group   string // the tree branch the captured node sits in
	Rank    int
	MaxRank int // 0 when the capture has none
	SpellID *int
}

// GuideResult is the deterministic next picks of a saved plan. It never calls the model.
type GuideResult struct {
	Spec     string
	GuideURL string
	Date     string
	Next     []GuidePick
	Missing  []string // plan nodes the captured tree does not hold
	Stale    bool     // the player's chosen spec is not the plan's
	Blocked  int      // plan nodes the capture says cannot be bought yet
	Done     bool     // every node of the plan is at its target rank
}

// GuideNext lists up to n next points of the character's saved plan against the captured ranks, in
// plan order, one point at a time. ok is false when the character has no plan. A stale plan (the
// profile names another spec) yields no picks, so a spec change never advises the old build.
func GuideNext(c character.Character, n int) (r GuideResult, ok bool) {
	p := c.Plan
	if p == nil || len(p.Picks) == 0 {
		return r, false
	}
	r = GuideResult{Spec: p.Spec, GuideURL: p.GuideURL, Date: p.Date}
	if c.Profile != nil && c.Profile.Spec != "" && !strings.EqualFold(c.Profile.Spec, p.Spec) {
		r.Stale = true
		return r, true
	}
	have := map[string]character.Talent{}
	if c.Talents != nil {
		for _, t := range c.Talents.Trees {
			if t.IsProfession() {
				continue
			}
			for _, tl := range t.Talents {
				have[strings.ToLower(tl.Name)] = tl
			}
		}
	}
	r.Done = true
	for _, pk := range p.Picks {
		tl, found := have[strings.ToLower(pk.Node)]
		if !found {
			r.Missing = append(r.Missing, pk.Node)
			continue
		}
		if tl.Rank >= pk.Rank {
			continue
		}
		r.Done = false
		if tl.CanBuy != nil && !*tl.CanBuy {
			r.Blocked++
			continue
		}
		mx := 0
		if tl.MaxRank != nil {
			mx = *tl.MaxRank
		}
		for rank := tl.Rank + 1; rank <= pk.Rank && len(r.Next) < n; rank++ {
			r.Next = append(r.Next, GuidePick{tl.Name, tl.Group, rank, mx, tl.SpellID})
		}
	}
	return r, true
}

// Describe is the picks as one phrase: "Whirlwind (rank 1/2) in Fury", consecutive ranks of one
// node joined ("ranks 1-2/2"), picks joined by "then".
func (r GuideResult) Describe() string {
	var parts []string
	for i := 0; i < len(r.Next); {
		j := i
		for j+1 < len(r.Next) && r.Next[j+1].Name == r.Next[i].Name {
			j++
		}
		a, b := r.Next[i], r.Next[j]
		rank := fmt.Sprintf("rank %d", a.Rank)
		if j > i {
			rank = fmt.Sprintf("ranks %d-%d", a.Rank, b.Rank)
		}
		if b.MaxRank > 0 {
			rank += fmt.Sprintf("/%d", b.MaxRank)
		}
		s := fmt.Sprintf("%s (%s)", a.Name, rank)
		if a.Group != "" {
			s += " in " + a.Group
		}
		parts = append(parts, s)
		i = j + 1
	}
	return strings.Join(parts, ", then ")
}

// Source is "<guide URL> <spec> build".
func (r GuideResult) Source() string { return strings.TrimSpace(r.GuideURL + " " + r.Spec + " build") }

// Notes are the plan's caveats in plain words: a stale plan, nodes missing from the captured tree,
// points the capture says cannot be bought yet.
func (r GuideResult) Notes() []string {
	var out []string
	if r.Stale {
		out = append(out, fmt.Sprintf("The saved plan is for %s, not the chosen spec: ask to refresh the talent plan.", r.Spec))
	}
	if len(r.Missing) > 0 {
		out = append(out, fmt.Sprintf("The plan names %d node(s) that are not in the captured tree (%s): ask the AI once to adapt the plan to this tree.", len(r.Missing), strings.Join(r.Missing[:min(3, len(r.Missing))], ", ")))
	}
	if r.Blocked > 0 {
		out = append(out, fmt.Sprintf("%d planned node(s) cannot be bought yet (a node before them is unspent).", r.Blocked))
	}
	return out
}

// RenderTree lists the captured class tree nodes, one per line, by branch: the names a plan may use.
func RenderTree(c character.Character) string {
	var b strings.Builder
	if c.Talents == nil {
		return "No talents captured.\n"
	}
	for _, t := range c.Talents.Trees {
		if t.IsProfession() || len(t.Talents) == 0 {
			continue
		}
		fmt.Fprintf(&b, "Tree %s:\n", t.Name)
		for _, tl := range t.Talents {
			fmt.Fprintf(&b, "- %s [%s] rank %d", tl.Name, tl.Group, tl.Rank)
			if tl.MaxRank != nil {
				fmt.Fprintf(&b, "/%d", *tl.MaxRank)
			}
			if tl.CanBuy != nil && *tl.CanBuy {
				b.WriteString(" can buy")
			}
			if tl.SpellID != nil {
				fmt.Fprintf(&b, " spell %d", *tl.SpellID)
			}
			b.WriteString("\n")
		}
	}
	if b.Len() == 0 {
		return "No class talent tree captured.\n"
	}
	return b.String()
}

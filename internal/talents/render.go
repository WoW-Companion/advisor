package talents

import (
	"fmt"
	"strings"

	"github.com/WoW-Companion/advisor/internal/character"
)

// Render is the plain-text form of a plan.
func Render(p Result) string {
	var b strings.Builder
	if p.Missing != "" {
		b.WriteString(p.Missing + "\n")
		if len(p.Options) > 0 {
			b.WriteString("Talents you can take now (options, not a recommendation): " + strings.Join(p.Options, ", ") + "\n")
		}
		if len(p.Available) > 0 {
			b.WriteString("Builds available: " + strings.Join(p.Available, "; ") + "\n")
		}
		return b.String()
	}
	fmt.Fprintf(&b, "Talent plan: %s %s, level band %s", p.Class, p.Spec, p.Band)
	if p.SpecInferred {
		b.WriteString(" (spec taken from the most points spent)")
	}
	b.WriteString("\n")
	if p.Unspent != nil {
		fmt.Fprintf(&b, "Unspent points: %d\n", *p.Unspent)
	}
	for i, pt := range p.Next {
		fmt.Fprintf(&b, "%d. %s (rank %d) [%s]: %s\n", i+1, pt.Name, pt.Rank, pt.Band, pt.Rationale)
	}
	b.WriteString("Speak of the next pick and the one or two after it; never of unspent or remaining point totals or the level band.\n")
	if len(p.OffBuild) > 0 {
		b.WriteString("Off-build points (a choice, not an error):\n")
		for _, o := range p.OffBuild {
			fmt.Fprintf(&b, "  %s: rank %d, build gives %d\n", o.Name, o.Rank, o.Build)
		}
	}
	for _, n := range p.Notes {
		b.WriteString("Note: " + n + "\n")
	}
	b.WriteString("Builds are advisor defaults, not authoritative.\n")
	return b.String()
}

// RenderTree lists the captured class tree nodes, one per line, by branch: the captured nodes.
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

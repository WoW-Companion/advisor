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
		if p.NoBuild {
			if p.Unspent != nil {
				fmt.Fprintf(&b, "Unspent points: %d\n", *p.Unspent)
			}
			for _, n := range p.Notes {
				b.WriteString("Note: " + n + "\n")
			}
			b.WriteString("Name no specific talent to pick and no build: the dataset has none checked for this spec.\n")
			return b.String()
		}
		if len(p.Options) > 0 {
			b.WriteString("Talents you can take now (options, not a recommendation): " + strings.Join(p.Options, ", ") + "\n")
		}
		if len(p.Available) > 0 {
			b.WriteString("Builds available: " + strings.Join(p.Available, "; ") + "\n")
		}
		return b.String()
	}
	fmt.Fprintf(&b, "Talent plan: %s %s, the Icy Veins WoW Forever leveling build", p.Class, p.Spec)
	switch {
	case p.SpecInferred:
		b.WriteString(" (spec taken from the most points spent)")
	case p.SpecDefault:
		b.WriteString(" (the class's default leveling spec)")
	}
	b.WriteString("\n")
	if p.Unspent != nil {
		fmt.Fprintf(&b, "Unspent points: %d\n", *p.Unspent)
	}
	b.WriteString(Progress(p) + "\n")
	for i, pt := range p.Next {
		fmt.Fprintf(&b, "%d. %s (rank %d of %d)", i+1, pt.Name, pt.Rank, pt.Max)
		if pt.Rationale != "" {
			b.WriteString(": " + pt.Rationale)
		}
		b.WriteString("\n")
	}
	b.WriteString("Speak of the next pick and the one or two after it; never of unspent or remaining point totals or the level.\n")
	if len(p.Priorities) > 0 {
		b.WriteString("The guide's key leveling talents, in priority order:\n")
		for _, pr := range p.Priorities {
			fmt.Fprintf(&b, "  %d. %s: %s\n", pr.Rank, pr.Name, pr.Reason)
		}
	}
	if n := OffBuildNote(p); n != "" {
		b.WriteString(n + " (a choice, not an error: never suggest a respec unless asked.)\n")
	}
	for _, n := range p.Notes {
		b.WriteString("Note: " + n + "\n")
	}
	b.WriteString("Talent picks come from this plan only, never from general knowledge. The build is Icy Veins' WoW Forever leveling guide.\n")
	return b.String()
}

// Progress is "Spirit Tap 5/5 done · next: Improved Shadow Word: Pain 1/2, ...": the last build point taken
// and the upcoming picks.
func Progress(p Result) string {
	var parts []string
	if p.Done != "" {
		parts = append(parts, p.Done+" done")
	}
	if len(p.Next) > 0 {
		var n []string
		for _, pt := range p.Next {
			n = append(n, fmt.Sprintf("%s %d/%d", pt.Name, pt.Rank, pt.Max))
		}
		parts = append(parts, "next: "+strings.Join(n, ", "))
	}
	return strings.Join(parts, " · ")
}

// OffBuildNote is the one plain sentence about points spent outside the build, "" when there are none.
func OffBuildNote(p Result) string {
	var out []string
	for _, o := range p.OffBuild {
		pts := fmt.Sprintf("%d points", o.Rank)
		if o.Rank == 1 {
			pts = "1 point"
		}
		if o.Build == 0 {
			out = append(out, fmt.Sprintf("You have %s in %s, which the Icy Veins leveling build skips", pts, o.Name))
		} else {
			out = append(out, fmt.Sprintf("You have %s in %s; the Icy Veins leveling build takes %d", pts, o.Name, o.Build))
		}
	}
	return strings.Join(out, ". ")
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

package talents

import (
	"fmt"
	"strings"
)

// Render is the plain-text form of a plan.
func Render(p Result) string {
	var b strings.Builder
	if p.Missing != "" {
		b.WriteString(p.Missing + "\n")
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
	if p.Remaining > len(p.Next) {
		fmt.Fprintf(&b, "%d more point(s) in the build after these.\n", p.Remaining-len(p.Next))
	}
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

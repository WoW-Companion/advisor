package professions

import (
	"fmt"
	"strconv"
	"strings"
)

func itoa(n int) string { return strconv.Itoa(n) }

func renderOption(b *strings.Builder, o Option, indent string) {
	if o.Learn != "" {
		fmt.Fprintf(b, "%sLearn it first: %s.\n", indent, o.Learn)
	}
	for _, n := range o.Needs {
		fmt.Fprintf(b, "%s%s: need %d, have %d", indent, n.Item, n.Total, n.Have)
		if n.Missing > 0 {
			fmt.Fprintf(b, ", missing %d, from %s", n.Missing, n.From)
		}
		b.WriteString("\n")
	}
}

// RenderPath is the plain-text answer: the next step first, then where you are, the bands, and prep.
func RenderPath(pa Path) string {
	var b strings.Builder
	if len(pa.Bands) > 0 && pa.Part == 1 {
		f := pa.Bands[0]
		fmt.Fprintf(&b, "Next: make ~%d × %s (skill %d→%d).\n\n", f.Best.Crafts, f.Best.Recipe, f.From, f.To)
	}
	fmt.Fprintf(&b, "%s: skill %d, goal %d.\n", pa.Profession, pa.Skill, pa.Target)
	for _, w := range pa.Where {
		b.WriteString(w + "\n")
	}
	for i, band := range pa.Bands {
		fmt.Fprintf(&b, "\n%d. Make ~%d × %s (skill %d→%d)", (pa.Part-1)*MaxBands+i+1, band.Best.Crafts, band.Best.Recipe, band.From, band.To)
		fmt.Fprintf(&b, ", %s there.\n", band.Best.Difficulty)
		renderOption(&b, band.Best, "   ")
		if band.Or != nil {
			fmt.Fprintf(&b, "   Or ~%d × %s:\n", band.Or.Crafts, band.Or.Recipe)
			renderOption(&b, *band.Or, "     ")
		}
	}
	if len(pa.Prep) > 0 {
		b.WriteString("\nBefore you start:\n")
		for _, p := range pa.Prep {
			b.WriteString("- " + p + "\n")
		}
	}
	for _, n := range pa.Notes {
		b.WriteString(n + "\n")
	}
	if pa.More {
		fmt.Fprintf(&b, "\nThat is part %d; ask for the next part to see the rest.\n", pa.Part)
	}
	return b.String()
}

package professions

import (
	"fmt"
	"strings"
)

// Render is the plain-text form of plans.
func Render(plans []Leveling) string {
	var b strings.Builder
	for i, p := range plans {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(p.Profession)
		if p.Skill != nil {
			fmt.Fprintf(&b, " (skill %d", *p.Skill)
			if p.Max != nil {
				fmt.Fprintf(&b, "/%d", *p.Max)
			}
			b.WriteString(")")
		} else {
			b.WriteString(" (skill unknown)")
		}
		b.WriteString("\n")
		for j, s := range p.Steps {
			fmt.Fprintf(&b, "%d. %s [%s]", j+1, s.Recipe.Name, s.Difficulty)
			if s.CreatesName != "" && s.CreatesName != s.Recipe.Name {
				fmt.Fprintf(&b, " makes %s", s.CreatesName)
			}
			if s.Recipe.LearnSkill != nil {
				fmt.Fprintf(&b, ", learn %d", *s.Recipe.LearnSkill)
			}
			if s.Recipe.YellowSkill != nil && s.Recipe.GreySkill != nil {
				fmt.Fprintf(&b, ", yellow %d, grey %d", *s.Recipe.YellowSkill, *s.Recipe.GreySkill)
			}
			b.WriteString("\n")
			if !s.ReagentsKnown {
				b.WriteString("   reagents unknown\n")
			}
			for _, r := range s.Reagents {
				fmt.Fprintf(&b, "   %s: need %d, have %d", r.Name, r.Need, r.Have)
				if r.Missing > 0 {
					fmt.Fprintf(&b, ", missing %d", r.Missing)
				}
				b.WriteString("\n")
			}
		}
		for _, n := range p.Notes {
			b.WriteString("Note: " + n + "\n")
		}
	}
	if len(plans) == 0 {
		return "No professions captured for this character.\n"
	}
	return b.String()
}

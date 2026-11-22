package route

import (
	"fmt"
	"strings"

	"github.com/WoW-Companion/advisor/internal/digest"
)

// Schema is the Plan.lua format version (docs/plan-format.md).
const Schema = "1.0"

// File is the file name, next to Digest.lua.
const File = "Plan.lua"

const header = "-- Data only: written by wowc-advisor; overwritten on every update. See docs/plan-format.md in the advisor repo.\n"

// Envelope is the whole file: one plan per character.
type Envelope struct {
	Generated int64
	Plans     []Plan
}

func stops(b *strings.Builder, indent string, list []Stop) {
	for _, s := range list {
		fmt.Fprintf(b, "%s{ n = %d, kind = %s, questID = %d, quest = %s, label = %s, map = %d, x = %.1f, y = %.1f, done = %t },\n",
			indent, s.N, digest.Quote(s.Kind), s.QuestID, digest.Quote(s.Quest), digest.Quote(s.Label), s.Map, s.X, s.Y, s.Done)
	}
}

// Lua renders the one data-only `WoWCompanionPlan = {...}` assignment.
func (e Envelope) Lua() string {
	var b strings.Builder
	b.WriteString(header)
	b.WriteString("WoWCompanionPlan = {\n")
	fmt.Fprintf(&b, "  schema = %s,\n  generated = %d,\n  characters = {\n", digest.Quote(Schema), e.Generated)
	for _, p := range e.Plans {
		fmt.Fprintf(&b, "    [%s] = {\n", digest.Quote(p.Character))
		if p.Start != nil {
			fmt.Fprintf(&b, "      start = { map = %d, x = %.1f, y = %.1f },\n", p.Start.Map, p.Start.X, p.Start.Y)
		}
		fmt.Fprintf(&b, "      zone = %s,\n      distance = %.1f,\n      steps = {\n", digest.Quote(p.Zone), p.Distance)
		stops(&b, "        ", p.Steps)
		b.WriteString("      },\n      later = {\n")
		stops(&b, "        ", p.Later)
		b.WriteString("      },\n      find = {\n")
		for _, f := range p.Find {
			fmt.Fprintf(&b, "        { n = %d, kind = \"objective\", questID = %d, quest = %s, label = %s, done = false },\n", f.N, f.QuestID, digest.Quote(f.Quest), digest.Quote(f.Label))
		}
		b.WriteString("      },\n    },\n")
	}
	b.WriteString("  },\n}\n")
	return b.String()
}

// Render is the plan as plain text for a prompt: the first limit steps and a count.
func Render(p Plan, limit int) string {
	if len(p.Steps) == 0 && len(p.Find) == 0 {
		if len(p.Notes) > 0 {
			return "Route plan: none. " + strings.Join(p.Notes, " ")
		}
		return "Route plan: no stops (nothing to do on this map)."
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Route plan (%d stops in %s, about %.0f map units; ordered by a distance optimiser, not guessed):\n", p.Total, cmpZone(p.Zone), p.Distance)
	for i, s := range p.Steps {
		if i >= limit {
			fmt.Fprintf(&b, "...and %d more stops\n", len(p.Steps)-limit)
			break
		}
		fmt.Fprintf(&b, "%d. %s (%.1f, %.1f)\n", s.N, s.Label, s.X, s.Y)
	}
	if len(p.Later) > 0 {
		fmt.Fprintf(&b, "%d stops are later (too high a level, off this map or past the 20-stop cap).\n", len(p.Later))
	}
	appendFind(&b, p)
	return strings.TrimRight(b.String(), "\n")
}

func appendFind(b *strings.Builder, p Plan) {
	for _, f := range p.Find {
		fmt.Fprintf(b, "Find: %s\n", f.Label)
	}
}

func cmpZone(z string) string {
	if z == "" {
		return "this zone"
	}
	return z
}

// Lines is the plan as short numbered lines for the overlay, with a one-line summary.
func Lines(p Plan, limit int) (lines []string, summary string) {
	for i, s := range p.Steps {
		if i >= limit {
			break
		}
		lines = append(lines, fmt.Sprintf("%s (%.0f, %.0f)", s.Label, s.X, s.Y))
	}
	for _, f := range p.Find {
		lines = append(lines, "Find: "+f.Label)
	}
	if len(p.Steps) > 0 {
		summary = fmt.Sprintf("%d stops, about %.0f map units", p.Total, p.Distance)
	}
	return lines, summary
}

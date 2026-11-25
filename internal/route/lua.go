package route

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/WoW-Companion/advisor/internal/digest"
	"github.com/WoW-Companion/advisor/internal/wording"
)

// Schema is the Plan.lua format version (docs/plan-format.md).
const Schema = "1.2"

// File is the file name, next to Digest.lua.
const File = "Plan.lua"

const header = "-- Data only: written by wowc-advisor; overwritten on every update. See docs/plan-format.md in the advisor repo.\n"

// Envelope is the whole file: one plan per character.
type Envelope struct {
	Generated int64
	Plans     []Plan
}

func idList(ids []int) string {
	var parts []string
	for _, id := range ids {
		parts = append(parts, strconv.Itoa(id))
	}
	return "{ " + strings.Join(parts, ", ") + " }"
}

func stops(b *strings.Builder, indent string, list []Stop) {
	for _, s := range list {
		extra := ""
		if s.Locked {
			extra += fmt.Sprintf(", locked = true, unlockedBy = %s, depth = %d", idList(s.UnlockedBy), s.Depth)
			if s.Chain != "" {
				extra += ", chain = " + digest.Quote(s.Chain)
			}
		}
		if len(s.QuestIDs) > 1 {
			extra += ", questIDs = " + idList(s.QuestIDs)
		}
		if len(s.Unlocks) > 0 {
			extra += ", unlocks = " + idList(s.Unlocks)
		}
		fmt.Fprintf(b, "%s{ n = %d, kind = %s, questID = %d, quest = %s, label = %s, map = %d, x = %.1f, y = %.1f, done = %t%s },\n",
			indent, s.N, digest.Quote(s.Kind), s.QuestID, digest.Quote(s.Quest), digest.Quote(s.Label), s.Map, s.X, s.Y, s.Done, extra)
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
		fmt.Fprintf(&b, "      basis = { captured = %d, inLog = %s },\n", p.Basis.Captured, idList(p.Basis.InLog))
		fmt.Fprintf(&b, "      zone = %s,\n      distance = %.1f,\n      steps = {\n", digest.Quote(p.Zone), p.Distance)
		stops(&b, "        ", p.Steps)
		b.WriteString("      },\n      later = {\n")
		stops(&b, "        ", p.Later)
		b.WriteString("      },\n      find = {\n")
		for _, f := range p.Find {
			fmt.Fprintf(&b, "        { n = %d, kind = \"objective\", questID = %d, quest = %s, label = %s, done = false },\n", f.N, f.QuestID, digest.Quote(f.Quest), digest.Quote(f.Label))
		}
		b.WriteString("      },\n      guidance = {\n")
		for _, g := range p.Guides {
			guideLua(&b, "        ", g)
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
	fmt.Fprintf(&b, "Route plan (%s in %s; ordered by a distance optimiser, not guessed):\n", wording.Count(p.Total, "stop", "stops"), cmpZone(p.Zone))
	for i, s := range p.Steps {
		if i >= limit {
			fmt.Fprintf(&b, "...and %d more stops\n", len(p.Steps)-limit)
			break
		}
		fmt.Fprintf(&b, "%d. %s (%.1f, %.1f)%s\n", s.N, s.Label, s.X, s.Y, lockedNote(s))
	}
	if len(p.Later) > 0 {
		fmt.Fprintf(&b, "%d stops are later (too high a level, off this map or past the 20-stop cap).\n", len(p.Later))
	}
	appendFind(&b, p)
	return strings.TrimRight(b.String(), "\n")
}

// lockedNote is the suffix of a locked step: it cannot be picked up until an earlier turn-in.
func lockedNote(s Stop) string {
	if !s.Locked {
		return ""
	}
	if s.After > 0 {
		return fmt.Sprintf(" [locked: after step %d]", s.After)
	}
	return " [locked]"
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

// StepText is one overlay line: the action, and for a locked step where it waits. The place is not
// in it: X and Y are separate fields for the UI to show dimly.
func StepText(s Stop) string {
	t := s.Label
	if s.Locked && s.After > 0 {
		t += fmt.Sprintf(" - after step %d", s.After)
	}
	return t
}

// Lines is the plan as short numbered lines for the overlay, with a one-line summary.
func Lines(p Plan, limit int) (lines []string, summary string) {
	for i, s := range p.Steps {
		if i >= limit {
			break
		}
		lines = append(lines, StepText(s))
	}
	for _, f := range p.Find {
		lines = append(lines, "Find: "+f.Label)
	}
	if len(p.Steps) > 0 {
		// The dataset and the capture hold no zone size, so there is no honest distance in yards: the
		// count alone is shown.
		summary = wording.Count(p.Total, "stop", "stops")
	}
	return lines, summary
}

func itemLua(it GuideItem) string {
	s := "text = " + digest.Quote(it.Text)
	if it.Map != 0 {
		s += fmt.Sprintf(", map = %d, x = %.1f, y = %.1f", it.Map, it.X, it.Y)
	}
	if it.Zone != "" {
		s += ", zone = " + digest.Quote(it.Zone)
	}
	return "{ " + s + " }"
}

// guideLua is one quest's "once accepted" block, keyed by its quest id.
func guideLua(b *strings.Builder, indent string, g Guide) {
	fmt.Fprintf(b, "%s[%d] = {\n%s  questID = %d, quest = %s, class = %t, steps = %s,\n", indent, g.QuestID, indent, g.QuestID, digest.Quote(g.Quest), g.Class, idList(g.Steps))
	fmt.Fprintf(b, "%s  objectives = {", indent)
	for i, o := range g.Objectives {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(b, "\n%s    %s", indent, itemLua(o))
	}
	if len(g.Objectives) > 0 {
		fmt.Fprintf(b, ",\n%s  ", indent)
	}
	b.WriteString("},\n")
	if g.TurnIn != nil {
		fmt.Fprintf(b, "%s  turnin = %s,\n", indent, itemLua(*g.TurnIn))
	}
	fmt.Fprintf(b, "%s  after = %s,\n%s},\n", indent, digest.Quote(g.After), indent)
}

package route

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/WoW-Companion/advisor/internal/digest"
	"github.com/WoW-Companion/advisor/internal/wording"
)

// Schema is the Plan.lua format version (docs/plan-format.md).
const Schema = "1.7"

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

// levelLua is the ", level = 38, diff = "red"" tail of an entry; empty when the level is unknown.
func levelLua(level int, diff string) string {
	if level <= 0 {
		return ""
	}
	out := fmt.Sprintf(", level = %d", level)
	if diff != "" {
		out += ", diff = " + digest.Quote(diff)
	}
	return out
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
		if s.Place != "" {
			extra += ", place = " + digest.Quote(s.Place)
		}
		extra += levelLua(s.Level, s.Diff)
		if s.Errand != "" {
			extra += ", errand = " + digest.Quote(s.Errand)
		}
		if s.Unplaced {
			extra += ", unplaced = true"
		}
		if s.Approx {
			extra += ", approx = true"
		}
		if s.Source == FromGame {
			extra += ", source = " + digest.Quote(FromGame)
		}
		if len(s.QuestIDs) > 1 {
			extra += ", questIDs = " + idList(s.QuestIDs)
		}
		if len(s.Unlocks) > 0 {
			extra += ", unlocks = " + idList(s.Unlocks)
		}
		if s.Trip {
			extra += ", trip = true, finisher = " + digest.Quote(s.Finisher) + ", finisherPlace = " + digest.Quote(s.FinisherPlace)
			if s.Instance != "" {
				extra += ", instance = " + digest.Quote(s.Instance)
			}
		}
		fmt.Fprintf(b, "%s{ n = %d, kind = %s, questID = %d, quest = %s, label = %s, map = %d, x = %.1f, y = %.1f, done = %t, after = %s%s },\n",
			indent, s.N, digest.Quote(s.Kind), s.QuestID, digest.Quote(s.Quest), digest.Quote(s.Label), s.Map, s.X, s.Y, s.Done, idList(s.Needs), extra)
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
		start := Point{X: NoPos, Y: NoPos}
		if p.Start != nil {
			start = *p.Start
		}
		fmt.Fprintf(&b, "      start = { map = %d, x = %.1f, y = %.1f, at = %d },\n", start.Map, start.X, start.Y, p.Basis.Captured)
		fmt.Fprintf(&b, "      basis = { captured = %d, inLog = %s },\n", p.Basis.Captured, idList(p.Basis.InLog))
		fmt.Fprintf(&b, "      zone = %s,\n      distance = %.1f,\n      steps = {\n", digest.Quote(p.Zone), p.Distance)
		stops(&b, "        ", p.Steps)
		b.WriteString("      },\n")
		if e := p.Exit; e != nil {
			fmt.Fprintf(&b, "      exit = { label = %s, map = %d, x = %.1f, y = %.1f, via = %s },\n", digest.Quote(e.Label), e.Map, e.X, e.Y, digest.Quote(e.Via))
		}
		b.WriteString("      trips = {\n")
		for _, t := range p.Trips {
			tripLua(&b, "        ", t)
		}
		b.WriteString("      },\n      parked = {\n")
		for _, k := range p.Parked {
			fmt.Fprintf(&b, "        { questID = %d, quest = %s, why = %s, kind = %s%s },\n", k.QuestID, digest.Quote(k.Quest), digest.Quote(k.Why), digest.Quote(k.Kind), levelLua(k.Level, k.Diff))
		}
		b.WriteString("      },\n      later = {\n")
		stops(&b, "        ", p.Later)
		b.WriteString("      },\n      find = {\n")
		for _, f := range p.Find {
			if f.Hint != "" {
				z := ""
				if f.Zone != "" {
					z = ", zone = " + digest.Quote(f.Zone)
				}
				fmt.Fprintf(&b, "        { n = %d, questID = %d, quest = %s, hint = %s%s%s, done = false },\n", f.N, f.QuestID, digest.Quote(f.Quest), digest.Quote(f.Hint), z, levelLua(f.Level, f.Diff))
				continue
			}
			fmt.Fprintf(&b, "        { n = %d, kind = \"objective\", questID = %d, quest = %s, label = %s%s, done = false },\n", f.N, f.QuestID, digest.Quote(f.Quest), digest.Quote(f.Label), levelLua(f.Level, f.Diff))
		}
		b.WriteString("      },\n")
		if len(p.Errands) > 0 {
			b.WriteString("      errands = {\n")
			for _, e := range p.Errands {
				fmt.Fprintf(&b, "        { n = %d, kind = %s, label = %s, map = %d, x = %.1f, y = %.1f, why = %s },\n", e.N, digest.Quote(e.Kind), digest.Quote(e.Label), e.Map, e.X, e.Y, digest.Quote(e.Why))
			}
			b.WriteString("      },\n")
		}
		b.WriteString("      flights = {\n")
		for _, f := range p.Flights {
			fmt.Fprintf(&b, "        { map = %d, x = %.1f, y = %.1f, name = %s, node = %s, known = %t },\n", f.Map, f.X, f.Y, digest.Quote(f.Name), digest.Quote(f.Node), f.Known)
		}
		b.WriteString("      },\n      guidance = {\n")
		for _, g := range p.Guides {
			guideLua(&b, "        ", g)
		}
		b.WriteString("      },\n      here = {\n")
		for _, h := range p.Here {
			hereLua(&b, "        ", h)
		}
		b.WriteString("      },\n      finishers = {\n")
		for _, f := range p.Finishers {
			who := "npc = " + digest.Quote(f.NPC)
			if f.Object != "" {
				who = "object = " + digest.Quote(f.Object)
			}
			trip := ""
			if f.Trip {
				trip = ", trip = true, finisher = " + digest.Quote(f.Finisher) + ", finisherPlace = " + digest.Quote(f.FinisherPlace)
				if f.Instance != "" {
					trip += ", instance = " + digest.Quote(f.Instance)
				}
			}
			fmt.Fprintf(&b, "        { questID = %d, quest = %s, %s, map = %d, x = %.1f, y = %.1f, place = %s%s },\n",
				f.QuestID, digest.Quote(f.Quest), who, f.Map, f.X, f.Y, digest.Quote(f.Place), trip)
		}
		b.WriteString("      },\n      zones = {\n")
		for _, z := range p.Zones {
			zoneLua(&b, "        ", z)
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
		fmt.Fprintf(&b, "%d. %s%s%s\n", s.N, s.Label, coordNote(s), lockedNote(s))
	}
	if len(p.Later) > 0 {
		fmt.Fprintf(&b, "%d stops are later (too high a level, off this map or past the 20-stop cap).\n", len(p.Later))
	}
	appendFind(&b, p)
	return strings.TrimRight(b.String(), "\n")
}

// coordNote is the stop's coordinates, or nothing for a stop whose position is not known.
func coordNote(s Stop) string {
	if s.ZoneOnly {
		return ""
	}
	return fmt.Sprintf(" (%.1f, %.1f)", s.X, s.Y)
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
		fmt.Fprintf(b, "%s\n", findLine(f))
	}
}

// findLine is a find entry as one line; a hint that already starts with a verb ("Find an ogre...") stands alone.
func findLine(f FindStop) string {
	if strings.HasPrefix(f.Label, "Find ") {
		return f.Label
	}
	return "Find: " + f.Label
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
		lines = append(lines, findLine(f))
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
	if it.Place != "" {
		s += ", place = " + digest.Quote(it.Place)
	}
	return "{ " + s + " }"
}

// guideLua is one quest's "once accepted" block, keyed by its quest id.
func guideLua(b *strings.Builder, indent string, g Guide) {
	fmt.Fprintf(b, "%s[%d] = ", indent, g.QuestID)
	guideBody(b, indent, g)
	b.WriteString(",\n")
}

// guideBody is the guide's table, without its key or a trailing comma.
func guideBody(b *strings.Builder, indent string, g Guide) {
	fmt.Fprintf(b, "{\n%s  questID = %d, quest = %s, class = %t, steps = %s,\n", indent, g.QuestID, digest.Quote(g.Quest), g.Class, idList(g.Steps))
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
	fmt.Fprintf(b, "%s  after = %s,\n%s}", indent, digest.Quote(g.After), indent)
}

// hereLua is one NPC's "here" block (Plan.lua 1.3).
func hereLua(b *strings.Builder, indent string, h Here) {
	fmt.Fprintf(b, "%s{\n%s  npc = %s, npcID = %d, map = %d, x = %.1f, y = %.1f, place = %s, steps = %s,\n", indent, indent,
		digest.Quote(h.NPC), h.NPCID, h.Map, h.X, h.Y, digest.Quote(h.Place), idList(h.Steps))
	var can []string
	for _, c := range h.Can {
		can = append(can, digest.Quote(c))
	}
	fmt.Fprintf(b, "%s  can = { %s },\n%s  quests = {\n", indent, strings.Join(can, ", "), indent)
	for _, q := range h.Quests {
		fmt.Fprintf(b, "%s    { questID = %d, quest = %s, role = %s, text = %s, missing = {", indent, q.QuestID, digest.Quote(q.Quest), digest.Quote(q.Role), digest.Quote(q.Text))
		for i, m := range q.Missing {
			if i > 0 {
				b.WriteString(",")
			}
			fmt.Fprintf(b, " %s", itemLua(m))
		}
		b.WriteString(" } },\n")
	}
	fmt.Fprintf(b, "%s  },\n%s  nearby = {\n", indent, indent)
	for _, n := range h.Nearby {
		fmt.Fprintf(b, "%s    [%d] = { questID = %d, quest = %s, giver = %s, map = %d, x = %.1f, y = %.1f, guidance = ", indent, n.QuestID, n.QuestID, digest.Quote(n.Quest), digest.Quote(n.Giver), n.Map, n.X, n.Y)
		guideBody(b, indent+"    ", n.Guidance)
		b.WriteString(" },\n")
	}
	fmt.Fprintf(b, "%s  },\n%s},\n", indent, indent)
}

// zoneLua is one zone's arrival and stop order, keyed by its map id (Plan.lua 1.5).
func zoneLua(b *strings.Builder, indent string, z ZonePlan) {
	a := z.Arrival
	arr := "kind = " + digest.Quote(a.Kind)
	if a.NPC != 0 {
		arr += fmt.Sprintf(", npc = %d", a.NPC)
	}
	if a.Name != "" {
		arr += ", name = " + digest.Quote(a.Name)
	}
	if a.Place != "" {
		arr += ", place = " + digest.Quote(a.Place)
	}
	if a.Known {
		arr += fmt.Sprintf(", map = %d, x = %.1f, y = %.1f", a.Map, a.X, a.Y)
	}
	fmt.Fprintf(b, "%s[%d] = { zone = %s, travel = %s, arrival = { %s }, order = %s },\n", indent, z.Map, digest.Quote(z.Zone), digest.Quote(z.Travel), arr, idList(z.Order))
}

// tripLua is one trip: the logged quests that wait in a zone outside the run (Plan.lua 1.6).
func tripLua(b *strings.Builder, indent string, t Trip) {
	extra := ""
	if t.Instance != "" {
		extra = ", instance = " + digest.Quote(t.Instance)
	}
	fmt.Fprintf(b, "%s{ order = %d, map = %d, zone = %s, ready = %d, todo = %d, pickups = %d, questIDs = %s, reason = %s, travel = %s, eta = %d%s },\n",
		indent, t.Order, t.Map, digest.Quote(t.Zone), t.Ready, t.Todo, t.Pickups, idList(t.QuestIDs), digest.Quote(t.Reason), digest.Quote(t.Travel), t.ETA, extra)
}

package character

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"
)

// NearFraction is the share of objectives done at which an open quest counts as nearly complete.
const NearFraction = 0.75

// RecentGiver is how recently a giver must have been met to be flagged as a recent interaction.
const RecentGiver = 7 * 24 * time.Hour

var leadingCount = regexp.MustCompile(`^\d+\s*/\s*\d+\b`)

var trailingCount = regexp.MustCompile(`:?\s*\d+\s*/\s*\d+\s*$`)

// ProgressText is the quest's objective progress, such as "Crab Season: 3/8 Crab Claws; 1/1 Boss".
// It is empty when the quest has no objective with a count.
func (q Quest) ProgressText() string {
	p := q.ObjectiveProgress()
	if p == "" {
		return ""
	}
	name := q.Title
	if name == "" {
		name = fmt.Sprintf("quest %d", q.QuestID)
	}
	return name + ": " + p
}

// ObjectiveProgress is ProgressText without the quest name: "3/8 Crab Claws; 1/1 Boss".
func (q Quest) ObjectiveProgress() string {
	var parts []string
	for _, o := range q.Objectives {
		if o.Have == nil || o.Need == nil || *o.Need <= 0 {
			continue
		}
		text := strings.TrimSpace(o.Text)
		if leadingCount.MatchString(text) {
			parts = append(parts, text) // ForeverScribe's text already starts with have/need
			continue
		}
		text = strings.TrimSpace(trailingCount.ReplaceAllString(text, ""))
		parts = append(parts, strings.TrimSpace(fmt.Sprintf("%d/%d %s", *o.Have, *o.Need, text)))
	}
	return strings.Join(parts, "; ")
}

// Label is the objective's text without its have/need count: "Windsong Crawler Meat".
func (o Objective) Label() string {
	text := strings.TrimSpace(o.Text)
	text = strings.TrimSpace(leadingCount.ReplaceAllString(text, ""))
	return strings.TrimSpace(trailingCount.ReplaceAllString(text, ""))
}

func (o Objective) finished() (done, known bool) {
	switch {
	case o.Done != nil:
		return *o.Done, true
	case o.Have != nil && o.Need != nil && *o.Need > 0:
		return *o.Have >= *o.Need, true
	}
	return false, false
}

// Fraction is the share of the quest's objectives that are done, and whether it is known. A
// complete quest is 1; a quest with no readable objectives is unknown.
func (q Quest) Fraction() (float64, bool) {
	if q.Complete != nil && *q.Complete {
		return 1, true
	}
	done, total := 0, 0
	for _, o := range q.Objectives {
		d, ok := o.finished()
		if !ok {
			continue
		}
		total++
		if d {
			done++
		}
	}
	if total == 0 {
		return 0, false
	}
	return float64(done) / float64(total), true
}

// NearlyDone reports a quest that is complete or has at least NearFraction of its objectives done.
func (q Quest) NearlyDone() bool {
	f, ok := q.Fraction()
	return ok && f >= NearFraction
}

// Position is the character's last known place: the saved location when it has coordinates,
// else (Approximate) where the most recent NPC interaction happened.
type Position struct {
	Zone, Subzone string
	MapID         *int
	X, Y          float64
	Approximate   bool
}

// LastKnownPosition is nil when neither a located save nor any NPC sighting exists.
func (c Character) LastKnownPosition() *Position {
	if l := c.Location; l != nil && l.X != nil && l.Y != nil {
		return &Position{Zone: l.Zone, Subzone: l.Subzone, MapID: l.MapID, X: *l.X, Y: *l.Y}
	}
	var best *NPC
	for i := range c.NPCs {
		n := &c.NPCs[i]
		if n.Seen == nil || n.X == nil || n.Y == nil {
			continue
		}
		if best == nil || *n.Seen > *best.Seen {
			best = n
		}
	}
	if best == nil {
		return nil
	}
	return &Position{Zone: best.Zone, Subzone: best.Subzone, MapID: best.MapID, X: *best.X, Y: *best.Y, Approximate: true}
}

// Label describes the position, marking an approximate one.
func (p Position) Label() string {
	where := p.Zone
	if p.Subzone != "" {
		where = p.Subzone + ", " + p.Zone
	}
	if where == "" {
		where = "unknown zone"
	}
	s := fmt.Sprintf("%s (%.1f, %.1f)", where, p.X, p.Y)
	if p.Approximate {
		s += " [approximate: from the most recent NPC you talked to]"
	}
	return s
}

// InZone reports whether a quest's zone name matches the position's zone or subzone.
func (p Position) InZone(zone string) bool {
	return zone != "" && (strings.EqualFold(zone, p.Zone) || strings.EqualFold(zone, p.Subzone))
}

// Giver is a quest-giver NPC the character has met, with its distance from the position.
type Giver struct {
	NPC
	Distance float64 // straight line in map percent
	Recent   bool    // met within RecentGiver
}

// NearbyGivers lists kind "quest" NPCs in the position's zone (same map when both ids are known),
// nearest first. A giver sighted at several spots is listed once.
func (c Character) NearbyGivers(now time.Time) []Giver {
	pos := c.LastKnownPosition()
	if pos == nil {
		return nil
	}
	var out []Giver
	seen := map[int]bool{}
	for _, n := range c.NPCs {
		if n.Kind != "quest" || n.X == nil || n.Y == nil {
			continue
		}
		if pos.MapID != nil && n.MapID != nil {
			if *pos.MapID != *n.MapID {
				continue
			}
		} else if !strings.EqualFold(n.Zone, pos.Zone) {
			continue
		}
		g := Giver{NPC: n, Distance: math.Hypot(*n.X-pos.X, *n.Y-pos.Y)}
		if n.Seen != nil {
			g.Recent = now.Sub(time.Unix(*n.Seen, 0)) <= RecentGiver
		}
		if n.ID != 0 {
			if seen[n.ID] {
				continue
			}
			seen[n.ID] = true
		}
		out = append(out, g)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Distance != out[j].Distance {
			return out[i].Distance < out[j].Distance
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// GiverLines renders up to max nearby givers, one line each; none when there are none.
func (c Character) GiverLines(now time.Time, max int) []string {
	gs := c.NearbyGivers(now)
	if max > 0 && len(gs) > max {
		gs = gs[:max]
	}
	var out []string
	for _, g := range gs {
		line := "Quest giver you've met nearby: " + g.Name
		if g.Subzone != "" {
			line += ", " + g.Subzone
		}
		line += fmt.Sprintf(" (%.1f, %.1f)", *g.X, *g.Y)
		if g.Recent {
			line += " — met in the last 7 days, may have quests"
		} else {
			line += " — may have quests"
		}
		out = append(out, line)
	}
	return out
}

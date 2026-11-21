// Package advice ranks what a captured character should do next. Every step names its source;
// a fact neither the capture nor the dataset holds is reported as unknown, never guessed.
// The rules are in docs/advice.md.
package advice

import (
	"fmt"
	"sort"
	"strings"

	"github.com/WoW-Companion/advisor/internal/brief"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/store"
)

// Step kinds.
const (
	KindTurnIn     = "turn_in"
	KindQuest      = "quest"
	KindTalent     = "talent"
	KindProfession = "profession"
	KindGear       = "gear"
)

// Quest level window around the character's level.
const (
	MaxBelow = 5 // a quest more than this far below is grey and skipped
	MaxAbove = 3 // a quest more than this far above is too hard and skipped
)

// Step is one piece of advice.
type Step struct {
	Kind    string   `json:"kind"`
	Title   string   `json:"title"`
	Reason  string   `json:"reason"`
	Zone    string   `json:"zone,omitempty"`
	Sources []string `json:"sources"`
}

const unknownZone = ""

// questInfo is a quest's level and zone, from the capture or else the dataset.
type questInfo struct {
	level   *int
	zone    string
	sources []string
}

func lookup(q character.Quest, ds store.DataStore) questInfo {
	info := questInfo{level: q.Level, zone: q.Zone, sources: []string{fmt.Sprintf("capture: quests[%d]", q.QuestID)}}
	if (info.level != nil && info.zone != "") || ds == nil || q.QuestID == 0 {
		return info
	}
	row, err := ds.Get("quests", int64(q.QuestID))
	if err != nil {
		return info
	}
	used := false
	if info.level == nil {
		if n, ok := row["level"].(int64); ok && n > 0 {
			l := int(n)
			info.level, used = &l, true
		}
	}
	if info.zone == "" {
		if s, ok := row["zone"].(string); ok && s != "" {
			info.zone, used = s, true
		}
	}
	if used {
		info.sources = append(info.sources, fmt.Sprintf("dataset: quests id %d", q.QuestID))
	}
	return info
}

func title(q character.Quest) string {
	if q.Title != "" {
		return q.Title
	}
	return fmt.Sprintf("quest %d", q.QuestID)
}

func isTrue(b *bool) bool { return b != nil && *b }

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// WhatsNext returns every step for the character, best first. ds may be nil (no dataset).
func WhatsNext(c character.Character, ds store.DataStore) []Step {
	var turnIns, quests, failed []Step
	type scored struct {
		step Step
		diff int // |quest level - character level|; -1 when the level is unknown
	}
	var open []scored
	for _, q := range c.Quests {
		info := lookup(q, ds)
		switch {
		case isTrue(q.Complete):
			turnIns = append(turnIns, Step{KindTurnIn, "Turn in " + title(q), "ready to turn in", info.zone, info.sources})
		case isTrue(q.Failed):
			failed = append(failed, Step{KindQuest, "Abandon " + title(q), "failed; it cannot be completed", info.zone, info.sources})
		default:
			s := Step{Kind: KindQuest, Title: title(q), Zone: info.zone, Sources: info.sources}
			switch {
			case info.level == nil:
				s.Reason = "quest level unknown"
				open = append(open, scored{s, -1})
			case c.Level == nil:
				s.Reason = fmt.Sprintf("level %d quest, your level is not captured", *info.level)
				open = append(open, scored{s, -1})
			default:
				d := *info.level - *c.Level
				if d < -MaxBelow || d > MaxAbove {
					continue
				}
				s.Reason = fmt.Sprintf("level %d quest, you are %d", *info.level, *c.Level)
				if d < 0 {
					d = -d
				}
				open = append(open, scored{s, d})
			}
		}
	}
	// Turn-ins: biggest zone first so one trip clears the most; unknown zone last.
	counts := map[string]int{}
	for _, s := range turnIns {
		counts[s.Zone]++
	}
	sort.SliceStable(turnIns, func(i, j int) bool {
		a, b := turnIns[i], turnIns[j]
		if a.Zone != b.Zone {
			if (a.Zone == unknownZone) != (b.Zone == unknownZone) {
				return b.Zone == unknownZone
			}
			if counts[a.Zone] != counts[b.Zone] {
				return counts[a.Zone] > counts[b.Zone]
			}
			return a.Zone < b.Zone
		}
		return a.Title < b.Title
	})
	sort.SliceStable(open, func(i, j int) bool {
		a, b := open[i], open[j]
		if (a.diff < 0) != (b.diff < 0) {
			return b.diff < 0
		}
		if a.diff != b.diff {
			return a.diff < b.diff
		}
		return a.step.Title < b.step.Title
	})
	for _, o := range open {
		quests = append(quests, o.step)
	}
	sort.SliceStable(failed, func(i, j int) bool { return failed[i].Title < failed[j].Title })

	out := turnIns
	if s, ok := talentStep(c); ok {
		out = append(out, s)
	}
	out = append(out, quests...)
	out = append(out, failed...)
	// Profession steps need trainer data the dataset does not hold yet; omitted, never guessed.
	return out
}

func talentStep(c character.Character) (Step, bool) {
	t := c.Talents
	if t == nil || t.Unspent == nil || *t.Unspent <= 0 {
		return Step{}, false
	}
	n := *t.Unspent
	s := Step{
		Kind:    KindTalent,
		Title:   fmt.Sprintf("Spend %d unspent talent %s", n, plural(n, "point", "points")),
		Reason:  "unspent points",
		Sources: []string{"capture: talents.unspent"},
	}
	if names, pts := brief.TopGroups(t); pts > 0 {
		if len(names) == 1 {
			s.Reason = fmt.Sprintf("unspent points; most points are in %s (%d)", names[0], pts)
		} else {
			s.Reason = fmt.Sprintf("unspent points; %s are tied at %d points", strings.Join(names, ", "), pts)
		}
		s.Sources = append(s.Sources, "capture: talents.trees")
	}
	return s, true
}

// Limit keeps the first n steps; n <= 0 keeps all.
func Limit(steps []Step, n int) []Step {
	if n > 0 && len(steps) > n {
		return steps[:n]
	}
	return steps
}

// Render is the plain-text rendering; total is the count before any limit.
func Render(c character.Character, steps []Step, total int) string {
	name := c.Name
	if name == "" {
		name = c.Key
	}
	var b strings.Builder
	fmt.Fprintf(&b, "What's next for %s", name)
	if c.Level != nil {
		fmt.Fprintf(&b, " (level %d)", *c.Level)
	}
	b.WriteString(":\n")
	if len(steps) == 0 {
		b.WriteString("Nothing to suggest from the capture and dataset (no quests to turn in, no unspent talents, no open quests in range).\n")
		return b.String()
	}
	for i, s := range steps {
		fmt.Fprintf(&b, "%d. [%s] %s: %s", i+1, s.Kind, s.Title, s.Reason)
		if s.Zone != "" {
			fmt.Fprintf(&b, " (zone: %s)", s.Zone)
		}
		fmt.Fprintf(&b, " [source: %s]\n", strings.Join(s.Sources, "; "))
	}
	if total > len(steps) {
		fmt.Fprintf(&b, "(%d more not shown)\n", total-len(steps))
	}
	return b.String()
}

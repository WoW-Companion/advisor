// Package advice ranks what a captured character should do next. Every step names its source;
// a fact neither the capture nor the dataset holds is reported as unknown, never guessed.
// The rules are in docs/advice.md.
package advice

import (
	"fmt"
	"github.com/WoW-Companion/advisor/internal/wording"
	"slices"
	"sort"
	"strings"

	"github.com/WoW-Companion/advisor/internal/brief"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/questie"
	"github.com/WoW-Companion/advisor/internal/quests"
	"github.com/WoW-Companion/advisor/internal/store"
	"github.com/WoW-Companion/advisor/internal/talents"
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
	Kind   string `json:"kind"`
	Title  string `json:"title"`
	Reason string `json:"reason"`
	Zone   string `json:"zone,omitempty"`
	// Progress is the quest's objective progress, such as "Crab Season: 3/8 Crab Claws".
	Progress string `json:"progress,omitempty"`
	QuestID  int    `json:"quest_id,omitempty"`
	// Location is where to go (an NPC or objective and its map percent), when the player's own
	// Questie could place it.
	Location string   `json:"location,omitempty"`
	Sources  []string `json:"sources"`

	// The structured parts of the step, for the digest; never part of the JSON the tools print.
	Name   string `json:"-"` // the quest's bare title
	Level  int    `json:"-"` // the quest's level, 0 when unknown
	NPC    string `json:"-"` // the giver of a pick-up, or the turn-in NPC
	Elite  bool   `json:"-"` // an elite or group quest
	Points int    `json:"-"` // unspent talent points, for a talent step
}

// AbandonPrefix starts the title of the step for a failed quest.
const AbandonPrefix = "Abandon "

const unknownZone = ""

// questInfo is a quest's level and zone, from the capture or else the dataset.
type questInfo struct {
	level   *int
	zone    string
	sources []string
	elite   bool
}

func lookup(q character.Quest, ds store.DataStore) questInfo {
	info := lookupBase(q, ds)
	info.elite = isElite(q, ds)
	return info
}

// isElite reads an optional boolean column of the dataset's quests table (elite, then group); false
// when the dataset has none.
func isElite(q character.Quest, ds store.DataStore) bool {
	if ds == nil || q.QuestID == 0 {
		return false
	}
	row, err := ds.Get("quests", int64(q.QuestID))
	if err != nil {
		return false
	}
	for _, col := range []string{"elite", "group"} {
		if b, ok := row[col].(bool); ok && b {
			return true
		}
	}
	return false
}

func lookupBase(q character.Quest, ds store.DataStore) questInfo {
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
		for _, col := range []string{"zone", "zone_name"} { // the built dataset names the column zone_name
			if s, ok := row[col].(string); ok && s != "" {
				info.zone, used = s, true
				break
			}
		}
	}
	if used {
		info.sources = append(info.sources, fmt.Sprintf("dataset: quests id %d", q.QuestID))
	}
	return info
}

// withChain appends the quest's place in its quest line to the step and reports whether turning
// it in unlocks a next step. Without a dataset or a line, the step is unchanged.
func withChain(s *Step, ds store.DataStore, q character.Quest, turnIn bool) bool {
	if ds == nil || q.QuestID == 0 {
		return false
	}
	c := quests.Of(ds, int64(q.QuestID))
	if !c.Found || c.Position == 0 {
		return false
	}
	s.Reason += fmt.Sprintf("; step %d of %d in %s", c.Position, c.Total(), c.Line)
	if turnIn && c.Unlocks() {
		s.Reason += fmt.Sprintf(", unlocks step %d", c.Position+1)
	}
	s.Sources = append(s.Sources, fmt.Sprintf("dataset: quest_lines line %d", c.LineID))
	return turnIn && c.Unlocks()
}

func title(q character.Quest) string {
	if q.Title != "" {
		return q.Title
	}
	return fmt.Sprintf("quest %d", q.QuestID)
}

func lvl(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

func isTrue(b *bool) bool { return b != nil && *b }

func plural(n int, one, many string) string { return wording.Plural(n, one, many) }

// WhatsNext returns every step for the character, best first. ds may be nil (no dataset).
func WhatsNext(c character.Character, ds store.DataStore) []Step { return WhatsNextWith(c, ds, nil) }

// WhatsNextWith is WhatsNext with the player's own QuestieDB, when they have one (nil otherwise):
// it supplies the pick-ups and the locations of the quests in the log.
func WhatsNextWith(c character.Character, ds store.DataStore, qx *questie.Index) []Step {
	var turnIns, openSteps, failed []Step
	unlocking := map[int]bool{} // quest ids of turn-ins that unlock the next chain step
	type scored struct {
		step Step
		diff int  // |quest level - character level|; -1 when the level is unknown
		near bool // complete or at least 75% of objectives done
		here bool // shares a zone or subzone with the last known position
	}
	pos := c.LastKnownPosition()
	var open []scored
	for _, q := range c.Quests {
		info := lookup(q, ds)
		switch {
		case isTrue(q.Complete):
			s := Step{Kind: KindTurnIn, Title: "Turn in " + title(q), Reason: "ready to turn in", Zone: info.zone, QuestID: q.QuestID, Sources: info.sources, Name: title(q), Level: lvl(info.level), Elite: info.elite}
			if withChain(&s, ds, q, true) {
				unlocking[q.QuestID] = true
			}
			turnIns = append(turnIns, s)
		case isTrue(q.Failed):
			failed = append(failed, Step{Kind: KindQuest, Title: AbandonPrefix + title(q), Reason: "failed; it cannot be completed", Zone: info.zone, QuestID: q.QuestID, Sources: info.sources})
		default:
			s := Step{Kind: KindQuest, Title: title(q), Zone: info.zone, QuestID: q.QuestID, Sources: info.sources, Name: title(q), Level: lvl(info.level), Elite: info.elite}
			chained := func() {
				withChain(&s, ds, q, false)
				if p := q.ProgressText(); p != "" {
					s.Progress = p
					s.Reason += "; " + p
				}
			}
			near, here := q.NearlyDone(), pos != nil && pos.InZone(info.zone)
			switch {
			case info.level == nil:
				s.Reason = "quest level unknown"
				chained()
				open = append(open, scored{s, -1, near, here})
			case c.Level == nil:
				s.Reason = fmt.Sprintf("level %d quest, your level is not captured", *info.level)
				chained()
				open = append(open, scored{s, -1, near, here})
			default:
				d := *info.level - *c.Level
				if d < -MaxBelow || d > MaxAbove {
					continue
				}
				s.Reason = fmt.Sprintf("level %d quest, you are %d", *info.level, *c.Level)
				chained()
				if d < 0 {
					d = -d
				}
				open = append(open, scored{s, d, near, here})
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
		if unlocking[a.QuestID] != unlocking[b.QuestID] {
			return unlocking[a.QuestID]
		}
		return a.Title < b.Title
	})
	sort.SliceStable(open, func(i, j int) bool {
		a, b := open[i], open[j]
		if a.near != b.near {
			return a.near
		}
		if a.here != b.here {
			return a.here
		}
		if (a.diff < 0) != (b.diff < 0) {
			return b.diff < 0
		}
		if a.diff != b.diff {
			return a.diff < b.diff
		}
		return a.step.Title < b.step.Title
	})
	for _, o := range open {
		openSteps = append(openSteps, o.step)
	}
	sort.SliceStable(failed, func(i, j int) bool { return failed[i].Title < failed[j].Title })

	avail, ok := questieAvailable(c, qx)
	if !ok {
		avail = AvailableQuests(c, ds)
	}
	// Milestones lead: class quests to pick up, then unspent class talent points, then profession
	// talent points; then the routine turn-ins, quests and pick-ups.
	var classQuests, pickUps []Step
	for _, s := range avail.Quests {
		if IsClassQuest(s) {
			classQuests = append(classQuests, s)
		} else {
			pickUps = append(pickUps, s)
		}
	}
	// Unspent class talent points come before everything: no travel, and the game nags until spent.
	var out []Step
	if s, ok := talentStep(c, ds); ok {
		out = append(out, s)
	}
	out = append(out, Limit(classQuests, MaxClassQuests)...)
	if s, ok := professionTalentStep(c); ok {
		out = append(out, s)
	}
	out = append(out, turnIns...)
	out = append(out, openSteps...)
	out = append(out, Limit(pickUps, MaxAvailable)...)
	out = append(out, failed...)
	locate(c, out, qx)
	// Profession steps need trainer data the dataset does not hold yet; omitted, never guessed.
	return out
}

// TalentAsk is the question that gets the player's talent picks.
const TalentAsk = `ask "What talents should I get?"`

// TalentsKnown reports whether the capture lists what the character has already picked.
func TalentsKnown(t *character.Talents) bool {
	if t == nil {
		return false
	}
	for _, tr := range t.Trees {
		if len(tr.Talents) > 0 || (tr.Points != nil && *tr.Points > 0) {
			return true
		}
		for _, g := range tr.Groups {
			if g.Spent > 0 {
				return true
			}
		}
	}
	return false
}

// TalentTitle is the top line for n unspent class talent points: the build's next pick, net of the
// ranks already spent, or a hedge when the capture cannot say what is spent.
func TalentTitle(c character.Character, ds store.DataStore, n int) string {
	spend := fmt.Sprintf("Spend %d talent %s", n, plural(n, "point", "points"))
	if !TalentsKnown(c.Talents) || talents.Unreadable(c, ds) {
		return spend + ". Open your talents (N); I can't see what you've already picked yet"
	}
	if ds != nil {
		if p := talents.Plan(c, ds, talents.Options{Limit: 1}); len(p.Next) > 0 {
			pick := p.Next[0].Name
			if p.Spec != "" {
				pick += " (" + p.Spec + ")"
			}
			return spend + ": " + pick
		}
	}
	return spend
}

func talentStep(c character.Character, ds store.DataStore) (Step, bool) {
	t := c.Talents
	if t == nil || t.Unspent == nil || *t.Unspent <= 0 {
		return Step{}, false
	}
	n := *t.Unspent
	s := Step{
		Kind:    KindTalent,
		Title:   TalentTitle(c, ds, n),
		Reason:  "milestone; " + TalentAsk,
		Sources: []string{"capture: talents.unspent"},
		Points:  n,
	}
	if names, pts := brief.TopGroups(t); pts > 0 {
		if len(names) == 1 {
			s.Reason += fmt.Sprintf("; most points are in %s (%d)", names[0], pts)
		} else {
			s.Reason += fmt.Sprintf("; %s are tied at %d points", strings.Join(names, ", "), pts)
		}
		s.Sources = append(s.Sources, "capture: talents.trees")
	}
	return s, true
}

// professionTalentStep is the milestone for unspent points in the profession trees; it ranks below
// the class one.
func professionTalentStep(c character.Character) (Step, bool) {
	n := c.Talents.ProfessionUnspent()
	if n <= 0 {
		return Step{}, false
	}
	return Step{
		Kind:    KindTalent,
		Title:   fmt.Sprintf("You have %d unspent profession talent %s", n, plural(n, "point", "points")),
		Reason:  "milestone; spend them in the profession's talent tree",
		Sources: []string{"capture: talents.trees (professions)"},
		Points:  n,
	}, true
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

// ByQuestOrder puts the items whose quest is in order into that order, each in a slot an on-route
// item held; everything else stays where it was. The panel's list and the answers' facts use it so
// they follow the route.
func ByQuestOrder[T any](items []T, quest func(T) int, order []int) []T {
	pos := map[int]int{}
	for i, id := range order {
		if _, ok := pos[id]; !ok {
			pos[id] = i
		}
	}
	var slots []int
	var moved []T
	for i, it := range items {
		if _, ok := pos[quest(it)]; ok && quest(it) != 0 {
			slots = append(slots, i)
			moved = append(moved, it)
		}
	}
	sort.SliceStable(moved, func(a, b int) bool { return pos[quest(moved[a])] < pos[quest(moved[b])] })
	out := slices.Clone(items)
	for k, i := range slots {
		out[i] = moved[k]
	}
	return out
}

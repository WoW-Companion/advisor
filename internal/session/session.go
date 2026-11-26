// Package session answers "I've got 30 minutes" and "is this quest worth it?". It turns the quest
// route into a loop that fits a time budget, and rates each quest in the log by XP for the time.
//
// The time model is deliberately simple and every number is a constant below:
//
//   - Travel: the straight-line distance between stops in map percent, times MapYards/100 yards
//     per percent, times PathFactor (roads and hills), at WalkSpeed yards a second (MountSpeed from
//     MountLevel).
//   - Each pick-up or turn-in costs TalkSecs.
//   - Each objective costs KillSecs per creature still to kill, or GatherSecs per item or object
//     still to collect or use, from the count in its label ("Kill Wolf (2/8)" is six kills).
//   - A trainer stop costs TrainerSecs; the loop ends at the nearest flight point or inn when the
//     session is short (up to ShortSession minutes), and that walk is counted.
//
// Nothing here promises minutes: a total is rounded to five and worded "about". Quest XP comes from
// the player's quest XP table when there is one, else from the quest's level (Estimate), and is cut
// the way the game cuts it for a quest well below the character's level.
package session

import (
	"fmt"
	"math"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/route"
)

// The time model.
const (
	WalkSpeed    = 7.0    // yards a second on foot (the game's run speed)
	MountSpeed   = 11.2   // yards a second mounted: the 60% ground mount of level 40 (the 100% mount costs far more)
	MountLevel   = 40     // the level a mount is assumed from
	MapYards     = 4000.0 // yards across a typical zone map (3,500 to 5,300 for the starting zones): one map percent is MapYards/100 yards
	PathFactor   = 1.3    // real paths are longer than the straight line
	TalkSecs     = 20.0   // a pick-up or turn-in
	KillSecs     = 25.0   // one creature: find, fight, recover
	GatherSecs   = 20.0   // one item or object
	TrainerSecs  = 60.0   // a trainer visit
	ShortSession = 45     // minutes; a session this short or shorter ends at a flight point or inn
	LogSize      = 20     // quests the log holds
	unknownMins  = 10.0   // the time a quest with no known place is ranked at
	FarPercent   = 60.0   // a quest this far away (map percent) from the start is far
)

// Info is what the planner learns about a quest from outside the route; every field is optional.
type Info struct {
	Level   func(id int) int               // the quest's level, 0 when unknown
	XP      func(id int) (xp int, ok bool) // the table's XP at the quest's own level
	Group   func(id int) bool              // elite objectives or a dungeon
	Upgrade func(id int) bool              // the reward is a gear upgrade
	Chain   func(id int) bool              // the quest starts a chain
}

// Anchor is a place to end a session: a flight point or an inn.
type Anchor struct {
	Name string
	Map  int
	X, Y float64
}

// In is everything a plan or a triage needs.
type In struct {
	Char    character.Character
	Route   route.Plan
	Info    Info
	Anchors []Anchor       // flight points and inns on the route's map
	Trainer *route.Stop    // the class trainer when training is due; nil otherwise
	Names   map[int]string // quest names, from the log
}

func (in In) level() int {
	if in.Char.Level != nil {
		return *in.Char.Level
	}
	return 1
}

func (in In) speed() float64 {
	if in.level() >= MountLevel {
		return MountSpeed
	}
	return WalkSpeed
}

// Estimate is the fallback quest XP from the quest's level alone: about 40 a level less 50, never
// under 30. It is a rough guide, and everything built on it says so.
func Estimate(level int) int { return max(40*level-50, 30) }

// Scale is the share of a quest's XP the game gives a character who is diff levels above it.
func Scale(diff int) float64 {
	switch {
	case diff <= 4:
		return 1
	case diff == 5:
		return 0.8
	case diff == 6:
		return 0.6
	case diff == 7:
		return 0.4
	case diff == 8:
		return 0.2
	}
	return 0.1
}

// QuestXP is the XP a quest gives this character, and whether it is an estimate from the level.
func (in In) QuestXP(id, level int) (xp int, estimated bool) {
	if level == 0 && in.Info.Level != nil {
		level = in.Info.Level(id)
	}
	if level == 0 {
		level = in.level()
	}
	base, ok := 0, false
	if in.Info.XP != nil {
		base, ok = in.Info.XP(id)
	}
	if !ok || base <= 0 {
		base, ok = Estimate(level), false
	}
	return int(math.Round(float64(base) * Scale(in.level()-level))), !ok
}

var counted = regexp.MustCompile(`\((\d+)/(\d+)\)\s*$`)

// units is how many creatures or items an objective stop still asks for.
func units(label string) int {
	if m := counted.FindStringSubmatch(label); m != nil {
		have, _ := strconv.Atoi(m[1])
		need, _ := strconv.Atoi(m[2])
		return max(1, min(need-have, 40))
	}
	return 1
}

// StopSecs is the time spent at a stop, not counting the walk to it.
func StopSecs(s route.Stop) float64 {
	switch s.Kind {
	case route.KindPickup, route.KindTurnIn:
		return TalkSecs
	case route.KindObjective:
		per := GatherSecs
		if strings.HasPrefix(strings.ToLower(s.Label), "kill") {
			per = KillSecs
		}
		return float64(units(s.Label)) * per
	}
	return 0
}

// WalkSecs is the time to go from a to b: zero across maps (a stop on another map is not planned).
func (in In) WalkSecs(a, b route.Point) float64 {
	if a.Map != b.Map {
		return 0
	}
	return math.Hypot(a.X-b.X, a.Y-b.Y) * MapYards / 100 * PathFactor / in.speed()
}

func at(s route.Stop) route.Point { return route.Point{Map: s.Map, X: s.X, Y: s.Y} }

// Step is one line of a session plan.
type Step struct {
	Label   string
	Kind    string
	Quest   string
	Minutes float64
}

// Skip is a quest left out of a plan and why.
type Skip struct {
	Quest, Why string
}

// Session is a plan that fits a time budget.
type Session struct {
	Budget    int // minutes asked for
	Steps     []Step
	Seconds   float64
	Quests    []string // quests finished by the plan
	XP        int
	SharePct  float64 // of the level; 0 when the level's size is unknown
	Estimated bool    // some XP is estimated from quest levels
	EndsAt    string  // the flight point or inn the loop ends at; "" when none
	Skipped   []Skip
	Notes     []string
}

// Minutes is the total in whole minutes, rounded to five (never under five).
func (s Session) Minutes() int { return RoundMinutes(s.Seconds) }

// RoundMinutes rounds seconds to the nearest five minutes, at least five.
func RoundMinutes(sec float64) int {
	return max(5, int(math.Round(sec/60/5))*5)
}

type job struct {
	id    int
	name  string
	stops []int // indexes into the route's steps, in order
	turn  bool  // has its turn-in on the route
	only  bool  // nothing but the turn-in: a quest ready to hand in
	xp    int
	est   bool
	group bool
	after []int // quests whose turn-in must come first
}

func (in In) jobs() []*job {
	byID := map[int]*job{}
	var order []*job
	for i, s := range in.Route.Steps {
		if s.Kind == route.KindTravel {
			continue
		}
		ids := s.QuestIDs
		if len(ids) == 0 && s.QuestID != 0 {
			ids = []int{s.QuestID}
		}
		for _, id := range ids {
			j := byID[id]
			if j == nil {
				j = &job{id: id, only: true}
				j.name = in.Names[id]
				if j.name == "" && len(ids) == 1 {
					j.name = s.Quest
				}
				byID[id] = j
				order = append(order, j)
			}
			j.stops = append(j.stops, i)
			if s.Kind == route.KindTurnIn {
				j.turn = true
			} else {
				j.only = false
			}
			if s.Locked {
				for _, a := range s.UnlockedBy {
					if !slices.Contains(j.after, a) {
						j.after = append(j.after, a)
					}
				}
			}
		}
	}
	for _, j := range order {
		lv := 0
		for _, q := range in.Char.Quests {
			if q.QuestID == j.id && q.Level != nil {
				lv = *q.Level
			}
		}
		j.xp, j.est = in.QuestXP(j.id, lv)
		j.group = in.Info.Group != nil && in.Info.Group(j.id)
		if j.name == "" {
			j.name = fmt.Sprintf("quest %d", j.id)
		}
	}
	return order
}

// loop is the time of the stops picked (indexes into the route's steps) walked in route order, with
// the trainer stop where it adds the least walking and the walk to an anchor at the end when asked.
// The text names the anchor.
func (in In) loop(picked map[int]bool, anchor bool) (secs float64, steps []Step, end string) {
	var seq []route.Stop
	for i, s := range in.Route.Steps {
		if picked[i] {
			seq = append(seq, s)
		}
	}
	var start *route.Point
	if in.Route.Start != nil {
		start = in.Route.Start
	}
	walk := func(s []route.Stop) float64 {
		var t float64
		var prev *route.Point = start
		for _, st := range s {
			if st.ZoneOnly { // no position is known: no walk is counted to it or from it
				t += StopSecs(st)
				continue
			}
			p := at(st)
			if prev != nil {
				t += in.WalkSecs(*prev, p)
			}
			t += StopSecs(st)
			prev = &p
		}
		return t
	}
	if tr := in.Trainer; tr != nil && len(seq) > 0 && (start == nil || tr.Map == start.Map) {
		best, bestT := 0, math.Inf(1)
		for i := 0; i <= len(seq); i++ {
			c := slices.Insert(slices.Clone(seq), i, *tr)
			if t := walk(c); t < bestT {
				best, bestT = i, t
			}
		}
		t := *tr
		t.Kind = "trainer"
		seq = slices.Insert(seq, best, t)
	}
	secs = walk(seq)
	if anchor && len(seq) > 0 && len(in.Anchors) > 0 {
		last := at(seq[len(seq)-1])
		bestD, bi := math.Inf(1), -1
		for i, a := range in.Anchors {
			if a.Map != last.Map {
				continue
			}
			if d := math.Hypot(a.X-last.X, a.Y-last.Y); d < bestD {
				bestD, bi = d, i
			}
		}
		if bi >= 0 {
			a := in.Anchors[bi]
			secs += in.WalkSecs(last, route.Point{Map: a.Map, X: a.X, Y: a.Y})
			end = a.Name
		}
	}
	var prev *route.Point = start
	for _, st := range seq {
		d := StopSecs(st)
		if !st.ZoneOnly {
			p := at(st)
			if prev != nil {
				d += in.WalkSecs(*prev, p)
			}
			prev = &p
		}
		steps = append(steps, Step{Label: stepLabel(st), Kind: st.Kind, Quest: st.Quest, Minutes: d / 60})
	}
	return secs, steps, end
}

func stepLabel(s route.Stop) string {
	if s.Kind == "trainer" {
		return "Visit your class trainer: " + s.Label
	}
	return s.Label
}

// Plan fits a session of minutes into the route: ready turn-ins first, then the quests that give the
// most XP for the time they add, never a quest that needs a group, ending at a flight point or inn
// when the session is short.
func Plan(in In, minutes int) Session {
	out := Session{Budget: minutes}
	budget := float64(minutes) * 60
	short := minutes <= ShortSession
	js := in.jobs()
	if in.Char.Level == nil || len(in.Route.Steps) == 0 {
		out.Notes = append(out.Notes, "There is no route to plan from yet.")
		return out
	}
	picked := map[int]bool{}
	chosen := map[int]*job{}
	used := func(j *job) map[int]bool {
		m := map[int]bool{}
		for k := range picked {
			m[k] = true
		}
		for _, i := range j.stops {
			m[i] = true
		}
		return m
	}
	cur, _, _ := in.loop(picked, short)
	eligible := func(j *job) bool {
		if chosen[j.id] != nil || !j.turn || j.group {
			return false
		}
		for _, a := range j.after {
			if chosen[a] == nil {
				return false
			}
		}
		return true
	}
	take := func(j *job) {
		for _, i := range j.stops {
			picked[i] = true
		}
		chosen[j.id] = j
		out.XP += j.xp
		out.Estimated = out.Estimated || j.est
		out.Quests = append(out.Quests, j.name)
	}
	// Ready turn-ins first: cheap and they pay at once.
	var ready []*job
	for _, j := range js {
		if j.only && eligible(j) {
			ready = append(ready, j)
		}
	}
	for _, j := range ready {
		if t, _, _ := in.loop(used(j), short); t <= budget {
			take(j)
			cur = t
		}
	}
	for {
		var best *job
		bestRate, bestT := -1.0, 0.0
		for _, j := range js {
			if !eligible(j) {
				continue
			}
			t, _, _ := in.loop(used(j), short)
			if t > budget {
				continue
			}
			if rate := float64(j.xp) / math.Max(t-cur, 30); rate > bestRate {
				best, bestRate, bestT = j, rate, t
			}
		}
		if best == nil {
			break
		}
		take(best)
		cur = bestT
	}
	out.Seconds, out.Steps, out.EndsAt = in.loop(picked, short)
	for _, j := range js {
		switch {
		case chosen[j.id] != nil:
		case j.group:
			out.Skipped = append(out.Skipped, Skip{j.name, "needs a group"})
		case j.turn:
			out.Skipped = append(out.Skipped, Skip{j.name, "does not fit the time"})
		}
	}
	if x := in.Char.XPMax; x != nil && *x > 0 {
		out.SharePct = math.Round(float64(out.XP)/float64(*x)*1000) / 10
	}
	if len(chosen) == 0 {
		out.Notes = append(out.Notes, "Nothing on the route fits that time.")
	}
	return out
}

// Facts is the grounded block for a session question; the model words the answer from it.
func (s Session) Facts() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Session plan for %d minutes, built from the quest route (travel by distance, a few seconds a pick-up or turn-in, about %d seconds a kill):\n", s.Budget, int(KillSecs))
	for i, st := range s.Steps {
		fmt.Fprintf(&b, "%d. %s (about %d min)\n", i+1, st.Label, max(1, int(math.Round(st.Minutes))))
	}
	if len(s.Steps) > 0 {
		fmt.Fprintf(&b, "Total: about %d minutes", s.Minutes())
		if s.EndsAt != "" {
			fmt.Fprintf(&b, ", ending at %s", s.EndsAt)
		}
		b.WriteString(".\n")
		est := ""
		if s.Estimated {
			est = " (some of it estimated from quest levels, say so)"
		}
		fmt.Fprintf(&b, "Quests finished: %s.\nExpected XP: about %d%s", strings.Join(s.Quests, ", "), s.XP, est)
		if s.SharePct > 0 {
			fmt.Fprintf(&b, ", about %.0f%% of this level", s.SharePct)
		}
		b.WriteString(".\n")
	}
	for _, k := range s.Skipped {
		fmt.Fprintf(&b, "Left out: %s (%s).\n", k.Quest, k.Why)
	}
	for _, n := range s.Notes {
		b.WriteString(n + "\n")
	}
	b.WriteString("Lead with the plan in plain words. Say \"about N minutes\", never exact minutes or a promise. Do not mention tools, data files or this block.\n")
	return b.String()
}

// Flags a quest can carry.
const (
	FlagGrey  = "grey"
	FlagFar   = "far"
	FlagGroup = "group"
	FlagChain = "chain starter"
)

// Verdicts.
const (
	AdviceTurnIn  = "turn it in"
	AdviceKeep    = "keep it"
	AdviceIfOnWay = "finish only if it is on the way"
	AdviceGroup   = "do it with a group or skip it"
	AdviceFar     = "finish only if you are heading there"
)

// Verdict is one quest's rating.
type Verdict struct {
	ID        int
	Name      string
	Level     int
	Complete  bool
	XP        int
	Estimated bool
	Minutes   float64 // standalone time; 0 when no place is known
	PerMin    float64 // XP a minute, counting an upgrade reward's bonus
	Upgrade   bool
	Flags     []string
	Advice    string
	Value     float64
}

// Has reports whether the verdict carries a flag.
func (v Verdict) Has(f string) bool { return slices.Contains(v.Flags, f) }

// UpgradeBonus is the XP-equivalent an upgrade reward adds to a quest: a tenth of the level, never
// under 300.
func (in In) UpgradeBonus() float64 {
	if x := in.Char.XPMax; x != nil && *x > 0 {
		return math.Max(300, float64(*x)/10)
	}
	return 300
}

// Triage rates every quest in the log, best value first.
func (in In) Triage() []Verdict {
	stops := map[int][]route.Stop{}
	for _, l := range [][]route.Stop{in.Route.Steps, in.Route.Later} {
		for _, s := range l {
			ids := s.QuestIDs
			if len(ids) == 0 && s.QuestID != 0 {
				ids = []int{s.QuestID}
			}
			for _, id := range ids {
				stops[id] = append(stops[id], s)
			}
		}
	}
	curMap, haveMap := 0, false
	if in.Route.Start != nil {
		curMap, haveMap = in.Route.Start.Map, true
	} else if len(in.Route.Steps) > 0 {
		curMap, haveMap = in.Route.Steps[0].Map, true
	}
	var out []Verdict
	for _, q := range in.Char.Quests {
		if q.Failed != nil && *q.Failed {
			continue
		}
		v := Verdict{ID: q.QuestID, Name: q.Title, Complete: q.Complete != nil && *q.Complete}
		if q.Level != nil {
			v.Level = *q.Level
		}
		v.XP, v.Estimated = in.QuestXP(q.QuestID, v.Level)
		if v.Level == 0 && in.Info.Level != nil {
			v.Level = in.Info.Level(q.QuestID)
		}
		v.Upgrade = in.Info.Upgrade != nil && in.Info.Upgrade(q.QuestID)
		sts := stops[q.QuestID]
		if v.Complete {
			var t []route.Stop
			for _, s := range sts {
				if s.Kind == route.KindTurnIn {
					t = append(t, s)
				}
			}
			sts = t
		}
		prev := in.Route.Start
		offMap, far := len(sts) > 0 && haveMap, false
		for i, s := range sts {
			p := at(s)
			if s.Map == curMap {
				offMap = false
			}
			if prev != nil {
				d := in.WalkSecs(*prev, p)
				v.Minutes += d / 60
				if i == 0 && prev.Map == p.Map && math.Hypot(prev.X-p.X, prev.Y-p.Y) > FarPercent {
					far = true
				}
			}
			v.Minutes += StopSecs(s) / 60
			prev = &p
		}
		if (far || offMap) && !v.Complete {
			v.Flags = append(v.Flags, FlagFar)
		}
		diff := in.level() - v.Level
		share := 100.0
		if x := in.Char.XPMax; x != nil && *x > 0 {
			share = float64(v.XP) / float64(*x) * 100
		}
		if v.Level > 0 && !v.Complete && (diff >= 6 || share < 1) {
			v.Flags = append(v.Flags, FlagGrey)
		}
		if !v.Complete && in.Info.Group != nil && in.Info.Group(q.QuestID) {
			v.Flags = append(v.Flags, FlagGroup)
		}
		if !v.Complete && !v.Has(FlagGrey) && in.Info.Chain != nil && in.Info.Chain(q.QuestID) {
			v.Flags = append(v.Flags, FlagChain)
		}
		mins := v.Minutes
		if mins <= 0 {
			mins = unknownMins
		}
		gain := float64(v.XP)
		if v.Upgrade {
			gain += in.UpgradeBonus()
		}
		v.PerMin = gain / mins
		v.Value = v.PerMin
		switch {
		case v.Complete:
			v.Advice = AdviceTurnIn
			v.Value = math.MaxFloat64 / 2
		case v.Has(FlagGroup):
			v.Advice = AdviceGroup
		case v.Has(FlagGrey) && !v.Upgrade:
			v.Advice = AdviceIfOnWay
		case v.Has(FlagFar) && !v.Upgrade:
			v.Advice = AdviceFar
		default:
			v.Advice = AdviceKeep
		}
		out = append(out, v)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Value > out[j].Value })
	return out
}

// Abandon picks the quests to drop to free room, weakest first: never a quest ready to hand in,
// one whose reward is an upgrade, or one that starts a chain. Flagged quests (grey, far, group) come
// first; only a full log is filled up with unflagged ones, so there is room to take new quests.
func Abandon(vs []Verdict, free int) []Verdict {
	var flagged, rest []Verdict
	for _, v := range vs {
		if v.Complete || v.Upgrade || v.Has(FlagChain) {
			continue
		}
		if v.Has(FlagGrey) || v.Has(FlagFar) || v.Has(FlagGroup) {
			flagged = append(flagged, v)
		} else {
			rest = append(rest, v)
		}
	}
	byValue := func(s []Verdict) {
		sort.SliceStable(s, func(i, j int) bool { return s[i].Value < s[j].Value })
	}
	byValue(flagged)
	byValue(rest)
	out := flagged
	if len(vs) >= LogSize {
		out = append(out, rest...)
	}
	if len(out) > free {
		out = out[:free]
	}
	return out
}

// Describe is the verdict as one grounded line.
func (v Verdict) Describe(in In) string {
	var b strings.Builder
	fmt.Fprintf(&b, "- %s (level %d)", v.Name, v.Level)
	xp := fmt.Sprintf("about %d XP", v.XP)
	if v.Estimated {
		xp += " (estimated from its level)"
	}
	if x := in.Char.XPMax; x != nil && *x > 0 {
		xp += fmt.Sprintf(", %.0f%% of a level", float64(v.XP)/float64(*x)*100)
	}
	b.WriteString(": " + xp)
	if v.Minutes > 0 {
		fmt.Fprintf(&b, ", about %d minutes", RoundMinutes(v.Minutes*60))
		fmt.Fprintf(&b, ", about %.0f XP a minute", v.PerMin)
	} else if !v.Complete {
		b.WriteString(", time not known (no place on the map)")
	}
	if v.Upgrade {
		b.WriteString(", rewards a gear upgrade")
	}
	if len(v.Flags) > 0 {
		b.WriteString(" [" + strings.Join(v.Flags, ", ") + "]")
	}
	b.WriteString(" -> " + v.Advice + "\n")
	return b.String()
}

// TriageFacts is the grounded block for "what should I abandon?", "my log is full" and "is <quest>
// worth it?" (name set): the rated quests and, for the first two, the quests to drop.
func (in In) TriageFacts(name string) string {
	vs := in.Triage()
	var b strings.Builder
	if len(vs) == 0 {
		return "The quest log in the capture is empty or was not captured.\n"
	}
	if name != "" {
		for _, v := range vs {
			if strings.Contains(strings.ToLower(v.Name), strings.ToLower(name)) {
				b.WriteString("Quest rating:\n" + v.Describe(in))
				b.WriteString(rules)
				return b.String()
			}
		}
		b.WriteString("That quest is not in the quest log, so it could not be rated; say so and offer to rate the log.\n")
		return b.String()
	}
	fmt.Fprintf(&b, "Quest log: %d of %d quests, rated by XP for the time (best first):\n", len(vs), LogSize)
	for _, v := range vs {
		b.WriteString(v.Describe(in))
	}
	if ab := Abandon(vs, 3); len(ab) > 0 {
		b.WriteString("Drop these first (weakest value, none ready to hand in, none a gear upgrade or chain start):\n")
		for _, v := range ab {
			fmt.Fprintf(&b, "- %s: %s\n", v.Name, strings.Join(v.Flags, ", "))
		}
	} else {
		b.WriteString("Nothing in the log is worth dropping yet.\n")
	}
	b.WriteString(rules)
	return b.String()
}

const rules = "Flags: grey = little XP, finish only if it is on the way; far = its work is in another zone; group = elite objectives or a dungeon, do it with a group or skip it; chain starter = worth keeping for what follows. Lead with the answer in plain words; say \"about N minutes\"; never claim exact minutes; do not mention tools or data files.\n"

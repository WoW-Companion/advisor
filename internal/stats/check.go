package stats

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/store"
)

// NeedsPlayStyle and Hook are ADV-093's structured answer for a character with no play style.
const (
	NeedsPlayStyle = "play_style"
	Hook           = "Choose a play style for a stat check"
)

// MaxInsights and MaxInsight bound what the player reads.
const (
	MaxInsights = 3
	MaxInsight  = 120
)

// LowLevel is the level below which the numbers are small and quest gear matters more than tuning.
const LowLevel = 20

// Sources of the totals.
const (
	SourceCaptured = "captured" // the addon's recorded totals
	SourceGear     = "gear"     // derived: how many worn items carry each stat
	SourceNone     = "none"
)

// Thresholds on shares of the five primary stats (internal; never shown): a stat is short by more than
// shortBy of the total, and "wasted" when its weight is at most wastedWeight yet it holds at least
// wastedShare.
const (
	shortBy      = 0.05
	wastedWeight = 0.15
	wastedShare  = 0.05
	fineWeight   = 0.3
)

// Result is a stat check. Insights are the player's words; Lean and the totals are internal.
type Result struct {
	Needs    string   `json:"needs,omitempty" jsonschema:"play_style when the character has no play style; offer the choices"`
	Hook     string   `json:"hook,omitempty"`
	Profile  string   `json:"profile,omitempty" jsonschema:"for example 'Fury · leveling'"`
	Insights []string `json:"insights,omitempty" jsonschema:"one to three plain sentences, ranked by impact"`
	Asks     []string `json:"asks,omitempty"`
	// Internal: for the answer's reasoning and the report, never to be quoted as numbers.
	Source string             `json:"source,omitempty" jsonschema:"captured totals, gear-only (derived from worn items) or none"`
	Lean   map[string]float64 `json:"lean,omitempty" jsonschema:"per stat, how far the wanted share is above (positive) or below the share held; internal"`
	Totals map[string]float64 `json:"totals,omitempty" jsonschema:"the numbers the check read; internal, never quote them"`
	Swap   []string           `json:"swap,omitempty" jsonschema:"worn items that carry a stat the play style barely uses, to swap first"`
	Weight map[string]float64 `json:"weights,omitempty" jsonschema:"the profile's stat weights after the preference; internal"`
	// Priority is the stats in order, the primary stat first; Suffixes the item suffixes that fit, best first.
	Priority []string `json:"priority,omitempty" jsonschema:"the stats that matter, the primary stat first; internal"`
	Suffixes []string `json:"suffixes,omitempty" jsonschema:"item suffixes that fit the priority, best first, such as 'of the Eagle'"`
}

// Prof is a character's profile and its adjusted weights: the one source every feature reads.
type Prof struct {
	Profile
	Weights map[string]float64 // after the stat preference
	Focus   string
}

// For is the profile and weights of a character with a play style; ok is false with none or for a
// class the table does not know. group is the character's top talent group, used when the play
// style names no spec.
func For(c character.Character, group string) (Prof, bool) {
	if c.Profile == nil {
		return Prof{}, false
	}
	spec := c.Profile.Spec
	if spec == "" {
		spec = group
	}
	p, ok := Lookup(c.Class, spec, c.Profile.Role)
	if !ok {
		return Prof{}, false
	}
	return Prof{Profile: p, Weights: Adjust(p.Weights, c.Profile.StatPref), Focus: c.Profile.Focus}, true
}

// Title is "Fury · leveling".
func (p Prof) Title() string {
	t := p.Label()
	if f := focusWord(p.Focus); f != "" {
		t += " · " + f
	}
	return t
}

func focusWord(f string) string {
	switch f {
	case "pvp":
		return "PvP"
	case "mixed":
		return "all-round play"
	}
	return f
}

// Asks are the digest's questions for the stat check.
func (p Prof) asks(top string) []string {
	out := []string{}
	if top != "" {
		out = append(out, fmt.Sprintf("How do I get more %s?", Name(top)))
	}
	if sp := p.Spec; sp != "" {
		out = append(out, fmt.Sprintf("Why does %s want %s?", sp, Name(p.top())))
	}
	if top != "" {
		return append(out, fmt.Sprintf("Which worn item costs me the most %s?", Name(top)))
	}
	return append(out, "Which stat matters most for my play style?")
}

// top is the stat the profile values most.
func (p Prof) top() string {
	best, bw := "", -1.0
	for _, s := range Primary {
		if v := p.Weights[s]; v > bw {
			best, bw = s, v
		}
	}
	return best
}

type held struct {
	have   map[string]float64 // per primary stat
	items  map[string][]string
	source string
}

// totalsOf reads the numbers to compare: the addon's totals when it recorded some, else how many
// worn items carry each stat (which needs the dataset).
func totalsOf(c character.Character, ds store.DataStore) held {
	h := held{have: map[string]float64{}, items: map[string][]string{}, source: SourceNone}
	if t := c.Totals; t.Any() {
		for s, v := range map[string]*float64{Strength: t.Str, Agility: t.Agi, Stamina: t.Sta, Intellect: t.Int, Spirit: t.Spi} {
			if v != nil && *v > 0 {
				h.have[s] = *v
			}
		}
		h.source = SourceCaptured
	}
	if ds == nil {
		return h
	}
	slots := make([]string, 0, len(c.Equipment))
	for s := range c.Equipment {
		slots = append(slots, s)
	}
	sort.Strings(slots)
	gear := map[string]float64{}
	for _, slot := range slots {
		eq := c.Equipment[slot]
		if eq.ItemID == nil || *eq.ItemID == 0 {
			continue
		}
		row, err := ds.Get("items", int64(*eq.ItemID))
		if err != nil || row["stats_status"] != "known" {
			continue
		}
		list, _ := row["stat_types"].([]any)
		for _, v := range list {
			id, _ := v.(int64)
			if s, ok := statByID[id]; ok {
				gear[s]++
				h.items[s] = append(h.items[s], eq.Name)
			}
		}
	}
	if h.source == SourceNone && len(gear) > 0 {
		h.have, h.source = gear, SourceGear
	}
	return h
}

// statByID maps the dataset's item stat type ids to the primary stats.
var statByID = map[int64]string{3: Agility, 4: Strength, 5: Intellect, 6: Spirit, 7: Stamina}

// Check reads the character's stats against its play style's profile. group is the character's top
// talent group name ("" when unknown). ds may be nil: only recorded totals are then read.
func Check(c character.Character, ds store.DataStore, group string) Result {
	if c.Profile == nil {
		return Result{Needs: NeedsPlayStyle, Hook: Hook}
	}
	p, ok := For(c, group)
	if !ok {
		return Result{Source: SourceNone, Insights: []string{"I have no stat priorities for this class yet."}}
	}
	r := Result{Profile: p.Title(), Source: SourceNone, Weight: p.Weights}
	h := totalsOf(c, ds)
	r.Source = h.source
	low := c.Level != nil && *c.Level < LowLevel
	// The head is the one priority: the primary stat first, then the item suffixes that carry it.
	var head []string
	if l := p.leadLine(Effects(c.Totals)); l != "" {
		head = append(head, l)
	}
	r.Priority = p.Order()
	if top := suffixesFor(p, c, ds); len(top) > 0 {
		head = append(head, suffixLine(top))
		for _, s := range top {
			r.Suffixes = append(r.Suffixes, s.Name)
		}
	}
	if h.source == SourceNone {
		r.Insights = append(head, "I can't see your stats yet, so I can't tell what to raise.")
		if low && len(r.Insights) < MaxInsights {
			r.Insights = append(r.Insights, lowLevelNote)
		}
		r.Asks = p.asks("")
		return r
	}
	var sumW, sumH float64
	for _, s := range Primary {
		sumW += p.Weights[s]
		sumH += h.have[s]
	}
	r.Totals, r.Lean = h.have, map[string]float64{}
	share := func(s string) float64 { return h.have[s] / sumH }
	var raise, fine string
	var raiseGap float64
	var wasted []string
	for _, s := range Primary {
		want := p.Weights[s] / sumW
		r.Lean[s] = round2(want - share(s))
		switch {
		case p.Weights[s] >= fineWeight && want-share(s) > shortBy && want-share(s) > raiseGap:
			raise, raiseGap = s, want-share(s)
		case p.Weights[s] <= wastedWeight && share(s) >= wastedShare && len(h.items[s]) > 0:
			wasted = append(wasted, s)
		case p.Weights[s] >= fineWeight && want-share(s) <= shortBy && (fine == "" || p.Weights[s] > p.Weights[fine]):
			fine = s
		}
	}
	label := p.Label()
	// Where the character stands: what to raise, what is wasted. The low-level note keeps its place.
	var standing []string
	if raise != "" {
		standing = append(standing, fit(
			fmt.Sprintf("%s wants more %s: %s.", label, Name(raise), p.Reason(raise)),
			fmt.Sprintf("More %s would help: %s.", Name(raise), p.Reason(raise)),
			fmt.Sprintf("%s wants more %s.", label, Name(raise))))
	}
	if len(wasted) > 0 {
		sort.SliceStable(wasted, func(i, j int) bool { return share(wasted[i]) > share(wasted[j]) })
		s := wasted[0]
		items := dedupe(h.items[s])
		r.Swap = items
		standing = append(standing, wastedLine(s, label, c.Class, items))
	}
	out := head
	room := MaxInsights - len(out)
	if low {
		room--
	}
	if len(standing) > room && raise != "" && raise == p.Lead() {
		standing = standing[1:] // the lead line already puts it first; keep the room for what to swap
	}
	for _, l := range standing {
		if room > 0 {
			out = append(out, l)
			room--
		}
	}
	if low {
		out = append(out, lowLevelNote)
	}
	if fine != "" && len(out) < MaxInsights {
		out = append(out, fit(fmt.Sprintf("%s is fine for %s.", Name(fine), forWhat(c.Profile.Focus))))
	}
	if len(out) == 0 {
		out = []string{"Your stats suit " + label + " well."}
	}
	r.Insights = out
	r.Asks = p.asks(raise)
	return r
}

const lowLevelNote = "Numbers are small at this level: quest gear matters more than fine-tuning."

func forWhat(focus string) string {
	switch focus {
	case "leveling":
		return "leveling"
	case "dungeons":
		return "dungeons"
	case "pvp":
		return "PvP"
	}
	return "how you play"
}

func wastedLine(stat, label, class string, items []string) string {
	n := len(items)
	noun := fmt.Sprintf("%d items give", n)
	them := "swap them first"
	if n == 1 {
		noun, them = "1 item gives", "swap it first"
	}
	who := "a " + strings.ToLower(class)
	if class == "" {
		who = "you"
	}
	named := ""
	if n > 0 {
		named = " (" + items[0] + ")"
	}
	return fit(
		fmt.Sprintf("%s %s, which %s barely uses: %s%s.", noun, Name(stat), who, them, named),
		fmt.Sprintf("%s %s, which %s barely uses: %s.", noun, Name(stat), who, them),
		fmt.Sprintf("%s %s, which %s barely uses.", noun, Name(stat), who),
		fmt.Sprintf("%s does little for %s: swap those items first.", Name(stat), who))
}

// fit is the first candidate within MaxInsight characters (the last one when none is).
func fit(cands ...string) string {
	for _, s := range cands {
		if utf8.RuneCountInString(s) <= MaxInsight {
			return s
		}
	}
	return cands[len(cands)-1]
}

func dedupe(l []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range l {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func round2(f float64) float64 { return float64(int(f*100+sign(f)*0.5)) / 100 }

func sign(f float64) float64 {
	if f < 0 {
		return -1
	}
	return 1
}

// equipWindow is how far above the character's level the suffix check looks for items.
const equipWindow = 10

// suffixesFor is the top two suffixes for the priority that the dataset confirms; none without a
// dataset or a level.
func suffixesFor(p Prof, c character.Character, ds store.DataStore) []Suffix {
	if ds == nil || c.Level == nil {
		return nil
	}
	rows, err := ds.Filter("items", store.Range{Column: "required_level", Min: 0, Max: int64(*c.Level + equipWindow)})
	if err != nil {
		return nil
	}
	return p.VerifiedTop(rows, 2)
}

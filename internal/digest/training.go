package digest

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/WoW-Companion/advisor/internal/advice"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/dungeons"
	"github.com/WoW-Companion/advisor/internal/gear"
	"github.com/WoW-Companion/advisor/internal/levelup"
	"github.com/WoW-Companion/advisor/internal/store"
	"github.com/WoW-Companion/advisor/internal/vendors"
	"github.com/WoW-Companion/advisor/internal/wording"
)

// KindTraining is the Kind of a class-training entry in next_quests (schema 1.4).
const KindTraining = "training"

// Caps for the per-entry context (schema 1.4).
const (
	MaxPrompt       = 500 // the in-game ask cap, in characters
	MaxDetails      = 4
	MaxTrainSpells  = 8
	maxDetailLength = 120
)

// TrainSpell is one trainer-taught ability or rank of a training entry.
type TrainSpell struct {
	ID   int
	Name string
	Rank int   // 0 when the ability has no rank
	New  bool  // false when it upgrades a known rank
	Cost int64 // copper; 0 when not known
}

// classIDs maps the capture's class token to the dataset's class_id.
var classIDs = map[string]int64{"WARRIOR": 1, "PALADIN": 2, "HUNTER": 3, "ROGUE": 4, "PRIEST": 5, "SHAMAN": 7, "MAGE": 8, "WARLOCK": 9, "DRUID": 11}

type row struct {
	TrainSpell
	level int
}

// trainingLine is the class-training entry for the levels after opt.PrevLevel up to the character's
// level, or for exactly the character's level when the earlier level is unknown. ok is false when
// the dataset has no class_spells table or nothing unlocks.
func trainingLine(c character.Character, ds store.DataStore, opt Options) (Line, bool) {
	if ds == nil || c.Level == nil {
		return Line{}, false
	}
	level := *c.Level
	from := level
	if opt.PrevLevel > 0 && opt.PrevLevel < level {
		from = opt.PrevLevel + 1
	}
	// With the known spells in the capture the entry is everything still untrained up to the
	// character's level; without them only the window of levels just reached.
	known := c.KnownSpells != nil
	if known {
		from = 1
	}
	sp := levelup.Spells(c, ds, from, level)
	var rs []row
	for _, x := range sp {
		rs = append(rs, row{TrainSpell{ID: int(x.ID), Name: x.Name, Rank: x.Rank, New: x.New, Cost: x.Cost}, x.Level})
	}
	if len(rs) == 0 {
		return Line{}, false
	}
	l := Line{Kind: KindTraining, Level: level}
	for _, r := range rs {
		if len(l.Spells) < MaxTrainSpells {
			l.Spells = append(l.Spells, r.TrainSpell)
		}
	}
	if len(l.Spells) == 0 {
		return Line{}, false
	}
	when := fmt.Sprintf("Level %d training", level)
	if from < level {
		when = fmt.Sprintf("Training, levels %d-%d", from, level)
	}
	var names []string
	for _, s := range l.Spells {
		names = append(names, spellLabel(s))
	}
	switch {
	case sp[0].Usually:
		when += " (usually)"
	case !known:
		when += " (if you don't have them yet)"
	}
	l.Text = clip(when + ": " + strings.Join(names, ", "))
	if known { // the player has untrained spells now: the NEXT bar says it is time to visit the trainer
		l.Text = clip("New spells to train: " + strings.Join(names[:min(2, len(names))], ", "))
	}
	l.trainer(c)
	place, hasPlace := levelup.TrainAt(c, opt.Questie)
	if hasPlace { // the one trainer choice: the panel, the NEXT line and the facts all follow it
		l.NPC, l.X, l.Y, l.HasXY = "", 0, 0, false
		l.Zone = place.City
		if t := place.Trainer; t != nil {
			l.NPC, l.Zone, l.X, l.Y, l.HasXY = t.Name, t.Zone, t.X, t.Y, true
		}
	}
	if hasPlace {
		l.trainAt = place.Line(c.Class)
		if known { // the NEXT bar names the same place the card does
			l.Text = clip(l.Text + " at " + place.City)
		}
	}
	l.Details = trainingDetails(l, from, c, ds, sp)
	l.Asks, l.Ref = trainingAsks(l.Spells), trainingRef(level)
	l.Why = trainingWhy(l.Spells)
	return l, true
}

func spellLabel(s TrainSpell) string {
	if s.Rank > 0 {
		return fmt.Sprintf("%s (Rank %d)", s.Name, s.Rank)
	}
	return s.Name
}

// trainer sets the entry's trainer from the NPCs the capture holds, only when one is named for the
// character's class ("<Class> Trainer"). The nearest one is the one in the character's zone, else
// the most recently seen. Nothing is guessed: with none the fields stay empty.
func (l *Line) trainer(c character.Character) {
	want := strings.ToLower(c.Class) + " trainer"
	var best *character.NPC
	for i := range c.NPCs {
		n := &c.NPCs[i]
		if !strings.Contains(strings.ToLower(n.Name), want) {
			continue
		}
		here := c.Location != nil && c.Location.Zone != "" && strings.EqualFold(n.Zone, c.Location.Zone)
		bestHere := best != nil && c.Location != nil && c.Location.Zone != "" && strings.EqualFold(best.Zone, c.Location.Zone)
		switch {
		case best == nil, here && !bestHere:
			best = n
		case here == bestHere && n.Seen != nil && (best.Seen == nil || *n.Seen > *best.Seen):
			best = n
		}
	}
	if best == nil {
		return
	}
	l.NPC, l.Zone = best.Name, best.Zone
	if best.X != nil && best.Y != nil {
		l.X, l.Y, l.HasXY = *best.X, *best.Y, true
	}
}

func trainingDetails(l Line, from int, c character.Character, ds store.DataStore, sp []levelup.Spell) []string {
	var d []string
	var fresh, up []string
	usually := len(sp) > 0 && sp[0].Usually
	for _, s := range l.Spells {
		if s.New {
			fresh = append(fresh, spellLabel(s))
		} else {
			up = append(up, spellLabel(s))
		}
	}
	if len(fresh) > 0 {
		lead := "New: "
		if c.KnownSpells == nil { // the capture lists no known spells: nothing is definitely new
			lead = "New, if you don't have them yet: "
		}
		if usually {
			lead = "Usually trained: "
		}
		d = append(d, lead+strings.Join(fresh, ", "))
	}
	if len(up) > 0 {
		d = append(d, "Upgrades: "+strings.Join(up, ", "))
	}
	for _, q := range levelup.QuestSpells(c, ds, from, l.Level) {
		d = append(d, q.Line())
	}
	if l.trainAt != "" {
		d = append(d, l.trainAt)
	} else if l.NPC != "" {
		w := "Trainer: " + l.NPC
		if l.Zone != "" {
			w += ", " + l.Zone
		}
		if l.HasXY {
			w += fmt.Sprintf(" (%.0f, %.0f)", l.X, l.Y)
		}
		d = append(d, w)
	}
	if total, _ := levelup.Total(sp); total > 0 {
		cost := "Bring about " + vendors.Money(total)
		if m := levelup.MoneyLine(c, total); m != "" {
			cost += "; you are short, sell from the Bags review"
		}
		d = append(d, cost)
	}
	if from < l.Level {
		d = append(d, fmt.Sprintf("Unlocked between levels %d and %d", from, l.Level))
	} else {
		d = append(d, "Available at your level")
	}
	return finishDetails(d)
}

// finishDetails keeps at most MaxDetails non-empty lines of at most maxDetailLength characters, each
// cut at a word boundary with no ellipsis.
func finishDetails(d []string) []string {
	var out []string
	for _, s := range d {
		if s = strings.TrimSpace(s); s != "" && len(out) < MaxDetails {
			out = append(out, wholeCut(s, maxDetailLength))
		}
	}
	return out
}

// danglers are words a sentence does not end on: a line that stops at one was cut.
var danglers = map[string]bool{"a": true, "an": true, "the": true, "of": true, "or": true, "and": true, "to": true, "for": true,
	"with": true, "in": true, "on": true, "at": true, "from": true, "by": true, "your": true, "my": true, "into": true, "as": true,
	"is": true, "are": true, "that": true, "than": true, "then": true, "but": true, "if": true, "so": true}

// wholeCut is s when it fits n characters; else its last whole sentence that fits (when that keeps at
// least half), else its words up to n with any dangling connector word removed. It never adds "...".
func wholeCut(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	cut := wording.CutWords(s, n)
	if i := strings.LastIndexAny(cut, ".!?"); i >= len(cut)/2 && i+1 < len(cut) {
		return cut[:i+1]
	}
	for {
		w := strings.Fields(cut)
		if len(w) < 2 {
			return cut
		}
		last := strings.ToLower(strings.Trim(w[len(w)-1], ".,;:!?()'\""))
		if !danglers[last] {
			return strings.TrimRight(cut, " ,;:-")
		}
		cut = strings.TrimRight(strings.TrimSuffix(cut, w[len(w)-1]), " ,;:-")
	}
}

// clipTo shortens s to n characters, ending in "...".
func clipTo(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return strings.TrimRight(string([]rune(s)[:n-3]), " ") + "..."
}

// style is the stored play style in words, for example "Arms, questing", or "" when the player has
// set none.
func style(c character.Character) string {
	var parts []string
	spec := ""
	if p := c.Profile; p != nil {
		spec = p.Spec
	}
	if spec != "" {
		parts = append(parts, spec)
	}
	if p := c.Profile; p != nil {
		if f := map[string]string{"leveling": "questing", "dungeons": "running dungeons", "pvp": "PvP", "mixed": "mixed play"}[p.Focus]; f != "" {
			parts = append(parts, f)
		}
		if p.Role != "" && p.Role != "balanced" && p.Role != "damage" {
			parts = append(parts, p.Role+" role")
		}
	}
	return strings.Join(parts, ", ")
}

// who is the character as the stored profile and capture describe it, for example "a level 10
// Skyborn warrior playing Arms, questing".
func who(c character.Character) string {
	var b strings.Builder
	b.WriteString("a")
	if c.Level != nil {
		fmt.Fprintf(&b, " level %d", *c.Level)
	}
	if c.Race != "" {
		b.WriteString(" " + c.Race)
	}
	class := strings.ToLower(c.Class)
	if class == "" {
		class = "adventurer"
	}
	b.WriteString(" " + class)
	if s := style(c); s != "" {
		b.WriteString(" playing " + s)
	}
	return b.String()
}

// spot is where a named NPC stands in the capture, " (42, 31)", or "".
func spot(c character.Character, npc string) string {
	for _, n := range c.NPCs {
		if n.Name == npc && n.X != nil && n.Y != nil {
			return fmt.Sprintf(" (%.0f, %.0f)", *n.X, *n.Y)
		}
	}
	return ""
}

// questContext is the details of a quest line: who and where, what is left or unlocked,
// and how the quest fits the player's level.
func questContext(c character.Character, l Line, reason string) []string {
	title := l.Title
	if title == "" {
		title = strings.TrimPrefix(l.Text, "Pick up: ")
	}
	var d []string
	where := ""
	switch {
	case l.Kind == KindTurnIn && l.NPC != "":
		where = "Turn in to " + l.NPC
	case l.Kind == KindPickup && l.NPC != "":
		where = "Pick up from " + l.NPC
	}
	if where != "" {
		where += spot(c, l.NPC)
	}
	if l.Zone != "" {
		if where != "" {
			where += " in " + l.Zone
		} else {
			where = "In " + l.Zone
		}
	}
	d = append(d, where)
	var open []string
	for _, o := range l.Objectives {
		if o.HasCount && o.Have >= o.Need {
			continue
		}
		t := o.Text
		if o.HasCount {
			t += fmt.Sprintf(" %d/%d", o.Have, o.Need)
		}
		open = append(open, t)
	}
	if len(open) > 0 {
		d = append(d, "Still to do: "+strings.Join(open, "; "))
	}
	if l.Kind != KindPickup {
		for _, q := range c.Quests {
			if q.QuestID == l.Quest && l.Quest != 0 {
				r := q.Readiness()
				if strings.HasSuffix(r, "turn in") {
					r += map[bool]string{true: " now", false: " yet"}[q.Ready()]
				}
				d = append(d, "Quest status: "+r)
			}
		}
	}
	if l.Kind == KindTurnIn && len(l.Unlocks) > 0 {
		d = append(d, "Unlocks: "+strings.Join(l.Unlocks, "; "))
	}
	fit := ""
	if c.Level != nil {
		fit = wording.Fit(l.Level, *c.Level)
	}
	if l.Class {
		fit = strings.TrimSpace(fit + ", a class quest")
		fit = strings.TrimPrefix(fit, ", ")
	}
	if l.Elite {
		fit = strings.TrimPrefix(fit+", needs a group", ", ")
	}
	d = append(d, fit)
	return finishDetails(d)
}

// talentStepContext is the details of the unspent-talents line. pick is the plan's next
// pick (Describe form) and name its bare node, both "" when there is no plan or dataset pick.
func talentStepContext(c character.Character, s advice.Step, pick, name string) []string {
	what := "talent point"
	if strings.Contains(s.Title, "profession") {
		what = "profession talent point"
	}
	var d []string
	if s.Points > 0 {
		d = append(d, wording.Count(s.Points, what, what+"s")+" to spend")
	}
	if pick != "" {
		d = append(d, "Next pick: "+pick)
	}
	return finishDetails(d)
}

// talentContext is the details of a talents entry: the node, its rank and its branch.
func talentContext(c character.Character, name string, rank int, group string) []string {
	d := []string{fmt.Sprintf("Next pick: %s, rank %d", name, rank)}
	if group != "" {
		d = append(d, "Branch: "+group)
	}
	return finishDetails(d)
}

// dungeonFit is the dungeon's level range with how it sits against the player, in words.
func dungeonFit(c character.Character, p dungeons.DungeonPick) string {
	if p.MinLevel == nil || p.MaxLevel == nil {
		return "The dungeon's level range is unknown"
	}
	s := fmt.Sprintf("For levels %d to %d", *p.MinLevel, *p.MaxLevel)
	switch {
	case p.Fit <= 0:
		return s + ", right for your level"
	case c.Level != nil && *c.Level < *p.MinLevel:
		return s + ", " + wording.Levels(p.Fit) + " above you"
	}
	return s + ", " + wording.Levels(p.Fit) + " below you"
}

// dungeonContext is the details of a dungeons entry.
func dungeonContext(c character.Character, p dungeons.DungeonPick, d dungeons.Drop) []string {
	det := []string{dungeonFit(c, p)}
	switch {
	case d.Item != "" && d.Boss != "":
		det = append(det, fmt.Sprintf("%s drops %s", d.Boss, d.Item))
	case d.Item != "":
		det = append(det, "Best upgrade: "+d.Item)
	}
	if d.Slot != "" && d.Item != "" {
		det = append(det, "It fits your "+wording.Slot(d.Slot)+" slot")
	}
	if n := len(p.Quests); n > 0 {
		det = append(det, wording.Count(n, "of your open quests leads", "of your open quests lead")+" here")
	}
	return finishDetails(det)
}

// upgradeContext is the details of an upgrades entry: the item, its source and the gain.
func upgradeContext(c character.Character, slot string, cd gear.Candidate) []string {
	item := cd.Item.Name
	if item == "" {
		item = "an unnamed item"
	}
	d := []string{fmt.Sprintf("%s: %s, item level %d", wording.Upper(wording.Slot(slot)), item, cd.Item.ItemLevel)}
	if cur, ok := c.Equipment[slot]; ok && cur.ItemLevel != nil && !cd.Empty {
		gain := int(cd.Item.ItemLevel) - *cur.ItemLevel
		name := cur.Name
		if name == "" {
			name = "your current item"
		}
		d = append(d, fmt.Sprintf("%+d item %s over %s", gain, wording.Plural(gain, "level", "levels"), name))
	} else if cd.Empty {
		d = append(d, "Your slot is empty")
	}
	for _, s := range cd.Sources {
		if src := wording.Source(s.Kind, s.Name); src != "" {
			d = append(d, wording.Upper(src))
			break
		}
	}
	if cd.Soon {
		d = append(d, fmt.Sprintf("Usable at level %d", cd.Item.ReqLevel))
	}
	return finishDetails(d)
}

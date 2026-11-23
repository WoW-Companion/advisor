package digest

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/WoW-Companion/advisor/internal/advice"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/dungeons"
	"github.com/WoW-Companion/advisor/internal/gear"
	"github.com/WoW-Companion/advisor/internal/store"
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
	Rank int  // 0 when the ability has no rank
	New  bool // false when it upgrades a known rank
}

// classIDs maps the capture's class token to the dataset's class_id.
var classIDs = map[string]int64{"WARRIOR": 1, "PALADIN": 2, "HUNTER": 3, "ROGUE": 4, "PRIEST": 5, "SHAMAN": 7, "MAGE": 8, "WARLOCK": 9, "DRUID": 11}

// trainingLine is the class-training entry for the levels after opt.PrevLevel up to the character's
// level, or for exactly the character's level when the earlier level is unknown. ok is false when
// the dataset has no class_spells table or nothing unlocks.
func trainingLine(c character.Character, ds store.DataStore, opt Options) (Line, bool) {
	cid, known := classIDs[strings.ToUpper(c.Class)]
	if ds == nil || !known || c.Level == nil {
		return Line{}, false
	}
	level := *c.Level
	from := level
	if opt.PrevLevel > 0 && opt.PrevLevel < level {
		from = opt.PrevLevel + 1
	}
	rows, err := ds.Filter("class_spells", store.Range{Column: "class_id", Min: cid, Max: cid}, store.Range{Column: "level", Min: int64(from), Max: int64(level)})
	if err != nil || len(rows) == 0 {
		return Line{}, false
	}
	type row struct {
		TrainSpell
		level int
	}
	var rs []row
	seen := map[int]bool{}
	for _, r := range rows {
		id, _ := r["spell_id"].(int64)
		name, _ := r["name"].(string)
		lv, _ := r["level"].(int64)
		if id == 0 || name == "" || seen[int(id)] {
			continue
		}
		seen[int(id)] = true
		rank, _ := r["rank"].(int64)
		sup, _ := r["superseded_spell_id"].(int64)
		rs = append(rs, row{TrainSpell{ID: int(id), Name: name, Rank: int(rank), New: sup == 0}, int(lv)})
	}
	sort.SliceStable(rs, func(i, j int) bool {
		if rs[i].level != rs[j].level {
			return rs[i].level < rs[j].level
		}
		return rs[i].Name < rs[j].Name
	})
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
	l.Text = clip(when + ": " + strings.Join(names, ", "))
	l.trainer(c)
	l.Details = trainingDetails(l, from)
	l.Prompt = trainingPrompt(c, l, from)
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

func trainingDetails(l Line, from int) []string {
	var d []string
	var fresh, up []string
	for _, s := range l.Spells {
		if s.New {
			fresh = append(fresh, spellLabel(s))
		} else {
			up = append(up, spellLabel(s))
		}
	}
	if len(fresh) > 0 {
		d = append(d, "New: "+strings.Join(fresh, ", "))
	}
	if len(up) > 0 {
		d = append(d, "Upgrades: "+strings.Join(up, ", "))
	}
	if l.NPC != "" {
		w := "Trainer: " + l.NPC
		if l.Zone != "" {
			w += ", " + l.Zone
		}
		if l.HasXY {
			w += fmt.Sprintf(" (%.0f, %.0f)", l.X, l.Y)
		}
		d = append(d, w)
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
			out = append(out, wording.CutWords(s, maxDetailLength))
		}
	}
	return out
}

// clipTo shortens s to n characters, ending in "...".
func clipTo(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return strings.TrimRight(string([]rune(s)[:n-3]), " ") + "..."
}

func trainingPrompt(c character.Character, l Line, from int) string {
	var names []string
	for _, s := range l.Spells {
		names = append(names, spellLabel(s))
	}
	when := fmt.Sprintf("At level %d I can train", l.Level)
	if from < l.Level {
		when = fmt.Sprintf("Between levels %d and %d I unlocked", from, l.Level)
	}
	return makePrompt(c, when+" ", strings.Join(names, ", ")+".", " Which should I train first, and how does it fit my rotation?")
}

// style is the stored play style in words, for example "Arms, questing", or "" when the player has
// set none.
func style(c character.Character) string {
	var parts []string
	spec := ""
	if p := c.Profile; p != nil {
		spec = p.Spec
	}
	if spec == "" && c.Plan != nil {
		spec = c.Plan.Spec
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

// makePrompt joins "I'm <who>. " + lead + facts + question, shortening only the facts so the whole
// is at most MaxPrompt characters and the question stays.
func makePrompt(c character.Character, lead, facts, question string) string {
	return makePromptAfter(c, ". "+lead, facts, question)
}

// makePromptAfter is makePrompt with the text that follows "I'm <who>" given whole.
func makePromptAfter(c character.Character, after, facts, question string) string {
	head := "I'm " + who(c) + after
	room := MaxPrompt - utf8.RuneCountInString(head) - utf8.RuneCountInString(question)
	if room < 20 {
		room = 20
	}
	facts = wording.CutWords(facts, room)
	return wording.CutWords(head+facts+question, MaxPrompt)
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

// questContext is the details and prompt of a quest line: who and where, what is left or unlocked,
// and how the quest fits the player's level.
func questContext(c character.Character, l Line, reason string) ([]string, string) {
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

	facts := fmt.Sprintf("%q", title)
	if l.Zone != "" {
		facts += " in " + l.Zone
	}
	facts += "."
	if l.Class {
		facts += " It is a class quest."
	}
	lead, ask := "My next step is the quest ", " How should I approach it, and what should I know before I go?"
	switch l.Kind {
	case KindTurnIn:
		lead = "I can turn in the quest "
		ask = " What does turning it in unlock, and what should I do next?"
		if len(l.Unlocks) > 0 {
			next, _, _ := strings.Cut(l.Unlocks[0], " (")
			facts += fmt.Sprintf(" Turning it in unlocks %q.", next)
		}
	case KindPickup:
		lead = "I can pick up the quest "
		ask = " Is it worth taking now, and what should I know before I accept it?"
	case KindProgress:
		lead = "I'm working on the quest "
		ask = " What is the best way to finish it, and what should I watch out for?"
	}
	return finishDetails(d), makePrompt(c, lead, facts, ask)
}

// talentStepContext is the details and prompt of the unspent-talents line. pick is the plan's next
// pick (Describe form) and name its bare node, both "" when there is no plan or dataset pick.
func talentStepContext(c character.Character, s advice.Step, pick, name string) ([]string, string) {
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
	after := ""
	if s.Points > 0 {
		sep := " with "
		if style(c) != "" {
			sep = ", with "
		}
		after = sep + wording.Count(s.Points, "unspent "+what, "unspent "+what+"s") + "."
	} else {
		after = "."
	}
	if name != "" {
		after += " "
		return finishDetails(d), makePromptAfter(c, after, "My plan's next pick is "+name+".", " Should I take it now, and why?")
	}
	them := wording.Plural(s.Points, "it", "them")
	return finishDetails(d), makePromptAfter(c, after, "", " What should I spend "+them+" on, and why?")
}

// talentContext is the details and prompt of a talents entry: the node, its rank and its branch.
func talentContext(c character.Character, name string, rank int, group string) ([]string, string) {
	d := []string{fmt.Sprintf("Next pick: %s, rank %d", name, rank)}
	if group != "" {
		d = append(d, "Branch: "+group)
	}
	facts := fmt.Sprintf("My next talent pick is %s, rank %d.", name, rank)
	ask := " Why this pick now, and what does it change?"
	if style(c) != "" {
		ask = " Why this pick now, for the way I play?"
	}
	return finishDetails(d), makePrompt(c, "", facts, ask)
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

// dungeonContext is the details and prompt of a dungeons entry.
func dungeonContext(c character.Character, p dungeons.DungeonPick, d dungeons.Drop) ([]string, string) {
	det := []string{dungeonFit(c, p)}
	facts := "the dungeon " + p.Name
	if p.MinLevel != nil && p.MaxLevel != nil {
		facts += fmt.Sprintf(" (levels %d to %d)", *p.MinLevel, *p.MaxLevel)
	}
	facts += "."
	switch {
	case d.Item != "" && d.Boss != "":
		det = append(det, fmt.Sprintf("%s drops %s", d.Boss, d.Item))
		facts += fmt.Sprintf(" Its best upgrade for me is %s, from %s.", d.Item, d.Boss)
	case d.Item != "":
		det = append(det, "Best upgrade: "+d.Item)
		facts += fmt.Sprintf(" Its best upgrade for me is %s.", d.Item)
	}
	if d.Slot != "" && d.Item != "" {
		det = append(det, "It fits your "+wording.Slot(d.Slot)+" slot")
	}
	if n := len(p.Quests); n > 0 {
		det = append(det, wording.Count(n, "of your open quests leads", "of your open quests lead")+" here")
	}
	return finishDetails(det), makePrompt(c, "I'm considering ", facts, " Is it worth running now, and how should I prepare?")
}

// upgradeContext is the details and prompt of an upgrades entry: the item, its source and the gain.
func upgradeContext(c character.Character, slot string, cd gear.Candidate) ([]string, string) {
	item := cd.Item.Name
	if item == "" {
		item = "an unnamed item"
	}
	d := []string{fmt.Sprintf("%s: %s, item level %d", wording.Upper(wording.Slot(slot)), item, cd.Item.ItemLevel)}
	facts := fmt.Sprintf("%s for my %s slot (item level %d)", item, wording.Slot(slot), cd.Item.ItemLevel)
	if cur, ok := c.Equipment[slot]; ok && cur.ItemLevel != nil && !cd.Empty {
		gain := int(cd.Item.ItemLevel) - *cur.ItemLevel
		name := cur.Name
		if name == "" {
			name = "your current item"
		}
		d = append(d, fmt.Sprintf("%+d item %s over %s", gain, wording.Plural(gain, "level", "levels"), name))
		facts += fmt.Sprintf(", replacing %s (item level %d)", name, *cur.ItemLevel)
	} else if cd.Empty {
		d = append(d, "Your slot is empty")
	}
	for _, s := range cd.Sources {
		if src := wording.Source(s.Kind, s.Name); src != "" {
			d = append(d, wording.Upper(src))
			facts += ". It is " + src
			break
		}
	}
	if cd.Soon {
		d = append(d, fmt.Sprintf("Usable at level %d", cd.Item.ReqLevel))
		facts += fmt.Sprintf(" and usable at level %d", cd.Item.ReqLevel)
	}
	facts += "."
	return finishDetails(d), makePrompt(c, "A possible upgrade is ", facts, " Is it worth getting for how I play, and how do I get it?")
}

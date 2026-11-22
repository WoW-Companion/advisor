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

// finishDetails keeps at most MaxDetails non-empty lines of at most maxDetailLength characters.
func finishDetails(d []string) []string {
	var out []string
	for _, s := range d {
		if s = strings.TrimSpace(s); s != "" && len(out) < MaxDetails {
			out = append(out, clipTo(s, maxDetailLength))
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
	return makePrompt(c, when+" ", strings.Join(names, ", ")+".", " Which should I prioritise, and how do they fit my play style?")
}

// who is the character as the stored profile and capture describe it, for example "level 10
// Skyborn warrior, Arms, solo questing".
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
	spec := ""
	if p := c.Profile; p != nil {
		spec = p.Spec
	}
	if spec == "" && c.Plan != nil {
		spec = c.Plan.Spec
	}
	if spec != "" {
		b.WriteString(", " + spec)
	}
	if p := c.Profile; p != nil {
		focus := map[string]string{"leveling": "questing", "dungeons": "running dungeons", "pvp": "PvP", "mixed": "mixed play"}[p.Focus]
		if focus != "" {
			b.WriteString(", " + focus)
		}
		if p.Role != "" && p.Role != "balanced" {
			b.WriteString(", " + p.Role + " role")
		}
	}
	return b.String()
}

// makePrompt joins "I'm <who>. " + lead + facts + question, shortening only the facts so the whole
// is at most MaxPrompt characters and the question stays.
func makePrompt(c character.Character, lead, facts, question string) string {
	head := "I'm " + who(c) + ". " + lead
	room := MaxPrompt - utf8.RuneCountInString(head) - utf8.RuneCountInString(question)
	if room < 20 {
		room = 20
	}
	facts = clipTo(facts, room)
	return clipTo(head+facts+question, MaxPrompt)
}

// questContext is the details and prompt of a quest line: the reason it is next, where it is, its
// level and the objectives still open.
func questContext(c character.Character, l Line, reason string) ([]string, string) {
	title := l.Title
	if title == "" {
		title = strings.TrimPrefix(l.Text, "Pick up: ")
	}
	var d []string
	d = append(d, reason)
	where := ""
	switch {
	case l.Kind == KindTurnIn && l.NPC != "":
		where = "Turn in to " + l.NPC
	case l.Kind == KindPickup && l.NPC != "":
		where = "Pick up from " + l.NPC
	}
	if l.Zone != "" {
		if where != "" {
			where += " in " + l.Zone
		} else {
			where = "Zone: " + l.Zone
		}
	}
	d = append(d, where)
	if l.Kind == KindTurnIn && len(l.Unlocks) > 0 {
		d = append(d, "Unlocks: "+strings.Join(l.Unlocks, "; "))
	}
	var tags []string
	if l.Level > 0 {
		tags = append(tags, fmt.Sprintf("Level %d", l.Level))
	}
	if l.Class {
		tags = append(tags, "class quest")
	}
	if l.Elite {
		tags = append(tags, "elite or group quest")
	}
	d = append(d, strings.Join(tags, ", "))
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
		d = append(d, "Open: "+strings.Join(open, "; "))
	}
	facts := fmt.Sprintf("the quest %q", title)
	if l.Level > 0 {
		facts += fmt.Sprintf(" (level %d)", l.Level)
	}
	if l.Zone != "" {
		facts += " in " + l.Zone
	}
	facts += "."
	if l.Class {
		facts += " It is a class quest."
	}
	if l.Kind == KindTurnIn && len(l.Unlocks) > 0 {
		next, _, _ := strings.Cut(l.Unlocks[0], " (")
		facts += fmt.Sprintf(" Turning it in unlocks %q.", next)
	}
	if len(open) > 0 {
		facts += " Still open: " + strings.Join(open, "; ") + "."
	}
	lead := "My next step is "
	switch l.Kind {
	case KindTurnIn:
		lead = "I can turn in "
	case KindPickup:
		lead = "I can pick up "
	case KindProgress:
		lead = "I'm working on "
	}
	return finishDetails(d), makePrompt(c, lead, facts, " How should I approach it, and what should I know before I go?")
}

// talentStepContext is the details and prompt of the unspent-talents line.
func talentStepContext(c character.Character, s advice.Step, text string) ([]string, string) {
	d := []string{text, s.Reason}
	if s.Points > 0 {
		d = append(d, fmt.Sprintf("%d unspent talent points", s.Points))
	}
	facts := text + "."
	if s.Points > 0 {
		facts = fmt.Sprintf("I have %d unspent talent points. %s.", s.Points, text)
	}
	return finishDetails(d), makePrompt(c, "", facts, " What should I spend them on, and why?")
}

// talentContext is the details and prompt of a talents entry: the node, its rank and the build's reason.
func talentContext(c character.Character, name string, rank int, group, why string) ([]string, string) {
	d := []string{fmt.Sprintf("Next pick: %s, rank %d", name, rank)}
	if group != "" {
		d = append(d, "Branch: "+group)
	}
	if why != "" {
		d = append(d, "Why: "+why)
	}
	facts := fmt.Sprintf("The next talent point in my plan is %s to rank %d.", name, rank)
	if why != "" {
		facts += " The plan says: " + why + "."
	}
	return finishDetails(d), makePrompt(c, "", facts, " Is this the right pick for me now, and what does it change?")
}

// dungeonContext is the details and prompt of a dungeons entry.
func dungeonContext(c character.Character, p dungeons.DungeonPick, d dungeons.Drop, item string) ([]string, string) {
	det := []string{fitText(p)}
	if d.Boss != "" {
		det = append(det, fmt.Sprintf("%s drops %s", d.Boss, item))
	} else {
		det = append(det, "Top upgrade: "+item)
	}
	if d.Slot != "" {
		det = append(det, "Slot: "+d.Slot)
	}
	if n := len(p.Quests); n > 0 {
		det = append(det, fmt.Sprintf("%d of your open quests lead here", n))
	}
	facts := fmt.Sprintf("the dungeon %s (%s). Its top upgrade for me is %s", p.Name, fitText(p), item)
	if d.Boss != "" {
		facts += ", from " + d.Boss
	}
	facts += "."
	return finishDetails(det), makePrompt(c, "I'm considering ", facts, " Is it worth running now, and how should I prepare?")
}

// upgradeContext is the details and prompt of an upgrades entry: the item, its source and the gain.
func upgradeContext(c character.Character, slot string, cd gear.Candidate, item string) ([]string, string) {
	d := []string{fmt.Sprintf("%s: %s, ilvl %d", slot, item, cd.Item.ItemLevel)}
	facts := fmt.Sprintf("%s for my %s slot (item level %d)", item, slot, cd.Item.ItemLevel)
	if cur, ok := c.Equipment[slot]; ok && cur.ItemLevel != nil && !cd.Empty {
		gain := cd.Item.ItemLevel - *cur.ItemLevel
		name := cur.Name
		if name == "" {
			name = "your current item"
		}
		d = append(d, fmt.Sprintf("Item level %+d over %s", gain, name))
		facts += fmt.Sprintf(", replacing %s (item level %d)", name, *cur.ItemLevel)
	} else if cd.Empty {
		d = append(d, "Your slot is empty")
	}
	if len(cd.Sources) > 0 {
		s := cd.Sources[0]
		src := s.Name
		if src == "" {
			src = fmt.Sprintf("%s %d", s.Kind, s.ID)
		}
		d = append(d, "From "+src+" ("+s.Kind+")")
		facts += ", from " + src
	}
	if cd.Soon {
		d = append(d, fmt.Sprintf("Needs level %d", cd.Item.ReqLevel))
		facts += fmt.Sprintf(", usable at level %d", cd.Item.ReqLevel)
	}
	facts += "."
	return finishDetails(d), makePrompt(c, "A possible upgrade is ", facts, " How do I get it, and is it worth the effort for my build?")
}

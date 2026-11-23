// Package digest builds the in-game digest the WoWCompanion_Digest slot addon shows (schema 2.0, one 1.2 entry per character, see
// docs/digest.md). It adds no advice logic of its own: quests come from advice.WhatsNext, gear from
// gear.Find, and the rest is read from the capture.
package digest

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/WoW-Companion/advisor/internal/advice"
	"github.com/WoW-Companion/advisor/internal/brief"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/dataset"
	"github.com/WoW-Companion/advisor/internal/dungeons"
	"github.com/WoW-Companion/advisor/internal/gear"
	"github.com/WoW-Companion/advisor/internal/questie"
	"github.com/WoW-Companion/advisor/internal/store"
	"github.com/WoW-Companion/advisor/internal/talents"
	"github.com/WoW-Companion/advisor/internal/wording"
)

// Schema is the version of one character's entry; EnvelopeSchema is the file's.
const (
	Schema         = "1.5"
	EnvelopeSchema = "2.0"
)

// MaxCharacters is the most characters one file holds.
const MaxCharacters = 30

// Section ids the panel knows.
const (
	NextQuests    = "next_quests"
	Upgrades      = "upgrades"
	KeepMaterials = "keep_materials"
	Notes         = "notes"
)

// Caps: the most lines a section holds, and the longest line, in characters. The panel is narrow.
const (
	MaxQuests    = 8
	MaxUpgrades  = 8
	MaxMaterials = 10
	MaxNotes     = 6
	MaxDungeons  = 3
	MaxTalents   = 3
	MaxText      = 110
)

// Line kinds, for a Line's Kind.
const (
	KindPickup   = "pickup"
	KindTurnIn   = "turnin"
	KindProgress = "progress"
	KindTalent   = "talent"
	KindNote     = "note"
	KindGiver    = "giver"
)

// Objective is one objective of a quest: its text without the count, and the count when known.
type Objective struct {
	Text       string
	Have, Need int
	HasCount   bool
}

// Line is one digest line; Item and Quest are zero when absent. Text is clipped to MaxText for older
// addons; every field after Quest is optional, never clipped, and left out when empty.
type Line struct {
	Text  string
	Item  int
	Quest int

	Kind       string // one of the Kind constants
	Title      string // the full quest title
	Level      int    // the quest's level
	Class      bool   // a class quest
	Elite      bool   // an elite or group quest
	NPC        string // the giver of a pick-up, or the turn-in NPC
	Zone       string
	Objectives []Objective
	Points     int // unspent talent points, for the talent line

	Slot    string // upgrades: the slot
	ILvl    int    // upgrades: the item's level
	AtLevel int    // upgrades: the character level the item needs, when not yet wearable

	// Schema 1.4. Details is 1-4 short context lines, Prompt a ready question for the companion's AI
	// (at most MaxPrompt characters); both are set on every entry.
	Details []string
	Prompt  string
	// Schema 1.5. Asks is 1-3 short natural questions for this entry, Ref the stable subject the
	// advisor expands (never shown to players).
	Asks []string
	Ref  string
	// Unlocks is on a turn-in line: the follow-ups turning it in unlocks (not written to Digest.lua).
	Unlocks []string
	Spells  []TrainSpell // training: the abilities and ranks
	X, Y    float64      // training: the trainer's map percent, when HasXY
	HasXY   bool
}

// DungeonLine is one dungeons entry: the dungeon, its level fit and top upgrade; Item is zero when
// absent.
type DungeonLine struct {
	Name    string
	Line    string
	Item    int
	Details []string
	Prompt  string
	Asks    []string
	Ref     string
}

// TalentLine is one talents entry: the next point to spend; Spell is zero when the dataset has none.
type TalentLine struct {
	Line    string
	Spell   int
	Details []string
	Prompt  string
	Asks    []string
	Ref     string
}

// Section is an ordered group of lines.
type Section struct {
	ID    string
	Lines []Line
}

// Digest is the whole file.
type Digest struct {
	Schema    string
	Generated int64
	Build     string
	Character string
	Sections  []Section
	Dungeons  []DungeonLine // optional, at most MaxDungeons
	Talents   []TalentLine  // optional, at most MaxTalents
}

// Options is what Build needs besides the capture and the dataset.
type Options struct {
	Now          time.Time
	Client       character.Client // the game client that wrote the capture
	DatasetBuild string           // the dataset's game build; empty when none
	Questie      *questie.Index   // the player's own QuestieDB; nil when they have none
	// PrevLevel is the character's level at the last capture, 0 when unknown. Training then covers
	// the levels after it; with 0 it covers the character's own level only.
	PrevLevel int
	// Unlocks is, per turn-in quest id, the follow-ups the plan says it unlocks, as "Quest (from NPC,
	// here)"; a turn-in entry then says "Unlocks: ..." and its prompt names the first.
	Unlocks map[int][]string
}

// NoDataText is what NEXT says when no game dataset has downloaded.
const NoDataText = "Game data not downloaded yet; see Setup"

// clip shortens s to at most MaxText characters, cutting at a word boundary: "..." follows a whole
// word, never half of one. Text is only shortened where an older addon needs a short line; the whole
// wording always goes in the details.
func clip(s string) string {
	if utf8.RuneCountInString(s) <= MaxText {
		return s
	}
	r := []rune(s)
	head := string(r[:MaxText-3])
	if r[MaxText-3] != ' ' {
		if i := strings.LastIndex(head, " "); i > 0 {
			head = head[:i]
		}
	}
	return strings.TrimRight(head, " ,;:") + "..."
}

// oneObjective keeps a quest's progress readable on one Next line: the first objective plus "+N more".
func oneObjective(p string) string {
	head, rest, found := strings.Cut(p, "; ")
	if !found {
		return p
	}
	return fmt.Sprintf("%s; +%d more", head, strings.Count(rest, "; ")+1)
}

// objectivesOf is the logged quest's objectives, whole: text without the count, the count when the
// capture has one, optional objectives included.
func objectivesOf(c character.Character, id int) []Objective {
	for _, q := range c.Quests {
		if q.QuestID != id {
			continue
		}
		var out []Objective
		for _, o := range q.Objectives {
			l := o.Label()
			if l == "" {
				continue
			}
			ob := Objective{Text: l}
			if o.Have != nil && o.Need != nil && *o.Need > 0 {
				ob.Have, ob.Need, ob.HasCount = *o.Have, *o.Need, true
			} else if o.Done != nil && *o.Done {
				ob.Have, ob.Need, ob.HasCount = 1, 1, true
			}
			out = append(out, ob)
		}
		return out
	}
	return nil
}

func name(c character.Character) string {
	if c.Name != "" {
		return c.Name
	}
	return c.Key
}

func buildOf(cl character.Client) string {
	if cl.Version != "" && cl.Build != "" {
		return cl.Version + "." + cl.Build
	}
	return cl.Build
}

// Build computes the digest for c. ds may be nil (no dataset); sections with nothing to say are
// left out.
func Build(c character.Character, ds store.DataStore, opt Options) Digest {
	d := Digest{Schema: Schema, Generated: opt.Now.Unix(), Build: buildOf(opt.Client), Character: name(c)}
	steps := advice.WhatsNextWith(c, ds, opt.Questie)
	notes := stepNotes(c, ds, steps)

	var quests, milestones []Line
	for _, s := range steps {
		if s.Kind == advice.KindTalent { // milestones lead the NEXT section
			l := Line{Text: talentNote(c, ds, s.Title), Kind: KindTalent, Points: s.Points}
			pick, name := nextPick(c, ds)
			l.Details, l.Prompt = talentStepContext(c, s, pick, name)
			l.Asks, l.Ref = talentAsks(s.Points, name), talentsRef
			quests, milestones = append(quests, l), append(milestones, l)
			continue
		}
		if advice.IsClassQuest(s) {
			l := Line{Text: clip(s.Title), Quest: s.QuestID, Kind: KindPickup, Title: s.Name, Level: s.Level, Class: true, Elite: s.Elite, NPC: s.NPC, Zone: s.Zone}
			l.Details, l.Prompt = questContext(c, l, s.Reason)
			l.Asks, l.Ref = questAsks(l), questRef(l.Quest)
			quests, milestones = append(quests, l), append(milestones, l)
			continue
		}
		if s.Kind != advice.KindTurnIn && s.Kind != advice.KindQuest || strings.HasPrefix(s.Title, advice.AbandonPrefix) {
			continue
		}
		text := s.Title
		if s.Progress != "" {
			text = oneObjective(s.Progress)
		}
		if s.Zone != "" {
			text += " (" + s.Zone + ")"
		}
		l := Line{Text: clip(text), Quest: s.QuestID, Kind: KindProgress, Title: s.Name, Level: s.Level, Elite: s.Elite, NPC: s.NPC, Zone: s.Zone}
		switch {
		case strings.HasPrefix(s.Title, advice.PickUpPrefix):
			l.Kind = KindPickup
		case s.Kind == advice.KindTurnIn:
			l.Kind = KindTurnIn
			l.Unlocks = opt.Unlocks[s.QuestID]
		default:
			l.Objectives = objectivesOf(c, s.QuestID)
		}
		l.Details, l.Prompt = questContext(c, l, s.Reason)
		l.Asks, l.Ref = questAsks(l), questRef(l.Quest)
		quests = append(quests, l)
	}
	if ds == nil {
		quests = append(milestones, Line{Text: NoDataText, Kind: KindNote, Asks: asksOf("How do I download the game data?", "How do I download the game data?"), Ref: noteRef("no-data")})
	} else if t, ok := trainingLine(c, ds, opt); ok {
		// Training is a level milestone: it follows the talent and class lines and leads the quests.
		quests = append(quests[:len(milestones):len(milestones)], append([]Line{t}, quests[len(milestones):]...)...)
	}
	ups, note := upgradeLines(c, ds)
	notes = append(notes, note...)
	if g, ok := FindGiver(c, opt.Questie, PlanQuests(steps), PlanNPCs(steps)); ok {
		notes = append(notes, g.Line(c))
	}
	notes = append(notes, captureNotes(c, ds, opt)...)

	for _, s := range []Section{
		{NextQuests, capLines(quests, MaxQuests)},
		{Upgrades, capLines(ups, MaxUpgrades)},
		{KeepMaterials, capLines(materials(c, ds), MaxMaterials)},
		{Notes, capLines(notes, MaxNotes)},
	} {
		if len(s.Lines) > 0 {
			d.Sections = append(d.Sections, s)
		}
	}
	d.Dungeons = dungeonLines(c, ds)
	d.Talents = talentLines(c, ds)
	return d
}

// Select picks the characters a file holds: the most recently captured first, at most MaxCharacters,
// one per name (the first wins). The rest come back as skipped.
func Select(chars []character.Character) (kept, skipped []character.Character) {
	sorted := append([]character.Character(nil), chars...)
	sort.SliceStable(sorted, func(i, j int) bool {
		a, b := sorted[i].CapturedAt, sorted[j].CapturedAt
		if a == nil || b == nil {
			return a != nil && b == nil
		}
		return *a > *b
	})
	seen := map[string]bool{}
	for _, c := range sorted {
		if n := name(c); len(kept) < MaxCharacters && !seen[n] {
			seen[n] = true
			kept = append(kept, c)
		} else {
			skipped = append(skipped, c)
		}
	}
	return kept, skipped
}

// dungeonLines is the top MaxDungeons dungeons of dungeons.Plan that have an upgrade to show.
func dungeonLines(c character.Character, ds store.DataStore) []DungeonLine {
	res, err := dungeons.Plan(c, ds)
	if err != nil {
		return nil
	}
	var out []DungeonLine
	for _, p := range res.Picks {
		if len(out) == MaxDungeons {
			break
		}
		if len(p.Drops) == 0 {
			continue
		}
		d := p.Drops[0]
		item := d.Item
		if item == "" {
			item = fmt.Sprintf("item %d", d.ItemID)
		}
		dl := DungeonLine{Name: p.Name, Line: clip(fitText(p) + ": " + item), Item: int(d.ItemID), Asks: dungeonAsks(p.Name), Ref: dungeonRef(p.MapID)}
		dl.Details, dl.Prompt = dungeonContext(c, p, d)
		out = append(out, dl)
	}
	return out
}

// fitText is the dungeon's level range and how the character sits against it, for the digest line.
func fitText(p dungeons.DungeonPick) string {
	s := fmt.Sprintf("lvl %d-%d", *p.MinLevel, *p.MaxLevel)
	if p.Fit > 0 {
		s += fmt.Sprintf(" (%d off)", p.Fit)
	}
	return s
}

// nextPick is the plan's next talent pick: its phrase with rank and branch, and its bare name; both
// "" when there is none.
func nextPick(c character.Character, ds store.DataStore) (phrase, name string) {
	if g, ok := talents.GuideNext(c, 1); ok && len(g.Next) > 0 {
		return g.Describe(), g.Next[0].Name
	}
	if ds != nil {
		if p := talents.Plan(c, ds, talents.Options{Limit: 1}); len(p.Next) > 0 {
			return fmt.Sprintf("%s (rank %d)", p.Next[0].Name, p.Next[0].Rank), p.Next[0].Name
		}
	}
	return "", ""
}

// talentLines is the next MaxTalents points of talents.Plan, with the node's spell id when the
// dataset has one.
func talentLines(c character.Character, ds store.DataStore) []TalentLine {
	if g, ok := talents.GuideNext(c, MaxTalents); ok && len(g.Next) > 0 {
		var out []TalentLine
		for _, p := range g.Next {
			l := TalentLine{Line: clip(fmt.Sprintf("%s rank %d", p.Name, p.Rank)), Asks: talentAsks(1, p.Name), Ref: talentsRef}
			l.Details, l.Prompt = talentContext(c, p.Name, p.Rank, p.Group)
			if p.SpellID != nil {
				l.Spell = *p.SpellID
			}
			out = append(out, l)
		}
		return out
	}
	if ds == nil {
		return nil
	}
	var out []TalentLine
	for _, p := range talents.Plan(c, ds, talents.Options{Limit: MaxTalents}).Next {
		l := TalentLine{Line: clip(fmt.Sprintf("%s rank %d", p.Name, p.Rank)), Spell: spellID(ds, p.NodeID), Asks: talentAsks(1, p.Name), Ref: talentsRef}
		l.Details, l.Prompt = talentContext(c, p.Name, p.Rank, "")
		out = append(out, l)
	}
	return out
}

// spellID is the spell of a talent node, 0 when the dataset has none.
func spellID(ds store.DataStore, node int64) int {
	rows, err := ds.Filter("talent_nodes", store.Range{Column: "node_id", Min: node, Max: node})
	if err != nil || len(rows) == 0 {
		return 0
	}
	n, _ := rows[0]["spell_id"].(int64)
	return int(n)
}

func capLines(l []Line, n int) []Line {
	if len(l) > n {
		return l[:n]
	}
	return l
}

// stepNotes turns the steps that are not next_quests lines into notes: unspent talents, failed quests.
func stepNotes(c character.Character, ds store.DataStore, steps []advice.Step) []Line {
	var out []Line
	failed := 0
	for _, s := range steps {
		switch {
		case s.Kind == advice.KindQuest && strings.HasPrefix(s.Title, advice.AbandonPrefix):
			failed++
		}
	}
	if failed > 0 {
		out = append(out, Line{Text: wording.Count(failed, "failed quest", "failed quests") + ": abandon", Kind: KindNote,
			Asks: asksOf("Should I abandon my failed quests?", "Should I abandon my failed quests?"), Ref: noteRef("failed-quests")})
	}
	return out
}

// talentNote is the unspent-points title, naming the build's next node when the dataset has one.
func talentNote(c character.Character, ds store.DataStore, title string) string {
	if strings.Contains(title, ": take ") { // the saved plan already named the picks
		return title
	}
	if ds == nil {
		return title
	}
	if p := talents.Plan(c, ds, talents.Options{Limit: 1}); len(p.Next) > 0 {
		return title + ": " + p.Next[0].Name
	}
	return title
}

// captureNotes are the gaps in the inputs the player can fix.
func captureNotes(c character.Character, ds store.DataStore, opt Options) []Line {
	var out []Line
	if !c.BankCaptured {
		out = append(out, Line{Text: "Bank not captured: open your bank once", Kind: KindNote, Asks: asksOf("Why does my bank need opening?", "Why does my bank need opening?"), Ref: noteRef("bank")})
	}
	switch {
	case ds == nil:
		// The NEXT section says so (NoDataText); no second line here.
	case opt.DatasetBuild != "" && opt.Client.Build != "" && !dataset.Matches(opt.DatasetBuild, opt.Client.Version, opt.Client.Build):
		out = append(out, Line{Text: fmt.Sprintf("Dataset is build %s, game is %s", opt.DatasetBuild, opt.Client.Build), Kind: KindNote, Asks: asksOf("What does the dataset mismatch mean?", "What does the dataset mismatch mean?"), Ref: noteRef("dataset-build")})
	}
	return out
}

// upgradeLines is one line per slot, the top candidate and its first source; the second result is
// a note when the search could not run.
func upgradeLines(c character.Character, ds store.DataStore) (lines, notes []Line) {
	if ds == nil {
		return nil, nil // captureNotes says so
	}
	group := ""
	if names, _ := brief.TopGroups(c.Talents); len(names) == 1 {
		group = names[0]
	}
	res, err := gear.Find(c, ds, group, gear.Options{Limit: 1})
	if err != nil {
		return nil, []Line{{Text: "No gear upgrades: " + err.Error(), Kind: KindNote, Asks: asksOf("Why are there no gear upgrades?", "Why are there no gear upgrades?"), Ref: noteRef("no-upgrades")}}
	}
	slots := make([]string, 0, len(res.Slots))
	for s := range res.Slots {
		slots = append(slots, s)
	}
	rank := map[string]int{}
	for i, s := range brief.Slots {
		rank[s] = i + 1
	}
	sort.Slice(slots, func(i, j int) bool {
		a, b := rank[slots[i]], rank[slots[j]]
		if a == 0 || b == 0 {
			if a != b {
				return a != 0
			}
			return slots[i] < slots[j]
		}
		return a < b
	})
	for _, slot := range slots {
		cd := res.Slots[slot][0]
		item := cd.Item.Name
		if item == "" {
			item = fmt.Sprintf("item %d", cd.Item.ID)
		}
		text := fmt.Sprintf("%s: %s (ilvl %d", slot, item, cd.Item.ItemLevel)
		if cd.Soon {
			text += fmt.Sprintf(", at level %d", cd.Item.ReqLevel)
		}
		text += ")"
		if len(cd.Sources) > 0 {
			s := cd.Sources[0]
			// An unnamed source is left out, never shown as an id. The source goes on only whole: when
			// it would push the line past the cap it is dropped.
			if s.Name != "" {
				if with := text + " from " + s.Name; utf8.RuneCountInString(with) <= MaxText {
					text = with
				}
			}
		}
		l := Line{Text: clip(text), Item: int(cd.Item.ID), Slot: slot, ILvl: int(cd.Item.ItemLevel)}
		if cd.Soon {
			l.AtLevel = int(cd.Item.ReqLevel)
		}
		l.Details, l.Prompt = upgradeContext(c, slot, cd)
		l.Asks, l.Ref = upgradeAsks(cd.Item.Name), itemRef(int(cd.Item.ID))
		lines = append(lines, l)
	}
	return lines, nil
}

// PlanQuests is the quest ids of the plan's steps.
func PlanQuests(steps []advice.Step) map[int]bool {
	out := map[int]bool{}
	for _, s := range steps {
		if s.QuestID != 0 {
			out[s.QuestID] = true
		}
	}
	return out
}

// PlanNPCs is the givers the plan's pick-up steps name (a turn-in NPC is not a giver).
func PlanNPCs(steps []advice.Step) map[string]bool {
	out := map[string]bool{}
	for _, s := range steps {
		if s.NPC != "" && (strings.HasPrefix(s.Title, advice.PickUpPrefix) || advice.IsClassQuest(s)) {
			out[s.NPC] = true
		}
	}
	return out
}

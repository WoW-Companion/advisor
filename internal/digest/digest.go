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
	"github.com/WoW-Companion/advisor/internal/questie"
	"github.com/WoW-Companion/advisor/internal/stats"
	"github.com/WoW-Companion/advisor/internal/store"
	"github.com/WoW-Companion/advisor/internal/talents"
	"github.com/WoW-Companion/advisor/internal/wording"
)

// Schema is the version of one character's entry; EnvelopeSchema is the file's.
const (
	Schema         = "1.9"
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
	trainAt string // the "Train at:" fact of a training entry
	Text    string
	Item    int
	Quest   int

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
	ILvl    int    // upgrades: the item's level (not shown to players)
	AtLevel int    // upgrades: the character level the item needs, when not yet wearable

	// Schema 1.4. Details is 1-4 short context lines, set on every entry. The legacy prompt field
	// was removed in schema 1.8: the companion builds its own question from the entry's ref.
	Details []string
	// Schema 1.5. Asks is 1-3 short natural questions for this entry, Ref the stable subject the
	// advisor expands (never shown to players).
	Asks []string
	Ref  string
	// Schema 1.6. Why is one short, factual reason (at most MaxWhy characters), left out when no fact fits.
	Why string
	// Schema 1.7, upgrades: Wear is "Wear now", "From level N" or "After you train" (a weapon skill to learn first), Gain "Big upgrade", "Solid upgrade" or
	// "Small upgrade", Replaces the worn item's name alone ("" for an empty slot; schema 1.9), How the best path's
	// one line and PathKind its kind (quest, craft, auction, dungeon or creature), for the icon.
	Wear, Gain, Replaces, How, PathKind string
	// Train is the weapon skill the upgrade needs and the character can learn, "" when none.
	Train string
	// Unlocks is on a turn-in line: the follow-ups turning it in unlocks (not written to Digest.lua).
	Unlocks []string
	Spells  []TrainSpell // training: the abilities and ranks
	X, Y    float64      // training: the trainer's map percent, when HasXY
	HasXY   bool
}

// StatsEntry is the optional stats entry (schema 1.7): the stat check in plain words.
type StatsEntry struct {
	Insights []string // 1-3 sentences, each at most stats.MaxInsight characters
	Profile  string   // for example "Fury · leveling"
	Asks     []string
}

// DungeonLine is one dungeons entry: the dungeon, its level fit and top upgrade; Item is zero when
// absent.
type DungeonLine struct {
	Name    string
	Line    string
	Item    int
	Details []string
	Asks    []string
	Ref     string
	Why     string
}

// TalentLine is one talents entry: the next point to spend; Spell is zero when the dataset has none.
type TalentLine struct {
	Line    string
	Spell   int
	Details []string
	Asks    []string
	Ref     string
	Why     string
}

// Section is an ordered group of lines.
type Section struct {
	ID    string
	Lines []Line
	// Summary is the section's one line count, such as "3 upgrades you can get" (schema 1.7).
	Summary string
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
	Stats     *StatsEntry   // optional: present only for a character with a play style
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
	// Order is the quest ids in route order; Next quests follows it, so the panel and the route agree.
	Order []int
}

// NoDataText is what NEXT says when no game dataset has downloaded.
const NoDataText = "Game data not downloaded yet; see Setup"

// clip is s for a line of text. Past MaxText it drops the optional tail that an older addon has no
// room for, a trailing "(zone)" or "from <giver>", and keeps what is left only when that reads whole;
// otherwise it keeps all of s: a line is never cut short, with or without "...".
func clip(s string) string {
	if utf8.RuneCountInString(s) <= MaxText {
		return s
	}
	t := s
	if i := strings.LastIndex(t, " ("); i > 0 && strings.HasSuffix(t, ")") {
		t = t[:i]
	}
	if utf8.RuneCountInString(t) > MaxText {
		if i := strings.LastIndex(t, " from "); i > 0 {
			t = t[:i]
		}
	}
	if utf8.RuneCountInString(t) <= MaxText {
		return t
	}
	return s
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
			if hedgedTalents(c, ds) { // what is spent is unknown: no rank-1 pick to name
				pick, name = "", ""
			}
			l.Details = talentStepContext(c, s, pick, name)
			l.Asks, l.Ref = talentAsks(s.Points, name), talentsRef
			l.Why = talentWhy(c, name)
			quests, milestones = append(quests, l), append(milestones, l)
			continue
		}
		if advice.IsClassQuest(s) || advice.IsSpellQuest(s) {
			l := Line{Text: clip(s.Title), Quest: s.QuestID, Kind: KindPickup, Title: s.Name, Level: s.Level, Class: true, Elite: s.Elite, NPC: s.NPC, Zone: s.Zone}
			l.Details = questContext(c, l, s.Reason)
			l.Asks, l.Ref = questAsks(l), questRef(l.Quest)
			l.Why = questWhy(c, l)
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
		if s.Kind == advice.KindQuest && !strings.HasPrefix(s.Title, advice.PickUpPrefix) {
			text = "Finish " + text // a step's first line is an action; the level fit goes below it
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
		l.Details = questContext(c, l, s.Reason)
		l.Asks, l.Ref = questAsks(l), questRef(l.Quest)
		l.Why = questWhy(c, l)
		quests = append(quests, l)
	}
	if ds == nil {
		quests = append(milestones, Line{Text: NoDataText, Kind: KindNote, Asks: asksOf("How do I download the game data?", "How do I download the game data?"), Ref: noteRef("no-data")})
	} else if t, ok := trainingLine(c, ds, opt); ok {
		// Training is a level milestone: it follows the talent and class lines and leads the quests.
		quests = append(quests[:len(milestones):len(milestones)], append([]Line{t}, quests[len(milestones):]...)...)
	}
	group := ""
	if names, _ := brief.TopGroups(c.Talents); len(names) == 1 {
		group = names[0]
	}
	sc := stats.Check(c, ds, group)
	ups, note, link := upgradeLines(c, ds, PlanQuests(steps), opt.Questie, shortStat(sc))
	notes = append(notes, note...)
	if n := bagCount(ups); n > 0 { // free upgrades lead NEXT, ahead of anything to buy
		quests = append([]Line{bagNextLine(n)}, quests...)
	}
	// Unspent class talent points lead everything, bag upgrades too.
	for i, l := range quests {
		if l.Kind == KindTalent && strings.HasPrefix(l.Text, "Spend ") {
			quests = append([]Line{l}, append(quests[:i:i], quests[i+1:]...)...)
			break
		}
	}
	for i := range quests {
		if w, ok := link.questWhy[quests[i].Quest]; ok && quests[i].Quest != 0 {
			quests[i].Why = w
		}
	}
	if g, ok := FindGiver(c, opt.Questie, PlanQuests(steps), PlanNPCs(steps)); ok {
		notes = append(notes, g.Line(c))
	}
	notes = append(notes, captureNotes(c, ds, opt)...)
	if l, ok := seasonalMention(c, opt); ok {
		notes = append(notes, l)
	}

	quests = routeOrder(quests, opt.Order)
	quests = withWeaponTraining(quests, weaponTraining(c, ds, link.trains))
	for _, s := range []Section{
		{NextQuests, capLines(quests, MaxQuests), ""},
		{Upgrades, capLines(ups, MaxUpgrades), upgradesSummary(len(capLines(ups, MaxUpgrades)))},
		{KeepMaterials, capLines(materials(c, ds, link.craftFor), MaxMaterials), ""},
		{Notes, capLines(notes, MaxNotes), ""},
	} {
		if len(s.Lines) > 0 {
			d.Sections = append(d.Sections, s)
		}
	}
	d.Dungeons = dungeonLines(c, ds)
	d.Talents = talentLines(c, ds)
	if sc.Needs == "" && len(sc.Insights) > 0 {
		d.Stats = &StatsEntry{Insights: sc.Insights, Profile: sc.Profile, Asks: asksOf("Which stat matters most for my play style?", sc.Asks...)}
	}
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
		dl.Details = dungeonContext(c, p, d)
		if d.Item != "" && d.Slot != "" {
			dl.Why = firstWhy("Drops "+d.Item+" for your "+wording.Slot(d.Slot)+" slot", "Has an upgrade for your "+wording.Slot(d.Slot)+" slot")
		}
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

// nextPick is the curated build's next talent pick: its phrase with rank and branch, and its bare name; both
// "" when there is none.
func nextPick(c character.Character, ds store.DataStore) (phrase, name string) {
	if ds != nil {
		if p := talents.Plan(c, ds, talents.Options{Limit: 1}); len(p.Next) > 0 {
			return fmt.Sprintf("%s (rank %d)", p.Next[0].Name, p.Next[0].Rank), p.Next[0].Name
		}
	}
	return "", ""
}

// hedgedTalents is true when the capture has unspent points but no readable list of what is spent.
func hedgedTalents(c character.Character, ds store.DataStore) bool {
	t := c.Talents
	return t != nil && t.Unspent != nil && *t.Unspent > 0 && (!advice.TalentsKnown(t) || talents.Unreadable(c, ds))
}

// talentLines is the next MaxTalents points of talents.Plan, with the node's spell id when the
// dataset has one.
func talentLines(c character.Character, ds store.DataStore) []TalentLine {
	if ds == nil || hedgedTalents(c, ds) {
		return nil
	}
	var out []TalentLine
	for _, p := range talents.Plan(c, ds, talents.Options{Limit: MaxTalents}).Next {
		l := TalentLine{Line: clip(fmt.Sprintf("%s rank %d", p.Name, p.Rank)), Spell: spellID(ds, p.NodeID), Asks: talentAsks(1, p.Name), Ref: talentsRef, Why: firstWhy(p.Rationale, "Next pick in your build")}
		l.Details = talentContext(c, p.Name, p.Rank, "")
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
	if strings.HasPrefix(title, "Spend ") || strings.Contains(title, ": take ") { // the step already named the pick
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
		if s.NPC != "" && (strings.HasPrefix(s.Title, advice.PickUpPrefix) || advice.IsClassQuest(s) || advice.IsSpellQuest(s)) {
			out[s.NPC] = true
		}
	}
	return out
}

// routeOrder puts the quest lines in route order. A line that moves ahead of where it stood and has no
// reason of its own says why: it is where the route goes first.
func routeOrder(lines []Line, order []int) []Line {
	if len(order) == 0 {
		return lines
	}
	was := map[int]int{}
	for i, l := range lines {
		if l.Quest != 0 {
			was[l.Quest] = i
		}
	}
	out := advice.ByQuestOrder(lines, func(l Line) int { return l.Quest }, order)
	for i := range out {
		if q := out[i].Quest; q != 0 && i < was[q] && out[i].Why == "" {
			out[i].Why = "First on your route"
		}
	}
	return out
}

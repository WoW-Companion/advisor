package digest

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/WoW-Companion/advisor/internal/advice"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/paths"
	"github.com/WoW-Companion/advisor/internal/questie"
	"github.com/WoW-Companion/advisor/internal/wording"
)

// Schema 1.5: every entry carries 1-3 short questions in the player's voice (Asks) and a stable
// subject reference (Ref) the advisor expands itself. Neither is built from clipped text.
const (
	MaxAsks      = 3
	MaxAskLength = 90
)

// Ref kinds: "quest:<id>", "npc:<id>", "talents", "training:<level>", "item:<id>", "dungeon:<id>",
// "note:<slug>".
func questRef(id int) string     { return fmt.Sprintf("quest:%d", id) }
func npcRef(id int) string       { return fmt.Sprintf("npc:%d", id) }
func itemRef(id int) string      { return fmt.Sprintf("item:%d", id) }
func dungeonRef(id int64) string { return fmt.Sprintf("dungeon:%d", id) }
func trainingRef(l int) string   { return fmt.Sprintf("training:%d", l) }
func noteRef(slug string) string { return "note:" + slug }

const talentsRef = "talents"

// asksOf keeps the candidates that fit MaxAskLength whole (a question is never cut), at most
// MaxAsks. fallback stands in when none fits.
func asksOf(fallback string, cands ...string) []string {
	var out []string
	for _, a := range cands {
		if a == "" || utf8.RuneCountInString(a) > MaxAskLength || len(out) == MaxAsks {
			continue
		}
		out = append(out, a)
	}
	if len(out) == 0 {
		out = []string{fallback}
	}
	return out
}

// verbs that start an objective's text: "Kill Wolves" is not something one finds.
var objectiveVerbs = []string{"kill ", "slay ", "speak ", "talk ", "use ", "explore ", "escort ", "defeat ", "reach ", "visit ", "discover ", "find ", "deliver ", "return ", "protect ", "clear "}

func isItemObjective(s string) bool {
	l := strings.ToLower(s)
	for _, v := range objectiveVerbs {
		if strings.HasPrefix(l, v) {
			return false
		}
	}
	return s != ""
}

// questAsks are the questions of a quest line: what the quest leads to and how it fits with the
// quests nearby, never what its card already lists (objectives, giver, level).
func questAsks(l Line) []string {
	t := l.Title
	if t == "" {
		t = l.Text
	}
	switch l.Kind {
	case KindTurnIn:
		return asksOf(fmt.Sprintf("What comes after I hand in %s?", t),
			fmt.Sprintf("What comes after I hand in %s?", t),
			fmt.Sprintf("Should I finish anything nearby before handing in %s?", t))
	case KindPickup:
		return asksOf(fmt.Sprintf("Is %s worth taking now?", t),
			fmt.Sprintf("What does %s lead to?", t),
			fmt.Sprintf("Should I take %s before the other quests nearby?", t))
	}
	return asksOf(fmt.Sprintf("What should I do alongside %s to save time?", t),
		fmt.Sprintf("What should I do alongside %s to save time?", t),
		fmt.Sprintf("What does %s lead to?", t))
}

// talentAsks are the questions of a talent entry: why the pick suits the play style and what it changes.
func talentAsks(points int, pick string) []string {
	which := "Which talent should I take next for my play style?"
	if points > 1 {
		which = "Which talents should I take next for my play style?"
	}
	if pick == "" {
		return asksOf(which, which)
	}
	return asksOf(which,
		fmt.Sprintf("Why does the %s talent suit my play style?", pick),
		fmt.Sprintf("What changes in play once I take the %s talent?", pick))
}

// upgradeAsks are the questions of one upgrade entry: fit, staying power and, for a craft, whether
// to spend the materials now. who is the player's spec and class ("Fury warrior"), slot the item's slot.
func upgradeAsks(item, slot, pathKind, who string, craftMats []string) []string {
	fallback := "Which of my upgrades should I get first?"
	if item == "" {
		return []string{fallback}
	}
	fit := fmt.Sprintf("How much does %s help my build?", item)
	if who != "" {
		fit = fmt.Sprintf("How much does %s help my %s?", item, who)
	}
	outgrow := fmt.Sprintf("Will I outgrow %s soon?", item)
	if slot != "" {
		outgrow = fmt.Sprintf("Will %s stay my best pick for the %s slot?", item, strings.ToLower(slot))
	}
	if pathKind == paths.Craft {
		mats := "the materials"
		if len(craftMats) > 0 {
			mats = "the " + strings.Join(craftMats, " and ")
		}
		return asksOf(fit, fmt.Sprintf("Is it worth making %s now, or should I save %s?", item, mats), fit, outgrow)
	}
	return asksOf(fit, fit, outgrow)
}

// shoppingAsks are the questions of a slot's Auction House shopping list: the alternative suffix,
// priority and price sense, and a follow-up. slot is the slot in lower case, suffixes the plain
// suffix names ("of Strength"), worn the item worn there ("" for an empty slot).
func shoppingAsks(slot string, suffixes []string, worn string) []string {
	if slot == "" {
		return asksOf("Is the Auction House my best bet for an upgrade?", "Is the Auction House my best bet for an upgrade?")
	}
	more := "What else should I pick up at the Auction House while I'm there?"
	if len(suffixes) == 0 {
		return asksOf(more, fmt.Sprintf("Is the Auction House my best bet for my %s slot?", slot), more)
	}
	pick := fmt.Sprintf("For my %s slot, is '%s' my best pick for now?", slot, suffixes[0])
	worth := fmt.Sprintf("Is '%s' gear for my %s slot worth a few silver?", suffixes[0], slot)
	if worn != "" {
		worth = fmt.Sprintf("Is '%s' worth a few silver over my %s?", suffixes[0], worn)
	}
	if len(suffixes) > 1 {
		alt := fmt.Sprintf("Would '%s' do as well as '%s' for my %s?", suffixes[1], suffixes[0], slot)
		return asksOf(pick, pick, alt, worth)
	}
	return asksOf(pick, pick, worth, more)
}

func materialAsks(item string) []string {
	return asksOf(fmt.Sprintf("Should I keep %s?", item),
		fmt.Sprintf("Will I need %s for a craft soon?", item), fmt.Sprintf("Is %s worth selling instead?", item))
}

func dungeonAsks(name string) []string {
	return asksOf(fmt.Sprintf("Am I ready for %s?", name),
		fmt.Sprintf("What should I bring to %s?", name), fmt.Sprintf("Should I run %s solo or wait for a group?", name))
}

// trainingAsks name the first new ability (or the first one), the entry's subject.
func trainingAsks(spells []TrainSpell) []string {
	name := ""
	for _, s := range spells {
		if s.Name != "" && (name == "" || s.New) {
			name = s.Name
			if s.New {
				break
			}
		}
	}
	if name == "" {
		return []string{"Which new ability should I train first?"}
	}
	return asksOf("Which new ability should I train first?", "Which new ability should I train first?",
		fmt.Sprintf("What does %s change in how I play?", name),
		fmt.Sprintf("Is %s worth the gold now, or should I wait?", name))
}

// giverAsks are the questions of a giver note: whether the quest is worth the trip now, and where it leads.
func giverAsks(npc, title string) []string {
	if title == "" {
		return asksOf(fmt.Sprintf("What does %s have for me?", npc), fmt.Sprintf("What does %s have for me?", npc))
	}
	return asksOf(fmt.Sprintf("Is %s worth the trip to %s now?", title, npc),
		fmt.Sprintf("Is %s worth the trip to %s now?", title, npc), fmt.Sprintf("What does %s lead to?", title))
}

// Giver is a quest giver worth a note: an NPC with a quest the character can pick up now that is not
// already a plan step.
type Giver struct {
	NPC, Zone, Title string
	QuestID, NPCID   int
	X, Y             float64
	Level            int
}

// FindGiver is the one giver the character has a quest to pick up from, or false. It uses the player's
// own Questie data, which names each quest's giver; nothing is shown without it. A quest that is
// already a plan step, or whose giver already has a quest in the plan, gives no note, and an NPC who
// is only the turn-in of an open quest is never a giver. planned is the plan's quest ids; planNPCs
// the NPC names the plan sends the player to.
func FindGiver(c character.Character, qx *questie.Index, planned map[int]bool, planNPCs map[string]bool) (Giver, bool) {
	if qx == nil || c.Level == nil {
		return Giver{}, false
	}
	p := c.LastKnownPosition()
	if p == nil {
		return Giver{}, false
	}
	at := questie.Where{MapID: p.MapID, Zone: p.Zone, X: p.X, Y: p.Y}
	for _, o := range qx.Available(advice.WhoOf(c), at, nil) {
		if planned[o.Quest.ID] || planNPCs[o.Giver.Name] || o.Quest.Name == "" || o.Giver.Name == "" {
			continue
		}
		return Giver{NPC: o.Giver.Name, Zone: o.Giver.Zone, Title: o.Quest.Name, QuestID: o.Quest.ID, NPCID: o.Giver.ID,
			X: o.Giver.X, Y: o.Giver.Y, Level: o.Quest.Level}, true
	}
	return Giver{}, false
}

// Text is the plain note: who, where and what.
func (g Giver) Text() string {
	where := ""
	if g.Zone != "" {
		where = " in " + g.Zone
	}
	return fmt.Sprintf("%s%s has a quest for you: %s", g.NPC, where, g.Title)
}

// Line is the giver note as a digest line, with its details.
func (g Giver) Line(c character.Character) Line {
	l := Line{Text: g.Text(), Kind: KindGiver, NPC: g.NPC, Zone: g.Zone, Quest: g.QuestID, Title: g.Title, Level: g.Level, X: g.X, Y: g.Y, HasXY: true}
	where := g.NPC
	if g.Zone != "" {
		where += " is in " + g.Zone
	}
	where += fmt.Sprintf(" (%.0f, %.0f)", g.X, g.Y)
	fit := ""
	if c.Level != nil {
		fit = wording.Fit(g.Level, *c.Level)
	}
	l.Details = finishDetails([]string{where, fit})
	l.Asks = giverAsks(g.NPC, g.Title)
	l.Why = questWhy(c, l)
	l.Ref = npcRef(g.NPCID)
	if g.NPCID == 0 {
		l.Ref = questRef(g.QuestID)
	}
	return l
}

// AskRef is one suggested question and the subject it is about.
type AskRef struct {
	Ask, Ref string
}

// Subject is what the advisor expands a ref into: the entry's own text and details, and where the
// plan puts it (Place is 0 when the entry is not a plan step).
type Subject struct {
	Ref, Kind, Text string
	Details         []string
	Place, Of       int // the 1-based position in the next-quests plan, and its length
	// Line is the entry itself when it is a section line (not a dungeon or talent line): its ids,
	// giver, zone, level, class flag, why and objectives.
	Line *Line
}

// AllAsks lists every entry's questions with their refs.
func (d Digest) AllAsks() []AskRef {
	var out []AskRef
	add := func(asks []string, ref string) {
		for _, a := range asks {
			out = append(out, AskRef{a, ref})
		}
	}
	for _, s := range d.Sections {
		for _, l := range s.Lines {
			add(l.Asks, l.Ref)
		}
	}
	for _, l := range d.Dungeons {
		add(l.Asks, l.Ref)
	}
	for _, l := range d.Talents {
		add(l.Asks, l.Ref)
	}
	return out
}

// Subject finds the first entry with this ref; ok is false when none has it.
func (d Digest) Subject(ref string) (Subject, bool) {
	if ref == "" {
		return Subject{}, false
	}
	for _, s := range d.Sections {
		for i, l := range s.Lines {
			if l.Ref != ref {
				continue
			}
			line := l
			sub := Subject{Ref: ref, Kind: l.Kind, Text: l.Text, Details: l.Details, Line: &line}
			if s.ID == NextQuests {
				sub.Place, sub.Of = i+1, len(s.Lines)
			}
			return sub, true
		}
	}
	for _, l := range d.Dungeons {
		if l.Ref == ref {
			return Subject{Ref: ref, Kind: "dungeon", Text: l.Name + ": " + l.Line, Details: l.Details}, true
		}
	}
	for _, l := range d.Talents {
		if l.Ref == ref {
			return Subject{Ref: ref, Kind: "talent", Text: l.Line, Details: l.Details}, true
		}
	}
	return Subject{}, false
}

// SameAsk reports whether two questions are the same ignoring case and punctuation.
func SameAsk(a, b string) bool {
	norm := func(s string) string {
		var out []rune
		for _, r := range strings.ToLower(s) {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				out = append(out, r)
			} else if unicode.IsSpace(r) && len(out) > 0 && out[len(out)-1] != ' ' {
				out = append(out, ' ')
			}
		}
		return strings.TrimSpace(string(out))
	}
	return norm(a) != "" && norm(a) == norm(b)
}

// RefFor is the ref of the entry whose suggested question is exactly text (case and punctuation
// aside); "" when none is.
func (d Digest) RefFor(text string) string {
	for _, a := range d.AllAsks() {
		if SameAsk(a.Ask, text) {
			return a.Ref
		}
	}
	return ""
}

// whoOf is the character as the player says it: "Fury warrior", "warrior" when no spec is known, ""
// when neither is.
func whoOf(c character.Character) string {
	class := strings.ToLower(c.Class)
	if g := groupOf(c); g != "" && class != "" {
		return g + " " + class
	}
	return class
}

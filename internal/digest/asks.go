package digest

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/WoW-Companion/advisor/internal/advice"
	"github.com/WoW-Companion/advisor/internal/character"
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

// questAsks are the questions of a quest line.
func questAsks(l Line) []string {
	t := l.Title
	if t == "" {
		t = "this quest"
	}
	switch l.Kind {
	case KindTurnIn:
		return asksOf("What does turning this in unlock?",
			fmt.Sprintf("What does turning in %s unlock?", t), "What should I do after I turn it in?")
	case KindPickup:
		return asksOf("Is this quest worth taking now?",
			fmt.Sprintf("What's %s for?", t), fmt.Sprintf("Is %s worth taking now?", t))
	}
	var cands []string
	for _, o := range l.Objectives {
		if o.HasCount && o.Have >= o.Need {
			continue
		}
		if isItemObjective(o.Text) {
			cands = append(cands, fmt.Sprintf("Where do I find %s?", o.Text))
		}
		break
	}
	cands = append(cands, fmt.Sprintf("How do I finish %s?", t))
	return asksOf("How do I finish this quest?", cands...)
}

func talentAsks(points int, pick string) []string {
	where := "Where should my talent point go?"
	if points > 1 {
		where = "Where should my talent points go?"
	}
	if pick == "" {
		return asksOf(where, where)
	}
	return asksOf(where, where, fmt.Sprintf("What does %s do?", pick))
}

func upgradeAsks(item string) []string {
	if item == "" {
		return asksOf("Is this upgrade worth getting now?", "Is this upgrade worth getting now?")
	}
	return asksOf("Is this upgrade worth getting now?",
		fmt.Sprintf("How do I get %s?", item), fmt.Sprintf("Is %s worth getting now?", item))
}

// shoppingAsks are the questions of a slot's Auction House shopping list.
func shoppingAsks() []string {
	return asksOf("What should I search for on the Auction House?", "What should I search for on the Auction House?", "Is this worth buying on the Auction House?")
}

func materialAsks(item string) []string {
	return asksOf("Should I keep this?", fmt.Sprintf("Should I keep %s?", item), fmt.Sprintf("What is %s used for?", item))
}

func dungeonAsks(name string) []string {
	return asksOf("Am I ready for this dungeon?", fmt.Sprintf("Am I ready for %s?", name), fmt.Sprintf("What should I bring to %s?", name))
}

func trainingAsks() []string {
	return []string{"What should I train first?", "Which new abilities matter most?"}
}

// giverAsks is the question of a giver note.
func giverAsks(npc string) []string {
	return asksOf("What does this NPC have for me?", fmt.Sprintf("What does %s have for me?", npc))
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
	l.Asks = giverAsks(g.NPC)
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

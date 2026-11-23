package ask

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/WoW-Companion/advisor/internal/advice"
	"github.com/WoW-Companion/advisor/internal/brief"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/digest"
	"github.com/WoW-Companion/advisor/internal/profile"
	"github.com/WoW-Companion/advisor/internal/questie"
	"github.com/WoW-Companion/advisor/internal/route"
	"github.com/WoW-Companion/advisor/internal/store"
	"github.com/WoW-Companion/advisor/internal/unlock"
)

// Pre-fetched facts: the character's brief and next steps are put in the prompt, so most questions
// need no tool call (each tool round trip resends the whole context). The tools stay available.
const (
	// FactsBudget is the most characters of pre-fetched facts in a prompt (about 1.5k tokens).
	FactsBudget = 6000
	// FactsSteps is how many next steps are pre-fetched.
	FactsSteps = 5
	// FactsPlanSteps is how many route plan stops are pre-fetched (the plan's count covers the rest).
	FactsPlanSteps = 8
)

// Facts is the pre-fetched facts block for q's character, or "" when the character cannot be
// chosen or the capture cannot be read (the agent then uses its tools, as before). The block is cut
// at FactsBudget characters.
func Facts(cfg Config, q Question) string {
	cc, ok := load(cfg, q.Character)
	if !ok {
		return ""
	}
	c, f := cc.c, cc.f
	if cc.ds == nil {
		return NoDataFacts + "\n\n" + subjectFacts(cfg, cc, q) + capRunes(brief.Character(c, f.ClientOf(c), time.Now()), FactsBudget)
	}
	all := advice.WhatsNextWith(c, cc.ds, cc.qx)
	next := advice.Render(c, advice.Limit(all, FactsSteps), len(all))
	if cc.qx != nil {
		next += "\n\n" + route.Render(route.BuildWith(c, cc.qx, route.OptionsFor(cfg.SavedVariables, cc.ds)), FactsPlanSteps)
	}
	var giver string
	if g, ok := digest.FindGiver(c, cc.qx, digest.PlanQuests(all), digest.PlanNPCs(all)); ok {
		giver = "Quest giver with something for the character now (not in the plan): " + g.Text() + "."
	}
	return capRunes(subjectFacts(cfg, cc, q)+brief.Character(c, f.ClientOf(c), time.Now())+"\n\n"+styleLine(c)+"\n\n"+next+join(giver), FactsBudget)
}

func join(s string) string {
	if s == "" {
		return ""
	}
	return "\n\n" + s
}

// loaded is a character read for a question.
type loaded struct {
	c  character.Character
	f  character.File
	ds store.DataStore
	qx *questie.Index
}

// load reads the capture and picks the question's character, with its saved play style; ok is false
// when the capture cannot be read or the character chosen (the agent then uses its tools).
func load(cfg Config, key string) (loaded, bool) {
	if cfg.SavedVariables == "" {
		return loaded{}, false
	}
	f, err := character.LoadFile(cfg.SavedVariables)
	if err != nil {
		return loaded{}, false
	}
	c, err := brief.Choose(f, key)
	if err != nil {
		return loaded{}, false
	}
	l := loaded{c: character.AttachTotals(unlock.AddonSV(cfg.SavedVariables), profile.Attach(profile.Path(), c)), f: f}
	if cfg.Dataset != "" {
		l.ds = store.NewDir(cfg.Dataset)
	}
	l.qx = questie.ForSavedVariables(cfg.SavedVariables)
	return l, true
}

// digestOf is the digest the player's panel shows for the loaded character.
func (l loaded) digest() digest.Digest {
	return digest.Build(l.c, l.ds, digest.Options{Now: time.Now(), Client: l.f.ClientOf(l.c), Questie: l.qx})
}

// ResolveRef is q's ref: the one it carries, else that of the current suggested question of the
// character that the text matches exactly (case and punctuation aside); "" when neither.
func ResolveRef(cfg Config, q Question) string {
	if q.Ref != "" {
		return q.Ref
	}
	cc, ok := load(cfg, q.Character)
	if !ok {
		return ""
	}
	return cc.digest().RefFor(q.Text)
}

// GroundingRules tell the agent how to use a subject's facts: they are the answer's first source, ids
// are the key for lookups (a quest or item can be missing from the dataset by name), and a miss is
// never reported to the player as "no match".
const GroundingRules = "How to answer: answer from these facts first. Look anything up by id (quest id, NPC id, item id), " +
	"never by name alone. A lookup that finds nothing is not an answer: never tell the player there was \"no match\" or that " +
	"you cannot tell what this is. When the facts are thin, say plainly what they do show (who gives it, where, the level, " +
	"whether it is a class quest, why it is listed) and what it usually means. Never invent specifics such as a reward, " +
	"an ability name or a location that is not in the facts."

// JustReadRule is added whenever a question arrives with a ref: the player clicked the entry and has
// read its card, so the answer is only what the card does not say, in the short answer shape.
const JustReadRule = "The player has just read this entry's card, so don't repeat its facts. Answer only the new part of their question, in the short shape above."

// subjectFacts is the block that expands q's ref into the entry's facts and its place in the plan, with
// a closing blank line; "" without a ref or when the entry is gone. The entry's own facts (title,
// kind, ids, giver, zone, coordinates, level, class flag, why, details, plan step) are always put in.
func subjectFacts(cfg Config, cc loaded, q Question) string {
	d := cc.digest()
	ref := q.Ref
	if ref == "" {
		ref = d.RefFor(q.Text)
	}
	sub, ok := d.Subject(ref)
	if !ok {
		return ""
	}
	var b strings.Builder
	b.WriteString("The player's short question is about this entry of their panel; answer it with this context.\nSubject: " + sub.Text + "\n")
	b.WriteString(entryFacts(cfg, cc, sub))
	for _, d := range sub.Details {
		b.WriteString("- " + d + "\n")
	}
	if sub.Place > 0 {
		fmt.Fprintf(&b, "Place in the plan: step %d of %d next steps.\n", sub.Place, sub.Of)
	} else {
		b.WriteString("Place in the plan: not one of the plan's steps.\n")
	}
	b.WriteString(GroundingRules + "\n")
	b.WriteString(JustReadRule + "\n")
	if sub.Line != nil && len(sub.Line.Asks) > 0 && digest.SameAsk(q.Text, sub.Line.Asks[0]) {
		if r := PathStepsRule(sub.Line.PathKind); r != "" {
			b.WriteString(r + "\n")
		}
	}
	return b.String() + "\n"
}

// entryFacts lists the structured facts of a subject, one "Label: value" line each; nothing unknown.
func entryFacts(cfg Config, cc loaded, sub digest.Subject) string {
	var b strings.Builder
	put := func(k, v string) {
		if v != "" {
			b.WriteString(k + ": " + v + "\n")
		}
	}
	put("Ref", sub.Ref)
	put("Kind", sub.Kind)
	l := sub.Line
	if l == nil {
		return b.String()
	}
	put("Title", l.Title)
	if l.Quest > 0 {
		put("Quest id", fmt.Sprint(l.Quest))
	}
	if l.Item > 0 {
		put("Item id", fmt.Sprint(l.Item))
	}
	if strings.HasPrefix(sub.Ref, "npc:") {
		put("NPC id", strings.TrimPrefix(sub.Ref, "npc:"))
	}
	put("Giver or turn-in NPC", l.NPC)
	put("Zone", l.Zone)
	if l.Level > 0 {
		put("Level", fmt.Sprint(l.Level))
	}
	if l.Class {
		put("Class quest", "yes (this character's own class)")
	}
	if l.Elite {
		put("Elite or group quest", "yes")
	}
	put("Why it is listed", l.Why)
	for _, o := range l.Objectives {
		put("Objective", o.Text)
	}
	if l.Quest > 0 && cc.qx != nil && cc.ds != nil {
		p := route.BuildWith(cc.c, cc.qx, route.OptionsFor(cfg.SavedVariables, cc.ds))
		for _, s := range p.Steps {
			if s.QuestID == l.Quest {
				put("Plan step", fmt.Sprintf("%d. %s (%.0f, %.0f)", s.N, s.Label, s.X, s.Y))
				break
			}
		}
	}
	return b.String()
}

// styleLine is one line of the saved play style, or the reminder that none is saved.
func styleLine(c character.Character) string {
	if c.Profile == nil {
		return "Play style: not saved yet (ask once before talent advice)."
	}
	s := "Play style: " + c.Profile.Role + ", " + c.Profile.Focus
	if c.Profile.Spec != "" {
		s += ", " + c.Profile.Spec
	}
	return s + "."
}

func capRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return strings.TrimRight(string([]rune(s)[:n]), " \n") + "\n[cut; use the tools for the rest]"
}

// NoDataFacts stands in for the next steps when no game dataset has downloaded.
const NoDataFacts = "Game data not downloaded yet; see Setup. Tell the player this, in those words, rather than guessing quests, zones or gear."

// PromptWithFacts is PromptFor plus the pre-fetched facts, when there are any.
func PromptWithFacts(q Question, length, facts string) string {
	p := PromptFor(q, length)
	if facts == "" {
		return p
	}
	i := strings.LastIndex(p, "\n\nQuestion: ")
	return p[:i] + "\n\nThe character's current facts are below, already fetched: answer from them and call a tool only for something they lack.\n---\n" + facts + "\n---" + p[i:]
}

// Prompt is what the agent is asked for q under cfg: PromptFor with the pre-fetched facts.
func (c Config) Prompt(q Question) string {
	p := PromptWithFacts(q, c.Options.Length, Facts(c, q))
	if !TalentQuestion(q.Text) {
		return p
	}
	i := strings.Index(p, " Plain text, under ")
	if i < 0 {
		return p
	}
	return p[:i] + talentRule() + p[i+1:]
}

// PathStepsRule is added when the player asks an upgrade's first question, the one that follows the
// path the card suggests: they want the steps, not the card's one line again. It keeps the short
// answer shape (one answer line, then at most 3 steps) and takes its facts from upgrade_paths.
func PathStepsRule(kind string) string {
	const shape = " Call upgrade_paths with the item id for the facts. Give one answer line, then at most 3 steps as short bullets; do not repeat the card's one line."
	switch kind {
	case "craft":
		return "The player asks how to make this item. Steps, in order: where the trainer is if the recipe is not known yet; the materials with the counts they have and still need, and where to get each missing one (mining nodes, a vendor or the Auction House); where to craft it (a forge and anvil for blacksmithing)." + shape
	case "quest":
		return "The player asks how to get this item from its quest. Steps: where to pick up the quest, what it asks, where to hand it in and choose the reward." + shape
	case "auction":
		return "The player asks how to find this on the Auction House. Steps: where the Auction House is, the Browse filters (slot, level range, quality), and what to look for in the results. Never name a price." + shape
	case "dungeon":
		return "The player asks how to get this item from a dungeon boss. Steps: where the dungeon entrance is, how to reach the boss, and how often it drops." + shape
	case "creature":
		return "The player asks where to farm this item. Steps: which creature carries it, where it lives, and how often it drops." + shape
	}
	return ""
}

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
		return NoDataFacts + "\n\n" + subjectFacts(cc, q) + capRunes(brief.Character(c, f.ClientOf(c), time.Now()), FactsBudget)
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
	return capRunes(subjectFacts(cc, q)+brief.Character(c, f.ClientOf(c), time.Now())+"\n\n"+styleLine(c)+"\n\n"+next+join(giver), FactsBudget)
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

// subjectFacts is the block that expands q's ref into the entry's facts and its place in the plan, with
// a closing blank line; "" without a ref or when the entry is gone.
func subjectFacts(cc loaded, q Question) string {
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
	for _, d := range sub.Details {
		b.WriteString("- " + d + "\n")
	}
	if sub.Place > 0 {
		fmt.Fprintf(&b, "Place in the plan: step %d of %d next steps.\n", sub.Place, sub.Of)
	} else {
		b.WriteString("Place in the plan: not one of the plan's steps.\n")
	}
	return b.String() + "\n"
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
	if c.Plan != nil {
		s += "; talent plan from " + c.Plan.GuideURL + " saved " + c.Plan.Date
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
	return p[:i] + talentRule(CanFetchGuide(c.Command, q.Text)) + p[i+1:]
}

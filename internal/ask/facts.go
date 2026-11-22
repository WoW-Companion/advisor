package ask

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/WoW-Companion/advisor/internal/advice"
	"github.com/WoW-Companion/advisor/internal/brief"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/profile"
	"github.com/WoW-Companion/advisor/internal/questie"
	"github.com/WoW-Companion/advisor/internal/route"
	"github.com/WoW-Companion/advisor/internal/store"
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
	if cfg.SavedVariables == "" {
		return ""
	}
	f, err := character.LoadFile(cfg.SavedVariables)
	if err != nil {
		return ""
	}
	c, err := brief.Choose(f, q.Character)
	if err != nil {
		return ""
	}
	c = profile.Attach(profile.Path(), c)
	var ds store.DataStore
	if cfg.Dataset != "" {
		ds = store.NewDir(cfg.Dataset)
	}
	if ds == nil {
		return NoDataFacts + "\n\n" + capRunes(brief.Character(c, f.ClientOf(c), time.Now()), FactsBudget)
	}
	qx := questie.ForSavedVariables(cfg.SavedVariables)
	all := advice.WhatsNextWith(c, ds, qx)
	next := advice.Render(c, advice.Limit(all, FactsSteps), len(all))
	if qx != nil {
		next += "\n\n" + route.Render(route.Build(c, qx), FactsPlanSteps)
	}
	return capRunes(brief.Character(c, f.ClientOf(c), time.Now())+"\n\n"+styleLine(c)+"\n\n"+next, FactsBudget)
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

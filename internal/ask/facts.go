package ask

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/WoW-Companion/advisor/internal/advice"
	"github.com/WoW-Companion/advisor/internal/brief"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/store"
)

// Pre-fetched facts: the character's brief and next steps are put in the prompt, so most questions
// need no tool call (each tool round trip resends the whole context). The tools stay available.
const (
	// FactsBudget is the most characters of pre-fetched facts in a prompt (about 1.5k tokens).
	FactsBudget = 6000
	// FactsSteps is how many next steps are pre-fetched.
	FactsSteps = 5
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
	var ds store.DataStore
	if cfg.Dataset != "" {
		ds = store.NewDir(cfg.Dataset)
	}
	all := advice.WhatsNext(c, ds)
	next := advice.Render(c, advice.Limit(all, FactsSteps), len(all))
	return capRunes(brief.Character(c, f.ClientOf(c), time.Now())+"\n\n"+next, FactsBudget)
}

func capRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return strings.TrimRight(string([]rune(s)[:n]), " \n") + "\n[cut; use the tools for the rest]"
}

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
	return PromptWithFacts(q, c.Options.Length, Facts(c, q))
}

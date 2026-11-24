package companion

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/WoW-Companion/advisor/internal/ask"
)

// The follow-up context card: what the player is talking about, not what was said. Each prompt carries
// at most CardEntities names, the last question (CardQuestionChars) and, for a pronoun follow-up only,
// one answer excerpt (CardExcerptChars): about 150 tokens. The tools still fetch every fact fresh.
const (
	CardEntities      = 6
	CardQuestionChars = 200
	CardExcerptChars  = 300
	CardNameChars     = 60
	CardTTL           = 30 * time.Minute // as long as the chat's gap divider
)

// card is the context of one character's last answered ask.
type card struct {
	Char     string
	Entities []ask.Entity // newest first
	Question string
	Answer   string
	At       time.Time
}

// ContextView is what the overlay shows as the "Following up on" chip.
type ContextView struct {
	Character string   `json:"character"`
	Names     []string `json:"names"` // newest first
	At        int64    `json:"at"`
}

var pronounRE = regexp.MustCompile(`(?i)\b(that|it|they|them|those|there|again|same)\b`)

// IsFollowUp reports whether q refers back with a pronoun.
func IsFollowUp(q string) bool { return pronounRE.MatchString(q) }

func cut(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "..."
	}
	return s
}

func (s *Server) clock() time.Time {
	if s.clockFn != nil {
		return s.clockFn()
	}
	return time.Now()
}

// label is an entity as the prompt names it: "The Skybreaker Bulwark (quest 94003)", "Seena (NPC)", "Zephras Isle".
func label(e ask.Entity) string {
	name := cut(e.Name, CardNameChars)
	switch {
	case e.Kind == "zone" || e.Kind == "":
		return name
	case e.Kind == "npc":
		return name + " (NPC)"
	case e.ID > 0:
		return fmt.Sprintf("%s (%s %d)", name, e.Kind, e.ID)
	}
	return name + " (" + e.Kind + ")"
}

// prompt is the card as a block of prompt text ahead of question q, empty when there is nothing to say.
func (c *card) prompt(q string) string {
	var parts []string
	if len(c.Entities) > 0 {
		var ls []string
		for _, e := range c.Entities {
			ls = append(ls, label(e))
		}
		parts = append(parts, "Talking about: "+strings.Join(ls, "; ")+".")
	}
	if c.Question != "" {
		parts = append(parts, "Previous question: "+c.Question)
	}
	if c.Answer != "" && IsFollowUp(q) {
		parts = append(parts, "Previous answer, excerpt: "+c.Answer)
	}
	if len(parts) == 0 {
		return ""
	}
	return "Context for resolving references only; look facts up again with the tools:\n" + strings.Join(parts, " ") + "\n\nCurrent question: "
}

// activeCard is the character's card, or nil when it has expired or belongs to another character.
// The caller holds s.mu.
func (s *Server) activeCard(char string) *card {
	c := s.card
	if c == nil || c.Char != char || s.clock().Sub(c.At) > CardTTL {
		return nil
	}
	return c
}

// historyFor is the context card of one character as prompt text ahead of the new question. The caller holds s.mu.
func (s *Server) historyFor(char, question string) string {
	if c := s.activeCard(char); c != nil {
		return c.prompt(question)
	}
	return ""
}

// remember records the card after an answered ask. An answer whose tools touched nothing keeps the
// earlier names, so a follow-up chain stays resolvable. The caller holds s.mu.
func (s *Server) remember(char, question, answer string, touched []ask.Entity) *ContextView {
	return s.rememberSubjects(char, question, answer, nil, touched)
}

// rememberSubjects is remember with the entities the answer text names, first mention first: they lead
// the card, so the chip shows what the answer is about, then come the touched ones (newest first) and
// the earlier card's names.
func (s *Server) rememberSubjects(char, question, answer string, subjects, touched []ask.Entity) *ContextView {
	var ents []ask.Entity
	seen := map[string]bool{}
	named := map[string]bool{} // the answer's subjects, by name: a touched entity of the same name adds nothing
	add := func(e ask.Entity) {
		if named[strings.ToLower(strings.TrimSpace(e.Name))] && !seen[fmt.Sprintf("%s|%d|%s", e.Kind, e.ID, strings.ToLower(e.Name))] {
			return
		}
		k := fmt.Sprintf("%s|%d|%s", e.Kind, e.ID, strings.ToLower(e.Name))
		if len(ents) < CardEntities && !seen[k] && strings.TrimSpace(e.Name) != "" {
			seen[k] = true
			ents = append(ents, e)
		}
	}
	for _, e := range subjects {
		add(e)
		named[strings.ToLower(strings.TrimSpace(e.Name))] = true
	}
	for i := len(touched) - 1; i >= 0; i-- {
		add(touched[i])
	}
	if old := s.activeCard(char); old != nil {
		for _, e := range old.Entities {
			add(e)
		}
	}
	s.card = &card{Char: char, Entities: ents, Question: cut(question, CardQuestionChars), Answer: cut(answer, CardExcerptChars), At: s.clock()}
	return s.card.view()
}

func (c *card) view() *ContextView {
	if c == nil || len(c.Entities) == 0 {
		return nil
	}
	v := &ContextView{Character: c.Char, At: c.At.Unix()}
	for _, e := range c.Entities {
		v.Names = append(v.Names, cut(e.Name, CardNameChars))
	}
	return v
}

// contextView is the active card for the state snapshot. The caller holds s.mu.
func (s *Server) contextView() *ContextView {
	if s.card == nil || s.clock().Sub(s.card.At) > CardTTL {
		return nil
	}
	return s.card.view()
}

// clearContext drops the card (the chip's ×, and the chat's Clear button).
func (s *Server) clearContext(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.card = nil
	s.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

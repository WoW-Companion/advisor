package companion

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/WoW-Companion/advisor/internal/ask"
)

func cardServer() (*Server, *time.Time) {
	t0 := time.Unix(1_700_000_000, 0)
	s := &Server{}
	s.clockFn = func() time.Time { return t0 }
	return s, &t0
}

var bulwark = []ask.Entity{
	{Kind: "npc", Name: "Seena Skybreaker"},
	{Kind: "quest", ID: 94003, Name: "The Skybreaker Bulwark"},
}

func TestCardPrompt(t *testing.T) {
	s, _ := cardServer()
	s.remember("c", "Where do I hand in the Bulwark quest?", "Go to the isle. "+strings.Repeat("x", 500), bulwark)
	got := s.historyFor("c", "Who gives that quest again?")
	for _, want := range []string{"Talking about: The Skybreaker Bulwark (quest 94003); Seena Skybreaker (NPC).", "Previous question: Where do I hand in", "Previous answer, excerpt: Go to the isle", "Current question: "} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
	if n := strings.Count(got, "x"); n > CardExcerptChars {
		t.Errorf("%d excerpt chars", n)
	}
	if plain := s.historyFor("c", "What about Orgrimmar"); strings.Contains(plain, "excerpt") {
		t.Errorf("excerpt without a pronoun: %q", plain)
	}
}

func TestCardCaps(t *testing.T) {
	s, _ := cardServer()
	var ents []ask.Entity
	for i := 1; i <= 9; i++ {
		ents = append(ents, ask.Entity{Kind: "item", ID: int64(i), Name: strings.Repeat("n", 100)})
	}
	v := s.remember("c", strings.Repeat("q", 500), "a", ents)
	if len(v.Names) != CardEntities || len(s.card.Entities) != CardEntities {
		t.Errorf("%d entities", len(s.card.Entities))
	}
	if s.card.Entities[0].ID != 9 {
		t.Errorf("newest first: %+v", s.card.Entities[0])
	}
	if len([]rune(s.card.Question)) > CardQuestionChars+3 {
		t.Errorf("question %d runes", len([]rune(s.card.Question)))
	}
	if n := len([]rune(s.historyFor("c", "it"))); n > 1200 {
		t.Errorf("context %d chars, want about 150 tokens", n)
	}
	// An answer that touched nothing keeps the earlier names, and a repeat does not duplicate.
	s.remember("c", "next", "a", nil)
	if len(s.card.Entities) != CardEntities {
		t.Errorf("names lost: %d", len(s.card.Entities))
	}
	s.remember("c", "again", "a", ents[8:])
	if s.card.Entities[0].ID != 9 || s.card.Entities[1].ID == 9 {
		t.Errorf("duplicate: %+v", s.card.Entities[:2])
	}
}

func TestIsFollowUp(t *testing.T) {
	for q, want := range map[string]bool{
		"Who gives that quest again?": true, "Is it worth it": true, "Where are they": true, "same as before": true,
		"What drops there?": true, "Those too?": true, "Where is Orgrimmar": false, "What about Italy": false, "Give me a quest": false,
	} {
		if IsFollowUp(q) != want {
			t.Errorf("%q: want %v", q, want)
		}
	}
}

func TestCardExpiry(t *testing.T) {
	s, now := cardServer()
	s.remember("c", "q", "a", bulwark)
	if s.historyFor("c", "that") == "" || s.contextView() == nil {
		t.Fatal("fresh card missing")
	}
	if s.historyFor("d", "that") != "" {
		t.Error("another character got the card")
	}
	*now = now.Add(CardTTL)
	if s.historyFor("c", "that") == "" {
		t.Error("expired at exactly 30 minutes")
	}
	*now = now.Add(time.Second)
	if s.historyFor("c", "that") != "" || s.contextView() != nil {
		t.Error("card survived 30 minutes")
	}
	// Expired names are not carried into the next card.
	s.remember("c", "q2", "a", nil)
	if s.contextView() != nil {
		t.Error("stale names carried over")
	}
	// Clear.
	s.remember("c", "q", "a", bulwark)
	rec := httptest.NewRecorder()
	s.clearContext(rec, httptest.NewRequest("POST", "/api/context/clear", nil))
	if rec.Code != 204 || s.historyFor("c", "that") != "" || s.contextView() != nil {
		t.Error("clear left the card")
	}
}

func TestPromptHasNoRawHistory(t *testing.T) {
	s, _ := cardServer()
	s.web = []Entry{{Character: "c", Question: "OLDQUESTION", Answer: "OLDANSWER", Status: "done"}}
	s.remember("c", "q1", "a1", bulwark)
	if got := s.historyFor("c", "Is it far?"); strings.Contains(got, "OLD") || strings.Count(got, "Previous answer") != 1 {
		t.Errorf("raw history in %q", got)
	}
}

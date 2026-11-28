package ask

import (
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
)

func TestNextQuestionWords(t *testing.T) {
	for _, q := range []string{"what's next", "whats next?", "what now?", "what do I do now?", "what should I be doing?", "what should I do?", "what's my next step?",
		"where should I go next?", "what should I work on next?", "what are my next steps", "next steps?", "I'm level 20, what's next?", "what now, I'm level 31"} {
		if !NextQuestion(Question{Text: q}) {
			t.Errorf("%q is a What next? question", q)
		}
		if c := Classify(q, nil, nil); c.Kind != KindState {
			t.Errorf("%q is the player's own question, got kind %d", q, c.Kind)
		}
	}
	for _, q := range []string{"what should I do with this sword?", "what is the next rank of Frostbolt?", "what is the next boss in the Deadmines?", "what now that I killed Edwin?"} {
		if NextQuestion(Question{Text: q}) {
			t.Errorf("%q is not a What next? question", q)
		}
	}
	if NextQuestion(Question{Text: "what's next", Ref: "talents"}) {
		t.Error("a talents card question carries no heads-up")
	}
}

func TestLevelUpSubjectWords(t *testing.T) {
	for _, tc := range []struct{ q, want string }{
		{"what will I learn at 12", "12"}, {"what do I get at 40", "40"}, {"which spells do I get at 14", "14"}, {"what does my class get at 20?", "20"},
		{"what do I get when I hit 30", "30"}, {"what do I learn next", "next"}, {"what spells do I get when I ding", "next"},
		{"do I have enough gold to train", "now"}, {"can I afford my next spells", "now"}, {"how much money do I need to train at 30", "30"},
	} {
		if c := Classify(tc.q, nil, nil); c.Topic != TopicLevelUp || c.Subject != tc.want {
			t.Errorf("%q: topic %q subject %q, want levelup %q", tc.q, c.Topic, c.Subject, tc.want)
		}
	}
	for _, q := range []string{"what do I get at 5 gold", "what will I learn by 10 minutes", "what do I get at 90", "can I afford a mount", "can I afford riding training"} {
		if c := Classify(q, nil, nil); c.Topic == TopicLevelUp {
			t.Errorf("%q names no level or training: %q", q, c.Subject)
		}
	}
}

func TestProfessionRankQuestions(t *testing.T) {
	for _, q := range []string{"when can I train expert", "what skill cap do I have at level 20", "what's the max skill for apprentice", "when do I train my professions again"} {
		if c := Classify(q, nil, nil); c.Topic != TopicProfession || c.Subject != ranksSubject {
			t.Errorf("%q: topic %q subject %q, want the rank table", q, c.Topic, c.Subject)
		}
	}
	if c := Classify("how do I level my mining", nil, nil); c.Subject != "mining" {
		t.Errorf("a named profession keeps its name: %+v", c)
	}
	n := 20
	cc := loaded{c: character.Character{Level: &n}}
	g := ground(cc, Question{Text: "when can I train expert"})
	if g.Topic != TopicProfession || g.Pack != "" {
		t.Errorf("no dataset, no rank facts: %+v", g)
	}
}

func TestLimitFactsCheckpoints(t *testing.T) {
	for _, tc := range []struct{ q, want string }{
		{"what should I have by level 40", "Level milestones:"}, {"what unlocks at level 40?", "Level milestones:"}, {"level 40 milestones", "Level milestones:"},
		{"how much gold should I have at 40", "Gold targets:"}, {"what should I spend my gold on", "Gold targets:"}, {"what should I buy at level 20", "Gold targets:"},
		{"how much gold for gear at 30", "Gold targets:"}, {"am I geared for level 40", "Gear checkpoints:"}, {"am I ready for Wailing Caverns", "Gear checkpoints:"},
		{"is my gear ok for my level", "Gear checkpoints:"}, {"should I buy bags", "Bag and bank size:"}, {"what bags should I have at level 30", "Bag and bank size:"},
	} {
		if got := LimitFacts(tc.q); !strings.Contains(got, tc.want) || !strings.Contains(got, "Say so") {
			t.Errorf("LimitFacts(%q) = %q, want %q", tc.q, got, tc.want)
		}
	}
	for _, q := range []string{"what quests can I do at level 20?", "how much gold do I have?", "do I have enough gold to train", "how much money do I need to train at 30", "where do I buy bags", "is this sword an upgrade for me?"} {
		got := LimitFacts(q)
		for _, l := range []string{"Level milestones:", "Gold targets:", "Gear checkpoints:"} {
			if strings.Contains(got, l) {
				t.Errorf("LimitFacts(%q) has %q", q, l)
			}
		}
	}
}

func TestGroundNowWindowIsTheSpellsDueNow(t *testing.T) {
	n, money := 12, 50
	c := character.Character{Class: "WARRIOR", Faction: "Alliance", Level: &n, Money: &money,
		Seen: &character.Observed{Trainers: []character.SeenTrainer{{NPC: 9, Name: "Trainer Tam", Faction: "Alliance", T: 1,
			Offers: []character.SeenOffer{{Name: "Rend", Rank: "Rank 2", State: "available", Cost: 700, Level: 10}}}}}}
	g := ground(loaded{c: c, ds: khLoaded(t).ds}, Question{Text: "can I afford my next spells"})
	if g.Topic != TopicLevelUp || g.Subject != "now" || !strings.Contains(g.Pack, "Rend (Rank 2)") || !strings.Contains(g.Pack, "short") {
		t.Errorf("the money check of the spells due now: %+v", g)
	}
}

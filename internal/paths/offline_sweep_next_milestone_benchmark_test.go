package paths_test

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/advice"
	"github.com/WoW-Companion/advisor/internal/ask"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/levelup"
	"github.com/WoW-Companion/advisor/internal/professions"
)

// The offline what-to-do-next, level-milestone and gold-and-gear checkpoint sweep: what the advisor says to synthetic characters (all nine
// classes, levels across 1 to 60, both factions, varied gold) when they ask what to do next, what a level brings, whether their gold covers
// training and what they should have by a level, beside the dataset's rows and the game's own rules, with no network. testdata/levelling_snapshot
// holds the class spells and testdata/tradeskills_snapshot the profession rank gates. The dataset holds no level milestone list, no gold target,
// no gear target and no riding, mount or bag row, so those questions must reach the line that says so, in whatever words a player uses; a
// next-step question must be read as the player's own (a personal question that may carry a heads-up), and a level question must reach the
// class trainer's facts for the level it names. The reference is written apart from the advisor: the documented ranking rules (quests within 5
// levels below and 3 above, turn-ins before open quests, unspent talent points first), the game's one talent point per level from level 10,
// and each profession rank's character level (Journeyman 10, Expert 20, Artisan 35). A difference the dataset's own rows cause is marked
// "(data)". make offline-benchmark runs it; WOWC_BENCH_REPORT receives the tables.

const (
	nmNextSrc  = "docs/advice.md ranking rules (milestones, turn-ins, open quests within the level window); WoW Companion What next? question"
	nmLevelSrc = "dataset class_spells (class_id, name, rank, level); Wowhead Classic class trainer lists; Icy Veins WoW Forever leveling guides"
	nmNoneSrc  = "the dataset has no level milestone, gold target, gear target, riding, mount or bag row (limit lines of the advisor)"
	nmRankSrc  = "dataset profession_notes (ranks: skill_min, skill_cap, min_level); game rule: Journeyman at level 10, Expert 20, Artisan 35"
)

func nmChar(class string, i, level int) character.Character {
	return character.Character{Name: "Benchtest", Faction: []string{"Alliance", "Horde"}[i%2], Class: class, Level: &level}
}

// nmNextAsks are everyday words for the What next? question; the answer is the player's own plan, with a heads-up allowed.
var nmNextAsks = []string{
	"What should I do next?", "what next?", "what's next", "whats next?", "what now?", "what do I do now?", "what should I do now", "what should I be doing?",
	"what should I do?", "what do I do next", "what's my next step?", "I'm lost, what should I do", "where should I go next?", "what's the next thing to do?",
	"any ideas on what to do next", "what are my next steps", "what should I focus on?", "what should I work on next?", "next steps?", "tell me what to do next",
	"What should I do first?", "i'm bored what should I do", "what do I do after this? what's next", "what should I tackle next",
}

// TestOfflineBenchmarkSweepNextPhrasings: every phrasing of the What next? question, with and without a level, is the player's own question
// (a state question, not a game-knowledge one) and carries the heads-up line.
func TestOfflineBenchmarkSweepNextPhrasings(t *testing.T) {
	var out []benchRow
	for _, q := range nmNextAsks {
		bad := 0
		var first string
		for i, level := range []int{0, 12, 31, 48} {
			text := q
			switch {
			case level > 0 && i%2 == 1:
				text = fmt.Sprintf("I'm level %d, %s", level, q)
			case level > 0:
				text = fmt.Sprintf("%s, I'm level %d", strings.TrimRight(q, "?"), level)
			}
			c := ask.Classify(text, nil, nil)
			if c.Kind != ask.KindState || !ask.NextQuestion(ask.Question{Text: text}) {
				bad++
				if first == "" {
					first = fmt.Sprintf("%q is kind %d, heads-up %v", text, c.Kind, ask.NextQuestion(ask.Question{Text: text}))
				}
			}
		}
		got := fmt.Sprintf("%d of 4 read as the player's own next-step question", 4-bad)
		if bad > 0 {
			got += "; " + first
		}
		out = append(out, benchRow{fmt.Sprintf("%q, with and without a level", q), got, "a personal question (the plan), heads-up line allowed", nmNextSrc, resultIf(bad == 0, "a phrasing read as game knowledge or without the heads-up")})
	}
	finish(t, "next-sweep-phrasings", "Offline what-next sweep: the question in a player's words", out)
}

// TestOfflineBenchmarkSweepNotNext: a question that only starts like the What next? question gets no heads-up.
func TestOfflineBenchmarkSweepNotNext(t *testing.T) {
	var out []benchRow
	for _, q := range []string{
		"what should I do with this sword?", "what is the next rank of Frostbolt?", "what is the next boss in the Deadmines?", "what should I do about my full bags",
		"what do I do with my extra gold", "what should I do in Stranglethorn", "what now that I killed Edwin?", "what is the next level of mining skill?",
	} {
		got := ask.NextQuestion(ask.Question{Text: q})
		out = append(out, benchRow{fmt.Sprintf("%q", q), fmt.Sprint("heads-up line: ", got), "a question about one thing, not the plan: no heads-up", nmNextSrc, resultIf(!got, "a heads-up line for a question that is not what next")})
	}
	finish(t, "next-sweep-not-next", "Offline what-next sweep: questions that are not what next", out)
}

// TestOfflineBenchmarkSweepNextWindow: for every class at levels across 1 to 60, a log of quests from 7 levels below to 5 above lists the
// quests from 5 below to 3 above (a ready turn-in always), the ready turn-in leads the open quests, and the open quests run from the level
// nearest the character's outward.
func TestOfflineBenchmarkSweepNextWindow(t *testing.T) {
	var out []benchRow
	for ci, class := range tzClasses {
		for _, level := range []int{2, 7, 13, 22, 34, 45, 60} {
			c := nmChar(class, ci, level)
			var want []int
			id := 100
			for off := -7; off <= 5; off++ {
				q := level + off
				if q < 1 {
					continue
				}
				id++
				c.Quests = append(c.Quests, character.Quest{QuestID: id, Title: fmt.Sprintf("Quest at %d", q), Level: ip(q), Zone: "Elwynn Forest"})
				if off >= -5 && off <= 3 {
					want = append(want, q)
				}
			}
			done := true
			c.Quests = append(c.Quests, character.Quest{QuestID: 900, Title: "Finished Errand", Level: ip(max(level-8, 1)), Zone: "Elwynn Forest", Complete: &done})
			steps := advice.WhatsNext(c, nil)
			var got []int
			turnFirst, lastDist, ordered := false, -1, true
			for i, s := range steps {
				if s.Kind == advice.KindTurnIn {
					turnFirst = turnFirst || i == 0
					continue
				}
				if s.Kind != advice.KindQuest || s.Level == 0 {
					continue
				}
				got = append(got, s.Level)
				d := s.Level - level
				if d < 0 {
					d = -d
				}
				if d < lastDist {
					ordered = false
				}
				lastDist = d
			}
			sort.Ints(got)
			sort.Ints(want)
			ref := fmt.Sprintf("quests at levels %v, the ready turn-in first, nearest level first", want)
			res := fmt.Sprint(got) == fmt.Sprint(want) && turnFirst && ordered
			out = append(out, benchRow{fmt.Sprintf("%s (%s) level %d, quests 7 below to 5 above", class, c.Faction, level), fmt.Sprintf("open quests at %v, turn-in first %v, nearest first %v", got, turnFirst, ordered), ref, nmNextSrc, resultIf(res, "the level window, the turn-in or the order")})
		}
	}
	finish(t, "next-sweep-window", "Offline what-next sweep: the quest window and order at every level", out)
}

// TestOfflineBenchmarkSweepNextMilestoneLeads: a character with unspent talent points (the game's one per level from 10) is told to spend
// them before any quest or turn-in, naming the count; with none unspent no talent step is listed.
func TestOfflineBenchmarkSweepNextMilestoneLeads(t *testing.T) {
	var out []benchRow
	for ci, class := range tzClasses {
		for _, level := range []int{9, 10, 25, 40, 60} {
			c := nmChar(class, ci, level)
			done := true
			c.Quests = []character.Quest{{QuestID: 1, Title: "Open Work", Level: ip(level), Zone: "Duskwood"}, {QuestID: 2, Title: "Done Work", Level: ip(level), Zone: "Duskwood", Complete: &done}}
			unspent := max(level-9, 0)
			c.Talents = &character.Talents{Unspent: &unspent}
			steps := advice.WhatsNext(c, nil)
			lead := len(steps) > 0 && steps[0].Kind == advice.KindTalent
			ok := (unspent == 0 && !lead) || (unspent > 0 && lead && strings.Contains(steps[0].Title, fmt.Sprintf("Spend %d talent", unspent)))
			first := "none"
			if len(steps) > 0 {
				first = steps[0].Title
			}
			out = append(out, benchRow{fmt.Sprintf("%s (%s) level %d, %d unspent talent points", class, c.Faction, level, unspent), "first step: " + first, fmt.Sprintf("spend the %d points first; no talent step at 0", unspent), nmNextSrc, resultIf(ok, "the talent milestone is missing, late or miscounted")})
		}
	}
	finish(t, "next-sweep-milestone-leads", "Offline what-next sweep: unspent talent points lead", out)
}

// nmLevelAsks are everyday words for what a level brings; {L} is the level asked about.
var nmLevelAsks = []string{
	"what's new at level {L}?", "what do I get at level {L}?", "what do I learn at level {L}", "what will I learn at {L}", "what new spells at level {L}",
	"what can I train at {L}", "what do I get at {L}", "what do I get when I hit {L}", "what abilities do I get at level {L}", "which spells do I get at {L}",
	"what does my class get at {L}?", "what comes at level {L}", "what's coming at level {L}", "what will I get by level {L}",
}

// TestOfflineBenchmarkSweepLevelBrings: every phrasing of "what does level L bring" reaches the class trainer's facts for the level it names,
// at every level, not the next level and not a game-knowledge answer.
func TestOfflineBenchmarkSweepLevelBrings(t *testing.T) {
	var out []benchRow
	for _, tpl := range nmLevelAsks {
		bad := 0
		var first string
		levels := []int{4, 6, 8, 10, 14, 20, 30, 40, 52, 60}
		for _, l := range levels {
			q := strings.ReplaceAll(tpl, "{L}", fmt.Sprint(l))
			c := ask.Classify(q, nil, nil)
			if c.Topic != ask.TopicLevelUp || c.Subject != fmt.Sprint(l) {
				bad++
				if first == "" {
					first = fmt.Sprintf("%q is topic %q subject %q", q, c.Topic, c.Subject)
				}
			}
		}
		got := fmt.Sprintf("%d of %d reach the level's trainer facts", len(levels)-bad, len(levels))
		if bad > 0 {
			got += "; " + first
		}
		out = append(out, benchRow{fmt.Sprintf("%q, levels 4 to 60", tpl), got, "the class_spells rows of the level named", nmLevelSrc, resultIf(bad == 0, "a phrasing that does not reach the named level")})
	}
	finish(t, "next-sweep-level-brings", "Offline level milestone sweep: what a level brings, in a player's words", out)
}

// TestOfflineBenchmarkSweepNextLevelAndAfford: "what do I learn next", "when I ding" and a money question about training reach the trainer facts
// (the next level, and the spells due now with the money check).
func TestOfflineBenchmarkSweepNextLevelAndAfford(t *testing.T) {
	var out []benchRow
	for _, tc := range []struct{ q, subject string }{
		{"what do I learn next", "next"}, {"what do I get next level?", "next"}, {"what spells do I get when I ding", "next"}, {"what do I get at my next level", "next"},
		{"what are the next two levels going to give me, what do I get", "next2"}, {"do I have enough gold to train", "now"}, {"can I afford my next spells", "now"}, {"can I afford training", "now"},
		{"do I have enough money to learn my new spells", "now"}, {"how much money do I need to train at 30", "30"}, {"how much will training cost me at level 20", "20"},
		{"can I afford the spells at level 12", "12"}, {"how much gold to train my spells", "now"},
	} {
		c := ask.Classify(tc.q, nil, nil)
		out = append(out, benchRow{fmt.Sprintf("%q", tc.q), fmt.Sprintf("topic %q subject %q", c.Topic, c.Subject), fmt.Sprintf("trainer facts, window %q", tc.subject), nmLevelSrc, resultIf(c.Topic == ask.TopicLevelUp && c.Subject == tc.subject, "another window or no trainer facts")})
	}
	finish(t, "next-sweep-next-and-afford", "Offline level milestone sweep: the next level and what training costs", out)
}

// TestOfflineBenchmarkSweepNotLevelBrings: a number after "at" or "by" that is no level (gold, minutes, mana) names no level window.
func TestOfflineBenchmarkSweepNotLevelBrings(t *testing.T) {
	var out []benchRow
	for _, q := range []string{
		"what do I get at 5 gold", "what can I train at 50 silver", "what will I learn by 10 minutes", "what do I get at 90", "what spells do I get at 12 mana",
	} {
		c := ask.Classify(q, nil, nil)
		out = append(out, benchRow{fmt.Sprintf("%q", q), fmt.Sprintf("topic %q subject %q", c.Topic, c.Subject), "no level named: no level window", nmLevelSrc, resultIf(c.Subject != "5" && c.Subject != "50" && c.Subject != "10" && c.Subject != "90" && c.Subject != "12", "an amount read as a level")})
	}
	finish(t, "next-sweep-not-level", "Offline level milestone sweep: numbers that are no level", out)
}

// nmSeen gives the character an opened class trainer that offers the spells due at the level, each at a known price, and returns the total.
func nmSeen(c character.Character, level int) (character.Character, int64) {
	var offers []character.SeenOffer
	var total int64
	for i, s := range levelup.Spells(c, levStore(), level, level) {
		cost := int64(100*level + 10*(i+1))
		rank := ""
		if s.Rank > 0 {
			rank = fmt.Sprintf("Rank %d", s.Rank)
		}
		offers = append(offers, character.SeenOffer{Name: s.Name, Rank: rank, State: "available", Cost: cost, Level: level})
		total += cost
	}
	c.Seen = &character.Observed{Trainers: []character.SeenTrainer{{NPC: 7, Name: "Benchtest Trainer", Faction: c.Faction, T: 1, Offers: offers}}}
	return c, total
}

// TestOfflineBenchmarkSweepTrainingMoney: with the trainer's prices seen, the money check says how far short a character is by the exact amount,
// and says the money covers it at exactly the total or above; with no money known it says nothing of money.
func TestOfflineBenchmarkSweepTrainingMoney(t *testing.T) {
	var out []benchRow
	for ci, class := range []string{"WARRIOR", "MAGE", "PRIEST", "DRUID"} {
		for _, level := range []int{16, 40} {
			base, total := nmSeen(nmChar(class, ci, level), level)
			if total == 0 {
				continue
			}
			for _, tc := range []struct {
				name  string
				money *int
				want  string
			}{{"1 copper short", ip(int(total) - 1), "short"}, {"exact", ip(int(total)), "covers"}, {"rich", ip(int(total) + 5000), "covers"}, {"unknown", nil, ""}} {
				c := base
				c.Money = tc.money
				facts, _, _ := levelup.Facts(c, levStore(), nil, level, level)
				short := strings.Contains(facts, "Money check: You have") && strings.Contains(facts, "short")
				covers := strings.Contains(facts, "Money check: the character's money covers it.")
				ok := tc.want == "short" && short || tc.want == "covers" && covers && !short || tc.want == "" && !short && !covers
				out = append(out, benchRow{fmt.Sprintf("%s (%s) level %d, %s, trainer total %d copper", class, c.Faction, level, tc.name, total), fmt.Sprintf("short %v, covers %v", short, covers), "total of the seen prices against the money held: " + tc.want, "capture trainers (seen offers); WoW Companion money check", resultIf(ok, "the money check")})
			}
		}
	}
	finish(t, "next-sweep-training-money", "Offline level milestone sweep: does the money cover the training", out)
}

// TestOfflineBenchmarkSweepProfessionRanks: the rank a character of each level may train to, and the rank after it, beside the game's gates.
func TestOfflineBenchmarkSweepProfessionRanks(t *testing.T) {
	n := professions.LoadNotes(tsStore())
	var out []benchRow
	for _, level := range []int{1, 5, 9, 10, 19, 20, 34, 35, 50, 60} {
		cap, tier := n.CapAt(level)
		want := 75
		switch {
		case level >= 35:
			want = 300
		case level >= 20:
			want = 225
		case level >= 10:
			want = 150
		}
		out = append(out, benchRow{fmt.Sprintf("character level %d: the highest profession rank to train", level), fmt.Sprintf("%s, skill cap %d", tier.Name, cap), fmt.Sprintf("skill cap %d", want), nmRankSrc, resultIf(cap == want, "a rank the level cannot train, or one it can but is not offered")})
	}
	finish(t, "next-sweep-profession-ranks", "Offline level milestone sweep: the profession rank each level trains", out)
}

// nmNoneAsks are the questions the dataset cannot answer, each with the limit line that must answer them (its start), in a player's words.
var nmNoneAsks = []struct{ line, area string }{
	{"Level milestones:", "what should I have by level 40|what should I have done by level 30?|what changes at level 20?|what unlocks at level 40?|what happens at level 40|checklist for level 40|what's my level 40 checklist|level 40 milestones|anything new when I ding?|what do I need to have by level 60"},
	{"Gold targets:", "how much gold should I have at 40|how much gold do I need for level 40|how much gold for gear at 30|what should I buy at level 20|what should I spend my gold on|what should I spend gold on at 30|how much gold should I save for my mount|how much money should I have by level 30|is there a gold goal for level 40"},
	{"Gear checkpoints:", "what gear should I have at level 40|am I geared for level 40|is my gear ok for my level|what gear should I have for Scarlet Monastery|am I ready for Wailing Caverns|what armor should I be wearing at 30|am I well geared for Blackrock Depths"},
	{"Riding and mounts:", "when do I get my mount|when can I learn riding|what level do I get a mount|when should I buy my first mount|how much gold do I need for riding at 40|should I save gold for my mount|what level is riding"},
	{"Bag and bank size:", "should I buy bags|should I buy a bigger bag|what bags should I have at level 30|how many bag slots should I have by 40|what bag should I get at 20"},
}

// TestOfflineBenchmarkSweepMilestoneNoAnswer: each milestone, gold, gear, riding and bag checkpoint question, in several words, reaches the line
// that the dataset holds no answer, so no milestone, gold figure, gear list, riding level or bag size is stated from memory.
func TestOfflineBenchmarkSweepMilestoneNoAnswer(t *testing.T) {
	var out []benchRow
	for _, g := range nmNoneAsks {
		parts := strings.Split(g.area, "|")
		for _, q := range parts {
			f := ask.LimitFacts(q)
			out = append(out, benchRow{fmt.Sprintf("%q", q), "limit line present: " + fmt.Sprint(strings.Contains(f, g.line)), "the dataset has no row: say so (" + strings.TrimSuffix(g.line, ":") + ")", nmNoneSrc, resultIf(strings.Contains(f, g.line), "no line that the data holds no answer")})
		}
	}
	finish(t, "next-sweep-no-answer", "Offline checkpoint sweep: questions the data cannot answer say so", out)
}

// TestOfflineBenchmarkSweepNotCheckpoint: a question that names a level, gold or gear but asks for a fact the data holds draws none of the
// checkpoint lines.
func TestOfflineBenchmarkSweepNotCheckpoint(t *testing.T) {
	var out []benchRow
	for _, q := range []string{
		"what quests can I do at level 20?", "how much does this sword sell for?", "how much gold do I have?", "what is the required level of this mount", "is this sword an upgrade for me?",
		"where do I buy bags", "how much does the Deadmines recommend", "what do I get at 5 gold", "can I afford my next spells", "do I have enough gold to train",
	} {
		f := ask.LimitFacts(q)
		hit := ""
		for _, l := range []string{"Level milestones:", "Gold targets:", "Gear checkpoints:"} {
			if strings.Contains(f, l) {
				hit = l
			}
		}
		out = append(out, benchRow{fmt.Sprintf("%q", q), "checkpoint line: " + fmt.Sprint(hit != ""), "a question the data holds: no checkpoint line", nmNoneSrc, resultIf(hit == "", "a checkpoint line for "+hit)})
	}
	finish(t, "next-sweep-not-checkpoint", "Offline checkpoint sweep: questions the data holds draw no checkpoint line", out)
}

// TestOfflineBenchmarkSweepProfessionRankAsks: a rank or skill-cap question with no profession named reaches the rank table of the data.
func TestOfflineBenchmarkSweepProfessionRankAsks(t *testing.T) {
	var out []benchRow
	for _, q := range []string{
		"when can I train expert", "when do I learn journeyman", "what's the max skill for apprentice", "what skill cap do I have at level 20", "what profession skill should I have at level 30",
		"when do I train my professions again", "should I visit my profession trainer", "what is the artisan skill cap", "what level for artisan",
	} {
		c := ask.Classify(q, nil, nil)
		out = append(out, benchRow{fmt.Sprintf("%q", q), fmt.Sprintf("topic %q", c.Topic), "the rank table (skill range and character level per rank)", nmRankSrc, resultIf(c.Topic == ask.TopicProfession, "no rank facts")})
	}
	finish(t, "next-sweep-profession-rank-asks", "Offline level milestone sweep: profession rank questions in a player's words", out)
}

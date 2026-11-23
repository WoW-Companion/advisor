package digest

import (
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/WoW-Companion/advisor/internal/advice"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/questie"
)

func fp(f float64) *float64 { return &f }

// valanaar is a synthetic QuestieDB: Endaria Mistgaze only turns in Unwelcome Spirits unless extra is
// set, in which case she also gives Lantern in the Mist (level 12, so after the plan's three pick-ups).
func valanaar(extra bool) *questie.Index {
	at := func(x float64) []questie.Spawn { return []questie.Spawn{{Area: 1, X: x, Y: 50}} }
	ix := &questie.Index{
		DB:    "test",
		Areas: map[int]questie.Area{1: {Zone: "Valanaar"}},
		Npcs: map[int]questie.Entity{
			1: {Name: "Endaria Mistgaze", Spawns: at(50)},
			2: {Name: "Hollis Reed", Spawns: at(42)},
			3: {Name: "Marla Dunn", Spawns: at(44)},
			4: {Name: "Tobin Vale", Spawns: at(46)},
		},
		Quests: map[int]*questie.Quest{
			700: {ID: 700, Name: "Unwelcome Spirits", Level: 10, Starters: []questie.Ref{{ID: 2}}, Finishers: []questie.Ref{{ID: 1}}},
			710: {ID: 710, Name: "Wind Hollow Essence", Level: 10, Starters: []questie.Ref{{ID: 2}}},
			711: {ID: 711, Name: "Dusty Ledgers", Level: 10, Starters: []questie.Ref{{ID: 3}}},
			712: {ID: 712, Name: "A Lost Hound", Level: 10, Starters: []questie.Ref{{ID: 4}}},
		},
	}
	if extra {
		ix.Quests[720] = &questie.Quest{ID: 720, Name: "Lantern in the Mist", Level: 12, Starters: []questie.Ref{{ID: 1}}}
	}
	return ix
}

// scout is a level 10 warrior in Valanaar with Unwelcome Spirits ready to turn in.
func scout() character.Character {
	c := warrior(10)
	c.Location = &character.Location{Zone: "Valanaar", X: fp(40), Y: fp(50)}
	c.Quests = []character.Quest{{QuestID: 700, Title: "Unwelcome Spirits", Level: ip(10), Complete: bp(true)}}
	c.Talents = &character.Talents{Unspent: ip(1), Trees: []character.TalentTree{{Name: "Arms", Talents: []character.Talent{{Name: "Test Cleave", Group: "Arms", Rank: 0, MaxRank: ip(3)}}}}}
	c.BankCaptured = true
	return c
}

func scoutDigest(t *testing.T, extra bool) Digest {
	t.Helper()
	opt := synthOpt
	opt.Questie = valanaar(extra)
	opt.PrevLevel = 8
	return Build(scout(), warriorStore(t), opt)
}

func notesOf(d Digest) []Line {
	for _, s := range d.Sections {
		if s.ID == Notes {
			return s.Lines
		}
	}
	return nil
}

func giverOf(d Digest) (Line, bool) {
	for _, l := range notesOf(d) {
		if l.Kind == KindGiver {
			return l, true
		}
	}
	return Line{}, false
}

func TestGiverNoteOnlyForAnAvailableQuestNotInThePlan(t *testing.T) {
	// She only turns in an open quest: no note, whatever the capture says about having met her.
	if l, ok := giverOf(scoutDigest(t, false)); ok {
		t.Errorf("turn-in-only NPC got a giver note: %+v", l)
	}
	// With a quest to give that the plan does not carry, the note is plain and whole.
	l, ok := giverOf(scoutDigest(t, true))
	if !ok {
		t.Fatal("no giver note for an NPC with an available quest")
	}
	if want := "Endaria Mistgaze in Valanaar has a quest for you: Lantern in the Mist"; l.Text != want {
		t.Errorf("text = %q, want %q", l.Text, want)
	}
	if l.NPC != "Endaria Mistgaze" || l.Zone != "Valanaar" || l.Quest != 720 || l.Title != "Lantern in the Mist" || !l.HasXY || l.X != 50 || l.Y != 50 {
		t.Errorf("fields = %+v", l)
	}
	if len(l.Details) == 0 || !strings.Contains(l.Details[0], "Valanaar") {
		t.Errorf("details = %q", l.Details)
	}
	if l.Ref != "npc:1" || len(l.Asks) != 1 || l.Asks[0] != "What does Endaria Mistgaze have for me?" {
		t.Errorf("ask/ref = %q %q", l.Asks, l.Ref)
	}
}

func TestGiverWithTheirQuestInThePlanGivesNoNote(t *testing.T) {
	ix := valanaar(false)
	// Hollis Reed's pick-up is a plan step: no note for him, even with another quest on offer.
	ix.Quests[730] = &questie.Quest{ID: 730, Name: "Extra Work", Level: 13, Starters: []questie.Ref{{ID: 2}}}
	c := scout()
	steps := advice.WhatsNextWith(c, warriorStore(t), ix)
	planned, npcs := PlanQuests(steps), PlanNPCs(steps)
	if !planned[710] || !npcs["Hollis Reed"] {
		t.Fatalf("fixture: plan = %v %v", planned, npcs)
	}
	if g, ok := FindGiver(c, ix, planned, npcs); ok {
		t.Errorf("giver already in the plan got a note: %+v", g)
	}
	if _, ok := FindGiver(c, nil, planned, npcs); ok {
		t.Error("a note without any Questie data")
	}
}

var (
	idsOnly = regexp.MustCompile(`^\d+$`)
	whoAmI  = regexp.MustCompile(`I'm a level|\.\.\.`)
)

// entries lists every entry's text fields, asks and ref, across sections, dungeons and talents.
type entry struct {
	text, ref string
	asks      []string
	others    []string // the entry's other fields: details and prompt
}

func entriesOf(d Digest) (out []entry) {
	for _, s := range d.Sections {
		for _, l := range s.Lines {
			out = append(out, entry{l.Text, l.Ref, l.Asks, append([]string{l.Prompt}, l.Details...)})
		}
	}
	for _, l := range d.Dungeons {
		out = append(out, entry{l.Name + ": " + l.Line, l.Ref, l.Asks, append([]string{l.Prompt, l.Line}, l.Details...)})
	}
	for _, l := range d.Talents {
		out = append(out, entry{l.Line, l.Ref, l.Asks, append([]string{l.Prompt}, l.Details...)})
	}
	return out
}

func TestEveryEntryKindHasNaturalAsks(t *testing.T) {
	kinds := map[string]bool{}
	for name, d := range map[string]Digest{
		"scout":  scoutDigest(t, true),
		"synth":  Build(synthChar(), synthStore(t), synthOpt),
		"rogue":  Build(synthRogue(), synthStore(t), synthOpt),
		"nodata": Build(warrior(10), nil, synthOpt),
	} {
		for _, e := range entriesOf(d) {
			id := name + "/" + e.text
			if len(e.asks) < 1 || len(e.asks) > MaxAsks {
				t.Errorf("%s: %d asks", id, len(e.asks))
			}
			if e.ref == "" {
				t.Errorf("%s: no ref", id)
			}
			kinds[strings.SplitN(e.ref, ":", 2)[0]] = true
			for _, a := range e.asks {
				switch {
				case utf8.RuneCountInString(a) > MaxAskLength:
					t.Errorf("%s: ask %q is longer than %d", id, a, MaxAskLength)
				case whoAmI.MatchString(a):
					t.Errorf("%s: ask %q has who-am-I text or a cut", id, a)
				case idsOnly.MatchString(a) || regexp.MustCompile(`\b\d{4,}\b`).MatchString(a):
					t.Errorf("%s: ask %q holds a raw id", id, a)
				case !strings.HasSuffix(a, "?"):
					t.Errorf("%s: ask %q is not a whole question", id, a)
				case a == e.text:
					t.Errorf("%s: ask is the line text", id)
				}
				for _, o := range e.others {
					if o != "" && a == o {
						t.Errorf("%s: ask %q repeats another field verbatim", id, a)
					}
				}
			}
			for _, o := range e.others {
				if o != "" && o == e.text {
					t.Errorf("%s: another field repeats the text", id)
				}
			}
		}
	}
	for _, want := range []string{"quest", "npc", "talents", "training", "item", "dungeon", "note"} {
		if !kinds[want] {
			t.Errorf("no entry with a %s ref", want)
		}
	}
}

func TestNoDigestTextIsCut(t *testing.T) {
	long := scout()
	long.Quests = []character.Quest{{QuestID: 701, Title: "A Very Long Quest Title That Keeps Going On And On Past Any Sensible Panel Width", Level: ip(10),
		Objectives: []character.Objective{{Text: "Gather the Whispering Hollow Essence from the ancient trees", Have: ip(1), Need: ip(9)}}}}
	opt := synthOpt
	opt.Questie = valanaar(true)
	d := Build(long, warriorStore(t), opt)
	for _, s := range d.Sections {
		for _, l := range s.Lines {
			if utf8.RuneCountInString(l.Text) > MaxText {
				t.Errorf("text too long: %q", l.Text)
			}
			if i := strings.Index(l.Text, "..."); i >= 0 && (i != len(l.Text)-3 || l.Text[i-1] == ' ') {
				t.Errorf("text cut oddly: %q", l.Text)
			}
			if s.ID == Notes && strings.HasSuffix(l.Text, "...") {
				t.Errorf("a note was cut: %q", l.Text)
			}
			for _, x := range append(l.Details, l.Prompt) {
				if strings.Contains(x, "...") {
					t.Errorf("cut text in %q", x)
				}
			}
		}
	}
}

func TestLuaCarriesAsksAndRef(t *testing.T) {
	out := one(scoutDigest(t, true)).Lua()
	for _, want := range []string{`schema = "1.6"`, `asks = { "What does turning in Unwelcome Spirits unlock?"`, `ref = "quest:700"`,
		`kind = "giver"`, `ref = "npc:1"`, `asks = { "Where should my talent point go?"`, `ref = "talents"`, `ref = "training:10"`, `asks = { "What should I train first?"`} {
		if !strings.Contains(out, want) {
			t.Errorf("lacks %s", want)
		}
	}
	if err := SlotCheck(out); err != nil {
		t.Error(err)
	}
}

func TestSubjectAndExactAskMatch(t *testing.T) {
	d := scoutDigest(t, true)
	if got := d.RefFor("what does Endaria Mistgaze have for me"); got != "npc:1" {
		t.Errorf("ref = %q", got)
	}
	if got := d.RefFor("  Where should my TALENT point go?!  "); got != "talents" {
		t.Errorf("ref = %q", got)
	}
	if got := d.RefFor("What does Endaria Mistgaze have for me and her sister?"); got != "" {
		t.Errorf("a different question got %q", got)
	}
	sub, ok := d.Subject("quest:700")
	if !ok || sub.Kind != KindTurnIn || sub.Place == 0 || len(sub.Details) == 0 {
		t.Errorf("subject = %+v, %v", sub, ok)
	}
	if _, ok := d.Subject("quest:99999"); ok {
		t.Error("an unknown ref found a subject")
	}
}

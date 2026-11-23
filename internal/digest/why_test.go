package digest

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/WoW-Companion/advisor/internal/character"
)

func TestWhyByKind(t *testing.T) {
	c := warrior(10)
	noStyle := character.Character{Name: "Plain", Class: "WARRIOR", Level: ip(10)}
	for _, tc := range []struct {
		name string
		got  string
		want string
	}{
		{"class quest, reward unknown", questWhy(c, Line{Kind: KindPickup, Class: true, Level: 10}), classQuestWhy},
		{"turn-in with unlock", questWhy(c, Line{Kind: KindTurnIn, Unlocks: []string{"Unnerving Silence (from Seena, here)"}}), "Unlocks Unnerving Silence"},
		{"turn-in, nothing known", questWhy(c, Line{Kind: KindTurnIn}), "Done — hand it in on your way"},
		{"in progress with count", questWhy(c, Line{Kind: KindProgress, Objectives: []Objective{{Text: "Wind Hollow Essence", Have: 2, Need: 8, HasCount: true}}}), "Still to do: Wind Hollow Essence (2/8)"},
		{"in progress, nothing open", questWhy(c, Line{Kind: KindProgress, Objectives: []Objective{{Text: "Boar", Have: 1, Need: 1, HasCount: true}}}), ""},
		{"in progress, no objectives", questWhy(c, Line{Kind: KindProgress}), ""},
		{"giver level fit", questWhy(c, Line{Kind: KindGiver, Level: 11}), "A level above you"},
		{"giver, level unknown", questWhy(c, Line{Kind: KindGiver}), ""},
		{"talent with pick", talentWhy(c, "Penance"), "Your plan's next pick: Penance"},
		{"talent, no style, no pick", talentWhy(noStyle, ""), "Choose a play style to get a pick"},
		{"talent, style, no pick", talentWhy(c, ""), ""},
		{"training", trainingWhy([]TrainSpell{{Name: "Rend", New: false}, {Name: "Taunt", New: true}}), "New: Taunt"},
		{"training, nothing new", trainingWhy([]TrainSpell{{Name: "Rend"}}), ""},
		{"too long", whyOf(strings.Repeat("word ", 20)), ""},
		{"raw id", whyOf("Unlocks quest 12345"), ""},
		{"ellipsis", whyOf("Still to do..."), ""},
	} {
		if tc.got != tc.want {
			t.Errorf("%s: why = %q, want %q", tc.name, tc.got, tc.want)
		}
	}
	// An unlock whose name is too long to fit whole falls back, never cut.
	long := questWhy(c, Line{Kind: KindTurnIn, Unlocks: []string{strings.Repeat("Longname ", 10)}})
	if long != "Unlocks a follow-up quest" {
		t.Errorf("long unlock: %q", long)
	}
	if strings.Contains(strings.ToLower(classQuestWhy), "reward") {
		t.Error("the class-quest why claims a reward")
	}
}

// Every why a build produces is short, whole, free of ids, and exists only for an entry with a fact.
func TestWhyInBuiltDigests(t *testing.T) {
	opt := synthOpt
	opt.PrevLevel = 8
	opt.Unlocks = map[int][]string{100: {"The Next Hunt (from Elder Test, here)"}}
	var kinds = map[string]bool{}
	check := func(what, why string) {
		if why == "" {
			return
		}
		kinds[what] = true
		if utf8.RuneCountInString(why) > MaxWhy || strings.Contains(why, "...") || rawID.MatchString(why) {
			t.Errorf("%s: why %q", what, why)
		}
	}
	for _, d := range []Digest{
		Build(synthChar(), synthStore(t), opt),
		Build(synthRogue(), synthStore(t), opt),
		Build(warrior(10), warriorStore(t), opt),
		Build(character.Character{Key: "Realm:Bare"}, nil, opt),
	} {
		for _, s := range d.Sections {
			for _, l := range s.Lines {
				check(s.ID+"/"+l.Kind, l.Why)
				if l.Kind == KindNote && l.Why != "" {
					t.Errorf("a note carries why %q", l.Why)
				}
				t.Logf("%s %-9s %q -> %q", s.ID, l.Kind, l.Text, l.Why)
			}
		}
		for _, l := range d.Dungeons {
			check("dungeons", l.Why)
		}
		for _, l := range d.Talents {
			check("talents", l.Why)
		}
	}
	for _, k := range []string{"next_quests/talent", "next_quests/turnin", "next_quests/training", "upgrades/", "dungeons", "talents"} {
		if !kinds[k] {
			t.Errorf("no why checked for %s; got %v", k, kinds)
		}
	}
}

func TestWhyInLua(t *testing.T) {
	out := one(Build(warrior(10), warriorStore(t), synthOpt)).Lua()
	if !strings.Contains(out, `why = "New: Taunt"`) {
		t.Errorf("Lua lacks why:\n%s", out)
	}
	if strings.Contains(one(Build(character.Character{Key: "Realm:Bare"}, nil, synthOpt)).Lua(), "why =") {
		t.Error("an entry without facts carries why")
	}
}

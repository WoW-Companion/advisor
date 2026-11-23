package digest

import (
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/WoW-Companion/advisor/internal/advice"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/dungeons"
	"github.com/WoW-Companion/advisor/internal/gear"
	"github.com/WoW-Companion/advisor/internal/wording"
)

type rendered struct {
	kind, text string
	details    []string
	prompt     string
}

func unstyled(level int) character.Character {
	c := warrior(level)
	c.Profile = nil
	return c
}

// renderAll renders every kind of prompt and details for c, from synthetic input only.
func renderAll(t *testing.T, c character.Character, points int) []rendered {
	t.Helper()
	var out []rendered
	add := func(kind, text string, d []string, p string) { out = append(out, rendered{kind, text, d, p}) }

	step := advice.Step{Title: "You have unspent talent points", Points: points}
	d, p := talentStepContext(c, step, "Whirlwind (rank 1/2) in Fury", "Whirlwind")
	add("talent points", step.Title, d, p)
	d, p = talentStepContext(c, step, "", "")
	add("talent points, no plan", step.Title, d, p)
	d, p = talentContext(c, "Whirlwind", 1, "Fury")
	add("talent pick", "Whirlwind rank 1", d, p)

	q := Line{Title: "Unnerving Silence", Text: "Pick up: Unnerving Silence (Dunemark)", NPC: "Elder Test", Zone: "Dunemark", Level: 10, Class: true}
	q.Kind = KindPickup
	d, p = questContext(c, q, "internal reason")
	add("pick up", q.Text, d, p)
	q.Kind, q.Text = KindProgress, "Boars: 3/8 Boar Tusk (Dunemark)"
	q.Objectives = []Objective{{Text: "Boar Tusk", Have: 3, Need: 8, HasCount: true}}
	d, p = questContext(c, q, "internal reason")
	add("in progress", q.Text, d, p)
	q.Kind, q.Text = KindTurnIn, "Turn in Unnerving Silence (Dunemark)"
	q.Unlocks = []string{"Rising Dread (from Elder Test, here)"}
	d, p = questContext(c, q, "ready to turn in")
	add("turn in", q.Text, d, p)

	tl, ok := trainingOf(Build(c, warriorStore(t), synthOpt))
	if !ok {
		t.Fatal("no training entry")
	}
	add("training", tl.Text, tl.Details, tl.Prompt)

	cur := 8
	c.Equipment = map[string]character.EquippedItem{"waist": {Name: "Old Belt", ItemLevel: &cur}}
	for _, src := range []gear.Source{
		{Kind: "creature", ID: 1252262, Name: "Dire Boar"},
		{Kind: "crafted", ID: 1252262},
		{Kind: "crafted", ID: 7, Name: "Blacksmithing"},
		{Kind: "quest reward", ID: 5, Name: "Unnerving Silence"},
	} {
		cd := gear.Candidate{Item: gear.Item{ID: 9, Name: "Veteran's Chain Belt", ItemLevel: 12, ReqLevel: 12}, Soon: true, Sources: []gear.Source{src}}
		d, p = upgradeContext(c, "waist", cd)
		add("upgrade/"+src.Kind+src.Name, "waist: Veteran's Chain Belt (ilvl 12)", d, p)
	}

	lo, hi := 10, 14
	dp := dungeons.DungeonPick{Name: "Wolf Den", MinLevel: &lo, MaxLevel: &hi, Quests: []dungeons.Quest{{ID: 1, Title: "x"}}}
	d, p = dungeonContext(c, dp, dungeons.Drop{Boss: "Near Wolf", Item: "Cloth Hood", Slot: "head", ItemID: 1252262})
	add("dungeon", "lvl 10-14: Cloth Hood", d, p)
	return out
}

var idRe = regexp.MustCompile(`\b\d{4,}\b`)

func TestPromptsAndDetailsArePlayerFacing(t *testing.T) {
	for _, tc := range []struct {
		name string
		c    character.Character
	}{{"styled", warrior(10)}, {"unstyled", unstyled(10)}} {
		for _, points := range []int{1, 2} {
			for _, r := range renderAll(t, tc.c, points) {
				id := tc.name + "/" + r.kind
				if n := utf8.RuneCountInString(r.prompt); n == 0 || n > MaxPrompt {
					t.Errorf("%s: prompt is %d characters", id, n)
				}
				if len(r.details) < 1 || len(r.details) > MaxDetails {
					t.Errorf("%s: %d details", id, len(r.details))
				}
				all := append([]string{r.prompt}, r.details...)
				for _, s := range all {
					if idRe.MatchString(s) {
						t.Errorf("%s: raw id in %q", id, s)
					}
					for _, bad := range []string{"milestone", "map units", "band", "prerequisites", "talent points.", " points. You have"} {
						if strings.Contains(s, bad) && !strings.Contains(s, "unspent talent points") {
							t.Errorf("%s: internal wording %q in %q", id, bad, s)
						}
					}
				}
				for _, s := range r.details {
					if strings.Contains(s, "...") {
						t.Errorf("%s: ellipsis in %q", id, s)
					}
					if strings.Contains(r.prompt, s) {
						t.Errorf("%s: prompt holds detail %q verbatim", id, s)
					}
				}
				if strings.Contains(r.prompt, r.text) {
					t.Errorf("%s: prompt holds the line text %q verbatim", id, r.text)
				}
				style := strings.Contains(r.prompt, "playing Arms, questing")
				if (tc.name == "styled") != style {
					t.Errorf("%s: play style in prompt = %v: %s", id, style, r.prompt)
				}
			}
		}
	}
}

func TestTalentPointsPlural(t *testing.T) {
	for points, want := range map[int]string{1: "with 1 unspent talent point.", 2: "with 2 unspent talent points."} {
		r := renderAll(t, warrior(10), points)[0]
		if !strings.Contains(r.prompt, want) || strings.Contains(r.prompt, "1 unspent talent points") {
			t.Errorf("%d: %s", points, r.prompt)
		}
		if !strings.HasPrefix(r.details[0], wording.Count(points, "talent point", "talent points")) {
			t.Errorf("%d: details %q", points, r.details)
		}
	}
	if got, want := renderAll(t, warrior(10), 1)[0].prompt, "I'm a level 10 Skyborn warrior playing Arms, questing, with 1 unspent talent point. My plan's next pick is Whirlwind. Should I take it now, and why?"; got != want {
		t.Errorf("talent prompt = %q", got)
	}
}

func TestGoldenPromptsPerKind(t *testing.T) {
	got := map[string]string{}
	for _, r := range renderAll(t, warrior(10), 2) {
		got[r.kind] = r.prompt
	}
	for kind, want := range map[string]string{
		"talent points, no plan":       `I'm a level 10 Skyborn warrior playing Arms, questing, with 2 unspent talent points. What should I spend them on, and why?`,
		"talent pick":                  `I'm a level 10 Skyborn warrior playing Arms, questing. My next talent pick is Whirlwind, rank 1. Why this pick now, for the way I play?`,
		"pick up":                      `I'm a level 10 Skyborn warrior playing Arms, questing. I can pick up the quest "Unnerving Silence" in Dunemark. It is a class quest. Is it worth taking now, and what should I know before I accept it?`,
		"turn in":                      `I'm a level 10 Skyborn warrior playing Arms, questing. I can turn in the quest "Unnerving Silence" in Dunemark. It is a class quest. Turning it in unlocks "Rising Dread". What does turning it in unlock, and what should I do next?`,
		"upgrade/creatureDire Boar":    `I'm a level 10 Skyborn warrior playing Arms, questing. A possible upgrade is Veteran's Chain Belt for my waist slot (item level 12), replacing Old Belt (item level 8). It is a drop from Dire Boar and usable at level 12. Is it worth getting for how I play, and how do I get it?`,
		"upgrade/craftedBlacksmithing": `I'm a level 10 Skyborn warrior playing Arms, questing. A possible upgrade is Veteran's Chain Belt for my waist slot (item level 12), replacing Old Belt (item level 8). It is crafted by Blacksmithing and usable at level 12. Is it worth getting for how I play, and how do I get it?`,
		"dungeon":                      `I'm a level 10 Skyborn warrior playing Arms, questing. I'm considering the dungeon Wolf Den (levels 10 to 14). Its best upgrade for me is Cloth Hood, from Near Wolf. Is it worth running now, and how should I prepare?`,
	} {
		if got[kind] != want {
			t.Errorf("%s:\n got %s\nwant %s", kind, got[kind], want)
		}
	}
	if !strings.Contains(got["training"], "Which should I train first, and how does it fit my rotation?") {
		t.Errorf("training: %s", got["training"])
	}
	if strings.Contains(got["upgrade/crafted"], "crafted") {
		t.Errorf("unnamed crafted source shown: %s", got["upgrade/crafted"])
	}
}

func TestWordingHelpers(t *testing.T) {
	for _, tc := range []struct{ got, want string }{
		{wording.Count(1, "talent point", "talent points"), "1 talent point"},
		{wording.Count(2, "talent point", "talent points"), "2 talent points"},
		{wording.Fit(10, 10), "Right for your level"},
		{wording.Fit(11, 10), "A level above you"},
		{wording.Fit(7, 10), "3 levels below you"},
		{wording.Fit(0, 10), ""},
		{wording.Source("creature", "Dire Boar"), "a drop from Dire Boar"},
		{wording.Source("crafted", ""), ""},
		{wording.Source("vendor", "Grim Trader"), "sold by Grim Trader"},
		{wording.CutWords("one two three four", 10), "one two"},
		{wording.Slot("finger1"), "ring"},
	} {
		if tc.got != tc.want {
			t.Errorf("got %q, want %q", tc.got, tc.want)
		}
	}
}

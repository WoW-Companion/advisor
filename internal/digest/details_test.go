package digest

import (
	"regexp"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/advice"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/dungeons"
	"github.com/WoW-Companion/advisor/internal/gear"
	"github.com/WoW-Companion/advisor/internal/wording"
)

type rendered struct {
	kind, text string
	details    []string
}

func unstyled(level int) character.Character {
	c := warrior(level)
	c.Profile = nil
	return c
}

// renderAll renders every kind of details for c, from synthetic input only.
func renderAll(t *testing.T, c character.Character, points int) []rendered {
	t.Helper()
	var out []rendered
	add := func(kind, text string, d []string) { out = append(out, rendered{kind, text, d}) }

	step := advice.Step{Title: "You have unspent talent points", Points: points}
	d := talentStepContext(c, step, "Whirlwind (rank 1/2) in Fury", "Whirlwind")
	add("talent points", step.Title, d)
	d = talentStepContext(c, step, "", "")
	add("talent points, no plan", step.Title, d)
	d = talentContext(c, "Whirlwind", 1, "Fury")
	add("talent pick", "Whirlwind rank 1", d)

	q := Line{Title: "Unnerving Silence", Text: "Pick up: Unnerving Silence (Dunemark)", NPC: "Elder Test", Zone: "Dunemark", Level: 10, Class: true}
	q.Kind = KindPickup
	d = questContext(c, q, "internal reason")
	add("pick up", q.Text, d)
	q.Kind, q.Text = KindProgress, "Boars: 3/8 Boar Tusk (Dunemark)"
	q.Objectives = []Objective{{Text: "Boar Tusk", Have: 3, Need: 8, HasCount: true}}
	d = questContext(c, q, "internal reason")
	add("in progress", q.Text, d)
	q.Kind, q.Text = KindTurnIn, "Turn in Unnerving Silence (Dunemark)"
	q.Unlocks = []string{"Rising Dread (from Elder Test, here)"}
	d = questContext(c, q, "ready to turn in")
	add("turn in", q.Text, d)

	tl, ok := trainingOf(Build(c, warriorStore(t), synthOpt))
	if !ok {
		t.Fatal("no training entry")
	}
	add("training", tl.Text, tl.Details)

	cur := 8
	c.Equipment = map[string]character.EquippedItem{"waist": {Name: "Old Belt", ItemLevel: &cur}}
	for _, src := range []gear.Source{
		{Kind: "creature", ID: 1252262, Name: "Dire Boar"},
		{Kind: "crafted", ID: 1252262},
		{Kind: "crafted", ID: 7, Name: "Blacksmithing"},
		{Kind: "quest reward", ID: 5, Name: "Unnerving Silence"},
	} {
		cd := gear.Candidate{Item: gear.Item{ID: 9, Name: "Veteran's Chain Belt", ItemLevel: 12, ReqLevel: 12}, Soon: true, Sources: []gear.Source{src}}
		d = upgradeContext(c, "waist", cd)
		add("upgrade/"+src.Kind+src.Name, "waist: Veteran's Chain Belt (ilvl 12)", d)
	}

	lo, hi := 10, 14
	dp := dungeons.DungeonPick{Name: "Wolf Den", MinLevel: &lo, MaxLevel: &hi, Quests: []dungeons.Quest{{ID: 1, Title: "x"}}}
	d = dungeonContext(c, dp, dungeons.Drop{Boss: "Near Wolf", Item: "Cloth Hood", Slot: "head", ItemID: 1252262})
	add("dungeon", "lvl 10-14: Cloth Hood", d)
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
				if len(r.details) < 1 || len(r.details) > MaxDetails {
					t.Errorf("%s: %d details", id, len(r.details))
				}
				all := r.details
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
				}
			}
		}
	}
}

func TestTalentPointsPlural(t *testing.T) {
	for points := range []int{1, 2} {
		points++
		r := renderAll(t, warrior(10), points)[0]
		if !strings.HasPrefix(r.details[0], wording.Count(points, "talent point", "talent points")) {
			t.Errorf("%d: details %q", points, r.details)
		}
	}
}

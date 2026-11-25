package advice

import (
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/questie"
)

// An invented QuestieDB holding quest 5658 (priests, Undead only, level 10) given by Aelthalyste.
func spellQuestIndex() *questie.Index {
	st := []questie.Ref{{ID: 100}}
	q := &questie.Quest{ID: 5658, Name: "Touch of Weakness", Level: 10, MinLevel: 10, Classes: 1 << 4, Races: 1 << 4, Starters: st, Finishers: st}
	return &questie.Index{DB: "v0", Quests: map[int]*questie.Quest{5658: q},
		Npcs:  map[int]questie.Entity{100: {Name: "Aelthalyste", Spawns: []questie.Spawn{{Area: 902, X: 50.5, Y: 60.5}}}},
		Areas: map[int]questie.Area{902: {UIMap: 7001, Zone: "Undercity"}}}
}

func spellQuestChar(level int, race string) character.Character {
	id, x, y := 7001, 40.0, 60.0
	return character.Character{Level: &level, Class: "PRIEST", Race: race, KnownSpells: map[int64]bool{2944: true},
		Location: &character.Location{Zone: "Undercity", MapID: &id, X: &x, Y: &y}}
}

func spellQuestSteps(c character.Character) []Step {
	var out []Step
	for _, s := range WhatsNextWith(c, nil, spellQuestIndex()) {
		if IsSpellQuest(s) {
			out = append(out, s)
		}
	}
	return out
}

func TestMissedSpellQuestStep(t *testing.T) {
	done := spellQuestChar(24, "Undead")
	done.CompletedQuests = []int{5658}
	known := spellQuestChar(24, "Undead")
	known.KnownSpells[2652] = true
	cases := []struct {
		name string
		c    character.Character
		want bool
	}{
		{"undead priest missed it", spellQuestChar(24, "Undead"), true},
		{"quest completed", done, false},
		{"spell known", known, false},
		{"other race", spellQuestChar(24, "Human"), false},
		{"level 9", spellQuestChar(9, "Undead"), false},
	}
	for _, tc := range cases {
		got := spellQuestSteps(tc.c)
		if (len(got) == 1) != tc.want || len(got) > 1 {
			t.Errorf("%s: steps = %+v", tc.name, got)
		}
		if tc.want && len(got) == 1 {
			const title = "Pick up Touch of Weakness from Aelthalyste (Undercity): teaches Touch of Weakness, needed before Rank 2"
			if got[0].Title != title || !strings.Contains(got[0].Location, "Aelthalyste") {
				t.Errorf("%s: step = %+v", tc.name, got[0])
			}
		}
	}
}

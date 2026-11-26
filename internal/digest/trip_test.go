package digest

import (
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/questie"
)

// tripWorld has a ready quest handed in inside a dungeon (900), one delivered to a town NPC (901) and
// a kill quest finished where it was given (902), as in a level 25 character's log.
func tripWorld() *questie.Index {
	sp := func(area int) []questie.Spawn { return []questie.Spawn{{Area: area, X: 30, Y: 40}} }
	return &questie.Index{
		Npcs: map[int]questie.Entity{
			1: {Name: "Hadrec", Spawns: sp(1)}, 2: {Name: "Deathstalker Vincent", Spawns: sp(2)},
			3: {Name: "Sputtervalve", Spawns: sp(3)}, 4: {Name: "Wolf", Spawns: sp(1)},
		},
		Areas: map[int]questie.Area{
			1: {UIMap: 1, Zone: "Undercity"}, 5: {UIMap: 5, Zone: "Silverpine Forest"},
			2: {UIMap: 2, Sub: "Shadowfang Keep", Zone: "Silverpine Forest"}, 3: {UIMap: 3, Sub: "Ratchet", Zone: "The Barrens"},
		},
		Dungeons: []questie.Dungeon{{Area: 2, Parent: 5, Name: "Shadowfang Keep"}},
		Quests: map[int]*questie.Quest{
			900: {ID: 900, Name: "Deathstalkers in Shadowfang", Starters: []questie.Ref{{ID: 1}}, Finishers: []questie.Ref{{ID: 2}}},
			901: {ID: 901, Name: "Further Instructions", Starters: []questie.Ref{{ID: 1}}, Finishers: []questie.Ref{{ID: 3}}},
			902: {ID: 902, Name: "Wolf Trouble", Starters: []questie.Ref{{ID: 1}}, Finishers: []questie.Ref{{ID: 1}}, Creatures: []questie.Ref{{ID: 4}}},
		},
	}
}

func TestTripLinesAreNotQuickTurnIns(t *testing.T) {
	c := warrior(25)
	c.Location = &character.Location{Zone: "Undercity", X: fp(40), Y: fp(50)}
	c.Quests = []character.Quest{
		{QuestID: 900, Title: "Deathstalkers in Shadowfang", Level: ip(25), Complete: bp(true)},
		{QuestID: 901, Title: "Further Instructions", Level: ip(25), Complete: bp(true)},
		{QuestID: 902, Title: "Wolf Trouble", Level: ip(25), Complete: bp(true)},
	}
	opt := synthOpt
	opt.Questie = tripWorld()
	d := Build(c, warriorStore(t), opt)
	var lines []Line
	for _, s := range d.Sections {
		if s.ID == NextQuests {
			lines = s.Lines
		}
	}
	by := map[int]Line{}
	for _, l := range lines {
		by[l.Quest] = l
	}
	if w := by[902].Why; w != "Done — hand it in on your way" || by[902].Trip {
		t.Errorf("a kill quest keeps the plain turn-in: %+v", by[902])
	}
	v, s := by[900], by[901]
	if !v.Trip || v.Why != "Find Deathstalker Vincent inside Shadowfang Keep (Silverpine Forest)" || v.Instance != "Shadowfang Keep" {
		t.Errorf("dungeon trip: %+v", v)
	}
	if !strings.Contains(v.Text, "(dungeon)") {
		t.Errorf("dungeon trip text: %q", v.Text)
	}
	if !s.Trip || s.Why != "Deliver to Sputtervalve in Ratchet (The Barrens)" || s.Instance != "" {
		t.Errorf("delivery: %+v", s)
	}
	for _, l := range []Line{v, s} {
		if strings.Contains(l.Why+l.Text+strings.Join(l.Details, " "), "on your way") {
			t.Errorf("trip called a stop on the way: %+v", l)
		}
	}
	// The dungeon trip follows the ordinary turn-ins, never a quick one nearby.
	pos := map[int]int{}
	for i, l := range lines {
		pos[l.Quest] = i
	}
	if !(pos[902] < pos[900]) || !(pos[901] < pos[900]) {
		t.Errorf("the dungeon trip should come after the other turn-ins: %v", pos)
	}
}

func TestTripWhyFitsAndNeverCuts(t *testing.T) {
	for _, l := range []Line{
		{Kind: KindTurnIn, Trip: true, Finisher: "Deathstalker Vincent", FinisherPlace: "Shadowfang Keep (Silverpine Forest)", Instance: "Shadowfang Keep", InstanceZone: "Silverpine Forest"},
		{Kind: KindTurnIn, Trip: true, Finisher: "A Very Long Named Finisher Of Quests", FinisherPlace: "A Rather Long Subzone Name (The Barrens)"},
	} {
		w := questWhy(warrior(25), l)
		if w == "" || strings.Contains(w, "on your way") || strings.Contains(w, "...") {
			t.Errorf("%+v: why %q", l, w)
		}
	}
}

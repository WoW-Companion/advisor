package route

import (
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/questie"
)

// townIndex is a town (map 7001) whose givers offer: a dungeon quest (id 1013, level 26, in Keep), a
// festival quest (id 2000, level 24), a plain quest (id 3000, level 24) and a quest of a far dungeon
// (id 4000, level 26, its dungeon 40-50).
func townIndex() *questie.Index {
	sp := func(x, y float64) []questie.Spawn { return []questie.Spawn{{Area: 901, X: x, Y: y}} }
	ix := &questie.Index{
		Quests: map[int]*questie.Quest{},
		Npcs: map[int]questie.Entity{
			1: {Name: "Keeper", Spawns: sp(50, 50)},
			2: {Name: "Festival Host", Spawns: sp(20, 20)},
			3: {Name: "Clerk", Spawns: sp(70, 70)},
			4: {Name: "Archivist", Spawns: sp(80, 30)},
		},
		Objects: map[int]questie.Entity{},
		Areas:   map[int]questie.Area{901: {UIMap: 7001, Zone: "Town"}},
		Dungeons: []questie.Dungeon{
			{Area: 209, Name: "Shadowfang Keep", Instance: 33, Quests: []int{1013}},
			{Area: 300, Name: "Far Hold", Instance: 44, Quests: []int{4000}},
		},
	}
	add := func(id, lvl, zone, giver int, name string) {
		ix.Quests[id] = &questie.Quest{ID: id, Name: name, Level: lvl, Zone: zone, Starters: []questie.Ref{{ID: giver}}, Finishers: []questie.Ref{{ID: giver}}}
	}
	add(1013, 26, 209, 1, "The Book of Ur")
	add(2000, 24, -366, 2, "Festival Fires")
	add(3000, 24, 901, 3, "Clerk Work")
	add(4000, 26, 300, 4, "Far Business")
	return ix
}

func townOpt() Options {
	return Options{
		Dungeons:  []DungeonLevel{{Map: 33, Name: "Shadowfang Keep", Min: 22, Max: 30}, {Map: 44, Name: "Far Hold", Min: 40, Max: 50}},
		XP:        map[int]int{1013: 2100},
		AnyPickup: true, PickupCap: 99,
	}
}

func townHero(logged int) character.Character {
	lvl, mp := 24, 7001
	x, y := 48.0, 49.0
	c := character.Character{Name: "Hero", QuestsCaptured: true, Level: &lvl, Location: &character.Location{Zone: "Town", MapID: &mp, X: &x, Y: &y}}
	for i := range logged {
		c.Quests = append(c.Quests, character.Quest{QuestID: 90000 + i})
	}
	return c
}

func labels(ss []Stop) string {
	var l []string
	for _, s := range ss {
		l = append(l, s.Label)
	}
	return strings.Join(l, " | ")
}

func TestTownDungeonPickupFirstAndFestivalLater(t *testing.T) {
	p := BuildWith(townHero(5), townIndex(), townOpt())
	if len(p.Steps) < 2 || p.Steps[0].Label != "Pick up The Book of Ur (Shadowfang Keep, 2,100 XP) from Keeper" {
		t.Fatalf("steps: %s", labels(p.Steps))
	}
	if !strings.Contains(labels(p.Steps), "Clerk Work") {
		t.Errorf("the in-level quest should still be a step: %s", labels(p.Steps))
	}
	if strings.Contains(labels(p.Steps), "Festival Fires") || !strings.Contains(labels(p.Later), "Festival Fires") {
		t.Errorf("festival quest should wait: steps %s, later %s", labels(p.Steps), labels(p.Later))
	}
}

func TestTownFarDungeonStaysLater(t *testing.T) {
	p := BuildWith(townHero(5), townIndex(), townOpt())
	// Its dungeon is far above the player: the quest is no town-dungeon pick-up (no dungeon tag).
	if strings.Contains(labels(p.Steps), "Far Hold") || strings.Contains(labels(p.Later), "Far Hold") {
		t.Errorf("steps %s, later %s", labels(p.Steps), labels(p.Later))
	}
}

func TestTownFestivalAloneIsPlanned(t *testing.T) {
	ix := townIndex()
	for _, id := range []int{1013, 3000, 4000} {
		delete(ix.Quests, id)
	}
	p := BuildWith(townHero(5), ix, townOpt())
	if !strings.Contains(labels(p.Steps), "Pick up Festival Fires") {
		t.Errorf("steps %s", labels(p.Steps))
	}
}

func TestTownNearlyFullLogLimitsPickups(t *testing.T) {
	p := BuildWith(townHero(39), townIndex(), townOpt())
	var picks []string
	for _, s := range p.Steps {
		if s.Kind == KindPickup {
			picks = append(picks, s.Label)
		}
	}
	if len(picks) != 1 || !strings.Contains(picks[0], "The Book of Ur") {
		t.Errorf("one pick-up, the dungeon one: %v", picks)
	}
	if !strings.Contains(strings.Join(p.Notes, " "), "nearly full") || !strings.Contains(labels(p.Later), "Clerk Work") {
		t.Errorf("notes %v later %s", p.Notes, labels(p.Later))
	}
}

func TestTownFullLogPlansNoPickups(t *testing.T) {
	p := BuildWith(townHero(40), townIndex(), townOpt())
	for _, s := range p.Steps {
		if s.Kind == KindPickup {
			t.Errorf("pick-up %q with a full log", s.Label)
		}
	}
}

func TestTownLogCapOption(t *testing.T) {
	opt := townOpt()
	opt.LogCap = 25
	p := BuildWith(townHero(25), townIndex(), opt)
	for _, s := range p.Steps {
		if s.Kind == KindPickup {
			t.Errorf("pick-up %q with a full 25-quest log", s.Label)
		}
	}
}

func TestTownRoomyLogNoNote(t *testing.T) {
	p := BuildWith(townHero(10), townIndex(), townOpt())
	if strings.Contains(strings.Join(p.Notes, " "), "nearly full") {
		t.Errorf("notes %v", p.Notes)
	}
}

func TestTownLabelWording(t *testing.T) {
	for _, tc := range []struct {
		xp   map[int]int
		want string
	}{
		{map[int]int{1013: 2100}, "Pick up The Book of Ur (Shadowfang Keep, 2,100 XP) from Keeper"},
		{map[int]int{1013: 950}, "Pick up The Book of Ur (Shadowfang Keep, 950 XP) from Keeper"},
		{nil, "Pick up The Book of Ur (Shadowfang Keep) from Keeper"},
	} {
		opt := townOpt()
		opt.XP = tc.xp
		p := BuildWith(townHero(5), townIndex(), opt)
		if got := p.Steps[0].Label; got != tc.want {
			t.Errorf("xp %v: %q, want %q", tc.xp, got, tc.want)
		}
	}
}

func TestThousands(t *testing.T) {
	for n, want := range map[int]string{5: "5", 999: "999", 1000: "1,000", 2100: "2,100", 1234567: "1,234,567"} {
		if got := thousands(n); got != want {
			t.Errorf("%d: %q", n, got)
		}
	}
}

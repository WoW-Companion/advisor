package unlock

import (
	"reflect"
	"testing"

	"github.com/WoW-Companion/advisor/internal/questie"
)

// chainIx is a synthetic isle: quest 30 is planned; the others hang off it in the ways under test.
func chainIx() *questie.Index {
	sp := func(x, y float64) []questie.Spawn { return []questie.Spawn{{Area: 901, X: x, Y: y}} }
	ix := &questie.Index{
		Quests: map[int]*questie.Quest{},
		Npcs: map[int]questie.Entity{
			1: {Name: "Elder Test", Spawns: sp(50, 50)},
			2: {Name: "Scout Test", Spawns: sp(52, 51)},
			3: {Name: "Lost Giver"}, // no spawns: its start is unknown
		},
		Areas: map[int]questie.Area{901: {UIMap: 7001, Zone: "Testisle"}},
	}
	add := func(id, lvl int, mut func(q *questie.Quest)) {
		q := &questie.Quest{ID: id, Name: "Quest " + itoa(id), Level: lvl, Starters: []questie.Ref{{ID: 1}}, Finishers: []questie.Ref{{ID: 1}}}
		if mut != nil {
			mut(q)
		}
		ix.Quests[id] = q
	}
	add(30, 9, nil)
	add(31, 9, func(q *questie.Quest) { q.PreSingle = []int{30} })
	add(32, 10, func(q *questie.Quest) { q.PreSingle = []int{31} })
	add(33, 10, func(q *questie.Quest) { q.PreSingle = []int{32} }) // three links from 30
	add(34, 9, func(q *questie.Quest) { q.PreGroup = []int{30, 35} })
	add(36, 9, func(q *questie.Quest) { q.PreSingle = []int{30}; q.Exclusive = []int{40} })
	add(37, 9, func(q *questie.Quest) { q.PreSingle = []int{30}; q.Until = 39 }) // a breadcrumb for 39
	add(38, 14, func(q *questie.Quest) { q.PreSingle = []int{30} })              // above the level band
	add(41, 9, func(q *questie.Quest) { q.PreSingle = []int{30}; q.Starters = []questie.Ref{{ID: 3}} })
	add(42, 9, func(q *questie.Quest) { q.PreGroup = []int{30, 43} }) // 43 is neither done nor planned
	add(44, 9, func(q *questie.Quest) { q.Next = 0 })
	ix.Quests[30].Next = 44 // next-in-chain
	return ix
}

func itoa(n int) string { return string(rune('0'+n/10)) + string(rune('0'+n%10)) }

func who() questie.Who {
	return questie.Who{Level: 9, Below: 2, Above: 2, Done: map[int]bool{35: true, 39: true, 40: true}, InLog: map[int]bool{}}
}

func ids(fs []Follow) map[int]int {
	out := map[int]int{}
	for _, f := range fs {
		out[f.ID] = f.Depth
	}
	return out
}

func TestFindUnlockRules(t *testing.T) {
	got := ids(Find(Input{Qx: chainIx(), Who: who(), Planned: []int{30}, OnMap: 7001}))
	want := map[int]int{
		31: 1, // a single prerequisite
		32: 2, // two links
		34: 1, // a group prerequisite: 30 planned, 35 done
		44: 1, // next in chain
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("follow-ups %v, want %v", got, want)
	}
	for id, why := range map[int]string{33: "past the depth cap", 36: "exclusive with a done quest", 37: "a breadcrumb for a done quest",
		38: "above the level band", 41: "start not known", 42: "group prerequisite unmet"} {
		if _, ok := got[id]; ok {
			t.Errorf("quest %d kept: %s", id, why)
		}
	}
}

func TestFindDepthOption(t *testing.T) {
	got := ids(Find(Input{Qx: chainIx(), Who: who(), Planned: []int{30}, OnMap: 7001, Depth: 3}))
	if got[33] != 3 {
		t.Fatalf("depth 3 follow-ups %v", got)
	}
	if got := ids(Find(Input{Qx: chainIx(), Who: who(), Planned: []int{30}, OnMap: 7001, Depth: 1})); got[32] != 0 {
		t.Fatalf("depth 1 kept quest 32: %v", got)
	}
}

func TestFindUnlockedByAndOtherMap(t *testing.T) {
	fs := Find(Input{Qx: chainIx(), Who: who(), Planned: []int{30}, OnMap: 7001})
	for _, f := range fs {
		if f.ID == 32 && !reflect.DeepEqual(f.UnlockedBy, []int{31}) {
			t.Errorf("quest 32 unlocked by %v", f.UnlockedBy)
		}
		if f.ID == 34 && !reflect.DeepEqual(f.UnlockedBy, []int{30}) {
			t.Errorf("quest 34 unlocked by %v", f.UnlockedBy)
		}
	}
	if got := Find(Input{Qx: chainIx(), Who: who(), Planned: []int{30}, OnMap: 7002}); len(got) != 0 {
		t.Errorf("givers on another map kept: %v", got)
	}
}

func TestFindAlreadyAvailableIsNotAFollowUp(t *testing.T) {
	w := who()
	w.Done[30] = true // 31 is now an ordinary pick-up
	if got := ids(Find(Input{Qx: chainIx(), Who: w, Planned: []int{99}, OnMap: 7001})); got[31] != 0 {
		t.Fatalf("an available quest was called a follow-up: %v", got)
	}
}

func TestFindFallsBackToQuestLines(t *testing.T) {
	ix := chainIx()
	ix.Quests[60] = &questie.Quest{ID: 60, Name: "Quest 60", Level: 9, Starters: []questie.Ref{{ID: 2}}, Finishers: []questie.Ref{{ID: 2}}} // no prerequisite fields
	lines := func(id int) (int, string) {
		if id == 30 {
			return 60, "The Test Line"
		}
		return 0, ""
	}
	fs := Find(Input{Qx: ix, Who: who(), Planned: []int{30}, Lines: lines, OnMap: 7001})
	var hit *Follow
	for i := range fs {
		if fs[i].ID == 60 {
			hit = &fs[i]
		}
	}
	if hit == nil || hit.Chain != "The Test Line" || !reflect.DeepEqual(hit.UnlockedBy, []int{30}) {
		t.Fatalf("lines-only follow-up %+v in %v", hit, fs)
	}
	if got := Find(Input{Who: who(), Planned: []int{30}, Lines: lines}); got != nil {
		t.Fatalf("with no Questie no quest can be named: %v", got)
	}
}

func TestFindObservedStartFillsAGap(t *testing.T) {
	ix := chainIx()
	ix.Quests[61] = &questie.Quest{ID: 61, Name: "Quest 61", Level: 9, Starters: nil, Finishers: []questie.Ref{{ID: 1}}}
	obs := []Obs{{From: 30, To: 61, NPC: "Seen Elder", Map: 7001, X: 49, Y: 50, Count: 1}}
	for _, f := range Find(Input{Qx: ix, Who: who(), Planned: []int{30}, Obs: obs, OnMap: 7001}) {
		if f.ID == 61 {
			if !f.Observed || f.Start == nil || f.Start.NPC != "Seen Elder" {
				t.Fatalf("observed follow-up %+v", f)
			}
			return
		}
	}
	t.Fatal("the observed unlock was not used")
}

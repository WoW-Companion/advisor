package route

import (
	"fmt"
	"math"
	"math/rand"
	"strconv"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/questie"
)

// fixture builds nq synthetic quests (pick-up, one or two objectives, turn-in) scattered around a
// few hubs, as a Problem and its naive quest-by-quest order. Every number is invented.
func fixture(seed int64, nq int) (*Problem, []int) {
	r := rand.New(rand.NewSource(seed))
	hubs := [][2]float64{{20, 30}, {70, 25}, {50, 75}}
	pt := func(h [2]float64, spread float64) (float64, float64) {
		return h[0] + (r.Float64()-.5)*spread, h[1] + (r.Float64()-.5)*spread
	}
	p := &Problem{Start: &[2]float64{50, 50}}
	var chains [][]int
	add := func(x, y float64, pred ...int) int {
		p.Nodes = append(p.Nodes, Node{X: x, Y: y, Pred: pred})
		return len(p.Nodes) - 1
	}
	for q := 0; q < nq; q++ {
		hub := hubs[r.Intn(len(hubs))]
		x, y := pt(hub, 6)
		pick := add(x, y)
		ch := []int{pick}
		var objs []int
		for k := 0; k < 1+r.Intn(2); k++ {
			ox, oy := pt(hubs[r.Intn(len(hubs))], 30)
			o := add(ox, oy, pick)
			objs = append(objs, o)
			ch = append(ch, o)
		}
		tx, ty := pt(hub, 6)
		ch = append(ch, add(tx, ty, objs...))
		chains = append(chains, ch)
	}
	var naive []int
	for _, ch := range chains {
		naive = append(naive, ch...)
	}
	return p, naive
}

func bruteForce(p *Problem) float64 {
	n := len(p.Nodes)
	best := math.Inf(1)
	order := make([]int, 0, n)
	used := make([]bool, n)
	var rec func(l float64)
	rec = func(l float64) {
		if l >= best {
			return
		}
		if len(order) == n {
			best = l
			return
		}
		for i := 0; i < n; i++ {
			ok := !used[i]
			for _, q := range p.Nodes[i].Pred {
				ok = ok && used[q]
			}
			if !ok {
				continue
			}
			d := p.fromStart(i)
			if len(order) > 0 {
				d = p.dist(order[len(order)-1], i)
			}
			used[i] = true
			order = append(order, i)
			rec(l + d)
			order = order[:len(order)-1]
			used[i] = false
		}
	}
	rec(0)
	return best
}

func randomFeasible(p *Problem, r *rand.Rand) []int {
	n := len(p.Nodes)
	done := make([]bool, n)
	var order []int
	for len(order) < n {
		var ready []int
		for i := 0; i < n; i++ {
			ok := !done[i]
			for _, q := range p.Nodes[i].Pred {
				ok = ok && done[q]
			}
			if ok {
				ready = append(ready, i)
			}
		}
		i := ready[r.Intn(len(ready))]
		done[i] = true
		order = append(order, i)
	}
	return order
}

func TestExactMatchesBruteForce(t *testing.T) {
	for _, tc := range []struct {
		seed int64
		nq   int
	}{{1, 2}, {2, 2}, {3, 3}, {4, 3}} {
		p, naive := fixture(tc.seed, tc.nq)
		if len(p.Nodes) > 9 {
			p.Nodes = p.Nodes[:9]
			naive = nil
			for i := range p.Nodes {
				p.Nodes[i].Pred = filterPred(p.Nodes[i].Pred, 9)
			}
		}
		order := Solve(p)
		if !p.Feasible(order) {
			t.Fatalf("seed %d: infeasible %v", tc.seed, order)
		}
		got, want := p.Length(order), bruteForce(p)
		if math.Abs(got-want) > 1e-9 {
			t.Errorf("seed %d: %d stops: optimiser %.3f, brute force %.3f", tc.seed, len(p.Nodes), got, want)
		}
		_ = naive
	}
}

func filterPred(pr []int, n int) []int {
	var out []int
	for _, q := range pr {
		if q < n {
			out = append(out, q)
		}
	}
	return out
}

func TestHeuristicBeatsRandomAndNaive(t *testing.T) {
	var saved []string
	for seed := int64(10); seed < 14; seed++ {
		for _, nq := range []int{4, 6} { // 12 stops is exact; these are 13 to 20
			p, naive := fixture(seed, nq)
			if len(p.Nodes) <= ExactMax {
				continue
			}
			order := Solve(p, naive)
			if !p.Feasible(order) {
				t.Fatalf("seed %d: infeasible", seed)
			}
			got, nv := p.Length(order), p.Length(naive)
			r := rand.New(rand.NewSource(seed))
			best := math.Inf(1)
			for i := 0; i < 1000; i++ {
				best = math.Min(best, p.Length(randomFeasible(p, r)))
			}
			if got > best*1.05 {
				t.Errorf("seed %d: %d stops: %.1f is over 5%% above the best random order %.1f", seed, len(p.Nodes), got, best)
			}
			if got > nv+1e-9 {
				t.Errorf("seed %d: optimiser %.1f is worse than naive %.1f", seed, got, nv)
			}
			saved = append(saved, strings.TrimSpace(strings.Join([]string{
				itoa(len(p.Nodes)) + " stops: naive " + f1(nv) + ", optimised " + f1(got) + ", best of 1000 random " + f1(best) + ", saved " + f1((nv-got)/nv*100) + "%"}, "")))
		}
	}
	t.Log("efficiency:\n" + strings.Join(saved, "\n"))
}

func TestExactNeverWorseThanNaive(t *testing.T) {
	var lines []string
	for seed := int64(20); seed < 24; seed++ {
		p, naive := fixture(seed, 3)
		if len(p.Nodes) > ExactMax {
			continue
		}
		got, nv := p.Length(Solve(p, naive)), p.Length(naive)
		if got > nv+1e-9 {
			t.Errorf("seed %d: %.1f worse than naive %.1f", seed, got, nv)
		}
		lines = append(lines, itoa(len(p.Nodes))+" stops: naive "+f1(nv)+", optimal "+f1(got)+", saved "+f1((nv-got)/nv*100)+"%")
	}
	t.Log("exact efficiency:\n" + strings.Join(lines, "\n"))
}

func itoa(n int) string   { return strconv.Itoa(n) }
func f1(v float64) string { return fmt.Sprintf("%.1f", v) }

// --- the plan from Questie-shaped data ---

func hubIsle() *questie.Index {
	sp := func(x, y float64) []questie.Spawn { return []questie.Spawn{{Area: 901, X: x, Y: y}} }
	ix := &questie.Index{
		Quests: map[int]*questie.Quest{},
		Npcs: map[int]questie.Entity{
			1: {Name: "Elder Test", Spawns: sp(50, 50)},
			2: {Name: "Scout Test", Spawns: sp(52, 51)},
			3: {Name: "Test Boar", Spawns: append(sp(60, 60), sp(61, 62)...)},
			4: {Name: "Test Wolf", Spawns: sp(30, 20)},
			5: {Name: "Far Elder", Spawns: []questie.Spawn{{Area: 902, X: 10, Y: 10}}},
		},
		Objects: map[int]questie.Entity{},
		Areas: map[int]questie.Area{
			901: {UIMap: 7001, Zone: "Testisle"},
			902: {UIMap: 7002, Zone: "Otherisle"},
		},
	}
	add := func(id, lvl int, starters, fin, creatures []int) {
		q := &questie.Quest{ID: id, Name: "Quest " + itoa(id), Level: lvl}
		for _, n := range starters {
			q.Starters = append(q.Starters, questie.Ref{ID: n})
		}
		for _, n := range fin {
			q.Finishers = append(q.Finishers, questie.Ref{ID: n})
		}
		for _, n := range creatures {
			q.Creatures = append(q.Creatures, questie.Ref{ID: n})
		}
		ix.Quests[id] = q
	}
	add(10, 9, []int{1}, []int{1}, []int{3}) // boars
	add(11, 9, []int{1}, []int{2}, []int{3}) // boars again: shares the objective stop
	add(12, 10, []int{2}, []int{2}, []int{4})
	add(13, 11, []int{1}, []int{1}, []int{4}) // too high: later
	add(14, 9, []int{1}, []int{5}, nil)       // turn-in on another map
	return ix
}

func hero() character.Character {
	lvl, mp := 9, 7001
	x, y := 48.0, 49.0
	return character.Character{Name: "Hero", QuestsCaptured: true, Level: &lvl, Location: &character.Location{Zone: "Testisle", MapID: &mp, X: &x, Y: &y}}
}

func TestPlanClustersAndOrders(t *testing.T) {
	p := Build(hero(), hubIsle())
	if p.Zone != "Testisle" || len(p.Steps) == 0 {
		t.Fatalf("plan %+v", p)
	}
	pos := map[string]int{}
	boars := 0
	for _, s := range p.Steps {
		if s.Kind == KindObjective && strings.Contains(s.Label, "Test Boar") {
			boars++
		}
		pos[s.Kind+s.Label] = s.N
	}
	if boars != 1 {
		t.Errorf("the boar objective of two quests should be one stop, got %d: %+v", boars, p.Steps)
	}
	for _, s := range p.Steps {
		if s.Kind == KindObjective && s.QuestID == 10 {
			for _, o := range p.Steps {
				if o.Kind == KindPickup && strings.Contains(o.Label, "Quest 10") && o.N > s.N {
					t.Errorf("pick-up after its objective: %+v", p.Steps)
				}
				if o.Kind == KindTurnIn && strings.Contains(o.Label, "Quest 10") && o.N < s.N {
					t.Errorf("turn-in before its objective: %+v", p.Steps)
				}
			}
		}
	}
	var later, travel bool
	for _, s := range p.Later {
		later = later || strings.Contains(s.Quest, "Quest 13")
	}
	for _, s := range p.Steps {
		travel = travel || (s.Kind == KindTravel && s.Label == "Walk or ride to Otherisle")
		if strings.Contains(s.Quest, "Quest 13") {
			t.Errorf("a too-high quest is on the route: %+v", s)
		}
	}
	if !later || !travel {
		t.Errorf("later=%v travel=%v: %+v", later, travel, p)
	}
	if p.Distance > p.Naive+1e-9 {
		t.Errorf("route %.1f longer than naive %.1f", p.Distance, p.Naive)
	}
}

func TestPlanSkipsDoneObjective(t *testing.T) {
	c := hero()
	yes := true
	c.Quests = []character.Quest{{QuestID: 10, Title: "Quest 10", Objectives: []character.Objective{{Text: "Test Boar slain: 8/8", Done: &yes}, {Text: "Test Wolf slain: 1/8"}}}}
	ix := hubIsle()
	ix.Quests[10].Creatures = append(ix.Quests[10].Creatures, questie.Ref{ID: 4})
	p := Build(c, ix)
	for _, s := range p.Steps {
		if s.Kind == KindObjective && s.QuestID == 10 && strings.Contains(s.Label, "Test Boar") {
			t.Errorf("a finished objective is on the route: %+v", s)
		}
		if s.Kind == KindPickup && s.QuestID == 10 {
			t.Errorf("a quest in the log has a pick-up: %+v", s)
		}
	}
}

func TestLuaIsDataOnly(t *testing.T) {
	p := Build(hero(), hubIsle())
	p.Character = `Evil"]=nil os.execute("x") --`
	lua := Envelope{Generated: 7, Plans: []Plan{p}}.Lua()
	if !strings.HasPrefix(lua, "-- Data only") || !strings.Contains(lua, "WoWCompanionPlan = {") || !strings.Contains(lua, `schema = "1.5"`) {
		t.Errorf("header: %s", lua)
	}
	if strings.Contains(lua, `Evil"]`) {
		t.Errorf("an unescaped quote reached the file:\n%s", lua)
	}
	if !strings.Contains(lua, `Evil\"]=nil`) {
		t.Errorf("the name was not escaped:\n%s", lua)
	}
	t.Log("sample Plan.lua:\n" + strings.ReplaceAll(Envelope{Generated: 1795363809, Plans: []Plan{Build(hero(), hubIsle())}}.Lua(), "\t", "  "))
}

func TestNoQuestieNoPlan(t *testing.T) {
	p := Build(hero(), nil)
	if len(p.Steps) != 0 || len(p.Notes) == 0 {
		t.Errorf("%+v", p)
	}
}

func TestClassQuestIsMustInclude(t *testing.T) {
	ix := hubIsle()
	ix.Quests[13].Classes = 1 << 7 // mage only; level 11 would otherwise wait in "later"
	ix.Quests[12].Classes = 1 << 4 // priest only: not for this mage
	c := hero()
	c.Class = "MAGE"
	p := Build(c, ix)
	var pick, other bool
	for _, s := range p.Steps {
		pick = pick || (s.Kind == KindPickup && strings.Contains(s.Quest, "Quest 13"))
		other = other || strings.Contains(s.Quest, "Quest 12")
	}
	if !pick {
		t.Errorf("class quest pick-up not on the route: %+v", p)
	}
	if other {
		t.Errorf("another class's quest on the route: %+v", p.Steps)
	}
}

func TestMustSetKeepsPrerequisites(t *testing.T) {
	nodes := []*spot{{}, {}, {must: true}, {}}
	got := mustSet(nodes, [][]int{nil, nil, {1}, nil})
	if !got[2] || !got[1] || got[0] || got[3] {
		t.Errorf("mustSet = %v", got)
	}
}

// --- objectives that are items, kills or unplaceable ---

// objIsle is a synthetic isle: quest 20 wants an item dropped by a mob, 21 an item from an object,
// 22 a kill, 23 an item nobody drops, 24 is finished.
func objIsle() *questie.Index {
	sp := func(x, y float64) []questie.Spawn { return []questie.Spawn{{Area: 901, X: x, Y: y}} }
	ix := hubIsle()
	ix.Quests = map[int]*questie.Quest{}
	ix.Npcs[6] = questie.Entity{Name: "Gem Imp", Spawns: sp(70, 40)}
	ix.Objects[1] = questie.Entity{Name: "Old Chest", Spawns: sp(20, 70)}
	ix.Items = map[int]questie.Item{
		100: {Name: "Spark Gem", Npcs: []int{6}},
		101: {Name: "Rusty Key", Objects: []int{1}},
		102: {Name: "Lost Ledger"},
	}
	add := func(id int, mut func(q *questie.Quest)) {
		q := &questie.Quest{ID: id, Name: "Quest " + itoa(id), Level: 9, Starters: []questie.Ref{{ID: 1}}, Finishers: []questie.Ref{{ID: 1}}}
		mut(q)
		ix.Quests[id] = q
	}
	add(20, func(q *questie.Quest) { q.Items = []int{100} })
	add(21, func(q *questie.Quest) { q.Items = []int{101} })
	add(22, func(q *questie.Quest) { q.Creatures = []questie.Ref{{ID: 4}} })
	add(23, func(q *questie.Quest) { q.Items = []int{102} })
	add(24, func(q *questie.Quest) { q.Items = []int{102} })
	return ix
}

func find(p Plan, kind string, q int) *Stop {
	for i, s := range p.Steps {
		if s.Kind == kind && strings.Contains(s.Quest, "Quest "+itoa(q)) {
			return &p.Steps[i]
		}
	}
	return nil
}

func TestItemObjectiveResolvedThroughNpcDrops(t *testing.T) {
	p := Build(hero(), objIsle())
	o := find(p, KindObjective, 20)
	if o == nil || o.X != 70 || o.Y != 40 || find(p, KindTurnIn, 20) == nil || find(p, KindTurnIn, 20).N < o.N {
		t.Errorf("quest 20: %+v", p.Steps)
	}
}

func TestItemObjectiveResolvedThroughObjectSources(t *testing.T) {
	p := Build(hero(), objIsle())
	if o := find(p, KindObjective, 21); o == nil || o.X != 20 || o.Y != 70 {
		t.Errorf("quest 21: %+v", p.Steps)
	}
}

func TestKillObjectiveResolved(t *testing.T) {
	if o := find(Build(hero(), objIsle()), KindObjective, 22); o == nil || o.X != 30 {
		t.Errorf("quest 22 has no kill stop")
	}
}

func TestUnresolvableObjectiveWithholdsTurnIn(t *testing.T) {
	p := Build(hero(), objIsle())
	if find(p, KindTurnIn, 23) != nil {
		t.Errorf("a turn-in for a quest with an unplaced objective: %+v", p.Steps)
	}
	if find(p, KindPickup, 23) == nil {
		t.Errorf("the pick-up must stay on the route")
	}
	var f *FindStop
	for i := range p.Find {
		if p.Find[i].QuestID == 23 {
			f = &p.Find[i]
		}
	}
	if f == nil || !strings.HasSuffix(f.Label, "(location unknown)") {
		t.Fatalf("find = %+v", p.Find)
	}
	lua := Envelope{Plans: []Plan{p}}.Lua()
	if !strings.Contains(lua, "find = {") || !strings.Contains(lua, "(location unknown)") {
		t.Errorf("no Find section:\n%s", lua)
	}
	if lines, _ := Lines(p, 50); !strings.Contains(strings.Join(lines, "\n"), "Find: ") {
		t.Errorf("overlay lines omit Find: %v", lines)
	}
}

func objChar() character.Character {
	c := hero()
	yes, no := true, false
	c.Quests = []character.Quest{
		{QuestID: 23, Title: "Quest 23", Complete: &no, Objectives: []character.Objective{{Text: "Lost Ledger: 0/1", Done: &no}, {Text: "Speak to the ghost: 0/1", Done: &no}}},
		{QuestID: 24, Title: "Quest 24", Complete: &yes, Objectives: []character.Objective{{Text: "Lost Ledger: 1/1", Done: &yes}}},
		{QuestID: 20, Title: "Quest 20", Complete: &no, Objectives: []character.Objective{{Text: "Spark Gem: 0/1", Done: &no}}},
	}
	return c
}

func TestCompletedQuestGoesStraightToTurnIn(t *testing.T) {
	p := Build(objChar(), objIsle())
	if find(p, KindTurnIn, 24) == nil || find(p, KindObjective, 24) != nil || find(p, KindPickup, 24) != nil {
		t.Errorf("completed quest: %+v", p.Steps)
	}
}

func TestLoggedUnfinishedQuestWithUnknownObjective(t *testing.T) {
	p := Build(objChar(), objIsle())
	// With no position anywhere, each open objective is still a step, by its zone alone.
	n := 0
	for _, s := range p.Steps {
		if s.Kind == KindObjective && s.QuestID == 23 && s.ZoneOnly && s.Place == "Testisle" && s.X == NoPos && s.Y == NoPos && s.Unplaced {
			n++
		}
	}
	if n != 2 {
		t.Errorf("want both objectives of quest 23 as zone-only steps, got %+v", p.Steps)
	}
	if i, j := stepIndex(p, KindTurnIn, 23), stepIndex(p, KindObjective, 23); i < 0 || j < 0 || j > i {
		t.Errorf("quest 23's objectives must come before its turn-in: %+v", p.Steps)
	}
}

// Every turn-in on a route is for a quest whose unfinished objectives all come earlier on it.
func TestNeverRouteTurnInForUnfinishedQuest(t *testing.T) {
	for name, tc := range map[string]struct {
		c  character.Character
		ix *questie.Index
	}{
		"hub": {hero(), hubIsle()}, "hub-log": {objChar(), hubIsle()},
		"obj": {hero(), objIsle()}, "obj-log": {objChar(), objIsle()},
	} {
		p := Build(tc.c, tc.ix)
		complete := map[string]bool{}
		for _, q := range tc.c.Quests {
			if q.Complete != nil && *q.Complete {
				complete[q.Title] = true
			}
		}
		for _, s := range p.Steps {
			if s.Kind != KindTurnIn {
				continue
			}
			for _, qn := range strings.Split(s.Quest, ", ") {
				var qid int
				fmt.Sscanf(qn, "Quest %d", &qid)
				for _, f := range p.Find {
					if f.QuestID == qid {
						t.Errorf("%s: turn-in for %s which has %q to find", name, qn, f.Label)
					}
				}
				q := tc.ix.Quests[qid]
				wants := len(q.Creatures) + len(q.Items) + len(q.Credits)
				if wants == 0 || complete[qn] {
					continue
				}
				seen := false
				for _, o := range p.Steps {
					if o.N < s.N && o.Kind == KindObjective && strings.Contains(o.Quest, qn) {
						seen = true
					}
				}
				if !seen {
					t.Errorf("%s: turn-in %q has no objective stop before it: %+v", name, s.Label, p.Steps)
				}
			}
		}
	}
}

func hasKind(p Plan, kind string, q int) bool { return stepIndex(p, kind, q) >= 0 }

func TestQuestsInTheLogHaveNoPickupAndNumberingStartsAtOne(t *testing.T) {
	c := hero()
	c.Quests = []character.Quest{{QuestID: 10, Title: "Quest 10", Objectives: []character.Objective{{Text: "Test Boar slain: 0/8"}}}}
	p := Build(c, hubIsle())
	if hasKind(p, KindPickup, 10) || !hasKind(p, KindObjective, 10) || !hasKind(p, KindTurnIn, 10) {
		t.Fatalf("a logged quest is routed from its objectives: %+v", p.Steps)
	}
	for i, s := range p.Steps {
		if s.N != i+1 || s.Done {
			t.Errorf("step %d is numbered %d, done=%v", i, s.N, s.Done)
		}
	}
}

func TestAbandonedQuestKeepsItsPickup(t *testing.T) {
	c := hero() // quest 10 is not in the log: it was abandoned, or never taken
	if p := Build(c, hubIsle()); !hasKind(p, KindPickup, 10) {
		t.Fatalf("an abandoned quest needs its pick-up: %+v", p.Steps)
	}
}

func TestNoPickupsWhenTheQuestLogIsUnknown(t *testing.T) {
	c := hero()
	c.QuestsCaptured = false
	p := Build(c, hubIsle())
	if hasKind(p, KindPickup, 10) || len(p.Notes) == 0 {
		t.Fatalf("pick-ups without a quest log, notes %v: %+v", p.Notes, p.Steps)
	}
}

func TestPlanBasisIsTheCaptureItWasBuiltFrom(t *testing.T) {
	c := hero()
	at := int64(1795363000)
	c.CapturedAt = &at
	c.Quests = []character.Quest{{QuestID: 12, Title: "Quest 12"}, {QuestID: 10, Title: "Quest 10"}}
	p := Build(c, hubIsle())
	if p.Basis.Captured != at || len(p.Basis.InLog) != 2 || p.Basis.InLog[0] != 10 {
		t.Fatalf("basis %+v", p.Basis)
	}
	lua := Envelope{Generated: at + 5, Plans: []Plan{p}}.Lua()
	if !strings.Contains(lua, "basis = { captured = 1795363000, inLog = { 10, 12 } },") {
		t.Errorf("Plan.lua lacks the basis:\n%s", lua)
	}
}

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
	return character.Character{Name: "Hero", Level: &lvl, Location: &character.Location{Zone: "Testisle", MapID: &mp, X: &x, Y: &y}}
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
		travel = travel || (s.Kind == KindTravel && s.Label == "Travel to Otherisle")
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
	if !strings.HasPrefix(lua, "-- Data only") || !strings.Contains(lua, "WoWCompanionPlan = {") || !strings.Contains(lua, `schema = "1.0"`) {
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

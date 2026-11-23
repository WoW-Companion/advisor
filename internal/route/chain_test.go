package route

import (
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/lua"
	"github.com/WoW-Companion/advisor/internal/questie"
	"github.com/WoW-Companion/advisor/internal/unlock"
)

// chainIsle: quest 30 (kill wolves, Elder) unlocks 31 (Elder to Scout), which unlocks 32 (Scout).
func chainIsle() *questie.Index {
	ix := hubIsle()
	ix.Quests = map[int]*questie.Quest{}
	add := func(id, lvl int, starters, fin, creatures, pre []int) {
		q := &questie.Quest{ID: id, Name: "Quest " + itoa(id), Level: lvl, PreSingle: pre}
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
	add(30, 9, []int{1}, []int{1}, []int{4}, nil)
	add(31, 9, []int{1}, []int{2}, []int{3}, []int{30})
	add(32, 10, []int{2}, []int{2}, nil, []int{31})
	return ix
}

func stepIndex(p Plan, kind string, q int) int {
	for i, s := range p.Steps {
		if s.Kind == kind && s.QuestID == q {
			return i
		}
	}
	return -1
}

func TestChainFollowUpsAreLockedAndOrdered(t *testing.T) {
	p := Build(hero(), chainIsle())
	t.Log("\n" + Render(p, 20))
	pick30, turn30 := stepIndex(p, KindPickup, 30), stepIndex(p, KindTurnIn, 30)
	pick31, turn31 := stepIndex(p, KindPickup, 31), stepIndex(p, KindTurnIn, 31)
	pick32 := stepIndex(p, KindPickup, 32)
	if pick30 < 0 || turn30 < 0 || pick31 < 0 || turn31 < 0 || pick32 < 0 {
		t.Fatalf("a chain stop is missing: %+v", p.Steps)
	}
	if !(pick30 < turn30 && turn30 < pick31 && pick31 < turn31 && turn31 < pick32) {
		t.Fatalf("the chain is out of order: %d %d %d %d %d", pick30, turn30, pick31, turn31, pick32)
	}
	s31, s32, t30 := p.Steps[pick31], p.Steps[pick32], p.Steps[turn30]
	if !s31.Locked || s31.Depth != 1 || len(s31.UnlockedBy) != 1 || s31.UnlockedBy[0] != 30 || s31.After != t30.N {
		t.Errorf("follow-up 31: %+v", s31)
	}
	if !s32.Locked || s32.Depth != 2 || s32.UnlockedBy[0] != 31 {
		t.Errorf("follow-up 32: %+v", s32)
	}
	if len(t30.Unlocks) != 1 || t30.Unlocks[0] != 31 || p.Steps[pick30].Locked {
		t.Errorf("turn-in 30: %+v", t30)
	}
	// The same NPC finishes 30 and starts 31: the pick-up comes straight after the turn-in.
	if pick31 != turn30+1 || !s31.Here {
		t.Errorf("31 is not right after the turn-in of 30 (steps %d, %d, here %t)", turn30, pick31, s31.Here)
	}
}

func TestNoFollowUpsWithoutPlannedTurnIn(t *testing.T) {
	ix := chainIsle()
	c := hero()
	c.CompletedQuests = []int{30, 31, 32}
	if p := Build(c, ix); len(p.Steps) != 0 {
		t.Fatalf("everything is done: %+v", p.Steps)
	}
}

func TestPrecedenceSolversStayFeasible(t *testing.T) {
	// Three quests in a chain; each pick-up needs the turn-in before it. Nodes 0..8: pick, obj, turn per quest.
	chain := func(n int) *Problem {
		p := &Problem{Start: &[2]float64{0, 0}}
		for q := 0; q < n; q++ {
			// The chain's natural order runs against distance: later quests are placed nearer the start.
			x := float64(90 - 30*q)
			base := len(p.Nodes)
			pred := []int{}
			if q > 0 {
				pred = []int{base - 1}
			}
			p.Nodes = append(p.Nodes, Node{X: x, Y: 10, Pred: pred}, Node{X: x, Y: 40, Pred: []int{base}}, Node{X: x, Y: 10, Pred: []int{base + 1}})
		}
		return p
	}
	p := chain(3)
	if got := Solve(p); !p.Feasible(got) || len(got) != 9 || got[0] != 0 || got[8] != 8 {
		t.Fatalf("exact order %v", got)
	}
	// Above ExactMax the heuristic runs: 5 chained quests (15 nodes), plus 2-opt must reject infeasible moves.
	big := chain(5)
	if got := Solve(big); !big.Feasible(got) {
		t.Fatalf("heuristic order %v is not feasible", got)
	}
	// 2-opt on its own: a worse feasible order is improved without breaking precedence.
	order := []int{0, 1, 2, 3, 4, 5, 6, 7, 8}
	improve(p, order)
	if !p.Feasible(order) {
		t.Fatalf("improve broke precedence: %v", order)
	}
}

func TestStepCapCountsFollowUps(t *testing.T) {
	ix := hubIsle()
	ix.Quests = map[int]*questie.Quest{}
	// Eight three-link chains, each link's giver at its own spot: 16 ordinary stops, so 9 are left for follow-ups.
	spot := func(i, row int) int {
		id := 50 + row*10 + i
		ix.Npcs[id] = questie.Entity{Name: "Giver " + itoa(id), Spawns: []questie.Spawn{{Area: 901, X: float64(5 + 11*i), Y: float64(10 + 20*row)}}}
		return id
	}
	for i := 0; i < 8; i++ {
		a, b, c, d := spot(i, 0), spot(i, 1), spot(i, 2), spot(i, 3)
		add := func(id int, pre []int, from, to int) {
			ix.Quests[id] = &questie.Quest{ID: id, Name: "Quest " + itoa(id), Level: 9, PreSingle: pre,
				Starters: []questie.Ref{{ID: from}}, Finishers: []questie.Ref{{ID: to}}}
		}
		add(100+i, nil, a, b)
		add(200+i, []int{100 + i}, b, c)
		add(300+i, []int{200 + i}, c, d)
	}
	p := Build(hero(), ix)
	if len(p.Steps) > MaxTotal || len(p.Steps) <= MaxSteps-4 {
		t.Fatalf("%d steps, cap %d", len(p.Steps), MaxTotal)
	}
	locked := 0
	for _, s := range p.Steps {
		if s.Locked {
			locked++
		}
		if s.Locked && s.Depth > 1 {
			t.Fatalf("a depth-%d step was kept while nearer, shallower follow-ups were cut: %+v", s.Depth, s)
		}
	}
	if locked == 0 || locked > MaxTotal-MaxSteps+4 {
		t.Fatalf("locked steps %d", locked)
	}
	if !strings.Contains(strings.Join(p.Notes, " "), "follow-up quests not planned") {
		t.Fatalf("notes %v", p.Notes)
	}
}

func TestPlanLuaCarriesChainFields(t *testing.T) {
	p := Build(hero(), chainIsle())
	text := Envelope{Generated: 1, Plans: []Plan{p}}.Lua()
	vars, err := lua.Parse(text)
	if err != nil {
		t.Fatal(err)
	}
	root := vars["WoWCompanionPlan"].(lua.Table)
	if root["schema"] != "1.1" {
		t.Fatalf("schema %v", root["schema"])
	}
	steps := lua.List(root["characters"].(lua.Table)["Hero"].(lua.Table)["steps"])
	var unlocks, locked lua.Table
	for _, s := range steps {
		st := s.(lua.Table)
		if st["kind"] == "turnin" && st["questID"] == int64(30) {
			unlocks = st
		}
		if st["kind"] == "pickup" && st["questID"] == int64(31) {
			locked = st
		}
	}
	if unlocks == nil || locked == nil {
		t.Fatalf("steps missing in %s", text)
	}
	if got := lua.List(unlocks["unlocks"]); len(got) != 1 || got[0] != int64(31) {
		t.Errorf("unlocks %v", got)
	}
	if locked["locked"] != true || locked["depth"] != int64(1) || len(lua.List(locked["unlockedBy"])) != 1 {
		t.Errorf("locked step %v", locked)
	}
	if _, has := unlocks["locked"]; has {
		t.Error("an ordinary step carries locked")
	}
}

func TestObservedAcceptedQuestIsRouted(t *testing.T) {
	ix := hubIsle()
	ix.Quests = map[int]*questie.Quest{
		// Level 9 but class-restricted prerequisites make it unavailable: only an observation puts it in the plan.
		50: {ID: 50, Name: "Quest 50", Level: 9, PreSingle: []int{999}, Starters: []questie.Ref{{ID: 2}}, Finishers: []questie.Ref{{ID: 2}}, Creatures: []questie.Ref{{ID: 4}}},
	}
	c := hero()
	captured := int64(1000)
	c.CapturedAt = &captured
	if p := Build(c, ix); len(p.Steps) != 0 {
		t.Fatalf("quest 50 should not be planned from the capture: %+v", p.Steps)
	}
	events := []unlock.Event{{T: 1500, Kind: unlock.EvAccepted, QuestID: 50, Character: "Hero", Map: 7001, X: 52, Y: 51, OffPlan: true}}
	p := BuildWith(c, ix, Options{Events: events})
	if stepIndex(p, KindObjective, 50) < 0 || stepIndex(p, KindTurnIn, 50) < 0 || stepIndex(p, KindPickup, 50) >= 0 {
		t.Fatalf("an accepted quest is routed from its objectives to its turn-in: %+v", p.Steps)
	}
}

func TestQuestLinesNeverInventAGiver(t *testing.T) {
	ix := chainIsle()
	ix.Npcs[9] = questie.Entity{Name: "Nowhere Man"} // known by name, no spawns
	ix.Quests[31].Starters = []questie.Ref{{ID: 9}}
	lines := func(id int) (int, string) {
		if id == 30 {
			return 31, "The Test Line"
		}
		return 0, ""
	}
	p := BuildWith(hero(), ix, Options{Lines: lines})
	if stepIndex(p, KindPickup, 31) >= 0 || stepIndex(p, KindPickup, 30) < 0 {
		t.Fatalf("a follow-up with an unknown start was routed: %+v", p.Steps)
	}
}

func TestObservedUnlockGivesTheGiver(t *testing.T) {
	ix := chainIsle()
	ix.Quests[31].Starters = nil // Questie does not know who starts 31
	obs := []unlock.Obs{{From: 30, To: 31, NPC: "Seen Elder", Map: 7001, X: 50, Y: 50, Count: 1}}
	p := BuildWith(hero(), ix, Options{Obs: obs})
	i := stepIndex(p, KindPickup, 31)
	if i < 0 || !p.Steps[i].Locked || !p.Steps[i].Observed || !strings.Contains(p.Steps[i].Label, "Seen Elder") {
		t.Fatalf("observed giver: %+v", p.Steps)
	}
	if got := Build(hero(), ix); stepIndex(got, KindPickup, 31) >= 0 {
		t.Fatal("without the observation the giver is unknown and 31 must be left out")
	}
}

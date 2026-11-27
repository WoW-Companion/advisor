package session

import (
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/route"
)

func ip(n int) *int   { return &n }
func bp(b bool) *bool { return &b }

func stop(n int, kind string, id int, label string, x, y float64) route.Stop {
	return route.Stop{N: n, Kind: kind, QuestID: id, QuestIDs: []int{id}, Quest: "Q" + label, Label: label, Map: 1, X: x, Y: y}
}

// fixture: a ready turn-in, a near wolf cluster, a far boar quest and an elite quest.
func fixture() In {
	steps := []route.Stop{
		stop(1, route.KindTurnIn, 1, "Turn in Ready to Mara", 50, 50),
		stop(2, route.KindObjective, 2, "Kill Wolf (0/6)", 52, 50),
		stop(3, route.KindTurnIn, 2, "Turn in Wolves to Mara", 50, 50),
		stop(4, route.KindObjective, 3, "Kill Boar (0/40)", 95, 95),
		stop(4, route.KindObjective, 3, "Collect Tusk (0/40)", 94, 95),
		stop(5, route.KindTurnIn, 3, "Turn in Boars to Gek", 95, 95),
		stop(6, route.KindObjective, 4, "Kill Ogre Chief (0/1)", 55, 55),
		stop(7, route.KindTurnIn, 4, "Turn in Chief to Mara", 50, 50),
	}
	c := character.Character{Level: ip(10), XPMax: ip(10000), Quests: []character.Quest{
		{QuestID: 1, Title: "Ready", Level: ip(10), Complete: bp(true)},
		{QuestID: 2, Title: "Wolves", Level: ip(10)},
		{QuestID: 3, Title: "Boars", Level: ip(10)},
		{QuestID: 4, Title: "Chief", Level: ip(12)},
	}}
	return In{Char: c, Route: route.Plan{Start: &route.Point{Map: 1, X: 50, Y: 52}, Steps: steps},
		Names: map[int]string{1: "Ready", 2: "Wolves", 3: "Boars", 4: "Chief"},
		Info: Info{
			XP:    func(id int) (int, bool) { return map[int]int{1: 300, 2: 400, 3: 900, 4: 1200}[id], id != 0 },
			Group: func(id int) bool { return id == 4 },
		},
		Anchors: []Anchor{{Name: "the inn at Mara's Rest", Map: 1, X: 49, Y: 50}}}
}

func TestPlanFitsBudgetWithTurnInsAndCluster(t *testing.T) {
	in := fixture()
	s := Plan(in, 30)
	if s.Seconds > 30*60 || s.Minutes() > 30 {
		t.Fatalf("plan is %.0fs, over 30 minutes", s.Seconds)
	}
	if len(s.Quests) < 2 || s.Quests[0] != "Ready" || !strings.Contains(strings.Join(s.Quests, ","), "Wolves") {
		t.Errorf("quests = %v, want the ready turn-in first, then the wolf cluster", s.Quests)
	}
	for _, q := range s.Quests {
		if q == "Chief" {
			t.Error("a group quest was planned")
		}
	}
	if s.XP != 700 || s.SharePct != 7 {
		t.Errorf("xp %d share %.1f, want 700 and 7", s.XP, s.SharePct)
	}
	if s.EndsAt == "" {
		t.Error("a short session ends at the inn")
	}
	if f := s.Facts(); !strings.Contains(f, "about") {
		t.Errorf("facts = %q", f)
	}
}

func TestPlanLongerTakesMore(t *testing.T) {
	in := fixture()
	if short, long := Plan(in, 15), Plan(in, 60); len(long.Quests) <= len(short.Quests) {
		t.Errorf("15 min %v, 60 min %v", short.Quests, long.Quests)
	}
}

func TestTimeModel(t *testing.T) {
	in := fixture()
	if got := StopSecs(route.Stop{Kind: route.KindObjective, Label: "Kill Wolf (2/8)"}); got != 6*KillSecs {
		t.Errorf("six kills = %v", got)
	}
	if got := StopSecs(route.Stop{Kind: route.KindObjective, Label: "Collect Linen (0/4)"}); got != 4*GatherSecs {
		t.Errorf("four items = %v", got)
	}
	if got := StopSecs(route.Stop{Kind: route.KindTurnIn}); got != TalkSecs {
		t.Errorf("turn in = %v", got)
	}
	a, b := route.Point{Map: 1, X: 0, Y: 0}, route.Point{Map: 1, X: 10, Y: 0}
	want := 10 * MapYards / 100 * PathFactor / WalkSpeed
	if got := in.WalkSecs(a, b); got != want {
		t.Errorf("walk = %v, want %v", got, want)
	}
	in.Char.Level = ip(40)
	if got, mounted := in.WalkSecs(a, b), want*WalkSpeed/MountSpeed; got != mounted {
		t.Errorf("mounted walk = %v, want %v", got, mounted)
	}
	for sec, want := range map[float64]int{60: 5, 25 * 60: 25, 27 * 60: 25, 28 * 60: 30} {
		if got := RoundMinutes(sec); got != want {
			t.Errorf("RoundMinutes(%v) = %d, want %d", sec, got, want)
		}
	}
}

func TestMissingXPFallsBackToQuestLevel(t *testing.T) {
	in := fixture()
	in.Info.XP = nil
	xp, est := in.QuestXP(2, 10)
	if !est || xp != Estimate(10) {
		t.Errorf("xp %d est %v, want %d estimated", xp, est, Estimate(10))
	}
	if s := Plan(in, 30); !s.Estimated || s.XP == 0 {
		t.Errorf("plan = %+v", s)
	}
}

func TestGreyAndGroupFlags(t *testing.T) {
	in := fixture()
	in.Char.Level = ip(20)
	in.Char.Quests[1].Level = ip(10) // Wolves: ten levels below
	vs := in.Triage()
	by := map[string]Verdict{}
	for _, v := range vs {
		by[v.Name] = v
	}
	if !by["Wolves"].Has(FlagGrey) || by["Wolves"].Advice != AdviceIfOnWay {
		t.Errorf("wolves = %+v", by["Wolves"])
	}
	if !by["Chief"].Has(FlagGroup) || by["Chief"].Advice != AdviceGroup {
		t.Errorf("chief = %+v", by["Chief"])
	}
	if by["Ready"].Advice != AdviceTurnIn || by["Ready"].Has(FlagGrey) {
		t.Errorf("a ready quest is turned in, never grey: %+v", by["Ready"])
	}
	if !by["Boars"].Has(FlagFar) {
		t.Errorf("boars are far: %+v", by["Boars"])
	}
}

func TestAbandonSuggestionsForFullLog(t *testing.T) {
	in := fixture()
	in.Char.Level = ip(20)
	for i := 10; i < 10+16; i++ { // 20 quests in all
		in.Char.Quests = append(in.Char.Quests, character.Quest{QuestID: i, Title: "Filler", Level: ip(20)})
	}
	vs := in.Triage()
	if len(vs) != 20 {
		t.Fatalf("log = %d", len(vs))
	}
	ab := Abandon(vs, 3)
	if len(ab) != 3 {
		t.Fatalf("abandon = %v", ab)
	}
	for _, v := range ab {
		if v.Complete || v.Upgrade {
			t.Errorf("never drop %+v", v)
		}
	}
	if ab[0].Name != "Wolves" && ab[0].Name != "Chief" && ab[0].Name != "Boars" {
		t.Errorf("flagged quests go first, got %q", ab[0].Name)
	}
	// a roomy log with nothing flagged drops nothing
	calm := fixture()
	calm.Info.Group = nil
	if ab := Abandon(calm.Triage(), 3); len(ab) != 1 || ab[0].Name != "Boars" { // only the far one
		t.Errorf("calm log abandon = %+v", ab)
	}
}

func TestUpgradeRewardRaisesValue(t *testing.T) {
	in := fixture()
	base := value(in.Triage(), "Boars")
	in.Info.Upgrade = func(id int) bool { return id == 3 }
	up := in.Triage()
	if value(up, "Boars") <= base {
		t.Errorf("upgrade value %v not above %v", value(up, "Boars"), base)
	}
	for _, v := range Abandon(up, 5) {
		if v.Name == "Boars" {
			t.Error("a quest with an upgrade reward is never dropped")
		}
	}
	if !strings.Contains(in.TriageFacts("boars"), "gear upgrade") {
		t.Error("the rating names the upgrade")
	}
}

func value(vs []Verdict, name string) float64 {
	for _, v := range vs {
		if v.Name == name {
			return v.Value
		}
	}
	return -1
}

func TestParseXP(t *testing.T) {
	m := ParseXP("QuestXP = {\n[100] = 450,\n [101]=1200, -- c\n[x] = 3\n}")
	if len(m) != 2 || m[100] != 450 || m[101] != 1200 {
		t.Errorf("m = %v", m)
	}
}

func TestScaleFor(t *testing.T) {
	for _, tc := range []struct {
		mine, quest int
		want        float64
	}{{30, 25, 1}, {30, 24, 0.8}, {30, 23, 0.6}, {30, 22, 0.4}, {30, 21, 0.2}, {30, 20, 0.1}, {14, 10, 1}, {14, 9, 0.8}, {12, 9, 1}} {
		if got := ScaleFor(tc.mine, tc.quest); got != tc.want {
			t.Errorf("ScaleFor(%d, %d) = %v, want %v", tc.mine, tc.quest, got, tc.want)
		}
	}
}

func TestGreyFlagFollowsTheGameGrayLevel(t *testing.T) {
	for _, c := range []struct {
		mine, quest int
		grey        bool
	}{{20, 14, false}, {20, 13, true}, {50, 40, false}, {50, 39, true}, {10, 5, false}, {10, 4, true}} {
		in := fixture()
		in.Char.Level = ip(c.mine)
		in.Char.XPMax = ip(10000) // a share never under 1%: only the level makes a quest grey
		in.Char.Quests[1].Level = ip(c.quest)
		in.Info.XP = func(int) (int, bool) { return 5000, true }
		for _, v := range in.Triage() {
			if v.Name == "Wolves" && v.Has(FlagGrey) != c.grey {
				t.Errorf("a level %d quest for a level %d character: grey %v, want %v", c.quest, c.mine, v.Has(FlagGrey), c.grey)
			}
		}
	}
}

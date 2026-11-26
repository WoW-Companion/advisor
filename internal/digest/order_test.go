package digest

import "testing"

func TestNextQuestsFollowRouteOrderAndSayWhy(t *testing.T) {
	lines := []Line{
		{Text: "talent", Kind: KindTalent},
		{Text: "a", Quest: 1, Kind: KindProgress},
		{Text: "b", Quest: 2, Kind: KindProgress},
		{Text: "c", Quest: 3, Kind: KindPickup, Class: true, Why: classQuestWhy},
		{Text: "off route", Quest: 9, Kind: KindProgress},
	}
	got := routeOrder(lines, []int{2, 3, 1})
	want := []string{"talent", "b", "c", "a", "off route"}
	for i, l := range got {
		if l.Text != want[i] {
			t.Fatalf("order %v, want %v", got, want)
		}
	}
	if got[2].Why != classQuestWhy {
		t.Errorf("the class quest keeps its reason, got %q", got[2].Why)
	}
	if got[1].Why != "First on your route" || got[3].Why != "" {
		t.Errorf("a line that jumps ahead says why, one that falls back stays quiet: %q / %q", got[1].Why, got[3].Why)
	}
	if same := routeOrder(lines, nil); same[1].Text != "a" {
		t.Errorf("no route, no reorder")
	}
}

func TestFurtherAwayComesLastAndNamesItsPlace(t *testing.T) {
	lines := []Line{
		{Text: "Blood Feeders: ready to turn in (Stonetalon Mountains)", Quest: 1, Kind: KindTurnIn, Zone: "Stonetalon Mountains"},
		{Text: "Pick up Light's Justice", Quest: 2, Kind: KindPickup},
		{Text: "Turn in here", Quest: 3, Kind: KindTurnIn},
	}
	far := map[int]string{1: "Stonetalon Mountains, Kalimdor: boat or zeppelin from Undercity"}
	got := furtherAway(routeOrder(lines, []int{3, 2, 1}), far)
	want := []string{"Turn in here", "Pick up Light's Justice", FurtherAway, "Blood Feeders: ready to turn in (Stonetalon Mountains, Kalimdor: boat or zeppelin from Undercity)"}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if got[i].Text != want[i] {
			t.Errorf("line %d = %q, want %q", i, got[i].Text, want[i])
		}
	}
	if same := furtherAway(lines, nil); len(same) != 3 {
		t.Errorf("nothing far, nothing added")
	}
}

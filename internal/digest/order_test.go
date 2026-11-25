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

package route

import "testing"

func TestQuestOrderCoversLaterAndFarAwayNamesOtherContinent(t *testing.T) {
	p := Plan{Zone: "Undercity",
		Steps: []Stop{{QuestID: 5, Place: "Undercity"}, {QuestIDs: []int{7}, Place: "Stonetalon Mountains"}},
		Later: []Stop{{QuestID: 6, Place: "Tirisfal Glades"}}}
	if got := QuestOrder(p); len(got) != 3 || got[0] != 5 || got[1] != 7 || got[2] != 6 {
		t.Errorf("order %v", got)
	}
	far := FarAway(p)
	if len(far) != 1 || far[7] != "Stonetalon Mountains, Kalimdor: boat or zeppelin from Undercity" {
		t.Errorf("far %v", far)
	}
}

package route

import (
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/questie"
)

// forecastWorld is a hand-built plan: five steps along a road, a kill quest at step 4, a vendor beside step 1 and
// a second beside step 3.
func forecastWorld() (*questie.Index, Plan) {
	ix := &questie.Index{Quests: map[int]*questie.Quest{
		1: {ID: 1, Name: "Hand-in", Items: []int{100, 101, 102}},
		2: {ID: 2, Name: "Panthers", Creatures: []questie.Ref{{ID: 9}}},
	}}
	st := func(n int, kind string, q int, label string, x float64) Stop {
		return Stop{N: n, Kind: kind, QuestID: q, QuestIDs: []int{q}, Label: label, Map: 7001, X: x, Y: 50}
	}
	p := Plan{Steps: []Stop{
		st(1, KindPickup, 2, "Pick up Panthers", 10),
		st(2, KindTurnIn, 1, "Turn in Hand-in", 20),
		st(3, KindObjective, 9, "Scout", 30),
		st(4, KindObjective, 2, "Kill Shadowmaw Panther", 40),
		st(5, KindTurnIn, 2, "Turn in Panthers", 50),
	}}
	return ix, p
}

func forecast(ix *questie.Index, p *Plan, bags, slots int, f *bagFacts) {
	c := character.Character{BagsCaptured: true, Bags: make([]character.ItemStack, bags)}
	for i := range c.Bags {
		c.Bags[i].ItemID = 5000 + i
	}
	p.bags = f
	addBagForecast(p, c, &questieSource{qx: ix}, Options{BagSlots: slots})
}

func shopAt(x float64) []Spot { return []Spot{{Name: "Vendor", Map: 7001, X: x, Y: 50}} }

func TestForecastPlentyOfRoomAddsNothing(t *testing.T) {
	ix, p := forecastWorld()
	forecast(ix, &p, 4, 20, &bagFacts{junk: 5, shops: shopAt(10)})
	if len(p.Errands) != 0 {
		t.Errorf("errands %+v", p.Errands)
	}
}

func TestForecastPlacesTheErrandAtTheLastVendorBeforeTheFill(t *testing.T) {
	ix, p := forecastWorld()
	// 9 of 10 used; the turn-in at step 2 adds a reward and the kill quest at step 4 two stacks of loot.
	forecast(ix, &p, 9, 10, &bagFacts{junk: 4, shops: append(shopAt(10), shopAt(30)...)})
	e := attached(p, ErrandSellJunk)
	if e == nil || e.Attach != 3 || !strings.Contains(e.Why, "bags fill around step 4 (Kill Shadowmaw Panther)") || !strings.Contains(e.Why, "frees 4 slots") {
		t.Fatalf("errand %+v", e)
	}
}

func TestForecastWithNoVendorBeforeGoesToTheFirstAfter(t *testing.T) {
	ix, p := forecastWorld()
	forecast(ix, &p, 9, 10, &bagFacts{junk: 4, shops: shopAt(50)})
	e := attached(p, ErrandSellJunk)
	if e == nil || e.Attach != 5 || !strings.Contains(e.Why, "first after") || !strings.Contains(e.Why, "step 4") {
		t.Fatalf("errand %+v", e)
	}
}

func TestForecastCountsWhatATurnInHandsIn(t *testing.T) {
	ix, p := forecastWorld()
	// The bags hold quest 1's three items; handing them in at step 2 leaves room for the panther loot.
	c := character.Character{BagsCaptured: true}
	for i, id := range []int{100, 101, 102, 1, 2, 3, 4, 5, 6} {
		c.Bags = append(c.Bags, character.ItemStack{ItemID: id, Count: 1})
		_ = i
	}
	p.bags = &bagFacts{junk: 4, shops: shopAt(10)}
	addBagForecast(&p, c, &questieSource{qx: ix}, Options{BagSlots: 10})
	if len(p.Errands) != 0 {
		t.Errorf("a turn-in frees the slots: %+v", p.Errands)
	}
	// Without the hand-in the same bags fill.
	delete(ix.Quests, 1)
	addBagForecast(&p, c, &questieSource{qx: ix}, Options{BagSlots: 10})
	if attached(p, ErrandSellJunk) == nil {
		t.Errorf("no hand-in, the bags fill")
	}
}

func TestForecastOffersNothingWhenOnlyQuestItemsAreLeft(t *testing.T) {
	ix, p := forecastWorld()
	// The guarded lists are empty: quest items are neither sold nor mailed, so there is nothing to place.
	forecast(ix, &p, 9, 10, &bagFacts{shops: shopAt(10), mailSpot: shopAt(10), guard: func(int64, string) bool { return true }})
	if len(p.Errands) != 0 {
		t.Errorf("errands %+v", p.Errands)
	}
}

func TestForecastNeedsABagCapture(t *testing.T) {
	ix, p := forecastWorld()
	p.bags = &bagFacts{junk: 4, shops: shopAt(10)}
	addBagForecast(&p, character.Character{}, &questieSource{qx: ix}, Options{BagSlots: 1})
	forecast(ix, &p, 9, 0, p.bags)
	if len(p.Errands) != 0 {
		t.Errorf("errands %+v", p.Errands)
	}
}

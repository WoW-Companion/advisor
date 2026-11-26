package route

import (
	"slices"
	"testing"

	"github.com/WoW-Companion/advisor/internal/advice"
	"github.com/WoW-Companion/advisor/internal/digest"
	"github.com/WoW-Companion/advisor/internal/store"
)

// nextQuestIDs is the quest ids of the panel's Next quests lines, in order.
func nextQuestIDs(d digest.Digest) []int {
	var ids []int
	for _, s := range d.Sections {
		if s.ID != digest.NextQuests {
			continue
		}
		for _, l := range s.Lines {
			if l.Quest != 0 {
				ids = append(ids, l.Quest)
			}
		}
	}
	return ids
}

// A character standing in a town with ready turn-ins and a pick-up right beside them: the turn-ins lead
// the route and the panel's Next quests, and the AI facts list the quests in that same order.
func TestTownTurnInsLeadRouteAndPanelAndFactsAgree(t *testing.T) {
	ix := readyWorld()
	c := readyChar(7003, 69, 69, 211, 212)
	ds := store.New(store.ReaderSource{})
	p := BuildWith(c, ix, Options{})
	order := QuestOrder(p)
	if len(order) < 3 || order[0] != 212 || order[1] != 211 || !slices.Contains(order, 213) {
		t.Fatalf("route order %v, want the ready turn-ins 212, 211 first and the pick-up 213 after", order)
	}

	panel := nextQuestIDs(digest.Build(c, ds, digest.Options{Questie: ix, Order: order}))
	if len(panel) < 3 || panel[0] != 212 || panel[1] != 211 {
		t.Fatalf("Next quests %v, want the ready turn-ins 212, 211 first", panel)
	}

	steps := advice.ByQuestOrder(advice.WhatsNextWith(c, ds, ix), func(s advice.Step) int { return s.QuestID }, order)
	var facts []int
	for _, s := range steps {
		if s.QuestID != 0 {
			facts = append(facts, s.QuestID)
		}
	}
	if !slices.Equal(facts, panel) {
		t.Errorf("facts order %v differs from the panel's %v", facts, panel)
	}
}

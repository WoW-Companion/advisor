package talents

import (
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/store"
)

type spellNodeRow struct {
	NodeID  int32  `parquet:"node_id"`
	SpellID int32  `parquet:"spell_id"`
	Name    string `parquet:"name"`
}

// withSpells is synth with a spell id on the first two nodes (1 Shadow Focus 7001, 2 Improved Mind Blast 7002).
func withSpells(t *testing.T) store.DataStore {
	b := func(order int16, node int32, pts int16) buildRow {
		return buildRow{"Priest", "Shadow Magic", "10-19", order, node, pts, "why", "curated"}
	}
	return store.New(store.ReaderSource{
		"talent_builds": table(t, []buildRow{b(1, 1, 5), b(2, 2, 5)}),
		"talent_nodes":  table(t, []spellNodeRow{{1, 7001, "Shadow Focus"}, {2, 7002, "Improved Mind Blast"}}),
	})
}

func spellNode(spell, rank int) character.Talent {
	return character.Talent{Rank: rank, SpellID: &spell}
}

func TestCapturedNodesBySpellID(t *testing.T) {
	ds := withSpells(t)
	// Shadow Focus is maxed, captured by spell id only: the next pick skips it.
	c := priest(12, 1, spellNode(7001, 5))
	c.Talents.Trees[0].Groups[1].Spent = 5
	p := Plan(c, ds, Options{Limit: 1})
	if p.Missing != "" || points(p) != "Improved Mind Blast=1" {
		t.Errorf("missing %q next %q", p.Missing, points(p))
	}
}

func TestNamelessNodesAreHonest(t *testing.T) {
	ds := withSpells(t)
	nameless := character.Talent{Rank: 3} // no name, no spell id: counts as spent, cannot be matched
	unknownSpell := spellNode(99999, 2)
	for _, tc := range []struct {
		name string
		tl   character.Talent
	}{{"nameless", nameless}, {"unknown spell", unknownSpell}} {
		c := priest(12, 1, tc.tl)
		c.Talents.Trees[0].Groups[1].Spent = tc.tl.Rank
		p := Plan(c, ds, Options{})
		if !strings.Contains(p.Missing, "can't see which talents you've taken") || len(p.Next) != 0 {
			t.Errorf("%s: missing %q next %q", tc.name, p.Missing, points(p))
		}
		if !Unreadable(c, ds) {
			t.Errorf("%s: readable", tc.name)
		}
	}
	if Unreadable(priest(12, 1, spellNode(7001, 5)), ds) {
		t.Error("a node matched by spell id is unreadable")
	}
}

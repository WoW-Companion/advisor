package companion

import (
	"testing"

	"github.com/WoW-Companion/advisor/internal/digest"
)

// The overlay's entries follow the panel's order, carry each entry's asks and ref once, and skip entries
// without questions.
func TestEntriesOf(t *testing.T) {
	d := digest.Digest{
		Sections: []digest.Section{
			{ID: digest.NextQuests, Lines: []digest.Line{
				{Text: "Turn in Wolves", Kind: digest.KindTurnIn, Title: "Wolves at the Door", Asks: []string{"What does it unlock?"}, Ref: "quest:100"},
				{Text: "2 unspent talent points", Kind: digest.KindTalent, Asks: []string{"Where should my talent points go?"}, Ref: "talents"},
				{Text: "No asks here"},
			}},
			{ID: digest.Upgrades, Lines: []digest.Line{{Text: "head: Hood", Kind: "upgrade", Asks: []string{"Is Hood worth it?"}, Ref: "item:1"}}},
		},
		Dungeons: []digest.DungeonLine{{Name: "Wolf Den", Line: "lvl 10-14", Asks: []string{"Am I ready?"}, Ref: "dungeon:1"}},
		Talents:  []digest.TalentLine{{Line: "Penance rank 1", Asks: []string{"Where should my talent points go?"}, Ref: "talents"}},
	}
	var got []string
	for _, e := range entriesOf(d) {
		got = append(got, e.Group+":"+e.Ref+":"+e.Title+":"+e.Status)
	}
	want := []string{"next:quest:100:Wolves at the Door:turn in", "next:talents:2 unspent talent points:", "upgrades:item:1:head: Hood:", "dungeons:dungeon:1:Wolf Den:lvl 10-14"}
	if len(got) != len(want) {
		t.Fatalf("got %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("entry %d = %q, want %q", i, got[i], want[i])
		}
	}
}

package digest

import (
	"bytes"
	"strings"
	"testing"

	"github.com/parquet-go/parquet-go"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/store"
)

func TestMaterialsFromRecipes(t *testing.T) {
	type recipe struct {
		SpellID       int32  `parquet:"spell_id"`
		SkillLineName string `parquet:"skill_line_name"`
		LearnSkill    *int16 `parquet:"learn_skill,optional"`
		GreySkill     *int16 `parquet:"grey_skill,optional"`
	}
	type reagent struct {
		SpellID       int32 `parquet:"spell_id"`
		ReagentItemID int32 `parquet:"reagent_item_id"`
		Count         int16 `parquet:"count"`
	}
	w := func(rows any) store.SizedReaderAt {
		var b bytes.Buffer
		var err error
		switch r := rows.(type) {
		case []recipe:
			err = parquet.Write(&b, r)
		case []reagent:
			err = parquet.Write(&b, r)
		}
		if err != nil {
			t.Fatal(err)
		}
		return store.SizedReaderAt{ReaderAt: bytes.NewReader(b.Bytes()), Size: int64(b.Len())}
	}
	n := func(v int16) *int16 { return &v }
	ds := store.New(store.ReaderSource{
		"recipes":         w([]recipe{{1, "Tailoring", n(1), n(80)}}),
		"recipe_reagents": w([]reagent{{1, 7, 2}}),
	})
	rank := 30
	c := character.Character{
		Professions: []character.Skill{{Name: "Tailoring", Rank: &rank}},
		Bags:        []character.ItemStack{{ItemID: 7, Count: 3, Name: "Bolt of Cloth"}, {ItemID: 8, Count: 9, Name: "Linen Cloth"}},
		Bank:        []character.ItemStack{{ItemID: 7, Count: 2, Name: "Bolt of Cloth"}},
	}
	// Recipes decide: the bank counts and Linen Cloth (a name match, not a reagent) is not kept.
	if got := materials(c, ds); len(got) != 1 || got[0].Text != "Bolt of Cloth x5" || got[0].Item != 7 {
		t.Errorf("with recipes: %+v", got)
	}
	// No recipes table: the name guess over the bags.
	if got := materials(c, store.New(store.ReaderSource{})); len(got) != 2 || !strings.HasPrefix(got[0].Text, "Linen Cloth") {
		t.Errorf("fallback: %+v", got)
	}
}

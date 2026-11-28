package paths_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/paths"
)

// The offline consumable use benchmark: whether the answer to "how do I get this potion, elixir, flask or food" says what
// the character needs to use it, beside the dataset's items rows (required_level, allowable_class_mask), with no network.
// The reference is the game's own rule: an item needs its required level, and a class potion or poison is for the classes
// its mask names. The snapshot is testdata/consumables_snapshot (see offline_consumables_benchmark_test.go).

const useSrc = "dataset items (required_level, allowable_class_mask)"

// useCases are crafted and bought consumables of every kind: a level gate from 5 to 60, a class potion and a poison, a
// flask, food and a drink with no gate.
var useCases = []string{
	"Rage Potion", "Mighty Rage Potion", "Mana Potion", "Elixir of Ogre Strength", "Elixir of the Mongoose", "Flask of Distilled Wisdom",
	"Swiftness Potion", "Westfall Stew", "Moonberry Juice", "Lesser Healing Potion",
}

// TestOfflineBenchmarkConsumableUse: the use line of a consumable's answer, for characters of five classes at three levels.
func TestOfflineBenchmarkConsumableUse(t *testing.T) {
	ds := consStore()
	masks := map[string]int64{"WARRIOR": 1, "ROGUE": 4, "PRIEST": 5, "MAGE": 8, "DRUID": 11}
	var out []benchRow
	for _, name := range useCases {
		it := rowByName(t, ds, "items", name)
		rl, mask := num(it["required_level"]), num(it["allowable_class_mask"])
		for _, cl := range []string{"WARRIOR", "ROGUE", "PRIEST", "MAGE", "DRUID"} {
			for _, lv := range []int{10, 30, 60} {
				l := lv
				c := character.Character{Faction: []string{"Alliance", "Horde"}[lv/30%2], Class: cl, Level: &l}
				text := paths.For(c, ds, num(it["id"]), paths.Options{}).Text()
				wantLevel := rl > 1 && int64(lv) < rl
				wantClass := mask > 0 && mask&(1<<(masks[cl]-1)) == 0
				hasLevel := strings.Contains(text, fmt.Sprintf("needs level %d to use", rl))
				hasClass := strings.Contains(text, "Only ") && strings.Contains(text, "can use it")
				ok := hasLevel == wantLevel && hasClass == wantClass
				ref := fmt.Sprintf("required_level %d, mask %d: level line %v, class line %v", rl, mask, wantLevel, wantClass)
				got := fmt.Sprintf("level line %v, class line %v", hasLevel, hasClass)
				out = append(out, benchRow{fmt.Sprintf("%s (%s, level %d): %s", cl, c.Faction, lv, name), got, ref, useSrc, resultIf(ok, "the answer omits the level or class an item needs, or states one it does not")})
			}
		}
	}
	finish(t, "advice-consumable-use", "Offline consumables: the level and class an item needs", out)
}

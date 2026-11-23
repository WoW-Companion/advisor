package paths

import (
	"errors"
	"sort"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/gear"
	"github.com/WoW-Companion/advisor/internal/store"
)

// TryPerSlot is how many of a slot's best candidates are checked for a way to get them.
const TryPerSlot = 40

// Gettable is an upgrade candidate with its ways to get it.
type Gettable struct {
	Slot      string
	Candidate gear.Candidate
	Result    Result
}

// InSlot returns up to limit upgrades for one slot (empty for every slot, one per slot) that have a
// dependable way to be got, best first. An upgrade with no way is skipped.
func InSlot(c character.Character, ds store.DataStore, group, slot string, opt Options, limit int) ([]Gettable, error) {
	if ds == nil {
		return nil, errors.New("no dataset configured")
	}
	res, err := gear.Find(c, ds, group, gear.Options{Slot: slot, NoSources: true})
	if err != nil {
		return nil, err
	}
	slots := make([]string, 0, len(res.Slots))
	for s := range res.Slots {
		if slot == "" || s == slot {
			slots = append(slots, s)
		}
	}
	sort.Strings(slots)
	var out []Gettable
	for _, s := range slots {
		n := 0
		for i, cd := range res.Slots[s] {
			if i == TryPerSlot || limit > 0 && n >= limit {
				break
			}
			if r := For(c, ds, cd.Item.ID, opt); r.Dependable() && cd.Item.Name != "" {
				r.Slot = s
				out = append(out, Gettable{s, cd, r})
				n++
				if slot == "" {
					break
				}
			}
		}
	}
	return out, nil
}

// SlotOf finds the capture slot an item is an upgrade for; "" when it is not one.
func SlotOf(c character.Character, ds store.DataStore, group string, itemID int64) string {
	res, err := gear.Find(c, ds, group, gear.Options{NoSources: true})
	if err != nil {
		return ""
	}
	var slots []string
	for s, l := range res.Slots {
		for _, cd := range l {
			if cd.Item.ID == itemID {
				slots = append(slots, s)
			}
		}
	}
	sort.Strings(slots)
	if len(slots) == 0 {
		return ""
	}
	return slots[0]
}

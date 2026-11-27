// Package inventory sorts a captured character's bags and bank into what to equip, keep for
// professions, hand in for quests, vendor, bank, or leave alone. The rules are in
// docs/inventory.md.
//
// Nothing is guessed: an item the dataset does not know, a price the dataset does not hold and a
// quest link the capture does not state are each reported as unknown.
package inventory

import (
	"fmt"
	"sort"
	"strings"

	"github.com/WoW-Companion/advisor/internal/brief"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/gear"
	"github.com/WoW-Companion/advisor/internal/professions"
	"github.com/WoW-Companion/advisor/internal/questie"
	"github.com/WoW-Companion/advisor/internal/store"
)

// Group is where an item belongs. An item takes the first group that fits, in this order.
type Group string

const (
	Equip   Group = "equip"
	Keep    Group = "keep"
	Quest   Group = "quest"
	Vendor  Group = "vendor"
	Bank    Group = "bank"
	Unknown Group = "unknown"
)

// Order is the precedence of the groups, and the order Render lists them.
var Order = []Group{Equip, Keep, Quest, Vendor, Bank, Unknown}

// poor is the quality id of grey items.
const poor = 0

// Entry is one stack and why it is in its group.
type Entry struct {
	ItemID    int64
	Name      string
	Count     int
	Where     string // "bags" or "bank"
	Group     Group
	Reason    string
	SellPrice *int64 // copper per item; nil when the dataset has no price
}

// Result is what Review found.
type Result struct {
	Groups map[Group][]Entry
	// VendorValue is the copper the vendor stacks sell for, over the stacks with a known price.
	VendorValue int64
	// VendorUnpriced counts vendor stacks with no known price; VendorValue leaves them out.
	VendorUnpriced int
	// SlotsFreed is the bag slots emptied by vendoring and banking what sits in the bags.
	SlotsFreed int
	Notes      []string
}

func i64(v any) int64 { n, _ := v.(int64); return n }

// BagReason is why a bag or quiver is left unplaced: the dataset holds no slot count for it.
const BagReason = "a bag or quiver: the dataset holds no bag size, so it can't say whether it beats the one you carry"

// IsBag reports whether an item row is a bag, a quiver or an ammo pouch: a container worn in a bag slot, which is no gear to bank or sell.
func IsBag(row store.Row) bool {
	cl := i64(row["class_id"])
	return (cl == 1 || cl == 11) && i64(row["slot"]) == 18
}

// HasProfession reports whether the character has the named profession.
func HasProfession(c character.Character, name string) bool {
	for _, s := range c.Professions {
		if strings.EqualFold(s.Name, name) {
			return true
		}
	}
	return false
}

// openItemQuests maps a lower-cased item name to the open quests whose item objectives name it.
func openItemQuests(c character.Character) map[string][]string {
	out := map[string][]string{}
	for _, q := range c.Quests {
		if q.Failed != nil && *q.Failed {
			continue
		}
		for _, o := range q.Objectives {
			if !strings.EqualFold(o.Type, "item") || o.Done != nil && *o.Done {
				continue
			}
			name, _, _ := strings.Cut(o.Text, ":")
			name = strings.ToLower(strings.TrimSpace(name))
			if name != "" && !contains(out[name], q.Title) {
				out[name] = append(out[name], q.Title)
			}
		}
	}
	return out
}

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

// classify picks the group of one item and says why.
func classify(c character.Character, id int64, name string, row store.Row, known bool, up map[int64]gear.Candidate,
	keep map[int64][]string, far map[int64][]string, needs Needs) (Group, string) {
	if q := needs.For(id, name, row); len(q) > 0 {
		// a quest in the log needs it, ready or not: kept by rule, before any sell or toss rule
		return Quest, Reason(q)
	}
	if cd, ok := up[id]; ok {
		if cd.Empty {
			return Equip, fmt.Sprintf("fills your empty %s slot", cd.Slot)
		}
		return Equip, fmt.Sprintf("upgrade for %s (+%.1f)", cd.Slot, cd.Delta)
	}
	if p, ok := keep[id]; ok {
		return Keep, "reagent for " + strings.Join(p, ", ") + " recipes within reach"
	}
	if p := professions.ToolOf(name); p != "" && HasProfession(c, p) {
		return Keep, "the " + p + " tool: " + p + " needs it in your bags"
	}
	if !known {
		return Unknown, "not in the dataset"
	}
	if IsBag(row) {
		return Unknown, BagReason
	}
	// a quest item is no vendor trash: the quest rule decides (a grey quest item nothing in the log asks for is not for the vendor)
	if row["quality"] != nil && i64(row["quality"]) == poor && !isQuestItem(row) {
		return Vendor, "poor quality (grey)"
	}
	// nor is it gear to bank: a quest item some quest does not need is the player's to destroy, as the all-characters review says
	if isQuestItem(row) {
		return Unknown, "a quest item no quest in your log needs: not for the vendor or the bank"
	}
	if p, ok := far[id]; ok {
		return Bank, "material for " + strings.Join(p, ", ") + " recipes more than " + fmt.Sprint(professions.KeepWindow) + " skill points away"
	}
	if i64(row["slot"]) != 0 {
		return Bank, "gear that is not an upgrade and has no other use"
	}
	return Unknown, "no rule places it"
}

// Review groups every bag and bank stack. ds may be nil: items are then placed only by the
// capture (quest objectives) and everything else is unknown.
func Review(c character.Character, ds store.DataStore) Result { return ReviewWith(c, ds, nil) }

// ReviewWith is Review that also reads the quests' items from the player's QuestieDB (qx may be nil).
func ReviewWith(c character.Character, ds store.DataStore, qx *questie.Index) Result {
	r := Result{Groups: map[Group][]Entry{}}
	if !c.BagsCaptured && len(c.Bags) == 0 {
		r.Notes = append(r.Notes, "Bags not captured.")
	}
	if !c.BankCaptured {
		r.Notes = append(r.Notes, "Bank not captured: open your bank once; only the bags were reviewed.")
	}
	up := map[int64]gear.Candidate{}
	keep := map[int64][]string{}
	var far map[int64][]string
	if ds == nil {
		r.Notes = append(r.Notes, "No dataset configured: gear, professions, quality and prices are unknown.")
	} else {
		group := ""
		if names, _ := brief.TopGroups(c.Talents); len(names) == 1 {
			group = names[0]
		}
		if res, err := gear.Find(c, ds, group, gear.Options{}); err != nil {
			r.Notes = append(r.Notes, "Gear upgrades unknown: "+err.Error()+".")
		} else {
			for _, cs := range res.Slots {
				for _, cd := range cs {
					if old, ok := up[cd.Item.ID]; !ok || cd.Delta > old.Delta {
						up[cd.Item.ID] = cd
					}
				}
			}
		}
		if ks, err := professions.KeepList(c, ds); err != nil {
			r.Notes = append(r.Notes, "Profession materials unknown: the dataset has no recipes for the character's professions.")
		} else {
			for _, k := range ks {
				keep[k.ItemID] = k.Professions
			}
		}
		far = professions.FarMaterials(c, ds)
	}
	needs := NeedsOf(c, qx)

	for _, loc := range []struct {
		where  string
		stacks []character.ItemStack
	}{{"bags", c.Bags}, {"bank", c.Bank}} {
		for _, s := range loc.stacks {
			id := int64(s.ItemID)
			var row store.Row
			known := false
			if ds != nil {
				if rw, err := ds.Get("items", id); err == nil && rw != nil {
					row, known = rw, true
				}
			}
			name := s.Name
			if name == "" && known {
				name, _ = row["name"].(string)
			}
			if name == "" {
				name = fmt.Sprintf("item %d", id)
			}
			g, why := classify(c, id, name, row, known, up, keep, far, needs)
			e := Entry{ItemID: id, Name: name, Count: s.Count, Where: loc.where, Group: g, Reason: why}
			if p, ok := row["sell_price"].(int64); ok && known {
				e.SellPrice = &p
			}
			r.Groups[g] = append(r.Groups[g], e)
			if loc.where == "bags" && (g == Vendor || g == Bank) {
				r.SlotsFreed++
			}
			if g == Vendor {
				if e.SellPrice == nil {
					r.VendorUnpriced++
				} else {
					r.VendorValue += *e.SellPrice * int64(s.Count)
				}
			}
		}
	}
	for _, l := range r.Groups {
		sort.SliceStable(l, func(i, j int) bool {
			if l[i].Name != l[j].Name {
				return l[i].Name < l[j].Name
			}
			return l[i].Where < l[j].Where
		})
	}
	if n := len(r.Groups[Vendor]); n > 0 && r.VendorUnpriced == n {
		r.Notes = append(r.Notes, "The dataset holds no sell prices: vendor value unknown.")
	} else if r.VendorUnpriced > 0 {
		r.Notes = append(r.Notes, fmt.Sprintf("%d vendor stack(s) have no price in the dataset: the value leaves them out.", r.VendorUnpriced))
	}
	return r
}

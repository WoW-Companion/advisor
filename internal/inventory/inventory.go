// Package inventory sorts a captured character's bags and bank into what to equip, keep for
// professions, hand in for quests, vendor, bank, or leave alone. The rules are in
// docs/inventory.md.
//
// Nothing is guessed: an item the dataset does not know, a price the dataset does not hold and a
// quest link the capture does not state are each reported as unknown.
package inventory

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/WoW-Companion/advisor/internal/brief"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/classreagent"
	"github.com/WoW-Companion/advisor/internal/gear"
	"github.com/WoW-Companion/advisor/internal/petfood"
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

// KeyReason is why a key is kept: the dataset says what a key is (item class 13), never what it opens or how to get another.
const KeyReason = "a key: keep it on your key ring; the dataset does not say what it opens or where to get another"

// ClassKeepReason is why a character keeps an item their own class's spells use up (a druid's Maple Seed, a warlock's Soul Shard) or make (a
// warlock's Healthstone), by the item's name; "" for any other item or another class's. The dataset holds no spell reagent table, so
// this is the advisor's own short list (package classreagent). A hunter also keeps the pet food of the advisor's own short list (package petfood).
func ClassKeepReason(c character.Character, name string) string {
	if r, ok := classreagent.Of(name); ok && strings.EqualFold(c.Class, r.Class) {
		return "a " + strings.ToLower(r.Class) + " reagent: your " + r.Spells + " use it up, so keep it, whatever a profession recipe also asks of it"
	}
	if cl := classreagent.MadeBy(name); cl != "" && strings.EqualFold(c.Class, cl) {
		return "made by your " + strings.ToLower(cl) + " Create spell: a " + strings.ToLower(cl) + " keeps it for the fight or the raid"
	}
	if d, ok := petfood.Of(name); ok && strings.EqualFold(c.Class, "HUNTER") {
		return PetFoodReason(d)
	}
	return ""
}

// bandageSubclass is the item subclass of a bandage among the consumables (class 0).
const bandageSubclass = 7

// lockboxName is a lockbox or a junkbox, which a rogue opens with Pick Lock.
var lockboxName = regexp.MustCompile(`(?i)\b(lock ?box(es)?|junk ?box(es)?)\b`)

// IsLockbox reports whether an item is a lockbox or a junkbox, by name: the dataset files them as ordinary consumables and holds no lock skill or contents.
func IsLockbox(name string) bool { return lockboxName.MatchString(name) }

// FirstAidReason is why anyone keeps a bandage or an anti-venom: any character can use one with or without First Aid, and the dataset holds no heal amount or cure to rank
// one against another. A bandage is an item the dataset files as class 0 subclass 7 or one named so (the dataset files the Dense Runecloth Bandage as subclass 8); an anti-venom is named.
// "" for any other item.
func FirstAidReason(name string, row store.Row) string {
	if i64(row["class_id"]) == 0 && i64(row["subclass_id"]) == bandageSubclass || firstAidName.MatchString(name) {
		return "First Aid supplies: any character can use a bandage or an anti-venom, with or without First Aid, so keep it; the dataset holds no heal amount or cure, so the advisor cannot rank one against another"
	}
	return ""
}

var firstAidName = regexp.MustCompile(`(?i)\b(anti-?venom|bandage)\b`)

// LockboxReason is why a rogue keeps a lockbox or a junkbox: Pick Lock opens it; the dataset holds no lock skill or contents.
func LockboxReason(c character.Character, name string) string {
	if IsLockbox(name) && strings.EqualFold(c.Class, "ROGUE") {
		return "a lockbox: your rogue picks it open, so keep it; the dataset holds no lock skill or contents"
	}
	return ""
}

// PetFoodReason is why a hunter keeps an item that is food for a pet: the dataset holds no pet diet, so the advisor's own short list of pet foods decides.
func PetFoodReason(diet string) string {
	return "pet food (" + diet + "): your pet eats it when you Feed Pet, so keep it, whatever a recipe or your own food rule says; the dataset holds no pet diet, so the advisor knows only its own short list of pet foods"
}

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
	if q := needs.Starts(id); len(q) > 0 {
		// an item that starts a quest the character has not taken: the item is the only way to the quest
		return Quest, StartsReason(q)
	}
	if cd, ok := up[id]; ok {
		if cd.Empty {
			return Equip, fmt.Sprintf("fills your empty %s slot", cd.Slot)
		}
		return Equip, fmt.Sprintf("upgrade for %s (+%.1f)", cd.Slot, cd.Delta)
	}
	if why := ClassKeepReason(c, name); why != "" {
		// a class reagent is used up by the class's own spells, whatever a profession recipe also asks of it
		return Keep, why
	}
	if why := FirstAidReason(name, row); why != "" {
		return Keep, why
	}
	if why := LockboxReason(c, name); why != "" {
		return Keep, why
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
	if isKey(row) {
		return Keep, KeyReason
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
	if g, why, ok := relicGroup(c, row); ok {
		return g, why
	}
	if i64(row["slot"]) != 0 {
		return Bank, "gear that is not an upgrade and has no other use"
	}
	return Unknown, "no rule places it"
}

// invRelic is the inventory type of a libram, idol or totem.
const invRelic = 28

// relicGroup places a relic (a libram, idol or totem) the character's class holds in its relic slot: it fills an empty relic slot, and is kept beside the one worn,
// because the dataset holds no relic effect, so one relic cannot be ranked against another. ok is false for any other item, and for another class's relic.
func relicGroup(c character.Character, row store.Row) (Group, string, bool) {
	if i64(row["class_id"]) != 4 || i64(row["slot"]) != invRelic {
		return "", "", false
	}
	if usable, known := gear.CanUse(c.Class, level(c), row); !known || !usable {
		return "", "", false
	}
	if rl, ok := requiredLevel(row); ok && rl > level(c) {
		return Keep, fmt.Sprintf("a relic for your class: you can use it from level %d", rl), true
	}
	if it, worn := c.Equipment["ranged"]; !worn || it.ItemID == nil {
		return Equip, "fills your empty relic slot (the dataset holds no relic effect, so it cannot rank relics)", true
	}
	return Keep, "a relic for your class: the dataset holds no relic effect to compare it with the one you wear", true
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

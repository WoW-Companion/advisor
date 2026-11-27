package paths

import (
	"fmt"
	"strings"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/gear"
	statcompass "github.com/WoW-Companion/advisor/internal/stats"
	"github.com/WoW-Companion/advisor/internal/store"
	"github.com/WoW-Companion/advisor/internal/wording"
)

// Shopping is a slot's Auction House search: not one item but a kind of item (a base item and a stat
// suffix), at a level range, assumed cheap. It never holds a price.
type Shopping struct {
	Slot     string   `json:"slot"`
	Type     string   `json:"type" jsonschema:"what to look for, such as 'mail chest'"`
	MinLevel int      `json:"min_level"`
	MaxLevel int      `json:"max_level"`
	Quality  string   `json:"quality"`
	Suffixes []string `json:"suffixes,omitempty" jsonschema:"stat suffixes that fit, in plain words"`
	Examples []string `json:"examples,omitempty" jsonschema:"one to three base items that drop in this range"`
	Line     string   `json:"line" jsonschema:"one line for the digest"`
	// Desc is the list's kind of item, level range and suffixes alone: no slot, no gain.
	Desc string `json:"-"`
	// Armor is the armor material of an armor slot ("cloth"), empty for other slots.
	Armor string   `json:"-"`
	Steps []string `json:"search_steps" jsonschema:"the in-game search, step by step"`
	// Best is the best upgrade among the items that fit the list.
	Best gear.Candidate `json:"-"`
}

// wantedSuffixes is what fits a class and role, best first, for a character with no play style (with one,
// the stat model ranks the suffixes, see stats.Prof.RankSuffixes). A role the table lacks falls back to the
// class's damage list.
var wantedSuffixes = map[string]map[string][]string{
	"WARRIOR": {"damage": {"bear", "strength", "tiger"}, "tank": {"bear", "stamina", "monkey", "strength"}},
	"PALADIN": {"damage": {"bear", "strength", "tiger"}, "tank": {"bear", "stamina", "strength"}, "healer": {"eagle", "owl", "intellect"}},
	"ROGUE":   {"damage": {"monkey", "agility", "tiger"}},
	"HUNTER":  {"damage": {"monkey", "agility", "falcon"}},
	"MAGE":    {"damage": {"eagle", "intellect", "owl"}},
	"WARLOCK": {"damage": {"eagle", "intellect"}},
	"PRIEST":  {"damage": {"eagle", "intellect"}, "healer": {"owl", "eagle", "intellect"}},
	"SHAMAN":  {"damage": {"eagle", "intellect", "owl"}, "healer": {"owl", "eagle", "intellect"}},
	"DRUID":   {"damage": {"eagle", "intellect", "owl"}, "tank": {"monkey", "stamina", "tiger"}, "healer": {"owl", "intellect", "eagle"}},
}

// roleOf is the stored play style's role, or the class's usual one when none is stored.
func roleOf(class string, p *character.Profile) string {
	role := ""
	if p != nil {
		role = strings.ToLower(strings.TrimSpace(p.Role))
	}
	if _, ok := wantedSuffixes[strings.ToUpper(class)][role]; ok {
		return role
	}
	if role == "" || role == "balanced" {
		if class == "PRIEST" {
			return "healer"
		}
	}
	return "damage"
}

// baseName is an item's name without a known stat suffix: "Striking Hammer of the Bear" is
// "Striking Hammer".
func baseName(name string) string {
	for _, s := range statcompass.Suffixes {
		if t := strings.TrimSuffix(name, " "+s.Name); t != name && t != "" {
			return t
		}
	}
	return name
}

// itemNoun is what to call a slot's item when searching: "chest", "boots".
var itemNoun = map[string]string{
	"head": "helm", "shoulder": "shoulders", "chest": "chest", "wrist": "bracers", "hands": "gloves",
	"waist": "belt", "legs": "leggings", "feet": "boots",
}

const shopTry = 60

// MaxSuffixes is how many suffixes a shopping list names: the top two for the player's priority.
const MaxSuffixes = 2

// Shop builds the slot's shopping list from its upgrade candidates, best first. bestKind is the kind
// of the best way already found for the slot ("" when none): a quest in reach or a craft is better
// than shopping, so then there is no list. ok is false when no candidate is an Auction House item.
func Shop(c character.Character, ds store.DataStore, slot string, cands []gear.Candidate, bestKind string) (Shopping, bool) {
	if ds == nil || c.Level == nil || bestKind == Quest || bestKind == Craft {
		return Shopping{}, false
	}
	type eligible struct {
		cd  gear.Candidate
		row store.Row
	}
	var els []eligible
	for i, cd := range cands {
		if i == shopTry {
			break
		}
		if cd.Soon {
			continue // not an upgrade for them now
		}
		if len(els) > 0 && cd.Item.Slot != els[0].cd.Item.Slot {
			continue // one kind of item per list: a one-hander list never holds a two-hander
		}
		row, err := ds.Get("items", cd.Item.ID)
		if err != nil {
			continue
		}
		f := &finder{c: c, ds: ds, itemID: cd.Item.ID, item: row, level: *c.Level}
		f.loot, _ = ds.Filter("loot_sources", store.Range{Column: "item_id", Min: cd.Item.ID, Max: cd.Item.ID})
		if _, ok := f.auction(f.creatures()); ok {
			els = append(els, eligible{cd, row})
		}
	}
	if len(els) == 0 {
		return Shopping{}, false
	}
	lo := els[0].cd.Item.ReqLevel
	for _, e := range els {
		lo = min(lo, e.cd.Item.ReqLevel)
	}
	hi := *c.Level
	lo = min(max(lo, hi-4), hi)
	sh := Shopping{Slot: slot, MinLevel: lo, MaxLevel: hi, Quality: "green or better", Best: els[0].cd}
	armor := ""
	for _, e := range els {
		if e.cd.Item.ReqLevel < lo {
			continue
		}
		if armor == "" {
			armor = gear.ArmorTypeWord(int(i64(e.row["class_id"])), int(i64(e.row["subclass_id"])))
		}
		if n := baseName(e.cd.Item.Name); len(sh.Examples) < 3 && !contains(sh.Examples, n) {
			sh.Examples = append(sh.Examples, n)
		}
	}
	noun, isArmorSlot := itemNoun[slot]
	if isArmorSlot && armor == "" {
		// no example sits in the level range, but the class still wears one armor type there
		armor = gear.ArmorFor(c.Class, hi)
	}
	switch {
	case isArmorSlot && armor != "":
		sh.Type = armor + " " + noun
	case isArmorSlot:
		sh.Type = noun
	default:
		sh.Type = strings.ToLower(wording.SlotName(slot, sh.Best.Item.Slot))
		if _, hand := map[int]bool{13: true, 17: true, 21: true, 22: true}[sh.Best.Item.Slot]; hand {
			sh.Type += " weapon"
		}
	}

	rows := statcompass.SuffixRows(ds)
	var quoted []string
	var top []statcompass.Suffix
	if p, ok := statcompass.For(c, ""); ok {
		top = p.VerifiedTop(rows, MaxSuffixes)
	} else {
		for _, key := range wantedSuffixes[strings.ToUpper(c.Class)][roleOf(c.Class, c.Profile)] {
			if s, ok := statcompass.SuffixByKey(key); ok && statcompass.Verified(s, rows) && len(top) < MaxSuffixes {
				top = append(top, s)
			}
		}
	}
	for _, s := range top {
		sh.Suffixes = append(sh.Suffixes, fmt.Sprintf("%s (%s)", s.Name, s.Words))
		quoted = append(quoted, "'"+s.Name+"'")
	}
	name := wording.SlotName(slot, sh.Best.Item.Slot)
	sh.Desc = fmt.Sprintf("any green %s, level %s", sh.Type, levelRange(lo, hi))
	if len(quoted) > 0 {
		sh.Desc += ", " + joinOr(quoted)
	}
	sh.Line = name + ": " + sh.Desc + " — cheap on the Auction House"
	if isArmorSlot {
		sh.Armor = armor
	}
	sh.Steps = searchSteps(sh, isArmorSlot, armor)
	return sh, true
}

func levelRange(lo, hi int) string {
	if lo >= hi {
		return fmt.Sprint(hi)
	}
	return fmt.Sprintf("%d–%d", lo, hi)
}

func joinOr(l []string) string {
	if len(l) < 2 {
		return strings.Join(l, "")
	}
	return strings.Join(l[:len(l)-1], ", ") + " or " + l[len(l)-1]
}

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

// searchSteps is the in-game Auction House search in plain steps. Never a price.
func searchSteps(sh Shopping, armorSlot bool, armor string) []string {
	filter := "In the Auction House's Browse tab, pick the " + strings.ToLower(wording.SlotName(sh.Slot, sh.Best.Item.Slot)) + " slot"
	if armorSlot {
		filter = "In the Auction House's Browse tab, pick Armor"
		if armor != "" {
			filter += ", then " + wording.Upper(armor)
		}
		filter += ", then the " + strings.ToLower(wording.SlotName(sh.Slot, sh.Best.Item.Slot)) + " slot"
	}
	steps := []string{
		filter + ".",
		"Set the level range to " + strings.Replace(levelRange(sh.MinLevel, sh.MaxLevel), "–", " to ", 1) + " and the quality to Uncommon (green) or better.",
	}
	switch {
	case len(sh.Suffixes) > 0 && len(sh.Examples) > 0:
		steps = append(steps, fmt.Sprintf("In the name box, type a base item such as \"%s\", or a suffix such as \"%s\", and look through the results for one of these suffixes: %s.",
			sh.Examples[0], suffixName(sh.Suffixes[0]), strings.Join(sh.Suffixes, "; ")))
	case len(sh.Suffixes) > 0:
		steps = append(steps, "In the name box, type a suffix such as \""+suffixName(sh.Suffixes[0])+"\"; the ones that fit you are: "+strings.Join(sh.Suffixes, "; ")+".")
	case len(sh.Examples) > 0:
		steps = append(steps, fmt.Sprintf("In the name box, type a base item such as \"%s\" (or leave it empty) and look through the results.", sh.Examples[0]))
	}
	return append(steps, "These are common world drops and usually cheap on this server, so buy the cheapest one with the stats you want.")
}

func suffixName(s string) string { n, _, _ := strings.Cut(s, " ("); return n }

// ShoppingList builds the shopping list for one slot, when the slot's best way to get an upgrade is
// the Auction House or there is no better quest or craft.
func ShoppingList(c character.Character, ds store.DataStore, group, slot string, opt Options) (Shopping, bool) {
	res, err := gear.Find(c, ds, group, gear.Options{Slot: slot, NoSources: true})
	if err != nil {
		return Shopping{}, false
	}
	cands := res.Slots[slot]
	kind := ""
	for i, cd := range cands {
		if i == TryPerSlot {
			break
		}
		if b, ok := For(c, ds, cd.Item.ID, opt).Best(); ok && cd.Item.Name != "" {
			kind = b.Kind
			break
		}
	}
	return Shop(c, ds, slot, cands, kind)
}

// Result renders a shopping list as an upgrade_paths result.
func (s Shopping) Result() Result {
	return Result{
		Item: "Any " + s.Type + ", level " + levelRange(s.MinLevel, s.MaxLevel), Slot: s.Slot,
		Paths:    []Path{{Kind: Auction, Line: s.Line, rank: rankAuction}},
		Shopping: &s,
	}
}

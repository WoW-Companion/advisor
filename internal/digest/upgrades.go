package digest

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/WoW-Companion/advisor/internal/brief"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/gear"
	"github.com/WoW-Companion/advisor/internal/paths"
	"github.com/WoW-Companion/advisor/internal/questie"
	"github.com/WoW-Companion/advisor/internal/stats"
	"github.com/WoW-Companion/advisor/internal/store"
	"github.com/WoW-Companion/advisor/internal/wording"
)

// Gain thresholds, in score points (item level plus quality, or the stat-weighted equivalent): a
// candidate that gains BigGain or more, or fills an empty slot, is a "Big upgrade"; SolidGain or more
// is "Solid"; anything else is "Small".
const (
	BigGain   = 10.0
	SolidGain = 5.0
)

// GainWord is a candidate's gain in words.
func GainWord(cd gear.Candidate) string {
	switch {
	case cd.Empty || cd.Delta >= BigGain:
		return "Big upgrade"
	case cd.Delta >= SolidGain:
		return "Solid upgrade"
	}
	return "Small upgrade"
}

// upgradesSummary is the section's count line.
func upgradesSummary(n int) string {
	if n == 0 {
		return ""
	}
	return fmt.Sprintf("%d %s you can get", n, wording.Plural(n, "upgrade", "upgrades"))
}

// upgradeLinks is what the upgrades say to the rest of the digest.
type upgradeLinks struct {
	questWhy map[int]string   // quest id -> why it is worth doing: it rewards an upgrade
	craftFor map[int64]string // reagent item id -> the upgrade it is for
	trains   []trainNeed      // weapon skills the listed upgrades need
}

// aOrAn puts the right article before a gain word's lower-case form: "a big", "a solid", "a small".
func lowerGain(g string) string { return strings.ToLower(strings.TrimSuffix(g, " upgrade")) }

// upgradeLines is one entry per slot: the best candidate that has a dependable way to be got, in
// plain words. An item with no way is never listed. The second result is a note when the search could
// not run.
func upgradeLines(c character.Character, ds store.DataStore, planQuests map[int]bool, qx *questie.Index, short string) (lines, notes []Line, link upgradeLinks) {
	link = upgradeLinks{questWhy: map[int]string{}, craftFor: map[int64]string{}}
	if ds == nil {
		return nil, nil, link // captureNotes says so
	}
	group := groupOf(c)
	bags := gear.BagUpgrades(c, ds, group)
	covered := map[string]bool{}
	for _, b := range bags {
		covered[b.Slot] = true
		lines = append(lines, bagLine(c, b))
		if b.Need != nil {
			g := ""
			if !b.Empty && !b.Likely {
				g = GainWord(gear.Candidate{Delta: b.Delta})
			}
			link.trains = append(link.trains, trainNeed{b.Need, b.Name, g, b.Empty || b.Likely || g != "Small upgrade"})
		}
	}
	res, err := gear.Find(c, ds, group, gear.Options{NoSources: true})
	if err != nil {
		return lines, []Line{{Text: "No gear upgrades: " + err.Error(), Kind: KindNote, Asks: asksOf("Why are there no gear upgrades?", "Why are there no gear upgrades?"), Ref: noteRef("no-upgrades")}}, link
	}
	slots := make([]string, 0, len(res.Slots))
	for s := range res.Slots {
		slots = append(slots, s)
	}
	rank := map[string]int{}
	for i, s := range brief.Slots {
		rank[s] = i + 1
	}
	sort.Slice(slots, func(i, j int) bool {
		a, b := rank[slots[i]], rank[slots[j]]
		if a == 0 || b == 0 {
			if a != b {
				return a != 0
			}
			return slots[i] < slots[j]
		}
		return a < b
	})
	opt := paths.Options{PlanQuests: planQuests, Questie: qx}
	taken := map[int64]bool{}
	var shops []shopAt
	for _, slot := range slots {
		if covered[slot] {
			continue // an item in the bags already covers this slot
		}
		var (
			found    bool
			foundCd  gear.Candidate
			foundWay paths.Path
		)
		for i, cd := range res.Slots[slot] {
			if i == paths.TryPerSlot {
				break
			}
			if taken[cd.Item.ID] {
				continue
			}
			best, ok := paths.For(c, ds, cd.Item.ID, opt).Best()
			if !ok || cd.Item.Name == "" {
				continue
			}
			found, foundCd, foundWay = true, cd, best
			break
		}
		// A slot whose best way is the Auction House (or that has no quest or craft) gets a shopping
		// list: what to search for, not one item.
		bestKind := ""
		if found {
			bestKind = foundWay.Kind
		}
		if sh, ok := paths.Shop(c, ds, slot, res.Slots[slot], bestKind); ok {
			shops = append(shops, shopAt{len(lines), slot, sh})
			lines = append(lines, Line{}) // placeholder: groupShopping fills it
			continue
		}
		if !found {
			continue
		}
		cd, best := foundCd, foundWay
		taken[cd.Item.ID] = true
		l := upgradeLine(c, slot, cd, best, short)
		lines = append(lines, l)
		if cd.Need != nil {
			link.trains = append(link.trains, trainNeed{cd.Need, cd.Item.Name, l.Gain, cd.Empty || l.Gain != "Small upgrade"})
		}
		if best.Kind == paths.Quest && best.QuestID != 0 {
			link.questWhy[best.QuestID] = firstWhy(
				fmt.Sprintf("Rewards %s, a %s %s upgrade", cd.Item.Name, lowerGain(l.Gain), strings.ToLower(l.Slot)),
				fmt.Sprintf("Rewards %s, an upgrade", cd.Item.Name),
				"Rewards an upgrade for you")
		}
		if best.Kind == paths.Craft {
			for _, g := range best.Reagents {
				if _, dup := link.craftFor[g.ItemID]; !dup {
					link.craftFor[g.ItemID] = cd.Item.Name
				}
			}
		}
	}
	return groupShopping(c, lines, shops), nil, link
}

// upgradeLine is one entry, in plain words: no item levels, no ids.
func upgradeLine(c character.Character, slot string, cd gear.Candidate, best paths.Path, short string) Line {
	name := wording.SlotName(slot, cd.Item.Slot)
	wear := "Wear now"
	if cd.Soon {
		wear = fmt.Sprintf("From level %d", cd.Item.ReqLevel)
	}
	if cd.Need != nil && !cd.Soon {
		wear = "After you train"
	}
	gain := GainWord(cd)
	l := Line{Text: clip(cd.Item.Name), Item: int(cd.Item.ID), Slot: name, ILvl: cd.Item.ItemLevel, Wear: wear, Gain: gain, Replaces: replacedName(c, slot, cd.Empty), How: oneLineHow(best.Line), PathKind: best.Kind}
	if cd.Soon {
		l.AtLevel = cd.Item.ReqLevel
	}
	details := []string{wording.Upper(best.Line)} // the way in full; the item, wear, gain and replaces are fields
	if cd.Need != nil {
		details = append(details, cd.Need.Line())
		l.Train = cd.Need.Skill
	}
	l.Details = finishDetails(details)
	var mats []string
	for _, g := range best.Reagents {
		if g.Name != "" && len(mats) < 2 {
			mats = append(mats, g.Name)
		}
	}
	l.Asks, l.Ref = upgradeAsks(cd.Item.Name, name, whoOf(c), best, mats), itemRef(int(cd.Item.ID))
	l.Why = upgradeWhy(c, slot, cd, short)
	return l
}

// bagLine is an item already in the bags that beats what is worn: free, so it leads the section.
func bagLine(c character.Character, b gear.BagUpgrade) Line {
	name := wording.SlotName(b.Slot, b.Item.Slot)
	l := Line{Text: clip(b.Name), Item: int(b.Item.ID), Slot: name, ILvl: b.Item.ItemLevel, Wear: "Wear now",
		Replaces: b.Replaces, How: "In your bags; equip it now", PathKind: PathBags}
	switch {
	case b.Empty:
		l.Gain = "Big upgrade"
	case !b.Likely:
		l.Gain = GainWord(gear.Candidate{Delta: b.Delta})
	}
	details := []string{name + ": " + b.Name + ", in your bags; equip it now"}
	if b.Need != nil {
		l.Wear, l.How, l.Train = "After you train", b.Need.Line(), b.Need.Skill
		details[0] = name + ": " + b.Name + ", in your bags"
		details = append(details, b.Need.Line())
	}
	if b.Replaces != "" {
		l.Why = firstWhy("Beats your " + b.Replaces)
		details = append(details, "Replaces "+b.Replaces)
	}
	if b.Likely && !b.Empty {
		details = append(details, "Likely better: higher quality or item level")
	}
	l.Details = finishDetails(details)
	l.Asks, l.Ref = asksOf("Should I equip "+b.Name+" now?", "Should I equip "+b.Name+" now?"), itemRef(int(b.Item.ID))
	return l
}

// bagNextLine is the NEXT line for n upgrades in the bags.
func bagNextLine(n int) Line {
	text := "You have an upgrade in your bags"
	if n > 1 {
		text = fmt.Sprintf("You have %d upgrades in your bags", n)
	}
	q := "Which items in my bags should I equip now?"
	return Line{Text: text, Kind: KindNote, Details: []string{"Equip them now; they are free"}, Asks: asksOf(q, q), Ref: noteRef("bag-upgrades"), Why: "Free upgrades, already yours"}
}

// PathBags is the path kind of an upgrade that is already in the bags.
const PathBags = "bags"

// bagCount is how many of the lines are bag upgrades.
func bagCount(lines []Line) int {
	n := 0
	for _, l := range lines {
		if l.PathKind == PathBags && l.Train == "" { // one that needs training is not free to equip
			n++
		}
	}
	return n
}

// shoppingLine is a slot's Auction House shopping list as one entry: a kind of item, a level range and
// the stat suffixes that fit, never a price.
func shoppingLine(c character.Character, slot string, sh paths.Shopping) Line {
	name := wording.SlotName(slot, sh.Best.Item.Slot)
	gain := GainWord(sh.Best)
	how := auctionHow(sh)
	l := Line{Text: clip(sh.Desc), Slot: name, Wear: "Wear now", Gain: gain, Replaces: replacedName(c, slot, sh.Best.Empty), How: how, PathKind: paths.Auction}
	var details []string
	if len(sh.Suffixes) > 0 {
		details = append(details, "Suffixes that fit: "+strings.Join(sh.Suffixes, "; "))
	}
	if len(sh.Examples) > 0 {
		details = append(details, "Examples: "+strings.Join(sh.Examples, ", "))
	}
	l.Details = finishDetails(details)
	if len(sh.Steps) > 0 { // the full search steps live here, whole, not in how
		l.Details = append(l.Details[:min(len(l.Details), MaxDetails-1)], "Search steps: "+strings.Join(sh.Steps, " "))
	}
	var sufs []string
	for _, x := range sh.Suffixes {
		n, _, _ := strings.Cut(x, " (")
		sufs = append(sufs, n)
	}
	l.Asks, l.Ref = shoppingAsks(strings.ToLower(name), sufs, l.Replaces), noteRef("auction-"+slot)
	return l
}

// replacedName is the name of the item in the slot, nothing but the name; empty for an empty slot (or
// when the capture holds no name for it).
func replacedName(c character.Character, slot string, empty bool) string {
	if empty {
		return ""
	}
	return c.Equipment[slot].Name
}

// groupOf is the character's one top talent group name; "" when unknown or split.
func groupOf(c character.Character) string {
	if names, _ := brief.TopGroups(c.Talents); len(names) == 1 {
		return names[0]
	}
	return ""
}

// shortStat is the stat the stat check says the character is most short of; "" when none.
func shortStat(r stats.Result) string {
	best, gap := "", 0.0
	for _, s := range stats.Primary {
		if v := r.Lean[s]; v > gap && v > 0.05 {
			best, gap = s, v
		}
	}
	return best
}

// MaxHow is the longest "how" line: one line, never a paragraph.
const MaxHow = 90

// auctionHow is a shopping list's one-line how, such as "Cheap on the Auction House: search 'of
// Strength', level 7–10". The full search steps go into the entry's details.
func auctionHow(sh paths.Shopping) string {
	lv := fmt.Sprintf("level %d", sh.MaxLevel)
	if sh.MinLevel < sh.MaxLevel {
		lv = fmt.Sprintf("level %d–%d", sh.MinLevel, sh.MaxLevel)
	}
	if len(sh.Suffixes) > 0 {
		name, _, _ := strings.Cut(sh.Suffixes[0], " (")
		if how := fmt.Sprintf("Cheap on the Auction House: search '%s', %s", name, lv); utf8.RuneCountInString(how) <= MaxHow {
			return how
		}
	}
	return oneLineHow("Cheap on the Auction House: " + sh.Type + ", " + lv)
}

// oneLineHow is s as one line of at most MaxHow characters: whole when it fits, else its first
// clause that fits, else its words without a dangling connector.
func oneLineHow(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) <= MaxHow {
		return s
	}
	for _, sep := range []string{". ", " — ", "; ", ": ", ", "} {
		if i := strings.LastIndex(s[:len(wording.CutWords(s, MaxHow))], sep); i > 20 {
			return strings.TrimRight(s[:i], " ,;:.—")
		}
	}
	return wholeCut(s, MaxHow)
}

// UpgradeQuests are the quests whose reward is a gear upgrade for the character, by quest id, each
// with the plain reason; empty when the dataset is missing or there is none.
func UpgradeQuests(c character.Character, ds store.DataStore, qx *questie.Index) map[int]string {
	_, _, link := upgradeLines(c, ds, nil, qx, "")
	return link.questWhy
}

// shopAt is a shopping list and where its placeholder sits among the lines.
type shopAt struct {
	idx  int
	slot string
	sh   paths.Shopping
}

// groupable are the slots whose shopping lists can be one line: the armor slots and the cloak.
func groupable(sh paths.Shopping) bool { return sh.Armor != "" || sh.Slot == "back" }

// groupShopping fills the placeholders: shopping lists with the same level range and suffixes (and the
// same armor material; the cloak joins the first group with the same range and suffixes) become one
// line, "Green cloth gear, level 20–24, 'of the Eagle' or 'of the Owl': helm, shoulders, ...". A list
// that is alone keeps its own line. Specific upgrades (not shopping lists) are untouched.
func groupShopping(c character.Character, lines []Line, shops []shopAt) []Line {
	key := func(sh paths.Shopping, material string) string {
		return fmt.Sprintf("%s|%d|%d|%s", material, sh.MinLevel, sh.MaxLevel, strings.Join(sh.Suffixes, ";"))
	}
	groups := map[string][]int{} // key -> indexes into shops
	var order []string
	for i, sa := range shops {
		if !groupable(sa.sh) || sa.sh.Armor == "" {
			continue
		}
		k := key(sa.sh, sa.sh.Armor)
		if _, ok := groups[k]; !ok {
			order = append(order, k)
		}
		groups[k] = append(groups[k], i)
	}
	for i, sa := range shops { // the cloak joins the first armor group of its range and suffixes
		if sa.sh.Slot != "back" || sa.sh.Armor != "" {
			continue
		}
		for _, k := range order {
			first := shops[groups[k][0]].sh
			if key(first, "") == key(sa.sh, "") {
				groups[k] = append(groups[k], i)
				break
			}
		}
	}
	drop, grouped := map[int]bool{}, map[int]bool{}
	for _, k := range order {
		idx := groups[k]
		if len(idx) < 2 {
			continue
		}
		sort.Ints(idx)
		lines[shops[idx[0]].idx] = groupLine(c, shops, idx)
		for _, i := range idx[1:] {
			drop[shops[i].idx] = true
		}
		for _, i := range idx {
			grouped[i] = true
		}
	}
	for i, sa := range shops {
		if !grouped[i] {
			lines[sa.idx] = shoppingLine(c, sa.slot, sa.sh)
		}
	}
	out := lines[:0:0]
	for i, l := range lines {
		if !drop[i] {
			out = append(out, l)
		}
	}
	return out
}

// groupLine is one line for several slots' shopping lists that share a level range and suffixes.
func groupLine(c character.Character, shops []shopAt, idx []int) Line {
	first := shops[idx[0]].sh
	for _, i := range idx { // the cloak may sort first, and has no armor word of its own
		if shops[i].sh.Armor != "" {
			first = shops[i].sh
			break
		}
	}
	var names []string
	cloak := false
	best := first.Best
	empty := false
	for _, i := range idx {
		sa := shops[i]
		names = append(names, groupSlotWord(sa.slot))
		cloak = cloak || sa.slot == "back"
		if sa.sh.Best.Delta > best.Delta {
			best = sa.sh.Best
		}
		empty = empty || sa.sh.Best.Empty
	}
	var quoted []string
	for _, x := range first.Suffixes {
		n, _, _ := strings.Cut(x, " (")
		quoted = append(quoted, "'"+n+"'")
	}
	head := "Green " + first.Armor + " gear, level " + levelRangeWord(first)
	if len(quoted) > 0 {
		head += ", " + joinOr(quoted)
	}
	shown := append([]string(nil), names...)
	if cloak { // a cloak is cloth for everyone, whatever the rest wear
		for i, n := range shown {
			if n == slotWords["back"] {
				shown[i] = n + " (any cloth)"
			}
		}
	}
	text := head + ": " + strings.Join(shown, ", ")
	if utf8.RuneCountInString(text) > MaxText {
		text = head
	}
	best.Empty = best.Empty || empty
	gain := GainWord(best)
	l := Line{Text: text, Slot: wording.Upper(strings.Join(names, ", ")), Wear: "Wear now", Gain: gain, How: auctionHow(first), PathKind: paths.Auction}
	details := []string{"Slots: " + strings.Join(names, ", ")}
	if len(first.Suffixes) > 0 {
		details = append(details, "Suffixes that fit: "+strings.Join(first.Suffixes, "; "))
	}
	if len(first.Examples) > 0 {
		details = append(details, "Examples: "+strings.Join(first.Examples, ", "))
	}
	if len(first.Steps) > 0 {
		details = append(details[:min(len(details), MaxDetails-1)], "Search steps: "+strings.Join(first.Steps, " "))
	}
	l.Details = finishDetails(details)
	var sufs []string
	for _, q := range quoted {
		sufs = append(sufs, strings.Trim(q, "'"))
	}
	l.Asks, l.Ref = shoppingAsks(strings.ToLower(strings.Join(names, ", ")), sufs, ""), noteRef("auction-"+first.Slot)
	return l
}

// slotWords are the slots as an item is called when shopping.
var slotWords = map[string]string{"head": "helm", "shoulder": "shoulders", "back": "cloak", "chest": "chest",
	"wrist": "bracers", "hands": "gloves", "waist": "belt", "legs": "legs", "feet": "boots"}

func groupSlotWord(slot string) string {
	if w, ok := slotWords[slot]; ok {
		return w
	}
	return strings.ToLower(wording.SlotName(slot, 0))
}

func levelRangeWord(sh paths.Shopping) string {
	if sh.MinLevel >= sh.MaxLevel {
		return fmt.Sprint(sh.MaxLevel)
	}
	return fmt.Sprintf("%d–%d", sh.MinLevel, sh.MaxLevel)
}

func joinOr(l []string) string {
	if len(l) < 2 {
		return strings.Join(l, "")
	}
	return strings.Join(l[:len(l)-1], ", ") + " or " + l[len(l)-1]
}

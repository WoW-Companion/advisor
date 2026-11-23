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
}

// aOrAn puts the right article before a gain word's lower-case form: "a big", "a solid", "a small".
func lowerGain(g string) string { return strings.ToLower(strings.TrimSuffix(g, " upgrade")) }

// upgradeLines is one entry per slot: the best candidate that has a dependable way to be got, in
// plain words. An item with no way is never listed. The second result is a note when the search could
// not run.
func upgradeLines(c character.Character, ds store.DataStore, planQuests map[int]bool, short string) (lines, notes []Line, link upgradeLinks) {
	link = upgradeLinks{questWhy: map[int]string{}, craftFor: map[int64]string{}}
	if ds == nil {
		return nil, nil, link // captureNotes says so
	}
	group := groupOf(c)
	res, err := gear.Find(c, ds, group, gear.Options{NoSources: true})
	if err != nil {
		return nil, []Line{{Text: "No gear upgrades: " + err.Error(), Kind: KindNote, Asks: asksOf("Why are there no gear upgrades?", "Why are there no gear upgrades?"), Ref: noteRef("no-upgrades")}}, link
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
	opt := paths.Options{PlanQuests: planQuests}
	taken := map[int64]bool{}
	for _, slot := range slots {
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
			lines = append(lines, shoppingLine(c, slot, sh))
			continue
		}
		if !found {
			continue
		}
		cd, best := foundCd, foundWay
		taken[cd.Item.ID] = true
		l := upgradeLine(c, slot, cd, best, short)
		lines = append(lines, l)
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
	return lines, nil, link
}

// upgradeLine is one entry, in plain words: no item levels, no ids.
func upgradeLine(c character.Character, slot string, cd gear.Candidate, best paths.Path, short string) Line {
	name := wording.SlotName(slot, cd.Item.Slot)
	wear := "Wear now"
	if cd.Soon {
		wear = fmt.Sprintf("From level %d", cd.Item.ReqLevel)
	}
	gain := GainWord(cd)
	replaces := "fills an empty slot"
	if !cd.Empty {
		replaces = "instead of what you wear"
		if eq, ok := c.Equipment[slot]; ok && eq.Name != "" {
			replaces = "instead of your " + eq.Name
		}
	}
	text := fmt.Sprintf("%s: %s. %s, %s, %s", name, cd.Item.Name, wear, strings.ToLower(gain), replaces)
	l := Line{Text: clip(text), Item: int(cd.Item.ID), Slot: name, ILvl: cd.Item.ItemLevel, Wear: wear, Gain: gain, Replaces: replaces, How: oneLineHow(best.Line), PathKind: best.Kind}
	if cd.Soon {
		l.AtLevel = cd.Item.ReqLevel
	}
	l.Details = finishDetails([]string{
		name + ": " + cd.Item.Name,
		wear + ". " + gain + ", " + replaces,
		wording.Upper(best.Line),
	})
	l.Asks, l.Ref = upgradeAsks(cd.Item.Name), itemRef(int(cd.Item.ID))
	l.Why = upgradeWhy(c, slot, cd, short)
	return l
}

// shoppingLine is a slot's Auction House shopping list as one entry: a kind of item, a level range and
// the stat suffixes that fit, never a price.
func shoppingLine(c character.Character, slot string, sh paths.Shopping) Line {
	name := wording.SlotName(slot, sh.Best.Item.Slot)
	gain := GainWord(sh.Best)
	replaces := "fills an empty slot"
	if !sh.Best.Empty {
		replaces = "instead of what you wear"
		if eq, ok := c.Equipment[slot]; ok && eq.Name != "" {
			replaces = "instead of your " + eq.Name
		}
	}
	how := auctionHow(sh)
	l := Line{Text: clip(sh.Line), Slot: name, Wear: "Wear now", Gain: gain, Replaces: replaces, How: how, PathKind: paths.Auction}
	details := []string{sh.Line}
	if len(sh.Suffixes) > 0 {
		details = append(details, "Suffixes that fit: "+strings.Join(sh.Suffixes, "; "))
	}
	if len(sh.Examples) > 0 {
		details = append(details, "Examples: "+strings.Join(sh.Examples, ", "))
	}
	l.Details = finishDetails(append(details, wear(gain, replaces)))
	if len(sh.Steps) > 0 { // the full search steps live here, whole, not in how
		l.Details = append(l.Details[:min(len(l.Details), MaxDetails-1)], "Search steps: "+strings.Join(sh.Steps, " "))
	}
	l.Asks, l.Ref = shoppingAsks(), noteRef("auction-"+slot)
	l.Why = firstWhy(gain + " for your " + strings.ToLower(name))
	return l
}

func wear(gain, replaces string) string { return "Wear now. " + gain + ", " + replaces }

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

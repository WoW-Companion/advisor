package digest

import (
	"fmt"
	"sort"
	"strings"

	"github.com/WoW-Companion/advisor/internal/brief"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/gear"
	"github.com/WoW-Companion/advisor/internal/paths"
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
func upgradeLines(c character.Character, ds store.DataStore, planQuests map[int]bool) (lines, notes []Line, link upgradeLinks) {
	link = upgradeLinks{questWhy: map[int]string{}, craftFor: map[int64]string{}}
	if ds == nil {
		return nil, nil, link // captureNotes says so
	}
	group := ""
	if names, _ := brief.TopGroups(c.Talents); len(names) == 1 {
		group = names[0]
	}
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
		l := upgradeLine(c, slot, cd, best)
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
func upgradeLine(c character.Character, slot string, cd gear.Candidate, best paths.Path) Line {
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
	l := Line{Text: clip(text), Item: int(cd.Item.ID), Slot: name, ILvl: cd.Item.ItemLevel, Wear: wear, Gain: gain, Replaces: replaces, How: best.Line, PathKind: best.Kind}
	if cd.Soon {
		l.AtLevel = cd.Item.ReqLevel
	}
	l.Details = finishDetails([]string{
		name + ": " + cd.Item.Name,
		wear + ". " + gain + ", " + replaces,
		wording.Upper(best.Line),
	})
	facts := fmt.Sprintf("%s for my %s slot, %s (%s). The best way to get it: %s", cd.Item.Name, strings.ToLower(name), strings.ToLower(gain), replaces, best.Line)
	l.Prompt = makePrompt(c, "A possible upgrade is ", facts+".", " Is it worth getting for how I play?")
	l.Asks, l.Ref = upgradeAsks(cd.Item.Name), itemRef(int(cd.Item.ID))
	l.Why = upgradeWhy(c, slot, cd)
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
	how := "Search the Auction House: " + strings.Join(sh.Steps, " ")
	l := Line{Text: clip(sh.Line), Slot: name, Wear: "Wear now", Gain: gain, Replaces: replaces, How: how, PathKind: paths.Auction}
	details := []string{sh.Line}
	if len(sh.Suffixes) > 0 {
		details = append(details, "Suffixes that fit: "+strings.Join(sh.Suffixes, "; "))
	}
	if len(sh.Examples) > 0 {
		details = append(details, "Examples: "+strings.Join(sh.Examples, ", "))
	}
	l.Details = finishDetails(append(details, wear(gain, replaces)))
	facts := fmt.Sprintf("a shopping list for my %s slot, %s (%s): %s. Search steps: %s", strings.ToLower(name), strings.ToLower(gain), replaces, sh.Line, strings.Join(sh.Steps, " "))
	l.Prompt = makePrompt(c, "A possible upgrade is ", facts, " What should I search for on the Auction House?")
	l.Asks, l.Ref = shoppingAsks(), noteRef("auction-"+slot)
	l.Why = firstWhy(gain + " for your " + strings.ToLower(name))
	return l
}

func wear(gain, replaces string) string { return "Wear now. " + gain + ", " + replaces }

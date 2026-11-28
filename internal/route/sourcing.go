package route

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/inventory"
	"github.com/WoW-Companion/advisor/internal/professions"
	"github.com/WoW-Companion/advisor/internal/questie"
	"github.com/WoW-Companion/advisor/internal/store"
)

// Sourcing: before a reagent is bought, the player's own bank gives what it holds, then another captured character
// on the same realm and faction mails what it can spare, and only the rest is bought. Both sides come from one rule
// (sources), so the other character's plan carries the matching mail errand.

// take is a part of a short count that one source covers.
type take struct {
	From   string // "" is the character's own bank
	Count  int
	Stale  bool // the other character's capture is older than this one's
	NoBank bool // the other character's bank was never captured
}

// sourcer holds what the sourcing rule reads about each character, once.
type sourcer struct {
	qx    *questie.Index
	needs map[string]inventory.Needs
}

func newSourcer(qx *questie.Index) *sourcer {
	return &sourcer{qx: qx, needs: map[string]inventory.Needs{}}
}

func sameChar(a, b character.Character) bool { return a.Key == b.Key && a.Name == b.Name }

func charName(c character.Character) string {
	if c.Name != "" {
		return c.Name
	}
	return c.Key
}

// canMail is true when o can mail c: the same realm and faction, as mail crosses neither.
func canMail(c, o character.Character) bool {
	if sameChar(c, o) || c.Faction == "" || !strings.EqualFold(c.Faction, o.Faction) {
		return false
	}
	return c.Realm == "" || o.Realm == "" || strings.EqualFold(c.Realm, o.Realm)
}

func countOf(stacks []character.ItemStack, id int64) int {
	n := 0
	for _, s := range stacks {
		if int64(s.ItemID) == id {
			n += s.Count
		}
	}
	return n
}

// reagentNeeds is what the character's recipes that still give skill-ups use up for RestockBatch crafts, by item, with
// the reagent names and the order they were first seen in.
func reagentNeeds(c character.Character) (need map[int64]int, names map[int64]string, order []int64) {
	need, names = map[int64]int{}, map[int64]string{}
	profs := make([]string, 0, len(c.Recipes))
	for k := range c.Recipes {
		profs = append(profs, k)
	}
	sort.Strings(profs)
	for _, k := range profs {
		for _, r := range c.Recipes[k].Entries {
			if r.Difficulty == "grey" || r.Difficulty == "gray" {
				continue
			}
			for _, rg := range r.Reagents {
				id := int64(rg.ItemID)
				if _, ok := need[id]; !ok {
					order = append(order, id)
				}
				need[id] += rg.Count * RestockBatch
				names[id] = rg.Name
			}
		}
	}
	return need, names, order
}

// spare is how much of an item o can give away: nothing a quest in its log needs or starts, and not what its own
// profession path will use up.
func (s *sourcer) spare(o character.Character, id int64, name string) int {
	k := o.Key + "|" + o.Name
	n, ok := s.needs[k]
	if !ok {
		n = inventory.NeedsOf(o, s.qx)
		s.needs[k] = n
	}
	if len(n.For(id, name, nil)) > 0 || len(n.Starts(id)) > 0 {
		return 0
	}
	own, _, _ := reagentNeeds(o)
	return max(0, countOf(o.Bags, id)+countOf(o.Bank, id)-own[id])
}

// sources covers up to need of an item for c: its own bank (when bank is true and was captured), then the characters
// that canMail it (when mail is true), in name order. rest is what no source covers.
func (s *sourcer) sources(c character.Character, chars []character.Character, id int64, name string, need int, bank, mail bool) (out []take, rest int) {
	rest = need
	if bank && c.BankCaptured && rest > 0 {
		if n := min(rest, countOf(c.Bank, id)); n > 0 {
			out = append(out, take{Count: n})
			rest -= n
		}
	}
	if !mail {
		return out, rest
	}
	alts := slices.DeleteFunc(slices.Clone(chars), func(o character.Character) bool { return !canMail(c, o) })
	sort.SliceStable(alts, func(i, j int) bool { return charName(alts[i]) < charName(alts[j]) })
	for _, o := range alts {
		if rest <= 0 {
			break
		}
		if n := min(rest, s.spare(o, id, name)); n > 0 {
			stale := c.CapturedAt != nil && o.CapturedAt != nil && *o.CapturedAt < *c.CapturedAt
			out = append(out, take{From: charName(o), Count: n, Stale: stale, NoBank: !o.BankCaptured})
			rest -= n
		}
	}
	return out, rest
}

// reachable keeps the spots a chore could attach to: within BundleReach of a stop with a position.
func reachable(p *Plan, spots []Spot) []Spot {
	var out []Spot
	for _, sp := range spots {
		for _, s := range p.Steps {
			if s.Map == sp.Map && s.X >= 0 && s.Y >= 0 && dist(s.X, s.Y, sp.X, sp.Y) <= BundleReach {
				out = append(out, sp)
				break
			}
		}
	}
	return out
}

// pickupChores are the bank and mail errands that cover part of a short item, and what is still to buy. A source
// with no stop to attach to is left out, so the buy stays.
func pickupChores(p *Plan, c character.Character, qx *questie.Index, opt Options, maps map[int]bool, id int64, name string, need int) ([]Chore, int) {
	bankers := reachable(p, spotsWhere(qx, c, maps, false, func(e questie.Entity) bool { return e.Flags&bankerFlag != 0 }))
	boxes := reachable(p, spotsWhere(qx, c, maps, true, func(e questie.Entity) bool { return strings.Contains(strings.ToLower(e.Name), "mailbox") }))
	takes, rest := newSourcer(qx).sources(c, opt.Others, id, name, need, len(bankers) > 0, len(boxes) > 0)
	var out []Chore
	for _, t := range takes {
		if t.From == "" {
			out = append(out, Chore{Kind: ErrandBank, Do: fmt.Sprintf("take %d %s", t.Count, name), Prep: "from your bank at", Why: "your bank holds it, so there is no need to buy it", Spots: bankers})
			continue
		}
		why := t.From + " holds it, so ask them to mail it before you buy"
		if t.Stale {
			why += " (as of their last login)"
		}
		if t.NoBank {
			why += "; their bank was not seen"
		}
		out = append(out, Chore{Kind: ErrandMail, Do: fmt.Sprintf("%s can mail you %d %s; collect it", t.From, t.Count, name), Prep: "at", Why: why, Spots: boxes})
	}
	return out, rest
}

// isPickup is true for an errand that collects items from the bank or the mailbox, not one that sends or stores them.
func isPickup(e Errand) bool {
	return e.Kind == ErrandBank && strings.HasPrefix(e.Label, "take ") || e.Kind == ErrandMail && strings.Contains(e.Label, " can mail you ")
}

// hasPickup is true when the plan already collects the item.
func hasPickup(p *Plan, item string) bool {
	for _, e := range p.Errands {
		if isPickup(e) && strings.Contains(e.Label, item) {
			return true
		}
	}
	return false
}

// outgoingMail is what the other characters' plans take from c: the matching mail lines of their pickup errands.
func outgoingMail(c character.Character, chars []character.Character, qx *questie.Index, nameOf func(id int64) string, ds store.DataStore) []Mailing {
	s := newSourcer(qx)
	var out []Mailing
	for _, o := range chars {
		if !canMail(c, o) {
			continue
		}
		for _, sh := range append(shortList(o, nameOf), pathShorts(o, ds)...) {
			takes, _ := s.sources(o, chars, sh.ID, sh.Item, sh.Need, true, true)
			for _, t := range takes {
				if t.From == charName(c) {
					out = append(out, Mailing{Item: sh.Item, ID: sh.ID, Count: t.Count, To: charName(o)})
				}
			}
		}
	}
	return out
}

// pathShorts is what the profession path's next step of o lacks, the part of its plan the route's profession lines buy.
func pathShorts(o character.Character, ds store.DataStore) []Short {
	if ds == nil || len(o.Professions) == 0 {
		return nil
	}
	var out []Short
	for _, h := range professions.RouteHints(o, ds) {
		if h.Next == nil {
			continue
		}
		for _, n := range h.Next.Needs {
			if n.Missing > 0 {
				out = append(out, Short{Item: n.Item, ID: n.ID, Need: n.Missing})
			}
		}
	}
	return out
}

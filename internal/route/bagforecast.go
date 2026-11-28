package route

import (
	"fmt"
	"strings"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/inventory"
	"github.com/WoW-Companion/advisor/internal/questie"
)

// The bag forecast's allowances, in bag slots. They are rough and lean high on purpose: an early sell stop costs
// less than full bags in the middle of a kill zone. They are never shown to the player as facts.
const (
	// lootPerKillQuest is the stacks of loot a quest with kill objectives adds: a grey item, a cloth or two. Mobs of
	// one zone drop the same items, so it is counted once per quest, not once per kill.
	lootPerKillQuest = 2
	// rewardPerTurnIn is the stack a turn-in's reward item (or the item that starts the next quest) takes.
	rewardPerTurnIn = 1
	// gatherSlots, craftSlots and buySlots are what a gathering, crafting or buying line adds to the bags.
	gatherSlots = 2
	craftSlots  = 1
	buySlots    = 1
	// maxClearErrands is how many clearing errands the forecast adds to one plan.
	maxClearErrands = 3
)

// bagFacts is what addChores learned about the bags, kept on the plan for the forecast that runs after the
// profession lines are in.
type bagFacts struct {
	junk     int // slots selling the safe junk frees
	mail     int // slots mailing the guarded mail list frees
	guard    func(id int64, name string) bool
	mailSpot []Spot
	shops    []Spot
}

// bagDeltas is the change in used bag slots at each step of the plan, walking the steps in order.
func bagDeltas(p *Plan, c character.Character, qx *questie.Index) []int {
	have := map[int]bool{}
	for _, b := range c.Bags {
		have[b.ItemID] = true
	}
	at := map[int]int{} // step number -> index
	for i, s := range p.Steps {
		at[s.N] = i
	}
	d := make([]int, len(p.Steps))
	seen := map[int]bool{}
	for i, s := range p.Steps {
		ids := s.QuestIDs
		if len(ids) == 0 && s.QuestID != 0 {
			ids = []int{s.QuestID}
		}
		for _, id := range ids {
			q := qx.Quests[id]
			if q == nil {
				continue
			}
			switch s.Kind {
			case KindObjective:
				if seen[id] {
					continue
				}
				seen[id] = true
				for _, it := range q.Items {
					if !have[it] {
						have[it] = true
						d[i]++
					}
				}
				if len(q.Creatures)+len(q.Credits) > 0 {
					d[i] += lootPerKillQuest
				}
			case KindTurnIn:
				for _, it := range q.Items {
					if have[it] {
						delete(have, it)
						d[i]--
					}
				}
				d[i] += rewardPerTurnIn
			}
		}
	}
	for _, e := range p.Errands {
		i, ok := at[e.Attach]
		if !ok || e.Attach == 0 {
			continue
		}
		switch {
		case e.Kind == ErrandProfession && strings.HasPrefix(e.Label, "Gather"):
			d[i] += gatherSlots
		case e.Kind == ErrandProfession && strings.HasPrefix(e.Label, "Craft"):
			d[i] += craftSlots
		case e.Kind == ErrandRestock:
			d[i] += buySlots
		}
	}
	return d
}

// firstFull is the first step index where the bags would hold more than slots, after the slots the clearing
// errands free at their steps.
func firstFull(d []int, frees map[int]int, used, slots int) (int, bool) {
	for i := range d {
		used += d[i] - frees[i]
		if used > slots {
			return i, true
		}
	}
	return 0, false
}

// serviceStop is the stop to clear the bags at: the last one at or before step index k with one of the spots
// within BundleReach, else the first after it (late is then true).
func serviceStop(steps []Stop, spots []Spot, k int) (idx int, sp Spot, late, ok bool) {
	near := func(s Stop) (Spot, bool) {
		best, found := Spot{}, false
		bestD := BundleReach
		if s.X < 0 || s.Y < 0 {
			return best, false
		}
		for _, c := range spots {
			if d := dist(s.X, s.Y, c.X, c.Y); c.Map == s.Map && d <= bestD {
				best, bestD, found = c, d, true
			}
		}
		return best, found
	}
	for i := k; i >= 0; i-- {
		if sp, ok := near(steps[i]); ok {
			return i, sp, false, true
		}
	}
	for i := k + 1; i < len(steps); i++ {
		if sp, ok := near(steps[i]); ok {
			return i, sp, true, true
		}
	}
	return 0, Spot{}, false, false
}

// bankSlots counts the bag stacks the bank-keep rules send to the bank and no quest needs.
func bankSlots(c character.Character, qx *questie.Index, opt Options, guard func(id int64, name string) bool) int {
	n := 0
	for _, e := range inventory.ReviewWith(c, opt.Data, qx).Groups[inventory.Bank] {
		if e.Where == "bags" && !guard(e.ItemID, e.Name) {
			n++
		}
	}
	return n
}

// addBagForecast walks the plan's steps, forecasts the bag slots in use, and puts a clearing errand (sell junk,
// mail, bank) at the last stop with the service before the bags would fill. It adds nothing when the bag size or
// the bags are not captured, or the forecast stays within the bags.
func addBagForecast(p *Plan, c character.Character, src source, opt Options) {
	qs, ok := src.(*questieSource)
	f := p.bags
	if !ok || f == nil || opt.BagSlots <= 0 || len(p.Steps) == 0 || (!c.BagsCaptured && len(c.Bags) == 0) {
		return
	}
	qx := qs.qx
	d := bagDeltas(p, c, qx)
	at := map[int]int{}
	for i, s := range p.Steps {
		at[s.N] = i
	}
	frees := map[int]int{}
	done := map[string]bool{}
	for _, e := range p.Errands {
		if i, ok := at[e.Attach]; ok && e.Attach > 0 {
			switch e.Kind {
			case ErrandSellJunk:
				frees[i] += f.junk
				done[ErrandSellJunk] = true
			case ErrandMail:
				done[ErrandMail] = true
			}
		}
	}
	if done[ErrandMail] { // mail errands are placed already; they free the list once, at the latest
		last := -1
		for _, e := range p.Errands {
			if i, ok := at[e.Attach]; ok && e.Attach > 0 && e.Kind == ErrandMail {
				last = max(last, i)
			}
		}
		frees[last] += f.mail
	}
	maps := map[int]bool{}
	for _, s := range p.Steps {
		maps[s.Map] = true
	}
	bank := -1 // slots the bank takes, counted when first needed
	for round := 0; round < maxClearErrands; round++ {
		k, full := firstFull(d, frees, len(c.Bags), opt.BagSlots)
		if !full {
			return
		}
		if done[ErrandSellJunk] { // a floor errand placed after the fill point moves up when a stop allows
			if j := sellErrand(p, at); j >= 0 && at[p.Errands[j].Attach] > k {
				if _, _, _, ok := serviceStop(p.Steps, f.shops, k); ok {
					frees[at[p.Errands[j].Attach]] -= f.junk
					p.Errands = append(p.Errands[:j], p.Errands[j+1:]...)
					done[ErrandSellJunk] = false
				}
			}
		}
		type option struct {
			kind, do, prep, verb string
			n                    int
			spots                []Spot
		}
		var opts []option
		if !done[ErrandSellJunk] && f.junk > 0 {
			opts = append(opts, option{ErrandSellJunk, "sell junk", "to", "selling it", f.junk, f.shops})
		}
		if !done[ErrandMail] && f.mail > 0 {
			opts = append(opts, option{ErrandMail, "mail the items in the mail list", "at", "mailing them", f.mail, f.mailSpot})
		}
		if !done[ErrandBank] {
			if bank < 0 {
				bank = bankSlots(c, qx, opt, f.guard)
			}
			if bank > 0 {
				bankers := spotsWhere(qx, c, maps, false, func(e questie.Entity) bool { return e.Flags&bankerFlag != 0 })
				opts = append(opts, option{ErrandBank, fmt.Sprintf("put %d stacks in the bank", bank), "at", "banking them", bank, bankers})
			}
		}
		placed, late := false, false
		for _, o := range opts {
			i, sp, l, ok := serviceStop(p.Steps, o.spots, k)
			if !ok {
				continue
			}
			s := p.Steps[i]
			why := fmt.Sprintf("bags fill around step %d (%s); %s frees %d slots", p.Steps[k].N, p.Steps[k].Label, o.verb, o.n)
			if l {
				why = fmt.Sprintf("bags fill around step %d (%s) and no stop before it has the service; this is the first after; %s frees %d slots", p.Steps[k].N, p.Steps[k].Label, o.verb, o.n)
			}
			label := strings.TrimSpace(o.do + " " + o.prep + " " + sp.Name)
			if o.kind == ErrandMail {
				label = "mail what the advice sends to your other characters at " + sp.Name
			}
			p.Errands = append(p.Errands, Errand{Kind: o.kind, Label: label, Map: sp.Map, X: sp.X, Y: sp.Y, Why: why, Attach: s.N, NPC: sp.Name})
			frees[i] += o.n
			done[o.kind] = true
			placed, late = true, l
			break
		}
		if !placed || late {
			return
		}
	}
}

// sellErrand is the index in p.Errands of the sell-junk errand riding on a step, -1 when none does.
func sellErrand(p *Plan, at map[int]int) int {
	for j, e := range p.Errands {
		if e.Kind == ErrandSellJunk && e.Attach > 0 {
			if _, ok := at[e.Attach]; ok {
				return j
			}
		}
	}
	return -1
}

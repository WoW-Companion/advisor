package route

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/inventory"
	"github.com/WoW-Companion/advisor/internal/questie"
	"github.com/WoW-Companion/advisor/internal/vendors"
)

// "While you're here" errand kinds (Plan.lua 1.9).
const (
	ErrandRestock  = "restock"
	ErrandRepair   = "repair"
	ErrandSellJunk = "selljunk"
	ErrandMail     = "mail"
)

// Bundling limits. A map percent is about 30 yards on a zone map, so BundleReach is about 150 yards.
const (
	BundleReach     = 5.0
	RestockBatch    = 5  // crafts of each recipe that still gives skill-ups
	maxRestock      = 6  // restock errands a plan lists
	repairBelow     = 50 // percent of all durability
	repairUrgent    = 25 // percent: an item or the whole set
	junkBagsPercent = 80 // percent of bag slots in use
	flagGeneralShop = 0x80
)

// Spot is a place an errand can be done: an NPC or a mailbox.
type Spot struct {
	Name string
	Map  int
	X, Y float64
}

// Chore is an errand the capture shows, before it has a place in the route. Label is the lower-case verb
// phrase ("buy 5 Empty Vial from Sly Garrett" is built from Do and the spot).
type Chore struct {
	Kind   string
	Do     string // "buy 5 Empty Vial", "repair", "sell junk", "mail Linen Cloth to Bob"
	Prep   string // "from", "at", "to"
	Why    string
	Urgent bool
	Spots  []Spot
}

// ErrandGaps says what the capture lacks for the "while you're here" errands, for the doctor.
func ErrandGaps(c character.Character) []string {
	var out []string
	if len(c.Equipment) > 0 {
		seen := false
		for _, e := range c.Equipment {
			seen = seen || e.Durability != nil && e.DurabilityMax != nil
		}
		if !seen {
			out = append(out, "durability is not captured, so repair errands are skipped")
		}
	}
	return append(out, "bag size is not captured, so sell-junk errands need Options.BagSlots")
}

func dist(ax, ay, bx, by float64) float64 { return math.Hypot(ax-bx, ay-by) }

// bundleChores places each chore: attached to the nearest route stop within BundleReach of one of its spots, else (only
// when urgent) as its own errand step at the spot nearest a stop on the route.
func bundleChores(p *Plan, chores []Chore) {
	for _, ch := range chores {
		bestD, bestN, bestSpot := math.MaxFloat64, 0, -1
		for i, sp := range ch.Spots {
			for _, s := range p.Steps {
				if s.Map != sp.Map || s.X < 0 || s.Y < 0 {
					continue
				}
				if d := dist(s.X, s.Y, sp.X, sp.Y); d < bestD {
					bestD, bestN, bestSpot = d, s.N, i
				}
			}
		}
		text := func(sp Spot) string { return strings.TrimSpace(ch.Do + " " + ch.Prep + " " + sp.Name) }
		switch {
		case bestSpot >= 0 && bestD <= BundleReach:
			sp := ch.Spots[bestSpot]
			p.Errands = append(p.Errands, Errand{Kind: ch.Kind, Label: text(sp), Map: sp.Map, X: sp.X, Y: sp.Y, Why: ch.Why, Attach: bestN, NPC: sp.Name})
		case ch.Urgent && len(ch.Spots) > 0:
			sp := ch.Spots[max(bestSpot, 0)]
			label := strings.ToUpper(text(sp)[:1]) + text(sp)[1:]
			n := len(p.Steps) + 1
			p.Steps = append(p.Steps, Stop{N: n, Kind: KindErrand, Label: label, Map: sp.Map, X: sp.X, Y: sp.Y, Place: p.Zone, Errand: ch.Kind})
			p.Errands = append(p.Errands, Errand{N: n, Kind: ch.Kind, Label: label, Map: sp.Map, X: sp.X, Y: sp.Y, Why: ch.Why, NPC: sp.Name})
		}
	}
}

// durability is the character's worst item and overall durability, in percent; ok is false when none is captured.
func durability(c character.Character) (worst, overall int, ok bool) {
	worst = 100
	var have, max int
	for _, e := range c.Equipment {
		if e.Durability == nil || e.DurabilityMax == nil || *e.DurabilityMax <= 0 {
			continue
		}
		ok = true
		have += *e.Durability
		max += *e.DurabilityMax
		worst = min(worst, *e.Durability*100 / *e.DurabilityMax)
	}
	if !ok {
		return 0, 0, false
	}
	return worst, have * 100 / max, true
}

func repairChore(c character.Character, spots []Spot) (Chore, bool) {
	worst, overall, ok := durability(c)
	if !ok || len(spots) == 0 || overall >= repairBelow && worst >= repairUrgent {
		return Chore{}, false
	}
	why := fmt.Sprintf("durability is %d%%", overall)
	if worst < repairUrgent {
		why = fmt.Sprintf("an item is at %d%% durability", worst)
	}
	return Chore{Kind: ErrandRepair, Do: "repair", Prep: "at", Why: why, Urgent: worst < repairUrgent || overall < repairUrgent, Spots: spots}, true
}

// junkChore is the sell-junk errand: bags at 80% or more, with something safe to sell.
func junkChore(used, slots, items int, spots []Spot) (Chore, bool) {
	if slots <= 0 || items == 0 || len(spots) == 0 || used*100 < slots*junkBagsPercent {
		return Chore{}, false
	}
	return Chore{Kind: ErrandSellJunk, Do: "sell junk", Prep: "to", Why: fmt.Sprintf("bags are %d/%d full, %d stacks are junk", used, slots, items), Urgent: used >= slots, Spots: spots}, true
}

// Mailing is a stack the advice sends to another character.
type Mailing struct {
	Item  string
	ID    int64
	Count int
	To    string
}

// mailChores is one mail errand per addressee, guarded: an item a quest in the log needs is never mailed.
func mailChores(items []Mailing, protected func(id int64, name string) bool, spots []Spot) []Chore {
	by := map[string][]string{}
	for _, m := range items {
		if m.To == "" || protected != nil && protected(m.ID, m.Item) {
			continue
		}
		by[m.To] = append(by[m.To], fmt.Sprintf("%d %s", m.Count, m.Item))
	}
	to := make([]string, 0, len(by))
	for t := range by {
		to = append(to, t)
	}
	sort.Strings(to)
	var out []Chore
	for _, t := range to {
		l := by[t]
		out = append(out, Chore{Kind: ErrandMail, Do: "mail " + strings.Join(l, ", ") + " to " + t, Prep: "at", Why: "the advice sends these to " + t, Spots: spots})
	}
	return out
}

// Short is a reagent the player is short of for a batch, and who sells it.
type Short struct {
	Item  string
	Need  int
	Spots []Spot
}

func restockChores(short []Short) []Chore {
	var out []Chore
	for _, s := range short {
		if s.Need > 0 && len(s.Spots) > 0 && len(out) < maxRestock {
			out = append(out, Chore{Kind: ErrandRestock, Do: fmt.Sprintf("buy %d %s", s.Need, s.Item), Prep: "from", Why: "short of reagents for a batch of " + fmt.Sprint(RestockBatch), Spots: s.Spots})
		}
	}
	return out
}

// shortOf is the reagents the character's recipes that still give skill-ups need for RestockBatch crafts, less what the bags hold.
func shortOf(c character.Character, nameOf func(id int64) string, spotsFor func(id int64, name string) []Spot) []Short {
	need := map[int64]int{}
	names := map[int64]string{}
	var order []int64
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
	have := map[int64]int{}
	for _, b := range c.Bags {
		have[int64(b.ItemID)] += b.Count
		if names[int64(b.ItemID)] == "" {
			names[int64(b.ItemID)] = b.Name
		}
	}
	var out []Short
	for _, id := range order {
		if need[id] <= have[id] {
			continue
		}
		if names[id] == "" && nameOf != nil {
			names[id] = nameOf(id)
		}
		if names[id] == "" {
			continue
		}
		out = append(out, Short{Item: names[id], Need: need[id] - have[id], Spots: spotsFor(id, names[id])})
	}
	return out
}

// spotsWhere lists the places of the NPCs and objects that pass keep, on the maps the route's steps are on.
func spotsWhere(qx *questie.Index, c character.Character, maps map[int]bool, objects bool, keep func(questie.Entity) bool) []Spot {
	if qx == nil {
		return nil
	}
	side := playerSide(c)
	src := qx.Npcs
	if objects {
		src = qx.Objects
	}
	ids := make([]int, 0, len(src))
	for id := range src {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	var out []Spot
	for _, id := range ids {
		e := src[id]
		if !keep(e) || e.Faction != "" && !questie.FactionAdmits(e.Faction, side) {
			continue
		}
		for _, sp := range e.Spawns {
			if a := qx.Areas[sp.Area]; maps[a.UIMap] {
				out = append(out, Spot{Name: e.Name, Map: a.UIMap, X: sp.X, Y: sp.Y})
			}
		}
	}
	return out
}

// vendorSpots is where the reagent is sold on the route's maps: vendors the player saw, then the dataset's.
func vendorSpots(c character.Character, qx *questie.Index, opt Options, maps map[int]bool, id int64, name string) []Spot {
	var out []Spot
	if c.Seen != nil {
		for _, v := range c.Seen.Vendors {
			for _, it := range v.Items {
				if it.ItemID == id && !it.Ext && maps[v.Map] {
					out = append(out, Spot{Name: v.Name, Map: v.Map, X: v.X, Y: v.Y})
				}
			}
		}
	}
	if len(out) > 0 || opt.Data == nil {
		return out
	}
	zones := map[string]int{}
	if qx != nil {
		for _, a := range qx.Areas {
			if maps[a.UIMap] && a.Zone != "" {
				zones[strings.ToLower(a.Zone)] = a.UIMap
			}
		}
	}
	for _, f := range vendors.Search(opt.Data, qx, vendors.Query{Text: name, Faction: c.Faction}).Found {
		if m, ok := zones[strings.ToLower(f.Zone)]; ok && f.HasXY && slices.Contains(f.Sells, name) {
			out = append(out, Spot{Name: f.Name, Map: m, X: f.X, Y: f.Y})
		}
	}
	return out
}

// reviewLines are the character's own sell and mail lines from the cross-character review.
func reviewLines(c character.Character, qx *questie.Index, opt Options) (junk []string, mail []Mailing) {
	chars := slices.Clone(opt.Others)
	if !slices.ContainsFunc(chars, func(o character.Character) bool { return o.Key == c.Key && o.Name == c.Name }) {
		chars = append(chars, c)
	}
	me := c.Name
	if me == "" {
		me = c.Key
	}
	for _, l := range inventory.ReviewAllWith(chars, opt.Data, qx).Lines {
		if l.Character != me || l.Where != "bags" {
			continue
		}
		switch {
		case l.Action == inventory.Sell && !l.AH:
			junk = append(junk, l.Item)
		case l.Action == inventory.Mail:
			mail = append(mail, Mailing{Item: l.Item, ID: l.ItemID, Count: l.Count, To: l.To})
		}
	}
	return junk, mail
}

// addChores adds the "while you're here" errands the capture shows to the plan's stops.
func addChores(p *Plan, c character.Character, src source, opt Options) {
	qs, ok := src.(*questieSource)
	if !ok || len(p.Steps) == 0 {
		return
	}
	qx := qs.qx
	maps := map[int]bool{}
	for _, s := range p.Steps {
		maps[s.Map] = true
	}
	var chores []Chore
	if ch, ok := repairChore(c, spotsWhere(qx, c, maps, false, func(e questie.Entity) bool { return e.Flags&flagRepair != 0 })); ok {
		chores = append(chores, ch)
	}
	chores = append(chores, restockChores(shortOf(c, func(id int64) string { return itemName(opt, id) }, func(id int64, name string) []Spot { return vendorSpots(c, qx, opt, maps, id, name) }))...)
	if c.BagsCaptured || len(c.Bags) > 0 {
		junk, mail := reviewLines(c, qx, opt)
		needs := inventory.NeedsOf(c, qx)
		guard := func(id int64, name string) bool {
			return len(needs.For(id, name, nil)) > 0 || len(needs.Starts(id)) > 0
		}
		safe := safeJunk(junk, guard)
		shops := spotsWhere(qx, c, maps, false, func(e questie.Entity) bool { return e.Flags&flagGeneralShop != 0 })
		if ch, ok := junkChore(len(c.Bags), opt.BagSlots, len(safe), shops); ok {
			chores = append(chores, ch)
		}
		boxes := spotsWhere(qx, c, maps, true, func(e questie.Entity) bool {
			return strings.Contains(strings.ToLower(e.Name), "mailbox")
		})
		chores = append(chores, mailChores(mail, guard, boxes)...)
		p.bags = &bagFacts{junk: len(safe), guard: guard, mailSpot: boxes, shops: shops}
		for _, m := range mail {
			if m.To != "" && !guard(m.ID, m.Item) {
				p.bags.mail++
			}
		}
	}
	bundleChores(p, chores)
}

// itemName is the dataset's name for an item, "" when it holds none.
func itemName(opt Options, id int64) string {
	if opt.Data == nil {
		return ""
	}
	row, err := opt.Data.Get("items", id)
	if err != nil {
		return ""
	}
	n, _ := row["name"].(string)
	return n
}

// safeJunk drops what a quest in the log needs: such an item is never sold.
func safeJunk(junk []string, protected func(id int64, name string) bool) []string {
	var out []string
	for _, j := range junk {
		if !protected(0, j) {
			out = append(out, j)
		}
	}
	return out
}

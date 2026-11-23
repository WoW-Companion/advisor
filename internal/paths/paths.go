// Package paths says how a character can dependably get an item: a quest reward, a craft, the
// Auction House, a dungeon boss or a named creature. An item with no such way is a random drop and
// is never offered as an upgrade. The rules are in docs/upgrade-paths.md.
//
// Everything it returns is plain words: names only, never an item, quest, recipe or NPC id.
package paths

import (
	"fmt"
	"sort"
	"strings"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/gear"
	"github.com/WoW-Companion/advisor/internal/items"
	"github.com/WoW-Companion/advisor/internal/store"
	"github.com/WoW-Companion/advisor/internal/wording"
)

// Order of the ways (lower first): a quest already in the log, a craft with every material on hand,
// the Auction House, a quest not yet picked up, a dungeon boss, a craft that is missing materials,
// then a named creature.
const (
	rankLogQuest = iota
	rankReadyCraft
	rankAuction
	rankOtherQuest
	rankDungeon
	rankCraft
	rankCreature
)

// Path kinds, most dependable first. The UI picks an icon from the kind.
const (
	Quest    = "quest"
	Craft    = "craft"
	Auction  = "auction"
	Dungeon  = "dungeon"
	Creature = "creature"
)

// Chance words: a drop chance above Often is "often", from Sometimes up to it "sometimes"; under
// Sometimes a drop is not dependable.
const (
	Often     = 15.0
	Sometimes = 5.0
	// NamedMin is the chance a named creature must carry the item at.
	NamedMin = 10.0
	// NamedMax is how many creatures may carry the item for it to count as theirs.
	NamedMax = 3
	// WorldDropMany is the number of creature sources above which an item is a random world drop.
	WorldDropMany = 5
	// DungeonWindow is how far a dungeon's level range may lie from the character's level.
	DungeonWindow = 3
)

// Options carries what the capture and the plan add to the dataset.
type Options struct {
	// PlanQuests are the quest ids the plan has as steps.
	PlanQuests map[int]bool
}

// Reagent is one reagent of a craft, with what the character holds and how to get the rest.
type Reagent struct {
	Name    string `json:"name"`
	Need    int    `json:"need"`
	Have    int    `json:"have"`
	InBags  int    `json:"in_bags"`
	InBank  int    `json:"in_bank"`
	Missing int    `json:"missing"`
	How     string `json:"how,omitempty"` // how to get the missing part, in words
	ItemID  int64  `json:"-"`
}

// Path is one dependable way to get an item.
type Path struct {
	Kind     string    `json:"kind"`
	Line     string    `json:"line"` // one line, plain words
	Reagents []Reagent `json:"reagents,omitempty"`
	Quest    string    `json:"quest,omitempty"`
	QuestID  int       `json:"-"`
	Choice   bool      `json:"choice_reward,omitempty"`
	rank     int       // order among the ways: see the rank constants
}

// Result is the ways to get one item, best first.
type Result struct {
	Item   string `json:"item"`
	Slot   string `json:"slot,omitempty"`
	Paths  []Path `json:"paths"`
	Reason string `json:"reason,omitempty"` // when there is no path: why, in words
	// Shopping is the Auction House search for a slot, when the result stands for a slot's shopping
	// list rather than one item.
	Shopping *Shopping `json:"shopping,omitempty"`
	ItemID   int64     `json:"-"`
}

// Best is the first path.
func (r Result) Best() (Path, bool) {
	if len(r.Paths) == 0 {
		return Path{}, false
	}
	return r.Paths[0], true
}

// Dependable reports whether the item has at least one path.
func (r Result) Dependable() bool { return len(r.Paths) > 0 }

// Text renders the result as plain sentences.
func (r Result) Text() string {
	var b strings.Builder
	if r.Shopping != nil {
		fmt.Fprintf(&b, "%s\n", r.Shopping.Line)
		if len(r.Shopping.Examples) > 0 {
			fmt.Fprintf(&b, "Examples of base items: %s.\n", strings.Join(r.Shopping.Examples, ", "))
		}
		for i, st := range r.Shopping.Steps {
			fmt.Fprintf(&b, "%d. %s\n", i+1, st)
		}
		return strings.TrimRight(b.String(), "\n")
	}
	if !r.Dependable() {
		fmt.Fprintf(&b, "%s: %s", r.Item, r.Reason)
		return b.String()
	}
	fmt.Fprintf(&b, "How to get %s, best way first:\n", r.Item)
	for i, p := range r.Paths {
		fmt.Fprintf(&b, "%d. %s\n", i+1, p.Line)
		for _, g := range p.Reagents {
			fmt.Fprintf(&b, "   - %s: have %d of %d", g.Name, g.Have, g.Need)
			if g.How != "" {
				fmt.Fprintf(&b, " (%s)", g.How)
			}
			b.WriteString("\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func i64(v any) int64 { n, _ := v.(int64); return n }

func str(v any) string { s, _ := v.(string); return s }

func optInt(v any) *int {
	n, ok := v.(int64)
	if !ok {
		return nil
	}
	i := int(n)
	return &i
}

func kindHas(kind string, subs ...string) bool {
	k := strings.ToLower(kind)
	for _, s := range subs {
		if strings.Contains(k, s) {
			return true
		}
	}
	return false
}

// Tradeable reports whether an item can be sold between players: not bound on pickup (1) or a quest
// item (4, 5), and common quality or better.
func Tradeable(bonding, quality int) bool {
	return bonding != 1 && bonding != 4 && bonding != 5 && quality >= 1
}

// creature is one creature that carries the item.
type creature struct {
	id         int64
	name, zone string
	chance     float64
	hasChance  bool
	tier       string
	inRange    bool
}

type finder struct {
	c      character.Character
	ds     store.DataStore
	opt    Options
	level  int
	itemID int64
	item   store.Row
	loot   []store.Row
}

// For finds the dependable ways the character can get the item, best first.
func For(c character.Character, ds store.DataStore, itemID int64, opt Options) Result {
	res := Result{ItemID: itemID, Paths: []Path{}}
	if ds == nil {
		res.Reason = "there is no game data to check"
		return res
	}
	row, err := ds.Get("items", itemID)
	if err != nil {
		res.Reason = "the game data does not know this item"
		return res
	}
	res.Item = str(row["name"])
	if res.Item == "" || items.Unobtainable(res.Item) {
		res.Reason = "this item can't be obtained in game"
		return res
	}
	f := &finder{c: c, ds: ds, opt: opt, itemID: itemID, item: row}
	if c.Level != nil {
		f.level = *c.Level
	}
	f.loot, _ = ds.Filter("loot_sources", store.Range{Column: "item_id", Min: itemID, Max: itemID})

	crs := f.creatures()
	if p, ok := f.quest(); ok {
		res.Paths = append(res.Paths, p)
	}
	if p, ok := f.craft(); ok {
		res.Paths = append(res.Paths, p)
	}
	if p, ok := f.auction(crs); ok {
		res.Paths = append(res.Paths, p)
	}
	if p, ok := f.dungeon(crs); ok {
		res.Paths = append(res.Paths, p)
	}
	if p, ok := f.named(crs); ok {
		res.Paths = append(res.Paths, p)
	}
	sort.SliceStable(res.Paths, func(i, j int) bool { return res.Paths[i].rank < res.Paths[j].rank })
	if len(res.Paths) == 0 {
		res.Reason = f.why(crs)
	}
	return res
}

// why says why an item with no path is not worth chasing.
func (f *finder) why(crs []creature) string {
	bonding := int(i64(f.item["bonding"]))
	switch {
	case len(f.loot) == 0:
		return "the game data knows no dependable way to get this, so it is not worth chasing"
	case bonding == 1 && len(crs) > 0:
		return "it is a random drop from creatures (and Bind on Pickup, so not on the Auction House), not worth chasing"
	case len(crs) > 0:
		return "it is a random drop, not worth chasing"
	}
	return "there is no dependable way to get this right now, so it is not worth chasing"
}

// quest is the quest reward path: a reward the character can earn now.
func (f *finder) quest() (Path, bool) {
	type cand struct {
		id     int64
		name   string
		giver  string
		zone   string
		rank   int // 0 in the log, 1 in the plan, 2 available now
		choice bool
	}
	var best *cand
	done := map[int]bool{}
	for _, q := range f.c.CompletedQuests {
		done[q] = true
	}
	for _, l := range f.loot {
		if !kindHas(str(l["source_kind"]), "quest") {
			continue
		}
		id := i64(l["source_id"])
		if done[int(id)] {
			continue
		}
		cd := cand{id: id, rank: 2}
		row, err := f.ds.Get("quests", id)
		if err == nil {
			cd.name, cd.zone, cd.giver = str(row["name"]), str(row["zone"]), firstText(row, "giver_name", "giver")
			if lv := optInt(row["level"]); lv != nil && f.level > 0 && *lv > f.level+2 {
				continue // too far above the character to finish
			}
		}
		for _, q := range f.c.Quests {
			if int64(q.QuestID) != id {
				continue
			}
			if q.Failed != nil && *q.Failed {
				break
			}
			cd.rank = 0
			if cd.name == "" {
				cd.name = q.Title
			}
			if cd.zone == "" {
				cd.zone = q.Zone
			}
		}
		if cd.rank == 2 && f.opt.PlanQuests[int(id)] {
			cd.rank = 1
		}
		if cd.name == "" {
			continue // never name a quest by its number
		}
		if rows, err := f.ds.Filter("loot_sources", store.Range{Column: "source_id", Min: id, Max: id}); err == nil {
			n := 0
			for _, r := range rows {
				if kindHas(str(r["source_kind"]), "quest") {
					n++
				}
			}
			cd.choice = n > 1
		}
		if best == nil || cd.rank < best.rank || cd.rank == best.rank && cd.id < best.id {
			c := cd
			best = &c
		}
	}
	if best == nil {
		return Path{}, false
	}
	var from []string
	switch best.rank {
	case 0:
		from = append(from, "it's in your quest log")
	case 1:
		from = append(from, "it's a step of your plan")
	}
	if best.giver != "" {
		from = append(from, "from "+best.giver)
	}
	if best.zone != "" {
		from = append(from, "in "+best.zone)
	}
	line := "Finish " + best.name
	if best.choice {
		line += " and choose it"
	} else {
		line += " for it"
	}
	if len(from) > 0 {
		line += " (" + strings.Join(from, ", ") + ")"
	}
	rank := rankOtherQuest
	if best.rank == 0 {
		rank = rankLogQuest
	}
	return Path{Kind: Quest, Line: line, Quest: best.name, QuestID: int(best.id), Choice: best.choice, rank: rank}, true
}

func firstText(row store.Row, keys ...string) string {
	for _, k := range keys {
		if s := str(row[k]); s != "" {
			return s
		}
	}
	return ""
}

// craft is the "you can make it" path: a recipe of one of the character's professions at their
// current skill, whose every missing reagent can be gathered, bought or has a way to be had.
func (f *finder) craft() (Path, bool) {
	recs, err := f.ds.Filter("recipes", store.Range{Column: "creates_item_id", Min: f.itemID, Max: f.itemID})
	if err != nil || len(recs) == 0 {
		return Path{}, false
	}
	sort.SliceStable(recs, func(i, j int) bool { return i64(recs[i]["spell_id"]) < i64(recs[j]["spell_id"]) })
	for _, r := range recs {
		prof, skill, ok := f.profession(str(r["skill_line_name"]))
		if !ok {
			continue
		}
		need := 0
		if n := optInt(r["learn_skill"]); n != nil {
			need = *n
		}
		have := 0
		if skill.Rank != nil {
			have = *skill.Rank
		}
		if have < need {
			continue // not within the character's current skill
		}
		reagents, ok := f.reagents(i64(r["spell_id"]))
		if !ok {
			continue
		}
		line := fmt.Sprintf("Learn it from a %s trainer, then make it", prof)
		if f.c.KnownSpells[i64(r["spell_id"])] {
			line = fmt.Sprintf("You can make this (%s)", prof)
		}
		var mats []string
		missing := 0
		for _, g := range reagents {
			mats = append(mats, fmt.Sprintf("%d/%d %s", min(g.Have, g.Need), g.Need, g.Name))
			missing += g.Missing
		}
		line += "; have " + strings.Join(mats, ", ")
		rank := rankCraft
		if missing == 0 {
			rank = rankReadyCraft
		}
		return Path{Kind: Craft, Line: line, Reagents: reagents, rank: rank}, true
	}
	return Path{}, false
}

// profession finds the character's profession by recipe skill line name.
func (f *finder) profession(line string) (string, character.Skill, bool) {
	for _, p := range f.c.Professions {
		if strings.EqualFold(p.Name, line) {
			return p.Name, p, true
		}
	}
	return "", character.Skill{}, false
}

// Materials of a craft: false when a missing one has no way to be had, which makes the craft not
// dependable.
func (f *finder) reagents(spell int64) ([]Reagent, bool) {
	rows, err := f.ds.Filter("recipe_reagents", store.Range{Column: "spell_id", Min: spell, Max: spell})
	if err != nil || len(rows) == 0 {
		return nil, false
	}
	sort.SliceStable(rows, func(i, j int) bool { return i64(rows[i]["reagent_item_id"]) < i64(rows[j]["reagent_item_id"]) })
	bags, bank := map[int64]int{}, map[int64]int{}
	for _, s := range f.c.Bags {
		bags[int64(s.ItemID)] += s.Count
	}
	for _, s := range f.c.Bank {
		bank[int64(s.ItemID)] += s.Count
	}
	var out []Reagent
	for _, r := range rows {
		id := i64(r["reagent_item_id"])
		it, err := f.ds.Get("items", id)
		if err != nil || str(it["name"]) == "" {
			return nil, false
		}
		g := Reagent{ItemID: id, Name: str(it["name"]), Need: int(i64(r["count"])), InBags: bags[id], InBank: bank[id]}
		if g.Need <= 0 {
			g.Need = 1
		}
		g.Have = g.InBags + g.InBank
		if g.Missing = g.Need - g.Have; g.Missing < 0 {
			g.Missing = 0
		}
		if g.Missing > 0 {
			g.How = f.getReagent(id, it)
			if g.How == "" {
				return nil, false
			}
		}
		out = append(out, g)
	}
	return out, true
}

// gatherer is the gathering profession that yields an item, by the item's name.
func gatherer(name string) string {
	w := " " + strings.ToLower(name) + " "
	has := func(subs ...string) bool {
		for _, s := range subs {
			if strings.Contains(w, " "+s+" ") || strings.HasSuffix(strings.TrimSpace(w), " "+s) {
				return true
			}
		}
		return false
	}
	switch {
	case has("ore", "stone"):
		return "Mining"
	case has("herb", "peacebloom", "silverleaf", "earthroot", "mageroyal", "briarthorn", "bruiseweed", "stranglekelp", "kingsblood", "liferoot", "fadeleaf", "goldthorn", "sungrass", "moss"):
		return "Herbalism"
	case has("leather", "hide", "pelt", "scale"):
		return "Skinning"
	}
	return ""
}

// getReagent is how to get a missing reagent: gather it with the character's own gathering skill,
// buy it from a vendor, or buy it on the Auction House when it can be traded. "" when none applies.
func (f *finder) getReagent(id int64, it store.Row) string {
	rows, _ := f.ds.Filter("loot_sources", store.Range{Column: "item_id", Min: id, Max: id})
	name := str(it["name"])
	if g := gatherer(name); g != "" {
		if _, _, ok := f.profession(g); ok {
			how := "gather it with your " + g
			var nodes, zones []string
			for _, l := range rows {
				if !kindHas(str(l["source_kind"]), "object", "gather", "node") {
					continue
				}
				if o, err := f.ds.Get("objects", i64(l["source_id"])); err == nil {
					nodes = appendUnique(nodes, str(o["name"]))
					zones = appendUnique(zones, str(o["zone_name"]))
				}
			}
			if len(nodes) > 0 {
				how += ": " + strings.Join(cut(nodes, 2), " or ")
			}
			if len(zones) > 0 {
				how += " in " + strings.Join(cut(zones, 3), ", ")
			}
			return how
		}
	}
	for _, l := range rows {
		if kindHas(str(l["source_kind"]), "vendor") {
			return "buy it from a vendor"
		}
	}
	if Tradeable(int(i64(it["bonding"])), int(i64(it["quality"]))) {
		return "buy it on the Auction House"
	}
	return ""
}

func appendUnique(l []string, s string) []string {
	if s == "" {
		return l
	}
	for _, x := range l {
		if x == s {
			return l
		}
	}
	return append(l, s)
}

func cut(l []string, n int) []string {
	if len(l) > n {
		return l[:n]
	}
	return l
}

// AuctionTradeable reports whether an item can be bought on the Auction House as gear: tradeable
// (not bound on pickup) and uncommon quality or better.
func AuctionTradeable(bonding, quality int) bool { return Tradeable(bonding, quality) && quality >= 2 }

// worldDrop reports whether an item is a random world drop: carried by many creatures, or only by
// creatures that drop it rarely.
func worldDrop(crs []creature) bool {
	if len(crs) == 0 {
		return false
	}
	if len(crs) > WorldDropMany {
		return true
	}
	for _, cr := range crs {
		if cr.hasChance && cr.chance >= Sometimes {
			return false
		}
	}
	return true
}

// auction is the Auction House path: a tradeable item of uncommon quality or better that is a world
// drop, or Bind on Equip from any source. It never says or depends on a price.
func (f *finder) auction(crs []creature) (Path, bool) {
	bonding, quality := int(i64(f.item["bonding"])), int(i64(f.item["quality"]))
	if _, known := f.item["bonding"].(int64); !known || !AuctionTradeable(bonding, quality) {
		return Path{}, false // an unknown binding is not claimed tradeable
	}
	if worldDrop(crs) {
		return Path{Kind: Auction, Line: "Common world drop. Usually cheap on the Auction House.", rank: rankAuction}, true
	}
	if bonding == 2 && len(f.loot) > 0 {
		return Path{Kind: Auction, Line: "Bind on Equip, so it can be bought. Usually on the Auction House.", rank: rankAuction}, true
	}
	return Path{}, false
}

// creatures lists the creatures that carry the item, one per creature, best chance first.
func (f *finder) creatures() []creature {
	type acc struct{ kills, drops int64 }
	sums := map[int64]*acc{}
	tiers := map[int64]string{}
	var ids []int64
	for _, l := range f.loot {
		if str(l["source_kind"]) != "creature" {
			continue
		}
		id := i64(l["source_id"])
		a := sums[id]
		if a == nil {
			a = &acc{}
			sums[id] = a
			ids = append(ids, id)
		}
		a.kills += i64(l["kills"])
		a.drops += i64(l["drops"])
		if t := str(l["tier"]); t != "" {
			tiers[id] = t
		}
	}
	var out []creature
	for _, id := range ids {
		cr := creature{id: id, tier: tiers[id]}
		if a := sums[id]; a.kills > 0 {
			cr.chance, cr.hasChance = 100*float64(a.drops)/float64(a.kills), true
		}
		if n, err := f.ds.Get("npcs", id); err == nil {
			cr.name, cr.zone = str(n["name"]), str(n["zone_name"])
			lo, hi := optInt(n["min_level"]), optInt(n["max_level"])
			if lo == nil {
				lo = hi
			}
			if hi == nil {
				hi = lo
			}
			cr.inRange = f.level > 0 && lo != nil && *hi >= f.level-gear.SourceBelow && *lo <= f.level+gear.SourceAbove
		}
		out = append(out, cr)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].chance != out[j].chance {
			return out[i].chance > out[j].chance
		}
		return out[i].id < out[j].id
	})
	return out
}

func chanceWord(ch float64) string {
	switch {
	case ch > Often:
		return "often"
	case ch >= Sometimes:
		return "sometimes"
	}
	return ""
}

// dungeon is the boss path: a boss of a dungeon in the character's level range that drops the item
// at least sometimes.
func (f *finder) dungeon(crs []creature) (Path, bool) {
	if f.level == 0 {
		return Path{}, false
	}
	for _, cr := range crs {
		word := chanceWord(cr.chance)
		if !cr.hasChance || word == "" {
			continue
		}
		bosses, err := f.ds.Filter("instance_bosses", store.Range{Column: "creature_id", Min: cr.id, Max: cr.id})
		if err != nil {
			continue
		}
		for _, b := range bosses {
			rows, err := f.ds.Filter("instances", store.Range{Column: "map_id", Min: i64(b["map_id"]), Max: i64(b["map_id"])})
			if err != nil || len(rows) == 0 || rows[0]["instance_type"] != "dungeon" {
				continue
			}
			in := rows[0]
			lo, hi := optInt(in["min_level"]), optInt(in["max_level"])
			if lo == nil || hi == nil || *hi < f.level-DungeonWindow || *lo > f.level+DungeonWindow {
				continue
			}
			boss := str(b["name"])
			if boss == "" {
				boss = cr.name
			}
			if boss == "" || str(in["name"]) == "" {
				continue
			}
			return Path{Kind: Dungeon, Line: fmt.Sprintf("Drops %s from %s in %s (levels %d to %d)", word, boss, str(in["name"]), *lo, *hi), rank: rankDungeon}, true
		}
	}
	return Path{}, false
}

// named is the creature path: one to three specific creatures carry the item at NamedMin or more,
// their drop is verified or confirmed, and they live in the character's level range.
func (f *finder) named(crs []creature) (Path, bool) {
	var good []creature
	for _, cr := range crs {
		if cr.hasChance && cr.chance >= NamedMin && cr.name != "" && cr.inRange && (cr.tier == "verified" || cr.tier == "confirmed") {
			good = append(good, cr)
		}
	}
	if len(good) == 0 || len(good) > NamedMax || len(crs) > WorldDropMany {
		return Path{}, false
	}
	var from []string
	for _, cr := range good {
		s := cr.name
		if cr.zone != "" {
			s += " in " + cr.zone
		}
		from = append(from, s)
	}
	return Path{Kind: Creature, Line: wording.Upper("drops often from " + strings.Join(from, " or ")), rank: rankCreature}, true
}

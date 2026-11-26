// Package learn answers "who teaches this recipe?": the nearest trainer or vendor for the
// character's faction, with zone and coordinates, and the recipe's real skill gate. Trainers come
// from the dataset's trainers table, vendors from vendor_sources; a place the dataset lacks comes
// from the player's own QuestieDB. The rules are in docs/professions.md.
package learn

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/names"
	"github.com/WoW-Companion/advisor/internal/questie"
	"github.com/WoW-Companion/advisor/internal/store"
	"github.com/WoW-Companion/advisor/internal/vendors"
	"github.com/WoW-Companion/advisor/internal/wording"
)

// MaxWays is how many ways to learn a recipe an answer lists.
const MaxWays = 3

// Way kinds.
const (
	Trainer = "trainer"
	Vendor  = "vendor"
	Drop    = "drop"
	Quest   = "quest"
)

// Way is one place to learn the recipe.
type Way struct {
	Kind    string  `json:"kind" jsonschema:"trainer, vendor, drop or quest"`
	Name    string  `json:"name,omitempty"`
	Title   string  `json:"title,omitempty" jsonschema:"a Questie trainer's rank title, such as Journeyman Blacksmith"`
	Zone    string  `json:"zone,omitempty"`
	X       float64 `json:"x,omitempty"`
	Y       float64 `json:"y,omitempty"`
	HasXY   bool    `json:"has_xy,omitempty"`
	Questie bool    `json:"from_questie,omitempty" jsonschema:"the place comes from the player's own Questie database"`
	Item    string  `json:"item,omitempty" jsonschema:"the recipe item, for a vendor, drop or quest"`
	Price   string  `json:"price,omitempty"`
	Seen    bool    `json:"seen_by_you,omitempty" jsonschema:"the player saw this trainer or vendor in game: it comes before the dataset"`
	Line    string  `json:"line"`

	req   int // the skill the trainer's own window gated the offer on; 0 when none
	tier  int
	dist  float64
	ttier int // a Questie trainer's rank tier; 0 for a dataset trainer
	miss  int // 1 when the trainer's rank tier does not reach the skill wanted
	rank  int64
	id    int64
}

// Gate is the skill needed to learn the recipe.
type Gate struct {
	Skill   int    `json:"skill,omitempty" jsonschema:"the skill needed; 0 when no number is known"`
	Unknown bool   `json:"unknown,omitempty" jsonschema:"a trainer recipe whose only value is the client minimum of 1: no number is stated"`
	Line    string `json:"line"`
}

// Result is where to learn one recipe.
type Result struct {
	Recipe     string `json:"recipe"`
	Profession string `json:"profession,omitempty"`
	Ways       []Way  `json:"ways"`
	Gate       Gate   `json:"gate"`
	Text       string `json:"text"`

	// fromItem is set when the recipe is learned from a recipe item: item names it ("" when the dataset has no
	// name), so an answer with no place does not send the player to a trainer.
	fromItem bool
	item     string
	// altItem names the recipe item of a recipe a trainer also teaches ("" when none), so a trainer answer with no
	// place still says the item teaches it.
	altItem string
}

func i64(v any) int64 { n, _ := v.(int64); return n }

func str(v any) string { s, _ := v.(string); return s }

func f64(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int64:
		return float64(x), x != 0
	}
	return 0, false
}

func rows(ds store.DataStore, table, col string, id int64) []store.Row {
	r, _ := ds.Filter(table, store.Range{Column: col, Min: id, Max: id})
	return r
}

// factionOK reports whether an NPC of the given faction ("Horde", "Alliance", "H", "A", "AH",
// neutral or empty) is open to the character.
func factionOK(npc, char string) bool {
	n, c := strings.ToLower(strings.TrimSpace(npc)), strings.ToLower(strings.TrimSpace(char))
	if n == "" || c == "" || n == "neutral" || n == "both" || n == "ah" || n == "ha" {
		return true
	}
	switch n {
	case "a":
		n = "alliance"
	case "h":
		n = "horde"
	}
	return n == c
}

type finder struct {
	c     character.Character
	ds    store.DataStore
	qx    *questie.Index
	at    questie.Where
	hasAt bool
	zone  string
	cont  string
	skill int // the character's skill in the recipe's profession
}

// place fills a way's zone and coordinates from the dataset row, then from Questie for what is
// missing. It returns the faction Questie names, for a row with none.
func (f *finder) place(w *Way, npc store.Row, id int64) (continent, qfaction string) {
	w.Zone = str(npc["zone_name"])
	if x, ok := f64(npc["x"]); ok {
		if y, ok := f64(npc["y"]); ok {
			w.X, w.Y, w.HasXY = x, y, true
		}
	}
	qfaction = f.qx.NpcFaction(int(id))
	defer func() { continent = continentOf(w.Zone) }()
	if w.HasXY && w.Zone != "" {
		return
	}
	if p, ok := f.qx.NpcPlace(int(id), f.at); ok {
		if w.Zone == "" {
			w.Zone = p.Zone
			w.Questie = true
		}
		if !w.HasXY {
			w.X, w.Y, w.HasXY = p.X, p.Y, true
			w.Questie = true
		}
	}
	return
}

// rank orders ways: same zone, then same continent, then the rest; nearest first within a tier.
func (f *finder) rank(w *Way, continent string) {
	w.tier, w.dist = 2, math.MaxFloat64
	switch {
	case f.zone != "" && strings.EqualFold(w.Zone, f.zone):
		w.tier = 0
		if f.hasAt && w.HasXY {
			w.dist = math.Hypot(w.X-f.at.X, w.Y-f.at.Y)
		}
	case f.cont != "" && strings.EqualFold(continent, f.cont):
		w.tier = 1
	}
}

func sortWays(ws []Way) {
	sort.SliceStable(ws, func(i, j int) bool {
		a, b := ws[i], ws[j]
		switch {
		case a.Seen != b.Seen:
			return a.Seen
		case a.miss != b.miss:
			return a.miss < b.miss
		case a.tier != b.tier:
			return a.tier < b.tier
		case a.dist != b.dist:
			return a.dist < b.dist
		case a.HasXY != b.HasXY:
			return a.HasXY
		case a.rank != b.rank:
			return a.rank > b.rank
		case a.ttier != b.ttier:
			return a.ttier < b.ttier
		}
		return a.id < b.id
	})
}

func where(w Way) string {
	s := w.Name
	if w.Title != "" {
		s += ", " + w.Title + ","
	}
	if w.Zone != "" {
		s += " in " + w.Zone
	}
	if w.HasXY {
		s += fmt.Sprintf(" (%.0f, %.0f)", w.X, w.Y)
	}
	return s
}

// trainers lists the trainers of the profession that teach the recipe, for the character's faction.
func (f *finder) trainers(prof string, spell int64, from string, need int64) []Way {
	teach := map[int64]bool{}
	for _, r := range rows(f.ds, "trainer_spells", "spell_id", spell) {
		teach[i64(r["npc_id"])] = true
	}
	all, _ := f.ds.Filter("trainers", store.Range{Column: "profession", Min: prof, Max: prof})
	var out []Way
	for _, r := range all {
		id := i64(r["npc_id"])
		if len(teach) > 0 && !teach[id] {
			continue
		}
		w := Way{Kind: Trainer, Name: str(r["name"]), rank: i64(r["recipes"]), id: id}
		if w.Name == "" {
			continue
		}
		cont, qf := f.place(&w, r, id)
		fac := str(r["faction"])
		if fac == "" {
			fac = qf
		}
		if !factionOK(fac, f.c.Faction) {
			continue
		}
		f.rank(&w, cont)
		out = append(out, w)
	}
	if len(out) == 0 {
		out = f.questieTrainers(prof, need)
	}
	return out
}

// seenZone names the zone of a place the player saw: the dataset's own row for the NPC, else the
// zone of the character's own map; "" when neither knows (the answer then gives the coordinates only).
func (f *finder) seenZone(mapID int, npc int64) string {
	if f.ds != nil && npc != 0 {
		if n, err := f.ds.Get("npcs", npc); err == nil {
			if z := str(n["zone_name"]); z != "" {
				return z
			}
		}
	}
	return f.c.ZoneOfMap(mapID)
}

// seenTrainers lists the trainers the player opened whose window offered the recipe (by spell id,
// else by name), for the character's faction. Cost and the skill gate are the window's own.
func (f *finder) seenTrainers(recipe string, spell int64) []Way {
	var out []Way
	for _, t := range f.c.Seen.TrainersFor(f.c.Faction) {
		for _, o := range t.Offers {
			if !(spell != 0 && o.Spell == spell || strings.EqualFold(strings.TrimSpace(o.Name), strings.TrimSpace(recipe))) {
				continue
			}
			w := Way{Kind: Trainer, Name: t.Name, Zone: f.seenZone(t.Map, t.NPC), X: t.X, Y: t.Y, HasXY: t.X != 0 || t.Y != 0,
				Seen: true, id: t.NPC, rank: t.T}
			if o.Cost > 0 {
				w.Price = vendors.Money(o.Cost)
			}
			if o.Skill != "" && o.SkillLevel > 1 {
				w.req = o.SkillLevel
			}
			f.rank(&w, continentOf(w.Zone))
			if f.c.Location != nil && f.c.Location.MapID != nil && *f.c.Location.MapID == t.Map {
				w.tier = 0 // the same map
			}
			out = append(out, w)
			break
		}
	}
	return out
}

// seenVendors lists the vendors the player opened that sold the item, with the price they showed.
func (f *finder) seenVendors(item int64, name string) []Way {
	var out []Way
	vs, stock := f.c.Seen.VendorsOf(item, f.c.Faction)
	for i, v := range vs {
		w := Way{Kind: Vendor, Name: v.Name, Item: name, Zone: f.seenZone(v.Map, v.NPC), X: v.X, Y: v.Y,
			HasXY: v.X != 0 || v.Y != 0, Seen: true, id: v.NPC, rank: v.T}
		if p := stock[i].Price; p > 0 {
			w.Price = vendors.Money(p)
		}
		if stock[i].Ext {
			if w.Price != "" {
				w.Price += " and "
			}
			w.Price += "other items"
		}
		f.rank(&w, continentOf(w.Zone))
		if f.c.Location != nil && f.c.Location.MapID != nil && *f.c.Location.MapID == v.Map {
			w.tier = 0
		}
		out = append(out, w)
	}
	return out
}

// dropSeen removes the dataset's ways for NPCs the player has already seen: the sighting replaces them.
func dropSeen(ways, seen []Way) []Way {
	have := map[int64]bool{}
	for _, w := range seen {
		have[w.id] = true
	}
	var out []Way
	for _, w := range ways {
		if !have[w.id] {
			out = append(out, w)
		}
	}
	return out
}

// questieTrainers lists the profession's trainers from the player's QuestieDB, for the character's
// faction, when the dataset has none for it. A trainer whose rank tier does not reach the skill
// wanted (the recipe's gate or the character's skill, whichever is higher) sorts after those that do.
func (f *finder) questieTrainers(prof string, need int64) []Way {
	want := int(need)
	if f.skill > want {
		want = f.skill
	}
	var out []Way
	for _, t := range f.qx.ProfessionTrainers(prof) {
		if !factionOK(f.qx.NpcFaction(t.ID), f.c.Faction) {
			continue
		}
		p, ok := f.qx.NpcPlace(t.ID, f.at)
		if !ok || p.Zone == "" {
			continue
		}
		w := Way{Kind: Trainer, Name: t.Name, Title: t.Sub, Zone: p.Zone, X: p.X, Y: p.Y, HasXY: true,
			Questie: true, id: int64(t.ID), ttier: t.Tier}
		if t.Tier < want {
			w.miss = 1
		}
		f.rank(&w, continentOf(p.Zone))
		out = append(out, w)
	}
	return out
}

// vendorWays lists the vendors that sell the recipe item.
func (f *finder) vendorWays(item int64, name string) []Way {
	var out []Way
	for _, r := range rows(f.ds, "vendor_sources", "item_id", item) {
		id := i64(r["npc_id"])
		n, err := f.ds.Get("npcs", id)
		if err != nil || str(n["name"]) == "" {
			continue
		}
		w := Way{Kind: Vendor, Name: str(n["name"]), Item: name, id: id}
		if p := vendors.BasePrice(f.ds, item, i64(r["price_copper"])); p > 0 {
			w.Price = vendors.Money(p)
		}
		cont, qf := f.place(&w, n, id)
		fac := str(n["faction"])
		if fac == "" {
			fac = qf
		}
		if !factionOK(fac, f.c.Faction) {
			continue
		}
		f.rank(&w, cont)
		out = append(out, w)
	}
	return out
}

func (f *finder) lootWays(item int64, name string) []Way {
	var out []Way
	for _, l := range rows(f.ds, "loot_sources", "item_id", item) {
		k := strings.ToLower(str(l["source_kind"]))
		switch {
		case strings.Contains(k, "creature") || strings.Contains(k, "npc"):
			n, err := f.ds.Get("npcs", i64(l["source_id"]))
			if err != nil || str(n["name"]) == "" {
				continue
			}
			w := Way{Kind: Drop, Name: str(n["name"]), Item: name, id: i64(l["source_id"])}
			cont, _ := f.place(&w, n, w.id)
			f.rank(&w, cont)
			out = append(out, w)
		case strings.Contains(k, "quest"):
			q, err := f.ds.Get("quests", i64(l["source_id"]))
			if err != nil || str(q["name"]) == "" {
				continue
			}
			out = append(out, Way{Kind: Quest, Name: str(q["name"]), Item: name, id: i64(l["source_id"]), tier: 2, dist: math.MaxFloat64})
		}
	}
	return out
}

func line(w Way) string {
	q := ""
	if w.Questie {
		q = " (from your Questie database)"
	}
	if w.Seen {
		q = " (" + character.SeenByYou + ")"
	}
	switch w.Kind {
	case Trainer:
		s := "Learn it from " + where(w)
		if w.Price != "" {
			s += " for " + w.Price
		}
		if w.req > 1 {
			s += fmt.Sprintf(", needs %d skill", w.req)
		}
		return s + q
	case Vendor:
		s := "Buy the recipe from " + where(w)
		if w.Price != "" {
			s += " for " + w.Price
		}
		return s + q
	case Drop:
		return "The recipe " + w.Item + " drops from " + where(w) + q
	}
	return "The recipe " + w.Item + " is a reward of the quest " + w.Name
}

// gate is the recipe's real skill gate. A trainer's required skill comes first; a client minimum
// of 1 on a trainer recipe is no gate at all and states no number.
func (f *finder) gate(r store.Row, prof string, trainerTaught bool) Gate {
	need := f.need(r)
	if need > 1 {
		if f.estimated(r, need) {
			return Gate{Skill: int(need), Line: fmt.Sprintf("It needs about %s %d.", prof, need)}
		}
		return Gate{Skill: int(need), Line: fmt.Sprintf("It needs %s %d.", prof, need)}
	}
	if trainerTaught {
		return Gate{Unknown: true, Line: "The trainer will show whether you can learn it yet."}
	}
	return Gate{}
}

// estimated reports whether the need is the dataset's own guess: the number came from the recipe's learn_skill and
// learn_skill_source says estimate (no trainer or recipe item gave one), so it is worded as "about".
func (f *finder) estimated(r store.Row, need int64) bool {
	return str(r["learn_skill_source"]) == "estimate" && i64(r["learn_skill"]) == need
}

// need is the skill the recipe needs, or 0 when no number is known.
func (f *finder) need(r store.Row) int64 {
	spell := i64(r["spell_id"])
	var need int64
	for _, t := range rows(f.ds, "trainer_spells", "spell_id", spell) {
		if n := i64(t["required_skill"]); n > 1 && (need == 0 || n < need) {
			need = n
		}
	}
	if need == 0 {
		for _, ri := range rows(f.ds, "recipe_items", "teaches_spell_id", spell) {
			if n := i64(ri["required_skill"]); n > 1 && (need == 0 || n < need) {
				need = n
			}
		}
	}
	if need == 0 {
		if n := i64(r["learn_skill"]); n > 1 {
			need = n
		}
	}
	return need
}

// Recipe says where to learn the recipe row r (a recipes row) for the character.
func Recipe(c character.Character, ds store.DataStore, qx *questie.Index, r store.Row) Result {
	f := &finder{c: c, ds: ds, qx: qx}
	if p := c.LastKnownPosition(); p != nil {
		f.at = questie.Where{MapID: p.MapID, Zone: p.Zone, X: p.X, Y: p.Y}
		f.hasAt, f.zone = true, p.Zone
	}
	if f.zone == "" && c.Location != nil {
		f.zone = c.Location.Zone
	}
	prof, spell, from := str(r["skill_line_name"]), i64(r["spell_id"]), strings.ToLower(str(r["learned_from"]))
	res := Result{Recipe: str(r["name"]), Profession: prof, Ways: []Way{}}
	var ways []Way
	f.cont = continentOf(f.zone)
	for _, p := range c.Professions {
		if strings.EqualFold(p.Name, prof) && p.Rank != nil {
			f.skill = *p.Rank
		}
	}
	byTrainer := from != "item"
	if byTrainer {
		ways = append(ways, f.trainers(prof, spell, from, f.need(r))...)
		if seen := f.seenTrainers(str(r["name"]), spell); len(seen) > 0 {
			ways = append(dropSeen(ways, seen), seen...)
		}
	}
	if from == "item" || from == "both" {
		for _, ri := range rows(ds, "recipe_items", "teaches_spell_id", spell) {
			id := i64(ri["item_id"])
			name := ""
			if it, err := ds.Get("items", id); err == nil {
				name = str(it["name"])
			}
			if from == "item" {
				res.fromItem = true
				if res.item == "" {
					res.item = name
				}
			} else if res.altItem == "" {
				res.altItem = name
			}
			vw := f.vendorWays(id, name)
			if seen := f.seenVendors(id, name); len(seen) > 0 {
				vw = append(dropSeen(vw, seen), seen...)
			}
			ways = append(ways, vw...)
			if len(ways) == 0 || from == "item" {
				ways = append(ways, f.lootWays(id, name)...)
			}
		}
	}
	sortWays(ways)
	if len(ways) > MaxWays {
		ways = ways[:MaxWays]
	}
	for i := range ways {
		ways[i].Line = line(ways[i])
	}
	res.Ways = append(res.Ways, ways...)
	res.Gate = f.gate(r, prof, byTrainer && from != "both" || from == "both" && len(ways) > 0 && ways[0].Kind == Trainer)
	if len(ways) > 0 && ways[0].Seen && ways[0].Kind == Trainer && ways[0].req > 1 {
		res.Gate = Gate{Skill: ways[0].req, Line: fmt.Sprintf("It needs %s %d (the trainer showed it to you).", prof, ways[0].req)}
	}
	res.Text = text(res)
	return res
}

func text(r Result) string {
	var b strings.Builder
	if len(r.Ways) == 0 && r.fromItem {
		// a recipe item teaches it, and the dataset names no vendor, creature or quest for the item
		item := "a recipe item"
		if r.item != "" {
			item = "the recipe item " + r.item
		}
		fmt.Fprintf(&b, "It is learned from %s, but the game data you have names no place that sells or drops it.", item)
	} else if len(r.Ways) == 0 {
		fmt.Fprintf(&b, "Ask %s trainer in a city or starter zone.", wording.A(r.Profession))
		if r.altItem != "" {
			fmt.Fprintf(&b, " It is also taught by the recipe item %s, but the game data you have names no place that sells or drops it.", r.altItem)
		}
	} else {
		b.WriteString(r.Ways[0].Line + ".")
		for _, w := range r.Ways[1:] {
			b.WriteString("\nAlso: " + strings.ToLower(w.Line[:1]) + w.Line[1:] + ".")
		}
	}
	if r.Gate.Line != "" {
		b.WriteString("\n" + r.Gate.Line)
	}
	return b.String()
}

// isRidingQuery is true for a question about the riding skill ("riding", "apprentice riding", "riding skill"): not the
// Enchant Gloves - Riding Skill recipe, which names the enchant.
func isRidingQuery(q string) bool {
	words := strings.Fields(names.Normalize(q))
	riding := false
	for _, w := range words {
		switch w {
		case "enchant", "formula":
			return false
		case "riding":
			riding = true
		}
	}
	return riding
}

// Find resolves a recipe or the item it makes by name and says where to learn it. A character's
// own profession wins when several recipes share the name.
func Find(c character.Character, ds store.DataStore, qx *questie.Index, query string) (Result, error) {
	q := strings.TrimSpace(query)
	for _, p := range []string{"recipe: ", "plans: ", "pattern: ", "formula: ", "schematic: ", "manual: "} {
		if strings.HasPrefix(strings.ToLower(q), p) {
			q = q[len(p):]
		}
	}
	if q == "" || ds == nil {
		return Result{}, fmt.Errorf("name the recipe or the item it makes")
	}
	if isRidingQuery(q) {
		return Result{Recipe: q, Ways: []Way{}, Text: "Riding is a skill from a riding trainer, not a recipe, and the game data you have lists no riding trainer, level or cost: " +
			"it can't say where to learn it. Do not name a trainer, a price or a level."}, nil
	}
	var cands []store.Row
	if rs, _ := ds.SearchName("recipes", q, 20); len(rs) > 0 {
		cands = rs
	} else if its, _ := ds.SearchName("items", q, 5); len(its) > 0 {
		for _, it := range its {
			cands = append(cands, rows(ds, "recipes", "creates_item_id", i64(it["id"]))...)
		}
	}
	if len(cands) == 0 {
		return Result{}, fmt.Errorf("no recipe found for %q", query)
	}
	has := map[string]bool{}
	for _, p := range c.Professions {
		has[strings.ToLower(p.Name)] = true
	}
	score := func(r store.Row) int {
		s := 0
		if strings.EqualFold(str(r["name"]), q) {
			s += 2
		}
		if has[strings.ToLower(str(r["skill_line_name"]))] {
			s++
		}
		return s
	}
	best := cands[0]
	for _, r := range cands[1:] {
		if score(r) > score(best) {
			best = r
		}
	}
	return Recipe(c, ds, qx, best), nil
}

// GateCounts counts, per profession, the trainer recipes whose only skill value is the client's
// minimum of 1: no number can be stated for them.
func GateCounts(ds store.DataStore) map[string]int {
	out := map[string]int{}
	all, _ := ds.Filter("recipes", store.Range{Column: "spell_id"})
	f := &finder{ds: ds}
	for _, r := range all {
		if strings.ToLower(str(r["learned_from"])) != "trainer" {
			continue
		}
		if g := f.gate(r, "", true); g.Unknown {
			out[str(r["skill_line_name"])]++
		}
	}
	return out
}

// CountsText writes the counts one profession per line, sorted.
func CountsText(m map[string]int) string {
	var names []string
	for n := range m {
		names = append(names, n)
	}
	sort.Strings(names)
	var b strings.Builder
	for _, n := range names {
		fmt.Fprintf(&b, "%s: %d\n", n, m[n])
	}
	return b.String()
}

// FactionOK reports whether an NPC's faction ("A", "H", "Alliance", "Horde", neutral or empty) serves
// the character's faction.
func FactionOK(npc, char string) bool { return factionOK(npc, char) }

// ProfessionTrainers lists the trainers of one profession open to the character's faction, nearest
// first. It is the profession's own trainers only, never a class trainer or a quest NPC.
func ProfessionTrainers(c character.Character, ds store.DataStore, qx *questie.Index, prof string) []Way {
	f := &finder{c: c, ds: ds, qx: qx}
	if p := c.LastKnownPosition(); p != nil {
		f.at = questie.Where{MapID: p.MapID, Zone: p.Zone, X: p.X, Y: p.Y}
		f.hasAt, f.zone = true, p.Zone
	}
	if f.zone == "" && c.Location != nil {
		f.zone = c.Location.Zone
	}
	f.cont = continentOf(f.zone)
	for _, p := range c.Professions {
		if strings.EqualFold(p.Name, prof) && p.Rank != nil {
			f.skill = *p.Rank
		}
	}
	ways := f.trainers(prof, 0, "trainer", 0)
	sortWays(ways)
	if len(ways) > MaxWays {
		ways = ways[:MaxWays]
	}
	for i := range ways {
		ways[i].Line = line(ways[i])
	}
	return ways
}

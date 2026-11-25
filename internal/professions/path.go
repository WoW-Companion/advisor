package professions

import (
	"fmt"
	"sort"
	"strings"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/gather"
	"github.com/WoW-Companion/advisor/internal/questie"
	"github.com/WoW-Companion/advisor/internal/store"
)

// MaxBands is how many bands one answer shows; the rest come with the next part.
const MaxBands = 8

// Effort of getting one unit of a material, in copper-like units. Stock is free; the Auction House
// is assumed cheap for common materials.
const (
	effortGather = 20
	effortDisenc = 100
	effortAH     = 150
	effortDrop   = 3000 // a recipe item that drops
	effortQuest  = 2000 // a recipe item from a quest
	// effortUnconfirmed is added to a trainer recipe no trainer offer confirms, so a confirmed trainer
	// or a vendor recipe at a similar cost wins.
	effortUnconfirmed = 1500
)

// Need is one material of a band: what it takes, what is held, and where the rest comes from.
type Need struct {
	ID      int64
	Item    string
	Total   int
	Have    int
	Missing int
	From    string // where to get the missing part; empty when nothing is missing
	Made    string `json:"made,omitempty"` // when the missing part is smelted or crafted: how, in a line
	Make    *Make  `json:"-"`              // the same as a tree, for the benchmark
}

// Option is one recipe that carries a band.
type Option struct {
	Recipe      string
	Crafts      int
	Difficulty  string
	Learn       string  // how to learn it first; empty when known or no learning is needed
	LearnSkill  *int    // the skill the recipe is learnt at, nil when the dataset has none
	LearnFrom   string  // where LearnSkill came from: trainer, recipe_item, client or estimate
	Around      bool    // LearnSkill is an estimate: say "around skill N"
	SubSkillUps float64 // skill-ups the intermediate crafts of this option give on the way
	Needs       []Need

	// Detail for the benchmark: the recipe's spell, crafts that are orange and yellow at the band's
	// start and the way it is learnt.
	Spell        int64
	OrangeCrafts int
	YellowCrafts int
	LearnInfo    learnInfo
}

// learnInfo says how a recipe is learnt: Via is "known", "trainer" or "item"; Copper is the price
// (0 when unknown or free); ItemID is the recipe item, with Source its kind ("vendor", "quest", "drop", "auction").
type learnInfo struct {
	Via    string
	Copper int64
	ItemID int64
	Item   string
	Source string
	Where  string
	// Confidence says how sure a trainer recipe is: "confirmed" when the dataset's trainer offers (or
	// the player's own sighting) list the spell, "unconfirmed" when it only says no recipe item teaches
	// it; empty for other ways to learn, and when the dataset has no trainer offers to check against.
	Confidence string
	// QuestOK is set for a quest recipe the character can take now: the quest's level and faction fit.
	QuestOK bool
}

// Band is one stretch of skill, from From to To, with the recipe to make and an alternative.
type Band struct {
	From, To  int
	Best      Option
	Or        *Option
	Patterns  []Option // recipes learnt from drops, quests or reputation, for a player who has the pattern
	Rank      string   // the rank to train before this band, when it starts one
	RankLevel int      // the character level that rank needs
	Gap       string   // set when no recipe covers the band: the plain-words line saying so; Best is empty
}

// Path is the computed skill-up path for one profession.
type Path struct {
	Profession string
	Skill      int
	Target     int
	Tier       string
	Where      []string
	Bands      []Band // the bands of this part
	Part       int    // 1-based
	More       bool   // later parts exist
	Prep       []string
	Notes      []string
}

type source struct {
	effort int
	text   string
}

type planner struct {
	c      character.Character
	ds     store.DataStore
	notes  Notes
	prof   string
	skill  int
	stock  map[int64]int
	known  map[int64]bool
	learnt map[int64]bool
	names  map[int64]string
	srcs   map[int64]source
	// materials still to fetch over the whole path, for the prep lines
	short map[int64]bool
	// gather names the zones of herb and ore nodes and fishing schools from the player's QuestieDB; nil without one.
	gather *gather.Gatherer
	// fresh marks a synthetic character: no recipe is known, and each one says how it is learnt.
	fresh bool
	// patterns plans only recipes learnt from items that drop, come from quests or the Auction House.
	patterns bool
	// made caches how each material is best made, by item and skill.
	made   map[makeKey]*resolved
	making map[int64]bool
	// trainerOffers caches whether the dataset lists trainer offers at all: 0 unknown, 1 yes, -1 no.
	trainerOffers int
	// expanding holds the recipes being laid out, so a recipe is never expanded inside itself.
	expanding map[int64]bool
}

func (p *planner) name(id int64) string {
	if n, ok := p.names[id]; ok {
		return n
	}
	n := itemName(p.ds, id)
	if n == "" {
		n = fmt.Sprintf("item %d", id)
	}
	p.names[id] = n
	return n
}

func (p *planner) has(profession string) bool {
	_, ok := skillOf(p.c, profession)
	return ok
}

func rowsBy(ds store.DataStore, table, col string, id int64) []store.Row {
	rows, _ := ds.Filter(table, store.Range{Column: col, Min: id, Max: id})
	return rows
}

// Money writes copper as gold, silver and copper.
func Money(c int64) string {
	switch {
	case c >= 10000:
		if s := (c % 10000) / 100; s > 0 {
			return fmt.Sprintf("%dg %ds", c/10000, s)
		}
		return fmt.Sprintf("%dg", c/10000)
	case c >= 100:
		if k := c % 100; k > 0 {
			return fmt.Sprintf("%ds %dc", c/100, k)
		}
		return fmt.Sprintf("%ds", c/100)
	}
	return fmt.Sprintf("%dc", c)
}

// sourceOf is the cheapest way to get one unit of a material the character does not hold.
func (p *planner) sourceOf(id int64) source {
	if s, ok := p.srcs[id]; ok {
		return s
	}
	best := source{p.baseEffort(id), "the Auction House"}
	offer := func(s source) {
		if s.effort < best.effort {
			best = s
		}
	}
	for _, v := range rowsBy(p.ds, "vendor_sources", "item_id", id) {
		price := asI64(v["price_copper"])
		offer(source{int(max(price, 1)), "a vendor, " + Money(price) + " each"})
	}
	for _, y := range rowsBy(p.ds, "gathering_yields", "item_id", id) {
		for _, n := range rowsBy(p.ds, "gathering_nodes", "node_id", asI64(y["node_id"])) {
			prof := asText(n["profession"])
			if p.has(prof) {
				offer(source{20 + 2*int(asI64(n["skill_required"])), "gathering (" + prof + ")"})
			}
		}
	}
	if rows := rowsBy(p.ds, "fishing_catches", "item_id", id); len(rows) > 0 && p.has("Fishing") {
		offer(source{effortGather, "fishing"})
	}
	if rows := rowsBy(p.ds, "disenchant_yields", "material_item_id", id); len(rows) > 0 {
		if len(p.disenchantable(id)) > 0 {
			offer(source{effortDisenc, "disenchanting your own gear"})
		} else if t := disenchantText(rows); t != "" {
			offer(source{effortDisenc, t})
		}
	}
	if strings.HasPrefix(best.text, "the Auction House") {
		best.text += p.materialHint(id)
	}
	p.srcs[id] = best
	return best
}

// learnOf says what learning a recipe costs and how: nothing when it is known.
func (p *planner) learnCost(r Recipe, from string, skill int) (effort int, text string, ok bool) {
	effort, text, _, ok = p.learnDetail(r, from, skill)
	return
}

func (p *planner) learnDetail(r Recipe, from string, skill int) (effort int, text string, info learnInfo, ok bool) {
	if p.known[r.SpellID] || p.learnt[r.SpellID] {
		return 0, "", learnInfo{Via: "known"}, !p.patterns
	}
	var byTrainer, byItem *source
	var tInfo, iInfo learnInfo
	if from != "item" && !p.patterns {
		t := source{0, "trainer"}
		rows := rowsBy(p.ds, "trainer_spells", "spell_id", r.SpellID)
		cost := int64(-1)
		for _, row := range rows {
			if c := asI64(row["cost_copper"]); c > 0 && (cost < 0 || c < cost) {
				cost = c
			}
		}
		if cost > 0 {
			t = source{int(cost), "trainer, " + Money(cost)}
			tInfo.Copper = cost
		}
		tInfo.Via = "trainer"
		switch {
		case len(rows) > 0 || p.seenOffer(r):
			tInfo.Confidence = "confirmed"
		case p.hasTrainerOffers():
			tInfo.Confidence = "unconfirmed"
			if _, dungeon := p.dungeonOnly(r); dungeon {
				t = source{}
				tInfo.Via = ""
			} else {
				t.text = "probably from your trainer; check when you visit"
				t.effort += effortUnconfirmed
			}
		}
		if tInfo.Via != "" {
			byTrainer = &t
		}
	}
	if p.patterns && from != "item" {
		if where, ok := p.dungeonOnly(r); ok {
			s := source{effortDrop, "a pattern that drops in " + where}
			byItem, iInfo = &s, learnInfo{Via: "item", Source: "drop", Where: where}
		}
	}
	if from == "item" || from == "both" {
		for _, ri := range rowsBy(p.ds, "recipe_items", "teaches_spell_id", r.SpellID) {
			if rs := asInt(ri["required_skill"]); rs != nil && *rs > skill {
				continue
			}
			id := asI64(ri["item_id"])
			s := source{effortAH, p.name(id) + " from the Auction House"}
			info := learnInfo{Via: "item", ItemID: id, Item: p.name(id), Source: "auction", Where: "the Auction House"}
			for _, v := range rowsBy(p.ds, "vendor_sources", "item_id", id) {
				if price := asI64(v["price_copper"]); int(price) < s.effort || s.effort == effortAH {
					s = source{int(price), p.name(id) + " from a vendor, " + Money(price)}
					info.Copper, info.Source, info.Where = price, "vendor", "a vendor"
				}
			}
			if s.effort == effortAH {
				var bestQuest *source
				var qInfo learnInfo
				for _, l := range rowsBy(p.ds, "loot_sources", "item_id", id) {
					switch k := strings.ToLower(asText(l["source_kind"])); {
					case strings.Contains(k, "quest"):
						qname, usable := p.questFor(asI64(l["source_id"]))
						if qname == "" {
							qname = "a quest"
						}
						cand := source{effortQuest, p.name(id) + " from the quest " + qname}
						ci := info
						ci.Source, ci.Where, ci.QuestOK = "quest", "the quest "+qname, usable
						if bestQuest == nil || usable && !qInfo.QuestOK {
							bestQuest, qInfo = &cand, ci
						}
					case strings.Contains(k, "creature") || strings.Contains(k, "object"):
						if s.effort != effortAH || bestQuest != nil {
							continue
						}
						who := ""
						if n, err := p.ds.Get("npcs", asI64(l["source_id"])); err == nil {
							who = asText(n["name"])
						}
						if who != "" {
							s = source{effortDrop, p.name(id) + " dropped by " + who}
							info.Where = who
						} else {
							s = source{effortDrop, p.name(id) + " from a drop"}
							info.Where = "a drop"
						}
						info.Source = "drop"
					}
				}
				if bestQuest != nil {
					s, info = *bestQuest, qInfo
				}
			}
			if isDefaultSource(info) == p.patterns {
				continue // by default trainer, vendor and quest recipes the character can take; the rest only for a pattern the player has
			}
			if byItem == nil || s.effort < byItem.effort {
				c := s
				byItem = &c
				iInfo = info
			}
		}
	}
	switch {
	case byTrainer != nil && (byItem == nil || byTrainer.effort <= byItem.effort):
		return byTrainer.effort, byTrainer.text, tInfo, true
	case byItem != nil:
		return byItem.effort, byItem.text, iInfo, true
	}
	return 0, "", learnInfo{}, false
}

type scored struct {
	opt     Option
	reach   int
	effort  int
	mats    int // raw materials the band takes in all
	crafts  int
	spell   int64
	use     map[int64]int // stock consumed
	from    int           // the skill the band starts at
	learned int64
}

// price a recipe over one band from skill to target.
func (p *planner) price(r Recipe, from string, skill, target int, left map[int64]int) (scored, bool) {
	if r.LearnSkill != nil && *r.LearnSkill > skill {
		return scored{}, false
	}
	if p.blockedRecipe(r) || p.usesCooldown(r.SpellID) {
		return scored{}, false
	}
	d := difficulty(r, skill)
	if d != Orange && d != Yellow {
		return scored{}, false
	}
	crafts, reach := craftsFor(r, skill, target)
	if crafts == 0 {
		return scored{}, false
	}
	le, ltext, linfo, ok := p.learnDetail(r, from, skill)
	if !ok {
		return scored{}, false
	}
	name := r.Name
	if name == "" {
		name = itemName(p.ds, r.CreatesItem)
	}
	if name == "" {
		name = fmt.Sprintf("spell %d", r.SpellID)
	}
	s := scored{opt: Option{Recipe: name, Crafts: crafts, Difficulty: d, Learn: ltext}, reach: reach, crafts: crafts,
		spell: r.SpellID, effort: le, use: map[int64]int{}, from: skill}
	s.opt.Spell, s.opt.LearnInfo = r.SpellID, linfo
	s.opt.LearnSkill, s.opt.LearnFrom, s.opt.Around = r.LearnSkill, r.LearnSource, r.LearnSource == "estimate"
	s.opt.OrangeCrafts, s.opt.YellowCrafts = splitCrafts(r, skill, reach)
	if !p.known[r.SpellID] {
		s.learned = r.SpellID
	}
	if !p.fresh && len(p.known) == 0 && linfo.Via == "trainer" && linfo.Confidence != "unconfirmed" {
		s.opt.Learn = "" // the capture does not list known recipes, so a trainer recipe may already be known
	}
	for _, row := range reagentsOf(p.ds, r.SpellID) {
		id := asI64(row["reagent_item_id"])
		per := 1
		if n := asInt(row["count"]); n != nil && *n > 0 {
			per = *n
		}
		total := per * crafts
		free := min(total, left[id]-s.use[id])
		s.use[id] += free
		n := Need{ID: id, Item: p.name(id), Total: total, Have: free, Missing: total - free}
		if n.Missing > 0 {
			src, ch := p.resolve(id, skill, 0)
			n.From = src.text
			s.effort += n.Missing * src.effort
			if ch != nil {
				n.Make = p.expand(ch, n.Missing, skill, 0)
				n.Made = n.Make.describe()
				s.opt.SubSkillUps += n.Make.skillUps()
			}
		}
		s.mats += n.leaves()
		s.opt.Needs = append(s.opt.Needs, n)
	}
	return s, true
}

// ComputePath is the whole skill-up path for one profession from the character's skill to target
// (the tier cap its level allows when target is 0), as parts of at most MaxBands bands; part is 1-based.
func ComputePath(c character.Character, ds store.DataStore, profession string, target, part int) Path {
	return ComputePathWith(c, ds, nil, profession, target, part)
}

// ComputePathWith is ComputePath with the player's QuestieDB (nil without one), whose herb and ore
// nodes and fishing schools name where to gather the materials still missing.
func ComputePathWith(c character.Character, ds store.DataStore, qx *questie.Index, profession string, target, part int) Path {
	var gz *gather.Gatherer
	if qx != nil || c.Seen != nil {
		gz = gather.New(qx, ds)
	}
	return computePath(c, ds, profession, target, part, false, func(p *planner) { p.gather = gz })
}

// computePath is ComputePath; whole returns every band in one part, and setup may adjust the planner
// before it plans.
func computePath(c character.Character, ds store.DataStore, profession string, target, part int, whole bool, setup func(*planner)) Path {
	sk, ok := skillOf(c, profession)
	pa := Path{Profession: profession, Part: max(part, 1)}
	if ok {
		pa.Profession = sk.Name
	}
	if !ok || sk.Rank == nil {
		pa.Notes = append(pa.Notes, "Open your "+profession+" window in the game once so I can read your skill, then ask again.")
		return pa
	}
	p := &planner{c: c, ds: ds, notes: LoadNotes(ds), prof: sk.Name, skill: *sk.Rank, stock: map[int64]int{}, known: c.KnownSpells,
		learnt: map[int64]bool{}, names: map[int64]string{}, srcs: map[int64]source{}, short: map[int64]bool{}}
	if setup != nil {
		setup(p)
	}
	bags, bank := held(c)
	for id, n := range bags {
		p.stock[id] += n
	}
	for id, n := range bank {
		p.stock[id] += n
	}
	pa.Skill = p.skill
	pa.Tier = p.notes.TierAt(p.skill).Name
	level := 0
	if c.Level != nil {
		level = *c.Level
	}
	allowed, allowedTier := p.notes.CapAt(level)
	if c.Level == nil {
		allowed, allowedTier = p.notes.Tiers[len(p.notes.Tiers)-1].SkillCap, p.notes.Tiers[len(p.notes.Tiers)-1]
	}
	if target <= 0 || target > allowed {
		if target > allowed {
			pa.Notes = append(pa.Notes, fmt.Sprintf("Level %d can train %s up to %d, so the path stops there.", level, allowedTier.Name, allowed))
		}
		target = allowed
	}
	pa.Target = target
	p.where(&pa, sk, allowed, allowedTier)
	if p.skill >= target {
		pa.Notes = append(pa.Notes, fmt.Sprintf("You are at %d, the most your level allows for now; nothing left to climb until you level.", p.skill))
		return pa
	}
	recipes, err := Recipes(ds, sk.Name)
	if err != nil {
		pa.Notes = append(pa.Notes, "I have no recipes listed for "+sk.Name+" yet.")
		return pa
	}
	var bands []Band
	skill := p.skill
	for len(bands) < 400 && skill < target {
		// A band never crosses a rank cap (75, 150, 225): the next rank is trained first.
		stop := min(target, p.notes.TierAt(skill).SkillCap)
		var all []scored
		for _, r := range recipes {
			if s, ok := p.price(r, r.LearnedFrom, skill, stop, p.stock); ok {
				all = append(all, s)
			}
		}
		if len(all) == 0 {
			// Nothing fits here: say so for the stretch no recipe covers, then go on from where one does.
			next := stop
			for s := skill + 1; s < stop; s++ {
				if p.anyFits(recipes, s, stop) {
					next = s
					break
				}
			}
			g := p.gapBand(recipes, skill, next, stop)
			if t, ok := p.rankStarting(skill); ok {
				g.Rank, g.RankLevel = t.Name, t.MinLevel
			}
			bands = append(bands, g)
			skill = next
			continue
		}
		all = withinCap(all)
		sortScored(all)
		best := all[0]
		b := Band{From: skill, To: best.reach, Best: best.opt}
		if t, ok := p.rankStarting(skill); ok {
			b.Rank, b.RankLevel = t.Name, t.MinLevel
		}
		b.Patterns = p.patternAlternatives(recipes, skill, stop, best.opt.Recipe)
		for _, alt := range all[1:] {
			if alt.opt.Recipe != best.opt.Recipe {
				o := alt.opt
				b.Or = &o
				break
			}
		}
		for id, n := range best.use {
			p.stock[id] -= n
		}
		if best.learned != 0 {
			p.learnt[best.learned] = true
		}
		for _, row := range reagentsOf(ds, best.spell) {
			p.short[asI64(row["reagent_item_id"])] = true
		}
		bands = append(bands, b)
		skill = best.reach
	}
	start := (pa.Part - 1) * MaxBands
	if start >= len(bands) {
		start = max(0, len(bands)-MaxBands)
		pa.Part = start/MaxBands + 1
	}
	end := min(len(bands), start+MaxBands)
	if whole {
		start, end, pa.Part = 0, len(bands), 1
	}
	pa.Bands, pa.More = bands[start:end], end < len(bands)
	p.prep(&pa, bands)
	return pa
}

// NextSkillUp is the one-line next step for a profession ("next skill-up: make 12 × Linen Bandage"),
// for the digest's profession entries; empty when no step is known.
func NextSkillUp(c character.Character, ds store.DataStore, profession string) string {
	if ds == nil {
		return ""
	}
	pa := ComputePath(c, ds, profession, 0, 1)
	if len(pa.Bands) == 0 {
		return ""
	}
	if pa.Bands[0].Gap != "" {
		return ""
	}
	return fmt.Sprintf("next skill-up: make %d × %s", pa.Bands[0].Best.Crafts, pa.Bands[0].Best.Recipe)
}

// sortScored orders options by cost per expected skill point (the skill-ups of intermediate
// crafts count), then fewer crafts, then spell id.
func sortScored(all []scored) {
	sort.Slice(all, func(i, j int) bool {
		a, b := all[i], all[j]
		if ai, bi := a.perPoint(), b.perPoint(); ai != bi {
			return ai < bi
		}
		if a.crafts != b.crafts {
			return a.crafts < b.crafts
		}
		return a.spell < b.spell
	})
}

// rankStarting is the rank a band starting at skill needs trained first: the one whose range opens
// exactly there (75, 150, 225), never the first rank.
func (p *planner) rankStarting(skill int) (Tier, bool) {
	for _, t := range p.notes.Tiers {
		if t.SkillMin == skill && skill > 1 {
			return t, true
		}
	}
	return Tier{}, false
}

// patternAlternatives lists up to two recipes from drops, quests or the Auction House that fit the
// band, for a player who has the pattern.
func (p *planner) patternAlternatives(recipes []Recipe, skill, stop int, best string) []Option {
	p.patterns = true
	defer func() { p.patterns = false }()
	var all []scored
	for _, r := range recipes {
		if s, ok := p.price(r, r.LearnedFrom, skill, stop, p.stock); ok && s.opt.Recipe != best {
			all = append(all, s)
		}
	}
	sortScored(all)
	var out []Option
	for _, s := range all[:min(2, len(all))] {
		out = append(out, s.opt)
	}
	return out
}

// isDefaultSource says a recipe item is a source a plan may count on: sold by a vendor, or the
// reward of a quest the character can take now.
func isDefaultSource(i learnInfo) bool {
	return i.Source == "vendor" || i.Source == "quest" && i.QuestOK
}

// questFor names a quest and says whether the character can take it: the quest's level is within
// the character's and its faction, when it has one, is theirs.
func (p *planner) questFor(id int64) (name string, usable bool) {
	row, err := p.ds.Get("quests", id)
	if err != nil {
		return "", false
	}
	name = asText(row["name"])
	req := int64(0)
	for _, col := range []string{"required_level", "min_level", "level"} {
		if v := asI64(row[col]); v > 0 {
			req = v
			break
		}
	}
	if p.c.Level != nil && req > int64(*p.c.Level) {
		return name, false
	}
	f := strings.ToLower(asText(row["faction"]))
	if (f == "horde" || f == "alliance") && p.c.Faction != "" && !strings.EqualFold(f, p.c.Faction) {
		return name, false
	}
	return name, name != ""
}

// disenchantText says where a dust, essence or shard comes from when no gear is held: the item
// levels of the green (or better) gear that disenchants into it.
func disenchantText(rows []store.Row) string {
	lo, hi := int64(-1), int64(0)
	for _, r := range rows {
		if asText(r["state"]) == "none" || asText(r["quality"]) != "uncommon" {
			continue
		}
		if l := asI64(r["min_item_level"]); lo < 0 || l < lo {
			lo = l
		}
		hi = max(hi, asI64(r["max_item_level"]))
	}
	if lo < 0 {
		return ""
	}
	return fmt.Sprintf("disenchant level-%d to %d greens or buy", lo, hi)
}

func (p *planner) anyFits(recipes []Recipe, skill, stop int) bool {
	for _, r := range recipes {
		if p.fitsAt(r, skill, stop) {
			return true
		}
	}
	return false
}

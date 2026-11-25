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
}

// Band is one stretch of skill, from From to To, with the recipe to make and an alternative.
type Band struct {
	From, To  int
	Best      Option
	Or        *Option
	Patterns  []Option // recipes learnt from drops, quests or reputation, for a player who has the pattern
	Rank      string   // the rank to train before this band, when it starts one
	RankLevel int      // the character level that rank needs
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
	best := source{effortAH, "the Auction House"}
	offer := func(s source) {
		if s.effort < best.effort {
			best = s
		}
	}
	for _, v := range rowsBy(p.ds, "vendor_sources", "item_id", id) {
		price := asI64(v["price_copper"])
		offer(source{int(price), "a vendor, " + Money(price) + " each"})
	}
	for _, y := range rowsBy(p.ds, "gathering_yields", "item_id", id) {
		for _, n := range rowsBy(p.ds, "gathering_nodes", "node_id", asI64(y["node_id"])) {
			prof := asText(n["profession"])
			if p.has(prof) {
				offer(source{effortGather, "gathering (" + prof + ")"})
			}
		}
	}
	if rows := rowsBy(p.ds, "fishing_catches", "item_id", id); len(rows) > 0 && p.has("Fishing") {
		offer(source{effortGather, "fishing"})
	}
	if len(rowsBy(p.ds, "disenchant_yields", "material_item_id", id)) > 0 && len(p.disenchantable(id)) > 0 {
		offer(source{effortDisenc, "disenchanting your own gear"})
	}
	if best.text == "the Auction House" {
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
		if rows := rowsBy(p.ds, "trainer_spells", "spell_id", r.SpellID); len(rows) > 0 {
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
		}
		tInfo.Via = "trainer"
		byTrainer = &t
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
				for _, l := range rowsBy(p.ds, "loot_sources", "item_id", id) {
					switch k := strings.ToLower(asText(l["source_kind"])); {
					case strings.Contains(k, "quest"):
						s = source{effortQuest, p.name(id) + " from a quest"}
						info.Source, info.Where = "quest", "a quest"
					case strings.Contains(k, "creature") || strings.Contains(k, "object"):
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
					if s.effort != effortAH {
						break
					}
				}
			}
			if (info.Source == "vendor") == p.patterns {
				continue // by default only trainer and vendor recipes; the rest only for a pattern the player has
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
	if !p.fresh && len(p.known) == 0 && ltext == "trainer" || strings.HasPrefix(ltext, "trainer, ") && !p.fresh && len(p.known) == 0 {
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
			pa.Notes = append(pa.Notes, fmt.Sprintf("No further recipe fits from skill %d: ask your trainer for the next ones.", skill))
			break
		}
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
	return fmt.Sprintf("next skill-up: make %d × %s", pa.Bands[0].Best.Crafts, pa.Bands[0].Best.Recipe)
}

// sortScored orders options by cost per skill point, then fewer crafts, then spell id.
func sortScored(all []scored) {
	sort.Slice(all, func(i, j int) bool {
		a, b := all[i], all[j]
		ai, bi := a.effort*(b.reach-a.from), b.effort*(a.reach-b.from) // cost per point, cross-multiplied
		if ai != bi {
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

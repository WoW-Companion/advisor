package professions

import (
	"fmt"
	"sort"
	"strings"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/store"
)

// BenchModel states how the benchmark counts skill-ups, in the output itself.
const BenchModel = "Skill-up model: an orange craft gives 1 skill point, a yellow craft 0.75, a green craft 0.25 " +
	"(green starts halfway between the dataset's yellow and grey skill), a grey craft none. Each band picks the recipe with the lowest " +
	"cost per point; ties go to fewer crafts, then the lower spell id. Nothing is held: every material is bought or gathered."

// BenchOptions describe the synthetic character of a benchmark.
type BenchOptions struct {
	Profession string
	From, To   int     // skill to start at (default 1) and to reach (0: the most the level allows)
	Level      int     // character level (default 60)
	Faction    string  // horde or alliance; empty for either
	Gather     string  // a gathering profession the character has, or empty
	Known      []int64 // recipes (spell ids) the character already knows, drops and quest patterns included
}

// BenchMaterial is one material and how many of it.
type BenchMaterial struct {
	Item  string     `json:"item"`
	ID    int64      `json:"item_id"`
	Count int        `json:"count"`
	Make  *BenchMake `json:"make,omitempty"` // set when the material is smelted or crafted from others
}

// BenchMake is how a material is made: the recipe, its learn skill and what it takes in turn.
type BenchMake struct {
	Recipe           string          `json:"recipe"`
	Profession       string          `json:"profession"`
	Verb             string          `json:"verb"`
	Crafts           int             `json:"crafts"`
	LearnSkill       *int            `json:"learn_skill,omitempty"`
	LearnSkillSource string          `json:"learn_skill_source,omitempty"`
	ExpectedSkillUps float64         `json:"expected_skill_ups,omitempty"`
	Materials        []BenchMaterial `json:"materials"`
}

// BenchBand is one stretch of skill.
type BenchBand struct {
	From             int             `json:"from"`
	To               int             `json:"to"`
	Recipe           string          `json:"recipe"`
	SpellID          int64           `json:"spell_id"`
	Crafts           int             `json:"crafts"`
	OrangeCrafts     int             `json:"orange_crafts"`
	YellowCrafts     int             `json:"yellow_crafts"`
	GreenCrafts      int             `json:"green_crafts"`
	ExpectedSkillUps float64         `json:"expected_skill_ups"`
	Learn            string          `json:"learn,omitempty"`
	LearnSkill       *int            `json:"learn_skill,omitempty"`
	LearnSkillSource string          `json:"learn_skill_source,omitempty"`
	Rank             string          `json:"rank,omitempty"`       // the rank to train before this band
	RankLevel        int             `json:"rank_level,omitempty"` // the character level that rank needs
	SubSkillUps      float64         `json:"sub_skill_ups,omitempty"`
	PatternOptions   []BenchPattern  `json:"if_you_have_the_pattern,omitempty"`
	Materials        []BenchMaterial `json:"materials"`
	Alternative      string          `json:"alternative,omitempty"`
	Gap              string          `json:"gap,omitempty"`    // set when no recipe covers the band
	Source           string          `json:"source,omitempty"` // how the recipe is learnt, in a few words
}

// BenchPattern is a recipe from a drop, quest or reputation that fits a band, for a player who has it.
type BenchPattern struct {
	Recipe           string `json:"recipe"`
	LearnSkill       *int   `json:"learn_skill,omitempty"`
	LearnSkillSource string `json:"learn_skill_source,omitempty"`
	Source           string `json:"source,omitempty"`
}

// BenchTotal is one material over the whole path with where it comes from.
type BenchTotal struct {
	Item       string `json:"item"`
	ID         int64  `json:"item_id"`
	Count      int    `json:"count"`
	Kind       string `json:"kind"` // vendor, gathering, fishing, disenchant, drop or rare
	Source     string `json:"source"`
	UnitCopper int64  `json:"unit_copper,omitempty"`
	CostCopper int64  `json:"cost_copper,omitempty"` // vendor materials only
	Rare       bool   `json:"rare"`
}

// BenchRecipeItem is a recipe that has to be bought or found before it can be learnt.
type BenchRecipeItem struct {
	Recipe string `json:"recipe"`
	Item   string `json:"item"`
	ID     int64  `json:"item_id"`
	Source string `json:"source"` // vendor, quest, drop or auction
	Where  string `json:"where"`
	Copper int64  `json:"copper,omitempty"`
}

// BenchTier is one rank of the profession with its gates and what its trainer costs along the path.
type BenchTier struct {
	Name          string `json:"name"`
	SkillMin      int    `json:"skill_min"`
	SkillCap      int    `json:"skill_cap"`
	MinLevel      int    `json:"min_level"`
	Trainer       string `json:"trainer,omitempty"`
	TrainerCopper int64  `json:"trainer_copper"`
	TrainerSpells int    `json:"trainer_recipes"`
	Reachable     bool   `json:"reachable"` // the benchmark level may train it
}

// BenchEfficiency scores a path: fewer materials, less vendor money and fewer rare materials is better.
type BenchEfficiency struct {
	TotalMaterials    int    `json:"total_materials"`
	DistinctMaterials int    `json:"distinct_materials"`
	VendorCopper      int64  `json:"vendor_copper"`
	RareMaterials     int    `json:"rare_materials"` // distinct rare materials
	RareUnits         int    `json:"rare_units"`
	GatheredUnits     int    `json:"gathered_units"`
	AuctionUnits      int    `json:"auction_units"`
	Score             int64  `json:"score"`
	Formula           string `json:"formula"`
}

// BenchFishZone is one zone's catch list, for Fishing.
type BenchFishZone struct {
	Zone    string          `json:"zone"`
	Catches []BenchFishItem `json:"catches"`
}

// BenchFishItem is one catch with its share of casts.
type BenchFishItem struct {
	Item string  `json:"item"`
	ID   int64   `json:"item_id"`
	Rate float64 `json:"rate"`
}

// Benchmark is a whole skill-up path for a synthetic character, with totals.
type Benchmark struct {
	Profession  string            `json:"profession"`
	Level       int               `json:"level"`
	Faction     string            `json:"faction,omitempty"`
	Gather      string            `json:"gather,omitempty"`
	From        int               `json:"from"`
	To          int               `json:"to"`
	Model       string            `json:"model"`
	Bands       []BenchBand       `json:"bands"`
	Materials   []BenchTotal      `json:"materials"`
	RecipeItems []BenchRecipeItem `json:"recipe_items"`
	Tiers       []BenchTier       `json:"tiers"`
	Efficiency  BenchEfficiency   `json:"efficiency"`
	FishZones   []BenchFishZone   `json:"fishing_zones,omitempty"`
	Notes       []string          `json:"notes,omitempty"`
	GuideCheck  string            `json:"guide_check"` // one line: each band's recipe and how it is learnt
}

// EfficiencyFormula is how the score is built; lower is better.
const EfficiencyFormula = "score = total materials + vendor silver + 250 per rare unit (no vendor, no gathering, fishing or disenchant source, no drop of 5% or more) + 4 per gathered unit (farming time) + 20 per unit bought at the Auction House (bars you could smelt yourself)"

// Per-unit weights of the efficiency score, on top of the 1 every unit counts.
const (
	scoreRareUnit     = 250
	scoreGatheredUnit = 4
	scoreAuctionUnit  = 20
)

const commonDrop = 5.0 // percent of kills

// NewBenchmark plans a profession for a synthetic fresh character with empty bags and bank.
func NewBenchmark(ds store.DataStore, o BenchOptions) Benchmark {
	if o.Level <= 0 {
		o.Level = 60
	}
	if o.From <= 0 {
		o.From = 1
	}
	name := titleCase(o.Profession)
	b := Benchmark{Profession: name, Level: o.Level, Faction: strings.ToLower(o.Faction), From: o.From, Model: BenchModel,
		Bands: []BenchBand{}, Materials: []BenchTotal{}, RecipeItems: []BenchRecipeItem{}, Tiers: []BenchTier{}}
	level, from := o.Level, o.From
	c := character.Character{Name: "Benchmark", Class: "None", Level: &level, Faction: titleCase(o.Faction),
		Professions: []character.Skill{{Name: name, Rank: &from}}}
	if len(o.Known) > 0 {
		c.KnownSpells = map[int64]bool{}
		for _, id := range o.Known {
			c.KnownSpells[id] = true
		}
	}
	if o.Gather != "" && !strings.EqualFold(o.Gather, o.Profession) {
		g := titleCase(o.Gather)
		one := 1
		c.Professions = append(c.Professions, character.Skill{Name: g, Rank: &one})
		b.Gather = g
	}
	pa := computePath(c, ds, name, o.To, 1, true, func(p *planner) { p.fresh = true })
	b.To = pa.Target
	for _, n := range pa.Notes {
		if !strings.EqualFold(name, "Fishing") || !strings.HasPrefix(n, "I have no recipes") {
			b.Notes = append(b.Notes, n)
		}
	}
	notes := LoadNotes(ds)
	tiers := notes.Tiers
	allowed, allowedTier := notes.CapAt(o.Level)
	if last := tiers[len(tiers)-1]; o.To <= 0 && allowed < last.SkillCap {
		b.Notes = append(b.Notes, fmt.Sprintf("Level %d can train %s up to %d, so the path stops there; %s needs level %d.", o.Level, allowedTier.Name, allowed, last.Name, last.MinLevel))
	}
	tierIdx := func(skill int) int {
		for i, t := range tiers {
			if skill < t.SkillCap {
				return i
			}
		}
		return len(tiers) - 1
	}
	trainerCost := make([]int64, len(tiers))
	trainerN := make([]int, len(tiers))
	totals := map[int64]*BenchTotal{}
	type recipeItemKey struct{ spell, item int64 }
	seenItem := map[recipeItemKey]bool{}
	p := &planner{c: c, ds: ds, notes: notes, prof: name, srcs: map[int64]source{}, names: map[int64]string{}}
	for _, band := range pa.Bands {
		opt := band.Best
		if band.Gap != "" {
			b.Bands = append(b.Bands, BenchBand{From: band.From, To: band.To, Gap: band.Gap, Rank: band.Rank, RankLevel: band.RankLevel, Materials: []BenchMaterial{}})
			continue
		}
		bb := BenchBand{From: band.From, To: band.To, Recipe: opt.Recipe, SpellID: opt.Spell, Crafts: opt.Crafts,
			OrangeCrafts: opt.OrangeCrafts, YellowCrafts: opt.YellowCrafts, GreenCrafts: opt.GreenCrafts, Learn: opt.Learn,
			Materials: []BenchMaterial{}, ExpectedSkillUps: float64(opt.OrangeCrafts) + chanceYellow*float64(opt.YellowCrafts) + chanceGreen*float64(opt.GreenCrafts)}
		if band.Or != nil {
			bb.Alternative = band.Or.Recipe
		}
		bb.LearnSkill, bb.LearnSkillSource, bb.Rank, bb.RankLevel, bb.SubSkillUps = opt.LearnSkill, opt.LearnFrom, band.Rank, band.RankLevel, opt.SubSkillUps
		for _, po := range band.Patterns {
			bb.PatternOptions = append(bb.PatternOptions, BenchPattern{Recipe: po.Recipe, LearnSkill: po.LearnSkill,
				LearnSkillSource: po.LearnFrom, Source: po.Learn})
		}
		add := func(n Need) {
			t := totals[n.ID]
			if t == nil {
				t = &BenchTotal{Item: n.Item, ID: n.ID}
				classify(p, t)
				totals[n.ID] = t
			}
			t.Count += n.Total
		}
		for _, n := range opt.Needs {
			bb.Materials = append(bb.Materials, benchMaterial(n))
			if n.Make != nil {
				// the part held in stock is a leaf of its own; the rest is counted as what it is made from
				if n.Have > 0 {
					add(Need{ID: n.ID, Item: n.Item, Total: n.Have})
				}
				for _, l := range n.Make.Leaves() {
					add(l)
				}
				continue
			}
			add(n)
		}
		bb.Source = learnWhere(opt)
		b.Bands = append(b.Bands, bb)
		li := opt.LearnInfo
		ti := tierIdx(band.From)
		switch li.Via {
		case "trainer":
			trainerCost[ti] += li.Copper
			trainerN[ti]++
		case "item":
			if k := (recipeItemKey{opt.Spell, li.ItemID}); !seenItem[k] {
				seenItem[k] = true
				b.RecipeItems = append(b.RecipeItems, BenchRecipeItem{Recipe: opt.Recipe, Item: li.Item, ID: li.ItemID,
					Source: li.Source, Where: li.Where, Copper: li.Copper})
			}
		}
	}
	e := &b.Efficiency
	for _, t := range totals {
		if t.Kind == "vendor" {
			t.CostCopper = t.UnitCopper * int64(t.Count)
			e.VendorCopper += t.CostCopper
		}
		switch {
		case t.Rare:
			e.RareMaterials++
			e.RareUnits += t.Count
		case t.Kind == "gathering":
			e.GatheredUnits += t.Count
		case t.Kind == "auction":
			e.AuctionUnits += t.Count
		}
		e.TotalMaterials += t.Count
		b.Materials = append(b.Materials, *t)
	}
	sort.Slice(b.Materials, func(i, j int) bool { return b.Materials[i].ID < b.Materials[j].ID })
	e.DistinctMaterials = len(b.Materials)
	e.Score = int64(e.TotalMaterials) + e.VendorCopper/100 + scoreRareUnit*int64(e.RareUnits) +
		scoreGatheredUnit*int64(e.GatheredUnits) + scoreAuctionUnit*int64(e.AuctionUnits)
	e.Formula = EfficiencyFormula
	trainer := p.trainer()
	for i, t := range tiers {
		if t.SkillCap <= b.From || t.SkillMin >= b.To {
			continue
		}
		b.Tiers = append(b.Tiers, BenchTier{Name: t.Name, SkillMin: t.SkillMin, SkillCap: t.SkillCap, MinLevel: t.MinLevel,
			Trainer: trainer, TrainerCopper: trainerCost[i], TrainerSpells: trainerN[i], Reachable: t.MinLevel <= o.Level && t.SkillMin < allowed})
	}
	if n := expertEarly(name, o.Faction, o.Level); n != "" && b.From < 125 && b.To > 125 {
		b.Notes = append(b.Notes, n)
	}
	if strings.EqualFold(name, "Fishing") && onlyGaps(b.Bands) {
		b.Bands = b.Bands[:0]
		b.fishing(ds, tiers)
	}
	b.GuideCheck = guideCheck(b.Bands)
	if b.GuideCheck != "" && strings.EqualFold(name, "First Aid") {
		b.GuideCheck += firstAidNote(b.Bands)
	}
	return b
}

func benchMaterial(n Need) BenchMaterial {
	m := BenchMaterial{Item: n.Item, ID: n.ID, Count: n.Total}
	if n.Make != nil {
		mk := n.Make
		bm := &BenchMake{Recipe: mk.Recipe, Profession: mk.Profession, Verb: mk.Verb, Crafts: mk.Crafts, LearnSkill: mk.LearnSkill,
			ExpectedSkillUps: mk.SkillUps, Materials: []BenchMaterial{}}
		if mk.Around {
			bm.LearnSkillSource = "estimate"
		}
		for _, sub := range mk.Needs {
			bm.Materials = append(bm.Materials, benchMaterial(sub))
		}
		m.Make = bm
	}
	return m
}

// classify says where one material comes from and whether it is rare.
func classify(p *planner, t *BenchTotal) {
	ds := p.ds
	if rows := rowsBy(ds, "vendor_sources", "item_id", t.ID); len(rows) > 0 {
		price := int64(-1)
		for _, r := range rows {
			if v := asI64(r["price_copper"]); price < 0 || v < price {
				price = v
			}
		}
		t.Kind, t.UnitCopper = "vendor", price
		t.Source = "a vendor, " + Money(price) + " each"
		return
	}
	if profs := gatherProfs(ds, t.ID); len(profs) > 0 {
		t.Kind, t.Source = "gathering", "gathering ("+strings.Join(profs, ", ")+")"
		return
	}
	if p.has("Mining") && minedBase(ds, t.ID, t.Item) {
		t.Kind, t.Source = "gathering", "gathering (Mining)"
		return
	}
	if madeByMining(ds, t.ID) {
		t.Kind, t.Source = "auction", "the Auction House, or smelt it with Mining"
		return
	}
	if rows := rowsBy(ds, "fishing_catches", "item_id", t.ID); len(rows) > 0 {
		sort.SliceStable(rows, func(i, j int) bool {
			if a, b := asFloat(rows[i]["rate"]), asFloat(rows[j]["rate"]); a != b {
				return a > b
			}
			return asText(rows[i]["zone_name"]) < asText(rows[j]["zone_name"])
		})
		var zs []string
		for _, r := range rows[:min(3, len(rows))] {
			zs = append(zs, asText(r["zone_name"]))
		}
		t.Kind, t.Source = "fishing", "fishing in "+strings.Join(zs, ", ")
		return
	}
	if rows := rowsBy(ds, "disenchant_yields", "material_item_id", t.ID); len(rows) > 0 {
		var how []string
		for _, r := range rows {
			if asText(r["state"]) == "none" {
				continue
			}
			how = append(how, fmt.Sprintf("%s %s, item level %d-%d", asText(r["quality"]), asText(r["item_class"]), asI64(r["min_item_level"]), asI64(r["max_item_level"])))
		}
		sort.Strings(how)
		if len(how) > 0 {
			t.Kind, t.Source = "disenchant", "disenchanting "+strings.Join(how[:min(3, len(how))], "; ")
			return
		}
	}
	if who, chance := bestDrop(ds, t.ID); who != "" && chance >= commonDrop {
		t.Kind, t.Source = "drop", fmt.Sprintf("%s, about %.0f%%", who, chance)
		return
	}
	if it, err := ds.Get("items", t.ID); err == nil && asI64(it["class_id"]) == 7 && asI64(it["subclass_id"]) == 5 {
		t.Kind, t.Source = "drop", "drops from humanoids, or the Auction House"
		return
	}
	t.Kind, t.Rare, t.Source = "rare", true, "no vendor, gathering or common drop listed: the Auction House"
}

func gatherProfs(ds store.DataStore, id int64) []string {
	set := map[string]bool{}
	for _, y := range rowsBy(ds, "gathering_yields", "item_id", id) {
		for _, n := range rowsBy(ds, "gathering_nodes", "node_id", asI64(y["node_id"])) {
			if p := asText(n["profession"]); p != "" {
				set[p] = true
			}
		}
	}
	var out []string
	for p := range set {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// bestDrop is the creature or object with the highest drop chance for an item.
func bestDrop(ds store.DataStore, id int64) (who string, chance float64) {
	var bestID int64
	for _, l := range rowsBy(ds, "loot_sources", "item_id", id) {
		k := asText(l["source_kind"])
		if k != "creature" && k != "object" {
			continue
		}
		kills := asI64(l["kills"])
		if kills <= 0 {
			continue
		}
		ch := 100 * float64(asI64(l["drops"])) / float64(kills)
		sid := asI64(l["source_id"])
		if ch > chance || ch == chance && sid < bestID {
			chance, bestID = ch, sid
			who = "a drop"
			if n, err := ds.Get("npcs", sid); err == nil && asText(n["name"]) != "" {
				who = asText(n["name"])
			}
		}
	}
	return
}

// fishing fills a Fishing benchmark: one band per tier with the catches it takes, and the zones.
func (b *Benchmark) fishing(ds store.DataStore, tiers []Tier) {
	b.Notes = append(b.Notes, "Fishing has no recipes: each band is catches, counted at one catch per skill point (the dataset holds no per-cast skill odds).")
	skill := b.From
	for _, t := range tiers {
		if t.SkillCap <= skill || skill >= b.To {
			continue
		}
		to := min(t.SkillCap, b.To)
		catches := to - skill
		b.Bands = append(b.Bands, BenchBand{From: skill, To: to, Recipe: "Fish (" + t.Name + ")", Crafts: catches, OrangeCrafts: catches,
			ExpectedSkillUps: float64(catches), Materials: []BenchMaterial{}})
		skill = to
	}
	rows, _ := ds.Filter("fishing_catches", store.Range{Column: "zone_name"})
	byZone := map[string]*BenchFishZone{}
	for _, r := range rows {
		z := asText(r["zone_name"])
		if byZone[z] == nil {
			byZone[z] = &BenchFishZone{Zone: z}
		}
		id := asI64(r["item_id"])
		name := itemName(ds, id)
		if name == "" {
			name = fmt.Sprintf("item %d", id)
		}
		byZone[z].Catches = append(byZone[z].Catches, BenchFishItem{Item: name, ID: id, Rate: asFloat(r["rate"])})
	}
	for _, z := range byZone {
		sort.Slice(z.Catches, func(i, j int) bool {
			if z.Catches[i].Rate != z.Catches[j].Rate {
				return z.Catches[i].Rate > z.Catches[j].Rate
			}
			return z.Catches[i].ID < z.Catches[j].ID
		})
		b.FishZones = append(b.FishZones, *z)
	}
	sort.Slice(b.FishZones, func(i, j int) bool { return b.FishZones[i].Zone < b.FishZones[j].Zone })
	if len(b.Bands) > 0 && len(b.Tiers) == 0 {
		b.Notes = append(b.Notes, "No ranks are listed for Fishing in the dataset.")
	}
}

// RenderBenchmark is the plain-text report of a benchmark.
func RenderBenchmark(b Benchmark) string {
	var w strings.Builder
	fmt.Fprintf(&w, "Profession plan: %s %d→%d, level %d", b.Profession, b.From, b.To, b.Level)
	if b.Faction != "" {
		w.WriteString(", " + b.Faction)
	}
	if b.Gather != "" {
		w.WriteString(", gathers with " + b.Gather)
	}
	w.WriteString(", empty bags and bank.\n" + b.Model + "\n")
	if len(b.Tiers) > 0 {
		w.WriteString("\nGates:\n")
		for _, t := range b.Tiers {
			fmt.Fprintf(&w, "- %s: skill %d-%d, character level %d", t.Name, t.SkillMin, t.SkillCap, t.MinLevel)
			if t.Trainer != "" {
				w.WriteString(", trainer " + t.Trainer)
			}
			fmt.Fprintf(&w, "; trainer recipes on this path: %d for %s", t.TrainerSpells, Money(t.TrainerCopper))
			if !t.Reachable {
				w.WriteString(" (not open at this level)")
			}
			w.WriteString("\n")
		}
	}
	w.WriteString("\nBands:\n")
	for i, x := range b.Bands {
		if x.Gap != "" {
			fmt.Fprintf(&w, "%d. Skill %d→%d: %s\n", i+1, x.From, x.To, x.Gap)
			continue
		}
		fmt.Fprintf(&w, "%d. Skill %d→%d: %d × %s (%d orange, %d yellow, %d green; expected skill-ups %s)", i+1, x.From, x.To, x.Crafts, x.Recipe,
			x.OrangeCrafts, x.YellowCrafts, x.GreenCrafts, trimFloat(x.ExpectedSkillUps))
		if x.Learn != "" {
			w.WriteString("; learn: " + x.Learn)
		}
		w.WriteString("\n")
		if x.Rank != "" {
			fmt.Fprintf(&w, "   First train %s at the trainer (character level %d).\n", x.Rank, x.RankLevel)
		}
		if len(x.Materials) > 0 {
			var ms []string
			for _, m := range x.Materials {
				ms = append(ms, renderBenchMaterial(m))
			}
			w.WriteString("   Materials: " + strings.Join(ms, ", ") + "\n")
		}
		if x.SubSkillUps >= 1 {
			fmt.Fprintf(&w, "   Making those materials also gives about %s skill-ups.\n", trimFloat(x.SubSkillUps))
		}
		if x.Alternative != "" {
			w.WriteString("   Alternative: " + x.Alternative + "\n")
		}
		for _, po := range x.PatternOptions {
			w.WriteString("   If you have the pattern: " + po.Recipe + " (" + learnWords(po.LearnSkill, po.LearnSkillSource) + ")\n")
		}
	}
	if len(b.Bands) > 0 {
		w.WriteString("\nCheck (recipe, learn skill and where it comes from):\n")
		for i, x := range b.Bands {
			if x.Gap != "" {
				fmt.Fprintf(&w, "%d. %d→%d: no recipe covers it\n", i+1, x.From, x.To)
				continue
			}
			fmt.Fprintf(&w, "%d. %d→%d %s: %s", i+1, x.From, x.To, x.Recipe, learnWords(x.LearnSkill, x.LearnSkillSource))
			if x.Learn != "" {
				if strings.HasPrefix(x.Learn, "probably") {
					w.WriteString("; " + x.Learn)
				} else {
					w.WriteString("; learnt from " + x.Learn)
				}
			}
			w.WriteString("\n")
		}
	}
	if len(b.FishZones) > 0 {
		w.WriteString("\nWhere to fish (share of casts):\n")
		for _, z := range b.FishZones {
			var cs []string
			for _, c := range z.Catches {
				cs = append(cs, fmt.Sprintf("%s %.0f%%", c.Item, 100*c.Rate))
			}
			fmt.Fprintf(&w, "- %s: %s\n", z.Zone, strings.Join(cs, ", "))
		}
	}
	if b.GuideCheck != "" {
		w.WriteString("\n" + b.GuideCheck + "\n")
	}
	w.WriteString("\nMaterial totals:\n")
	for _, t := range b.Materials {
		fmt.Fprintf(&w, "- %d × %s: %s", t.Count, t.Item, t.Source)
		if t.Kind == "vendor" {
			fmt.Fprintf(&w, " = %s", Money(t.CostCopper))
		}
		if t.Rare {
			w.WriteString(" [rare]")
		}
		w.WriteString("\n")
	}
	if len(b.RecipeItems) > 0 {
		w.WriteString("\nRecipes to buy or find:\n")
		for _, r := range b.RecipeItems {
			fmt.Fprintf(&w, "- %s for %s: %s", r.Item, r.Recipe, r.Source)
			if r.Where != "" {
				w.WriteString(" (" + r.Where + ")")
			}
			if r.Copper > 0 {
				w.WriteString(", " + Money(r.Copper))
			}
			w.WriteString("\n")
		}
	}
	e := b.Efficiency
	fmt.Fprintf(&w, "\nEfficiency: %d materials in all, %d distinct, vendor cost %s, %d rare (%d units), %d gathered, %d bought; score %d (lower is better).\n%s.\n",
		e.TotalMaterials, e.DistinctMaterials, Money(e.VendorCopper), e.RareMaterials, e.RareUnits, e.GatheredUnits, e.AuctionUnits, e.Score, e.Formula)
	for _, n := range b.Notes {
		w.WriteString(n + "\n")
	}
	return w.String()
}

// learnWords says at which skill a recipe is learnt and how the dataset knows: "around" for an estimate.
func learnWords(skill *int, source string) string {
	if skill == nil {
		return "learn skill unknown"
	}
	switch source {
	case "estimate":
		return fmt.Sprintf("learn around skill %d, estimated", *skill)
	case "":
		return fmt.Sprintf("learn skill %d", *skill)
	}
	return fmt.Sprintf("learn skill %d, source %s", *skill, strings.ReplaceAll(source, "_", " "))
}

func renderBenchMaterial(m BenchMaterial) string {
	s := fmt.Sprintf("%d × %s", m.Count, m.Item)
	if m.Make == nil {
		return s
	}
	var subs []string
	for _, x := range m.Make.Materials {
		subs = append(subs, renderBenchMaterial(x))
	}
	how := fmt.Sprintf("%s %d × %s, %s", m.Make.Verb, m.Make.Crafts, m.Make.Recipe, m.Make.Profession)
	if m.Make.LearnSkill != nil {
		around, skill := "", "skill"
		if m.Make.LearnSkillSource == "estimate" {
			around = "around "
		}
		if m.Make.Verb == "smelt" {
			skill = "needs Mining skill"
		}
		how += fmt.Sprintf(", %s %s%d", skill, around, *m.Make.LearnSkill)
	}
	if m.Make.ExpectedSkillUps >= 1 {
		how += ", about " + trimFloat(m.Make.ExpectedSkillUps) + " skill-ups"
	}
	return s + " (" + how + ": " + strings.Join(subs, ", ") + ")"
}

func trimFloat(f float64) string {
	if f == float64(int64(f)) {
		return fmt.Sprintf("%d", int64(f))
	}
	return fmt.Sprintf("%.1f", f)
}

// learnWhere says in a few words how the band's recipe is learnt: known, trainer, a vendor
// pattern, a quest reward, or another pattern.
func learnWhere(o Option) string {
	li := o.LearnInfo
	switch li.Via {
	case "known":
		return "known"
	case "trainer":
		switch li.Confidence {
		case "confirmed":
			return "trainer, confirmed"
		case "unconfirmed":
			return "trainer, unconfirmed: ask your trainer"
		}
		return "trainer"
	case "item":
		switch li.Source {
		case "vendor":
			return "vendor pattern " + li.Item
		case "quest":
			return "quest recipe " + li.Item + " from " + li.Where
		}
		return "pattern " + li.Item + " (" + li.Source + ")"
	}
	return "unknown"
}

// guideCheck is the one line a guide can be compared with: each band, its recipe and how it is
// learnt, with a gap named as such.
func guideCheck(bands []BenchBand) string {
	if len(bands) == 0 {
		return ""
	}
	var parts []string
	for _, x := range bands {
		if x.Gap != "" {
			parts = append(parts, fmt.Sprintf("%d-%d GAP", x.From, x.To))
			continue
		}
		parts = append(parts, fmt.Sprintf("%d-%d %s (%s)", x.From, x.To, x.Recipe, x.Source))
	}
	return "Guide check: " + strings.Join(parts, "; ") + "."
}

// firstAidNote says why First Aid lists the healing potions: the client does, and the trainer offers agree.
func firstAidNote(bands []BenchBand) string {
	for _, x := range bands {
		if x.Recipe == "Minor Healing Potion" || x.Recipe == "Lesser Healing Potion" {
			return " First Aid lists Minor and Lesser Healing Potion on WoW Forever (the client and the trainer data agree): keep them."
		}
	}
	return ""
}

// onlyGaps says no band names a recipe (or there is no band).
func onlyGaps(bands []BenchBand) bool {
	for _, x := range bands {
		if x.Gap == "" {
			return false
		}
	}
	return true
}

package route

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/professions"
	"github.com/WoW-Companion/advisor/internal/questie"
	"github.com/WoW-Companion/advisor/internal/store"
)

// maxProfessionLines is how many profession lines a plan holds: the route stays about questing first.
const maxProfessionLines = 3

// profWhyPrefix starts the reason of a restock errand that serves a profession path, so it counts as a profession line.
const profWhyPrefix = "for your "

// professionHints are the profession path's facts for the route, computed once per plan. They are empty without
// a dataset or a profession; the wording comes from the skill-up path, so the route and the profession answer agree.
func professionHints(p *Plan, c character.Character, opt Options) []professions.RouteHint {
	if !p.hintsDone {
		p.hintsDone = true
		if opt.Data != nil && len(c.Professions) > 0 {
			p.hints = professions.RouteHints(c, opt.Data)
		}
	}
	return p.hints
}

// professionLines counts the profession lines the plan already has.
func professionLines(p *Plan) int {
	n := 0
	for _, e := range p.Errands {
		if e.Kind == ErrandProfession || e.Kind == ErrandRestock && strings.HasPrefix(e.Why, profWhyPrefix) {
			n++
		}
	}
	return n
}

// rankTrainers are the trainers the dataset's profession_tiers names for a rank on the player's side: where each stands
// and who it is. The table holds a row per trainer and city, so a rank taught in fewer cities than a profession has
// trainers (Expert Alchemy in Undercity and Darnassus, not in Orgrimmar) is told apart from the lower ranks' trainers.
type rankTrainers struct {
	places, names []string
}

// rankTrainersOf reads the table's rows of the profession and rank that a trainer teaches, for the side's faction. It
// is empty when the table has none (no dataset, or no row naming a place), which leaves every trainer allowed.
func rankTrainersOf(ds store.DataStore, profession, tier, faction string) (rt rankTrainers) {
	if ds == nil || tier == "" {
		return rt
	}
	rows, _ := ds.Filter("profession_tiers", store.Range{Column: "profession", Min: profession, Max: profession})
	for _, r := range rows {
		text := func(k string) string { v, _ := r[k].(string); return v }
		if !strings.EqualFold(text("tier"), tier) || text("how") != "trainer" || text("place") == "" {
			continue
		}
		if f := text("faction"); f != "" && !strings.EqualFold(f, "both") && faction != "" && !strings.EqualFold(f, faction) {
			continue
		}
		rt.places = append(rt.places, strings.ToLower(text("place")))
		if n := text("trainer_name"); n != "" {
			rt.names = append(rt.names, strings.ToLower(n))
		}
	}
	return rt
}

// teachesIn is true when a trainer of the rank stands in the city, or when the table names none (then it cannot say).
func (rt rankTrainers) teachesIn(zone string) bool {
	if len(rt.places) == 0 {
		return true
	}
	zone = strings.ToLower(strings.TrimSpace(zone))
	for _, p := range rt.places {
		if p == zone || strings.HasPrefix(p, zone+",") {
			return true
		}
	}
	return false
}

func (rt rankTrainers) isNamed(name string) bool {
	return slices.Contains(rt.names, strings.ToLower(name))
}

// trainerIn is a profession trainer standing in the city (map m, or the zone's name): an NPC with the trainer flag
// whose subName names the profession, preferring the one the dataset names for the rank, then one that names the
// rank, then the one nearest the player. ok is false when Questie holds none there, or when the dataset says the rank
// is not taught in this city.
func (s *questieSource) trainerIn(m int, zone, side string, from *Point, profession, tier string, rt rankTrainers) (name string, tm int, x, y float64, ok bool) {
	if !rt.teachesIn(cmp.Or(zone, zoneOfMap(s.qx, m))) {
		return "", 0, 0, 0, false
	}
	stems := professions.ProfessionStems(profession)
	bestScore, bestD := -1, 0.0
	for _, id := range sortedIDs(s.qx.Npcs) {
		e := s.qx.Npcs[id]
		if e.Flags&(questie.FlagTrainer|questie.FlagProfTrainer) == 0 || len(e.Spawns) == 0 || e.Flags&questie.FlagClassTrainer != 0 {
			continue
		}
		if e.Faction != "" && !questie.FactionAdmits(e.Faction, side) {
			continue
		}
		sub := strings.ToLower(e.Sub)
		named := false
		for _, st := range stems {
			named = named || strings.Contains(sub, st)
		}
		if !named {
			continue
		}
		score := 0
		if tier != "" && strings.Contains(sub, strings.ToLower(tier)) {
			score = 1
		}
		if rt.isNamed(e.Name) {
			score = 2
		}
		for _, sp := range e.Spawns {
			a := s.qx.Areas[sp.Area]
			if a.UIMap != m && !strings.EqualFold(a.Zone, zone) {
				continue
			}
			d := 0.0
			if from != nil && from.Map == a.UIMap {
				d = math.Hypot(sp.X-from.X, sp.Y-from.Y)
			}
			if score > bestScore || score == bestScore && d < bestD {
				bestScore, bestD, name, tm, x, y, ok = score, d, e.Name, a.UIMap, sp.X, sp.Y, true
			}
		}
	}
	return name, tm, x, y, ok
}

// guideTrainRecipe is the recipe the dataset's guide step covering the character's skill needs, when a trainer teaches it
// and the character does not know it yet, with the rank whose trainer teaches it. found says the dataset holds a guide
// for the profession and the character's known spells are captured, so its answer stands over the skill-up path's: the
// route's craft line and its train line then name the same recipe.
func guideTrainRecipe(c character.Character, ds store.DataStore, profession string) (recipe, tier string, found bool) {
	if ds == nil || len(c.KnownSpells) == 0 {
		return "", "", false
	}
	skill := -1
	for _, sk := range c.Professions {
		if strings.EqualFold(sk.Name, profession) && sk.Rank != nil {
			skill = *sk.Rank
		}
	}
	rt, ok := professions.LoadRoute(ds, profession)
	if skill < 0 || !ok {
		return "", "", false
	}
	i := rt.Covering(skill)
	if i < 0 {
		return "", "", true
	}
	st := rt.Steps[i]
	if st.Gather || st.LearnedFrom != "trainer" || st.SpellID == 0 || c.KnownSpells[st.SpellID] {
		return "", "", true
	}
	return st.Recipe, professions.LoadNotes(ds).TierAt(skill).Name, true
}

// addProfessionTrainers adds a numbered profession errand for each profession that can train now, when a trainer
// for it stands in the hub. A profession with no placeable trainer adds nothing; nor does one whose rank the
// dataset says is taught in other cities only.
func addProfessionTrainers(p *Plan, c character.Character, qs *questieSource, opt Options, cur int, zone string, add func(kind, label, why string, m int, x, y float64)) {
	if qs == nil {
		return
	}
	for _, h := range professionHints(p, c, opt) {
		if professionLines(p) >= maxProfessionLines {
			return
		}
		what, tier, why := h.TrainTier, h.TrainTier, ""
		if h.TrainTier == "" {
			recipe, rank, found := guideTrainRecipe(c, opt.Data, h.Profession)
			if !found {
				recipe, rank = h.TrainRecipe, ""
			}
			if recipe != "" {
				what, tier, why = recipe, rank, "your next "+h.Profession+" step uses a recipe a trainer teaches"
			}
		} else {
			why = h.TrainTier + " is open at your level and skill"
		}
		if why == "" {
			continue
		}
		name, m, x, y, ok := qs.trainerIn(cur, zone, playerSide(c), p.Start, h.Profession, tier, rankTrainersOf(opt.Data, h.Profession, tier, c.Faction))
		if !ok {
			continue
		}
		add(ErrandProfession, fmt.Sprintf("Train %s (%s) with %s", h.Profession, what, name), why, m, x, y)
	}
}

// sortedIDs are the keys of an id-keyed map, ascending.
func sortedIDs[V any](m map[int]V) []int {
	ids := make([]int, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	return ids
}

// zoneOfMap is the zone name Questie gives a map, "" when none.
func zoneOfMap(qx *questie.Index, m int) string {
	if qx == nil {
		return ""
	}
	for _, id := range sortedIDs(qx.Areas) {
		if a := qx.Areas[id]; a.UIMap == m && a.Zone != "" {
			return a.Zone
		}
	}
	return ""
}

// addProfessionChores adds the while-you're-here profession lines: path materials a vendor sells near a stop,
// gathering in a trip's zone, and a craft batch at the first city stop. Together with the trainer errands they
// never pass maxProfessionLines.
func addProfessionChores(p *Plan, c character.Character, src source, opt Options) {
	qs, ok := src.(*questieSource)
	if !ok || len(p.Steps) == 0 {
		return
	}
	hints := professionHints(p, c, opt)
	if len(hints) == 0 {
		return
	}
	qx := qs.qx
	maps := map[int]bool{}
	for _, s := range p.Steps {
		maps[s.Map] = true
	}
	for _, h := range hints { // materials on the way: only for a step whose whole craft can be made
		if h.Next == nil {
			continue
		}
		type buy struct {
			need    professions.Need
			missing int
			spots   []Spot
		}
		var picks []Chore
		var buys []buy
		whole := true
		for _, n := range h.Next.Needs {
			if n.Missing <= 0 || hasRestock(p, n.Item) || hasPickup(p, n.Item) {
				continue
			}
			pick, missing := pickupChores(p, c, qx, opt, maps, n.ID, n.Item, n.Missing)
			picks = append(picks, pick...)
			if missing <= 0 {
				continue
			}
			spots := vendorSpots(c, qx, opt, maps, n.ID, n.Item)
			if len(spots) == 0 {
				whole = false // a reagent neither held, mailable nor sold here: no craft can be finished
				break
			}
			buys = append(buys, buy{n, missing, spots})
		}
		if !whole || professionLines(p)+len(buys) > maxProfessionLines {
			continue
		}
		bundleChores(p, picks)
		for _, b := range buys {
			bundleChores(p, []Chore{{Kind: ErrandRestock, Do: fmt.Sprintf("buy %d %s", b.missing, b.need.Item), Prep: "from",
				Why: fmt.Sprintf("%s%s skill-ups (%s, %d to %d)", profWhyPrefix, h.Profession, h.Next.Recipe, h.Next.From, h.Next.To), Spots: b.spots}})
		}
	}
	tripUsed := map[int]bool{}
	for _, h := range hints { // gathering in a trip's zone
		if h.Gather == nil {
			continue
		}
		for _, t := range p.Trips {
			if tripUsed[t.Order] || professionLines(p) >= maxProfessionLines {
				continue
			}
			zone := cmp.Or(t.Zone, zoneOfMap(qx, t.Map))
			node := nodeIn(h.Gather, zone)
			if node == "" {
				continue
			}
			st, ok := stepOnMap(p, t.Map)
			if !ok {
				continue
			}
			tripUsed[t.Order] = true
			p.Errands = append(p.Errands, Errand{Kind: ErrandProfession, Label: fmt.Sprintf("Gather %s here (skill-ups to %d)", node, h.Gather.To),
				Map: st.Map, X: st.X, Y: st.Y, Why: fmt.Sprintf("%s%s skill-ups: %s grows in %s", profWhyPrefix, h.Profession, node, zone), Attach: st.N})
		}
	}
	city, ok := firstCityStop(p, c, qx)
	for _, h := range hints { // a craft batch at the first city stop
		if !ok || professionLines(p) >= maxProfessionLines {
			break
		}
		if h.Next.Held() {
			p.Errands = append(p.Errands, Errand{Kind: ErrandProfession, Label: fmt.Sprintf("Craft %d %s (to %d)", h.Next.Crafts, h.Next.Recipe, h.Next.To),
				Map: city.Map, X: city.X, Y: city.Y, Why: profWhyPrefix + h.Profession + " skill-ups: you hold the materials, and a city has every crafting station", Attach: city.N})
		}
	}
}

func hasRestock(p *Plan, item string) bool {
	for _, e := range p.Errands {
		if e.Kind == ErrandRestock && strings.Contains(e.Label, item) {
			return true
		}
	}
	return false
}

// nodeIn is the first node of the hint that grows in the zone, "" when none does.
func nodeIn(g *professions.GatherHint, zone string) string {
	for _, n := range g.Nodes {
		for _, z := range n.Zones {
			if zone != "" && strings.EqualFold(z, zone) {
				return n.Name
			}
		}
	}
	return ""
}

// stepOnMap is the first stop with a position on the map, else the first on it.
func stepOnMap(p *Plan, m int) (Stop, bool) {
	var any *Stop
	for i, s := range p.Steps {
		if s.Map != m {
			continue
		}
		if s.X >= 0 && s.Y >= 0 {
			return s, true
		}
		if any == nil {
			any = &p.Steps[i]
		}
	}
	if any != nil {
		return *any, true
	}
	return Stop{}, false
}

// firstCityStop is the first stop of the route that stands in a capital the character can use, with a position.
func firstCityStop(p *Plan, c character.Character, qx *questie.Index) (Stop, bool) {
	for _, s := range p.Steps {
		if s.X < 0 || s.Y < 0 {
			continue
		}
		if isHub(c, s.Place) || isHub(c, zoneOfMap(qx, s.Map)) {
			return s, true
		}
	}
	return Stop{}, false
}

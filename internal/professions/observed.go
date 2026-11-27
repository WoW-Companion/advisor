package professions

import (
	"fmt"
	"hash/fnv"
	"strings"
	"time"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/store"
)

// observedStore puts the character's own recipe list in front of the dataset: a recipe the trade
// skill window listed keeps its observed reagents and product, and a recipe the dataset lacks is added.
type observedStore struct {
	store.DataStore
	profession string
	reagents   map[int64][]character.ItemStack // by spell id, the observed reagents
	makes      map[int64]int64                 // by spell id, the observed product
	added      []store.Row                     // observed-only recipes
}

// observedID is the id of a recipe only the player's list has: negative, so it never meets a dataset spell.
func observedID(name string) int64 {
	h := fnv.New32a()
	h.Write([]byte(strings.ToLower(name)))
	return -int64(h.Sum32()) - 1
}

func (o *observedStore) Filter(table string, ranges ...store.Range) ([]store.Row, error) {
	rows, err := o.DataStore.Filter(table, ranges...)
	switch table {
	case "recipes":
		if err != nil {
			rows = nil
		}
		for _, r := range rows {
			id, _ := r["spell_id"].(int64)
			if m := o.makes[id]; m != 0 {
				r["creates_item_id"] = m
			}
		}
		if len(ranges) == 1 && ranges[0].Column == "skill_line_name" && strings.EqualFold(fmt.Sprint(ranges[0].Min), o.profession) {
			rows = append(rows, o.added...)
		}
		return rows, nil
	case "recipe_reagents":
		if len(ranges) == 1 && ranges[0].Column == "spell_id" {
			if id, ok := ranges[0].Min.(int64); ok {
				if rg, ok := o.reagents[id]; ok {
					out := make([]store.Row, 0, len(rg))
					for _, x := range rg {
						out = append(out, store.Row{"spell_id": id, "reagent_item_id": int64(x.ItemID), "count": int64(x.Count)})
					}
					return out, nil
				}
			}
		}
	}
	return rows, err
}

// withObserved wraps ds with the character's recipes for the profession, and returns the names of
// the recipes only the player's list has. It returns ds itself when nothing was recorded.
func withObserved(ds store.DataStore, c character.Character, profession string) (store.DataStore, *observedStore) {
	rs, ok := c.Recipes[strings.ToLower(profession)]
	if !ok || len(rs.Entries) == 0 || ds == nil {
		return ds, nil
	}
	o := &observedStore{DataStore: ds, profession: profession, reagents: map[int64][]character.ItemStack{}, makes: map[int64]int64{}}
	byName := map[string]int64{}
	if rows, err := ds.Filter("recipes", store.Range{Column: "skill_line_name", Min: profession, Max: profession}); err == nil {
		for _, r := range rows {
			id, _ := r["spell_id"].(int64)
			n, _ := r["name"].(string)
			if id != 0 && n != "" {
				byName[strings.ToLower(strings.TrimSpace(n))] = id
			}
		}
	}
	for _, e := range rs.Entries {
		id, inData := byName[strings.ToLower(e.Name)]
		if !inData {
			id = observedID(e.Name)
			row := store.Row{"spell_id": id, "skill_line_name": profession, "name": e.Name, "learned_from": "client"}
			if e.Makes != 0 {
				row["creates_item_id"] = e.Makes
			}
			o.added = append(o.added, row)
		}
		if len(e.Reagents) > 0 {
			o.reagents[id] = e.Reagents
		}
		if e.Makes != 0 {
			o.makes[id] = e.Makes
		}
	}
	return o, o
}

// seenOffer finds an offer a trainer the player opened made for the recipe, by name (a recipe has no
// rank), newest sighting first.
func (p *planner) seenOffer(r Recipe) bool {
	_, _, ok := p.seenOfferOf(r)
	return ok
}

func (p *planner) seenOfferOf(r Recipe) (character.SeenOffer, character.SeenTrainer, bool) {
	if p.c.Seen == nil || strings.TrimSpace(r.Name) == "" {
		return character.SeenOffer{}, character.SeenTrainer{}, false
	}
	for _, t := range p.c.Seen.TrainersFor(p.c.Faction) {
		for _, o := range t.Offers {
			if strings.EqualFold(strings.TrimSpace(o.Name), strings.TrimSpace(r.Name)) || r.SpellID > 0 && o.Spell == r.SpellID {
				return o, t, true
			}
		}
	}
	return character.SeenOffer{}, character.SeenTrainer{}, false
}

// offerText says what the trainer the player opened asked for the recipe, in plain words.
func offerText(o character.SeenOffer, t character.SeenTrainer, now time.Time) string {
	s := "your trainer"
	if t.Name != "" {
		s += " (" + t.Name + ")"
	}
	s += " teaches it"
	if o.Cost > 0 {
		s += ", " + Money(o.Cost)
	}
	if o.State == "unavailable" {
		switch {
		case o.Skill != "" && o.SkillLevel > 0:
			s += fmt.Sprintf(", needs %s %d", o.Skill, o.SkillLevel)
		case o.Level > 0:
			s += fmt.Sprintf(", needs level %d", o.Level)
		}
	} else if o.Level > 1 {
		s += fmt.Sprintf(", needs level %d", o.Level)
	}
	if n := character.SeenNote(t.T, now); n != "" {
		s += " (" + n + ")"
	}
	return s
}

// nearestVendor picks the observed vendor of an item to use: on the character's map first and the
// nearest there, else the newest sighting. ok is false when no open vendor sold it for coin.
func (p *planner) nearestVendor(item int64) (character.SeenVendor, character.SeenStock, bool) {
	vs, stock := p.c.Seen.VendorsOf(item, p.c.Faction)
	here := -1
	var px, py float64
	if l := p.c.Location; l != nil && l.MapID != nil {
		here = *l.MapID
		if l.X != nil {
			px = *l.X
		}
		if l.Y != nil {
			py = *l.Y
		}
	}
	best := -1
	var bestKey [3]float64
	for i, v := range vs {
		if stock[i].Price <= 0 {
			continue
		}
		key := [3]float64{1, 0, -float64(v.T)}
		if v.Map == here {
			key = [3]float64{0, (v.X-px)*(v.X-px) + (v.Y-py)*(v.Y-py), -float64(v.T)}
		}
		if best < 0 || less3(key, bestKey) {
			best, bestKey = i, key
		}
	}
	if best < 0 {
		return character.SeenVendor{}, character.SeenStock{}, false
	}
	return vs[best], stock[best], true
}

func less3(a, b [3]float64) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}

// vendorText names a vendor the player opened: "a vendor, 5s each: Name (Zone 41, 52), seen 2 days ago".
func (p *planner) vendorText(v character.SeenVendor, s character.SeenStock, now time.Time) string {
	where := v.Name
	if z := p.c.ZoneOfMap(v.Map); z != "" {
		where += ", " + z
	}
	if v.X != 0 || v.Y != 0 {
		where += fmt.Sprintf(" %.0f, %.0f", v.X, v.Y)
	}
	t := "a vendor, " + Money(s.Price) + " each"
	if s.Ext {
		t += " plus other items"
	}
	t += " (" + strings.TrimSpace(where)
	if n := character.SeenNote(v.T, now); n != "" {
		t += ", " + n
	}
	return t + ")"
}

// applyObserved settles what the player's own recipe list says about each recipe at the current
// skill: a recipe listed there is known, and its colour fixes where the skill-up thresholds stand
// (a recipe only the player's list has gets estimated ones). It returns the known set to plan with
// and the names of the recipes the dataset lacks.
func (p *planner) applyObserved(recipes []Recipe) (known map[int64]bool, only map[string]bool) {
	known = map[int64]bool{}
	for id, v := range p.c.KnownSpells {
		known[id] = v
	}
	only = map[string]bool{}
	for i := range recipes {
		r := &recipes[i]
		e, ok := p.c.Recipe(p.prof, r.Name)
		if !ok {
			if r.CreatesItem != 0 && p.c.LikelyKnowsMaker(p.prof, r.CreatesItem) {
				// No recipe list was captured and the product is in the bags or bank: the recipe is probably known.
				known[r.SpellID] = true
				p.likely = append(p.likely, r.Name)
				continue
			}
			if o, _, seen := p.seenOfferOf(*r); seen {
				if o.State == "used" {
					known[r.SpellID] = true
				} else if o.Skill != "" && o.SkillLevel > 0 {
					lv := o.SkillLevel
					r.LearnSkill, r.LearnSource = &lv, "trainer" // the trainer's own gate
				}
			}
			continue
		}
		known[r.SpellID] = true
		sk := p.skill
		y, g := sk, sk+1
		switch e.Difficulty {
		case "orange":
			y = sk + 1
			if r.YellowSkill != nil && *r.YellowSkill > y {
				y = *r.YellowSkill
			}
			g = y + 20
			if r.GreySkill != nil && *r.GreySkill > g {
				g = *r.GreySkill
			}
		case "yellow":
			y = min(sk, valueOr(r.YellowSkill, sk))
			g = max(sk+1, valueOr(r.GreySkill, sk+10))
		case "green", "grey":
			y, g = sk-20, sk // no skill-up left worth planning
		default:
			continue
		}
		if r.LearnedFrom == "client" {
			only[strings.ToLower(r.Name)] = true
			r.LearnSkill = nil
		}
		r.YellowSkill, r.GreySkill = &y, &g
	}
	return known, only
}

func valueOr(p *int, d int) int {
	if p == nil {
		return d
	}
	return *p
}

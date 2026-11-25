package professions

import (
	"strings"

	"github.com/WoW-Companion/advisor/internal/store"
)

// dungeonOnlyRecipes are crafts whose pattern drops only inside an instance, though the dataset links
// no recipe item to them; the key is the lower-case recipe name.
var dungeonOnlyRecipes = map[string]string{
	"gordok ogre suit": "Dire Maul",
}

// hasTrainerOffers says the dataset lists trainer offers at all; without any, no recipe can be called
// unconfirmed.
func (p *planner) hasTrainerOffers() bool {
	if p.trainerOffers == 0 {
		p.trainerOffers = -1
		if rows, err := p.ds.Filter("trainer_spells", store.Range{Column: "spell_id", Min: 1, Max: 1 << 40}); err == nil && len(rows) > 0 {
			p.trainerOffers = 1
		}
	}
	return p.trainerOffers > 0
}

// seenOffer says a trainer the player opened offered the recipe (by spell id, else by name).
func (p *planner) seenOffer(r Recipe) bool {
	if p.c.Seen == nil {
		return false
	}
	for _, t := range p.c.Seen.TrainersFor(p.c.Faction) {
		for _, o := range t.Offers {
			if r.SpellID != 0 && o.Spell == r.SpellID || r.Name != "" && strings.EqualFold(strings.TrimSpace(o.Name), strings.TrimSpace(r.Name)) {
				return true
			}
		}
	}
	return false
}

// dungeonOnly says a recipe no trainer offer confirms is taught by a pattern that comes only from an
// instance: a recipe item whose only sources are instance bosses, or a known dungeon-only craft. It
// returns the dungeon's name.
func (p *planner) dungeonOnly(r Recipe) (string, bool) {
	if len(rowsBy(p.ds, "trainer_spells", "spell_id", r.SpellID)) > 0 || p.seenOffer(r) {
		return "", false
	}
	for _, ri := range rowsBy(p.ds, "recipe_items", "teaches_spell_id", r.SpellID) {
		id := asI64(ri["item_id"])
		if len(rowsBy(p.ds, "vendor_sources", "item_id", id)) > 0 {
			return "", false
		}
		where := ""
		other := false
		for _, l := range rowsBy(p.ds, "loot_sources", "item_id", id) {
			k := strings.ToLower(asText(l["source_kind"]))
			if !strings.Contains(k, "creature") && !strings.Contains(k, "object") {
				other = true
				continue
			}
			if d := p.instanceOf(asI64(l["source_id"])); d != "" {
				where = d
			} else {
				other = true
			}
		}
		if where != "" && !other {
			return where, true
		}
	}
	if d, ok := dungeonOnlyRecipes[strings.ToLower(r.Name)]; ok {
		return d, true
	}
	return "", false
}

// instanceOf names the dungeon or raid a creature is a boss of, or "" when it is not one.
func (p *planner) instanceOf(creature int64) string {
	for _, b := range rowsBy(p.ds, "instance_bosses", "creature_id", creature) {
		for _, in := range rowsBy(p.ds, "instances", "map_id", asI64(b["map_id"])) {
			if n := asText(in["name"]); n != "" {
				return n
			}
		}
	}
	return ""
}

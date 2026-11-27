package stats

import "github.com/WoW-Companion/advisor/internal/store"

// ItemStats is how much of each stat an items row carries, keyed by the client's stat type id: the
// dataset's `stats` column (type and amount of each stat). The store reads that column as one flat
// list, all the types and then all the amounts, so the list is used only when it is twice as long as
// `stat_types` and its first half is exactly those types; any other shape (an older dataset without
// the column, a mismatch) gives nil, and the caller falls back to counting the types.
func ItemStats(row store.Row) map[int64]int {
	types, _ := row["stat_types"].([]any)
	flat, _ := row["stats"].([]any)
	n := len(types)
	if n == 0 || len(flat) != 2*n {
		return nil
	}
	out := make(map[int64]int, n)
	for i := 0; i < n; i++ {
		id, ok1 := flat[i].(int64)
		amount, ok2 := flat[n+i].(int64)
		if want, ok := types[i].(int64); !ok1 || !ok2 || !ok || want != id || amount <= 0 {
			return nil
		}
		out[id] += int(amount)
	}
	return out
}

// SpellPower is the weight name of the item stat that adds damage and healing to spells.
const SpellPower = "spell_power"

// The weight names of the older item stats: bonus healing and bonus damage on a Classic item, and the extra armor
// a green item adds to its base armor.
const (
	SpellHealing = "spell_healing"
	SpellDamage  = "spell_damage"
	ExtraArmor   = "extra_armor"
)

// The weight names of the combat ratings a Classic-era item carries: Defense Rating, Hit Rating and mana per five
// seconds. Crit is the stat package's own Crit.
const (
	Defense = "defense"
	Hit     = "hit"
	MP5     = "mp5"
)

// The weights of the combat ratings, from 0 to 1. The dataset records the amount an item carries but no cap (no hit cap,
// no defense skill for crit immunity), so these say a rating is wanted, never how much is enough.
const (
	weightDefense    = 1.0 // a tank's defense is the stat that makes crits impossible: it counts as much as Stamina
	weightHit        = 0.5 // a damage dealer misses without it
	weightCrit       = 0.5
	weightHealerCrit = 0.3
	weightMP5        = 0.4 // a healer's mana lasts longer with it
)

// ItemWeights are the profile's weights read for gear: the five primaries as they stand, plus the item
// stats a profile does not list but that do the same job as its lead stat. Spell Power counts as much
// as Intellect for a caster or healer (it is what their gear carries instead of, or besides, Intellect), and so
// does the older bonus healing for a healer and bonus damage for a caster; Attack Power counts four-fifths of
// the better of Strength and Agility for a melee, ranged or feral build, as the gear weights file does for the
// specs it lists. A tank, whose profile already names armor and defences, counts the extra armor a green item
// adds as much as armor, and Defense Rating as much as Stamina. A damage dealer counts Hit and Crit Rating, a healer
// Crit Rating and mana per five seconds. The stat check and the digest keep reading Weights.
func (p Prof) ItemWeights() map[string]float64 {
	out := make(map[string]float64, len(p.Weights)+1)
	for k, v := range p.Weights {
		out[k] = v
	}
	switch p.Arch {
	case archCaster, archHealer:
		out[SpellPower] = p.Weights[Intellect]
		if p.Arch == archHealer {
			out[SpellHealing] = p.Weights[Intellect]
			out[Crit], out[MP5] = weightHealerCrit, weightMP5
		} else {
			out[SpellDamage] = p.Weights[Intellect]
			out[Hit], out[Crit] = weightHit, weightCrit
		}
	case archTank:
		out[ExtraArmor] = p.Weights[Armor]
		out[Defense] = weightDefense
	case archStr, archAgi, archRanged, archFeral:
		out[AttackPower] = 0.8 * max(p.Weights[Strength], p.Weights[Agility])
		out[Hit], out[Crit] = weightHit, weightCrit
	}
	return out
}

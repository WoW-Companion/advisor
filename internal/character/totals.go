package character

import (
	"strings"

	"github.com/WoW-Companion/advisor/internal/lua"
)

// Totals are a character's stat numbers as the WoW Companion addon records them read-only in
// WoWCompanionDB.characters[<character key>].stats (every field optional; nil when the addon wrote none).
type Totals struct {
	At    *int64 // stats.seenAt
	Level *int   // stats.level
	// The five stats, as the totals the player sees (stats.stats.<stat>.total); Base, Pos and Neg are the
	// same stat's unmodified value and its gear/buff and penalty parts.
	Str, Agi, Sta, Int, Spi *float64
	Base, Pos, Neg          map[string]float64 // keyed "str", "agi", "sta", "int", "spi"
	HealthMax, ManaMax      *float64
	Armor, AttackPower      *float64
	RangedAttackPower       *float64
	// Crit is the melee crit chance (stats.meleeCrit); RangedCrit and SpellCrit (the best school) are percents.
	Crit, RangedCrit, SpellCrit *float64
	Dodge, Parry, Block         *float64
	ManaRegenBase               *float64 // stats.manaRegen.base
	ManaRegenCasting            *float64 // stats.manaRegen.casting
	SpellDamage, SpellHealing   *float64
	// Per are what the client's own per-point getters report for this character (stats.perPoint); nil when
	// the addon wrote none.
	Per *PerPoint
}

// PerPoint are the client's per-point getters, every field optional: stats.perPoint.spellCritFromInt,
// critFromAgi, manaRegenFromSpi and healthRegenFromSpi.
type PerPoint struct {
	SpellCritFromInt, CritFromAgi, ManaRegenFromSpi, HealthRegenFromSpi *float64
}

func num(v any) *float64 {
	switch n := v.(type) {
	case int64:
		f := float64(n)
		return &f
	case float64:
		return &n
	}
	return nil
}

// Total is the stat's total (0 when the capture has none), keyed like Base.
func (t *Totals) Total(k string) float64 {
	for key, p := range map[string]*float64{"str": t.Str, "agi": t.Agi, "sta": t.Sta, "int": t.Int, "spi": t.Spi} {
		if key == k && p != nil {
			return *p
		}
	}
	return 0
}

// Any reports whether at least one total is present and positive.
func (t *Totals) Any() bool {
	if t == nil {
		return false
	}
	for _, p := range []*float64{t.Str, t.Agi, t.Sta, t.Int, t.Spi} {
		if p != nil && *p > 0 {
			return true
		}
	}
	return false
}

// ParseTotals reads WoWCompanionDB.characters[key].stats out of the addon's SavedVariables text, tolerantly: a
// missing table, entry or field gives nil or a nil field. The key is matched as captured, then
// case-insensitively, then by character name alone.
func ParseTotals(text, key, name string) *Totals {
	vars, err := lua.Parse(text)
	if err != nil {
		return nil
	}
	return totalsOf(vars, key, name)
}

func totalsOf(vars map[string]any, key, name string) *Totals {
	db, _ := vars["WoWCompanionDB"].(lua.Table)
	all, _ := db["characters"].(lua.Table)
	var rec lua.Table
	if t, ok := all[key].(lua.Table); ok {
		rec = t
	} else {
		for k, v := range all {
			ks, _ := k.(string)
			if t, ok := v.(lua.Table); ok && (strings.EqualFold(ks, key) || name != "" && strings.EqualFold(ks, name)) {
				rec = t
				break
			}
		}
	}
	e, _ := rec["stats"].(lua.Table)
	if e == nil {
		return nil
	}
	t := &Totals{At: asInt64(e["seenAt"]), Level: asInt(e["level"]),
		HealthMax: num(e["healthMax"]), ManaMax: num(e["manaMax"]), Armor: num(e["armor"]),
		AttackPower: num(e["attackPower"]), RangedAttackPower: num(e["rangedAttackPower"]),
		Crit: num(e["meleeCrit"]), RangedCrit: num(e["rangedCrit"]),
		Dodge: num(e["dodge"]), Parry: num(e["parry"]), Block: num(e["block"]), Per: perPoint(e["perPoint"])}
	if sc, ok := e["spellCrit"].(lua.Table); ok {
		t.SpellCrit = num(sc["max"])
	}
	if mr, ok := e["manaRegen"].(lua.Table); ok {
		t.ManaRegenBase, t.ManaRegenCasting = num(mr["base"]), num(mr["casting"])
	}
	if sb, ok := e["spellBonus"].(lua.Table); ok {
		t.SpellDamage, t.SpellHealing = num(sb["damage"]), num(sb["healing"])
	}
	five, _ := e["stats"].(lua.Table)
	t.Base, t.Pos, t.Neg = map[string]float64{}, map[string]float64{}, map[string]float64{}
	for k, dst := range map[string]**float64{"str": &t.Str, "agi": &t.Agi, "sta": &t.Sta, "int": &t.Int, "spi": &t.Spi} {
		st, _ := five[k].(lua.Table)
		if st == nil {
			continue
		}
		*dst = num(st["total"])
		for field, m := range map[string]map[string]float64{"base": t.Base, "pos": t.Pos, "neg": t.Neg} {
			if f := num(st[field]); f != nil {
				m[k] = *f
			}
		}
		// Before addon 0.6.39 the capture's base equals the total; the unmodified value is total - pos + neg.
		if b, ok := t.Base[k]; ok && t.Pos[k] > 0 && t.Total(k) == b {
			t.Base[k] = b - t.Pos[k] + t.Neg[k]
		}
	}
	return t
}

// perPoint reads the optional per-point table; nil when absent or empty.
func perPoint(v any) *PerPoint {
	t, ok := v.(lua.Table)
	if !ok {
		return nil
	}
	p := &PerPoint{SpellCritFromInt: num(t["spellCritFromInt"]), CritFromAgi: num(t["critFromAgi"]),
		ManaRegenFromSpi: num(t["manaRegenFromSpi"]), HealthRegenFromSpi: num(t["healthRegenFromSpi"])}
	if *p == (PerPoint{}) {
		return nil
	}
	return p
}

// AttachTotals returns c with what the addon recorded in its SavedVariables file: its stat totals
// and what the player saw (Seen). c is unchanged when the file is unreadable or holds none.
func AttachTotals(addonSV string, c Character) Character { return AttachAddon(addonSV, c) }

package character

import (
	"strings"

	"github.com/WoW-Companion/advisor/internal/lua"
)

// Totals are a character's stat totals as the WoW Companion addon records them read-only in
// WoWCompanionDB.stats[<character key>] (every field optional; nil when the addon wrote none).
type Totals struct {
	At                                   *int64
	Level                                *int
	Str, Agi, Sta, Int, Spi              *float64
	Armor, AP, Crit, Dodge, Parry, Block *float64
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

// ParseTotals reads WoWCompanionDB.stats[key] out of the addon's SavedVariables text, tolerantly: a
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
	all, _ := db["stats"].(lua.Table)
	var e lua.Table
	if t, ok := all[key].(lua.Table); ok {
		e = t
	} else {
		for k, v := range all {
			ks, _ := k.(string)
			if t, ok := v.(lua.Table); ok && (strings.EqualFold(ks, key) || name != "" && strings.EqualFold(ks, name)) {
				e = t
				break
			}
		}
	}
	if e == nil {
		return nil
	}
	return &Totals{At: asInt64(e["at"]), Level: asInt(e["level"]),
		Str: num(e["str"]), Agi: num(e["agi"]), Sta: num(e["sta"]), Int: num(e["int"]), Spi: num(e["spi"]),
		Armor: num(e["armor"]), AP: num(e["ap"]), Crit: num(e["crit"]), Dodge: num(e["dodge"]),
		Parry: num(e["parry"]), Block: num(e["block"])}
}

// AttachTotals returns c with what the addon recorded in its SavedVariables file: its stat totals
// and what the player saw (Seen). c is unchanged when the file is unreadable or holds none.
func AttachTotals(addonSV string, c Character) Character { return AttachAddon(addonSV, c) }

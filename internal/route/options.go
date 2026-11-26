package route

import (
	"os"

	"github.com/WoW-Companion/advisor/internal/questie"
	"github.com/WoW-Companion/advisor/internal/store"
	"github.com/WoW-Companion/advisor/internal/unlock"
)

// OptionsFor gathers the chain-aware inputs of a plan: the dataset's quest lines (ds may be nil),
// the addon's observation log read from addonSV (the addon's WoWCompanion.lua; empty or unreadable:
// none) and the local file of observed unlocks, which this call updates with what the log newly
// shows. Everything stays on the player's machine.
func OptionsFor(addonSV string, ds store.DataStore) Options {
	opt := Options{Lines: unlock.FromDataset(ds)}
	st := unlock.Open(unlock.LocalPath())
	if addonSV != "" {
		if raw, err := os.ReadFile(addonSV); err == nil {
			opt.Events = unlock.ParseEvents(string(raw))
			if st.Merge(unlock.Learn(opt.Events)) {
				_ = st.Save()
			}
		}
	}
	opt.Obs = st.Obs
	opt.XP = questie.LoadXP(addonSV)
	opt.Dungeons = dungeonLevels(ds)
	return opt
}

// dungeonLevels reads the level ranges of the dataset's dungeons (ds may be nil).
func dungeonLevels(ds store.DataStore) []DungeonLevel {
	if ds == nil {
		return nil
	}
	rows, err := ds.Filter("instances", store.Range{Column: "map_id", Min: int64(0)})
	if err != nil {
		return nil
	}
	var out []DungeonLevel
	for _, r := range rows {
		if r["instance_type"] != "dungeon" {
			continue
		}
		lo, ok1 := r["min_level"].(int64)
		hi, ok2 := r["max_level"].(int64)
		m, _ := r["map_id"].(int64)
		name, _ := r["name"].(string)
		if ok1 && ok2 && lo > 0 && hi >= lo {
			out = append(out, DungeonLevel{Map: int(m), Name: name, Min: int(lo), Max: int(hi)})
		}
	}
	return out
}

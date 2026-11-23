package route

import (
	"os"

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
	return opt
}

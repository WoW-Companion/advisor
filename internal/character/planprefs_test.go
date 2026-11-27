package character

import (
	"testing"

	"github.com/WoW-Companion/advisor/internal/lua"
)

func TestPlanPrefsReading(t *testing.T) {
	if wowcPlanPrefs(nil) != nil {
		t.Error("no table means defaults")
	}
	c := Character{}
	if c.Reach() != 3 || c.IsHidden(1) || c.ElitesIncluded() {
		t.Errorf("defaults: %d", c.Reach())
	}
	pp := wowcPlanPrefs(lua.Table{"levelReach": int64(99), "hidden": lua.Table{int64(12): true, int64(13): false}, "includeElite": true})
	if pp.LevelReach != 10 || !pp.Hidden[12] || pp.Hidden[13] || !pp.IncludeElite {
		t.Errorf("prefs %+v", pp)
	}
}

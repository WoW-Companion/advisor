package route

import (
	"strings"

	"github.com/WoW-Companion/advisor/internal/store"
)

// unknownPlace is what a map the data cannot name is called: never a bare map number.
const unknownPlace = "an unknown place"

// UIMap is a row of the dataset's ui_maps table.
type UIMap struct {
	ID       int
	Name     string
	Parent   int // the parent UI map, 0 when none
	Instance int // the instance map id the UI map shows, 0 when it is not an instance
}

// Instance is a row of the dataset's instances table.
type Instance struct {
	Map      int
	Name     string
	Entrance string // the entrance zone's name
}

// Places names the maps Questie's area table does not: the dataset's UI maps and instances.
// The zero value (and nil) knows only the built-in Classic dungeon table.
type Places struct {
	UIMaps    map[int]UIMap
	Instances map[int]Instance
}

// classicDungeonUIMaps maps a Classic dungeon's UI map id to its instance map id, for a dataset
// without a ui_maps table. UI map ids are from the Blizzard UiMapID list (warcraft.wiki.gg
// "UiMapID"); instance ids are the instances table's map ids (Shadowfang Keep 33, Wailing Caverns 43).
var classicDungeonUIMaps = map[int]int{
	310: 33,  // Shadowfang Keep
	279: 43,  // Wailing Caverns
	291: 36,  // The Deadmines
	213: 389, // Ragefire Chasm
}

// Name is the name of UI map m from the dataset, "" when it knows none: its ui_maps name, else the
// instance's name, else the parent's name plus " (instance)".
func (p *Places) Name(m int) string {
	n, _ := p.describe(m)
	return n
}

// describe is Name with the entrance zone of an instance ("" for a plain map).
func (p *Places) describe(m int) (name, entrance string) {
	var u UIMap
	var has bool
	var inst Instance
	if p != nil {
		u, has = p.UIMaps[m]
	}
	im := u.Instance
	if im == 0 {
		im = classicDungeonUIMaps[m]
	}
	if p != nil && im != 0 {
		inst = p.Instances[im]
	}
	switch {
	case has && u.Name != "":
		return u.Name, inst.Entrance
	case inst.Name != "":
		return inst.Name, inst.Entrance
	case has && u.Parent != 0:
		if pn := p.Name(u.Parent); pn != "" && pn != u.Name && u.Parent != m {
			return pn + " (instance)", ""
		}
	}
	return "", ""
}

// IsInstance reports whether UI map m shows an instance (a dungeon or raid).
func (p *Places) IsInstance(m int) bool {
	if classicDungeonUIMaps[m] != 0 {
		return true
	}
	if p == nil {
		return false
	}
	return p.UIMaps[m].Instance != 0
}

// Travel is the name for a travel target: "Shadowfang Keep (Silverpine Forest)" for an instance.
func (p *Places) Travel(m int) string {
	n, e := p.describe(m)
	if n != "" && e != "" && !strings.EqualFold(n, e) {
		return n + " (" + e + ")"
	}
	return n
}

func rowInt(r store.Row, k string) int {
	v, _ := r[k].(int64)
	return int(v)
}

func rowStr(r store.Row, k string) string {
	v, _ := r[k].(string)
	return v
}

// placesFrom reads the instances and, when the dataset has it, the ui_maps table (ds may be nil).
func placesFrom(ds store.DataStore) *Places {
	if ds == nil {
		return nil
	}
	p := &Places{UIMaps: map[int]UIMap{}, Instances: map[int]Instance{}}
	if rows, err := ds.Filter("instances", store.Range{Column: "map_id", Min: int64(0)}); err == nil {
		for _, r := range rows {
			m := rowInt(r, "map_id")
			p.Instances[m] = Instance{Map: m, Name: rowStr(r, "name"), Entrance: rowStr(r, "entrance_zone_name")}
		}
	}
	if rows, err := ds.Filter("ui_maps", store.Range{Column: "ui_map_id", Min: int64(0)}); err == nil {
		for _, r := range rows {
			id := rowInt(r, "ui_map_id")
			p.UIMaps[id] = UIMap{ID: id, Name: rowStr(r, "name"), Parent: rowInt(r, "parent_ui_map_id"), Instance: rowInt(r, "instance_map_id")}
		}
	}
	return p
}

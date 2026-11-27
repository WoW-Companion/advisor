package questie

import (
	"sort"
	"strings"
)

// The service bits of an NPC's npcFlags: a trainer of any kind, a vendor of any kind (general, ammo,
// food, poison or reagent goods) and a flight master.
const (
	trainerMask = 0x70
	vendorMask  = 0xF80
	taxiFlag    = 0x2000
)

// IsVendor reports whether the NPC sells goods.
func (e Entity) IsVendor() bool { return e.Flags&vendorMask != 0 }

// IsTrainer reports whether the NPC trains a class or a profession.
func (e Entity) IsTrainer() bool { return e.Flags&trainerMask != 0 }

// IsFlightMaster reports whether the NPC runs a flight path.
func (e Entity) IsFlightMaster() bool { return e.Flags&taxiFlag != 0 }

// Service is an NPC with a place, found by its subname or name.
type Service struct {
	ID     int
	Entity Entity
}

// Services lists the vendors ("vendor"), trainers ("trainer") or flight masters ("flight") the index
// holds, by id. An NPC without a spawn is left out: it has no place to name.
func (ix *Index) Services(kind string) []Service {
	if ix == nil {
		return nil
	}
	var out []Service
	for id, e := range ix.Npcs {
		if e.Name == "" || len(e.Spawns) == 0 {
			continue
		}
		switch strings.ToLower(kind) {
		case "trainer":
			if !e.IsTrainer() {
				continue
			}
		case "flight":
			if !e.IsFlightMaster() {
				continue
			}
		default:
			if !e.IsVendor() {
				continue
			}
		}
		out = append(out, Service{ID: id, Entity: e})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// keepService adds the NPC rows with a service bit to the index, whether or not a quest names them, so
// a vendor such as "Shady Goods" can be found by its subname.
func (ix *Index) keepService(npcs map[int]rawRow, l Layout) {
	fl := l.sc("npc", "npcFlags")
	for id, row := range npcs {
		if _, ok := ix.Npcs[id]; ok {
			continue
		}
		if f := num(row.scalars()[fl]); f&(vendorMask|trainerMask|taxiFlag) == 0 {
			continue
		}
		if e := buildEntity(row, l, "npc"); e.Name != "" && len(e.Spawns) > 0 {
			ix.Npcs[id] = e
		}
	}
}

// NpcEntity is the NPC's index entry.
func (ix *Index) NpcEntity(id int) (Entity, bool) {
	if ix == nil {
		return Entity{}, false
	}
	e, ok := ix.Npcs[id]
	return e, ok
}

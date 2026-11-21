package eval

// The captures and the Parquet dataset are built from the specs in this file when the tests start
// (TestMain) and written to a temporary directory. Everything here is synthetic: invented names,
// invented ids, nothing taken from a real capture or the real dataset.

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/parquet-go/parquet-go"
)

const capturedAt = 1700000000

// Client of every capture.
const (
	clientVersion = "9.9.9"
	clientBuild   = "99999"
)

// Zones of the synthetic dataset; a band's zone names carry the band so a mix-up shows.
func zone1(b int) string { return fmt.Sprintf("Evalzone %d-A", b) }
func zone2(b int) string { return fmt.Sprintf("Evalzone %d-B", b) }

// ---- dataset ----

type questRow struct {
	ID    int64   `parquet:"id"`
	Level *int32  `parquet:"level,optional"`
	Zone  *string `parquet:"zone,optional"`
}

type itemRow struct {
	ID            int64   `parquet:"id"`
	Name          string  `parquet:"name"`
	ItemLevel     int32   `parquet:"item_level"`
	RequiredLevel int32   `parquet:"required_level"`
	Quality       int32   `parquet:"quality"`
	ClassID       int32   `parquet:"class_id"`
	SubclassID    int32   `parquet:"subclass_id"`
	Slot          int32   `parquet:"slot"`
	ClassMask     int32   `parquet:"allowable_class_mask"`
	StatTypes     []int16 `parquet:"stat_types,list"`
	StatsStatus   string  `parquet:"stats_status"`
}

type lootRow struct {
	ItemID int64  `parquet:"item_id"`
	Kind   string `parquet:"source_kind"`
	ID     int64  `parquet:"source_id"`
	Kills  int32  `parquet:"kills"`
	Drops  int32  `parquet:"drops"`
}

type npcRow struct {
	ID       int64  `parquet:"id"`
	Name     string `parquet:"name"`
	MinLevel *int32 `parquet:"min_level,optional"`
	MaxLevel *int32 `parquet:"max_level,optional"`
}

// Item classes, armor and weapon subclasses, inventory types and stat types of the client's tables.
const (
	clsWeapon, clsArmor                                     = 2, 4
	cloth, leather, mail, plate                             = 1, 2, 3, 4
	mace1H, staff, fist, dagger, wand                       = 4, 10, 13, 15, 19
	slotHead, slotRing, slotOne, slotChest, slotLegs        = 1, 11, 13, 5, 7
	slotRobe, slotTwoHand, slotOff, slotRanged              = 20, 17, 22, 26
	statAgi, statStr, statInt, statSpirit, statSta          = 3, 4, 5, 6, 7
	maskNone, maskRogue, maskPriest                         = -1, 1 << 3, 1 << 4
	bands                                                   = 3
	q1, q2, q3, q4, q5, q6, q7, qUnknownLevel, qUnknownZone = 1, 2, 3, 4, 5, 6, 7, 50, 51
)

var allBands = [bands]int{12, 24, 40}

func i32(n int) *int32     { v := int32(n); return &v }
func str(s string) *string { return &s }

type itemSpec struct {
	n                int // id is band*1000 + n
	name             string
	ilvl, req, q     int // item level and required level as offsets from the band; quality
	class, sub, slot int
	mask             int
	stats            []int16
	creature, far    bool   // drops from an in-range creature, from a creature far above the band
	other            string // another source kind, with the id band*10+9
}

// bandItems is the gear of one band. ilvl and req are offsets from the band level.
var bandItems = []itemSpec{
	{n: 1, name: "Cloth Hood", ilvl: 4, req: -2, q: 2, class: clsArmor, sub: cloth, slot: slotHead, mask: maskNone, stats: []int16{statInt, statSta}, creature: true, far: true},
	{n: 2, name: "Spirit Hood", ilvl: 3, req: -2, q: 2, class: clsArmor, sub: cloth, slot: slotHead, mask: maskNone, stats: []int16{statSpirit, statSta}, other: "object"},
	{n: 3, name: "Leather Cap", ilvl: 4, req: -2, q: 2, class: clsArmor, sub: leather, slot: slotHead, mask: maskNone, stats: []int16{statAgi, statSta}, creature: true},
	{n: 4, name: "Mail Helm", ilvl: 6, req: -2, q: 2, class: clsArmor, sub: mail, slot: slotHead, mask: maskNone, stats: []int16{statStr, statSta}, creature: true},
	{n: 5, name: "Plate Helm", ilvl: 7, req: -2, q: 3, class: clsArmor, sub: plate, slot: slotHead, mask: maskNone, stats: []int16{statStr, statSta}, creature: true},
	{n: 6, name: "Cloth Robe", ilvl: 5, req: -2, q: 2, class: clsArmor, sub: cloth, slot: slotRobe, mask: maskNone, stats: []int16{statInt, statSpirit, statSta}, creature: true},
	{n: 7, name: "Leather Tunic", ilvl: 5, req: -2, q: 2, class: clsArmor, sub: leather, slot: slotChest, mask: maskNone, stats: []int16{statAgi, statSta}, creature: true},
	{n: 8, name: "Cloth Tunic", ilvl: 4, req: -2, q: 2, class: clsArmor, sub: cloth, slot: slotChest, mask: maskNone, stats: []int16{statInt, statSta}},
	{n: 9, name: "Cloth Leggings", ilvl: 3, req: -2, q: 2, class: clsArmor, sub: cloth, slot: slotLegs, mask: maskNone, stats: []int16{statInt, statSta}, creature: true},
	{n: 10, name: "Leather Leggings", ilvl: 3, req: -2, q: 2, class: clsArmor, sub: leather, slot: slotLegs, mask: maskNone, stats: []int16{statAgi, statSta}},
	{n: 11, name: "Ring of Mind", ilvl: 2, req: -4, q: 2, class: clsArmor, sub: 0, slot: slotRing, mask: maskNone, stats: []int16{statInt}, other: "object"},
	{n: 12, name: "Ring of Agility", ilvl: 2, req: -4, q: 2, class: clsArmor, sub: 0, slot: slotRing, mask: maskNone, stats: []int16{statAgi}, creature: true},
	{n: 13, name: "Plain Ring", ilvl: 1, req: -4, q: 2, class: clsArmor, sub: 0, slot: slotRing, mask: maskNone}, // stats unknown
	{n: 14, name: "Priest Mace", ilvl: 4, req: -2, q: 2, class: clsWeapon, sub: mace1H, slot: slotOne, mask: maskNone, stats: []int16{statInt, statSpirit}, creature: true},
	{n: 15, name: "Rogue Dagger", ilvl: 4, req: -2, q: 2, class: clsWeapon, sub: dagger, slot: slotOne, mask: maskNone, stats: []int16{statAgi, statSta}, creature: true},
	{n: 16, name: "Rogue Fist Claw", ilvl: 3, req: -2, q: 2, class: clsWeapon, sub: fist, slot: slotOne, mask: maskNone, stats: []int16{statAgi}, other: "quest"},
	{n: 17, name: "Caster Staff", ilvl: 5, req: -2, q: 2, class: clsWeapon, sub: staff, slot: slotTwoHand, mask: maskNone, stats: []int16{statInt, statSta}, creature: true},
	{n: 18, name: "Caster Wand", ilvl: 3, req: -2, q: 2, class: clsWeapon, sub: wand, slot: slotRanged, mask: maskNone, stats: []int16{statInt}},
	{n: 19, name: "Off-hand Dagger", ilvl: 3, req: -2, q: 2, class: clsWeapon, sub: dagger, slot: slotOff, mask: maskNone, stats: []int16{statAgi}, creature: true},
	{n: 20, name: "Future Hood", ilvl: 20, req: 10, q: 3, class: clsArmor, sub: cloth, slot: slotHead, mask: maskNone, stats: []int16{statInt}, creature: true},
	{n: 21, name: "Rogue-Only Mace", ilvl: 8, req: -2, q: 3, class: clsWeapon, sub: mace1H, slot: slotOne, mask: maskRogue, stats: []int16{statAgi, statSta}, creature: true},
	{n: 22, name: "Priest-Only Dagger", ilvl: 8, req: -2, q: 3, class: clsWeapon, sub: dagger, slot: slotOne, mask: maskPriest, stats: []int16{statInt, statSta}, creature: true},
	{n: 23, name: "Junk Hood", ilvl: 30, req: -2, q: 0, class: clsArmor, sub: cloth, slot: slotHead, mask: maskNone, stats: []int16{statInt}},
	{n: 24, name: "Weak Hood", ilvl: -10, req: -8, q: 2, class: clsArmor, sub: cloth, slot: slotHead, mask: maskNone, stats: []int16{statInt, statSta}, creature: true},
	{n: 25, name: "Weak Cap", ilvl: -10, req: -8, q: 2, class: clsArmor, sub: leather, slot: slotHead, mask: maskNone, stats: []int16{statAgi, statSta}, creature: true},
	// What the characters start out wearing (ids band*1000+9xx).
	{n: 901, name: "Worn Cloth Cap", ilvl: -8, req: -10, q: 1, class: clsArmor, sub: cloth, slot: slotHead, mask: maskNone, stats: []int16{statInt, statSta}},
	{n: 902, name: "Worn Leather Cap", ilvl: -8, req: -10, q: 1, class: clsArmor, sub: leather, slot: slotHead, mask: maskNone, stats: []int16{statAgi, statSta}},
	{n: 903, name: "Worn Cloth Robe", ilvl: -7, req: -10, q: 1, class: clsArmor, sub: cloth, slot: slotRobe, mask: maskNone, stats: []int16{statInt, statSta}},
	{n: 904, name: "Worn Leather Tunic", ilvl: -7, req: -10, q: 1, class: clsArmor, sub: leather, slot: slotChest, mask: maskNone, stats: []int16{statAgi, statSta}},
	{n: 905, name: "Worn Mace", ilvl: -7, req: -10, q: 1, class: clsWeapon, sub: mace1H, slot: slotOne, mask: maskNone, stats: []int16{statInt}},
	{n: 906, name: "Worn Dagger", ilvl: -7, req: -10, q: 1, class: clsWeapon, sub: dagger, slot: slotOne, mask: maskNone, stats: []int16{statAgi}},
}

func datasetTables() map[string]any {
	var quests []questRow
	var items []itemRow
	var loot []lootRow
	var npcs []npcRow
	for _, b := range allBands {
		qb := int64(b * 100)
		quests = append(quests,
			questRow{qb + q1, i32(b), str(zone1(b))}, questRow{qb + q2, i32(b + 1), str(zone1(b))},
			questRow{qb + q3, i32(b + 1), str(zone2(b))}, questRow{qb + q4, i32(b - 8), str(zone1(b))},
			questRow{qb + q5, i32(b + 8), str(zone2(b))}, questRow{qb + q6, i32(b), str(zone2(b))},
			questRow{qb + q7, i32(b), str(zone1(b))})
		near, farID := int64(b*10+1), int64(b*10+2)
		npcs = append(npcs,
			npcRow{near, fmt.Sprintf("Eval Wolf %d", b), i32(b - 1), i32(b + 1)},
			npcRow{farID, fmt.Sprintf("Eval Giant %d", b), i32(b + 20), i32(b + 22)})
		for _, s := range bandItems {
			id := int64(b*1000 + s.n)
			status := "known"
			if s.stats == nil {
				status = "unknown"
			}
			items = append(items, itemRow{id, fmt.Sprintf("Eval %s %d", s.name, b), int32(b + s.ilvl), int32(b + s.req), int32(s.q),
				int32(s.class), int32(s.sub), int32(s.slot), int32(s.mask), s.stats, status})
			if s.creature {
				loot = append(loot, lootRow{id, "creature", near, 100, 7})
			}
			if s.far {
				loot = append(loot, lootRow{id, "creature", farID, 100, 40})
			}
			if s.other != "" {
				loot = append(loot, lootRow{id, s.other, int64(b*10 + 9), 0, 0})
			}
		}
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return map[string]any{"quests": quests, "items": items, "loot_sources": loot, "npcs": npcs}
}

func writeParquet[T any](path string, rows []T) error {
	var buf bytes.Buffer
	if err := parquet.Write(&buf, rows); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

func writeTable(path string, rows any) error {
	switch r := rows.(type) {
	case []questRow:
		return writeParquet(path, r)
	case []itemRow:
		return writeParquet(path, r)
	case []lootRow:
		return writeParquet(path, r)
	default:
		return writeParquet(path, rows.([]npcRow))
	}
}

// ---- captures ----

type worn struct {
	slot string
	n    int // item id is band*1000 + n; 0 for an item the dataset does not hold
	name string
	ilvl int // offset from the band
}

type talentGroup struct {
	name  string
	spent int
}

type questMix struct {
	turnZ1, turnZ2, turnNoZone, open, grey, hard, unknownLevel, failed bool
	twoInZ1                                                            bool // both turn-ins of zone A (else one)
}

type charSpec struct {
	file, name, class, race string
	level                   int
	groups                  []talentGroup
	unspent                 int
	bank                    bool
	mix                     questMix
	gear                    []worn
	professions             []string
	bags                    map[string]int
}

var charSpecs = []charSpec{
	{file: "priest12", name: "Evalpriesta", class: "PRIEST", race: "Human", level: 12,
		groups: []talentGroup{{"Discipline", 5}, {"Holy", 1}, {"Shadow", 0}}, unspent: 2,
		mix:         questMix{turnZ1: true, twoInZ1: true, turnZ2: true, turnNoZone: true, open: true, grey: true, hard: true, unknownLevel: true, failed: true},
		gear:        []worn{{"head", 901, "Worn Cloth Cap", -8}, {"chest", 903, "Worn Cloth Robe", -7}, {"mainhand", 905, "Worn Mace", -7}},
		professions: []string{"Tailoring", "Herbalism"}, bags: map[string]int{"Linen Cloth": 20, "Peacebloom": 6, "Blood Shard": 12, "Spring Water": 4}},
	{file: "priest24", name: "Evalpriestb", class: "PRIEST", race: "Dwarf", level: 24, bank: true,
		groups:      []talentGroup{{"Discipline", 0}, {"Holy", 2}, {"Shadow", 9}},
		mix:         questMix{turnZ1: true, open: true, grey: true, hard: true, unknownLevel: true},
		gear:        []worn{{"head", 901, "Worn Cloth Cap", -8}, {"chest", 0, "Mystery Robe", -3}, {"mainhand", 905, "Worn Mace", -7}, {"neck", 0, "Mystery Amulet", -4}},
		professions: []string{"Tailoring"}, bags: map[string]int{"Linen Cloth": 40, "Silk Thread": 3}},
	{file: "priest40", name: "Evalpriestc", class: "PRIEST", race: "Night Elf", level: 40, bank: true,
		groups: []talentGroup{{"Discipline", 20}, {"Holy", 20}, {"Shadow", 0}}, unspent: 1,
		mix:         questMix{open: true, grey: true, hard: true, unknownLevel: true},
		gear:        []worn{{"head", 901, "Worn Cloth Cap", -8}, {"chest", 903, "Worn Cloth Robe", -7}},
		professions: []string{"Enchanting"}, bags: map[string]int{"Strange Dust": 15, "Soul Shard": 3}},
	{file: "rogue12", name: "Evalrogue-a", class: "ROGUE", race: "Orc", level: 12,
		groups: []talentGroup{{"Assassination", 4}, {"Combat", 0}, {"Subtlety", 0}}, unspent: 1,
		mix:         questMix{turnZ1: true, twoInZ1: true, open: true, grey: true, hard: true},
		gear:        []worn{{"head", 902, "Worn Leather Cap", -8}, {"mainhand", 906, "Worn Dagger", -7}},
		professions: []string{"Skinning"}, bags: map[string]int{"Light Leather": 9, "Ruined Leather Scraps": 4}},
	{file: "rogue24", name: "Evalrogue-b", class: "ROGUE", race: "Undead", level: 24, bank: true,
		groups:      []talentGroup{{"Assassination", 0}, {"Combat", 9}, {"Subtlety", 3}},
		mix:         questMix{turnZ1: true, turnZ2: true, open: true, hard: true},
		gear:        []worn{{"head", 902, "Worn Leather Cap", -8}, {"chest", 904, "Worn Leather Tunic", -7}, {"mainhand", 906, "Worn Dagger", -7}, {"legs", 0, "Mystery Breeches", -4}},
		professions: []string{"Mining", "Blacksmithing"}, bags: map[string]int{"Copper Ore": 22, "Copper Bar": 8}},
	{file: "rogue40", name: "Evalrogue-c", class: "ROGUE", race: "Gnome", level: 40,
		groups: []talentGroup{{"Assassination", 2}, {"Combat", 0}, {"Subtlety", 18}}, unspent: 3,
		mix:         questMix{turnZ1: true, turnNoZone: true, open: true, grey: true, unknownLevel: true, failed: true},
		gear:        []worn{{"chest", 904, "Worn Leather Tunic", -7}, {"mainhand", 906, "Worn Dagger", -7}},
		professions: []string{"Alchemy"}, bags: map[string]int{"Peacebloom": 5, "Empty Vial": 10}},
}

// lv is a Lua value: a map (table with string keys), a slice (list), a string, an int or a bool.
func lv(b *strings.Builder, v any, depth int) {
	pad := strings.Repeat("\t", depth)
	switch x := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		b.WriteString("{\n")
		for _, k := range keys {
			fmt.Fprintf(b, "%s\t[%q] = ", pad, k)
			lv(b, x[k], depth+1)
			b.WriteString(",\n")
		}
		b.WriteString(pad + "}")
	case []any:
		b.WriteString("{\n")
		for _, e := range x {
			b.WriteString(pad + "\t")
			lv(b, e, depth+1)
			b.WriteString(",\n")
		}
		b.WriteString(pad + "}")
	case string:
		fmt.Fprintf(b, "%q", x)
	case int:
		fmt.Fprint(b, x)
	case bool:
		fmt.Fprint(b, x)
	default:
		panic(fmt.Sprintf("lv: %T", v))
	}
}

func (s charSpec) lua() string {
	b := s.level
	key := "Evalrealm:" + s.name
	equip := map[string]any{}
	for _, w := range s.gear {
		e := map[string]any{"name": w.name, "ilvl": b + w.ilvl, "quality": 1}
		if w.n != 0 {
			e["itemID"] = b*1000 + w.n
		} else {
			e["itemID"] = 900000 + b*10 + len(equip)
		}
		equip[w.slot] = e
	}
	var items []any
	ids := 0
	names := make([]string, 0, len(s.bags))
	for n := range s.bags {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		ids++
		items = append(items, map[string]any{"itemID": 800000 + b*10 + ids, "count": s.bags[n], "name": n})
	}
	quest := func(id int, title string, level int, zone string, complete, failed bool) any {
		q := map[string]any{"questID": id, "title": title, "complete": complete, "failed": failed}
		if level != 0 {
			q["level"] = level
		}
		if zone != "" {
			q["zone"] = zone
		}
		return q
	}
	qb := b * 100
	var quests []any
	m := s.mix
	if m.turnZ1 {
		quests = append(quests, quest(qb+q1, "Eval Quest One", 0, "", true, false))
		if m.twoInZ1 {
			quests = append(quests, quest(qb+q2, "Eval Quest Two", 0, "", true, false))
		}
	}
	if m.turnZ2 {
		quests = append(quests, quest(qb+q6, "Eval Quest Six", b, zone2(b), true, false))
	}
	if m.turnNoZone {
		quests = append(quests, quest(qb+qUnknownZone, "Eval Quest Mystery Turn-in", b, "", true, false))
	}
	if m.open {
		quests = append(quests, quest(qb+q3, "Eval Quest Three", b+1, zone2(b), false, false))
	}
	if m.grey {
		quests = append(quests, quest(qb+q4, "Eval Quest Grey", b-8, zone1(b), false, false))
	}
	if m.hard {
		quests = append(quests, quest(qb+q5, "Eval Quest Hard", b+8, zone2(b), false, false))
	}
	if m.unknownLevel {
		quests = append(quests, quest(qb+qUnknownLevel, "Eval Quest Unknown Level", 0, "", false, false))
	}
	if m.failed {
		quests = append(quests, quest(qb+q7, "Eval Quest Failed", 0, "", false, true))
	}
	var groups, skills []any
	spent := 0
	for i, g := range s.groups {
		groups = append(groups, map[string]any{"name": g.name, "order": i + 1, "spent": g.spent})
		spent += g.spent
	}
	for i, p := range s.professions {
		skills = append(skills, map[string]any{"name": p, "skillID": 100 + i, "rank": 40, "max": 75, "group": "Professions"})
	}
	snap := map[string]any{
		"identity": map[string]any{"name": s.name, "realm": "Evalrealm", "class": s.class, "race": s.race, "faction": "Alliance",
			"client": map[string]any{"version": clientVersion, "build": clientBuild, "toc": clientBuild}},
		"level": b, "xp": 1000, "xpMax": 9000, "rested": 0, "money": 54321, "capturedAt": capturedAt, "addonVersion": "0.10.1",
		"equipment": equip,
		"bags":      map[string]any{"items": items},
		"quests":    quests,
		"skills":    skills,
		"talents": map[string]any{"system": "traits", "unspent": s.unspent, "trees": []any{map[string]any{
			"name": "Eval " + s.class, "points": spent, "currency": map[string]any{"spent": spent, "unspent": s.unspent}, "groups": groups}}},
	}
	rec := map[string]any{"key": key, "snapshot": snap}
	if s.bank {
		rec["bank"] = map[string]any{"scannedAt": capturedAt, "items": []any{map[string]any{"itemID": 700000 + b, "count": 1, "name": "Eval Bank Item"}}}
	}
	var out strings.Builder
	out.WriteString("-- Synthetic capture for the evaluation set (generated by eval/gen_test.go). Not a real character.\nForeverScribeDB = ")
	lv(&out, map[string]any{"schema": 1, "addonVersion": "0.10.1", "settings": map[string]any{}, "chars": map[string]any{key: rec}}, 0)
	out.WriteString("\n")
	return out.String()
}

// writeFixtures writes the synthetic dataset to dir/dataset and the captures to dir/captures.
func writeFixtures(dir string) error {
	for _, d := range []string{"captures", "dataset"} {
		if err := os.MkdirAll(filepath.Join(dir, d), 0o755); err != nil {
			return err
		}
	}
	for table, rows := range datasetTables() {
		if err := writeTable(filepath.Join(dir, "dataset", table+".parquet"), rows); err != nil {
			return err
		}
	}
	for _, s := range charSpecs {
		if err := os.WriteFile(filepath.Join(dir, "captures", s.file+".lua"), []byte(s.lua()), 0o644); err != nil {
			return err
		}
	}
	return nil
}

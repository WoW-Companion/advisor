package questie

import (
	"cmp"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Dungeon is one row of Zones/dungeons.lua plus what the index links to it: the creatures that
// spawn in its areas and the quests that send the player in.
type Dungeon struct {
	Area      int
	Name      string
	Alt       []int // alternative area ids of the same dungeon
	Parent    int   // the zone area the entrance stands in; 0 when unknown
	X, Y      float64
	Entrance  bool // X and Y hold the entrance
	Instance  int  // the instance (map) id, from instanceIdToAreaId.lua; 0 when unknown
	Creatures []int
	Quests    []int
}

// Areas is the dungeon's own area id and its alternatives.
func (d Dungeon) Areas() []int { return append([]int{d.Area}, d.Alt...) }

// luaTable is a parsed Lua table literal: positional values, and values stored under integer keys.
type luaTable struct {
	pos   []any
	keyed map[int]any
}

type luaParser struct {
	s string
	i int
}

func (p *luaParser) skip() {
	for p.i < len(p.s) {
		switch c := p.s[p.i]; {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			p.i++
		case strings.HasPrefix(p.s[p.i:], "--"):
			for p.i < len(p.s) && p.s[p.i] != '\n' {
				p.i++
			}
		default:
			return
		}
	}
}

// value parses a table, string, number or bare word (nil, true, a name); ok is false at a syntax
// this reader does not know.
func (p *luaParser) value(depth int) (any, bool) {
	p.skip()
	if p.i >= len(p.s) || depth > 8 {
		return nil, false
	}
	switch c := p.s[p.i]; {
	case c == '{':
		return p.table(depth + 1)
	case c == '"' || c == '\'':
		end := strings.IndexByte(p.s[p.i+1:], c)
		if end < 0 {
			return nil, false
		}
		v := p.s[p.i+1 : p.i+1+end]
		p.i += end + 2
		return v, true
	case c == '-' || c == '.' || c >= '0' && c <= '9':
		j := p.i + 1
		for j < len(p.s) && strings.IndexByte("0123456789.eE+-", p.s[j]) >= 0 {
			j++
		}
		f, err := strconv.ParseFloat(p.s[p.i:j], 64)
		if err != nil {
			return nil, false
		}
		p.i = j
		return f, true
	default:
		j := p.i
		for j < len(p.s) && (p.s[j] == '_' || p.s[j] >= 'a' && p.s[j] <= 'z' || p.s[j] >= 'A' && p.s[j] <= 'Z' || p.s[j] >= '0' && p.s[j] <= '9') {
			j++
		}
		if j == p.i {
			return nil, false
		}
		p.i = j
		return nil, true
	}
}

func (p *luaParser) table(depth int) (any, bool) {
	p.i++ // {
	t := &luaTable{keyed: map[int]any{}}
	for {
		p.skip()
		if p.i >= len(p.s) {
			return nil, false
		}
		if p.s[p.i] == '}' {
			p.i++
			return t, true
		}
		if p.s[p.i] == ',' || p.s[p.i] == ';' {
			p.i++
			continue
		}
		key := 0
		if p.s[p.i] == '[' {
			end := strings.IndexByte(p.s[p.i:], ']')
			if end < 0 {
				return nil, false
			}
			k, err := strconv.Atoi(strings.TrimSpace(p.s[p.i+1 : p.i+end]))
			if err != nil {
				return nil, false
			}
			key = k
			p.i += end + 1
			p.skip()
			if p.i >= len(p.s) || p.s[p.i] != '=' {
				return nil, false
			}
			p.i++
		}
		v, ok := p.value(depth)
		if !ok {
			return nil, false
		}
		if key != 0 {
			t.keyed[key] = v
		} else {
			t.pos = append(t.pos, v)
		}
	}
}

// parseLuaTable reads the first table literal in a Lua file, never running it.
func parseLuaTable(src string) (*luaTable, bool) {
	i := strings.IndexByte(src, '{')
	if i < 0 {
		return nil, false
	}
	p := &luaParser{s: src, i: i}
	v, ok := p.table(0)
	t, _ := v.(*luaTable)
	return t, ok && t != nil
}

func fnum(v any) (float64, bool) { f, ok := v.(float64); return f, ok }

// dungeonRow reads one row: a name, optionally the area id, its alternative areas, the parent zone
// and the entrance. The row's key is the area id unless the row names it itself (two plain numbers:
// area, then parent zone). A list of two numbers is the entrance (x, y); three is (zone, x, y); any
// other list of whole numbers is the alternative areas.
func dungeonRow(key int, row *luaTable) (Dungeon, bool) {
	d := Dungeon{Area: key}
	var scalars []int
	for _, v := range row.pos {
		switch x := v.(type) {
		case string:
			if d.Name == "" {
				d.Name = x
			}
		case float64:
			scalars = append(scalars, int(x))
		case *luaTable:
			nums := make([]float64, 0, len(x.pos))
			for _, e := range x.pos {
				f, ok := fnum(e)
				if !ok {
					nums = nil
					break
				}
				nums = append(nums, f)
			}
			switch {
			case len(nums) == 3 && !d.Entrance:
				d.Parent, d.X, d.Y, d.Entrance = int(nums[0]), nums[1], nums[2], true
			case len(nums) == 2 && !d.Entrance:
				d.X, d.Y, d.Entrance = nums[0], nums[1], true
			default:
				for _, f := range nums {
					d.Alt = append(d.Alt, int(f))
				}
			}
		}
	}
	switch {
	case len(scalars) >= 2:
		d.Area, d.Parent = scalars[0], scalars[1]
	case len(scalars) == 1 && d.Parent == 0 && key != 0:
		d.Parent = scalars[0]
	case len(scalars) == 1:
		d.Area = scalars[0]
	}
	return d, d.Area > 0 && d.Name != ""
}

// parseDungeons reads Zones/dungeons.lua as text. A missing or unrecognised file gives no dungeons
// and a reason, never an error that stops the rest of the index.
func parseDungeons(dir string) ([]Dungeon, string) {
	b, err := os.ReadFile(filepath.Join(dir, "dungeons.lua"))
	if err != nil {
		return nil, "no dungeons.lua in your QuestieDB"
	}
	return parseDungeonsText(string(b))
}

func parseDungeonsText(src string) ([]Dungeon, string) {
	t, ok := parseLuaTable(src)
	if !ok {
		return nil, "dungeons.lua is in a format this reader does not know"
	}
	var out []Dungeon
	add := func(key int, v any) {
		if row, ok := v.(*luaTable); ok {
			if d, ok := dungeonRow(key, row); ok {
				out = append(out, d)
			}
		}
	}
	for k, v := range t.keyed {
		add(k, v)
	}
	for _, v := range t.pos {
		add(0, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Area < out[j].Area })
	if len(out) == 0 {
		return nil, "dungeons.lua held no dungeon this reader could use"
	}
	return out, ""
}

// parseInstances reads Zones/instanceIdToAreaId.lua's "[instance] = area" lines.
func parseInstances(dir string) map[int]int {
	a := parseAreas(filepath.Join(dir, "instanceIdToAreaId.lua"))
	out := map[int]int{}
	for inst, v := range a {
		out[inst] = v.UIMap // the second number of the line is the area id here
	}
	return out
}

// spawnAreas is the area ids of a spawn field, without the cap spawns applies.
func spawnAreas(v any) []int {
	m, _ := v.(map[any]any)
	var out []int
	for k := range m {
		if a, ok := k.(int64); ok {
			out = append(out, int(a))
		}
	}
	return out
}

const (
	maxDungeonCreatures = 60
	maxDungeonQuests    = 30
)

// linkDungeons ties creatures, their drops and quests to each dungeon, keeping their rows in ix.
func (ix *Index) linkDungeons(dir string, lay Layout, npcs, itemRows map[int]rawRow) {
	var reason string
	ix.Dungeons, reason = parseDungeons(dir)
	ix.DungeonReason = reason
	if len(ix.Dungeons) == 0 {
		return
	}
	inst := parseInstances(dir)
	byArea := map[int][]int{} // area -> dungeon index
	for i := range ix.Dungeons {
		d := &ix.Dungeons[i]
		for in, area := range inst {
			if area == d.Area {
				d.Instance = in
			}
		}
		for _, a := range d.Areas() {
			byArea[a] = append(byArea[a], i)
		}
	}
	var inside = map[int][]int{} // dungeon -> npc ids
	for id, r := range npcs {
		for _, a := range spawnAreas(r[lay.at("npc", "spawns")]) {
			for _, di := range byArea[a] {
				inside[di] = append(inside[di], id)
			}
		}
	}
	rank := func(id int) int { return num(npcs[id].scalars()[lay.sc("npc", "rank")]) }
	kept := map[int]bool{}
	for di, ids := range inside {
		sort.Slice(ids, func(i, j int) bool {
			ri, rj := dungeonRank(rank(ids[i])), dungeonRank(rank(ids[j]))
			return ri > rj || ri == rj && ids[i] < ids[j]
		})
		if len(ids) > maxDungeonCreatures {
			ids = ids[:maxDungeonCreatures]
		}
		ix.Dungeons[di].Creatures = ids
		for _, id := range ids {
			kept[id] = true
			ix.Npcs[id] = buildEntity(npcs[id], lay, "npc")
		}
	}
	ix.Drops = map[int][]int{}
	for id, r := range itemRows {
		for _, n := range ints(r[lay.at("item", "npcDrops")]) {
			if kept[n] {
				ix.Drops[n] = append(ix.Drops[n], id)
				if _, ok := ix.Items[id]; !ok {
					ix.Items[id] = Item{Name: text(r.scalars()[lay.sc("item", "name")])}
				}
			}
		}
	}
	for n := range ix.Drops {
		sort.Ints(ix.Drops[n])
	}
	for di := range ix.Dungeons {
		d := &ix.Dungeons[di]
		in := map[int]bool{}
		for _, id := range d.Creatures {
			in[id] = true
		}
		drops := map[int]bool{}
		for _, id := range d.Creatures {
			for _, it := range ix.Drops[id] {
				drops[it] = true
			}
		}
		areas := map[int]bool{}
		for _, a := range d.Areas() {
			areas[a] = true
		}
		for id, q := range ix.Quests {
			hit := areas[q.Zone]
			for _, r := range q.Creatures {
				hit = hit || !r.Object && in[r.ID]
			}
			for _, c := range q.Credits {
				for _, n := range c.NPCs {
					hit = hit || in[n]
				}
			}
			for _, it := range q.Items {
				hit = hit || drops[it]
			}
			if hit {
				d.Quests = append(d.Quests, id)
			}
		}
		sort.Ints(d.Quests)
		if len(d.Quests) > maxDungeonQuests {
			d.Quests = d.Quests[:maxDungeonQuests]
		}
	}
}

// dungeonRank orders ranks for listing: boss, then rare elite, elite, the rest.
func dungeonRank(r int) int {
	switch r {
	case 3:
		return 4
	case 2:
		return 3
	case 1:
		return 2
	}
	return 1
}

// Creature is a creature inside a dungeon.
type Creature struct {
	ID    int
	Name  string
	Level int
	Elite bool
	Boss  bool
}

// Inside is what the index knows about one dungeon.
type Inside struct {
	Name      string
	Zone      string // where the entrance is, the parent zone's name
	X, Y      float64
	Entrance  bool
	Creatures []Creature
	Quests    []*Quest
}

// FindDungeon is the dungeon for an instance (map) id, else the one named name (exactly, else by
// containment either way, case-insensitive). nil when there is none.
func (ix *Index) FindDungeon(instance int, name string) *Dungeon {
	if ix == nil {
		return nil
	}
	if instance != 0 {
		for i := range ix.Dungeons {
			if ix.Dungeons[i].Instance == instance {
				return &ix.Dungeons[i]
			}
		}
	}
	n := strings.ToLower(strings.TrimSpace(name))
	if n == "" {
		return nil
	}
	var part *Dungeon
	for i := range ix.Dungeons {
		d := &ix.Dungeons[i]
		dn := strings.ToLower(d.Name)
		if dn == n {
			return d
		}
		if part == nil && (strings.Contains(dn, n) || strings.Contains(n, dn)) {
			part = d
		}
	}
	return part
}

// Inside lists what lives in the dungeon and which quests send you there.
func (ix *Index) Inside(d *Dungeon) Inside {
	out := Inside{Name: d.Name, X: d.X, Y: d.Y, Entrance: d.Entrance}
	if a, ok := ix.Areas[d.Parent]; ok {
		out.Zone = cmp.Or(a.Sub, a.Zone)
	}
	for _, id := range d.Creatures {
		e := ix.Npcs[id]
		if e.Name == "" {
			continue
		}
		out.Creatures = append(out.Creatures, Creature{ID: id, Name: e.Name, Level: e.Level, Boss: e.Rank == 3, Elite: e.Rank == 1 || e.Rank == 2})
	}
	for _, id := range d.Quests {
		if q := ix.Quests[id]; q != nil && q.Name != "" {
			out.Quests = append(out.Quests, q)
		}
	}
	return out
}

// DropsOf is the items the creature drops, by id.
func (ix *Index) DropsOf(npc int) []int { return ix.Drops[npc] }

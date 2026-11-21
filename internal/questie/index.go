package questie

import (
	"bufio"
	"crypto/sha256"
	"encoding/gob"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const indexSchema = 1 // bump when the cached index's shape changes

// Spawn is a place, as percent of the area's map.
type Spawn struct {
	Area int
	X, Y float64
}

type Entity struct {
	Name   string
	Sub    string // the NPC's subname, such as a title
	Spawns []Spawn
}

type Ref struct {
	Object bool
	ID     int
}

// Quest is the part of a quest row the advisor uses.
type Quest struct {
	ID         int
	Name       string
	Level      int
	MinLevel   int
	MaxLevel   int // 0: none
	Races      uint64
	Classes    uint64
	PreSingle  []int
	PreGroup   []int
	Exclusive  []int
	Next       int // quest that makes this one moot once started or completed
	Parent     int
	Until      int // quest whose completion ends availability
	Disabled   int // quest that disables this one while it is in the log
	Starters   []Ref
	Finishers  []Ref
	Creatures  []Ref // objectives that are kills or talks
	Restricted bool  // a requirement this reader cannot check (skill, reputation, spell, event)
}

type Area struct {
	UIMap int
	Sub   string // the subzone's own name; empty for a zone
	Zone  string
}

type Index struct {
	Schema  int
	DB      string
	Quests  map[int]*Quest
	Npcs    map[int]Entity
	Objects map[int]Entity
	Areas   map[int]Area
}

func num(v any) int {
	switch x := v.(type) {
	case int64:
		return int(x)
	case float64:
		return int(x)
	}
	return 0
}

func mask(v any) uint64 {
	n, _ := v.(int64)
	return uint64(n) & 0xFFFFFFFF // the high bits mark a faction, not a race
}

func ints(v any) []int {
	a, _ := v.([]any)
	var out []int
	for _, e := range a {
		if n := num(e); n != 0 {
			out = append(out, n)
		}
	}
	return out
}

// slot returns element i (1-based) of a positional list that may be stored as an array or as a map
// of the filled positions.
func slot(v any, i int) any {
	switch x := v.(type) {
	case []any:
		if i-1 < len(x) {
			return x[i-1]
		}
	case map[any]any:
		return x[int64(i)]
	}
	return nil
}

func refs(list any, object bool) []Ref {
	var out []Ref
	for _, id := range ints(list) {
		out = append(out, Ref{object, id})
	}
	return out
}

func text(v any) string { s, _ := v.(string); return s }

func spawns(v any) []Spawn {
	m, _ := v.(map[any]any)
	var out []Spawn
	for k, pts := range m {
		area, ok := k.(int64)
		if !ok {
			continue
		}
		pa, _ := pts.([]any)
		for _, p := range pa {
			xy, _ := p.([]any)
			if len(xy) < 2 {
				continue
			}
			x, ok1 := xy[0].(float64)
			y, ok2 := xy[1].(float64)
			if !ok1 || !ok2 {
				if a, b := num(xy[0]), num(xy[1]); a != 0 || b != 0 {
					x, y = float64(a), float64(b)
				} else {
					continue
				}
			}
			out = append(out, Spawn{int(area), x, y})
		}
	}
	sort.Slice(out, func(i, j int) bool { // a stable order keeps the cache reproducible
		a, b := out[i], out[j]
		return a.Area < b.Area || a.Area == b.Area && (a.X < b.X || a.X == b.X && a.Y < b.Y)
	})
	if len(out) > maxSpawns {
		out = out[:maxSpawns]
	}
	return out
}

const maxSpawns = 64

func buildQuest(id int, r rawRow) *Quest {
	s := r.scalars()
	q := &Quest{ID: id, Name: text(s[int64(1)]), MinLevel: num(s[int64(4)]), Level: num(s[int64(5)]),
		Races: mask(s[int64(6)]), Classes: mask(s[int64(7)]), Next: num(s[int64(22)])}
	q.Parent, q.Until, q.Disabled = num(s[int64(25)]), num(s[int64(33)]), num(s[int64(36)])
	q.MaxLevel = num(s[int64(32)])
	// A skill, reputation, spell, specialisation, rank, availability-by-quest or special flag.
	for _, f := range []int64{18, 19, 20, 24, 30, 31, 34, 35} {
		if v, ok := s[f]; ok && v != nil && v != int64(0) {
			q.Restricted = true
		}
	}
	for t, list := range []any{slot(r["2"], 1), slot(r["2"], 2)} {
		q.Starters = append(q.Starters, refs(list, t == 1)...)
	}
	for t, list := range []any{slot(r["3"], 1), slot(r["3"], 2)} {
		q.Finishers = append(q.Finishers, refs(list, t == 1)...)
	}
	q.PreGroup, q.PreSingle, q.Exclusive = ints(r["12"]), ints(r["13"]), ints(r["16"])
	if o := r["10"]; o != nil {
		for _, row := range asList(slot(o, 1)) {
			if id := num(slot(row, 1)); id != 0 {
				q.Creatures = append(q.Creatures, Ref{false, id})
			}
		}
		for _, row := range asList(slot(o, 2)) {
			if id := num(slot(row, 1)); id != 0 {
				q.Creatures = append(q.Creatures, Ref{true, id})
			}
		}
	}
	return q
}

func asList(v any) []any { a, _ := v.([]any); return a }

func buildEntity(r rawRow, spawnField string) Entity {
	s := r.scalars()
	return Entity{Name: text(s[int64(1)]), Sub: text(s[int64(14)]), Spawns: spawns(r[spawnField])}
}

var areaLine = regexp.MustCompile(`^\s*\[(\d+)\]\s*=\s*(\d+),?\s*(?:--\s*(.*?))?\s*$`)

// parseAreas reads Zones/areaIdToUiMapId.lua's "[area] = uiMap, -- Name -> Zone" lines. The file is
// read as text, never run. Display names come from the comments, which only the main table has.
func parseAreas(path string) map[int]Area {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	out := map[int]Area{}
	named := false
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		l := sc.Text()
		if strings.Contains(l, "areaIdToUiMap") && strings.Contains(l, "= [[") {
			named = strings.Contains(l, "areaIdToUiMapId = [[")
			continue
		}
		m := areaLine.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		id, _ := strconv.Atoi(m[1])
		ui, _ := strconv.Atoi(m[2])
		a := Area{UIMap: ui}
		if named {
			if sub, zone, ok := strings.Cut(m[3], " -> "); ok {
				a.Sub, a.Zone = sub, zone
			} else {
				a.Zone = m[3]
			}
		}
		out[id] = a
	}
	return out
}

// Parse reads the baked TOC at st.TOC and the zone tables beside it. It returns ErrUnreadable
// (wrapped) when the rows do not decode as expected.
func Parse(st Status) (*Index, error) {
	f, err := os.Open(st.TOC)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	ix := &Index{Schema: indexSchema, DB: st.DB,
		Quests: map[int]*Quest{}, Npcs: map[int]Entity{}, Objects: map[int]Entity{}}
	quests, npcs, objects := map[int]rawRow{}, map[int]rawRow{}, map[int]rawRow{}
	good, bad := 0, 0
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	type key struct {
		typ   string
		id    int
		field string
	}
	pending := map[key][]string{} // long values arrive as a "~N~" head and N chunk lines
	store := func(k key, val string) {
		rows := map[string]map[int]rawRow{"Quest": quests, "Npc": npcs, "Object": objects}[k.typ]
		v, err := decodeField(val)
		if err != nil {
			bad++
			return
		}
		good++
		if rows[k.id] == nil {
			rows[k.id] = rawRow{}
		}
		rows[k.id][k.field] = v
	}
	for sc.Scan() {
		typ, id, field, chunk, val, ok := parseLine(sc.Text())
		if !ok || !wanted[typ][field] {
			continue
		}
		k := key{typ, id, field}
		n, _ := strconv.Atoi(strings.Trim(val, "~"))
		switch {
		case chunk > 0:
			if parts := pending[k]; chunk <= len(parts) {
				parts[chunk-1] = val
			}
		case strings.HasPrefix(val, "~") && n > 0 && n <= 64:
			pending[k] = make([]string, n)
		default:
			store(k, val)
		}
	}
	for k, parts := range pending {
		store(k, strings.Join(parts, ""))
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(quests) == 0 || bad*10 > good+bad {
		return nil, fmt.Errorf("%w: %d rows decoded, %d did not", ErrUnreadable, good, bad)
	}
	named := 0
	for id, r := range quests {
		q := buildQuest(id, r)
		if q.Name != "" {
			named++
		}
		ix.Quests[id] = q
	}
	if named*2 < len(quests) {
		return nil, fmt.Errorf("%w: most quests have no name", ErrUnreadable)
	}
	// Keep only the NPCs and objects a quest names.
	for _, q := range ix.Quests {
		for _, group := range [][]Ref{q.Starters, q.Finishers, q.Creatures} {
			for _, r := range group {
				if r.Object {
					if row, ok := objects[r.ID]; ok {
						ix.Objects[r.ID] = buildEntity(row, "4")
					}
				} else if row, ok := npcs[r.ID]; ok {
					ix.Npcs[r.ID] = buildEntity(row, "7")
				}
			}
		}
	}
	zones := filepath.Join(filepath.Dir(st.TOC), "support", "Forever", "Zones", "areaIdToUiMapId.lua")
	ix.Areas = parseAreas(zones)
	return ix, nil
}

// CacheDir is where the parsed index is kept: the player's local cache folder, never the repo.
// Tests point it at a temp folder.
var CacheDir = func() string {
	d, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(d, "wowc-advisor", "questie")
}

func cacheFile(st Status, fi os.FileInfo) string {
	dir := CacheDir()
	if dir == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d|%s|%d|%d|%s", indexSchema, st.TOC, fi.Size(), fi.ModTime().UnixNano(), st.DB)))
	return filepath.Join(dir, "index-"+hex.EncodeToString(sum[:8])+".gob")
}

// Load returns the index for the installed QuestieDB: from the local cache, else a
// fresh parse that is then cached. A cache that cannot be read or written is ignored.
func Load(st Status) (*Index, error) {
	if !st.Readable {
		return nil, fmt.Errorf("%w: %s", ErrUnreadable, st.Reason)
	}
	fi, err := os.Stat(st.TOC)
	if err != nil {
		return nil, err
	}
	cf := cacheFile(st, fi)
	if cf != "" {
		if ix := readCache(cf); ix != nil {
			return ix, nil
		}
	}
	ix, err := Parse(st)
	if err != nil {
		return nil, err
	}
	if cf != "" {
		writeCache(cf, ix)
	}
	return ix, nil
}

func readCache(path string) *Index {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var ix Index
	if gob.NewDecoder(f).Decode(&ix) != nil || ix.Schema != indexSchema {
		return nil
	}
	return &ix
}

func writeCache(path string, ix *Index) {
	dir := filepath.Dir(path)
	if os.MkdirAll(dir, 0o700) != nil {
		return
	}
	old, _ := filepath.Glob(filepath.Join(dir, "index-*.gob"))
	for _, o := range old {
		os.Remove(o) // an older QuestieDB build's index
	}
	tmp, err := os.CreateTemp(dir, "index-*.tmp")
	if err != nil {
		return
	}
	err = gob.NewEncoder(tmp).Encode(ix)
	if e := tmp.Close(); err == nil {
		err = e
	}
	if err != nil || os.Rename(tmp.Name(), path) != nil {
		os.Remove(tmp.Name())
	}
}

// OpenGame probes the game folder and loads its index. A nil index with a non-nil Status is the
// normal "no Questie" or "unreadable" case.
func OpenGame(game string) (*Index, Status) {
	st := Probe(game)
	if !st.Readable {
		return nil, st
	}
	ix, err := Load(st)
	if err != nil {
		st.Readable, st.Reason = false, "could not read it ("+err.Error()+")"
		return nil, st
	}
	return ix, st
}

// GameFromSavedVariables derives the game folder from a ForeverScribe.lua path,
// <game>/WTF/Account/<account>/SavedVariables/<file>; "" when the path has another shape.
func GameFromSavedVariables(sv string) string {
	d := filepath.Dir(filepath.Clean(sv))
	for _, want := range []string{"SavedVariables", "", "Account", "WTF"} {
		if want != "" && filepath.Base(d) != want {
			return ""
		}
		d = filepath.Dir(d)
	}
	return d
}

// ForSavedVariables is the index for the game folder a ForeverScribe.lua path sits in, or nil when
// there is no readable Questie there (the normal case). It never fails.
func ForSavedVariables(sv string) *Index {
	if g := GameFromSavedVariables(sv); g != "" {
		ix, _ := OpenGame(g)
		return ix
	}
	return nil
}

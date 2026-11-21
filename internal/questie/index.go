package questie

import (
	"bufio"
	"compress/gzip"
	"crypto/sha256"
	"encoding/gob"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// indexSchema changes whenever the cached index's shape does.
const indexSchema = 1

// Spawn is a place, as percent of the area's map.
type Spawn struct {
	Area int
	X, Y float64
}

// Entity is a quest-giver NPC or object.
type Entity struct {
	Name   string
	Sub    string // the NPC's subname, such as a title
	Spawns []Spawn
}

// Ref names an NPC or an object.
type Ref struct {
	Object bool
	ID     int
}

// Quest is the part of a quest row the advisor uses.
type Quest struct {
	ID        int
	Name      string
	Level     int
	MinLevel  int
	MaxLevel  int // 0: none
	Races     uint64
	Classes   uint64
	Zone      int // area id; negative is a quest sort
	PreSingle []int
	PreGroup  []int
	Exclusive []int
	Next      int // quest that makes this one moot once started or completed
	Parent    int
	Until     int // quest whose completion ends availability
	Disabled  int // quest that disables this one while it is in the log
	Starters  []Ref
	Finishers []Ref
	Creatures []Ref // objectives that are kills or talks
	Trigger   []Spawn
	// Restricted marks a requirement this reader cannot check (a skill, reputation, spell, holiday
	// or similar); such a quest is never suggested.
	Restricted bool
}

// Area is a Forever area id with its map and display names.
type Area struct {
	UIMap int
	Sub   string // the subzone's own name; empty for a zone
	Zone  string
}

// Index is everything read from one QuestieDB build.
type Index struct {
	Schema  int
	TOC     string
	DB      string
	Flavor  string
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

func spawns(v any, limit int) []Spawn {
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
	sortSpawns(out)
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func sortSpawns(s []Spawn) {
	for i := 1; i < len(s); i++ { // short lists; a stable order keeps the cache reproducible
		for j := i; j > 0 && less(s[j], s[j-1]); j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

func less(a, b Spawn) bool {
	if a.Area != b.Area {
		return a.Area < b.Area
	}
	if a.X != b.X {
		return a.X < b.X
	}
	return a.Y < b.Y
}

const maxSpawns = 64

func buildQuest(id int, r rawRow) *Quest {
	s := r.scalars()
	q := &Quest{ID: id, Name: text(s[int64(1)]), MinLevel: num(s[int64(4)]), Level: num(s[int64(5)]),
		Races: mask(s[int64(6)]), Classes: mask(s[int64(7)]), Zone: num(s[int64(17)]), Next: num(s[int64(22)])}
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
	if t := r["9"]; t != nil {
		q.Trigger = spawns(slot(t, 2), maxSpawns)
	}
	return q
}

func asList(v any) []any { a, _ := v.([]any); return a }

func buildEntity(r rawRow, spawnField string) Entity {
	s := r.scalars()
	return Entity{Name: text(s[int64(1)]), Sub: text(s[int64(14)]), Spawns: spawns(r[spawnField], maxSpawns)}
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
	ix := &Index{Schema: indexSchema, TOC: st.TOC, DB: st.DB, Flavor: st.Flavor,
		Quests: map[int]*Quest{}, Npcs: map[int]Entity{}, Objects: map[int]Entity{}}
	quests, npcs, objects := map[int]rawRow{}, map[int]rawRow{}, map[int]rawRow{}
	good, bad := 0, 0
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for sc.Scan() {
		typ, id, field, val, ok := parseLine(sc.Text())
		if !ok {
			continue
		}
		var rows map[int]rawRow
		switch {
		case typ == "Quest" && questFields[field]:
			rows = quests
		case typ == "Npc" && npcFields[field]:
			rows = npcs
		case typ == "Object" && objectFields[field]:
			rows = objects
		default:
			continue
		}
		v, err := decodeField(val)
		if err != nil {
			bad++
			continue
		}
		good++
		if rows[id] == nil {
			rows[id] = rawRow{}
		}
		rows[id][field] = v
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
	return filepath.Join(dir, "index-"+hex.EncodeToString(sum[:8])+".gob.gz")
}

var (
	memoMu sync.Mutex
	memo   = map[string]*Index{}
)

// Load returns the index for the installed QuestieDB: from memory, else the local cache, else a
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
	memoMu.Lock()
	defer memoMu.Unlock()
	if ix := memo[cf]; ix != nil && cf != "" {
		return ix, nil
	}
	if cf != "" {
		if ix := readCache(cf); ix != nil {
			memo[cf] = ix
			return ix, nil
		}
	}
	ix, err := Parse(st)
	if err != nil {
		return nil, err
	}
	if cf != "" {
		writeCache(cf, ix)
		memo[cf] = ix
	}
	return ix, nil
}

func readCache(path string) *Index {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return nil
	}
	var ix Index
	if gob.NewDecoder(zr).Decode(&ix) != nil || ix.Schema != indexSchema {
		return nil
	}
	return &ix
}

func writeCache(path string, ix *Index) {
	dir := filepath.Dir(path)
	if os.MkdirAll(dir, 0o700) != nil {
		return
	}
	if old, _ := filepath.Glob(filepath.Join(dir, "index-*.gob.gz")); len(old) > 0 {
		for _, o := range old {
			os.Remove(o) // an older QuestieDB build's index
		}
	}
	tmp, err := os.CreateTemp(dir, "index-*.tmp")
	if err != nil {
		return
	}
	zw := gzip.NewWriter(tmp)
	err = gob.NewEncoder(zw).Encode(ix)
	if e := zw.Close(); err == nil {
		err = e
	}
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
// <game>/WTF/Account/<account>/SavedVariables/<file>.
func GameFromSavedVariables(sv string) string {
	d := filepath.Dir(filepath.Clean(sv))
	if filepath.Base(d) != "SavedVariables" {
		return ""
	}
	g := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(d))))
	if filepath.Base(filepath.Dir(filepath.Dir(d))) != "Account" || filepath.Base(filepath.Dir(filepath.Dir(filepath.Dir(d)))) != "WTF" {
		return ""
	}
	return g
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

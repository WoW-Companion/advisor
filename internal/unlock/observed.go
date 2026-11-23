package unlock

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/lua"
)

// Event kinds in WoWCompanionDB.observed.events.
const (
	EvAccepted  = "accepted"
	EvTurnedIn  = "turnedin"
	EvAbandoned = "abandoned"
)

// MaxEvents is the most events read; the newest are kept.
const MaxEvents = 400

// LearnWindow is the seconds after a turn-in within which an acceptance counts as unlocked by it.
const LearnWindow = 120

// LearnRadius is how close (map percent, same map) the acceptance must be to the turn-in.
const LearnRadius = 12.0

// Event is one entry of the addon's bounded observation log.
type Event struct {
	T         int64
	Kind      string
	QuestID   int
	Character string
	Map       int
	X, Y      float64
	NPC       string
	OffPlan   bool // an acceptance of a quest the plan did not list
}

func asInt(v any) int {
	switch n := v.(type) {
	case int64:
		return int(n)
	case float64:
		return int(n)
	}
	return 0
}

func asFloat(v any) float64 {
	switch n := v.(type) {
	case int64:
		return float64(n)
	case float64:
		return n
	}
	return 0
}

func asStr(v any) string { s, _ := v.(string); return s }

// ParseEvents reads WoWCompanionDB.observed.events out of a SavedVariables file's text. Entries with
// an unknown kind or no quest id are skipped; at most MaxEvents of the newest are returned, oldest
// first.
func ParseEvents(text string) []Event {
	vars, err := lua.Parse(text)
	if err != nil {
		return nil
	}
	db, _ := vars["WoWCompanionDB"].(lua.Table)
	obs, _ := db["observed"].(lua.Table)
	var out []Event
	for _, e := range lua.List(obs["events"]) {
		t, ok := e.(lua.Table)
		if !ok {
			continue
		}
		ev := Event{T: int64(asInt(t["t"])), Kind: strings.ToLower(asStr(t["ev"])), QuestID: asInt(t["questID"]),
			Character: asStr(t["character"]), Map: asInt(t["map"]), X: asFloat(t["x"]), Y: asFloat(t["y"]), NPC: asStr(t["npc"])}
		ev.OffPlan, _ = t["offPlan"].(bool)
		if ev.QuestID <= 0 || (ev.Kind != EvAccepted && ev.Kind != EvTurnedIn && ev.Kind != EvAbandoned) {
			continue
		}
		out = append(out, ev)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].T < out[j].T })
	if len(out) > MaxEvents {
		out = out[len(out)-MaxEvents:]
	}
	return out
}

func forCharacter(c character.Character, ev Event) bool {
	return ev.Character == "" || strings.EqualFold(ev.Character, c.Name) || strings.EqualFold(ev.Character, c.Key)
}

// Apply returns c with the events newer than its capture applied: a quest accepted since is in the
// log (marked Observed, so its objectives are routed from Questie), one turned in is done, one
// abandoned is gone. Events for other characters are ignored; c itself is not changed.
func Apply(c character.Character, events []Event) character.Character {
	var since int64
	if c.CapturedAt != nil {
		since = *c.CapturedAt
	}
	var log []character.Quest
	log = append(log, c.Quests...)
	done := append([]int(nil), c.CompletedQuests...)
	has := func(id int) int {
		for i, q := range log {
			if q.QuestID == id {
				return i
			}
		}
		return -1
	}
	changed := false
	for _, ev := range events {
		if !forCharacter(c, ev) || ev.T <= since {
			continue
		}
		i := has(ev.QuestID)
		switch ev.Kind {
		case EvAccepted:
			if i < 0 {
				log = append(log, character.Quest{QuestID: ev.QuestID, Observed: true})
				changed = true
			}
		case EvTurnedIn:
			if i >= 0 {
				log = append(log[:i], log[i+1:]...)
			}
			done = append(done, ev.QuestID)
			changed = true
		case EvAbandoned:
			if i >= 0 {
				log = append(log[:i], log[i+1:]...)
				changed = true
			}
		}
	}
	if changed {
		c.Quests, c.CompletedQuests = log, done
	}
	return c
}

// Learn finds the unlocks in events: a turn-in of A followed within LearnWindow seconds, on the same
// map and near it, by the acceptance of B with a known place.
func Learn(events []Event) []Obs {
	var out []Obs
	for i, a := range events {
		if a.Kind != EvTurnedIn {
			continue
		}
		for _, b := range events[i+1:] {
			if b.T-a.T > LearnWindow {
				break
			}
			if b.Kind != EvAccepted || b.QuestID == a.QuestID || b.Character != a.Character || b.Map == 0 || b.Map != a.Map {
				continue
			}
			if math.Hypot(a.X-b.X, a.Y-b.Y) > LearnRadius {
				continue
			}
			out = append(out, Obs{From: a.QuestID, To: b.QuestID, NPC: b.NPC, Map: b.Map, X: b.X, Y: b.Y, Count: 1, Last: b.T})
		}
	}
	return out
}

// Store is the local file of observed unlocks. It stays on the player's machine.
type Store struct {
	Path string
	Obs  []Obs
}

type fileObs struct {
	From  int     `json:"from"`
	To    int     `json:"to"`
	NPC   string  `json:"npc,omitempty"`
	Map   int     `json:"map"`
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	Count int     `json:"count"`
	Last  int64   `json:"last"`
}

type fileDoc struct {
	Version int       `json:"version"`
	Unlocks []fileObs `json:"unlocks"`
}

// Env names a file to use instead of the default path.
const Env = "WOWC_OBSERVED_UNLOCKS"

// LocalPath is the default file: observed-unlocks.json beside the play-style profiles.
func LocalPath() string {
	if p := os.Getenv(Env); p != "" {
		return p
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "wowc-advisor", "observed-unlocks.json")
}

// Open reads the file; a missing or unreadable one is an empty store.
func Open(path string) *Store {
	s := &Store{Path: path}
	if path == "" {
		return s
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return s
	}
	var d fileDoc
	if json.Unmarshal(b, &d) != nil {
		return s
	}
	for _, o := range d.Unlocks {
		s.Obs = append(s.Obs, Obs{From: o.From, To: o.To, NPC: o.NPC, Map: o.Map, X: o.X, Y: o.Y, Count: o.Count, Last: o.Last})
	}
	return s
}

// Merge adds learned unlocks, newest place winning, and reports whether anything changed. An
// observation already counted (same Last) is not counted twice.
func (s *Store) Merge(found []Obs) bool {
	changed := false
	for _, f := range found {
		hit := -1
		for i, o := range s.Obs {
			if o.From == f.From && o.To == f.To {
				hit = i
				break
			}
		}
		if hit < 0 {
			s.Obs = append(s.Obs, f)
			changed = true
			continue
		}
		if o := &s.Obs[hit]; f.Last > o.Last {
			o.Count += f.Count
			o.Last, o.Map, o.X, o.Y = f.Last, f.Map, f.X, f.Y
			if f.NPC != "" {
				o.NPC = f.NPC
			}
			changed = true
		}
	}
	return changed
}

// Save writes the file atomically; it does nothing without a path.
func (s *Store) Save() error {
	if s.Path == "" {
		return nil
	}
	d := fileDoc{Version: 1}
	sort.Slice(s.Obs, func(i, j int) bool {
		if s.Obs[i].From != s.Obs[j].From {
			return s.Obs[i].From < s.Obs[j].From
		}
		return s.Obs[i].To < s.Obs[j].To
	})
	for _, o := range s.Obs {
		d.Unlocks = append(d.Unlocks, fileObs{o.From, o.To, o.NPC, o.Map, o.X, o.Y, o.Count, o.Last})
	}
	b, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.Path), ".observed-*.tmp")
	if err != nil {
		return err
	}
	_, werr := tmp.Write(b)
	cerr := tmp.Close()
	if werr != nil || cerr != nil {
		os.Remove(tmp.Name())
		if werr != nil {
			return werr
		}
		return cerr
	}
	return os.Rename(tmp.Name(), s.Path)
}

// AddonSV is the addon's SavedVariables file beside ForeverScribe.lua; empty when the path has no
// folder.
func AddonSV(foreverScribe string) string {
	cut := strings.LastIndexAny(foreverScribe, `/\`)
	if cut < 0 {
		return ""
	}
	return foreverScribe[:cut+1] + "WoWCompanion.lua"
}

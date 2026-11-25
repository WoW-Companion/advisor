package character

import (
	"os"
	"sort"
	"strings"

	"github.com/WoW-Companion/advisor/internal/lua"
)

// SeenByYou is the mark on a fact that came from what the player saw in game
// (WoWCompanionDB.observed, docs/observed-format.md of the addon).
const SeenByYou = "seen by you"

// SeenOffer is one thing a trainer teaches, as the trainer window showed it.
type SeenOffer struct {
	Name       string
	Rank       string
	State      string // available, unavailable or used (already known)
	Cost       int64  // copper
	Skill      string // the skill gate's profession, "" when none
	SkillLevel int
	Spell      int64
}

// SeenTrainer is one trainer the player opened.
type SeenTrainer struct {
	NPC     int64
	Name    string
	Map     int
	X, Y    float64
	Faction string
	T       int64
	Offers  []SeenOffer
}

// SeenStock is one item a vendor sold, with its price in copper.
type SeenStock struct {
	ItemID int64
	Price  int64
	Ext    bool // also costs tokens or items
}

// SeenVendor is one vendor the player opened.
type SeenVendor struct {
	NPC     int64
	Name    string
	Map     int
	X, Y    float64
	Faction string
	T       int64
	Items   []SeenStock
}

// SeenGather is one aggregated gathering or fishing row: where, what and how much.
type SeenGather struct {
	Kind               string // herb, mine, skin or fish
	Source             string
	Map                int
	CX, CY             int // the cell: floor(percent / 5)
	N                  int
	T                  int64
	SkillMin, SkillMax int
	Items              map[int64]int // item id -> total looted
}

// SeenGiver is where the player saw a quest's giver, from an accepted event.
type SeenGiver struct {
	QuestID int
	NPC     string
	NPCID   int64
	Map     int
	X, Y    float64
	T       int64
}

// Observed is what the player saw, read from the addon's SavedVariables. Every list may be empty.
type Observed struct {
	Trainers []SeenTrainer
	Vendors  []SeenVendor
	Gather   []SeenGather
	Givers   []SeenGiver
}

// Empty reports whether nothing was observed.
func (o *Observed) Empty() bool {
	return o == nil || len(o.Trainers) == 0 && len(o.Vendors) == 0 && len(o.Gather) == 0 && len(o.Givers) == 0
}

func fl(v any) float64 {
	if p := asFloat(v); p != nil {
		return *p
	}
	if p := asInt(v); p != nil {
		return float64(*p)
	}
	return 0
}

func i0(v any) int { return derefInt(asInt(v)) }

func i640(v any) int64 {
	if p := asInt64(v); p != nil {
		return *p
	}
	return 0
}

// ParseObserved reads WoWCompanionDB.observed out of parsed variables: trainers, vendors,
// gathering and the quest givers of accepted events (the newest sighting of each quest wins).
// It is tolerant: a missing section or field is empty.
func ParseObserved(vars map[string]any) *Observed {
	db := table(vars[WoWCVariable])
	obs := table(db["observed"])
	if obs == nil {
		return nil
	}
	o := &Observed{}
	o.Trainers, _ = each(obs["trainers"], func(t lua.Table) (SeenTrainer, bool) {
		tr := SeenTrainer{NPC: i640(t["npc"]), Name: asStr(t["name"]), Map: i0(t["map"]), X: fl(t["x"]), Y: fl(t["y"]),
			Faction: asStr(t["faction"]), T: i640(t["t"])}
		tr.Offers, _ = each(t["offers"], func(e lua.Table) (SeenOffer, bool) {
			return SeenOffer{Name: asStr(e["name"]), Rank: asStr(e["rank"]), State: asStr(e["state"]), Cost: i640(e["cost"]),
				Skill: asStr(e["skill"]), SkillLevel: i0(e["skillLevel"]), Spell: i640(e["spell"])}, asStr(e["name"]) != "" || i640(e["spell"]) != 0
		})
		return tr, tr.Name != "" || tr.NPC != 0
	})
	o.Vendors, _ = each(obs["vendors"], func(t lua.Table) (SeenVendor, bool) {
		v := SeenVendor{NPC: i640(t["npc"]), Name: asStr(t["name"]), Map: i0(t["map"]), X: fl(t["x"]), Y: fl(t["y"]),
			Faction: asStr(t["faction"]), T: i640(t["t"])}
		v.Items, _ = each(t["items"], func(e lua.Table) (SeenStock, bool) {
			id := i640(e["id"])
			ext := asBool(e["ext"])
			return SeenStock{ItemID: id, Price: i640(e["price"]), Ext: ext != nil && *ext}, id != 0
		})
		return v, v.Name != "" || v.NPC != 0
	})
	o.Gather, _ = each(obs["gathering"], func(t lua.Table) (SeenGather, bool) {
		g := SeenGather{Kind: asStr(t["kind"]), Source: asStr(t["source"]), Map: i0(t["map"]), CX: i0(t["cx"]), CY: i0(t["cy"]),
			N: i0(t["n"]), T: i640(t["t"]), SkillMin: i0(t["skillMin"]), SkillMax: i0(t["skillMax"]), Items: map[int64]int{}}
		for k, v := range table(t["items"]) {
			if id := asInt64(k); id != nil {
				c := i0(table(v)["count"])
				if c == 0 {
					c = 1
				}
				g.Items[*id] = c
			}
		}
		return g, g.Kind != ""
	})
	latest := map[int]SeenGiver{}
	for _, e := range lua.List(obs["events"]) {
		t := table(e)
		if asStr(t["ev"]) != "accepted" || asStr(t["npc"]) == "" || t["npcX"] == nil || t["npcY"] == nil {
			continue
		}
		g := SeenGiver{QuestID: i0(t["questID"]), NPC: asStr(t["npc"]), NPCID: i640(t["npcId"]), Map: i0(t["npcMap"]),
			X: fl(t["npcX"]), Y: fl(t["npcY"]), T: i640(t["t"])}
		if g.QuestID != 0 && g.T >= latest[g.QuestID].T {
			latest[g.QuestID] = g
		}
	}
	for _, g := range latest {
		o.Givers = append(o.Givers, g)
	}
	sort.Slice(o.Givers, func(i, j int) bool { return o.Givers[i].QuestID < o.Givers[j].QuestID })
	return o
}

// factionOpen reports whether a trainer or vendor of the faction (as the addon records it) serves a
// character of faction char; an empty one serves everyone.
func factionOpen(npc, char string) bool {
	return npc == "" || char == "" || strings.EqualFold(npc, char)
}

// TrainersFor lists the seen trainers open to the character's faction, newest first.
func (o *Observed) TrainersFor(faction string) []SeenTrainer {
	if o == nil {
		return nil
	}
	var out []SeenTrainer
	for _, t := range o.Trainers {
		if factionOpen(t.Faction, faction) {
			out = append(out, t)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].T > out[j].T })
	return out
}

// VendorsOf lists the seen vendors open to the faction that sold the item, with the stock row.
func (o *Observed) VendorsOf(item int64, faction string) (vs []SeenVendor, stock []SeenStock) {
	if o == nil {
		return nil, nil
	}
	for _, v := range o.Vendors {
		if !factionOpen(v.Faction, faction) {
			continue
		}
		for _, it := range v.Items {
			if it.ItemID == item {
				vs, stock = append(vs, v), append(stock, it)
				break
			}
		}
	}
	return vs, stock
}

// GiverOf is the seen giver of a quest.
func (o *Observed) GiverOf(questID int) (SeenGiver, bool) {
	if o == nil {
		return SeenGiver{}, false
	}
	for _, g := range o.Givers {
		if g.QuestID == questID {
			return g, true
		}
	}
	return SeenGiver{}, false
}

// ParseAddon reads the addon's SavedVariables text: the stat totals of one character and what the
// player saw.
func ParseAddon(text, key, name string) (*Totals, *Observed) {
	vars, err := lua.Parse(text)
	if err != nil {
		return nil, nil
	}
	return totalsOf(vars, key, name), ParseObserved(vars)
}

// AttachAddon returns c with what the addon recorded in its SavedVariables file: the stat totals and
// the observations ("seen by you"). Nothing is attached when the file is missing or unreadable.
func AttachAddon(addonSV string, c Character) Character {
	if addonSV == "" {
		return c
	}
	raw, err := os.ReadFile(addonSV)
	if err != nil {
		return c
	}
	t, o := ParseAddon(string(raw), c.Key, c.Name)
	if t != nil {
		c.Totals = t
	}
	if !o.Empty() {
		c.Seen = o
	}
	return c
}

// ZoneOfMap names the zone of a uiMapID from what the capture knows (where the character stood, the
// NPCs it met); "" when it knows none. A name is never invented.
func (c Character) ZoneOfMap(mapID int) string {
	if l := c.Location; l != nil && l.Zone != "" && l.MapID != nil && *l.MapID == mapID {
		return l.Zone
	}
	for _, n := range c.NPCs {
		if n.MapID != nil && *n.MapID == mapID && n.Zone != "" {
			return n.Zone
		}
	}
	return ""
}

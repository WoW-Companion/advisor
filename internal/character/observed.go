package character

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

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
	Level      int    // the character level it needs, 0 when none shown
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
	// Flights are the flight masters the player opened.
	Flights []SeenFlight
}

// Empty reports whether nothing was observed.
func (o *Observed) Empty() bool {
	return o == nil || len(o.Trainers) == 0 && len(o.Vendors) == 0 && len(o.Gather) == 0 && len(o.Givers) == 0 && len(o.Flights) == 0
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
			return SeenOffer{Name: asStr(e["name"]), Rank: asStr(e["rank"]), State: asStr(e["state"]), Cost: i640(e["cost"]), Level: i0(e["level"]),
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
	o.Flights, _ = each(obs["flightmasters"], func(t lua.Table) (SeenFlight, bool) {
		f := SeenFlight{NPC: i640(t["npc"]), Name: asStr(t["name"]), Map: i0(t["map"]), X: fl(t["x"]), Y: fl(t["y"]),
			Faction: asStr(t["faction"]), T: i640(t["t"])}
		return f, f.Name != "" || f.NPC != 0
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

// SeenFlight is one flight master the player opened: a known flight point.
type SeenFlight struct {
	NPC     int64
	Name    string
	Map     int
	X, Y    float64
	Faction string
	T       int64
}

// FlightsFor lists the seen flight masters open to the faction, newest first.
func (o *Observed) FlightsFor(faction string) []SeenFlight {
	if o == nil {
		return nil
	}
	var out []SeenFlight
	for _, f := range o.Flights {
		if factionOpen(f.Faction, faction) {
			out = append(out, f)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].T > out[j].T })
	return out
}

// StaleAfter is how old an observation may be and still count as current price data.
const StaleAfter = 30 * 24 * time.Hour

// SeenAgo says when something was seen, in plain words ("seen 3 days ago"); "" when t is unknown.
func SeenAgo(t int64, now time.Time) string {
	if t <= 0 {
		return ""
	}
	d := now.Sub(time.Unix(t, 0))
	n := func(v int64, unit string) string {
		if v == 1 {
			return "seen 1 " + unit + " ago"
		}
		return fmt.Sprintf("seen %d %ss ago", v, unit)
	}
	switch {
	case d < time.Hour:
		return "seen just now"
	case d < 24*time.Hour:
		return n(int64(d/time.Hour), "hour")
	case d < 60*24*time.Hour:
		return n(int64(d/(24*time.Hour)), "day")
	}
	return n(int64(d/(30*24*time.Hour)), "month")
}

// Stale reports whether an observation made at t is too old to count as current prices.
func Stale(t int64, now time.Time) bool {
	return t > 0 && now.Sub(time.Unix(t, 0)) > StaleAfter
}

// SeenNote is the freshness phrase for an observation: "seen 3 days ago", with "; prices may have
// changed" added once it is older than StaleAfter.
func SeenNote(t int64, now time.Time) string {
	s := SeenAgo(t, now)
	if s != "" && Stale(t, now) {
		s += "; prices may have changed"
	}
	return s
}

// SeenRecipe is one recipe the player's own trade skill window listed.
type SeenRecipe struct {
	Name       string
	ID         int64  // the spell or item id in the recipe's link, 0 when none
	Difficulty string // orange, yellow, green or grey at the skill the window was opened at
	Makes      int64  // item id, 0 when none
	MakesCount int
	Reagents   []ItemStack
}

// SeenRecipes is one profession's recipes and when the window was read.
type SeenRecipes struct {
	SeenAt  int64
	Entries []SeenRecipe
}

func wowcRecipes(v any) map[string]SeenRecipes {
	t := table(v)
	if len(t) == 0 {
		return nil
	}
	out := map[string]SeenRecipes{}
	for k, e := range t {
		pt := table(e)
		prof := strings.ToLower(strings.TrimSpace(asStr(k)))
		if pt == nil || prof == "" {
			continue
		}
		r := SeenRecipes{SeenAt: i640(pt["seenAt"])}
		r.Entries, _ = each(pt["entries"], func(x lua.Table) (SeenRecipe, bool) {
			sr := SeenRecipe{Name: strings.TrimSpace(asStr(x["name"])), ID: i640(x["id"]), Difficulty: strings.ToLower(asStr(x["difficulty"]))}
			if m := table(x["makes"]); m != nil {
				sr.Makes, sr.MakesCount = i640(m["item"]), i0(m["count"])
			}
			for _, rg := range lua.List(x["reagents"]) {
				rt := table(rg)
				if id := i640(rt["item"]); id != 0 {
					n := i0(rt["count"])
					if n <= 0 {
						n = 1
					}
					sr.Reagents = append(sr.Reagents, ItemStack{ItemID: int(id), Count: n})
				}
			}
			return sr, sr.Name != ""
		})
		out[prof] = r
	}
	return out
}

// Recipe finds a recipe the player's window listed for the profession, by name (case-blind).
func (c Character) Recipe(profession, name string) (SeenRecipe, bool) {
	rs, ok := c.Recipes[strings.ToLower(strings.TrimSpace(profession))]
	if !ok {
		return SeenRecipe{}, false
	}
	for _, r := range rs.Entries {
		if strings.EqualFold(r.Name, strings.TrimSpace(name)) {
			return r, true
		}
	}
	return SeenRecipe{}, false
}

// WithRecipeProfessions adds a profession for each one the character has recorded recipes for and
// the skills do not list: a recipe window only opens for a profession the character has. The skill
// level of an added one is unknown. It returns c unchanged when nothing needs adding.
func (c Character) WithRecipeProfessions() Character {
	var add []Skill
	for prof, rs := range c.Recipes {
		if len(rs.Entries) == 0 {
			continue
		}
		have := false
		for _, p := range c.Professions {
			have = have || strings.EqualFold(p.Name, prof)
		}
		if !have {
			words := strings.Fields(prof)
			for i, w := range words {
				words[i] = strings.ToUpper(w[:1]) + w[1:]
			}
			add = append(add, Skill{Name: strings.Join(words, " "), Group: "Professions", FromRecipes: true})
		}
	}
	if len(add) == 0 {
		return c
	}
	sort.Slice(add, func(i, j int) bool { return add[i].Name < add[j].Name })
	c.Professions = append(append([]Skill{}, c.Professions...), add...)
	return c
}

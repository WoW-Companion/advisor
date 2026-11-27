package vendors

import (
	"cmp"
	"fmt"
	"sort"
	"strings"

	"github.com/WoW-Companion/advisor/internal/questie"
	"github.com/WoW-Companion/advisor/internal/store"
)

// Kinds a Search can look for.
const (
	KindVendor  = "vendor"
	KindTrainer = "trainer"
	KindFlight  = "flight"
)

// MaxFound is how many NPCs a search lists; MaxSells is how many goods each one names.
const (
	MaxFound = 5
	MaxSells = 6
)

// Query is one who-sells question. Text is a keyword, an item name or a subname word ("poison",
// "Shady Goods", "reagents"); empty lists the NPCs of the kind in Zone.
type Query struct {
	Text    string
	Zone    string
	Kind    string // vendor (default), trainer or flight
	Faction string // the player's faction; "" lists both
	At      questie.Where
}

// Found is one NPC the search matched.
type Found struct {
	NPCID   int64   `json:"npc_id"`
	Name    string  `json:"name"`
	Sub     string  `json:"subname,omitempty" jsonschema:"the NPC's title, such as Shady Goods"`
	Zone    string  `json:"zone,omitempty"`
	Subzone string  `json:"subzone,omitempty"`
	X       float64 `json:"x,omitempty"`
	Y       float64 `json:"y,omitempty"`
	HasXY   bool    `json:"has_coordinates"`
	Faction string  `json:"faction,omitempty" jsonschema:"Alliance, Horde, or empty for a vendor both factions can use"`
	// FactionKnown is true when a source holds the NPC's side: QuestieDB (an empty side there is a vendor both sides use) or the NPC row.
	FactionKnown bool     `json:"faction_known" jsonschema:"false when no data says which side can use the NPC; Faction is then empty"`
	Sells        []string `json:"sells,omitempty"`
	Questie      bool     `json:"from_questie" jsonschema:"the place or the subname was read from the player's QuestieDB"`
	rank         int
	dist         float64
	matched      map[string]bool
}

// Result is a search's answer: the NPCs (nearest first), the line to say, and Gap when nothing matched.
type Result struct {
	Found []Found `json:"found"`
	Text  string  `json:"text"`
	Gap   string  `json:"gap,omitempty" jsonschema:"set when nothing matched: the data that holds no such NPC"`
}

var stopWords = map[string]bool{"a": true, "an": true, "the": true, "in": true, "at": true, "for": true, "of": true, "to": true, "any": true, "is": true,
	"there": true, "here": true, "who": true, "which": true, "where": true, "vendor": true, "vendors": true, "seller": true, "sellers": true,
	"sells": true, "sell": true, "merchant": true, "merchants": true, "shop": true, "store": true, "trainer": true, "trainers": true,
	"flight": true, "master": true, "path": true, "and": true, "buy": true, "can": true, "i": true}

// flagTerms are the keywords an NPC's own vendor type answers when the install's flavor has such a bit
// (the classic-era layout has none): a poison vendor sells poison goods whatever its subname says. Without
// the bit a keyword is matched by name, subname and the items sold (the dataset's vendor_sources).
var flagTerms = map[string]int{"poison": questie.FlagVendorPoison, "poisons": questie.FlagVendorPoison, "reagent": questie.FlagVendorReagent,
	"reagents": questie.FlagVendorReagent, "food": questie.FlagVendorFood, "ammo": questie.FlagVendorAmmo}

func keywords(s string) []string {
	var out []string
	for _, w := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '\'') }) {
		if !stopWords[w] {
			out = append(out, w)
		}
	}
	return out
}

func zoneIs(have, want string) bool {
	have, want = strings.ToLower(strings.TrimSpace(have)), strings.ToLower(strings.TrimSpace(want))
	return have != "" && want != "" && (have == want || strings.Contains(have, want) || strings.Contains(want, have))
}

func f64(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	}
	return 0, false
}

func open(npc, side string) bool {
	n, c := strings.ToLower(strings.TrimSpace(npc)), strings.ToLower(strings.TrimSpace(side))
	switch n {
	case "", "neutral", "both", "ah", "ha":
		return true
	case "a":
		n = "alliance"
	case "h":
		n = "horde"
	}
	return c == "" || n == c
}

// Search finds NPCs of the kind for the query, from the dataset's vendor offers and NPC rows and from
// the player's QuestieDB (its subnames and vendor types). Either source may be nil.
func Search(ds store.DataStore, qx *questie.Index, q Query) Result {
	kind := cmp.Or(strings.ToLower(q.Kind), KindVendor)
	terms := keywords(q.Text)
	cand := map[int64]*Found{}
	get := func(id int64) *Found {
		if f := cand[id]; f != nil {
			return f
		}
		f := &Found{NPCID: id, matched: map[string]bool{}}
		cand[id] = f
		return f
	}
	sold := map[int64]map[string]bool{} // npc -> item names, kept for the "sells" list
	if ds != nil && kind == KindVendor {
		datasetVendors(ds, terms, q.Zone, cand, get, sold)
	}
	for _, s := range qx.Services(kind) {
		f := cand[int64(s.ID)]
		text := strings.ToLower(s.Entity.Name + " " + s.Entity.Sub)
		hit := false
		for _, t := range terms {
			if strings.Contains(text, t) || s.Entity.Flags&flagTerms[t] != 0 {
				hit = true
			}
		}
		if f == nil && (len(terms) == 0 && q.Zone != "" || hit) {
			f = get(int64(s.ID))
		}
		if f == nil {
			continue
		}
		for _, t := range terms {
			if strings.Contains(text, t) || s.Entity.Flags&flagTerms[t] != 0 {
				f.matched[t] = true
			}
		}
		f.Name, f.Sub, f.Questie = s.Entity.Name, s.Entity.Sub, true
		if f.Faction == "" {
			f.Faction = factionName(s.Entity.Faction)
		}
		f.FactionKnown = true
	}
	var list []*Found
	for id, f := range cand {
		if !place(ds, qx, f, id, q) || !open(f.Faction, q.Faction) {
			continue
		}
		all := true
		for _, t := range terms {
			all = all && f.matched[t]
		}
		if !all {
			continue
		}
		list = append(list, f)
	}
	sort.Slice(list, func(i, j int) bool {
		a, b := list[i], list[j]
		if a.rank != b.rank {
			return a.rank < b.rank
		}
		if a.dist != b.dist {
			return a.dist < b.dist
		}
		return a.NPCID < b.NPCID
	})
	if len(list) > MaxFound {
		list = list[:MaxFound]
	}
	res := Result{Found: []Found{}}
	for _, f := range list {
		fillSells(ds, f, terms, sold[f.NPCID])
		res.Found = append(res.Found, *f)
	}
	res.Text, res.Gap = render(res.Found, q, kind)
	return res
}

func factionName(q string) string {
	switch q {
	case "A":
		return "Alliance"
	case "H":
		return "Horde"
	}
	return ""
}

// datasetVendors adds the vendors the dataset lists for the keywords' items, or every vendor offer
// when there are no keywords and a zone is named.
func datasetVendors(ds store.DataStore, terms []string, zone string, cand map[int64]*Found, get func(int64) *Found, sold map[int64]map[string]bool) {
	if len(terms) == 0 {
		if zone == "" {
			return
		}
		rows, err := ds.Filter("vendor_sources", store.Range{Column: "npc_id", Min: int64(0)})
		if err != nil {
			return
		}
		for _, r := range rows {
			get(i64(r["npc_id"]))
		}
		return
	}
	names := map[int64]string{}
	byTerm := map[int64][]string{}
	lo, hi := int64(1<<62), int64(0)
	for _, t := range terms {
		items, err := ds.SearchName("items", t, 60)
		if err != nil {
			continue
		}
		for _, it := range items {
			id := i64(it["id"])
			if id == 0 {
				continue
			}
			names[id] = str(it["name"])
			byTerm[id] = append(byTerm[id], t)
			lo, hi = min(lo, id), max(hi, id)
		}
	}
	if len(names) == 0 {
		return
	}
	rows, err := ds.Filter("vendor_sources", store.Range{Column: "item_id", Min: lo, Max: hi})
	if err != nil {
		return
	}
	for _, r := range rows {
		item := i64(r["item_id"])
		ts, ok := byTerm[item]
		if !ok {
			continue
		}
		npc := i64(r["npc_id"])
		f := get(npc)
		for _, t := range ts {
			f.matched[t] = true
		}
		if sold[npc] == nil {
			sold[npc] = map[string]bool{}
		}
		sold[npc][names[item]] = true
	}
}

// place fills the NPC's name and place from the dataset row, then Questie; false when it has no name
// or stands outside the zone asked for.
func place(ds store.DataStore, qx *questie.Index, f *Found, id int64, q Query) bool {
	if ds != nil {
		if n, err := ds.Get("npcs", id); err == nil {
			if f.Name == "" {
				f.Name = str(n["name"])
			}
			f.Zone = cmp.Or(str(n["zone_name"]), str(n["located_in"]))
			if x, ok := f64(n["x"]); ok {
				if y, ok := f64(n["y"]); ok {
					f.X, f.Y, f.HasXY = x, y, true
				}
			}
			f.Faction = cmp.Or(f.Faction, str(n["faction"]))
		}
	}
	if f.Name == "" {
		return false
	}
	at := q.At
	if q.Zone != "" {
		at = questie.Where{Zone: q.Zone}
	}
	if e, ok := qx.NpcEntity(int(id)); ok {
		f.Sub = cmp.Or(f.Sub, e.Sub)
		f.Faction = cmp.Or(f.Faction, factionName(e.Faction))
		f.FactionKnown = true // QuestieDB holds the NPC: an empty side there is a vendor both sides use
	}
	f.FactionKnown = f.FactionKnown || f.Faction != ""
	if p, ok := qx.NpcPlace(int(id), at); ok && (f.Zone == "" || !f.HasXY || q.Zone != "" && !zoneIs(f.Zone, q.Zone)) {
		if f.Zone == "" || q.Zone != "" && !zoneIs(f.Zone, q.Zone) {
			f.Zone, f.Subzone = p.Zone, p.Sub
		}
		f.X, f.Y, f.HasXY, f.Questie = p.X, p.Y, true, true
		f.dist = p.Dist
	} else if ok {
		f.Subzone = p.Sub
	}
	if q.Zone != "" && !zoneIs(f.Zone, q.Zone) && !zoneIs(f.Subzone, q.Zone) {
		return false
	}
	f.rank = 1
	if q.At.Zone != "" && (zoneIs(f.Zone, q.At.Zone) || zoneIs(f.Subzone, q.At.Zone)) {
		f.rank = 0
	}
	if f.dist == 0 && !f.Questie {
		f.dist = 1e9
	}
	return true
}

func fillSells(ds store.DataStore, f *Found, terms []string, known map[string]bool) {
	if ds == nil {
		return
	}
	names := map[string]bool{}
	for n := range known {
		names[n] = true
	}
	if len(names) < MaxSells {
		if rows, err := ds.Filter("vendor_sources", store.Range{Column: "npc_id", Min: f.NPCID, Max: f.NPCID}); err == nil {
			for _, r := range rows {
				if it, err := ds.Get("items", i64(r["item_id"])); err == nil && str(it["name"]) != "" {
					names[str(it["name"])] = true
				}
			}
		}
	}
	for n := range names {
		f.Sells = append(f.Sells, n)
	}
	hits := func(n string) int {
		c := 0
		for _, t := range terms {
			if strings.Contains(strings.ToLower(n), t) {
				c++
			}
		}
		return c
	}
	sort.Slice(f.Sells, func(i, j int) bool {
		if a, b := hits(f.Sells[i]), hits(f.Sells[j]); a != b {
			return a > b
		}
		return f.Sells[i] < f.Sells[j]
	})
	if len(f.Sells) > MaxSells {
		f.Sells = f.Sells[:MaxSells]
	}
}

func render(found []Found, q Query, kind string) (text, gap string) {
	what := map[string]string{KindVendor: "vendor", KindTrainer: "trainer", KindFlight: "flight master"}[kind]
	if what == "" {
		what = kind
	}
	if len(found) == 0 {
		subject := what
		if t := strings.TrimSpace(q.Text); t != "" {
			subject = fmt.Sprintf("%s for %q", what, t)
		}
		if q.Zone != "" {
			subject += " in " + q.Zone
		}
		gap = "The dataset's vendor offers and the player's QuestieDB list no " + subject + "; that is a gap in the data, not proof there is none."
		return "Not in my data: no " + subject + ". " + gap, gap
	}
	var b strings.Builder
	for i, f := range found {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(f.Line())
	}
	return b.String(), ""
}

// Line is "Sly Garrett <Shady Goods>, Booty Bay, Stranglethorn Vale (27.1, 77.2): sells ...".
func (f Found) Line() string {
	s := f.Name
	if f.Sub != "" {
		s += " <" + f.Sub + ">"
	}
	var loc []string
	for _, p := range []string{f.Subzone, f.Zone} {
		if p != "" && (len(loc) == 0 || loc[len(loc)-1] != p) {
			loc = append(loc, p)
		}
	}
	if len(loc) > 0 {
		s += ", " + strings.Join(loc, ", ")
	}
	if f.HasXY {
		s += fmt.Sprintf(" (%.1f, %.1f)", f.X, f.Y)
	}
	switch {
	case f.Faction != "":
		s += ", " + f.Faction
	case f.FactionKnown:
		s += ", open to both factions"
	default:
		s += ", which side can use it is not in the data"
	}
	if len(f.Sells) > 0 {
		s += ": sells " + strings.Join(f.Sells, ", ")
	}
	if f.Questie {
		s += " (from your Questie database)"
	}
	return s
}

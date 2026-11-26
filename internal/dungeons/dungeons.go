// Package dungeons picks the dungeons that fit a captured character from the dataset's instances,
// instance_bosses and loot_sources tables, and lists the boss drops that gear.Upgrades scores as
// upgrades for the character's spec. The rules are in docs/dungeons.md.
package dungeons

import (
	"errors"
	"sort"
	"strings"

	"github.com/WoW-Companion/advisor/internal/brief"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/gear"
	"github.com/WoW-Companion/advisor/internal/questie"
	"github.com/WoW-Companion/advisor/internal/quests"
	"github.com/WoW-Companion/advisor/internal/store"
)

// LevelWindow is how far below the character a dungeon's range may top out and still fit. A
// dungeon is open from its minimum level, so none fits a character under it.
const LevelWindow = 3

// Drop is one upgrade a boss drops. Chance is nil when the dataset has no kill count.
type Drop struct {
	Boss   string
	ItemID int64
	Item   string
	Slot   string
	Delta  float64
	Empty  bool     // the slot is empty
	Chance *float64 // percent of loot windows
	// Questie is true when the drop comes from the player's QuestieDB, not the dataset.
	Questie bool
}

// Quest is an open quest of the capture linked to a dungeon.
type Quest struct {
	ID    int
	Title string
}

// Pickup is a dungeon quest worth taking before going in.
type Pickup struct {
	ID        int64
	Title     string
	Giver     string
	Questie   bool // from the player's QuestieDB
	GiverSeen bool // the giver is one the player saw give the quest
}

// Inhabitant is a creature inside a dungeon, from the player's QuestieDB.
type Inhabitant struct {
	Name  string
	Elite bool
	Boss  bool
}

// DungeonPick is one dungeon and what it offers the character.
type DungeonPick struct {
	MapID      int64
	Name       string
	MinLevel   *int
	MaxLevel   *int
	MaxPlayers *int
	// Raid is true for a raid map: its lockout and attunement are not in the dataset.
	Raid bool
	// EntranceZone is where the dungeon's entrance is; empty when not known.
	EntranceZone string
	// EntranceX and EntranceY are the entrance in map percent of EntranceZone; EntranceFromQuestie
	// is true when Questie supplied the zone or the coordinates.
	EntranceX, EntranceY float64
	HasEntranceXY        bool
	EntranceFromQuestie  bool
	// Inside are the creatures that live in the dungeon, bosses first, when Questie knows them.
	Inside []Inhabitant
	// Curated is true when the level range is the advisor's typical range (level_source curated).
	Curated bool
	// Faction is "Horde" or "Alliance" when the dungeon is one side's; empty for both.
	Faction string
	// Pickups are the dungeon quests to take first, with their givers.
	Pickups []Pickup
	// LevelUnknown is true when the dataset holds no level range; such picks are unranked.
	LevelUnknown bool
	// Fit is how many levels the character is above the range (0 inside it).
	Fit int
	// Upgrades counts the distinct items among Drops.
	Upgrades int
	Drops    []Drop
	Quests   []Quest
	// Bosses are the dungeon's encounters by name, from the dataset.
	Bosses []string
	// ahead is set by Named: the pickups are not held to the character's level.
	ahead bool
	// UnlinkedBosses are the encounters the dataset cannot tie to a creature, so their drops are
	// unknown.
	UnlinkedBosses []string
}

// Result is a plan: ranked picks, then the dungeons with no level range.
type Result struct {
	Picks   []DungeonPick
	Unknown []DungeonPick
	Spec    string
	// First is set when the character is below every dungeon's range: the lowest one, which they
	// reach in LevelsToGo levels. Meanwhile is one useful thing to do in the meantime.
	First      *DungeonPick
	LevelsToGo int
	Meanwhile  string
	// ItemLevelOnly is true when upgrades were scored without stat weights.
	ItemLevelOnly bool
	Reason        string // why scoring fell back to item level, when it did
}

// candidates is the best upgrade candidate per item id.
func candidates(gr gear.Result) map[int64]gear.Candidate {
	cands := map[int64]gear.Candidate{}
	for _, list := range gr.Slots {
		for _, cd := range list {
			if old, ok := cands[cd.Item.ID]; !ok || cd.Delta > old.Delta {
				cands[cd.Item.ID] = cd
			}
		}
	}
	return cands
}

// pickOf is the dungeon an instances row describes; ok is false for a row that is not a dungeon
// players run: not typed dungeon, or with neither an entrance (the dataset's zone or Questie's) nor
// a group size (the client has map rows such as Ruins of Lordaeron that are neither).
func pickOf(row store.Row, qx *questie.Index) (DungeonPick, bool) {
	return pickOfType(row, qx, false)
}

// pickOfType is pickOf; with raids set, a raid map is described too (Named and Mentioned, never a plan).
func pickOfType(row store.Row, qx *questie.Index, raids bool) (DungeonPick, bool) {
	kind, _ := row["instance_type"].(string)
	if kind != "dungeon" && !(raids && kind == "raid") {
		return DungeonPick{}, false
	}
	p := DungeonPick{MapID: i64(row["map_id"]), MinLevel: optInt(row["min_level"]), MaxLevel: optInt(row["max_level"]), MaxPlayers: optInt(row["max_players"])}
	p.Raid = kind == "raid"
	p.Name, _ = row["name"].(string)
	p.EntranceZone, _ = row["entrance_zone_name"].(string)
	if p.EntranceZone == "" && p.MaxPlayers == nil && qx.FindDungeon(int(p.MapID), p.Name) == nil {
		return DungeonPick{}, false
	}
	src, _ := row["level_source"].(string)
	p.Curated = src == "curated"
	p.Faction, _ = row["faction"].(string)
	if p.Faction == "" {
		p.Faction = capitalFaction[strings.ToLower(p.EntranceZone)]
	}
	return p, true
}

// Named is the dungeon the player asked about by name (exactly, else by containment either way),
// described whatever the character's level or side: it is for planning ahead, so nothing is
// filtered. The int is how many levels the character is under its minimum (0 when open). It returns
// nil when no dungeon has that name.
func Named(c character.Character, ds store.DataStore, qx *questie.Index, name string) (*DungeonPick, int, error) {
	if ds == nil {
		return nil, 0, errors.New("no dataset configured")
	}
	rows, err := ds.Filter("instances", store.Range{Column: "map_id", Min: int64(0)})
	if err != nil {
		return nil, 0, errors.New("instances table: " + err.Error())
	}
	want := strings.ToLower(strings.TrimSpace(name))
	if want == "" {
		return nil, 0, nil
	}
	var best *DungeonPick
	for _, row := range rows {
		p, ok := pickOfType(row, qx, true)
		if !ok {
			continue
		}
		n := strings.ToLower(p.Name)
		if n == want {
			best = &p
			break
		}
		if best == nil && n != "" && (strings.Contains(n, want) || strings.Contains(want, n)) {
			best = &p
		}
	}
	if best == nil {
		return nil, 0, nil
	}
	group := ""
	if names, _ := brief.TopGroups(c.Talents); len(names) == 1 {
		group = names[0]
	}
	cands := map[int64]gear.Candidate{}
	if c.Level != nil {
		if gr, err := gear.Find(c, ds, group, gear.Options{}); err == nil {
			cands = candidates(gr)
		}
	}
	best.ahead = true
	fill(best, c, ds, cands, qx)
	best.LevelUnknown = best.MinLevel == nil || best.MaxLevel == nil
	togo := 0
	if best.MinLevel != nil && c.Level != nil && *c.Level < *best.MinLevel {
		togo = *best.MinLevel - *c.Level
	}
	return best, togo, nil
}

func i64(v any) int64 { n, _ := v.(int64); return n }

func optInt(v any) *int {
	n, ok := v.(int64)
	if !ok {
		return nil
	}
	i := int(n)
	return &i
}

// Plan finds the dungeons whose level range comes within LevelWindow of the character's level,
// ranked by upgrade count then level fit, with the unranked level-unknown ones after.
func Plan(c character.Character, ds store.DataStore) (Result, error) {
	return PlanWith(c, ds, nil)
}

// PlanWith is Plan with the player's QuestieDB index, when they have one: it fills what the
// dataset lacks (the entrance, the creatures inside, their drops and the dungeon quests). The
// dataset always wins where both have an answer.
func PlanWith(c character.Character, ds store.DataStore, qx *questie.Index) (Result, error) {
	var res Result
	if ds == nil {
		return res, errors.New("no dataset configured")
	}
	if c.Level == nil {
		return res, errors.New("the capture has no character level")
	}
	level := *c.Level
	group := ""
	if names, _ := brief.TopGroups(c.Talents); len(names) == 1 {
		group = names[0]
	}
	gr, err := gear.Find(c, ds, group, gear.Options{})
	if err != nil {
		return res, err
	}
	res.Spec, res.ItemLevelOnly, res.Reason = gr.Spec, gr.ItemLevelOnly, gr.Reason
	cands := candidates(gr)

	rows, err := ds.Filter("instances", store.Range{Column: "map_id", Min: int64(0)})
	if err != nil {
		return res, errors.New("instances table: " + err.Error())
	}
	var all []DungeonPick // every ranged dungeon the character's side may enter
	for _, row := range rows {
		p, ok := pickOf(row, qx)
		if !ok {
			continue
		}
		if p.Faction != "" && c.Faction != "" && !strings.EqualFold(p.Faction, c.Faction) {
			continue
		}
		if p.MinLevel != nil && p.MaxLevel != nil {
			all = append(all, p)
		}
		lo, hi := p.MinLevel, p.MaxLevel
		if lo == nil || hi == nil {
			p.LevelUnknown = true
		} else {
			if *hi < level-LevelWindow || *lo > level {
				continue
			}
			if level > *hi {
				p.Fit = level - *hi
			}
		}
		fill(&p, c, ds, cands, qx)
		if p.LevelUnknown {
			res.Unknown = append(res.Unknown, p)
		} else {
			res.Picks = append(res.Picks, p)
		}
	}
	sort.SliceStable(res.Picks, func(i, j int) bool {
		a, b := res.Picks[i], res.Picks[j]
		if a.Upgrades != b.Upgrades {
			return a.Upgrades > b.Upgrades
		}
		if a.Fit != b.Fit {
			return a.Fit < b.Fit
		}
		return a.MapID < b.MapID
	})
	if len(res.Picks) == 0 {
		sort.SliceStable(all, func(i, j int) bool {
			if *all[i].MinLevel != *all[j].MinLevel {
				return *all[i].MinLevel < *all[j].MinLevel
			}
			return all[i].MapID < all[j].MapID
		})
		if len(all) > 0 && level < *all[0].MinLevel {
			first := all[0]
			fill(&first, c, ds, cands, qx)
			res.First, res.LevelsToGo = &first, *first.MinLevel-level
			res.Meanwhile = meanwhile(c)
		}
	}
	sort.SliceStable(res.Unknown, func(i, j int) bool { return res.Unknown[i].MapID < res.Unknown[j].MapID })
	return res, nil
}

// capitalFaction is the side a dungeon belongs to when the dataset says nothing but its entrance
// zone is a capital.
var capitalFaction = map[string]string{"orgrimmar": "Horde", "thunder bluff": "Horde", "undercity": "Horde",
	"stormwind city": "Alliance", "ironforge": "Alliance", "darnassus": "Alliance"}

// meanwhile names one open quest of the capture to work on while the character levels.
func meanwhile(c character.Character) string {
	for _, q := range c.Quests {
		if q.Title == "" || q.Complete != nil && *q.Complete || q.Failed != nil && *q.Failed {
			continue
		}
		return "finish your quest " + q.Title
	}
	return "keep questing"
}

// fill adds the boss drops that are upgrades, and the linked open quests, to a pick.
func fill(p *DungeonPick, c character.Character, ds store.DataStore, cands map[int64]gear.Candidate, qx *questie.Index) {
	bosses, _ := ds.Filter("instance_bosses", store.Range{Column: "map_id", Min: p.MapID, Max: p.MapID})
	bosses = standardEncounters(bosses)
	named := map[string]bool{}
	for _, b := range bosses {
		if name, _ := b["name"].(string); name != "" && !named[strings.ToLower(name)] {
			named[strings.ToLower(name)] = true
			p.Bosses = append(p.Bosses, name)
		}
	}
	seen := map[[2]int64]bool{}
	items := map[int64]bool{}
	unlinked := map[string]bool{}
	for _, b := range bosses {
		name, _ := b["name"].(string)
		cid, ok := b["creature_id"].(int64)
		if !ok {
			// the dataset keeps one encounter row per difficulty: a boss is named once
			if !unlinked[strings.ToLower(name)] {
				unlinked[strings.ToLower(name)] = true
				p.UnlinkedBosses = append(p.UnlinkedBosses, name)
			}
			continue
		}
		loot, _ := ds.Filter("loot_sources", store.Range{Column: "source_id", Min: cid, Max: cid})
		for _, l := range loot {
			id := i64(l["item_id"])
			cd, ok := cands[id]
			if !ok || l["source_kind"] != "creature" || seen[[2]int64{cid, id}] {
				continue
			}
			seen[[2]int64{cid, id}] = true
			d := Drop{Boss: name, ItemID: id, Item: cd.Item.Name, Slot: cd.Slot, Delta: cd.Delta, Empty: cd.Empty}
			if kills := i64(l["kills"]); kills > 0 {
				ch := 100 * float64(i64(l["drops"])) / float64(kills)
				d.Chance = &ch
			}
			p.Drops = append(p.Drops, d)
			items[id] = true
		}
	}
	p.Pickups = pickups(p, c, ds)
	if d := qx.FindDungeon(int(p.MapID), p.Name); d != nil {
		in := qx.Inside(d)
		p.fromQuestie(in)
		p.questieDrops(qx, d, cands, items)
		p.questiePickups(qx, in, c)
	}
	p.Upgrades = len(items)
	sort.SliceStable(p.Drops, func(i, j int) bool {
		if p.Drops[i].Delta != p.Drops[j].Delta {
			return p.Drops[i].Delta > p.Drops[j].Delta
		}
		return p.Drops[i].ItemID < p.Drops[j].ItemID
	})
	p.Quests = openQuests(p, c, ds)
}

// standardDifficulty is the highest difficulty id of the dungeon's ordinary encounters: the dataset keeps 0 (the
// legacy id) and 1 (normal) for the base game, and a later id (198, 201, 215) for an alternate mode of the same map
// whose encounter set differs (a different boss list, two bosses fought as one, a boss spelled another way).
const standardDifficulty = 1

// standardEncounters keeps the encounters of the ordinary difficulties when the map has any, so an alternate
// mode's bosses are not listed beside the real ones; a map with only alternate rows keeps them all.
func standardEncounters(rows []store.Row) []store.Row {
	var base []store.Row
	for _, r := range rows {
		if d, ok := r["difficulty_id"].(int64); !ok || d <= standardDifficulty {
			base = append(base, r)
		}
	}
	if len(base) == 0 {
		return rows
	}
	return base
}

// maxPickups is how many dungeon quests a pick lists.
const maxPickups = 3

// maxAhead is how many dungeon quests a named dungeon lists.
const maxAhead = 8

func (p *DungeonPick) limit() int {
	if p.ahead {
		return maxAhead
	}
	return maxPickups
}

// pickups are the dungeon's quests from instance_quests that the character can take now: not
// already in the log, not above their level by more than the window, and not the other side's.
func pickups(p *DungeonPick, c character.Character, ds store.DataStore) []Pickup {
	rows, err := ds.Filter("instance_quests", store.Range{Column: "map_id", Min: p.MapID, Max: p.MapID})
	if err != nil {
		return nil
	}
	have := map[int64]bool{}
	for _, q := range c.Quests {
		have[int64(q.QuestID)] = true
	}
	var level int
	if c.Level != nil {
		level = *c.Level
	}
	var out []Pickup
	seen := map[int64]bool{}
	for _, r := range rows {
		id := i64(r["quest_id"])
		if id == 0 || have[id] || seen[id] {
			continue
		}
		seen[id] = true
		f := quests.FactsOf(ds, nil, id)
		if f == nil || f.Name == "" {
			continue
		}
		if f.Faction != "" && c.Faction != "" && !strings.EqualFold(f.Faction, c.Faction) && !strings.EqualFold(f.Faction, "both") {
			continue
		}
		if f.Level > level+LevelWindow && !p.ahead {
			continue
		}
		giver, fromSeen := quests.GiverNameSeen(ds, c.Seen, id)
		out = append(out, Pickup{ID: id, Title: f.Name, Giver: giver, GiverSeen: fromSeen})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	if n := p.limit(); len(out) > n {
		out = out[:n]
	}
	return out
}

// openQuests are the capture's unfinished quests the dataset ties to the dungeon: the quest's
// point-of-interest map is the dungeon's map, or its zone name is the dungeon's name. A quest the
// dataset cannot place is left out.
func openQuests(p *DungeonPick, c character.Character, ds store.DataStore) []Quest {
	var out []Quest
	for _, q := range c.Quests {
		if q.Complete != nil && *q.Complete || q.Failed != nil && *q.Failed {
			continue
		}
		linked := q.Zone != "" && strings.EqualFold(q.Zone, p.Name)
		if row, err := ds.Get("quests", int64(q.QuestID)); err == nil {
			if i64(row["map_id"]) == p.MapID && row["map_id"] != nil {
				linked = true
			}
			if z, _ := row["zone_name"].(string); z != "" && strings.EqualFold(z, p.Name) {
				linked = true
			}
		}
		if linked {
			out = append(out, Quest{ID: q.QuestID, Title: q.Title})
		}
	}
	return out
}

// fromQuestie fills the entrance (the dataset's zone wins) and the creatures inside.
func (p *DungeonPick) fromQuestie(in questie.Inside) {
	if p.EntranceZone == "" && in.Zone != "" {
		p.EntranceZone, p.EntranceFromQuestie = in.Zone, true
	}
	if in.Entrance && strings.EqualFold(p.EntranceZone, in.Zone) {
		p.EntranceX, p.EntranceY, p.HasEntranceXY, p.EntranceFromQuestie = in.X, in.Y, true, true
	}
	for _, cr := range in.Creatures {
		p.Inside = append(p.Inside, Inhabitant{Name: cr.Name, Elite: cr.Elite, Boss: cr.Boss})
	}
}

// questieDrops adds Questie's drops of the dungeon's creatures that score as upgrades and that the
// dataset did not already list for this dungeon.
func (p *DungeonPick) questieDrops(qx *questie.Index, d *questie.Dungeon, cands map[int64]gear.Candidate, items map[int64]bool) {
	for _, cid := range d.Creatures {
		name := qx.Npcs[cid].Name
		for _, it := range qx.DropsOf(cid) {
			id := int64(it)
			cd, ok := cands[id]
			if !ok || items[id] {
				continue
			}
			items[id] = true
			p.Drops = append(p.Drops, Drop{Boss: name, ItemID: id, Item: cd.Item.Name, Slot: cd.Slot, Delta: cd.Delta, Empty: cd.Empty, Questie: true})
		}
	}
}

// questiePickups adds dungeon quests from Questie after the dataset's, up to maxPickups: not in the
// log, not too high a level, and open to the character's side and class when they are known.
func (p *DungeonPick) questiePickups(qx *questie.Index, in questie.Inside, c character.Character) {
	have := map[int64]bool{}
	titles := map[string]bool{}
	for _, q := range c.Quests {
		have[int64(q.QuestID)] = true
	}
	for _, q := range p.Pickups {
		titles[strings.ToLower(q.Title)] = true
	}
	level := 0
	if c.Level != nil {
		level = *c.Level
	}
	for _, q := range in.Quests {
		if len(p.Pickups) >= p.limit() {
			return
		}
		if have[int64(q.ID)] || titles[strings.ToLower(q.Name)] || q.Restricted || q.Parent != 0 || len(q.Starters) == 0 {
			continue
		}
		if level > 0 && !p.ahead && (q.Level > level+LevelWindow || q.MinLevel > level) {
			continue
		}
		p.Pickups = append(p.Pickups, Pickup{ID: int64(q.ID), Title: q.Name, Giver: qx.Describe(q.Starters), Questie: true})
	}
}

// Mentioned is the name of the one dungeon a text names: its full name, else (when it has several
// words and the first is distinctive) that first word. "" when none matches or the word is shared by
// several dungeons.
func Mentioned(ds store.DataStore, qx *questie.Index, text string) string {
	if ds == nil {
		return ""
	}
	rows, err := ds.Filter("instances", store.Range{Column: "map_id", Min: int64(0)})
	if err != nil {
		return ""
	}
	// an apostrophe is dropped from the text and from each name alike: "Onyxia's Lair" is asked as "onyxias lair"
	plain := strings.NewReplacer("'", "", "\u2019", "", "?", " ", ",", " ", ".", " ", "!", " ")
	hay := " " + strings.ToLower(plain.Replace(text)) + " "
	word := map[string][]string{}
	var longest string
	for _, row := range rows {
		p, ok := pickOfType(row, qx, true)
		if !ok || p.Name == "" {
			continue
		}
		n := strings.ToLower(plain.Replace(p.Name))
		if strings.Contains(hay, " "+n+" ") && len(n) > len(longest) {
			longest = p.Name
		}
		if f := strings.Fields(n); len(f) > 1 && len(f[0]) >= 6 {
			word[f[0]] = append(word[f[0]], p.Name)
		}
	}
	if longest != "" {
		return longest
	}
	for w, ns := range word {
		if len(ns) == 1 && strings.Contains(hay, " "+w+" ") {
			return ns[0]
		}
	}
	return ""
}

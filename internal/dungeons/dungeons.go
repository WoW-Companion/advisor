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
	"github.com/WoW-Companion/advisor/internal/store"
)

// LevelWindow is how far a dungeon's level range may lie from the character's level.
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
}

// Quest is an open quest of the capture linked to a dungeon.
type Quest struct {
	ID    int
	Title string
}

// DungeonPick is one dungeon and what it offers the character.
type DungeonPick struct {
	MapID      int64
	Name       string
	MinLevel   *int
	MaxLevel   *int
	MaxPlayers *int
	// LevelUnknown is true when the dataset holds no level range; such picks are unranked.
	LevelUnknown bool
	// Fit is how many levels the character is outside the range (0 inside it).
	Fit int
	// Upgrades counts the distinct items among Drops.
	Upgrades int
	Drops    []Drop
	Quests   []Quest
	// UnlinkedBosses are the encounters the dataset cannot tie to a creature, so their drops are
	// unknown.
	UnlinkedBosses []string
}

// Result is a plan: ranked picks, then the dungeons with no level range.
type Result struct {
	Picks   []DungeonPick
	Unknown []DungeonPick
	Spec    string
	// ItemLevelOnly is true when upgrades were scored without stat weights.
	ItemLevelOnly bool
	Reason        string // why scoring fell back to item level, when it did
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
	cands := map[int64]gear.Candidate{} // best candidate per item id
	for _, list := range gr.Slots {
		for _, cd := range list {
			if old, ok := cands[cd.Item.ID]; !ok || cd.Delta > old.Delta {
				cands[cd.Item.ID] = cd
			}
		}
	}

	rows, err := ds.Filter("instances", store.Range{Column: "map_id", Min: int64(0)})
	if err != nil {
		return res, errors.New("instances table: " + err.Error())
	}
	for _, row := range rows {
		if row["instance_type"] != "dungeon" {
			continue
		}
		p := DungeonPick{MapID: i64(row["map_id"]), MinLevel: optInt(row["min_level"]), MaxLevel: optInt(row["max_level"]), MaxPlayers: optInt(row["max_players"])}
		p.Name, _ = row["name"].(string)
		lo, hi := p.MinLevel, p.MaxLevel
		if lo == nil || hi == nil {
			p.LevelUnknown = true
		} else {
			if *hi < level-LevelWindow || *lo > level+LevelWindow {
				continue
			}
			switch {
			case level < *lo:
				p.Fit = *lo - level
			case level > *hi:
				p.Fit = level - *hi
			}
		}
		fill(&p, c, ds, cands)
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
	sort.SliceStable(res.Unknown, func(i, j int) bool { return res.Unknown[i].MapID < res.Unknown[j].MapID })
	return res, nil
}

// fill adds the boss drops that are upgrades, and the linked open quests, to a pick.
func fill(p *DungeonPick, c character.Character, ds store.DataStore, cands map[int64]gear.Candidate) {
	bosses, _ := ds.Filter("instance_bosses", store.Range{Column: "map_id", Min: p.MapID, Max: p.MapID})
	seen := map[[2]int64]bool{}
	items := map[int64]bool{}
	for _, b := range bosses {
		name, _ := b["name"].(string)
		cid, ok := b["creature_id"].(int64)
		if !ok {
			p.UnlinkedBosses = append(p.UnlinkedBosses, name)
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
	p.Upgrades = len(items)
	sort.SliceStable(p.Drops, func(i, j int) bool {
		if p.Drops[i].Delta != p.Drops[j].Delta {
			return p.Drops[i].Delta > p.Drops[j].Delta
		}
		return p.Drops[i].ItemID < p.Drops[j].ItemID
	})
	p.Quests = openQuests(p, c, ds)
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

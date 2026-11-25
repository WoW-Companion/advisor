package session

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/digest"
	"github.com/WoW-Companion/advisor/internal/levelup"
	"github.com/WoW-Companion/advisor/internal/questie"
	"github.com/WoW-Companion/advisor/internal/route"
	"github.com/WoW-Companion/advisor/internal/store"
)

// NPC flag bits of Questie's npcFlags.
const (
	flagInnkeeper = 0x80
	flagFlight    = 0x2000
)

var xpRow = regexp.MustCompile(`\[(\d+)\]\s*=\s*(\d+)`)

// ParseXP reads a quest XP table ("[quest id] = xp" rows, as in a Lua table) into a map; rows that do
// not parse are skipped.
func ParseXP(src string) map[int]int {
	out := map[int]int{}
	for _, m := range xpRow.FindAllStringSubmatch(src, -1) {
		id, _ := strconv.Atoi(m[1])
		xp, _ := strconv.Atoi(m[2])
		if id > 0 && xp > 0 {
			out[id] = xp
		}
	}
	return out
}

// LoadXP reads the quest XP table (xpDB-classic.lua) from the game folder of the capture; nil when
// there is none, in which case XP is estimated from quest levels.
func LoadXP(savedVariables string) map[int]int {
	game := questie.GameFromSavedVariables(savedVariables)
	if game == "" {
		return nil
	}
	for _, pat := range []string{
		filepath.Join(game, "support", "Forever", "QuestXP", "xpDB-classic.lua"),
		filepath.Join(game, "Interface", "AddOns", "*", "QuestXP", "xpDB-classic.lua"),
		filepath.Join(game, "Interface", "AddOns", "*", "support", "Forever", "QuestXP", "xpDB-classic.lua"),
	} {
		hits, _ := filepath.Glob(pat)
		for _, h := range hits {
			if raw, err := os.ReadFile(h); err == nil {
				if m := ParseXP(string(raw)); len(m) > 0 {
					return m
				}
			}
		}
	}
	return nil
}

// Live builds the inputs for a character from the route, the player's QuestieDB and the dataset.
// xp is the quest XP table (may be nil).
func Live(c character.Character, ds store.DataStore, qx *questie.Index, plan route.Plan, xp map[int]int) In {
	in := In{Char: c, Route: plan, Names: map[int]string{}}
	for _, q := range c.Quests {
		in.Names[q.QuestID] = q.Title
	}
	if qx == nil {
		return in
	}
	dungeonArea := map[int]bool{}
	for i := range qx.Dungeons {
		for _, a := range qx.Dungeons[i].Areas() {
			dungeonArea[a] = true
		}
	}
	follows := map[int]bool{}
	for _, q := range qx.Quests {
		for _, p := range q.PreSingle {
			follows[p] = true
		}
	}
	upgrades := digest.UpgradeQuests(c, ds, qx)
	in.Info = Info{
		Level: func(id int) int {
			if q := qx.Quests[id]; q != nil {
				return q.Level
			}
			return 0
		},
		XP: func(id int) (int, bool) { v, ok := xp[id]; return v, ok },
		Group: func(id int) bool {
			q := qx.Quests[id]
			if q == nil {
				return false
			}
			for _, r := range q.Creatures {
				e, ok := qx.Npcs[r.ID]
				if !ok || r.Object {
					continue
				}
				if e.Rank == 1 || e.Rank == 2 {
					return true
				}
				for _, sp := range e.Spawns {
					if dungeonArea[sp.Area] {
						return true
					}
				}
			}
			return false
		},
		Upgrade: func(id int) bool { _, ok := upgrades[id]; return ok },
		Chain:   func(id int) bool { return follows[id] },
	}
	for _, e := range qx.Npcs {
		kind := ""
		switch {
		case e.Flags&flagFlight != 0:
			kind = "the flight point"
		case e.Flags&flagInnkeeper != 0:
			kind = "the inn"
		default:
			continue
		}
		for _, sp := range e.Spawns {
			if a, ok := qx.Areas[sp.Area]; ok && a.UIMap != 0 {
				in.Anchors = append(in.Anchors, Anchor{Name: kind + " at " + e.Name + ", " + a.Zone, Map: a.UIMap, X: sp.X, Y: sp.Y})
			}
		}
	}
	if c.Level != nil && ds != nil {
		if sp := levelup.Spells(c, ds, *c.Level, *c.Level); len(sp) > 0 {
			if t, ok := levelup.NearestTrainer(c, qx); ok && plan.Zone != "" && strings.EqualFold(t.Zone, plan.Zone) && plan.Start != nil {
				in.Trainer = &route.Stop{Kind: "trainer", Label: t.String(), Map: plan.Start.Map, X: t.X, Y: t.Y}
			}
		}
	}
	return in
}

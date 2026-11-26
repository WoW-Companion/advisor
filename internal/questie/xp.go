package questie

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
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

// LoadXP reads the quest XP table (xpDB-classic.lua) from the game folder of a SavedVariables file;
// nil when there is none, in which case XP is estimated from quest levels.
func LoadXP(savedVariables string) map[int]int {
	game := GameFromSavedVariables(savedVariables)
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

// DungeonOf is the dungeon a quest leads to, nil when the index links it to none.
func (ix *Index) DungeonOf(quest int) *Dungeon {
	if ix == nil {
		return nil
	}
	for i := range ix.Dungeons {
		for _, id := range ix.Dungeons[i].Quests {
			if id == quest {
				return &ix.Dungeons[i]
			}
		}
	}
	return nil
}

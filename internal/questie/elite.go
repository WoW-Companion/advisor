package questie

// A quest is elite when the install's own quest row says so. The field numbers come from the install's
// src/meta/questMeta.lua (Layout), never from a table kept here: an install that names none of these
// fields has no elite answer, and Quest.Elite stays false.

// eliteTagFields are the quest fields, by the name a meta file gives them, that hold a quest tag or type.
var eliteTagFields = []string{"questTagId", "questTagID", "questTag", "questType"}

// eliteGroupFields are the quest fields that hold a suggested group size.
var eliteGroupFields = []string{"suggestedGroup", "suggestedGroupNum", "groupSize"}

// eliteTags are the game's quest tag ids for work that needs a group: group, raid, dungeon, heroic,
// 10-player raid and 25-player raid.
var eliteTags = map[int]bool{1: true, 62: true, 81: true, 85: true, 88: true, 89: true}

// eliteQuest reads the elite answer from a quest row's scalars.
func (l Layout) eliteQuest(s map[any]any) bool {
	for _, n := range eliteTagFields {
		if f := l.sc("quest", n); f > 0 && eliteTags[num(s[f])] {
			return true
		}
	}
	for _, n := range eliteGroupFields {
		if f := l.sc("quest", n); f > 0 && num(s[f]) >= 2 {
			return true
		}
	}
	return false
}

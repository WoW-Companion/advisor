package quests

import (
	"strings"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/questie"
	"github.com/WoW-Companion/advisor/internal/store"
)

// observed reads quest id from the dataset's quests_observed table (what players saw in game and
// reported to Everloot). The giver is named from npcs, with the NPC's zone; nil when the table or
// the row is missing.
func observed(ds store.DataStore, id int64) *Facts {
	if ds == nil {
		return nil
	}
	rows, err := ds.Filter("quests_observed", store.Range{Column: "quest_id", Min: id, Max: id})
	if err != nil || len(rows) == 0 {
		return nil
	}
	r := rows[0]
	f := &Facts{QuestID: id, Name: str(r["title"]), Level: int(i64(r["level"])), Faction: str(r["faction"]),
		Category: str(r["category"]), Source: SourceEverloot, Credit: Credit}
	if g := i64(r["giver_npc_id"]); g > 0 {
		f.Giver = npcLabel(ds, g)
	}
	if t := i64(r["turn_in_npc_id"]); t > 0 {
		f.TurnIn = npcLabel(ds, t)
	}
	return f
}

// ObservedLevel is the level players reported for quest id in the dataset's quests_observed table
// (Everloot); false when the table or the row is missing or holds no level.
func ObservedLevel(ds store.DataStore, id int64) (int, bool) {
	if f := observed(ds, id); f != nil && f.Level > 0 {
		return f.Level, true
	}
	return 0, false
}

// npcLabel is "Name (Zone)" for an NPC the dataset knows, else "".
func npcLabel(ds store.DataStore, id int64) string {
	n, err := ds.Get("npcs", id)
	if err != nil {
		return ""
	}
	name := str(n["name"])
	if name == "" {
		return ""
	}
	zone := str(n["zone_name"])
	if zone == "" {
		zone = str(n["located_in"])
	}
	if zone != "" {
		return name + " (" + zone + ")"
	}
	return name
}

// client reads the dataset's own quests table (client data): name, level and, when it has them,
// the giver and zone.
func client(ds store.DataStore, id int64) *Facts {
	if ds == nil {
		return nil
	}
	row, err := ds.Get("quests", id)
	if err != nil {
		return nil
	}
	f := &Facts{QuestID: id, Name: str(row["name"]), Level: int(i64(row["level"])), Source: SourceDataset}
	f.Giver = str(row["giver_name"])
	if f.Giver == "" {
		f.Giver = str(row["giver"])
	}
	if z := str(row["zone"]); z != "" && f.Giver != "" {
		f.Giver += " (" + z + ")"
	}
	return f
}

// FactsOf describes quest id, filling each fact from the first source that has it: the dataset's
// client tables, then quests_observed (Everloot), then the player's QuestieDB (ix may be nil). The
// Source is everloot when an Everloot fact was used, else questie; nil when no source knows the quest.
func FactsOf(ds store.DataStore, ix *questie.Index, id int64) *Facts {
	srcs := []*Facts{client(ds, id), observed(ds, id), QuestieFacts(ix, id)}
	out := &Facts{QuestID: id}
	used := map[string]bool{}
	pick := func(dst *string, v string, s *Facts) {
		if *dst == "" && v != "" {
			*dst = v
			used[s.Source] = true
		}
	}
	any := false
	for _, s := range srcs {
		if s == nil {
			continue
		}
		any = true
		pick(&out.Name, s.Name, s)
		pick(&out.Giver, s.Giver, s)
		pick(&out.TurnIn, s.TurnIn, s)
		pick(&out.Objective, s.Objective, s)
		pick(&out.Faction, s.Faction, s)
		pick(&out.Category, s.Category, s)
		pick(&out.Classes, s.Classes, s)
		pick(&out.Races, s.Races, s)
		if out.Level == 0 && s.Level > 0 {
			out.Level = s.Level
			used[s.Source] = true
		}
		if out.MinLevel == 0 {
			out.MinLevel = s.MinLevel
		}
		if out.Followers == nil {
			out.Followers, out.Precursors = s.Followers, s.Precursors
		}
	}
	if !any {
		return nil
	}
	switch {
	case used[SourceEverloot]:
		out.Source, out.Credit = SourceEverloot, Credit
	case used[SourceQuestie]:
		out.Source = SourceQuestie
	default:
		out.Source = SourceDataset
	}
	return out
}

// GiverName is the bare name of the quest's giver: the client tables' first, else the NPC that
// quests_observed (Everloot) saw give it; "" when neither knows. Callers fall back to Questie.
func GiverName(ds store.DataStore, id int64) string {
	if f := client(ds, id); f != nil && f.Giver != "" {
		return strings.TrimSpace(strings.SplitN(f.Giver, " (", 2)[0])
	}
	if f := observed(ds, id); f != nil && f.Giver != "" {
		return strings.TrimSpace(strings.SplitN(f.Giver, " (", 2)[0])
	}
	return ""
}

// GiverNameSeen is GiverName, falling back to the NPC the player saw give the quest (the addon's
// own record); seen is true only for that fallback, so the caller can mark it "seen by you".
func GiverNameSeen(ds store.DataStore, seen *character.Observed, id int64) (name string, fromSeen bool) {
	if n := GiverName(ds, id); n != "" {
		return n, false
	}
	if g, ok := seen.GiverOf(int(id)); ok && g.NPC != "" {
		return g.NPC, true
	}
	return "", false
}

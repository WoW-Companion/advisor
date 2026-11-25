package character

import (
	"sort"
	"strings"

	"github.com/WoW-Companion/advisor/internal/lua"
)

// WoWCVariable is the global the WoW Companion addon writes; its characters sit in `characters`
// (docs/capture-format.md of the addon).
const WoWCVariable = "WoWCompanionDB"

// invSlots names the client's inventory slot numbers the way ForeverScribe keys equipment.
var invSlots = map[int]string{
	1: "head", 2: "neck", 3: "shoulder", 4: "shirt", 5: "chest", 6: "waist", 7: "legs", 8: "feet",
	9: "wrist", 10: "hands", 11: "finger1", 12: "finger2", 13: "trinket1", 14: "trinket2",
	15: "back", 16: "mainhand", 17: "offhand", 18: "ranged", 19: "tabard",
}

func wowcEquipment(v any) map[string]EquippedItem {
	t := table(v)
	if len(t) == 0 {
		return nil
	}
	out := map[string]EquippedItem{}
	for k, e := range t {
		et, ok := e.(lua.Table)
		n := asInt(k)
		if !ok || n == nil {
			continue
		}
		name, ok := invSlots[*n]
		if !ok {
			continue
		}
		out[name] = EquippedItem{Slot: name, ItemID: asInt(et["id"]), Enchant: asInt(et["enchant"])}
	}
	return out
}

// wowcContainers flattens bags (or bank) = { [bag] = { size, items = { [slot] = {id, count} } } }.
func wowcContainers(v any) []ItemStack {
	t := table(v)
	bags, _ := each(t, func(b lua.Table) (lua.Table, bool) { return b, true })
	var out []ItemStack
	for _, b := range bags {
		items, _ := each(b["items"], func(e lua.Table) (ItemStack, bool) {
			id := asInt(e["id"])
			if id == nil {
				return ItemStack{}, false
			}
			count := 1
			if c := asInt(e["count"]); c != nil && *c > 0 {
				count = *c
			}
			return ItemStack{ItemID: *id, Count: count}, true
		})
		out = append(out, items...)
	}
	return out
}

func wowcQuest(e lua.Table) (Quest, bool) {
	id := asInt(e["id"])
	if id == nil {
		return Quest{}, false
	}
	objs, _ := each(e["objectives"], func(o lua.Table) (Objective, bool) {
		return Objective{Text: asStr(o["text"]), Have: asInt(o["have"]), Need: asInt(o["need"]), Done: asBool(o["done"])}, true
	})
	return Quest{QuestID: *id, Title: asStr(e["title"]), Level: asInt(e["level"]), Complete: asBool(e["complete"]),
		Failed: asBool(e["failed"]), Objectives: objs}, true
}

func wowcTalents(v any) *Talents {
	trees, ok := each(v, func(t lua.Table) (TalentTree, bool) {
		tree := TalentTree{Name: asStr(t["tree"])}
		pts := 0
		tree.Talents, _ = each(t["nodes"], func(n lua.Table) (Talent, bool) {
			r := derefInt(asInt(n["rank"]))
			pts += r
			return Talent{Name: asStr(n["name"]), Rank: r, MaxRank: asInt(n["max"])}, true
		})
		tree.Points = &pts
		return tree, true
	})
	if !ok {
		return nil
	}
	return &Talents{Trees: trees}
}

// LoadWoWCharacter builds one character from a WoWCompanionDB.characters record.
func LoadWoWCharacter(key string, rec lua.Table) Character {
	c := Character{
		Key: key, Name: asStr(rec["name"]), Realm: asStr(rec["realm"]), Level: asInt(rec["level"]),
		Class: asStr(rec["class"]), Race: asStr(rec["race"]), Faction: asStr(rec["faction"]),
		Money: asInt(rec["money"]), XP: asInt(rec["xp"]), XPMax: asInt(rec["xpMax"]), RestedXP: asInt(rec["rested"]),
		CapturedAt: asInt64(rec["capturedAt"]), Client: clientBlock(rec["client"]),
		Equipment: wowcEquipment(rec["equipment"]), Hearth: asStr(rec["hearth"]),
		Talents: wowcTalents(rec["talents"]), Source: SourceWoWC,
	}
	if c.Realm == "" {
		c.Key = strings.TrimPrefix(key, ":")
		if c.Key == "" {
			c.Key = c.Name
		}
	}
	if loc := table(rec["location"]); len(loc) > 0 {
		c.Location = &Location{MapID: asInt(loc["map"]), X: asFloat(loc["x"]), Y: asFloat(loc["y"])}
	}
	if rec["bags"] != nil {
		c.BagsCaptured = true
		c.Bags = wowcContainers(rec["bags"])
	}
	if rec["bank"] != nil || rec["bankSeenAt"] != nil {
		c.BankCaptured = true
		c.Bank = wowcContainers(rec["bank"])
	}
	if q := table(rec["quests"]); q != nil {
		c.Quests, _ = each(q["entries"], wowcQuest)
		c.QuestsCaptured = asBool(q["captured"]) == nil || *asBool(q["captured"])
	}
	for _, x := range lua.List(rec["completedQuests"]) {
		if i := asInt(x); i != nil {
			c.CompletedQuests = append(c.CompletedQuests, *i)
		}
	}
	c.Skills, c.SkillsCaptured = each(rec["skills"], func(e lua.Table) (Skill, bool) {
		s, ok := skill(e)
		if ok && asBool(e["profession"]) != nil && *asBool(e["profession"]) && !isProfessionGroup(s.Group) {
			s.Group = "Professions"
		}
		return s, ok
	})
	if c.SkillsCaptured {
		c.Professions = professions(nil, c.Skills)
	}
	c.Reputations, _ = each(rec["reputations"], func(e lua.Table) (Reputation, bool) {
		name := asStr(e["faction"])
		return Reputation{Name: name, Standing: asInt(e["standing"]), Value: asInt(e["value"]), Min: asInt(e["min"]), Max: asInt(e["max"])}, name != ""
	})
	if sp := lua.List(rec["spells"]); len(sp) > 0 {
		c.KnownSpells = map[int64]bool{}
		for _, x := range sp {
			if i := asInt64(x); i != nil {
				c.KnownSpells[*i] = true
			}
		}
	}
	c.Recipes = wowcRecipes(rec["recipes"])
	return c.WithRecipeProfessions()
}

// LoadWoWC builds models from parsed variables holding WoWCompanionDB.characters. The result has
// no characters (and HasWoWC false upstream) when the table is absent.
func LoadWoWC(vars map[string]any) (File, error) {
	db := table(vars[WoWCVariable])
	if db == nil {
		return File{}, &lua.ParseError{Line: 1, Msg: "no " + WoWCVariable + " table found"}
	}
	chars := table(db["characters"])
	keys := make([]string, 0, len(chars))
	for k, rec := range chars {
		if _, ok := rec.(lua.Table); ok {
			keys = append(keys, asStr(k))
		}
	}
	sort.Strings(keys)
	f := File{HasWoWC: len(keys) > 0}
	for _, k := range keys {
		f.Characters = append(f.Characters, LoadWoWCharacter(k, chars[k].(lua.Table)))
	}
	return f, nil
}

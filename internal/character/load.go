package character

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/WoW-Companion/advisor/internal/lua"
)

// DBVariable is the one global the addon writes.
const DBVariable = "ForeverScribeDB"

func asInt(v any) *int {
	switch n := v.(type) {
	case int64:
		i := int(n)
		return &i
	case float64:
		i := int(n)
		return &i
	}
	return nil
}

func asInt64(v any) *int64 {
	switch n := v.(type) {
	case int64:
		return &n
	case float64:
		i := int64(n)
		return &i
	}
	return nil
}

func asStr(v any) string {
	switch s := v.(type) {
	case string:
		return s
	case int64:
		return strconv.FormatInt(s, 10)
	case float64:
		return strconv.FormatFloat(s, 'f', -1, 64)
	}
	return ""
}

func asBool(v any) *bool {
	if b, ok := v.(bool); ok {
		return &b
	}
	return nil
}

func table(v any) lua.Table {
	t, _ := v.(lua.Table)
	return t
}

// each builds one value per table entry, ordered by key (numbers first). It reports whether the
// section was present.
func each[T any](v any, build func(lua.Table) (T, bool)) ([]T, bool) {
	t, ok := v.(lua.Table)
	if !ok {
		return nil, false
	}
	keys := make([]any, 0, len(t))
	for k := range t {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, aok := keys[i].(int64)
		b, bok := keys[j].(int64)
		if aok && bok {
			return a < b
		}
		if aok != bok {
			return aok
		}
		return fmt.Sprint(keys[i]) < fmt.Sprint(keys[j])
	})
	out := []T{}
	for _, k := range keys {
		if e, ok := t[k].(lua.Table); ok {
			if x, ok := build(e); ok {
				out = append(out, x)
			}
		}
	}
	return out, true
}

func equipment(v any) map[string]EquippedItem {
	t, ok := v.(lua.Table)
	if !ok {
		return nil
	}
	out := map[string]EquippedItem{}
	for slot, e := range t {
		et, ok := e.(lua.Table)
		if !ok {
			continue
		}
		s := asStr(slot)
		out[s] = EquippedItem{
			Slot: s, ItemID: asInt(et["itemID"]), Enchant: asInt(et["enchant"]),
			Link: asStr(et["link"]), Name: asStr(et["name"]), ItemLevel: asInt(et["ilvl"]),
			Quality: asInt(et["quality"]), Durability: asInt(et["dur"]),
			DurabilityMax: asInt(et["durMax"]), Stats: itemStats(et["stats"]),
		}
	}
	return out
}

func stack(e lua.Table) (ItemStack, bool) {
	id := asInt(e["itemID"])
	if id == nil {
		return ItemStack{}, false
	}
	count := 1
	if c := asInt(e["count"]); c != nil && *c != 0 {
		count = *c
	}
	return ItemStack{ItemID: *id, Count: count, Name: asStr(e["name"]), Link: asStr(e["link"])}, true
}

func quest(e lua.Table) (Quest, bool) {
	id := asInt(e["questID"])
	if id == nil {
		return Quest{}, false
	}
	objs, _ := each(e["obj"], func(o lua.Table) (Objective, bool) {
		return Objective{asStr(o["text"]), asInt(o["have"]), asInt(o["need"]), asBool(o["done"]), asStr(o["type"])}, true
	})
	return Quest{
		QuestID: *id, Title: asStr(e["title"]), Level: asInt(e["level"]), Zone: asStr(e["zone"]),
		Complete: asBool(e["complete"]), Failed: asBool(e["failed"]), Objectives: objs,
	}, true
}

func npc(e lua.Table) (NPC, bool) {
	n := NPC{ID: derefInt(asInt(e["id"])), Name: asStr(e["name"]), MapID: asInt(e["map"]), Zone: asStr(e["zone"]),
		Subzone: asStr(e["subzone"]), X: asFloat(e["x"]), Y: asFloat(e["y"]), Kind: asStr(e["kind"]),
		First: asInt64(e["first"]), Seen: asInt64(e["t"]), Count: asInt(e["n"])}
	n.Spots, _ = each(e["spots"], func(s lua.Table) (Spot, bool) {
		return Spot{X: asFloat(s["x"]), Y: asFloat(s["y"]), T: asInt64(s["t"]), N: asInt(s["n"])}, true
	})
	return n, n.Name != "" || n.ID != 0
}

func rank(n lua.Table) (int, *int) {
	r := 0
	for _, k := range []string{"rank", "currentRank", "spent"} {
		if v := asInt(n[k]); v != nil {
			r = *v
			break
		}
	}
	for _, k := range []string{"max", "maxRank", "maxRanks"} {
		if v := asInt(n[k]); v != nil {
			return r, v
		}
	}
	return r, nil
}

func talentTree(t lua.Table) (TalentTree, bool) {
	tree := TalentTree{Name: asStr(t["name"]), Key: asStr(t["key"]), Points: asInt(t["points"])}
	if cur := table(t["currency"]); cur != nil {
		tree.Unspent = asInt(cur["unspent"])
		tree.MaxPoints = asInt(cur["max"])
		tree.MaxAvail = asInt(cur["maxAvailable"])
		if tree.Points == nil {
			tree.Points = asInt(cur["spentInTree"])
		}
		if tree.Points == nil {
			tree.Points = asInt(cur["spent"])
		}
	}
	tree.Groups, _ = each(t["groups"], func(g lua.Table) (TalentGroup, bool) {
		return TalentGroup{Name: asStr(g["name"]), Order: asInt(g["order"]), Spent: derefInt(asInt(g["spent"]))}, true
	})
	tree.Talents, _ = each(t["talents"], func(n lua.Table) (Talent, bool) {
		r, max := rank(n)
		return Talent{
			Name: asStr(n["name"]), Group: asStr(n["group"]), Rank: r, MaxRank: max,
			NodeID: asInt(n["nodeID"]), SpellID: asInt(n["spellID"]), Type: asStr(n["type"]), CanBuy: asBool(n["canBuy"]),
		}, true
	})
	return tree, true
}

func derefInt(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

func talents(v any) *Talents {
	t := table(v)
	if t == nil {
		return nil
	}
	out := &Talents{System: asStr(t["system"]), Loadout: asStr(t["loadout"]), Unspent: asInt(t["unspent"])}
	out.Trees, _ = each(t["trees"], talentTree)
	var unspent *int
	for _, tr := range out.Trees {
		if tr.Unspent != nil && !tr.IsProfession() {
			if unspent == nil {
				unspent = new(int)
			}
			if *tr.Unspent > *unspent {
				*unspent = *tr.Unspent // the unspent pool is shared across a capture's trees
			}
		}
	}
	if unspent != nil {
		out.Unspent = unspent
	}
	return out
}

func skill(e lua.Table) (Skill, bool) {
	name := asStr(e["name"])
	if name == "" {
		return Skill{}, false
	}
	return Skill{name, asInt(e["skillID"]), asInt(e["rank"]), asInt(e["max"]), asStr(e["group"]), false}, true
}

func isProfessionGroup(group string) bool {
	g := strings.ToLower(group)
	return strings.Contains(g, "profession") || g == "primary skills" || g == "secondary skills"
}

// professions merges the profession list with the profession entries of `skills`, each
// profession once (by skill id, else by name); the list entry wins.
func professions(list, skills []Skill) []Skill {
	out := []Skill{}
	seen := map[string]bool{}
	add := func(s Skill) {
		key := strings.ToLower(s.Name)
		if s.SkillID != nil {
			key = "id:" + strconv.Itoa(*s.SkillID)
		}
		if seen[key] || seen["name:"+strings.ToLower(s.Name)] {
			return
		}
		seen[key], seen["name:"+strings.ToLower(s.Name)] = true, true
		out = append(out, s)
	}
	for _, s := range list {
		add(s)
	}
	for _, s := range skills {
		if isProfessionGroup(s.Group) {
			add(s)
		}
	}
	return out
}

func reputation(e lua.Table) (Reputation, bool) {
	id, name := asInt(e["id"]), asStr(e["name"])
	if id == nil || name == "" {
		return Reputation{}, false
	}
	return Reputation{*id, name, asInt(e["standing"]), asInt(e["value"]), asInt(e["min"]), asInt(e["max"]), asBool(e["atWar"]), asStr(e["group"])}, true
}

func clientBlock(v any) Client {
	t := table(v)
	if t == nil {
		return Client{}
	}
	return Client{Version: asStr(t["version"]), Build: asStr(t["build"]), TOC: asStr(t["toc"])}
}

func firstClient(vs ...any) Client {
	for _, v := range vs {
		if c := clientBlock(v); !c.Empty() {
			return c
		}
	}
	return Client{}
}

// LoadCharacter builds one character from its record. key is the record's key in `chars`.
func asFloat(v any) *float64 {
	if f, ok := v.(float64); ok {
		return &f
	}
	return nil
}

func location(v any) *Location {
	t := table(v)
	if len(t) == 0 {
		return nil
	}
	return &Location{Zone: asStr(t["zone"]), Subzone: asStr(t["subzone"]), MapName: asStr(t["mapName"]),
		MapID: asInt(t["mapID"]), X: asFloat(t["x"]), Y: asFloat(t["y"])}
}

func LoadCharacter(key string, rec lua.Table) Character {
	snap := table(rec["snapshot"])
	ident := table(snap["identity"])
	bags, bank := table(snap["bags"]), table(rec["bank"])
	c := Character{
		Name: asStr(ident["name"]), Realm: asStr(ident["realm"]), Level: asInt(snap["level"]),
		Class: asStr(ident["class"]), Race: asStr(ident["race"]), Faction: asStr(ident["faction"]),
		Money: asInt(snap["money"]), XP: asInt(snap["xp"]), XPMax: asInt(snap["xpMax"]),
		RestedXP: asInt(snap["rested"]), CapturedAt: asInt64(snap["capturedAt"]),
		Addon:     asStr(snap["addonVersion"]),
		Client:    firstClient(ident["client"], snap["client"], rec["client"]),
		Equipment: equipment(snap["equipment"]), Talents: talents(snap["talents"]),
		Location: location(snap["location"]), Hearth: asStr(snap["hearth"]),
		HearthHave: asBool(snap["hearthHave"]), HearthReadyIn: asInt(snap["hearthReadyIn"]), HearthCD: asInt(snap["hearthCooldown"]),
	}
	// The key is "Realm:Name"; with no realm captured the client leaves it empty, so key by name.
	c.Key = key
	if c.Realm == "" {
		c.Key = strings.TrimPrefix(key, ":")
		if c.Key == "" {
			c.Key = c.Name
		}
	}
	if bags != nil {
		c.BagsCaptured = true
		c.Bags, _ = each(bags["items"], stack)
	}
	if bank != nil {
		c.Bank, _ = each(bank["items"], stack)
		c.BankCaptured = bank["items"] != nil || bank["scannedAt"] != nil
	}
	c.Quests, c.QuestsCaptured = each(snap["quests"], quest)
	c.QuestPOI = questPOIs(cmpAny(rec["questPOI"], snap["questPOI"]))
	for _, x := range lua.List(snap["questsCompleted"]) {
		if i := asInt(x); i != nil {
			c.CompletedQuests = append(c.CompletedQuests, *i)
		}
	}
	// The npcs table sits on the character record; older captures may keep it in the snapshot.
	npcs := rec["npcs"]
	if npcs == nil {
		npcs = snap["npcs"]
	}
	c.NPCs, _ = each(npcs, npc)
	c.Skills, c.SkillsCaptured = each(snap["skills"], skill)
	listed, listedOK := each(snap["professions"], skill)
	if c.SkillsCaptured || listedOK {
		c.Professions = professions(listed, c.Skills)
	}
	if rep := table(rec["reputation"]); rep != nil {
		c.Reputations, _ = each(rep["factions"], reputation)
	}
	return c
}

// Load builds models from parsed variables (the result of lua.Parse).
func Load(vars map[string]any) (File, error) {
	db := table(vars[DBVariable])
	if db == nil {
		return File{}, &lua.ParseError{Line: 1, Msg: "no " + DBVariable + " table found"}
	}
	f := File{Addon: asStr(db["addonVersion"]), Schema: asInt(db["schema"]), Client: clientBlock(db["client"])}
	chars := table(db["chars"])
	byKey := map[string]lua.Table{}
	keys := make([]string, 0, len(chars))
	for k, rec := range chars {
		if rt, ok := rec.(lua.Table); ok {
			byKey[asStr(k)] = rt
			keys = append(keys, asStr(k))
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		c := LoadCharacter(k, byKey[k])
		c.Source = SourceFS
		f.Characters = append(f.Characters, c)
	}
	return f, nil
}

// Source names for Character.Source.
const (
	SourceWoWC = "WoW Companion addon"
	SourceFS   = "ForeverScribe"
)

// cmpAny is the first of a and b that is present.
func cmpAny(a, b any) any {
	if a != nil {
		return a
	}
	return b
}

// questPOIs reads questPOI[questID] = { map, x, y, seenAt }. A position given as a 0..1 fraction is
// turned into percent; an entry with no map or no position is dropped.
func questPOIs(v any) map[int]QuestPOI {
	t, ok := v.(lua.Table)
	if !ok {
		return nil
	}
	out := map[int]QuestPOI{}
	for k, e := range t {
		id, ok := k.(int64)
		et, ok2 := e.(lua.Table)
		if !ok || !ok2 {
			continue
		}
		m, x, y := asInt(et["map"]), asFloat(et["x"]), asFloat(et["y"])
		if m == nil || *m == 0 || x == nil || y == nil {
			continue
		}
		p := QuestPOI{Map: *m, X: *x, Y: *y}
		if p.X <= 1 && p.Y <= 1 {
			p.X, p.Y = p.X*100, p.Y*100
		}
		if p.X <= 0 && p.Y <= 0 {
			continue
		}
		if at := asInt64(et["seenAt"]); at != nil {
			p.SeenAt = *at
		}
		out[int(id)] = p
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// itemStats reads an addon's per-item stats table (name to amount); nil when absent or empty.
func itemStats(v any) map[string]float64 {
	t, ok := v.(lua.Table)
	if !ok {
		return nil
	}
	out := map[string]float64{}
	for k, e := range t {
		switch n := e.(type) {
		case int64:
			out[asStr(k)] = float64(n)
		case float64:
			out[asStr(k)] = n
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

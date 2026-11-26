package professions

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/advice"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/farm"
	"github.com/WoW-Companion/advisor/internal/gear"
	"github.com/WoW-Companion/advisor/internal/learn"
	"github.com/WoW-Companion/advisor/internal/quests"
	"github.com/WoW-Companion/advisor/internal/store"
)

// The offline quest, gear and recipe-source benchmark: the next quest in a chain, which gear piece counts as
// an upgrade, and where a recipe is learned or a material drops, beside the dataset rows, with no network.
// The tables come from testdata/places_snapshot (quest_lines, quests, quests_observed, npcs, trainers,
// vendor_sources) and testdata/advice_snapshot (items, recipes, recipe_items, trainer_spells, loot_sources).
// The reference of every case is read straight from those tables, never through the advisor. A case where the
// dataset has no answer passes when the advisor says so and names nothing. make offline-benchmark runs it;
// WOWC_BENCH_REPORT receives the result tables in Markdown.

const (
	linesSrc    = "dataset quest_lines (line_id, position, quest_id)"
	observedSrc = "dataset quests_observed (level, faction, title)"
	questsSrc   = "dataset quests (quest_line_id, level)"
	gearSrc     = "dataset items (item_level, required_level, slot, class_id, subclass_id, allowable_class_mask)"
	recipeSrc   = "dataset recipes, recipe_items, trainer_spells, trainers, vendor_sources, npcs"
	dropSrc     = "dataset loot_sources, npcs"
)

func (r benchRow) withResult(ok bool, bad string) benchRow {
	r.result = resultIf(ok, bad)
	return r
}

// refLine is the dataset's quest_lines rows of one line, in position order.
func refLine(t *testing.T, ds store.DataStore, line int64) (name string, ids []int64) {
	t.Helper()
	rows, err := ds.Filter("quest_lines", store.Range{Column: "line_id", Min: line, Max: line})
	if err != nil || len(rows) == 0 {
		t.Fatalf("quest line %d is not in the snapshot: %v", line, err)
	}
	sort.Slice(rows, func(i, j int) bool { return asI64(rows[i]["position"]) < asI64(rows[j]["position"]) })
	for _, r := range rows {
		ids = append(ids, asI64(r["quest_id"]))
	}
	return asText(rows[0]["name"]), ids
}

// chainCases are quests of the dataset's quest lines, by line and position (from 0).
var chainCases = []struct {
	line int64
	pos  int
}{{6026, 0}, {6026, 3}, {6026, 9}, {6026, 10}, {6293, 0}, {6293, 5}, {6293, 9}, {5963, 0}}

// TestOfflineBenchmarkQuestNextStep: the quest after a quest in a line, its place in the line and the words,
// beside the dataset's quest_lines rows.
func TestOfflineBenchmarkQuestNextStep(t *testing.T) {
	ds := placesStore()
	var out []benchRow
	for _, tc := range chainCases {
		lname, ids := refLine(t, ds, tc.line)
		id := ids[tc.pos]
		wantNext := int64(0)
		if tc.pos+1 < len(ids) {
			wantNext = ids[tc.pos+1]
		}
		c := quests.Of(ds, id)
		got := fmt.Sprintf("step %d of %d in %s, next %d", c.Position, c.Total(), c.Line, c.NextQuestID)
		ref := fmt.Sprintf("step %d of %d in %s, next %d", tc.pos+1, len(ids), lname, wantNext)
		text := quests.Render(c, "")
		wording := strings.Contains(text, fmt.Sprintf("is step %d of %d in %s", tc.pos+1, len(ids), lname))
		if wantNext == 0 {
			wording = wording && strings.Contains(text, "it is the last step")
		} else {
			wording = wording && strings.Contains(text, fmt.Sprintf("next is quest %d", wantNext))
		}
		out = append(out, benchRow{fmt.Sprintf("quest %d (%s, position %d): next quest", id, lname, tc.pos), got, ref, linesSrc, resultIf(got == ref && wording, "the place, the next quest or the words")})
	}

	// the turn-in step of a quest log: a ready quest says what it unlocks, the last one does not
	for _, tc := range []struct {
		line int64
		pos  int
	}{{6026, 4}, {6293, 9}} {
		lname, ids := refLine(t, ds, tc.line)
		yes := true
		lvl := 60
		c := character.Character{Class: "WARRIOR", Faction: "Horde", Level: &lvl,
			Quests: []character.Quest{{QuestID: int(ids[tc.pos]), Title: "Synthetic Step", Complete: &yes}}}
		steps := advice.WhatsNext(c, ds)
		reason := ""
		if len(steps) > 0 {
			reason = steps[0].Reason
		}
		want := fmt.Sprintf("step %d of %d in %s", tc.pos+1, len(ids), lname)
		unlock := fmt.Sprintf("unlocks step %d", tc.pos+2)
		ok := strings.Contains(reason, want) && (tc.pos+1 < len(ids)) == strings.Contains(reason, unlock)
		ref := want + ", " + unlock
		if tc.pos+1 == len(ids) {
			ref = want + ", unlocks nothing"
		}
		out = append(out, benchRow{fmt.Sprintf("turn in quest %d, position %d of %d: what it unlocks", ids[tc.pos], tc.pos, len(ids)), reason, ref, linesSrc, resultIf(ok, "the position or the unlock")})
	}
	finishOffline(t, "advice-quest-chain", "Offline quest next step", out)
}

// questLevel is the quests_observed row of a quest: its level and faction, ok false when the dataset has none.
func questLevel(ds store.DataStore, id int64) (level int, faction string, ok bool) {
	rows, _ := ds.Filter("quests_observed", store.Range{Column: "quest_id", Min: id, Max: id})
	if len(rows) == 0 || rows[0]["level"] == nil {
		return 0, "", false
	}
	return int(asI64(rows[0]["level"])), asText(rows[0]["faction"]), true
}

// observedByLevel finds the first observed quests at the offsets from a character level, so the gate cases do
// not depend on a quest id picked by hand: below the grey window, inside it, and above the hard window.
func observedAt(t *testing.T, ds store.DataStore, level int) (id int64, title string) {
	t.Helper()
	rows, _ := ds.Filter("quests_observed", store.Range{Column: "level", Min: int64(level), Max: int64(level)})
	sort.Slice(rows, func(i, j int) bool { return asI64(rows[i]["quest_id"]) < asI64(rows[j]["quest_id"]) })
	for _, r := range rows {
		if asText(r["title"]) != "" {
			return asI64(r["quest_id"]), asText(r["title"])
		}
	}
	t.Fatalf("no observed quest of level %d in the snapshot", level)
	return 0, ""
}

// TestOfflineBenchmarkQuestLevelGate: a quest in the log is listed when its level is within the window the
// advisor keeps (5 below, 3 above) and left out when it is grey or too hard, by the level the dataset's
// quests_observed table holds for it.
func TestOfflineBenchmarkQuestLevelGate(t *testing.T) {
	ds := placesStore()
	var out []benchRow
	for _, tc := range []struct{ char, quest int }{{20, 20}, {20, 23}, {20, 15}, {20, 24}, {20, 14}, {40, 25}, {10, 12}} {
		id, title := observedAt(t, ds, tc.quest)
		lvl, _, _ := questLevel(ds, id)
		c := character.Character{Class: "MAGE", Faction: "Horde", Level: &tc.char,
			Quests: []character.Quest{{QuestID: int(id), Title: title}}}
		steps := advice.WhatsNext(c, ds)
		var listed bool
		reason := "not listed"
		for _, s := range steps {
			if s.QuestID == int(id) {
				listed, reason = true, s.Reason
			}
		}
		d := lvl - tc.char
		wantListed := d >= -advice.MaxBelow && d <= advice.MaxAbove
		ref := "not listed (level gate)"
		if wantListed {
			ref = fmt.Sprintf("level %d quest, you are %d", lvl, tc.char)
		}
		ok := listed == wantListed && (!wantListed || strings.HasPrefix(reason, ref))
		out = append(out, benchRow{fmt.Sprintf("level %d character, quest %d (level %d)", tc.char, id, lvl), reason, ref, observedSrc, resultIf(ok, "the quest level gate read the dataset's level")})
	}
	finishOffline(t, "advice-quest-level", "Offline quest level gate", out)
}

// TestOfflineBenchmarkQuestFactionFacts: the faction and level the advisor says for a quest, beside the
// dataset's quests_observed row.
func TestOfflineBenchmarkQuestFactionFacts(t *testing.T) {
	ds := placesStore()
	var out []benchRow
	for _, want := range []string{"horde", "alliance", ""} {
		rows, _ := ds.Filter("quests_observed", store.Range{Column: "level", Min: int64(1), Max: int64(60)})
		sort.Slice(rows, func(i, j int) bool { return asI64(rows[i]["quest_id"]) < asI64(rows[j]["quest_id"]) })
		for _, r := range rows {
			if asText(r["faction"]) != want || asText(r["title"]) == "" {
				continue
			}
			id := asI64(r["quest_id"])
			f := quests.FactsOf(ds, nil, id)
			if f == nil {
				out = append(out, benchRow{fmt.Sprintf("quest %d facts", id), "no facts", want, observedSrc, "differ: the quest is in the dataset"})
				break
			}
			got := fmt.Sprintf("%s, level %d, %s", f.Name, f.Level, f.Faction)
			ref := fmt.Sprintf("%s, level %d, %s", asText(r["title"]), asI64(r["level"]), want)
			c := quests.OfWith(ds, nil, id)
			text := quests.Render(c, f.Name)
			ok := got == ref && strings.Contains(text, f.Name) && (want == "" || strings.Contains(text, want))
			out = append(out, benchRow{fmt.Sprintf("quest %d: name, level, faction", id), got, ref, observedSrc, resultIf(ok, "a fact or the words")})
			break
		}
	}
	finishOffline(t, "advice-quest-facts", "Offline quest facts and faction", out)
}

// TestOfflineBenchmarkQuestNoAnswer: quests and lines the dataset has nothing for. The advisor says so.
func TestOfflineBenchmarkQuestNoAnswer(t *testing.T) {
	ds := placesStore()
	var out []benchRow

	// a quest the client lists that sits in no quest line
	var free int64
	rows, _ := ds.Filter("quests", store.Range{Column: "id", Min: int64(1), Max: int64(1 << 30)})
	for _, r := range rows {
		if r["quest_line_id"] == nil {
			free = asI64(r["id"])
			break
		}
	}
	c := quests.Of(ds, free)
	text := quests.Render(c, "")
	out = append(out, benchRow{fmt.Sprintf("quest %d in no line: next quest", free), fmt.Sprintf("found %v, next %d", c.Found, c.NextQuestID), placesNoReply + ": no quest_lines row", linesSrc,
		resultIf(!c.Found && c.NextQuestID == 0 && strings.Contains(text, "No quest line or follow-up quest is listed"), "a next quest was named")})

	c = quests.Of(ds, 99999999)
	out = append(out, benchRow{"a quest id the dataset does not hold: next quest", fmt.Sprintf("found %v", c.Found), placesNoReply, linesSrc,
		resultIf(!c.Found && c.NextQuestID == 0, "a chain was invented")})

	id, _, cands := quests.Resolve(ds, "Quest Of The Unwritten Moon")
	out = append(out, benchRow{"a quest name the dataset does not hold", fmt.Sprintf("id %d, %d candidates", id, len(cands)), placesNoReply, observedSrc,
		resultIf(id == 0 && len(cands) == 0, "a quest was matched")})

	// a quest in the log whose level the dataset holds nowhere
	lvl := 30
	var bare int64
	for _, r := range rows {
		i := asI64(r["id"])
		if _, _, ok := questLevel(ds, i); !ok && r["level"] == nil {
			bare = i
			break
		}
	}
	ch := character.Character{Class: "MAGE", Faction: "Horde", Level: &lvl, Quests: []character.Quest{{QuestID: int(bare), Title: "Synthetic Errand"}}}
	steps := advice.WhatsNext(ch, ds)
	reason := "not listed"
	for _, s := range steps {
		if s.QuestID == int(bare) {
			reason = s.Reason
		}
	}
	out = append(out, benchRow{fmt.Sprintf("quest %d in the log with no level in the dataset", bare), reason, placesNoReply + ": no level", questsSrc,
		resultIf(strings.HasPrefix(reason, "quest level unknown"), "a level was guessed")})

	// quests to pick up: the dataset's quests table has no level or mask column, so nothing is suggested and the advisor says why
	av := advice.AvailableQuests(character.Character{Class: "MAGE", Faction: "Horde", Level: &lvl}, ds)
	out = append(out, benchRow{"quests to pick up at level 30", fmt.Sprintf("%d suggested, %d unchecked rules", len(av.Quests), len(av.Unchecked)), placesNoReply + ": no quest level column values", questsSrc,
		resultIf(len(av.Quests) == 0 && len(av.Unchecked) > 0, "a pick-up was guessed or the gap was not named")})

	// an Everloot quest with no giver: the giver is not guessed
	f := quests.FactsOf(ds, nil, mustObservedWithoutGiver(t, ds))
	out = append(out, benchRow{"an observed quest with no giver: the giver", fmt.Sprintf("%q", f.Giver), placesNoReply + ": giver_npc_id is null", observedSrc, resultIf(f.Giver == "", "a giver was named")})
	finishOffline(t, "advice-quest-no-answer", "Offline quest advice, no answer in the dataset", out)
}

func mustObservedWithoutGiver(t *testing.T, ds store.DataStore) int64 {
	t.Helper()
	rows, _ := ds.Filter("quests_observed", store.Range{Column: "level", Min: int64(1), Max: int64(60)})
	sort.Slice(rows, func(i, j int) bool { return asI64(rows[i]["quest_id"]) < asI64(rows[j]["quest_id"]) })
	for _, r := range rows {
		if r["giver_npc_id"] == nil {
			return asI64(r["quest_id"])
		}
	}
	t.Fatal("every observed quest has a giver")
	return 0
}

// ---- gear upgrades ----

// slotTypes are the inventory types (items.slot) that fit a capture slot, written out here from the game's
// inventory type ids.
var slotTypes = map[string][]int64{
	"head": {1}, "neck": {2}, "shoulder": {3}, "chest": {5, 20}, "waist": {6}, "legs": {7}, "feet": {8}, "wrist": {9},
	"hands": {10}, "finger1": {11}, "back": {16}, "mainhand": {13, 17, 21}, "offhand": {13, 14, 22, 23}, "ranged": {15, 25, 26},
}

// armorOf is the armor subclass the class wears in the dataset's items.subclass_id (1 cloth, 2 leather, 3 mail,
// 4 plate), by level.
func armorOf(class string, level int) int64 {
	switch class {
	case "MAGE", "PRIEST", "WARLOCK":
		return 1
	case "ROGUE", "DRUID":
		return 2
	case "HUNTER", "SHAMAN":
		if level < 40 {
			return 2
		}
		return 3
	}
	if level < 40 { // warrior, paladin
		return 3
	}
	return 4
}

// usefulStats are the stat types (items.stat_types: 3 agility, 4 strength, 5 intellect, 6 spirit, 7 stamina, 41 healing,
// 42 spell damage, 45 spell power) a class gains from; an item whose stats are all outside the set is no upgrade.
var usefulStats = map[string][]int64{
	"MAGE": {5, 6, 7, 41, 42, 45}, "WARLOCK": {5, 6, 7, 41, 42, 45}, "PRIEST": {5, 6, 7, 41, 42, 45},
	"WARRIOR": {3, 4, 7}, "ROGUE": {3, 4, 7}, "HUNTER": {3, 4, 5, 7}, "PALADIN": {4, 5, 6, 7, 41, 42, 45},
	"SHAMAN": {3, 4, 5, 6, 7, 41, 42, 45}, "DRUID": {3, 4, 5, 6, 7, 41, 42, 45},
}

var armorSlots = map[string]bool{"head": true, "shoulder": true, "chest": true, "waist": true, "legs": true, "feet": true, "wrist": true, "hands": true}

var classBitOf = map[string]int64{"WARRIOR": 1, "PALADIN": 2, "HUNTER": 3, "ROGUE": 4, "PRIEST": 5, "SHAMAN": 7, "MAGE": 8, "WARLOCK": 9, "DRUID": 11}

var gearCases = []struct {
	class      string
	level      int
	slot       string
	worn       int64 // an items id worn in the slot; 0 for an empty slot
	wornLevel  int   // item level the capture states for the slot when no item id
	minOffered int
}{
	{"MAGE", 30, "chest", 0, 0, 1},
	{"MAGE", 33, "chest", 7065, 0, 0},
	{"WARRIOR", 30, "chest", 0, 0, 1},
	{"WARRIOR", 45, "chest", 0, 0, 0},
	{"ROGUE", 35, "legs", 0, 0, 0},
	{"PRIEST", 25, "head", 0, 0, 0},
	{"HUNTER", 35, "feet", 0, 0, 0},
	{"PALADIN", 50, "chest", 0, 0, 0},
	{"DRUID", 40, "hands", 0, 0, 0},
	{"SHAMAN", 45, "shoulder", 0, 0, 0},
	{"WARLOCK", 20, "wrist", 0, 0, 0},
	{"WARRIOR", 40, "offhand", 0, 0, 0},
	{"MAGE", 40, "back", 0, 0, 0},
	{"MAGE", 40, "offhand", 0, 0, 0},
	{"ROGUE", 40, "mainhand", 0, 0, 0},
}

// TestOfflineBenchmarkGearUpgrades: every item the gear finder offers for a slot is one the dataset's rows allow
// for the class: its slot fits, its armor type is the class's, its class mask includes the class, it asks no more
// than the equip window above the level and it beats the worn item's level and a candidate with none of the stats the class uses ranks after every one that has some.
func TestOfflineBenchmarkGearUpgradesA(t *testing.T) { gearUpgrades(t, "a", gearCases[:5]) }
func TestOfflineBenchmarkGearUpgradesB(t *testing.T) { gearUpgrades(t, "b", gearCases[5:10]) }
func TestOfflineBenchmarkGearUpgradesC(t *testing.T) { gearUpgrades(t, "c", gearCases[10:]) }

func gearUpgrades(t *testing.T, part string, cases []struct {
	class      string
	level      int
	slot       string
	worn       int64
	wornLevel  int
	minOffered int
}) {
	ds := placesStore()
	var out []benchRow
	for _, g := range cases {
		lvl := g.level
		c := character.Character{Class: g.class, Level: &lvl, Profile: &character.Profile{Role: "damage", Focus: "leveling"}}
		wornIlvl := int64(0)
		if g.worn != 0 {
			id := int(g.worn)
			c.Equipment = map[string]character.EquippedItem{g.slot: {Slot: g.slot, ItemID: &id}}
			row, err := ds.Get("items", g.worn)
			if err != nil {
				t.Fatalf("worn item %d: %v", g.worn, err)
			}
			wornIlvl = asI64(row["item_level"])
		}
		res, err := gear.Find(c, ds, "", gear.Options{Slot: g.slot, NoSources: true})
		if err != nil {
			t.Fatal(err)
		}
		cands := res.Slots[g.slot]
		var bad, names []string
		firstUseless := ""
		for _, cd := range cands {
			row, err := ds.Get("items", cd.Item.ID)
			if err != nil {
				bad = append(bad, fmt.Sprintf("%d is not in the dataset", cd.Item.ID))
				continue
			}
			names = append(names, fmt.Sprintf("%s (ilvl %d)", asText(row["name"]), asI64(row["item_level"])))
			fits := false
			for _, s := range slotTypes[g.slot] {
				fits = fits || s == asI64(row["slot"])
			}
			if !fits {
				bad = append(bad, fmt.Sprintf("%s: slot %d", asText(row["name"]), asI64(row["slot"])))
			}
			if armorSlots[g.slot] && asI64(row["class_id"]) == 4 && asI64(row["subclass_id"]) != armorOf(g.class, g.level) {
				bad = append(bad, fmt.Sprintf("%s: armor type %d", asText(row["name"]), asI64(row["subclass_id"])))
			}
			if m := asI64(row["allowable_class_mask"]); m > 0 && m&(1<<(classBitOf[g.class]-1)) == 0 {
				bad = append(bad, fmt.Sprintf("%s: not for the class", asText(row["name"])))
			}
			if rl := asI64(row["required_level"]); rl > int64(g.level+gear.EquipWindow) {
				bad = append(bad, fmt.Sprintf("%s: needs level %d", asText(row["name"]), rl))
			}
			// an item with none of the class's stats may fill an empty slot, but only after every item that has one
			types := ints64(row["stat_types"])
			if len(types) > 0 && !anyIn(types, usefulStats[g.class]) {
				if firstUseless == "" {
					firstUseless = asText(row["name"])
				}
			} else if len(types) > 0 && firstUseless != "" {
				bad = append(bad, fmt.Sprintf("%s (stats %v) is ranked after %s, which has no stat the class uses", asText(row["name"]), types, firstUseless))
			}
			if asI64(row["item_level"]) <= wornIlvl {
				bad = append(bad, fmt.Sprintf("%s: item level %d, worn %d", asText(row["name"]), asI64(row["item_level"]), wornIlvl))
			}
		}
		label := fmt.Sprintf("%s level %d, %s", strings.ToLower(g.class), g.level, g.slot)
		if g.worn != 0 {
			label += fmt.Sprintf(" (worn item level %d)", wornIlvl)
		}
		adv := fmt.Sprintf("%d offered: %s", len(cands), strings.Join(names, ", "))
		out = append(out, benchRow{label + ": upgrades", adv, "every item fits the slot, class, level and beats the worn item", gearSrc,
			resultIf(len(bad) == 0 && len(cands) >= g.minOffered, strings.Join(bad, "; "))})
	}
	finishOffline(t, "advice-gear-"+part, "Offline gear upgrades against the dataset ("+part+")", out)
}

func anyIn(have, want []int64) bool {
	for _, h := range have {
		for _, w := range want {
			if h == w {
				return true
			}
		}
	}
	return false
}

// TestOfflineBenchmarkGearNoAnswer: gear the dataset cannot rank. The advisor says it cannot and offers nothing.
func TestOfflineBenchmarkGearNoAnswer(t *testing.T) {
	ds := placesStore()
	var out []benchRow
	lvl := 30
	_, err := gear.Find(character.Character{Class: "DEATHKNIGHT", Level: &lvl}, ds, "", gear.Options{})
	out = append(out, benchRow{"a class with no gear rules", fmt.Sprint(err), placesNoReply + ": no armor rule for the class", gearSrc,
		resultIf(err != nil && strings.Contains(err.Error(), "no gear rules"), "gear was offered")})
	_, err = gear.Find(character.Character{Class: "MAGE"}, ds, "", gear.Options{})
	out = append(out, benchRow{"a capture with no level", fmt.Sprint(err), placesNoReply + ": no level to gate on", gearSrc,
		resultIf(err != nil && strings.Contains(err.Error(), "no character level"), "gear was offered")})
	res, err := gear.Find(character.Character{Class: "MAGE", Level: levelOf(1)}, ds, "", gear.Options{Slot: "neck"})
	for _, cd := range res.Slots["neck"] {
		if cd.Basis == gear.BasisStats && !cd.Item.StatsKnown {
			err = fmt.Errorf("%s ranked on stats the dataset lacks", cd.Item.Name)
		}
	}
	out = append(out, benchRow{"a level 1 neck: stat-ranked items need known stats", fmt.Sprintf("%d offered, error %v", len(res.Slots["neck"]), err), "items with no stat rows are ranked by item level, not stats", gearSrc,
		resultIf(err == nil, "an item without stats was ranked by stats")})
	finishOffline(t, "advice-gear-no-answer", "Offline gear advice, no answer in the dataset", out)
}

// ---- recipe sources ----

func npcName(ds store.DataStore, id int64) string {
	n, err := ds.Get("npcs", id)
	if err != nil {
		return ""
	}
	return asText(n["name"])
}

// refVendors are the names of the creatures that sell the item in vendor_sources, open to the side, and how many
// vendor rows the table holds for it (a vendor with no name in npcs is counted but not named).
func refVendors(ds store.DataStore, item int64, faction string) (named []string, rows int) {
	vs, _ := ds.Filter("vendor_sources", store.Range{Column: "item_id", Min: item, Max: item})
	for _, r := range vs {
		rows++
		n, err := ds.Get("npcs", asI64(r["npc_id"]))
		if err != nil || asText(n["name"]) == "" || !learn.FactionOK(asText(n["faction"]), faction) {
			continue
		}
		named = append(named, asText(n["name"]))
	}
	sort.Strings(named)
	return named, rows
}

var vendorRecipeCases = []struct {
	recipe, faction string
	item            int64
}{
	{"Brilliant Smallfish", "Horde", 6325}, {"Rainbow Fin Albacore", "Alliance", 6368}, {"Blue Linen Robe", "Horde", 6272},
	{"Blue Overalls", "Alliance", 6274}, {"Smoked Bear Meat", "Horde", 6892}, {"Enchant Chest - Minor Intellect", "Alliance", 6342},
}

// TestOfflineBenchmarkRecipeVendor: a recipe a recipe item teaches, sold by vendors in vendor_sources. The advisor
// names the vendors the dataset names for the side and no one else; when the dataset names none it says the
// recipe is learned from the item and names no place, and does not send the player to a trainer.
func TestOfflineBenchmarkRecipeVendor(t *testing.T) {
	ds := placesStore()
	var out []benchRow
	for _, tc := range vendorRecipeCases {
		res, err := learn.Find(character.Character{Faction: tc.faction, Class: "MAGE"}, ds, nil, tc.recipe)
		if err != nil {
			t.Fatalf("%s: %v", tc.recipe, err)
		}
		itemRow, _ := ds.Get("items", tc.item)
		named, rows := refVendors(ds, tc.item, tc.faction)
		var got []string
		ok := true
		for _, w := range res.Ways {
			ok = ok && w.Kind == learn.Vendor && strings.Contains(strings.Join(named, "|"), w.Name)
			got = append(got, w.Name)
		}
		ref := fmt.Sprintf("learned from %s; %d vendor rows, %d named", asText(itemRow["name"]), rows, len(named))
		if len(named) > 0 {
			ok = ok && len(got) > 0 && strings.Contains(res.Text, "Buy the recipe from")
		} else {
			ok = ok && len(got) == 0 && strings.Contains(res.Text, "learned from the recipe item "+asText(itemRow["name"])) && !strings.Contains(res.Text, "trainer")
		}
		out = append(out, benchRow{fmt.Sprintf("%s (%s): where to learn it", tc.recipe, tc.faction), firstLine(res.Text), ref, recipeSrc, resultIf(ok, "a vendor the dataset does not name for the side, or a trainer for an item recipe")})
		if rows > 0 && len(named) == 0 {
			out = append(out, benchRow{fmt.Sprintf("%s (%s): vendor name", tc.recipe, tc.faction), "no vendor named", fmt.Sprintf("%d vendors sell it", rows), recipeSrc,
				"differ (data): npcs holds no name for any vendor of the recipe item"})
		}
	}
	finishOffline(t, "advice-recipe-vendor", "Offline recipe source: vendors", out)
}

var trainerRecipeCases = []struct{ recipe, faction string }{
	{"Cured Medium Hide", "Horde"}, {"Linen Bag", "Alliance"}, {"Barbaric Linen Vest", "Horde"}, {"Woolen Cape", "Alliance"},
}

// TestOfflineBenchmarkRecipeTrainer: a recipe a trainer teaches: the skill it needs is the lowest required_skill
// above 1 in trainer_spells (else the recipe's own learn skill), and the advisor names a trainer only when
// the trainers table names one.
func TestOfflineBenchmarkRecipeTrainer(t *testing.T) {
	ds := placesStore()
	var out []benchRow
	for _, tc := range trainerRecipeCases {
		res, err := learn.Find(character.Character{Faction: tc.faction, Class: "MAGE"}, ds, nil, tc.recipe)
		if err != nil {
			t.Fatalf("%s: %v", tc.recipe, err)
		}
		recs, _ := ds.SearchName("recipes", tc.recipe, 5)
		spell := asI64(recs[0]["spell_id"])
		need := int64(0)
		named, trainers := 0, 0
		trs, _ := ds.Filter("trainer_spells", store.Range{Column: "spell_id", Min: spell, Max: spell})
		for _, r := range trs {
			trainers++
			if n := asI64(r["required_skill"]); n > 1 && (need == 0 || n < need) {
				need = n
			}
			if row, err := ds.Get("trainers", asI64(r["npc_id"])); err == nil && asText(row["name"]) != "" {
				named++
			}
		}
		if need == 0 {
			need = asI64(recs[0]["learn_skill"])
		}
		ref := fmt.Sprintf("a %s trainer; needs %d; %d trainers, %d named", asText(recs[0]["skill_line_name"]), need, trainers, named)
		got := firstLine(res.Text)
		ok := res.Gate.Skill == int(need) && strings.Contains(res.Text, fmt.Sprintf("It needs %s %d.", asText(recs[0]["skill_line_name"]), need))
		if named == 0 {
			ok = ok && len(res.Ways) == 0 && strings.HasPrefix(res.Text, "Ask a "+asText(recs[0]["skill_line_name"])+" trainer")
		} else {
			ok = ok && len(res.Ways) > 0 && res.Ways[0].Kind == learn.Trainer
		}
		out = append(out, benchRow{fmt.Sprintf("%s (%s): trainer and skill", tc.recipe, tc.faction), got + " / " + res.Gate.Line, ref, recipeSrc, resultIf(ok, "the skill needed, or a trainer the dataset does not name")})
		if trainers > 0 && named == 0 {
			out = append(out, benchRow{fmt.Sprintf("%s (%s): trainer name", tc.recipe, tc.faction), "no trainer named", fmt.Sprintf("%d trainers teach it", trainers), recipeSrc,
				"differ (data): trainers.name is empty for every trainer of the recipe"})
		}
	}
	finishOffline(t, "advice-recipe-trainer", "Offline recipe source: trainers", out)
}

var dropCases = []struct {
	item  int64
	name  string
	level int
}{{4306, "Silk Cloth", 40}, {2592, "Wool Cloth", 25}, {3172, "Boar Intestines", 10}}

// TestOfflineBenchmarkMaterialDrops: where a material drops: the creatures the loot_sources rows name, each
// with the drop chance the rows give.
func TestOfflineBenchmarkMaterialDrops(t *testing.T) {
	ds := placesStore()
	var out []benchRow
	for _, tc := range dropCases {
		lvl := tc.level
		spots := farm.Spots(character.Character{Class: "MAGE", Level: &lvl}, ds, tc.item)
		rows, _ := ds.Filter("loot_sources", store.Range{Column: "item_id", Min: tc.item, Max: tc.item})
		type tot struct{ kills, drops int64 }
		ref := map[int64]*tot{}
		for _, r := range rows {
			if k := asText(r["source_kind"]); k != "creature" && k != "object" {
				continue
			}
			x := ref[asI64(r["source_id"])]
			if x == nil {
				x = &tot{}
				ref[asI64(r["source_id"])] = x
			}
			x.kills += asI64(r["kills"])
			x.drops += asI64(r["drops"])
		}
		ok := len(spots) == len(ref)
		var got []string
		for _, s := range spots {
			x := ref[s.ID]
			if x == nil || (s.Kind == "creature" && s.Name != npcName(ds, s.ID)) || s.Kills != x.kills || s.Drops != x.drops {
				ok = false
				continue
			}
			if x.kills > 0 && (s.Chance == nil || *s.Chance != 100*float64(x.drops)/float64(x.kills)) {
				ok = false
			}
			got = append(got, fmt.Sprintf("%s %d/%d", s.Name, s.Drops, s.Kills))
		}
		unnamed := 0
		for _, s := range spots {
			if s.Kind == "creature" && s.Name == "" {
				unnamed++
			}
		}
		if unnamed > 0 {
			out = append(out, benchRow{fmt.Sprintf("%s at level %d: creature names", tc.name, tc.level), fmt.Sprintf("%d of %d creatures unnamed", unnamed, len(spots)), "every dropping creature named", dropSrc,
				"differ (data): npcs holds no name for the creature"})
		}
		out = append(out, benchRow{fmt.Sprintf("%s at level %d: drops", tc.name, tc.level), fmt.Sprintf("%d creatures: %s", len(spots), strings.Join(firstN(got, 3), ", ")), fmt.Sprintf("%d creatures in loot_sources", len(ref)), dropSrc, resultIf(ok, "a creature, a name or a drop chance")})
	}
	finishOffline(t, "advice-recipe-drops", "Offline material drops", out)
}

func firstN(s []string, n int) []string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// TestOfflineBenchmarkRecipeNoAnswer: recipes whose source the dataset does not hold. The advisor says it does
// not know, and names no place.
func TestOfflineBenchmarkRecipeNoAnswer(t *testing.T) {
	ds := placesStore()
	var out []benchRow
	for _, tc := range []struct {
		name string
		item bool // learned from a recipe item
	}{{"Red Linen Vest", true}, {"Runed Copper Breastplate", true}, {"Westfall Stew", true}, {"Copper Chain Belt", false}} {
		res, err := learn.Find(character.Character{Faction: "Horde", Class: "MAGE"}, ds, nil, tc.name)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		// an item recipe is not sent to a trainer; a trainer recipe with no named trainer is sent to the profession's trainers
		words := "Ask a "
		if tc.item {
			words = "It is learned from the recipe item "
		}
		out = append(out, benchRow{tc.name + ": where to learn it", fmt.Sprintf("%d places; %s", len(res.Ways), firstLine(res.Text)), placesNoReply + ": no named vendor, creature or trainer", recipeSrc,
			resultIf(len(res.Ways) == 0 && strings.HasPrefix(res.Text, words), "a place was named, or an item recipe was sent to a trainer")})
	}
	_, err := learn.Find(character.Character{Faction: "Horde", Class: "MAGE"}, ds, nil, "Elixir Of The Unwritten Moon")
	out = append(out, benchRow{"a recipe the dataset does not hold", fmt.Sprint(err), placesNoReply, recipeSrc, resultIf(err != nil, "a recipe was matched")})
	finishOffline(t, "advice-recipe-no-answer", "Offline recipe source, no answer in the dataset", out)
}

func firstLine(s string) string {
	l, _, _ := strings.Cut(s, "\n")
	return l
}

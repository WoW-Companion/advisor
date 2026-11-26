package paths_test

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/dungeons"
	"github.com/WoW-Companion/advisor/internal/farm"
	"github.com/WoW-Companion/advisor/internal/gear"
	"github.com/WoW-Companion/advisor/internal/learn"
	"github.com/WoW-Companion/advisor/internal/paths"
	"github.com/WoW-Companion/advisor/internal/quests"
	"github.com/WoW-Companion/advisor/internal/store"
)

// The offline attunement, dungeon-key, reputation-gated gear, consumable-buff and level-requirement benchmark: what the advisor
// says of a dungeon key, of the entry to a dungeon or raid, of the Thorium Brotherhood plans and patterns (the gear that is
// held behind a reputation standing), of a flask, elixir or food, and of the level an item or a quest asks for, beside the
// dataset's rows and the guides, with no network. It reads testdata/gating_snapshot (the key and attunement items, the plans, patterns,
// reagents and gear of the reputation recipes, worn items with required levels), testdata/places_snapshot (instances, bosses,
// quests_observed) and testdata/consumables_snapshot (every consumable with its recipe). The dataset holds no key-to-dungeon
// link, no attunement, no reputation faction or standing, no minimum level for a quest and no flask duration or stat, so the cases
// it cannot answer check that the advisor says so and states none. A difference the dataset's own rows cause is marked "(data)".
// make offline-benchmark runs it; WOWC_BENCH_REPORT receives the tables.

const (
	gateDir    = "../../testdata/gating_snapshot"
	gateKeySrc = "dataset items (class 13, key; no row links a key to a dungeon), loot_sources (no source); Warcraft Wiki key item pages"
	gateEntSrc = "dataset instances (min_level, max_level, level_source curated); Warcraft Wiki dungeon pages (minimum level); the dataset has no key or attunement row"
	gateRepSrc = "dataset recipes (learn_skill, learned_from), recipe_items (the plan or pattern); Warcraft Wiki, Thorium Brotherhood (Lokhtos Darkbargainer's standings); the dataset has no faction or standing column"
	gateBuffSr = "dataset items (class 0), recipes (learn_skill), vendor_sources; Warcraft Wiki (Alchemy and Cooking skills; flasks and elixirs are sold on the Auction House)"
	gateLvlSrc = "dataset items (required_level, the client table); Warcraft Wiki Classic item lines (Sulfuron Hammer 60, Fiery Plate Gauntlets 53)"
	gateQstSrc = "dataset quests_observed (level, Everloot); Warcraft Wiki quest pages (level and required level); the dataset quests table holds no level or min_level"
	gateNoSrc  = "the dataset has no such row, column or table"
)

var (
	gateOnce  sync.Once
	gateCache store.DataStore
)

// gateStore serves the gating snapshot's tables first and the places snapshot's for the rest.
func gateStore() store.DataStore {
	gateOnce.Do(func() {
		gateCache = store.New(qbMix{first: store.DirSource(gateDir), then: store.DirSource(qbPlacesDir)})
	})
	return gateCache
}

var standing = regexp.MustCompile(`(?i)\b(reputation|standing|friendly|honou?red|revered|exalted|thorium brotherhood|lokhtos|faction)\b`)

// ---- dungeon keys ----

var gateKeys = []struct{ name, opens string }{
	{"Shadowforge Key", "Blackrock Depths"}, {"Crescent Key", "Dire Maul"}, {"Skeleton Key", "Scholomance"},
	{"Key to the City", "Stratholme"}, {"Viewing Room Key", "Scholomance"}, {"Gordok Courtyard Key", "Dire Maul"},
	{"Unforged Seal of Ascension", "Blackrock Spire"},
}

// TestOfflineBenchmarkDungeonKeys: a key asked for by name is found, and the advisor says the data lists no source for it, names
// no dungeon it opens and no attunement; the dataset links no key to a dungeon.
func TestOfflineBenchmarkDungeonKeys(t *testing.T) {
	ds := gateStore()
	var out []benchRow
	for _, k := range gateKeys {
		row := rowByName(t, ds, "items", k.name)
		id := num(row["id"])
		loot, _ := ds.Filter("loot_sources", store.Range{Column: "item_id", Min: id, Max: id})
		got, name, cands := farm.Resolve(ds, k.name)
		res := paths.For(horde(60), ds, id, paths.Options{})
		txt := res.Text()
		ok := got == id && name == k.name && len(cands) == 0 && !res.Dependable() && len(loot) == 0 && strings.Contains(res.Reason, "lists no source") &&
			!strings.Contains(txt, k.opens) && !regexp.MustCompile(`(?i)attun|unlock|opens|\bdoor\b`).MatchString(txt)
		out = append(out, benchRow{k.name + ": how to get it", firstLine(txt), fmt.Sprintf("item %d, class %d (key), %d loot rows", id, num(row["class_id"]), len(loot)), gateKeySrc,
			resultIf(ok, "another item, a source, a dungeon or an attunement was stated")})
		out = append(out, benchRow{k.name + ": the dungeon it opens", "none named", k.opens + " (Warcraft Wiki)", gateKeySrc,
			"differ (data): the dataset links no key to a dungeon, so the advisor names none"})
	}
	finish(t, "advice-gating-dungeon-keys", "Offline dungeon keys: a key's source and the dungeon it opens", out)
}

// ---- dungeon and raid entry ----

var gateDungeons = []struct {
	name  string
	level int64
	needs string
}{
	{"Blackrock Depths", 52, "no key to enter; the Shadowforge Key opens the vault"},
	{"Scholomance", 58, "the Skeleton Key (or a rogue's lockpicking)"},
	{"Stratholme", 58, "no key to enter; the Key to the City opens the service gate"},
	{"Dire Maul", 55, "no key to enter; the Crescent Key opens a shortcut"},
	{"Blackrock Spire", 55, "the Seal of Ascension for the upper spire"},
	{"Zul'Farrak", 44, "none"},
	{"Sunken Temple", 50, "none"},
	{"Maraudon", 40, "none"},
}

// TestOfflineBenchmarkDungeonEntry: a dungeon asked about by name gives the entry level the guides give and says its key or entry
// requirement is not known, naming no key.
func TestOfflineBenchmarkDungeonEntry(t *testing.T) {
	ds := instStore()
	c := character.Character{Faction: "Horde", Class: "WARRIOR"}
	var out []benchRow
	for _, d := range gateDungeons {
		r := rowByName(t, ds, "instances", d.name)
		p, togo, err := dungeons.Named(c, ds, nil, d.name)
		if err != nil || p == nil {
			out = append(out, benchRow{d.name + ": the dungeon", "no answer", "an instances row", gateEntSrc, "differ: the dungeon was not found"})
			continue
		}
		txt := dungeons.RenderNamed(c, *p, togo)
		want := fmt.Sprintf("about levels %d–%d", num(r["min_level"]), num(r["max_level"]))
		out = append(out, benchRow{d.name + ": the level to enter", firstLine(txt), fmt.Sprintf("entry level %d; instances %d-%d", d.level, num(r["min_level"]), num(r["max_level"])), gateEntSrc,
			resultIf(p.MinLevel != nil && int64(*p.MinLevel) == d.level && num(r["min_level"]) == d.level && strings.Contains(txt, want), "another entry level")})
		line := ""
		for _, l := range strings.Split(txt, "\n") {
			if strings.Contains(l, "Key or entry requirement") {
				line = strings.TrimSpace(l)
			}
		}
		out = append(out, benchRow{d.name + ": the key or entry requirement", orNone(line), d.needs, gateEntSrc,
			resultIf(line == "Key or entry requirement: not known yet" && !strings.Contains(strings.ToLower(txt), "skeleton key") && !strings.Contains(strings.ToLower(txt), "seal of ascension"), "no statement that the requirement is not known, or a key was named")})
	}
	finish(t, "advice-gating-dungeon-entry", "Offline dungeon entry: the level, and no key guessed", out)
}

func orNone(s string) string {
	if s == "" {
		return "nothing said"
	}
	return s
}

var gateRaids = []struct {
	name, gate string
}{
	{"Ahn'Qiraj Temple", "the gate must be opened by a server-wide event"}, {"Ruins of Ahn'Qiraj", "none"}, {"Zul'Gurub", "none"},
	{"Molten Core", "none to enter (the Molten Core attunement is a quest)"}, {"Onyxia's Lair", "the Drakefire Amulet chain"}, {"Blackwing Lair", "the Blackhand's Command chain"},
}

// TestOfflineBenchmarkRaidEntry: a raid asked about by name is worded as a single level (a raid that opens at 60 has no range), says its
// lockout and attunement are not known, and states no attunement or reset rule.
func TestOfflineBenchmarkRaidEntry(t *testing.T) {
	ds := instStore()
	c := character.Character{Faction: "Horde", Class: "WARRIOR"}
	var out []benchRow
	for _, r := range gateRaids {
		p, togo, _ := dungeons.Named(c, ds, nil, r.name)
		if p == nil {
			out = append(out, benchRow{r.name + ": the raid", "no answer", "a raid row in instances", gateEntSrc, "differ: the raid was not found"})
			continue
		}
		txt := dungeons.RenderNamed(c, *p, togo)
		head := firstLine(strings.TrimSpace(txt))
		ok := p.Raid && strings.Contains(head, "(level 60;") && !strings.Contains(head, "60–60") && strings.Contains(txt, "Lockout and attunement: not known yet") &&
			!strings.Contains(txt, "Key or entry requirement") && instReset.FindString(stated(txt)) == ""
		out = append(out, benchRow{r.name + ": the level and the attunement", head, "level 60 raid; attunement: " + r.gate, gateEntSrc, resultIf(ok, "a level range for a level 60 raid, or an attunement was stated")})
	}
	finish(t, "advice-gating-raid-entry", "Offline raid entry: one level, and no attunement guessed", out)
}

// ---- reputation-gated gear ----

// gatePlans are Thorium Brotherhood plans and patterns, each with the standing Lokhtos Darkbargainer sells it at.
var gatePlans = []struct{ recipe, prof, standing string }{
	{"Corehound Boots", "Leatherworking", "Friendly"}, {"Flarecore Gloves", "Tailoring", "Friendly"}, {"Dark Iron Destroyer", "Blacksmithing", "Honored"},
	{"Dark Iron Helm", "Blacksmithing", "Honored"}, {"Lava Belt", "Leatherworking", "Honored"}, {"Chromatic Gauntlets", "Leatherworking", "Revered"},
	{"Dark Iron Gauntlets", "Blacksmithing", "Revered"}, {"Blackfury", "Blacksmithing", "Revered"}, {"Dark Iron Boots", "Blacksmithing", "Exalted"},
	{"Nightfall", "Blacksmithing", "Exalted"},
}

// recipeNamed is the recipe row of a profession by name.
func recipeNamed(t *testing.T, ds store.DataStore, name, prof string) store.Row {
	t.Helper()
	rows, _ := ds.Filter("recipes", store.Range{Column: "name", Min: name, Max: name})
	for _, r := range rows {
		if text(r["skill_line_name"]) == prof {
			return r
		}
	}
	t.Fatalf("recipe %s (%s): not in the snapshot", name, prof)
	return nil
}

// TestOfflineBenchmarkReputationPlans: where to learn a recipe whose plan a reputation vendor sells: the advisor gives the profession and
// skill the recipe needs and says the data names no place that sells the plan, stating no faction or standing.
func TestOfflineBenchmarkReputationPlans(t *testing.T) {
	ds := gateStore()
	var out []benchRow
	for _, p := range gatePlans {
		rec := recipeNamed(t, ds, p.recipe, p.prof)
		res, err := learn.Find(horde(60), ds, nil, p.recipe)
		need := num(rec["learn_skill"])
		ok := err == nil && res.Profession == p.prof && int64(res.Gate.Skill) == need && strings.Contains(res.Text, "names no place that sells or drops it") && !standing.MatchString(res.Text)
		out = append(out, benchRow{p.recipe + ": where to learn it", strings.ReplaceAll(res.Text, "\n", " "), fmt.Sprintf("%s %d, learned from %s", p.prof, need, text(rec["learned_from"])), gateRepSrc,
			resultIf(ok, "another profession or skill, a place, or a standing was stated")})
		out = append(out, benchRow{p.recipe + ": the reputation to buy the plan", "none stated", "Thorium Brotherhood, " + p.standing, gateRepSrc, "differ (data): the dataset has no faction or standing for a vendor or a plan"})
	}
	finish(t, "advice-gating-reputation-plans", "Offline reputation gear: plans and patterns held behind a standing", out)
}

// TestOfflineBenchmarkReputationGearPaths: the gear those plans make is a Bind on Equip item, so the advisor says to buy it on the Auction House
// and, for a crafter, how to make it, and names no reputation.
func TestOfflineBenchmarkReputationGearPaths(t *testing.T) {
	ds := gateStore()
	var out []benchRow
	for _, p := range gatePlans[2:9] {
		it := rowByName(t, ds, "items", p.recipe)
		id := num(it["id"])
		res := paths.For(horde(60), ds, id, paths.Options{})
		best, found := res.Best()
		ok := found && best.Kind == paths.Auction && num(it["bonding"]) == 2 && !standing.MatchString(res.Text())
		out = append(out, benchRow{p.recipe + ": how to get the gear", firstLine(strings.TrimPrefix(res.Text(), "How to get "+p.recipe+", best way first:\n")), fmt.Sprintf("bonding %d (Bind on Equip); plan sold at %s with Thorium Brotherhood", num(it["bonding"]), p.standing), gateRepSrc,
			resultIf(ok, "no Auction House way, or a reputation was named")})
	}
	finish(t, "advice-gating-reputation-gear", "Offline reputation gear: how to get what the plans make", out)
}

// ---- consumable buffs ----

var gateBuffs = []struct {
	name, prof string
	skill      int64
}{
	{"Flask of the Titans", "Alchemy", 300}, {"Flask of Supreme Power", "Alchemy", 300}, {"Flask of Distilled Wisdom", "Alchemy", 300},
	{"Flask of Chromatic Resistance", "Alchemy", 300}, {"Elixir of the Mongoose", "Alchemy", 280}, {"Greater Arcane Elixir", "Alchemy", 285},
	{"Smoked Desert Dumplings", "Cooking", 285}, {"Nightfin Soup", "Cooking", 250}, {"Spiced Chili Crab", "Cooking", 225},
}

// TestOfflineBenchmarkBuffConsumables: a flask, elixir or food a profession makes and no vendor sells: the advisor gives the skill that makes it and the
// Auction House as the way to get it, never "not worth chasing".
func TestOfflineBenchmarkBuffConsumables(t *testing.T) {
	ds := consStore()
	var out []benchRow
	for _, b := range gateBuffs {
		it, rec := itemOf(t, ds, b.name)
		id := num(it["id"])
		res := paths.For(horde(60), ds, id, paths.Options{})
		best, found := res.Best()
		line := craftLine(res)
		got := "no way"
		if found {
			got = best.Kind
		}
		m := madeBy.FindStringSubmatch(line)
		skill := "no skill stated"
		if m != nil {
			skill = "skill " + m[1]
		}
		vend, _ := ds.Filter("vendor_sources", store.Range{Column: "item_id", Min: id, Max: id})
		ok := found && best.Kind == paths.Auction && !strings.Contains(res.Text(), "not worth chasing") && skill == fmt.Sprintf("skill %d", b.skill) &&
			text(rec["skill_line_name"]) == b.prof && num(rec["learn_skill"]) == b.skill && len(vend) == 0
		out = append(out, benchRow{b.name + ": how to get it, and the skill", got + "; " + skill, fmt.Sprintf("%s %d recipe; bonding %d, quality %d, %d vendor rows: sold on the Auction House", b.prof, b.skill, num(it["bonding"]), num(it["quality"]), len(vend)), gateBuffSr,
			resultIf(ok, "no Auction House way, \"not worth chasing\", or another skill")})
	}
	finish(t, "advice-gating-buff-consumables", "Offline buff consumables: the way to get a flask, elixir or food", out)
}

var gateMissing = []string{"Mageblood Potion", "Major Troll's Blood Potion", "Elixir of Superior Defense", "Elixir of Giants", "Gurubashi Gumbo", "Hardened Mushroom"}

// TestOfflineBenchmarkBuffConsumableNoAnswer: consumables the game has and the dataset lacks are not found or invented, and nothing is stated of a buff's
// duration or stats.
func TestOfflineBenchmarkBuffConsumableNoAnswer(t *testing.T) {
	ds := consStore()
	var out []benchRow
	for _, n := range gateMissing {
		id, name, cands := farm.Resolve(ds, n)
		_, err := learn.Find(horde(60), ds, nil, n)
		// a part of a name that only a recipe item carries finds that item, named as the recipe: never the elixir itself
		ok := id == 0 && name == "" && len(cands) == 0 && err != nil || strings.HasPrefix(name, "Recipe: ") && len(cands) == 0
		res := "differ (data): the dataset has no row for the consumable the game has; the advisor names none, or only its recipe item"
		if !ok {
			res = "differ: an item was matched or invented"
		}
		out = append(out, benchRow{n + ": the item", fmt.Sprintf("item %d, %d candidates, learn: %v", id, len(cands), err), "a consumable of the game (Warcraft Wiki); no items row", gateNoSrc, res})
	}
	for _, n := range []string{"Flask of the Titans", "Greater Arcane Elixir", "Smoked Desert Dumplings"} {
		it := rowByName(t, ds, "items", n)
		res := paths.For(horde(60), ds, num(it["id"]), paths.Options{})
		out = append(out, benchRow{n + ": how long it lasts and what it adds", res.Text(), "no duration or stat in the dataset (items.stats is empty)", gateNoSrc,
			resultIf(!numbered.MatchString(res.Text()) && num(it["required_level"]) > 0, "a duration or stat amount was stated")})
	}
	finish(t, "advice-gating-buff-no-answer", "Offline buff consumables: no answer in the dataset", out)
}

// ---- level requirements ----

var gateItemLevels = []struct {
	name  string
	level int
}{
	{"Drakefire Amulet", 50}, {"Fiery Plate Gauntlets", 53}, {"Frostwolf Legionnaire's Cloak", 55}, {"Arcanite Reaper", 58}, {"Sulfuron Hammer", 60}, {"Dark Iron Boots", 60},
}

// offered is whether gear.Find offers the item to a warrior of the level.
func offered(t *testing.T, ds store.DataStore, level int, name string) bool {
	t.Helper()
	c := character.Character{Faction: "Horde", Class: "WARRIOR", Level: &level}
	r, err := gear.Find(c, ds, "", gear.Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, cs := range r.Slots {
		for _, cd := range cs {
			if cd.Item.Name == name {
				return true
			}
		}
	}
	return false
}

// TestOfflineBenchmarkItemRequiredLevel: a gear search offers an item from two levels under the level it requires (the equip window) and not before.
func TestOfflineBenchmarkItemRequiredLevel(t *testing.T) {
	ds := gateStore()
	var out []benchRow
	for _, tc := range gateItemLevels {
		rl := num(rowByName(t, ds, "items", tc.name)["required_level"])
		lo := tc.level - gear.EquipWindow
		in, before := offered(t, ds, lo, tc.name), offered(t, ds, lo-1, tc.name)
		out = append(out, benchRow{tc.name + ": the level it is offered from", fmt.Sprintf("offered at level %d: %v; at level %d: %v", lo, in, lo-1, before), fmt.Sprintf("required_level %d: offered from %d (window %d)", rl, int(rl)-gear.EquipWindow, gear.EquipWindow), gateLvlSrc,
			resultIf(rl == int64(tc.level) && in && !before, "offered too early or not at the required level")})
	}
	finish(t, "advice-gating-item-levels", "Offline level requirements: the level a gear search offers an item from", out)
}

var gateQuests = []struct {
	title        string
	level, min   int
	observedDiff bool
}{
	{"The Book of Ur", 26, 16, false}, {"Gahz'rilla", 50, 40, false}, {"Tran'rek", 45, 40, false}, {"Into The Scarlet Monastery", 42, 30, false},
	{"Hearts of Zeal", 33, 30, false}, {"Compendium of the Fallen", 33, 29, true},
}

// TestOfflineBenchmarkQuestLevels: the level the advisor gives a quest beside quests_observed and the Warcraft Wiki; the level a player needs to take it is
// not in the dataset (the quests table has no min_level value) and the advisor states none.
func TestOfflineBenchmarkQuestLevels(t *testing.T) {
	ds := qbStore()
	var out []benchRow
	for _, q := range gateQuests {
		id, name, cands := quests.Resolve(ds, q.title)
		f := quests.FactsOf(ds, nil, id)
		rows, _ := ds.Filter("quests_observed", store.Range{Column: "quest_id", Min: id, Max: id})
		if f == nil || len(rows) == 0 {
			t.Fatalf("%s: not found", q.title)
		}
		same := f.Level == int(num(rows[0]["level"])) && name == q.title && len(cands) == 0
		res := resultIf(same && f.Level == q.level, "another level than the quest's")
		if same && f.Level != q.level {
			res = fmt.Sprintf("differ (data): quests_observed holds level %d, the guide level %d", f.Level, q.level)
		}
		out = append(out, benchRow{q.title + ": the quest level", fmt.Sprintf("level %d", f.Level), fmt.Sprintf("quests_observed level %d; guide level %d", num(rows[0]["level"]), q.level), gateQstSrc, res})
		txt := quests.Render(quests.Of(ds, id), name)
		out = append(out, benchRow{q.title + ": the level needed to take it", "none stated", fmt.Sprintf("requires level %d (Warcraft Wiki)", q.min), gateQstSrc,
			map[bool]string{true: "differ (data): the quests table holds no min_level, so the advisor names none", false: "differ: a minimum level was stated"}[f.MinLevel == 0 && !strings.Contains(txt, "needs level")]})
	}
	finish(t, "advice-gating-quest-levels", "Offline level requirements: the level of a quest and the level to take it", out)
}

// TestOfflineBenchmarkGatingNoAnswer: what no table of the dataset holds: reputation, attunement and key tables, and the advisor's answer to the standing a plan needs.
func TestOfflineBenchmarkGatingNoAnswer(t *testing.T) {
	ds := gateStore()
	var out []benchRow
	for _, tbl := range []string{"reputations", "factions", "attunements", "dungeon_keys", "lockouts"} {
		_, err := ds.Filter(tbl, store.Range{Column: "id", Min: int64(0)})
		out = append(out, benchRow{"a table of " + strings.ReplaceAll(tbl, "_", " "), fmt.Sprint(err), "no such table in the dataset", gateNoSrc, resultIf(err != nil, "a table was invented")})
	}
	v, _ := ds.Filter("vendor_sources", store.Range{Column: "price_copper", Min: int64(0)})
	cols := map[string]bool{}
	for _, r := range v {
		for k := range r {
			cols[k] = true
		}
	}
	out = append(out, benchRow{"vendor_sources: a faction or standing column", fmt.Sprintf("%d columns, none for a faction or standing", len(cols)), "no faction, standing or discount column", gateNoSrc,
		resultIf(!cols["faction"] && !cols["standing"] && !cols["reputation"], "a column was invented")})
	finish(t, "advice-gating-no-answer", "Offline attunement, key and reputation: no answer in the dataset", out)
}

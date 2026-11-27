package paths_test

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/paths"
	"github.com/WoW-Companion/advisor/internal/professions"
	"github.com/WoW-Companion/advisor/internal/store"
)

// The offline trainer-location, profession-tool, reagent-source and skill-up-route benchmark: which trainer the route
// advice sends a player to for a rank, which tool it says a profession needs and where it says to buy it, how it says to
// get a crafting material, and the next steps of a profession route, beside the dataset's rows, with no network.
// testdata/tradeskills_snapshot (rebuild it with testdata/tradeskills_snapshot/build.sh) holds the whole trainers,
// trainer_spells, profession_tiers, profession_routes (the wow-professions.com guide steps), profession_notes, recipes,
// recipe_reagents and gathering tables, and the items, loot and vendor rows those name. The dataset's trainers table
// holds no trainer name or coordinates, and the tiers name no trainer for Mining, Skinning, Herbalism or the first rank
// of Fishing and Cooking, so those cases check that the advisor says so and names nobody. make offline-benchmark runs
// it; WOWC_BENCH_REPORT receives the tables.

const (
	tsDir      = "../../testdata/tradeskills_snapshot"
	tsTierSrc  = "dataset profession_tiers (tier, trainer_name, place, faction, how)"
	tsRouteSrc = "dataset profession_routes (the wow-professions.com guide: skill_from, skill_to, recipe_name, crafts), recipe_reagents"
	tsToolSrc  = "dataset items (class 2, subclass 14; buy_price), vendor_sources"
	tsReagSrc  = "dataset loot_sources (crafted, sold by, node), recipes, items (bonding)"
)

var (
	tsOnce  sync.Once
	tsCache store.DataStore
)

// tsStore reads the snapshot once for the package: the tables are decoded on first use and the tests share them.
func tsStore() store.DataStore {
	tsOnce.Do(func() { tsCache = store.NewDir(tsDir) })
	return tsCache
}

type tsSkill struct {
	prof      string
	rank, max int
}

func tsChar(faction string, level int, skills ...tsSkill) character.Character {
	c := character.Character{Name: "Benchtest", Faction: faction, Class: "MAGE", Level: &level}
	for _, s := range skills {
		rank, top := s.rank, s.max
		c.Professions = append(c.Professions, character.Skill{Name: s.prof, Rank: &rank, Max: &top})
	}
	return c
}

func tsRows(t *testing.T, table, col string, key any) []store.Row {
	t.Helper()
	rows, err := tsStore().Filter(table, store.Range{Column: col, Min: key, Max: key})
	if err != nil {
		t.Fatalf("%s: %v", table, err)
	}
	return rows
}

// tsTierTrainers are the trainers the dataset's profession_tiers names for the tier on the faction's side, written as
// the advisor writes them ("Name in Place"), read straight from the table.
func tsTierTrainers(t *testing.T, prof, tier, faction string) (named []string, other []string) {
	t.Helper()
	for _, r := range tsRows(t, "profession_tiers", "profession", prof) {
		name, place := text(r["trainer_name"]), text(r["place"])
		if name == "" || !strings.EqualFold(text(r["tier"]), tier) || text(r["how"]) != "trainer" {
			continue
		}
		if place != "" {
			name += " in " + place
		}
		if f := text(r["faction"]); f != "" && !strings.EqualFold(f, "both") && !strings.EqualFold(f, faction) {
			other = append(other, name)
			continue
		}
		named = append(named, name)
	}
	return named, other
}

// tsGate is the line of a route answer that says which rank to train and where.
func tsGate(answer string) string {
	for _, l := range strings.Split(answer, "\n") {
		if strings.HasPrefix(l, "Before ") {
			return l
		}
	}
	return ""
}

// ---- trainer location ----

var tsTrainerCases = []struct {
	faction, prof, tier string
	rank, max, level    int
}{
	{"Horde", "Alchemy", "Expert", 140, 150, 25},
	{"Alliance", "Alchemy", "Expert", 140, 150, 25},
	{"Horde", "Blacksmithing", "Expert", 140, 150, 25},
	{"Alliance", "Blacksmithing", "Expert", 140, 150, 25},
	{"Alliance", "Engineering", "Expert", 140, 150, 25},
	{"Horde", "Leatherworking", "Journeyman", 70, 75, 12},
	{"Alliance", "Tailoring", "Artisan", 215, 225, 40},
	{"Horde", "Tailoring", "Artisan", 215, 225, 40},
}

// TestOfflineBenchmarkTrainerLocation: the trainer and place the skill-up route names for the rank a character must
// train next, for both sides, beside the trainers profession_tiers names for the side. The other side's trainers are
// never named, and neither is a trainer id.
func TestOfflineBenchmarkTrainerLocation(t *testing.T) {
	ds := tsStore()
	var out []benchRow
	for _, tc := range tsTrainerCases {
		c := tsChar(tc.faction, tc.level, tsSkill{tc.prof, tc.rank, tc.max})
		gate := tsGate(professions.RouteFacts(c, ds, nil, tc.prof))
		named, other := tsTierTrainers(t, tc.prof, tc.tier, tc.faction)
		ok := len(named) > 0 && gate != ""
		for _, n := range named {
			ok = ok && strings.Contains(gate, n)
		}
		for _, n := range other {
			ok = ok && !strings.Contains(gate, n)
		}
		out = append(out, benchRow{fmt.Sprintf("%s %s %s: where to train it", tc.faction, tc.prof, tc.tier), gate,
			fmt.Sprintf("%d trainer(s) named for the side: %s", len(named), strings.Join(named, "; ")), tsTierSrc,
			resultIf(ok, "a trainer the tier names is missing, or the other side's trainer is named")})
	}
	finish(t, "advice-trainer-location", "Offline profession trainer location by rank", out)
}

// ---- profession tools ----

var tsToolCases = []struct {
	label, prof, tool string
	held              string // the item name carried, "" for none
	worn              bool
}{
	{"no pick", "Mining", "Mining Pick", "", false},
	{"no knife", "Skinning", "Skinning Knife", "", false},
	{"no hammer", "Blacksmithing", "Blacksmith Hammer", "", false},
	{"no pole", "Fishing", "Fishing Pole", "", false},
	{"no rod", "Enchanting", "Runed Copper Rod", "", false},
	{"pick in bags", "Mining", "Mining Pick", "Mining Pick", false},
	{"knife worn", "Skinning", "Skinning Knife", "Skinning Knife", true},
	{"better pole worn", "Fishing", "Fishing Pole", "Big Iron Fishing Pole", true},
}

// TestOfflineBenchmarkProfessionTool: the tool a profession needs, named as the dataset's tool item (class 2, subclass
// 14), what it costs at a vendor (items.buy_price, when vendor_sources has rows for it), and silence when the
// character already carries or wears one.
func TestOfflineBenchmarkProfessionTool(t *testing.T) {
	ds := tsStore()
	var out []benchRow
	for _, tc := range tsToolCases {
		c := tsChar("Alliance", 20, tsSkill{tc.prof, 30, 75})
		if tc.held != "" && tc.worn {
			c.Equipment = map[string]character.EquippedItem{"MainHandSlot": {Slot: "MainHandSlot", Name: tc.held}}
		} else if tc.held != "" {
			c.Bags = []character.ItemStack{{Name: tc.held, Count: 1}}
		}
		ans := professions.RouteFacts(c, ds, nil, tc.prof)
		line := ""
		for _, l := range strings.Split(ans, "\n") {
			if strings.HasPrefix(l, "They have no ") {
				line = l
			}
		}
		item := rowByName(t, ds, "items", tc.tool)
		vend := len(tsRows(t, "vendor_sources", "item_id", num(item["id"])))
		price := text(item["name"])
		var ok bool
		var ref string
		switch {
		case tc.held != "":
			ok = line == ""
			ref = fmt.Sprintf("the character carries %q: no tool line", tc.held)
		case tc.prof == "Enchanting":
			// the rod is made by Enchanting from a Blacksmith-made rod: the route's own note says where to get one
			ok = strings.Contains(line, tc.tool) && strings.Contains(line, "Blacksmith-made")
			ref = fmt.Sprintf("%s (class %d, subclass %d), %d vendor rows; guide note: rod is Blacksmith-made", price, num(item["class_id"]), num(item["subclass_id"]), vend)
		default:
			ok = strings.Contains(line, tc.tool) && vend > 0 && strings.Contains(line, "from a vendor") && !strings.Contains(line, "rod, pick or knife")
			ref = fmt.Sprintf("%s (class %d, subclass %d), buy_price %d, %d vendor rows", price, num(item["class_id"]), num(item["subclass_id"]), num(item["buy_price"]), vend)
		}
		out = append(out, benchRow{fmt.Sprintf("%s: %s tool", tc.label, tc.prof), strings.TrimSpace(line), ref, tsToolSrc,
			resultIf(ok, "the tool item is not named, the vendor way is not given, or a carried tool is not recognised")})
	}
	finish(t, "advice-profession-tool", "Offline profession tools", out)
}

// ---- reagent sources ----

var tsReagentCases = []struct {
	label, reagent string
	craft          int64
	skills         []tsSkill
	want           string // make, vendor, gather, auction
}{
	{"Copper Bar, Mining 60", "Copper Bar", 2851, []tsSkill{{"Blacksmithing", 40, 75}, {"Mining", 60, 75}}, "make"},
	{"Copper Bar, no Mining", "Copper Bar", 2851, []tsSkill{{"Blacksmithing", 40, 75}}, "auction"},
	{"Silver Bar, Mining 80", "Silver Bar", 4404, []tsSkill{{"Engineering", 100, 150}, {"Mining", 80, 150}}, "make"},
	{"Silver Bar, Mining 50 (recipe needs 75)", "Silver Bar", 4404, []tsSkill{{"Engineering", 100, 150}, {"Mining", 50, 75}}, "auction"},
	{"Rough Stone, Mining known", "Rough Stone", 4357, []tsSkill{{"Engineering", 30, 75}, {"Mining", 20, 75}}, "gather"},
	{"Copper Rod (sold by vendors)", "Copper Rod", 6218, []tsSkill{{"Enchanting", 1, 75}}, "vendor"},
	{"Mote of Magic (sold by vendors)", "Mote of Magic", 6218, []tsSkill{{"Enchanting", 1, 75}}, "vendor"},
	{"Thick Leather, Leatherworking 200", "Thick Leather", 18238, []tsSkill{{"Leatherworking", 200, 225}}, "make"},
}

// TestOfflineBenchmarkReagentSource: how the advisor says to get a missing crafting material beside the dataset's rows:
// a recipe of a profession the character has at the skill it is learned at means making it, a sold-by row a vendor, a
// node within the character's gathering skill gathering it, and the Auction House otherwise for a tradeable item.
func TestOfflineBenchmarkReagentSource(t *testing.T) {
	ds := tsStore()
	var out []benchRow
	for _, tc := range tsReagentCases {
		c := tsChar("Horde", 30, tc.skills...)
		var how string
		var matched paths.Path
		for _, p := range paths.For(c, ds, tc.craft, paths.Options{}).Paths {
			for _, g := range p.Reagents {
				if g.Name == tc.reagent {
					how, matched = g.How, p
				}
			}
		}
		if how == "" {
			t.Fatalf("%s: no reagent line for %s in %+v", tc.label, tc.reagent, matched)
		}
		it := rowByName(t, ds, "items", tc.reagent)
		id := num(it["id"])
		var crafted, sold, node int
		var made []string
		for _, l := range tsRows(t, "loot_sources", "item_id", id) {
			switch k := text(l["source_kind"]); k {
			case "crafted":
				crafted++
				made = append(made, fmt.Sprintf("%s %d", text(l["profession"]), num(l["learn_skill"])))
			case "sold by":
				sold++
			case "object":
				node++
			}
		}
		got := map[bool]string{true: "make", false: ""}[strings.HasPrefix(how, "make it")]
		switch {
		case got == "make":
		case strings.HasPrefix(how, "gather"):
			got = "gather"
		case strings.HasPrefix(how, "buy it from a vendor"):
			got = "vendor"
		case strings.HasPrefix(how, "buy it on the Auction House"):
			got = "auction"
		}
		ref := fmt.Sprintf("%s: %d crafted row(s) %v, %d sold-by rows, %d node rows, bonding %d", tc.want, crafted, made, sold, node, num(it["bonding"]))
		out = append(out, benchRow{tc.label + ": how to get it", how, ref, tsReagSrc, resultIf(got == tc.want, "the way to get it: "+got+" for "+tc.want)})
	}
	finish(t, "advice-reagent-source", "Offline crafting material sources", out)
}

// ---- skill-up routes ----

var tsRouteCases = []struct {
	faction, prof string
	rank, max     int
}{
	{"Horde", "Enchanting", 100, 150},
	{"Alliance", "Enchanting", 30, 75},
	{"Horde", "Alchemy", 120, 150},
	{"Alliance", "Blacksmithing", 40, 75},
	{"Alliance", "Leatherworking", 70, 75},
	{"Horde", "Tailoring", 190, 225},
	{"Horde", "Engineering", 50, 75},
	{"Alliance", "Alchemy", 80, 150},
	{"Horde", "Cooking", 60, 75},
	{"Alliance", "First Aid", 40, 75},
}

// TestOfflineBenchmarkSkillUpRoute: the step of the guide's route that covers the character's skill: its recipe, the
// crafts left once the skill already gained within the step is taken off, and each material's count (recipe_reagents
// per craft times the crafts left). No alternative the character has already passed is offered, and no count is below
// zero.
func TestOfflineBenchmarkSkillUpRoute(t *testing.T) {
	ds := tsStore()
	var out []benchRow
	for _, tc := range tsRouteCases {
		c := tsChar(tc.faction, 30, tsSkill{tc.prof, tc.rank, tc.max})
		ans := professions.RouteFacts(c, ds, nil, tc.prof)
		var step store.Row
		for _, r := range tsRows(t, "profession_routes", "profession", tc.prof) {
			if r["alternative_of"] == nil && num(r["skill_from"]) <= int64(tc.rank) && int64(tc.rank) < num(r["skill_to"]) {
				step = r
			}
		}
		if step == nil {
			t.Fatalf("%s %d: no route step covers the skill", tc.prof, tc.rank)
		}
		from, to, crafts := num(step["skill_from"]), num(step["skill_to"]), num(step["crafts"])
		left := crafts
		if int64(tc.rank) > from {
			left = (crafts*(to-int64(tc.rank)) + (to - from) - 1) / (to - from)
		}
		name := text(step["recipe_name"])
		want := fmt.Sprintf("Now: skill %d to %d: %d x %s", tc.rank, to, left, name)
		ok := strings.Contains(ans, want)
		ref := want
		for _, m := range tsRows(t, "recipe_reagents", "spell_id", num(step["recipe_spell_id"])) {
			it := itemsRowOf2(t, num(m["reagent_item_id"]))
			part := fmt.Sprintf("%d %s", num(m["count"])*left, text(it["name"]))
			ref += "; " + part
			ok = ok && strings.Contains(ans, part)
		}
		for _, l := range strings.Split(ans, "\n") {
			if strings.HasPrefix(strings.TrimSpace(l), "Alternative") && (tsNegative.MatchString(l) || strings.Contains(l, "step: 0 ")) {
				ok = false // an alternative with no crafts left, or a negative count
			}
		}
		now := ""
		for _, l := range strings.Split(ans, "\n") {
			if strings.HasPrefix(l, "Now:") {
				now = l
			}
		}
		out = append(out, benchRow{fmt.Sprintf("%s %s at %d: the step now", tc.faction, tc.prof, tc.rank), now, ref + fmt.Sprintf(" (guide step %d-%d, %d crafts)", from, to, crafts), tsRouteSrc,
			resultIf(ok, "the recipe, the crafts left or a material count is not the guide step's, or an alternative has a count below one")})
	}
	finish(t, "advice-skillup-route", "Offline skill-up route steps against the guide", out)
}

// tsNegative finds a count below zero ("-20 x", "-20 Strange Dust") in an answer line.
var tsNegative = regexp.MustCompile(`(^|[ :;])-\d`)

// itemsRowOf2 is the snapshot's items row for an id.
func itemsRowOf2(t *testing.T, id int64) store.Row {
	t.Helper()
	r, err := tsStore().Get("items", id)
	if err != nil || r == nil {
		t.Fatalf("item %d: not in the snapshot (%v)", id, err)
	}
	return r
}

// TestOfflineBenchmarkSkillUpGate: the rank gate of a route answer names the first step that needs the new rank, never
// a step the character's current rank already covers, and states the level and skill the tier needs.
func TestOfflineBenchmarkSkillUpGate(t *testing.T) {
	ds := tsStore()
	var out []benchRow
	for _, tc := range []struct {
		faction, prof     string
		rank, max         int
		wantStep, notStep string
	}{
		{"Horde", "Mining", 1, 75, "Before the skill 65 to 125 step", "Before the skill 1 to 65 step"},
		{"Alliance", "Herbalism", 10, 75, "Before the skill 70 to 115 step", "Before the skill 10 to 70 step"},
		{"Horde", "Skinning", 1, 75, "Before the skill 75 to 130 step", "Before the skill 1 to 75 step"},
		{"Horde", "Fishing", 1, 75, "Before the skill 75 to 150 step", "Before the skill 1 to 75 step"},
	} {
		c := tsChar(tc.faction, 12, tsSkill{tc.prof, tc.rank, tc.max})
		gate := tsGate(professions.RouteFacts(c, ds, nil, tc.prof))
		// the rank opens at the skill and level the tier row holds (Journeyman: skill 50, level 10), and the rank held stops at 75
		// (a tier row with no level takes the level of profession_notes' Journeyman rank)
		need := ""
		level := int64(0)
		for _, r := range tsRows(t, "profession_notes", "section", "ranks") {
			if text(r["subject"]) == "Journeyman" {
				level = num(r["min_level"])
			}
		}
		for _, r := range tsRows(t, "profession_tiers", "profession", tc.prof) {
			if text(r["tier"]) == "Journeyman" {
				if l := num(r["min_level"]); l > 0 {
					level = l
				}
				need = fmt.Sprintf("character level %d, skill %d", level, num(r["skill_required"]))
			}
		}
		ok := strings.HasPrefix(gate, tc.wantStep) && !strings.HasPrefix(gate, tc.notStep) && need != "" && strings.Contains(gate, need+"; the rank you hold stops at 75")
		var firstOver string
		var firstFrom int64
		for _, r := range tsRows(t, "profession_routes", "profession", tc.prof) {
			if r["alternative_of"] == nil && num(r["skill_to"]) > int64(tc.max) && (firstOver == "" || num(r["skill_from"]) < firstFrom) {
				firstOver, firstFrom = fmt.Sprintf("skill %d to %d", num(r["skill_from"]), num(r["skill_to"])), num(r["skill_from"])
			}
		}
		out = append(out, benchRow{fmt.Sprintf("%s %s %d/%d: the step that needs Journeyman", tc.faction, tc.prof, tc.rank, tc.max), gate,
			"the first route step ending above the skill cap of " + fmt.Sprint(tc.max) + ": " + firstOver, tsRouteSrc,
			resultIf(ok, "the gate names a step the current rank already covers")})
	}
	finish(t, "advice-skillup-gate", "Offline skill-up route: the step that needs the next rank", out)
}

// ---- no answer in the dataset ----

// TestOfflineBenchmarkTradeskillNoAnswer: questions the dataset has no answer for. A rank no tier row names a trainer
// for gets no trainer name and no coordinates; a profession with no tool item gets no tool; a crafted item with a
// material nothing drops or sells says the material has no source and never claims the character lacks the profession.
func TestOfflineBenchmarkTradeskillNoAnswer(t *testing.T) {
	ds := tsStore()
	var out []benchRow
	for _, tc := range []struct {
		faction, prof string
		rank, max     int
	}{
		{"Horde", "Mining", 70, 75}, {"Alliance", "Mining", 70, 75}, {"Alliance", "Skinning", 70, 75}, {"Horde", "Herbalism", 70, 75}, {"Alliance", "Fishing", 70, 75},
	} {
		c := tsChar(tc.faction, 12, tsSkill{tc.prof, tc.rank, tc.max})
		gate := tsGate(professions.RouteFacts(c, ds, nil, tc.prof))
		named, _ := tsTierTrainers(t, tc.prof, "Journeyman", tc.faction)
		unnamed := 0
		for _, r := range tsRows(t, "trainers", "profession", tc.prof) {
			if text(r["name"]) == "" {
				unnamed++
			}
		}
		ok := len(named) == 0 && strings.Contains(gate, "names no "+tc.prof+" trainer") && !strings.Contains(gate, "trainer: ") && !strings.Contains(gate, "coordinates")
		out = append(out, benchRow{fmt.Sprintf("%s %s: the trainer", tc.faction, tc.prof), gate,
			fmt.Sprintf("%s: %d tier row(s) name a trainer, %d trainers rows with no name, no coordinates", compNoneSrc, len(named), unnamed), tsTierSrc + ", dataset trainers",
			resultIf(ok, "a trainer is named or invented")})
	}
	// a profession with no tool item
	{
		c := tsChar("Horde", 20, tsSkill{"Alchemy", 60, 75})
		ans := professions.RouteFacts(c, ds, nil, "Alchemy")
		ok := !strings.Contains(ans, "They have no ")
		out = append(out, benchRow{"Alchemy: the tool", "no tool line", "no Alchemy tool item in the dataset's tool class", tsToolSrc, resultIf(ok, "a tool is invented for a profession that has none")})
	}
	// a crafted item whose material nothing drops or sells
	for _, tc := range []struct {
		item   int64
		name   string
		skills []tsSkill
	}{
		{19931, "Gurubashi Mojo Madness", []tsSkill{{"Alchemy", 300, 300}}},
		{18258, "Gordok Ogre Suit", []tsSkill{{"Tailoring", 300, 300}}},
		{213379, "Hyperconductive Arcano-Filament", []tsSkill{{"Tailoring", 300, 300}}},
	} {
		c := tsChar("Horde", 60, tc.skills...)
		txt := paths.For(c, ds, tc.item, paths.Options{}).Text()
		ok := strings.Contains(txt, "has no source in the game data") && !strings.Contains(txt, "You don't have")
		out = append(out, benchRow{tc.name + ": a material with no source", strings.ReplaceAll(txt, "\n", "; "), "the recipe's reagent has no loot_sources or vendor_sources row and binds when picked up", tsReagSrc,
			resultIf(ok, "the answer says the character lacks the profession, or invents a source")})
	}
	finish(t, "advice-tradeskill-no-answer", "Offline trade skills: where the dataset has no answer", out)
}

// ---- dataset-side gaps ----

// TestOfflineBenchmarkTradeskillDataGaps: what the dataset itself lacks for trainers and tool vendors. The advisor names
// nobody it has no name for; the table lists the rows the dataset would have to fill. Each row is a dataset-side
// difference, so none fails the run.
func TestOfflineBenchmarkTradeskillDataGaps(t *testing.T) {
	ds := tsStore()
	var out []benchRow
	for _, prof := range []string{"Mining", "Skinning", "Herbalism", "Fishing", "Cooking", "First Aid"} {
		rows := tsRows(t, "trainers", "profession", prof)
		var noName, noZone, noXY int
		for _, r := range rows {
			if text(r["name"]) == "" {
				noName++
			}
			if text(r["zone_name"]) == "" {
				noZone++
			}
			if r["x"] == nil {
				noXY++
			}
		}
		tiers := 0
		for _, r := range tsRows(t, "profession_tiers", "profession", prof) {
			if text(r["how"]) == "trainer" && text(r["trainer_name"]) == "" {
				tiers++
			}
		}
		out = append(out, benchRow{prof + ": trainer names and places", "names no trainer, gives no coordinates",
			fmt.Sprintf("%d trainers rows: %d with no name, %d with no zone, %d with no coordinates; %d trainer tier(s) with no trainer_name", len(rows), noName, noZone, noXY, tiers),
			tsTierSrc + ", dataset trainers", "differ (data): the dataset's trainers table holds no name or coordinates and the tier rows name no trainer"})
	}
	for _, tool := range []string{"Mining Pick", "Skinning Knife", "Blacksmith Hammer", "Fishing Pole"} {
		item := rowByName(t, ds, "items", tool)
		vend := tsRows(t, "vendor_sources", "item_id", num(item["id"]))
		named := 0
		for _, v := range vend {
			if n, err := ds.Get("npcs", num(v["npc_id"])); err == nil && n != nil && text(n["name"]) != "" {
				named++
			}
		}
		out = append(out, benchRow{tool + ": who sells it", "a vendor, no name", fmt.Sprintf("%d vendor rows, %d of the vendors named in npcs, buy_price %d", len(vend), named, num(item["buy_price"])), tsToolSrc,
			"differ (data): vendor_sources names the vendors by id and npcs holds no name for most of them"})
	}
	finish(t, "advice-tradeskill-data-gaps", "Offline trade skills: dataset-side gaps", out)
}

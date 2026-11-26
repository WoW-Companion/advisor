package paths_test

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/farm"
	"github.com/WoW-Companion/advisor/internal/gather"
	"github.com/WoW-Companion/advisor/internal/learn"
	"github.com/WoW-Companion/advisor/internal/levelup"
	"github.com/WoW-Companion/advisor/internal/names"
	"github.com/WoW-Companion/advisor/internal/professions"
	"github.com/WoW-Companion/advisor/internal/store"
	"github.com/WoW-Companion/advisor/internal/vendors"
	"github.com/WoW-Companion/advisor/internal/wording"
)

// The offline companion-pet, fishing, cooking-recipe and rune/glyph benchmark: where the advisor says a pet comes from,
// which zones it sends a fisher to and which rank item a fishing or cooking rank needs, where it says a cooking recipe is
// learned and at what skill, and what it offers of the class abilities that are runes, beside the dataset's rows, with
// no network. testdata/companions_snapshot (rebuild it with testdata/companions_snapshot/build.sh) holds the companion pet
// items (items class 15, subclass 2) with their loot and vendor rows, the Cooking and Fishing recipes with their recipe
// items, reagents and trainer rows, every fishing_catches row, the Cooking and Fishing profession routes and tiers, the
// profession notes, and the class spells and talents of four classes. The dataset holds no pet species table, no pet
// source other than a crafted or sold-by row of four items, no glyph and no rune item, so the cases it cannot answer
// check that the advisor says so and invents none. A difference the dataset's own rows cause is marked "(data)".
// make offline-benchmark runs it; WOWC_BENCH_REPORT receives the tables.

const (
	compDir     = "../../testdata/companions_snapshot"
	compPetSrc  = "dataset items (class 15, subclass 2; buy_price), loot_sources (crafted, sold by, creature, object), vendor_sources, npcs"
	compNameSrc = "dataset items (name), names index"
	compFishSrc = "dataset fishing_catches (zone_name, item_id, casts, rate); game home zones of the two factions"
	compRankSrc = "dataset profession_tiers (tier, skill_required, min_level, how, trainer_name, place, faction)"
	compCookSrc = "dataset recipes (learn_skill, learn_skill_source, learned_from), recipe_items, vendor_sources, trainer_spells, trainers"
	compRuneSrc = "dataset class_spells (level, spell_id, rank), talent_nodes; Season of Discovery engravings are level 1 spells above id 400000"
	compNoneSrc = "the dataset has no such row, column or table"
)

var (
	compOnce  sync.Once
	compCache store.DataStore
)

// compStore reads the snapshot once for the package: the tables are decoded on first use and the tests share them.
func compStore() store.DataStore {
	compOnce.Do(func() { compCache = store.NewDir(compDir) })
	return compCache
}

func compChar(faction, class string, level int, prof string, skill, maxSkill int) character.Character {
	c := character.Character{Name: "Benchtest", Faction: faction, Class: class, Level: &level}
	if prof != "" {
		c.Professions = []character.Skill{{Name: prof, Rank: &skill, Max: &maxSkill}}
	}
	return c
}

func compRows(t *testing.T, table, col string, id int64) []store.Row {
	t.Helper()
	rows, err := compStore().Filter(table, store.Range{Column: col, Min: id, Max: id})
	if err != nil {
		t.Fatalf("%s: %v", table, err)
	}
	return rows
}

// ---- companion pets ----

// compPets are pet items of every source kind the dataset has: crafted, sold by an unnamed creature and no source row.
var compPets = []int64{4401, 15996, 21277, 284664, 8485, 8501, 10360, 13342, 18963, 19450}

// TestOfflineBenchmarkPetSources: where the farm answer says a companion pet comes from beside the item's loot_sources
// and vendor_sources rows: a crafted pet says it is crafted and not dropped, a pet no creature drops names no creature,
// and no vendor is named for a pet whose vendor row has no name.
func TestOfflineBenchmarkPetSources(t *testing.T) {
	ds := compStore()
	var out []benchRow
	for _, id := range compPets {
		item := itemsRowOf(t, id)
		name := text(item["name"])
		rid, rname, _ := farm.Resolve(ds, name)
		spots := farm.Spots(character.Character{Class: "MAGE"}, ds, id)
		crafted := farm.Crafted(ds, id)
		txt := farm.Render(rname, spots, crafted, 5)
		loot := compRows(t, "loot_sources", "item_id", id)
		var drops, craftRows, soldRows int
		var profession string
		var learn int64
		for _, l := range loot {
			switch k := text(l["source_kind"]); {
			case k == "creature" || k == "object":
				drops++
			case k == "crafted":
				craftRows++
				profession, learn = text(l["profession"]), num(l["learn_skill"])
			case k == "sold by":
				soldRows++
			}
		}
		vend := compRows(t, "vendor_sources", "item_id", id)
		offer, hasOffer := vendors.Best(ds, id)
		ok := rid == id && len(spots) == drops && crafted == (craftRows > 0) && !hasOffer
		ok = ok && strings.Contains(txt, "No creature or object drops it") == (drops == 0)
		ok = ok && strings.Contains(txt, "crafted") == crafted && !strings.Contains(txt, "can also be crafted") == (drops == 0 || !crafted)
		ref := fmt.Sprintf("%d creature or object rows, %d crafted rows", drops, craftRows)
		if craftRows > 0 {
			ref += fmt.Sprintf(" (%s %d)", profession, learn)
		}
		ref += fmt.Sprintf(", %d sold-by rows, %d vendor rows, buy_price %d", soldRows, len(vend), num(item["buy_price"]))
		out = append(out, benchRow{name + ": where the pet comes from", firstLine(txt) + " | " + strings.ReplaceAll(strings.TrimSpace(strings.SplitN(txt, "\n", 2)[1]), "\n", " "), ref, compPetSrc,
			resultIf(ok, "a creature, a vendor or a crafted source the dataset does not hold, or a crafted pet called dropped")})
		if hasOffer {
			t.Errorf("%s: a vendor %q named from vendor rows with no name", name, offer.Vendor)
		}
		switch {
		case len(vend) > 0:
			out = append(out, benchRow{name + ": who sells it", "no vendor named", fmt.Sprintf("%d vendor rows, no npcs name for the seller", len(vend)), compPetSrc, "differ (data): npcs holds no name for the vendor that sells the pet"})
		case num(item["buy_price"]) > 0 && craftRows == 0:
			out = append(out, benchRow{name + ": who sells it", "no vendor named", fmt.Sprintf("buy_price %d, so a vendor sells it", num(item["buy_price"])), compPetSrc, "differ (data): the item has a buy price and no vendor_sources row"})
		}
	}
	finish(t, "advice-pet-source", "Offline companion pets: where each comes from", out)
}

func itemsRowOf(t *testing.T, id int64) store.Row {
	t.Helper()
	r, err := compStore().Get("items", id)
	if err != nil || r == nil {
		t.Fatalf("item %d: not in the snapshot (%v)", id, err)
	}
	return r
}

// TestOfflineBenchmarkPetNames: a pet named in full, by a leading part, or by a name two items share: one item when one
// matches, and the candidates with their ids when several do.
func TestOfflineBenchmarkPetNames(t *testing.T) {
	ds := compStore()
	var out []benchRow
	for _, tc := range []struct{ query, kind string }{
		{"Parrot Cage (Senegal)", "full"}, {"Tiny Crimson Whelpling", "full"}, {"Wood Frog Box", "full"}, {"Lifelike Mechanical Toad", "full"},
		{"Cat Carrier", "part"}, {"Parrot Cage", "part"}, {"Turtle Egg", "part"}, {"Panda Collar", "twin"}, {"Tranquil Mechanical", "part-one"},
	} {
		id, name, cands := farm.Resolve(ds, tc.query)
		var want []string
		items, _ := ds.Filter("items", store.Range{Column: "class_id", Min: int64(15), Max: int64(15)})
		for _, it := range items {
			if strings.HasPrefix(strings.ToLower(text(it["name"])), strings.ToLower(tc.query)) && num(it["subclass_id"]) == 2 {
				want = append(want, fmt.Sprintf("%s (%d)", text(it["name"]), num(it["id"])))
			}
		}
		slices.Sort(want)
		got := slices.Clone(cands)
		slices.Sort(got)
		var ok bool
		if len(want) == 1 {
			ok = id != 0 && strings.EqualFold(name, want[0][:strings.LastIndex(want[0], " (")])
		} else {
			ok = id == 0 && slices.Equal(got, want)
		}
		out = append(out, benchRow{tc.query + ": which pet", fmt.Sprintf("id %d %q, candidates %v", id, name, cands), fmt.Sprintf("%d pet items start with it: %v", len(want), want), compNameSrc,
			resultIf(ok, "one item for a name that fits several, or a candidate list that is not the items")})
	}
	finish(t, "advice-pet-names", "Offline companion pets: finding a pet by name", out)
}

// ---- fishing ----

var compHomeZones = map[string]string{
	"Orgrimmar": "horde", "Durotar": "horde", "Undercity": "horde", "Tirisfal Glades": "horde", "Silverpine Forest": "horde", "Mulgore": "horde", "Thunder Bluff": "horde",
	"Stormwind City": "alliance", "Elwynn Forest": "alliance", "Westfall": "alliance", "Ironforge": "alliance", "Dun Morogh": "alliance", "Loch Modan": "alliance", "Darnassus": "alliance", "Teldrassil": "alliance",
}

// catchZonesOf are the zones the dataset has an item caught in, best rate first (ties by name), without the other
// faction's home zones, at most limit of them.
func catchZonesOf(t *testing.T, id int64, faction string, limit int) []string {
	t.Helper()
	rows := compRows(t, "fishing_catches", "item_id", id)
	slices.SortStableFunc(rows, func(a, b store.Row) int {
		x, _ := a["rate"].(float64)
		y, _ := b["rate"].(float64)
		switch {
		case x > y:
			return -1
		case x < y:
			return 1
		}
		return strings.Compare(text(a["zone_name"]), text(b["zone_name"]))
	})
	var out []string
	for _, r := range rows {
		z := text(r["zone_name"])
		if side := compHomeZones[z]; side != "" && !strings.EqualFold(side, faction) {
			continue
		}
		if out = append(out, z); len(out) == limit {
			break
		}
	}
	return out
}

var fishLine = regexp.MustCompile(`^Fish in (.+?) for (.+?) \(`)

// fishingZones: the zones a Cooking plan from skill to target sends a character to for raw fish, beside the zones of
// fishing_catches that give the fish, best catch rate first, and no capital or starting zone of the other faction.
// One plan to a test: a plan takes most of a test's time.
func fishingZones(t *testing.T, part, faction string, skill, target int) {
	ds := compStore()
	var out []benchRow
	pa := professions.ComputePath(compChar(faction, "MAGE", 60, "Cooking", skill, 300), ds, "Cooking", target, 1)
	for _, line := range pa.Prep {
		m := fishLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		hits, _, _ := names.Find(ds, names.Items, m[2], 3)
		if len(hits) == 0 {
			t.Fatalf("%s: not an item of the snapshot", m[2])
		}
		want := catchZonesOf(t, hits[0].ID, faction, 2)
		got := strings.Split(m[1], " or ")
		out = append(out, benchRow{fmt.Sprintf("%s: where a %s Cooking plan (skill %d) fishes it", m[2], faction, skill), strings.Join(got, ", "), strings.Join(want, ", "), compFishSrc,
			resultIf(slices.Equal(got, want), "a zone with a lower catch rate, or a zone of the other faction's home")})
	}
	if len(out) == 0 {
		t.Fatal("no fishing line in the plan")
	}
	finish(t, "advice-fishing-zones-"+part, "Offline fishing: the zones a plan names for raw fish ("+part+")", out)
}

func TestOfflineBenchmarkFishingZonesHordeLow(t *testing.T) {
	fishingZones(t, "horde-low", "Horde", 1, 100)
}
func TestOfflineBenchmarkFishingZonesHordeMid(t *testing.T) {
	fishingZones(t, "horde-mid", "Horde", 120, 170)
}
func TestOfflineBenchmarkFishingZonesAllyLow(t *testing.T) {
	fishingZones(t, "alliance-low", "Alliance", 1, 100)
}
func TestOfflineBenchmarkFishingZonesAllyMid(t *testing.T) {
	fishingZones(t, "alliance-mid", "Alliance", 120, 170)
}

// TestOfflineBenchmarkFishingWhere: "where can I fish X?" with no Questie database: the dataset's catch zones, best rate
// first and without the other faction's home, and none for an item no cast has caught.
func TestOfflineBenchmarkFishingWhere(t *testing.T) {
	ds := compStore()
	g := gather.New(nil, ds)
	var out []benchRow
	for _, tc := range []struct{ item, faction string }{
		{"Raw Longjaw Mud Snapper", "Horde"}, {"Raw Longjaw Mud Snapper", "Alliance"}, {"Raw Brilliant Smallfish", "Horde"}, {"Raw Brilliant Smallfish", "Alliance"},
		{"Raw Rockscale Cod", "Alliance"}, {"Deviate Fish", "Horde"}, {"Raw Bristle Whisker Catfish", "Alliance"},
	} {
		hits, _, _ := names.Find(ds, names.Items, tc.item, 3)
		want := catchZonesOf(t, hits[0].ID, tc.faction, gather.MaxZones)
		res := g.Find(compChar(tc.faction, "MAGE", 40, "", 0, 0), tc.item, "fish")
		var got []string
		for _, z := range res.Zones {
			got = append(got, z.Zone)
		}
		ok := slices.Equal(got, want) && strings.HasPrefix(res.Text, "Fish for "+tc.item+": ")
		out = append(out, benchRow{fmt.Sprintf("%s (%s): where to fish", tc.item, tc.faction), res.Text, strings.Join(want, ", "), compFishSrc, resultIf(ok, "no zone, or zones in another order")})
	}
	finish(t, "advice-fishing-where", "Offline fishing: where to fish an item with no Questie database", out)
}

// TestOfflineBenchmarkFishingRanks: the rank a character must get before the next step, beside profession_tiers: a trainer
// teaches Journeyman, a book teaches Expert and a quest gives Artisan, and the advice names that source and its place,
// not a trainer.
func TestOfflineBenchmarkFishingRanks(t *testing.T) {
	ds := compStore()
	var out []benchRow
	for _, prof := range []string{"Fishing", "Cooking"} {
		for _, rk := range []struct {
			tier  string
			skill int
		}{{"Journeyman", 70}, {"Expert", 145}, {"Artisan", 220}} {
			for _, faction := range []string{"Horde", "Alliance"} {
				var how, who, place string
				var needLevel int64
				rows, _ := ds.Filter("profession_tiers", store.Range{Column: "profession", Min: prof, Max: prof})
				for _, r := range rows {
					if f := text(r["faction"]); text(r["tier"]) == rk.tier && (f == "" || f == "Both" || f == faction) && how == "" {
						how, who, place, needLevel = text(r["how"]), text(r["trainer_name"]), text(r["place"]), num(r["min_level"])
					}
				}
				c := compChar(faction, "MAGE", 60, prof, rk.skill, rk.skill+5)
				facts := professions.RouteFacts(c, ds, nil, prof)
				line := ""
				for _, l := range strings.Split(facts, "\n") {
					if strings.HasPrefix(l, "Before ") {
						line = l
					}
				}
				ok := line != "" && !strings.Contains(line, "step  ") && strings.Contains(line, rk.tier)
				switch how {
				case "book", "quest":
					ok = ok && strings.Contains(line, "you must get "+prof+" "+rk.tier) && strings.Contains(line, who) && strings.Contains(line, place) && !strings.Contains(line, "trainer")
				default:
					ok = ok && strings.Contains(line, "you must train "+prof+" "+rk.tier)
				}
				if how == "trainer" && needLevel > 0 {
					ok = ok && strings.Contains(line, fmt.Sprintf("character level %d", needLevel))
				}
				ref := fmt.Sprintf("%s: %s %s %s (level %d)", rk.tier, how, who, place, needLevel)
				if how == "" {
					ref = rk.tier + ": no profession_tiers row"
				}
				out = append(out, benchRow{fmt.Sprintf("%s %s (%s): how the rank is gained", prof, rk.tier, faction), line, ref, compRankSrc,
					resultIf(ok, "a trainer named for a rank a book or a quest gives, or an empty step name")})
			}
		}
	}
	finish(t, "advice-fishing-ranks", "Offline Fishing and Cooking ranks: trainer, book or quest", out)
}

// rankPath: the plan's "First ..." line for a rank a book or a quest gives names that source and its place, and says
// "train at your trainer" for a rank a trainer teaches.
func rankPath(t *testing.T, part, prof, faction, tier string, skill, target int) {
	ds := compStore()
	var how, who, place string
	rows, _ := ds.Filter("profession_tiers", store.Range{Column: "profession", Min: prof, Max: prof})
	for _, r := range rows {
		if f := text(r["faction"]); text(r["tier"]) == tier && (f == "" || f == "Both" || f == faction) && how == "" {
			how, who, place = text(r["how"]), text(r["trainer_name"]), text(r["place"])
		}
	}
	path := professions.RenderPath(professions.ComputePath(compChar(faction, "MAGE", 60, prof, skill, skill+5), ds, prof, target, 1))
	line := ""
	for _, l := range strings.Split(path, "\n") {
		if strings.Contains(l, tier+" ") && strings.Contains(l, "First ") {
			line = strings.TrimSpace(l)
		}
	}
	ok := line != ""
	if how == "book" || how == "quest" {
		ok = ok && strings.HasPrefix(line, "First get "+tier) && strings.Contains(line, who) && strings.Contains(line, place)
	} else {
		ok = ok && strings.HasPrefix(line, "First train "+tier)
	}
	out := []benchRow{{fmt.Sprintf("%s %s (%s): the plan's first line", prof, tier, faction), line, fmt.Sprintf("%s: %s %s %s", tier, how, who, place), compRankSrc,
		resultIf(ok, "a trainer named for a rank a book or a quest gives")}}
	finish(t, "advice-rank-path-"+part, "Offline "+prof+" "+tier+": the plan's rank line ("+part+")", out)
}

func TestOfflineBenchmarkRankPathCooking(t *testing.T) {
	rankPath(t, "cooking-expert", "Cooking", "Horde", "Expert", 145, 175)
}
func TestOfflineBenchmarkRankPathFishing(t *testing.T) {
	rankPath(t, "fishing-artisan", "Fishing", "Alliance", "Artisan", 220, 260)
}
func TestOfflineBenchmarkRankPathJourneyman(t *testing.T) {
	rankPath(t, "fishing-journeyman", "Fishing", "Horde", "Journeyman", 70, 100)
}

// ---- cooking and fishing recipes ----

// compRecipes are Cooking and Fishing recipes of each source: taught by a trainer, by a recipe item a vendor sells, by
// both, by an item the dataset holds no row for, one whose skill is an estimate, one with no skill and one with two spells.
var compRecipes = []struct{ name, faction string }{
	{"Spiced Wolf Meat", "Horde"}, {"Coyote Steak", "Alliance"}, {"Boiled Clams", "Horde"}, {"Crab Cake", "Alliance"}, {"Dry Pork Ribs", "Horde"},
	{"Goblin Deviled Clams", "Alliance"}, {"Barbecued Buzzard Wing", "Horde"}, {"Spider Sausage", "Alliance"}, {"Roasted Boar Meat", "Horde"},
	{"Smoked Desert Dumplings", "Alliance"}, {"Brilliant Smallfish", "Horde"}, {"Slitherskin Mackerel", "Alliance"}, {"Tasty Raptor Bites", "Horde"},
	{"Grand Lobster Banquet", "Horde"}, {"Cleansing Stew", "Alliance"}, {"Herb Baked Egg", "Horde"}, {"Basic Campfire", "Alliance"}, {"Iron Oven", "Horde"},
	{"Dirge's Kickin' Chimaerok Chops", "Alliance"}, {"Fish Bowl", "Horde"}, {"Fishing Rack", "Alliance"}, {"Fishing Hut", "Horde"},
}

// TestOfflineBenchmarkCookingRecipeSources: where a Cooking or Fishing recipe is learned and at what skill: the skill the
// row holds (said as "about" when it is an estimate, and not said when it is 1 or unknown), a trainer when the row says
// a trainer teaches it, the recipe item when an item does, and no vendor or trainer the dataset does not name.
func TestOfflineBenchmarkCookingRecipeSources(t *testing.T) {
	ds := compStore()
	var out []benchRow
	for _, tc := range compRecipes {
		recs, err := ds.Filter("recipes", store.Range{Column: "name", Min: tc.name, Max: tc.name})
		if err != nil || len(recs) == 0 {
			t.Fatalf("%s: not in the snapshot (%v)", tc.name, err)
		}
		r := recs[0]
		spell := num(r["spell_id"])
		skillLine := text(r["skill_line_name"])
		res, err := learn.Find(character.Character{Faction: tc.faction, Class: "MAGE"}, ds, nil, tc.name)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		from, need, src := text(r["learned_from"]), num(r["learn_skill"]), text(r["learn_skill_source"])
		var itemNames []string
		var vendRows int
		for _, ri := range compRows(t, "recipe_items", "teaches_spell_id", spell) {
			if it, err := ds.Get("items", num(ri["item_id"])); err == nil && it != nil && text(it["name"]) != "" {
				itemNames = append(itemNames, text(it["name"]))
			}
			vendRows += len(compRows(t, "vendor_sources", "item_id", num(ri["item_id"])))
		}
		trainerRows := len(compRows(t, "trainer_spells", "spell_id", spell))
		ok := true
		switch {
		case need > 1 && src == "estimate":
			ok = strings.Contains(res.Text, fmt.Sprintf("It needs about %s %d.", skillLine, need)) && res.Gate.Skill == int(need)
		case need > 1:
			ok = strings.Contains(res.Text, fmt.Sprintf("It needs %s %d.", skillLine, need)) && res.Gate.Skill == int(need)
		default:
			ok = !strings.Contains(res.Text, "It needs")
		}
		if from == "trainer" || from == "both" {
			ok = ok && strings.HasPrefix(res.Text, "Ask "+wording.A(skillLine)+" trainer")
		} else {
			ok = ok && !strings.Contains(res.Text, "Ask "+wording.A(skillLine)+" trainer") && strings.Contains(res.Text, "recipe item")
			if len(itemNames) > 0 {
				found := false
				for _, n := range itemNames {
					found = found || strings.Contains(res.Text, n)
				}
				ok = ok && found
			}
		}
		ok = ok && len(res.Ways) == 0 // no trainer or vendor row of the snapshot is named
		ref := fmt.Sprintf("learned from %s; learn_skill %d (%s); %d trainer rows; recipe items %v, %d vendor rows", from, need, src, trainerRows, itemNames, vendRows)
		out = append(out, benchRow{fmt.Sprintf("%s (%s): where the recipe is learned", tc.name, tc.faction), strings.ReplaceAll(strings.TrimSpace(res.Text), "\n", " "), ref, compCookSrc,
			resultIf(ok, "the skill, the trainer or the recipe item the row says, or a place the dataset does not name")})
		if trainerRows > 0 || vendRows > 0 {
			out = append(out, benchRow{tc.name + ": trainer or vendor name", "no place named", fmt.Sprintf("%d trainer rows, %d vendor rows", trainerRows, vendRows), compCookSrc, "differ (data): trainers.name and npcs.name hold no name for the trainer or vendor"})
		}
	}
	finish(t, "advice-cooking-recipes", "Offline Cooking and Fishing recipes: where each is learned and at what skill", out)
}

// TestOfflineBenchmarkTrainerRecipeSkill: the skill a trainer asks for a Cooking recipe (trainer_spells.required_skill)
// is the skill the advisor states for it (recipes.learn_skill).
func TestOfflineBenchmarkTrainerRecipeSkill(t *testing.T) {
	ds := compStore()
	rows, err := ds.Filter("trainer_spells", store.Range{Column: "spell_id", Min: int64(0), Max: int64(1 << 40)})
	if err != nil {
		t.Fatal(err)
	}
	var out []benchRow
	done := map[int64]bool{}
	for _, tr := range rows {
		spell := num(tr["spell_id"])
		if done[spell] {
			continue
		}
		done[spell] = true
		name := text(tr["recipe_name"])
		res, err := learn.Find(character.Character{Faction: "Horde", Class: "MAGE"}, ds, nil, name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		want := int(num(tr["required_skill"]))
		out = append(out, benchRow{fmt.Sprintf("%s: the skill the trainer asks", name), fmt.Sprintf("skill %d: %s", res.Gate.Skill, strings.ReplaceAll(strings.TrimSpace(res.Gate.Line), "\n", " ")), fmt.Sprintf("trainer_spells.required_skill %d, cost %d copper", want, num(tr["cost_copper"])), compCookSrc,
			resultIf(res.Gate.Skill == want, "another skill than the trainer asks")})
	}
	if len(out) < 6 {
		t.Fatalf("only %d trainer recipes", len(out))
	}
	finish(t, "advice-cooking-trainer-skill", "Offline Cooking trainer recipes: the skill asked", out)
}

// ---- runes, pet abilities and glyphs ----

// TestOfflineBenchmarkRuneAbilities: the class abilities the dataset lists at level 1 with a spell id above 400000 are
// engravings a rune item teaches; the trainer list the advisor gives for levels 1 to 10 holds none of them, and holds the
// level-appropriate trainer spells.
func TestOfflineBenchmarkRuneAbilities(t *testing.T) {
	ds := compStore()
	var out []benchRow
	for _, tc := range []struct {
		class string
		id    int64
	}{{"WARRIOR", 1}, {"PALADIN", 2}, {"HUNTER", 3}, {"MAGE", 8}} {
		rows, _ := ds.Filter("class_spells", store.Range{Column: "class_id", Min: tc.id, Max: tc.id}, store.Range{Column: "level", Min: int64(0), Max: int64(1)})
		runes := map[int64]string{}
		for _, r := range rows {
			if id := num(r["spell_id"]); id >= 400000 {
				runes[id] = text(r["name"])
			}
		}
		spells := levelup.Spells(compChar("Horde", tc.class, 10, "", 0, 0), ds, 1, 10)
		var bad, listed []string
		for _, s := range spells {
			listed = append(listed, s.Label())
			if n, ok := runes[s.ID]; ok {
				bad = append(bad, n+" is a rune ability")
			}
		}
		out = append(out, benchRow{fmt.Sprintf("%s: the trainer spells of levels 1 to 10", strings.ToLower(tc.class)), fmt.Sprintf("%d listed: %s", len(listed), strings.Join(listed, ", ")), fmt.Sprintf("%d rune abilities at level 1 (spell id above 400000)", len(runes)), compRuneSrc,
			resultIf(len(runes) > 0 && len(listed) > 0 && len(bad) == 0, strings.Join(bad, "; "))})
	}
	finish(t, "advice-rune-abilities", "Offline rune abilities: never offered as trainer spells", out)
}

// TestOfflineBenchmarkHunterPetAbilities: the Hunter's Mend Pet ranks are offered at the levels the dataset gives them, and
// "Improved Mend Pet", which is a talent in the dataset's talent nodes, is not offered as a trainer spell.
func TestOfflineBenchmarkHunterPetAbilities(t *testing.T) {
	ds := compStore()
	c := compChar("Horde", "HUNTER", 60, "", 0, 0)
	rows, _ := ds.Filter("class_spells", store.Range{Column: "class_id", Min: int64(3), Max: int64(3)})
	var out []benchRow
	for _, lv := range []int{12, 20, 28, 36, 44, 52, 60} {
		var want int64
		for _, r := range rows {
			if text(r["name"]) == "Mend Pet" && num(r["level"]) == int64(lv) {
				want = num(r["rank"])
			}
		}
		got := int64(-1)
		for _, s := range levelup.Spells(c, ds, lv, lv) {
			if s.Name == "Mend Pet" {
				got = int64(s.Rank)
			}
		}
		out = append(out, benchRow{fmt.Sprintf("Mend Pet at level %d", lv), fmt.Sprintf("rank %d", got), fmt.Sprintf("class_spells rank %d", want), compRuneSrc, resultIf(got == want && want > 0, "another rank than the row")})
	}
	nodes, _ := ds.Filter("talent_nodes", store.Range{Column: "node_id", Min: int64(0), Max: int64(1 << 40)})
	isTalent := false
	for _, n := range nodes {
		isTalent = isTalent || text(n["name"]) == "Improved Mend Pet"
	}
	offered := false
	for _, s := range levelup.Spells(c, ds, 55, 60) {
		offered = offered || s.Name == "Improved Mend Pet"
	}
	out = append(out, benchRow{"Improved Mend Pet: trainer or talent", fmt.Sprintf("offered by a trainer: %v", offered), fmt.Sprintf("a talent node of that name: %v", isTalent), compRuneSrc, resultIf(!(isTalent && offered), "a talent offered as a trainer spell")})
	finish(t, "advice-hunter-pet-abilities", "Offline Hunter pet abilities: ranks and talents", out)
}

// ---- no answer ----

// TestOfflineBenchmarkCompanionNoAnswer: what the dataset does not hold. No glyph item, glyph recipe or Inscription
// profession, no pet or rune item of a name the game never had, no table of pet species, and a fish no cast has caught:
// the advisor finds nothing and names no place.
func TestOfflineBenchmarkCompanionNoAnswer(t *testing.T) {
	ds := compStore()
	var out []benchRow
	for _, name := range []string{"Glyph of Frost Nova", "Glyph of Mend Pet", "Glyph of Sinister Strike", "Rune of the Archmage", "Mini Thor", "Spirit of Competition Cage"} {
		id, _, cands := farm.Resolve(ds, name)
		out = append(out, benchRow{name + ": an item", fmt.Sprintf("id %d, %d candidates", id, len(cands)), "no items row of that name", compNoneSrc, resultIf(id == 0 && len(cands) == 0, "an item was invented")})
	}
	for _, name := range []string{"Glyph of Fire Blast", "Minor Glyph of Slowing"} {
		res, err := learn.Find(character.Character{Faction: "Horde", Class: "MAGE"}, ds, nil, name)
		out = append(out, benchRow{name + ": a glyph recipe", fmt.Sprintf("%d ways, %v", len(res.Ways), err), "no recipes row, no Inscription skill line", compNoneSrc, resultIf(err != nil && len(res.Ways) == 0, "a recipe was invented")})
	}
	pa := professions.ComputePath(compChar("Horde", "MAGE", 60, "Inscription", 10, 75), ds, "Inscription", 0, 1)
	out = append(out, benchRow{"Inscription: a plan", fmt.Sprintf("%d bands, notes %v", len(pa.Bands), pa.Notes), "no Inscription recipes or route", compNoneSrc, resultIf(len(pa.Bands) == 0, "a plan was invented")})
	for _, table := range []string{"pets", "pet_species", "glyphs", "runes"} {
		_, err := ds.Filter(table, store.Range{Column: "id", Min: int64(0), Max: int64(1)})
		out = append(out, benchRow{table + ": a table", fmt.Sprint(err), "no such table in the dataset", compNoneSrc, resultIf(err != nil, "a table was invented")})
	}
	g := gather.New(nil, ds)
	for _, item := range []string{"Raw Slitherskin Mackerel", "Raw Nightfin Snapper"} {
		res := g.Find(compChar("Horde", "MAGE", 40, "", 0, 0), item, "fish")
		out = append(out, benchRow{item + ": where to fish", res.Text, "no fishing_catches row for the item", compNoneSrc, resultIf(len(res.Zones) == 0 && strings.HasPrefix(res.Text, "No "), "a zone was invented")})
	}
	finish(t, "advice-companion-no-answer", "Offline pets, fishing, cooking and glyphs: no answer in the dataset", out)
}

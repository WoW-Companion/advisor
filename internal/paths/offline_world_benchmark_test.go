package paths_test

import (
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/learn"
	"github.com/WoW-Companion/advisor/internal/professions"
	"github.com/WoW-Companion/advisor/internal/questie"
	"github.com/WoW-Companion/advisor/internal/store"
	"github.com/WoW-Companion/advisor/internal/vendors"
)

// The offline zone-connection, vendor-price, recipe-level and world-event benchmark: what the advisor answers about the
// continent a zone is on and how to cross between continents, what a vendor charges for an item, the skill a recipe is
// learned at and turns yellow and grey at, and which holiday events run when, beside the dataset's rows, with no network.
// testdata/world_snapshot (rebuild it with testdata/world_snapshot/build.sh) holds every ui_maps row (a zone's parent
// continent) and the vendor offers of the items priced here; the recipes come from testdata/advice_snapshot. The dataset
// holds no zone border, no boat or zeppelin, no vendor name, no event table and no event window, so the cases it cannot
// answer check that the advisor says so and states none. A vendor the snapshot names nowhere is given a synthetic name so
// the price logic can run; the rows stay the dataset's. A difference the dataset's own rows cause is marked "(data)".
// make offline-benchmark runs it; WOWC_BENCH_REPORT receives the tables.

const (
	worldDir      = "../../testdata/world_snapshot"
	zoneSrc       = "dataset ui_maps (name, type, parent_ui_map_id: a zone's parent is its continent)"
	crossSrc      = "game transport network (zeppelins Undercity-Orgrimmar, boats Menethil Harbor-Auberdine, Dark Portal in the Blasted Lands; Wowhead transportation guides); the dataset has no such row"
	priceSrc      = "dataset vendor_sources (price_copper, reporters), items (buy_price)"
	recipeLevSrc  = "dataset recipes (learn_skill, learn_skill_source, yellow_skill, grey_skill), trainer_spells, recipe_items"
	eventSrc      = "game holiday calendar (Blizzard news and Wowhead event guides); the dataset has no event table"
	eventSortSrc  = "game quest sort ids of holiday quests (QuestSort: -1001 Winter Veil, -1002 Children's Week, -1003 Hallow's End, ...)"
	worldNoneSrc  = "the dataset has no such row, column or table"
	worldPriceNil = "no gold price: the offer costs tokens or items"
)

var (
	worldOnce  sync.Once
	worldCache store.DataStore
)

// worldStore reads the snapshot once for the package: the tables are decoded on first use and the tests share them.
func worldStore() store.DataStore {
	worldOnce.Do(func() { worldCache = consVendors{store.NewDir(worldDir)} })
	return worldCache
}

// ---- zone connections ----

// worldZones are the zones of ui_maps by name with the name of their parent continent, as the dataset has them.
func worldZones(t *testing.T) map[string]string {
	t.Helper()
	ds := worldStore()
	rows, err := ds.Filter("ui_maps", store.Range{Column: "ui_map_id", Min: int64(0)})
	if err != nil {
		t.Fatal(err)
	}
	byID := map[int64]store.Row{}
	for _, r := range rows {
		byID[num(r["ui_map_id"])] = r
	}
	out := map[string]string{}
	for _, r := range rows {
		if text(r["type"]) != "zone" || r["parent_ui_map_id"] == nil {
			continue
		}
		p := byID[num(r["parent_ui_map_id"])]
		if text(p["type"]) == "continent" {
			out[text(r["name"])] = text(p["name"])
		}
	}
	return out
}

var worldZoneCases = []string{"Elwynn Forest", "Durotar", "Tirisfal Glades", "Teldrassil", "The Barrens", "Dun Morogh", "Stranglethorn Vale", "Mulgore", "Stormwind City", "Orgrimmar", "Un'Goro Crater", "Riverglades", "Shen'dralas"}

// TestOfflineBenchmarkZoneContinent: the continent the advisor puts a zone on beside the parent continent of the zone's
// ui_maps row, for named zones and for every zone of the table.
func TestOfflineBenchmarkZoneContinent(t *testing.T) {
	zones := worldZones(t)
	var out []benchRow
	for _, z := range worldZoneCases {
		want, ok := zones[z]
		if !ok {
			t.Fatalf("%s: not in ui_maps", z)
		}
		got := learn.ContinentOf(z)
		out = append(out, benchRow{z + ": continent", got, want, zoneSrc, resultIf(got == want, "the zone is on another continent or on none")})
	}
	var bad []string
	for z, want := range zones {
		if learn.ContinentOf(z) != want {
			bad = append(bad, z)
		}
	}
	slices.Sort(bad)
	out = append(out, benchRow{fmt.Sprintf("all %d zones of ui_maps with a continent parent", len(zones)), fmt.Sprintf("%d placed elsewhere or nowhere %v", len(bad), bad), "every zone on its parent continent", zoneSrc, resultIf(len(bad) == 0, "zones placed wrongly: "+strings.Join(bad, ", "))})
	// a zone whose parent is the world map, not a continent, is on no continent
	out = append(out, benchRow{"Zephras Isle: continent", fmt.Sprintf("%q", learn.ContinentOf("Zephras Isle")), "none: its ui_maps parent is the Azeroth world map", zoneSrc, resultIf(learn.ContinentOf("Zephras Isle") == "", "a continent was invented")})
	finish(t, "advice-zone-continent", "Offline zone connections: the continent of a zone", out)
}

var worldCrossings = []struct {
	faction, from, to, want string
}{
	{"Horde", "Eastern Kingdoms", "Kalimdor", "Zeppelin to Orgrimmar"},
	{"Horde", "Kalimdor", "Eastern Kingdoms", "Zeppelin to Undercity"},
	{"Alliance", "Eastern Kingdoms", "Kalimdor", "Boat to Auberdine"},
	{"Alliance", "Kalimdor", "Eastern Kingdoms", "Boat to Menethil Harbor"},
	{"Horde", "Eastern Kingdoms", "Outland", "Travel to the Dark Portal in the Blasted Lands and step through"},
	{"Alliance", "Kalimdor", "Outland", "Boat to Menethil Harbor, then the Dark Portal in the Blasted Lands"},
	{"Horde", "Outland", "Kalimdor", "Step through the Dark Portal in Hellfire Peninsula, then Zeppelin to Orgrimmar"},
	{"Horde", "Kalimdor", "Kalimdor", ""},
}

// TestOfflineBenchmarkZoneCrossing: the step the advisor gives between two continents for a faction, beside the game's
// transport network. The dataset holds none of it.
func TestOfflineBenchmarkZoneCrossing(t *testing.T) {
	var out []benchRow
	for _, tc := range worldCrossings {
		got := learn.Crossing(tc.faction, tc.from, tc.to)
		want := tc.want
		if want == "" {
			want = "none: the same continent"
		}
		out = append(out, benchRow{fmt.Sprintf("%s from %s to %s", tc.faction, tc.from, tc.to), got, want, crossSrc, resultIf(got == tc.want, "the boat, zeppelin or portal")})
	}
	finish(t, "advice-zone-crossing", "Offline zone connections: crossing between continents", out)
}

// TestOfflineBenchmarkZoneNoAnswer: connections the dataset does not hold. A zone it lacks has no continent, no
// crossing is given from or to a continent the advisor does not know, and no border, travel time or flight path between
// two zones of one continent is stated.
func TestOfflineBenchmarkZoneNoAnswer(t *testing.T) {
	ds := worldStore()
	zones := worldZones(t)
	var out []benchRow
	for _, z := range []string{"The Synthetic Vale", "Pale Oak Reach"} {
		_, in := zones[z]
		got := learn.ContinentOf(z)
		out = append(out, benchRow{z + ": continent", fmt.Sprintf("%q", got), "no ui_maps row", worldNoneSrc, resultIf(!in && got == "", "a continent was invented")})
	}
	for _, tc := range []struct{ from, to string }{{"", "Kalimdor"}, {"Eastern Kingdoms", ""}, {"Synthetic Isles", "Kalimdor"}} {
		got := learn.Crossing("Horde", tc.from, tc.to)
		out = append(out, benchRow{fmt.Sprintf("crossing from %q to %q", tc.from, tc.to), fmt.Sprintf("%q", got), "no crossing for an unknown continent", worldNoneSrc, resultIf(got == "", "a crossing was invented")})
	}
	got := learn.Crossing("", "Eastern Kingdoms", "Kalimdor")
	out = append(out, benchRow{"crossing for a character with no faction", fmt.Sprintf("%q", got), "no crossing: boats and zeppelins are faction-bound", crossSrc, resultIf(got == "", "a crossing was invented")})
	for _, tb := range []string{"zone_connections", "flight_paths", "travel_times"} {
		_, err := ds.Get(tb, 1)
		out = append(out, benchRow{tb + ": table", fmt.Sprint(err != nil), "the dataset has no such table", worldNoneSrc, "differ (data): the dataset holds no zone border, flight path or travel time"})
	}
	// the zones the advisor lists for Outland and the Eastern Kingdoms the dataset does not hold
	var missing []string
	for _, z := range []string{"Hellfire Peninsula", "Zangarmarsh", "Eversong Woods", "Northern Barrens"} {
		if _, ok := zones[z]; !ok && learn.ContinentOf(z) != "" {
			missing = append(missing, z)
		}
	}
	out = append(out, benchRow{"zones the advisor knows and the dataset lacks", fmt.Sprint(missing), "no ui_maps row for them", zoneSrc, "differ (data): ui_maps holds no Outland, Eversong or Northern Barrens zone"})
	finish(t, "advice-zone-no-answer", "Offline zone connections: no answer in the dataset", out)
}

// ---- vendor prices ----

var worldPrices = []struct {
	item int64
	name string
	want int64  // copper a player with no standing pays
	why  string // how the dataset's rows give it
}{
	{6256, "Fishing Pole", 23, "every vendor at the buy price, some at a discount"},
	{2604, "Red Dye", 50, "vendors at the buy price"},
	{2324, "Bleach", 25, "vendors at the buy price"},
	{2321, "Fine Thread", 100, "vendors at the buy price, others at a tenth off"},
	{2325, "Black Dye", 1000, "vendors at the buy price"},
	{2692, "Hot Spices", 40, "vendors at the buy price"},
	{2880, "Weak Flux", 100, "vendors at the buy price"},
	{2901, "Mining Pick", 10, "vendors at the buy price"},
	{1205, "Melon Juice", 500, "reporters saw 450: a tenth off the buy price"},
	{2320, "Coarse Thread", 10, "reporters saw 9: a tenth off the buy price"},
	{1645, "Moonberry Juice", 2000, "reporters saw 1800: a tenth off the buy price"},
	{276200, "Crawler's Clackers", 27547, "the one vendor lists above the buy price: its own price stands"},
}

// TestOfflineBenchmarkVendorPrice: the price the advisor says a vendor charges, beside vendor_sources and the item's buy
// price: a reputation discount seen by reporters is not the price a player with no standing pays, and a price above
// the buy price is kept.
func TestOfflineBenchmarkVendorPriceA(t *testing.T) { worldPriceCases(t, "a", worldPrices[:6]) }
func TestOfflineBenchmarkVendorPriceB(t *testing.T) { worldPriceCases(t, "b", worldPrices[6:]) }

func worldPriceCases(t *testing.T, part string, cases []struct {
	item int64
	name string
	want int64
	why  string
}) {
	ds := worldStore()
	var out []benchRow
	for _, tc := range cases {
		o, ok := vendors.Best(ds, tc.item)
		rows, _ := ds.Filter("vendor_sources", store.Range{Column: "item_id", Min: tc.item, Max: tc.item})
		it, _ := ds.Get("items", tc.item)
		ref := fmt.Sprintf("%s (%d copper); %d vendor rows, buy price %d: %s", vendors.Money(tc.want), tc.want, len(rows), num(it["buy_price"]), tc.why)
		got := o.Price
		out = append(out, benchRow{tc.name + ": price", fmt.Sprintf("%s (%d copper)", got, o.PriceCopper), ref, priceSrc, resultIf(ok && text(it["name"]) == tc.name && o.PriceCopper == tc.want && got == vendors.Money(tc.want) && strings.Contains(o.Line(), " for "+got), "the price")})
	}
	finish(t, "advice-vendor-price-"+part, "Offline vendor price: "+part, out)
}

// TestOfflineBenchmarkVendorPriceOdd: offers whose price is not a plain gold price: a price of nothing is no price, a
// price a fifth of the buy price is the dataset's own disagreement, and the money wording and the cheap line.
func TestOfflineBenchmarkVendorPriceOdd(t *testing.T) {
	ds := worldStore()
	var out []benchRow
	for _, id := range []int64{249479, 285291} {
		it, _ := ds.Get("items", id)
		o, ok := vendors.Best(ds, id)
		out = append(out, benchRow{text(it["name"]) + ": offer listed at a price of 0", fmt.Sprintf("price %q, line %q", o.Price, o.Line()), fmt.Sprintf("no gold price (listed 0 by %d reporters; buy price %d)", 3, num(it["buy_price"])), priceSrc,
			resultIf(ok && o.PriceCopper == 0 && o.Price == "" && !strings.Contains(o.Line(), " for "), worldPriceNil)})
	}
	g, _ := vendors.Best(ds, 252027)
	out = append(out, benchRow{"Gustberry Juice: reporters saw 25 copper, buy price 125", g.Price, "25 copper listed against a buy price of 125: a stack price against a unit price", priceSrc, "differ (data): the dataset's two prices disagree five times over; the advisor states the buy price"})
	for _, tc := range []struct {
		c    int64
		want string
	}{{0, ""}, {7, "7c"}, {100, "1s"}, {1050, "10s 50c"}, {10000, "1g"}, {123456, "12g 34s 56c"}, {27547, "2g 75s 47c"}} {
		got := vendors.Money(tc.c)
		out = append(out, benchRow{fmt.Sprintf("%d copper in words", tc.c), fmt.Sprintf("%q", got), fmt.Sprintf("%q: gold, silver and copper", tc.want), "game money units (100 copper to the silver, 100 silver to the gold)", resultIf(got == tc.want, "the money wording")})
	}
	for _, tc := range []struct {
		price int64
		level int
		cheap bool
	}{{9999, 5, true}, {10001, 5, false}, {20000, 15, true}, {30000, 15, false}, {0, 30, false}} {
		got := (vendors.Offer{PriceCopper: tc.price}).Cheap(tc.level)
		out = append(out, benchRow{fmt.Sprintf("%d copper at level %d: cheap", tc.price, tc.level), fmt.Sprint(got), fmt.Sprint(tc.cheap), "advisor rule: 1 gold per ten levels of the band (docs/upgrade-paths.md); an unknown price is never cheap", resultIf(got == tc.cheap, "the cheap rule")})
	}
	finish(t, "advice-vendor-price-odd", "Offline vendor price: odd offers, money and cheap", out)
}

// TestOfflineBenchmarkVendorPriceNoAnswer: a price the dataset does not hold is not guessed: an item no vendor sells, the
// vendor name the snapshot lacks (the real rows name none), and a price seen by the player comes before the dataset's.
func TestOfflineBenchmarkVendorPriceNoAnswer(t *testing.T) {
	synth := worldStore()
	raw := store.NewDir(worldDir)
	var out []benchRow
	_, ok := vendors.Best(synth, 99999999)
	out = append(out, benchRow{"an item no vendor sells", fmt.Sprint(ok), "no vendor_sources row", worldNoneSrc, resultIf(!ok, "an offer was invented")})
	_, ok = vendors.Best(synth, 0)
	out = append(out, benchRow{"item 0", fmt.Sprint(ok), "no item", worldNoneSrc, resultIf(!ok, "an offer was invented")})
	for _, id := range []int64{6256, 2604, 1205} {
		it, _ := raw.Get("items", id)
		o, ok := vendors.Best(raw, id)
		out = append(out, benchRow{text(it["name"]) + ": vendor of the dataset's rows, unnamed", fmt.Sprintf("offer %v", ok), "vendor_sources names creature ids only; npcs holds no name for them", worldNoneSrc,
			resultIf(!ok && o.Vendor == "", "a vendor was named from an id")})
	}
	out = append(out, benchRow{"the name of the vendors", "none stated", "npcs holds no row for any vendor in vendor_sources", "dataset npcs", "differ (data): no vendor is named, so the advisor offers no vendor of the dataset's rows"})
	lvl := 20
	c := character.Character{Faction: "Horde", Class: "MAGE", Level: &lvl, Seen: &character.Observed{Vendors: []character.SeenVendor{{NPC: 4775, Name: "Synthetic Fisher", Faction: "Horde", Items: []character.SeenStock{{ItemID: 6256, Price: 20}}}}}}
	o, ok := vendors.BestFor(c, synth, 6256)
	out = append(out, benchRow{"Fishing Pole: a price the player saw", fmt.Sprintf("%s %q", o.Vendor, o.Price), "the vendor the player opened, at the price it showed, before the dataset's", priceSrc, resultIf(ok && o.Seen() && o.PriceCopper == 20 && o.Vendor == "Synthetic Fisher", "the seen price")})
	finish(t, "advice-vendor-price-no-answer", "Offline vendor price: no answer in the dataset", out)
}

// ---- recipe levels ----

type worldRecipe struct {
	name, prof string
}

// worldRecipes are recipes of the advice snapshot: a trainer's, an estimate, a recipe item's, both, one whose
// yellow and grey skills are one, and one whose learn skill lies past its grey skill.
var worldRecipes = []worldRecipe{
	{"Spiced Wolf Meat", "Cooking"}, {"Heavy Linen Bandage", "First Aid"}, {"Reinforced Linen Cape", "Tailoring"},
	{"Heavy Copper Broadsword", "Blacksmithing"}, {"Rough Copper Bomb", "Engineering"}, {"Handstitched Leather Boots", "Leatherworking"},
	{"Linen Bandage", "First Aid"}, {"Brilliant Smallfish", "Cooking"}, {"Moonglow Boots", "Leatherworking"}, {"First Aid Kit", "First Aid"},
	{"Coarse Sharpening Stone", "Blacksmithing"}, {"Lesser Healing Potion", "First Aid"},
}

func TestOfflineBenchmarkRecipeLevelA(t *testing.T) { worldRecipeCases(t, "a", worldRecipes[:4]) }
func TestOfflineBenchmarkRecipeLevelB(t *testing.T) { worldRecipeCases(t, "b", worldRecipes[4:8]) }
func TestOfflineBenchmarkRecipeLevelC(t *testing.T) { worldRecipeCases(t, "c", worldRecipes[8:]) }

// worldRecipeCases: for each recipe, the skill it needs (the plan's and the recipe answer's) and the skills at which it
// turns yellow and grey beside the recipes row, and that a plan lists it as orange below the yellow skill, as yellow
// up to the grey skill and never from the grey skill on.
func worldRecipeCases(t *testing.T, part string, cases []worldRecipe) {
	ds := qbStore()
	var out []benchRow
	for _, tc := range cases {
		recs, err := ds.SearchName("recipes", tc.name, 5)
		if err != nil || len(recs) == 0 {
			t.Fatalf("%s: not in the snapshot (%v)", tc.name, err)
		}
		row := recs[0]
		learnSk, yellow, grey, src := int(num(row["learn_skill"])), int(num(row["yellow_skill"])), int(num(row["grey_skill"])), text(row["learn_skill_source"])
		all, err := professions.Recipes(ds, tc.prof)
		if err != nil {
			t.Fatal(err)
		}
		var rc professions.Recipe
		for _, r := range all {
			if r.SpellID == num(row["spell_id"]) {
				rc = r
			}
		}
		if rc.SpellID == 0 {
			t.Fatalf("%s: not read by the advisor", tc.name)
		}
		res, err := learn.Find(character.Character{Faction: "Horde", Class: "MAGE"}, ds, nil, tc.name)
		if err != nil {
			t.Fatal(err)
		}

		// the skill needed: the recipe answer's gate (nothing stated at a minimum of 1 or less), worded as an estimate when it is one
		wantGate := ""
		if learnSk > 1 {
			wantGate = fmt.Sprintf("It needs %s %d.", tc.prof, learnSk)
			if src == "estimate" {
				wantGate = fmt.Sprintf("It needs about %s %d.", tc.prof, learnSk)
			}
		}
		var okGate bool
		if learnSk > 1 {
			okGate = res.Gate.Skill == learnSk && res.Gate.Line == wantGate && strings.Contains(res.Text, wantGate)
		} else {
			okGate = res.Gate.Skill == 0 && !strings.Contains(res.Text, "It needs")
		}
		ref := fmt.Sprintf("learn_skill %d (%s), from the dataset", learnSk, src)
		out = append(out, benchRow{tc.name + ": skill needed to learn it", res.Gate.Line + fmt.Sprintf(" [skill %d]", res.Gate.Skill), ref, recipeLevSrc, resultIf(okGate, "the skill needed or its wording")})

		// the plan's own learn skill: the dataset's, held back to ten under the yellow skill for an estimate
		wantPlan := learnSk
		if src == "estimate" && learnSk > 5 && learnSk > 30 && yellow-10 > learnSk {
			wantPlan = yellow - 10
		}
		okPlan := rc.LearnSkill != nil && *rc.LearnSkill == wantPlan && rc.YellowSkill != nil && *rc.YellowSkill == yellow && rc.GreySkill != nil && *rc.GreySkill == grey
		gotPlan := "no learn skill"
		if rc.LearnSkill != nil {
			gotPlan = fmt.Sprintf("learn %d, yellow %v, grey %v", *rc.LearnSkill, deref(rc.YellowSkill), deref(rc.GreySkill))
		}
		out = append(out, benchRow{tc.name + ": learn, yellow and grey skills in a plan", gotPlan, fmt.Sprintf("learn %d (%s), yellow %d, grey %d", learnSk, src, yellow, grey), recipeLevSrc, resultIf(okPlan, "a threshold read from the row")})

		// the colour at the edges of the thresholds
		at := func(skill int) string {
			for _, s := range professions.Select([]professions.Recipe{rc}, skill) {
				return s.Difficulty
			}
			return "none"
		}
		var got, want []string
		lo := max(1, wantPlan)
		check := func(skill int, expect string) {
			if skill < lo {
				return
			}
			got = append(got, fmt.Sprintf("%d:%s", skill, at(skill)))
			want = append(want, fmt.Sprintf("%d:%s", skill, expect))
		}
		if yellow > grey || learnSk > grey || wantPlan > grey {
			check(grey, "none")
			check(lo, "none")
			if lo > grey {
				got, want = []string{fmt.Sprintf("%d:%s", lo, at(lo))}, []string{fmt.Sprintf("%d:none", lo)}
			}
		} else {
			check(yellow-1, professions.Orange)
			if yellow < grey {
				// yellow up to halfway to grey, green from there: the game's colours, which the advisor shows
				mid := (yellow + grey) / 2
				shade := func(skill int) string {
					if skill >= mid {
						return professions.Green
					}
					return professions.Yellow
				}
				check(yellow, shade(yellow))
				check(mid-1, shade(mid-1))
				check(mid, shade(mid))
				check(grey-1, shade(grey-1))
			}
			check(grey, "none")
		}
		note := resultIf(slices.Equal(got, want), "a recipe is listed past its grey skill or in the wrong colour")
		if learnSk > grey {
			note = "differ (data): the dataset's learn skill lies past its grey skill; the advisor lists it at no skill"
			if !slices.Equal(got, want) {
				note = resultIf(false, "a recipe the player cannot learn before it is grey is listed")
			}
		}
		out = append(out, benchRow{tc.name + ": colour at the thresholds", strings.Join(got, " "), strings.Join(want, " ") + " (orange below yellow, yellow to halfway, green to grey, none from grey)", recipeLevSrc, note})
	}
	finish(t, "advice-recipe-level-"+part, "Offline recipe levels: "+part, out)
}

func deref(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}

// TestOfflineBenchmarkRecipeLevelNoAnswer: recipes with no level the dataset states: a recipe the dataset lacks has no
// level, a trainer recipe whose only value is the client minimum states no number, a recipe item with no requirement
// states none, and a profession it has no recipes for lists none.
func TestOfflineBenchmarkRecipeLevelNoAnswer(t *testing.T) {
	ds := qbStore()
	var out []benchRow
	for _, name := range []string{"Elixir Of The Pale Oak Moon", "Pattern: Synthetic Sunken Cloak"} {
		res, err := learn.Find(character.Character{Faction: "Horde", Class: "MAGE"}, ds, nil, name)
		out = append(out, benchRow{name + ": skill needed", fmt.Sprintf("skill %d, error %v", res.Gate.Skill, err != nil), "no recipes row", worldNoneSrc, resultIf(err != nil && res.Gate.Skill == 0 && res.Gate.Line == "", "a level was invented")})
	}
	for _, name := range []string{"Rough Sharpening Stone", "Bolt of Linen Cloth", "Minor Healing Potion"} {
		res, _ := learn.Find(character.Character{Faction: "Horde", Class: "MAGE"}, ds, nil, name)
		out = append(out, benchRow{name + ": a trainer recipe at the client minimum", res.Gate.Line, "learn_skill 1 on a trainer recipe: no number to state", recipeLevSrc, resultIf(res.Gate.Unknown && res.Gate.Skill == 0 && !strings.Contains(res.Text, "needs"), "a number was stated for a minimum of 1")})
	}
	_, err := professions.Recipes(ds, "Jewelcrafting")
	out = append(out, benchRow{"Jewelcrafting: recipes", fmt.Sprint(err), "no recipes row of that profession in the dataset", worldNoneSrc, resultIf(err != nil, "recipes were invented")})
	row := qbRecipeRow(t, "Coarse Sharpening Stone")
	out = append(out, benchRow{"Coarse Sharpening Stone: thresholds", fmt.Sprintf("learn %d, yellow %d, grey %d", num(row["learn_skill"]), num(row["yellow_skill"]), num(row["grey_skill"])), "learn skill 75 (client) past grey skill 55: the row contradicts itself", recipeLevSrc, "differ (data): learn_skill is past grey_skill, so the recipe is never yellow when it can be learned"})
	finish(t, "advice-recipe-level-no-answer", "Offline recipe levels: no answer in the dataset", out)
}

func qbRecipeRow(t *testing.T, name string) store.Row {
	t.Helper()
	recs, err := qbStore().SearchName("recipes", name, 5)
	if err != nil || len(recs) == 0 {
		t.Fatalf("%s: not in the snapshot (%v)", name, err)
	}
	return recs[0]
}

// ---- world events ----

var worldEventPhrases = []struct{ phrase, key string }{
	{"Feast of Winter Veil", "winter-veil"}, {"winter veil", "winter-veil"}, {"Hallow's End", "hallows-end"}, {"hallows end", "hallows-end"},
	{"Midsummer Fire Festival", "midsummer"}, {"midsummer", "midsummer"}, {"Lunar Festival", "lunar-festival"}, {"Love is in the Air", "love-is-in-the-air"},
	{"noblegarden", "noblegarden"}, {"Children's Week", "childrens-week"}, {"childrens week", "childrens-week"}, {"Brewfest", "brewfest"}, {"Harvest Festival", "harvest-festival"},
}

// TestOfflineBenchmarkEventNames: which event the advisor names for the words a player uses, beside the game's holiday
// names, and the quest sort ids it gives each event beside the game's.
func TestOfflineBenchmarkEventNames(t *testing.T) {
	var out []benchRow
	for _, tc := range worldEventPhrases {
		e, ok := questie.FindEvent(tc.phrase)
		out = append(out, benchRow{fmt.Sprintf("%q: the event named", tc.phrase), e.Key, tc.key, eventSrc, resultIf(ok && e.Key == tc.key, "the wrong event or none")})
	}
	for _, tc := range []struct {
		key  string
		sort int
	}{{"winter-veil", -1001}, {"childrens-week", -1002}, {"hallows-end", -1003}, {"harvest-festival", -1005}, {"brewfest", -370}, {"midsummer", -369}, {"lunar-festival", -366}, {"love-is-in-the-air", -376}, {"noblegarden", -374}} {
		e, _ := questie.EventByKey(tc.key)
		out = append(out, benchRow{tc.key + ": quest sort id", fmt.Sprint(e.Sort), fmt.Sprint(tc.sort), eventSortSrc, resultIf(e.Sort == tc.sort, "the quest sort id")})
	}
	finish(t, "advice-event-names", "Offline world events: names and quest sorts", out)
}

// worldEventWindows are the holiday windows of the game's calendar for the events the earlier recurring-activity
// benchmark does not check, and Winter Veil, which begins on December 16.
var worldEventWindows = []struct {
	key        string
	start, end [2]int
	basis      string
}{
	{"winter-veil", [2]int{12, 16}, [2]int{1, 2}, "Blizzard's Classic holiday post: December 16 to January 2"},
	{"love-is-in-the-air", [2]int{2, 6}, [2]int{2, 20}, "February 6 to 20"},
}

// TestOfflineBenchmarkEventWindows: the days an event runs beside the game's calendar, at the edges of its window.
func TestOfflineBenchmarkEventWindows(t *testing.T) {
	var out []benchRow
	day := func(y, m, d int) time.Time { return time.Date(y, time.Month(m), d, 12, 0, 0, 0, time.UTC) }
	runs := func(tm time.Time, key string) bool {
		for _, e := range questie.RunningEvents(tm) {
			if e.Key == key {
				return true
			}
		}
		return false
	}
	for _, w := range worldEventWindows {
		yr := 2026
		endYr := yr
		if w.end[0] < w.start[0] {
			endYr = yr + 1
		}
		first, last := runs(day(yr, w.start[0], w.start[1]), w.key), runs(day(endYr, w.end[0], w.end[1]), w.key)
		before := runs(day(yr, w.start[0], w.start[1]).AddDate(0, 0, -1), w.key)
		after := runs(day(endYr, w.end[0], w.end[1]).AddDate(0, 0, 1), w.key)
		got := fmt.Sprintf("first day %v, last day %v, day before %v, day after %v", first, last, before, after)
		out = append(out, benchRow{w.key + ": window", got, "true, true, false, false (" + w.basis + ")", eventSrc, resultIf(first && last && !before && !after, "an edge of the window")})
	}
	// the Lunar Festival is placed by the guides in late January and early February
	lunar, _ := questie.EventByKey("lunar-festival")
	out = append(out, benchRow{"lunar-festival: window", fmt.Sprintf("%d/%d to %d/%d", lunar.Start[0], lunar.Start[1], lunar.End[0], lunar.End[1]), "late January to early February", eventSrc,
		resultIf(lunar.Start[0] == 1 && lunar.Start[1] >= 20 && lunar.End[0] == 2 && lunar.End[1] <= 10, "the Lunar Festival is not in late January or early February")})
	// no two days of a year hold a event the table does not list, and opting in is per event
	var set questie.Seasonal
	set = set.With("brewfest", true)
	out = append(out, benchRow{"opting in to Brewfest only", fmt.Sprintf("brewfest %v, midsummer %v", set.Wants("brewfest"), set.Wants("midsummer")), "true, false", "the player's own setting", resultIf(set.Wants("brewfest") && !set.Wants("midsummer"), "the setting")})
	nb, _ := questie.EventByKey("noblegarden")
	out = append(out, benchRow{"noblegarden: window", fmt.Sprintf("%d/%d to %d/%d, fixed each year", nb.Start[0], nb.Start[1], nb.End[0], nb.End[1]), "the week after Easter, which moves between March 22 and April 25", eventSrc, "differ (data): the dataset holds no event date, and the guides move the date with Easter"})
	finish(t, "advice-event-windows", "Offline world events: windows", out)
}

// TestOfflineBenchmarkEventNoAnswer: events the table does not hold are not matched or dated: the advisor says it has no
// such event rather than naming a window.
func TestOfflineBenchmarkEventNoAnswer(t *testing.T) {
	var out []benchRow
	for _, name := range []string{"Darkmoon Faire", "Gurubashi Arena", "Stranglethorn Fishing Extravaganza", "Fireworks Spectacular", "Pilgrim's Bounty"} {
		e, ok := questie.FindEvent(name)
		out = append(out, benchRow{name + ": the event", fmt.Sprintf("%v %q", ok, e.Key), "no event row", worldNoneSrc, resultIf(!ok, "an event was matched")})
	}
	_, ok := questie.FindEvent("")
	out = append(out, benchRow{"an empty question", fmt.Sprint(ok), "no event", worldNoneSrc, resultIf(!ok, "an event was matched")})
	_, ok = questie.EventByKey("synthetic-festival")
	out = append(out, benchRow{"an unknown event key", fmt.Sprint(ok), "no event", worldNoneSrc, resultIf(!ok, "an event was found")})
	finish(t, "advice-event-no-answer", "Offline world events: no answer in the dataset", out)
}

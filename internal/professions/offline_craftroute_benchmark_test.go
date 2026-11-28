package professions

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/store"
)

// The offline craft-route benchmark: the colour the advisor shows a recipe at a skill (the "what do I make next" answer, the route's
// bands and the Forever guides' steps), the gathered materials the advisor sends to a craft (which professions use an item, and which
// node yields it), and what it says to a player who holds two primary professions and weighs a change, beside the dataset's rows, with
// no network. It reads testdata/tradeskills_snapshot (recipes, recipe_reagents, gathering_nodes, gathering_yields, profession_notes).
// make offline-benchmark runs it; WOWC_BENCH_REPORT receives the tables.

const (
	crSrc       = "dataset recipes (learn_skill, yellow_skill, grey_skill); game: orange, yellow, green (halfway from yellow to grey), grey"
	crGatherSrc = "dataset gathering_yields (item_id, node_id), gathering_nodes (name, profession, skill_required, zone_name), recipe_reagents, recipes (skill_line_name)"
	crSlotSrc   = "dataset profession_notes rules (slots), professions (primary or secondary), pairings"
)

// crSample is the benchmark's sample of the snapshot: the recipes learnt at skill 75 or below (the first rank), every other table whole.
// The route and material answers read every recipe of every craft, so the whole table would cost a test over its CPU cap.
type crSample struct{ store.DataStore }

func (c crSample) Filter(table string, r ...store.Range) ([]store.Row, error) {
	rows, err := c.DataStore.Filter(table, r...)
	if table != "recipes" {
		return rows, err
	}
	var out []store.Row
	for _, row := range rows {
		if row["learn_skill"] == nil || asI64(row["learn_skill"]) <= 75 {
			out = append(out, row)
		}
	}
	return out, err
}

var crOnce sync.Once
var crStoreCache store.DataStore

func crStore() store.DataStore {
	crOnce.Do(func() { crStoreCache = crSample{thrStore()} })
	return crStoreCache
}

var crUsersOnce sync.Once
var crUsersCache map[int64][]string

// crUsers is ReagentUsers of the sample, read once for the tests.
func crUsers() map[int64][]string {
	crUsersOnce.Do(func() { crUsersCache, _ = ReagentUsers(crStore()) })
	return crUsersCache
}

// crCase is a recipe at a skill and the colour the game gives it there ("" for grey or not yet learnable).
type crCase struct {
	prof, recipe string
	skill        int
	colour       string
}

var crCases = []crCase{
	{"Blacksmithing", "Copper Bracers", 19, "orange"}, {"Blacksmithing", "Copper Bracers", 20, "yellow"}, {"Blacksmithing", "Copper Bracers", 39, "yellow"}, {"Blacksmithing", "Copper Bracers", 40, "green"},
	{"Blacksmithing", "Rough Copper Vest", 34, "yellow"}, {"Blacksmithing", "Rough Copper Vest", 35, "green"}, {"Blacksmithing", "Rough Copper Vest", 55, ""},
	{"Leatherworking", "Handstitched Leather Boots", 39, "orange"}, {"Leatherworking", "Handstitched Leather Boots", 54, "yellow"}, {"Leatherworking", "Handstitched Leather Boots", 55, "green"}, {"Leatherworking", "Handstitched Leather Boots", 70, ""},
	{"Cooking", "Boiled Clams", 89, "orange"}, {"Cooking", "Boiled Clams", 109, "yellow"}, {"Cooking", "Boiled Clams", 110, "green"}, {"Cooking", "Boiled Clams", 130, ""},
	{"First Aid", "Minor Healing Potion", 54, "orange"}, {"First Aid", "Minor Healing Potion", 74, "yellow"}, {"First Aid", "Minor Healing Potion", 75, "green"},
	{"Alchemy", "Elixir of Minor Fortitude", 49, ""}, {"Alchemy", "Elixir of Minor Fortitude", 50, "orange"}, {"Alchemy", "Elixir of Minor Fortitude", 99, "yellow"}, {"Alchemy", "Elixir of Minor Fortitude", 100, "green"},
	{"Tailoring", "Brown Linen Vest", 56, "yellow"}, {"Tailoring", "Brown Linen Vest", 57, "green"}, {"Tailoring", "Brown Linen Vest", 70, ""},
	{"Blacksmithing", "Bronze Mace", 114, "orange"}, {"Blacksmithing", "Bronze Mace", 129, "yellow"}, {"Blacksmithing", "Bronze Mace", 130, "green"},
	{"Leatherworking", "Cured Light Hide", 59, "yellow"}, {"Leatherworking", "Cured Light Hide", 60, "green"},
	{"First Aid", "Lesser Healing Potion", 104, "yellow"}, {"First Aid", "Lesser Healing Potion", 105, "green"},
}

func TestOfflineBenchmarkCraftColoursA(t *testing.T) { craftColours(t, "a", crCases[:8]) }
func TestOfflineBenchmarkCraftColoursB(t *testing.T) { craftColours(t, "b", crCases[8:16]) }
func TestOfflineBenchmarkCraftColoursC(t *testing.T) { craftColours(t, "c", crCases[16:24]) }
func TestOfflineBenchmarkCraftColoursD(t *testing.T) { craftColours(t, "d", crCases[24:]) }

// craftColours: the colour in the "what do I make next" answer (the step's colour and the rendered word) is the colour the dataset's
// thresholds give: a recipe that gives a skill-up one craft in four is green, never called yellow; a grey recipe is not listed.
func craftColours(t *testing.T, part string, cases []crCase) {
	ds := thrStore()
	var out []benchRow
	for _, tc := range cases {
		row := thrRecipe(t, tc.recipe, tc.prof)
		learn, yellow, grey := int(asI64(row["learn_skill"])), int(asI64(row["yellow_skill"])), int(asI64(row["grey_skill"]))
		ref := refColour(learn, yellow, grey, tc.skill)
		if ref == "grey" {
			ref = ""
		}
		one := thrOne{ds, asI64(row["spell_id"])}
		pl := Plan(routeChar(tc.prof, tc.skill), one, tc.prof)
		got, shown := "", false
		if len(pl.Steps) == 1 {
			got = pl.Steps[0].Difficulty
			shown = strings.Contains(Render([]Leveling{pl}), "["+got+"]")
		}
		ok := ref == tc.colour && got == tc.colour && (tc.colour == "" || shown)
		want := tc.colour
		if want == "" {
			want = "no recipe listed (grey or not yet learnable)"
		}
		out = append(out, benchRow{fmt.Sprintf("%s %s at skill %d (learn %d, yellow %d, green %d, grey %d): colour in the next-recipe answer", tc.prof, tc.recipe, tc.skill, learn, yellow, (yellow+grey)/2, grey),
			ifElse(got == "", "no recipe listed", got), want, crSrc, resultIf(ok, "the colour is not the one the thresholds give")})
	}
	finishOffline(t, "craft-colours-"+part, "Offline craft colour: next-recipe answer ("+part+")", out)
}

func TestOfflineBenchmarkCraftColourInRouteA(t *testing.T) { routeColours(t, "a", "Alchemy", 100, 125) }
func TestOfflineBenchmarkCraftColourInRouteB(t *testing.T) {
	routeColours(t, "b", "First Aid", 70, 100)
}
func TestOfflineBenchmarkCraftColourInRouteC(t *testing.T) { routeColours(t, "c", "Cooking", 40, 70) }

// routeColours: every band of a route is headed with the colour the dataset's thresholds give its recipe at the skill the band starts at.
func routeColours(t *testing.T, part, prof string, from, to int) {
	ds := crStore()
	var out []benchRow
	pa := ComputePath(routeChar(prof, from), ds, prof, to, 0)
	var got, want []string
	for _, b := range pa.Bands {
		if b.Gap != "" {
			continue
		}
		rows, _ := ds.Filter("recipes", store.Range{Column: "spell_id", Min: b.Best.Spell, Max: b.Best.Spell})
		if len(rows) == 0 {
			continue
		}
		c := refColour(0, int(asI64(rows[0]["yellow_skill"])), int(asI64(rows[0]["grey_skill"])), b.From)
		got = append(got, fmt.Sprintf("%d %s", b.From, b.Best.Difficulty))
		want = append(want, fmt.Sprintf("%d %s", b.From, c))
	}
	out = append(out, benchRow{fmt.Sprintf("%s %d→%d: the colour each band is headed with", prof, from, to), strings.Join(got, ", "), strings.Join(want, ", "), crSrc,
		resultIf(len(got) > 0 && slices.Equal(got, want), "a band's colour is not the colour at its start")})
	finishOffline(t, "craft-colours-route-"+part, "Offline craft colour: route bands ("+part+")", out)
}

// TestOfflineBenchmarkCraftColourGuides: the recipe a Forever guide step starts at a skill is, by the dataset's thresholds, orange or
// yellow at that skill (a guide never starts a step on a recipe that is green or grey), and the advisor colours it the same.
func TestOfflineBenchmarkCraftColourGuides(t *testing.T) {
	ds := crStore()
	var out []benchRow
	for _, g := range loadGuides(t).Professions {
		recipes, err := Recipes(ds, g.Profession)
		if err != nil {
			continue
		}
		for _, st := range g.Steps {
			if st.Crafts < 8 || st.From > 70 {
				continue
			}
			var rc *Recipe
			for i := range recipes {
				if recipes[i].Name == st.Recipe {
					rc = &recipes[i]
				}
			}
			label := fmt.Sprintf("%s %d→%d %s: colour at the step's start", g.Profession, st.From, st.To, st.Recipe)
			if rc == nil || rc.YellowSkill == nil || rc.GreySkill == nil {
				out = append(out, benchRow{label, "-", "orange or yellow", g.Source, "differ (data): the recipe or its thresholds are not in the dataset snapshot"})
				continue
			}
			got := colourAt(*rc, st.From)
			ref := refColour(0, *rc.YellowSkill, *rc.GreySkill, st.From)
			res := resultIf((ref == "orange" || ref == "yellow") && got == ref, "the advisor's colour is not the dataset's")
			if got == ref && ref != "orange" && ref != "yellow" {
				res = "differ (data): the dataset's thresholds make the guide's recipe " + ref + " where the guide starts it"
			}
			out = append(out, benchRow{label, colourWord(got), "orange or yellow", g.Source + "; " + crSrc, res})
		}
	}
	finishOffline(t, "craft-colours-guides", "Offline craft colour: Forever guide steps", out)
}

// ---- gathered materials to crafts ----

type gatherCase struct {
	item     string // the gathered item
	item_id  int64
	prof     string // the profession whose path needs it
	from, to int
}

var gatherCases = []gatherCase{
	{"Peacebloom", 2447, "Alchemy", 1, 40}, {"Silverleaf", 765, "Alchemy", 1, 40}, {"Earthroot", 2449, "Alchemy", 100, 125},
	{"Rough Stone", 2835, "Blacksmithing", 1, 20}, {"Copper Ore", 2770, "Mining", 1, 25}, {"Tin Ore", 2771, "Mining", 40, 100},
}

// datasetUsers are the skill lines whose recipes list the item as a reagent, read from the raw tables, left to the crafts a player
// takes the material to (a gathering profession's own line is not one).
func datasetUsers(ref *refData, id int64) []string {
	rows, _ := ref.ds.Filter("recipe_reagents", store.Range{Column: "reagent_item_id", Min: id, Max: id})
	set := map[string]bool{}
	for _, r := range rows {
		sps, _ := ref.ds.Filter("recipes", store.Range{Column: "spell_id", Min: asI64(r["spell_id"]), Max: asI64(r["spell_id"])})
		for _, sp := range sps {
			if l := asText(sp["skill_line_name"]); l != "Herbalism" && l != "Skinning" && l != "Fishing" {
				set[l] = true
			}
		}
	}
	var out []string
	for l := range set {
		out = append(out, l)
	}
	sort.Strings(out)
	return out
}

func TestOfflineBenchmarkGatherToCraftPeacebloom(t *testing.T) { gatherToCraft(t, gatherCases[0]) }
func TestOfflineBenchmarkGatherToCraftSilverleaf(t *testing.T) { gatherToCraft(t, gatherCases[1]) }
func TestOfflineBenchmarkGatherToCraftEarthroot(t *testing.T)  { gatherToCraft(t, gatherCases[2]) }
func TestOfflineBenchmarkGatherToCraftRoughStone(t *testing.T) { gatherToCraft(t, gatherCases[3]) }
func TestOfflineBenchmarkGatherToCraftCopperOre(t *testing.T)  { gatherToCraft(t, gatherCases[4]) }
func TestOfflineBenchmarkGatherToCraftTinOre(t *testing.T)     { gatherToCraft(t, gatherCases[5]) }

// gatherToCraft: a gathered item goes to every craft whose recipes list it, and the route names, for it, the nodes the dataset's
// yields name, with the node's profession, skill and zone, and says which profession to learn when the character lacks it.
func gatherToCraft(t *testing.T, tc gatherCase) {
	ref := &refData{ds: crStore(), recipes: map[string]map[int64]store.Row{}}
	users := crUsers()
	var out []benchRow
	{
		want := datasetUsers(ref, tc.item_id)
		got := append([]string(nil), users[tc.item_id]...)
		sort.Strings(got)
		out = append(out, benchRow{tc.item + ": the crafts that use it", strings.Join(got, ", "), strings.Join(want, ", "), crGatherSrc,
			resultIf(slices.Equal(got, want), "a craft that uses the item is missed, or one that does not is listed")})

		// the route's lines for the item against the dataset's yields
		valid := map[string]bool{}
		yields, _ := ref.ds.Filter("gathering_yields", store.Range{Column: "item_id", Min: tc.item_id, Max: tc.item_id})
		for _, y := range yields {
			nodes, _ := ref.ds.Filter("gathering_nodes", store.Range{Column: "node_id", Min: asI64(y["node_id"]), Max: asI64(y["node_id"])})
			for _, n := range nodes {
				if n["skill_required"] != nil {
					valid[fmt.Sprintf("%s in %s (%s %d)", asText(n["name"]), asText(n["zone_name"]), asText(n["profession"]), asI64(n["skill_required"]))] = true
				}
			}
		}
		pa := ComputePath(routeChar(tc.prof, tc.from), ref.ds, tc.prof, tc.to, 0)
		var lines []string
		bad := ""
		for _, p := range pa.Prep {
			rest, ok := strings.CutPrefix(p, "Gather "+tc.item+": ")
			if !ok {
				continue
			}
			lines = append(lines, rest)
			node := strings.TrimSuffix(strings.TrimSuffix(rest, "."), "; needs "+nodeProfession(rest))
			if !valid[node] {
				bad = rest
			}
		}
		res := resultIf(len(lines) > 0 && len(lines) <= maxNodeLines && bad == "", "the route names a node the dataset does not hold for the item, or none: "+bad)
		out = append(out, benchRow{fmt.Sprintf("%s: where to gather it on the %s route %d→%d", tc.item, tc.prof, tc.from, tc.to), strings.Join(lines, " | "),
			fmt.Sprintf("up to %d of the dataset's %d nodes with a named skill and zone", maxNodeLines, len(valid)), crGatherSrc, res})
	}
	finishOffline(t, "craft-gather-"+strings.ReplaceAll(strings.ToLower(tc.item), " ", "-"), "Offline gathered material to craft: "+tc.item, out)
}

// nodeProfession is the profession in the last "(Profession N)" of a gather line.
func nodeProfession(line string) string {
	i := strings.LastIndex(line, "(")
	if i < 0 {
		return ""
	}
	f := strings.Fields(strings.TrimSuffix(strings.SplitN(line[i+1:], ")", 2)[0], ")"))
	if len(f) == 0 {
		return ""
	}
	return f[0]
}

// ---- dropping or switching a primary profession ----

var switchCases = []struct {
	label string
	char  character.Character
	to    [2]string // the pair the player weighs a change to
}{
	{"Mining and Herbalism held, weighing Mining + Blacksmithing", character.Character{Class: "WARRIOR", Professions: skillsOf("Mining", "Herbalism")}, [2]string{"Mining", "Blacksmithing"}},
	{"Alchemy and Herbalism held, weighing Tailoring + Enchanting", character.Character{Class: "MAGE", Professions: skillsOf("Alchemy", "Herbalism", "Cooking")}, [2]string{"Tailoring", "Enchanting"}},
	{"Skinning and Leatherworking held with Cooking, weighing Herbalism + Alchemy", character.Character{Class: "ROGUE", Professions: skillsOf("Skinning", "Leatherworking", "Cooking", "First Aid")}, [2]string{"Herbalism", "Alchemy"}},
	{"Mining held with Cooking and Fishing, weighing Mining + Blacksmithing", character.Character{Class: "WARRIOR", Professions: skillsOf("Mining", "Cooking", "Fishing")}, [2]string{"Mining", "Blacksmithing"}},
}

// TestOfflineBenchmarkProfessionSwitch: what a change of primary profession keeps and drops, read from the dataset's slot rule (two
// primary professions; Cooking, Fishing and First Aid are secondary and do not count): a pair that shares a held profession keeps it
// and drops only the other held one; a pair that shares none drops both; a secondary profession is never dropped; and with a slot
// free nothing is dropped.
func TestOfflineBenchmarkProfessionSwitch(t *testing.T) {
	ds := thrStore()
	n := LoadNotes(ds)
	var out []benchRow
	for _, tc := range switchCases {
		held := heldPrimaries(tc.char, n)
		var kept, dropped []string
		for _, h := range held {
			if strings.EqualFold(h, tc.to[0]) || strings.EqualFold(h, tc.to[1]) {
				kept = append(kept, h)
			} else {
				dropped = append(dropped, h)
			}
		}
		full := len(held) >= 2
		var line string
		for _, a := range Pairings(tc.char, ds) {
			if a.A == tc.to[0] && a.B == tc.to[1] || a.A == tc.to[1] && a.B == tc.to[0] {
				line = a.Reason
			}
		}
		ok := line != ""
		switch {
		case !full:
			ok = ok && !strings.Contains(line, "dropping")
		case len(kept) == 1:
			ok = ok && strings.Contains(line, "keeps "+kept[0]) && strings.Contains(line, "means dropping "+dropped[0]+" for ") && !strings.Contains(line, "dropping both")
		case len(kept) == 0:
			ok = ok && strings.Contains(line, "dropping both "+held[0]+" and "+held[1]) && !strings.Contains(line, "keeps")
		}
		for _, s := range []string{"Cooking", "Fishing", "First Aid"} {
			ok = ok && !strings.Contains(line, "dropping "+s) && !strings.Contains(line, "dropping both "+s)
		}
		want := fmt.Sprintf("keeps %s; drops %s", ifElse(len(kept) > 0, strings.Join(kept, ", "), "none"), ifElse(full && len(dropped) > 0, strings.Join(dropped, ", "), "none"))
		out = append(out, benchRow{tc.label + ": what the change keeps and drops", strings.TrimSpace(line), want + " (secondary professions do not count against two primary)", crSlotSrc,
			resultIf(ok, "the answer names a profession as dropped that the pair keeps, drops one that is not held, or drops with a slot free")})
	}
	finishOffline(t, "craft-switch", "Offline profession drop and switch", out)
}

// ---- no answer ----

// TestOfflineBenchmarkCraftNoAnswer: what the dataset cannot answer is said, not guessed.
func TestOfflineBenchmarkCraftNoAnswer(t *testing.T) {
	ds := crStore()
	var out []benchRow

	// what dropping a profession costs: the dataset holds no skill lost and no recipes forgotten
	full := Pairings(character.Character{Class: "WARRIOR", Professions: skillsOf("Mining", "Herbalism")}, ds)
	claims := false
	for _, a := range full {
		l := strings.ToLower(a.Reason)
		claims = claims || strings.Contains(l, "you lose") || strings.Contains(l, "you keep your skill") || strings.Contains(l, "gold") || strings.Contains(l, "refund")
	}
	says := len(full) > 0 && strings.Contains(full[0].Reason, "does not say what dropping a profession costs")
	out = append(out, benchRow{"the cost of dropping a profession (skill kept or lost, recipes forgotten)", ifElse(says, "says the game data does not state it", "silent or a claim"), "the dataset has no column for it: say so, claim nothing", crSlotSrc,
		resultIf(says && !claims, "the answer claims a loss or a refund, or does not say it is unknown")})

	// Skinning: the dataset lists no skinning source
	sk := ComputePath(routeChar("Skinning", 20), ds, "Skinning", 100, 0)
	var skText []string
	for _, b := range sk.Bands {
		skText = append(skText, b.Gap)
	}
	rows, _ := ds.Filter("gathering_nodes", store.Range{Column: "profession", Min: "Skinning", Max: "Skinning"})
	out = append(out, benchRow{"Skinning 20→100: where to skin", strings.Join(skText, " "), fmt.Sprintf("%d Skinning rows in gathering_nodes: no source is named", len(rows)), crGatherSrc,
		resultIf(len(rows) == 0 && len(skText) > 0 && strings.Contains(skText[0], "dataset lists no skinning sources"), "a skinning source was named, or the gap was not stated")})

	// Mithril Ore: the dataset's only node for it has no name or skill the player can use
	pa := ComputePath(routeChar("Mining", 150), ds, "Mining", 200, 0)
	var mith []string
	for _, p := range pa.Prep {
		if strings.Contains(p, "Mithril Ore") {
			mith = append(mith, p)
		}
	}
	yields, _ := ds.Filter("gathering_yields", store.Range{Column: "item_id", Min: int64(3858), Max: int64(3858)})
	named := 0
	for _, y := range yields {
		nodes, _ := ds.Filter("gathering_nodes", store.Range{Column: "node_id", Min: asI64(y["node_id"]), Max: asI64(y["node_id"])})
		for _, n := range nodes {
			if n["skill_required"] != nil {
				named++
			}
		}
	}
	out = append(out, benchRow{"Mining 150→200: where to dig Mithril Ore", ifElse(len(mith) == 0, "no gather line", strings.Join(mith, " | ")), fmt.Sprintf("%d nodes with a name and skill for Mithril Ore: no node is named", named), crGatherSrc,
		resultIf(named == 0 && len(mith) == 0, "a node was named for Mithril Ore, which the dataset holds no usable node for")})

	// Herbalism above the top node
	hb := ComputePath(routeChar("Herbalism", 100), ds, "Herbalism", 150, 0)
	text := ""
	if len(hb.Bands) > 0 {
		text = hb.Bands[0].Gap
	}
	out = append(out, benchRow{"Herbalism 100→150: which herb to pick", text, "the dataset's highest herb node needs skill 70: none is named for 100", crGatherSrc,
		resultIf(strings.Contains(text, "The dataset lists no node above skill 70 yet"), "a herb was named above the dataset's nodes")})
	finishOffline(t, "craft-no-answer", "Offline craft route, no answer in the dataset", out)
}

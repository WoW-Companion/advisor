package paths_test

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/ask"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/dungeons"
	"github.com/WoW-Companion/advisor/internal/gear"
	"github.com/WoW-Companion/advisor/internal/items"
	"github.com/WoW-Companion/advisor/internal/paths"
	"github.com/WoW-Companion/advisor/internal/store"
)

// The offline PvP benchmark: what the advisor puts in front of the model, and answers, about a battleground's entry level and
// bracket, an honor vendor or reward, a PvP rank piece or title, and a PvP question the dataset cannot answer, beside the
// dataset's rows and the game's own rules, with no network. It reads testdata/usability_snapshot (rank gear),
// testdata/consumables_snapshot (battleground rations, bandages, standards, Marks of Honor and the test items) and
// testdata/levelling_snapshot (the dungeon list). The dataset holds no battleground, honor, rank, title or PvP vendor row, so those
// cases check that the advisor says so and states none; a difference only the dataset's gap causes is marked "(data)".
// make offline-benchmark runs it; WOWC_BENCH_REPORT receives the tables.

const (
	pvBGSrc     = "dataset instances (types dungeon and raid only: no battleground row); game: Warcraft Wiki battlegrounds (Warsong Gulch 10+, Arathi Basin 20+, Alterac Valley 51+)"
	pvRationSrc = "dataset items (required_level) of the battleground rations and bandages; game: Wowhead Classic, the 25/35/45 ration and bandage ladder"
	pvHonorSrc  = "dataset items, loot_sources, vendor_sources (no honor cost, rank, title or PvP vendor row); game honor system (public)"
	pvRankSrc   = "dataset items (name, required_level, allowable_class_mask: no faction or race column); game: Alliance ranks Private to Grand Marshal, Horde ranks Scout to High Warlord"
	pvTestSrc   = "dataset items (name only): the client's PVP TEST placeholder items; game: no player can obtain them"
	pvNoneSrc   = "the dataset has no honor, rank, title, battleground bracket or PvP vendor row"
)

var pvDigit = regexp.MustCompile(`\d`)

// ---- battleground entry level and bracket ----

var pvBGCases = []struct{ bg, question string }{
	{"Warsong Gulch", "what level do I need to join Warsong Gulch?"}, {"Arathi Basin", "which Arathi Basin bracket am I in at level 29?"},
	{"Alterac Valley", "can my level 45 join Alterac Valley?"}, {"Warsong Gulch", "how many players are on a team in Warsong Gulch?"},
	{"Arathi Basin", "when can I queue for Arathi Basin?"}, {"Alterac Valley", "which bracket is the Alterac Valley level range?"},
}

// pvBGGame is the lowest level the game lets a player queue for the battleground.
var pvBGGame = map[string]int{"Warsong Gulch": 10, "Arathi Basin": 20, "Alterac Valley": 51}

// TestOfflineBenchmarkPvPBattlegroundEntry: a question about a battleground's entry level, bracket, team size or queue gets the fact
// that the data holds none (with no figure in it), is never taken for a dungeon or planned as one, and the dataset's own gap is listed.
func TestOfflineBenchmarkPvPBattlegroundEntry(t *testing.T) {
	ds := levStore()
	var out []benchRow
	for _, tc := range pvBGCases {
		f := ask.LimitFacts(tc.question)
		cl := ask.Classify(tc.question, ds, nil)
		p, _, err := dungeons.Named(levChar("WARRIOR", 45), ds, nil, tc.bg)
		ok := strings.Contains(f, "Battlegrounds: the game data holds no battleground row") && strings.Contains(f, "Say so") && !pvDigit.MatchString(f) &&
			cl.Topic != ask.TopicDungeon && p == nil && err == nil
		out = append(out, benchRow{"\"" + tc.question + "\"", firstLine(f), fmt.Sprintf("no instances row for %s: the data holds none, say so, name no level", tc.bg), pvBGSrc,
			resultIf(ok, "no statement that the data holds none, a figure in it, or a dungeon plan for a battleground")})
	}
	for _, bg := range []string{"Warsong Gulch", "Arathi Basin", "Alterac Valley"} {
		rows, _ := ds.Filter("instances", rangeName(bg))
		res := fmt.Sprintf("differ (data): the dataset has no row, the game says from level %d", pvBGGame[bg])
		if len(rows) > 0 {
			res = "differ: an instances row exists for a battleground"
		}
		out = append(out, benchRow{bg + ": entry level", fmt.Sprintf("says the data holds none (%d instances rows)", len(rows)), fmt.Sprintf("game: from level %d", pvBGGame[bg]), pvBGSrc, res})
	}
	finish(t, "advice-pvp-battleground-entry", "Offline PvP: battleground entry level and bracket", out)
}

func rangeName(name string) store.Range { return store.Range{Column: "name", Min: name, Max: name} }

func rangeInt(col string, id int64) store.Range { return store.Range{Column: col, Min: id, Max: id} }

// gearOffered is true when the gear search offers the item to the character.
func gearOffered(c character.Character, ds store.DataStore, id int64) (bool, error) {
	res, err := gear.Find(c, ds, "", gear.Options{NoSources: true})
	_, ok := found(res, id)
	return ok, err
}

// ---- battleground consumables ----

var pvRationCases = []struct {
	item  string
	level int64 // the level the game gives the item
}{
	{"Warsong Gulch Field Ration", 25}, {"Warsong Gulch Iron Ration", 35}, {"Warsong Gulch Enriched Ration", 45},
	{"Arathi Basin Silk Bandage", 25}, {"Arathi Basin Mageweave Bandage", 35}, {"Arathi Basin Runecloth Bandage", 45},
	{"Defiler's Field Ration", 25}, {"Highlander's Enriched Ration", 45},
}

// TestOfflineBenchmarkPvPRationLevels: the level a battleground ration or bandage needs, as the tooltip facts word it, beside the dataset's required level
// and the game's 25, 35 and 45 ladder.
func TestOfflineBenchmarkPvPRationLevels(t *testing.T) {
	ds := consStore()
	var out []benchRow
	for _, tc := range pvRationCases {
		rl := num(rowByName(t, ds, "items", tc.item)["required_level"])
		f := ask.ItemTooltipFacts(ds, "what level do I need to use ["+tc.item+"]?")
		want := fmt.Sprintf("requires level %d;", rl)
		res := resultIf(strings.Contains(f, want), "the tooltip facts do not give the dataset's required level")
		if res == "match" && rl != tc.level {
			res = fmt.Sprintf("differ (data): the dataset gives %d, the game %d", rl, tc.level)
		}
		out = append(out, benchRow{tc.item + ": the level it needs", firstLine(strings.TrimPrefix(f, "Item tooltips (from the item rows; answer from these, not from memory):\n")), fmt.Sprintf("dataset required_level %d; game %d", rl, tc.level), pvRationSrc, res})
	}
	finish(t, "advice-pvp-ration-levels", "Offline PvP: the level a battleground ration or bandage needs", out)
}

// ---- honor vendors and rewards ----

var pvHonorQuestions = []string{
	"what does the honor vendor sell?", "how many honor points do I need for rank 10?", "how much honor does a kill give?",
	"what is the weekly honor decay?", "which PvP vendor sells the rank 8 gear and what does it cost?", "how many Marks of Honor do I need for the Arathi Basin reward?",
}

// pvHonorItems are battleground rewards and standards of the dataset the game gives for honor, marks or a battleground win.
var pvHonorItems = []string{
	"Alterac Valley Mark of Honor", "Stormpike Battle Standard", "Frostwolf Battle Standard", "Horn of the Frostwolf Howler",
	"Defiler's Basic Care Package", "Highlander's Field Ration",
}

// TestOfflineBenchmarkPvPHonorVendors: a question about an honor vendor, an honor cost or a rank requirement gets the fact that the data holds none, with no figure; an
// honor reward item the dataset lists no source for is not called a random drop, and no honor cost, rank or vendor is stated for it.
func TestOfflineBenchmarkPvPHonorVendors(t *testing.T) {
	ds := consStore()
	var out []benchRow
	for _, q := range pvHonorQuestions {
		f := ask.LimitFacts(q)
		ok := strings.Contains(f, "Honor and PvP ranks: the game data holds no honor points") && strings.Contains(f, "Say so") && !pvDigit.MatchString(f)
		out = append(out, benchRow{"\"" + q + "\"", firstLine(f), "no honor, rank or PvP vendor row: say so, name no cost or rank", pvHonorSrc, resultIf(ok, "no statement that the data holds none, or a figure in it")})
	}
	for _, name := range pvHonorItems {
		r := rowByName(t, ds, "items", name)
		id := num(r["id"])
		v, _ := ds.Filter("vendor_sources", rangeInt("item_id", id))
		l, _ := ds.Filter("loot_sources", rangeInt("item_id", id))
		res := paths.For(usabChar("WARRIOR", 60, "Horde"), ds, id, paths.Options{})
		txt := strings.ReplaceAll(res.Reason+" "+res.Text(), name, "")
		ok := len(v)+len(l) == 0 && !res.Dependable() && strings.Contains(res.Reason, "lists no source") && !strings.Contains(txt, "worth chasing") && !honorWords.MatchString(txt)
		out = append(out, benchRow{name + ": where to get it", res.Reason, fmt.Sprintf("dataset: %d vendor rows, %d loot rows; game: a battleground reward", len(v), len(l)), pvHonorSrc,
			resultIf(ok, "a source, an honor cost or rank, or a verdict was stated")})
	}
	finish(t, "advice-pvp-honor-vendors", "Offline PvP: honor vendors and rewards", out)
}

// ---- PvP rank gear and titles ----

var pvFactionCases = []struct {
	class, faction, item string
}{
	{"HUNTER", "Horde", "Knight-Captain's Chain Girdle"}, {"HUNTER", "Alliance", "Knight-Captain's Chain Girdle"},
	{"HUNTER", "Horde", "First Sergeant's Mail Wristguards"}, {"HUNTER", "Alliance", "First Sergeant's Mail Wristguards"},
	{"WARRIOR", "Horde", "Sergeant Major's Plate Wristguards"}, {"WARRIOR", "Alliance", "Sergeant Major's Plate Wristguards"},
	{"SHAMAN", "Horde", "Legionnaire's Mail Cinch"}, {"SHAMAN", "Alliance", "Blood Guard's Mail Grips"},
	{"DRUID", "Horde", "Knight-Lieutenant's Dragonhide Footwraps"}, {"DRUID", "Alliance", "Knight-Lieutenant's Dragonhide Footwraps"},
}

// pvOwn is the faction whose rank ladder the item belongs to, by the game's rank names.
func pvOwn(item string) string {
	if strings.HasPrefix(item, "Knight-") || strings.HasPrefix(item, "Sergeant Major's") {
		return "Alliance"
	}
	return "Horde"
}

// TestOfflineBenchmarkPvPRankGearFaction: PvP rank gear is one faction's: a Horde character is not offered the Alliance rank pieces and an Alliance one is not offered
// the Horde pieces, in the gear search and in the item lookup; the dataset's own class mask still applies.
func TestOfflineBenchmarkPvPRankGearFaction(t *testing.T) {
	ds := usabStore()
	var out []benchRow
	for _, tc := range pvFactionCases {
		r := usabItem(t, tc.item)
		id := num(r["id"])
		res := paths.For(usabChar(tc.class, 60, tc.faction), ds, id, paths.Options{})
		own := pvOwn(tc.item) == tc.faction
		if own != !res.Unfit {
			out = append(out, benchRow{fmt.Sprintf("%s %s: %s (item lookup)", tc.faction, strings.ToLower(tc.class), tc.item), fmt.Sprintf("unfit %v", res.Unfit), fmt.Sprintf("a %s rank piece: unfit %v", pvOwn(tc.item), !own), pvRankSrc, "differ: the lookup treats the piece as the wrong faction's or the right one's"})
			continue
		}
		g, err := gearOffered(usabChar(tc.class, 60, tc.faction), ds, id)
		if err != nil {
			t.Fatal(err)
		}
		want := own && maskAllows(num(r["allowable_class_mask"]), classID(tc.class))
		ref := fmt.Sprintf("a %s rank piece, class mask %d: offered %v", pvOwn(tc.item), num(r["allowable_class_mask"]), want)
		out = append(out, benchRow{fmt.Sprintf("%s %s: %s", tc.faction, strings.ToLower(tc.class), tc.item), fmt.Sprintf("offered %v, lookup unfit %v", g, res.Unfit), ref, pvRankSrc, resultIf(g == want, "a rank piece offered to the other faction, or one of its own withheld")})
	}
	for _, tc := range []struct{ name, want string }{
		{"Knight-Captain's Chain Girdle", "Alliance"}, {"Blood Guard's Mail Grips", "Horde"}, {"Sergeant's Cloak", ""}, {"Knightly Longsword", ""}, {"Mantle of Honor", ""},
	} {
		got := items.RankFaction(tc.name)
		out = append(out, benchRow{tc.name + ": the faction its rank name belongs to", fmt.Sprintf("%q", got), fmt.Sprintf("%q (no faction when the name does not settle it)", tc.want), pvRankSrc, resultIf(got == tc.want, "the wrong faction, or one for an ordinary item")})
	}
	for _, f := range []string{"Horde", "Alliance"} {
		r := usabItem(t, "Knightly Longsword")
		g, err := gearOffered(usabChar("WARRIOR", 60, f), ds, num(r["id"]))
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, benchRow{f + " warrior: Knightly Longsword (not rank gear)", fmt.Sprintf("offered %v", g), "an ordinary item: offered to both factions", pvRankSrc, resultIf(g, "an ordinary item withheld for its name")})
	}
	finish(t, "advice-pvp-rank-gear-faction", "Offline PvP: rank gear is one faction's", out)
}

// TestOfflineBenchmarkPvPTitles: a question about a PvP title or rank gets the fact that the data holds no rank or title, with no figure; a rank piece is never given a rank number.
func TestOfflineBenchmarkPvPTitles(t *testing.T) {
	var out []benchRow
	for _, q := range []string{"what do I need to become Grand Marshal?", "how long does it take to reach High Warlord?", "which rank gives the Knight-Captain title?", "what is the best PvP gear for my class?"} {
		f := ask.LimitFacts(q)
		out = append(out, benchRow{"\"" + q + "\"", firstLine(f), "no rank, title or honor requirement row: say so, name none", pvHonorSrc,
			resultIf(strings.Contains(f, "Honor and PvP ranks:") && strings.Contains(f, "Say so") && !pvDigit.MatchString(f), "no statement that the data holds none, or a figure in it")})
	}
	for _, name := range []string{"Knight-Captain's Chain Girdle", "Legionnaire's Mail Cinch"} {
		r := usabItem(t, name)
		f := ask.ItemTooltipFacts(usabStore(), "what level does ["+name+"] need?")
		rl := num(r["required_level"])
		ok := strings.Contains(f, fmt.Sprintf("requires level %d;", rl)) && !honorWords.MatchString(strings.ReplaceAll(f, name, ""))
		out = append(out, benchRow{name + ": the tooltip facts", firstLine(strings.TrimPrefix(f, "Item tooltips (from the item rows; answer from these, not from memory):\n")), fmt.Sprintf("required_level %d, no rank stated", rl), pvRankSrc, resultIf(ok, "a rank or honor cost was stated, or the level is wrong")})
	}
	finish(t, "advice-pvp-titles", "Offline PvP: titles and rank pieces", out)
}

// ---- placeholder PvP items and questions with no PvP in them ----

// TestOfflineBenchmarkPvPTestItems: the client's PVP TEST placeholder items are no item a player can get: the lookup says so, and an ordinary item with a
// rank-like name is not taken for one.
func TestOfflineBenchmarkPvPTestItems(t *testing.T) {
	ds := consStore()
	var out []benchRow
	for _, name := range []string{"PVP TEST Alliance Ear", "PVP TEST Horde Ear"} {
		res := paths.For(usabChar("WARRIOR", 60, "Horde"), ds, num(rowByName(t, ds, "items", name)["id"]), paths.Options{})
		out = append(out, benchRow{name + ": where to get it", res.Reason, "a client test item: not obtainable", pvTestSrc, resultIf(res.Reason == "this item can't be obtained in game", "a test item was treated as a real item")})
	}
	for _, name := range []string{"Marshal Haggard's Badge", "Alliance Battle Standard"} {
		res := paths.For(usabChar("WARRIOR", 60, "Horde"), ds, num(rowByName(t, ds, "items", name)["id"]), paths.Options{})
		out = append(out, benchRow{name + ": where to get it", res.Reason, "a real item: not called unobtainable", pvTestSrc, resultIf(res.Reason != "this item can't be obtained in game" && !res.Unfit, "a real item was called unobtainable or unfit")})
	}
	finish(t, "advice-pvp-test-items", "Offline PvP: placeholder items", out)
}

// TestOfflineBenchmarkPvPQuietQuestions: a question that only shares a word with PvP gets no PvP facts.
func TestOfflineBenchmarkPvPQuietQuestions(t *testing.T) {
	var out []benchRow
	for _, q := range []string{"where is the weapon master in the Valley of Honor?", "how do I get to the Valley of Honor?", "what does rank 3 Frostbolt cost?", "I am honored with Orgrimmar, what next?", "which quest should I do next?"} {
		f := ask.LimitFacts(q)
		out = append(out, benchRow{"\"" + q + "\"", f, "no PvP facts on a question about something else", pvNoneSrc, resultIf(f == "", "PvP facts on a question about something else")})
	}
	finish(t, "advice-pvp-quiet", "Offline PvP: questions that are not about PvP", out)
}

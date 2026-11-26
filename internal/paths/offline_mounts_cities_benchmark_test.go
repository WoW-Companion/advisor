package paths_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/farm"
	"github.com/WoW-Companion/advisor/internal/learn"
	"github.com/WoW-Companion/advisor/internal/levelup"
	"github.com/WoW-Companion/advisor/internal/paths"
	"github.com/WoW-Companion/advisor/internal/questie"
	"github.com/WoW-Companion/advisor/internal/quests"
	"github.com/WoW-Companion/advisor/internal/route"
	"github.com/WoW-Companion/advisor/internal/session"
	"github.com/WoW-Companion/advisor/internal/store"
)

// The offline mount, riding-skill, capital-city and starting-zone benchmark: how the advisor says to get a mount, where to
// learn riding, where a class is trained and which city errands a capital offers, and what it says of a race's starting zone
// and its first quests, beside the dataset's rows and the guides, with no network. It reads the snapshots that already hold
// the rows: testdata/consumables_snapshot (every mount item with its vendor and loot rows), testdata/world_snapshot (ui_maps)
// and testdata/places_snapshot (quests_observed). The dataset holds no mount source, no riding skill, trainer or cost, no
// class trainer, bank, auction house or innkeeper row, and no first quest of the Alliance's starting zones, so the cases it
// cannot answer check that the advisor says so and names none. The class trainer cities are those of the Warcraft Wiki's
// class trainer pages. A difference the dataset's own rows cause is marked "(data)".
// make offline-benchmark runs it; WOWC_BENCH_REPORT receives the tables.

const (
	mcMountSrc  = "dataset items (class 15, subclass 5), vendor_sources, loot_sources (no row for any mount)"
	mcRideSrc   = "Warcraft Wiki, Riding: Apprentice Riding at level 40 (60% ground mount), Journeyman Riding at level 60 (100%); dataset items.required_level of the mounts; the dataset has no riding skill, trainer or cost row"
	mcClassSrc  = "Warcraft Wiki class trainer pages (Warrior, Mage, Priest, Rogue, Warlock, Hunter, Shaman trainers); the dataset has no class trainer row"
	mcErrandSrc = "game capitals; the dataset ui_maps names the six Classic capitals and no bank, auction house or innkeeper row"
	mcStartSrc  = "dataset ui_maps (parent continent), quests_observed (title, level, faction, category); Wowhead Classic starting quests"
	mcNoneSrc   = "the dataset has no such row, column or table"
)

// ---- mount sources ----

var mcMounts = []struct{ name, query string }{
	{"Swift Palomino", "swift palomino"}, {"Black War Steed Bridle", "Black War Steed Bridle"}, {"Brown Horse Bridle", "brown horse bridle"},
	{"Whistle of the Ivory Raptor", "Whistle of the Ivory Raptor"}, {"Black Battlestrider", "Black Battlestrider"}, {"Swift Zulian Tiger", "swift zulian tiger"},
	{"Blue Qiraji Resonating Crystal", "Blue Qiraji Resonating Crystal"}, {"Reins of the Winterspring Frostsaber", "Winterspring Frostsaber"},
}

// TestOfflineBenchmarkMountSourceByName: a mount asked for by the name a player types (any case, a part of the name) is the dataset's
// item, and for either faction the answer is that the data lists no source, never a vendor, a drop or a price.
func TestOfflineBenchmarkMountSourceByName(t *testing.T) {
	ds := consStore()
	var out []benchRow
	for _, m := range mcMounts {
		row := rowByName(t, ds, "items", m.name)
		id := num(row["id"])
		v, _ := ds.Filter("vendor_sources", store.Range{Column: "item_id", Min: id, Max: id})
		l, _ := ds.Filter("loot_sources", store.Range{Column: "item_id", Min: id, Max: id})
		got, name, cands := farm.Resolve(ds, m.query)
		ok := got == id && len(cands) == 0
		for _, c := range []character.Character{horde(60), allianceAt(60)} {
			res := paths.For(c, ds, id, paths.Options{})
			ok = ok && !res.Dependable() && len(res.Paths) == 0 && strings.Contains(res.Reason, "lists no source for this mount") && !numbered.MatchString(res.Text())
		}
		out = append(out, benchRow{fmt.Sprintf("%q: the mount and how to get it", m.query), fmt.Sprintf("item %d %s; %s", got, name, paths.For(horde(60), ds, id, paths.Options{}).Reason),
			fmt.Sprintf("item %d; %d vendor rows, %d loot rows", id, len(v), len(l)), mcMountSrc, resultIf(ok, "another item, or a source, price or verdict the dataset does not give")})
	}
	finish(t, "advice-mount-by-name", "Offline mounts: asked for by name, no source is invented", out)
}

func allianceAt(level int) character.Character {
	return character.Character{Faction: "Alliance", Class: "MAGE", Level: &level}
}

// TestOfflineBenchmarkMountAmbiguous: a part of a name that fits many mounts lists them and picks none.
func TestOfflineBenchmarkMountAmbiguous(t *testing.T) {
	ds := consStore()
	var out []benchRow
	for _, q := range []string{"Mechanostrider", "Skeletal Horse", "Galestrider", "Kodo"} {
		id, _, cands := farm.Resolve(ds, q)
		out = append(out, benchRow{fmt.Sprintf("%q: which mount", q), fmt.Sprintf("item %d, %d candidates", id, len(cands)), "several items carry the name: no single answer", mcMountSrc, resultIf(id == 0 && len(cands) > 1, "one mount was picked")})
	}
	finish(t, "advice-mount-ambiguous", "Offline mounts: a name that fits several lists them", out)
}

// ---- riding skill ----

// TestOfflineBenchmarkRidingSkillWhere: where to learn riding is not answered from a recipe: the dataset has no riding trainer, level or cost, and the advisor
// says so for every way of asking, and still finds the real Enchant Gloves - Riding Skill recipe by its name.
func TestOfflineBenchmarkRidingSkillWhere(t *testing.T) {
	ds := consStore()
	var out []benchRow
	for _, q := range []string{"Riding", "riding skill", "Apprentice Riding", "Journeyman Riding", "Expert Riding", "learn riding"} {
		res, err := learn.Find(horde(60), ds, nil, q)
		ok := err == nil && len(res.Ways) == 0 && strings.Contains(res.Text, "lists no riding trainer, level or cost") && !strings.Contains(res.Text, "Enchanting") && !numbered.MatchString(res.Text)
		out = append(out, benchRow{fmt.Sprintf("%q: where to learn it", q), firstLine(res.Text), "no riding trainer, skill or cost row in the dataset (the game's riding trainers are not in it)", mcRideSrc, resultIf(ok, "an enchant recipe, a trainer or a number was given for the riding skill")})
	}
	res, err := learn.Find(horde(60), ds, nil, "Enchant Gloves - Riding Skill")
	rec, _ := ds.Filter("recipes", store.Range{Column: "name", Min: "Enchant Gloves - Riding Skill", Max: "Enchant Gloves - Riding Skill"})
	need := int64(0)
	if len(rec) > 0 {
		need = num(rec[0]["learn_skill"])
	}
	out = append(out, benchRow{"Enchant Gloves - Riding Skill: the recipe", fmt.Sprintf("%s; %v", strings.ReplaceAll(res.Text, "\n", " "), err), fmt.Sprintf("recipes row, learn_skill %d", need), mcRideSrc,
		resultIf(err == nil && res.Recipe == "Enchant Gloves - Riding Skill" && strings.Contains(res.Text, "Enchanting 250") && need == 250, "the enchant is not found by its name")})
	finish(t, "advice-riding-where", "Offline riding skill: no trainer or cost in the dataset", out)
}

// TestOfflineBenchmarkRidingSkillLevel: the level a mount is assumed from beside the mounts' required levels, and the 100% mount the
// advisor does not assume for a level 60 character (the capture holds no riding skill and no mount).
func TestOfflineBenchmarkRidingSkillLevel(t *testing.T) {
	ds := consStore()
	var out []benchRow
	for _, tc := range []struct {
		name string
		lv   int64
	}{{"Brown Horse Bridle", 40}, {"Gray Kodo", 40}, {"Red Mechanostrider", 40}, {"Swift Palomino", 60}, {"Great White Kodo", 60}, {"Swift Zulian Tiger", 60}} {
		rl := num(rowByName(t, ds, "items", tc.name)["required_level"])
		out = append(out, benchRow{tc.name + ": the level to ride it", fmt.Sprintf("a mount is assumed from level %d", session.MountLevel), fmt.Sprintf("required_level %d: Apprentice Riding is level 40 (60%%), Journeyman level 60 (100%%)", rl), mcRideSrc,
			resultIf(rl == tc.lv && (rl == 40) == (rl <= int64(session.MountLevel)), "another required level than the riding tiers")})
	}
	out = append(out, benchRow{"a level 60 character: mounted speed", fmt.Sprintf("%.1f yards a second (the 60%% mount)", session.MountSpeed), "14 yards a second with the 100% mount", mcRideSrc,
		"differ (data): the capture holds no riding skill or mount, so the 100% mount is not assumed; the estimate is the slower one"})
	finish(t, "advice-riding-level", "Offline riding skill: the levels of the tiers", out)
}

// ---- capital cities ----

// mcTrainers are the capitals whose class trainers the Warcraft Wiki lists, by side and class.
var mcTrainers = map[string]map[string][]string{
	"Horde": {"WARRIOR": {"Orgrimmar", "Undercity", "Thunder Bluff"}, "MAGE": {"Orgrimmar", "Undercity", "Thunder Bluff"}, "PRIEST": {"Orgrimmar", "Undercity", "Thunder Bluff"},
		"ROGUE": {"Orgrimmar", "Undercity"}, "WARLOCK": {"Orgrimmar", "Undercity", "Thunder Bluff"}, "HUNTER": {"Orgrimmar", "Thunder Bluff"}, "SHAMAN": {"Orgrimmar", "Thunder Bluff"}, "DRUID": {"Thunder Bluff"}},
	"Alliance": {"WARRIOR": {"Stormwind City", "Ironforge", "Darnassus"}, "MAGE": {"Stormwind City", "Ironforge"}, "PRIEST": {"Stormwind City", "Ironforge", "Darnassus"},
		"ROGUE": {"Stormwind City", "Ironforge", "Darnassus"}, "WARLOCK": {"Stormwind City", "Ironforge"}, "HUNTER": {"Ironforge", "Darnassus"}, "DRUID": {"Darnassus"}},
}

var mcTrainCases = []struct{ side, class, zone string }{
	{"Horde", "MAGE", "Orgrimmar"}, {"Horde", "WARRIOR", "Durotar"}, {"Horde", "PRIEST", "Mulgore"}, {"Horde", "ROGUE", "Tirisfal Glades"},
	{"Horde", "WARLOCK", "The Barrens"}, {"Horde", "HUNTER", "Thunder Bluff"}, {"Alliance", "ROGUE", "Teldrassil"}, {"Alliance", "MAGE", "Elwynn Forest"},
	{"Alliance", "PRIEST", "Darnassus"}, {"Alliance", "WARRIOR", "Dun Morogh"}, {"Alliance", "HUNTER", "Ashenvale"}, {"Alliance", "WARLOCK", "Ironforge"},
}

// TestOfflineBenchmarkCapitalClassTrainer: the capital a character is sent to for the class trainer holds a trainer of the class, is
// the city they stand in when it does, and is on their continent when a city there does.
func TestOfflineBenchmarkCapitalClassTrainer(t *testing.T) {
	var out []benchRow
	for _, tc := range mcTrainCases {
		lv := 20
		c := character.Character{Faction: tc.side, Class: tc.class, Level: &lv, Location: &character.Location{Zone: tc.zone}}
		pl, ok := levelup.TrainAt(c, nil)
		list := mcTrainers[tc.side][tc.class]
		here := slices.Contains(list, tc.zone)
		sameLand := false
		for _, city := range list {
			sameLand = sameLand || learn.ContinentOf(city) == learn.ContinentOf(tc.zone)
		}
		good := ok && slices.Contains(list, pl.City)
		if here {
			good = good && pl.City == tc.zone && pl.Here
		} else if sameLand {
			good = good && learn.ContinentOf(pl.City) == learn.ContinentOf(tc.zone)
		}
		out = append(out, benchRow{fmt.Sprintf("%s %s in %s: the capital to train in", tc.side, strings.ToLower(tc.class), tc.zone), pl.Line(tc.class),
			fmt.Sprintf("class trainers in %s; the character's own city or continent first", strings.Join(list, ", ")), mcClassSrc,
			resultIf(good, "a city with no trainer of the class, or one across the sea when a nearer city has one")})
	}
	finish(t, "advice-capital-class-trainer", "Offline capital cities: the class trainer's city", out)
}

// mcErrandPlan is the plan of a character standing in zone with a spell to train and an unseen bank.
func mcErrandPlan(side, zone string) route.Plan {
	lvl, mp := 30, 7001
	x, y := 50.0, 50.0
	qx := &questie.Index{Quests: map[int]*questie.Quest{}, Npcs: map[int]questie.Entity{}, Objects: map[int]questie.Entity{},
		Areas: map[int]questie.Area{901: {UIMap: 7001, Zone: zone}}}
	c := character.Character{Name: "Benchtest", Faction: side, Class: "MAGE", Level: &lvl, QuestsCaptured: true, Location: &character.Location{Zone: zone, MapID: &mp, X: &x, Y: &y}}
	return route.BuildWith(c, qx, route.Options{Trainable: func(character.Character) int { return 2 }, AnyPickup: true, PickupCap: 99})
}

// TestOfflineBenchmarkCapitalErrands: a character in a capital of their side is offered the trainer and the bank errand; in the
// other side's capital, or outside a city, none.
func TestOfflineBenchmarkCapitalErrands(t *testing.T) {
	var out []benchRow
	for _, tc := range []struct {
		side, zone string
		want       bool
		why        string
	}{
		{"Horde", "Orgrimmar", true, "Horde capital"}, {"Horde", "Undercity", true, "Horde capital"}, {"Horde", "Thunder Bluff", true, "Horde capital"},
		{"Alliance", "Stormwind City", true, "Alliance capital"}, {"Alliance", "Ironforge", true, "Alliance capital"}, {"Alliance", "Darnassus", true, "Alliance capital"},
		{"Horde", "Stormwind City", false, "Alliance capital: no trainer or bank for a Horde character"}, {"Alliance", "Orgrimmar", false, "Horde capital: no trainer or bank for an Alliance character"},
		{"Horde", "Durotar", false, "a zone, not a city"},
	} {
		p := mcErrandPlan(tc.side, tc.zone)
		kinds := map[string]bool{}
		for _, e := range p.Errands {
			kinds[e.Kind] = true
		}
		got := kinds[route.ErrandTrain] && kinds[route.ErrandBank]
		out = append(out, benchRow{fmt.Sprintf("%s in %s: the city errands", tc.side, tc.zone), fmt.Sprintf("%d errands (train %v, bank %v)", len(p.Errands), kinds[route.ErrandTrain], kinds[route.ErrandBank]),
			fmt.Sprintf("errands offered: %v (%s)", tc.want, tc.why), mcErrandSrc, resultIf(got == tc.want && (tc.want || len(p.Errands) == 0), "an errand in the wrong city, or none in the right one")})
	}
	finish(t, "advice-capital-errands", "Offline capital cities: the trainer and bank errands", out)
}

// TestOfflineBenchmarkCapitalNoAnswer: what the dataset does not hold of a capital. No table of banks, auction houses, innkeepers or class
// trainers exists, so the advisor names no NPC or position for a class trainer, and a capital the dataset has no map for is
// not placed on a continent.
func TestOfflineBenchmarkCapitalNoAnswer(t *testing.T) {
	ds := worldStore()
	var out []benchRow
	for _, tbl := range []string{"banks", "auction_houses", "innkeepers", "class_trainers"} {
		_, err := ds.Filter(tbl, store.Range{Column: "zone_id", Min: int64(0)})
		out = append(out, benchRow{"a table of " + strings.ReplaceAll(tbl, "_", " "), fmt.Sprint(err), "no such table in the dataset", mcNoneSrc, resultIf(err != nil, "a table was invented")})
	}
	lv := 20
	c := character.Character{Faction: "Horde", Class: "MAGE", Level: &lv, Location: &character.Location{Zone: "Orgrimmar"}}
	pl, _ := levelup.TrainAt(c, nil)
	line := pl.Line("MAGE")
	out = append(out, benchRow{"Horde mage in Orgrimmar: the trainer's name and place", line, "no class trainer row: the city only, no NPC and no coordinates", mcNoneSrc,
		resultIf(pl.Trainer == nil && !strings.ContainsAny(line, "0123456789"), "an NPC or position was named")})
	zones := worldZones(t)
	for _, city := range []string{"Silvermoon City", "The Exodar"} {
		_, mapped := zones[city]
		cont := learn.ContinentOf(city)
		res := "match"
		if cont != "" && !mapped {
			res = "differ (data): the dataset has no map for the city the advisor places on a continent"
		}
		out = append(out, benchRow{city + ": a map of the city", fmt.Sprintf("continent %q", cont), fmt.Sprintf("ui_maps row: %v", mapped), mcNoneSrc, res})
	}
	finish(t, "advice-capital-no-answer", "Offline capital cities: no answer in the dataset", out)
}

// ---- starting zones and first quests ----

var mcStarts = []struct{ race, zone, continent string }{
	{"Orc and Troll", "Durotar", "Kalimdor"}, {"Tauren", "Mulgore", "Kalimdor"}, {"Undead", "Tirisfal Glades", "Eastern Kingdoms"},
	{"Human", "Elwynn Forest", "Eastern Kingdoms"}, {"Dwarf and Gnome", "Dun Morogh", "Eastern Kingdoms"}, {"Night Elf", "Teldrassil", "Kalimdor"},
}

// TestOfflineBenchmarkStartingZoneContinent: the continent of each race's starting zone beside the parent continent of the zone's
// ui_maps row, and of the starting areas named in the quest log (Deathknell, Valley of Trials, ...) beside the continent of the zone they
// lie in.
func TestOfflineBenchmarkStartingZoneContinent(t *testing.T) {
	zones := worldZones(t)
	var out []benchRow
	for _, s := range mcStarts {
		want := zones[s.zone]
		got := learn.ContinentOf(s.zone)
		out = append(out, benchRow{s.race + ": the continent of " + s.zone, got, want, mcStartSrc, resultIf(got == want && want == s.continent, "another continent than the zone's ui_maps parent")})
	}
	for _, a := range []struct{ area, parent string }{{"Deathknell", "Tirisfal Glades"}, {"Valley of Trials", "Durotar"}, {"Coldridge Valley", "Dun Morogh"}, {"Northshire Valley", "Elwynn Forest"},
		{"Shadowglen", "Teldrassil"}, {"Camp Narache", "Mulgore"}} {
		want, got := zones[a.parent], learn.ContinentOf(a.area)
		out = append(out, benchRow{a.area + ": the continent of the starting area", fmt.Sprintf("%q", got), fmt.Sprintf("%s, the continent of %s", want, a.parent), mcStartSrc, resultIf(got == want, "no continent for a starting area")})
	}
	finish(t, "advice-start-continent", "Offline starting zones: the continent of a starting area", out)
}

// TestOfflineBenchmarkStartingZoneTrainer: a level 10 character of each race's starting zone is sent to a city of their side that
// holds the class trainer, on their continent when one does.
func TestOfflineBenchmarkStartingZoneTrainer(t *testing.T) {
	var out []benchRow
	for _, tc := range []struct{ side, class, zone, city string }{
		{"Horde", "MAGE", "Durotar", "Orgrimmar"}, {"Horde", "WARRIOR", "Valley of Trials", "Orgrimmar"}, {"Horde", "WARLOCK", "Tirisfal Glades", "Undercity"},
		{"Horde", "ROGUE", "Deathknell", "Undercity"}, {"Alliance", "WARRIOR", "Elwynn Forest", "Stormwind City"}, {"Alliance", "HUNTER", "Dun Morogh", "Ironforge"},
		{"Alliance", "PRIEST", "Teldrassil", "Darnassus"}, {"Alliance", "ROGUE", "Shadowglen", "Darnassus"},
	} {
		lv := 10
		c := character.Character{Faction: tc.side, Class: tc.class, Level: &lv, Location: &character.Location{Zone: tc.zone}}
		pl, _ := levelup.TrainAt(c, nil)
		out = append(out, benchRow{fmt.Sprintf("%s %s in %s: the city to train in", tc.side, strings.ToLower(tc.class), tc.zone), pl.City, tc.city + " (the nearest city with the class trainer)", mcClassSrc, resultIf(pl.City == tc.city, "another city")})
	}
	finish(t, "advice-start-trainer", "Offline starting zones: the city to train in", out)
}

var mcFirstQuests = []struct {
	query, title, category string
	level                  int
}{
	{"Rude Awakening", "Rude Awakening", "Deathknell", 1}, {"Your Place in the World", "Your Place in the World", "Valley of Trials", 1},
	{"cutting teeth", "Cutting Teeth", "Valley of Trials", 2}, {"The Mindless Ones", "The Mindless Ones", "Deathknell", 2},
	{"Sting of the Scorpid", "Sting of the Scorpid", "Valley of Trials", 3}, {"A Rogues Deal", "A Rogue's Deal", "Deathknell", 5},
	{"Rattling the Rattlecages", "Rattling the Rattlecages", "Deathknell", 3}, {"Coming of Age", "Coming of Age", "Zephras Isle", 1},
}

// TestOfflineBenchmarkFirstQuests: the first quests of the Horde's starting areas found by the name a player types (any case, an apostrophe
// left out), with the level, faction and area players saw them in, beside the dataset's quests_observed row and Wowhead's level.
func TestOfflineBenchmarkFirstQuests(t *testing.T) {
	ds := qbStore()
	var out []benchRow
	for _, q := range mcFirstQuests {
		rows, _ := ds.Filter("quests_observed", store.Range{Column: "title", Min: q.title, Max: q.title})
		if len(rows) == 0 {
			t.Fatalf("%s: not in quests_observed", q.title)
		}
		r := rows[0]
		id, name, cands := quests.Resolve(ds, q.query)
		var f *quests.Facts
		if id != 0 {
			f = quests.FactsOf(ds, nil, id)
		}
		got := fmt.Sprintf("quest %d %q (%d candidates)", id, name, len(cands))
		ok := f != nil && id == num(r["quest_id"]) && f.Level == q.level && f.Category == q.category && f.Faction == "horde" && num(r["level"]) == int64(q.level)
		if f != nil {
			got += fmt.Sprintf(": level %d, %s, %s", f.Level, f.Faction, f.Category)
		}
		out = append(out, benchRow{fmt.Sprintf("%q: the first quest", q.query), got, fmt.Sprintf("quests_observed %d: level %d, horde, %s; Wowhead level %d", num(r["quest_id"]), num(r["level"]), q.category, q.level), mcStartSrc,
			resultIf(ok, "another quest, level, side or area")})
	}
	finish(t, "advice-start-first-quests", "Offline starting zones: the first quests of an area", out)
}

// TestOfflineBenchmarkStartingZoneNoAnswer: the Alliance's and the Tauren's, Night Elves' and Dwarves' first quests are not in quests_observed
// (its Elwynn Forest rows start at level 7, Dun Morogh at 8, Teldrassil at 7): the advisor names no such quest and no giver, level or hub.
func TestOfflineBenchmarkStartingZoneNoAnswer(t *testing.T) {
	ds := qbStore()
	var out []benchRow
	for _, q := range []struct{ title, who string }{{"A Threat Within", "Human, Northshire Valley, level 1"}, {"Dwarven Outfitters", "Dwarf and Gnome, Coldridge Valley, level 1"},
		{"The Balance of Nature", "Night Elf, Shadowglen, level 1"}, {"Rites of the Earthmother", "Tauren, Camp Narache, level 1"}} {
		id, name, cands := quests.Resolve(ds, q.title)
		out = append(out, benchRow{q.title + ": the first quest of " + strings.SplitN(q.who, ",", 2)[0], fmt.Sprintf("quest %d %q, %d candidates", id, name, len(cands)), "Wowhead: " + q.who + "; no quests_observed row", mcNoneSrc,
			resultIf(id == 0 && len(cands) == 0 && name == "", "a quest was invented")})
	}
	_, err := ds.Filter("races", store.Range{Column: "id", Min: int64(0)})
	out = append(out, benchRow{"a table of races and their starting zones", fmt.Sprint(err), "no such table in the dataset", mcNoneSrc, resultIf(err != nil, "a table was invented")})
	id, _, cands := quests.Resolve(ds, "Mage Trial of the Starting Hub")
	out = append(out, benchRow{"an unknown quest named like a starting quest", fmt.Sprintf("quest %d, %d candidates", id, len(cands)), "no row", mcNoneSrc, resultIf(id == 0 && len(cands) == 0, "a quest was invented")})
	finish(t, "advice-start-no-answer", "Offline starting zones: no answer in the dataset", out)
}

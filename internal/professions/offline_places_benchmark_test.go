package professions

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/dungeons"
	"github.com/WoW-Companion/advisor/internal/gear"
	"github.com/WoW-Companion/advisor/internal/store"
	"github.com/WoW-Companion/advisor/internal/vendors"
)

// The offline places benchmark: where the advisor sends a player (which dungeon for a level, which master, trainer
// or vendor for a skill or an item) beside the dataset rows, with no network. testdata/places_snapshot
// (testdata/places_snapshot/build.sh) holds the published instances, instance_bosses, npcs, vendor_sources,
// trainers, weapon_masters, weapon_skills and profession_tiers tables; items and loot come from
// testdata/advice_snapshot. The dataset holds no level range for an open-world zone, so no zone-for-level
// answer is read from it: the advisor names a zone only as a dungeon's entrance, a trainer's or a gathering
// place, and those are the zone checks here. The reference of each case is read straight from the tables (never
// through the advisor). A case where the dataset has no answer passes when the advisor says so and names
// nothing. Run it with make offline-benchmark; WOWC_BENCH_REPORT receives the result tables in Markdown.

const (
	placesDir     = "../../testdata/places_snapshot"
	instancesSrc  = "dataset instances (min_level, max_level, entrance_zone_name)"
	mastersSrc    = "dataset weapon_masters, weapon_skills"
	tiersSrc      = "dataset profession_tiers"
	trainersSrc   = "dataset trainers"
	vendorsSrc    = "dataset vendor_sources, npcs"
	placesNoReply = "the dataset has no answer"
)

// mixSource serves the places snapshot's tables first and the advice snapshot's for the rest.
type mixSource struct{ first, then store.DirSource }

func (m mixSource) Open(table string) (io.ReaderAt, int64, error) {
	if r, n, err := m.first.Open(table); err == nil {
		return r, n, nil
	}
	return m.then.Open(table)
}

var (
	placesOnce  sync.Once
	placesCache store.DataStore
)

// placesStore reads the snapshot once for the package: the tests share the decoded tables.
func placesStore() store.DataStore {
	placesOnce.Do(func() {
		placesCache = store.New(mixSource{first: store.DirSource(placesDir), then: store.DirSource(snapshotDir)})
	})
	return placesCache
}

func levelOf(n int) *int { return &n }

// refDungeon is an instances row of a dungeon with a level range.
type refDungeon struct {
	name   string
	lo, hi int
	zone   string
}

func (d refDungeon) String() string { return fmt.Sprintf("%s %d-%d %s", d.name, d.lo, d.hi, d.zone) }

// homeFaction is the side a capital entrance belongs to (docs/dungeons.md: a dungeon with no faction column is
// hidden from the other side when its entrance is that side's capital).
var homeFaction = map[string]string{"Orgrimmar": "Horde", "Thunder Bluff": "Horde", "Undercity": "Horde",
	"Stormwind City": "Alliance", "Ironforge": "Alliance", "Darnassus": "Alliance"}

// refDungeons are the dungeons the dataset's ranges put in reach of a level: from the minimum up to the maximum
// plus the three levels the advisor allows past it, open to the faction.
func refDungeons(t *testing.T, ds store.DataStore, faction string, level int) []refDungeon {
	t.Helper()
	rows, err := ds.Filter("instances", store.Range{Column: "map_id", Min: int64(0)})
	if err != nil {
		t.Fatal(err)
	}
	var out []refDungeon
	for _, r := range rows {
		if !isDungeon(r) || r["min_level"] == nil || r["max_level"] == nil {
			continue
		}
		d := refDungeon{asText(r["name"]), int(asI64(r["min_level"])), int(asI64(r["max_level"])), asText(r["entrance_zone_name"])}
		if side := homeFaction[d.zone]; side != "" && side != faction {
			continue
		}
		if d.lo <= level && level <= d.hi+dungeons.LevelWindow {
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}

// isDungeon is a dungeon players run: typed dungeon, with an entrance zone or a group size (the client's Ruins of
// Lordaeron has neither; docs/dungeons.md).
func isDungeon(r store.Row) bool {
	return asText(r["instance_type"]) == "dungeon" && (asText(r["entrance_zone_name"]) != "" || r["max_players"] != nil)
}

// refUnranged are the dungeons the dataset holds with no level range.
func refUnranged(t *testing.T, ds store.DataStore) []string {
	t.Helper()
	rows, _ := ds.Filter("instances", store.Range{Column: "map_id", Min: int64(0)})
	var out []string
	for _, r := range rows {
		if isDungeon(r) && (r["min_level"] == nil || r["max_level"] == nil) {
			out = append(out, asText(r["name"]))
		}
	}
	sort.Strings(out)
	return out
}

func resultIf(ok bool, bad string) string {
	if ok {
		return "match"
	}
	return "differ: " + bad
}

var dungeonCases = []struct {
	faction string
	level   int
}{
	{"Horde", 10}, {"Horde", 13}, {"Horde", 18}, {"Horde", 25}, {"Horde", 34}, {"Horde", 45}, {"Horde", 55}, {"Horde", 60},
	{"Alliance", 5}, {"Alliance", 15}, {"Alliance", 22}, {"Alliance", 30}, {"Alliance", 38}, {"Alliance", 52}, {"Alliance", 60},
}

// The dungeons dungeon_plan lists for a level and side, with the range and entrance zone each is worded with, beside
// the dataset's instances rows; three tests, so each stays inside the CPU cap of a test.
func TestOfflineBenchmarkDungeonForLevelA(t *testing.T) { dungeonForLevel(t, "a", dungeonCases[0:5]) }
func TestOfflineBenchmarkDungeonForLevelB(t *testing.T) { dungeonForLevel(t, "b", dungeonCases[5:10]) }
func TestOfflineBenchmarkDungeonForLevelC(t *testing.T) { dungeonForLevel(t, "c", dungeonCases[10:]) }

func dungeonForLevel(t *testing.T, part string, cases []struct {
	faction string
	level   int
}) {
	ds := placesStore()
	var out []benchRow
	for _, tc := range cases {
		c := character.Character{Faction: tc.faction, Class: "WARRIOR", Level: levelOf(tc.level)}
		label := fmt.Sprintf("%s level %d: dungeons", tc.faction, tc.level)
		res, err := dungeons.PlanWith(c, ds, nil)
		if err != nil {
			t.Fatal(err)
		}
		want := refDungeons(t, ds, tc.faction, tc.level)
		if len(want) == 0 {
			// below every range: the first dungeon, with the levels still to go
			rows, _ := ds.Filter("instances", store.Range{Column: "map_id", Min: int64(0)})
			var first refDungeon
			for _, r := range rows {
				if !isDungeon(r) || r["min_level"] == nil || r["max_level"] == nil {
					continue
				}
				d := refDungeon{asText(r["name"]), int(asI64(r["min_level"])), int(asI64(r["max_level"])), asText(r["entrance_zone_name"])}
				if side := homeFaction[d.zone]; side != "" && side != tc.faction {
					continue
				}
				if first.name == "" || d.lo < first.lo {
					first = d
				}
			}
			got, text := "no dungeon listed", dungeons.Render(c, res, 0)
			if res.First != nil {
				got = fmt.Sprintf("%s, %d to go", res.First.Name, res.LevelsToGo)
			}
			ref := fmt.Sprintf("%s, %d to go", first.name, first.lo-tc.level)
			ok := got == ref && strings.Contains(text, fmt.Sprintf("about levels %d–%d", first.lo, first.hi)) && strings.Contains(text, "in "+first.zone)
			out = append(out, benchRow{label + " (too low: the first one)", got, ref + " " + first.zone, instancesSrc, resultIf(ok, "the first dungeon, its range or its entrance")})
			continue
		}
		var have []refDungeon
		for _, p := range res.Picks {
			if p.MinLevel == nil || p.MaxLevel == nil {
				continue
			}
			have = append(have, refDungeon{p.Name, *p.MinLevel, *p.MaxLevel, p.EntranceZone})
		}
		sort.Slice(have, func(i, j int) bool { return have[i].name < have[j].name })
		join := func(l []refDungeon) string {
			var s []string
			for _, d := range l {
				s = append(s, d.String())
			}
			return strings.Join(s, "; ")
		}
		out = append(out, benchRow{label, join(have), join(want), instancesSrc, resultIf(join(have) == join(want), "the list, a range or an entrance zone")})

		// the words: every listed dungeon is worded with its range and its entrance zone
		text := dungeons.Render(c, res, 0)
		var missing []string
		for _, d := range want {
			if !strings.Contains(text, fmt.Sprintf("%s (about levels %d–%d; entrance in %s", d.name, d.lo, d.hi, d.zone)) {
				missing = append(missing, d.name)
			}
		}
		out = append(out, benchRow{label + ": wording", fmt.Sprintf("%d of %d worded with range and entrance", len(want)-len(missing), len(want)),
			fmt.Sprintf("%d of %d", len(want), len(want)), instancesSrc, resultIf(len(missing) == 0, strings.Join(missing, ", ")+" not worded with its range and entrance")})

		// no boss name is listed twice (the dataset keeps one encounter row per difficulty)
		var dup []string
		for _, line := range strings.Split(text, "Drops not known yet: ") {
			seen := map[string]bool{}
			line, _, _ = strings.Cut(line, "\n")
			for _, n := range strings.Split(line, ", ") {
				if seen[strings.ToLower(n)] && n != "" {
					dup = append(dup, n)
				}
				seen[strings.ToLower(n)] = true
			}
		}
		out = append(out, benchRow{label + ": boss names", fmt.Sprintf("%d repeated", len(dup)), "each boss once", "dataset instance_bosses", resultIf(len(dup) == 0, "listed twice: "+strings.Join(dup, ", "))})
	}
	finishOffline(t, "places-dungeons-"+part, "Offline dungeon for a level, both sides ("+part+")", out)
}

var namedCases = []struct {
	name  string
	level int
}{{"Ragefire Chasm", 10}, {"Deadmines", 20}, {"Blackrock Depths", 40}, {"Zul'Farrak", 60}}

// TestOfflineBenchmarkNamedDungeon: a dungeon asked for by name, whatever the level: range, entrance, group size and
// the levels still to go.
func TestOfflineBenchmarkNamedDungeon(t *testing.T) {
	ds := placesStore()
	var out []benchRow
	for _, tc := range namedCases {
		rows, _ := ds.Filter("instances", store.Range{Column: "name", Min: tc.name, Max: tc.name})
		if len(rows) == 0 {
			t.Fatalf("%s: not in the snapshot", tc.name)
		}
		r := rows[0]
		c := character.Character{Faction: "Horde", Class: "WARRIOR", Level: levelOf(tc.level)}
		p, togo, err := dungeons.Named(c, ds, nil, tc.name)
		if err != nil || p == nil {
			t.Fatalf("%s: %v %v", tc.name, p, err)
		}
		lo, hi := int(asI64(r["min_level"])), int(asI64(r["max_level"]))
		wantTogo := max(lo-tc.level, 0)
		got := fmt.Sprintf("levels %d-%d, %s, %d players, %d to go", *p.MinLevel, *p.MaxLevel, p.EntranceZone, *p.MaxPlayers, togo)
		ref := fmt.Sprintf("levels %d-%d, %s, %d players, %d to go", lo, hi, asText(r["entrance_zone_name"]), dungeons.PartySize(tc.name, false, int(asI64(r["max_players"]))), wantTogo) // a dungeon's party is five, whatever the table's cap
		text := dungeons.RenderNamed(c, *p, togo)
		ok := got == ref && strings.Contains(text, fmt.Sprintf("about levels %d–%d", lo, hi)) && (wantTogo == 0) == !strings.Contains(text, "it opens around")
		out = append(out, benchRow{fmt.Sprintf("%s asked at level %d", tc.name, tc.level), got, ref, instancesSrc, resultIf(ok, "the range, entrance, group size or levels to go")})
	}
	finishOffline(t, "places-named-dungeon", "Offline dungeon asked by name", out)
}

// TestOfflineBenchmarkNoDungeonAnswer: dungeons the dataset has no level for, and a name it does not hold. The advisor
// says the level is not known and ranks nothing.
func TestOfflineBenchmarkNoDungeonAnswer(t *testing.T) {
	ds := placesStore()
	var out []benchRow
	c := character.Character{Faction: "Horde", Class: "WARRIOR", Level: levelOf(30)}
	res, err := dungeons.PlanWith(c, ds, nil)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, p := range res.Unknown {
		got = append(got, p.Name)
	}
	sort.Strings(got)
	want := refUnranged(t, ds)
	text := dungeons.Render(c, res, 0)
	ranked := false
	for _, p := range res.Picks {
		if p.MinLevel == nil || p.MaxLevel == nil {
			ranked = true
		}
	}
	ok := strings.Join(got, ",") == strings.Join(want, ",") && strings.Contains(text, "Level not known yet: ") && !ranked
	out = append(out, benchRow{"dungeons with no level range, level 30 Horde", strings.Join(got, ", "), strings.Join(want, ", "), instancesSrc,
		resultIf(ok, "an unranged dungeon is missing, ranked or not said to be unknown")})
	for _, name := range []string{"Caverns of Time", "Demon Fall Canyon"} {
		p, _, err := dungeons.Named(c, ds, nil, name)
		ok := err == nil && p != nil && p.LevelUnknown && strings.Contains(dungeons.RenderNamed(c, *p, 0), name) && !strings.Contains(dungeons.RenderNamed(c, *p, 0), "about levels")
		out = append(out, benchRow{name + " asked by name", "no level range worded", placesNoReply + ": no min_level or max_level", instancesSrc, resultIf(ok, "a level range is worded")})
	}
	p, _, err := dungeons.Named(c, ds, nil, "Keep of the Unnamed Dawn")
	out = append(out, benchRow{"a dungeon the dataset does not hold", fmt.Sprintf("answer %v", p != nil), placesNoReply, instancesSrc, resultIf(err == nil && p == nil, "a dungeon was invented")})
	finishOffline(t, "places-no-dungeon-answer", "Offline dungeon, no answer in the dataset", out)
}

// refMaster is a weapon_masters row.
type refMaster struct {
	name, faction, city, place string
	skills, cost, level        []int64
}

func ints64(v any) []int64 {
	l, _ := v.([]any)
	var out []int64
	for _, x := range l {
		out = append(out, asI64(x))
	}
	return out
}

func refMasters(t *testing.T, ds store.DataStore) []refMaster {
	t.Helper()
	rows, err := ds.Filter("weapon_masters", store.Range{Column: "npc_id", Min: int64(0), Max: int64(1 << 40)})
	if err != nil {
		t.Fatal(err)
	}
	var out []refMaster
	for _, r := range rows {
		out = append(out, refMaster{asText(r["name"]), asText(r["faction"]), asText(r["city"]), asText(r["place"]), ints64(r["skills"]), ints64(r["cost_copper"]), ints64(r["min_level"])})
	}
	return out
}

// masterClass is a class that can learn the skill, read from weapon_skills.
func masterClass(t *testing.T, ds store.DataStore, skill int64) string {
	t.Helper()
	rows, _ := ds.Filter("weapon_skills", store.Range{Column: "skill_line_id", Min: skill, Max: skill})
	for _, r := range rows {
		if b, _ := r["learnable"].(bool); b {
			for name, id := range map[string]int64{"WARRIOR": 1, "PALADIN": 2, "HUNTER": 3, "ROGUE": 4, "PRIEST": 5, "SHAMAN": 7, "MAGE": 8, "WARLOCK": 9, "DRUID": 11} {
				if asI64(r["class_id"]) == id {
					return name
				}
			}
		}
	}
	return ""
}

var masterCases = []struct {
	faction string
	skill   int64
	stand   string // the zone the player stands in; "" for unknown
}{
	{"Horde", 44, ""}, {"Horde", 172, ""}, {"Horde", 136, ""}, {"Horde", 45, ""}, {"Horde", 176, ""}, {"Horde", 54, ""},
	{"Horde", 226, ""}, {"Horde", 229, ""}, {"Horde", 473, ""}, {"Horde", 136, "Thunder Bluff"},
	{"Alliance", 44, ""}, {"Alliance", 136, ""}, {"Alliance", 173, ""}, {"Alliance", 229, ""}, {"Alliance", 226, ""},
	{"Alliance", 473, ""}, {"Alliance", 136, "Stormwind City"},
}

// TestOfflineBenchmarkWeaponMasters: who teaches a weapon skill and where, for each side, beside weapon_masters.
func TestOfflineBenchmarkWeaponMasters(t *testing.T) {
	ds := placesStore()
	masters := refMasters(t, ds)
	var out []benchRow
	for _, tc := range masterCases {
		class := masterClass(t, ds, tc.skill)
		if class == "" {
			t.Fatalf("skill %d: no class learns it", tc.skill)
		}
		c := character.Character{Faction: tc.faction, Class: class, Level: levelOf(40)}
		if tc.stand != "" {
			x, y := 50.0, 50.0
			c.Location = &character.Location{Zone: tc.stand, X: &x, Y: &y}
		}
		f := gear.LoadWeaponSkills(c, ds)
		var have, want []string
		cost := map[string]string{}
		for _, m := range masters {
			for i, s := range m.skills {
				if s == tc.skill && strings.EqualFold(m.faction, tc.faction) {
					want = append(want, fmt.Sprintf("%s, %s", m.name, joinPlace(m.city, m.place)))
					cost[m.name] = fmt.Sprintf("%s, level %d", gear.CostText(m.cost[i]), m.level[i])
				}
			}
		}
		var haveCost []string
		for _, ta := range f.MastersFor(tc.skill) {
			have = append(have, fmt.Sprintf("%s, %s", ta.Master.Name, ta.Master.Where()))
			if want := cost[ta.Master.Name]; want != "" {
				haveCost = append(haveCost, fmt.Sprintf("%s, level %d", gear.CostText(ta.Cost), ta.MinLevel))
				if haveCost[len(haveCost)-1] != want {
					have[len(have)-1] += " (price or level differs)"
				}
			}
		}
		label := fmt.Sprintf("%s: who teaches %s", tc.faction, f.Names[tc.skill])
		if tc.stand != "" {
			label += ", standing in " + tc.stand
		}
		got, ref := strings.Join(have, "; "), strings.Join(want, "; ")
		if tc.stand == "" {
			sort.Strings(have)
			sort.Strings(want)
			got, ref = strings.Join(have, "; "), strings.Join(want, "; ")
		} else if len(have) > 0 && len(want) > 0 {
			// nearest first: the master in the zone the player stands in leads
			ref = ""
			for _, m := range masters {
				if strings.EqualFold(m.city, tc.stand) && strings.EqualFold(m.faction, tc.faction) && priceOf(m, tc.skill) > 0 {
					ref = fmt.Sprintf("%s, %s", m.name, joinPlace(m.city, m.place))
				}
			}
			got = have[0]
		}
		out = append(out, benchRow{label, got, ref, mastersSrc, resultIf(got == ref, "the master, city, district, price or level")})

		// the sentence the player reads names a master and a city the dataset has
		if tc.stand == "" {
			sub := -1
			for s := 0; s < 30; s++ {
				if gear.SkillOfItem(s) == tc.skill {
					sub = s
				}
			}
			c.SkillsCaptured = true
			_, need := f.ItemSkill(c, 2, sub)
			line := ""
			if need != nil {
				line = need.Line()
			}
			ok := need != nil && len(want) > 0
			if ok {
				ok = false
				for _, m := range masters {
					if strings.Contains(line, "at "+m.name+" in "+m.city+" ("+gear.CostText(priceOf(m, tc.skill))+")") && strings.EqualFold(m.faction, tc.faction) {
						ok = true
					}
				}
			}
			out = append(out, benchRow{fmt.Sprintf("%s: the line for a %s weapon", tc.faction, f.Names[tc.skill]), line, "a master of the side, in the master's city, at the master's price", mastersSrc, resultIf(ok, "the line names a master, city or price the dataset lacks")})
		}
	}
	finishOffline(t, "places-weapon-masters", "Offline weapon masters, both sides", out)
}

func priceOf(m refMaster, skill int64) int64 {
	for i, s := range m.skills {
		if s == skill {
			return m.cost[i]
		}
	}
	return 0
}

func joinPlace(city, place string) string {
	if place == "" {
		return city
	}
	return city + ", " + place
}

// TestOfflineBenchmarkNoMasterAnswer: skills no master of the dataset teaches (Wands). The advisor names none.
func TestOfflineBenchmarkNoMasterAnswer(t *testing.T) {
	ds := placesStore()
	var out []benchRow
	for _, side := range []string{"Horde", "Alliance"} {
		c := character.Character{Faction: side, Class: "MAGE", Level: levelOf(30), SkillsCaptured: true}
		f := gear.LoadWeaponSkills(c, ds)
		_, need := f.ItemSkill(c, 2, 19) // wands
		line := "(no line)"
		if need != nil {
			line = need.Line()
		}
		ok := len(f.MastersFor(228)) == 0 && need != nil && need.Master == "" && strings.Contains(line, "no Weapon Master teaches it") && !strings.Contains(line, "train it at")
		out = append(out, benchRow{side + ": who teaches Wands", line, placesNoReply + ": no weapon_masters row lists skill 228", mastersSrc, resultIf(ok, "a master is named")})
	}
	finishOffline(t, "places-no-master-answer", "Offline weapon master, no answer in the dataset", out)
}

var trainerCases = []struct{ faction, prof string }{
	{"Horde", "Tailoring"}, {"Alliance", "Tailoring"},
	{"Horde", "Alchemy"}, {"Alliance", "Alchemy"},
	{"Horde", "Blacksmithing"}, {"Alliance", "Blacksmithing"},
	{"Horde", "Leatherworking"}, {"Alliance", "Leatherworking"},
	{"Horde", "Engineering"}, {"Alliance", "Engineering"},
}

// refTrainer is a profession_tiers row that names a trainer.
type refTrainer struct{ name, place, faction, tier string }

func refTrainers(t *testing.T, ds store.DataStore, prof, faction string) []refTrainer {
	t.Helper()
	rows, err := ds.Filter("profession_tiers", store.Range{Column: "profession", Min: prof, Max: prof})
	if err != nil {
		t.Fatal(err)
	}
	var out []refTrainer
	for _, r := range rows {
		f := asText(r["faction"])
		if asText(r["trainer_name"]) == "" || asText(r["how"]) != "trainer" || (f != "Both" && f != "" && !strings.EqualFold(f, faction)) {
			continue
		}
		out = append(out, refTrainer{asText(r["trainer_name"]), asText(r["place"]), f, asText(r["tier"])})
	}
	return out
}

// TestOfflineBenchmarkProfessionTrainer: the trainer a profession path sends a player to, for each side, beside the
// dataset's named trainers and places (profession_tiers) and the trainers table.
func TestOfflineBenchmarkProfessionTrainer(t *testing.T) {
	ds := placesStore()
	var out []benchRow
	for _, tc := range trainerCases {
		p := &planner{c: character.Character{Faction: tc.faction, Level: levelOf(30)}, ds: ds, prof: tc.prof}
		got := p.trainerStep("Journeyman")
		ref := refTrainers(t, ds, tc.prof, tc.faction)
		var names []string
		okName := false
		for _, r := range ref {
			names = append(names, r.name+" in "+r.place)
			if strings.Contains(got, r.name+" in "+r.place) {
				okName = true
			}
		}
		other := "Alliance"
		if tc.faction == "Alliance" {
			other = "Horde"
		}
		wrongSide := false
		for _, r := range refTrainers(t, ds, tc.prof, other) {
			if r.faction == other && strings.Contains(got, r.name) {
				wrongSide = true
			}
		}
		// a trainer the dataset names with a place is named with it; an id is never the answer when a name exists
		res := resultIf(okName && !wrongSide && !strings.Contains(got, "name not in the dataset"), "the trainer's name or place, the other side's trainer, or an id instead of a name")
		out = append(out, benchRow{fmt.Sprintf("%s: %s trainer", tc.faction, tc.prof), got, strings.Join(names, "; "), tiersSrc + ", " + trainersSrc, res})
	}
	finishOffline(t, "places-profession-trainer", "Offline profession trainer, both sides", out)
}

// TestOfflineBenchmarkNoTrainerAnswer: professions where the dataset names no trainer for a side. The advisor says no
// trainer is known and names nobody, never an id and never a trainer of the other side.
func TestOfflineBenchmarkNoTrainerAnswer(t *testing.T) {
	ds := placesStore()
	var out []benchRow
	for _, tc := range []struct{ faction, prof string }{{"Alliance", "Mining"}, {"Horde", "Mining"}, {"Alliance", "Skinning"}, {"Horde", "Skinning"}} {
		p := &planner{c: character.Character{Faction: tc.faction, Level: levelOf(30)}, ds: ds, prof: tc.prof}
		got := p.trainerStep("Journeyman")
		ref := refTrainers(t, ds, tc.prof, tc.faction)
		rows, _ := ds.Filter("trainers", store.Range{Column: "profession", Min: tc.prof, Max: tc.prof})
		usable := 0
		for _, r := range rows {
			if f := asText(r["faction"]); f != "" && !strings.EqualFold(f, tc.faction) {
				continue
			}
			if side := sideOfZone(asText(r["zone_name"])); side != "" && !strings.EqualFold(side, tc.faction) {
				continue
			}
			usable++
		}
		if len(ref) > 0 {
			t.Fatalf("%s %s: the snapshot now names a trainer: %v", tc.faction, tc.prof, ref)
		}
		// the dataset has no named trainer: the line says so, or names a place with no invented name
		ok := strings.Contains(got, "name not in the dataset") && usable > 0 || usable == 0 && strings.Contains(got, "names no "+tc.prof+" trainer")
		out = append(out, benchRow{fmt.Sprintf("%s: %s trainer, none named", tc.faction, tc.prof), got,
			fmt.Sprintf("%s: %d unnamed trainer row(s) for the side", placesNoReply, usable), tiersSrc + ", " + trainersSrc, resultIf(ok, "the answer is silent or names someone")})
	}
	finishOffline(t, "places-no-trainer-answer", "Offline profession trainer, no named trainer in the dataset", out)
}

// TestOfflineBenchmarkNoVendorAnswer: items the dataset lists vendors for but names none of. The advisor names no
// vendor (a vendor it cannot name is skipped), so no "buy from" line is made up.
func TestOfflineBenchmarkNoVendorAnswer(t *testing.T) {
	ds := placesStore()
	rows, err := ds.Filter("vendor_sources", store.Range{Column: "item_id", Min: int64(1)})
	if err != nil || len(rows) == 0 {
		t.Fatal(err)
	}
	var out []benchRow
	seen := map[int64]bool{}
	for _, r := range rows {
		id := asI64(r["item_id"])
		if seen[id] || len(out) == 6 {
			continue
		}
		seen[id] = true
		n, err := ds.Get("npcs", asI64(r["npc_id"]))
		named := err == nil && asText(n["name"]) != ""
		o, ok := vendors.Best(ds, id)
		got := "no vendor named"
		if ok {
			got = o.Line()
		}
		ref := fmt.Sprintf("npc %d sells it; npcs holds no name", asI64(r["npc_id"]))
		if named {
			ref = "vendor named"
		}
		out = append(out, benchRow{fmt.Sprintf("item %d: who sells it", id), got, placesNoReply + ": " + ref, vendorsSrc, resultIf(ok == named, "a vendor is named without the dataset naming one")})
	}
	if len(out) < 6 {
		t.Fatalf("only %d items", len(out))
	}
	finishOffline(t, "places-no-vendor-answer", "Offline vendor, no named vendor in the dataset", out)
}

// TestOfflineBenchmarkTailoringExpertTrainers: the Expert Tailoring trainers the guide names, by side.
func TestOfflineBenchmarkTailoringExpertTrainers(t *testing.T) {
	ds := placesStore()
	var out []benchRow
	for _, side := range []string{"Horde", "Alliance"} {
		got := expertEarly("Tailoring", side, 30)
		var want []string
		okAll := true
		for _, r := range refTrainers(t, ds, "Tailoring", side) {
			if r.tier == "Expert" && r.faction == side {
				place := strings.TrimSuffix(r.place, " City")
				want = append(want, r.name+" in "+r.place)
				if !strings.Contains(got, r.name+" in "+place) {
					okAll = false
				}
			}
		}
		out = append(out, benchRow{side + ": the Expert Tailoring trainer", got, strings.Join(want, "; "), tiersSrc, resultIf(okAll && len(want) > 0, "the trainer or town differs")})
	}
	finishOffline(t, "places-expert-trainer", "Offline Expert Tailoring trainer, both sides", out)
}

// TestOfflineBenchmarkPlacesDataGaps: what the published tables lack for the places the advisor names. Each row is a
// difference on the dataset's side (the data is read-only here); it fails nothing, and it is read from the tables, so
// the row goes when the gap is filled.
func TestOfflineBenchmarkPlacesDataGaps(t *testing.T) {
	ds := placesStore()
	count := func(table string, keep func(store.Row) bool) (n, of int) {
		rows, err := ds.Filter(table, store.Range{Column: columnOf[table], Min: int64(0), Max: int64(1 << 40)})
		if err != nil {
			t.Fatalf("%s: %v", table, err)
		}
		for _, r := range rows {
			of++
			if keep(r) {
				n++
			}
		}
		return n, of
	}
	gap := func(check, advisor, source string, have, of int, what string) benchRow {
		res := "match"
		if have < of {
			res = fmt.Sprintf("differ (data): %d of %d %s", of-have, of, what)
		}
		return benchRow{check, advisor, fmt.Sprintf("%d of %d rows", have, of), source, res}
	}
	var out []benchRow

	vendorNPCs := map[int64]bool{}
	vrows, _ := ds.Filter("vendor_sources", store.Range{Column: "npc_id", Min: int64(0), Max: int64(1 << 40)})
	for _, r := range vrows {
		vendorNPCs[asI64(r["npc_id"])] = true
	}
	named := 0
	for id := range vendorNPCs {
		if n, err := ds.Get("npcs", id); err == nil && asText(n["name"]) != "" {
			named++
		}
	}
	out = append(out, gap("vendors that sell an item: a name", "names a vendor only when npcs has the name", vendorsSrc, named, len(vendorNPCs), "vendors have no name, so none is named"))
	n, of := count("npcs", func(r store.Row) bool { return asText(r["zone_name"]) != "" })
	out = append(out, gap("creatures: a zone", "names a zone only when npcs has one", "dataset npcs.zone_name", n, of, "creatures have no zone, so no zone-for-level answer can be read"))
	n, of = count("npcs", func(r store.Row) bool { return r["min_level"] != nil && r["max_level"] != nil })
	out = append(out, gap("creatures: a level range", "calls a creature in range only when npcs has levels", "dataset npcs.min_level, max_level", n, of, "creatures have no level range"))
	n, of = count("quests", func(r store.Row) bool { return r["level"] != nil })
	out = append(out, gap("quests: a level", "places a quest by its level only when quests has one", "dataset quests.level", n, of, "quests have no level"))
	n, of = count("trainers", func(r store.Row) bool { return asText(r["name"]) != "" })
	out = append(out, gap("profession trainers: a name", "names a trainer from trainers, else from the guide's tiers", trainersSrc, n, of, "trainers have no name"))
	n, of = count("trainers", func(r store.Row) bool { return asText(r["zone_name"]) != "" && !continents[asText(r["zone_name"])] })
	out = append(out, gap("profession trainers: a place", "gives a place only when it is a zone or town", trainersSrc, n, of, "trainers have no zone or sit on a continent map"))
	n, of = count("trainers", func(r store.Row) bool { return asText(r["faction"]) != "" })
	out = append(out, gap("profession trainers: a side", "hides a trainer in the other side's home zone", trainersSrc, n, of, "trainers have no faction"))
	n, of = count("profession_tiers", func(r store.Row) bool { return asText(r["trainer_name"]) != "" })
	out = append(out, gap("guide rank trainers: a name", "names a trainer when the guide does", tiersSrc, n, of, "guide rows name no trainer (Mining, Skinning)"))

	// one encounter row per difficulty: more rows than boss names, and two bosses in one name
	rows, _ := ds.Filter("instance_bosses", store.Range{Column: "map_id", Min: int64(0), Max: int64(1 << 40)})
	names, joined := map[string]bool{}, 0
	for _, r := range rows {
		names[strings.ToLower(asText(r["name"]))] = true
		if strings.Contains(asText(r["name"]), " and ") {
			joined++
		}
	}
	out = append(out, gap("dungeon bosses: one row a boss", "names each boss once", "dataset instance_bosses", len(names), len(rows), "encounter rows repeat a boss name"))
	out = append(out, gap("dungeon bosses: one boss a row", "names the boss as the row does", "dataset instance_bosses", len(rows)-joined, len(rows), "encounter rows name two bosses"))
	n, of = 0, 0
	irows, _ := ds.Filter("instances", store.Range{Column: "map_id", Min: int64(0)})
	for _, r := range irows {
		if isDungeon(r) {
			of++
			if r["min_level"] != nil && r["max_level"] != nil {
				n++
			}
		}
	}
	out = append(out, gap("dungeons: a level range", "says the level is not known and ranks nothing", instancesSrc, n, of, "dungeon rows have no level range"))
	finishOffline(t, "places-data-gaps", "Offline places: gaps in the dataset", out)
}

// columnOf is a numeric column of each table, to read every row.
var columnOf = map[string]string{"npcs": "id", "trainers": "npc_id", "profession_tiers": "skill_required", "quests": "id"}

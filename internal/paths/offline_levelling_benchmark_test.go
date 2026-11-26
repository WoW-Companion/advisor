package paths_test

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/WoW-Companion/advisor/internal/advice"
	"github.com/WoW-Companion/advisor/internal/brief"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/dungeons"
	"github.com/WoW-Companion/advisor/internal/levelup"
	"github.com/WoW-Companion/advisor/internal/session"
	"github.com/WoW-Companion/advisor/internal/store"
	"github.com/WoW-Companion/advisor/internal/talents"
	"github.com/WoW-Companion/advisor/internal/wording"
)

// The offline class-ability, talent-point, level-pacing and XP benchmark: what the advisor says a class learns at a level,
// which talent point it names, which dungeons it sends a level to and how much quest XP it expects, beside the dataset's
// rows and the guides, with no network. testdata/levelling_snapshot (rebuild it with testdata/levelling_snapshot/build.sh)
// holds the class_spells of the nine classes, the talent trees, nodes, builds and priorities, the instances and the trainer
// spell rows. The dataset holds no talent build with levels (every build is "curated (unverified)"), no table of XP per
// level, quest XP, rested XP or zone levels, and no talent point count by level, so the cases it cannot answer check that
// the advisor says so and states none. A difference the dataset's own rows cause is marked "(data)".
// make offline-benchmark runs it; WOWC_BENCH_REPORT receives the tables.

const (
	levDir      = "../../testdata/levelling_snapshot"
	levSpellSrc = "dataset class_spells (class_id, name, rank, level, skill_line_name); Wowhead Classic class trainer lists"
	levTalSrc   = "dataset talent_builds (source, level, rank_after), talent_nodes, talent_trees"
	levDunSrc   = "dataset instances (min_level, max_level, level_source); Warcraft Tavern and Wowhead Classic dungeon level lists"
	levXPSrc    = "Warcraft Wiki, Experience point: quest XP by level difference, rested XP"
	levNoneSrc  = "the dataset has no such row, column or table"
)

var (
	levOnce  sync.Once
	levCache store.DataStore
)

// levStore reads the snapshot once for the package: the tables are decoded on first use and the tests share them.
func levStore() store.DataStore {
	levOnce.Do(func() { levCache = store.NewDir(levDir) })
	return levCache
}

func levChar(class string, level int) character.Character {
	return character.Character{Name: "Benchtest", Faction: "Horde", Class: class, Level: &level}
}

func levRows(t *testing.T, table string, class int64) []store.Row {
	t.Helper()
	rows, err := levStore().Filter(table, store.Range{Column: "class_id", Min: class, Max: class})
	if err != nil {
		t.Fatalf("%s: %v", table, err)
	}
	return rows
}

// levRow is the dataset's class_spells row of a ranked ability, or of the rank-less one when rank is 0.
func levRow(t *testing.T, class, name string, rank int64) store.Row {
	t.Helper()
	for _, r := range levRows(t, "class_spells", levelup.ClassIDs[class]) {
		if text(r["name"]) == name && num(r["rank"]) == rank {
			return r
		}
	}
	t.Fatalf("%s %s rank %d: not in the snapshot", class, name, rank)
	return nil
}

func levLabels(sp []levelup.Spell) string {
	var out []string
	for _, s := range sp {
		out = append(out, fmt.Sprintf("%s at %d", s.Label(), s.Level))
	}
	if len(out) == 0 {
		return "none"
	}
	return strings.Join(out, ", ")
}

func levHas(sp []levelup.Spell, name string) (levelup.Spell, bool) {
	for _, s := range sp {
		if s.Name == name {
			return s, true
		}
	}
	return levelup.Spell{}, false
}

// ---- class abilities by level ----

// levAbilities are class abilities the trainer teaches at a level the dataset's class_spells gives, as the Classic trainer
// lists give it too (Wowhead Classic): the Warrior's Charge at 4, the Mage's Frostbolt at 4, the Rogue's Kick at 12.
var levAbilities = []struct {
	class, name string
	rank        int64
}{
	{"WARRIOR", "Charge", 1}, {"WARRIOR", "Hamstring", 1}, {"WARRIOR", "Execute", 1}, {"MAGE", "Frostbolt", 1}, {"MAGE", "Polymorph", 2},
	{"ROGUE", "Kick", 1}, {"ROGUE", "Sprint", 1}, {"PRIEST", "Psychic Scream", 1}, {"WARLOCK", "Corruption", 2}, {"PALADIN", "Blessing of Might", 2},
}

// TestOfflineBenchmarkAbilityLevel: an ability or rank is offered in the window of the level the dataset row gives, and not in the
// window of the level before it.
func TestOfflineBenchmarkAbilityLevel(t *testing.T) {
	ds := levStore()
	var out []benchRow
	for _, tc := range levAbilities {
		row := levRow(t, tc.class, tc.name, tc.rank)
		lv := int(num(row["level"]))
		at, _ := levHas(levelup.Spells(levChar(tc.class, lv), ds, lv, lv), tc.name)
		before := levelup.Spells(levChar(tc.class, lv-1), ds, lv-1, lv-1)
		_, early := levHas(before, tc.name)
		ok := at.Rank == int(tc.rank) && at.Level == lv && !early
		out = append(out, benchRow{fmt.Sprintf("%s %s (Rank %d): the level it is offered", strings.ToLower(tc.class), tc.name, tc.rank),
			fmt.Sprintf("offered at level %d as rank %d; at level %d: %v", at.Level, at.Rank, lv-1, early),
			fmt.Sprintf("class_spells level %d, rank %d, skill line %s", lv, tc.rank, text(row["skill_line_name"])), levSpellSrc,
			resultIf(ok, "another level or rank than the row, or offered a level early")})
	}
	finish(t, "advice-ability-level", "Offline class abilities: the level each is offered", out)
}

// levNotTrained are rows the dataset lists with the abilities that no class trainer teaches: talent ranks listed at level 1, the
// passive half of a stance or form, an effect spell, and the damage half of Execute listed at level 1 (its rank 1 is level 24).
var levNotTrained = []struct{ class, name string }{
	{"WARRIOR", "Improved Battle Shout"}, {"WARRIOR", "Improved Pummel"}, {"WARRIOR", "Battle Stance Passive"}, {"WARRIOR", "Charge Rage Bonus Effect"},
	{"ROGUE", "Rogue Passive (DND)"}, {"ROGUE", "Fist Weapon Specialization"}, {"PALADIN", "Shield Specialization"}, {"DRUID", "Cat Form (Passive)"},
	{"DRUID", "Bear Form (Passive)"}, {"WARLOCK", "Hellfire Effect"}, {"MAGE", "Improved Frostbolt"},
}

// TestOfflineBenchmarkAbilityNotTrained: no window of levels 1 to 60 offers a talent, a passive or an effect row as a trainer spell,
// and the Warrior's Execute is offered once, at 24.
func TestOfflineBenchmarkAbilityNotTrained(t *testing.T) {
	ds := levStore()
	var out []benchRow
	for _, tc := range levNotTrained {
		var row store.Row
		for _, r := range levRows(t, "class_spells", levelup.ClassIDs[tc.class]) {
			if text(r["name"]) == tc.name {
				row = r
				break
			}
		}
		if row == nil {
			t.Fatalf("%s %s: not in the snapshot", tc.class, tc.name)
		}
		_, offered := levHas(levelup.Spells(levChar(tc.class, 60), ds, 1, 60), tc.name)
		_, ahead := levHas(levelup.Ahead(levChar(tc.class, 1), ds), tc.name)
		out = append(out, benchRow{fmt.Sprintf("%s %s: offered by a trainer", strings.ToLower(tc.class), tc.name),
			fmt.Sprintf("offered %v, ahead %v", offered, ahead),
			fmt.Sprintf("class_spells row at level %d, skill line %s: a talent, passive or effect, not trainer-taught", num(row["level"]), text(row["skill_line_name"])), levSpellSrc,
			resultIf(!offered && !ahead, "a talent, passive or effect listed as a trainer spell")})
	}
	_, early := levHas(levelup.Spells(levChar("WARRIOR", 23), ds, 1, 23), "Execute")
	ex, ok := levHas(levelup.Spells(levChar("WARRIOR", 24), ds, 24, 24), "Execute")
	out = append(out, benchRow{"warrior Execute: the level offered", fmt.Sprintf("rank %d at level %d; offered before 24: %v", ex.Rank, ex.Level, early), "class_spells Execute rank 1 level 24; the rank-less row at level 1 is its damage effect", levSpellSrc,
		resultIf(ok && ex.Level == 24 && ex.Rank == 1 && !early, "Execute offered at level 1")})
	finish(t, "advice-ability-not-trained", "Offline class abilities: talents, passives and effects are never trainer spells", out)
}

// TestOfflineBenchmarkAbilityAhead: the abilities Ahead lists for a level 8 Warrior are the rows from level 9 to 14, once each name.
func TestOfflineBenchmarkAbilityAhead(t *testing.T) {
	ds := levStore()
	c := levChar("WARRIOR", 8)
	got := levelup.Ahead(c, ds)
	var out []benchRow
	bad := ""
	for _, s := range got {
		if s.Level < 9 || s.Level > 8+levelup.AheadLevels || s.State != "later" {
			bad = fmt.Sprintf("%s at %d is %q", s.Label(), s.Level, s.State)
		}
	}
	_, ov := levHas(got, "Overpower")
	_, bs := levHas(got, "Battle Shout")
	out = append(out, benchRow{"warrior at level 8: abilities ahead", levLabels(got), "class_spells rows at levels 9 to 14: Bloodrage, Defensive Stance, Sunder Armor and Taunt at 10, Overpower and Shield Bash at 12", levSpellSrc,
		resultIf(len(got) > 0 && bad == "" && ov && bs, "an ability outside the window, or one missing "+bad)})
	finish(t, "advice-ability-ahead", "Offline class abilities: what is ahead of a level", out)
}

// ---- no class ability answer ----

// TestOfflineBenchmarkAbilityNoAnswer: what the dataset does not hold. A class it has no rows for has no abilities, no talent build
// and no spec; a window with no ability says there is nothing to train; and a cost is never invented (trainer_spells holds only
// recipes).
func TestOfflineBenchmarkAbilityNoAnswer(t *testing.T) {
	ds := levStore()
	var out []benchRow
	dk := levChar("DEATHKNIGHT", 60)
	sp := levelup.Spells(dk, ds, 1, 60)
	out = append(out, benchRow{"death knight: abilities", fmt.Sprintf("%d listed", len(sp)), "no class_spells row for class 6", levNoneSrc, resultIf(len(sp) == 0, "an ability was invented")})
	tp := talents.Plan(dk, ds, talents.Options{})
	out = append(out, benchRow{"death knight: a talent build", tp.Missing, "no talent_builds row for the class", levNoneSrc, resultIf(tp.Missing != "" && len(tp.Next) == 0 && tp.Spec == "", "a build was invented")})
	text1, spells, _ := levelup.Facts(levChar("MAGE", 8), ds, nil, 5, 5)
	out = append(out, benchRow{"mage: what to train at level 5", firstLine(text1), "no class_spells row at level 5", levNoneSrc, resultIf(!spells && strings.Contains(text1, "nothing to train"), "a spell was invented for a level with none")})
	text2, spells2, _ := levelup.Facts(levChar("MAGE", 20), ds, nil, 20, 20)
	out = append(out, benchRow{"mage: the training cost at level 20", firstLine(text2), "trainer_spells holds no class spell: no cost", levNoneSrc,
		resultIf(spells2 && strings.Contains(text2, "Training costs are not known for any of these: do not give a price.") && strings.Contains(text2, "usually trains"), "a price was stated, or the list was called what the trainer sells")})
	_, err := ds.Filter("class_spell_costs", store.Range{Column: "class_id", Min: int64(0), Max: int64(1)})
	out = append(out, benchRow{"a table of class spell costs", fmt.Sprint(err), "no such table in the dataset", levNoneSrc, resultIf(err != nil, "a table was invented")})
	_, err = ds.Filter("level_xp", store.Range{Column: "level", Min: int64(1), Max: int64(60)})
	out = append(out, benchRow{"a table of XP per level", fmt.Sprint(err), "no such table in the dataset", levNoneSrc, resultIf(err != nil, "a table was invented")})
	_, err = ds.Filter("zone_levels", store.Range{Column: "min_level", Min: int64(1), Max: int64(60)})
	out = append(out, benchRow{"a table of zone levels", fmt.Sprint(err), "no such table in the dataset", levNoneSrc, resultIf(err != nil, "a table was invented")})
	finish(t, "advice-levelling-no-answer", "Offline levelling: no answer in the dataset", out)
}

// ---- talent points ----

// TestOfflineBenchmarkTalentBuildChecked: for each class the advisor says it has no checked build, and names no point, because every
// talent_builds row of the class is "curated (unverified)" with no level. The dataset holds no Icy Veins leveling build yet.
func TestOfflineBenchmarkTalentBuildChecked(t *testing.T) {
	ds := levStore()
	rows, err := ds.Filter("talent_builds", store.Range{Column: "order", Min: int64(1)})
	if err != nil {
		t.Fatal(err)
	}
	var out []benchRow
	for _, class := range []string{"WARRIOR", "PALADIN", "HUNTER", "ROGUE", "PRIEST", "SHAMAN", "MAGE", "WARLOCK", "DRUID"} {
		verified, unverified := 0, 0
		for _, r := range rows {
			if strings.EqualFold(text(r["class"]), class) {
				if text(r["source"]) == "curated (unverified)" || r["level"] == nil {
					unverified++
				} else {
					verified++
				}
			}
		}
		p := talents.Plan(levChar(class, 20), ds, talents.Options{})
		ok := (verified > 0) == !p.NoBuild && (verified > 0 || len(p.Next) == 0)
		out = append(out, benchRow{strings.ToLower(class) + ": a checked talent build", fmt.Sprintf("no build %v, %d next points: %s", p.NoBuild, len(p.Next), p.Missing),
			fmt.Sprintf("%d verified rows, %d unverified rows", verified, unverified), levTalSrc, resultIf(ok, "a build called checked, or points named, with no verified row")})
		if verified == 0 {
			out = append(out, benchRow{strings.ToLower(class) + ": the leveling build", "no point named", "no row with a level and a rank (the Icy Veins WoW Forever guide is not in the dataset)", levTalSrc, "differ (data): the dataset holds no leveling build for the class"})
		}
	}
	finish(t, "advice-talent-build", "Offline talent builds: no checked build, no pick", out)
}

// TestOfflineBenchmarkTalentSpecs: the specs the advisor lists for each class are the class's talent groups the dataset's builds
// name (three each in this dataset).
func TestOfflineBenchmarkTalentSpecs(t *testing.T) {
	ds := levStore()
	rows, _ := ds.Filter("talent_builds", store.Range{Column: "order", Min: int64(1)})
	var out []benchRow
	for _, class := range []string{"WARRIOR", "MAGE", "DRUID", "PRIEST", "ROGUE", "HUNTER"} {
		want := map[string]bool{}
		for _, r := range rows {
			if strings.EqualFold(text(r["class"]), class) {
				want[text(r["spec"])] = true
			}
		}
		got := talents.Specs(ds, levChar(class, 20))
		ok := len(got) == len(want) && len(got) == 3
		for _, g := range got {
			ok = ok && want[g]
		}
		out = append(out, benchRow{strings.ToLower(class) + ": the specs", strings.Join(got, ", "), fmt.Sprintf("%d distinct specs in talent_builds", len(want)), levTalSrc, resultIf(ok, "a spec the builds do not name, or one missing")})
	}
	finish(t, "advice-talent-specs", "Offline talent specs: the trees of each class", out)
}

// TestOfflineBenchmarkTalentPointWording: the line for unspent points counts them as the capture says, and says it cannot see what is
// spent when the capture lists no talent, whatever the level.
func TestOfflineBenchmarkTalentPointWording(t *testing.T) {
	ds := levStore()
	var out []benchRow
	for _, tc := range []struct {
		level, points int
		want          string
	}{{10, 1, "Spend 1 talent point."}, {11, 2, "Spend 2 talent points."}, {30, 21, "Spend 21 talent points."}, {60, 51, "Spend 51 talent points."}} {
		c := levChar("WARRIOR", tc.level)
		c.Talents = &character.Talents{Unspent: &tc.points}
		title := advice.TalentTitle(c, ds, tc.points)
		out = append(out, benchRow{fmt.Sprintf("level %d with %d unspent points: the line", tc.level, tc.points), title,
			fmt.Sprintf("the game gives one point a level from 10, so %d points at level %d; the capture says %d", tc.level-9, tc.level, tc.points), levTalSrc,
			resultIf(strings.HasPrefix(title, tc.want[:len(tc.want)-1]) && strings.Contains(title, "can't see what you've already picked"), "another count or a pick named without the spent talents")})
	}
	for _, level := range []int{9, 10} {
		c := levChar("MAGE", level)
		p := talents.Plan(c, ds, talents.Options{})
		out = append(out, benchRow{fmt.Sprintf("level %d: talent points the character has", level), fmt.Sprintf("unspent %v, next %d, notes %v", p.Unspent, len(p.Next), p.Notes),
			"no table of points by level in the dataset", levNoneSrc, resultIf(p.Unspent == nil && len(p.Next) == 0, "a point count was invented")})
	}
	finish(t, "advice-talent-points", "Offline talent points: the count comes from the capture", out)
}

// ---- level pacing and zone for level ----

// levGuide is each dungeon's level range in two reliable guides (Warcraft Tavern's list, and Wowhead's Classic list where it
// has one) beside the dataset's curated range. A guide's range is one group's suggestion, so a gap is a dataset-side note.
var levGuide = []struct {
	name        string
	guide       [2]int
	guideSource string
}{
	{name: "Ragefire Chasm", guide: [2]int{13, 18}, guideSource: "Warcraft Tavern, Wowhead"},
	{name: "Deadmines", guide: [2]int{15, 22}, guideSource: "Warcraft Tavern"},
	{name: "Wailing Caverns", guide: [2]int{17, 24}, guideSource: "Warcraft Tavern, buffed.de"},
	{name: "Shadowfang Keep", guide: [2]int{22, 30}, guideSource: "Wowhead, buffed.de"},
	{name: "Gnomeregan", guide: [2]int{24, 34}, guideSource: "Wowhead"},
	{name: "Scarlet Monastery", guide: [2]int{26, 45}, guideSource: "Wowhead"},
}

// TestOfflineBenchmarkPacingDungeonRange: the level range the advisor states for a dungeon is the dataset's; beside the guides' it is a
// dataset-side difference when they differ.
func TestOfflineBenchmarkPacingDungeonRange(t *testing.T) {
	ds := levStore()
	var out []benchRow
	for _, g := range levGuide {
		p, _, err := dungeons.Named(levChar("MAGE", 30), ds, nil, g.name)
		if err != nil || p == nil || p.MinLevel == nil || p.MaxLevel == nil {
			t.Fatalf("%s: %v", g.name, err)
		}
		rows, _ := ds.Filter("instances", store.Range{Column: "name", Min: g.name, Max: g.name})
		lo, hi := num(rows[0]["min_level"]), num(rows[0]["max_level"])
		ref := fmt.Sprintf("instances level %d-%d (%s)", lo, hi, text(rows[0]["level_source"]))
		out = append(out, benchRow{g.name + ": the level range", fmt.Sprintf("%d-%d", *p.MinLevel, *p.MaxLevel), ref, levDunSrc, resultIf(int64(*p.MinLevel) == lo && int64(*p.MaxLevel) == hi, "another range than the row")})
		res := "match"
		if int64(g.guide[0]) != lo || int64(g.guide[1]) != hi {
			res = "differ (data): the curated range is not the guides' range"
		}
		out = append(out, benchRow{g.name + ": the range in the guides", fmt.Sprintf("dataset %d-%d", lo, hi), fmt.Sprintf("%s: %d-%d", g.guideSource, g.guide[0], g.guide[1]), levDunSrc, res})
	}
	finish(t, "advice-pacing-range", "Offline dungeon level ranges: the advisor, the dataset and the guides", out)
}

// TestOfflineBenchmarkPacingDungeonForLevel: the dungeons offered to a Horde character of a level are those of the dataset whose range
// tops out no more than three levels below the level and opens at or below it, and none of the Alliance's.
func TestOfflineBenchmarkPacingDungeonForLevel(t *testing.T) {
	ds := levStore()
	rows, _ := ds.Filter("instances", store.Range{Column: "map_id", Min: int64(0)})
	alliance := map[string]bool{"Stormwind Stockade": true}
	var out []benchRow
	for _, level := range []int{14, 20, 27, 35, 45, 52, 58} {
		res, err := dungeons.Plan(levChar("MAGE", level), ds)
		if err != nil {
			t.Fatal(err)
		}
		got := map[string]bool{}
		for _, p := range res.Picks {
			got[p.Name] = true
		}
		var want []string
		for _, r := range rows {
			lo, hi, name := num(r["min_level"]), num(r["max_level"]), text(r["name"])
			if r["min_level"] != nil && text(r["instance_type"]) == "dungeon" && int64(level) >= lo && int64(level-dungeons.LevelWindow) <= hi && !alliance[name] && name != "Ruins of Lordaeron" {
				want = append(want, name)
			}
		}
		missing := ""
		for _, w := range want {
			if !got[w] {
				missing += w + "; "
			}
		}
		extra := ""
		for n := range got {
			found := false
			for _, w := range want {
				found = found || w == n
			}
			if !found {
				extra += n + "; "
			}
		}
		var names []string
		for _, p := range res.Picks {
			names = append(names, fmt.Sprintf("%s %d-%d", p.Name, *p.MinLevel, *p.MaxLevel))
		}
		out = append(out, benchRow{fmt.Sprintf("level %d: the dungeons offered", level), strings.Join(names, ", "), fmt.Sprintf("%d dungeon rows with min_level <= %d and max_level >= %d", len(want), level, level-dungeons.LevelWindow), levDunSrc,
			resultIf(missing == "" && extra == "", "missing "+missing+" extra "+extra)})
	}
	finish(t, "advice-pacing-for-level", "Offline level pacing: the dungeons offered for a level", out)
}

// TestOfflineBenchmarkLevelFit: how the advisor words a quest or dungeon level against the player's.
func TestOfflineBenchmarkLevelFit(t *testing.T) {
	var out []benchRow
	for _, tc := range []struct {
		level, mine int
		want        string
	}{{20, 20, "Right for your level"}, {23, 20, "3 levels above you"}, {21, 20, "A level above you"}, {17, 20, "3 levels below you"}, {19, 20, "A level below you"}, {0, 20, ""}} {
		got := wording.Fit(tc.level, tc.mine)
		out = append(out, benchRow{fmt.Sprintf("level %d for a level %d character", tc.level, tc.mine), got, fmt.Sprintf("the difference %d, worded: %q", tc.level-tc.mine, tc.want), levDunSrc, resultIf(strings.EqualFold(got, tc.want), "another wording")})
	}
	finish(t, "advice-level-fit", "Offline level pacing: a level against the player's", out)
}

// TestOfflineBenchmarkPacingNoAnswer: a level with no dungeon in range says how far the first one is; a dungeon with no range in
// the dataset is listed as having none and never fitted to a level.
func TestOfflineBenchmarkPacingNoAnswer(t *testing.T) {
	ds := levStore()
	var out []benchRow
	res, _ := dungeons.Plan(levChar("MAGE", 8), ds)
	ok := len(res.Picks) == 0 && res.First != nil && res.First.Name == "Ragefire Chasm" && res.LevelsToGo == 5
	first := ""
	if res.First != nil {
		first = res.First.Name
	}
	out = append(out, benchRow{"level 8: the first dungeon", fmt.Sprintf("%d picks, first %s in %d levels", len(res.Picks), first, res.LevelsToGo), "the lowest min_level in instances: Ragefire Chasm 13, 5 levels away", levDunSrc, resultIf(ok, "a dungeon fitted below its range")})
	res, _ = dungeons.Plan(levChar("MAGE", 60), ds)
	names := ""
	for _, u := range res.Unknown {
		names += u.Name + ", "
	}
	hasCavern := strings.Contains(names, "Caverns of Time")
	fitted := false
	for _, p := range res.Picks {
		fitted = fitted || p.Name == "Caverns of Time"
	}
	out = append(out, benchRow{"level 60: Caverns of Time", fmt.Sprintf("listed with no range %v, fitted to the level %v", hasCavern, fitted), "instances row with a group size and no min_level or max_level", levNoneSrc, resultIf(hasCavern && !fitted, "a level range was invented")})
	p, _, err := dungeons.Named(levChar("MAGE", 60), ds, nil, "Caverns of Time")
	known := err == nil && p != nil && p.LevelUnknown && p.MinLevel == nil
	out = append(out, benchRow{"Caverns of Time: a level range", fmt.Sprintf("range unknown %v", known), "instances row with no level range", levNoneSrc, resultIf(known, "a range was invented")})
	for _, name := range []string{"Karazhan Crypts", "Shadow Hold"} {
		p, _, err := dungeons.Named(levChar("MAGE", 60), ds, nil, name)
		out = append(out, benchRow{name + ": described", fmt.Sprintf("%v, %v", p != nil, err), "instances row with no entrance zone, group size or level: nothing to say", levNoneSrc, resultIf(err == nil && p == nil, "a dungeon was described from no row")})
	}
	finish(t, "advice-pacing-no-answer", "Offline level pacing: no answer in the dataset", out)
}

// ---- XP and rested XP ----

// TestOfflineBenchmarkQuestXPScale: the share of a quest's XP the advisor expects by the level difference, beside the table of the
// Warcraft Wiki: full up to five levels above the quest, 80% at six, 60% at seven, 40% at eight, 20% at nine and 10% from ten; a
// quest under level 10 gives full XP only up to four levels above.
func TestOfflineBenchmarkQuestXPScale(t *testing.T) {
	var out []benchRow
	for _, tc := range []struct {
		mine, quest int
		share       float64
	}{{30, 30, 1}, {30, 25, 1}, {30, 24, 0.8}, {30, 23, 0.6}, {30, 22, 0.4}, {30, 21, 0.2}, {30, 20, 0.1}, {40, 20, 0.1}, {14, 10, 1}, {14, 9, 0.8}, {12, 9, 1}} {
		in := session.In{Char: levChar("MAGE", tc.mine), Info: session.Info{XP: func(int) (int, bool) { return 1000, true }}}
		xp, est := in.QuestXP(1, tc.quest)
		want := int(tc.share * 1000)
		out = append(out, benchRow{fmt.Sprintf("a level %d character, a level %d quest worth 1000 XP", tc.mine, tc.quest), fmt.Sprintf("%d XP, estimated %v", xp, est),
			fmt.Sprintf("%d%% of the XP: %d (level difference %d)", int(tc.share*100), want, tc.mine-tc.quest), levXPSrc, resultIf(xp == want && !est, "another share than the table")})
	}
	finish(t, "advice-xp-scale", "Offline quest XP: the share by level difference", out)
}

// TestOfflineBenchmarkQuestXPEstimate: with no quest XP table (the dataset has none) the XP is an estimate from the quest's level, flagged
// as one, never worded as exact.
func TestOfflineBenchmarkQuestXPEstimate(t *testing.T) {
	var out []benchRow
	for _, lv := range []int{5, 20, 40} {
		in := session.In{Char: levChar("MAGE", lv)}
		xp, est := in.QuestXP(1, lv)
		out = append(out, benchRow{fmt.Sprintf("a level %d quest with no XP table", lv), fmt.Sprintf("%d XP, estimated %v", xp, est), "no quest XP row in the dataset; xpDB-classic.lua is the player's, not the dataset's", levNoneSrc, resultIf(est && xp == session.Estimate(lv), "an estimate given as exact")})
	}
	finish(t, "advice-xp-estimate", "Offline quest XP: an estimate says it is one", out)
}

// TestOfflineBenchmarkProgressLine: the XP line of the character brief states what the capture holds: the XP and the level's size, the
// rested XP in points (not a percent), and says so when either is not captured. The rested pool cannot pass 150% of a level.
func TestOfflineBenchmarkProgressLine(t *testing.T) {
	var out []benchRow
	xp, max, rested, none := 450, 2100, 1500, 0
	for _, tc := range []struct {
		label string
		xp    *int
		max   *int
		rest  *int
		want  string
	}{
		{"XP and rested XP captured", &xp, &max, &rested, "XP 450/2100, rested 1500 XP"},
		{"rested XP none", &xp, &max, &none, "XP 450/2100, rested 0 XP"},
		{"rested XP not captured", &xp, &max, nil, "XP 450/2100, rested XP not captured"},
		{"XP not captured", nil, nil, &rested, "XP not captured, rested 1500 XP"},
	} {
		c := levChar("MAGE", 12)
		c.XP, c.XPMax, c.RestedXP = tc.xp, tc.max, tc.rest
		line := ""
		for _, l := range strings.Split(brief.Character(c, character.Client{}, time.Unix(0, 0)), "\n") {
			if strings.HasPrefix(l, "Progress: ") {
				line = strings.TrimPrefix(l, "Progress: ")
			}
		}
		out = append(out, benchRow{"level 12: " + tc.label, line, tc.want, levXPSrc, resultIf(line == tc.want, "another line than the capture")})
	}
	capPoints := max * 3 / 2
	out = append(out, benchRow{"rested XP pool of a level 12 mage at the cap", fmt.Sprintf("%d XP (the level is %d XP)", capPoints, max), "the pool holds at most 150% of the level: 3150 XP", levXPSrc, resultIf(capPoints == 3150, "another cap")})
	finish(t, "advice-xp-progress", "Offline XP and rested XP: the progress line", out)
}

// TestOfflineBenchmarkSessionShare: the share of the level a session's XP is, from the XP and the level's size the capture holds, and none
// when the size is not captured.
func TestOfflineBenchmarkSessionShare(t *testing.T) {
	var out []benchRow
	size := 2800
	for _, tc := range []struct {
		max  *int
		want float64
	}{{&size, 25}, {nil, 0}} {
		c := levChar("MAGE", 15)
		c.XPMax = tc.max
		in := session.In{Char: c, Info: session.Info{XP: func(int) (int, bool) { return 700, true }}}
		s := session.Plan(in, 30)
		share := s.SharePct
		out = append(out, benchRow{fmt.Sprintf("700 XP for a level size %v", tc.max != nil), fmt.Sprintf("%.1f%% of the level, %d XP", share, s.XP), fmt.Sprintf("700 / 2800 = %.0f%%; none when the size is unknown", tc.want), levXPSrc, resultIf(s.XP == 0 || share == tc.want, "another share")})
	}
	finish(t, "advice-xp-share", "Offline XP: a session's share of a level", out)
}

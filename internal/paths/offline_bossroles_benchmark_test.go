package paths_test

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/ask"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/dungeons"
	"github.com/WoW-Companion/advisor/internal/stats"
	"github.com/WoW-Companion/advisor/internal/store"
	"github.com/WoW-Companion/advisor/internal/talents"
)

// The offline boss ability, raid lockout, dungeon difficulty, group size and group role benchmark: what the advisor says when a player asks
// what a boss does, when a raid resets, whether a dungeon has a heroic mode, how many players a dungeon takes and which talent tree fits a
// tank, a healer or a damage dealer, beside the dataset's rows and the game's own rules, with no network. It reads testdata/places_snapshot
// (instances, instance_bosses), testdata/advice_snapshot (a class's trees) and the advisor's stat table. The dataset holds no boss ability,
// health, phase or enrage row, no lockout or reset, no difficulty rating or heroic mode, and no tank, healer or damage count or spec role;
// the cases of those check that the answer is framed with the line that says so (ask.LimitFacts, the lines every answer's facts carry) and
// that nothing is stated from memory. A difference the dataset's own rows cause is marked "(data)". make offline-benchmark runs it;
// WOWC_BENCH_REPORT receives the tables.

const (
	brAbilitySrc    = "dataset instance_bosses (name, order_index, difficulty_id; creature_id null on every row: no ability, spell, phase, enrage or health column); game: boss abilities are in the Dungeon Journal and guides, not in the data"
	brLockoutSrc    = "dataset instances (no reset, lockout or saved-instance column); game: raid lockouts reset on a fixed schedule and instance entries are limited per hour (Warcraft Wiki, Instance)"
	brDifficultySrc = "dataset instances, instance_bosses (difficulty_id is a mode id, not a named difficulty); game: Classic dungeons have one difficulty, no heroic mode (Warcraft Wiki, Instance)"
	brSizeSrc       = "dataset instances (max_players); game: a Classic dungeon is a five-player instance, Blackrock Spire ten, Zul'Gurub and the Ruins of Ahn'Qiraj twenty, the other raids forty (Warcraft Wiki)"
	brRoleSrc       = "advisor stat table (class, spec, role) and the talent trees of the dataset; game: Protection tanks, Holy, Discipline and Restoration heal, a Feral druid tanks as a bear and deals damage as a cat"
	brRoleNoneSrc   = "the dataset has no tank, healer or damage count, no spec role flag and no threat table (instances: group size and level range only)"
)

// brFact is the limit fact the answer's facts carry for a question: the line that says the data holds no answer.
func brFact(q string) string { return ask.LimitFacts(q) }

// brSaysNone is true when a fact says the data holds nothing and tells the answer to say so.
func brSaysNone(f, head string) bool {
	return strings.HasPrefix(f, head) && strings.Contains(f, "Say so") && strings.Contains(f, "from memory")
}

// brColumns are the columns of a table's rows, joined, for a check that none names the thing the advisor does not state.
func brColumns(ds store.DataStore, table, col string) []string {
	min := any(int64(0))
	if table == "talent_builds" {
		min = 1
	}
	rows, _ := ds.Filter(table, store.Range{Column: col, Min: min})
	seen := map[string]bool{}
	for _, r := range rows {
		for k := range r {
			seen[k] = true
		}
	}
	var out []string
	for k := range seen {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

func brHasColumn(cols []string, words ...string) string {
	for _, c := range cols {
		for _, w := range words {
			if strings.Contains(strings.ToLower(c), w) {
				return c
			}
		}
	}
	return ""
}

// brBossCases are bosses a player asks about: the question, the boss as the dataset names it and the map it is on.
var brBossCases = []struct{ q, boss, dungeon string }{
	{"what abilities does Ragnaros use?", "Ragnaros", "Molten Core"},
	{"how do I beat Onyxia, what are her phases?", "Onyxia", "Onyxia's Lair"},
	{"what does Edwin VanCleef do in the Deadmines, does he summon adds?", "Edwin VanCleef", "Deadmines"},
	{"what is Nefarian's enrage timer?", "Nefarian", "Blackwing Lair"},
	{"what should I interrupt on Hakkar?", "Hakkar", "Zul'Gurub"},
	{"how much health does Kel'Thuzad have?", "Kel'Thuzad", "Naxxramas"},
	{"what spells does Archmage Arugal cast in Shadowfang Keep?", "Archmage Arugal", "Shadowfang Keep"},
	{"how do I kill Baron Rivendare, what are his mechanics?", "Baron Rivendare", "Stratholme"},
}

// TestOfflineBenchmarkBossAbilityNoAnswer: a question about what a boss does, its phases, enrage timer, health or the way to beat it: the dataset names the boss
// and its place in the run, no more, so the answer is framed with the line that says it holds no ability, phase, timer or health and names none.
func TestOfflineBenchmarkBossAbilityNoAnswer(t *testing.T) {
	ds := instStore()
	var out []benchRow
	for _, tc := range brBossCases {
		f := brFact(tc.q)
		listed := slices.Contains(instBosses(t, ds, tc.dungeon), tc.boss)
		res := resultIf(brSaysNone(f, "Boss abilities and strategy:") && strings.Contains(f, "name no ability"), "no line saying the data holds no boss ability, so one may be made up from memory")
		got := "no line: the answer is free to name abilities"
		if f != "" {
			got = firstLine(f)[:strings.Index(firstLine(f), ":")+1] + " the data has none, say so"
		}
		ref := tc.boss + " is listed in " + tc.dungeon + "; the data holds no ability, phase, timer or health"
		if !listed {
			ref = tc.boss + " is not among " + tc.dungeon + "'s encounters of the dataset"
			if strings.HasPrefix(res, "match") {
				res = "differ (data): the dataset does not list the boss, the game has it"
			}
		}
		out = append(out, benchRow{tc.q, got, ref, brAbilitySrc, res})
	}
	cols := brColumns(ds, "instance_bosses", "map_id")
	bad := brHasColumn(cols, "abilit", "spell", "phase", "enrage", "health", "hp", "resist", "mechanic")
	out = append(out, benchRow{"instance_bosses: an ability, spell, phase, enrage or health column", fmt.Sprintf("%d columns (%s): none", len(cols), strings.Join(cols, ", ")), "no such column", brAbilitySrc, resultIf(bad == "", "a column the advisor never reads holds "+bad)})
	rows, _ := ds.Filter("instance_bosses", store.Range{Column: "map_id", Min: int64(0)})
	linked := 0
	for _, r := range rows {
		if r["creature_id"] != nil {
			linked++
		}
	}
	out = append(out, benchRow{"instance_bosses: a boss linked to a creature (its level, health and spells)", fmt.Sprintf("%d of %d encounter rows linked", linked, len(rows)), "none linked", brAbilitySrc, resultIf(linked == 0, "a linked boss is not used for its abilities")})
	finish(t, "advice-boss-abilities", "Offline boss abilities: the data names the bosses, not what they do", out)
}

// TestOfflineBenchmarkBossAbilityNotOverMatched: a question that names a boss or a spell without asking what the boss does gets no ability line, so a drop,
// an order or a class spell question is answered from its own rows.
func TestOfflineBenchmarkBossAbilityNotOverMatched(t *testing.T) {
	var out []benchRow
	for _, q := range []string{
		"which boss drops Thunderfury?", "who is the last boss of the Deadmines?", "what spells does a mage learn at level 20?",
		"what abilities does a warrior get at level 10?", "how do I kill 10 boars for the quest?", "what is the first boss in Molten Core?",
	} {
		f := brFact(q)
		out = append(out, benchRow{q, firstLine(f) + "(no boss ability line)", "no boss ability line: the question asks for a row the data has", brAbilitySrc, resultIf(!strings.Contains(f, "Boss abilities and strategy:"), "the boss ability line was added to a question the data answers")})
	}
	finish(t, "advice-boss-abilities-scope", "Offline boss abilities: questions the data answers get no such line", out)
}

// TestOfflineBenchmarkRaidResetAndEntryLimit: when a raid's lockout resets, whether an instance is saved and how many instances a character may enter an hour:
// the dataset holds none of it, so the answer is framed with the line that says so and states no day, length or count.
func TestOfflineBenchmarkRaidResetAndEntryLimit(t *testing.T) {
	ds := instStore()
	var out []benchRow
	for _, q := range []string{
		"when does the Molten Core lockout reset?", "what day does Blackwing Lair reset?", "how long is the Naxxramas lockout?",
		"am I saved to Onyxia's Lair, what is my raid ID?", "how many instances can I enter per hour?", "does Zul'Gurub reset every three days?",
	} {
		f := brFact(q)
		out = append(out, benchRow{q, firstLine(f), "no reset, lockout or entry limit in the data: say so", brLockoutSrc, resultIf(brSaysNone(f, "Raid lockouts and resets:") && strings.Contains(f, "no reset day, length or entry limit"), "no line saying the data holds no lockout, so a reset day may be made up")})
	}
	cols := brColumns(ds, "instances", "map_id")
	bad := brHasColumn(cols, "reset", "lockout", "saved", "attun")
	out = append(out, benchRow{"instances: a reset, lockout or attunement column", fmt.Sprintf("%d columns: none", len(cols)), "no such column", brLockoutSrc, resultIf(bad == "", "a column the advisor never reads holds "+bad)})
	finish(t, "advice-raid-reset-entry-limit", "Offline raid reset and entry limit: not in the dataset", out)
}

// brAltModeMaps are maps the dataset keeps alternate-mode encounters for (difficulty_id above 1).
var brAltModeMaps = []string{"Blackfathom Deeps", "Gnomeregan", "Sunken Temple"}

var brHeroicWords = regexp.MustCompile(`(?i)\b(heroic|mythic|hard ?mode|normal mode|difficulty)\b`)

// TestOfflineBenchmarkDungeonDifficulty: a question about a heroic mode or a dungeon's difficulty is framed with the line that the data has no difficulty, and a
// dungeon whose map has alternate-mode encounters is not worded as heroic or hard.
func TestOfflineBenchmarkDungeonDifficulty(t *testing.T) {
	ds := instStore()
	var out []benchRow
	for _, q := range []string{
		"does Stratholme have a heroic mode?", "how difficult is Blackrock Depths?", "is Scarlet Monastery too hard for a group of level 38?",
		"is there a normal and a heroic Dire Maul?", "how hard is Molten Core for a first raid?",
	} {
		f := brFact(q)
		out = append(out, benchRow{q, firstLine(f), "no difficulty rating or heroic mode in the data: say so", brDifficultySrc, resultIf(brSaysNone(f, "Dungeon difficulty and modes:") && strings.Contains(f, "no difficulty rating"), "no line saying the data holds no difficulty, so a heroic or easy rating may be made up")})
	}
	c := character.Character{Faction: "Horde", Class: "WARRIOR"}
	for _, name := range brAltModeMaps {
		m := rowByName(t, ds, "instances", name)
		rows, _ := ds.Filter("instance_bosses", store.Range{Column: "map_id", Min: num(m["map_id"]), Max: num(m["map_id"])})
		alt := 0
		for _, r := range rows {
			if num(r["difficulty_id"]) > 1 {
				alt++
			}
		}
		p, togo, err := dungeons.Named(c, ds, nil, name)
		txt := ""
		if err == nil && p != nil {
			txt = dungeons.RenderNamed(c, *p, togo)
		}
		out = append(out, benchRow{name + ": the answer for a dungeon with " + fmt.Sprint(alt) + " alternate-mode encounter rows", "heroic/difficulty words: " + fmt.Sprint(brHeroicWords.FindString(txt) != ""), "no heroic or difficulty words: the alternate rows are a mode id, not a named difficulty", brDifficultySrc,
			resultIf(alt > 0 && p != nil && !brHeroicWords.MatchString(txt), "the dungeon was worded as heroic or by a difficulty")})
	}
	finish(t, "advice-dungeon-difficulty", "Offline dungeon difficulty: no heroic mode or rating in the dataset", out)
}

var brSizes = []struct {
	name  string
	guide int64
}{
	{"Wailing Caverns", 5}, {"Shadowfang Keep", 5}, {"Stormwind Stockade", 5}, {"Uldaman", 5}, {"Maraudon", 5}, {"Scholomance", 5}, {"Stratholme", 5}, {"Dire Maul", 5},
	{"Blackrock Spire", 10}, {"Zul'Gurub", 20}, {"Ruins of Ahn'Qiraj", 20}, {"Molten Core", 40}, {"Ahn'Qiraj Temple", 40}, {"Naxxramas", 40},
}

// TestOfflineBenchmarkGroupSizeOfInstances: how many players a dungeon or raid takes, as the answer words it, beside the dataset's max_players and the game's size:
// a size the dataset gets wrong is a data difference.
func TestOfflineBenchmarkGroupSizeOfInstances(t *testing.T) {
	ds := instStore()
	c := character.Character{Faction: "Horde", Class: "WARRIOR"}
	var out []benchRow
	for _, tc := range brSizes {
		m := rowByName(t, ds, "instances", tc.name)
		size := num(m["max_players"])
		p, togo, err := dungeons.Named(c, ds, nil, tc.name)
		if err != nil || p == nil || p.MaxPlayers == nil {
			out = append(out, benchRow{tc.name + ": group size", "no answer", fmt.Sprintf("%d players", tc.guide), brSizeSrc, "differ: the instance or its size was not found"})
			continue
		}
		txt := dungeons.RenderNamed(c, *p, togo)
		res := "match"
		switch {
		case int64(*p.MaxPlayers) != size || !strings.Contains(txt, fmt.Sprintf("%d players", size)):
			res = "differ: the answer words another size than the row"
		case size != tc.guide:
			res = fmt.Sprintf("differ (data): the dataset sizes it at %d players, the game at %d", size, tc.guide)
		}
		out = append(out, benchRow{tc.name + ": group size", fmt.Sprintf("%d players", *p.MaxPlayers), fmt.Sprintf("%d players (dataset %d)", tc.guide, size), brSizeSrc, res})
	}
	finish(t, "advice-instance-group-size", "Offline dungeon and raid group size", out)
}

var brRoles = []string{"tank", "healer", "damage"}

// TestOfflineBenchmarkRoleSpecOrder: the trees offered a tank, a healer and a damage dealer of each class come with the trees that fit the role first, the same
// trees the advisor's stat table gives that role. A tree the stat table has no profile for (a data tree) is left out of the comparison.
func TestOfflineBenchmarkRoleSpecOrder(t *testing.T) {
	ds := store.NewDir(qbAdviceDir)
	var out []benchRow
	for _, class := range []string{"WARRIOR", "PALADIN", "HUNTER", "ROGUE", "PRIEST", "SHAMAN", "MAGE", "WARLOCK", "DRUID"} {
		specs := talents.Specs(ds, character.Character{Class: class})
		var gots, wants []string
		ok := true
		for _, role := range brRoles {
			want := map[string]bool{}
			for _, s := range specs {
				if brFits(class, s, role) {
					want[s] = true
				}
			}
			order := ask.OrderSpecs(specs, role)
			lead := order
			if len(want) < len(order) {
				// the leading trees that the stat table knows: a tree it holds no profile for is a data tree and not counted
				lead = nil
				for _, s := range order {
					if want[s] || brStatKnown(class, s) {
						lead = append(lead, s)
					}
					if len(lead) == len(want) {
						break
					}
				}
			}
			for _, s := range lead {
				if len(want) > 0 && !want[s] {
					ok = false
				}
			}
			gots = append(gots, role+": "+strings.Join(order, " "))
			var w []string
			for _, s := range specs {
				if want[s] {
					w = append(w, s)
				}
			}
			wants = append(wants, role+": "+strings.Join(w, " "))
		}
		out = append(out, benchRow{class + ": the trees offered a tank, a healer and a damage dealer", strings.Join(gots, "; "), strings.Join(wants, "; ") + " (first)", brRoleSrc, resultIf(ok, "a tree that does not fit the role is offered before one that does")})
	}
	finish(t, "advice-role-spec-order", "Offline group roles: the trees offered each role", out)
}

// brStatKnown is true when the stat table holds a profile for the tree under any role.
func brStatKnown(class, spec string) bool {
	for _, role := range brRoles {
		if brFits(class, spec, role) {
			return true
		}
	}
	return false
}

// TestOfflineBenchmarkRoleHybridTrees: a tree that is a role's tree by the stat table is among the first offered that role: a Feral Combat druid is offered
// first to a tank and to a damage dealer, a Protection warrior and paladin to a tank only, a Holy paladin and priest to a healer only.
func TestOfflineBenchmarkRoleHybridTrees(t *testing.T) {
	ds := store.NewDir(qbAdviceDir)
	var out []benchRow
	for _, tc := range []struct {
		class, spec, role string
		first             bool
	}{
		{"DRUID", "Feral Combat", "tank", true}, {"DRUID", "Feral Combat", "damage", true}, {"DRUID", "Restoration", "healer", true}, {"DRUID", "Feral Combat", "healer", false},
		{"WARRIOR", "Protection", "tank", true}, {"WARRIOR", "Protection", "damage", false}, {"PALADIN", "Holy", "healer", true}, {"PALADIN", "Retribution", "healer", false},
		{"PRIEST", "Shadow Magic", "damage", true}, {"SHAMAN", "Restoration", "healer", true},
	} {
		specs := talents.Specs(ds, character.Character{Class: tc.class})
		order := ask.OrderSpecs(specs, tc.role)
		pos := slices.Index(order, tc.spec)
		// a tree that fits the role has every tree ahead of it fit the role too; one that does not has every fitting tree ahead of it
		ok := pos >= 0 && brFits(tc.class, tc.spec, tc.role) == tc.first
		for i, s := range order {
			switch {
			case tc.first && i < pos && !brFits(tc.class, s, tc.role) && brStatKnown(tc.class, s):
				ok = false
			case !tc.first && i > pos && brFits(tc.class, s, tc.role):
				ok = false
			}
		}
		want := fmt.Sprintf("%s offered a %s %s", tc.spec, tc.role, map[bool]string{true: "among the first (the stat table lists it for the role)", false: "after the trees that fit (the stat table does not list it for the role)"}[tc.first])
		out = append(out, benchRow{fmt.Sprintf("%s %s for a %s", tc.class, tc.spec, tc.role), strings.Join(order, " "), want, brRoleSrc, resultIf(ok, "the tree is on the wrong side of the role's trees")})
	}
	// the trees in the order a capture lists them, not the dataset's alphabetical order: the damage trees of a druid, Feral Combat and Balance, come before Restoration
	for _, role := range []string{"damage", "tank"} {
		order := ask.OrderSpecs([]string{"Restoration", "Feral Combat", "Balance"}, role)
		first := slices.Index(order, "Restoration")
		want := "Restoration after Feral Combat and Balance (both the stat table's damage trees)"
		ok := first == 2
		if role == "tank" {
			want = "Feral Combat first (the stat table's tank tree), the others in the order given"
			ok = order[0] == "Feral Combat"
		}
		out = append(out, benchRow{"DRUID trees listed Restoration, Feral Combat, Balance for a " + role, strings.Join(order, " "), want, brRoleSrc, resultIf(ok, "a healing tree is offered ahead of a damage tree, or Feral Combat is not first for a tank")})
	}
	finish(t, "advice-role-hybrid-trees", "Offline group roles: hybrid and role trees", out)
}

// brFits is true when the stat table gives the tree a profile for the role.
func brFits(class, spec, role string) bool {
	p, ok := stats.Lookup(class, spec, role)
	return ok && p.Role == role && p.Spec != ""
}

// TestOfflineBenchmarkRolesNoAnswer: the roles a dungeon or raid needs, the threat a tank must hold and a role on a spec: the dataset holds a group size and a level
// range, no tank, healer or damage count, no threat and no spec role, so the answer is framed with the line that says so and states none.
func TestOfflineBenchmarkRolesNoAnswer(t *testing.T) {
	ds := instStore()
	var out []benchRow
	for _, q := range []string{
		"how many tanks and healers do I need for Zul'Gurub?", "what group composition works for Stratholme?", "do I need a tank and a healer for Wailing Caverns?",
		"how much threat does a warrior generate per second?", "what raid composition do I need for Molten Core, how many healers?",
	} {
		f := brFact(q)
		out = append(out, benchRow{q, firstLine(f), "no role count, composition or threat value in the data: say so", brRoleNoneSrc, resultIf(brSaysNone(f, "Group roles and threat:") && strings.Contains(f, "no group composition or threat figure"), "no line saying the data holds no roles, so a composition or threat figure may be made up")})
	}
	cols := brColumns(ds, "instances", "map_id")
	bad := brHasColumn(cols, "role", "tank", "heal", "dps", "threat")
	out = append(out, benchRow{"instances: a tank, healer, damage or threat column", fmt.Sprintf("%d columns: none", len(cols)), "no such column", brRoleNoneSrc, resultIf(bad == "", "a column the advisor never reads holds "+bad)})
	tb := store.NewDir(qbAdviceDir)
	cols = brColumns(tb, "talent_builds", "order")
	bad = brHasColumn(cols, "role", "tank", "heal", "threat")
	out = append(out, benchRow{"talent_builds: a role column on a spec", fmt.Sprintf("%d columns: none", len(cols)), "no role column: the role of a tree is the advisor's own table", brRoleNoneSrc, resultIf(len(cols) > 0 && bad == "", "a role column the advisor does not read")})
	finish(t, "advice-roles-no-answer", "Offline group roles and threat: no answer in the dataset", out)
}

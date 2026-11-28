package paths_test

import (
	"bytes"
	"fmt"
	"io"
	"sort"
	"strings"
	"testing"

	"github.com/parquet-go/parquet-go"

	"github.com/WoW-Companion/advisor/internal/advice"
	"github.com/WoW-Companion/advisor/internal/ask"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/store"
	"github.com/WoW-Companion/advisor/internal/talents"
)

// The offline talent, specialization and zone-leveling sweep: what the advisor says to synthetic characters (all nine classes, levels
// across 1 to 60, both factions) about their talent points and the zone to level in, beside the dataset's rows and the game's rules, with
// no network. testdata/levelling_snapshot holds the talent trees, nodes and banded builds and testdata/world_snapshot the ui_maps. The
// dataset holds no verified Icy Veins build (every row is "curated (unverified)"), so the points part writes the snapshot's real nodes and
// band order as one row per point (point i at level 9+i, the game's one talent point per level from 10) in memory, as the talent
// benchmark does, and the plain no-build answer is checked on the snapshot itself. The reference is written apart from the advisor: the
// game gives one point per level from level 10 (a level-L character has L-9), a spec's role by the game's own trees, and the dataset has
// no zone level range, so a "where do I level" question must reach the line that says so, in whatever words a player uses. A difference
// the dataset's own rows cause is marked "(data)". make offline-benchmark runs it; WOWC_BENCH_REPORT receives the tables.

const (
	tzPointSrc = "game rule: one talent point per level from level 10; dataset talent_builds (order, points, node_id), talent_nodes (fixture rows in the per-point schema)"
	tzRoleSrc  = "dataset talent_trees (class, name); Warcraft Wiki Classic talent trees, role of each: tank Protection/Feral Combat, healer Holy/Discipline/Restoration"
	tzZoneSrc  = "dataset ui_maps (no level column), quests_observed; Warcraft Wiki zone level brackets (Classic)"
)

var (
	tzClasses = []string{"WARRIOR", "PALADIN", "HUNTER", "ROGUE", "PRIEST", "SHAMAN", "MAGE", "WARLOCK", "DRUID"}
	tzLevels  = []int{1, 9, 10, 11, 20, 30, 39, 50, 60}
)

// tzSide is the side a class is played on in the era: paladins are Alliance, shamans Horde, the rest either (alternating by class).
func tzSide(class string, i int) string {
	switch class {
	case "PALADIN":
		return "Alliance"
	case "SHAMAN":
		return "Horde"
	}
	return []string{"Alliance", "Horde"}[i%2]
}

func tzTitle(class string) string { return class[:1] + strings.ToLower(class[1:]) }

func tzChar(class string, i, level int) character.Character {
	return character.Character{Name: "Benchtest", Faction: tzSide(class, i), Class: class, Level: &level}
}

type tzPointRow struct {
	Class     string `parquet:"class"`
	Spec      string `parquet:"spec"`
	Order     int16  `parquet:"order"`
	NodeID    int32  `parquet:"node_id"`
	Level     int16  `parquet:"level"`
	Name      string `parquet:"talent_name"`
	RankAfter int16  `parquet:"rank_after"`
	Source    string `parquet:"source"`
}

// tzBuilds expands every spec of the snapshot's banded builds into one row per point (point i at level 9+i), and returns the store with those
// rows beside the snapshot's other tables, the spec names by class and the points of each spec.
func tzBuilds(t *testing.T) (ds store.DataStore, specs map[string][]string, points map[string]int) {
	t.Helper()
	snap := levStore()
	rows, err := snap.Filter("talent_builds", store.Range{Column: "order", Min: 1})
	if err != nil {
		t.Fatal(err)
	}
	names := map[int64]string{}
	nodes, err := snap.Filter("talent_nodes", store.Range{Column: "node_id", Min: 1})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range nodes {
		names[num(r["node_id"])] = text(r["name"])
	}
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if text(a["class"]) != text(b["class"]) {
			return text(a["class"]) < text(b["class"])
		}
		if text(a["spec"]) != text(b["spec"]) {
			return text(a["spec"]) < text(b["spec"])
		}
		return num(a["order"]) < num(b["order"])
	})
	specs, points = map[string][]string{}, map[string]int{}
	var out []tzPointRow
	ranks := map[string]int{}
	for _, r := range rows {
		cl, sp := text(r["class"]), text(r["spec"])
		key := cl + "/" + sp
		if points[key] == 0 {
			specs[strings.ToUpper(cl)] = append(specs[strings.ToUpper(cl)], sp)
		}
		for k := int64(0); k < num(r["points"]); k++ {
			id := num(r["node_id"])
			ranks[fmt.Sprint(key, id)]++
			points[key]++
			out = append(out, tzPointRow{cl, sp, int16(points[key]), int32(id), int16(9 + points[key]), names[id], int16(ranks[fmt.Sprint(key, id)]), "icy-veins.com WoW Forever leveling guides (fixture rows)"})
		}
	}
	var buf bytes.Buffer
	if err := parquet.Write(&buf, out); err != nil {
		t.Fatal(err)
	}
	mem := store.ReaderSource{"talent_builds": {ReaderAt: bytes.NewReader(buf.Bytes()), Size: int64(buf.Len())}}
	return store.New(tzSource{mem, snap}), specs, points
}

// tzSource serves talent_builds from memory and every other table from the snapshot's store.
type tzSource struct {
	mem  store.ReaderSource
	snap store.DataStore
}

func (s tzSource) Open(table string) (io.ReaderAt, int64, error) {
	if _, ok := s.mem[table]; ok {
		return s.mem.Open(table)
	}
	return store.DirSource(levDir).Open(table)
}

// TestOfflineBenchmarkSweepTalentPoints: a character of every class at every level who has taken nothing is told the build's first points,
// as many as the game's one point per level from 10 allows; below level 10 none; past the build's last point the advisor says the build ends.
func TestOfflineBenchmarkSweepTalentPoints(t *testing.T) {
	ds, specs, points := tzBuilds(t)
	var out []benchRow
	for ci, class := range tzClasses {
		for _, level := range tzLevels {
			c := tzChar(class, ci, level)
			spec := specs[class][0]
			p := talents.Plan(c, ds, talents.Options{Spec: spec, Limit: 100})
			n := points[tzTitle(class)+"/"+spec]
			want := min(max(level-9, 0), n)
			res := resultIf(p.Spec == spec && p.Remaining == want && len(p.Next) == want, "the points due at this level")
			ref := fmt.Sprintf("%d point(s) due (level %d gives %d, the build has %d)", want, level, max(level-9, 0), n)
			got := fmt.Sprintf("%d due", p.Remaining)
			notes := strings.Join(p.Notes, " ")
			if level < 10 && !strings.Contains(notes, "starts at level 10") {
				res = "differ: no note that the build starts at level 10"
			}
			out = append(out, benchRow{fmt.Sprintf("%s %s (%s) level %d, nothing taken", class, spec, c.Faction, level), got, ref, tzPointSrc, res})
		}
	}
	finish(t, "talents-sweep-points", "Offline talents sweep: points due at every level of every class", out)
}

// TestOfflineBenchmarkSweepBuildEnds: a character who has taken every point of the build is told, at every level from the build's last, that
// the build ends there and has no pick for the points after it; the game gives 51 points at level 60, the dataset's build 30.
func TestOfflineBenchmarkSweepBuildEnds(t *testing.T) {
	ds, specs, points := tzBuilds(t)
	rows, err := ds.Filter("talent_builds", store.Range{Column: "order", Min: 1})
	if err != nil {
		t.Fatal(err)
	}
	var out []benchRow
	for ci, class := range tzClasses {
		spec := specs[class][0]
		var taken []character.Talent
		ranks := map[int64]int{}
		for _, r := range rows {
			if strings.EqualFold(text(r["class"]), class) && text(r["spec"]) == spec {
				id := int(num(r["node_id"]))
				ranks[int64(id)] = int(num(r["rank_after"]))
				found := false
				for i := range taken {
					if *taken[i].NodeID == id {
						taken[i].Rank, found = ranks[int64(id)], true
					}
				}
				if !found {
					taken = append(taken, character.Talent{Name: text(r["talent_name"]), Rank: ranks[int64(id)], NodeID: &id})
				}
			}
		}
		for _, level := range []int{39, 45, 60} {
			c := tzChar(class, ci, level)
			unspent := level - 9 - points[tzTitle(class)+"/"+spec]
			c.Talents = &character.Talents{Unspent: &unspent, Trees: []character.TalentTree{{Name: spec, Groups: []character.TalentGroup{{Name: spec, Spent: points[tzTitle(class)+"/"+spec]}}, Talents: taken}}}
			p := talents.Plan(c, ds, talents.Options{Spec: spec})
			note := strings.Join(p.Notes, " ")
			out = append(out, benchRow{fmt.Sprintf("%s %s (%s) level %d, all %d build points taken, %d unspent", class, spec, c.Faction, level, points[tzTitle(class)+"/"+spec], unspent), fmt.Sprintf("%d next; %s", len(p.Next), note), "the build ends at level 39 with 30 points; the game gives level-9 points, so the rest have no build pick: say the build ends", tzPointSrc, resultIf(len(p.Next) == 0 && strings.Contains(note, "ends at level 39") && !strings.Contains(note, "nothing more to take"), "the build is said to have nothing more instead of ending")})
		}
	}
	finish(t, "talents-sweep-build-ends", "Offline talents sweep: a finished build says it ends", out)
}

// tzRoles is the specs the game gives a role for each class: the answer to "I tank" or "I heal" must be one of them, or say the class has none.
var tzRoles = map[string]map[string][]string{
	"WARRIOR": {"tank": {"Protection"}, "damage": {"Arms", "Fury"}},
	"PALADIN": {"tank": {"Protection"}, "healer": {"Holy"}, "damage": {"Retribution"}},
	"HUNTER":  {"damage": {"Beast Mastery", "Marksmanship", "Survival"}},
	"ROGUE":   {"damage": {"Assassination", "Combat", "Subtlety"}},
	"PRIEST":  {"healer": {"Discipline", "Holy"}, "damage": {"Shadow Magic"}},
	"SHAMAN":  {"healer": {"Restoration"}, "damage": {"Elemental", "Enhancement"}},
	"MAGE":    {"damage": {"Arcane", "Fire", "Frost"}},
	"WARLOCK": {"damage": {"Affliction", "Demonology", "Destruction"}},
	"DRUID":   {"tank": {"Feral Combat"}, "healer": {"Restoration"}, "damage": {"Balance", "Feral Combat"}},
}

// TestOfflineBenchmarkSweepRoleSpec: the stored role (tank, healer, damage) picks a spec the game gives that role, for every class; a role
// the class has no spec for is said to have none.
func TestOfflineBenchmarkSweepRoleSpec(t *testing.T) {
	ds, _, _ := tzBuilds(t)
	var out []benchRow
	for ci, class := range tzClasses {
		for _, role := range []string{"tank", "healer", "damage"} {
			c := tzChar(class, ci, 30)
			c.Profile = &character.Profile{Role: role, Focus: "leveling"}
			p := talents.Plan(c, ds, talents.Options{})
			allowed := tzRoles[class][role]
			note := strings.Join(p.Notes, " ")
			ok := false
			for _, a := range allowed {
				ok = ok || a == p.Spec
			}
			ref := strings.Join(allowed, " or ")
			if len(allowed) == 0 {
				ok = strings.Contains(note, "no "+role+" talent tree")
				ref = "no " + role + " spec: say so"
			}
			out = append(out, benchRow{fmt.Sprintf("%s (%s) stored role %q", class, tzSide(class, ci), role), p.Spec + " " + note, ref, tzRoleSrc, resultIf(ok, "a spec of another role, or no word that the class has none")})
		}
	}
	finish(t, "talents-sweep-role-spec", "Offline talents sweep: the spec a stored role picks, every class", out)
}

// TestOfflineBenchmarkSweepSpecNames: the specs the advisor lists for each class (the play style dropdown) against the game's three trees.
func TestOfflineBenchmarkSweepSpecNames(t *testing.T) {
	game := map[string]string{
		"WARRIOR": "Arms Fury Protection", "PALADIN": "Holy Protection Retribution", "HUNTER": "Beast Mastery Marksmanship Survival",
		"ROGUE": "Assassination Combat Subtlety", "PRIEST": "Discipline Holy Shadow Magic", "SHAMAN": "Elemental Enhancement Restoration",
		"MAGE": "Arcane Fire Frost", "WARLOCK": "Affliction Demonology Destruction", "DRUID": "Balance Feral Combat Restoration",
	}
	var out []benchRow
	for ci, class := range tzClasses {
		got := strings.Join(talents.Specs(levStore(), tzChar(class, ci, 30)), " ")
		out = append(out, benchRow{class + ": spec names", got, game[class], tzRoleSrc, resultIf(got == game[class], "the spec names")})
	}
	finish(t, "talents-sweep-spec-names", "Offline talents sweep: the spec names of every class", out)
}

// TestOfflineBenchmarkSweepNoBuild: on the snapshot itself (no verified build) every class at every level is told there is no checked build
// and is given no talent name to pick; the milestone title for unspent points names none either.
func TestOfflineBenchmarkSweepNoBuild(t *testing.T) {
	var out []benchRow
	for ci, class := range tzClasses {
		for _, level := range []int{12, 30, 55} {
			c := tzChar(class, ci, level)
			unspent := level - 9
			c.Talents = &character.Talents{Unspent: &unspent, Trees: []character.TalentTree{{Name: "x", Groups: []character.TalentGroup{{Name: "x", Spent: 1}}, Talents: []character.Talent{{Name: "Placeholder", Rank: 1}}}}}
			p := talents.Plan(c, levStore(), talents.Options{})
			title := advice.TalentTitle(c, levStore(), unspent)
			bad := !p.NoBuild || len(p.Next) != 0 || strings.Contains(title, ":")
			out = append(out, benchRow{fmt.Sprintf("%s (%s) level %d, %d unspent", class, c.Faction, level, unspent), p.Missing + " / " + title, "no verified build in the dataset: no pick named", "dataset talent_builds (source \"curated (unverified)\")", resultIf(!bad, "a talent pick from an unverified build")})
		}
	}
	finish(t, "talents-sweep-no-build", "Offline talents sweep: no verified build, no pick", out)
}

// tzZoneAsks are the words a player uses to ask where to level at a level; {L} is the level, {C} a class and {Z} a zone.
var tzZoneAsks = []string{
	"where should I level at level {L}?", "where should my level {L} {C} quest?", "best zone to level at {L}", "best place to level at level {L}",
	"I'm level {L}, where do I quest?", "where to level at {L}", "where should I be leveling at {L}", "where do I go at level {L}?",
	"which zones are good for a level {L} {C}?", "what zone should I quest in at level {L}?", "where should a level {L} {C} go?",
	"I just hit {L}, where next?", "is {Z} good for level {L}?", "should I go to {Z} at level {L}?", "where should I level up at {L}",
}

// TestOfflineBenchmarkSweepWhereToLevel: every phrasing, at every level, for every class and both sides, reaches the line that the data holds
// no zone level range, so no zone is named from memory.
func TestOfflineBenchmarkSweepWhereToLevel(t *testing.T) {
	var out []benchRow
	for _, tpl := range tzZoneAsks {
		missed := 0
		total := 0
		var first string
		for ci, class := range tzClasses {
			// each class asks at one of six levels, rotated across the classes
			for _, level := range []int{[]int{8, 14, 22, 33, 47, 55}[(ci+len(tpl))%6]} {
				side := tzSide(class, ci)
				zone := map[string]string{"Alliance": "Duskwood", "Horde": "Ashenvale"}[side]
				q := strings.NewReplacer("{L}", fmt.Sprint(level), "{C}", strings.ToLower(class), "{Z}", zone).Replace(tpl)
				total++
				if !strings.Contains(ask.LimitFacts(q), "Zone levels:") {
					missed++
					if first == "" {
						first = q
					}
				}
			}
		}
		got := fmt.Sprintf("%d of %d reach the zone level line", total-missed, total)
		if missed > 0 {
			got += "; missed: " + first
		}
		out = append(out, benchRow{fmt.Sprintf("%q, nine classes, both sides, levels 8 to 55", tpl), got, "the dataset has no zone level range: say so (ui_maps has no level column)", tzZoneSrc, resultIf(missed == 0, "a phrasing that does not reach the line")})
	}
	finish(t, "talents-sweep-where-to-level", "Offline zones sweep: where to level, in a player's words", out)
}

// TestOfflineBenchmarkSweepNotWhereToLevel: a profession, an item or a quest question that names a level is no zone question.
func TestOfflineBenchmarkSweepNotWhereToLevel(t *testing.T) {
	var out []benchRow
	for _, q := range []string{
		"where do I level my mining?", "where do I level up my first aid to 150?", "best place to level mining at 25", "what quests should I do at level 20?",
		"where do I find a level 20 quest?", "where can I buy a level 30 weapon?", "is this sword good for level 25?", "what is the best gear at level 40?",
		"where do I learn level 20 spells", "where do I get the level 20 mage quest?",
	} {
		f := ask.LimitFacts(q)
		out = append(out, benchRow{fmt.Sprintf("%q", q), "no zone level line: " + fmt.Sprint(!strings.Contains(f, "Zone levels:")), "not a zone bracket question", tzZoneSrc, resultIf(!strings.Contains(f, "Zone levels:"), "a zone line for a question that is none")})
	}
	finish(t, "talents-sweep-not-where-to-level", "Offline zones sweep: level questions that are not about a zone", out)
}

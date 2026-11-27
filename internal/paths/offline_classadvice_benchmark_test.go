package paths_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/ask"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/gear"
	"github.com/WoW-Companion/advisor/internal/levelup"
	"github.com/WoW-Companion/advisor/internal/store"
)

// The offline class-spell and weapon-skill benchmark, second part: what a character who already knows a spell is told
// when the next rank comes, which rank of a spell the advisor lists over a span of levels, what it says past the last
// level, when a weapon master teaches a skill (the level it needs), who can wield a weapon by the skills the dataset
// says each class learns, the nearest master and the client's skill names, beside the dataset's class_spells,
// weapon_skills and weapon_masters rows, with no network. It reads testdata/usability_snapshot. The dataset's rank rows
// carry no link to the rank before for most classes (superseded_spell_id is empty for 1014 ranked rows), so a rank is
// told from its number. make offline-benchmark runs it; WOWC_BENCH_REPORT receives the tables.

const (
	clsSpellSrc  = "dataset class_spells (name, rank, level, spell_id); Wowhead Classic class trainer lists"
	clsWeaponSrc = "dataset weapon_skills (learnable, min_level), weapon_masters (city, cost_copper, min_level)"
	clsNoneSrc   = "the dataset has no such row"
)

// clsRow is the dataset's class_spells row of a ranked spell.
func clsRow(t *testing.T, class, name string, rank int64) store.Row {
	t.Helper()
	for _, r := range classRows(t, classID(class), 1, 60) {
		if text(r["name"]) == name && num(r["rank"]) == rank {
			return r
		}
	}
	t.Fatalf("%s %s rank %d: not in the snapshot", class, name, rank)
	return nil
}

// TestOfflineBenchmarkClassSpellNewRank: a character whose capture lists the spell's rank before knows the spell: the
// advisor calls the next rank "a new rank of a spell you have", not a new spell, whether or not the dataset links the two.
func TestOfflineBenchmarkClassSpellNewRank(t *testing.T) {
	ds := usabStore()
	var out []benchRow
	for _, tc := range []struct {
		class, name string
		rank        int64
	}{
		{"MAGE", "Frostbolt", 2}, {"PRIEST", "Renew", 2}, {"DRUID", "Rejuvenation", 2}, {"HUNTER", "Arcane Shot", 2},
		{"WARLOCK", "Immolate", 2}, {"SHAMAN", "Healing Wave", 2}, {"ROGUE", "Sinister Strike", 2}, {"WARRIOR", "Rend", 2},
	} {
		prev, row := clsRow(t, tc.class, tc.name, tc.rank-1), clsRow(t, tc.class, tc.name, tc.rank)
		lv := int(num(row["level"]))
		c := usabChar(tc.class, lv, "Horde")
		c.KnownSpells = map[int64]bool{num(prev["spell_id"]): true}
		facts, _, _ := levelup.Facts(c, ds, nil, lv, lv)
		line := ""
		for _, l := range strings.Split(facts, "\n") {
			if strings.Contains(l, tc.name+" (Rank 2)") {
				line = l
			}
		}
		ok := strings.Contains(line, "new rank of a spell you have") && strings.Contains(line, fmt.Sprintf("Level %d:", lv))
		out = append(out, benchRow{fmt.Sprintf("%s knows %s rank %d: rank %d at level %d", strings.ToLower(tc.class), tc.name, tc.rank-1, tc.rank, lv), line,
			fmt.Sprintf("class_spells: %s rank %d, level %d; rank %d is already known", tc.name, tc.rank, lv, tc.rank-1), clsSpellSrc, resultIf(ok, "a new spell, or another level")})
	}
	finish(t, "advice-class-spell-new-rank", "Offline class trainer: a new rank is not a new spell", out)
}

// TestOfflineBenchmarkClassSpellKnownLeft: a spell the capture says the character knows is left out of what to train, and
// the ranks above it stay.
func TestOfflineBenchmarkClassSpellKnownLeft(t *testing.T) {
	ds := usabStore()
	var out []benchRow
	for _, tc := range []struct {
		class, name string
		from, to    int
	}{{"MAGE", "Frostbolt", 4, 8}, {"WARLOCK", "Immolate", 1, 10}, {"PRIEST", "Renew", 8, 14}} {
		c := usabChar(tc.class, tc.to, "Alliance")
		first := clsRow(t, tc.class, tc.name, 1)
		c.KnownSpells = map[int64]bool{num(first["spell_id"]): true}
		var got []string
		for _, s := range levelup.Spells(c, ds, tc.from, tc.to) {
			if s.Name == tc.name {
				got = append(got, fmt.Sprintf("rank %d at %d", s.Rank, s.Level))
			}
		}
		var want []string
		for _, r := range classRows(t, classID(tc.class), tc.from, tc.to) {
			if text(r["name"]) == tc.name && num(r["rank"]) > 1 {
				want = append(want, fmt.Sprintf("rank %d at %d", num(r["rank"]), num(r["level"])))
			}
		}
		slices.Sort(want)
		slices.Sort(got)
		out = append(out, benchRow{fmt.Sprintf("%s knows %s rank 1: levels %d to %d", strings.ToLower(tc.class), tc.name, tc.from, tc.to), strings.Join(got, ", "),
			"class_spells rows above rank 1: " + strings.Join(want, ", "), clsSpellSrc, resultIf(len(got) > 0 && len(got) <= len(want) && !slices.Contains(got, "rank 1 at "+fmt.Sprint(num(first["level"]))), "the known rank listed, or no later rank")})
	}
	finish(t, "advice-class-spell-known", "Offline class trainer: a known spell is not trained again", out)
}

// TestOfflineBenchmarkClassSpellTopRank: over a span of levels the advisor lists each spell once, at the highest rank the
// dataset gives the span, with that row's level.
func TestOfflineBenchmarkClassSpellTopRank(t *testing.T) {
	ds := usabStore()
	var out []benchRow
	for _, tc := range []struct {
		class    string
		from, to int
	}{{"MAGE", 10, 20}, {"WARRIOR", 14, 26}, {"ROGUE", 12, 24}, {"PRIEST", 16, 30}} {
		cid := classID(tc.class)
		top := map[string]store.Row{}
		for _, r := range classRows(t, cid, tc.from, tc.to) {
			name := text(r["name"])
			if num(r["rank"]) == 0 {
				continue
			}
			if old, ok := top[name]; !ok || num(r["rank"]) > num(old["rank"]) {
				top[name] = r
			}
		}
		bad, listed := []string{}, 0
		for _, s := range levelup.Spells(usabChar(tc.class, tc.to, "Horde"), ds, tc.from, tc.to) {
			if s.Rank == 0 {
				continue
			}
			listed++
			r, ok := top[s.Name]
			if !ok || int(num(r["rank"])) != s.Rank || int(num(r["level"])) != s.Level {
				bad = append(bad, fmt.Sprintf("%s rank %d at %d", s.Name, s.Rank, s.Level))
			}
		}
		out = append(out, benchRow{fmt.Sprintf("%s: levels %d to %d, one line per spell at its top rank", strings.ToLower(tc.class), tc.from, tc.to),
			fmt.Sprintf("%d ranked spells listed, %d not the top rank: %s", listed, len(bad), strings.Join(bad, ", ")), fmt.Sprintf("%d ranked spells in the class_spells rows", len(top)), clsSpellSrc,
			resultIf(len(bad) == 0 && listed > 0 && listed <= len(top), strings.Join(bad, ", "))})
	}
	finish(t, "advice-class-spell-top-rank", "Offline class trainer: the top rank over a span of levels", out)
}

// TestOfflineBenchmarkClassSpellPastCap: past the last level the dataset has rows for, and with nothing to learn, the answer
// says there is nothing to train and lists no spell or price.
func TestOfflineBenchmarkClassSpellPastCap(t *testing.T) {
	ds := usabStore()
	var out []benchRow
	for _, tc := range []struct {
		class    string
		from, to int
	}{{"MAGE", 61, 70}, {"WARRIOR", 61, 62}} {
		rows := classRows(t, classID(tc.class), tc.from, tc.to)
		facts, spells, _ := levelup.Facts(usabChar(tc.class, tc.from, "Horde"), ds, nil, tc.from, tc.to)
		out = append(out, benchRow{fmt.Sprintf("%s: levels %d to %d", strings.ToLower(tc.class), tc.from, tc.to), firstLine(facts), fmt.Sprintf("%d class_spells rows above level 60", len(rows)), clsNoneSrc,
			resultIf(len(rows) == 0 && !spells && strings.Contains(facts, "say there is nothing to train") && !strings.Contains(facts, "\n- Level"), "a spell named past the last level")})
	}
	finish(t, "advice-class-spell-no-answer", "Offline class trainer: no answer past the last level", out)
}

// ---- weapon skills ----

// clsMasterRef reads the dataset's masters of a skill for a faction: city, price and the level they teach it from.
func clsMasterRef(t *testing.T, faction string, skill, classLevel int64) (cities []string, cost, from int64) {
	t.Helper()
	rows, err := usabStore().Filter("weapon_masters", store.Range{Column: "npc_id", Min: int64(0), Max: int64(1 << 40)})
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range rows {
		if text(m["faction"]) != faction {
			continue
		}
		for i, s := range ints64(m["skills"]) {
			if s == skill {
				cities = append(cities, text(m["city"]))
				cost, from = ints64(m["cost_copper"])[i], max(ints64(m["min_level"])[i], classLevel)
			}
		}
	}
	return
}

// TestOfflineBenchmarkWeaponNeedLevel: a weapon whose skill a Weapon Master teaches only from a level (Polearms, from 20 in
// the dataset) is offered to a character below it with that level in the line; one at or above it is not told a level.
func TestOfflineBenchmarkWeaponNeedLevel(t *testing.T) {
	ds := usabStore()
	var out []benchRow
	for _, tc := range []struct {
		faction, class string
		level          int
	}{{"Horde", "WARRIOR", 15}, {"Alliance", "PALADIN", 18}, {"Horde", "DRUID", 12}, {"Alliance", "HUNTER", 19}, {"Horde", "WARRIOR", 20}, {"Alliance", "PALADIN", 35}} {
		c := usabChar(tc.class, tc.level, tc.faction)
		c.SkillsCaptured = true
		f := gear.LoadWeaponSkills(c, ds)
		_, need := f.ItemSkill(c, 2, 6)
		skillMin := int64(0)
		rows, _ := ds.Filter("weapon_skills", store.Range{Column: "skill_line_id", Min: int64(229), Max: int64(229)})
		for _, r := range rows {
			if num(r["class_id"]) == classID(tc.class) {
				skillMin = num(r["min_level"])
			}
		}
		cities, cost, from := clsMasterRef(t, tc.faction, 229, skillMin)
		line, ok := "no skill needed", need != nil
		if need != nil {
			line = need.Line()
			ok = strings.Contains(line, "from level 20") == (tc.level < int(from)) && strings.Contains(line, gear.CostText(cost)) && slices.Contains(cities, need.City)
		}
		out = append(out, benchRow{fmt.Sprintf("%s %s at level %d: a polearm needs the Polearms skill", strings.ToLower(tc.faction), strings.ToLower(tc.class), tc.level), line,
			fmt.Sprintf("master in %s, %s, from level %d", strings.Join(cities, "/"), gear.CostText(cost), from), clsWeaponSrc, resultIf(ok, "the level the master teaches it from")})
	}
	finish(t, "advice-weapon-need-level", "Offline weapon skills: the level a master teaches from", out)
}

// TestOfflineBenchmarkWeaponWhoCanWield: for a weapon type, the classes the advisor says may use it are the classes whose
// weapon_skills row says the skill is learnable.
func TestOfflineBenchmarkWeaponWhoCanWield(t *testing.T) {
	ds := usabStore()
	var out []benchRow
	for _, tc := range []struct {
		name     string
		subclass int
	}{{"Staves", 10}, {"One-Handed Maces", 4}, {"Fist Weapons", 13}, {"Polearms", 6}, {"Daggers", 15}, {"Wands", 19}, {"Thrown", 16}, {"Two-Handed Axes", 1}} {
		var got, want []string
		for _, cl := range usabClasses {
			c := usabChar(cl.token, 40, "Horde")
			f := gear.LoadWeaponSkills(c, ds)
			if ok, _ := f.ItemSkill(c, 2, tc.subclass); ok {
				got = append(got, strings.ToLower(cl.token[:4]))
			}
			if learnable(t, cl.id, weaponSkillOf[int64(tc.subclass)]) {
				want = append(want, strings.ToLower(cl.token[:4]))
			}
		}
		out = append(out, benchRow{tc.name + ": classes that can use it", strings.Join(got, " "), strings.Join(want, " ") + " (weapon_skills learnable)", clsWeaponSrc, resultIf(slices.Equal(got, want), "the classes that can learn it")})
	}
	finish(t, "advice-weapon-who-can-wield", "Offline weapon skills: which classes can use which weapon", out)
}

// TestOfflineBenchmarkWeaponNearestMaster: the master the advisor names first is the one in the zone the character stands
// in, else the first by city; the client's own skill names ("Swords", "Axes") count as the skills the dataset names.
func TestOfflineBenchmarkWeaponNearestMaster(t *testing.T) {
	ds := usabStore()
	var out []benchRow
	for _, tc := range []struct {
		faction, class, zone string
		subclass             int
		want                 string
	}{
		{"Alliance", "WARRIOR", "Stormwind City", 10, "Woo Ping"}, {"Alliance", "WARRIOR", "Darnassus", 10, "Ilyenia Moonfire"},
		{"Horde", "WARRIOR", "Thunder Bluff", 10, "Ansekhwa"}, {"Horde", "WARRIOR", "Orgrimmar", 10, "Hanashi"},
		{"Alliance", "ROGUE", "Ironforge", 13, "Buliwyf Stonehand"},
	} {
		c := usabChar(tc.class, 40, tc.faction)
		c.SkillsCaptured = true
		x, y := 50.0, 50.0
		c.Location = &character.Location{Zone: tc.zone, X: &x, Y: &y}
		f := gear.LoadWeaponSkills(c, ds)
		_, need := f.ItemSkill(c, 2, tc.subclass)
		got := "no skill needed"
		if need != nil {
			got = need.Master + " in " + need.City
		}
		out = append(out, benchRow{fmt.Sprintf("%s %s standing in %s: who teaches skill %d", strings.ToLower(tc.faction), strings.ToLower(tc.class), tc.zone, weaponSkillOf[int64(tc.subclass)]), got, tc.want + " (the master in that city)", clsWeaponSrc,
			resultIf(need != nil && need.Master == tc.want && need.City == tc.zone, "another master first")})
	}
	// The client calls the one-handed skills Swords, Axes and Maces.
	c := usabChar("PALADIN", 40, "Alliance")
	c.SkillsCaptured = true
	c.Skills = []character.Skill{{Name: "Swords", Group: "Weapon Skills"}, {Name: "Axes", Group: "Weapon Skills"}, {Name: "Maces", Group: "Weapon Skills"}}
	f := gear.LoadWeaponSkills(c, ds)
	var lacking []string
	for _, id := range f.Lacking(c) {
		lacking = append(lacking, f.Names[id])
	}
	out = append(out, benchRow{"paladin who lists Swords, Axes and Maces: skills still to learn", strings.Join(lacking, ", "), "learnable and not known: Polearms, Two-Handed Axes, Two-Handed Maces, Two-Handed Swords (the one-handed skills are known)", clsWeaponSrc,
		resultIf(strings.Join(lacking, ", ") == "Polearms, Two-Handed Axes, Two-Handed Maces, Two-Handed Swords", "a one-handed skill the client names Swords, Axes or Maces counted as missing")})
	finish(t, "advice-weapon-nearest-master", "Offline weapon skills: the nearest master and the client's skill names", out)
}

// TestOfflineBenchmarkWeaponClassNoAnswer: a class the dataset's weapon tables do not hold gets no weapon fact, and the
// answer says it has none; a skill no master teaches is named without a master.
func TestOfflineBenchmarkWeaponClassNoAnswer(t *testing.T) {
	ds := usabStore()
	c := usabChar("MONK", 30, "Horde")
	facts := ask.WeaponFacts(c, ds, "which weapons can I use?")
	out := []benchRow{{"monk: which weapons can I use", firstLine(facts), "no weapon_skills row for the class", clsNoneSrc, resultIf(strings.Contains(facts, "no weapon-skill facts") && !strings.Contains(facts, "can learn"), "a weapon list for a class the dataset lacks")}}
	finish(t, "advice-weapon-class-no-answer", "Offline weapon skills: no answer in the dataset", out)
}

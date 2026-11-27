package paths_test

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/ask"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/gear"
	"github.com/WoW-Companion/advisor/internal/store"
)

// The offline class trainer, weapon skill, riding and mount benchmark: what the advisor puts in front of the model for a question about
// the weapon skills a class can train and who teaches them, when a class learns Dual Wield, what a mount needs to ride, and what a
// class trainer, a riding trainer or a mount costs, beside the dataset's rows and the game's own rules, with no network. It reads
// testdata/usability_snapshot (weapon_skills, weapon_masters, class_spells, trainer_spells) and testdata/consumables_snapshot (the mount
// items). The dataset holds no class spell price, no class trainer, no riding skill, trainer or cost, no mount vendor and no
// reputation requirement, so those cases check that the advisor says so and states none; a difference only the dataset's gap causes is
// marked "(data)". make offline-benchmark runs it; WOWC_BENCH_REPORT receives the tables.

const (
	trWeaponSrc = "dataset weapon_skills (learnable, min_level), weapon_masters (name, city, place, faction, cost_copper, min_level)"
	trDualSrc   = "Warcraft Wiki, Dual Wield: Rogue level 10, Warrior and Hunter level 20, Shaman level 40; the dataset class_spells has no Dual Wield row"
	trMountSrc  = "dataset items (class 15, subclass 5: required_level, bonding); Warcraft Wiki, Riding: ground mounts need level 40 (60% speed) or level 60 (100% speed)"
	trNoneSrc   = "the dataset has no class spell price, class trainer, riding skill, riding trainer or cost, mount vendor or mount reputation row"
)

var (
	trClause = regexp.MustCompile(`^(.+) in (.+) for (\S+?)(?: \(level (\d+)\))?$`)
	trDigit  = regexp.MustCompile(`\d`)
)

// trLine is the facts' line for one weapon skill, "" when there is none.
func trLine(facts, skill string) string {
	for _, l := range strings.Split(facts, "\n") {
		if rest, ok := strings.CutPrefix(l, "  - "+skill+": "); ok {
			return rest
		}
	}
	return ""
}

// trShown is the masters a line names, as "master|where|price|level" in sorted order; "" for a line that does not parse.
func trShown(line string) []string {
	var out []string
	for _, clause := range strings.Split(line, "; ") {
		m := trClause.FindStringSubmatch(clause)
		if m == nil {
			return []string{"unparsed: " + line}
		}
		lvl := "1"
		if m[4] != "" {
			lvl = m[4]
		}
		for _, name := range strings.Split(m[1], " or ") {
			out = append(out, strings.Join([]string{name, m[2], m[3], lvl}, "|"))
		}
	}
	slices.Sort(out)
	return out
}

// trSkillID is a weapon skill's SkillLine id by its name, from weapon_skills.
func trSkillID(t *testing.T, ds store.DataStore, name string) int64 {
	t.Helper()
	rows, err := ds.Filter("weapon_skills", store.Range{Column: "skill_name", Min: name, Max: name})
	if err != nil || len(rows) == 0 {
		t.Fatalf("no weapon skill %q: %v", name, err)
	}
	return num(rows[0]["skill_line_id"])
}

// trExpected is the masters the dataset gives a side for a skill and a class, as "master|where|price|level", sorted: the level is the
// master's own or the class's minimum for the skill, whichever is higher.
func trExpected(t *testing.T, ds store.DataStore, faction, class string, skill int64) []string {
	t.Helper()
	min := int64(1)
	rows, _ := ds.Filter("weapon_skills", store.Range{Column: "skill_line_id", Min: skill, Max: skill})
	for _, r := range rows {
		if num(r["class_id"]) == classID(class) {
			min = max(min, num(r["min_level"]))
		}
	}
	masters, err := ds.Filter("weapon_masters", store.Range{Column: "npc_id", Min: int64(0), Max: int64(1 << 40)})
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, m := range masters {
		if text(m["faction"]) != faction {
			continue
		}
		for i, s := range ints64(m["skills"]) {
			if s == skill {
				out = append(out, strings.Join([]string{text(m["name"]), trWhere(m), gear.CostText(ints64(m["cost_copper"])[i]), strconv.FormatInt(max(min, ints64(m["min_level"])[i]), 10)}, "|"))
			}
		}
	}
	slices.Sort(out)
	return out
}

// trWhere is a master's city and district, or the city alone for a master with no district.
func trWhere(m store.Row) string {
	if text(m["place"]) == "" {
		return text(m["city"])
	}
	return text(m["city"]) + ", " + text(m["place"])
}

var trWeaponCases = []struct {
	faction, class, skill string
	level                 int
}{
	{"Horde", "WARRIOR", "Polearms", 12}, {"Horde", "WARRIOR", "Daggers", 12}, {"Horde", "SHAMAN", "Two-Handed Maces", 30},
	{"Horde", "ROGUE", "Thrown", 20}, {"Horde", "HUNTER", "Crossbows", 25}, {"Alliance", "PALADIN", "Polearms", 40},
	{"Alliance", "PRIEST", "Staves", 30}, {"Alliance", "HUNTER", "Guns", 10}, {"Alliance", "HUNTER", "Fist Weapons", 10},
	{"Alliance", "WARRIOR", "Two-Handed Swords", 20},
}

// TestOfflineBenchmarkTrainerWeaponMasterLines: for a weapon skill the class lacks, the line a weapon question puts in front of the model
// names the Weapon Masters of the character's side with their city, district, price and level, the same as weapon_masters.
func TestOfflineBenchmarkTrainerWeaponMasterLines(t *testing.T) {
	ds := usabStore()
	var out []benchRow
	for _, tc := range trWeaponCases {
		c := usabChar(tc.class, tc.level, tc.faction)
		c.SkillsCaptured = true
		facts := ask.WeaponFacts(c, ds, "where do I train "+strings.ToLower(tc.skill)+"?")
		want := trExpected(t, ds, tc.faction, tc.class, trSkillID(t, ds, tc.skill))
		line := trLine(facts, tc.skill)
		got := trShown(line)
		out = append(out, benchRow{fmt.Sprintf("%s %s level %d: where to train %s", tc.faction, strings.ToLower(tc.class), tc.level, tc.skill), line,
			strings.Join(want, "; "), trWeaponSrc, resultIf(line != "" && len(want) > 0 && slices.Equal(got, want), "the master, city, district, price or level")})
	}
	finish(t, "advice-trainer-weapon-master-lines", "Offline weapon skills: the masters, price and level a weapon question names", out)
}

var trClassCases = []struct{ faction, class string }{
	{"Horde", "WARRIOR"}, {"Horde", "SHAMAN"}, {"Horde", "ROGUE"}, {"Alliance", "PALADIN"}, {"Alliance", "PRIEST"}, {"Alliance", "HUNTER"},
}

// TestOfflineBenchmarkTrainerWeaponSkillList: the skills a class can learn are the weapon_skills rows the class can learn, a skill the
// character already has is not offered again, and a skill with no master in the dataset is said to have none.
func TestOfflineBenchmarkTrainerWeaponSkillList(t *testing.T) {
	ds := usabStore()
	var out []benchRow
	for _, tc := range trClassCases {
		var want []string
		rows, _ := ds.Filter("weapon_skills", store.Range{Column: "class_id", Min: classID(tc.class), Max: classID(tc.class)})
		for _, r := range rows {
			if b, _ := r["learnable"].(bool); b {
				want = append(want, text(r["skill_name"]))
			}
		}
		slices.Sort(want)
		c := usabChar(tc.class, 30, tc.faction)
		c.SkillsCaptured = true
		facts := ask.WeaponFacts(c, ds, "which weapons can I use?")
		line := ""
		for _, l := range strings.Split(facts, "\n") {
			if rest, ok := strings.CutPrefix(l, "- The class can learn: "); ok {
				line = strings.TrimSuffix(rest, ".")
			}
		}
		out = append(out, benchRow{fmt.Sprintf("%s %s: the weapon skills the class can learn", tc.faction, strings.ToLower(tc.class)), line, strings.Join(want, ", "), trWeaponSrc, resultIf(line == strings.Join(want, ", "), "the list of skills")})
	}
	one := 1
	c := usabChar("WARRIOR", 30, "Horde")
	c.SkillsCaptured = true
	c.Skills = []character.Skill{{Name: "One-Handed Axes", Group: "Weapon Skills", Rank: &one, Max: &one}}
	facts := ask.WeaponFacts(c, ds, "where do I train weapon skills?")
	out = append(out, benchRow{"Horde warrior with One-Handed Axes: is it offered again", firstLine(trLine(facts, "One-Handed Axes") + "(no line)"), "a learned skill is not a skill to train", trWeaponSrc,
		resultIf(trLine(facts, "One-Handed Axes") == "" && strings.Contains(facts, "has learned: One-Handed Axes 1/1") && trLine(facts, "Daggers") != "", "a learned skill was offered again, or an unlearned one dropped")})
	mage := usabChar("MAGE", 30, "Alliance")
	mage.SkillsCaptured = true
	wands := trLine(ask.WeaponFacts(mage, ds, "where do I train wands?"), "Wands")
	out = append(out, benchRow{"Alliance mage: who teaches Wands", wands, "no weapon_masters row lists the Wands skill", trWeaponSrc, resultIf(wands == "no master in the data", "a master was named for Wands")})
	finish(t, "advice-trainer-weapon-skill-list", "Offline weapon skills: what a class can learn", out)
}

// TestOfflineBenchmarkTrainerDualWield: a question about dual wielding gets the level the class learns it at, from the class trainer and
// not a Weapon Master, and no level for a class that cannot dual wield. The dataset's class_spells has no Dual Wield row.
func TestOfflineBenchmarkTrainerDualWield(t *testing.T) {
	ds := usabStore()
	game := map[string]int{"ROGUE": 10, "WARRIOR": 20, "HUNTER": 20, "SHAMAN": 40}
	var out []benchRow
	rows, _ := ds.Filter("class_spells", store.Range{Column: "name", Min: "Dual Wield", Max: "Dual Wield"})
	for _, cl := range usabClasses {
		c := usabChar(cl.token, 60, "Horde")
		c.SkillsCaptured = true
		f := ask.WeaponFacts(c, ds, "at what level can I dual wield?")
		line := ""
		for _, l := range strings.Split(f, "\n") {
			if strings.HasPrefix(l, "- Dual Wield") {
				line = strings.TrimPrefix(l, "- ")
			}
		}
		lv, can := game[cl.token]
		ref := "cannot dual wield: no level"
		ok := strings.Contains(line, "cannot dual wield") && !trDigit.MatchString(line)
		if can {
			ref = fmt.Sprintf("level %d, from the class trainer", lv)
			ok = strings.Contains(line, fmt.Sprintf("learns it at level %d.", lv)) && strings.Contains(line, "not a weapon skill")
		}
		out = append(out, benchRow{fmt.Sprintf("%s: when does the class learn Dual Wield", strings.ToLower(cl.token)), line, fmt.Sprintf("%s (%d class_spells rows named Dual Wield)", ref, len(rows)), trDualSrc, resultIf(ok && len(rows) == 0, "the level, or no statement that the class cannot")})
	}
	finish(t, "advice-trainer-dual-wield", "Offline class trainer: the level Dual Wield is learned at", out)
}

var trMounts = []struct {
	name string
	game int64 // the level the game gives the mount
}{
	{"Brown Horse Bridle", 40}, {"Gray Kodo", 40}, {"Red Mechanostrider", 40}, {"Swift Palomino", 60}, {"Great White Kodo", 60}, {"Black War Steed Bridle", 60},
}

// TestOfflineBenchmarkMountRequiredLevel: the level a question about riding a mount gets is the dataset's required level for the item, and
// the item is not given a source, a price or a reputation.
func TestOfflineBenchmarkMountRequiredLevel(t *testing.T) {
	ds := consStore()
	var out []benchRow
	for _, m := range trMounts {
		rl := num(rowByName(t, ds, "items", m.name)["required_level"])
		q := "what level do I need to use the mount [" + m.name + "]?"
		f := ask.ItemTooltipFacts(ds, q)
		lim := ask.LimitFacts(q)
		ok := strings.Contains(f, fmt.Sprintf("requires level %d;", rl)) && strings.Contains(lim, "Riding and mounts:") && strings.Contains(lim, "Say so")
		res := resultIf(ok, "the tooltip lacks the dataset's required level, or the facts do not say the data holds no riding or reputation row")
		if ok && rl != m.game {
			res = fmt.Sprintf("differ (data): the dataset gives %d, the game %d", rl, m.game)
		}
		out = append(out, benchRow{m.name + ": the level to ride it", firstLine(strings.TrimPrefix(f, "Item tooltips (from the item rows; answer from these, not from memory):\n")), fmt.Sprintf("dataset required_level %d; game %d", rl, m.game), trMountSrc, res})
	}
	finish(t, "advice-trainer-mount-level", "Offline mounts: the level to ride a mount", out)
}

var trNoAnswer = []struct{ q, topic, ref string }{
	{"where is the riding trainer for orcs?", "Riding and mounts:", "no riding trainer row: say so, name none"},
	{"how much does Apprentice Riding cost?", "Riding and mounts:", "no riding training cost row: say so, name none"},
	{"what level do I need to learn Journeyman Riding?", "Riding and mounts:", "no riding skill level row: say so, name none"},
	{"do I need reputation for my racial mount?", "Riding and mounts:", "no mount reputation or race row: say so, name none"},
	{"how much gold is the Black War Steed at the stable?", "Riding and mounts:", "no mount vendor or gold price row: say so, name none"},
	{"when does my paladin get the Summon Warhorse mount?", "Riding and mounts:", "no class mount row: say so, name none"},
	{"how much does my mage trainer charge for new spells?", "Class trainer prices:", "no class spell price row: say so, name none"},
	{"what does the class trainer charge at level 20?", "Class trainer prices:", "no class spell price row: say so, name none"},
	{"how much does it cost to train my new abilities?", "Class trainer prices:", "no class spell price row: say so, name none"},
}

// TestOfflineBenchmarkTrainerRidingNoAnswer: what the dataset cannot answer about a riding trainer, riding cost or level, a racial mount's
// reputation, a class mount and a class trainer's price gets the line that the data holds none, with no figure in it.
func TestOfflineBenchmarkTrainerRidingNoAnswer(t *testing.T) {
	ds := usabStore()
	var out []benchRow
	for _, tc := range trNoAnswer {
		f := ask.LimitFacts(tc.q)
		ok := strings.Contains(f, tc.topic) && strings.Contains(f, "Say so") && !trDigit.MatchString(f)
		out = append(out, benchRow{"\"" + tc.q + "\"", firstLine(f), tc.ref, trNoneSrc, resultIf(ok, "no statement that the data holds none, or a figure in it")})
	}
	for _, n := range []string{"Summon Warhorse", "Summon Felsteed", "Apprentice Riding"} {
		rows, _ := ds.Filter("class_spells", store.Range{Column: "name", Min: n, Max: n})
		out = append(out, benchRow{n + ": a class_spells row", fmt.Sprintf("%d rows", len(rows)), "the dataset holds no row for a class mount or a riding skill", trNoneSrc, resultIf(len(rows) == 0, "a row exists: the fact that the data holds none is wrong")})
	}
	finish(t, "advice-trainer-riding-no-answer", "Offline trainers and riding: no answer in the dataset", out)
}

// TestOfflineBenchmarkTrainerQuietQuestions: a question that only shares a word with riding or a class trainer's price gets no such facts.
func TestOfflineBenchmarkTrainerQuietQuestions(t *testing.T) {
	var out []benchRow
	for _, q := range []string{"how much does the alchemy trainer charge?", "what does rank 3 Frostbolt cost?", "how do I get to Blackrock Mountain?",
		"where is the weapon master in Orgrimmar?", "which spells does my mage learn at level 20?", "what is the cost of the Elixir of Agility recipe?"} {
		f := ask.LimitFacts(q)
		out = append(out, benchRow{"\"" + q + "\"", f, "no riding or class trainer price facts on a question about something else", trNoneSrc, resultIf(f == "", "riding or class trainer facts on a question about something else")})
	}
	finish(t, "advice-trainer-quiet", "Offline trainers and riding: questions about something else", out)
}

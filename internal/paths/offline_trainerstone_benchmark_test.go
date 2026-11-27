package paths_test

import (
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/ask"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/gear"
	"github.com/WoW-Companion/advisor/internal/levelup"
	"github.com/WoW-Companion/advisor/internal/questie"
	"github.com/WoW-Companion/advisor/internal/store"
)

// The offline class trainer location, weapon and armor proficiency and summoning stone benchmark: the city and trainer the advisor sends a class to, the weapons and
// armor it says a class can use and the gear it offers, and what it says of a dungeon's summoning stone, beside the dataset's rows and the game's own rules, with no network. It reads
// testdata/places_snapshot (instances: entrance zone, level range and group size; weapon_skills; weapon_masters; trainers: profession trainers only) and
// testdata/usability_snapshot (items, weapon_skills). The dataset holds no class trainer row (an NPC, a city or a position), no armor proficiency table and no summoning stone row, so the capitals that hold a
// class trainer are the Warcraft Wiki's Classic class trainer lists written out here, the trainer NPCs of the location cases are composed in a synthetic QuestieDB (as the quest benchmarks do), the armor
// ladder is the game's (cloth, leather, mail, plate; plate and mail from level 40), and the cases the dataset cannot answer check that the advisor says so and names none. A summoning stone stands by its
// dungeon's entrance (Warcraft Wiki, Summoning Stone), so the dungeon's entrance zone in the dataset is the place the advisor may give. A difference the dataset's own rows cause is marked "(data)". make
// offline-benchmark runs it; WOWC_BENCH_REPORT receives the tables.

const (
	czTrainerSrc = "dataset trainers (profession trainers only; no class trainer row); game: Warcraft Wiki class trainer pages (the capitals with a class trainer); player's QuestieDB (NPC subname \"<Class> Trainer\", spawns)"
	czProfSrc    = "dataset weapon_skills (class_id, skill_line_id, learnable), items (class_id, subclass_id, slot); game armor proficiency (Warcraft Wiki armor and weapon skill pages)"
	czStoneSrc   = "dataset instances (entrance_zone_name, min_level, max_level, max_players); no summoning stone row; game: Warcraft Wiki, Summoning Stone (stands by the dungeon entrance)"
	czNoneSrc    = "the dataset has no such row or column (no class trainer NPC or position, armor proficiency table, summoning stone row or list)"
)

// czTrainerCases are a character in a zone and the capital the Warcraft Wiki's class trainer lists send the class to from there.
var czTrainerCases = []struct {
	faction, class, zone, city string
	x, y                       float64
}{
	{"Horde", "MAGE", "Durotar", "Orgrimmar", 38, 85},
	{"Alliance", "WARLOCK", "Elwynn Forest", "Stormwind City", 25, 76},
	{"Horde", "DRUID", "Mulgore", "Thunder Bluff", 76, 10},
	{"Alliance", "HUNTER", "Dun Morogh", "Ironforge", 70, 82},
	{"Horde", "ROGUE", "Tirisfal Glades", "Undercity", 72, 33},
	{"Alliance", "PRIEST", "Teldrassil", "Darnassus", 39, 82},
}

// czIndex is a synthetic QuestieDB with one class trainer of the case's class in its city, friendly to its side, and a starting-area trainer of the class that must never be named.
func czIndex(class, faction, city string, x, y float64) *questie.Index {
	side := "H"
	if faction == "Alliance" {
		side = "A"
	}
	sub := czTitle(class) + " Trainer"
	return &questie.Index{DB: "t", Areas: map[int]questie.Area{1: {Zone: "Starting Vale"}, 2: {Zone: city}},
		Npcs: map[int]questie.Entity{
			10: {Name: "Benchtest Novice " + czTitle(class), Sub: sub, Faction: side, Spawns: []questie.Spawn{{Area: 1, X: 40, Y: 40}}},
			11: {Name: "Benchtest " + czTitle(class) + " Master", Sub: sub, Faction: side, Spawns: []questie.Spawn{{Area: 2, X: x, Y: y}}},
		}}
}

func czTitle(token string) string { return token[:1] + strings.ToLower(token[1:]) }

func czChar(faction, class, zone string, level int) character.Character {
	c := usabChar(class, level, faction)
	x, y := 50.0, 50.0
	c.Location = &character.Location{Zone: zone, X: &x, Y: &y}
	return c
}

// TestOfflineBenchmarkClassTrainerLocation: the city and the trainer the advisor names for a class, beside the Warcraft Wiki's class trainer city and the trainer a QuestieDB places there.
func TestOfflineBenchmarkClassTrainerLocation(t *testing.T) {
	var out []benchRow
	for _, tc := range czTrainerCases {
		c := czChar(tc.faction, tc.class, tc.zone, 30)
		qx := czIndex(tc.class, tc.faction, tc.city, tc.x, tc.y)
		pl, ok := levelup.TrainAt(c, qx)
		line := pl.Line(tc.class)
		want := fmt.Sprintf("%s (%.0f, %.0f)", "Benchtest "+czTitle(tc.class)+" Master", tc.x, tc.y)
		out = append(out, benchRow{fmt.Sprintf("%s %s in %s: where to train, with a QuestieDB trainer", tc.faction, strings.ToLower(tc.class), tc.zone), line,
			fmt.Sprintf("Train at: %s: %s; the starting-area trainer is not named; no dataset row", tc.city, want), czTrainerSrc,
			resultIf(ok && pl.City == tc.city && strings.Contains(line, "Train at: "+tc.city+": "+want) && !strings.Contains(line, "Novice"), "the city or trainer is not the one the wiki list and QuestieDB give")})
	}
	finish(t, "advice-class-trainer-location", "Offline class trainer: the city and trainer a class is sent to", out)
}

// TestOfflineBenchmarkClassTrainerNoTrainerName: with no QuestieDB the advisor names the city and no trainer or position, the dataset having none.
func TestOfflineBenchmarkClassTrainerNoTrainerName(t *testing.T) {
	var out []benchRow
	for _, tc := range czTrainerCases[:4] {
		pl, ok := levelup.TrainAt(czChar(tc.faction, tc.class, tc.zone, 30), nil)
		line := pl.Line(tc.class)
		out = append(out, benchRow{fmt.Sprintf("%s %s in %s: where to train, with no QuestieDB", tc.faction, strings.ToLower(tc.class), tc.zone), line,
			fmt.Sprintf("Train at: %s, your %s trainer there; no trainer name and no position", tc.city, strings.ToLower(tc.class)), czTrainerSrc,
			resultIf(ok && pl.City == tc.city && pl.Trainer == nil && !regexp.MustCompile(`\d`).MatchString(strings.Split(line, "(")[0]), "a trainer or position named from no source")})
	}
	finish(t, "advice-class-trainer-no-name", "Offline class trainer: no trainer name without a source", out)
}

var czWhereCases = []struct {
	q, class string
	has, no  []string // cities the answer lists, and cities it must not
}{
	{"where is the mage trainer in Orgrimmar?", "MAGE", []string{"Undercity", "Orgrimmar", "Stormwind City", "Ironforge"}, []string{"Thunder Bluff", "Darnassus"}},
	{"where can I find a druid trainer?", "MAGE", []string{"Thunder Bluff", "Darnassus"}, []string{"Orgrimmar", "Undercity", "Stormwind City", "Ironforge"}},
	{"which city has the shaman trainer for the Horde?", "WARRIOR", []string{"Orgrimmar", "Thunder Bluff", "Ironforge"}, []string{"Undercity", "Stormwind City", "Darnassus"}},
	{"where do I train my spells?", "HUNTER", []string{"Orgrimmar", "Thunder Bluff", "Ironforge", "Darnassus"}, []string{"Undercity", "Stormwind City"}},
}

// TestOfflineBenchmarkClassTrainerWhereQuestion: a question about where a class trainer is gets the capitals that hold one for the class it names (else the character's), and no NPC.
func TestOfflineBenchmarkClassTrainerWhereQuestion(t *testing.T) {
	var out []benchRow
	for _, tc := range czWhereCases {
		f := ask.TrainerWhereFacts(czChar("Horde", tc.class, "Durotar", 30), tc.q)
		ok := strings.Contains(f, "holds no class trainer NPC") && !strings.Contains(f, "(Horde: none; Alliance: none)")
		for _, c := range tc.has {
			ok = ok && strings.Contains(f, c)
		}
		for _, c := range tc.no {
			ok = ok && !strings.Contains(f, c)
		}
		out = append(out, benchRow{"\"" + tc.q + "\"", czAbbrev(f), "cities: " + strings.Join(tc.has, ", ") + "; none of " + strings.Join(tc.no, ", ") + "; no NPC (the dataset has none)", czTrainerSrc,
			resultIf(ok, "the cities are not the wiki's lists for the class, or the answer does not say the data holds no trainer")})
	}
	finish(t, "advice-class-trainer-where", "Offline class trainer: a question about where a class trainer is", out)
}

var czStands = regexp.MustCompile(`an? \w+ trainer stands in \([^)]*\)`)

// czAbbrev is the part of a trainer location line that names the class and its cities.
func czAbbrev(s string) string {
	if m := czStands.FindString(s); m != "" {
		return m
	}
	return s
}

// ---- weapon and armor proficiency ----

// czWeaponSkills are the skill names the dataset's weapon_skills rows say the class can learn.
func czWeaponSkills(t *testing.T, class string) []string {
	t.Helper()
	rows, err := usabStore().Filter("weapon_skills", store.Range{Column: "class_id", Min: grClassOf(class), Max: grClassOf(class)})
	if err != nil || len(rows) == 0 {
		t.Fatalf("%s: no weapon_skills rows (%v)", class, err)
	}
	var out []string
	for _, r := range rows {
		if b, _ := r["learnable"].(bool); b {
			out = append(out, text(r["skill_name"]))
		}
	}
	sort.Strings(out)
	return out
}

// TestOfflineBenchmarkProficiencyWeaponQuestion: a question about the weapons another class can use gets that class's weapon skills (not the asking character's), the dataset's learnable set exactly.
func TestOfflineBenchmarkProficiencyWeaponQuestion(t *testing.T) {
	var out []benchRow
	for _, tc := range []struct{ q, class, asker string }{
		{"what weapons can a priest use?", "PRIEST", "WARRIOR"}, {"can my friend's druid use a sword?", "DRUID", "ROGUE"},
		{"which weapons can a mage wield?", "MAGE", "HUNTER"}, {"can a paladin use a bow?", "PALADIN", "WARLOCK"},
		{"what weapons can a rogue use?", "ROGUE", "MAGE"}, {"can a shaman use a two-handed axe?", "SHAMAN", "PRIEST"},
	} {
		c := czChar("Horde", tc.asker, "Durotar", 30)
		f := ask.ProficiencyFacts(c, usabStore(), tc.q)
		want := czWeaponSkills(t, tc.class)
		ok := strings.Contains(f, "can learn these weapon skills: "+strings.Join(want, ", ")+" (")
		out = append(out, benchRow{fmt.Sprintf("%q (asked by a %s)", tc.q, strings.ToLower(tc.asker)), firstLineMatching(f, czTitle(tc.class)), "weapon skills: " + strings.Join(want, ", "), czProfSrc,
			resultIf(ok, "the class's weapon skills are not the dataset's learnable set")})
	}
	finish(t, "advice-proficiency-weapon-question", "Offline proficiency: a question about what weapons a class can use", out)
}

var czArmorCases = []struct {
	q, class, ladder string
	shield           bool
}{
	{"what armor can a priest wear?", "PRIEST", "cloth at every level", false},
	{"can my mage wear plate armor?", "MAGE", "cloth at every level", false},
	{"can a rogue wear mail?", "ROGUE", "leather at every level", false},
	{"when can a shaman wear mail armor?", "SHAMAN", "leather until level 40, then mail", true},
	{"what armor type does a paladin wear at level 40?", "PALADIN", "mail until level 40, then plate", true},
	{"can a hunter equip plate?", "HUNTER", "leather until level 40, then mail", false},
}

// TestOfflineBenchmarkProficiencyArmorQuestion: a question about the armor a class can wear gets the class's armor ladder and shield, the game's rule (the dataset has no armor proficiency table).
func TestOfflineBenchmarkProficiencyArmorQuestion(t *testing.T) {
	var out []benchRow
	for _, tc := range czArmorCases {
		f := ask.ProficiencyFacts(czChar("Horde", "WARRIOR", "Durotar", 30), usabStore(), tc.q)
		shield := "Cannot use a shield."
		if tc.shield {
			shield = "Can carry a shield."
		}
		line := firstLineMatching(f, czTitle(tc.class))
		out = append(out, benchRow{fmt.Sprintf("%q", tc.q), line[max(0, strings.Index(line, "Armor:")):], "Armor: " + tc.ladder + ". " + shield, czProfSrc,
			resultIf(strings.Contains(line, "Armor: "+tc.ladder+".") && strings.Contains(line, shield), "the armor ladder or the shield is not the game's rule")})
	}
	// the character's own class when the question names none
	f := ask.ProficiencyFacts(czChar("Alliance", "PRIEST", "Elwynn Forest", 12), usabStore(), "what armor can I wear?")
	out = append(out, benchRow{"\"what armor can I wear?\" (a priest)", firstLineMatching(f, "Priest"), "Armor: cloth at every level. Cannot use a shield.", czProfSrc,
		resultIf(strings.Contains(f, "Armor: cloth at every level. Cannot use a shield."), "no armor rule for the asking character's class")})
	finish(t, "advice-proficiency-armor-question", "Offline proficiency: a question about what armor a class can wear", out)
}

var czGearCases = []struct {
	class, item string
	level       int
}{
	{"PRIEST", "Bastard Sword", 30}, {"PALADIN", "Polished Shortbow", 30}, {"MAGE", "Brass Knuckles", 30}, {"DRUID", "Brass Knuckles", 30},
	{"ROGUE", "Light Crossbow", 30}, {"DRUID", "Cutlass", 30}, {"WARLOCK", "Tabar", 30}, {"SHAMAN", "Tabar", 30}, {"PRIEST", "Steel Plate Helm", 50}, {"MAGE", "Scalemail Vest", 50},
}

// TestOfflineBenchmarkProficiencyGearOffered: the advisor offers an item only to a class whose weapon skills (or armor ladder) allow it: the dataset's weapon_skills for a weapon, the game's ladder for armor.
func TestOfflineBenchmarkProficiencyGearOffered(t *testing.T) {
	var out []benchRow
	for _, tc := range czGearCases {
		r := usabItem(t, tc.item)
		sub, cls := num(r["subclass_id"]), num(r["class_id"])
		var want bool
		if cls == 2 {
			want = slices.Contains(czWeaponSkills(t, tc.class), czSkillName(t, weaponSkillOf[sub]))
		} else {
			want = sub == armorWorn(tc.class, tc.level)
		}
		got, known := gear.CanUse(tc.class, tc.level, r)
		out = append(out, benchRow{fmt.Sprintf("%s (class %d, subclass %d) offered to a level %d %s", tc.item, cls, sub, tc.level, strings.ToLower(tc.class)), fmt.Sprint(got), fmt.Sprint(want), czProfSrc,
			resultIf(known && got == want, "the advisor offers gear the class cannot use, or withholds gear it can")})
	}
	finish(t, "advice-proficiency-gear-offered", "Offline proficiency: gear offered to a class", out)
}

// skillName is a weapon skill line's name in weapon_skills.
func czSkillName(t *testing.T, id int64) string {
	t.Helper()
	rows, err := usabStore().Filter("weapon_skills", store.Range{Column: "skill_line_id", Min: id, Max: id})
	if err != nil || len(rows) == 0 {
		t.Fatalf("skill line %d: no weapon_skills row (%v)", id, err)
	}
	return text(rows[0]["skill_name"])
}

// ---- summoning stones ----

// czInstance is a dungeon's row of the instances table.
func czInstance(t *testing.T, name string) store.Row {
	t.Helper()
	return rowByName(t, qbStore(), "instances", name)
}

// TestOfflineBenchmarkSummoningStonePlace: a question about a dungeon's summoning stone gets the dungeon's entrance zone from the dataset and the line that the data holds no stone row.
func TestOfflineBenchmarkSummoningStonePlace(t *testing.T) {
	var out []benchRow
	for _, tc := range []struct{ q, dungeon string }{
		{"where is the summoning stone for Deadmines?", "Deadmines"}, {"where is the meeting stone for Wailing Caverns?", "Wailing Caverns"},
		{"where do I find the summoning stone for Blackrock Depths?", "Blackrock Depths"}, {"is there a summoning stone outside Shadowfang Keep?", "Shadowfang Keep"},
		{"where is the BRD summoning stone?", "Blackrock Depths"}, {"where is the summoning stone for Zul'Farrak?", "Zul'Farrak"},
	} {
		r := czInstance(t, tc.dungeon)
		zone := text(r["entrance_zone_name"])
		f := ask.SummonStoneFacts(qbStore(), tc.q)
		ok := strings.Contains(f, "its entrance is in "+zone+" (level") && strings.Contains(f, "holds no summoning stone row") && strings.Contains(f, tc.dungeon)
		out = append(out, benchRow{"\"" + tc.q + "\"", czAbbrevStone(f), fmt.Sprintf("%s: entrance in %s (level %d-%d, %d players); no stone row", tc.dungeon, zone, num(r["min_level"]), num(r["max_level"]), num(r["max_players"])), czStoneSrc,
			resultIf(ok, "the entrance zone is not the dataset's, or the data's lack of a stone row is not stated")})
	}
	finish(t, "advice-summoning-stone-place", "Offline summoning stones: the dungeon and where to go", out)
}

func czAbbrevStone(s string) string {
	if i := strings.Index(s, "What the data holds"); i >= 0 {
		return s[i:]
	}
	return s[max(0, len(s)-120):]
}

// ---- no answer ----

var czCoords = regexp.MustCompile(`\(\d+, \d+\)|\b\d{1,2}\.\d\b`)

// TestOfflineBenchmarkTrainerStoneNoAnswer: what the dataset cannot answer, a class trainer's NPC, a stone's coordinates, which dungeons have a stone, an armor proficiency row, the advisor says so and names none.
func TestOfflineBenchmarkTrainerStoneNoAnswer(t *testing.T) {
	var out []benchRow
	c := czChar("Alliance", "MAGE", "Elwynn Forest", 30)
	f := ask.TrainerWhereFacts(c, "who is the mage trainer in Stormwind?")
	out = append(out, benchRow{"\"who is the mage trainer in Stormwind?\"", czAbbrev(f), "no class trainer NPC row: say so, name none", czNoneSrc,
		resultIf(strings.Contains(f, "holds no class trainer NPC, name or position") && strings.Contains(f, "name no trainer"), "no statement that the data holds no class trainer NPC")})
	f = ask.TrainerWhereFacts(c, "what are the coordinates of the warrior trainer in Ironforge?")
	out = append(out, benchRow{"\"what are the coordinates of the warrior trainer in Ironforge?\"", czAbbrev(f), "no class trainer position: say so, give none", czNoneSrc,
		resultIf(strings.Contains(f, "no class trainer NPC, name or position") && !czCoords.MatchString(f), "a position, or no statement that none is held")})
	f = ask.SummonStoneFacts(qbStore(), "what are the exact coordinates of the Deadmines summoning stone?")
	out = append(out, benchRow{"\"what are the exact coordinates of the Deadmines summoning stone?\"", czAbbrevStone(f), "no stone position row: say so, give none", czNoneSrc,
		resultIf(strings.Contains(f, "no stone position") && strings.Contains(f, "give no stone coordinates") && !czCoords.MatchString(f), "stone coordinates, or no statement that none is held")})
	f = ask.SummonStoneFacts(qbStore(), "which dungeons have a summoning stone?")
	out = append(out, benchRow{"\"which dungeons have a summoning stone?\"", czAbbrevStone(f), "no list of dungeons with a stone: say so, name none", czNoneSrc,
		resultIf(strings.Contains(f, "no list of which dungeons or raids have one") && strings.Contains(f, "No dungeon is named, so name none"), "a dungeon list, or no statement that none is held")})
	f = ask.ProficiencyFacts(c, usabStore(), "can a warlock wear plate?")
	_, err := usabStore().Filter("armor_skills", store.Range{Column: "class_id", Min: int64(0), Max: int64(99)})
	out = append(out, benchRow{"\"can a warlock wear plate?\"", firstLineMatching(f, "Warlock"), "armor from the advisor's class rules; the data has no armor proficiency table", czNoneSrc,
		resultIf(err != nil && strings.Contains(f, "no armor proficiency table") && strings.Contains(f, "Armor: cloth at every level."), "an armor proficiency table in the data, or armor stated as the data's")})
	for _, q := range []string{"where is the alchemy trainer in Stormwind?", "where is the weapon master in Orgrimmar?", "what is the best mage spec at level 40?", "how many gold does the Deadmines drop?"} {
		got := ask.TrainerWhereFacts(c, q) + ask.SummonStoneFacts(qbStore(), q) + ask.ProficiencyFacts(c, usabStore(), q)
		out = append(out, benchRow{"\"" + q + "\" (a mage)", got, "no class trainer, summoning stone or proficiency facts on a question about something else", czNoneSrc,
			resultIf(got == "", "trainer, stone or proficiency facts on a question about something else")})
	}
	finish(t, "advice-trainer-stone-no-answer", "Offline class trainer, proficiency and summoning stones: no answer in the dataset", out)
}

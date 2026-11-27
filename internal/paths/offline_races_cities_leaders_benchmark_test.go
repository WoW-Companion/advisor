package paths_test

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/ask"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/learn"
	"github.com/WoW-Companion/advisor/internal/questie"
	"github.com/WoW-Companion/advisor/internal/store"
)

// The offline racial trait, capital city service and faction leader benchmark: what the advisor puts in front of the model for a
// question about a race's traits, the services of a capital city and who leads a faction, beside the dataset's rows and the game's
// own rules, with no network. It reads testdata/usability_snapshot (class_spells, weapon_skills, npcs), testdata/world_snapshot
// (ui_maps) and testdata/places_snapshot (trainers). The dataset holds no race table, no racial ability, no bank, auction house,
// mailbox, innkeeper or portal and no faction leader, so those cases check that the advisor says so and states none; the profession
// trainers of a capital are the one service the dataset places, by zone, and its faction column is always empty, so those cases check
// that a character is not sent to the other side's capital. make offline-benchmark runs it; WOWC_BENCH_REPORT receives the tables.

const (
	rcRaceSrc    = "Warcraft Wiki, Racial ability (Classic): the race's passive and active racials; the dataset has no races table and no racial spell in class_spells"
	rcSideSrc    = "Warcraft Wiki, Race: Horde races (Orc, Undead, Tauren, Troll), Alliance races (Human, Dwarf, Night Elf, Gnome)"
	rcCitySrc    = "dataset trainers (profession, zone_name; faction always empty); Warcraft Wiki, Capital cities: Orgrimmar, Undercity, Thunder Bluff (Horde), Stormwind City, Ironforge, Darnassus (Alliance)"
	rcServiceSrc = "dataset ui_maps names the capital's map only; no bank, auction house, mailbox, innkeeper, stable master or portal row"
	rcLeaderSrc  = "dataset npcs has no title, leader or ruler column; Warcraft Wiki, Faction leaders: Thrall, Cairne Bloodhoof, Sylvanas Windrunner, Varian Wrynn, Magni Bronzebeard, Tyrande Whisperwind"
)

var (
	rcDigit      = regexp.MustCompile(`[0-9%]`)
	rcHordeCaps  = []string{"Orgrimmar", "Undercity", "Thunder Bluff"}
	rcAllyCaps   = []string{"Stormwind City", "Ironforge", "Darnassus"}
	rcRaceTables = []string{"races", "racial_traits", "racial_abilities", "race_skills"}
)

// rcNamed gives a trainer the snapshot lacks a name for a synthetic one, so the trainer finder can run; the zone is the dataset's own.
type rcNamed struct{ store.DataStore }

func (n rcNamed) Filter(table string, r ...store.Range) ([]store.Row, error) {
	rows, err := n.DataStore.Filter(table, r...)
	if table == "trainers" {
		for _, row := range rows {
			if row["name"] == nil {
				row["name"] = fmt.Sprintf("Synthetic Trainer %d", num(row["npc_id"]))
			}
		}
	}
	return rows, err
}

// rcSpellRows counts the class_spells rows that carry a racial's name.
func rcSpellRows(ds store.DataStore, names ...string) int {
	n := 0
	for _, name := range names {
		rows, _ := ds.Filter("class_spells", store.Range{Column: "name", Min: name, Max: name})
		n += len(rows)
	}
	return n
}

var rcRacials = []struct {
	q, race string
	spells  []string
	ref     string
}{
	{"what is the Orc racial?", "Orc", []string{"Blood Fury", "Hardiness", "Command"}, "Orc: Blood Fury, Hardiness, Command, Axe Specialization"},
	{"does a troll get Berserking and a bow bonus?", "Troll", []string{"Berserking", "Beast Slaying", "Bow Specialization", "Throwing Specialization"}, "Troll: Berserking, Regeneration, Beast Slaying, Bow and Throwing Specialization"},
	{"does a dwarf get Stoneform and a gun bonus?", "Dwarf", []string{"Stoneform", "Gun Specialization", "Frost Resistance"}, "Dwarf: Stoneform, Gun Specialization, Frost Resistance"},
	{"which race gets Engineering Specialization?", "Gnome", []string{"Engineering Specialization", "Expansive Mind", "Escape Artist"}, "Gnome: Engineering Specialization, Expansive Mind, Escape Artist"},
	{"what racial bonuses does a night elf have?", "Night Elf", []string{"Shadowmeld", "Wisp Spirit", "Quickness"}, "Night Elf: Shadowmeld, Wisp Spirit, Quickness, Nature Resistance"},
	{"what does Will of the Forsaken do for an undead?", "Undead", []string{"Will of the Forsaken", "Cannibalize"}, "Undead: Will of the Forsaken, Cannibalize, Underwater Breathing"},
	{"does a tauren get a herbalism bonus from Cultivation?", "Tauren", []string{"War Stomp", "Cultivation", "Endurance"}, "Tauren: War Stomp, Cultivation (Herbalism), Endurance, Nature Resistance"},
	{"which racial weapon bonus does a human get, Sword Specialization?", "Human", []string{"Sword Specialization", "Mace Specialization", "Perception", "Diplomacy"}, "Human: Sword and Mace Specialization, Perception, Diplomacy, The Human Spirit"},
}

// TestOfflineBenchmarkRacialTraits: a question about a race's trait, weapon skill or profession bonus gets the line that the data holds
// no race table or racial ability, with no figure in it, and the dataset holds no class_spells row for any of the racial's names.
func TestOfflineBenchmarkRacialTraits(t *testing.T) {
	ds := usabStore()
	var out []benchRow
	for _, tc := range rcRacials {
		f := ask.LimitFacts(tc.q)
		n := rcSpellRows(ds, tc.spells...)
		ok := strings.Contains(f, "Racial traits:") && strings.Contains(f, "Say so") && !rcDigit.MatchString(f) && n == 0
		out = append(out, benchRow{fmt.Sprintf("%s: \"%s\"", tc.race, tc.q), firstLine(f), fmt.Sprintf("%d class_spells rows for %s; game: %s", n, strings.Join(tc.spells, ", "), tc.ref), rcRaceSrc,
			resultIf(ok, "no statement that the data holds no racial, or a figure in it, or a racial row exists")})
	}
	finish(t, "advice-racial-traits", "Offline racial traits: the data holds none, and the advisor says so", out)
}

var rcSides = []struct{ race, side string }{
	{"Orc", "Horde"}, {"Troll", "Horde"}, {"Tauren", "Horde"}, {"Undead", "Horde"}, {"Scourge", "Horde"},
	{"Human", "Alliance"}, {"Dwarf", "Alliance"}, {"NightElf", "Alliance"}, {"Gnome", "Alliance"},
}

// TestOfflineBenchmarkRaceSide: the faction a race belongs to, as the advisor's race masks have it, beside the game's own rule: each race
// is in its side's mask and in the other side's not.
func TestOfflineBenchmarkRaceSide(t *testing.T) {
	var out []benchRow
	for _, tc := range rcSides {
		bit, ok := questie.RaceBit(tc.race, tc.side)
		got := "neither"
		if ok && bit&questie.HordeMask != 0 && bit&questie.AllianceMask == 0 {
			got = "Horde"
		} else if ok && bit&questie.AllianceMask != 0 && bit&questie.HordeMask == 0 {
			got = "Alliance"
		}
		_, side, fell := questie.RaceMasks(tc.race, "")
		out = append(out, benchRow{tc.race + ": which faction", got, tc.side, rcSideSrc, resultIf(got == tc.side && side == 0 && !fell, "the race is on the wrong side or unknown")})
	}
	finish(t, "advice-race-side", "Offline races: which faction a race belongs to", out)
}

var rcTrainerCases = []struct {
	faction, city, prof string
	other               []string
}{
	{"Horde", "Orgrimmar", "Cooking", rcAllyCaps}, {"Horde", "Undercity", "Alchemy", rcAllyCaps}, {"Horde", "Orgrimmar", "Tailoring", rcAllyCaps},
	{"Alliance", "Stormwind City", "Tailoring", rcHordeCaps}, {"Alliance", "Ironforge", "Herbalism", rcHordeCaps}, {"Alliance", "Stormwind City", "Cooking", rcHordeCaps},
}

func rcAt(faction, city string) character.Character {
	lv := 40
	return character.Character{Faction: faction, Class: "MAGE", Level: &lv, Location: &character.Location{Zone: city}}
}

// TestOfflineBenchmarkCapitalProfessionTrainers: a character in a capital who asks where to train a profession is sent first to the
// dataset's trainer in that capital, and to no trainer in the other side's capitals (the dataset's faction column is empty).
func TestOfflineBenchmarkCapitalProfessionTrainers(t *testing.T) {
	ds := rcNamed{store.NewDir(qbPlacesDir)}
	var out []benchRow
	for _, tc := range rcTrainerCases {
		rows, _ := ds.Filter("trainers", store.Range{Column: "profession", Min: tc.prof, Max: tc.prof})
		var want []string
		for _, r := range rows {
			if text(r["zone_name"]) == tc.city {
				want = append(want, fmt.Sprintf("Synthetic Trainer %d", num(r["npc_id"])))
			}
		}
		ways := learn.ProfessionTrainers(rcAt(tc.faction, tc.city), ds, nil, tc.prof)
		var lines, wrong []string
		for _, w := range ways {
			lines = append(lines, w.Line)
			if slices.Contains(tc.other, w.Zone) {
				wrong = append(wrong, w.Zone)
			}
		}
		first := len(ways) > 0 && ways[0].Zone == tc.city && slices.Contains(want, ways[0].Name)
		out = append(out, benchRow{fmt.Sprintf("%s in %s: where to train %s", tc.faction, tc.city, tc.prof), strings.Join(lines, "; "),
			fmt.Sprintf("trainers in %s: %s", tc.city, strings.Join(want, ", ")), rcCitySrc, resultIf(first && len(wrong) == 0, "the capital's trainer is not first, or another side's capital is offered")})
	}
	finish(t, "advice-capital-profession-trainers", "Offline capital cities: the profession trainer of the character's own capital", out)
}

var rcCrossCases = []struct {
	faction, prof string
	other         []string
}{
	{"Horde", "Herbalism", rcAllyCaps}, {"Horde", "Tailoring", rcAllyCaps}, {"Alliance", "Mining", rcHordeCaps}, {"Alliance", "Skinning", rcHordeCaps},
	{"Alliance", "Alchemy", rcHordeCaps}, {"Horde", "Cooking", rcAllyCaps},
}

// TestOfflineBenchmarkCapitalOtherSide: a character anywhere is not offered a profession trainer in the other side's capital, where the
// dataset has a row for one (it holds no faction for any trainer).
func TestOfflineBenchmarkCapitalOtherSide(t *testing.T) {
	ds := rcNamed{store.NewDir(qbPlacesDir)}
	var out []benchRow
	for _, tc := range rcCrossCases {
		rows, _ := ds.Filter("trainers", store.Range{Column: "profession", Min: tc.prof, Max: tc.prof})
		var inOther []string
		for _, r := range rows {
			if slices.Contains(tc.other, text(r["zone_name"])) {
				inOther = append(inOther, text(r["zone_name"]))
			}
		}
		c := character.Character{Faction: tc.faction, Class: "MAGE"}
		var lines, wrong []string
		for _, w := range learn.ProfessionTrainers(c, ds, nil, tc.prof) {
			lines = append(lines, w.Line)
			if slices.Contains(tc.other, w.Zone) {
				wrong = append(wrong, w.Zone)
			}
		}
		out = append(out, benchRow{fmt.Sprintf("%s: %s trainers", tc.faction, tc.prof), strings.Join(lines, "; "),
			fmt.Sprintf("dataset rows in the other side's capitals: %v; none to be offered", inOther), rcCitySrc, resultIf(len(wrong) == 0, "a trainer in the other side's capital is offered")})
	}
	finish(t, "advice-capital-other-side", "Offline capital cities: no trainer in the other side's capital", out)
}

var rcServices = []struct{ q, city, what string }{
	{"where is the bank in Stormwind?", "Stormwind City", "bank"},
	{"where is the auctioneer in Orgrimmar?", "Orgrimmar", "auction house"},
	{"which innkeeper is in Ironforge?", "Ironforge", "innkeeper"},
	{"can a mage portal me from Undercity to Stormwind?", "Undercity", "portal"},
	{"where is the mailbox in Thunder Bluff?", "Thunder Bluff", "mailbox"},
	{"where do I find the stable master in Darnassus?", "Darnassus", "stable master"},
}

// TestOfflineBenchmarkCapitalServices: a question about a bank, auction house, innkeeper, mailbox, stable master or portal in a capital
// gets the line that the data holds none and names no NPC or position, and the dataset has no table of one.
func TestOfflineBenchmarkCapitalServices(t *testing.T) {
	ds := worldStore()
	zones := worldZones(t)
	var out []benchRow
	for _, tc := range rcServices {
		f := ask.LimitFacts(tc.q)
		_, mapped := zones[tc.city]
		tbl := strings.ReplaceAll(tc.what, " ", "_") + "s"
		_, err := ds.Filter(tbl, store.Range{Column: "zone_id", Min: int64(0)})
		ok := strings.Contains(f, "Capital city services:") && strings.Contains(f, "Say so") && !rcDigit.MatchString(f) && err != nil
		out = append(out, benchRow{fmt.Sprintf("%s: \"%s\"", tc.city, tc.q), firstLine(f), fmt.Sprintf("table %s: none; ui_maps zone row for %s: %v", tbl, tc.city, mapped), rcServiceSrc,
			resultIf(ok, "no statement that the data holds none, a figure in it, or a table exists")})
	}
	finish(t, "advice-capital-services", "Offline capital cities: bank, auction house, inn, mailbox, stable master and portal", out)
}

var rcLeaders = []struct{ q, who string }{
	{"who is the Warchief of the Horde?", "Thrall"},
	{"where do I find Thrall in Orgrimmar?", "Thrall"},
	{"who leads the Alliance, Varian Wrynn?", "Varian Wrynn"},
	{"where is Cairne Bloodhoof?", "Cairne Bloodhoof"},
	{"who is the high king of Ironforge, Magni?", "Magni Bronzebeard"},
	{"what quest does Sylvanas give in Undercity?", "Sylvanas Windrunner"},
}

// TestOfflineBenchmarkFactionLeaders: a question about a faction leader gets the line that the data marks none, with no title or position
// in it, and the dataset's npcs table has no column that names a leader.
func TestOfflineBenchmarkFactionLeaders(t *testing.T) {
	ds := usabStore()
	rows, _ := ds.Filter("npcs", store.Range{Column: "id", Min: int64(0), Max: int64(1 << 40)})
	var cols []string
	if len(rows) > 0 {
		for k := range rows[0] {
			cols = append(cols, k)
		}
		slices.Sort(cols)
	}
	leaderCol := slices.ContainsFunc(cols, func(c string) bool {
		return strings.Contains(c, "leader") || strings.Contains(c, "title") || strings.Contains(c, "ruler")
	})
	var out []benchRow
	for _, tc := range rcLeaders {
		f := ask.LimitFacts(tc.q)
		named, _ := ds.Filter("npcs", store.Range{Column: "name", Min: tc.who, Max: tc.who})
		ok := strings.Contains(f, "Faction leaders:") && strings.Contains(f, "Say so") && !rcDigit.MatchString(f) && !leaderCol
		out = append(out, benchRow{fmt.Sprintf("\"%s\"", tc.q), firstLine(f), fmt.Sprintf("npcs columns: %s (no leader column); %d npcs rows named %s in the snapshot", strings.Join(cols, ", "), len(named), tc.who), rcLeaderSrc,
			resultIf(ok, "no statement that the data marks no leader, a figure in it, or a leader column exists")})
	}
	finish(t, "advice-faction-leaders", "Offline faction leaders: the data marks none, and the advisor says so", out)
}

var rcNoAnswer = []struct{ q, topic, ref string }{
	{"how much frost resistance does a dwarf get?", "Racial traits:", "no racial resistance row: say so, name no percentage"},
	{"how much haste does Berserking give a troll?", "Racial traits:", "no racial ability row: say so, name no haste"},
	{"is there a bank in Silvermoon City?", "Capital city services:", "no bank row and no map of the city: say so, name none"},
	{"what is the auction house cut in Stormwind?", "Auction House:", "no auction house fee row: say so, name none"},
	{"who is the leader of the Horde in Thunder Bluff?", "Faction leaders:", "no leader row: say so, name none"},
	{"which portal takes a mage to Ironforge?", "Capital city services:", "no portal row: say so, name none"},
}

// TestOfflineBenchmarkRaceCityLeaderNoAnswer: what the dataset cannot answer about a racial's size, a bank, an auction house cut, a portal
// or a leader gets the line that the data holds none, with no figure in it, and the dataset has no table that would hold one.
func TestOfflineBenchmarkRaceCityLeaderNoAnswer(t *testing.T) {
	ds := usabStore()
	var out []benchRow
	for _, tc := range rcNoAnswer {
		f := ask.LimitFacts(tc.q)
		ok := strings.Contains(f, tc.topic) && strings.Contains(f, "Say so") && !rcDigit.MatchString(f)
		out = append(out, benchRow{"\"" + tc.q + "\"", firstLine(f), tc.ref, rcLeaderSrc, resultIf(ok, "no statement that the data holds none, or a figure in it")})
	}
	for _, tbl := range rcRaceTables {
		_, err := ds.Filter(tbl, store.Range{Column: "id", Min: int64(0)})
		out = append(out, benchRow{"a table of " + strings.ReplaceAll(tbl, "_", " "), fmt.Sprint(err), "no such table in the dataset", rcRaceSrc, resultIf(err != nil, "a table was invented")})
	}
	rows, _ := ds.Filter("weapon_skills", store.Range{Column: "skill_line_id", Min: int64(0)})
	raceCol := false
	for k := range rows[0] {
		raceCol = raceCol || strings.Contains(k, "race")
	}
	out = append(out, benchRow{"weapon_skills: a race column", fmt.Sprint(raceCol), "the dataset's weapon skills carry a class, not a race", rcRaceSrc, resultIf(!raceCol, "a race column exists: the racial weapon bonus is answerable")})
	finish(t, "advice-race-city-leader-no-answer", "Offline races, capitals and leaders: no answer in the dataset", out)
}

// TestOfflineBenchmarkRaceCityLeaderQuiet: a question that only shares a word with a racial, a city service or a leader gets none of those facts.
func TestOfflineBenchmarkRaceCityLeaderQuiet(t *testing.T) {
	var out []benchRow
	for _, q := range []string{"where is the weapon master in Orgrimmar?", "what quest should I do next?", "how do I get from Orgrimmar to Undercity?",
		"which race can be a paladin?", "what level is Thunder Bluff's Skinning trainer?", "what does rank 3 Frostbolt cost?"} {
		f := ask.LimitFacts(q)
		bad := strings.Contains(f, "Racial traits:") || strings.Contains(f, "Capital city services:") || strings.Contains(f, "Faction leaders:")
		out = append(out, benchRow{"\"" + q + "\"", f, "no racial, capital service or leader facts on a question about something else", rcServiceSrc, resultIf(!bad, "racial, city service or leader facts on a question about something else")})
	}
	finish(t, "advice-race-city-leader-quiet", "Offline races, capitals and leaders: questions about something else", out)
}

package professions

import (
	"fmt"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/store"
)

// The offline profession pairing and rank-up quest benchmark: which two primary professions the advisor suggests (and what it says
// when both slots are already taken or the pair has no note), and the book or quest a rank after Apprentice is gained from, with
// who gives it, the skill and the character level it needs, beside the dataset's rows, with no network. Pairings read
// profession_notes (pairings, feeders, rules) of testdata/advice_snapshot; ranks read profession_tiers (how, trainer_name, place,
// skill_required, min_level, note) of testdata/places_snapshot. The class a pair suits is the armor convention of the game
// (public: cloth for Tailoring, leather for Skinning and Leatherworking, plate and mail for Mining and Blacksmithing); the dataset
// has no class column, and no prerequisite of a rank quest. A case whose answer the tables lack checks that the advisor says so and
// claims nothing. make offline-benchmark runs it; WOWC_BENCH_REPORT receives the tables.

const (
	pairSrc      = "dataset profession_notes pairings (subject, related, text), feeders, rules (slots)"
	pairClassSrc = "game armor convention (public: cloth Tailoring, leather Skinning, plate Mining and Blacksmithing); dataset profession_notes pairings"
	questSrc     = "dataset profession_tiers (how, trainer_name, place, skill_required, min_level, note)"
	questNoSrc   = "the dataset has no prerequisite column for a rank quest or book"
)

func skillsOf(names ...string) []character.Skill {
	var out []character.Skill
	for _, n := range names {
		out = append(out, character.Skill{Name: n})
	}
	return out
}

// refPairing is the dataset's pairing row of two professions, in either order ("" when it holds none).
func refPairing(t *testing.T, a, b string) string {
	t.Helper()
	rows, err := placesStore().Filter("profession_notes", store.Range{Column: "section", Min: "pairings", Max: "pairings"})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		s, o := asText(r["subject"]), asText(r["related"])
		if strings.EqualFold(s, a) && strings.EqualFold(o, b) || strings.EqualFold(s, b) && strings.EqualFold(o, a) {
			return s + " + " + o + ": " + asText(r["text"])
		}
	}
	return ""
}

var pairCases = []struct {
	label string
	char  character.Character
	first [2]string // the pair the advisor puts first
	why   string    // the class or holdings reason the pair leads
}{
	{"mage, no professions", character.Character{Class: "MAGE"}, [2]string{"Tailoring", "Enchanting"}, "a cloth wearer: Tailoring makes the cloth and Enchanting disenchants it"},
	{"warrior, no professions", character.Character{Class: "WARRIOR"}, [2]string{"Mining", "Blacksmithing"}, "a plate wearer: Mining feeds the forge"},
	{"rogue, no professions", character.Character{Class: "ROGUE"}, [2]string{"Skinning", "Leatherworking"}, "a leather wearer: skinned hides become leather armor"},
	{"priest with Herbalism", character.Character{Class: "PRIEST", Professions: skillsOf("Herbalism")}, [2]string{"Herbalism", "Alchemy"}, "the herbs she gathers are the potions' reagent"},
	{"paladin with Alchemy", character.Character{Class: "PALADIN", Professions: skillsOf("Alchemy")}, [2]string{"Herbalism", "Alchemy"}, "the Alchemy she holds is fed by Herbalism"},
	{"warrior with Mining", character.Character{Class: "WARRIOR", Professions: skillsOf("Mining")}, [2]string{"Mining", "Blacksmithing"}, "Mining holds, and a plate wearer takes the forge, not Engineering"},
	{"warlock with Enchanting", character.Character{Class: "WARLOCK", Professions: skillsOf("Enchanting")}, [2]string{"Tailoring", "Enchanting"}, "Enchanting holds, and Tailoring is the cloth that feeds it"},
}

// TestOfflineBenchmarkPairings: the pair the advisor puts first, with the dataset row's own reason and the profession the character
// already has, and nothing the dataset does not hold: five pairs, each worded as its row.
func TestOfflineBenchmarkPairings(t *testing.T) {
	ds := placesStore()
	var out []benchRow
	for _, tc := range pairCases {
		got := Pairings(tc.char, ds)
		ref := refPairing(t, tc.first[0], tc.first[1])
		line, ok := "no pairing", false
		if len(got) > 0 {
			line = fmt.Sprintf("%s + %s: %s", got[0].A, got[0].B, got[0].Reason)
			ok = got[0].A == tc.first[0] && got[0].B == tc.first[1] && ref != "" && strings.HasPrefix(line, ref)
			for _, h := range tc.char.Professions {
				if strings.EqualFold(h.Name, tc.first[0]) || strings.EqualFold(h.Name, tc.first[1]) {
					ok = ok && strings.Contains(got[0].Reason, "You already have "+h.Name+".")
				}
			}
		}
		out = append(out, benchRow{tc.label + ": the pair put first", line, ref + " (" + tc.why + ")", pairClassSrc, resultIf(ok, "another pair first, or a reason other than the dataset row's")})
	}
	// every pair listed is a dataset row, once, and worded as the row
	for _, c := range []character.Character{{Class: "MAGE"}, {Class: "WARRIOR"}, {Class: "SHAMAN"}} {
		got := Pairings(c, ds)
		okAll := len(got) == 5
		var seen []string
		for _, a := range got {
			ref := refPairing(t, a.A, a.B)
			okAll = okAll && ref != "" && strings.HasPrefix(a.A+" + "+a.B+": "+a.Reason, ref)
			seen = append(seen, a.A+"+"+a.B)
		}
		out = append(out, benchRow{strings.ToLower(c.Class) + ": every pair listed", strings.Join(seen, ", "), "the five pairings rows, each with its own text", pairSrc, resultIf(okAll, "a pair the dataset does not hold, a missing pair, or another text")})
	}
	finishOffline(t, "advice-pairings", "Offline profession pairings: first pair and the dataset's wording", out)
}

var pairFullCases = []struct {
	label string
	char  character.Character
	pair  [2]string
	noted bool // the dataset has a pairing row for the held pair
}{
	{"warrior with Mining and Blacksmithing", character.Character{Class: "WARRIOR", Professions: skillsOf("Mining", "Blacksmithing")}, [2]string{"Mining", "Blacksmithing"}, true},
	{"rogue with Skinning, Leatherworking and Cooking", character.Character{Class: "ROGUE", Professions: skillsOf("Skinning", "Leatherworking", "Cooking")}, [2]string{"Skinning", "Leatherworking"}, true},
	{"hunter with Mining and Herbalism", character.Character{Class: "HUNTER", Professions: skillsOf("Mining", "Herbalism")}, [2]string{"Mining", "Herbalism"}, false},
	{"mage with Alchemy and Engineering", character.Character{Class: "MAGE", Professions: skillsOf("Alchemy", "Engineering")}, [2]string{"Alchemy", "Engineering"}, false},
}

// TestOfflineBenchmarkPairingsSlotsFull: a character holds two primary professions (profession_notes rules, slots; Cooking, Fishing
// and First Aid do not count), so with both taken the answer is about that pair: the dataset's note when it has one, and that it
// holds none when it does not, and any other pair is a change that drops one, never offered as an addition.
func TestOfflineBenchmarkPairingsSlotsFull(t *testing.T) {
	ds := placesStore()
	var out []benchRow
	for _, tc := range pairFullCases {
		got := Pairings(tc.char, ds)
		ref := refPairing(t, tc.pair[0], tc.pair[1])
		first := Advice{}
		if len(got) > 0 {
			first = got[0]
		}
		line := fmt.Sprintf("%s + %s: %s", first.A, first.B, first.Reason)
		ok := len(got) > 0 && first.A+first.B == tc.pair[0]+tc.pair[1] || len(got) > 0 && first.A+first.B == tc.pair[1]+tc.pair[0]
		if tc.noted {
			ok = ok && ref != "" && strings.Contains(first.Reason, "You already have both.") && strings.HasPrefix(line, ref)
		} else {
			ok = ok && ref == "" && strings.Contains(first.Reason, "no pairing note") && !strings.Contains(first.Reason, "You already have both.")
		}
		for _, o := range got[min(1, len(got)):] {
			ok = ok && strings.Contains(o.Reason, "slots are taken")
		}
		out = append(out, benchRow{tc.label + ": both slots taken", line, ifElse(tc.noted, ref, "no pairing row for the held pair") + "; other pairs mean dropping one", pairSrc, resultIf(ok, "a third profession is offered as an addition, or the held pair's note is wrong or made up")})
	}
	finishOffline(t, "advice-pairings-full", "Offline profession pairings: both primary slots taken", out)
}

func ifElse(c bool, a, b string) string {
	if c {
		return a
	}
	return b
}

// TestOfflineBenchmarkPairingsNoData: with no profession notes at all the answer says any two are fine and names no pair.
func TestOfflineBenchmarkPairingsNoData(t *testing.T) {
	var out []benchRow
	got := Pairings(character.Character{Class: "MAGE"}, store.NewDir(t.TempDir()))
	text := RenderPairings(got)
	out = append(out, benchRow{"no notes in the dataset: the pairing answer", strings.TrimSpace(text), "no pairings rows: no pair is named", pairSrc,
		resultIf(len(got) == 0 && strings.Contains(text, "Any two primary professions are fine") && !strings.Contains(text, " + "), "a pair was made up")})
	full := Pairings(character.Character{Class: "MAGE", Professions: skillsOf("Tailoring", "Enchanting")}, store.NewDir(t.TempDir()))
	line := ""
	if len(full) > 0 {
		line = full[0].A + " + " + full[0].B + ": " + full[0].Reason
	}
	out = append(out, benchRow{"no notes, both slots taken: the held pair", line, "no pairings rows: the pair has no note", pairSrc,
		resultIf(len(full) == 1 && strings.Contains(full[0].Reason, "no pairing note"), "a pairing note was made up")})
	finishOffline(t, "advice-pairings-no-answer", "Offline profession pairings: no answer in the dataset", out)
}

// ---- rank-up quests ----

type questCase struct {
	prof, faction, tier string
	level               int
}

var questCases = []questCase{
	{"First Aid", "Alliance", "Artisan", 40},
	{"First Aid", "Horde", "Artisan", 36},
	{"Cooking", "Alliance", "Artisan", 40},
	{"Cooking", "Horde", "Artisan", 35},
	{"Fishing", "Alliance", "Artisan", 38},
	{"Fishing", "Horde", "Artisan", 42},
	{"First Aid", "Horde", "Expert", 25},
	{"Cooking", "Alliance", "Expert", 22},
}

// questRow is the tier row of a quest or book rank on the character's side.
type questRow struct {
	how, note, giver, place string
	skill, level            int64
}

func readQuest(t *testing.T, c questCase) questRow {
	t.Helper()
	rows, err := placesStore().Filter("profession_tiers", store.Range{Column: "profession", Min: c.prof, Max: c.prof})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if asText(r["tier"]) != c.tier {
			continue
		}
		if f := asText(r["faction"]); f != "" && f != "Both" && !strings.EqualFold(f, c.faction) {
			continue
		}
		return questRow{asText(r["how"]), strings.TrimSpace(strings.TrimSuffix(asText(r["note"]), " "+asText(r["how"]))), asText(r["trainer_name"]), asText(r["place"]), asI64(r["skill_required"]), asI64(r["min_level"])}
	}
	t.Fatalf("no tier row for %+v", c)
	return questRow{}
}

func (q questRow) String() string {
	return fmt.Sprintf("%s %q from %s in %s, skill %d, character level %d", q.how, q.note, q.giver, q.place, q.skill, q.level)
}

func TestOfflineBenchmarkRankQuestsA(t *testing.T) { rankQuests(t, "a", questCases[:4]) }
func TestOfflineBenchmarkRankQuestsB(t *testing.T) { rankQuests(t, "b", questCases[4:]) }

// rankQuests: the rank gained by a quest or a book is named with its quest or book, the giver and place of the character's side, and
// the skill and character level it needs (the character holds the rank before it at its cap).
func rankQuests(t *testing.T, part string, cases []questCase) {
	var out []benchRow
	for _, tc := range cases {
		ref := readQuest(t, tc)
		cap := 225
		if tc.tier == "Expert" {
			cap = 150
		}
		words := rankWhere(rankCase{tc.prof, tc.faction, tc.tier, cap - 5, cap, tc.level})
		gate := firstSentence(words, "Train now: ")
		given := firstSentence(words, "It is not taught by a trainer")
		label := fmt.Sprintf("%s %s (%s, level %d)", tc.prof, tc.tier, strings.ToLower(tc.faction), tc.level)
		okGate := strings.HasPrefix(gate, fmt.Sprintf("Train now: %s %s (character level %d, skill %d,", tc.prof, tc.tier, ref.level, ref.skill))
		if int64(tc.level) < ref.level {
			okGate = okGate && strings.Contains(gate, fmt.Sprintf("You are level %d, so not yet.", tc.level))
		}
		out = append(out, benchRow{label + ": skill and level gate", gate, ref.String(), questSrc, resultIf(okGate, "another skill or character level than the tier row")})
		kind := map[string]string{"quest": "it comes from a quest", "book": "it is a book"}[ref.how]
		okGive := strings.Contains(given, kind) && strings.Contains(given, ref.giver+" in "+ref.place) && !strings.Contains(words, "Trainer to visit")
		out = append(out, benchRow{label + ": the " + ref.how + " and who gives it", given, ref.String(), questSrc, resultIf(okGive, "another giver, or no trainer line rule kept")})
		okName := ref.note != "" && strings.Contains(given, "("+ref.note+")")
		out = append(out, benchRow{label + ": the " + ref.how + "'s name", given, ref.String(), questSrc, resultIf(okName, "the "+ref.how+" is not named")})
	}
	finishOffline(t, "advice-rank-quests-"+part, "Offline profession rank-up quests and books ("+part+")", out)
}

// TestOfflineBenchmarkRankQuestRoute: the skill-up route names the same quest and giver as the path.
func TestOfflineBenchmarkRankQuestRoute(t *testing.T) {
	ds := placesStore()
	var out []benchRow
	for _, tc := range []questCase{{"First Aid", "Horde", "Artisan", 40}, {"Cooking", "Alliance", "Artisan", 40}, {"Fishing", "Horde", "Artisan", 40}} {
		ref := readQuest(t, tc)
		c := rankChar(rankCase{tc.prof, tc.faction, tc.tier, 215, 225, tc.level})
		got := ""
		for _, l := range strings.Split(RouteFacts(c, ds, nil, tc.prof), "\n") {
			if strings.HasPrefix(l, "Before ") {
				got = l
			}
		}
		ok := strings.Contains(got, fmt.Sprintf("you must get %s %s (character level %d, skill %d;", tc.prof, tc.tier, ref.level, ref.skill)) &&
			strings.Contains(got, "("+ref.note+")") && strings.Contains(got, ref.giver)
		out = append(out, benchRow{fmt.Sprintf("%s %s (%s): the route's gate", tc.prof, tc.tier, strings.ToLower(tc.faction)), got, ref.String(), questSrc, resultIf(ok, "the route names another quest, giver, skill or level")})
	}
	finishOffline(t, "advice-rank-quest-route", "Offline skill-up route: the rank quest", out)
}

// TestOfflineBenchmarkRankQuestNoAnswer: what the tier rows lack is not made up: no prerequisite quest, no training price for a quest, and no
// quest at all for a rank a trainer teaches.
func TestOfflineBenchmarkRankQuestNoAnswer(t *testing.T) {
	var out []benchRow
	tc := questCase{"First Aid", "Alliance", "Artisan", 40}
	words := rankWhere(rankCase{tc.prof, tc.faction, tc.tier, 215, 225, tc.level})
	low := strings.ToLower(words)
	out = append(out, benchRow{"First Aid Artisan quest: its prerequisite", words, "no prerequisite in the dataset", questNoSrc,
		resultIf(!strings.Contains(low, "prerequisite") && !strings.Contains(low, "after you finish") && !strings.Contains(low, "must first"), "a prerequisite was claimed")})
	out = append(out, benchRow{"First Aid Artisan quest: what it costs", rankPrice(words), "cost_copper is empty in the tier row", questSrc,
		resultIf(readQuest(t, tc).how == "quest" && strings.Contains(words, "lists no training price") && !strings.Contains(words, "Training costs"), "a price was given")})
	alch := rankWhere(rankCase{"Alchemy", "Horde", "Expert", 140, 150, 22})
	out = append(out, benchRow{"Alchemy Expert: a quest or book", firstSentence(alch, "Train now: "), "the tier row says trainer: no quest or book", questSrc,
		resultIf(readRank(t, "Alchemy", "Expert", "Horde").how == "trainer" && !strings.Contains(alch, "It is not taught by a trainer") && strings.Contains(alch, "Trainer to visit"), "a quest or book was named for a trainer rank")})
	finishOffline(t, "advice-rank-quest-no-answer", "Offline rank-up quests: no answer in the dataset", out)
}

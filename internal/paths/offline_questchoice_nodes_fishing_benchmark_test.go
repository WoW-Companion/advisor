package paths_test

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/ask"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/gather"
	"github.com/WoW-Companion/advisor/internal/paths"
	"github.com/WoW-Companion/advisor/internal/store"
)

// The offline quest-reward-choice, gathering-node and fishing benchmark: the quest reward line the advisor gives a character (whether
// the reward is one of a choice), the facts it puts in front of the model for a question about the skill an ore or herb node needs and
// where it is, and for a question about where a fish or a zone's catches are, beside the dataset's rows and the game's skill tables,
// with no network. Characters are synthetic (several classes, levels and both factions). The dataset holds no quest reward row, so the
// reward rows are composed from real quests and real items as in the quest benchmark. It reads testdata/places_snapshot and
// advice_snapshot (quests), testdata/tradeskills_snapshot (gathering nodes) and testdata/companions_snapshot (fishing catches).
// A difference the dataset's own rows cause is marked "(data)". make offline-benchmark runs it; WOWC_BENCH_REPORT receives the tables.

const (
	qcQuestSrc = "dataset loot_sources (quest reward rows: one distinct item is a plain reward, two or more a choice), quests_observed (title, level, faction, zone)"
	qcNodeSrc  = "dataset gathering_nodes (skill_required, zone_name); game: Warcraft Wiki Mining and Herbalism skill tables, Wowhead Classic node pages"
	qcFishSrc  = "dataset fishing_catches (zone_name, item_id, casts, rate); game home zones of the two factions"
)

// ---- quest reward choice ----

type qcQuestCase struct {
	label string
	c     character.Character
	item  int64
	quest int64
	rows  []int64 // the reward rows of the quest: one entry per row, so a repeated item is a duplicate row
}

func qcChar(faction, class string, level int) character.Character {
	return character.Character{Name: "Benchtest", Faction: faction, Class: class, Level: &level}
}

func qcQuestCases() []qcQuestCase {
	return []qcQuestCase{
		{"Horde warrior 30, one reward", qcChar("Horde", "WARRIOR", 30), iBelt, qBattleHillsbrad, []int64{iBelt}},
		{"Horde mage 28, one reward", qcChar("Horde", "MAGE", 28), iPants, qMissingShipment, []int64{iPants}},
		{"Horde priest 26, choice of three", qcChar("Horde", "PRIEST", 26), iVest, qBetrayal, []int64{iBreastplate, iVest, iBarbaric}},
		{"Horde rogue 27, choice of three", qcChar("Horde", "ROGUE", 27), iBarbaric, qBetrayal, []int64{iBreastplate, iVest, iBarbaric}},
		{"Horde hunter 24, choice of two", qcChar("Horde", "HUNTER", 24), iBoots, qBerensPeril, []int64{iBoots, iGauntlets}},
		{"Horde shaman 25, choice of two", qcChar("Horde", "SHAMAN", 25), iGauntlets, qBerensPeril, []int64{iBoots, iGauntlets}},
		{"Alliance paladin 22, one reward", qcChar("Alliance", "PALADIN", 22), iBelt, qDusky, []int64{iBelt}},
		{"Alliance druid 21, choice of two", qcChar("Alliance", "DRUID", 21), iBoots, qDusky, []int64{iBoots, iGauntlets}},
		{"Alliance warlock 25, quest of the Horde", qcChar("Alliance", "WARLOCK", 25), iBelt, qBattleHillsbrad, []int64{iBelt}},
		{"Horde warrior 30, one reward listed twice", qcChar("Horde", "WARRIOR", 30), iBelt, qBattleHillsbrad, []int64{iBelt, iBelt}},
		{"Horde mage 30, a choice with one item listed twice", qcChar("Horde", "MAGE", 30), iVest, qBetrayal, []int64{iBreastplate, iVest, iVest}},
		{"Horde priest 30, one reward listed three times", qcChar("Horde", "PRIEST", 30), iPants, qMissingShipment, []int64{iPants, iPants, iPants}},
		{"Horde warrior 10, quest 16 levels above", qcChar("Horde", "WARRIOR", 10), iBelt, qBattleHillsbrad, []int64{iBelt}},
		{"Horde warrior 60, a quest far below", qcChar("Horde", "WARRIOR", 60), iBelt, qBattleHillsbrad, []int64{iBelt}},
	}
}

// qcOffered is whether the dataset's quest row lets the character take the quest: a level at most two above the character's and the
// character's own side (the quest row's faction is empty for either side).
func qcOffered(t *testing.T, c character.Character, quest int64) (bool, store.Row) {
	t.Helper()
	q := qbQuest(t, quest)
	side := text(q["faction"])
	ok := num(q["level"]) <= int64(*c.Level)+2 && (side == "" || strings.EqualFold(side, c.Faction))
	return ok, q
}

// TestOfflineBenchmarkQuestRewardChoice: a quest reward is worded as a choice only when the dataset gives the quest two or more different
// reward items; a reward the dataset lists twice is still one reward. The quest is named with its zone, and is offered only for the
// character's own side and to a level within two of the quest's.
func TestOfflineBenchmarkQuestRewardChoice(t *testing.T) {
	var out []benchRow
	for _, tc := range qcQuestCases() {
		var rows []store.Row
		distinct := map[int64]bool{}
		for _, it := range tc.rows {
			rows = append(rows, qbReward(it, tc.quest))
			distinct[it] = true
		}
		got := questLine(paths.For(tc.c, qbRewards{qbStore(), rows}, tc.item, paths.Options{}))
		offered, q := qcOffered(t, tc.c, tc.quest)
		title, zone := text(q["title"]), text(q["category"])
		ref := fmt.Sprintf("%d reward row(s), %d distinct item(s); level %d %s quest %s (%s)", len(tc.rows), len(distinct), num(q["level"]), text(q["faction"]), title, zone)
		ok := got == ""
		if offered {
			verb := " for it"
			if len(distinct) > 1 {
				verb = " and choose it"
			}
			ok = strings.HasPrefix(got, "Finish "+title+verb) && strings.Contains(got, "in "+zone)
		} else {
			ref = "not offered: " + ref
		}
		if got == "" {
			got = "no quest path"
		}
		out = append(out, benchRow{tc.label + ": quest reward line", got, ref, qcQuestSrc, resultIf(ok, "the choice wording, the quest or whether it is offered")})
	}
	finish(t, "advice-quest-reward-choice", "Offline quest reward choice: the line the advisor gives", out)
}

// ---- gathering nodes ----

type qcNodeCase struct {
	question string
	profs    []string // the professions whose nodes the question is about
	nodes    []string // the nodes it names; empty for every node of the professions
	skill    int      // the skill the question states, 0 for none
}

var qcNodeCases = []qcNodeCase{
	{"what skill do I need to mine tin?", []string{"Mining"}, []string{"Tin Vein"}, 0},
	{"what mining skill do I need for copper vein?", []string{"Mining"}, []string{"Copper Vein"}, 0},
	{"which mining skill does an Iron Deposit need and where is it?", []string{"Mining"}, []string{"Iron Deposit"}, 0},
	{"what herbalism skill do I need to pick Briarthorn?", []string{"Herbalism"}, []string{"Briarthorn"}, 0},
	{"where can I gather peacebloom?", []string{"Herbalism"}, []string{"Peacebloom"}, 0},
	{"what skill do I need to mine gold?", []string{"Mining"}, nil, 0},
	{"what can I mine at 100 mining?", []string{"Mining"}, nil, 100},
	{"what can I mine at mining 70?", []string{"Mining"}, nil, 70},
	{"what ore should I mine at 130 mining?", []string{"Mining"}, nil, 130},
	{"what herbs can I pick at herbalism 50?", []string{"Herbalism"}, nil, 50},
	{"which herbs can I gather with herbalism skill of 15?", []string{"Herbalism"}, nil, 15},
	{"can I gather briarthorn at skill 60?", []string{"Herbalism"}, []string{"Briarthorn"}, 60},
	{"can I pick briarthorn at herbalism 70?", []string{"Herbalism"}, []string{"Briarthorn"}, 70},
	{"where to find tin?", []string{"Mining"}, []string{"Tin Vein"}, 0},
	{"where do I farm iron ore with 125 mining?", []string{"Mining"}, []string{"Iron Deposit"}, 125},
	{"what mining skill is needed for silver?", []string{"Mining"}, []string{"Silver Vein"}, 0},
	{"which herbalism skill do I need for Earthroot and where does it grow?", []string{"Herbalism"}, []string{"Earthroot"}, 0},
	{"where is mine?", nil, nil, 0},
	{"what is mine to keep?", nil, nil, 0},
}

// qcDatasetNodes are the nodes of the dataset with a skill, as the advisor lists them: the lowest skill of each name.
func qcDatasetNodes(t *testing.T, prof string) map[string]int64 {
	t.Helper()
	out := map[string]int64{}
	for _, r := range tsRows(t, "gathering_nodes", "profession", prof) {
		name, skill := text(r["name"]), num(r["skill_required"])
		if skill <= 0 {
			continue
		}
		if cur, ok := out[name]; !ok || skill < cur {
			out[name] = skill
		}
	}
	return out
}

// TestOfflineBenchmarkGatherNodeQuestions: every phrasing of a question about an ore or herb node, the skill it needs or what a skill
// reaches, gets the dataset's skill and zones for the node (or for every node of the profession), and a skill the question states is
// worked out against them; a question that is no gathering question gets no node facts.
func TestOfflineBenchmarkGatherNodeQuestions(t *testing.T) {
	ds := tsStore()
	var out []benchRow
	for _, tc := range qcNodeCases {
		facts := ask.GatherFacts(ds, tc.question)
		if len(tc.profs) == 0 {
			out = append(out, benchRow{"\"" + tc.question + "\"", firstLine(facts) + "(no node facts)", "not a question about a node", qcNodeSrc, resultIf(facts == "", "node facts for a question that is not about a node")})
			continue
		}
		want := map[string]int64{}
		for _, p := range tc.profs {
			for n, s := range qcDatasetNodes(t, p) {
				if len(tc.nodes) == 0 || slices.Contains(tc.nodes, n) {
					want[n] = s
				}
			}
		}
		ok := len(want) > 0
		for n, s := range want {
			ok = ok && strings.Contains(facts, fmt.Sprintf("- %s: needs %s %d; found in ", n, tc.profs[0], s))
		}
		for _, l := range strings.Split(facts, "\n") {
			if strings.HasPrefix(l, "- ") {
				name, _, _ := strings.Cut(l[2:], ":")
				_, listed := want[name]
				ok = ok && listed
			}
		}
		ref := fmt.Sprintf("%d node(s) of the dataset", len(want))
		if tc.skill > 0 {
			var can, not []string
			byName := make([]string, 0, len(want))
			for n := range want {
				byName = append(byName, n)
			}
			// the advisor lists the nodes by skill, then by name
			slices.SortFunc(byName, func(a, b string) int {
				if want[a] != want[b] {
					return int(want[a] - want[b])
				}
				return strings.Compare(a, b)
			})
			for _, n := range byName {
				if want[n] <= int64(tc.skill) {
					can = append(can, n)
				} else {
					not = append(not, fmt.Sprintf("%s (needs %d)", n, want[n]))
				}
			}
			line := fmt.Sprintf("At %s %d: ", tc.profs[0], tc.skill)
			if len(can) > 0 {
				line += "can gather " + strings.Join(can, ", ")
			} else {
				line += "can gather none of the listed nodes"
			}
			if len(not) > 0 {
				line += "; not yet " + strings.Join(not, ", ")
			}
			ref += "; " + line + "."
			ok = ok && strings.Contains(facts, line+".")
		} else {
			ok = ok && !strings.Contains(facts, "\nAt ")
		}
		ok = ok && strings.Contains(facts, "name no skill or zone from memory")
		out = append(out, benchRow{"\"" + tc.question + "\"", strings.ReplaceAll(facts, "\n", " / "), ref, qcNodeSrc, resultIf(ok, "the node's skill or zones, or the skill the question states")})
	}
	finish(t, "advice-gather-node-questions", "Offline gathering: the facts for a question about a node", out)
}

// TestOfflineBenchmarkGatherNodeGameSkill: the lowest skill the dataset gives each node against the game's own table.
func TestOfflineBenchmarkGatherNodeGameSkill(t *testing.T) {
	var out []benchRow
	for _, prof := range []string{"Mining", "Herbalism"} {
		for n, s := range qcDatasetNodes(t, prof) {
			guide, ok := gnGuide[n]
			if !ok {
				continue
			}
			out = append(out, benchRow{n + ": the skill it needs", fmt.Sprintf("%d", s), fmt.Sprintf("%d (game)", guide), qcNodeSrc, resultIf(s == guide, fmt.Sprintf("the dataset gives %d, the game %d", s, guide))})
		}
	}
	slices.SortFunc(out, func(a, b benchRow) int { return strings.Compare(a.check, b.check) })
	finish(t, "advice-gather-node-game-skill", "Offline gathering: the dataset's node skill beside the game's", out)
}

// ---- fishing ----

type qcFishCase struct {
	question string
	zone     string // the zone the question names, "" for none
	fish     string // the fish it names, "" for none
}

var qcFishCases = []qcFishCase{
	{"where can I fish in the Barrens?", "The Barrens", ""},
	{"where can I fish in barrens?", "The Barrens", ""},
	{"what do I catch fishing in Hillsbrad Foothills?", "Hillsbrad Foothills", ""},
	{"fishing in stranglethorn vale, what can I catch?", "Stranglethorn Vale", ""},
	{"what fish can I get in silverpine forest?", "Silverpine Forest", ""},
	{"where do I catch longjaw mud snapper?", "", "Raw Longjaw Mud Snapper"},
	{"where can I fish for sagefish?", "", "Raw Sagefish"},
	{"where can I catch a firefin snapper?", "", "Firefin Snapper"},
	{"where do I fish for rockscale cod?", "", "Raw Rockscale Cod"},
	{"where is the best place to level fishing?", "", ""},
	{"how high does my fishing need to be for stranglethorn?", "Stranglethorn Vale", ""},
	{"what can I fish in stormwind?", "", ""},
}

// qcCatchesOf are the rows of the dataset's fishing_catches for the zone or the item.
func qcCatchesOf(t *testing.T, zone string, item int64) []store.Row {
	t.Helper()
	rows, err := compStore().Filter("fishing_catches", store.Range{Column: "zone_id"})
	if err != nil {
		t.Fatal(err)
	}
	var out []store.Row
	for _, r := range rows {
		if zone != "" && text(r["zone_name"]) == zone || item != 0 && num(r["item_id"]) == item {
			out = append(out, r)
		}
	}
	return out
}

var qcCasts = regexp.MustCompile(`\((?:[A-Za-z ]+ home zone, )?(\d+)% of (\d+) casts\)`)

// TestOfflineBenchmarkFishingQuestions: a question that names a fishing zone, with or without the "the" of its name, or a fish gets the
// catches or the zones the dataset's reports give, with their rates and casts; the question never gets a fishing skill, a pool or a lure
// effect (the dataset has none), and a zone or fish with no report is said to have none.
func TestOfflineBenchmarkFishingQuestions(t *testing.T) {
	ds := compStore()
	var out []benchRow
	for _, tc := range qcFishCases {
		facts := ask.GatherFacts(ds, tc.question)
		var item int64
		if tc.fish != "" {
			item = num(rowByName(t, ds, "items", tc.fish)["id"])
		}
		rows := qcCatchesOf(t, tc.zone, item)
		ok := strings.Contains(facts, "holds no fishing skill")
		var ref string
		switch {
		case tc.zone != "":
			ref = fmt.Sprintf("%d catch row(s) in %s", len(rows), tc.zone)
			ok = ok && strings.Contains(facts, "Catches reported in "+tc.zone+", best rate first:") && len(qcCasts.FindAllString(facts, -1)) == min(maxCatchesShown, len(rows))
		case tc.fish != "":
			ref = fmt.Sprintf("%s caught in %d zone(s)", tc.fish, len(rows))
			ok = ok && strings.Contains(facts, tc.fish+" is caught in these zones, best rate first:") && len(qcCasts.FindAllString(facts, -1)) == min(6, len(rows))
			for _, r := range rows[:min(6, len(rows))] {
				ok = ok && strings.Contains(facts, text(r["zone_name"]))
			}
		default:
			ref = "no zone or fish named: the zones the reports cover, and that others have no report"
			ok = ok && strings.Contains(facts, "cover only these zones") && !strings.Contains(facts, "Catches reported in") && !strings.Contains(facts, " is caught in ")
		}
		out = append(out, benchRow{"\"" + tc.question + "\"", strings.ReplaceAll(strings.TrimPrefix(facts, firstLine(facts)), "\n", " / "), ref, qcFishSrc, resultIf(ok, "the zone's or the fish's catches, or the no-data line")})
	}
	finish(t, "advice-fishing-questions", "Offline fishing: the facts for a question about a zone or a fish", out)
}

// maxCatchesShown is the catches the advisor lists for one zone.
const maxCatchesShown = 5

// TestOfflineBenchmarkFishingHomeZones: a fish's zones name a capital or starting zone of one faction as that faction's, as the advisor's
// own fishing finder leaves it out for the other side; the finder's zones for each faction are beside the facts.
func TestOfflineBenchmarkFishingHomeZones(t *testing.T) {
	ds := compStore()
	g := gather.New(nil, ds)
	var out []benchRow
	for _, faction := range []string{"Horde", "Alliance"} {
		const fish = "Raw Longjaw Mud Snapper"
		facts := ask.GatherFacts(ds, "where do I catch longjaw mud snapper?")
		res := g.Find(compChar(faction, "MAGE", 40, "Fishing", 100, 150), fish, "fish")
		other := "Alliance"
		if faction == "Alliance" {
			other = "Horde"
		}
		ok := true
		for _, r := range qcCatchesOf(t, "", num(rowByName(t, ds, "items", fish)["id"])) {
			z := text(r["zone_name"])
			side := gather.SideOfZone(z)
			inFinder := false
			for _, fz := range res.Zones {
				inFinder = inFinder || fz.Zone == z
			}
			if side != "" && strings.EqualFold(side, other) {
				ok = ok && !inFinder && strings.Contains(facts, z+" ("+other+" home zone, ")
			}
		}
		out = append(out, benchRow{faction + ": " + fish + ", the other side's home zones", firstLine(strings.TrimPrefix(facts, firstLine(facts))), "the finder leaves out the " + other + " home zones; the facts mark them", qcFishSrc, resultIf(ok, "an "+other+" home zone is unmarked in the facts")})
	}
	finish(t, "advice-fishing-home-zones", "Offline fishing: the other faction's home zones", out)
}

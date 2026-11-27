package paths_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/ask"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/gear"
	"github.com/WoW-Companion/advisor/internal/paths"
	"github.com/WoW-Companion/advisor/internal/quests"
	"github.com/WoW-Companion/advisor/internal/store"
)

// The offline class weapon trainer, class quest and class reagent benchmark: which weapon skills a class starts knowing and which a Weapon Master teaches, the words a
// player uses to ask about weapons, the Weapon Masters of every class and side, the class quests the dataset's quests_observed holds (their heading is the class, not a
// zone) as a quest reward path for the class and for another class, and the questions on a class spell's reagent, beside the dataset's weapon_skills, weapon_masters,
// quests_observed and items rows and the game's own rules, with no network. It reads testdata/usability_snapshot (weapon_skills, weapon_masters, items), testdata/places_snapshot
// (quests_observed, npcs) and testdata/tradeskills_snapshot (reagent items). The dataset holds no spell reagent table, no class quest line, giver or reward and no class
// trainer NPC, so the cases of those check that the answer is framed with the line that says so. A difference the dataset's own rows cause is marked "(data)".
// make offline-benchmark runs it; WOWC_BENCH_REPORT receives the tables.

const (
	ctWeaponSrc  = "dataset weapon_skills (learnable, default_known, min_level), weapon_masters (skills, cost_copper, min_level, city); docs/sources.md: default_known = the class starts knowing it, for at least one race"
	ctPhraseSrc  = "dataset weapon_skills, weapon_masters; the words a player uses for a weapon type (Wowhead Classic weapon guides: 1H/2H, one-hander, two-hander)"
	ctQuestSrc   = "dataset quests_observed (title, level, faction, category = the class heading players file a class quest under); Wowhead Classic class quest pages"
	ctReagentSrc = "dataset tradeskills items (class_id 15 subclass_id 1: buy_price, sell_price, bonding); game: class reagents (Warcraft Wiki, Reagent)"
	ctNoRowSrc   = "the dataset has no spell reagent table, class quest line, class quest giver or reward row"
)

// ctChar is a captured character of the faction and class, level 60, who lists no weapon skills.
func ctChar(faction, class string) character.Character {
	c := usabChar(class, 60, faction)
	c.SkillsCaptured = true
	return c
}

// ctStartNoMaster are the learnable weapon skills a class starts knowing that no Weapon Master of either side teaches: the dataset's weapon_skills and weapon_masters rows.
func ctStartNoMaster(t *testing.T, ds store.DataStore) map[string][]string {
	t.Helper()
	taught := map[int64]bool{}
	ms, err := ds.Filter("weapon_masters", store.Range{Column: "npc_id", Min: int64(0), Max: int64(1 << 40)})
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range ms {
		for _, s := range ints64(m["skills"]) {
			taught[s] = true
		}
	}
	rows, _ := ds.Filter("weapon_skills", store.Range{Column: "class_id", Min: int64(0), Max: int64(99)})
	out := map[string][]string{}
	for _, r := range rows {
		l, _ := r["learnable"].(bool)
		d, _ := r["default_known"].(bool)
		if l && d && !taught[num(r["skill_line_id"])] {
			cl := ""
			for _, c := range usabClasses {
				if c.id == num(r["class_id"]) {
					cl = c.token
				}
			}
			out[cl] = append(out[cl], text(r["skill_name"]))
		}
	}
	return out
}

// TestOfflineBenchmarkClassWeaponStartSkills: a skill the class starts knowing that no Weapon Master teaches (Wands for the mage, priest and warlock) is no skill to
// train: the weapon facts and the weapon an item needs say so, not "train it at a Weapon Master". Before the fix both sent the player to a master who does not teach it.
func TestOfflineBenchmarkClassWeaponStartSkills(t *testing.T) {
	ds := usabStore()
	var out []benchRow
	start := ctStartNoMaster(t, ds)
	for _, cl := range []string{"MAGE", "PRIEST", "WARLOCK"} {
		if !slices.Equal(start[cl], []string{"Wands"}) {
			t.Errorf("%s: the dataset's start-known skills without a master are %v, want [Wands]", cl, start[cl])
		}
		c := ctChar("Horde", cl)
		facts := ask.WeaponFacts(c, ds, "which weapon master teaches wands?")
		line := trLine(facts, "Wands")
		out = append(out, benchRow{fmt.Sprintf("horde %s: who teaches Wands", strings.ToLower(cl)), line, "Wands: learnable, default_known, in no master's skills: the class starts with it", ctWeaponSrc,
			resultIf(strings.Contains(line, "starts knowing it") && strings.Contains(line, "no Weapon Master teaches it"), "a Weapon Master was sent for a skill the class starts with")})
		_, need := gear.LoadWeaponSkills(c, ds).ItemSkill(c, 2, 19)
		nl := "no skill needed"
		if need != nil {
			nl = need.Line()
		}
		out = append(out, benchRow{fmt.Sprintf("horde %s: a wand that needs Wands", strings.ToLower(cl)), nl, "no master to train: the skill is the class's own from the start", ctWeaponSrc,
			resultIf(need != nil && strings.Contains(nl, "starts knowing it") && !strings.Contains(nl, "train it at"), "a master was named for a wand")})
	}
	rogue := ctChar("Alliance", "ROGUE")
	usable, _ := gear.LoadWeaponSkills(rogue, ds).ItemSkill(rogue, 2, 19)
	out = append(out, benchRow{"alliance rogue: a wand", fmt.Sprintf("usable %v", usable), "weapon_skills: Rogue Wands learnable false: never offered", ctWeaponSrc, resultIf(!usable, "a wand was offered to a class that cannot learn Wands")})
	// a start skill a master does teach: the capture decides, the dataset cannot say which races start with it
	hunter := ctChar("Horde", "HUNTER")
	bows := trLine(ask.WeaponFacts(hunter, ds, "where do I train bows?"), "Bows")
	out = append(out, benchRow{"horde hunter whose capture lacks Bows: who teaches it", bows, "Bows: default_known true for at least one race; the race masks are not published, so a race that starts without it cannot be told from one that starts with it", ctWeaponSrc,
		"differ: the dataset cannot say which races start with Bows, so the capture's lack of it is taken as true (data)"})
	finish(t, "advice-class-weapon-start-skills", "Offline class weapon skills: the skills a class starts with, and those no master teaches", out)
}

// TestOfflineBenchmarkClassWeaponPhrasings: a question about weapons in the words a player uses (a two-hander, 2h, a one-hand sword) is a weapon question: the Weapon Masters of the character's side are in front of the model.
func TestOfflineBenchmarkClassWeaponPhrasings(t *testing.T) {
	ds := usabStore()
	var out []benchRow
	for _, tc := range []struct{ faction, class, q, skill string }{
		{"Horde", "WARRIOR", "where do I learn to use a two-hander?", "Two-Handed Swords"}, {"Alliance", "PALADIN", "where can I train 2h maces?", "Two-Handed Maces"},
		{"Horde", "SHAMAN", "where do I get one-hander skills?", "One-Handed Maces"}, {"Alliance", "ROGUE", "which master teaches 1h swords?", "One-Handed Swords"},
		{"Horde", "HUNTER", "where do I learn to use a two hander?", "Two-Handed Axes"},
	} {
		c := ctChar(tc.faction, tc.class)
		facts := ask.WeaponFacts(c, ds, tc.q)
		want := trExpected(t, ds, tc.faction, tc.class, trSkillID(t, ds, tc.skill))
		line := trLine(facts, tc.skill)
		if line == "" {
			// a skill the class does not lack is not listed: take the first lacking skill the dataset gives a master for
			line = "(not listed)"
		}
		ok := ask.WeaponQuestion(tc.q) && (len(want) == 0 || slices.Equal(trShown(line), want))
		out = append(out, benchRow{fmt.Sprintf("\"%s\" (%s %s)", tc.q, strings.ToLower(tc.faction), strings.ToLower(tc.class)), line, strings.Join(want, "; "), ctPhraseSrc, resultIf(ok, "no weapon facts, or other masters, for the question")})
	}
	for _, q := range []string{"how long is the two hour wait for the boat?", "which quest do I do at level 12?"} {
		out = append(out, benchRow{"\"" + q + "\"", fmt.Sprintf("weapon question %v", ask.WeaponQuestion(q)), "not a weapon question: no weapon facts", ctPhraseSrc, resultIf(!ask.WeaponQuestion(q), "weapon facts for a question that is none")})
	}
	finish(t, "advice-class-weapon-phrasings", "Offline class weapon skills: the words a player uses to ask", out)
}

// TestOfflineBenchmarkClassWeaponMasterSweep: for every class and both sides, each weapon skill the class lacks names the masters of weapon_masters (master, city, price, level), or says no master teaches it.
func TestOfflineBenchmarkClassWeaponMasterSweep(t *testing.T) {
	ds := usabStore()
	var out []benchRow
	for _, faction := range []string{"Horde", "Alliance"} {
		for _, cl := range usabClasses {
			c := ctChar(faction, cl.token)
			facts := ask.WeaponFacts(c, ds, "where do I train my weapon skills?")
			f := gear.LoadWeaponSkills(c, ds)
			var bad []string
			n := 0
			for id := range f.Learnable {
				want := trExpected(t, ds, faction, cl.token, id)
				line := trLine(facts, f.Names[id])
				n++
				switch {
				case len(want) == 0 && (strings.HasPrefix(line, "no master in the data") || strings.Contains(line, "no Weapon Master teaches it")):
				case len(want) > 0 && slices.Equal(trShown(line), want):
				default:
					bad = append(bad, f.Names[id])
				}
			}
			slices.Sort(bad)
			out = append(out, benchRow{fmt.Sprintf("%s %s: the masters of its %d learnable skills", strings.ToLower(faction), strings.ToLower(cl.token), n), fmt.Sprintf("%d lines differ %v", len(bad), bad),
				"every skill: the weapon_masters rows of the side, with price and level", ctWeaponSrc, resultIf(len(bad) == 0 && n > 0, "a master, city, price or level: "+strings.Join(bad, ", "))})
		}
	}
	finish(t, "advice-class-weapon-master-sweep", "Offline class weapon skills: the masters of every class and side", out)
}

// ctClassQuests are class quests the dataset holds (quests_observed, category = the class): the class, the id and the quest the character of that class meets.
var ctClassQuests = []struct {
	class string
	id    int64
}{{"WARLOCK", 1507}, {"WARLOCK", 1508}, {"WARLOCK", 1740}, {"WARLOCK", 1801}, {"PALADIN", 98601}, {"PALADIN", 90902}}

// TestOfflineBenchmarkClassQuestRewardPath: a quest reward path through a class quest names the quest and says it is the class's own, never "in Warlock" (the heading is the class, not a
// place), and a character of another class is not sent to it. Before the fix every class was told to "Finish Devourer of Souls (in Warlock)".
func TestOfflineBenchmarkClassQuestRewardPath(t *testing.T) {
	var out []benchRow
	for _, tc := range ctClassQuests {
		q := qbQuest(t, tc.id)
		title, class := text(q["title"]), text(q["category"])
		ds := qbRewards{qbStore(), []store.Row{qbReward(iBelt, tc.id)}}
		lv := 30
		own := character.Character{Name: "Benchtest", Faction: "Horde", Class: tc.class, Level: &lv}
		got := questLine(paths.For(own, ds, iBelt, paths.Options{}))
		ref := fmt.Sprintf("level %d %s quest of the %s class (heading %q): Finish %s, no zone", num(q["level"]), text(q["faction"]), class, class, title)
		out = append(out, benchRow{fmt.Sprintf("quest %d %s: the path for a level 30 %s", tc.id, title, strings.ToLower(class)), got, ref, ctQuestSrc,
			resultIf(strings.HasPrefix(got, "Finish "+title+" for it") && !strings.Contains(got, "in "+class) && strings.Contains(got, strings.ToLower(class)+" class quest"), "the heading was told as a zone, or the class not named")})
		other := character.Character{Name: "Benchtest", Faction: "Horde", Class: "WARRIOR", Level: &lv}
		got = questLine(paths.For(other, ds, iBelt, paths.Options{}))
		out = append(out, benchRow{fmt.Sprintf("quest %d %s: the path for a level 30 warrior", tc.id, title), got + "(no path)", "a " + strings.ToLower(class) + " class quest is not the warrior's: no path", ctQuestSrc,
			resultIf(got == "", "a class quest of another class was offered")})
	}
	finish(t, "advice-class-quest-reward-path", "Offline class quests: the reward path of a class quest, for its class and another", out)
}

// TestOfflineBenchmarkClassQuestHeading: the dataset's class quest headings are the nine class names or zone names, and the advisor reads a class heading as a class.
func TestOfflineBenchmarkClassQuestHeading(t *testing.T) {
	var out []benchRow
	for _, tc := range ctClassQuests {
		f := quests.FactsOf(qbStore(), nil, tc.id)
		out = append(out, benchRow{fmt.Sprintf("quest %d: the heading %q", tc.id, text(qbQuest(t, tc.id)["category"])), strings.ToUpper(f.ClassHeading()), strings.ToUpper(text(qbQuest(t, tc.id)["category"])) + " (a class, not a zone)", ctQuestSrc,
			resultIf(strings.ToUpper(f.ClassHeading()) == tc.class, "the class heading read as a place")})
	}
	for _, id := range []int64{2, 5, 8, 9} {
		f := quests.FactsOf(qbStore(), nil, id)
		out = append(out, benchRow{fmt.Sprintf("quest %d %s: the heading %q", id, f.Name, f.Category), "class \"" + f.ClassHeading() + "\"", "a zone heading: no class", ctQuestSrc, resultIf(f.ClassHeading() == "" && f.Category != "", "a zone read as a class")})
	}
	// the giver of a class quest the dataset names by an NPC id its npcs table does not hold
	f := quests.FactsOf(qbStore(), nil, 1508)
	out = append(out, benchRow{"Blind Cazul (quest 1508): who gives it", fmt.Sprintf("giver %q", f.Giver), "quests_observed giver_npc_id 5909; the snapshot's npcs has no row for it", ctQuestSrc,
		"differ: the giver NPC has no npcs row, so the advisor cannot name or place the giver (data)"})
	finish(t, "advice-class-quest-heading", "Offline class quests: the class heading, and a giver the data cannot place", out)
}

// ctSays is whether the limit facts for a question carry the topic and the instruction to say so, with no figure in them.
func ctSays(q, topic string) (string, bool) {
	f := ask.LimitFacts(q)
	return firstLine(f), strings.Contains(f, topic) && strings.Contains(f, "Say so") && !trDigit.MatchString(f)
}

// TestOfflineBenchmarkClassReagentPhrasings: a question about what a class spell needs, in the words a player uses, is framed with the line that the dataset holds no spell reagent table. Before the
// fix "what do I need to cast Vanish?" and "do I need reagents for Rebirth?" got no line, so the answer came from memory.
func TestOfflineBenchmarkClassReagentPhrasings(t *testing.T) {
	var out []benchRow
	for _, q := range []string{
		"do I need reagents for Rebirth?", "what do I need to cast Vanish?", "what does a paladin need for Divine Intervention?", "what do shamans need for Reincarnation?",
		"do I need an ankh to resurrect myself?", "what do I need for Gift of the Wild?", "do mages need reagents for Arcane Brilliance?", "what do I need to summon a felhunter?",
		"what do I need to cast Teleport: Ironforge?", "how many Wild Thornroot does Gift of the Wild use?",
	} {
		line, ok := ctSays(q, "Class reagents, soul shards, totems and rituals:")
		out = append(out, benchRow{"\"" + q + "\"", line, "no spell reagent table: say so, name no reagent or count", ctNoRowSrc, resultIf(ok, "no line saying the data holds no spell reagent row, or a figure in it")})
	}
	for _, q := range []string{"where do I buy flash powder?", "what do I need to level Alchemy?", "what do I need to craft Elixir of Fortitude?"} {
		got := ask.LimitFacts(q)
		out = append(out, benchRow{"\"" + q + "\"", firstLine(got) + "(none)", "no spell is named: no class reagent line", ctNoRowSrc, resultIf(!strings.Contains(got, "Class reagents,"), "the class reagent line on a question about a purchase or a recipe")})
	}
	finish(t, "advice-class-reagent-phrasings", "Offline class reagents: the words a player uses to ask what a spell needs", out)
}

// TestOfflineBenchmarkClassReagentItems: what the dataset holds of a class reagent item (its vendor price and binding) is told from its row, and the line for a spell's reagent stays the no-answer one.
func TestOfflineBenchmarkClassReagentItems(t *testing.T) {
	ds := tsStore()
	var out []benchRow
	for _, it := range []string{"Light Feather", "Demonic Figurine", "Maple Seed", "Stranglethorn Seed"} {
		row := rowByName(t, ds, "items", it)
		f := ask.ItemTooltipFacts(ds, "what does ["+it+"] cost and can I trade it?")
		line := strings.ReplaceAll(strings.TrimPrefix(f, "Item tooltips (from the item rows; answer from these, not from memory):\n"), "\n", " ")
		sell := fmt.Sprintf("%d copper", num(row["sell_price"]))
		out = append(out, benchRow{it + ": sale price and binding", line, fmt.Sprintf("sell_price %s, buy_price %d copper, bonding %d: can be traded", sell, num(row["buy_price"]), num(row["bonding"])), ctReagentSrc,
			resultIf(strings.Contains(line, "can be traded") && strings.Contains(line, sell), "the price or binding of a reagent")})
	}
	for _, it := range []string{"Ankh", "Holy Candle"} {
		f := ask.ItemTooltipFacts(ds, "can I trade ["+it+"]?")
		out = append(out, benchRow{it + ": the row", strings.ReplaceAll(firstLine(strings.TrimPrefix(f, "Item tooltips (from the item rows; answer from these, not from memory):\n")), "\n", " ") + "(none)", "no row in the snapshot: no binding or price is stated", ctReagentSrc,
			resultIf(!strings.Contains(f, "soulbound") && !strings.Contains(f, "vendor for"), "a price or binding was stated for an item with no row")})
	}
	finish(t, "advice-class-reagent-items", "Offline class reagents: the price and binding of a reagent item", out)
}

// TestOfflineBenchmarkClassNoAnswer: what the dataset cannot answer about a class quest, a class's weapon price or a spell's reagent is said to be missing, and no figure or name is given.
func TestOfflineBenchmarkClassNoAnswer(t *testing.T) {
	var out []benchRow
	for _, tc := range []struct{ q, topic, ref string }{
		{"what reward does the paladin level 12 quest give?", "Class quest chains:", "no class quest reward row"},
		{"who gives the shaman totem quests?", "Class quest chains:", "no class quest giver row for the shaman"},
		{"where do I start the druid quest for Aquatic Form?", "Class quest chains:", "no class quest line or giver row"},
		{"how many Symbols of Kings does Greater Blessing of Kings use?", "Class reagents, soul shards, totems and rituals:", "no spell reagent count"},
		{"what does a hunter need to start the pet quest?", "Class quest chains:", "no class quest line or prerequisite row"},
	} {
		line, ok := ctSays(tc.q, tc.topic)
		out = append(out, benchRow{"\"" + tc.q + "\"", line, tc.ref + ": say so, name none", ctNoRowSrc, resultIf(ok, "no statement that the data holds none, or a figure in it")})
	}
	rogue := ctChar("Alliance", "ROGUE")
	dw := ask.WeaponFacts(rogue, usabStore(), "how much does dual wield cost?")
	out = append(out, benchRow{"alliance rogue: what does Dual Wield cost", firstLine(strings.TrimPrefix(dw[strings.Index(dw, "- Dual Wield"):], "- ")), "the dataset holds no price for a class spell: name none", ctNoRowSrc,
		resultIf(strings.Contains(dw, "holds no price for it, so name none"), "a price for Dual Wield")})
	finish(t, "advice-class-no-answer", "Offline class trainer, quest and reagent advice: no answer in the dataset", out)
}

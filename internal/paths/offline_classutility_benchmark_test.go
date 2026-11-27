package paths_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/levelup"
	"github.com/WoW-Companion/advisor/internal/professions"
	"github.com/WoW-Companion/advisor/internal/store"
)

// The offline class-utility benchmark: the hunter's pet spells, the warlock's minion spells, the shaman's totems, the
// rogue's poisons and lockpicking and the mage's teleports and portals, beside the dataset's class_spells,
// trainer_spells, trainers and quests_observed rows, with no network. It reads testdata/usability_snapshot (class
// spells), testdata/tradeskills_snapshot (the Poisons recipes and trainers) and testdata/places_snapshot (observed
// quests). The dataset has no tameable-pet flag, no beast-training trainer, no totem or minion quest rewards, no
// spell reagent table and no Pick Lock row, so those cases check that the advisor says nothing it cannot back, and a
// game fact the dataset lacks is marked "(data)". make offline-benchmark runs it; WOWC_BENCH_REPORT receives the tables.

const (
	utilSpellSrc = "dataset class_spells (name, rank, level); Warcraft Wiki class spell pages"
	utilQuestSrc = "dataset quests_observed (title, level, faction, category); Warcraft Wiki spell pages"
	utilNoneSrc  = "the dataset has no such row or table"
)

// utilRow is the dataset's lowest-level class_spells row of a spell; rank 0 is the unranked spell.
func utilRow(t *testing.T, class, name string, rank int64) store.Row {
	t.Helper()
	var best store.Row
	for _, r := range classRows(t, classID(class), 1, 60) {
		if text(r["name"]) == name && num(r["rank"]) == rank && (best == nil || num(r["level"]) < num(best["level"])) {
			best = r
		}
	}
	if best == nil {
		t.Fatalf("%s %s rank %d: not in the snapshot", class, name, rank)
	}
	return best
}

// utilSpells lists what the advisor says a class trains over a span of levels, as "name rank@level".
func utilSpells(class, side string, from, to int) []string {
	var out []string
	for _, s := range levelup.Spells(usabChar(class, to, side), usabStore(), from, to) {
		out = append(out, fmt.Sprintf("%s %d@%d", s.Name, s.Rank, s.Level))
	}
	return out
}

func utilHas(list []string, name string, rank int) bool {
	for _, s := range list {
		if strings.HasPrefix(s, fmt.Sprintf("%s %d@", name, rank)) {
			return true
		}
	}
	return false
}

// utilSpellsTo is what a character of the level is told to train over levels 1 to that level, read once for each class,
// side and level (the case tables repeat them).
var utilSpellsTo = map[string][]levelup.Spell{}

func utilTrainList(class, side string, lv int) []levelup.Spell {
	k := fmt.Sprintf("%s|%s|%d", class, side, lv)
	if _, ok := utilSpellsTo[k]; !ok {
		utilSpellsTo[k] = levelup.Spells(usabChar(class, lv, side), usabStore(), 1, lv)
	}
	return utilSpellsTo[k]
}

// utilTrained: over the levels up to the dataset's level the advisor lists the spell, and at that level.
func utilTrained(t *testing.T, class, side, name string, rank int64) benchRow {
	t.Helper()
	r := utilRow(t, class, name, rank)
	lv := int(num(r["level"]))
	label := name
	if rank > 0 {
		label = fmt.Sprintf("%s (Rank %d)", name, rank)
	}
	got, at := "not listed", 0
	for _, s := range utilTrainList(class, side, lv) {
		if s.Name == name && int64(s.Rank) == rank {
			at = s.Level
			got = fmt.Sprintf("trained at level %d", at)
		}
	}
	return benchRow{fmt.Sprintf("%s %s: level to train %s", strings.ToLower(side), strings.ToLower(class), label), got,
		fmt.Sprintf("class_spells: %s at level %d (spell %d)", label, lv, num(r["spell_id"])), utilSpellSrc, resultIf(at == lv, "another level, or not listed")}
}

// TestOfflineBenchmarkUtilityHunterPet: the pet spells a hunter trains and the level each comes at.
func TestOfflineBenchmarkUtilityHunterPet(t *testing.T) {
	var out []benchRow
	for _, tc := range []struct {
		name string
		rank int64
	}{{"Tame Beast", 0}, {"Call Pet", 0}, {"Dismiss Pet", 0}, {"Feed Pet", 0}, {"Revive Pet", 0}, {"Mend Pet", 1}, {"Eyes of the Beast", 0}, {"Scare Beast", 1}, {"Beast Lore", 0}, {"Mend Pet", 3}} {
		out = append(out, utilTrained(t, "HUNTER", "Alliance", tc.name, tc.rank))
	}
	// a hunter below level 10 has no pet spell to train
	got := utilSpells("HUNTER", "Horde", 1, 9)
	pets := 0
	for _, s := range got {
		if strings.Contains(s, "Pet") || strings.Contains(s, "Tame") {
			pets++
		}
	}
	out = append(out, benchRow{"horde hunter, levels 1 to 9: pet spells", fmt.Sprintf("%d pet spells of %d listed", pets, len(got)), "class_spells: the first pet spell is at level 10", utilSpellSrc, resultIf(pets == 0 && len(got) > 0, "a pet spell before level 10")})
	finish(t, "advice-utility-hunter-pet", "Offline class utility: hunter pet spells", out)
}

// utilQuest is the dataset's observed quest row of a title in a class heading.
func utilQuest(t *testing.T, title, category string) store.Row {
	t.Helper()
	rows, err := store.NewDir("../../testdata/places_snapshot").Filter("quests_observed", store.Range{Column: "title", Min: title, Max: title})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if text(r["category"]) == category {
			return r
		}
	}
	t.Fatalf("quest %q under %s: not in the snapshot", title, category)
	return nil
}

// TestOfflineBenchmarkUtilityWarlockMinion: the minion and summoning spells a warlock trains, and the two minions the
// game teaches by class quest, which the dataset's rows cannot tie to their spell.
func TestOfflineBenchmarkUtilityWarlockMinion(t *testing.T) {
	var out []benchRow
	for _, name := range []string{"Summon Imp", "Summon Voidwalker", "Summon Felhunter", "Ritual of Summoning", "Sense Demons"} {
		out = append(out, utilTrained(t, "WARLOCK", "Horde", name, 0))
	}
	// the Succubus and the Felhunter come from a class quest in the game; the dataset holds the quest rows but no
	// quest-reward table, so the advisor lists the spell as a trainer spell it hedges ("usually trains")
	for _, tc := range []struct{ spell, title string }{{"Summon Succubus", "The Binding"}, {"Summon Felhunter", "Tome of the Cabal"}} {
		q := utilQuest(t, tc.title, "Warlock")
		sp := utilRow(t, "WARLOCK", tc.spell, 0)
		facts, _, _ := levelup.Facts(usabChar("WARLOCK", int(num(sp["level"])), "Horde"), usabStore(), nil, int(num(sp["level"])), int(num(sp["level"])))
		hedged := strings.Contains(facts, "usually trains") && strings.Contains(facts, tc.spell)
		out = append(out, benchRow{fmt.Sprintf("horde warlock: %s comes from the class quest %q", tc.spell, tc.title),
			"listed as a trainer spell, hedged: the player has not opened the trainer", fmt.Sprintf("quests_observed %d: %s, level %d, %s, %s; no quest-to-spell row", num(q["quest_id"]), tc.title, num(q["level"]), text(q["faction"]), text(q["category"])),
			utilQuestSrc, map[bool]string{true: "differ (data): no quest_rewards table links the quest to the spell, so the advisor cannot say the trainer does not teach it", false: "differ: listed as definitely available"}[hedged]})
	}
	finish(t, "advice-utility-warlock-minion", "Offline class utility: warlock minion spells", out)
}

// TestOfflineBenchmarkUtilityShamanTotem: the totem spells a shaman trains and the level of each.
func TestOfflineBenchmarkUtilityShamanTotem(t *testing.T) {
	var out []benchRow
	for _, tc := range []struct {
		name string
		rank int64
	}{{"Stoneskin Totem", 1}, {"Earthbind Totem", 0}, {"Searing Totem", 1}, {"Healing Stream Totem", 1}, {"Grounding Totem", 0}, {"Windfury Totem", 1}} {
		out = append(out, utilTrained(t, "SHAMAN", "Horde", tc.name, tc.rank))
	}
	finish(t, "advice-utility-shaman-totem", "Offline class utility: shaman totem spells", out)
}

// TestOfflineBenchmarkUtilityShamanTalentTotem: Mana Tide Totem, a Restoration talent, is not sold at level 1.
func TestOfflineBenchmarkUtilityShamanTalentTotem(t *testing.T) {
	var out []benchRow
	early := utilSpells("SHAMAN", "Horde", 1, 37)
	r2 := utilRow(t, "SHAMAN", "Mana Tide Totem", 2)
	at := utilSpells("SHAMAN", "Horde", int(num(r2["level"])), int(num(r2["level"])))
	out = append(out, benchRow{"horde shaman, levels 1 to 37: Mana Tide Totem", fmt.Sprintf("listed: %v; rank 2 at level %d: %v", utilHas(early, "Mana Tide Totem", 1), num(r2["level"]), utilHas(at, "Mana Tide Totem", 2)),
		"class_spells lists rank 1 at level 1 under a talent name (talent_nodes: Mana Tide Totem); rank 2 at level 38", utilSpellSrc,
		resultIf(!utilHas(early, "Mana Tide Totem", 1) && utilHas(at, "Mana Tide Totem", 2), "the talent sold as a level 1 spell")})
	finish(t, "advice-utility-shaman-talent-totem", "Offline class utility: a talent totem is not a level 1 spell", out)
}

// TestOfflineBenchmarkUtilityRogue: the rogue's utility spells and poisons.
func TestOfflineBenchmarkUtilityRogue(t *testing.T) {
	var out []benchRow
	for _, tc := range []struct {
		name string
		rank int64
	}{{"Pick Pocket", 0}, {"Distract", 0}, {"Detect Traps", 0}, {"Sprint", 1}} {
		out = append(out, utilTrained(t, "ROGUE", "Alliance", tc.name, tc.rank))
	}
	// Lockpicking: the game teaches Pick Lock at level 16; the dataset has no row, so the advisor lists none
	l16 := strings.Join(utilSpells("ROGUE", "Horde", 1, 20), "|")
	out = append(out, benchRow{"rogue, levels 1 to 20: Pick Lock (level 16 in the game)", map[bool]string{true: "listed", false: "not listed"}[strings.Contains(l16, "Pick Lock")], "no class_spells row for Pick Lock; no Lockpicking recipe or trainer row",
		"dataset class_spells; Warcraft Wiki Pick Lock page", "differ (data): the dataset lacks the Pick Lock row, so the advisor cannot say when a rogue learns it"})
	finish(t, "advice-utility-rogue", "Offline class utility: rogue utility spells", out)
}

// TestOfflineBenchmarkUtilityRoguePoison: what the Poisons plan says about the trainer and its price.
func TestOfflineBenchmarkUtilityRoguePoison(t *testing.T) {
	var out []benchRow
	// Crippling Poison: the cost the plan gives to learn it is a trainer_spells price
	ds := tsStore()
	lv, rk, top := 30, 1, 300
	c := character.Character{Faction: "Alliance", Class: "ROGUE", Level: &lv, Professions: []character.Skill{{Name: "Poisons", Rank: &rk, Max: &top}}}
	p := professions.ComputePath(c, ds, "Poisons", 75, 0)
	var price int64 = -1
	rows, _ := ds.Filter("trainer_spells", store.Range{Column: "recipe_name", Min: "Crippling Poison", Max: "Crippling Poison"})
	for _, r := range rows {
		if cp := num(r["cost_copper"]); cp > 0 && (price < 0 || cp < price) {
			price = cp
		}
	}
	got := int64(0)
	if len(p.Bands) > 0 {
		got = p.Bands[0].Best.LearnInfo.Copper
	}
	out = append(out, benchRow{"rogue, Poisons skill 1: what the trainer asks for Crippling Poison", vendorsMoney(got), fmt.Sprintf("trainer_spells: Crippling Poison, required skill 1, %s", vendorsMoney(price)), "dataset trainer_spells (cost_copper, required_skill), recipes", resultIf(price > 0 && got == price, "another price")})
	// the Poisons trainers carry no name or zone in the dataset: the plan says so, not a name
	trainers, _ := ds.Filter("trainers", store.Range{Column: "profession", Min: "Poisons", Max: "Poisons"})
	named := 0
	for _, r := range trainers {
		if text(r["name"]) != "" || text(r["zone_name"]) != "" {
			named++
		}
	}
	where := strings.Join(p.Where, " ")
	out = append(out, benchRow{"rogue, Poisons: where the trainer is", firstSentence(where, "Trainer to visit"), fmt.Sprintf("%d Poisons trainers, %d with a name or zone", len(trainers), named), "dataset trainers (name, zone_name)",
		resultIf(named == 0 && strings.Contains(where, "name not in the dataset"), "a trainer name or place the dataset lacks")})
	finish(t, "advice-utility-rogue-poison", "Offline class utility: rogue poisons", out)
}

func vendorsMoney(c int64) string { return fmt.Sprintf("%dc", c) }

func firstSentence(s, prefix string) string {
	if i := strings.Index(s, prefix); i >= 0 {
		return strings.SplitN(s[i:], "\n", 2)[0]
	}
	return "no trainer line"
}

// TestOfflineBenchmarkUtilityMageTeleport: the teleport and portal spells a mage trains are those of their own side's
// cities, at the level the dataset gives.
func TestOfflineBenchmarkUtilityMageTeleport(t *testing.T) {
	var out []benchRow
	out = append(out,
		utilTrained(t, "MAGE", "Alliance", "Teleport: Stormwind", 0),
		utilTrained(t, "MAGE", "Horde", "Teleport: Orgrimmar", 0),
		utilTrained(t, "MAGE", "Alliance", "Portal: Ironforge", 0),
		utilTrained(t, "MAGE", "Horde", "Portal: Thunder Bluff", 0),
		utilTrained(t, "MAGE", "Alliance", "Teleport: Darnassus", 0))
	for _, tc := range []struct{ side, other string }{{"Alliance", "Orgrimmar|Undercity|Thunder Bluff"}, {"Horde", "Stormwind|Ironforge|Darnassus"}} {
		var wrong []string
		for _, s := range utilSpells("MAGE", tc.side, 1, 60) {
			for _, city := range strings.Split(tc.other, "|") {
				if strings.HasPrefix(s, "Teleport: "+city) || strings.HasPrefix(s, "Portal: "+city) {
					wrong = append(wrong, s)
				}
			}
		}
		for _, s := range levelup.Learned(usabChar("MAGE", 60, tc.side), usabStore(), 60) {
			for _, city := range strings.Split(tc.other, "|") {
				if s.Name == "Teleport: "+city || s.Name == "Portal: "+city {
					wrong = append(wrong, "learned "+s.Name)
				}
			}
		}
		out = append(out, benchRow{strings.ToLower(tc.side) + " mage, levels 1 to 60: teleports and portals to the other side's cities", fmt.Sprintf("%d listed: %s", len(wrong), strings.Join(wrong, ", ")),
			"none: the other side's cities are not taught (class_spells has no faction column and lists all six)", "dataset class_spells; Warcraft Wiki Teleport and Portal pages", resultIf(len(wrong) == 0, strings.Join(wrong, ", "))})
	}
	finish(t, "advice-utility-mage-teleport", "Offline class utility: mage teleports and portals", out)
}

// TestOfflineBenchmarkUtilityNoAnswer: what the dataset cannot answer: which pets are tameable where, a beast-training
// trainer, the warlock's mount quest, a shaman totem quest and a teleport's reagent. The advisor names none.
func TestOfflineBenchmarkUtilityNoAnswer(t *testing.T) {
	var out []benchRow
	ds := usabStore()
	hunter := usabChar("HUNTER", 20, "Horde")
	facts, _, _ := levelup.Facts(hunter, ds, nil, 10, 20)
	lower := strings.ToLower(facts)
	beasts := store.Row(nil)
	rows, _ := ds.Filter("npcs", store.Range{Column: "creature_type", Min: int64(1), Max: int64(1)})
	if len(rows) > 0 {
		beasts = rows[0]
	}
	out = append(out, benchRow{"hunter level 20: which beasts can I tame, and where", firstLine(facts), fmt.Sprintf("npcs: %d beast rows in the snapshot, and no tameable flag in any case", len(rows)), utilNoneSrc,
		resultIf(beasts == nil && !strings.Contains(lower, "tameable") && !strings.Contains(lower, "tame a "), "a beast or place named")})
	cov := professions.CoverageOf(hunter, ds)
	line := ""
	trainers := -1
	for _, cv := range cov {
		if cv.Profession == "Beast Training" {
			line, trainers = cv.Line(), cv.Trainers
		}
	}
	out = append(out, benchRow{"hunter: who trains my pet's skills (Beast Training)", line, "trainers: no Beast Training row", utilNoneSrc, resultIf(trainers == 0 && strings.Contains(line, "GAP"), "a pet trainer named")})
	lock := usabChar("WARLOCK", 40, "Alliance")
	facts, _, _ = levelup.Facts(lock, ds, nil, 40, 41)
	out = append(out, benchRow{"warlock level 40: the mount quest", firstLine(facts), "class_spells: no Summon Felsteed row; quests_observed: no warlock mount quest", utilNoneSrc, resultIf(!strings.Contains(strings.ToLower(facts), "steed") && !strings.Contains(strings.ToLower(facts), "mount"), "a mount or quest named")})
	sham := usabChar("SHAMAN", 10, "Horde")
	facts, _, _ = levelup.Facts(sham, ds, nil, 4, 10)
	out = append(out, benchRow{"shaman level 10: the totem quests, level and zone", firstLine(facts), "quests_observed: no Shaman heading and no totem quest", utilNoneSrc, resultIf(!strings.Contains(facts, "Class quests coming up") && !strings.Contains(strings.ToLower(facts), "call of"), "a totem quest named")})
	mage := usabChar("MAGE", 20, "Horde")
	facts, _, _ = levelup.Facts(mage, ds, nil, 20, 20)
	reag, _ := ds.Filter("recipe_reagents", store.Range{Column: "spell_id", Min: int64(3567), Max: int64(3567)})
	out = append(out, benchRow{"mage level 20: the reagent for Teleport: Orgrimmar", firstLine(facts), fmt.Sprintf("recipe_reagents: %d rows for spell 3567; no spell reagent table", len(reag)), utilNoneSrc,
		resultIf(len(reag) == 0 && !strings.Contains(strings.ToLower(facts), "rune"), "a reagent named")})
	finish(t, "advice-utility-no-answer", "Offline class utility: no answer in the dataset", out)
}

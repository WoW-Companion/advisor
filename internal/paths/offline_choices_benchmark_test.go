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

// The offline spell-choice, gear-upgrade and threshold benchmark: which abilities the advisor says a class has by a level (the list a
// rotation or "how do I play my class" answer is framed with), which bag item it calls an upgrade over what is worn, and the level a
// choice opens at, beside the dataset's rows, with no network. It reads testdata/usability_snapshot (class_spells, talent_nodes,
// talent_trees, items, weapon_skills). The dataset holds no spell priority, rotation, cooldown, mana cost, damage figure,
// best-in-slot list or talent tier, so the cases it cannot answer check that the advisor says so and states none.
// make offline-benchmark runs it; WOWC_BENCH_REPORT receives the tables.

const (
	choiceSpellSrc = "dataset class_spells (name, rank, level, superseded_spell_id), talent_nodes; Wowhead Classic class trainer lists"
	choiceGearSrc  = "dataset items (item_level, required_level, class_id, subclass_id, slot, allowable_class_mask), weapon_skills; game armor proficiency"
	choiceNoneSrc  = "the dataset has no such row, column or table"
)

// ---- the abilities a class has by a level ----

// firstRankLevel is the level the dataset's class_spells row of the lowest rank of an ability gives; 0 when there is none.
func firstRankLevel(t *testing.T, classID int64, name string) int64 {
	t.Helper()
	best := int64(0)
	for _, r := range classRows(t, classID, 1, 60) {
		if text(r["name"]) != name || num(r["superseded_spell_id"]) != 0 {
			continue
		}
		if best == 0 || num(r["level"]) < best {
			best = num(r["level"])
		}
	}
	return best
}

func learnedHas(sp []levelup.Spell, name string) (levelup.Spell, bool) {
	for _, s := range sp {
		if s.Name == name {
			return s, true
		}
	}
	return levelup.Spell{}, false
}

var choiceAbilities = []struct {
	class, name string
}{
	{"WARRIOR", "Charge"}, {"WARRIOR", "Thunder Clap"}, {"MAGE", "Frostbolt"}, {"MAGE", "Polymorph"}, {"ROGUE", "Kick"}, {"ROGUE", "Sprint"},
	{"PRIEST", "Psychic Scream"}, {"WARLOCK", "Corruption"}, {"HUNTER", "Multi-Shot"}, {"DRUID", "Moonfire"}, {"PALADIN", "Hammer of Justice"},
}

// TestOfflineBenchmarkLearnedAbilityLevel: the list of abilities a class has by a level (the facts a rotation or class question is
// framed with) names an ability from the level of its first rank in class_spells, and not from the level before.
func TestOfflineBenchmarkLearnedAbilityLevel(t *testing.T) {
	ds := usabStore()
	var out []benchRow
	for _, tc := range choiceAbilities {
		lv := firstRankLevel(t, classID(tc.class), tc.name)
		if lv < 2 {
			t.Fatalf("%s %s: no first rank row above level 1 in the snapshot", tc.class, tc.name)
		}
		at, has := learnedHas(levelup.Learned(usabChar(tc.class, int(lv), "Horde"), ds, int(lv)), tc.name)
		_, early := learnedHas(levelup.Learned(usabChar(tc.class, int(lv)-1, "Horde"), ds, int(lv)-1), tc.name)
		out = append(out, benchRow{fmt.Sprintf("%s %s: the level it is first listed", strings.ToLower(tc.class), tc.name),
			fmt.Sprintf("listed from level %d: %v (rank %d); at level %d: %v", lv, has, at.Rank, lv-1, early), fmt.Sprintf("class_spells first rank at level %d", lv), choiceSpellSrc,
			resultIf(has && at.Level == int(lv) && !early, "another level than the first rank, or listed a level early")})
	}
	finish(t, "advice-learned-ability-level", "Offline spell choice: the level an ability is first listed", out)
}

// choiceNotLearned are the rows the dataset's class_spells lists that no level and no trainer gives a player as an ability: talents
// and their effects, the passive half of a stance, and the damage half of Execute listed at level 1 (its rank 1 is level 24).
var choiceNotLearned = []struct{ class, name string }{
	{"WARRIOR", "Improved Pummel"}, {"WARRIOR", "Battle Stance Passive"}, {"WARRIOR", "Charge Rage Bonus Effect"}, {"WARRIOR", "Flurry"},
	{"MAGE", "Improved Frostbolt"}, {"MAGE", "Presence of Mind"}, {"ROGUE", "Fist Weapon Specialization"}, {"WARLOCK", "Hellfire Effect"},
	{"PALADIN", "Shield Specialization"}, {"SHAMAN", "Weapon Mastery"}, {"DRUID", "Cat Form (Passive)"},
}

// TestOfflineBenchmarkLearnedNotTalents: the list of abilities a level 60 class has holds no talent, no talent effect and no passive
// half of a stance, though class_spells carries those rows at level 1; Execute is listed once, at level 24.
func TestOfflineBenchmarkLearnedNotTalents(t *testing.T) {
	ds := usabStore()
	var out []benchRow
	for _, tc := range choiceNotLearned {
		var row store.Row
		for _, r := range classRows(t, classID(tc.class), 1, 60) {
			if text(r["name"]) == tc.name {
				row = r
				break
			}
		}
		if row == nil {
			t.Fatalf("%s %s: not in the snapshot", tc.class, tc.name)
		}
		_, listed := learnedHas(levelup.Learned(usabChar(tc.class, 60, "Horde"), ds, 60), tc.name)
		out = append(out, benchRow{fmt.Sprintf("%s %s: listed as an ability", strings.ToLower(tc.class), tc.name), fmt.Sprintf("listed %v", listed),
			fmt.Sprintf("class_spells row at level %d, rank %d: a talent, an effect or a passive, no trainer teaches it", num(row["level"]), num(row["rank"])), choiceSpellSrc,
			resultIf(!listed, "a talent, effect or passive listed as an ability the class has learned")})
	}
	// rank-less level 1 rows that are talent effects in the game but that the dataset flags nowhere: no rank gap, no talent_nodes row
	for _, tc := range []struct{ class, name string }{{"MAGE", "Arcane Focus"}, {"MAGE", "Piercing Ice"}, {"MAGE", "Chilled"}, {"WARRIOR", "Toughness"}} {
		_, listed := learnedHas(levelup.Learned(usabChar(tc.class, 60, "Horde"), ds, 60), tc.name)
		res := "match"
		if talentSet(t, classID(tc.class))[strings.ToLower(tc.name)] {
			res = resultIf(!listed, "a talent named in talent_nodes listed as an ability")
		} else if listed {
			res = "differ (data): class_spells lists the row at level 1 with no rank and talent_nodes holds no such talent, so the row looks like an ability"
		}
		out = append(out, benchRow{fmt.Sprintf("%s %s: listed as an ability", strings.ToLower(tc.class), tc.name), fmt.Sprintf("listed %v", listed), "Classic: a talent's passive or effect, not a trainer spell; class_spells rank 0, level 1", choiceSpellSrc, res})
	}
	ex, ok := learnedHas(levelup.Learned(usabChar("WARRIOR", 60, "Horde"), ds, 60), "Execute")
	_, early := learnedHas(levelup.Learned(usabChar("WARRIOR", 23, "Horde"), ds, 23), "Execute")
	out = append(out, benchRow{"warrior Execute: the level it is listed from", fmt.Sprintf("level %d; listed at 23: %v", ex.Level, early), "class_spells Execute rank 1 at level 24; the rank-less row at level 1 is its damage effect", choiceSpellSrc,
		resultIf(ok && ex.Level == 24 && !early, "Execute listed from level 1")})
	finish(t, "advice-learned-not-talents", "Offline spell choice: no talent or passive listed as an ability", out)
}

// choiceChains are abilities with several ranks: each rank is offered in the window of its own level.
var choiceChains = []struct{ class, name string }{
	{"MAGE", "Frostbolt"}, {"MAGE", "Fireball"}, {"WARRIOR", "Rend"}, {"WARRIOR", "Heroic Strike"}, {"WARLOCK", "Shadow Bolt"}, {"PRIEST", "Smite"},
}

// TestOfflineBenchmarkAbilityRankChainA, B and C: each rank of an ability is offered at the level of its class_spells row and
// not at the level before, so the next rank to train is the dataset's.
func TestOfflineBenchmarkAbilityRankChainA(t *testing.T) { rankChain(t, "a", choiceChains[:2]) }
func TestOfflineBenchmarkAbilityRankChainB(t *testing.T) { rankChain(t, "b", choiceChains[2:4]) }
func TestOfflineBenchmarkAbilityRankChainC(t *testing.T) { rankChain(t, "c", choiceChains[4:]) }

func rankChain(t *testing.T, part string, chains []struct{ class, name string }) {
	ds := usabStore()
	var out []benchRow
	for _, tc := range chains {
		var got, want []string
		bad := ""
		rows := classRows(t, classID(tc.class), 1, 60)
		seen := map[int64]bool{}
		for _, r := range rows {
			rk, lv := num(r["rank"]), int(num(r["level"]))
			if text(r["name"]) != tc.name || rk == 0 || lv < 2 || seen[rk] {
				continue
			}
			seen[rk] = true
			want = append(want, fmt.Sprintf("%d@%d", rk, lv))
			s, ok := levHas(levelup.Spells(usabChar(tc.class, lv, "Horde"), ds, lv, lv), tc.name)
			got = append(got, fmt.Sprintf("%d@%d", s.Rank, s.Level))
			if !ok || int64(s.Rank) != rk || s.Level != lv {
				// the highest rank of a window supersedes the others in it
				if !ok || int64(s.Rank) < rk {
					bad = fmt.Sprintf("rank %d at level %d not offered", rk, lv)
				}
			}
			if _, early := levHas(levelup.Spells(usabChar(tc.class, lv-1, "Horde"), ds, lv-1, lv-1), tc.name); early {
				if s2, _ := levHas(levelup.Spells(usabChar(tc.class, lv-1, "Horde"), ds, lv-1, lv-1), tc.name); int64(s2.Rank) >= rk {
					bad = fmt.Sprintf("rank %d offered at level %d, a level early", rk, lv-1)
				}
			}
		}
		slices.Sort(want)
		slices.Sort(got)
		out = append(out, benchRow{fmt.Sprintf("%s %s: the level of each rank", strings.ToLower(tc.class), tc.name), strings.Join(got, " "), strings.Join(want, " ") + " (rank@level)", choiceSpellSrc,
			resultIf(len(want) > 1 && bad == "", bad)})
	}
	finish(t, "advice-ability-rank-chain-"+part, "Offline spell choice: the level of each rank of an ability", out)
}

// ---- which bag item is an upgrade ----

type bagCase struct {
	label, class string
	level        int
	worn         map[string]string // slot to the item worn there
	bag          string
}

var bagCases = []bagCase{
	{"a mage's chest: a robe made for mages over a cloth vest", "MAGE", 40, map[string]string{"chest": "Tattered Cloth Vest"}, "Manaweave Robe"},
	{"a priest's chest: the same robe, made for mages", "PRIEST", 40, map[string]string{"chest": "Tattered Cloth Vest"}, "Manaweave Robe"},
	{"a mage's chest: plate", "MAGE", 40, map[string]string{"chest": "Tattered Cloth Vest"}, "Heavy Mithril Breastplate"},
	{"a warrior's chest at 40: plate that needs level 41", "WARRIOR", 40, map[string]string{"chest": "Scalemail Vest"}, "Heavy Mithril Breastplate"},
	{"a warrior's chest at 41: plate that needs level 41", "WARRIOR", 41, map[string]string{"chest": "Scalemail Vest"}, "Heavy Mithril Breastplate"},
	{"a rogue's chest: mail", "ROGUE", 30, map[string]string{"chest": "Dirty Leather Vest"}, "Scalemail Vest"},
	{"a hunter's chest at 25: mail before the armor change", "HUNTER", 25, map[string]string{"chest": "Dirty Leather Vest"}, "Scalemail Vest"},
	{"a hunter's chest at 40: mail after the armor change", "HUNTER", 40, map[string]string{"chest": "Dirty Leather Vest"}, "Scalemail Vest"},
	{"a warlock's empty head: a hat made for warlocks", "WARLOCK", 35, nil, "Dread Mage Hat"},
	{"a mage's empty head: the warlock's hat", "MAGE", 35, nil, "Dread Mage Hat"},
	{"a rogue's empty finger: a ring", "ROGUE", 30, nil, "Ring of Pure Silver"},
	{"a rogue's weapon: a mace over a worn dagger", "ROGUE", 30, map[string]string{"mainhand": "Worn Dagger"}, "Mace"},
	{"a mage's weapon: a mace over a worn dagger", "MAGE", 30, map[string]string{"mainhand": "Worn Dagger"}, "Mace"},
	{"a mage's chest: the vest already worn", "MAGE", 40, map[string]string{"chest": "Tattered Cloth Vest"}, "Tattered Cloth Vest"},
}

// bagChar is a character of the class and level wearing the named items with the bag item in the bags.
func bagChar(t *testing.T, class string, level int, worn map[string]string, bag string) character.Character {
	t.Helper()
	c := usabChar(class, level, "Horde")
	c.BagsCaptured, c.Equipment = true, map[string]character.EquippedItem{}
	for slot, name := range worn {
		r := usabItem(t, name)
		id := int(num(r["id"]))
		c.Equipment[slot] = character.EquippedItem{Slot: slot, ItemID: &id, Name: name}
	}
	r := usabItem(t, bag)
	c.Bags = []character.ItemStack{{ItemID: int(num(r["id"])), Count: 1, Name: bag}}
	return c
}

// effectiveLevel is the level an item needs: the dataset's required_level, or the estimate from its item level when it holds 0.
func effectiveLevel(r store.Row) int64 {
	if rl := num(r["required_level"]); rl > 0 {
		return rl
	}
	return int64(gear.EstimateRequiredLevel(int(num(r["item_level"]))))
}

// bagWant is the dataset's verdict on a bag item: an upgrade when the class can use it at the level, it needs no more than the level,
// and it is better by item level than what is worn (or the slot is empty); the reason otherwise.
func bagWant(t *testing.T, tc bagCase) (up bool, why string) {
	t.Helper()
	b := usabItem(t, tc.bag)
	id := classID(tc.class)
	switch {
	case !refUsable(t, b, tc.class, id, tc.level):
		return false, "the class cannot wear or wield it at this level (class mask, armor type or weapon skill)"
	case effectiveLevel(b) > int64(tc.level):
		return false, fmt.Sprintf("needs level %d", effectiveLevel(b))
	}
	for _, name := range tc.worn {
		w := usabItem(t, name)
		if num(w["id"]) == num(b["id"]) {
			return false, "the item is the one worn"
		}
		if num(b["item_level"]) <= num(w["item_level"]) {
			return false, fmt.Sprintf("item level %d is not above the worn %d", num(b["item_level"]), num(w["item_level"]))
		}
		return true, fmt.Sprintf("item level %d above the worn %d", num(b["item_level"]), num(w["item_level"]))
	}
	return true, "an empty slot"
}

func bagBatch(t *testing.T, area, title string, cases []bagCase) {
	ds := usabStore()
	var out []benchRow
	for _, tc := range cases {
		want, why := bagWant(t, tc)
		ups := gear.BagUpgrades(bagChar(t, tc.class, tc.level, tc.worn, tc.bag), ds, "")
		got := "no upgrade"
		if len(ups) == 1 {
			got = ups[0].Line()
		}
		ref := "no upgrade: " + why
		if want {
			ref = "an upgrade: " + why
		}
		out = append(out, benchRow{fmt.Sprintf("%s (%s, level %d): %s", tc.label, strings.ToLower(tc.class), tc.level, tc.bag), got, ref, choiceGearSrc,
			resultIf(len(ups) == btoi(want), "an upgrade the dataset's rows do not give, or one missed")})
	}
	finish(t, area, title, out)
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

// TestOfflineBenchmarkBagUpgradeArmor: a bag piece is an upgrade only when the class wears its armor type at the level, the class mask allows
// the class and the level needed is reached.
func TestOfflineBenchmarkBagUpgradeArmor(t *testing.T) {
	bagBatch(t, "advice-bag-upgrade-armor", "Offline gear upgrade: armor in the bags", bagCases[:9])
}

// TestOfflineBenchmarkBagUpgradeSlots: an empty slot takes any usable piece, a ring fills a finger, a weapon needs a skill the class can learn.
func TestOfflineBenchmarkBagUpgradeSlots(t *testing.T) {
	bagBatch(t, "advice-bag-upgrade-slots", "Offline gear upgrade: empty slots, rings and weapons in the bags", bagCases[9:])
}

var bagThresholds = []struct {
	class, item string
}{
	{"WARRIOR", "Steel Plate Helm"}, {"ROGUE", "Studded Hat"}, {"WARRIOR", "Chainmail Belt"}, {"PALADIN", "Scalemail Vest"}, {"MAGE", "Defias Mage Ring"}, {"ROGUE", "Iron Fists"},
}

// TestOfflineBenchmarkBagUpgradeLevel: a bag piece is an upgrade from the level the dataset says it needs, not a level before (the gear
// search looks two levels ahead, a bag piece is for wearing now).
func TestOfflineBenchmarkBagUpgradeLevel(t *testing.T) {
	ds := usabStore()
	var out []benchRow
	for _, tc := range bagThresholds {
		r := usabItem(t, tc.item)
		rl := int(effectiveLevel(r))
		slot := ""
		at := gear.BagUpgrades(bagChar(t, tc.class, rl, nil, tc.item), ds, "")
		before := gear.BagUpgrades(bagChar(t, tc.class, rl-1, nil, tc.item), ds, "")
		if len(at) == 1 {
			slot = at[0].Slot
		}
		out = append(out, benchRow{fmt.Sprintf("%s (%s): the level it is an upgrade from", tc.item, strings.ToLower(tc.class)),
			fmt.Sprintf("at level %d: %d upgrade in %s; at level %d: %d", rl, len(at), slot, rl-1, len(before)), fmt.Sprintf("required_level %d (item level %d)", num(r["required_level"]), num(r["item_level"])), choiceGearSrc,
			resultIf(len(at) == 1 && len(before) == 0, "offered a level early, or not from the level needed")})
	}
	finish(t, "advice-bag-upgrade-level", "Offline gear upgrade: the level a bag piece is an upgrade from", out)
}

// ---- no answer in the dataset ----

// TestOfflineBenchmarkChoicesNoAnswer: what the dataset cannot answer about a spell or a talent choice, a rotation, a spell's cost or
// damage, a best-in-slot list and the percent an upgrade gives. The advisor treats a rotation question as know-how, frames it with the
// abilities the class has learned and the dataset's levels only, and states no figure.
func TestOfflineBenchmarkChoicesNoAnswer(t *testing.T) {
	ds := usabStore()
	var out []benchRow
	mage := usabChar("MAGE", 30, "Horde")
	for _, q := range []string{"what is the best rotation for my mage?", "what is the spell priority for a fire mage?", "how do i play my class?"} {
		cl := ask.Classify(q, ds, &mage)
		out = append(out, benchRow{"the question \"" + q + "\"", fmt.Sprintf("kind %d, topic %q, subject %q", cl.Kind, cl.Topic, cl.Subject), "a know-how question about a class: no rotation or spell priority in the dataset", choiceNoneSrc,
			resultIf(cl.Kind == ask.KindKnowHow && cl.Topic == ask.TopicClass && cl.Subject == "mage", "a rotation question answered as the player's own state")})
	}
	rows := classRows(t, classID("MAGE"), 1, 60)
	var extra []string
	for _, col := range []string{"priority", "cooldown", "mana_cost", "cost", "damage", "cast_time", "rotation"} {
		if len(rows) > 0 {
			if _, ok := rows[0][col]; ok {
				extra = append(extra, col)
			}
		}
	}
	out = append(out, benchRow{"a spell's cooldown, mana cost, damage or priority", fmt.Sprintf("columns held: %v", extra), "class_spells holds a name, rank, level and skill line only", choiceNoneSrc,
		resultIf(len(extra) == 0 && len(rows) > 0, "the dataset holds a figure the answer could state")})
	for _, table := range []string{"spell_priority", "rotations", "best_in_slot", "talent_tiers", "item_upgrade_values"} {
		_, err := ds.Filter(table, store.Range{Column: "id", Min: int64(0), Max: int64(1)})
		out = append(out, benchRow{"a table of " + strings.ReplaceAll(table, "_", " "), fmt.Sprint(err != nil), "no such table in the dataset", choiceNoneSrc, resultIf(err != nil, "a table was invented")})
	}
	for _, tc := range []bagCase{bagCases[0], bagCases[4]} {
		ups := gear.BagUpgrades(bagChar(t, tc.class, tc.level, tc.worn, tc.bag), ds, "")
		line := ""
		if len(ups) == 1 {
			line = ups[0].Line()
		}
		out = append(out, benchRow{fmt.Sprintf("%s: how much better the upgrade is", tc.bag), line, "no stat amounts or percent in the dataset to measure an upgrade by", choiceNoneSrc,
			resultIf(line != "" && !strings.ContainsAny(line, "%") && ups[0].Likely, "a percent or a stat figure was claimed")})
	}
	finish(t, "advice-choices-no-answer", "Offline spell and gear choices: no answer in the dataset", out)
}

package gear

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	itemdata "github.com/WoW-Companion/advisor/internal/items"
	"github.com/WoW-Companion/advisor/internal/stats"
	"github.com/WoW-Companion/advisor/internal/store"
)

// The offline gear sweep benchmark: the gear finder's answer for synthetic characters (every class with rules, a damage and a
// shield or healer build, levels across 1 to 60, both factions, empty slots so every usable item is a candidate) beside what the
// dataset's item rows allow, with no network. The reference is written apart from the finder: the Classic armor type per class and
// level (cloth, leather, mail, plate; the plate and mail classes gain theirs at 40), the weapon types per class, the shield
// classes and Dual Wield levels, read against each row's class mask, quality, bonding, required level and inventory type. The
// stat-priority part sets the lead stat of every class and spec of the stat table beside the primary stat the Classic guides
// name, in the stat check and in the gear weights, and the loot roll part sets the Need or Greed answer for every gear row beside
// the same reference. make offline-benchmark runs it; WOWC_BENCH_REPORT receives the tables.

const sweepSrc = "dataset items (class_id, subclass_id, slot, allowable_class_mask, required_level, quality, bonding) against the Classic armor and weapon rules"

// sweepArmor is the armor subclass a class wears below 40 and from 40.
var sweepArmor = map[string][2]int{
	"WARRIOR": {3, 4}, "PALADIN": {3, 4}, "HUNTER": {2, 3}, "SHAMAN": {2, 3},
	"ROGUE": {2, 2}, "DRUID": {2, 2}, "PRIEST": {1, 1}, "MAGE": {1, 1}, "WARLOCK": {1, 1},
}

// sweepWeapons is the weapon subclasses each class may wield: axe 0/1, mace 4/5, sword 7/8, polearm 6, staff 10, fist 13, dagger 15,
// bow 2, gun 3, crossbow 18, thrown 16, wand 19.
var sweepWeapons = map[string][]int{
	"WARRIOR": {0, 1, 2, 3, 4, 5, 6, 7, 8, 10, 13, 15, 16, 18},
	"PALADIN": {0, 1, 4, 5, 6, 7, 8},
	"HUNTER":  {0, 1, 2, 3, 6, 7, 8, 10, 13, 15, 16, 18},
	"ROGUE":   {0, 2, 3, 4, 7, 13, 15, 16, 18},
	"PRIEST":  {4, 10, 15, 19},
	"SHAMAN":  {0, 1, 4, 5, 10, 13, 15},
	"MAGE":    {7, 10, 15, 19},
	"WARLOCK": {7, 10, 15, 19},
	"DRUID":   {4, 5, 6, 10, 13, 15},
}

var sweepDual = map[string]int{"WARRIOR": 20, "HUNTER": 20, "ROGUE": 10, "SHAMAN": 40}
var sweepShield = map[string]bool{"WARRIOR": true, "PALADIN": true, "SHAMAN": true}
var sweepClassID = map[string]int{"WARRIOR": 1, "PALADIN": 2, "HUNTER": 3, "ROGUE": 4, "PRIEST": 5, "SHAMAN": 7, "MAGE": 8, "WARLOCK": 9, "DRUID": 11}

var (
	sweepOnce sync.Once
	sweepRows []store.Row
)

func sweepItems() []store.Row {
	sweepOnce.Do(func() {
		rows, err := snapshot().Filter("items", store.Range{Column: "id", Min: 0, Max: 1 << 40})
		if err != nil {
			panic(err)
		}
		sweepRows = rows
	})
	return sweepRows
}

// sweepUsable is whether the class, at the level the item needs or the character has (the later), can wear or wield the row: armor type,
// weapon type, shield, class mask. Slots and the dual-wield rule are the caller's.
func sweepUsable(class string, level int, row store.Row) bool {
	if mask := i64(row["allowable_class_mask"]); mask > 0 && mask&(1<<(sweepClassID[class]-1)) == 0 {
		return false
	}
	sub, slot := int(i64(row["subclass_id"])), int(i64(row["slot"]))
	if i64(row["class_id"]) == 2 {
		for _, w := range sweepWeapons[class] {
			if w == sub {
				return true
			}
		}
		return false
	}
	switch {
	case slot == 2 || slot == 11 || slot == 12 || slot == 16 || slot == 23:
		return true
	case slot == 14 || sub == 6:
		return sweepShield[class]
	}
	a := sweepArmor[class]
	if level < 40 {
		return sub == a[0]
	}
	return sub == a[1]
}

// sweepSlots are the capture slots a usable row fills for the class at the level, by its inventory type; finger and trinket slots are
// one, since the pair never offers one item twice.
func sweepSlots(class string, level int, slot int, shield bool) []string {
	dual := false
	if need, ok := sweepDual[class]; ok && level >= need {
		dual = true
	}
	switch slot {
	case 1:
		return []string{"head"}
	case 2:
		return []string{"neck"}
	case 3:
		return []string{"shoulder"}
	case 5, 20:
		return []string{"chest"}
	case 6:
		return []string{"waist"}
	case 7:
		return []string{"legs"}
	case 8:
		return []string{"feet"}
	case 9:
		return []string{"wrist"}
	case 10:
		return []string{"hands"}
	case 11:
		return []string{"finger"}
	case 12:
		return []string{"trinket"}
	case 16:
		return []string{"back"}
	case 13:
		if dual {
			return []string{"mainhand", "offhand"}
		}
		return []string{"mainhand"}
	case 17, 21:
		return []string{"mainhand"}
	case 22:
		if dual {
			return []string{"offhand"}
		}
		return nil
	case 14, 23:
		return []string{"offhand"}
	case 15, 25, 26:
		return []string{"ranged"}
	}
	return nil
}

func sweepSlotName(s string) string {
	if strings.HasPrefix(s, "finger") {
		return "finger"
	}
	if strings.HasPrefix(s, "trinket") {
		return "trinket"
	}
	return s
}

// sweepReference is the (slot, item id) pairs the dataset's rows allow the class at the level.
func sweepReference(class, faction string, level int, shields bool) map[string]bool {
	out := map[string]bool{}
	for _, row := range sweepItems() {
		slot := int(i64(row["slot"]))
		cls := i64(row["class_id"])
		if slot == 0 || (cls != 2 && cls != 4) {
			continue
		}
		it := itemOf(row)
		if itemdata.Unobtainable(it.Name) || itemdata.WrongFaction(it.Name, faction) || it.ItemLevel <= 1 || it.Quality <= 0 || it.Quality == 6 || it.Bonding == 4 || it.Bonding == 5 {
			continue
		}
		// the level the item needs: the dataset's, or item level less 5 when it holds none or one far too low
		req, _ := ReqLevelOf(it.RequiredLevel, it.ItemLevel)
		if req > level+2 {
			continue
		}
		if !sweepUsable(class, max(level, req), row) {
			continue
		}
		isShield := cls == 4 && (slot == 14 || int(i64(row["subclass_id"])) == 6)
		if isShield && !shields {
			continue // a two-hand or dual-wield damage build keeps no shield
		}
		for _, s := range sweepSlots(class, level, slot, isShield) {
			out[fmt.Sprintf("%s/%d", s, it.ID)] = true
		}
	}
	return out
}

type sweepBuild struct{ spec, role string }

// sweepBuilds: a damage build for every class, and the shield or healer build for the classes that carry a shield.
var sweepBuilds = map[string][]sweepBuild{
	"WARRIOR": {{"Fury", "damage"}, {"Protection", "tank"}},
	"PALADIN": {{"Retribution", "damage"}, {"Protection", "tank"}},
	"HUNTER":  {{"Beast Mastery", "damage"}},
	"ROGUE":   {{"Combat", "damage"}},
	"PRIEST":  {{"Holy", "healer"}},
	"SHAMAN":  {{"Enhancement", "damage"}, {"Restoration", "healer"}},
	"MAGE":    {{"Frost", "damage"}},
	"WARLOCK": {{"Affliction", "damage"}},
	"DRUID":   {{"Feral Combat", "damage"}, {"Restoration", "healer"}},
}

var sweepLevels = []int{4, 12, 22, 33, 39, 40, 51, 60}

func sweepClass(t *testing.T, class string) {
	t.Helper()
	ds := snapshot()
	var rows []benchRow
	for _, b := range sweepBuilds[class] {
		shields := sweepShield[class] && b.role != "damage"
		for i, lv := range sweepLevels {
			faction := []string{"Alliance", "Horde"}[i%2] // both factions across the levels
			lvl := lv
			c := character.Character{Name: "Sweep", Class: class, Faction: faction, Level: &lvl, Profile: &character.Profile{Role: b.role, Focus: "leveling", Spec: b.spec}}
			res, err := Find(c, ds, "", Options{NoSources: true})
			if err != nil {
				t.Fatal(err)
			}
			app := map[string]bool{}
			for slot, cands := range res.Slots {
				for _, cd := range cands {
					app[fmt.Sprintf("%s/%d", sweepSlotName(slot), cd.Item.ID)] = true
				}
			}
			ref := sweepReference(class, faction, lv, shields)
			var extra, missing []string
			for k := range app {
				if !ref[k] {
					extra = append(extra, k)
				}
			}
			for k := range ref {
				if !app[k] {
					missing = append(missing, k)
				}
			}
			sort.Strings(extra)
			sort.Strings(missing)
			bad := ""
			if len(extra) > 0 {
				bad += fmt.Sprintf(" offers %v the rows do not allow;", extra)
			}
			if len(missing) > 0 {
				bad += fmt.Sprintf(" leaves out %v the rows allow;", missing)
			}
			rows = append(rows, benchRow{
				fmt.Sprintf("%s %s level %d, %s: usable gear in every slot", strings.Title(strings.ToLower(class)), b.spec, lv, faction),
				fmt.Sprintf("%d (slot, item) pairs", len(app)), fmt.Sprintf("%d (slot, item) pairs", len(ref)), sweepSrc,
				resultIf(bad == "", strings.TrimSuffix(bad, ";"))})
		}
	}
	finish(t, "gear-sweep-"+strings.ToLower(class), "Offline gear sweep: "+class, rows)
}

func TestOfflineBenchmarkGearSweepWarrior(t *testing.T) { sweepClass(t, "WARRIOR") }
func TestOfflineBenchmarkGearSweepPaladin(t *testing.T) { sweepClass(t, "PALADIN") }
func TestOfflineBenchmarkGearSweepHunter(t *testing.T)  { sweepClass(t, "HUNTER") }
func TestOfflineBenchmarkGearSweepRogue(t *testing.T)   { sweepClass(t, "ROGUE") }
func TestOfflineBenchmarkGearSweepPriest(t *testing.T)  { sweepClass(t, "PRIEST") }
func TestOfflineBenchmarkGearSweepShaman(t *testing.T)  { sweepClass(t, "SHAMAN") }
func TestOfflineBenchmarkGearSweepMage(t *testing.T)    { sweepClass(t, "MAGE") }
func TestOfflineBenchmarkGearSweepWarlock(t *testing.T) { sweepClass(t, "WARLOCK") }
func TestOfflineBenchmarkGearSweepDruid(t *testing.T)   { sweepClass(t, "DRUID") }

// sweepLead is the primary stat the Classic guides name for each build (the stat the class turns into power), and the stat the
// guides call the tank's first.
var sweepLead = map[string]string{
	"WARRIOR Fury": "strength", "WARRIOR Arms": "strength", "WARRIOR Protection": "stamina",
	"PALADIN Retribution": "strength", "PALADIN Protection": "stamina", "PALADIN Holy": "intellect",
	"HUNTER Marksmanship": "agility", "HUNTER Beast Mastery": "agility", "HUNTER Survival": "agility",
	"ROGUE Combat": "agility", "ROGUE Assassination": "agility", "ROGUE Subtlety": "agility",
	"PRIEST Holy": "intellect", "PRIEST Discipline": "intellect", "PRIEST Shadow": "intellect",
	"SHAMAN Enhancement": "strength", "SHAMAN Elemental": "intellect", "SHAMAN Restoration": "intellect",
	"MAGE Fire": "intellect", "MAGE Frost": "intellect", "MAGE Arcane": "intellect",
	"WARLOCK Affliction": "intellect", "WARLOCK Destruction": "intellect", "WARLOCK Demonology": "intellect",
	"DRUID Balance": "intellect", "DRUID Feral Combat": "agility", "DRUID Restoration": "intellect",
	"DRUID Feral Combat/tank": "stamina", // a bear is a tank: health first
}

// TestOfflineBenchmarkStatLeadSweep: for every class and spec the stat table holds, under each stat preference, the stat check's lead
// stat and the stat the gear finder weighs highest are the primary stat the guides name; and no wording tells a class about a resource
// or block it does not have.
func TestOfflineBenchmarkStatLeadSweep(t *testing.T) {
	var rows []benchRow
	for _, class := range stats.Classes() {
		for key, want := range sweepLead {
			cls, spec, _ := strings.Cut(key, " ")
			spec, onlyRole, _ := strings.Cut(spec, "/")
			if cls != class {
				continue
			}
			for _, role := range []string{"damage", "tank", "healer"} {
				if onlyRole != "" && role != onlyRole {
					continue
				}
				if _, tanked := sweepLead[cls+" "+spec+"/"+role]; tanked && onlyRole == "" {
					continue
				}
				c := character.Character{Class: class, Profile: &character.Profile{Role: role, Spec: spec}}
				p, ok := stats.For(c, "")
				if !ok || p.Spec != spec {
					continue // the spec has no profile of this role
				}
				for _, pref := range []string{stats.PrefBalanced, stats.PrefDamage, stats.PrefSurvival} {
					c.Profile.StatPref = pref
					p, _ = stats.For(c, "")
					order := p.Order()
					gearTop, tw := "", -1.0
					for _, s := range stats.Primary {
						if w := p.ItemWeights()[s]; w > tw {
							gearTop, tw = s, w
						}
					}
					got := ""
					if len(order) > 0 {
						got = order[0]
					}
					rows = append(rows, benchRow{
						fmt.Sprintf("%s %s (%s), %s: lead stat", strings.Title(strings.ToLower(class)), spec, role, pref),
						got + " (check), " + gearTop + " (gear)", want, "Classic guides: primary stat per spec; the dataset has no stat priority table",
						resultIf(got == want && gearTop == want, "the lead stat is not the guides' "+want)})
				}
			}
		}
	}
	finish(t, "stat-lead-sweep", "Offline stat lead sweep: every spec, every stat preference", rows)
}

// TestOfflineBenchmarkPlainItemRankSweep: in every slot of every class's damage build, an item that carries stats never ranks below a
// plain one (the dataset says it has no stats) of no higher item level: a plain piece counts half its item level, so it cannot outrank
// a piece that carries the stats the build weighs. The dataset's plain pieces are white.
func TestOfflineBenchmarkPlainItemRankSweep(t *testing.T) {
	ds := snapshot()
	var rows []benchRow
	for _, class := range stats.Classes() {
		b := sweepBuilds[class][0]
		lvl := 30
		c := character.Character{Name: "Sweep", Class: class, Level: &lvl, Profile: &character.Profile{Role: b.role, Focus: "leveling", Spec: b.spec}}
		res, err := Find(c, ds, "", Options{NoSources: true})
		if err != nil {
			t.Fatal(err)
		}
		var bad []string
		for slot, cands := range res.Slots {
			for i, plain := range cands {
				if !plain.Item.NoStats {
					continue
				}
				for _, other := range cands[i+1:] {
					if other.Item.StatsKnown && other.Item.ItemLevel >= plain.Item.ItemLevel {
						bad = append(bad, fmt.Sprintf("%s: %s (item level %d) below plain %s (%d)", slot, other.Item.Name, other.Item.ItemLevel, plain.Item.Name, plain.Item.ItemLevel))
					}
				}
			}
		}
		sort.Strings(bad)
		rows = append(rows, benchRow{
			fmt.Sprintf("%s %s level 30: a stat item ranks above a plain item of no higher item level", strings.Title(strings.ToLower(class)), b.spec),
			fmt.Sprintf("%d inversions", len(bad)), "0 inversions", "dataset items.stats_status none (plain), known (stats)",
			resultIf(len(bad) == 0, strings.Join(bad, "; "))})
	}
	finish(t, "gear-plain-rank-sweep", "Offline gear sweep: plain items against items with stats", rows)
}

// sweepRollRef is the roll the dataset's row and the Classic rules give a class at a level: a quest item is passed, an item the class
// can wear or wield (at the level the item needs, when that is above its own) is Needed if it helps, anything else is Greed.
func sweepRollRef(class string, level int, row store.Row) Roll {
	if b := i64(row["bonding"]); b == 4 || b == 5 || i64(row["class_id"]) == 12 {
		return RollPass
	}
	cls, slot := i64(row["class_id"]), int(i64(row["slot"]))
	if slot == 0 || (cls != 2 && cls != 4) {
		return RollGreed
	}
	if mask := i64(row["allowable_class_mask"]); mask > 0 && mask&(1<<(sweepClassID[class]-1)) == 0 {
		return RollGreed
	}
	if slot == 4 || slot == 19 {
		return RollNeed // a shirt or tabard suits every class
	}
	req, _ := ReqLevelOf(int(i64(row["required_level"])), int(i64(row["item_level"])))
	if sweepUsable(class, max(level, req), row) {
		return RollNeed
	}
	return RollGreed
}

// TestOfflineBenchmarkLootRollSweep: the roll for every worn-gear row of the dataset, for every class at three levels, beside the
// armor and weapon rules read at the level the item needs.
func TestOfflineBenchmarkLootRollSweep(t *testing.T) {
	var rows []benchRow
	for _, class := range stats.Classes() {
		for _, lv := range []int{12, 38, 45} {
			var bad []string
			n := 0
			for _, row := range sweepItems() {
				if i64(row["slot"]) == 28 {
					continue // relics are the paladin, druid and shaman's own
				}
				n++
				if got, want := LootRoll(class, lv, row).Roll, sweepRollRef(class, lv, row); got != want && len(bad) < 3 {
					bad = append(bad, fmt.Sprintf("%v rolls %s, rows say %s", row["name"], got, want))
				}
			}
			rows = append(rows, benchRow{
				fmt.Sprintf("%s level %d: the roll on each of %d dataset rows", strings.Title(strings.ToLower(class)), lv, n),
				fmt.Sprintf("%d differ", len(bad)), "0 differ", sweepSrc,
				resultIf(len(bad) == 0, strings.Join(bad, "; "))})
		}
	}
	finish(t, "gear-loot-roll-sweep", "Offline loot roll sweep: every class and row", rows)
}

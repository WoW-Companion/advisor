// Package levelup answers "what's new at my next level?": the spells and ranks a class trainer
// teaches in a level window, what they cost when the dataset knows, the nearest class trainer,
// whether the character's money covers it, and the class quests coming up. Everything comes from
// the dataset's class_spells and trainer_spells, the capture, and the player's QuestieDB; nothing is
// guessed.
package levelup

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/WoW-Companion/advisor/internal/advice"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/items"
	"github.com/WoW-Companion/advisor/internal/learn"
	"github.com/WoW-Companion/advisor/internal/questie"
	"github.com/WoW-Companion/advisor/internal/store"
	"github.com/WoW-Companion/advisor/internal/vendors"
)

// ClassIDs maps the capture's class token to the dataset's class_id.
var ClassIDs = map[string]int64{"WARRIOR": 1, "PALADIN": 2, "HUNTER": 3, "ROGUE": 4, "PRIEST": 5, "SHAMAN": 7, "MAGE": 8, "WARLOCK": 9, "DRUID": 11}

// Spell is one ability or rank a trainer teaches.
type Spell struct {
	ID    int64
	Name  string
	Rank  int   // 0 when the ability has no rank
	Level int   // the level it is learned at
	New   bool  // false when it upgrades a known rank
	Cost  int64 // copper; 0 when no corroborated offer carries one
	Seen  bool  // the cost is the one a trainer showed the player
	// SeenAt is when that trainer window was read (unix s); NeedLevel is the level its window said, 0 when none.
	SeenAt    int64
	NeedLevel int
	Blocked   bool // the trainer showed it as not yet available
	// Requires is the spell id of the rank to learn first (0 when none or already known); RequiresQuest the
	// quest that teaches that prerequisite (0 when none). State is set by Ahead: "blocked" or "later".
	Requires      int64
	RequiresQuest int
	State         string
	// Usually marks a spell from the dataset's class list, with no opened class trainer to confirm it.
	Usually bool
}

// Label is the spell as a player reads it: "Rend (Rank 2)".
func (s Spell) Label() string {
	if s.Rank > 0 {
		return fmt.Sprintf("%s (Rank %d)", s.Name, s.Rank)
	}
	return s.Name
}

// classRows lists the dataset's class spells for levels from to to; nil when there are none.
func classRows(c character.Character, ds store.DataStore, from, to int) []store.Row {
	cid, ok := ClassIDs[strings.ToUpper(c.Class)]
	if ds == nil || !ok || to < from {
		return nil
	}
	rows, err := ds.Filter("class_spells", store.Range{Column: "class_id", Min: cid, Max: cid}, store.Range{Column: "level", Min: int64(from), Max: int64(to)})
	if err != nil {
		return nil
	}
	return rows
}

// Spells lists the trainer-taught spells of the character's class for levels from to to, in level
// order. When the player opened their class trainer, that trainer's own offers are the list: offered,
// at or below the window, not shown as learned. Without one the dataset's class list stands in and
// each spell is marked Usually. A spell the capture says the character knows is left out, a rank a
// higher rank in the same window supersedes is too, and so is a spell a quest teaches (see QuestSpells).
func Spells(c character.Character, ds store.DataStore, from, to int) []Spell {
	rows := classRows(c, ds, from, to)
	if t := classTrainerSeen(c, ds); t != nil {
		return trainerSpells(c, ds, t, from, to)
	}
	if len(rows) == 0 {
		return nil
	}
	quest := questTaught(ds)
	superseded := map[int64]bool{}
	for _, r := range rows {
		if s, _ := r["superseded_spell_id"].(int64); s != 0 {
			superseded[s] = true
		}
	}
	lo, hi := int64(1<<62), int64(0)
	for _, r := range rows {
		if id, _ := r["spell_id"].(int64); id != 0 {
			lo, hi = min(lo, id), max(hi, id)
		}
	}
	costs := costsOf(ds, lo, hi)
	talents := talentNames(ds, ClassIDs[strings.ToUpper(c.Class)])
	late := lateRanked(ds, ClassIDs[strings.ToUpper(c.Class)])
	var out []Spell
	seen := map[int64]bool{}
	for _, r := range rows {
		id, _ := r["spell_id"].(int64)
		name, _ := r["name"].(string)
		lv, _ := r["level"].(int64)
		if t, ok := r["trained"].(bool); ok && !t { // learned automatically, not at a trainer
			continue
		}
		rk, _ := r["rank"].(int64)
		if id == 0 || name == "" || seen[id] || superseded[id] || c.KnownSpells[id] || quest[id].Quest != "" || isRuneAbility(id, lv) || isTalentEffect(talents, name, rk) || sideEffect(late, name, rk, lv) || questLocked(c, name, int(rk), false) {
			continue
		}
		seen[id] = true
		rank, _ := r["rank"].(int64)
		sup, _ := r["superseded_spell_id"].(int64)
		out = append(out, Spell{ID: id, Name: name, Rank: int(rank), Level: int(lv), New: sup == 0 && rk <= 1, Cost: costs[id], Usually: true})
	}
	out = dedupeByName(out)
	sortSpells(out)
	return out
}

// Learned lists the abilities of the character's class a player has by level to: every first rank
// of the dataset's class_spells at or below it, with the level that rank is learned at, in level
// order. It leaves out what no trainer teaches and no level gives: a talent or its effect, the
// passive half of a stance or form, a rune ability, and a spell a class quest teaches that the
// character has not learned. Ranks above the first are not listed; the first rank's level stands.
func Learned(c character.Character, ds store.DataStore, to int) []Spell {
	rows := classRows(c, ds, 1, to)
	if len(rows) == 0 {
		return nil
	}
	cid := ClassIDs[strings.ToUpper(c.Class)]
	quest := questTaught(ds)
	talents := talentNames(ds, cid)
	late := lateRanked(ds, cid)
	var out []Spell
	for _, r := range rows {
		id, _ := r["spell_id"].(int64)
		name, _ := r["name"].(string)
		lv, _ := r["level"].(int64)
		rk, _ := r["rank"].(int64)
		sup, _ := r["superseded_spell_id"].(int64)
		if id == 0 || name == "" || sup != 0 || isRuneAbility(id, lv) || isTalentEffect(talents, name, rk) || sideEffect(late, name, rk, lv) {
			continue
		}
		if !c.KnownSpells[id] && (quest[id].Quest != "" || questLocked(c, name, int(rk), false)) {
			continue
		}
		out = append(out, Spell{ID: id, Name: name, Rank: int(rk), Level: int(lv), New: true, Usually: true})
	}
	out = dedupeLowest(out)
	sortSpells(out)
	return out
}

// dedupeLowest keeps one spell for each name: the one learned at the lowest level.
func dedupeLowest(in []Spell) []Spell {
	at := map[string]int{}
	var out []Spell
	for _, s := range in {
		k := strings.ToLower(strings.TrimSpace(s.Name))
		if i, dup := at[k]; dup {
			if s.Level < out[i].Level {
				out[i] = s
			}
			continue
		}
		at[k] = len(out)
		out = append(out, s)
	}
	return out
}

// talentNames is the lower-cased names of the talents of the class's trees (talent_trees by class_id,
// talent_nodes by tree); nil when the dataset has neither table. A class spell with no rank that
// carries a talent's name is that talent or its effect, which a talent point buys and no class
// trainer teaches (the dataset's class_spells lists them with the abilities).
func talentNames(ds store.DataStore, classID int64) map[string]bool {
	if ds == nil || classID == 0 {
		return nil
	}
	trees, err := ds.Filter("talent_trees", store.Range{Column: "class_id", Min: classID, Max: classID})
	if err != nil || len(trees) == 0 {
		return nil
	}
	out := map[string]bool{}
	done := map[int64]bool{} // a tree has one row per talent group
	for _, t := range trees {
		id, _ := t["tree_id"].(int64)
		if done[id] {
			continue
		}
		done[id] = true
		nodes, err := ds.Filter("talent_nodes", store.Range{Column: "tree_id", Min: id, Max: id})
		if err != nil {
			continue
		}
		for _, n := range nodes {
			if name, _ := n["name"].(string); name != "" {
				out[strings.ToLower(strings.TrimSpace(name))] = true
			}
		}
	}
	return out
}

// isTalentEffect reports a class spell that a talent point buys or the game applies, never a class
// trainer: a rank-less spell named like one of the class's talents, a spell named like a talent
// effect or a stance's passive half (see hiddenName), or a rank-less level 1 spell whose ranks are
// taught much later (the damage half of Execute, which the dataset lists at level 1).
func isTalentEffect(talents map[string]bool, name string, rank int64) bool {
	return rank == 0 && talents[strings.ToLower(strings.TrimSpace(name))] || hiddenName(name)
}

// hiddenName reports a spell name no trainer teaches: a talent ("Improved Pummel", "Shield
// Specialization", ranks 1 to 5 listed at level 1), or the passive or effect half of an ability
// ("Battle Stance Passive", "Cat Form (Passive)", "Rogue Passive (DND)", "Hellfire Effect").
func hiddenName(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	return strings.HasPrefix(n, "improved ") || strings.Contains(n, " specialization") ||
		strings.Contains(n, "passive") || strings.Contains(n, "(dnd)") || strings.HasSuffix(n, " effect")
}

// lateRanked is the lower-cased names of the class's spells that have a ranked row above level 1:
// a rank-less level 1 row of the same name is a side effect of that spell, not a spell to train.
// It also holds, under talentRanksKey, the names with a second or later rank listed at level 1: no
// trainer teaches a rank at level 1 beyond the first, so the name is a talent bought with points
// ("Weapon Mastery" ranks 1 to 5, "Magic Attunement" ranks 1 and 2).
func lateRanked(ds store.DataStore, classID int64) map[string]bool {
	rows, err := ds.Filter("class_spells", store.Range{Column: "class_id", Min: classID, Max: classID})
	if err != nil {
		return nil
	}
	out := map[string]bool{}
	for _, r := range rows {
		name, _ := r["name"].(string)
		rk, _ := r["rank"].(int64)
		lv, _ := r["level"].(int64)
		n := strings.ToLower(strings.TrimSpace(name))
		if rk > 0 && lv > 1 && name != "" {
			out[n] = true
		}
		if rk >= 2 && lv <= 1 && name != "" {
			out[talentRanksKey(n)] = true
		}
	}
	return out
}

func talentRanksKey(lowerName string) string { return "\x00talent:" + lowerName }

// sideEffect reports a rank-less level 1 row whose name has ranked rows later (see lateRanked), or a
// level 1 row of a name whose later ranks are listed at level 1 too (a talent).
func sideEffect(late map[string]bool, name string, rank, level int64) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	return rank == 0 && level <= 1 && late[n] || rank > 0 && level <= 1 && late[talentRanksKey(n)]
}

// isRuneAbility reports a class spell the dataset lists at level 1 with a high spell id: an engraving
// (rune) ability, taught by a rune item and never by a class trainer. The dataset's `trained` column is
// empty, so this is the stand-in rule. It applies only to the dataset's fallback list: a spell the
// player's own trainer capture offers is always kept (see trainerSpells).
func isRuneAbility(id, level int64) bool { return id >= runeSpellMin && level <= 1 }

// runeSpellMin is the lowest spell id treated as a rune ability when it is also a level 1 spell.
const runeSpellMin = 400000

// dedupeByName keeps one spell per name: a trainer's own offer first, then the highest rank. The
// order of the first appearance of each name is kept.
func dedupeByName(in []Spell) []Spell {
	at := map[string]int{}
	var out []Spell
	for _, s := range in {
		k := strings.ToLower(strings.TrimSpace(s.Name))
		i, dup := at[k]
		if !dup {
			at[k] = len(out)
			out = append(out, s)
			continue
		}
		if o := out[i]; s.Seen && !o.Seen || s.Seen == o.Seen && s.Rank > o.Rank {
			out[i] = s
		}
	}
	return out
}

func sortSpells(out []Spell) {
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Level != out[j].Level {
			return out[i].Level < out[j].Level
		}
		return out[i].Name < out[j].Name
	})
}

// classTrainerSeen is the class trainer the player opened whose offers include a spell of their class
// (by the dataset's class list when it has one); nil when none was opened.
func classTrainerSeen(c character.Character, ds store.DataStore) *character.SeenTrainer {
	t := seenClassTrainer(c)
	if t == nil {
		return nil
	}
	all := classRows(c, ds, 1, 1000)
	if len(all) == 0 {
		return t // no class list to check against: the capture's word stands
	}
	mine := map[string]bool{}
	for _, r := range all {
		n, _ := r["name"].(string)
		mine[strings.ToLower(strings.TrimSpace(n))] = true
	}
	for _, o := range t.Offers {
		if mine[strings.ToLower(strings.TrimSpace(o.Name))] {
			return t
		}
	}
	return nil
}

// trainerSpells is what the opened class trainer offers for levels from to to.
func trainerSpells(c character.Character, ds store.DataStore, t *character.SeenTrainer, from, to int) []Spell {
	byName := map[string]store.Row{}
	byID := map[int64]store.Row{}
	for _, r := range classRows(c, ds, 1, 1000) {
		n, _ := r["name"].(string)
		id, _ := r["spell_id"].(int64)
		rk, _ := r["rank"].(int64)
		byName[fmt.Sprintf("%s|%d", strings.ToLower(strings.TrimSpace(n)), rk)] = r
		byID[id] = r
	}
	superseded := map[int64]bool{}
	var out []Spell
	seen := map[string]bool{}
	for _, o := range t.Offers {
		if o.Skill != "" || o.State == "used" || o.Name == "" && o.Spell == 0 {
			continue
		}
		rank := 0
		if m := rankDigits.FindString(o.Rank); m != "" {
			rank, _ = strconv.Atoi(m)
		}
		r, hit := byID[o.Spell]
		if !hit || o.Spell == 0 {
			r, hit = byName[fmt.Sprintf("%s|%d", strings.ToLower(strings.TrimSpace(o.Name)), rank)]
		}
		sp := Spell{ID: o.Spell, Name: o.Name, Rank: rank, Level: o.Level, New: rank <= 1, Cost: o.Cost, Seen: true, SeenAt: t.T,
			NeedLevel: o.Level, Blocked: o.State == "unavailable"}
		if hit {
			if id, _ := r["spell_id"].(int64); id != 0 {
				sp.ID = id
			}
			if sp.Name == "" {
				sp.Name, _ = r["name"].(string)
			}
			if lv, _ := r["level"].(int64); sp.Level == 0 {
				sp.Level = int(lv)
			}
			sup, _ := r["superseded_spell_id"].(int64)
			sp.New = sup == 0 && sp.Rank <= 1
			if sup != 0 {
				superseded[sup] = true
				if !c.KnownSpells[sup] {
					sp.Requires = sup
				}
			}
			sp.RequiresQuest = questNeeded(c, sp.Name, sp.Rank)
		}
		if sp.Name == "" || sp.Level > to || sp.Level < from || c.KnownSpells[sp.ID] && sp.ID != 0 || questLocked(c, sp.Name, sp.Rank, sp.Blocked) {
			continue
		}
		key := fmt.Sprintf("%s|%d", strings.ToLower(sp.Name), sp.Rank)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, sp)
	}
	kept := out[:0]
	for _, s := range out {
		if !superseded[s.ID] {
			kept = append(kept, s)
		}
	}
	kept = dedupeByName(kept)
	sortSpells(kept)
	return kept
}

// QuestSpell is a class spell a quest teaches, not a trainer.
type QuestSpell struct {
	ID    int64
	Name  string
	Level int
	Quest string // the quest that rewards it
}

// Line is "Defensive Stance: from the class quest The Bulwark".
func (q QuestSpell) Line() string { return q.Name + ": from the class quest " + q.Quest }

// questTaught reads the dataset's optional quest_rewards table (spell_id, quest_name); empty when absent.
func questTaught(ds store.DataStore) map[int64]QuestSpell {
	out := map[int64]QuestSpell{}
	if ds == nil {
		return out
	}
	rows, err := ds.Filter("quest_rewards", store.Range{Column: "spell_id", Min: int64(1), Max: int64(1 << 40)})
	if err != nil {
		return out
	}
	for _, r := range rows {
		id, _ := r["spell_id"].(int64)
		q, _ := r["quest_name"].(string)
		if id != 0 && q != "" {
			out[id] = QuestSpell{ID: id, Quest: q}
		}
	}
	return out
}

// QuestSpells lists the class spells of levels from to to that a quest rewards and the character does
// not know yet, never offered by a trainer.
func QuestSpells(c character.Character, ds store.DataStore, from, to int) []QuestSpell {
	quest := questTaught(ds)
	if len(quest) == 0 {
		return nil
	}
	var out []QuestSpell
	seen := map[int64]bool{}
	for _, r := range classRows(c, ds, from, to) {
		id, _ := r["spell_id"].(int64)
		name, _ := r["name"].(string)
		lv, _ := r["level"].(int64)
		if q, ok := quest[id]; ok && !seen[id] && !c.KnownSpells[id] && name != "" {
			seen[id] = true
			out = append(out, QuestSpell{ID: id, Name: name, Level: int(lv), Quest: q.Quest})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Level != out[j].Level {
			return out[i].Level < out[j].Level
		}
		return out[i].Name < out[j].Name
	})
	return out
}

var rankDigits = regexp.MustCompile(`\d+`)

// SeenClassTrainer is the class trainer the player opened (one whose window carries no profession
// skill gate), the newest on the character's own map first.
func SeenClassTrainer(c character.Character) (Trainer, bool) {
	best := seenClassTrainer(c)
	if best == nil {
		return Trainer{}, false
	}
	return Trainer{Name: best.Name, Zone: c.ZoneOfMap(best.Map), X: best.X, Y: best.Y, Map: best.Map}, true
}

func seenClassTrainer(c character.Character) *character.SeenTrainer {
	var here *int
	if c.Location != nil {
		here = c.Location.MapID
	}
	var best *character.SeenTrainer
	trs := c.Seen.TrainersFor(c.Faction)
	for i := range trs {
		t := &trs[i]
		class := len(t.Offers) > 0
		for _, o := range t.Offers {
			if o.Skill != "" {
				class = false
			}
		}
		if !class {
			continue
		}
		if best == nil || here != nil && t.Map == *here && best.Map != *here {
			best = t
		}
	}
	return best
}

// costsOf is the lowest corroborated trainer cost of each spell, from trainer_spells.
func costsOf(ds store.DataStore, lo, hi int64) map[int64]int64 {
	rows, err := ds.Filter("trainer_spells", store.Range{Column: "spell_id", Min: lo, Max: hi})
	if err != nil {
		return nil
	}
	m := map[int64]int64{}
	for _, r := range rows {
		id, _ := r["spell_id"].(int64)
		cost, _ := r["cost_copper"].(int64)
		if id != 0 && cost > 0 && (m[id] == 0 || cost < m[id]) {
			m[id] = cost
		}
	}
	return m
}

// Total is the cost of the spells that have one, and how many have none on record.
func Total(sp []Spell) (copper int64, unknown int) {
	for _, s := range sp {
		if s.Cost > 0 {
			copper += s.Cost
		} else {
			unknown++
		}
	}
	return copper, unknown
}

// MoneyLine is the plain money check: "" when the character's money is unknown or covers the total.
func MoneyLine(c character.Character, total int64) string {
	if c.Money == nil || total <= 0 || int64(*c.Money) >= total {
		return ""
	}
	have := "nothing"
	if m := vendors.Money(int64(*c.Money)); m != "" {
		have = m
	}
	return fmt.Sprintf("You have %s and training needs about %s: you are about %s short. Sell what you do not need from the Bags review to cover it.",
		have, vendors.Money(total), vendors.Money(total-int64(*c.Money)))
}

// Trainer is the nearest class trainer from the player's QuestieDB.
type Trainer struct {
	Name, Zone string
	X, Y       float64
	Map        int // the uiMapID the position is on, 0 when unknown
}

// NearestTrainer is the character's class trainer for their faction, in their zone first, else the
// nearest by map distance, else the first. ok is false without a QuestieDB or a trainer.
func NearestTrainer(c character.Character, qx *questie.Index) (Trainer, bool) {
	var at questie.Where
	if p := c.LastKnownPosition(); p != nil {
		at = questie.Where{MapID: p.MapID, Zone: p.Zone, X: p.X, Y: p.Y}
	}
	var best Trainer
	bestRank, bestDist, found := 3, 0.0, false
	for _, t := range qx.ClassTrainers(c.Class) {
		if !learn.FactionOK(qx.NpcFaction(t.ID), c.Faction) {
			continue
		}
		p, ok := qx.NpcPlace(t.ID, at)
		if !ok || p.Zone == "" {
			continue
		}
		rank := 2
		if p.Dist >= 0 {
			rank = 0
		} else if at.Zone != "" && strings.EqualFold(p.Zone, at.Zone) {
			rank = 1
		}
		if !found || rank < bestRank || (rank == bestRank && rank == 0 && p.Dist < bestDist) {
			best, bestRank, bestDist, found = Trainer{t.Name, p.Zone, p.X, p.Y, p.Map}, rank, p.Dist, true
		}
	}
	return best, found
}

func (t Trainer) String() string {
	if t.Zone == "" {
		return fmt.Sprintf("%s (%.0f, %.0f)", t.Name, t.X, t.Y)
	}
	return fmt.Sprintf("%s, %s (%.0f, %.0f)", t.Name, t.Zone, t.X, t.Y)
}

// ClassQuest is a class quest coming up: its giver and where.
type ClassQuest struct {
	ID        int
	Name      string
	Level     int
	Giver     string
	Zone      string
	X, Y      float64
	OpensAt   int // the level it opens at; 0 when open now
	Objective string
}

// ClassQuests lists the character's class quests available now or within ahead levels.
func ClassQuests(c character.Character, qx *questie.Index, ahead int) []ClassQuest {
	if c.Level == nil || qx == nil {
		return nil
	}
	var at questie.Where
	if p := c.LastKnownPosition(); p != nil {
		at = questie.Where{MapID: p.MapID, Zone: p.Zone, X: p.X, Y: p.Y}
	}
	var out []ClassQuest
	for _, o := range qx.ClassQuests(advice.WhoOf(c), at, ahead, items.Unobtainable) {
		out = append(out, ClassQuest{ID: o.Quest.ID, Name: o.Quest.Name, Level: o.Quest.Level, Giver: o.Giver.Name, Zone: o.Giver.Zone,
			X: o.Giver.X, Y: o.Giver.Y, OpensAt: o.StartLevel, Objective: o.Quest.Objective})
	}
	return out
}

// Facts is the grounded block for "what's new at my next level": the spells to train from level
// from to to with costs, the total, the money check, the trainer and the class quests coming up. ok
// is false when the dataset holds nothing for the class and window.
func Facts(c character.Character, ds store.DataStore, qx *questie.Index, from, to int) (text string, spells, quests bool) {
	sp := Spells(c, ds, from, to)
	var b strings.Builder
	class := strings.ToLower(c.Class)
	b.WriteString(TrainerSells(c, ds))
	hedge := c.KnownSpells == nil
	if len(sp) > 0 {
		if sp[0].Usually {
			fmt.Fprintf(&b, "Spells and ranks a %s usually trains at levels %d to %d (the player has not opened their class trainer, so this is the dataset's list, not what the trainer sells; never call one definitely available):\n", class, from, to)
		} else {
			fmt.Fprintf(&b, "Spells and ranks the player's class trainer offers at levels %d to %d (the trainer's own window, new ranks replace the old):\n", from, to)
		}
		if hedge {
			b.WriteString("The capture lists no known spells: say \"if you don't have it yet\" for each one; never state that any of these is definitely new.\n")
		}
		for _, s := range sp {
			kind := "new spell"
			if !s.New {
				kind = "new rank of a spell you have"
			}
			cost := "cost not known"
			if s.Cost > 0 {
				cost = "costs " + vendors.Money(s.Cost)
				if s.Seen {
					cost = "your trainer teaches it for " + vendors.Money(s.Cost) + ", " + character.SeenByYou
				}
			} else if s.Seen {
				cost = "your trainer teaches it, " + character.SeenByYou
			}
			if s.Seen {
				if s.Blocked && s.NeedLevel > 0 {
					cost += fmt.Sprintf(", not yet available: needs level %d", s.NeedLevel)
				} else if s.NeedLevel > 1 {
					cost += fmt.Sprintf(", needs level %d", s.NeedLevel)
				}
				if n := character.SeenNote(s.SeenAt, time.Now()); n != "" {
					cost += ", " + n
				}
			}
			fmt.Fprintf(&b, "- Level %d: %s (%s, %s)\n", s.Level, s.Label(), kind, cost)
		}
		total, unknown := Total(sp)
		switch {
		case total > 0 && unknown > 0:
			fmt.Fprintf(&b, "Training total: bring about %s; %d of these have no cost on record, so it is more.\n", vendors.Money(total), unknown)
		case total > 0:
			fmt.Fprintf(&b, "Training total: bring about %s.\n", vendors.Money(total))
		default:
			b.WriteString("Training costs are not known for any of these: do not give a price.\n")
		}
		if m := MoneyLine(c, total); m != "" {
			b.WriteString("Money check: " + m + "\n")
		} else if c.Money != nil && total > 0 {
			b.WriteString("Money check: the character's money covers it.\n")
		}
		if p := c.Profile; p != nil && (p.Spec != "" || p.Role != "") {
			fmt.Fprintf(&b, "Saved spec %q and role %q: say which of these to train first for them.\n", p.Spec, p.Role)
		}
		b.WriteString("Give one plain line on what each spell does; never invent a spell, rank or cost that is not listed.\n")
		spells = true
	} else {
		fmt.Fprintf(&b, "No spells for a %s to train at levels %d to %d are known (the trainer offers none, or the character already knows them): say there is nothing to train.\n", class, from, to)
	}
	for _, q := range QuestSpells(c, ds, from, to) {
		fmt.Fprintf(&b, "Not sold by the trainer: %s (level %d). Say it is learned by finishing the class quest %q, and only if the player does not have it yet.\n", q.Name, q.Level, q.Quest)
	}
	for _, l := range QuestBlocked(c, ds, qx) {
		fmt.Fprintf(&b, "Not trainable yet: %s. Say so; never list it as trainable.\n", l)
	}
	if p, ok := TrainAt(c, qx); ok && spells {
		b.WriteString(p.Line(c.Class) + ".\n")
		if p.Trainer == nil {
			b.WriteString("No source names a class trainer NPC or position here: name only the city, never an NPC or coordinates.\n")
		}
	}
	if cq := ClassQuests(c, qx, 5); len(cq) > 0 {
		b.WriteString("Class quests coming up (from the player's Questie database):\n")
		for i, q := range cq {
			if i == 3 {
				break
			}
			when := "available now"
			if q.OpensAt > 0 {
				when = fmt.Sprintf("opens at level %d", q.OpensAt)
			}
			fmt.Fprintf(&b, "- %s (level %d, %s): starts with %s in %s (%.0f, %.0f)\n", q.Name, q.Level, when, q.Giver, q.Zone, q.X, q.Y)
		}
		quests = true
	}
	return b.String(), spells, quests
}

// TrainerSells states plainly what the player's opened class trainer sells (every level, minus what
// the trainer shows as learned), "" when the player opened none. The facts give it so an answer never
// names a spell the trainer does not sell.
func TrainerSells(c character.Character, ds store.DataStore) string {
	t := classTrainerSeen(c, ds)
	if t == nil {
		return ""
	}
	var parts []string
	for _, o := range t.Offers {
		if o.Skill != "" || o.State == "used" || o.Name == "" {
			continue
		}
		n := o.Name
		if m := rankDigits.FindString(o.Rank); m != "" {
			n += " (Rank " + m + ")"
		}
		if o.Level > 0 {
			n += fmt.Sprintf(" at level %d", o.Level)
		}
		parts = append(parts, n)
	}
	who := t.Name
	if who == "" {
		who = "the class trainer"
	}
	if len(parts) == 0 {
		return who + " sells nothing the player has not learned (" + character.SeenByYou + "). Never name a spell as trainable.\n"
	}
	return who + " sells exactly: " + strings.Join(parts, ", ") + " (" + character.SeenByYou + "). Never call any other spell trainable from this trainer.\n"
}

// questLocked reports whether a trainer rank can't be trained because its Rank 1 is taught by a quest
// the player has not finished: Rank 1 itself is never a trainer spell, and a later rank is locked
// while Rank 1 is unknown (always when the trainer showed it unavailable, else only when the capture
// lists the known spells).
func questLocked(c character.Character, name string, rank int, shownBlocked bool) bool {
	sq, ok := advice.QuestSpellFor(name, int(ClassIDs[strings.ToUpper(c.Class)]))
	if !ok {
		return false
	}
	for _, id := range sq.Rank1 {
		if c.KnownSpells[id] {
			return false
		}
	}
	return rank <= 1 || shownBlocked || c.KnownSpells != nil
}

// QuestBlocked is one line per missed spell quest: the lowest blocked trainer rank (recorded as
// unavailable at the player's class trainer, else from the dataset's class list up to the character's
// level) and the quest that teaches the spell, or just where the quest is when no rank is blocked yet.
func QuestBlocked(c character.Character, ds store.DataStore, qx *questie.Index) []string {
	var out []string
	for _, m := range advice.MissedSpellQuests(c, qx) {
		where := fmt.Sprintf("it comes from the quest %s at %s", m.Quest.Name, m.Giver.Name)
		if m.Giver.Zone != "" {
			where += " (" + m.Giver.Zone + ")"
		}
		rank := 0
		if t := seenClassTrainer(c); t != nil {
			for _, o := range t.Offers {
				if n := rankDigits.FindString(o.Rank); strings.EqualFold(strings.TrimSpace(o.Name), m.Spell) && n != "" && o.State == "unavailable" {
					if r, _ := strconv.Atoi(n); r >= 2 && (rank == 0 || r < rank) {
						rank = r
					}
				}
			}
		}
		if rank == 0 && c.Level != nil {
			for _, r := range classRows(c, ds, 1, *c.Level) {
				if n, _ := r["name"].(string); strings.EqualFold(n, m.Spell) {
					if rk, _ := r["rank"].(int64); rk >= 2 && (rank == 0 || int(rk) < rank) {
						rank = int(rk)
					}
				}
			}
		}
		if rank >= 2 {
			out = append(out, fmt.Sprintf("%s (Rank %d) needs Rank 1 first: %s", m.Spell, rank, where))
		} else {
			out = append(out, fmt.Sprintf("%s: %s", m.Spell, where))
		}
	}
	return out
}

// AheadLevels is how many levels above the character the Ahead offers reach.
const AheadLevels = 6

// questNeeded is the id of the quest that teaches the Rank 1 a later rank of the spell needs, while
// the character does not know it; 0 for a trainer-taught spell or once learned.
func questNeeded(c character.Character, name string, rank int) int {
	sq, ok := advice.QuestSpellFor(name, int(ClassIDs[strings.ToUpper(c.Class)]))
	if !ok || rank < 2 {
		return 0
	}
	for _, id := range sq.Rank1 {
		if c.KnownSpells[id] {
			return 0
		}
	}
	return sq.QuestID
}

// Ahead lists the offers the trainer cannot teach yet, from the dataset's class list: a rank behind
// a quest-taught first rank ("blocked", at or below the character's level) and any rank within
// AheadLevels above their level ("later"). Each carries the prerequisite spell id and quest, so a
// reader can unlock it the moment the prerequisite is learned. Known spells are left out.
func Ahead(c character.Character, ds store.DataStore) []Spell {
	if c.Level == nil {
		return nil
	}
	level := *c.Level
	rows := classRows(c, ds, 1, level+AheadLevels)
	if len(rows) == 0 {
		return nil
	}
	quest := questTaught(ds)
	lo, hi := int64(1<<62), int64(0)
	for _, r := range rows {
		if id, _ := r["spell_id"].(int64); id != 0 {
			lo, hi = min(lo, id), max(hi, id)
		}
	}
	costs := costsOf(ds, lo, hi)
	talents := talentNames(ds, ClassIDs[strings.ToUpper(c.Class)])
	late := lateRanked(ds, ClassIDs[strings.ToUpper(c.Class)])
	var out []Spell
	seen := map[int64]bool{}
	for _, r := range rows {
		id, _ := r["spell_id"].(int64)
		name, _ := r["name"].(string)
		lv, _ := r["level"].(int64)
		rk, _ := r["rank"].(int64)
		sup, _ := r["superseded_spell_id"].(int64)
		if t, ok := r["trained"].(bool); ok && !t {
			continue
		}
		if id == 0 || name == "" || seen[id] || c.KnownSpells[id] || quest[id].Quest != "" || isRuneAbility(id, lv) || isTalentEffect(talents, name, rk) || sideEffect(late, name, rk, lv) {
			continue
		}
		s := Spell{ID: id, Name: name, Rank: int(rk), Level: int(lv), New: sup == 0 && rk <= 1, Cost: costs[id], Usually: true}
		if sup != 0 && !c.KnownSpells[sup] {
			s.Requires = sup
		}
		s.RequiresQuest = questNeeded(c, name, int(rk))
		if rk <= 1 && questLocked(c, name, int(rk), false) {
			continue // a quest teaches it: no trainer ever offers it
		}
		switch {
		case int(lv) > level:
			s.State = "later"
		case questLocked(c, name, int(rk), false):
			s.State = "blocked"
		default:
			continue // trainable now: Spells lists it
		}
		seen[id] = true
		out = append(out, s)
	}
	sortSpells(out)
	return out
}

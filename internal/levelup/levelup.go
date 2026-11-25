// Package levelup answers "what's new at my next level?": the spells and ranks a class trainer
// teaches in a level window, what they cost when the dataset knows, the nearest class trainer,
// whether the character's money covers it, and the class quests coming up. Everything comes from
// the dataset's class_spells and trainer_spells, the capture, and the player's QuestieDB; nothing is
// guessed.
package levelup

import (
	"fmt"
	"sort"
	"strings"

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
}

// Label is the spell as a player reads it: "Rend (Rank 2)".
func (s Spell) Label() string {
	if s.Rank > 0 {
		return fmt.Sprintf("%s (Rank %d)", s.Name, s.Rank)
	}
	return s.Name
}

// Spells lists the trainer-taught spells of the character's class for levels from to to, in level
// order. A spell the capture says the character knows is left out (when it lists known spells), and
// so is a rank a higher rank in the same window supersedes. Costs come from trainer_spells.
func Spells(c character.Character, ds store.DataStore, from, to int) []Spell {
	cid, ok := ClassIDs[strings.ToUpper(c.Class)]
	if ds == nil || !ok || to < from {
		return nil
	}
	rows, err := ds.Filter("class_spells", store.Range{Column: "class_id", Min: cid, Max: cid}, store.Range{Column: "level", Min: int64(from), Max: int64(to)})
	if err != nil || len(rows) == 0 {
		return nil
	}
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
	var out []Spell
	seen := map[int64]bool{}
	for _, r := range rows {
		id, _ := r["spell_id"].(int64)
		name, _ := r["name"].(string)
		lv, _ := r["level"].(int64)
		if t, ok := r["trained"].(bool); ok && !t { // learned automatically, not at a trainer
			continue
		}
		if id == 0 || name == "" || seen[id] || superseded[id] || c.KnownSpells[id] {
			continue
		}
		seen[id] = true
		rank, _ := r["rank"].(int64)
		sup, _ := r["superseded_spell_id"].(int64)
		out = append(out, Spell{ID: id, Name: name, Rank: int(rank), Level: int(lv), New: sup == 0, Cost: costs[id]})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Level != out[j].Level {
			return out[i].Level < out[j].Level
		}
		return out[i].Name < out[j].Name
	})
	return out
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
			best, bestRank, bestDist, found = Trainer{t.Name, p.Zone, p.X, p.Y}, rank, p.Dist, true
		}
	}
	return best, found
}

func (t Trainer) String() string {
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
	if len(sp) > 0 {
		fmt.Fprintf(&b, "Spells and ranks a %s trains at levels %d to %d (spells already known are left out, new ranks replace the old):\n", class, from, to)
		for _, s := range sp {
			kind := "new spell"
			if !s.New {
				kind = "new rank of a spell you have"
			}
			cost := "cost not known"
			if s.Cost > 0 {
				cost = "costs " + vendors.Money(s.Cost)
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
		fmt.Fprintf(&b, "The dataset lists no spells for a %s to train at levels %d to %d (or the character already knows them).\n", class, from, to)
	}
	if t, ok := NearestTrainer(c, qx); ok && spells {
		b.WriteString("Nearest class trainer for the character's faction: " + t.String() + ".\n")
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

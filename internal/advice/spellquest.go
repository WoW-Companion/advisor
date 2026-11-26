package advice

import (
	"fmt"
	"strings"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/questie"
)

// SpellQuest is a spell whose first rank a quest teaches, not a trainer. Every later rank is then
// blocked at the trainer until the quest is done.
type SpellQuest struct {
	QuestID int
	Spell   string
	Class   int     // the game's class id
	Rank1   []int64 // spell ids of Rank 1 (any of them known means the spell is learned)
}

// SpellQuests is the curated table of quest-taught spells. Facts only, from the quest and spell pages
// on wowhead and warcraft.wiki. The quest's own level, race and giver come from the player's Questie.
// Add a row per class or racial quest that grants a spell.
var SpellQuests = []SpellQuest{
	// Quest 5658 "Touch of Weakness" (Undercity, level 10, Undead priests) teaches Touch of Weakness Rank 1.
	{QuestID: 5658, Spell: "Touch of Weakness", Class: 5, Rank1: []int64{2652, 2943}},
}

// QuestSpellFor is the table row for a spell by name and class id.
func QuestSpellFor(name string, class int) (SpellQuest, bool) {
	for _, q := range SpellQuests {
		if strings.EqualFold(q.Spell, strings.TrimSpace(name)) && q.Class == class {
			return q, true
		}
	}
	return SpellQuest{}, false
}

// MissedSpellQuest is a spell quest the character can take, has not, and so lacks its spell.
type MissedSpellQuest struct {
	SpellQuest
	Quest *questie.Quest
	Giver questie.Place
}

// MissedSpellQuests lists the spell quests the character is eligible for (class, race, level), whose
// spell is not known, and that are neither done nor in the log. It needs the capture's known spells and
// the player's Questie; without them it says nothing.
func MissedSpellQuests(c character.Character, qx *questie.Index) []MissedSpellQuest {
	if qx == nil || c.Level == nil || c.KnownSpells == nil {
		return nil
	}
	w := whoOf(c)
	at, _ := whereOf(c)
	var out []MissedSpellQuest
	for _, sq := range SpellQuests {
		if sq.Class != w.ClassID || w.Done[sq.QuestID] || w.InLog[sq.QuestID] {
			continue
		}
		known := false
		for _, id := range sq.Rank1 {
			known = known || c.KnownSpells[id]
		}
		q := qx.Quests[sq.QuestID]
		if known || q == nil || q.Name == "" || q.MinLevel > *c.Level || !w.RaceAllows(q.Races) || qx.Hostile(q, w) ||
			q.Classes != 0 && q.Classes&(1<<(w.ClassID-1)) == 0 {
			continue
		}
		g, ok := qx.Giver(q, at)
		if !ok {
			continue
		}
		out = append(out, MissedSpellQuest{SpellQuest: sq, Quest: q, Giver: g})
	}
	return out
}

// SpellQuestPrefix starts the title of a spell-quest pick-up step.
const SpellQuestPrefix = "Pick up "

func spellQuestStep(c character.Character, qx *questie.Index, m MissedSpellQuest) Step {
	g := m.Giver
	title := fmt.Sprintf("%s%s from %s (%s): teaches %s, needed before Rank 2", SpellQuestPrefix, m.Quest.Name, g.Name, g.Zone, m.Spell)
	reason := fmt.Sprintf("you don't know %s: its first rank comes from this level %d quest, and the trainer won't teach any later rank without it; hand it in to the same NPC", m.Spell, m.Quest.Level)
	return Step{Kind: KindQuest, Title: title, Reason: reason, Zone: g.Zone, QuestID: m.Quest.ID, Location: g.String(), Name: m.Quest.Name,
		Level: m.Quest.Level, NPC: g.Name, Sources: []string{fmt.Sprintf("questie: QuestieDB %s quest %d", qx.DB, m.Quest.ID)}}
}

// IsSpellQuest reports whether the step is a spell-quest pick-up.
func IsSpellQuest(s Step) bool {
	return strings.HasPrefix(s.Title, SpellQuestPrefix) && strings.Contains(s.Title, ": teaches ")
}

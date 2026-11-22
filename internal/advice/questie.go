package advice

import (
	"fmt"
	"strings"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/items"
	"github.com/WoW-Companion/advisor/internal/questie"
)

// Class quest wording. A class quest is a pick-up limited to the player's class (or a few classes).
const (
	ClassQuestPrefix = "Class quest: pick up "
	ClassQuestNote   = "Class quests often teach a core ability."
	// MaxClassQuests caps how many class quests lead the list (the prompt budget).
	MaxClassQuests = 3
)

// IsClassQuest reports whether the step is a class quest pick-up.
func IsClassQuest(s Step) bool { return strings.HasPrefix(s.Title, ClassQuestPrefix) }

func isPickUp(s Step) bool { return strings.HasPrefix(s.Title, PickUpPrefix) || IsClassQuest(s) }

// MaxLocated is how many quests in the log get a location from Questie (the prompt budget).
const MaxLocated = 5

func whereOf(c character.Character) (questie.Where, bool) {
	p := c.LastKnownPosition()
	if p == nil {
		return questie.Where{}, false
	}
	return questie.Where{MapID: p.MapID, Zone: p.Zone, X: p.X, Y: p.Y}, true
}

func whoOf(c character.Character) questie.Who {
	w := questie.Who{Level: *c.Level, Below: AvailBelow, Above: AvailAbove,
		ClassID: classIDs[strings.ToUpper(c.Class)], RaceID: raceIDs[strings.ToLower(strings.ReplaceAll(c.Race, " ", ""))],
		Done: map[int]bool{}, InLog: map[int]bool{}}
	for _, id := range c.CompletedQuests {
		w.Done[id] = true
	}
	for _, q := range c.Quests {
		w.InLog[q.QuestID] = true
	}
	return w
}

// questieAvailable lists pick-ups from the player's QuestieDB; ok is false when it cannot say (no
// level or position), and the caller falls back to the dataset.
func questieAvailable(c character.Character, qx *questie.Index) (Available, bool) {
	at, ok := whereOf(c)
	if qx == nil || c.Level == nil || !ok {
		return Available{}, false
	}
	var out Available
	for _, o := range qx.Available(whoOf(c), at, items.Unobtainable) {
		q := o.Quest
		g := o.Giver
		reason := fmt.Sprintf("level %d quest, you are %d (band %d to %d); not in your log, prerequisites done", q.Level, *c.Level, *c.Level-AvailBelow, *c.Level+AvailAbove)
		if g.Dist >= 0 {
			reason += fmt.Sprintf("; giver is %.0f map units away", g.Dist)
		}
		title := fmt.Sprintf("%s%s (level %d) from %s", PickUpPrefix, q.Name, q.Level, g)
		if o.Class {
			title = fmt.Sprintf("%s%s from %s", ClassQuestPrefix, q.Name, g)
			reason += ". " + ClassQuestNote // the reward is never named: no local source gives it
		}
		out.Quests = append(out.Quests, Step{Kind: KindQuest, Title: title, Reason: reason,
			Zone: g.Zone, QuestID: q.ID, Location: g.String(), Name: q.Name, Level: q.Level, NPC: g.Name,
			Sources: []string{fmt.Sprintf("questie: QuestieDB %s quest %d", qx.DB, q.ID)}})
	}
	return out, true
}

// locate adds the nearest objective or turn-in place from Questie to the first MaxLocated quests
// in the log.
func locate(c character.Character, steps []Step, qx *questie.Index) {
	at, ok := whereOf(c)
	if qx == nil || !ok {
		return
	}
	n := 0
	for i := range steps {
		s := &steps[i]
		if s.QuestID == 0 || isPickUp(*s) || strings.HasPrefix(s.Title, AbandonPrefix) {
			continue
		}
		if n++; n > MaxLocated {
			return
		}
		p, objective, ok := qx.Locate(s.QuestID, s.Kind == KindTurnIn, at)
		if !ok {
			continue
		}
		verb := "turn in at "
		if objective {
			verb = "objective near "
		}
		s.Location = p.String()
		if !objective {
			s.NPC = p.Name
		}
		s.Reason += "; " + verb + s.Location
		s.Sources = append(s.Sources, fmt.Sprintf("questie: QuestieDB %s quest %d", qx.DB, s.QuestID))
	}
}

// WhoOf is the eligibility facts of c for the player's QuestieDB (the same band as the pick-up steps).
func WhoOf(c character.Character) questie.Who { return whoOf(c) }

package route

import (
	"cmp"
	"fmt"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/questie"
)

// Coverage counts how the objectives of the quests in the character's log are placed.
type Coverage struct {
	Quests     int // quests in the log
	NoData     int // of those, quests the Questie database does not know
	Questie    int // objectives placed from the Questie database
	Game       int // objectives placed from the game's own quest map point
	Approx     int // objectives placed approximately, at a subzone named in the quest text
	Unplaced   int // objectives with no position at all
	TriggerEnd int // of the placed ones, exploration objectives placed from Questie's triggerEnd
	Layout     string
	FlagLayout string // where the npcFlags bit names came from
	Available  bool   // false: no Questie database to read
}

// Total is the number of objectives counted.
func (c Coverage) Total() int { return c.Questie + c.Game + c.Approx + c.Unplaced }

// Line is the one-line summary.
func (c Coverage) Line() string {
	if !c.Available {
		return "quests: no readable Questie database, so objectives cannot be placed"
	}
	return fmt.Sprintf("quests: %d in the log, %d objectives: %d placed from Questie, %d from the game's quest map points, %d approximately, %d not placed, %d placed from triggerEnd (%d quests unknown to Questie; field layout %s; npcFlags %s)",
		c.Quests, c.Total(), c.Questie, c.Game, c.Approx, c.Unplaced, c.TriggerEnd, c.NoData, c.Layout, cmp.Or(c.FlagLayout, "unknown"))
}

// CoverageOf walks the quests in c's log the way the route does and counts their objectives.
func CoverageOf(c character.Character, qx *questie.Index, opt Options) Coverage {
	cov := Coverage{Quests: len(c.Quests)}
	if qx == nil {
		return cov
	}
	cov.Available, cov.Layout, cov.FlagLayout = true, qx.Layout, qx.FlagLayout
	inLog := map[int]bool{}
	for _, lq := range c.Quests {
		inLog[lq.QuestID] = true
		if qx.Quests[lq.QuestID] == nil {
			cov.NoData++
		}
	}
	src := &questieSource{qx: qx, opt: opt}
	for _, q := range src.quests(c, nil) {
		if !inLog[q.id] {
			continue
		}
		for _, sp := range q.objs {
			if sp.endObj && !sp.zoneOnly {
				cov.TriggerEnd++
			}
			switch {
			case sp.zoneOnly:
				cov.Unplaced++
			case sp.game:
				cov.Game++
			case sp.approx:
				cov.Approx++
			default:
				cov.Questie++
			}
		}
		cov.Unplaced += len(q.unplaced)
	}
	return cov
}

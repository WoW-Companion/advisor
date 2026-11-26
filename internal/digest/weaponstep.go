package digest

import (
	"fmt"
	"strings"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/gear"
	"github.com/WoW-Companion/advisor/internal/levelup"
	"github.com/WoW-Companion/advisor/internal/store"
)

// trainNeed is a listed upgrade that needs a weapon skill the character can learn.
type trainNeed struct {
	need        *gear.SkillNeed
	item, gain  string
	significant bool // worth a step of its own in the plan
}

// weaponTraining is one "Train <skill>" step for each distinct skill a significant upgrade needs, when a
// master of the player's faction is in the town they stand in or one flight from it.
func weaponTraining(c character.Character, ds store.DataStore, needs []trainNeed) []Line {
	if len(needs) == 0 || ds == nil {
		return nil
	}
	f := gear.LoadWeaponSkills(c, ds)
	var out []Line
	done := map[int64]bool{}
	for _, n := range needs {
		if !n.significant || done[n.need.SkillID] {
			continue
		}
		done[n.need.SkillID] = true
		for _, t := range f.MastersFor(n.need.SkillID) {
			if c.Level != nil && t.MinLevel > *c.Level {
				continue
			}
			where := reach(f.Pos, t.Master.City)
			if where == "" {
				continue
			}
			l := Line{Kind: KindTraining, Train: n.need.Skill, NPC: t.Master.Name, Zone: t.Master.City, X: t.Master.X, Y: t.Master.Y, HasXY: t.Master.X != 0 || t.Master.Y != 0}
			l.Text = clip(fmt.Sprintf("Train %s at %s in %s (%s)", n.need.Skill, t.Master.Name, t.Master.City, gear.CostText(t.Cost)))
			l.Details = finishDetails([]string{
				"Weapon Master " + t.Master.Name + ", " + t.Master.Where(),
				"Unlocks " + n.item + ", an upgrade in your bags or from a quest",
				where,
			})
			q := "Where do I train " + n.need.Skill + "?"
			l.Asks, l.Ref = asksOf(q, q), fmt.Sprintf("weapon-skill:%d", n.need.SkillID)
			l.Why = firstWhy("Unlocks "+n.item, "Unlocks an upgrade")
			out = append(out, l)
			break
		}
	}
	return out
}

// reach says how the town is reached: "You are there", "One flight from <point>"; "" when out of reach.
func reach(zone, city string) string {
	if zone == "" {
		return ""
	}
	if strings.EqualFold(zone, city) {
		return "You are in " + city + " now"
	}
	if p := levelup.FlightFrom(city, zone); p != "" {
		return "One flight from " + p
	}
	return ""
}

// withWeaponTraining puts the training steps after the leading ready turn-ins, talent and bag lines:
// the plan's turn-ins come first, the training before pick-ups and objectives.
func withWeaponTraining(lines, train []Line) []Line {
	if len(train) == 0 {
		return lines
	}
	i := 0
	for i < len(lines) && (lines[i].Kind == KindTurnIn || lines[i].Kind == KindTalent || lines[i].Kind == KindNote) {
		i++
	}
	out := append([]Line(nil), lines[:i]...)
	out = append(out, train...)
	return append(out, lines[i:]...)
}

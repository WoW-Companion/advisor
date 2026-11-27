package professions

import (
	"strings"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/store"
)

// RouteHint is what one of the character's professions asks of the route: the facts the skill-up path already
// holds, so a route line and the profession answer never disagree.
type RouteHint struct {
	Profession string
	// TrainTier is the rank the character can train now (level and skill allow it, a trainer teaches it); empty
	// when none is due. TrainRecipe is a recipe the next band needs that a trainer teaches, when no rank is due.
	TrainTier   string
	TrainRecipe string
	// Next is the next crafting step of the path: the band a craft batch or a purchase would serve. Nil for a
	// gathering profession or when the path has no step.
	Next *NextStep
	// Gather is set for Herbalism, Mining and Skinning when the path's next stretch is gathered: the nodes it names.
	Gather *GatherHint
}

// NextStep is the next skill-up band of a path: Crafts of Recipe to reach skill To, with its materials.
type NextStep struct {
	Recipe string
	Crafts int
	From   int
	To     int
	Needs  []Need
}

// GatherHint names the nodes a gathering path stretch opens, the skill it climbs to and the character's skill.
type GatherHint struct {
	Skill int
	To    int
	Nodes []GatherNode
}

// RouteHints computes a hint for each profession the character has and the dataset can plan. ds may be nil.
func RouteHints(c character.Character, ds store.DataStore) []RouteHint {
	if ds == nil {
		return nil
	}
	var out []RouteHint
	for _, sk := range c.Professions {
		if sk.Rank == nil {
			continue
		}
		h := RouteHint{Profession: sk.Name, TrainTier: trainDue(c, ds, sk)}
		g, gathers := gatheringProfession(sk.Name)
		switch {
		case gathers && g == "Fishing":
		case gathers:
			h.Gather = gatherHint(c, ds, g, *sk.Rank)
		default:
			pa := ComputePath(c, ds, sk.Name, 0, 1)
			var first *Band
			if len(pa.Bands) > 0 {
				first = &pa.Bands[0]
			}
			h.Next = nextStep(c, ds, sk, first)
			if h.TrainTier == "" && first != nil && first.Gap == "" && len(c.KnownSpells) > 0 &&
				first.Best.Learn != "" && first.Best.LearnInfo.Via == "trainer" {
				h.TrainRecipe = first.Best.Recipe
			}
		}
		if h.TrainTier != "" || h.TrainRecipe != "" || h.Next != nil || h.Gather != nil {
			out = append(out, h)
		}
	}
	return out
}

// gatherHint names the nodes a gathering path stretch opens at the skill: the stretch runs to the lower of the
// level's rank cap and the current rank's cap, as a band of the path does, and the nodes are the highest the
// dataset lists up to there, those the character can gather now. Nil when the dataset lists none.
func gatherHint(c character.Character, ds store.DataStore, prof string, skill int) *GatherHint {
	notes := LoadNotes(ds)
	to := notes.TierAt(skill).SkillCap
	if c.Level != nil {
		if allowed, _ := notes.CapAt(*c.Level); allowed < to {
			to = allowed
		}
	}
	if to <= skill {
		return nil
	}
	nodes, _ := gatherNodes(ds, prof, to)
	if len(nodes) > 3 {
		nodes = nodes[:3]
	}
	gh := &GatherHint{Skill: skill, To: to}
	for _, n := range nodes {
		if n.Skill <= skill {
			gh.Nodes = append(gh.Nodes, *n)
		}
	}
	if len(gh.Nodes) == 0 {
		return nil
	}
	return gh
}

// trainDue is the rank the character can train now: the one after the rank cap they hold, when their level and
// skill reach its gate and a trainer teaches it (not a book or a quest).
func trainDue(c character.Character, ds store.DataStore, sk character.Skill) string {
	if sk.Max == nil || sk.Rank == nil || c.Level == nil {
		return ""
	}
	notes := LoadNotes(ds)
	allowed, _ := notes.CapAt(*c.Level)
	if *sk.Max >= allowed {
		return ""
	}
	t, ok := notes.NextTier(*sk.Max)
	if !ok {
		return ""
	}
	g, found := tierGate(ds, sk.Name, t.Name, c.Faction)
	level := g.Level
	if level == 0 {
		level = t.MinLevel
	}
	if *c.Level < level || found && g.Skill > 0 && *sk.Rank < g.Skill {
		return ""
	}
	if how, _ := tierAccess(ds, sk.Name, t.Name, c.Faction); how != "" {
		return ""
	}
	return t.Name
}

// nextStep is the step NextSkillUp words: the dataset's route step covering the skill, else the path's first band.
func nextStep(c character.Character, ds store.DataStore, sk character.Skill, first *Band) *NextStep {
	have := *sk.Rank
	if r, ok := LoadRoute(ds, sk.Name); ok {
		if i := r.Covering(have); i >= 0 && !r.Steps[i].Gather {
			s := r.Steps[i]
			stock := stockOf(c)
			n := &NextStep{Recipe: s.Recipe, Crafts: s.craftsLeft(have), From: have, To: s.To}
			for _, m := range s.Materials {
				need := m.Count * n.Crafts
				name := itemName(ds, m.ItemID)
				if name == "" {
					continue
				}
				h := stock[m.ItemID].Total()
				n.Needs = append(n.Needs, Need{ID: m.ItemID, Item: name, Total: need, Have: min(h, need), Missing: max(0, need-h)})
			}
			if len(n.Needs) < len(s.Materials) {
				n.Needs = nil // a material the dataset does not name: the step's materials are not fully known
			}
			return n
		}
	}
	if first == nil || first.Gap != "" || first.Best.Recipe == "" {
		return nil
	}
	return &NextStep{Recipe: first.Best.Recipe, Crafts: first.Best.Crafts, From: first.From, To: first.To, Needs: first.Best.Needs}
}

// Held says every material of the step is already in bags or bank, and the step has materials.
func (n *NextStep) Held() bool {
	if n == nil || len(n.Needs) == 0 || n.Crafts <= 0 {
		return false
	}
	for _, x := range n.Needs {
		if x.Missing > 0 {
			return false
		}
	}
	return true
}

// ProfessionStems are the words a trainer's subName uses for a profession ("Master Tailor", "Herbalist Trainer").
func ProfessionStems(profession string) []string {
	p := strings.ToLower(strings.TrimSpace(profession))
	switch p {
	case "tailoring":
		return []string{"tailor"}
	case "leatherworking":
		return []string{"leatherwork"}
	case "enchanting":
		return []string{"enchant"}
	case "engineering":
		return []string{"engineer"}
	case "herbalism":
		return []string{"herbalis", "herbalism"}
	case "mining":
		return []string{"mining", "miner"}
	case "skinning":
		return []string{"skinning", "skinner"}
	case "cooking":
		return []string{"cook"}
	case "blacksmithing":
		return []string{"blacksmith"}
	case "alchemy":
		return []string{"alchem"}
	case "fishing":
		return []string{"fishing", "fisherman"}
	}
	return []string{p}
}

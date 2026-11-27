package route

import (
	"cmp"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/levelup"
	"github.com/WoW-Companion/advisor/internal/questie"
)

// Errand kinds (Plan.lua 1.7).
const (
	ErrandTrain      = "train"
	ErrandBank       = "bank"
	ErrandClass      = "class"
	ErrandProfession = "profession"
	ErrandTurnIn     = "turnin"
)

// Errand marks one step of the run as a city errand, so the addon can label it. The stop itself is an
// ordinary numbered step (N).
type Errand struct {
	N     int     `json:"n"`
	Kind  string  `json:"kind"`
	Label string  `json:"label"`
	Map   int     `json:"map"`
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	Why   string  `json:"why"`
	// Attach is the step a "while you're here" errand (restock, repair, selljunk, mail) rides on; N is 0 then.
	// NPC names who or what to use.
	Attach int    `json:"attach,omitempty"`
	NPC    string `json:"npc,omitempty"`
}

// KindErrand is the kind of a stop that is only an errand: a trainer, the bank.
const KindErrand = "errand"

// capitals are the capitals and major capitals where a visit gets errands, lower case, by the side they serve:
// "H" Horde, "A" Alliance, "" both.
var capitals = map[string]string{
	"undercity": "H", "orgrimmar": "H", "thunder bluff": "H", "silvermoon city": "H",
	"stormwind city": "A", "ironforge": "A", "darnassus": "A", "the exodar": "A",
	"shattrath city": "", "dalaran": "",
}

// isHub is true for a capital the character can use: a city of the other side has no trainer or bank for them.
// A character whose side is not known gets every capital.
func isHub(c character.Character, zone string) bool {
	side, ok := capitals[strings.ToLower(strings.TrimSpace(zone))]
	if !ok {
		return false
	}
	mine := playerSide(c)
	return side == "" || mine == "" || side == mine
}

// addErrands adds the city errands to the run when the player's zone or the run's zone is a capital or a
// major hub: the stops that are only errands (a trainer, the bank) join the steps, and every errand,
// those and the ordinary stops that are one (a class quest, a ready turn-in), is listed in p.Errands.
func addErrands(p *Plan, c character.Character, src source, opt Options, in sharedInput) {
	zone := p.Zone
	if pos := c.LastKnownPosition(); pos != nil && isHub(c, pos.Zone) {
		zone = pos.Zone
	}
	if !isHub(c, zone) {
		return
	}
	cur := in.cur
	done := map[int]bool{}
	for _, q := range in.listed {
		done[q.id] = q.done
	}
	add := func(kind, label, why string, m int, x, y float64) {
		n := len(p.Steps) + 1
		p.Steps = append(p.Steps, Stop{N: n, Kind: KindErrand, Label: label, Map: m, X: x, Y: y, Place: zone, Errand: kind, ZoneOnly: x == NoPos})
		p.Errands = append(p.Errands, Errand{N: n, Kind: kind, Label: label, Map: m, X: x, Y: y, Why: why})
	}
	// Ordinary stops that are errands keep their number.
	for _, s := range p.Steps {
		switch {
		case s.Kind == KindPickup && s.Priority:
			p.Errands = append(p.Errands, Errand{N: s.N, Kind: ErrandClass, Label: s.Label, Map: s.Map, X: s.X, Y: s.Y, Why: "a class quest is offered here"})
		case s.Kind == KindTurnIn && allDone(s, done):
			p.Errands = append(p.Errands, Errand{N: s.N, Kind: ErrandTurnIn, Label: s.Label, Map: s.Map, X: s.X, Y: s.Y, Why: "ready to turn in here"})
		}
	}
	qs, _ := src.(*questieSource)
	if n := trainableCount(c, opt); n > 0 {
		m, x, y, label := cur, float64(NoPos), float64(NoPos), "Train now at your class trainer"
		placed := false
		if qs != nil {
			if pl, ok := levelup.TrainAt(c, qs.qx); ok && pl.Trainer != nil && pl.Trainer.X > 0 && (pl.Here || strings.EqualFold(pl.City, zone)) {
				m, x, y, label, placed = cmp.Or(pl.Trainer.Map, cur), pl.Trainer.X, pl.Trainer.Y, "Train now with "+pl.Trainer.Name, true
			}
		}
		if !placed {
			if area := trainerArea(zone, c.Class); area != "" {
				label += " (" + area + ")"
			}
		}
		add(ErrandTrain, label, trainWhy(n), m, x, y)
	}
	if !c.BankCaptured {
		m, x, y, label := cur, float64(NoPos), float64(NoPos), "Open your bank once"
		if qs != nil {
			if name, bm, bx, by, ok := qs.bankerIn(cur, zone, playerSide(c), p.Start); ok {
				m, x, y, label = cmp.Or(bm, cur), bx, by, "Open your bank once, at "+name
			}
		}
		add(ErrandBank, label, "open your bank once so the advisor can see it", m, x, y)
	}
	sort.SliceStable(p.Errands, func(i, j int) bool { return p.Errands[i].N < p.Errands[j].N })
	for i := range p.Steps {
		for _, e := range p.Errands {
			if e.N == p.Steps[i].N && p.Steps[i].Errand == "" {
				p.Steps[i].Errand = e.Kind
			}
		}
	}
}

func allDone(s Stop, done map[int]bool) bool {
	ids := s.QuestIDs
	if len(ids) == 0 {
		ids = []int{s.QuestID}
	}
	for _, id := range ids {
		if !done[id] {
			return false
		}
	}
	return len(ids) > 0
}

func trainWhy(n int) string {
	if n == 1 {
		return "1 new spell or rank to train at this level"
	}
	return strconv.Itoa(n) + " new spells or ranks to train at this level"
}

// trainableCount is how many spells or ranks the character can train now.
func trainableCount(c character.Character, opt Options) int {
	if opt.Trainable != nil {
		return opt.Trainable(c)
	}
	if opt.Data == nil || c.Level == nil {
		return 0
	}
	n := 0
	for _, x := range levelup.Spells(c, opt.Data, 1, *c.Level) {
		if !x.Blocked {
			n++
		}
	}
	return n
}

// bankerFlag is the banker bit of an NPC's npcFlags.
const bankerFlag = 0x20000

// bankerIn is a banker standing in the city (map m, or the zone's name), the one nearest the player's
// position when it is known, else the lowest id. ok is false when Questie holds none there.
func (s *questieSource) bankerIn(m int, zone, side string, from *Point) (name string, bm int, x, y float64, ok bool) {
	ids := make([]int, 0, 16)
	for id, e := range s.qx.Npcs {
		if e.Flags&bankerFlag != 0 && len(e.Spawns) > 0 {
			ids = append(ids, id)
		}
	}
	sort.Ints(ids)
	best := -1.0
	for _, id := range ids {
		e := s.qx.Npcs[id]
		if e.Faction != "" && !questie.FactionAdmits(e.Faction, side) {
			continue
		}
		for _, sp := range e.Spawns {
			a := s.qx.Areas[sp.Area]
			if a.UIMap != m && !strings.EqualFold(a.Zone, zone) {
				continue
			}
			d := 0.0
			if from != nil && from.Map == a.UIMap {
				d = math.Hypot(sp.X-from.X, sp.Y-from.Y)
			}
			if best < 0 || d < best {
				best, name, bm, x, y, ok = d, e.Name, a.UIMap, sp.X, sp.Y, true
			}
		}
	}
	return name, bm, x, y, ok
}

// classQuarters is where each class's trainers stand in a capital: the named area, from public game facts.
var classQuarters = map[string]map[string]string{
	"undercity": {"ROGUE": "Rogues' Quarter", "WARRIOR": "War Quarter", "MAGE": "Magic Quarter", "PRIEST": "Magic Quarter"},
	"orgrimmar": {"ROGUE": "Cleft of Shadow", "WARRIOR": "Valley of Honor"},
}

// trainerArea names the part of a city the class trainers are in, "" when it is not known.
func trainerArea(zone, class string) string {
	return classQuarters[strings.ToLower(strings.TrimSpace(zone))][strings.ToUpper(strings.TrimSpace(class))]
}

package route

import (
	"sort"
	"strconv"
	"strings"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/levelup"
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
}

// KindErrand is the kind of a stop that is only an errand: a trainer, the bank.
const KindErrand = "errand"

// capitals are the capitals and major capitals where a visit gets errands, lower case.
var capitals = map[string]bool{
	"undercity": true, "orgrimmar": true, "thunder bluff": true, "silvermoon city": true,
	"stormwind city": true, "ironforge": true, "darnassus": true, "the exodar": true,
	"shattrath city": true, "dalaran": true,
}

func isHub(zone string) bool { return capitals[strings.ToLower(strings.TrimSpace(zone))] }

// addErrands adds the city errands to the run when the player's zone or the run's zone is a capital or a
// major hub: the stops that are only errands (a trainer, the bank) join the steps, and every errand,
// those and the ordinary stops that are one (a class quest, a ready turn-in), is listed in p.Errands.
func addErrands(p *Plan, c character.Character, src source, opt Options, in sharedInput) {
	zone := p.Zone
	if pos := c.LastKnownPosition(); pos != nil && isHub(pos.Zone) {
		zone = pos.Zone
	}
	if !isHub(zone) {
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
	if n := trainableCount(c, opt); n > 0 {
		m, x, y, label := cur, float64(NoPos), float64(NoPos), "Train now at your class trainer"
		if qs, ok := src.(*questieSource); ok {
			if pl, ok := levelup.TrainAt(c, qs.qx); ok && pl.Here && pl.Trainer != nil && pl.Trainer.Map != 0 {
				m, x, y, label = pl.Trainer.Map, pl.Trainer.X, pl.Trainer.Y, "Train now with "+pl.Trainer.Name
			}
		}
		add(ErrandTrain, label, trainWhy(n), m, x, y)
	}
	if !c.BankCaptured {
		add(ErrandBank, "Open your bank once", "open your bank once so the advisor can see it", cur, NoPos, NoPos)
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

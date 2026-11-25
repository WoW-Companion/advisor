package ask

import (
	"strconv"
	"strings"

	"github.com/WoW-Companion/advisor/internal/route"
	"github.com/WoW-Companion/advisor/internal/session"
)

// sessionFacts is the grounded block for "I've got 30 minutes" and quest-triage questions: the
// session plan or the rated quest log, built from the route and the player's QuestieDB. ok is false
// when there is no route data to build from.
func sessionFacts(cc loaded, q Question, cl Class) (string, bool) {
	if cc.qx == nil || cc.ds == nil || cc.c.Level == nil {
		return "", false
	}
	plan := route.BuildWith(cc.c, cc.qx, route.OptionsFor(cc.sv, cc.ds))
	in := session.Live(cc.c, cc.ds, cc.qx, plan, session.LoadXP(cc.sv))
	if m, ok := strings.CutPrefix(cl.Subject, "min:"); ok {
		n, _ := strconv.Atoi(m)
		return session.Plan(in, n).Facts(), true
	}
	name := strings.TrimPrefix(cl.Subject, "quest:")
	if cl.Subject == "abandon" {
		name = ""
	}
	return in.TriageFacts(name), true
}

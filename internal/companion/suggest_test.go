package companion

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/WoW-Companion/advisor/internal/ask"
	"github.com/WoW-Companion/advisor/internal/digest"
	"github.com/WoW-Companion/advisor/internal/route"
)

func entry(group, kind, title, ref string, class bool) PlanEntry {
	return PlanEntry{Group: group, Kind: kind, Title: title, Ref: ref, Class: class, Why: "because", Asks: []string{"What about " + title + "?"}}
}

func ids(ss []Suggestion) string {
	var out []string
	for _, s := range ss {
		out = append(out, s.ID+":"+s.Action)
	}
	return strings.Join(out, " ")
}

func TestSuggestRankingAndCap(t *testing.T) {
	entries := []PlanEntry{
		entry("next", digest.KindTalent, "Spend", "talents", false),
		entry("next", digest.KindPickup, "Plain quest", "quest:1", false),
		entry("next", digest.KindTurnIn, "Done quest", "quest:2", false),
		entry("next", digest.KindPickup, "Class quest", "quest:3", true),
	}
	for _, tc := range []struct {
		name string
		in   suggestInput
		want string
	}{
		{"everything: capped at three, style first", suggestInput{Entries: entries, PlanStale: true, HasPlanSrc: true},
			"play-style:play_style entry-quest:2:ask entry-quest:3:ask"},
		{"style set: quests, then talent", suggestInput{HasStyle: true, Entries: entries, PlanStale: true, HasPlanSrc: true},
			"entry-quest:2:ask entry-quest:3:ask entry-talents:ask"},
		{"talent then stale plan", suggestInput{HasStyle: true, Entries: entries[:1], PlanStale: true, HasPlanSrc: true},
			"entry-talents:ask refresh-plan:refresh_plan"},
		{"stale plan with no plan to refresh", suggestInput{HasStyle: true, PlanStale: true}, ""},
		{"a plain pick-up is not suggested", suggestInput{HasStyle: true, Entries: entries[1:2]}, ""},
	} {
		if got := ids(suggest(tc.in)); got != tc.want {
			t.Errorf("%s: %q, want %q", tc.name, got, tc.want)
		}
	}
	a, b := suggest(suggestInput{Entries: entries}), suggest(suggestInput{Entries: entries})
	if ids(a) != ids(b) || a[1].Question == "" || a[1].Ref != "quest:2" || a[1].Why != "because" {
		t.Errorf("not deterministic or incomplete: %+v", a)
	}
}

func TestRefreshPlanRewritesBothFiles(t *testing.T) {
	s, _, out := digestEnv(t, true, false) // the automatic digest is off
	plan := digest.SiblingPath(out, route.File)
	write(t, out, "stale")
	write(t, plan, "stale")
	res, err := s.refreshPlan()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{out, plan} {
		if b, err := os.ReadFile(p); err != nil || strings.Contains(string(b), "stale") || len(b) == 0 {
			t.Errorf("%s not rewritten: %v %q", filepath.Base(p), err, b)
		}
	}
	if !strings.HasPrefix(res.Message, "Plan updated from your ") || !strings.Contains(res.Message, " save: ") || strings.Contains(res.Message, "map units") {
		t.Errorf("message %q", res.Message)
	}
	if got := refreshMessage(1800000000, 2); !strings.HasSuffix(got, "save: 2 new steps") {
		t.Errorf("message %q", got)
	}
}

// asksEnv is a companion whose game asks are one fresh and one older than 12 hours.
func asksEnv(t *testing.T, ledger, command string) *Server {
	t.Helper()
	dir := t.TempDir()
	write(t, filepath.Join(dir, "WoWCompanion.lua"), `WoWCompanionDB = { asks = {
  { id = "9-1", asked_at = 99000, character = "R:New", text = "Fresh question?" },
  { id = "9-2", asked_at = 1000, character = "R:New", text = "Old question?" },
} }
`)
	write(t, filepath.Join(dir, "ForeverScribe.lua"), capture)
	return New(Config{
		ForeverScribe: filepath.Join(dir, "ForeverScribe.lua"), Ledger: ledger,
		Now: func() time.Time { return time.Unix(100000, 0) },
		Ask: ask.Config{Command: []string{command}, Timeout: 5 * time.Second, SavedVariables: filepath.Join(dir, "WoWCompanion.lua")},
	})
}

func statuses(s *Server) map[string]string {
	out := map[string]string{}
	for _, e := range s.snapshot().History {
		out[e.Question] = e.Status
	}
	return out
}

// A launch answers a fresh ask once; a second launch (a new companion, the ask still in the game's
// file) shows it answered and never asks the agent again; an ask over 12 hours old is only pending.
func TestAnsweredAsksLedger(t *testing.T) {
	t.Setenv("FAKE_AGENT", "text")
	ledger := filepath.Join(t.TempDir(), "answered.json")
	first := asksEnv(t, ledger, os.Args[0])
	first.answerGame(context.Background())
	first.showGame()
	if got := statuses(first); got["Fresh question?"] != "done" || got["Old question?"] != "pending" || len(first.gameDone) != 1 {
		t.Fatalf("first launch: %v, answered %d", got, len(first.gameDone))
	}
	// The agent is gone: any answer attempt would end as an error entry.
	second := asksEnv(t, ledger, filepath.Join(t.TempDir(), "no-such-agent"))
	second.answerGame(context.Background())
	second.showGame()
	if got := statuses(second); got["Fresh question?"] != "done" || got["Old question?"] != "pending" || len(second.gameDone) != 0 {
		t.Errorf("second launch: %v, answered %d", got, len(second.gameDone))
	}
	for _, e := range second.snapshot().History {
		if e.Question == "Fresh question?" && !strings.Contains(e.Answer, "Plain answer") {
			t.Errorf("the earlier answer is not shown: %+v", e)
		}
	}
}

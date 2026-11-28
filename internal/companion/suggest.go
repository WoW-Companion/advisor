package companion

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/WoW-Companion/advisor/internal/digest"
	"github.com/WoW-Companion/advisor/internal/server"
)

// MaxSuggestions is the most suggested actions a character carries.
const MaxSuggestions = 3

// Suggestion actions.
const (
	ActionAsk         = "ask"          // send Question with Ref to /api/ask
	ActionPlayStyle   = "play_style"   // open the play-style choice
	ActionRefreshPlan = "refresh_plan" // POST /api/refresh-plan
	ActionOpenEntry   = "open_entry"   // show the entry whose ref is Ref
)

// Suggestion is one suggested next action for the overlay. ID is stable for the same advice, so the
// overlay can dismiss it for the day. Question is set for an "ask"; Ref for an "ask" and an "open_entry".
type Suggestion struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Why      string `json:"why,omitempty"`
	Action   string `json:"action"`
	Question string `json:"question,omitempty"`
	Ref      string `json:"ref,omitempty"`
}

// suggestInput is everything the ranking looks at.
type suggestInput struct {
	HasStyle   bool
	Entries    []PlanEntry
	PlanStale  bool // the plan is older than the newest capture or dataset
	HasPlanSrc bool // there is a plan to refresh
}

// suggest ranks the character's suggested actions: no play style, then a ready turn-in or class
// quest, then an unspent talent point, then an out-of-date plan. At most MaxSuggestions; the same
// input always gives the same list, and no AI is asked.
func suggest(in suggestInput) []Suggestion {
	var out []Suggestion
	if !in.HasStyle {
		out = append(out, Suggestion{ID: "play-style", Label: "Choose how you play", Why: "So your picks and advice fit your play style", Action: ActionPlayStyle})
	}
	for _, e := range in.Entries {
		var label string
		switch {
		case e.Group == "next" && e.Kind == digest.KindTurnIn && !e.Trip: // a trip is not a quick turn-in
			label = "Turn in " + e.Title
		case e.Group == "next" && e.Kind == digest.KindPickup && e.Class:
			label = "Start the class quest " + e.Title
		default:
			continue
		}
		out = append(out, entrySuggestion(e, label))
	}
	for _, e := range in.Entries {
		if e.Group == "next" && e.Kind == digest.KindTalent {
			out = append(out, entrySuggestion(e, "Spend your talent point"))
			break
		}
	}
	if in.PlanStale && in.HasPlanSrc {
		out = append(out, Suggestion{ID: "refresh-plan", Label: "Update your plan", Why: "It is older than your latest save", Action: ActionRefreshPlan})
	}
	return out[:min(len(out), MaxSuggestions)]
}

func entrySuggestion(e PlanEntry, label string) Suggestion {
	s := Suggestion{ID: "entry-" + e.Ref, Label: label, Why: e.Why, Action: ActionOpenEntry, Ref: e.Ref}
	if len(e.Asks) > 0 {
		s.Action, s.Question = ActionAsk, e.Asks[0]
	}
	return s
}

// planStale reports whether the plan file is older than the newest capture or the dataset.
func planStale(planPath string, capturedAt int64, datasetDir string) bool {
	st, err := os.Stat(planPath)
	if err != nil {
		return true
	}
	at := st.ModTime().Unix()
	if capturedAt > at {
		return true
	}
	if datasetDir != "" {
		if m, err := os.Stat(datasetDir); err == nil && m.ModTime().Unix() > at {
			return true
		}
	}
	return false
}

func (s *Server) suggestions(w http.ResponseWriter, r *http.Request) {
	char := r.URL.Query().Get("character")
	s.mu.Lock()
	var out []Suggestion
	for _, c := range s.chars {
		if char == "" && c.Key == s.defaultKey() || c.Key == char || c.Name == char {
			out = c.Suggestions
			break
		}
	}
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"suggestions": append([]Suggestion{}, out...)})
}

// defaultKey is the most recently captured character's key. The caller holds s.mu.
func (s *Server) defaultKey() string {
	var best int64 = -1
	key := ""
	for _, c := range s.chars {
		if c.CapturedAt > best {
			best, key = c.CapturedAt, c.Key
		}
	}
	return key
}

// RefreshResult is what /api/refresh-plan answers.
type RefreshResult struct {
	Message  string   `json:"message"`
	NewSteps int      `json:"new_steps"`
	Files    []string `json:"files,omitempty"`
}

// refreshMessage is the one plain sentence about what changed.
func refreshMessage(savedAt int64, added int) string {
	from := "your latest save"
	if savedAt > 0 {
		from = "your " + time.Unix(savedAt, 0).Format("3:04 PM") + " save"
	}
	switch added {
	case 0:
		return "Plan updated from " + from + ": no new steps"
	case 1:
		return "Plan updated from " + from + ": 1 new step"
	}
	return fmt.Sprintf("Plan updated from %s: %d new steps", from, added)
}

// refreshPlan rewrites Digest.lua and Plan.lua from the newest capture at once, even when the
// automatic digest is off.
func (s *Server) refreshPlan() (RefreshResult, error) {
	s.refresh.Lock()
	defer s.refresh.Unlock()
	s.mu.Lock()
	before := s.planKeys
	s.mu.Unlock()
	out, err := server.WriteDigest(s.withPending(server.Config{
		SavedVariables: s.cfg.ForeverScribe, Dataset: s.datasetDir(), DigestPath: s.cfg.DigestPath,
	}), "")
	if err != nil {
		return RefreshResult{}, err
	}
	s.reload()
	s.mu.Lock()
	after, saved := s.planKeys, s.captured
	s.digest.UpdatedAt, s.digest.Status, s.digest.Error = time.Now().Unix(), DigestOn, ""
	s.mu.Unlock()
	added := 0
	for char, keys := range after {
		for k := range keys {
			if !before[char][k] {
				added++
			}
		}
	}
	s.publish("game", s.snapshot())
	res := RefreshResult{Message: refreshMessage(saved, added), NewSteps: added}
	res.Files = []string{out.Path, out.PlanPath}
	return res, nil
}

func (s *Server) refreshPlanHandler(w http.ResponseWriter, r *http.Request) {
	res, err := s.refreshPlan()
	if err != nil {
		http.Error(w, "Could not update the plan: "+err.Error(), http.StatusConflict)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

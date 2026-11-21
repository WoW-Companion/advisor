package companion

import (
	"encoding/json"
	"net/http"

	"github.com/WoW-Companion/advisor/internal/ask"
)

// Settings are the answer settings the overlay changes while it runs.
type Settings struct {
	Length    string `json:"length"`               // brief, standard or detailed
	Model     string `json:"model"`                // efficient, balanced, best or custom
	Custom    string `json:"custom_model"`         // the model name for "custom"
	Usage     bool   `json:"usage"`                // show the token usage line under answers
	ModelNote string `json:"model_note,omitempty"` // what the tiers mean for the current AI client
}

func (s *Server) settings() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.settingsLocked()
}

func (s *Server) settingsLocked() Settings {
	o, u := s.opts, s.usageOn
	return Settings{Length: o.Length, Model: o.Tier, Custom: o.Custom, Usage: u, ModelNote: ask.TierNote(ask.ClientName(s.cfg.Ask.Command))}
}

// askCfg is the agent config with the current answer options.
func (s *Server) askCfg() ask.Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.cfg.Ask
	c.Options = s.opts
	return c
}

// setSettings takes {"length","model","custom_model","usage"}; unknown values fall back to the defaults.
func (s *Server) setSettings(w http.ResponseWriter, r *http.Request) {
	var req Settings
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, MaxBody)).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	s.opts = ask.Options{Length: req.Length, Tier: req.Model, Custom: req.Custom}.Normalize()
	s.usageOn = req.Usage
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.settings())
}

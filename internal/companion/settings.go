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
	Reasoning string `json:"reasoning"`            // low, medium or high
	Usage     bool   `json:"usage"`                // show the token usage line under answers
	ModelNote string `json:"model_note,omitempty"` // what the models mean for the current AI client
	// Client and Choices tell the overlay what to offer for the current AI client; the overlay
	// remembers each client's last choice and sends it back. Both are ignored in a request.
	Client  string      `json:"client"`
	Choices ask.Choices `json:"choices"`
}

func (s *Server) settings() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.settingsLocked()
}

func (s *Server) settingsLocked() Settings {
	o, u := s.opts, s.usageOn
	c := ask.ClientName(s.cfg.Ask.Command)
	ch := ask.ChoicesFor(c)
	return Settings{Length: o.Length, Model: o.Tier, Custom: o.Custom, Reasoning: o.Reasoning, Usage: u, ModelNote: ch.ModelNote, Client: c, Choices: ch}
}

// askCfg is the agent config with the current answer options.
func (s *Server) askCfg() ask.Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.cfg.Ask
	c.Options = s.opts
	c.Dataset = s.datasetDir()
	return c
}

// setSettings takes {"length","model","custom_model","reasoning","usage"}; unknown values fall back to the defaults.
func (s *Server) setSettings(w http.ResponseWriter, r *http.Request) {
	var req Settings
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, MaxBody)).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	s.opts = ask.Options{Length: req.Length, Tier: req.Model, Custom: req.Custom, Reasoning: req.Reasoning}.Normalize()
	s.usageOn = req.Usage
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.settings())
}

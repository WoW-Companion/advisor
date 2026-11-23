package companion

import (
	"cmp"
	"encoding/json"
	"net"
	"net/http"
	"slices"

	"github.com/WoW-Companion/advisor/internal/ask"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/profile"
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

func (s *Server) profilesPath() string {
	if s.cfg.Profiles != "" {
		return s.cfg.Profiles
	}
	return profile.Path()
}

// setProfile takes {"character","role","focus","spec"}: the overlay's Play style row. It saves the
// character's play style locally, keeps its guide and plan, and re-reads the next steps, which the
// saved plan shapes. An unknown character or a role or focus that is not on the lists is a 400.
func (s *Server) setProfile(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Character string `json:"character"`
		character.Profile
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, MaxBody)).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	known := slices.ContainsFunc(s.chars, func(c CharInfo) bool { return c.Key == req.Character })
	s.mu.Unlock()
	if !known {
		http.Error(w, "unknown character", http.StatusBadRequest)
		return
	}
	_, err := profile.Update(s.profilesPath(), req.Character, func(e *profile.Entry) error {
		if e.Profile != nil {
			req.GuideURL = cmp.Or(req.GuideURL, e.Profile.GuideURL)
		}
		p, err := profile.Clean(req.Profile)
		e.Profile = &p
		return err
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.reload()
	// A question that was waiting for this play style is asked again, once.
	s.mu.Lock()
	again, waiting := s.pending[req.Character]
	delete(s.pending, req.Character)
	s.mu.Unlock()
	if waiting {
		client, _, _ := net.SplitHostPort(r.RemoteAddr)
		if id, ok := s.startAsk(client, again); ok {
			s.publish("reask", map[string]string{"id": id, "character": again.Character, "question": again.Text})
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.snapshot().Characters)
}

package companion

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/WoW-Companion/advisor/internal/ask"
	"github.com/WoW-Companion/advisor/internal/provider"
)

// The local API for choosing how asks are answered. A key is write-only: it is never returned, only
// whether one is set.

func (s *Server) providersPath() string {
	if s.cfg.Providers != "" {
		return s.cfg.Providers
	}
	return provider.SettingsPath()
}

func (s *Server) keyStore() provider.KeyStore {
	if s.cfg.Keys != nil {
		return s.cfg.Keys
	}
	return provider.DefaultKeyStore()
}

func (s *Server) providerSettings() provider.Settings {
	st, _ := provider.Load(s.providersPath())
	return st
}

func (s *Server) hasKey(id string) bool {
	k, err := s.keyStore().Get(id)
	return err == nil && k != ""
}

// clientView is one configured client as the overlay sees it.
type clientView struct {
	provider.ClientRef
	HasKey bool `json:"has_key"`
}

type presetView struct {
	provider.Preset
	HasKey bool `json:"has_key"`
}

func (s *Server) providerState() map[string]any {
	st := s.providerSettings()
	presets := make([]presetView, 0, len(provider.Presets))
	for _, p := range provider.Presets {
		presets = append(presets, presetView{p, s.hasKey(p.ID)})
	}
	clients := make([]clientView, 0, len(st.Clients))
	for _, c := range st.Clients {
		clients = append(clients, clientView{c, c.Kind == provider.ClientAPI && s.hasKey(c.Provider)})
	}
	return map[string]any{"presets": presets, "clients": clients, "tiers": []string{"efficient", "balanced", "best"}}
}

// providersList is GET /api/providers: the presets, the saved clients in order (the first is active),
// and whether each provider has a key.
func (s *Server) providersList(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.providerState())
}

func (s *Server) saveClients(w http.ResponseWriter, clients []provider.ClientRef) {
	var clean []provider.ClientRef
	for _, c := range clients {
		c, err := c.Validate()
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		clean = append(clean, c)
	}
	s.pmu.Lock()
	err := provider.Save(s.providersPath(), provider.Settings{Clients: clean})
	s.pmu.Unlock()
	if err != nil {
		http.Error(w, "couldn't save the settings", http.StatusInternalServerError)
		return
	}
	writeJSON(w, s.providerState())
}

// setActiveProvider is POST /api/providers/active {provider, base_url, model, tier}: it makes that
// API client the first one, keeping the others behind it.
func (s *Server) setActiveProvider(w http.ResponseWriter, r *http.Request) {
	var ref provider.ClientRef
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, MaxBody)).Decode(&ref); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	ref.Kind = provider.ClientAPI
	ref, err := ref.Validate()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	rest := slices.DeleteFunc(s.providerSettings().Clients, func(c provider.ClientRef) bool {
		return c.Kind == provider.ClientAPI && c.Provider == ref.Provider
	})
	s.saveClients(w, append([]provider.ClientRef{ref}, rest...))
}

// setClients is POST /api/providers/clients {clients:[...]}: the whole ordered list, API providers and
// CLI agents mixed; the first that can start answers. An empty list is the CLI-only setup.
func (s *Server) setClients(w http.ResponseWriter, r *http.Request) {
	var req provider.Settings
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, MaxBody)).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if len(req.Clients) > 8 {
		http.Error(w, "at most 8 clients", http.StatusBadRequest)
		return
	}
	s.saveClients(w, req.Clients)
}

// setKey is POST /api/providers/key {provider, key}.
func (s *Server) setKey(w http.ResponseWriter, r *http.Request) {
	var req struct{ Provider, Key string }
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, MaxBody)).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if _, ok := provider.PresetByID(req.Provider); !ok {
		http.Error(w, "unknown provider", http.StatusBadRequest)
		return
	}
	key, err := provider.CleanKey(req.Key)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.keyStore().Set(req.Provider, key); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"provider": req.Provider, "has_key": true})
}

// deleteKey is POST /api/providers/key/delete {provider}.
func (s *Server) deleteKey(w http.ResponseWriter, r *http.Request) {
	var req struct{ Provider string }
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, MaxBody)).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if _, ok := provider.PresetByID(req.Provider); !ok {
		http.Error(w, "unknown provider", http.StatusBadRequest)
		return
	}
	if err := s.keyStore().Delete(req.Provider); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"provider": req.Provider, "has_key": false})
}

// apiFromQuery builds the API client for a request naming a provider (and maybe a base URL, model, tier).
func (s *Server) apiFromRef(ref provider.ClientRef) (*ask.API, error) {
	ref.Kind = provider.ClientAPI
	ref, err := ref.Validate()
	if err != nil {
		return nil, err
	}
	return ask.APIFor(ref, s.askCfg().Options, s.keyStore(), s.cfg.HTTP)
}

// providerModels is GET /api/providers/models?provider=&base_url=: the models the server lists.
func (s *Server) providerModels(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	a, err := s.apiFromRef(provider.ClientRef{Provider: q.Get("provider"), BaseURL: q.Get("base_url")})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	l, ok := a.Adapter.(provider.Lister)
	if !ok {
		http.Error(w, "this provider has no models list", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	ids, err := l.Models(ctx)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "message": err.Error(), "models": []string{}})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "models": ids})
}

// testProvider is POST /api/providers/test {provider, base_url, model, tier}: one tiny request with no
// tools. With no provider named, it tests the active API client.
func (s *Server) testProvider(w http.ResponseWriter, r *http.Request) {
	var ref provider.ClientRef
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, MaxBody)).Decode(&ref); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(ref.Provider) == "" {
		for _, c := range s.providerSettings().Clients {
			if c.Kind == provider.ClientAPI {
				ref = c
				break
			}
		}
	}
	a, err := s.apiFromRef(ref)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "message": err.Error()})
		return
	}
	ok, msg := s.pingAPI(r.Context(), a)
	writeJSON(w, map[string]any{"ok": ok, "message": msg})
}

// pingAPI sends one tiny request, no tools.
func (s *Server) pingAPI(ctx context.Context, a *ask.API) (bool, string) {
	if a.NeedKey && !a.HasKey {
		return false, "No API key is saved for " + a.Name + "."
	}
	if a.Model == "" {
		return false, "Choose a model first."
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	_, err := a.Adapter.Chat(ctx, provider.Request{Model: a.Model, Messages: []provider.Message{{Role: "user", Text: "Reply with the single word OK."}}}, nil)
	if err != nil {
		return false, err.Error()
	}
	return true, "OK"
}

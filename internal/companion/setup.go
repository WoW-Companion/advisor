package companion

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/WoW-Companion/advisor/internal/gamesetup"
)

// latestTTL is how long a looked-up addon release is reused.
const latestTTL = 10 * time.Minute

// setupCache holds the last release lookup.
type setupCache struct {
	mu  sync.Mutex
	at  time.Time
	rel *gamesetup.AddonRelease
	err string
}

func (s *Server) latestAddon(ctx context.Context, force bool) (*gamesetup.AddonRelease, string) {
	c := &s.setup
	c.mu.Lock()
	defer c.mu.Unlock()
	if !force && !c.at.IsZero() && time.Since(c.at) < latestTTL {
		return c.rel, c.err
	}
	url := s.cfg.AddonAPI
	if url == "" {
		url = gamesetup.AddonLatestURL
	}
	rel, err := gamesetup.LatestAddon(ctx, s.cfg.HTTP, url)
	c.at, c.rel, c.err = time.Now(), rel, ""
	if err != nil {
		c.err = err.Error()
	}
	return c.rel, c.err
}

func (s *Server) setupOptions(game string, force bool, ctx context.Context) gamesetup.Options {
	if game == "" {
		game = s.cfg.GameDir
	}
	rel, e := s.latestAddon(ctx, force)
	return gamesetup.Options{GameDir: game, SavedVariables: s.cfg.ForeverScribe, Latest: rel, LatestErr: e}
}

func (s *Server) setupItems(game string, force bool, ctx context.Context) []gamesetup.Item {
	items := gamesetup.Check(s.setupOptions(game, force, ctx))
	items = append(items, s.datasetItem())
	return append(items, gamesetup.AIItem(s.cfg.Ask.Name))
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// setupState is GET /api/setup?game=<folder>: every checklist row.
func (s *Server) setupState(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{"items": s.setupItems(r.URL.Query().Get("game"), false, r.Context())})
}

// setupInstall is POST /api/setup/install-addon {"game": folder}: the player's click on Install or Update.
func (s *Server) setupInstall(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Game string `json:"game"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, MaxBody)).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	o := s.setupOptions(req.Game, true, r.Context())
	res := map[string]any{}
	game, ok := gamesetup.Resolve(o)
	switch {
	case !ok:
		res["error"] = "WoW folder not found: choose it first."
	case o.Latest == nil:
		res["error"] = "Could not look up the latest addon release: " + o.LatestErr
	default:
		if err := gamesetup.InstallAddon(r.Context(), s.cfg.HTTP, game, o.Latest); err != nil {
			res["error"] = err.Error()
		} else {
			res["ok"] = true
			res["message"] = "Installed " + o.Latest.Version + ". Restart the game fully if this was the first install."
		}
	}
	res["items"] = s.setupItems(req.Game, false, r.Context())
	writeJSON(w, res)
}

// setupTestAI is POST /api/setup/test-ai: one tiny prompt through the configured agent.
func (s *Server) setupTestAI(w http.ResponseWriter, r *http.Request) {
	ok, msg := gamesetup.TestAI(r.Context(), s.cfg.Ask.Command)
	writeJSON(w, map[string]any{"ok": ok, "message": msg})
}

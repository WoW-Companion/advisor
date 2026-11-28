package companion

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"github.com/WoW-Companion/advisor/internal/digest"
	"github.com/WoW-Companion/advisor/internal/gamesetup"
	"github.com/WoW-Companion/advisor/internal/server"
)

// latestTTL is how long a looked-up addon release is reused.
const latestTTL = 10 * time.Minute

// setupCache holds the last release lookup.
type setupCache struct {
	mu  sync.Mutex
	at  time.Time
	rel *gamesetup.AddonRelease
	err string
	fb  *gamesetup.AddonRelease // the release offered in place of a pulled one
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
	o := gamesetup.Options{GameDir: game, SavedVariables: s.cfg.ForeverScribe, Latest: rel, LatestErr: e}
	if s.cfg.Updates != nil {
		if _, pulls, err := s.cfg.Updates(ctx, false); err == nil {
			o.Pulled = pulls.Addon
		}
	}
	if len(o.Pulled) > 0 {
		o.Fallback = s.fallbackAddon(ctx, o)
	}
	return o
}

// fallbackAddon looks up the addon release to offer in place of the newest one when that, or the
// installed one, has been pulled. nil when the newest release is itself the offer.
func (s *Server) fallbackAddon(ctx context.Context, o gamesetup.Options) *gamesetup.AddonRelease {
	game, _ := gamesetup.Resolve(o)
	have, _ := gamesetup.InstalledAddonVersion(game)
	if game == "" || have == "unknown" {
		have = ""
	}
	plan, rel := o.Offer(have)
	if plan.Version == "" || rel != nil {
		return nil
	}
	c := &s.setup
	c.mu.Lock()
	if c.fb != nil && c.fb.Version == plan.Version {
		defer c.mu.Unlock()
		return c.fb
	}
	c.mu.Unlock()
	fb, err := gamesetup.LatestAddon(ctx, s.cfg.HTTP, gamesetup.AddonTagURL(plan.Version))
	if err != nil {
		return nil
	}
	c.mu.Lock()
	c.fb = fb
	c.mu.Unlock()
	return fb
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
		res["error"] = gamesetup.NoGameMessage
	case o.Latest == nil:
		res["error"] = "Could not look up the latest addon release: " + o.LatestErr
	default:
		have, _ := gamesetup.InstalledAddonVersion(game)
		if _, ok := gamesetup.InstalledAddonVersion(game); !ok {
			have = ""
		}
		_, rel := o.Offer(have)
		if rel == nil {
			res["error"] = "There is no safe addon version to install right now: the newest one has a known problem."
			break
		}
		note, err := gamesetup.InstallAddonNote(r.Context(), s.cfg.HTTP, game, rel, s.addonBackups())
		if err != nil {
			res["error"] = err.Error()
		} else {
			res["ok"] = true
			res["message"] = "Installed " + rel.Version + ". " + note.Note
			res["restart"] = note.Restart
		}
	}
	res["items"] = s.setupItems(req.Game, false, r.Context())
	writeJSON(w, res)
}

// setupWriteData is POST /api/setup/write-data {"game": folder}: the "Write now" click. It writes the
// digest and route plan files, or empty stubs when nothing is captured yet.
func (s *Server) setupWriteData(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Game string `json:"game"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, MaxBody)).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	res := map[string]any{}
	cfg := s.withPending(server.Config{SavedVariables: s.cfg.ForeverScribe, Dataset: s.datasetDir(), DigestPath: s.cfg.DigestPath, SkipUnchanged: true})
	if game, ok := gamesetup.Resolve(s.setupOptions(req.Game, false, r.Context())); ok && cfg.DigestPath == "" {
		cfg.DigestPath = filepath.Join(game, "Interface", "AddOns", digest.SlotAddon, "Digest.lua")
	}
	// Nothing captured yet is not a failure: WriteDigest then leaves stubs, which keep the game quiet.
	server.WriteDigest(cfg, "")
	if err := server.EnsureDataFiles(cfg); err != nil {
		res["error"] = err.Error()
	} else {
		res["ok"] = true
		res["message"] = "Wrote the data files. Type /reload in game to show them."
	}
	res["items"] = s.setupItems(req.Game, false, r.Context())
	writeJSON(w, res)
}

// setupTestAI is POST /api/setup/test-ai: one tiny prompt through the configured agent.
func (s *Server) setupTestAI(w http.ResponseWriter, r *http.Request) {
	if ac := s.askCfg(); ac.API != nil {
		ok, msg := s.pingAPI(r.Context(), ac.API)
		writeJSON(w, map[string]any{"ok": ok, "message": msg})
		return
	}
	ok, msg := gamesetup.TestAI(r.Context(), s.cfg.Ask.Command)
	writeJSON(w, map[string]any{"ok": ok, "message": msg})
}

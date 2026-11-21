package companion

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/WoW-Companion/advisor/internal/dataset"
	"github.com/WoW-Companion/advisor/internal/gamesetup"
	"github.com/WoW-Companion/advisor/internal/server"
)

// NoDataMessage is what the player sees instead of advice when no game data has downloaded.
const NoDataMessage = "Game data not downloaded yet; see Setup"

// downloadTimeout bounds one "Download now" click.
const downloadTimeout = 10 * time.Minute

// dataState is the dataset directory in use and the last download's outcome.
type dataState struct {
	mu     sync.Mutex
	dir    string // the dataset directory advice reads; empty until one is found
	failed string // the last failed download's reason
	busy   bool
}

// datasetDir is the dataset directory advice reads from, empty when there is none.
func (s *Server) datasetDir() string {
	s.data.mu.Lock()
	defer s.data.mu.Unlock()
	return s.data.dir
}

func (s *Server) cacheStore() (dataset.Store, error) {
	st := s.cfg.Cache
	if st.Root == "" {
		root, err := dataset.DefaultRoot()
		if err != nil {
			return st, err
		}
		st.Root = root
	}
	if st.HTTP == nil {
		st.HTTP = s.cfg.HTTP
	}
	return st, nil
}

// datasetItem is the "Game data" checklist row, from the existing loader's cache.
func (s *Server) datasetItem() gamesetup.Item {
	st := gamesetup.DatasetState{Client: server.CaptureClient(s.cfg.ForeverScribe)}
	if store, err := s.cacheStore(); err == nil {
		st.Cached, _ = store.Cached()
	} else {
		st.Failed = err.Error()
	}
	s.data.mu.Lock()
	st.Busy = s.data.busy
	if st.Failed == "" {
		st.Failed = s.data.failed
	}
	s.data.mu.Unlock()
	return gamesetup.DatasetItem(st)
}

// fetchDataset downloads (or finds cached) the dataset for the capture's build through the
// existing loader and starts using it. It returns the failure reason, "" on success.
func (s *Server) fetchDataset(ctx context.Context) string {
	s.data.mu.Lock()
	if s.data.busy {
		s.data.mu.Unlock()
		return "a download is already running"
	}
	s.data.busy = true
	s.data.mu.Unlock()

	reason := ""
	dir := ""
	store, err := s.cacheStore()
	if err == nil {
		var res dataset.Result
		ctx, cancel := context.WithTimeout(ctx, downloadTimeout)
		res, err = store.Ensure(ctx, server.CaptureClient(s.cfg.ForeverScribe).Build, false)
		cancel()
		dir = res.Dir
		if err == nil && res.Offline != nil {
			reason = "" // the cache answered; nothing to retry
		}
	}
	if err != nil {
		reason = err.Error()
	}
	s.data.mu.Lock()
	s.data.busy, s.data.failed = false, reason
	if reason == "" {
		s.data.dir = dir
	}
	s.data.mu.Unlock()
	if reason == "" {
		s.reload()
		s.publish("game", s.snapshot())
	}
	return reason
}

// setupDownloadData is POST /api/setup/download-dataset: the player's click on Download now or Retry.
func (s *Server) setupDownloadData(w http.ResponseWriter, r *http.Request) {
	res := map[string]any{}
	if reason := s.fetchDataset(r.Context()); reason != "" {
		res["error"] = "Download failed: " + reason
	} else {
		res["ok"] = true
		res["message"] = "Game data downloaded."
	}
	res["items"] = s.setupItems("", false, r.Context())
	writeJSON(w, res)
}

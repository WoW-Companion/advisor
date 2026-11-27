package companion

import (
	"context"
	"log"
	"net/http"
	"path/filepath"
	"regexp"
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
	last   time.Time // when the last republish check ran
	notice string    // shown dim once an update has installed
	prev   string    // the dataset directory in use before the last switch, kept for a go-back
}

// DataNotice is the dim line shown after a dataset update installs.
const DataNotice = "Game data updated (talent builds and more)"

// UpdatedNotice is the update notice after the dataset switches to a build matching the game.
func UpdatedNotice(build string) string {
	return "Updated: data " + build + " (matches your game)"
}

// updateEvery is the least time between two checks for a republished dataset.
const updateEvery = 24 * time.Hour

func (s *Server) dataNotice() string {
	s.data.mu.Lock()
	defer s.data.mu.Unlock()
	return s.data.notice
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
		for _, b := range st.Cached {
			if dataset.Matches(b, st.Client.Version, st.Client.Build) || st.Client.Build == "" {
				if m, err := store.CachedManifest(b); err == nil {
					st.Revision = revisionDate(m)
				}
				break
			}
		}
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

var dateRE = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}`)

// revisionDate is the day a dataset revision was published: the date the revision starts with,
// else the manifest's created date, else the revision itself.
func revisionDate(m dataset.Manifest) string {
	for _, v := range []string{m.Revision, m.Created} {
		if d := dateRE.FindString(v); d != "" {
			return d
		}
	}
	return m.Revision
}

// checkDatasetUpdate looks for a republished dataset for the capture's build, at most once every
// 24 hours. A new revision is downloaded and verified, then the digest and plan are rewritten so
// in-game advice uses it. Offline or on error the cache stays and one line is logged. It reports
// whether an update installed.
func (s *Server) checkDatasetUpdate(ctx context.Context) bool {
	s.data.mu.Lock()
	if s.data.busy || (!s.data.last.IsZero() && s.now().Sub(s.data.last) < updateEvery) {
		s.data.mu.Unlock()
		return false
	}
	s.data.busy, s.data.last = true, s.now()
	s.data.mu.Unlock()

	var res dataset.SyncResult
	store, err := s.cacheStore()
	client := server.CaptureClient(s.cfg.ForeverScribe)
	if err == nil {
		cctx, cancel := context.WithTimeout(ctx, downloadTimeout)
		res, err = store.Sync(cctx, client.Build)
		cancel()
	}
	updated := false
	s.data.mu.Lock()
	s.data.busy = false
	if err == nil && res.Build != "" {
		cur := ""
		if s.data.dir != "" {
			cur = filepath.Base(s.data.dir)
		}
		switch {
		case cur != "" && dataset.BuildNewer(res.Build, cur):
			updated = true // a newer build: switch
		case res.Build == cur && res.Updated:
			updated = true // the current build was republished
		case cur == "" && res.Updated:
			updated = true
		}
		if updated {
			if s.data.dir != "" && cur != res.Build {
				s.data.prev = s.data.dir
			}
			s.data.dir = store.Dir(res.Build)
			s.data.notice = DataNotice
			if !res.Missing && res.Build != cur && client.Build != "" {
				s.data.notice = UpdatedNotice(res.Build)
			}
		}
		// Never downgrade: a build older than the one in use is left in the cache only.
	}
	s.data.mu.Unlock()
	if err != nil {
		log.Printf("wowc-advisor: dataset update check failed (keeping the cached dataset): %v", err)
		return false
	}
	if updated {
		s.reload()
		s.refreshPlan() // no addon folder is not an error here: there is nothing to rewrite
		s.publish("game", s.snapshot())
	}
	return updated
}

// watchDataset checks for a republished dataset on start and then daily while running.
func (s *Server) watchDataset(ctx context.Context) {
	for {
		s.checkDatasetUpdate(ctx)
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Hour):
		}
	}
}

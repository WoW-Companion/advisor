package companion

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	advisor "github.com/WoW-Companion/advisor"
	"github.com/WoW-Companion/advisor/internal/release"
	"github.com/WoW-Companion/advisor/internal/whatsnew"
)

// notesPart is one component's notes in the "What's new" view: a release per version, newest first.
type notesPart struct {
	Entries []whatsnew.Release `json:"entries"`
	More    bool               `json:"more"`
}

// addonChangelogCache is where the addon's CHANGELOG.md is kept, so the view works offline for
// every addon version already installed.
func (s *Server) addonChangelogCache() string {
	return filepath.Join(filepath.Dir(s.addonBackups()), "addon-changelog.md")
}

// addonChangelog is the addon CHANGELOG at the installed version's tag, from the network when it
// answers (and then kept), else from the kept copy. "" when neither has it.
func (s *Server) addonChangelog(ctx context.Context, installed string) string {
	url := s.cfg.AddonNotes
	if url == "" {
		url = AddonNotesURL
	}
	client := s.cfg.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	cache := s.addonChangelogCache()
	if installed != "" {
		if md, err := release.Changelog(ctx, client, url, installed); err == nil && md != "" {
			if os.MkdirAll(filepath.Dir(cache), 0o755) == nil {
				_ = os.WriteFile(cache, []byte(md), 0o644)
			}
			return md
		}
	}
	b, _ := os.ReadFile(cache)
	return string(b)
}

// notesSince is GET /api/notes-since?app_from=&app=&addon_from=&addon=: for each component, the
// notes of every version after *_from up to the installed one (capped by whatsnew.Since). The app's
// come from the changelog bundled in this binary, the addon's from its repository or the kept copy.
func (s *Server) notesSince(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	now := s.now()
	var pulled []string
	dates := map[string]time.Time{}
	if s.cfg.Updates != nil {
		if rels, p, err := s.cfg.Updates(r.Context(), false); err == nil {
			pulled = pulledVersions(p)
			for _, rel := range rels {
				dates[rel.Version] = rel.Published
			}
		}
	}
	res := map[string]notesPart{"app": {Entries: []whatsnew.Release{}}, "addon": {Entries: []whatsnew.Release{}}}
	if e, more := whatsnew.Since(advisor.Changelog, q.Get("app_from"), q.Get("app"), pulled, dates, now); e != nil {
		res["app"] = notesPart{Entries: e, More: more}
	}
	if from, to := q.Get("addon_from"), strings.TrimPrefix(q.Get("addon"), "v"); from != "" && to != "" && to != "unknown" {
		if md := s.addonChangelog(r.Context(), to); md != "" {
			if e, more := whatsnew.Since(md, from, to, nil, nil, now); e != nil {
				res["addon"] = notesPart{Entries: e, More: more}
			}
		}
	}
	writeJSON(w, res)
}

package companion

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/WoW-Companion/advisor/internal/gamesetup"
	"github.com/WoW-Companion/advisor/internal/release"
	"github.com/WoW-Companion/advisor/internal/whatsnew"
)

// AddonNotesURL is the addon repo's CHANGELOG.md at a release tag; %s is the bare version.
const AddonNotesURL = "https://raw.githubusercontent.com/WoW-Companion/addon/v%s/CHANGELOG.md"

// addonBackups is where the last few addon versions are kept. It is outside the game folder.
func (s *Server) addonBackups() string {
	if s.cfg.AddonBackups != "" {
		return s.cfg.AddonBackups
	}
	d, err := os.UserCacheDir()
	if err != nil {
		d = os.TempDir()
	}
	return filepath.Join(d, "wowc-advisor", "addon-backups")
}

// addonNotes is the addon CHANGELOG between the installed version and the offered one, in the
// app's own bullet format. "" when the lookup fails: the notice then has no list.
func (s *Server) addonNotes(ctx context.Context, from, to string, pulled []release.Pull) string {
	url := s.cfg.AddonNotes
	if url == "" {
		url = AddonNotesURL
	}
	client := s.cfg.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	md, err := release.Changelog(ctx, client, url, to)
	if err != nil {
		return ""
	}
	var skip []string
	for _, p := range pulled {
		skip = append(skip, p.Version)
	}
	return whatsnew.Range(md, from, to, skip)
}

// addonUpdate is GET /api/addon/update?game=&now=1&fresh=1: the installed addon, the version to offer
// (after the settle delay, with the same "right away" switch as the app) and the versions kept.
func (s *Server) addonUpdate(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	o := s.setupOptions(q.Get("game"), q.Get("fresh") == "1", r.Context())
	st := gamesetup.StatusFor(o, s.addonBackups(), q.Get("now") == "1", s.now())
	if st.Available {
		st.Notes = s.addonNotes(r.Context(), st.Installed, st.Latest, o.Pulled)
	}
	writeJSON(w, st)
}

// addonInstall is POST /api/addon/install {"game": folder, "goback": "<kept version>"}: installs the
// offered version, or restores a kept one. The reply has the new status.
func (s *Server) addonInstall(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Game   string `json:"game"`
		GoBack string `json:"goback"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, MaxBody)).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	res := map[string]any{}
	o := s.setupOptions(req.Game, true, r.Context())
	game, ok := gamesetup.Resolve(o)
	switch {
	case !ok:
		res["error"] = gamesetup.NoGameMessage
	case req.GoBack != "":
		note, err := gamesetup.GoBack(game, s.addonBackups(), req.GoBack)
		if err != nil {
			res["error"] = err.Error()
		} else {
			res["ok"] = true
			res["message"] = "Went back to addon " + strings.TrimPrefix(req.GoBack, "v") + ". " + note
		}
	default:
		have, installed := gamesetup.InstalledAddonVersion(game)
		if !installed || have == "unknown" {
			have = ""
		}
		_, rel := o.Offer(have)
		if rel == nil {
			res["error"] = "There is no addon update to install right now."
			break
		}
		note, err := gamesetup.InstallAddonNote(r.Context(), s.cfg.HTTP, game, rel, s.addonBackups())
		if err != nil {
			res["error"] = err.Error()
		} else {
			res["ok"] = true
			res["message"] = "Addon " + rel.Version + " is installed. " + note
		}
	}
	res["status"] = gamesetup.StatusFor(s.setupOptions(req.Game, false, r.Context()), s.addonBackups(), false, s.now())
	writeJSON(w, res)
}

// Package dataset finds, downloads, verifies and caches the published dataset for a game build.
//
// Releases are GitHub releases of the data repo named `dataset-<build>`, each with a
// `manifest-<build>.json` and a `wowc-dataset-<build>.tar.zst` asset (see the data repo's
// docs/releases.md). A dataset is cached at `<root>/<build>`; the cache only ever holds directories
// whose every file matched the manifest's sha256 when they were put there.
package dataset

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	DefaultReleasesURL = "https://github.com/WoW-Companion/data/releases"
	DefaultAPIURL      = "https://api.github.com/repos/WoW-Companion/data/releases"

	tagPrefix = "dataset-"
	// maxManifest bounds a manifest download; maxUnpacked bounds all bytes written for one dataset.
	maxManifest = 8 << 20
	maxUnpacked = 8 << 30
)

// ErrNoDataset means no cached dataset satisfies the request and none could be downloaded.
var ErrNoDataset = errors.New("no dataset available")

var buildRE = regexp.MustCompile(`^[0-9]+(\.[0-9]+)*$`)

// Manifest is the part of manifest.json the loader checks.
type Manifest struct {
	DatasetVersion string `json:"dataset_version"`
	GameBuild      string `json:"game_build"`
	Created        string `json:"created"`
	Revision       string `json:"revision"` // bumped when the data repo republishes the same build
	Tables         []struct {
		Path   string `json:"path"`
		SHA256 string `json:"sha256"`
	} `json:"tables"`
}

// Store downloads into and reads from one cache root.
type Store struct {
	Root        string       // cache root; each dataset is Root/<build>
	ReleasesURL string       // download base, default DefaultReleasesURL
	APIURL      string       // releases API, default DefaultAPIURL
	HTTP        *http.Client // default http.DefaultClient
}

// DefaultRoot is the per-user cache: %LOCALAPPDATA%\wowc\dataset on Windows, the XDG cache
// ($XDG_CACHE_HOME or ~/.cache) on Linux and ~/Library/Caches on macOS.
func DefaultRoot() (string, error) {
	d, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "wowc", "dataset"), nil
}

func (s Store) client() *http.Client {
	if s.HTTP != nil {
		return s.HTTP
	}
	return http.DefaultClient
}

func (s Store) releases() string {
	return strings.TrimRight(orDefault(s.ReleasesURL, DefaultReleasesURL), "/")
}
func (s Store) api() string { return strings.TrimRight(orDefault(s.APIURL, DefaultAPIURL), "/") }

func orDefault(v, d string) string {
	if v == "" {
		return d
	}
	return v
}

// Dir is where the dataset for a full build lives.
func (s Store) Dir(build string) string { return filepath.Join(s.Root, build) }

// Matches reports whether a dataset's game build is the capture's client: the full
// "<version>.<build>" or a dotted build ending in the client's build number.
func Matches(gameBuild, clientVersion, clientBuild string) bool {
	if gameBuild == "" || clientBuild == "" {
		return false
	}
	if clientVersion != "" && gameBuild == clientVersion+"."+clientBuild {
		return true
	}
	return gameBuild == clientBuild || strings.HasSuffix(gameBuild, "."+clientBuild)
}

// VerifyDir re-hashes every file the manifest in dir lists and returns the manifest.
func VerifyDir(dir string) (Manifest, error) {
	data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return Manifest{}, err
	}
	m, err := parseManifest(data)
	if err != nil {
		return m, err
	}
	for _, t := range m.Tables {
		sum, err := fileSHA(filepath.Join(dir, filepath.FromSlash(t.Path)))
		if err != nil {
			return m, fmt.Errorf("%s: %w", t.Path, err)
		}
		if sum != t.SHA256 {
			return m, fmt.Errorf("%s: sha256 %s, manifest says %s", t.Path, sum, t.SHA256)
		}
	}
	return m, nil
}

// Cached lists the full builds in the cache whose directories verify, newest first. Directories
// that fail verification are returned in bad with the reason.
func (s Store) Cached() (good []string, bad map[string]error) {
	bad = map[string]error{}
	entries, _ := os.ReadDir(s.Root)
	for _, e := range entries {
		if !e.IsDir() || !buildRE.MatchString(e.Name()) {
			continue
		}
		if _, err := VerifyDir(filepath.Join(s.Root, e.Name())); err != nil {
			bad[e.Name()] = err
			continue
		}
		good = append(good, e.Name())
	}
	sort.Slice(good, func(i, j int) bool { return buildLess(good[j], good[i]) })
	return good, bad
}

func buildLess(a, b string) bool {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(pa) && i < len(pb); i++ {
		x, _ := strconv.Atoi(pa[i])
		y, _ := strconv.Atoi(pb[i])
		if x != y {
			return x < y
		}
	}
	return len(pa) < len(pb)
}

// Result is a usable dataset directory.
type Result struct {
	Dir        string
	Build      string
	Downloaded bool  // fetched by this call
	Offline    error // set when the network failed and the cache was used instead
}

// Ensure returns a verified dataset for want, a client build (a full "1.60.1.71037" or just the
// number "71037"), downloading it when the cache lacks it. An empty want means the newest: the
// newest cached dataset when update is false, else the newest release. When the network fails
// and the cache has a match, the cache is used and Result.Offline says why.
func (s Store) Ensure(ctx context.Context, want string, update bool) (Result, error) {
	good, _ := s.Cached()
	if cached := pick(good, want); cached != "" && (want != "" || !update) {
		return Result{Dir: s.Dir(cached), Build: cached}, nil
	}
	build, err := s.resolve(ctx, want)
	if err == nil {
		if contains(good, build) {
			return Result{Dir: s.Dir(build), Build: build}, nil
		}
		if err = s.download(ctx, build); err == nil {
			return Result{Dir: s.Dir(build), Build: build, Downloaded: true}, nil
		}
	}
	if cached := pick(good, want); cached != "" {
		return Result{Dir: s.Dir(cached), Build: cached, Offline: err}, nil
	}
	return Result{}, fmt.Errorf("%w: %v", ErrNoDataset, err)
}

func contains(l []string, v string) bool {
	for _, x := range l {
		if x == v {
			return true
		}
	}
	return false
}

// pick returns the newest of builds (sorted newest first) matching want, or "".
func pick(builds []string, want string) string {
	for _, b := range builds {
		if want == "" || b == want || strings.HasSuffix(b, "."+want) {
			return b
		}
	}
	return ""
}

type release struct {
	Tag string `json:"tag_name"`
}

// resolve turns a wanted build into the full build of a published release.
func (s Store) resolve(ctx context.Context, want string) (string, error) {
	if want != "" && !buildRE.MatchString(want) {
		return "", fmt.Errorf("invalid build %q", want)
	}
	if strings.Count(want, ".") >= 3 {
		return want, nil
	}
	endpoint := s.api() + "/latest"
	if want != "" {
		endpoint = s.api() + "?per_page=100"
	}
	body, err := s.get(ctx, endpoint, maxManifest)
	if err != nil {
		return "", err
	}
	if want == "" {
		var r release
		if err := json.Unmarshal(body, &r); err != nil {
			return "", fmt.Errorf("latest release: %w", err)
		}
		return tagBuild(r.Tag)
	}
	var rs []release
	if err := json.Unmarshal(body, &rs); err != nil {
		return "", fmt.Errorf("release list: %w", err)
	}
	for _, r := range rs {
		if b, err := tagBuild(r.Tag); err == nil && strings.HasSuffix(b, "."+want) {
			return b, nil
		}
	}
	return "", fmt.Errorf("no published dataset for build %s", want)
}

func tagBuild(tag string) (string, error) {
	b := strings.TrimPrefix(tag, tagPrefix)
	if b == tag || !buildRE.MatchString(b) {
		return "", fmt.Errorf("release tag %q is not a dataset", tag)
	}
	return b, nil
}

func (s Store) assetURL(build, name string) string {
	return s.releases() + "/download/" + url.PathEscape(tagPrefix+build) + "/" + url.PathEscape(name)
}

func (s Store) get(ctx context.Context, u string, limit int64) ([]byte, error) {
	resp, err := s.open(ctx, u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", u, err)
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("GET %s: larger than %d bytes", u, limit)
	}
	return data, nil
}

func (s Store) open(ctx context.Context, u string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.client().Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("GET %s: %s", u, resp.Status)
	}
	return resp, nil
}

// download fetches, verifies and installs one build. The cache directory is only ever replaced by
// a complete verified one, by a rename; on any failure the temporary directory is removed.
func (s Store) download(ctx context.Context, build string) error {
	manifest, err := s.get(ctx, s.assetURL(build, "manifest-"+build+".json"), maxManifest)
	if err != nil {
		return err
	}
	want, err := parseManifest(manifest)
	if err != nil {
		return fmt.Errorf("manifest-%s.json: %w", build, err)
	}
	if want.GameBuild != build {
		return fmt.Errorf("release %s holds a manifest for build %s", build, want.GameBuild)
	}
	if err := os.MkdirAll(s.Root, 0o755); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(s.Root, ".download-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	resp, err := s.open(ctx, s.assetURL(build, "wowc-dataset-"+build+".tar.zst"))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if err := unpack(resp.Body, build, want, manifest, tmp); err != nil {
		return fmt.Errorf("dataset %s: %w", build, err)
	}
	if _, err := VerifyDir(tmp); err != nil {
		return fmt.Errorf("dataset %s: %w", build, err)
	}
	final := s.Dir(build)
	old := ""
	if _, err := os.Stat(final); err == nil {
		old = tmp + ".old"
		if err := os.Rename(final, old); err != nil {
			return err
		}
	}
	if err := os.Rename(tmp, final); err != nil {
		if old != "" {
			os.Rename(old, final)
		}
		return err
	}
	if old != "" {
		os.RemoveAll(old)
	}
	return nil
}

// ParseManifest decodes and checks manifest.json bytes: a game build and tables with safe paths and
// sha256 sums.
func ParseManifest(data []byte) (Manifest, error) { return parseManifest(data) }

func parseManifest(data []byte) (Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return m, err
	}
	if m.GameBuild == "" || len(m.Tables) == 0 {
		return m, errors.New("manifest lacks game_build or tables")
	}
	for _, t := range m.Tables {
		if !safePath(t.Path) || t.Path == "manifest.json" || len(t.SHA256) != 64 {
			return m, fmt.Errorf("manifest entry %q is not usable", t.Path)
		}
	}
	return m, nil
}

// safePath accepts a clean relative slash path that stays inside its directory.
func safePath(p string) bool {
	return p != "" && !strings.ContainsAny(p, `\:`) && path.Clean(p) == p && !path.IsAbs(p) &&
		p != "." && !strings.HasPrefix(p, "../")
}

func fileSHA(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	return hashReader(f)
}

// Same reports whether two manifests describe the same published dataset: equal revisions, or,
// when either lacks one, the same table sha256 set.
func Same(a, b Manifest) bool {
	if a.Revision != "" && b.Revision != "" {
		return a.Revision == b.Revision
	}
	return shaSet(a) == shaSet(b)
}

func shaSet(m Manifest) string {
	l := make([]string, 0, len(m.Tables))
	for _, t := range m.Tables {
		l = append(l, t.Path+"="+t.SHA256)
	}
	sort.Strings(l)
	return strings.Join(l, "\n")
}

// Refresh checks the published manifest for a cached build and, when it differs from the cached
// one (a republish of the same build), downloads and verifies the new archive and replaces the
// cache directory by a rename. want is a client build as for Ensure; "" means the newest release.
// It returns the full build checked and whether the cache was replaced. A build that is not cached
// is left to Ensure; on any error the cache is untouched.
func (s Store) Refresh(ctx context.Context, want string) (build string, updated bool, err error) {
	build, err = s.resolve(ctx, want)
	if err != nil {
		return "", false, err
	}
	cached, err := os.ReadFile(filepath.Join(s.Dir(build), "manifest.json"))
	if err != nil {
		return build, false, nil // nothing cached for it; Ensure downloads first-time datasets
	}
	have, err := parseManifest(cached)
	if err != nil {
		return build, false, nil
	}
	body, err := s.get(ctx, s.assetURL(build, "manifest-"+build+".json"), maxManifest)
	if err != nil {
		return build, false, err
	}
	latest, err := parseManifest(body)
	if err != nil {
		return build, false, fmt.Errorf("manifest-%s.json: %w", build, err)
	}
	if Same(have, latest) {
		return build, false, nil
	}
	if err := s.download(ctx, build); err != nil {
		return build, false, err
	}
	return build, true, nil
}

// CachedManifest reads the manifest of the cached dataset for a full build.
func (s Store) CachedManifest(build string) (Manifest, error) {
	data, err := os.ReadFile(filepath.Join(s.Dir(build), "manifest.json"))
	if err != nil {
		return Manifest{}, err
	}
	return parseManifest(data)
}

// SyncResult is what Sync did for one game build.
type SyncResult struct {
	Build   string // full build now preferred for the game (the one checked or downloaded)
	Updated bool   // the cache gained or replaced Build by this call
	Missing bool   // no dataset is published for the game's build; Build is the newest older one ("" if none)
}

// Sync is the update rule. game is the client build ("71139" or a full build). A published
// dataset for the game's build is downloaded and verified when the cache lacks it (or replaced
// when republished). When none is published, the newest published dataset whose build is not
// newer than the game's is used instead and Missing is set. It only ever adds or replaces cache
// directories: older builds stay for a go-back, and the caller decides whether to switch.
func (s Store) Sync(ctx context.Context, game string) (SyncResult, error) {
	if game == "" {
		b, up, err := s.Refresh(ctx, "")
		return SyncResult{Build: b, Updated: up}, err
	}
	if !buildRE.MatchString(game) {
		return SyncResult{}, fmt.Errorf("invalid build %q", game)
	}
	target, err := s.resolve(ctx, game)
	missing := false
	if err != nil {
		if strings.Count(game, ".") >= 3 || !strings.HasPrefix(err.Error(), "no published dataset") {
			return SyncResult{}, err
		}
		if target, err = s.newestAtMost(ctx, game); err != nil {
			return SyncResult{}, err
		}
		missing = true
	}
	if target == "" {
		return SyncResult{Missing: true}, nil
	}
	if good, _ := s.Cached(); !contains(good, target) {
		if err := s.download(ctx, target); err != nil {
			return SyncResult{Build: target, Missing: missing}, err
		}
		return SyncResult{Build: target, Updated: true, Missing: missing}, nil
	}
	b, up, err := s.Refresh(ctx, target)
	return SyncResult{Build: b, Updated: up, Missing: missing}, err
}

// newestAtMost is the newest published dataset whose client number is at most game's, or "".
func (s Store) newestAtMost(ctx context.Context, game string) (string, error) {
	body, err := s.get(ctx, s.api()+"?per_page=100", maxManifest)
	if err != nil {
		return "", err
	}
	var rs []release
	if err := json.Unmarshal(body, &rs); err != nil {
		return "", fmt.Errorf("release list: %w", err)
	}
	limit, _ := strconv.Atoi(game[strings.LastIndex(game, ".")+1:])
	best := ""
	for _, r := range rs {
		b, err := tagBuild(r.Tag)
		if err != nil {
			continue
		}
		n, _ := strconv.Atoi(b[strings.LastIndex(b, ".")+1:])
		if n <= limit && (best == "" || buildLess(best, b)) {
			best = b
		}
	}
	return best, nil
}

// BuildNewer reports whether build a is strictly newer than b ("" is oldest).
func BuildNewer(a, b string) bool { return a != "" && (b == "" || buildLess(b, a)) }

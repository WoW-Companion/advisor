package release

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	// ListURL is the unauthenticated GitHub endpoint for the newest published releases.
	ListURL = "https://api.github.com/repos/WoW-Companion/advisor/releases?per_page=30"
	// ChangelogURL is the CHANGELOG.md at a release tag; %s is the bare version.
	ChangelogURL = "https://raw.githubusercontent.com/WoW-Companion/advisor/v%s/CHANGELOG.md"
	// PullsURL is the pulled-version file on the main branch.
	PullsURL = "https://raw.githubusercontent.com/WoW-Companion/advisor/main/updates/updates.json"
	// SettleDelay is how long a release must have been public before it is offered, unless the
	// player asked for updates right away.
	SettleDelay = 24 * time.Hour
	// ListSize is how many other versions the Settings list shows beside the running one.
	ListSize = 5
)

// Rel is one published release.
type Rel struct {
	Version   string    // "0.7.53", with no leading "v"
	Published time.Time // published_at
	Body      string    // the release notes
}

// Pull is one pulled version: a release that must not be offered, and where to go back to.
type Pull struct {
	Version string `json:"version"`
	Reason  string `json:"reason"`
	BackTo  string `json:"back_to"`
}

// Pulls is updates/updates.json.
type Pulls struct {
	Schema      int               `json:"schema"`
	Pulled      []Pull            `json:"pulled"`
	AddonPulled []json.RawMessage `json:"addon_pulled"`
	Notice      json.RawMessage   `json:"notice"`
	// Addon is addon_pulled read leniently: an entry that is not a usable object is dropped, never
	// an error. It has the same shape as a pulled app version.
	Addon []Pull `json:"-"`
}

// ParsePulls reads the pull file. A file that cannot be parsed pulls nothing: a broken file must
// never block an update.
func ParsePulls(b []byte) Pulls {
	var p Pulls
	if err := json.Unmarshal(b, &p); err != nil {
		return Pulls{}
	}
	var keep []Pull
	for _, x := range p.Pulled {
		if x.Version = bare(x.Version); x.Version != "" {
			x.BackTo = bare(x.BackTo)
			keep = append(keep, x)
		}
	}
	p.Pulled = keep
	for _, raw := range p.AddonPulled {
		var x Pull
		if json.Unmarshal(raw, &x) == nil {
			if x.Version = bare(x.Version); x.Version != "" {
				x.BackTo = bare(x.BackTo)
				p.Addon = append(p.Addon, x)
			}
		}
	}
	return p
}

// Find is the pull entry for a version.
func (p Pulls) Find(version string) (Pull, bool) {
	v := bare(version)
	for _, x := range p.Pulled {
		if x.Version == v {
			return x, true
		}
	}
	return Pull{}, false
}

func bare(v string) string { return strings.TrimPrefix(strings.TrimSpace(v), "v") }

// Options are the player's side of the offer rule.
type Options struct {
	Running   string
	Skipped   []string
	Immediate bool // "Get updates right away": no settle delay
	Now       time.Time
}

func (o Options) skipped(v string) bool {
	for _, s := range o.Skipped {
		if bare(s) == v {
			return true
		}
	}
	return false
}

// newest is the release with the highest version among those keep accepts.
func newest(rels []Rel, keep func(Rel) bool) (Rel, bool) {
	var best Rel
	found := false
	for _, r := range rels {
		if !keep(r) {
			continue
		}
		if c, _ := Compare(r.Version, best.Version); !found || c > 0 {
			best, found = r, true
		}
	}
	return best, found
}

// Pick is the release to offer: the newest one that is not pulled, not skipped, newer than the
// running version and at least SettleDelay old (no wait when Immediate). A development build has no
// comparable version and is offered nothing.
func Pick(rels []Rel, pulls Pulls, o Options) (Rel, bool) {
	return newest(rels, func(r Rel) bool {
		if cmp, ok := Compare(r.Version, o.Running); !ok || cmp <= 0 {
			return false
		}
		if _, pulled := pulls.Find(r.Version); pulled || o.skipped(r.Version) {
			return false
		}
		return o.Immediate || o.Now.Sub(r.Published) >= SettleDelay
	})
}

// BackTo is the version to go back to from a pulled running version: the pull's own back_to, or
// when that is missing or itself pulled, the newest older release that is not pulled.
func BackTo(running string, rels []Rel, pulls Pulls) (string, bool) {
	if p, ok := pulls.Find(running); ok && p.BackTo != "" {
		if _, bad := pulls.Find(p.BackTo); !bad {
			return p.BackTo, true
		}
	}
	r, ok := newest(rels, func(r Rel) bool {
		c, known := Compare(r.Version, running)
		_, bad := pulls.Find(r.Version)
		return known && c < 0 && !bad
	})
	return r.Version, ok
}

// Item is one row of the Settings version list.
type Item struct {
	Version string `json:"version"`
	Date    string `json:"date"`
	Current bool   `json:"current,omitempty"`
}

// PulledRunning is what the red banner needs when the running version has been pulled.
type PulledRunning struct {
	Version string `json:"version"`
	Reason  string `json:"reason"`
	BackTo  string `json:"back_to"`
}

// Status is /api/update's answer. Available and Latest keep the shape of the older notice.
type Status struct {
	Available bool           `json:"available"`
	Latest    string         `json:"latest"`
	Published string         `json:"published,omitempty"`
	Notes     string         `json:"notes,omitempty"` // that version's CHANGELOG section; the caller fills it, never the release body
	Pulled    *PulledRunning `json:"pulled,omitempty"`
	Versions  []Item         `json:"versions,omitempty"`
}

// Compute turns the release list and the pull file into the overlay's update status.
func Compute(rels []Rel, pulls Pulls, o Options) Status {
	var st Status
	if r, ok := Pick(rels, pulls, o); ok {
		st.Available, st.Latest, st.Published = true, r.Version, r.Published.UTC().Format(time.RFC3339)
	}
	if p, ok := pulls.Find(o.Running); ok {
		pr := &PulledRunning{Version: bare(o.Running), Reason: p.Reason}
		pr.BackTo, _ = BackTo(o.Running, rels, pulls)
		st.Pulled = pr
	}
	running := bare(o.Running)
	if _, ok := parse(running); ok {
		cur := Item{Version: running, Current: true}
		for _, r := range rels {
			if r.Version == running {
				cur.Date = r.Published.UTC().Format("2006-01-02")
			}
		}
		st.Versions = append(st.Versions, cur)
	}
	sorted := append([]Rel(nil), rels...)
	sort.SliceStable(sorted, func(i, j int) bool { c, _ := Compare(sorted[i].Version, sorted[j].Version); return c > 0 })
	others := 0
	for _, r := range sorted {
		if _, pulled := pulls.Find(r.Version); pulled || r.Version == running || others == ListSize {
			continue
		}
		st.Versions = append(st.Versions, Item{Version: r.Version, Date: r.Published.UTC().Format("2006-01-02")})
		others++
	}
	return st
}

// Releases reads the published, non-draft releases that carry the Windows installer.
func Releases(ctx context.Context, client *http.Client, url string) ([]Rel, error) {
	b, err := get(ctx, client, url, "application/vnd.github+json", 4<<20)
	if err != nil {
		return nil, err
	}
	var raw []struct {
		Tag       string    `json:"tag_name"`
		Draft     bool      `json:"draft"`
		Published time.Time `json:"published_at"`
		Body      string    `json:"body"`
		Assets    []struct {
			Name string `json:"name"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("release list: %w", err)
	}
	var out []Rel
	for _, r := range raw {
		v := bare(r.Tag)
		if _, ok := parse(v); !ok || r.Draft || r.Published.IsZero() {
			continue
		}
		for _, a := range r.Assets {
			if a.Name == "WoW-Companion-Setup-"+v+"-x64.exe" {
				out = append(out, Rel{Version: v, Published: r.Published, Body: r.Body})
				break
			}
		}
	}
	return out, nil
}

// FetchPulls reads the pull file; any failure reads as nothing pulled.
func FetchPulls(ctx context.Context, client *http.Client, url string) Pulls {
	b, err := get(ctx, client, url, "application/json", 1<<20)
	if err != nil {
		return Pulls{}
	}
	return ParsePulls(b)
}

func get(ctx context.Context, client *http.Client, url, accept string, limit int64) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("User-Agent", "wowc-advisor")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("lookup: " + resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, limit))
}

// Source reads the release list and the pull file, and keeps the answer for a few minutes so the
// 4-hourly check, a wake-up and "Check now" together stay far inside GitHub's 60-an-hour limit.
type Source struct {
	Client            *http.Client
	ListURL, PullsURL string
	TTL               time.Duration // default 10 minutes
	mu                sync.Mutex
	at                time.Time
	rels              []Rel
	pulls             Pulls
}

// Get returns the releases and the pulls. fresh skips the kept answer unless it is under 30 s old.
func (s *Source) Get(ctx context.Context, fresh bool) ([]Rel, Pulls, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ttl := s.TTL
	if ttl == 0 {
		ttl = 10 * time.Minute
	}
	if fresh {
		ttl = 30 * time.Second
	}
	if !s.at.IsZero() && time.Since(s.at) < ttl {
		return s.rels, s.pulls, nil
	}
	rels, err := Releases(ctx, s.Client, s.ListURL)
	if err != nil {
		return nil, Pulls{}, err
	}
	s.rels, s.pulls, s.at = rels, FetchPulls(ctx, s.Client, s.PullsURL), time.Now()
	return s.rels, s.pulls, nil
}

// Changelog reads CHANGELOG.md as it stood at the tag of a version. url has one %s for the version.
func Changelog(ctx context.Context, client *http.Client, url, version string) (string, error) {
	b, err := get(ctx, client, fmt.Sprintf(url, bare(version)), "text/plain", 1<<20)
	return string(b), err
}

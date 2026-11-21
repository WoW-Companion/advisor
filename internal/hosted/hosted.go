// Package hosted is the knowledge-only mode: game-data tools answered from a dataset published at
// a URL, read over HTTP range requests, with no character tools and no capture access.
package hosted

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/WoW-Companion/advisor/internal/dataset"
	"github.com/WoW-Companion/advisor/internal/store"
)

// Limits bound what one request to the dataset host may cost.
type Limits struct {
	Timeout          time.Duration // per HTTP request, headers and body
	MaxResponseBytes int64         // per HTTP response body
	MaxConcurrent    int           // HTTP requests in flight at once
}

// DefaultLimits suit a static file host: tables are a few hundred kilobytes.
func DefaultLimits() Limits {
	return Limits{Timeout: 10 * time.Second, MaxResponseBytes: 8 << 20, MaxConcurrent: 8}
}

// ErrTooLarge means a response is bigger than Limits.MaxResponseBytes.
var ErrTooLarge = errors.New("response larger than the size cap")

// RequiredTables are the tables the knowledge tools read.
var RequiredTables = []string{"items", "quests", "npcs", "loot_sources", "instances", "instance_bosses", "recipes", "recipe_reagents", "quest_lines"}

// limitedTransport enforces the response size cap and the concurrency cap, and counts requests.
type limitedTransport struct {
	base http.RoundTripper
	max  int64
	sem  chan struct{}
	reqs atomic.Int64
}

func (t *limitedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	select {
	case t.sem <- struct{}{}:
	case <-req.Context().Done():
		return nil, req.Context().Err()
	}
	t.reqs.Add(1)
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		<-t.sem
		return nil, err
	}
	if resp.ContentLength > t.max {
		resp.Body.Close()
		<-t.sem
		return nil, fmt.Errorf("%s %s: %d bytes: %w (%d)", req.Method, req.URL.Redacted(), resp.ContentLength, ErrTooLarge, t.max)
	}
	resp.Body = &cappedBody{rc: resp.Body, left: t.max, release: func() { <-t.sem }}
	return resp, nil
}

// cappedBody fails the read that would pass the cap, and frees its concurrency slot on Close.
type cappedBody struct {
	rc      io.ReadCloser
	left    int64
	release func()
	closed  bool
}

func (b *cappedBody) Read(p []byte) (int, error) {
	if b.left < 0 {
		return 0, ErrTooLarge
	}
	n, err := b.rc.Read(p)
	b.left -= int64(n)
	if b.left < 0 {
		return n, ErrTooLarge
	}
	return n, err
}

func (b *cappedBody) Close() error {
	if !b.closed {
		b.closed = true
		b.release()
	}
	return b.rc.Close()
}

// Dataset is a remote dataset opened over range requests.
type Dataset struct {
	Base     string
	Manifest dataset.Manifest
	Store    *store.ParquetStore
	Limits   Limits
	tr       *limitedTransport
}

// Requests is the number of HTTP requests made to the dataset host so far.
func (d *Dataset) Requests() int64 { return d.tr.reqs.Load() }

// Open loads manifest.json from base, checks it, confirms every listed table is served with range
// support and a readable Parquet footer. Hashes are not checked:
// that would download every table; the manifest and tables must come over a trusted channel (HTTPS).
func Open(ctx context.Context, base string, lim Limits) (*Dataset, error) {
	u, err := url.Parse(base)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("dataset URL %q must be an http(s) URL", base)
	}
	base = strings.TrimRight(base, "/")
	def := DefaultLimits()
	if lim.Timeout <= 0 {
		lim.Timeout = def.Timeout
	}
	if lim.MaxResponseBytes <= 0 {
		lim.MaxResponseBytes = def.MaxResponseBytes
	}
	if lim.MaxConcurrent <= 0 {
		lim.MaxConcurrent = def.MaxConcurrent
	}
	tr := &limitedTransport{base: http.DefaultTransport, max: lim.MaxResponseBytes, sem: make(chan struct{}, lim.MaxConcurrent)}
	client := &http.Client{Transport: tr, Timeout: lim.Timeout}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/manifest.json", nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("manifest: %w", err)
	}
	data, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err == nil && resp.StatusCode != http.StatusOK {
		err = errors.New(resp.Status)
	}
	if err != nil {
		return nil, fmt.Errorf("manifest: %w", err)
	}
	m, err := dataset.ParseManifest(data)
	if err != nil {
		return nil, fmt.Errorf("manifest: %w", err)
	}
	listed := map[string]bool{}
	for _, t := range m.Tables {
		listed[strings.TrimSuffix(t.Path, ".parquet")] = true
	}
	for _, t := range RequiredTables {
		if !listed[t] {
			return nil, fmt.Errorf("manifest lists no %s table", t)
		}
	}

	st := store.New(store.HTTPSource{Base: base, Client: client})
	for _, t := range RequiredTables {
		if _, err := st.Column(t, firstColumn(t)); err != nil {
			return nil, fmt.Errorf("table %s: %w", t, err)
		}
	}
	return &Dataset{Base: u.Redacted(), Manifest: m, Store: st, Limits: lim, tr: tr}, nil
}

// firstColumn is a column every required table has, to prove its footer and one column read.
func firstColumn(table string) string {
	switch table {
	case "instances", "instance_bosses":
		return "map_id"
	case "recipes", "recipe_reagents":
		return "spell_id"
	case "loot_sources":
		return "item_id"
	case "quest_lines":
		return "line_id"
	}
	return "id"
}

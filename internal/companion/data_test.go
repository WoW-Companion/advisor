package companion

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/klauspost/compress/zstd"

	"github.com/WoW-Companion/advisor/internal/dataset"
)

const dataBuild = "1.60.1.71037"

// releaseServer is a fake data-repo release: a manifest and a tarball for dataBuild.
func releaseServer(t *testing.T, failing *atomic.Bool) dataset.Store {
	content := "item bytes"
	sum := sha256.Sum256([]byte(content))
	manifest, _ := json.Marshal(map[string]any{
		"dataset_version": "v1", "game_build": dataBuild,
		"tables": []map[string]string{{"path": "items.parquet", "sha256": hex.EncodeToString(sum[:])}},
	})
	var buf bytes.Buffer
	zw, _ := zstd.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	for name, c := range map[string]string{dataBuild + "/manifest.json": string(manifest), dataBuild + "/items.parquet": content} {
		tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(c)), Typeflag: tar.TypeReg})
		tw.Write([]byte(c))
	}
	tw.Close()
	zw.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if failing.Load() {
			http.Error(w, "boom", http.StatusServiceUnavailable)
			return
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/releases"):
			fmt.Fprintf(w, `[{"tag_name":"dataset-%s"}]`, dataBuild)
		case strings.HasSuffix(r.URL.Path, ".json"):
			w.Write(manifest)
		case strings.HasSuffix(r.URL.Path, ".tar.zst"):
			w.Write(buf.Bytes())
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return dataset.Store{Root: filepath.Join(t.TempDir(), "cache"), ReleasesURL: srv.URL + "/dl", APIURL: srv.URL + "/api/releases"}
}

func captureFor(build string) string {
	return fmt.Sprintf(`ForeverScribeDB = { client = { version = "1.60.1", build = %q }, chars = {
  ["R:New"] = { snapshot = { identity = { name = "New", realm = "R", class = "ROGUE" }, level = 20, capturedAt = 200 } },
} }
`, build)
}

func TestGameDataRow(t *testing.T) {
	var failing atomic.Bool
	failing.Store(true)
	store := releaseServer(t, &failing)
	sv := filepath.Join(t.TempDir(), "ForeverScribe.lua")
	write(t, sv, captureFor("71037"))
	s := New(Config{ForeverScribe: sv, Cache: store, AddonAPI: "http://127.0.0.1:1/x", App: true})
	s.port = "4317"
	h := s.Handler()
	call := func(method, path string) map[string]any {
		r := httptest.NewRequest(method, "http://127.0.0.1:4317"+path, nil)
		r.Header.Set(TokenHeader, s.Token)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		var m map[string]any
		json.Unmarshal(w.Body.Bytes(), &m)
		return m
	}
	row := func(m map[string]any) map[string]any {
		for _, it := range m["items"].([]any) {
			if it.(map[string]any)["id"] == "dataset" {
				return it.(map[string]any)
			}
		}
		t.Fatalf("no dataset row in %v", m)
		return nil
	}

	// Not downloaded yet: the row offers Download now, and NEXT says so.
	s.reload()
	if r := row(call("GET", "/api/setup")); r["status"] != "bad" || r["label"] != "Download now" || !strings.Contains(r["detail"].(string), "Not downloaded yet") {
		t.Errorf("not downloaded: %v", r)
	}
	if next := s.snapshot().Characters[0].Next; len(next) != 1 || next[0] != NoDataMessage {
		t.Errorf("NEXT without data: %q", next)
	}

	// A failed download reports a one-line reason and offers Retry.
	res := call("POST", "/api/setup/download-dataset")
	if res["error"] == nil {
		t.Errorf("failed download reported no error: %v", res)
	}
	if r := row(res); r["status"] != "bad" || r["label"] != "Retry" || !strings.HasPrefix(r["detail"].(string), "Download failed: ") || strings.Contains(r["detail"].(string), "\n") {
		t.Errorf("failed row: %v", r)
	}

	// Retry succeeds; the row turns green and advice uses the dataset.
	failing.Store(false)
	res = call("POST", "/api/setup/download-dataset")
	if res["ok"] != true {
		t.Fatalf("retry: %v", res)
	}
	if r := row(res); r["status"] != "ok" || r["detail"] != "Downloaded (build "+dataBuild+", matches your game)." || r["action"] != nil {
		t.Errorf("downloaded row: %v", r)
	}
	if s.datasetDir() != store.Dir(dataBuild) {
		t.Errorf("dataset in use: %q", s.datasetDir())
	}
	if next := s.snapshot().Characters[0].Next; len(next) == 1 && next[0] == NoDataMessage {
		t.Errorf("NEXT still says no data after the download")
	}

	// The game updates to another build: the old dataset is kept and the row warns.
	write(t, sv, captureFor("72000"))
	r := row(call("GET", "/api/setup"))
	if r["status"] != "warn" || !strings.Contains(r["detail"].(string), "Downloaded for build "+dataBuild+"; your game is build 1.60.1.72000;") {
		t.Errorf("mismatch row: %v", r)
	}

	// The cache folder goes missing: Download now restores it.
	write(t, sv, captureFor("71037"))
	os.RemoveAll(store.Root)
	if r := row(call("GET", "/api/setup")); r["status"] != "bad" {
		t.Errorf("missing cache: %v", r)
	}
	if res := call("POST", "/api/setup/download-dataset"); res["ok"] != true {
		t.Errorf("restore: %v", res)
	}
}

// republishServer is a fake data repo whose one release can be republished with a new revision.
type republishServer struct {
	rev     atomic.Value // string: the manifest revision served
	failing atomic.Bool
	hits    atomic.Int32 // manifest requests
	store   dataset.Store
}

func newRepublishServer(t *testing.T) *republishServer {
	rs := &republishServer{}
	rs.rev.Store("2026-01-01.1")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if rs.failing.Load() {
			http.Error(w, "boom", http.StatusServiceUnavailable)
			return
		}
		rev := rs.rev.Load().(string)
		content := "items " + rev
		sum := sha256.Sum256([]byte(content))
		manifest, _ := json.Marshal(map[string]any{
			"dataset_version": "v1", "game_build": dataBuild, "revision": rev,
			"tables": []map[string]string{{"path": "items.parquet", "sha256": hex.EncodeToString(sum[:])}},
		})
		switch {
		case strings.HasSuffix(r.URL.Path, "/releases"):
			fmt.Fprintf(w, `[{"tag_name":"dataset-%s"}]`, dataBuild)
		case strings.HasSuffix(r.URL.Path, ".json"):
			rs.hits.Add(1)
			w.Write(manifest)
		case strings.HasSuffix(r.URL.Path, ".tar.zst"):
			var buf bytes.Buffer
			zw, _ := zstd.NewWriter(&buf)
			tw := tar.NewWriter(zw)
			for name, c := range map[string]string{dataBuild + "/manifest.json": string(manifest), dataBuild + "/items.parquet": content} {
				tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(c)), Typeflag: tar.TypeReg})
				tw.Write([]byte(c))
			}
			tw.Close()
			zw.Close()
			w.Write(buf.Bytes())
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	rs.store = dataset.Store{Root: filepath.Join(t.TempDir(), "cache"), ReleasesURL: srv.URL + "/dl", APIURL: srv.URL + "/api/releases"}
	return rs
}

func TestDatasetRepublish(t *testing.T) {
	rs := newRepublishServer(t)
	s, _, digestOut := digestEnv(t, true, true)
	s.cfg.Cache = rs.store
	s.cfg.ForeverScribe = filepath.Join(filepath.Dir(s.cfg.ForeverScribe), "ForeverScribe.lua")
	write(t, s.cfg.ForeverScribe, captureFor("71037"))
	clock := time.Unix(1_000_000, 0)
	s.cfg.Now = func() time.Time { return clock }

	if _, err := rs.store.Ensure(context.Background(), "71037", false); err != nil {
		t.Fatal(err)
	}
	items := func() string { return rs.store.Dir(dataBuild) + "/items.parquet" }
	read := func() string { b, _ := os.ReadFile(items()); return string(b) }
	ctx := context.Background()

	// The same revision does nothing: no re-download, no notice, no digest.
	if s.checkDatasetUpdate(ctx) || s.dataNotice() != "" || read() != "items 2026-01-01.1" {
		t.Fatalf("same revision changed something: %q", read())
	}
	if _, err := os.Stat(digestOut); err == nil {
		t.Error("digest written without an update")
	}

	// The 24-hour throttle: a republish inside the day is not looked at.
	rs.rev.Store("2026-02-02.1")
	hits := rs.hits.Load()
	clock = clock.Add(23 * time.Hour)
	if s.checkDatasetUpdate(ctx) || rs.hits.Load() != hits {
		t.Fatal("checked again inside 24 hours")
	}

	// Offline keeps the cache.
	clock = clock.Add(2 * time.Hour)
	rs.failing.Store(true)
	if s.checkDatasetUpdate(ctx) || read() != "items 2026-01-01.1" {
		t.Fatalf("offline touched the cache: %q", read())
	}
	rs.failing.Store(false)

	// A new revision of the same build replaces the cache, tells the player and rewrites the digest.
	clock = clock.Add(25 * time.Hour)
	if !s.checkDatasetUpdate(ctx) {
		t.Fatal("republished dataset not picked up")
	}
	if read() != "items 2026-02-02.1" {
		t.Errorf("cache not replaced: %q", read())
	}
	if s.dataNotice() != "Game data updated (talent builds and more)" || s.snapshot().DataNotice != s.dataNotice() {
		t.Errorf("notice %q", s.dataNotice())
	}
	if b, err := os.ReadFile(digestOut); err != nil || !strings.Contains(string(b), "New") {
		t.Errorf("digest not rewritten: %v", err)
	}
	if got := s.datasetItem().Detail; !strings.Contains(got, "updated 2026-02-02") {
		t.Errorf("row lacks the revision date: %q", got)
	}
}

func TestDatasetUpdateSwitchesAndNeverDowngrades(t *testing.T) {
	rs := newRepublishServer(t) // publishes dataBuild only
	s, _, _ := digestEnv(t, true, true)
	s.cfg.Cache = rs.store
	s.cfg.ForeverScribe = filepath.Join(filepath.Dir(s.cfg.ForeverScribe), "ForeverScribe.lua")
	write(t, s.cfg.ForeverScribe, captureFor("71037"))
	clock := time.Unix(1_000_000, 0)
	s.cfg.Now = func() time.Time { return clock }
	ctx := context.Background()

	// Installed an older build than the game's: the matching one is downloaded and switched to.
	old := filepath.Join(rs.store.Root, "1.59.0.1")
	s.data.dir = old
	if !s.checkDatasetUpdate(ctx) || s.datasetDir() != rs.store.Dir(dataBuild) {
		t.Fatalf("did not switch: %q", s.datasetDir())
	}
	if want := UpdatedNotice(dataBuild); s.dataNotice() != want {
		t.Errorf("notice %q, want %q", s.dataNotice(), want)
	}
	if s.data.prev != old {
		t.Errorf("previous dataset not remembered: %q", s.data.prev)
	}

	// A game on an older build than the dataset in use never downgrades it.
	write(t, s.cfg.ForeverScribe, captureFor("71000"))
	clock = clock.Add(25 * time.Hour)
	s.checkDatasetUpdate(ctx)
	if s.datasetDir() != rs.store.Dir(dataBuild) {
		t.Errorf("downgraded to %q", s.datasetDir())
	}
}

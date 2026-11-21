package companion

import (
	"archive/tar"
	"bytes"
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

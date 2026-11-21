package dataset

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

	"github.com/klauspost/compress/zstd"
)

const build = "1.60.1.71037"

var files = map[string]string{"items.parquet": "item bytes", "sub/spells.parquet": "spell bytes"}

func manifestFor(b string, files map[string]string) []byte {
	type table struct {
		Path   string `json:"path"`
		SHA256 string `json:"sha256"`
	}
	m := struct {
		Version string  `json:"dataset_version"`
		Build   string  `json:"game_build"`
		Tables  []table `json:"tables"`
	}{Version: "v1", Build: b}
	for p, c := range files {
		sum := sha256.Sum256([]byte(c))
		m.Tables = append(m.Tables, table{p, hex.EncodeToString(sum[:])})
	}
	data, _ := json.Marshal(m)
	return data
}

// archive makes a .tar.zst of `<b>/<name>` entries.
func archive(t *testing.T, b string, entries map[string]string) []byte {
	var buf bytes.Buffer
	zw, _ := zstd.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	for name, c := range entries {
		tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(c)), Typeflag: tar.TypeReg})
		tw.Write([]byte(c))
	}
	tw.Close()
	zw.Close()
	return buf.Bytes()
}

func goodEntries(manifest []byte) map[string]string {
	e := map[string]string{build + "/manifest.json": string(manifest)}
	for p, c := range files {
		e[build+"/"+p] = c
	}
	return e
}

type fake struct {
	manifest, tarball []byte
	truncate          bool
	hits              atomic.Int64
}

func (f *fake) serve(t *testing.T) Store {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.hits.Add(1)
		switch {
		case strings.HasSuffix(r.URL.Path, "/latest"):
			fmt.Fprintf(w, `{"tag_name":"dataset-%s"}`, build)
		case strings.HasSuffix(r.URL.Path, "/releases"):
			fmt.Fprintf(w, `[{"tag_name":"dataset-1.59.0.1"},{"tag_name":"dataset-%s"}]`, build)
		case strings.HasSuffix(r.URL.Path, ".json"):
			w.Write(f.manifest)
		case strings.HasSuffix(r.URL.Path, ".tar.zst"):
			if f.truncate {
				w.Header().Set("Content-Length", fmt.Sprint(len(f.tarball)))
				w.Write(f.tarball[:len(f.tarball)/2])
				return
			}
			w.Write(f.tarball)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return Store{Root: filepath.Join(t.TempDir(), "cache"), ReleasesURL: srv.URL + "/dl", APIURL: srv.URL + "/api/releases"}
}

func newFake(t *testing.T) *fake {
	m := manifestFor(build, files)
	return &fake{manifest: m, tarball: archive(t, build, goodEntries(m))}
}

func TestEnsureDownloadAndOfflineReuse(t *testing.T) {
	f := newFake(t)
	st := f.serve(t)
	res, err := st.Ensure(context.Background(), "71037", false)
	if err != nil || !res.Downloaded || res.Build != build {
		t.Fatalf("first: %+v %v", res, err)
	}
	if b, _ := os.ReadFile(filepath.Join(res.Dir, "sub/spells.parquet")); string(b) != "spell bytes" {
		t.Fatalf("unpacked %q", b)
	}
	entries, _ := os.ReadDir(st.Root)
	if len(entries) != 1 {
		t.Fatalf("cache holds %v", entries)
	}
	hits := f.hits.Load()
	st.ReleasesURL, st.APIURL = "http://127.0.0.1:1", "http://127.0.0.1:1" // network gone
	for _, want := range []string{build, "71037", ""} {
		res, err := st.Ensure(context.Background(), want, false)
		if err != nil || res.Downloaded || res.Build != build || res.Offline != nil {
			t.Fatalf("offline %q: %+v %v", want, res, err)
		}
	}
	// an explicit update with the network down falls back to the cache and says why
	res, err = st.Ensure(context.Background(), "", true)
	if err != nil || res.Offline == nil || res.Build != build {
		t.Fatalf("update offline: %+v %v", res, err)
	}
	if f.hits.Load() != hits {
		t.Fatal("offline reuse touched the server")
	}
}

func TestEnsureFailures(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(t *testing.T, f *fake)
	}{
		{"hash mismatch", func(t *testing.T, f *fake) {
			e := goodEntries(f.manifest)
			e[build+"/items.parquet"] = "tampered!!"
			f.tarball = archive(t, build, e)
		}},
		{"interrupted download", func(_ *testing.T, f *fake) { f.truncate = true }},
		{"unlisted file", func(t *testing.T, f *fake) {
			e := goodEntries(f.manifest)
			e[build+"/extra.bin"] = "x"
			f.tarball = archive(t, build, e)
		}},
		{"path traversal", func(t *testing.T, f *fake) {
			e := goodEntries(f.manifest)
			e[build+"/../evil"] = "x"
			f.tarball = archive(t, build, e)
		}},
		{"missing file", func(t *testing.T, f *fake) {
			e := goodEntries(f.manifest)
			delete(e, build+"/items.parquet")
			f.tarball = archive(t, build, e)
		}},
		{"manifest differs", func(t *testing.T, f *fake) {
			e := goodEntries(f.manifest)
			e[build+"/manifest.json"] += " "
			f.tarball = archive(t, build, e)
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newFake(t)
			c.mutate(t, f)
			st := f.serve(t)
			if _, err := st.Ensure(context.Background(), build, false); err == nil {
				t.Fatal("expected a refusal")
			}
			entries, _ := os.ReadDir(st.Root)
			if len(entries) != 0 {
				t.Fatalf("cache touched: %v", entries)
			}
		})
	}
}

func TestFailedUpdateKeepsCache(t *testing.T) {
	f := newFake(t)
	st := f.serve(t)
	if _, err := st.Ensure(context.Background(), build, false); err != nil {
		t.Fatal(err)
	}
	// corrupt the cache, then serve a tampered archive: the bad cache is not replaced by it
	os.WriteFile(filepath.Join(st.Dir(build), "items.parquet"), []byte("rot"), 0o644)
	e := goodEntries(f.manifest)
	e[build+"/items.parquet"] = "tampered!!"
	f.tarball = archive(t, build, e)
	if _, err := st.Ensure(context.Background(), build, false); err == nil {
		t.Fatal("expected refusal")
	}
	if b, _ := os.ReadFile(filepath.Join(st.Dir(build), "items.parquet")); string(b) != "rot" {
		t.Fatalf("cache replaced: %q", b)
	}
	if good, bad := st.Cached(); len(good) != 0 || len(bad) != 1 {
		t.Fatalf("cached %v %v", good, bad)
	}
}

func TestMatches(t *testing.T) {
	for _, c := range []struct {
		game, ver, build string
		want             bool
	}{
		{"1.60.1.71037", "1.60.1", "71037", true},
		{"1.60.1.71037", "", "71037", true},
		{"1.60.1.71037", "1.60.1", "71038", false},
		{"1.60.1.71037", "1.60.1", "1037", false},
		{"1.60.1.71037", "", "", false},
		{"71037", "", "71037", true},
	} {
		if got := Matches(c.game, c.ver, c.build); got != c.want {
			t.Errorf("Matches(%q,%q,%q)=%v", c.game, c.ver, c.build, got)
		}
	}
}

func TestBuildOrder(t *testing.T) {
	if !buildLess("1.9.0.1", "1.10.0.1") || buildLess("1.60.1.71037", "1.60.1.71037") {
		t.Fatal("numeric order")
	}
	if pick([]string{"1.60.1.2", "1.60.1.1"}, "1") != "1.60.1.1" || pick([]string{"2.0.0.5"}, "") != "2.0.0.5" {
		t.Fatal("pick")
	}
}

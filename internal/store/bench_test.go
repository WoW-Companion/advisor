package store

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

const benchRows = 32000

// benchStores builds the synthetic table once per benchmark and returns a local and an HTTP store.
func benchStores(b *testing.B) (local, remote *ParquetStore) {
	b.Helper()
	dir := b.TempDir()
	data := writeParquet(b, synthItems(benchRows), 16384)
	if err := os.WriteFile(filepath.Join(dir, "items.parquet"), data, 0o644); err != nil {
		b.Fatal(err)
	}
	srv := httptest.NewServer(http.FileServer(http.Dir(dir)))
	b.Cleanup(srv.Close)
	return NewDir(dir), New(HTTPSource{Base: srv.URL})
}

func benchLookup(b *testing.B, s *ParquetStore) {
	if _, err := s.Get("items", 1); err != nil { // open the file outside the timed loop
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		id := int64(i*7919%benchRows + 1)
		if _, err := s.Get("items", id); err != nil {
			b.Fatal(err)
		}
	}
}

func benchRange(b *testing.B, s *ParquetStore) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		rows, err := s.Filter("items", Range{"id", int64(20000), int64(20099)})
		if err != nil || len(rows) != 100 {
			b.Fatal(len(rows), err)
		}
	}
}

func BenchmarkGetByID_Local(b *testing.B) {
	local, _ := benchStores(b)
	benchLookup(b, local)
}

func BenchmarkGetByID_HTTPRange(b *testing.B) {
	_, remote := benchStores(b)
	benchLookup(b, remote)
}

func BenchmarkRangeFilter_Local(b *testing.B) {
	local, _ := benchStores(b)
	benchRange(b, local)
}

func BenchmarkRangeFilter_HTTPRange(b *testing.B) {
	_, remote := benchStores(b)
	benchRange(b, remote)
}

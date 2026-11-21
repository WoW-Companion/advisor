package store

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/parquet-go/parquet-go"
)

type item struct {
	ID      int64  `parquet:"id"`
	Name    string `parquet:"name"`
	Level   int32  `parquet:"level"`
	Quality string `parquet:"quality"`
}

// synthItems is a synthetic table: n rows, ids 1..n, levels cycling 1..60.
func synthItems(n int) []item {
	rows := make([]item, n)
	for i := range rows {
		id := int64(i + 1)
		rows[i] = item{id, fmt.Sprintf("Synthetic Item %d", id), int32(i%60 + 1), []string{"common", "rare"}[i%2]}
	}
	return rows
}

// writeParquet encodes rows with a Bloom filter on id and the given row group size.
func writeParquet(tb testing.TB, rows []item, groupRows int64) []byte {
	tb.Helper()
	var buf bytes.Buffer
	w := parquet.NewGenericWriter[item](&buf,
		parquet.MaxRowsPerRowGroup(groupRows),
		parquet.BloomFilters(parquet.SplitBlockFilter(10, "id")),
	)
	if _, err := w.Write(rows); err != nil {
		tb.Fatal(err)
	}
	if err := w.Close(); err != nil {
		tb.Fatal(err)
	}
	return buf.Bytes()
}

func memSource(b []byte) ReaderSource {
	return ReaderSource{"items": {ReaderAt: bytes.NewReader(b), Size: int64(len(b))}}
}

func TestDataStore(t *testing.T) {
	data := writeParquet(t, synthItems(100), 30) // four row groups
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "items.parquet"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.FileServer(http.Dir(dir)))
	defer srv.Close()

	stores := map[string]DataStore{
		"dir":    NewDir(dir),
		"reader": New(memSource(data)),
		"http":   New(HTTPSource{Base: srv.URL}),
	}
	for name, s := range stores {
		t.Run(name, func(t *testing.T) {
			for _, id := range []int64{1, 31, 100} {
				row, err := s.Get("items", id)
				if err != nil || row["id"] != id || row["name"] != fmt.Sprintf("Synthetic Item %d", id) {
					t.Errorf("Get(%d) = %v, %v", id, row, err)
				}
			}
			if _, err := s.Get("items", 101); !errors.Is(err, ErrNotFound) {
				t.Errorf("Get(101) err = %v, want ErrNotFound", err)
			}
			if _, err := s.Get("nothing", 1); !errors.Is(err, ErrNotFound) {
				t.Errorf("missing table err = %v", err)
			}
			rows, err := s.Filter("items", Range{"id", int64(25), int64(35)}, Range{"quality", "rare", "rare"})
			if err != nil || len(rows) != 5 { // even ids' rows i%2==1 -> ids 26,28,30,32,34
				t.Errorf("Filter = %d rows, %v", len(rows), err)
			}
			rows, _ = s.Filter("items", Range{Column: "level", Min: 60})
			if len(rows) != 1 || rows[0]["id"] != int64(60) {
				t.Errorf("open-ended Filter = %v", rows)
			}
			rows, err = s.SearchName("items", "item 9", 5)
			if err != nil || len(rows) != 5 {
				t.Errorf("SearchName = %d rows, %v", len(rows), err)
			}
		})
	}
}

func TestRangeHolds(t *testing.T) {
	cases := []struct {
		r    Range
		v    any
		want bool
	}{
		{Range{"x", 1, 5}, int64(3), true},
		{Range{"x", 1, 5}, int64(6), false},
		{Range{"x", nil, 5}, int64(-9), true},
		{Range{"x", "b", "d"}, "c", true},
		{Range{"x", "b", "d"}, "e", false},
		{Range{"x", 1, nil}, nil, false},
		{Range{"x", 1, nil}, "text", false},
	}
	for _, c := range cases {
		if got := c.r.holds(c.v); got != c.want {
			t.Errorf("%v.holds(%v) = %v", c.r, c.v, got)
		}
	}
}

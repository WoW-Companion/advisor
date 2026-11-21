// Package store is the data-access seam: a DataStore interface and its parquet-go implementation,
// reading Parquet tables from a directory or from any io.ReaderAt (for example an HTTP range
// reader).
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/parquet-go/parquet-go"
)

// ErrNotFound is returned by Get when no row has the id, and by table lookups for a missing table.
var ErrNotFound = errors.New("not found")

// Row is one table row by column name. Integers are int64, floats float64, text string,
// booleans bool, nulls nil.
type Row map[string]any

// Range keeps rows whose Column lies in [Min, Max]; a nil bound is open. Bounds are numbers or
// strings, matching the column's type.
type Range struct {
	Column   string
	Min, Max any
}

// DataStore is read access to game data tables. Every table has a numeric `id` column; text
// search uses the `name` column.
type DataStore interface {
	// Get returns the row whose id column equals id, or an error wrapping ErrNotFound.
	Get(table string, id int64) (Row, error)
	// Filter returns the rows where every range holds, in file order.
	Filter(table string, ranges ...Range) ([]Row, error)
	// SearchName returns up to limit rows whose name contains text, case-insensitively.
	SearchName(table, text string, limit int) ([]Row, error)
}

// Source opens a table's bytes.
type Source interface {
	Open(table string) (io.ReaderAt, int64, error)
}

var identRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// DirSource reads `<dir>/<table>.parquet`.
type DirSource string

func (d DirSource) Open(table string) (io.ReaderAt, int64, error) {
	if !identRE.MatchString(table) {
		return nil, 0, fmt.Errorf("invalid table name %q", table)
	}
	f, err := os.Open(filepath.Join(string(d), table+".parquet"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, 0, fmt.Errorf("table %s: %w", table, ErrNotFound)
	}
	if err != nil {
		return nil, 0, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, 0, err
	}
	return f, st.Size(), nil
}

// ReaderSource serves tables from memory or any other io.ReaderAt, by table name.
type ReaderSource map[string]SizedReaderAt

// SizedReaderAt is an io.ReaderAt that knows its length.
type SizedReaderAt struct {
	io.ReaderAt
	Size int64
}

func (s ReaderSource) Open(table string) (io.ReaderAt, int64, error) {
	r, ok := s[table]
	if !ok {
		return nil, 0, fmt.Errorf("table %s: %w", table, ErrNotFound)
	}
	return r.ReaderAt, r.Size, nil
}

// ParquetStore is a DataStore over a Source.
type ParquetStore struct {
	src   Source
	mu    sync.Mutex
	files map[string]*parquet.File
}

func New(src Source) *ParquetStore {
	return &ParquetStore{src: src, files: map[string]*parquet.File{}}
}

// NewDir reads `<dir>/<table>.parquet`.
func NewDir(dir string) *ParquetStore { return New(DirSource(dir)) }

func (s *ParquetStore) open(table string) (*parquet.File, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if f, ok := s.files[table]; ok {
		return f, nil
	}
	r, size, err := s.src.Open(table)
	if err != nil {
		return nil, err
	}
	f, err := parquet.OpenFile(r, size)
	if err != nil {
		return nil, fmt.Errorf("table %s: %w", table, err)
	}
	s.files[table] = f
	return f, nil
}

func columnIndex(f *parquet.File, name string) (int, error) {
	for i, c := range f.Schema().Columns() {
		if len(c) == 1 && c[0] == name {
			return i, nil
		}
	}
	return 0, fmt.Errorf("no such column: %s", name)
}

// number and text view a parquet value as a comparable.
func scalar(v parquet.Value) any {
	if v.IsNull() {
		return nil
	}
	switch v.Kind() {
	case parquet.Boolean:
		return v.Boolean()
	case parquet.Int32:
		return int64(v.Int32())
	case parquet.Int64:
		return v.Int64()
	case parquet.Float:
		return float64(v.Float())
	case parquet.Double:
		return v.Double()
	default:
		return string(v.ByteArray())
	}
}

func asFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case float32:
		return float64(n), true
	case float64:
		return n, true
	}
	return 0, false
}

// cmp orders two scalars; ok is false when they are not comparable.
func cmp(a, b any) (int, bool) {
	if x, ok := asFloat(a); ok {
		if y, ok := asFloat(b); ok {
			switch {
			case x < y:
				return -1, true
			case x > y:
				return 1, true
			}
			return 0, true
		}
		return 0, false
	}
	if x, ok := a.(string); ok {
		if y, ok := b.(string); ok {
			return strings.Compare(x, y), true
		}
	}
	return 0, false
}

func (r Range) holds(v any) bool {
	if v == nil {
		return false
	}
	if r.Min != nil {
		if c, ok := cmp(v, r.Min); !ok || c < 0 {
			return false
		}
	}
	if r.Max != nil {
		if c, ok := cmp(v, r.Max); !ok || c > 0 {
			return false
		}
	}
	return true
}

// mayOverlap reports whether the row group's column index leaves room for rows in r. It answers
// true when the file holds no usable statistics.
func (r Range) mayOverlap(chunk parquet.ColumnChunk) bool {
	ci, err := chunk.ColumnIndex()
	if err != nil || ci == nil || ci.NumPages() == 0 {
		return true
	}
	for p := 0; p < ci.NumPages(); p++ {
		if ci.NullPage(p) {
			continue
		}
		lo, hi := scalar(ci.MinValue(p)), scalar(ci.MaxValue(p))
		if r.Max != nil {
			if c, ok := cmp(lo, r.Max); ok && c > 0 {
				continue
			}
		}
		if r.Min != nil {
			if c, ok := cmp(hi, r.Min); ok && c < 0 {
				continue
			}
		}
		return true
	}
	return false
}

// scan walks one column chunk and returns the row numbers (within the group) where keep holds,
// stopping after limit hits (limit <= 0 means all).
func scan(chunk parquet.ColumnChunk, keep func(any) bool, limit int) ([]int64, error) {
	var hits []int64
	pages := chunk.Pages()
	defer pages.Close()
	buf := make([]parquet.Value, 1024)
	var row int64
	for {
		page, err := pages.ReadPage()
		if err == io.EOF {
			return hits, nil
		}
		if err != nil {
			return nil, err
		}
		vr := page.Values()
		for {
			n, err := vr.ReadValues(buf)
			for _, v := range buf[:n] {
				if keep(scalar(v)) {
					hits = append(hits, row)
					if limit > 0 && len(hits) >= limit {
						parquet.Release(page)
						return hits, nil
					}
				}
				row++
			}
			if err != nil {
				break
			}
		}
		parquet.Release(page)
	}
}

// readRows reads the given rows (ascending) of a row group.
func readRows(f *parquet.File, rg parquet.RowGroup, rows []int64) ([]Row, error) {
	if len(rows) == 0 {
		return nil, nil
	}
	cols := f.Schema().Columns()
	r := rg.Rows()
	defer r.Close()
	out := make([]Row, 0, len(rows))
	buf := make([]parquet.Row, 1)
	for _, want := range rows {
		if err := r.SeekToRow(want); err != nil {
			return nil, err
		}
		n, err := r.ReadRows(buf)
		if n == 0 {
			if err == nil {
				err = io.ErrUnexpectedEOF
			}
			return nil, err
		}
		row := Row{}
		for _, v := range buf[0] {
			row[cols[v.Column()][0]] = scalar(v)
		}
		out = append(out, row)
	}
	return out, nil
}

// query finds rows by testing one column, then reads the hits; ranges may prune row groups.
func (s *ParquetStore) query(table, column string, keep func(any) bool, prune *Range, limit int) ([]Row, error) {
	f, err := s.open(table)
	if err != nil {
		return nil, err
	}
	ci, err := columnIndex(f, column)
	if err != nil {
		return nil, err
	}
	var out []Row
	for _, rg := range f.RowGroups() {
		chunk := rg.ColumnChunks()[ci]
		if prune != nil && !prune.mayOverlap(chunk) {
			continue
		}
		if prune != nil {
			if bf := chunk.BloomFilter(); bf != nil && prune.Min != nil && prune.Min == prune.Max {
				if ok, err := bf.Check(bloomValue(chunk, prune.Min)); err == nil && !ok {
					continue
				}
			}
		}
		left := 0
		if limit > 0 {
			left = limit - len(out)
		}
		hits, err := scan(chunk, keep, left)
		if err != nil {
			return nil, err
		}
		rows, err := readRows(f, rg, hits)
		if err != nil {
			return nil, err
		}
		out = append(out, rows...)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (s *ParquetStore) Get(table string, id int64) (Row, error) {
	rows, err := s.query(table, "id", func(v any) bool { return v == id }, &Range{"id", id, id}, 1)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("%s id %d: %w", table, id, ErrNotFound)
	}
	return rows[0], nil
}

func (s *ParquetStore) Filter(table string, ranges ...Range) ([]Row, error) {
	if len(ranges) == 0 {
		return nil, errors.New("filter needs at least one range")
	}
	// Scan the first range's column; the rest are checked on the rows it returns.
	rows, err := s.query(table, ranges[0].Column, ranges[0].holds, &ranges[0], 0)
	if err != nil {
		return nil, err
	}
	out := rows[:0]
next:
	for _, row := range rows {
		for _, r := range ranges[1:] {
			if _, ok := row[r.Column]; !ok {
				return nil, fmt.Errorf("no such column: %s", r.Column)
			}
			if !r.holds(row[r.Column]) {
				continue next
			}
		}
		out = append(out, row)
	}
	return out, nil
}

func (s *ParquetStore) SearchName(table, text string, limit int) ([]Row, error) {
	needle := strings.ToLower(text)
	return s.query(table, "name", func(v any) bool {
		name, ok := v.(string)
		return ok && strings.Contains(strings.ToLower(name), needle)
	}, nil, limit)
}

// ReadManifest reads `<dir>/manifest.json`; it returns nil, nil when there is none.
func ReadManifest(dir string) (map[string]any, error) {
	data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("manifest.json: %w", err)
	}
	return m, nil
}

// bloomValue converts an id bound to the chunk's physical type, which the filter hashes.
func bloomValue(chunk parquet.ColumnChunk, v any) parquet.Value {
	if n, ok := v.(int64); ok && chunk.Type().Kind() == parquet.Int32 {
		return parquet.Int32Value(int32(n))
	}
	return parquet.ValueOf(v)
}

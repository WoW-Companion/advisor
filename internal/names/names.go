// Package names resolves item, creature, recipe and instance names through the dataset's
// names.parquet index (see the data repository's docs/schema.md and docs/layout.md), reading only
// the row groups whose norm range and Bloom filter admit the name. A dataset without the index,
// and a table the index does not list (quests), are searched by scanning the table's name column.
package names

import (
	"errors"
	"log"
	"strconv"
	"strings"
	"unicode"

	"github.com/parquet-go/parquet-go"
	"golang.org/x/text/unicode/norm"

	"github.com/WoW-Companion/advisor/internal/store"
)

// Tables the index lists, as its table column spells them.
const (
	Items     = "items"
	Npcs      = "npcs"
	Instances = "instances"
	Recipes   = "recipes"
	ItemSets  = "item_sets"
)

// indexTable is the dataset table that holds the index.
const indexTable = "names"

var indexed = map[string]bool{Items: true, Npcs: true, Instances: true, Recipes: true, ItemSets: true}

// Match is one named row: its id in the table (items.id, npcs.id, instances.map_id,
// recipes.spell_id, item_sets.set_id) and its name as the table spells it.
type Match struct {
	ID   int64
	Name string
}

// Normalize lowercases a name, folds accents (é to e), drops apostrophes and turns every other run
// of non-letters and non-digits into one space, trimmed at both ends. It is how names.parquet's
// norm column is built.
func Normalize(s string) string {
	var b strings.Builder
	space := false
	for _, r := range norm.NFD.String(strings.ToLower(s)) {
		switch {
		case unicode.Is(unicode.Mn, r):
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if space && b.Len() > 0 {
				b.WriteByte(' ')
			}
			space = false
			b.WriteRune(r)
		case r == '\'' || r == '’':
		default:
			space = true
		}
	}
	return b.String()
}

// fileStore is a DataStore that can hand out a table's Parquet file; store.ParquetStore is one.
type fileStore interface {
	OpenFile(table string) (*parquet.File, error)
}

// Find returns up to limit rows of table named text: those whose normalised name equals the
// normalised text, else those that start with it, in index order. more is true when the limit cut
// the list. Text with no letter or digit finds nothing.
func Find(ds store.DataStore, table, text string, limit int) (hits []Match, more bool, err error) {
	q := Normalize(text)
	if q == "" || limit <= 0 {
		return nil, false, nil
	}
	if fs, ok := ds.(fileStore); ok && indexed[table] {
		f, err := fs.OpenFile(indexTable)
		if err == nil {
			return lookup(f, table, q, limit)
		}
		if !errors.Is(err, store.ErrNotFound) {
			return nil, false, err
		}
		log.Printf("names: dataset has no names.parquet; scanning the %s table by name", table)
	}
	return scan(ds, table, text, q, limit)
}

// scan is the fallback for a dataset without the index and for a table it does not list: the
// table's name column, exact matches before partial ones.
func scan(ds store.DataStore, table, text, q string, limit int) ([]Match, bool, error) {
	rows, err := ds.SearchName(table, strings.TrimSpace(text), limit+1)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, false, nil
		}
		return nil, false, err
	}
	var exact, all []Match
	for _, r := range rows {
		id, _ := r["id"].(int64)
		if id == 0 { // recipes are keyed by spell_id
			id, _ = r["spell_id"].(int64)
		}
		name, _ := r["name"].(string)
		m := Match{id, name}
		all = append(all, m)
		if Normalize(name) == q {
			exact = append(exact, m)
		}
	}
	if len(exact) > 0 {
		all = exact
	}
	if len(all) > limit {
		return all[:limit], true, nil
	}
	return all, false, nil
}

// lookup reads the index: an exact norm match, or when there is none a prefix match.
func lookup(f *parquet.File, table, q string, limit int) ([]Match, bool, error) {
	for _, prefix := range []bool{false, true} {
		var hits []Match
		more := false
		err := read(f, q, prefix, func(r row) bool {
			if r.Table != table {
				return true
			}
			if len(hits) == limit {
				more = true
				return false
			}
			hits = append(hits, Match{int64(r.ID), r.Name})
			return true
		})
		if err != nil || len(hits) > 0 {
			return hits, more, err
		}
	}
	return nil, false, nil
}

type row struct {
	Norm  string
	Name  string
	Table string
	ID    int32
}

// read calls yield for the rows whose norm equals q (exact) or starts with q (prefix), in file
// order, until it returns false. The file is sorted by norm, so a group whose min/max range cannot
// hold q is skipped from the footer alone, and for an exact lookup a group whose Bloom filter
// refuses q is skipped too; reading stops at the first row past q.
func read(f *parquet.File, q string, prefix bool, yield func(row) bool) error {
	meta := f.Metadata().RowGroups
	for i, rg := range f.RowGroups() {
		st := meta[i].Columns[0].MetaData.Statistics
		lo, hi := string(st.MinValue), string(st.MaxValue)
		if hi < q || (lo > q && !strings.HasPrefix(lo, q)) {
			continue
		}
		if bf := rg.ColumnChunks()[0].BloomFilter(); bf != nil && !prefix {
			if hit, err := bf.Check(parquet.ByteArrayValue([]byte(q))); err != nil {
				return err
			} else if !hit {
				continue
			}
		}
		rows := rg.Rows()
		buf := make([]parquet.Row, 64)
		for {
			n, err := rows.ReadRows(buf)
			for _, pr := range buf[:n] {
				r := row{string(pr[0].ByteArray()), string(pr[1].ByteArray()), string(pr[2].ByteArray()), pr[3].Int32()}
				switch {
				case r.Norm == q || prefix && strings.HasPrefix(r.Norm, q):
					if !yield(r) {
						rows.Close()
						return nil
					}
				case r.Norm > q && !strings.HasPrefix(r.Norm, q):
					rows.Close()
					return nil
				}
			}
			if err != nil {
				break
			}
		}
		rows.Close()
	}
	return nil
}

// Resolve turns a name into one row of table: a name that matches one row gives its id and
// name; otherwise it returns the candidates as "name (id)" and id 0, none when nothing matches.
func Resolve(ds store.DataStore, table, text string) (id int64, name string, candidates []string) {
	hits, _, err := Find(ds, table, text, 10)
	if err != nil || len(hits) == 0 {
		return 0, "", nil
	}
	if len(hits) == 1 {
		return hits[0].ID, hits[0].Name, nil
	}
	for _, h := range hits {
		candidates = append(candidates, h.Name+" ("+strconv.FormatInt(h.ID, 10)+")")
	}
	return 0, "", candidates
}

package names

import (
	"bytes"
	"fmt"
	"log"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/parquet-go/parquet-go"

	"github.com/WoW-Companion/advisor/internal/store"
)

type nameRow struct {
	Norm  string `parquet:"norm"`
	Name  string `parquet:"name"`
	Table string `parquet:"table,dict"`
	ID    int32  `parquet:"id"`
}

type itemRow struct {
	ID   int64  `parquet:"id"`
	Name string `parquet:"name"`
}

type npcRow struct {
	ID   int64  `parquet:"id"`
	Name string `parquet:"name"`
}

type setRow struct {
	SetID  int64  `parquet:"set_id"`
	Name   string `parquet:"name"`
	ItemID int64  `parquet:"item_id"`
}

type recipeRow struct {
	SpellID int64  `parquet:"spell_id"`
	Name    string `parquet:"name"`
}

func sized(t *testing.T, b *bytes.Buffer) store.SizedReaderAt {
	t.Helper()
	return store.SizedReaderAt{ReaderAt: bytes.NewReader(b.Bytes()), Size: int64(b.Len())}
}

func encode[T any](t *testing.T, rows []T, opts ...parquet.WriterOption) store.SizedReaderAt {
	t.Helper()
	var b bytes.Buffer
	w := parquet.NewGenericWriter[T](&b, opts...)
	if _, err := w.Write(rows); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return sized(t, &b)
}

var items = []itemRow{{1, "Linen Cloth"}, {2, "Linen Bandage"}, {3, "Thunderfury, Blessed Blade"}, {4, "Crème Brûlée"}, {5, "Linen"}}
var npcs = []npcRow{{100, "Linen Thief"}, {101, "Timber Wolf"}}
var recipes = []recipeRow{{300, "Heavy Linen Bandage"}}

// sets has a row for each member of a set, as the item_sets table does.
var sets = []setRow{{281, "Champion's Battlegear", 1}, {281, "Champion's Battlegear", 2}, {187, "The Elements", 3}, {1678, "The Elements", 4}}

// fixture is a dataset with the index (a Bloom filter on norm, two row groups) or without it.
func fixture(t *testing.T, index bool) store.DataStore {
	t.Helper()
	src := store.ReaderSource{"items": encode(t, items), "npcs": encode(t, npcs), "recipes": encode(t, recipes), "item_sets": encode(t, sets)}
	if index {
		var rows []nameRow
		for _, i := range items {
			rows = append(rows, nameRow{Normalize(i.Name), i.Name, Items, int32(i.ID)})
		}
		for _, n := range npcs {
			rows = append(rows, nameRow{Normalize(n.Name), n.Name, Npcs, int32(n.ID)})
		}
		for _, r := range recipes {
			rows = append(rows, nameRow{Normalize(r.Name), r.Name, Recipes, int32(r.SpellID)})
		}
		for _, r := range sets {
			rows = append(rows, nameRow{Normalize(r.Name), r.Name, ItemSets, int32(r.SetID)})
		}
		sort.Slice(rows, func(a, b int) bool { return rows[a].Norm < rows[b].Norm })
		src["names"] = encode(t, rows, parquet.MaxRowsPerRowGroup(3), parquet.BloomFilters(parquet.SplitBlockFilter(10, "norm")))
	}
	return store.New(src)
}

func TestNormalize(t *testing.T) {
	for in, want := range map[string]string{
		"Linen Cloth": "linen cloth", "Thunderfury, Blessed Blade": "thunderfury blessed blade", "Thunderfury's": "thunderfurys",
		"Crème Brûlée": "creme brulee", "  A -- B  ": "a b", "!!": "",
	} {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFind(t *testing.T) {
	ds := fixture(t, true)
	for _, tc := range []struct {
		name, table, text string
		limit             int
		want              string
		more              bool
	}{
		{"exact beats prefix", Items, "linen", 10, "5", false},
		{"exact ignores case", Items, "LINEN CLOTH", 10, "1", false},
		{"prefix", Items, "linen b", 10, "2", false},
		{"prefix lists several in norm order", Items, "lin", 10, "5 2 1", false},
		{"limit cuts the list", Items, "lin", 2, "5 2", true},
		{"accent folded", Items, "creme brulee", 10, "4", false},
		{"accent folded in the query", Items, "Crème", 10, "4", false},
		{"punctuation dropped", Items, "thunderfury blessed", 10, "3", false},
		{"table filter", Npcs, "linen", 10, "100", false},
		{"recipe by spell id", Recipes, "heavy", 10, "300", false},
		{"no match", Items, "zzz", 10, "", false},
		{"a middle word is no prefix", Items, "bandage", 10, "", false},
		{"punctuation only", Items, "!!", 10, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hits, more, err := Find(ds, tc.table, tc.text, tc.limit)
			if err != nil {
				t.Fatal(err)
			}
			var ids []string
			for _, h := range hits {
				ids = append(ids, fmt.Sprint(h.ID))
			}
			if got := strings.Join(ids, " "); got != tc.want || more != tc.more {
				t.Errorf("Find(%q) = %q, %v; want %q, %v", tc.text, got, more, tc.want, tc.more)
			}
		})
	}
}

func TestFindFallback(t *testing.T) {
	var logged bytes.Buffer
	log.SetOutput(&logged)
	defer log.SetOutput(log.Writer())
	ds := fixture(t, false)
	for _, tc := range []struct {
		table, text string
		want        []int64
	}{
		{Items, "linen", []int64{5}},                   // exact first
		{Items, "bandage", []int64{2}},                 // scan matches any part
		{Items, "Creme Brulee", nil},                   // the scan does not fold accents
		{Recipes, "linen", []int64{300}},               // recipes are keyed by spell_id
		{ItemSets, "battlegear", []int64{281}},         // a set is keyed by set_id and named once, not once a member
		{ItemSets, "The Elements", []int64{187, 1678}}, // two sets may share a name
		{"quests", "anything", nil},                    // a missing table finds nothing
	} {
		hits, _, err := Find(ds, tc.table, tc.text, 10)
		if err != nil {
			t.Fatal(err)
		}
		var ids []int64
		for _, h := range hits {
			ids = append(ids, h.ID)
		}
		if !slices.Equal(ids, tc.want) {
			t.Errorf("fallback Find(%s, %q) = %v, want %v", tc.table, tc.text, ids, tc.want)
		}
	}
	// an exact name that follows the partial matches in file order is still the one found when the caller asks for one hit
	if hits, _, err := Find(ds, Items, "linen", 1); err != nil || len(hits) != 1 || hits[0].ID != 5 {
		t.Errorf("fallback Find(items, linen, 1) = %v, %v; want the exact Linen (5)", hits, err)
	}
	if !strings.Contains(logged.String(), "no names.parquet") {
		t.Errorf("fallback was not logged: %q", logged.String())
	}
}

func TestResolve(t *testing.T) {
	ds := fixture(t, true)
	for _, tc := range []struct {
		text  string
		id    int64
		cands int
	}{
		{"linen cloth", 1, 0}, {"Linen B", 2, 0}, {"lin", 0, 3}, {"zzz", 0, 0},
	} {
		if id, _, c := Resolve(ds, Items, tc.text); id != tc.id || len(c) != tc.cands {
			t.Errorf("Resolve(%q) = %d, %v", tc.text, id, c)
		}
	}
}

func TestObtainableFirst(t *testing.T) {
	real, dnt := Match{1, "Old Cloth Hood"}, Match{2, "DNT HEAD"}
	for _, tc := range []struct{ in, want []Match }{
		{[]Match{dnt, real}, []Match{real}},
		{[]Match{dnt}, []Match{dnt}},
		{[]Match{real}, []Match{real}},
		{nil, nil},
	} {
		got := ObtainableFirst(tc.in)
		if len(got) != len(tc.want) || len(got) > 0 && got[0] != tc.want[0] {
			t.Errorf("ObtainableFirst(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

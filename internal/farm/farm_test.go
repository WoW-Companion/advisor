package farm

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/parquet-go/parquet-go"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/store"
)

type itemRow struct {
	ID   int64  `parquet:"id"`
	Name string `parquet:"name"`
}

type lootRow struct {
	ItemID int64  `parquet:"item_id"`
	Kind   string `parquet:"source_kind"`
	ID     int64  `parquet:"source_id"`
	Kills  int32  `parquet:"kills"`
	Drops  int32  `parquet:"drops"`
}

type npcRow struct {
	ID       int64   `parquet:"id"`
	Name     string  `parquet:"name"`
	MinLevel *int16  `parquet:"min_level,optional"`
	MaxLevel *int16  `parquet:"max_level,optional"`
	Zone     *string `parquet:"zone_name,optional"`
}

func table[T any](t *testing.T, rows []T) store.SizedReaderAt {
	t.Helper()
	var buf bytes.Buffer
	if err := parquet.Write(&buf, rows); err != nil {
		t.Fatal(err)
	}
	return store.SizedReaderAt{ReaderAt: bytes.NewReader(buf.Bytes()), Size: int64(buf.Len())}
}

func i16(n int16) *int16  { return &n }
func sp(s string) *string { return &s }

// Item 1 (Linen Cloth) drops from: 10 Low Boar (level 1-2, 50%), 11 Mid Wolf (level 10-12, 10%,
// two rows summed to 10/100), 12 Mid Bear (10-12, no kills), 13 Mid Fox (10-12, 30%, no zone),
// 14 Nameless (not in npcs, 90%), object 50 (80%), and a crafted and a quest source.
func synth(t *testing.T) store.DataStore {
	return store.New(store.ReaderSource{
		"items": table(t, []itemRow{{1, "Linen Cloth"}, {2, "Linen Bandage"}, {3, "Linen Robe"}, {4, "linen"}}),
		"loot_sources": table(t, []lootRow{
			{1, "creature", 10, 100, 50}, {1, "creature", 11, 60, 5}, {1, "creature", 11, 40, 5},
			{1, "creature", 12, 0, 0}, {1, "creature", 13, 10, 3}, {1, "creature", 14, 10, 9},
			{1, "object", 50, 10, 8}, {1, "crafting", 7, 0, 0}, {1, "quest", 8, 0, 0}, {2, "crafting", 7, 0, 0},
		}),
		"npcs": table(t, []npcRow{
			{10, "Low Boar", i16(1), i16(2), sp("Elwynn")}, {11, "Mid Wolf", i16(10), i16(12), sp("Westfall")},
			{12, "Mid Bear", i16(10), i16(12), sp("Duskwood")}, {13, "Mid Fox", i16(10), i16(12), nil},
		}),
	})
}

func at(level int) character.Character {
	return character.Character{Level: &level}
}

func order(spots []Spot) string {
	var s []string
	for _, p := range spots {
		s = append(s, fmt.Sprintf("%s%d", p.Kind[:1], p.ID))
	}
	return strings.Join(s, " ")
}

func TestSpots(t *testing.T) {
	ds := synth(t)
	for _, tc := range []struct {
		name  string
		level int
		want  string
	}{
		// level 10: window 5..13 fits 11, 12, 13; the rest follow by chance; unknown chance after
		// known within a group; the nameless creature and the object have no level, so are out of range.
		{"mid", 10, "c13 c11 c12 c14 o50 c10"},
		// level 1: only the boar (1-2) is in range, then the rest by chance.
		{"low", 1, "c10 c14 o50 c13 c11 c12"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := order(Spots(at(tc.level), ds, 1)); got != tc.want {
				t.Errorf("order = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSpotFields(t *testing.T) {
	ds := synth(t)
	by := map[int64]Spot{}
	for _, s := range Spots(at(10), ds, 1) {
		by[s.ID] = s
	}
	if w := by[11]; w.Chance == nil || *w.Chance != 10 || w.Kills != 100 || w.Zone != "Westfall" || w.ZoneSource != "npcs.zone_name" || !w.InRange {
		t.Errorf("wolf = %+v", w)
	}
	if b := by[12]; b.Chance != nil || b.Zone != "Duskwood" {
		t.Errorf("bear chance = %v, want unknown", b.Chance)
	}
	if f := by[13]; f.Zone != "" || f.ZoneSource != "" || f.Chance == nil {
		t.Errorf("fox = %+v", f)
	}
	if n := by[14]; n.Name != "" || n.MinLevel != nil || n.InRange || n.Zone != "" {
		t.Errorf("nameless = %+v", n)
	}
	if len(by) != 6 {
		t.Errorf("%d spots, want 6 (crafting and quest sources left out)", len(by))
	}
	if got := Spots(character.Character{}, ds, 1); got[0].InRange {
		t.Errorf("no character level: %+v marked in range", got[0])
	}
}

func TestCrafted(t *testing.T) {
	ds := synth(t)
	for id, want := range map[int64]bool{1: true, 2: true, 3: false} {
		if got := Crafted(ds, id); got != want {
			t.Errorf("Crafted(%d) = %v, want %v", id, got, want)
		}
	}
	if n := len(Spots(at(10), ds, 2)); n != 0 {
		t.Errorf("item 2 has %d spots, want 0", n)
	}
}

func TestResolve(t *testing.T) {
	ds := synth(t)
	for _, tc := range []struct {
		in    string
		id    int64
		cands int
	}{
		{"1", 1, 0}, {"999", 0, 0}, {"Linen Cloth", 1, 0}, {"linen cloth", 1, 0}, {"Linen Rob", 3, 0},
		{"Linen B", 2, 0}, {"Linen", 4, 0}, {"Lin", 0, 4}, {"zzz", 0, 0},
	} {
		id, _, cands := Resolve(ds, tc.in)
		if id != tc.id || len(cands) != tc.cands {
			t.Errorf("Resolve(%q) = %d, %v; want %d, %d candidates", tc.in, id, cands, tc.id, tc.cands)
		}
	}
}

func TestRender(t *testing.T) {
	ds := synth(t)
	text := Render("Linen Cloth", Spots(at(10), ds, 1), true, 3)
	for _, want := range []string{
		"1. Mid Fox (creature 13): level 10-12, zone unknown, 30.0% of 10 loot windows, in your level range",
		"2. Mid Wolf (creature 11): level 10-12, Westfall (from npcs.zone_name), 10.0% of 100 loot windows",
		"3. Mid Bear (creature 12): level 10-12, Duskwood (from npcs.zone_name), chance unknown",
		"(3 more not shown)", "profession_plan",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("text lacks %q:\n%s", want, text)
		}
	}
	if got := Render("X", nil, false, 5); !strings.Contains(got, "No creature or object") {
		t.Errorf("empty = %q", got)
	}
	if got := Render("X", Spots(at(10), ds, 1)[4:5], false, 5); !strings.Contains(got, "object 50: level unknown, zone unknown, 80.0%") {
		t.Errorf("object line = %q", got)
	}
}

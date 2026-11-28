package inventory

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/WoW-Companion/advisor/internal/benchsummary"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/store"
)

// The offline consumable inventory benchmark: what the advisor tells a character to do with the food, drink and
// class potions in its bags, beside the dataset's item rows, with no network. testdata/consumables_snapshot holds the
// real rows (every consumable item). The reference is written apart from the advisor: the game's own rules (a drink
// restores mana, so a warrior or rogue has no use for one; a class potion or poison is for the classes in the item's
// allowable_class_mask; an item above the character's level is kept for later; of the food or drink a character can
// use, only the one needing the highest level is the best) read against each row's required_level, bonding and
// allowable_class_mask. The dataset has no food or drink flag, no mana value and no buff table, so the kind of a
// food item is the game's own: the names below. make offline-benchmark runs it; WOWC_BENCH_REPORT receives the tables.

const (
	cbDir  = "../../testdata/consumables_snapshot"
	cbRows = "dataset items (class_id 0, subclass_id 5, required_level, allowable_class_mask, bonding); game rules for mana and class potions"
)

type cbRow struct{ check, advisor, reference, source, result string }

var (
	cbOnce  sync.Once
	cbCache store.DataStore
)

// cbItems serves the snapshot's item rows from memory and no other table: a review of a bag reads items alone for
// food and potions, and reading the parquet file for every row would make each review take 100 ms.
type cbItems struct {
	byID  map[int64]store.Row
	all   []store.Row
	inner store.DataStore
}

func (c cbItems) Get(table string, id int64) (store.Row, error) {
	if r, ok := c.byID[id]; ok && table == "items" {
		return r, nil
	}
	return nil, fmt.Errorf("%s %d: %w", table, id, store.ErrNotFound)
}

func cbNum(v any) (float64, bool) {
	switch n := v.(type) {
	case int64:
		return float64(n), true
	case int32:
		return float64(n), true
	case int:
		return float64(n), true
	case float64:
		return n, true
	}
	return 0, false
}

// cbHolds is the store's range rule on one value: numbers and strings compare with their kind, a nil bound is open.
func cbHolds(v any, r store.Range) bool {
	if f, ok := cbNum(v); ok {
		if lo, ok := cbNum(r.Min); ok && f < lo {
			return false
		}
		if hi, ok := cbNum(r.Max); ok && f > hi {
			return false
		}
		return true
	}
	s, _ := v.(string)
	if lo, ok := r.Min.(string); ok && s < lo {
		return false
	}
	if hi, ok := r.Max.(string); ok && s > hi {
		return false
	}
	return true
}

func (c cbItems) Filter(table string, ranges ...store.Range) ([]store.Row, error) {
	if table != "items" {
		return nil, nil
	}
	var out []store.Row
next:
	for _, row := range c.all {
		for _, r := range ranges {
			if !cbHolds(row[r.Column], r) {
				continue next
			}
		}
		out = append(out, row)
	}
	return out, nil
}

func (c cbItems) SearchName(table, text string, limit int) ([]store.Row, error) {
	return c.inner.SearchName(table, text, limit)
}

func cbStore() store.DataStore {
	cbOnce.Do(func() {
		in := store.NewDir(cbDir)
		all, err := in.Filter("items", store.Range{Column: "id"})
		if err != nil {
			panic(err)
		}
		m := cbItems{byID: map[int64]store.Row{}, all: all, inner: in}
		for _, r := range all {
			m.byID[i64(r["id"])] = r
		}
		cbCache = m
	})
	return cbCache
}

func cbFinish(t *testing.T, area, title string, rows []cbRow) {
	t.Helper()
	results := make([]string, len(rows))
	for i, r := range rows {
		results[i] = r.result
		if strings.HasPrefix(r.result, "differ") {
			t.Errorf("%s: advisor %q, reference %q: %s", r.check, r.advisor, r.reference, r.result)
		}
	}
	fmt.Println(benchsummary.AreaLine(area, results))
	path := os.Getenv("WOWC_BENCH_REPORT")
	if path == "" || len(rows) == 0 {
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "### %s\n\n| Check | Advisor | Reference | Source | Result |\n| --- | --- | --- | --- | --- |\n", title)
	for _, r := range rows {
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s |\n", r.check, r.advisor, r.reference, r.source, r.result)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(b.String() + "\n"); err != nil {
		t.Fatal(err)
	}
}

func cbItem(t *testing.T, name string, rl int64) store.Row {
	t.Helper()
	rows, err := cbStore().Filter("items", store.Range{Column: "name", Min: name, Max: name})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	for _, r := range rows {
		if i64(r["required_level"]) == rl {
			return r
		}
	}
	t.Fatalf("%s (level %d): not in the snapshot", name, rl)
	return nil
}

// cbClass is a class's id in the game's class table, the bit its mask uses is 1 << (id-1); mana says whether it has any.
var cbClass = []struct {
	name string
	id   uint
	mana bool
}{
	{"WARRIOR", 1, false}, {"PALADIN", 2, true}, {"HUNTER", 3, true}, {"ROGUE", 4, false}, {"PRIEST", 5, true},
	{"SHAMAN", 7, true}, {"MAGE", 8, true}, {"WARLOCK", 9, true}, {"DRUID", 11, true},
}

func cbAllows(mask int64, id uint) bool { return mask == -1 || mask&(1<<(id-1)) != 0 }

// cbFoods are food and drink items of the snapshot that the game itself files: a drink restores mana, food health.
// One per level step, so each is the best of its kind for a range of levels.
var cbFoods = []struct {
	name  string
	rl    int64
	drink bool
}{
	{"Tough Jerky", 1, false}, {"Refreshing Spring Water", 1, true}, {"Haunch of Meat", 5, false}, {"Ice Cold Milk", 5, true},
	{"Mutton Chop", 15, false}, {"Melon Juice", 15, true}, {"Wild Hog Shank", 25, false}, {"Sweet Nectar", 25, true},
	{"Cured Ham Steak", 35, false}, {"Moonberry Juice", 35, true}, {"Homemade Cherry Pie", 45, false}, {"Morning Glory Dew", 45, true},
	{"Conjured Cinnamon Roll", 55, false}, {"Hyjal Nectar", 55, true},
}

// TestOfflineBenchmarkFoodAndDrink: with every food and drink of the list in the bags, which the advisor keeps: the
// best food and the best drink the character can use now, anything needing more level for later, and for a class with
// no mana no drink at all.
func TestOfflineBenchmarkFoodAndDrink(t *testing.T) {
	ds := cbStore()
	var out []cbRow
	var stacks []character.ItemStack
	rows := map[string]store.Row{}
	for _, f := range cbFoods {
		r := cbItem(t, f.name, f.rl)
		rows[f.name] = r
		stacks = append(stacks, character.ItemStack{ItemID: int(i64(r["id"])), Count: 4, Name: f.name})
	}
	for ci, cl := range cbClass {
		for _, lv := range []int{1, 5, 12, 25, 38, 45, 57, 60} {
			l := lv
			faction := []string{"Alliance", "Horde"}[(ci+lv)%2]
			c := character.Character{Name: "Bench", Faction: faction, Class: cl.name, Level: &l, BagsCaptured: true, BankCaptured: true, Bags: stacks}
			got := map[string]Line{}
			for _, ln := range ReviewAll([]character.Character{c}, ds).Lines {
				got[ln.Item] = ln
			}
			best := map[bool]int64{}
			for _, f := range cbFoods {
				if f.rl <= int64(lv) && (!f.drink || cl.mana) && f.rl > best[f.drink] {
					best[f.drink] = f.rl
				}
			}
			for _, f := range cbFoods {
				if cl.name == "HUNTER" && !f.drink {
					continue // a hunter's meat is its pet's food, kept by the pet food rule; the pet food benchmark covers it
				}
				want := Sell
				switch {
				case f.drink && !cl.mana:
				case f.rl > int64(lv), f.rl == best[f.drink]:
					want = Keep_
				}
				ln := got[f.name]
				out = append(out, cbRow{fmt.Sprintf("%s level %d (%s): %s (needs %d)", cl.name, lv, faction, f.name, f.rl), string(ln.Action) + ": " + ln.Reason, string(want), cbRows,
					resultIf(ln.Action == want, "keep or sell the wrong stack")})
			}
		}
	}
	cbFinish(t, "advice-consumable-food-drink", "Offline consumables: food and drink kept and sold", out)
}

func resultIf(ok bool, bad string) string {
	if ok {
		return "match"
	}
	return "differ: " + bad
}

// cbRestricted are class potions, poisons and teas of the snapshot whose allowable_class_mask names classes: a Rage
// Potion is for warriors and druids, a poison for rogues.
var cbRestricted = []struct {
	name string
	rl   int64
}{
	{"Rage Potion", 4}, {"Great Rage Potion", 25}, {"Mighty Rage Potion", 46}, {"Thistle Tea", 5},
	{"Instant Poison IV", 44}, {"Deadly Poison V", 60}, {"Crippling Poison II", 50}, {"Wound Poison III", 48},
}

// TestOfflineBenchmarkClassConsumables: a class potion or poison in the bags of a class the item's mask leaves out
// goes to an alt whose class it names (unless it is bound to the holder), else it is sold; for a class the mask names
// it is never sold or mailed away.
func TestOfflineBenchmarkClassConsumables(t *testing.T) {
	ds := cbStore()
	var out []cbRow
	lv := 60
	for ci, it := range cbRestricted {
		row := cbItem(t, it.name, it.rl)
		mask, bound := i64(row["allowable_class_mask"]), i64(row["bonding"]) == 1
		stack := character.ItemStack{ItemID: int(i64(row["id"])), Count: 3, Name: it.name}
		for hi, holder := range cbClass {
			l := lv
			faction := []string{"Alliance", "Horde"}[(ci+hi)%2]
			me := character.Character{Name: "Holder", Faction: faction, Class: holder.name, Level: &l, BagsCaptured: true, BankCaptured: true, Bags: []character.ItemStack{stack}}
			usable := cbAllows(mask, holder.id)
			// alone: sold when the class cannot use it
			ln := lineOf(ReviewAll([]character.Character{me}, ds).Lines, it.name)
			want := "not sold or mailed"
			if !usable {
				want = "Sell"
			}
			got := string(ln.Action)
			ok := usable && ln.Action != Sell && ln.Action != Mail || !usable && ln.Action == Sell
			out = append(out, cbRow{fmt.Sprintf("%s (%s, level %d): %s, mask %d, alone", holder.name, faction, lv, it.name, mask), got + ": " + ln.Reason, want, cbRows, resultIf(ok, "a class potion the class cannot use is not sold, or one it can use is")})
			if usable {
				continue
			}
			// with an alt whose class the mask names
			for _, alt := range cbClass {
				if !cbAllows(mask, alt.id) {
					continue
				}
				a := character.Character{Name: "Alt", Faction: faction, Class: alt.name, Level: &l, BagsCaptured: true, BankCaptured: true}
				ln := lineOf(ReviewAll([]character.Character{me, a}, ds).Lines, it.name)
				wantAct := Mail
				if bound {
					wantAct = Sell
				}
				out = append(out, cbRow{fmt.Sprintf("%s (%s, level %d): %s, mask %d, with a level %d %s alt", holder.name, faction, lv, it.name, mask, lv, alt.name), string(ln.Action) + " " + ln.To + ": " + ln.Reason, string(wantAct), cbRows,
					resultIf(ln.Action == wantAct && (wantAct == Sell || ln.To == "Alt"), "a class potion for the alt's class is not mailed to it, or a bound one is mailed")})
				break
			}
		}
	}
	cbFinish(t, "advice-consumable-class-potions", "Offline consumables: class potions and poisons", out)
}

func lineOf(lines []Line, item string) Line {
	for _, l := range lines {
		if l.Item == item {
			return l
		}
	}
	return Line{}
}

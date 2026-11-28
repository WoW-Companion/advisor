package route

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/questie"
	"github.com/WoW-Companion/advisor/internal/store"
)

// The offline profession route benchmark: the profession lines the route plan carries (the trainer to train a
// rank with, the craft batch, the restock purchases) beside the dataset's tables, with no network. The reference
// is read straight from testdata/advice_snapshot (profession_tiers, profession_routes, recipes) and
// testdata/places_snapshot (vendor_sources), never through the advisor: profession_tiers holds the rank gates and
// the trainers of the wow-professions Forever guides, profession_routes their skill-up steps. The Questie index is
// synthetic: each tier trainer of the dataset stands in its city, and each vendor of vendor_sources stands in the
// capital its id picks, so the route has places to name.

const (
	pbAdviceDir = "../../testdata/advice_snapshot"
	pbPlacesDir = "../../testdata/places_snapshot"
)

var (
	pbOnce  sync.Once
	pbCache store.DataStore
)

// pbStore holds both snapshots in memory, the places snapshot's tables first: a route plan reads the tables many
// times, and a read from disk each time is slow.
func pbStore() store.DataStore {
	pbOnce.Do(func() {
		src := store.ReaderSource{}
		for _, dir := range []string{pbAdviceDir, pbPlacesDir} {
			files, _ := filepath.Glob(filepath.Join(dir, "*.parquet"))
			for _, f := range files {
				b, err := os.ReadFile(f)
				if err != nil {
					panic(err)
				}
				src[strings.TrimSuffix(filepath.Base(f), ".parquet")] = store.SizedReaderAt{ReaderAt: bytes.NewReader(b), Size: int64(len(b))}
			}
		}
		pbCache = &pbMemo{DataStore: store.New(src), filter: map[string][]store.Row{}, search: map[string][]store.Row{}, get: map[string]store.Row{}}
	})
	return pbCache
}

// pbMemo remembers each answer of the store: a plan asks the same question many times and the benchmark plans
// hundreds of characters.
type pbMemo struct {
	store.DataStore
	mu             sync.Mutex
	filter, search map[string][]store.Row
	get            map[string]store.Row
}

func (m *pbMemo) Filter(table string, ranges ...store.Range) ([]store.Row, error) {
	k := fmt.Sprint(table, ranges)
	m.mu.Lock()
	defer m.mu.Unlock()
	if r, ok := m.filter[k]; ok {
		return r, nil
	}
	r, err := m.DataStore.Filter(table, ranges...)
	if err == nil {
		m.filter[k] = r
	}
	return r, err
}

func (m *pbMemo) SearchName(table, text string, limit int) ([]store.Row, error) {
	k := fmt.Sprint(table, text, limit)
	m.mu.Lock()
	defer m.mu.Unlock()
	if r, ok := m.search[k]; ok {
		return r, nil
	}
	r, err := m.DataStore.SearchName(table, text, limit)
	if err == nil {
		m.search[k] = r
	}
	return r, err
}

func (m *pbMemo) Get(table string, id int64) (store.Row, error) {
	k := fmt.Sprint(table, id)
	m.mu.Lock()
	defer m.mu.Unlock()
	if r, ok := m.get[k]; ok {
		return r, nil
	}
	r, err := m.DataStore.Get(table, id)
	if err == nil {
		m.get[k] = r
	}
	return r, err
}

type pbCity struct {
	name, side string
	id         int
}

var pbCities = []pbCity{
	{"Orgrimmar", "H", 0}, {"Undercity", "H", 1}, {"Thunder Bluff", "H", 2},
	{"Stormwind City", "A", 3}, {"Ironforge", "A", 4}, {"Darnassus", "A", 5},
}

func (c pbCity) uimap() int { return 8000 + c.id }
func (c pbCity) area() int  { return 800 + c.id }

func pbCitiesOf(side string) []pbCity {
	var out []pbCity
	for _, c := range pbCities {
		if c.side == side {
			out = append(out, c)
		}
	}
	return out
}

// pbFull says the whole matrix runs (WOWC_BENCH_FULL=1): every capital, both skills of a step, every placement. A plain
// run keeps to a sample that fits the time a test may take; the fixed cases hold the same differences.
func pbFull() bool { return os.Getenv("WOWC_BENCH_FULL") != "" }

func pbSide(faction string) string {
	if faction == "Horde" {
		return "H"
	}
	return "A"
}

func pbText(v any) string { s, _ := v.(string); return s }

func pbInt(v any) int {
	switch x := v.(type) {
	case int64:
		return int(x)
	case int32:
		return int(x)
	case int16:
		return int(x)
	case float64:
		return int(x)
	}
	return 0
}

// pbTier is a profession_tiers row.
type pbTier struct {
	prof, tier, how, faction, trainer, place string
	skill, level                             int
}

func pbTiers(t *testing.T) []pbTier {
	t.Helper()
	rows, err := pbStore().Filter("profession_tiers", store.Range{Column: "profession", Min: "", Max: "~"})
	if err != nil || len(rows) == 0 {
		t.Fatalf("profession_tiers: %d rows, %v", len(rows), err)
	}
	var out []pbTier
	for _, r := range rows {
		out = append(out, pbTier{pbText(r["profession"]), pbText(r["tier"]), pbText(r["how"]), pbText(r["faction"]), pbText(r["trainer_name"]),
			pbText(r["place"]), pbInt(r["skill_required"]), pbInt(r["min_level"])})
	}
	return out
}

// pbOpen is true when a dataset row for faction ("Horde", "Alliance", "Both" or "") serves the player's faction.
func pbOpen(rowFaction, faction string) bool {
	return rowFaction == "" || rowFaction == "Both" || rowFaction == faction
}

// pbWorld is a Questie index of the six capitals: the dataset's tier trainers where the dataset puts them, the
// vendors of vendor_sources spread over the capitals by id, and one quest giver in each so a route has stops.
func pbWorld(t *testing.T, tiers []pbTier, vendors map[int]pbCity) *questie.Index {
	t.Helper()
	ix := &questie.Index{Quests: map[int]*questie.Quest{}, Npcs: map[int]questie.Entity{}, Objects: map[int]questie.Entity{}, Areas: map[int]questie.Area{}}
	for _, c := range pbCities {
		ix.Areas[c.area()] = questie.Area{UIMap: c.uimap(), Zone: c.name}
		giver := 1000 + c.id
		ix.Npcs[giver] = questie.Entity{Name: "Keeper of " + c.name, Spawns: []questie.Spawn{{Area: c.area(), X: 50, Y: 50}}}
		ix.Quests[100+c.id] = &questie.Quest{ID: 100 + c.id, Name: "Errand in " + c.name, Level: 58, Zone: c.area(),
			Starters: []questie.Ref{{ID: giver}}, Finishers: []questie.Ref{{ID: giver}}}
	}
	nounOf := map[string]string{"Tailoring": "Tailor", "Leatherworking": "Leatherworker", "Blacksmithing": "Blacksmith", "Alchemy": "Alchemist",
		"Enchanting": "Enchanter", "Engineering": "Engineer", "Cooking": "Cook", "First Aid": "First Aid", "Herbalism": "Herbalist",
		"Mining": "Miner", "Skinning": "Skinner", "Fishing": "Fisherman"}
	n := 0
	for _, r := range tiers {
		if r.how != "trainer" || r.trainer == "" {
			continue
		}
		for _, c := range pbCities {
			if r.place != c.name {
				continue
			}
			n++
			fac := map[string]string{"Horde": "H", "Alliance": "A"}[r.faction]
			ix.Npcs[5000+n] = questie.Entity{Name: r.trainer, Sub: r.tier + " " + nounOf[r.prof], Faction: fac,
				Flags: questie.FlagTrainer | questie.FlagProfTrainer, Spawns: []questie.Spawn{{Area: c.area(), X: float64(20 + n%50), Y: 20}}}
		}
	}
	for _, r := range pbAllProfessions {
		for _, c := range pbCities {
			n++
			ix.Npcs[9000+n] = questie.Entity{Name: r + " Apprentice " + c.name, Sub: nounOf[r] + " Trainer", Flags: questie.FlagTrainer | questie.FlagProfTrainer,
				Spawns: []questie.Spawn{{Area: c.area(), X: 10, Y: 10}}}
		}
	}
	for id, c := range vendors {
		fac := "H"
		if c.side == "A" {
			fac = "A"
		}
		ix.Npcs[id] = questie.Entity{Name: fmt.Sprintf("Vendor %d", id), Faction: fac, Flags: questie.FlagVendor,
			Spawns: []questie.Spawn{{Area: c.area(), X: 51, Y: 51}}}
	}
	return ix
}

func pbHero(city pbCity, faction string, level int, bags []character.ItemStack, skills ...character.Skill) character.Character {
	mp := city.uimap()
	x, y := 50.0, 50.0
	c := character.Character{Name: "Bench", Key: "R:Bench", Class: "MAGE", Faction: faction, Level: &level, QuestsCaptured: true, BankCaptured: true, BagsCaptured: true,
		Location: &character.Location{Zone: city.name, MapID: &mp, X: &x, Y: &y}, Professions: skills, Bags: bags}
	return c
}

func pbSkill(name string, rank, max int) character.Skill {
	return character.Skill{Name: name, Rank: &rank, Max: &max}
}

func pbOpts(ds store.DataStore) Options {
	return Options{Data: ds, AnyPickup: true, PickupCap: 99}
}

var pbPrevCap = map[string]int{"Journeyman": 75, "Expert": 150, "Artisan": 225}

func trainerLine(p Plan, prof string) (Errand, bool) {
	for _, e := range p.Errands {
		if e.Kind == ErrandProfession && strings.HasPrefix(e.Label, "Train "+prof+" (") {
			return e, true
		}
	}
	return Errand{}, false
}

func TestOfflineBenchmarkProfessionTrainers(t *testing.T) {
	ds, tiers := pbStore(), pbTiers(t)
	world := pbWorld(t, tiers, nil)
	var bad []string
	checked := 0
	seen := map[string]bool{}
	for _, r := range tiers {
		cap, ok := pbPrevCap[r.tier]
		if !ok || r.skill == 0 || seen[r.prof+r.tier+r.faction] {
			continue
		}
		seen[r.prof+r.tier+r.faction] = true
		if r.how != "trainer" { // a book or a quest gives the rank: no trainer line at the gate
			for _, faction := range []string{"Horde", "Alliance"} {
				if pbOpen(r.faction, faction) {
					checked++
					c := pbCitiesOf(pbSide(faction))[0]
					level := map[string]int{"Journeyman": 10, "Expert": 20, "Artisan": 35}[r.tier]
					if e, got := trainerLine(BuildWith(pbHero(c, faction, level, nil, pbSkill(r.prof, r.skill, cap)), world, pbOpts(ds)), r.prof); got {
						bad = append(bad, fmt.Sprintf("%s %s %s: line %s, but the dataset has it from a %s", faction, r.prof, r.tier, e.Label, r.how))
					}
				}
			}
			continue
		}
		level := r.level
		if level == 0 {
			level = map[string]int{"Journeyman": 10, "Expert": 20, "Artisan": 35}[r.tier]
		}
		for _, faction := range []string{"Horde", "Alliance"} {
			if !pbOpen(r.faction, faction) {
				continue
			}
			for ci, c := range pbCitiesOf(pbSide(faction)) {
				// the dataset's trainers of the rank for this side: listed says the table names any place, names those in this city
				listed, names := false, []string(nil)
				for _, o := range tiers {
					if o.prof == r.prof && o.tier == r.tier && o.how == "trainer" && o.place != "" && pbOpen(o.faction, faction) {
						listed = true
						if o.place == c.name {
							names = append(names, o.trainer)
						}
					}
				}
				for _, k := range []struct {
					skill, level int
					due          bool
				}{{r.skill, level, true}, {r.skill - 1, level, false}, {r.skill, level - 1, false}} {
					if !k.due && ci > 0 && !pbFull() {
						continue
					}
					checked++
					p := BuildWith(pbHero(c, faction, k.level, nil, pbSkill(r.prof, k.skill, cap)), world, pbOpts(ds))
					e, got := trainerLine(p, r.prof)
					label := fmt.Sprintf("%s %s %s in %s skill %d level %d", faction, r.prof, r.tier, c.name, k.skill, k.level)
					switch {
					case !k.due && got:
						bad = append(bad, label+": line "+e.Label)
					case !k.due:
					case listed && len(names) == 0 && got:
						bad = append(bad, label+": line "+e.Label+", but the dataset teaches the rank in other cities only")
					case listed && len(names) > 0 && !got:
						bad = append(bad, label+": no trainer line, dataset trainers "+strings.Join(names, ", "))
					case listed && len(names) > 0 && !pbHasName(e.Label, names):
						bad = append(bad, label+": line "+e.Label+", dataset trainers "+strings.Join(names, ", "))
					case !listed && !got:
						bad = append(bad, label+": no trainer line, the dataset names no place for the rank")
					}
				}
			}
		}
	}
	sort.Strings(bad)
	t.Logf("%d trainer checks, %d differ", checked, len(bad))
	for _, b := range bad {
		t.Error(b)
	}
}

func pbHasName(label string, names []string) bool {
	for _, n := range names {
		if strings.HasSuffix(label, " with "+n) {
			return true
		}
	}
	return false
}

// pbStep is a profession_routes row: one skill-up step of a Forever guide.
type pbStep struct {
	prof, recipe, kind, learned string
	from, to, crafts, spell     int
	mats                        []pbMat
}

type pbMat struct {
	id    int64
	name  string
	count int
}

// pbRoute reads the main steps of a profession's route, in skill order, as the table holds them. The store hands a
// list of structs back column by column: all the item ids, then all the names, then all the counts.
func pbRoute(t *testing.T, prof string) []pbStep {
	t.Helper()
	rows, err := pbStore().Filter("profession_routes", store.Range{Column: "profession", Min: prof, Max: prof})
	if err != nil || len(rows) == 0 {
		t.Fatalf("%s: no route rows: %v", prof, err)
	}
	var out []pbStep
	for _, r := range rows {
		if pbInt(r["option"]) != 0 || pbText(r["kind"]) == "gather" || pbText(r["recipe_name"]) == "" {
			continue
		}
		s := pbStep{prof: prof, recipe: pbText(r["recipe_name"]), kind: pbText(r["kind"]), from: pbInt(r["skill_from"]), to: pbInt(r["skill_to"]), crafts: pbInt(r["crafts"]), learned: pbText(r["learned_from"]), spell: pbInt(r["recipe_spell_id"])}
		l, _ := r["materials"].([]any)
		n := len(l) / 3
		for i := 0; i < n; i++ {
			id, _ := l[i].(int64)
			name, _ := l[n+i].(string)
			c, _ := l[2*n+i].(int64)
			s.mats = append(s.mats, pbMat{id, name, int(c)})
		}
		out = append(out, s)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].from < out[j].from })
	// a guide that lists two steps over the same skills (Engineering 135 to 150) is read as the advisor reads it: the
	// first covers the skills
	var main []pbStep
	for _, s := range out {
		if len(main) > 0 && main[len(main)-1].from == s.from {
			continue
		}
		main = append(main, s)
	}
	return main
}

// left is the crafts still to make from a skill inside the step: the step's crafts scaled by the skill range left, rounded up.
func (s pbStep) left(skill int) int {
	if skill <= s.from {
		return s.crafts
	}
	return (s.crafts*(s.to-skill) + (s.to - s.from) - 1) / (s.to - s.from)
}

// pbVendors maps an item to the vendor ids vendor_sources lists for it.
func pbVendors(t *testing.T, items map[int64]bool) map[int64][]int {
	t.Helper()
	rows, err := pbStore().Filter("vendor_sources", store.Range{Column: "npc_id", Min: int64(0)})
	if err != nil {
		t.Fatal(err)
	}
	out := map[int64][]int{}
	for _, r := range rows {
		it := int64(pbInt(r["item_id"]))
		if items[it] {
			out[it] = append(out[it], pbInt(r["npc_id"]))
		}
	}
	return out
}

var pbAllProfessions = []string{"Alchemy", "Blacksmithing", "Cooking", "Enchanting", "Engineering", "First Aid", "Fishing", "Herbalism", "Leatherworking", "Mining", "Skinning", "Tailoring"}

var pbCraftProfessions = []string{"Alchemy", "Blacksmithing", "Cooking", "Enchanting", "Engineering", "First Aid", "Leatherworking", "Tailoring"}

// pbStepCases are a few steps of each crafting profession: the first, a middle one and the last, each at its start
// and halfway through.
func pbStepCases(t *testing.T) []pbStep {
	var out []pbStep
	for _, p := range pbCraftProfessions {
		steps := pbRoute(t, p)
		out = append(out, steps...)
	}
	return out
}

// pbPlacements are the two ways the benchmark spreads a step's vendors over the capitals: by id (every capital has
// some), and with only the last vendor of each item in the character's own capital, the rest in another capital of the same side.
var pbPlacements = []string{"by id", "last in town"}

func pbPlace(kind string, vend map[int64][]int, hub pbCity) map[int]pbCity {
	out := map[int]pbCity{}
	other := pbCitiesOf(hub.side)[1]
	for _, ids := range vend {
		for i, id := range ids {
			switch {
			case kind == "by id":
				out[id] = pbCities[id%len(pbCities)]
			case i == len(ids)-1:
				out[id] = hub
			default:
				if _, set := out[id]; !set {
					out[id] = other
				}
			}
		}
	}
	return out
}

// pbBuy reads a restock line "buy N Item from Vendor".
func pbBuy(label string) (n int, item, vendor string) {
	rest := strings.TrimPrefix(label, "buy ")
	body, vendor, _ := strings.Cut(rest, " from ")
	count, item, _ := strings.Cut(body, " ")
	fmt.Sscanf(count, "%d", &n)
	return n, item, vendor
}

// pbSkills are the skills a step is planned at: halfway through it, and its start when the whole matrix runs.
func pbSkills(s pbStep) []int {
	if pbFull() {
		return []int{s.from, (s.from + s.to) / 2}
	}
	return []int{(s.from + s.to) / 2}
}

func TestOfflineBenchmarkProfessionRestock(t *testing.T) {
	ds, tiers := pbStore(), pbTiers(t)
	cases := pbStepCases(t)
	items := map[int64]bool{}
	for _, s := range cases {
		for _, m := range s.mats {
			items[m.id] = true
		}
	}
	vend := pbVendors(t, items)
	if !pbFull() { // the sample keeps the steps a vendor could supply whole
		var keep []pbStep
		for _, s := range cases {
			all := len(s.mats) > 0
			for _, m := range s.mats {
				all = all && len(vend[m.id]) > 0
			}
			if all {
				keep = append(keep, s)
			}
		}
		cases = keep
	}
	var bad []string
	checked, bought := 0, 0
	for _, kind := range pbPlacements {
		for _, faction := range []string{"Horde", "Alliance"} {
			city := pbCitiesOf(pbSide(faction))[0]
			if faction == "Alliance" && !pbFull() {
				continue // the sides differ in their cities, not in the rule
			}
			placed := pbPlace(kind, vend, city)
			world := pbWorld(t, tiers, placed)
			for _, s := range cases {
				for _, skill := range pbSkills(s) {
					checked++
					crafts := s.left(skill)
					label := fmt.Sprintf("%s %s at skill %d (%s, %d to %d, %s, vendors %s)", faction, s.prof, skill, s.recipe, s.from, s.to, city.name, kind)
					// the reference: every material sold in this city, else no purchase is listed
					whole, want, sellers := len(s.mats) > 0, map[string]int{}, map[string]map[string]bool{}
					for _, m := range s.mats {
						sellers[m.name] = map[string]bool{}
						for _, id := range vend[m.id] {
							if placed[id].name == city.name {
								sellers[m.name][fmt.Sprintf("Vendor %d", id)] = true
							}
						}
						whole = whole && len(sellers[m.name]) > 0
						want[m.name] = m.count * crafts
					}
					p := BuildWith(pbHero(city, faction, 60, nil, pbSkill(s.prof, skill, 300)), world, pbOpts(ds))
					got := map[string]int{}
					for _, e := range restocks(p) {
						n, item, who := pbBuy(e.Label)
						got[item] = n
						if !strings.Contains(e.Why, s.prof+" skill-ups ("+s.recipe+", ") {
							bad = append(bad, label+": restock why "+e.Why)
						}
						if !sellers[item][who] {
							bad = append(bad, fmt.Sprintf("%s: buys %s from %q, who the dataset does not list in this city", label, item, who))
						}
					}
					if whole && len(want) <= maxProfessionLines {
						bought++
					}
					switch {
					case whole && len(want) <= maxProfessionLines && fmt.Sprint(got) != fmt.Sprint(want):
						bad = append(bad, fmt.Sprintf("%s: buys %v, dataset step needs %v", label, got, want))
					case (!whole || len(want) > maxProfessionLines) && len(got) > 0:
						bad = append(bad, fmt.Sprintf("%s: buys %v, though a whole craft cannot be bought here", label, got))
					}
				}
			}
		}
	}
	sort.Strings(bad)
	t.Logf("%d restock checks (%d with a whole craft sold here), %d differ", checked, bought, len(bad))
	for _, b := range bad {
		t.Error(b)
	}
}

func TestOfflineBenchmarkProfessionCraft(t *testing.T) {
	ds := pbStore()
	var bad []string
	checked := 0
	for _, s := range pbStepCases(t) {
		for _, faction := range []string{"Horde", "Alliance"} {
			city := pbCitiesOf(pbSide(faction))[0]
			if faction == "Alliance" && !pbFull() {
				continue
			}
			for _, skill := range pbSkills(s) {
				checked++
				crafts := s.left(skill)
				var bags []character.ItemStack
				for _, m := range s.mats {
					bags = append(bags, character.ItemStack{ItemID: int(m.id), Count: m.count * crafts, Name: m.name})
				}
				p := BuildWith(pbHero(city, faction, 60, bags, pbSkill(s.prof, skill, 300)), pbWorld(t, nil, nil), pbOpts(ds))
				want := fmt.Sprintf("Craft %d %s (to %d)", crafts, s.recipe, s.to)
				var got []string
				for _, e := range profErrands(p) {
					got = append(got, e.Label)
				}
				if len(s.mats) > 0 && fmt.Sprint(got) != fmt.Sprint([]string{want}) {
					bad = append(bad, fmt.Sprintf("%s %s at skill %d (%s, %d to %d, %s): lines %q, dataset step says %q", faction, s.prof, skill, s.recipe, s.from, s.to, city.name, got, want))
				}
			}
		}
	}
	sort.Strings(bad)
	t.Logf("%d craft checks, %d differ", checked, len(bad))
	for _, b := range bad {
		t.Error(b)
	}
}

func TestOfflineBenchmarkProfessionTrainRecipe(t *testing.T) {
	ds, tiers := pbStore(), pbTiers(t)
	world := pbWorld(t, tiers, nil)
	var bad []string
	checked := 0
	for _, prof := range pbCraftProfessions {
		steps := pbRoute(t, prof)
		for i, s := range steps {
			known := map[int64]bool{1: true}
			for _, o := range steps[:i] {
				known[int64(o.spell)] = true
			}
			// the rank whose trainer teaches the step's recipe, by the skill it is made at
			tier := "Apprentice"
			for _, b := range []struct {
				at   int
				tier string
			}{{75, "Journeyman"}, {150, "Expert"}, {225, "Artisan"}} {
				if s.from >= b.at {
					tier = b.tier
				}
			}
			for _, faction := range []string{"Horde", "Alliance"} {
				for _, city := range pbCitiesOf(pbSide(faction)) {
					if !pbFull() && (faction == "Alliance" || city.id > 1) {
						continue
					}
					listed, here := false, false
					for _, o := range tiers {
						if o.prof == prof && o.tier == tier && o.how == "trainer" && o.place != "" && pbOpen(o.faction, faction) {
							listed = true
							here = here || o.place == city.name
						}
					}
					checked++
					c := pbHero(city, faction, 60, nil, pbSkill(prof, s.from, 300))
					c.KnownSpells = known
					p := BuildWith(c, world, pbOpts(ds))
					var got []string
					for _, e := range profErrands(p) {
						if strings.HasPrefix(e.Label, "Train ") {
							got = append(got, e.Label)
						}
					}
					label := fmt.Sprintf("%s %s at skill %d (step %s %d to %d, learned from %s, %s)", faction, prof, s.from, s.recipe, s.from, s.to, s.learned, city.name)
					switch {
					case (s.learned != "trainer" || known[int64(s.spell)] || listed && !here) && len(got) > 0:
						bad = append(bad, label+": lines "+strings.Join(got, "; "))
					case s.learned == "trainer" && !known[int64(s.spell)] && (!listed || here) && (len(got) != 1 || !strings.HasPrefix(got[0], "Train "+prof+" ("+s.recipe+") with ")):
						bad = append(bad, label+": lines "+strings.Join(got, "; ")+", the dataset step is a trainer recipe")
					}
				}
			}
		}
	}
	sort.Strings(bad)
	t.Logf("%d train-recipe checks, %d differ", checked, len(bad))
	for _, b := range bad {
		t.Error(b)
	}
}

package digest

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/parquet-go/parquet-go"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/lua"
	"github.com/WoW-Companion/advisor/internal/store"
)

var update = flag.Bool("update", false, "rewrite the golden files")

func ip(n int) *int      { return &n }
func bp(b bool) *bool    { return &b }
func i32(n int32) *int32 { return &n }

type questRow struct {
	ID    int64   `parquet:"id"`
	Level *int32  `parquet:"level,optional"`
	Zone  *string `parquet:"zone,optional"`
}

type itemRow struct {
	ID            int64   `parquet:"id"`
	Name          string  `parquet:"name"`
	ItemLevel     int32   `parquet:"item_level"`
	RequiredLevel int32   `parquet:"required_level"`
	Quality       int32   `parquet:"quality"`
	ClassID       int32   `parquet:"class_id"`
	SubclassID    int32   `parquet:"subclass_id"`
	Slot          int32   `parquet:"slot"`
	ClassMask     int32   `parquet:"allowable_class_mask"`
	StatTypes     []int16 `parquet:"stat_types,list"`
	StatsStatus   string  `parquet:"stats_status"`
}

type lootRow struct {
	ItemID int64  `parquet:"item_id"`
	Kind   string `parquet:"source_kind"`
	ID     int64  `parquet:"source_id"`
	Kills  int32  `parquet:"kills"`
	Drops  int32  `parquet:"drops"`
}

type npcRow struct {
	ID       int64  `parquet:"id"`
	Name     string `parquet:"name"`
	MinLevel *int32 `parquet:"min_level,optional"`
	MaxLevel *int32 `parquet:"max_level,optional"`
}

type instRow struct {
	MapID      int32  `parquet:"map_id"`
	Name       string `parquet:"name"`
	Type       string `parquet:"instance_type"`
	MaxPlayers *int16 `parquet:"max_players,optional"`
	MinLevel   *int16 `parquet:"min_level,optional"`
	MaxLevel   *int16 `parquet:"max_level,optional"`
}

type bossRow struct {
	MapID      int32  `parquet:"map_id"`
	Name       string `parquet:"name"`
	CreatureID *int32 `parquet:"creature_id,optional"`
}

type buildRow struct {
	Class     string `parquet:"class"`
	Spec      string `parquet:"spec"`
	Level     int16  `parquet:"level"`
	Order     int16  `parquet:"order"`
	NodeID    int32  `parquet:"node_id"`
	Name      string `parquet:"talent_name"`
	RankAfter int16  `parquet:"rank_after"`
	Source    string `parquet:"source"`
}

type priorityRow struct {
	Class  string `parquet:"class"`
	Spec   string `parquet:"spec"`
	Rank   int16  `parquet:"rank"`
	NodeID int32  `parquet:"node_id"`
	Name   string `parquet:"talent_name"`
	Reason string `parquet:"reason"`
}

type nodeRow struct {
	NodeID  int32  `parquet:"node_id"`
	SpellID *int32 `parquet:"spell_id,optional"`
	Name    string `parquet:"name"`
}

func tblAny[T any](t *testing.T, rows []T) store.SizedReaderAt {
	t.Helper()
	var buf bytes.Buffer
	if err := parquet.Write(&buf, rows); err != nil {
		t.Fatal(err)
	}
	return store.SizedReaderAt{ReaderAt: bytes.NewReader(buf.Bytes()), Size: int64(buf.Len())}
}

func tbl(t *testing.T, rows any) store.SizedReaderAt {
	t.Helper()
	var buf bytes.Buffer
	var err error
	switch r := rows.(type) {
	case []questRow:
		err = parquet.Write(&buf, r)
	case []itemRow:
		err = parquet.Write(&buf, r)
	case []lootRow:
		err = parquet.Write(&buf, r)
	case []npcRow:
		err = parquet.Write(&buf, r)
	case []instRow:
		err = parquet.Write(&buf, r)
	case []bossRow:
		err = parquet.Write(&buf, r)
	case []buildRow:
		err = parquet.Write(&buf, r)
	case []priorityRow:
		err = parquet.Write(&buf, r)
	case []nodeRow:
		err = parquet.Write(&buf, r)
	case []spellRow:
		err = parquet.Write(&buf, r)
	}
	if err != nil {
		t.Fatal(err)
	}
	return store.SizedReaderAt{ReaderAt: bytes.NewReader(buf.Bytes()), Size: int64(buf.Len())}
}

var id501 = int32(501)

func i16(n int16) *int16  { return &n }
func i32p(n int32) *int32 { return &n }

func zone(s string) *string { return &s }

// synthStore holds quests with zones, two cloth head items and a ring, and where they drop.
func synthStore(t *testing.T) store.DataStore {
	const cloth, headSlot, ringSlot = 1, 1, 11
	lo, hi := int32(10), int32(12)
	return store.New(store.ReaderSource{
		"quests": tbl(t, []questRow{{100, i32(11), zone("Dunemark")}, {101, i32(12), zone("Dunemark")}, {102, i32(40), zone("Far Peaks")}}),
		"items": tbl(t, []itemRow{
			{1, "Cloth Hood of the \"Sage\"", 20, 10, 2, 4, cloth, headSlot, -1, []int16{5, 7}, "known"},
			{2, "Plain Cloth Cap", 16, 10, 2, 4, cloth, headSlot, -1, []int16{5, 7}, "known"},
			{6, "Ring of Mind", 18, 10, 2, 4, 0, ringSlot, -1, []int16{5}, "known"},
		}),
		"loot_sources": tbl(t, []lootRow{{1, "creature", 501, 100, 5}, {6, "object", 900, 0, 0}}),
		"npcs":         tbl(t, []npcRow{{501, "Near Wolf", &lo, &hi}}),
		"instances": tbl(t, []instRow{
			{1, "Wolf Den", "dungeon", i16(5), i16(10), i16(14)},
			{2, "Empty Cellar", "dungeon", i16(5), i16(10), i16(14)},
			{3, "Far Keep", "dungeon", i16(5), i16(40), i16(45)},
		}),
		"instance_bosses": tbl(t, []bossRow{{1, "Near Wolf", &id501}, {2, "Nobody", nil}}),
		"talent_builds": tbl(t, []buildRow{
			{"PRIEST", "Discipline", 10, 1, 71, "Penance", 1, "test"},
			{"PRIEST", "Discipline", 11, 2, 72, "Power Word: Shield", 1, "test"},
			{"PRIEST", "Discipline", 12, 3, 72, "Power Word: Shield", 2, "test"},
			{"ROGUE", "Assassination", 10, 1, 81, "Deadly Poison", 1, "test"},
		}),
		"talent_priorities": tbl(t, []priorityRow{{"PRIEST", "Discipline", 1, 71, "Penance", "Core heal"}, {"PRIEST", "Discipline", 2, 72, "Power Word: Shield", "Shield"}}),
		"talent_nodes":      tbl(t, []nodeRow{{71, i32p(1001), "Penance"}, {72, nil, "Power Word: Shield"}, {81, i32p(2001), "Deadly Poison"}}),
	})
}

// synthChar is a level 12 discipline priest with every kind of advice and every kind of gap.
func synthChar() character.Character {
	return character.Character{
		Name: "Testpriest", Class: "PRIEST", Level: ip(12),
		Equipment: map[string]character.EquippedItem{"head": {Slot: "head", ItemID: ip(2), ItemLevel: ip(16), Quality: ip(2)}},
		Quests: []character.Quest{
			{QuestID: 100, Title: "Wolves at the Door", Complete: bp(true)},
			{QuestID: 101, Title: "Gather Peacebloom"},
			{QuestID: 102, Title: "Too Hard", Level: ip(40)},
			{QuestID: 103, Title: "Lost Cause", Level: ip(12), Failed: bp(true)},
		},
		Talents:     &character.Talents{Unspent: ip(2), Trees: []character.TalentTree{{Groups: []character.TalentGroup{{Name: "Discipline", Spent: 5}}}}},
		Professions: []character.Skill{{Name: "Herbalism"}, {Name: "Mining"}},
		Bags: []character.ItemStack{
			{ItemID: 2447, Count: 12, Name: "Peacebloom"},
			{ItemID: 2840, Count: 5, Name: "Copper Bar"},
			{ItemID: 2770, Count: 20, Name: "Copper Ore"},
			{ItemID: 5000, Count: 99, Name: "Blood Shard"},
			{ItemID: 2589, Count: 40, Name: "Linen Cloth"},
			{ItemID: 159, Count: 3, Name: "Refreshing Spring Water"},
		},
	}
}

// synthRogue is a level 12 rogue with an assassination build to follow and no quests.
func synthRogue() character.Character {
	return character.Character{
		Name: "Testrogue", Class: "ROGUE", Level: ip(12), BankCaptured: true,
		Equipment: map[string]character.EquippedItem{"head": {Slot: "head", ItemID: ip(2), ItemLevel: ip(16), Quality: ip(2)}},
		Talents:   &character.Talents{Trees: []character.TalentTree{{Groups: []character.TalentGroup{{Name: "Assassination", Spent: 1}}}}},
	}
}

var synthOpt = Options{Now: time.Unix(1800000000, 0), Client: character.Client{Version: "1.60.0", Build: "64000"}, DatasetBuild: "1.59.0.63000"}

// one wraps a single entry in the file envelope.
func one(d Digest) Envelope { return Envelope{Generated: d.Generated, Characters: []Digest{d}} }

func TestBuildGolden(t *testing.T) {
	cases := []struct {
		name, golden string
		c            character.Character
		ds           bool
	}{
		{"full", "full.lua", synthChar(), true},
		{"rogue", "rogue.lua", synthRogue(), true},
		{"no dataset, bare capture", "bare.lua", character.Character{Key: "Realm:Bare", BankCaptured: true}, false},
	}
	for _, tc := range cases {
		var ds store.DataStore
		if tc.ds {
			ds = synthStore(t)
		}
		got := one(Build(tc.c, ds, synthOpt)).Lua()
		path := filepath.Join("..", "..", "testdata", "digest", tc.golden)
		if *update {
			os.WriteFile(path, []byte(got), 0o644)
		}
		want, err := os.ReadFile(path)
		if err != nil || got != string(want) {
			t.Errorf("%s differs from %s (-update rewrites it):\n%s", tc.name, path, got)
		}
		if err := SlotCheck(got); err != nil {
			t.Errorf("%s: %v", tc.name, err)
		}
		if _, err := lua.Parse(got); err != nil {
			t.Errorf("%s: does not parse: %v", tc.name, err)
		}
	}
}

// Two characters in one capture: each entry is what the single-character build gives, indented, and
// the file is the golden, passes the slot check and parses with both names as keys.
func TestEnvelopeTwoCharacters(t *testing.T) {
	ds := synthStore(t)
	a, b := synthChar(), synthRogue()
	a.Name, b.Name = "Testchar", "Otherchar"
	da, db := Build(a, ds, synthOpt), Build(b, ds, synthOpt)
	got := Envelope{Generated: synthOpt.Now.Unix(), Characters: []Digest{da, db}}.Lua()
	path := filepath.Join("..", "..", "testdata", "digest", "two.lua")
	if *update {
		os.WriteFile(path, []byte(got), 0o644)
	}
	if want, err := os.ReadFile(path); err != nil || got != string(want) {
		t.Errorf("differs from %s (-update rewrites it):\n%s", path, got)
	}
	if err := SlotCheck(got); err != nil {
		t.Error(err)
	}
	for _, d := range []Digest{da, db} {
		if want := strings.ReplaceAll(d.Lua(), "\n", "\n    "); !strings.Contains(got, "["+Quote(d.Character)+"] = "+want+",\n") {
			t.Errorf("entry %s is not the single-character build", d.Character)
		}
	}
	parsed, err := lua.Parse(got)
	root, _ := parsed["WoWCompanionDigest"].(lua.Table)
	chars, _ := root["characters"].(lua.Table)
	if err != nil || root["schema"] != "2.0" || len(chars) != 2 || chars["Testchar"] == nil || chars["Otherchar"] == nil {
		t.Errorf("parse: %v %v", err, parsed)
	}
	if e, _ := chars["Testchar"].(lua.Table); e["schema"] != "1.10" || e["character"] != "Testchar" {
		t.Errorf("entry = %v", e)
	}
}

func TestSelect(t *testing.T) {
	at := func(n int64) *int64 { return &n }
	chars := []character.Character{{Name: "Old", CapturedAt: at(1)}, {Name: "Never"}, {Name: "New", CapturedAt: at(9)}, {Name: "New", CapturedAt: at(5)}}
	kept, skipped := Select(chars)
	if got := names(kept); got != "New,Old,Never" || names(skipped) != "New" {
		t.Errorf("kept %s skipped %s", got, names(skipped))
	}
	var many []character.Character
	for i := 0; i < MaxCharacters+3; i++ {
		many = append(many, character.Character{Name: fmt.Sprint("C", i), CapturedAt: at(int64(i))})
	}
	kept, skipped = Select(many)
	if len(kept) != MaxCharacters || len(skipped) != 3 || kept[0].Name != "C32" || skipped[0].Name != "C2" {
		t.Errorf("kept %d skipped %s", len(kept), names(skipped))
	}
}

func names(cs []character.Character) string {
	var n []string
	for _, c := range cs {
		n = append(n, c.Name)
	}
	return strings.Join(n, ",")
}

func TestCaps(t *testing.T) {
	c := synthChar()
	c.Professions = []character.Skill{{Name: "Mining"}}
	c.Bags = nil
	for i := 0; i < 30; i++ {
		c.Quests = append(c.Quests, character.Quest{QuestID: 200 + i, Title: strings.Repeat("Long Quest Title ", 10), Complete: bp(true)})
		c.Bags = append(c.Bags, character.ItemStack{ItemID: 300 + i, Count: i + 1, Name: "Iron Ore"})
	}
	d := Build(c, synthStore(t), synthOpt)
	max := map[string]int{NextQuests: MaxQuests, Upgrades: MaxUpgrades, KeepMaterials: MaxMaterials, Notes: MaxNotes}
	for _, s := range d.Sections {
		if len(s.Lines) > max[s.ID] {
			t.Errorf("%s has %d lines, cap %d", s.ID, len(s.Lines), max[s.ID])
		}
		for _, l := range s.Lines {
			if n := len([]rune(l.Text)); n == 0 || strings.HasSuffix(l.Text, "...") {
				t.Errorf("%s line %q is empty or cut", s.ID, l.Text)
			}
		}
	}
	if len(d.Sections) < 3 || len(d.Sections[0].Lines) != MaxQuests {
		t.Errorf("sections = %+v", d.Sections)
	}
}

func TestSchema12Sections(t *testing.T) {
	ds := synthStore(t)
	d := Build(synthChar(), ds, synthOpt)
	if d.Schema != "1.10" || len(d.Dungeons) != 1 || d.Dungeons[0].Name != "Wolf Den" || d.Dungeons[0].Item != 1 {
		t.Errorf("dungeons = %+v (schema %s)", d.Dungeons, d.Schema)
	}
	if len(d.Talents) != 3 || d.Talents[0].Spell != 1001 || d.Talents[1].Spell != 0 {
		t.Errorf("talents = %+v", d.Talents)
	}
	// Nothing to show: the sections are left out, as is the whole of them with no dataset.
	for _, c := range []struct {
		name string
		c    character.Character
		ds   store.DataStore
	}{{"no dataset", synthChar(), nil}, {"no level", character.Character{Class: "PRIEST"}, ds}} {
		if d := Build(c.c, c.ds, synthOpt); len(d.Dungeons)+len(d.Talents) != 0 || strings.Contains(one(d).Lua(), "dungeons =") || strings.Contains(one(d).Lua(), "talents =") {
			t.Errorf("%s: %+v", c.name, d)
		}
	}
}

func TestIsMaterial(t *testing.T) {
	cases := []struct {
		item  string
		profs []string
		want  bool
	}{
		{"Copper Bar", []string{"Mining"}, true},
		{"Copper Bar", []string{"Herbalism"}, false},
		{"Copper Bar", nil, false},
		{"Peacebloom", []string{"herbalism"}, true},
		{"Strange Dust", []string{"Enchanting"}, true},
		{"Blood Shard", []string{"Enchanting"}, false}, // quest currency, excluded
		{"Small Glimmering Shard", []string{"Enchanting"}, true},
		{"Barrel of Rum", []string{"Mining"}, false}, // whole words only
		{"Hearthstone", []string{"Blacksmithing"}, false},
		{"Rough Stone", []string{"Blacksmithing"}, true},
		{"Linen Cloth", []string{"Tailoring", "Mining"}, true},
		{"Linen Cloth", []string{"Unknown Craft"}, false},
		{"Infernal Stone", []string{"Blacksmithing", "Engineering"}, false}, // a warlock's reagent, not a smithing stone
		{"Arcane Powder", []string{"Engineering"}, false},                   // a mage's reagent, not blasting powder
		{"Major Healthstone", []string{"Jewelcrafting"}, false},
	}
	for _, tc := range cases {
		if got := isMaterial(tc.item, tc.profs); got != tc.want {
			t.Errorf("isMaterial(%q, %v) = %v", tc.item, tc.profs, got)
		}
	}
}

func TestQuote(t *testing.T) {
	cases := []struct{ in, want string }{
		{"plain", `"plain"`},
		{`say "hi"`, `"say \"hi\""`},
		{`back\slash`, `"back\\slash"`},
		{"two\nlines", `"two\nlines"`},
		{"tab\there\r", `"tab\009here\013"`},
		{"]] -- x", `"]] -- x"`},
		{"naïve", `"naïve"`},
	}
	for _, tc := range cases {
		if got := Quote(tc.in); got != tc.want {
			t.Errorf("Quote(%q) = %s, want %s", tc.in, got, tc.want)
		}
	}
}

// Hostile text in any entry, its key included, stays data and comes back as it went in.
func TestHostileTextStaysData(t *testing.T) {
	hostile := func(n string) Digest {
		return Digest{Schema: "1.2", Build: `"}; os.execute("x") --` + n, Character: "a\nb\"]] f() --" + n, Sections: []Section{{ID: Notes, Lines: []Line{{Text: "\\\"]]; f() --\n" + n}}}}}
	}
	ds := []Digest{hostile("1"), hostile("2")}
	src := Envelope{Generated: 1, Characters: ds}.Lua()
	if err := SlotCheck(src); err != nil {
		t.Fatal(err)
	}
	got, err := lua.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	chars, _ := got["WoWCompanionDigest"].(lua.Table)["characters"].(lua.Table)
	for _, d := range ds {
		e, _ := chars[d.Character].(lua.Table)
		if len(chars) != 2 || e["build"] != d.Build || e["character"] != d.Character {
			t.Errorf("round trip of %q: %v", d.Character, chars)
		}
	}
}

func TestDigestPath(t *testing.T) {
	cases := []struct{ in, want string }{
		{"/home/u/Games/wow/_retail_/WTF/Account/ACCT1/SavedVariables/ForeverScribe.lua", "/home/u/Games/wow/_retail_/Interface/AddOns/WoWCompanion_Digest/Digest.lua"},
		{"/Applications/World of Warcraft/_retail_/WTF/Account/ACCT1/SavedVariables/ForeverScribe.lua", "/Applications/World of Warcraft/_retail_/Interface/AddOns/WoWCompanion_Digest/Digest.lua"},
		{`C:\Program Files (x86)\World of Warcraft\_retail_\WTF\Account\ACCT1\SavedVariables\ForeverScribe.lua`, `C:\Program Files (x86)\World of Warcraft\_retail_\Interface\AddOns\WoWCompanion_Digest\Digest.lua`},
		{`C:/Games/WoW/WTF/Account/ACCT1/SavedVariables/ForeverScribe.lua`, `C:/Games/WoW/Interface/AddOns/WoWCompanion_Digest/Digest.lua`},
		{`\\nas\games\WoW\wtf\account\A\savedvariables\ForeverScribe.lua`, `\\nas\games\WoW\Interface\AddOns\WoWCompanion_Digest\Digest.lua`},
		{"/tmp/ForeverScribe.lua", ""},
		{"/g/WTF/Account/SavedVariables/ForeverScribe.lua", ""},
		{"/g/WTF/Account/A/SavedVariables/", ""},
		{"", ""},
	}
	for _, tc := range cases {
		got, err := DigestPath(tc.in)
		if (err != nil) != (tc.want == "") || got != tc.want {
			t.Errorf("DigestPath(%q) = %q, %v; want %q", tc.in, got, err, tc.want)
		}
	}
}

func TestWriteIsAtomicAndNeedsTheAddon(t *testing.T) {
	game := t.TempDir()
	path := filepath.Join(game, "Interface", "AddOns", SlotAddon, "Digest.lua")
	if err := CheckInstalled(path); err == nil || !strings.Contains(err.Error(), "not installed") {
		t.Errorf("missing addon: %v", err)
	}
	if err := Write(path, Envelope{}); err == nil {
		t.Error("Write created the missing addon folder")
	}
	os.MkdirAll(filepath.Dir(path), 0o755)
	os.WriteFile(path, []byte("WoWCompanionDigest = {}\n"), 0o644)
	if err := CheckInstalled(path); err != nil {
		t.Fatal(err)
	}
	if err := Write(path, one(Digest{Schema: "1.2", Character: "A"})); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); !strings.Contains(string(b), `["A"] = {`) {
		t.Errorf("not replaced: %s", b)
	}
	if left, _ := filepath.Glob(filepath.Join(filepath.Dir(path), "*.tmp")); len(left) != 0 {
		t.Errorf("temp files left: %v", left)
	}
}

func TestClipNeverCuts(t *testing.T) {
	long := "Pick up: Restored Plate Helm of the Fallen Champion from Someone Very Far Away In The Mountains Of Testland (Dunemark)"
	for _, tc := range []struct{ in, want string }{
		{"head: Cloth Hood (ilvl 20)", "head: Cloth Hood (ilvl 20)"},
		{long, "Pick up: Restored Plate Helm of the Fallen Champion from Someone Very Far Away In The Mountains Of Testland"},
		{strings.Repeat("x", 120), strings.Repeat("x", 120)}, // no shorter whole form: kept whole
	} {
		if got := clip(tc.in); got != tc.want {
			t.Errorf("clip(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestOneObjective(t *testing.T) {
	for in, want := range map[string]string{
		"Test: 0/1 Find A":                    "Test: 0/1 Find A",
		"Test: 0/1 Find A; 1/1 List B; 2/2 C": "Test: 0/1 Find A; +2 more",
		"Test: 0/1 Find A; 1/1 List B":        "Test: 0/1 Find A; +1 more",
	} {
		if got := oneObjective(in); got != want {
			t.Errorf("%q = %q, want %q", in, got, want)
		}
	}
}

// structuredDigest holds one line of each kind, a class quest, an elite quest, a multi-objective quest
// with an optional objective, a pick-up and the talent line; titles carry apostrophes, quotes and colons.
func structuredDigest() Digest {
	return Digest{Schema: Schema, Generated: 1800000000, Build: "1.60.0.64000", Character: "Testpriest", Sections: []Section{
		{ID: NextQuests, Lines: []Line{
			{Text: "You have 2 unspent talent points: Penance", Kind: KindTalent, Points: 2},
			{Text: "Class quest: pick up The Skybreaker's Bulwark from Seena Sk...", Quest: 900, Kind: KindPickup, Title: "The Skybreaker's Bulwark", Level: 10, Class: true, NPC: "Seena Skyward", Zone: "Dunemark"},
			{Text: "Turn in \"Blood\" Tithe: Part 2 (Dunemark)", Quest: 901, Kind: KindTurnIn, Title: "\"Blood\" Tithe: Part 2", Level: 9, Elite: true, NPC: "Aamelia O'Windfield", Zone: "Dunemark"},
			{Text: "0/6 Windsong Crawler Meat; +2 more (Dunemark)", Quest: 902, Kind: KindProgress, Title: "Crawler Season: A Very Long Quest Title That Is Never Truncated In The New Fields", Level: 12, Zone: "Dunemark",
				Objectives: []Objective{{"Windsong Crawler Meat", 0, 6, true}, {"Find Aamelia Windfield", 0, 1, true}, {"Optional: Bring Tea", 0, 0, false}}},
			{Text: "Pick up: Wolves at the Door (Dunemark)", Quest: 903, Kind: KindPickup, Title: "Wolves at the Door", Level: 11, Zone: "Dunemark"},
			{Text: "Game data not downloaded yet; see Setup", Kind: KindNote},
		}},
		{ID: Upgrades, Summary: "1 upgrade you can get", Lines: []Line{{Text: "Head: Cloth Hood. From level 14, big upgrade, fills an empty slot", Item: 1, Slot: "Head", ILvl: 20, AtLevel: 14,
			Wear: "From level 14", Gain: "Big upgrade", Replaces: "fills an empty slot", How: "Drops sometimes from Near Wolf in Wolf Den (levels 10 to 14)", PathKind: "dungeon"}}},
	}}
}

func TestStructuredLinesGolden(t *testing.T) {
	got := one(structuredDigest()).Lua()
	path := filepath.Join("..", "..", "testdata", "digest", "structured.lua")
	if *update {
		os.WriteFile(path, []byte(got), 0o644)
	}
	want, err := os.ReadFile(path)
	if err != nil || got != string(want) {
		t.Errorf("differs from %s (-update rewrites it):\n%s", path, got)
	}
	if err := SlotCheck(got); err != nil {
		t.Error(err)
	}
	if _, err := lua.Parse(got); err != nil {
		t.Errorf("does not parse: %v", err)
	}
}

// A progress line takes every objective of the logged quest, the optional one too, uncut.
func TestBuildProgressObjectives(t *testing.T) {
	c := synthChar()
	c.Quests = []character.Quest{{QuestID: 101, Title: "Gather Peacebloom", Objectives: []character.Objective{
		{Text: "Peacebloom: 3/8", Have: ip(3), Need: ip(8)},
		{Text: "Bring a flask of tea", Done: bp(false)},
	}}}
	var got Line
	for _, l := range Build(c, synthStore(t), synthOpt).Sections[0].Lines {
		if l.Quest == 101 {
			got = l
		}
	}
	want := []Objective{{"Peacebloom", 3, 8, true}, {Text: "Bring a flask of tea"}}
	if got.Kind != KindProgress || got.Title != "Gather Peacebloom" || got.Level != 12 || got.Zone != "Dunemark" || fmt.Sprint(got.Objectives) != fmt.Sprint(want) {
		t.Errorf("got %+v", got)
	}
}

// A character with a play style gets the optional stats entry, short and plain; one without gets none.
func TestStatsEntry(t *testing.T) {
	ds := synthStore(t)
	c := synthChar()
	if d := Build(c, ds, synthOpt); d.Stats != nil {
		t.Errorf("stats without a play style: %+v", d.Stats)
	}
	c.Profile = &character.Profile{Role: "healer", Focus: "leveling", Spec: "Discipline"}
	d := Build(c, ds, synthOpt)
	if d.Stats == nil || len(d.Stats.Insights) == 0 || len(d.Stats.Insights) > 3 || d.Stats.Profile != "Discipline · leveling" || len(d.Stats.Asks) == 0 {
		t.Fatalf("stats = %+v", d.Stats)
	}
	for _, in := range d.Stats.Insights {
		if utf8.RuneCountInString(in) > 90 || strings.Contains(in, "%") {
			t.Errorf("insight %q", in)
		}
	}
	got := one(d).Lua()
	if err := SlotCheck(got); err != nil {
		t.Error(err)
	}
	if _, err := lua.Parse(got); err != nil || !strings.Contains(got, "stats = { profile = ") {
		t.Errorf("lua: %v", err)
	}
}

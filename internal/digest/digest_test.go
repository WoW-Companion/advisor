package digest

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	}
	if err != nil {
		t.Fatal(err)
	}
	return store.SizedReaderAt{ReaderAt: bytes.NewReader(buf.Bytes()), Size: int64(buf.Len())}
}

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

var synthOpt = Options{Now: time.Unix(1800000000, 0), Client: character.Client{Version: "1.60.0", Build: "64000"}, DatasetBuild: "1.59.0.63000"}

func TestBuildGolden(t *testing.T) {
	cases := []struct {
		name, golden string
		c            character.Character
		ds           bool
	}{
		{"full", "full.lua", synthChar(), true},
		{"no dataset, bare capture", "bare.lua", character.Character{Key: "Realm:Bare", BankCaptured: true}, false},
	}
	for _, tc := range cases {
		var ds store.DataStore
		if tc.ds {
			ds = synthStore(t)
		}
		got := Build(tc.c, ds, synthOpt).Lua()
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
			if n := len([]rune(l.Text)); n > MaxText || n == 0 {
				t.Errorf("%s line %q is %d characters", s.ID, l.Text, n)
			}
		}
	}
	if len(d.Sections) < 3 || len(d.Sections[0].Lines) != MaxQuests {
		t.Errorf("sections = %+v", d.Sections)
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

func TestHostileTextStaysData(t *testing.T) {
	d := Digest{Schema: 1, Build: `"}; os.execute("x") --`, Character: "a\nb", Sections: []Section{{ID: Notes, Lines: []Line{{Text: "\\\"]]; f() --\n"}}}}}
	src := d.Lua()
	if err := SlotCheck(src); err != nil {
		t.Fatal(err)
	}
	got, err := lua.Parse(src)
	if err != nil || got["WoWCompanionDigest"].(lua.Table)["build"] != d.Build {
		t.Errorf("round trip: %v, %v", got, err)
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
	if err := Write(path, Digest{}); err == nil {
		t.Error("Write created the missing addon folder")
	}
	os.MkdirAll(filepath.Dir(path), 0o755)
	os.WriteFile(path, []byte("WoWCompanionDigest = {}\n"), 0o644)
	if err := CheckInstalled(path); err != nil {
		t.Fatal(err)
	}
	if err := Write(path, Digest{Schema: 1, Character: "A"}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); !strings.Contains(string(b), `character = "A"`) {
		t.Errorf("not replaced: %s", b)
	}
	if left, _ := filepath.Glob(filepath.Join(filepath.Dir(path), "*.tmp")); len(left) != 0 {
		t.Errorf("temp files left: %v", left)
	}
}

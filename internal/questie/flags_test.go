package questie

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const classicEnum = `local npcFlags = {
    Forever = { GOSSIP = 1, QUEST_GIVER = 2, VENDOR = 4, FLIGHT_MASTER = 8, TRAINER = 16, INNKEEPER = 128, BANKER = 256, REPAIR = 16384 },
    Retail = { TRAINER = 16, VENDOR = 128, REPAIR = 4096, FLIGHT_MASTER = 8192, INNKEEPER = 65536 },
}`

const modernEnum = `local npcFlags = {
    Forever = { TRAINER = 16, VENDOR = 128, REPAIR = 4096, FLIGHT_MASTER = 8192, INNKEEPER = 65536 },
    Classic = { VENDOR = 4, TRAINER = 16, REPAIR = 16384 },
}`

func flagRows() []row {
	rows := fixtureRows()
	return append(rows,
		row{"Npc", 2622, "S", map[int]any{1: "Sly Garrett", 14: "Shady Goods", 15: 4}},
		row{"Npc", 2622, "7", map[int]any{901: []any{[]any{40.0, 70.0}}}},
		row{"Npc", 2623, "S", map[int]any{1: "Forge Hand", 14: "Armorer", 15: 16384 | 4}},
		row{"Npc", 2623, "7", map[int]any{901: []any{[]any{41.0, 71.0}}}})
}

func withEnum(t *testing.T, game, body string) {
	t.Helper()
	dir := filepath.Join(game, "Interface", "AddOns", "QuestieDB", "src", "corrections", "enum")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "expansions.lua"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestClassicFlagLayoutFindsFlagFourVendor(t *testing.T) {
	game := install(t, header, flagRows())
	withEnum(t, game, classicEnum)
	ix, st := OpenGame(game)
	if ix == nil {
		t.Fatalf("not readable: %+v", st)
	}
	if !strings.HasPrefix(ix.FlagLayout, "enum Forever") {
		t.Errorf("layout %q", ix.FlagLayout)
	}
	if e := ix.Npcs[2622]; !e.IsVendor() || e.Flags&FlagRepair != 0 {
		t.Errorf("Sly Garrett %+v", e)
	}
	if e := ix.Npcs[2623]; e.Flags&FlagRepair == 0 || !e.IsVendor() {
		t.Errorf("repair NPC %+v", e)
	}
}

func TestModernFlagLayoutStillWorks(t *testing.T) {
	rows := append(fixtureRows(), row{"Npc", 2622, "S", map[int]any{1: "Modern Vendor", 15: 128 | 4096}},
		row{"Npc", 2622, "7", map[int]any{901: []any{[]any{40.0, 70.0}}}})
	game := install(t, header, rows)
	withEnum(t, game, modernEnum)
	ix, _ := OpenGame(game)
	if ix == nil {
		t.Fatal("not readable")
	}
	if e := ix.Npcs[2622]; !e.IsVendor() || e.Flags&FlagRepair == 0 {
		t.Errorf("modern vendor %+v (%s)", e, ix.FlagLayout)
	}
}

func TestUnresolvedFlavorFallsBackToClassicEra(t *testing.T) {
	game := install(t, header, flagRows())
	ix, _ := OpenGame(game)
	if ix == nil {
		t.Fatal("not readable")
	}
	if !strings.HasPrefix(ix.FlagLayout, "fallback classic-era") {
		t.Errorf("layout %q", ix.FlagLayout)
	}
	if !ix.Npcs[2622].IsVendor() || ix.Npcs[2623].Flags&FlagRepair == 0 {
		t.Errorf("fallback flags %+v %+v", ix.Npcs[2622], ix.Npcs[2623])
	}
	if got := ReadFlagLayout(Status{Flavor: "Retail"}).Source; !strings.HasPrefix(got, "fallback modern") {
		t.Errorf("retail fallback %q", got)
	}
}

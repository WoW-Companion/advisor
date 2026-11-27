package questie

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// The npcFlags bits the advisor reads, in one fixed numbering (the "canonical" bits). QuestieDB numbers
// them differently per game flavor, so the index translates each NPC's native flags into these by the
// bit's name when it is built; everything outside this package tests only the canonical bits.
const (
	FlagGossip        = 0x1
	FlagQuestGiver    = 0x2
	FlagTrainer       = 0x10 // any trainer; ClassTrainer and ProfTrainer are its kinds
	FlagClassTrainer  = 0x20
	FlagProfTrainer   = 0x40
	FlagVendor        = 0x80
	FlagVendorAmmo    = 0x100
	FlagVendorFood    = 0x200
	FlagVendorPoison  = 0x400
	FlagVendorReagent = 0x800
	FlagRepair        = 0x1000
	FlagFlightMaster  = 0x2000
	FlagInnkeeper     = 0x10000
	FlagBanker        = 0x20000
)

var canonicalFlags = map[string]int{
	"GOSSIP": FlagGossip, "QUEST_GIVER": FlagQuestGiver, "TRAINER": FlagTrainer, "TRAINER_CLASS": FlagClassTrainer,
	"TRAINER_PROFESSION": FlagProfTrainer, "VENDOR": FlagVendor, "VENDOR_AMMO": FlagVendorAmmo, "VENDOR_FOOD": FlagVendorFood,
	"VENDOR_POISON": FlagVendorPoison, "VENDOR_REAGENT": FlagVendorReagent, "REPAIR": FlagRepair, "FLIGHT_MASTER": FlagFlightMaster,
	"INNKEEPER": FlagInnkeeper, "BANKER": FlagBanker,
}

// FlagLayout maps a flag's name to its bit in one flavor's npcFlags. Source says where it came from:
// "enum <path>" when read from the install, else "fallback classic-era" or "fallback modern" with the reason.
type FlagLayout struct {
	Source string
	bits   map[string]int
}

// Fallback layouts, used only when the install's enum file cannot be read.
var (
	classicFlagBits = map[string]int{"GOSSIP": 1, "QUEST_GIVER": 2, "VENDOR": 4, "FLIGHT_MASTER": 8, "TRAINER": 16,
		"INNKEEPER": 128, "BANKER": 256, "REPAIR": 16384}
	modernFlagBits = map[string]int{"GOSSIP": 1, "QUEST_GIVER": 2, "TRAINER": 16, "TRAINER_CLASS": 32, "TRAINER_PROFESSION": 64,
		"VENDOR": 128, "VENDOR_AMMO": 256, "VENDOR_FOOD": 512, "VENDOR_POISON": 1024, "VENDOR_REAGENT": 2048,
		"REPAIR": 4096, "FLIGHT_MASTER": 8192, "INNKEEPER": 65536, "BANKER": 131072}
)

type flagTable struct {
	path string
	bits map[string]int
}

var (
	luaOpen  = regexp.MustCompile(`(?:\[\s*['"]([^'"]+)['"]\s*\]|([A-Za-z_][A-Za-z0-9_]*))\s*=\s*\{`)
	luaFlag  = regexp.MustCompile(`\b([A-Z][A-Z_]*)\s*=\s*(0[xX][0-9a-fA-F]+|\d+)`)
	luaShift = regexp.MustCompile(`\b([A-Z][A-Z_]*)\s*=\s*(\d+)\s*<<\s*(\d+)`)
)

// parseFlagTables reads the Lua text as text (it is never run) and returns every table that names
// both VENDOR and TRAINER bits, with its dotted path of enclosing table names.
func parseFlagTables(src string) []flagTable {
	var out []flagTable
	var walk func(body, path string)
	walk = func(body, path string) {
		own := strings.Builder{}
		i := 0
		for i < len(body) {
			loc := luaOpen.FindStringSubmatchIndex(body[i:])
			if loc == nil {
				own.WriteString(body[i:])
				break
			}
			own.WriteString(body[i : i+loc[0]])
			name := ""
			for _, g := range []int{2, 4} {
				if loc[g] >= 0 {
					name = body[i+loc[g] : i+loc[g+1]]
				}
			}
			start := i + loc[1]
			depth, j := 1, start
			for ; j < len(body) && depth > 0; j++ {
				switch body[j] {
				case '{':
					depth++
				case '}':
					depth--
				}
			}
			end := max(start, j-1)
			walk(body[start:end], strings.TrimPrefix(path+"."+name, "."))
			i = j
		}
		bits := map[string]int{}
		text := own.String()
		for _, m := range luaFlag.FindAllStringSubmatch(text, -1) {
			if n, err := strconv.ParseInt(m[2], 0, 64); err == nil {
				bits[m[1]] = int(n)
			}
		}
		for _, m := range luaShift.FindAllStringSubmatch(text, -1) {
			a, _ := strconv.Atoi(m[2])
			s, _ := strconv.Atoi(m[3])
			bits[m[1]] = a << s
		}
		if bits["VENDOR"] > 0 && bits["TRAINER"] > 0 {
			out = append(out, flagTable{path: path, bits: bits})
		}
	}
	walk(src, "")
	return out
}

// classicEra reports whether the flavor is a classic-era build (Forever, Classic, Era, Vanilla, or unnamed).
func classicEra(flavor string) bool {
	f := strings.ToLower(flavor)
	if f == "" {
		return true
	}
	for _, w := range []string{"forever", "classic", "era", "vanilla"} {
		if strings.Contains(f, w) {
			return true
		}
	}
	return false
}

// pickFlagTable chooses the table for flavor: the one whose path names the flavor, else the only one.
func pickFlagTable(ts []flagTable, flavor string) (flagTable, bool) {
	f := strings.ToLower(flavor)
	if f != "" {
		for _, t := range ts {
			if strings.Contains(strings.ToLower(t.path), f) {
				return t, true
			}
		}
	}
	if len(ts) == 1 {
		return ts[0], true
	}
	return flagTable{}, false
}

// ReadFlagLayout reads the npcFlags table for the flavor from the install's src/corrections/enum/expansions.lua.
// When that file or the flavor's table cannot be found, a classic-era build gets the classic-era layout and
// any other the modern one; Source says which, and why.
func ReadFlagLayout(st Status) FlagLayout {
	dir := filepath.Dir(st.TOC)
	reason := "no enum/expansions.lua"
	for _, base := range []string{dir, filepath.Join(dir, "support", "Forever")} {
		p := filepath.Join(base, "src", "corrections", "enum", "expansions.lua")
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		ts := parseFlagTables(string(b))
		if t, ok := pickFlagTable(ts, st.Flavor); ok {
			return FlagLayout{Source: "enum " + t.path, bits: t.bits}
		}
		reason = "no npcFlags table for flavor " + strconv.Quote(st.Flavor)
	}
	if classicEra(st.Flavor) {
		return FlagLayout{Source: "fallback classic-era (" + reason + ")", bits: classicFlagBits}
	}
	return FlagLayout{Source: "fallback modern (" + reason + ")", bits: modernFlagBits}
}

// canon translates a native npcFlags value into the canonical bits by name.
func (f FlagLayout) canon(native int) int {
	if f.bits == nil {
		return native
	}
	out := 0
	for name, bit := range f.bits {
		if bit > 0 && native&bit == bit {
			out |= canonicalFlags[name]
		}
	}
	return out
}

// has reports whether the native value carries any of the named bits.
func (f FlagLayout) has(native int, names ...string) bool {
	for _, n := range names {
		if b := f.bits[n]; b > 0 && native&b == b {
			return true
		}
	}
	return false
}

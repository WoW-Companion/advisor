package questie

import "strings"

// Race mask bits. A bit is not always 1<<(id-1): Forever's Skyborne has one race id per side
// (95 Alliance, 96 Horde) that Questie masks as bits 32 and 33, so masks need 64 bits.
const (
	bitHuman, bitOrc, bitDwarf, bitNightElf, bitUndead, bitTauren uint64 = 1 << 0, 1 << 1, 1 << 2, 1 << 3, 1 << 4, 1 << 5
	bitGnome, bitTroll, bitGoblin, bitBloodElf, bitDraenei        uint64 = 1 << 6, 1 << 7, 1 << 8, 1 << 9, 1 << 10
	bitWorgen, bitPandaren                                        uint64 = 1 << 21, 1 << 23
	BitSkyborneAlliance, BitSkyborneHorde                         uint64 = 1 << 32, 1 << 33
)

// Side masks: every known race bit of one side. Pandaren take either side.
const (
	AllianceMask = bitHuman | bitDwarf | bitNightElf | bitGnome | bitDraenei | bitWorgen | bitPandaren | BitSkyborneAlliance
	HordeMask    = bitOrc | bitUndead | bitTauren | bitTroll | bitGoblin | bitBloodElf | bitPandaren | BitSkyborneHorde
)

var raceBits = map[string]uint64{"human": bitHuman, "orc": bitOrc, "dwarf": bitDwarf, "nightelf": bitNightElf,
	"scourge": bitUndead, "undead": bitUndead, "tauren": bitTauren, "gnome": bitGnome, "troll": bitTroll,
	"goblin": bitGoblin, "bloodelf": bitBloodElf, "draenei": bitDraenei, "worgen": bitWorgen, "pandaren": bitPandaren}

var skyborne = map[string]bool{"skyborne": true, "skyborn": true, "windshaperskyborne": true, "highorderskyborne": true}

func normRace(s string) string {
	return strings.ToLower(strings.NewReplacer(" ", "", "'", "", "-", "", "_", "").Replace(s))
}

// SideMaskOf is the full race mask of a faction ("Horde" or "Alliance"); 0 when it is neither.
func SideMaskOf(faction string) uint64 {
	switch strings.ToLower(strings.TrimSpace(faction)) {
	case "horde":
		return HordeMask
	case "alliance":
		return AllianceMask
	}
	return 0
}

// RaceBit is the mask bit of a capture's race token; ok is false for a race the table does not know.
// Skyborne needs the faction to pick its side's bit.
func RaceBit(race, faction string) (bit uint64, ok bool) {
	n := normRace(race)
	if skyborne[n] {
		switch SideMaskOf(faction) {
		case HordeMask:
			return BitSkyborneHorde, true
		case AllianceMask:
			return BitSkyborneAlliance, true
		}
		return 0, false
	}
	bit, ok = raceBits[n]
	return
}

// RaceMasks resolves the player's race rule: the bit when the race is known, else the side's full
// mask as a fallback (fellback true), else neither (the race cannot be checked).
func RaceMasks(race, faction string) (bit, side uint64, fellback bool) {
	if b, ok := RaceBit(race, faction); ok {
		return b, 0, false
	}
	if s := SideMaskOf(faction); s != 0 {
		return 0, s, true
	}
	return 0, 0, false
}

// raceAllows reports whether a quest's race mask admits the player.
func (w Who) raceAllows(mask uint64) bool {
	switch {
	case mask == 0:
		return true
	case w.RaceMask != 0:
		return mask&w.RaceMask != 0
	case w.RaceID > 0:
		return mask&(1<<(w.RaceID-1)) != 0
	case w.SideMask != 0:
		return mask&w.SideMask != 0
	}
	return true
}

// RaceAllows is raceAllows for callers outside the package.
func (w Who) RaceAllows(mask uint64) bool { return w.raceAllows(mask) }

// FactionAdmits reports whether a friendlyToFaction value ("A", "H", "AH") includes the player's side;
// an unknown side admits everything. It is the one test of "friendly to the player's side".
func FactionAdmits(faction, side string) bool {
	return side == "" || strings.Contains(faction, side)
}

// Hostile reports whether every NPC starter of the quest is friendly only to the other side, so the
// player cannot take it from them. Unknown factions and object starters are never hostile.
func (ix *Index) Hostile(q *Quest, w Who) bool {
	if w.Side == "" || q == nil || len(q.Starters) == 0 {
		return false
	}
	for _, r := range q.Starters {
		if r.Object {
			return false
		}
		f := ix.NpcFaction(r.ID)
		if f == "" || FactionAdmits(f, w.Side) {
			return false
		}
	}
	return true
}

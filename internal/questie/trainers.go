package questie

import (
	"sort"
	"strings"
)

// trainerFlag is the trainer bit of an NPC's npcFlags.
const trainerFlag = 0x10

// Trainer is a profession trainer found among the NPCs: Tier is the highest skill it teaches
// (75, 150, 225, 300 or 375).
type Trainer struct {
	ID   int
	Name string
	Sub  string
	Tier int
}

var professionNames = []string{"Blacksmithing", "Tailoring", "Leatherworking", "Alchemy", "Enchanting",
	"Engineering", "Cooking", "Fishing", "First Aid", "Mining", "Herbalism", "Skinning"}

// persons are the nouns a rank-titled trainer is named by: "Journeyman Blacksmith".
var persons = map[string]string{"blacksmith": "Blacksmithing", "tailor": "Tailoring",
	"leatherworker": "Leatherworking", "alchemist": "Alchemy", "enchanter": "Enchanting", "engineer": "Engineering"}

var ranks = map[string]int{"journeyman": 150, "expert": 225, "artisan": 300, "master": 375}

// TrainerOf says which profession the NPC trains and up to which skill. The subname decides:
// "<Profession> Trainer" and "Junior <Profession> Trainer" teach up to 75; a rank word (Journeyman
// 150, Expert 225, Artisan 300, Master 375) before the profession's person noun gives that tier.
// A subname with "Supplies" is a vendor. A bare person noun ("Blacksmith") counts only with the
// trainer bit of npcFlags, at tier 75.
func TrainerOf(e Entity) (profession string, tier int, ok bool) {
	sub := strings.ToLower(strings.Join(strings.Fields(e.Sub), " "))
	if sub == "" || strings.Contains(sub, "supplies") {
		return "", 0, false
	}
	flagged := e.Flags&trainerFlag != 0
	for _, p := range professionNames {
		if strings.Contains(sub, strings.ToLower(p)+" trainer") {
			return p, 75, true
		}
	}
	words := strings.Fields(sub)
	for i, w := range words {
		prof, isPerson := persons[w]
		if !isPerson {
			continue
		}
		if i > 0 {
			if t, isRank := ranks[words[i-1]]; isRank {
				return prof, t, true
			}
		}
		if flagged && len(words) == 1 {
			return prof, 75, true
		}
	}
	return "", 0, false
}

// ProfessionTrainers lists the NPCs that train the profession, by id.
func (ix *Index) ProfessionTrainers(profession string) []Trainer {
	if ix == nil {
		return nil
	}
	var out []Trainer
	for id, e := range ix.Npcs {
		if p, tier, ok := TrainerOf(e); ok && strings.EqualFold(p, profession) {
			out = append(out, Trainer{ID: id, Name: e.Name, Sub: e.Sub, Tier: tier})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

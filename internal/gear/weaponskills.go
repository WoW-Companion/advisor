package gear

import (
	"fmt"
	"sort"
	"strings"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/store"
)

// Weapon skills from the dataset's weapon_skills (class against skill: learnable, default known) and
// weapon_masters (who teaches what, where, for how much). Forever differs from Classic here, so the
// advice reads these tables and the character's captured skills, never memory. A dataset without the
// tables gives no facts: callers say so and never guess.

// weaponSkillGroup is the capture's skill group that holds weapon skills.
const weaponSkillGroup = "Weapon Skills"

// skillOfSubclass is the client skill line a weapon subclass is used with.
var skillOfSubclass = map[int]int64{
	axe1H: 44, axe2H: 172, bow: 45, gun: 46, mace1H: 54, mace2H: 160, polearm: 229, sword1H: 43,
	sword2H: 55, staff: 136, fist: 473, dagger: 173, thrown: 176, crossbow: 226, wand: 228,
}

// skillAlias maps the names the game client shows in the skill list to the dataset's skill names.
var skillAlias = map[string]string{"swords": "one-handed swords", "axes": "one-handed axes", "maces": "one-handed maces"}

// Master is a Weapon Master and what they teach, for the player's faction.
type Master struct {
	ID       int64
	Name     string
	Faction  string
	City     string
	Place    string
	X, Y     float64
	Skills   []int64
	Cost     []int64 // copper, aligned with Skills
	MinLevel []int64 // aligned with Skills
}

// teaches is the index of the skill in the master's list, or -1.
func (m Master) teaches(skill int64) int {
	for i, s := range m.Skills {
		if s == skill {
			return i
		}
	}
	return -1
}

// Where is "Orgrimmar, Valley of Honor" (the city alone when no district is known).
func (m Master) Where() string {
	if m.Place == "" {
		return m.City
	}
	return m.City + ", " + m.Place
}

// WeaponSkillFacts is what the dataset says about the class's weapon skills.
type WeaponSkillFacts struct {
	// Have is false when the dataset lacks the weapon-skill tables: nothing else is set.
	Have      bool
	Names     map[int64]string // skill line id -> dataset name, for every skill the tables list
	Learnable map[int64]bool   // the class's learnable skills
	MinLevel  map[int64]int
	Masters   []Master // masters of the character's faction
	// Pos is the zone the character was last known to be in, "" when unknown.
	Pos string
}

// LoadWeaponSkills reads the class's weapon skills and the faction's masters; Have is false when the
// dataset has no weapon_skills table, or the class is unknown.
func LoadWeaponSkills(c character.Character, ds store.DataStore) WeaponSkillFacts {
	var f WeaponSkillFacts
	cid, ok := classIDs[strings.ToUpper(c.Class)]
	if ds == nil || !ok {
		return f
	}
	all, err := ds.Filter("weapon_skills", store.Range{Column: "class_id", Min: int64(0), Max: int64(99)})
	if err != nil || len(all) == 0 {
		return f
	}
	f.Names, f.Learnable, f.MinLevel = map[int64]string{}, map[int64]bool{}, map[int64]int{}
	for _, r := range all {
		id := i64(r["skill_line_id"])
		name, _ := r["skill_name"].(string)
		f.Names[id] = name
		if i64(r["class_id"]) == cid {
			if b, _ := r["learnable"].(bool); b {
				f.Learnable[id] = true
				f.MinLevel[id] = int(i64(r["min_level"]))
			}
		}
	}
	f.Have = true
	if p := c.LastKnownPosition(); p != nil {
		f.Pos = p.Zone
	}
	rows, err := ds.Filter("weapon_masters", store.Range{Column: "npc_id", Min: int64(0), Max: int64(1 << 40)})
	if err != nil {
		return f
	}
	for _, r := range rows {
		m := Master{ID: i64(r["npc_id"]), X: f64(r["x"]), Y: f64(r["y"])}
		m.Name, _ = r["name"].(string)
		m.Faction, _ = r["faction"].(string)
		m.City, _ = r["city"].(string)
		m.Place, _ = r["place"].(string)
		m.Skills, m.Cost, m.MinLevel = ints(r["skills"]), ints(r["cost_copper"]), ints(r["min_level"])
		if c.Faction != "" && !strings.EqualFold(m.Faction, c.Faction) {
			continue
		}
		f.Masters = append(f.Masters, m)
	}
	sort.SliceStable(f.Masters, func(i, j int) bool { return f.Masters[i].Name < f.Masters[j].Name })
	return f
}

func f64(v any) float64 {
	x, _ := v.(float64)
	return x
}

func ints(v any) []int64 {
	l, _ := v.([]any)
	out := make([]int64, 0, len(l))
	for _, x := range l {
		out = append(out, i64(x))
	}
	return out
}

// Known reports whether the character's capture lists the skill (by id, else by name). captured is
// false when the capture holds no skills at all: then nothing is known either way.
func (f WeaponSkillFacts) Known(c character.Character, skill int64) (known, captured bool) {
	if !c.SkillsCaptured {
		return false, false
	}
	want := strings.ToLower(f.Names[skill])
	for _, s := range c.Skills {
		if s.SkillID != nil && int64(*s.SkillID) == skill {
			return true, true
		}
		if s.Group != "" && !strings.EqualFold(s.Group, weaponSkillGroup) {
			continue
		}
		n := strings.ToLower(s.Name)
		if a, ok := skillAlias[n]; ok {
			n = a
		}
		if n == want {
			return true, true
		}
	}
	return false, true
}

// Lacking is the class's learnable skills the character has not learned, by skill id; nil when the
// capture holds no skills.
func (f WeaponSkillFacts) Lacking(c character.Character) []int64 {
	var out []int64
	for id := range f.Learnable {
		if known, captured := f.Known(c, id); captured && !known {
			out = append(out, id)
		}
	}
	sort.Slice(out, func(i, j int) bool { return f.Names[out[i]] < f.Names[out[j]] })
	return out
}

// Taught is a master's offer of one skill.
type Taught struct {
	Master   Master
	Cost     int64
	MinLevel int
}

// MastersFor lists the faction's masters of the skill, the nearest first: the one in the zone the
// character stands in, then by city and name so the order is stable.
func (f WeaponSkillFacts) MastersFor(skill int64) []Taught {
	var out []Taught
	for _, m := range f.Masters {
		if i := m.teaches(skill); i >= 0 {
			t := Taught{Master: m}
			if i < len(m.Cost) {
				t.Cost = m.Cost[i]
			}
			if i < len(m.MinLevel) {
				t.MinLevel = int(m.MinLevel[i])
			}
			out = append(out, t)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := strings.EqualFold(out[i].Master.City, f.Pos), strings.EqualFold(out[j].Master.City, f.Pos)
		if a != b {
			return a
		}
		return out[i].Master.City < out[j].Master.City
	})
	return out
}

// SkillOfItem is the skill line a weapon of the subclass needs; 0 for anything else.
func SkillOfItem(subclass int) int64 { return skillOfSubclass[subclass] }

// CostText is a price in copper in plain words: "10s", "1g", "1g 50s".
func CostText(copper int64) string {
	g, s, cp := copper/10000, copper%10000/100, copper%100
	var parts []string
	if g > 0 {
		parts = append(parts, fmt.Sprintf("%dg", g))
	}
	if s > 0 {
		parts = append(parts, fmt.Sprintf("%ds", s))
	}
	if cp > 0 || len(parts) == 0 {
		parts = append(parts, fmt.Sprintf("%dc", cp))
	}
	return strings.Join(parts, " ")
}

// SkillNeed says an item needs a weapon skill the character can learn but has not.
type SkillNeed struct {
	Skill    string
	SkillID  int64
	Master   string // the nearest master; "" when the dataset names none
	Where    string // "Orgrimmar, Valley of Honor"
	City     string
	Cost     int64
	MinLevel int
}

// Line is "Needs the Staves skill: train it at Hanashi in Orgrimmar, Valley of Honor (10s)".
func (n SkillNeed) Line() string {
	s := "Needs the " + n.Skill + " skill"
	if n.Master == "" {
		return s + ": train it at a Weapon Master"
	}
	s += ": train it at " + n.Master + " in " + n.City + " (" + CostText(n.Cost) + ")"
	return s
}

// ItemSkill checks a weapon of the dataset's item row (class and subclass) against the character's
// weapon skills. usable is false when the dataset says the class can never learn the skill (the item
// is never suggested); need is set when the class can learn it and the character has not. Without the
// weapon-skill tables, or for anything but a weapon, it says usable with no need.
func (f WeaponSkillFacts) ItemSkill(c character.Character, classID, subclass int) (usable bool, need *SkillNeed) {
	if !f.Have || classID != classWeapon {
		return true, nil
	}
	id := SkillOfItem(subclass)
	name, listed := f.Names[id]
	if id == 0 || !listed {
		return true, nil
	}
	if !f.Learnable[id] {
		return false, nil
	}
	if known, captured := f.Known(c, id); known || !captured {
		return true, nil
	}
	n := &SkillNeed{Skill: name, SkillID: id, MinLevel: f.MinLevel[id]}
	if ms := f.MastersFor(id); len(ms) > 0 {
		m := ms[0]
		n.Master, n.Where, n.City, n.Cost = m.Master.Name, m.Master.Where(), m.Master.City, m.Cost
		if m.MinLevel > n.MinLevel {
			n.MinLevel = m.MinLevel
		}
	}
	return true, n
}

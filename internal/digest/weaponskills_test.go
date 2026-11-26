package digest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/gear"
	"github.com/WoW-Companion/advisor/internal/store"
)

type wsSkillRow struct {
	ClassID   int32  `parquet:"class_id"`
	ClassName string `parquet:"class_name"`
	SkillID   int32  `parquet:"skill_line_id"`
	SkillName string `parquet:"skill_name"`
	Learnable bool   `parquet:"learnable"`
	Default   bool   `parquet:"default_known"`
	MinLevel  int16  `parquet:"min_level"`
}

type wsMasterRow struct {
	NPC      int32   `parquet:"npc_id"`
	Name     string  `parquet:"name"`
	Faction  string  `parquet:"faction"`
	City     string  `parquet:"city"`
	Place    string  `parquet:"place"`
	Zone     int32   `parquet:"zone_id"`
	X        float64 `parquet:"x"`
	Y        float64 `parquet:"y"`
	Skills   []int32 `parquet:"skills,list"`
	Cost     []int32 `parquet:"cost_copper,list"`
	MinLevel []int16 `parquet:"min_level,list"`
	Source   string  `parquet:"source"`
}

const (
	itStaff, itPolearm, itAxe                     = 301, 302, 303
	skStaves, skDaggers, skAxes, skPolearms int32 = 136, 173, 44, 229
)

// weaponSkillStore holds three weapons and, when tables is set, the weapon-skill tables of a priest,
// a rogue and a warrior, with Horde and Alliance masters.
func weaponSkillStore(t *testing.T, tables bool) store.DataStore {
	w := func(id int64, name string, sub, slot int32) upItem {
		return upItem{ID: id, Name: name, ItemLevel: 24, RequiredLevel: 20, Quality: 3, ClassID: 2, SubclassID: sub, Slot: slot, ClassMask: -1, StatsStatus: "none"}
	}
	src := store.ReaderSource{"items": upTbl(t, []upItem{w(itStaff, "Gnarled Quarterstaff", 10, 17), w(itPolearm, "Spiked Halberd", 6, 17), w(itAxe, "Keen Hatchet", 0, 13)})}
	if !tables {
		return store.New(src)
	}
	var skills []wsSkillRow
	for _, c := range []struct {
		id    int32
		name  string
		learn map[int32]bool
	}{
		{5, "Priest", map[int32]bool{skStaves: true, skDaggers: true}},
		{4, "Rogue", map[int32]bool{skAxes: true, skDaggers: true}},
	} {
		for id, name := range map[int32]string{skStaves: "Staves", skDaggers: "Daggers", skAxes: "One-Handed Axes", skPolearms: "Polearms"} {
			skills = append(skills, wsSkillRow{c.id, c.name, id, name, c.learn[id], false, 1})
		}
	}
	src["weapon_skills"] = upTbl(t, skills)
	m := func(id int32, name, fac, city, place string, ids ...int32) wsMasterRow {
		r := wsMasterRow{NPC: id, Name: name, Faction: fac, City: city, Place: place, X: 50, Y: 60, Source: "test"}
		for _, s := range ids {
			r.Skills, r.Cost, r.MinLevel = append(r.Skills, s), append(r.Cost, 1000), append(r.MinLevel, 1)
		}
		return r
	}
	src["weapon_masters"] = upTbl(t, []wsMasterRow{
		m(1, "Hanashi", "Horde", "Orgrimmar", "Valley of Honor", skStaves, skAxes),
		m(2, "Sayoc", "Horde", "Orgrimmar", "Valley of Honor", skStaves, skDaggers, skAxes),
		m(3, "Ansekhwa", "Horde", "Thunder Bluff", "Lower Rise", skStaves),
		m(4, "Archibald", "Horde", "Undercity", "War Quarter", skDaggers, skPolearms),
		m(5, "Buzzek", "Alliance", "Ironforge", "", skAxes, skDaggers),
	})
	return store.New(src)
}

func weaponSkillChar(class, faction string, level int, knows []string, bags ...int) character.Character {
	c := character.Character{Name: "Testchar", Class: class, Faction: faction, Level: ip(level), SkillsCaptured: true,
		Location: &character.Location{Zone: "The Barrens", X: fp(50), Y: fp(50)}}
	for _, n := range knows {
		c.Skills = append(c.Skills, character.Skill{Name: n, Group: "Weapon Skills", Rank: ip(1), Max: ip(120)})
	}
	for _, id := range bags {
		c.Bags = append(c.Bags, character.ItemStack{ItemID: id, Count: 1})
	}
	return c
}

func upgradeTexts(d Digest) string {
	var b []string
	for _, s := range d.Sections {
		for _, l := range s.Lines {
			b = append(b, s.ID+": "+l.Text+" | "+l.How+" | "+strings.Join(l.Details, " / "))
		}
	}
	return strings.Join(b, "\n")
}

func TestWeaponSkillAdvice(t *testing.T) {
	cases := []struct {
		name       string
		tables     bool
		c          character.Character
		want, deny []string
	}{
		{"priest with a staff and no Staves skill", true, weaponSkillChar("PRIEST", "Horde", 24, []string{"Daggers"}, itStaff),
			[]string{"Gnarled Quarterstaff", "Needs the Staves skill: train it at Hanashi in Orgrimmar (10s)", "Train Staves at Hanashi in Orgrimmar (10s)", "One flight from the Crossroads"}, nil},
		{"priest with Staves known", true, weaponSkillChar("PRIEST", "Horde", 24, []string{"Staves"}, itStaff),
			[]string{"Gnarled Quarterstaff"}, []string{"Needs the", "Train Staves"}},
		{"rogue with a one-handed axe", true, weaponSkillChar("ROGUE", "Horde", 24, []string{"Daggers"}, itAxe),
			[]string{"Keen Hatchet", "Needs the One-Handed Axes skill: train it at Hanashi in Orgrimmar (10s)"}, nil},
		{"alliance rogue with a one-handed axe", true, weaponSkillChar("ROGUE", "Alliance", 24, []string{"Daggers"}, itAxe),
			[]string{"train it at Buzzek in Ironforge (10s)"}, []string{"Orgrimmar"}},
		{"priest with a polearm", true, weaponSkillChar("PRIEST", "Horde", 24, []string{"Daggers"}, itPolearm),
			nil, []string{"Spiked Halberd"}},
		{"old dataset without the tables", false, weaponSkillChar("PRIEST", "Horde", 24, []string{"Daggers"}, itStaff),
			nil, []string{"Needs the", "Train Staves"}},
	}
	for _, tc := range cases {
		got := upgradeTexts(Build(tc.c, weaponSkillStore(t, tc.tables), synthOpt))
		for _, w := range tc.want {
			if !strings.Contains(got, w) {
				t.Errorf("%s: lacks %q in\n%s", tc.name, w, got)
			}
		}
		for _, w := range tc.deny {
			if strings.Contains(got, w) {
				t.Errorf("%s: has %q in\n%s", tc.name, w, got)
			}
		}
	}
}

func TestWeaponTrainingStepComesAfterReadyTurnIns(t *testing.T) {
	c := weaponSkillChar("PRIEST", "Horde", 24, []string{"Daggers"}, itStaff)
	lines := withWeaponTraining([]Line{{Kind: KindTurnIn, Text: "a"}, {Kind: KindPickup, Text: "b"}}, []Line{{Kind: KindTraining, Text: "Train Staves"}})
	if lines[0].Text != "a" || lines[1].Text != "Train Staves" || lines[2].Text != "b" {
		t.Errorf("order: %+v", lines)
	}
	// Out of reach (a zone with no flight to a master's city, and not in one): no step, the advice stays.
	c.Location.Zone = "Stonetalon Mountains"
	got := upgradeTexts(Build(c, weaponSkillStore(t, true), synthOpt))
	if strings.Contains(got, "Train Staves") || !strings.Contains(got, "Needs the Staves skill") {
		t.Errorf("out of reach:\n%s", got)
	}
}

func TestWeaponSkillItemRules(t *testing.T) {
	f := gear.LoadWeaponSkills(weaponSkillChar("PRIEST", "Horde", 24, nil), weaponSkillStore(t, true))
	if !f.Have || len(f.MastersFor(int64(skStaves))) != 3 {
		t.Fatalf("facts: %+v", f)
	}
	if got := f.MastersFor(int64(skStaves)); got[0].Master.City == "" {
		t.Errorf("masters: %+v", got)
	}
	if gear.LoadWeaponSkills(weaponSkillChar("PRIEST", "Horde", 24, nil), weaponSkillStore(t, false)).Have {
		t.Error("an old dataset claims weapon-skill facts")
	}
}

// TestWeaponSkillGolden pins the digest of a priest with a staff in the bags and no Staves skill.
func TestWeaponSkillGolden(t *testing.T) {
	got := one(Build(weaponSkillChar("PRIEST", "Horde", 24, []string{"Daggers"}, itStaff), weaponSkillStore(t, true), synthOpt)).Lua()
	path := filepath.Join("..", "..", "testdata", "digest", "priest_weapon.lua")
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
}

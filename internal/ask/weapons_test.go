package ask

import (
	"bytes"
	"strings"
	"testing"

	"github.com/parquet-go/parquet-go"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/store"
)

type skillRow struct {
	ClassID   int32  `parquet:"class_id"`
	SkillID   int32  `parquet:"skill_line_id"`
	SkillName string `parquet:"skill_name"`
	Learnable bool   `parquet:"learnable"`
	MinLevel  int16  `parquet:"min_level"`
}

type masterRow struct {
	NPC      int32   `parquet:"npc_id"`
	Name     string  `parquet:"name"`
	Faction  string  `parquet:"faction"`
	City     string  `parquet:"city"`
	Place    string  `parquet:"place"`
	X        float64 `parquet:"x"`
	Y        float64 `parquet:"y"`
	Skills   []int32 `parquet:"skills,list"`
	Cost     []int32 `parquet:"cost_copper,list"`
	MinLevel []int16 `parquet:"min_level,list"`
}

func parquetOf[T any](t *testing.T, rows []T) store.SizedReaderAt {
	t.Helper()
	var buf bytes.Buffer
	if err := parquet.Write(&buf, rows); err != nil {
		t.Fatal(err)
	}
	return store.SizedReaderAt{ReaderAt: bytes.NewReader(buf.Bytes()), Size: int64(buf.Len())}
}

func weaponDS(t *testing.T) store.DataStore {
	m := func(id int32, name, city, place string, skills ...int32) masterRow {
		r := masterRow{NPC: id, Name: name, Faction: "Horde", City: city, Place: place, X: 50, Y: 50}
		for _, s := range skills {
			r.Skills, r.Cost, r.MinLevel = append(r.Skills, s), append(r.Cost, 1000), append(r.MinLevel, 1)
		}
		return r
	}
	return store.New(store.ReaderSource{
		"weapon_skills": parquetOf(t, []skillRow{
			{5, 136, "Staves", true, 1}, {5, 173, "Daggers", true, 1}, {5, 229, "Polearms", false, 20},
			{4, 136, "Staves", false, 1}, {4, 173, "Daggers", true, 1}, {4, 229, "Polearms", false, 20},
		}),
		"weapon_masters": parquetOf(t, []masterRow{
			m(1, "Hanashi", "Orgrimmar", "Valley of Honor", 136),
			m(2, "Sayoc", "Orgrimmar", "Valley of Honor", 136, 173),
			m(3, "Ansekhwa", "Thunder Bluff", "Lower Rise", 136),
			m(4, "Archibald", "Undercity", "War Quarter", 173, 229),
		}),
	})
}

func weaponPriest() character.Character {
	one := 1
	return character.Character{Class: "PRIEST", Faction: "Horde", Level: &one, SkillsCaptured: true,
		Skills: []character.Skill{{Name: "Daggers", Group: "Weapon Skills", Rank: &one, Max: &one}}}
}

func TestWeaponFactsAnswerTheStaffQuestion(t *testing.T) {
	q := "Where can I train in 2 handed staffs?"
	if !WeaponQuestion(q) {
		t.Fatal("not a weapon question")
	}
	got := weaponFacts(weaponPriest(), weaponDS(t), q)
	for _, want := range []string{
		"The character has learned: Daggers 1/1",
		"The class can learn: Daggers, Staves",
		"Staves: Hanashi or Sayoc in Orgrimmar, Valley of Honor for 10s; Ansekhwa in Thunder Bluff, Lower Rise for 10s",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("facts lack %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Archibald") || strings.Contains(got, "Polearms") {
		t.Errorf("facts name a master that does not teach it, or a skill the class cannot learn:\n%s", got)
	}
	// Not a weapon question: the masters stay out of the facts.
	if got := weaponFacts(weaponPriest(), weaponDS(t), "What next?"); strings.Contains(got, "Hanashi") || !strings.Contains(got, "Learnable but not learned yet: Staves") {
		t.Errorf("short form: %s", got)
	}
}

func TestWeaponPromptSaysFactsNotMemory(t *testing.T) {
	p, g := Config{Command: Presets["claude"]}.PromptGround(Question{Text: "Where can I train in 2 handed staffs?"})
	if !strings.Contains(p, "never from your general knowledge") || !strings.Contains(p, "Forever differs from Classic") {
		t.Errorf("prompt lacks the weapon rule: %s", p)
	}
	if g.KnowHow || g.Footer() != "" {
		t.Errorf("a weapon answer carries a general-knowledge footer: %q", g.Footer())
	}
}

func TestWeaponFactsWithoutTablesSaySo(t *testing.T) {
	old := store.New(store.ReaderSource{})
	if got := weaponFacts(weaponPriest(), old, "Can priests use staves?"); got != NoWeaponFacts || !strings.Contains(got, "don't have those facts") {
		t.Errorf("old dataset: %q", got)
	}
	if got := weaponFacts(weaponPriest(), old, "What next?"); got != "" {
		t.Errorf("an old dataset adds weapon text to every question: %q", got)
	}
}

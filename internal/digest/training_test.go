package digest

import (
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/lua"
	"github.com/WoW-Companion/advisor/internal/store"
)

type spellRow struct {
	ClassID    int32  `parquet:"class_id"`
	SpellID    int32  `parquet:"spell_id"`
	Name       string `parquet:"name"`
	Rank       *int32 `parquet:"rank,optional"`
	Level      int32  `parquet:"level"`
	Superseded *int32 `parquet:"superseded_spell_id,optional"`
}

// warriorStore holds a synthetic warrior ability table; the priest row must never show for a warrior.
func warriorStore(t *testing.T) store.DataStore {
	r2, r1, s1 := i32p(2), i32p(1), i32p(100)
	return store.New(store.ReaderSource{"class_spells": tbl(t, []spellRow{
		{1, 284, "Heroic Strike", r2, 8, s1},
		{1, 1715, "Hamstring", r1, 8, nil},
		{1, 6546, "Rend", r2, 10, i32p(772)},
		{1, 355, "Taunt", nil, 10, nil},
		{1, 72, "Shield Bash", r1, 12, nil},
		{5, 585, "Smite", r2, 10, nil},
	})})
}

func warrior(level int) character.Character {
	return character.Character{
		Name: "Testwarrior", Class: "WARRIOR", Race: "Skyborn", Level: ip(level),
		Profile: &character.Profile{Role: "damage", Focus: "leveling", Spec: "Arms"},
	}
}

func trainingOf(d Digest) (Line, bool) {
	for _, s := range d.Sections {
		for _, l := range s.Lines {
			if l.Kind == KindTraining {
				return l, true
			}
		}
	}
	return Line{}, false
}

func TestTrainingEntryWarriorEightToTen(t *testing.T) {
	ds := warriorStore(t)
	opt := synthOpt
	opt.PrevLevel = 8
	l, ok := trainingOf(Build(warrior(10), ds, opt))
	if !ok || l.Level != 10 {
		t.Fatalf("no training entry: %+v", l)
	}
	var got []string
	for _, s := range l.Spells {
		got = append(got, spellLabel(s)+map[bool]string{true: " new", false: " up"}[s.New])
	}
	if want := "Rend (Rank 2) up,Taunt new"; strings.Join(got, ",") != want {
		t.Errorf("spells = %v, want %s", got, want)
	}
	// Level 8 itself is before the window; level 12 is after it.
	if strings.Contains(l.Text, "Hamstring") || strings.Contains(l.Text, "Shield Bash") {
		t.Errorf("text = %q", l.Text)
	}
	// Level unknown: only the character's own level.
	l, _ = trainingOf(Build(warrior(8), ds, synthOpt))
	if len(l.Spells) != 2 || !strings.Contains(strings.Join(l.Details, "|"), "Available at your level") {
		t.Errorf("unknown prev: %+v", l)
	}
	if _, ok := trainingOf(Build(warrior(9), ds, synthOpt)); ok {
		t.Error("level 9 unlocks nothing but an entry was made")
	}
}

func TestTrainingNeverInventsTrainer(t *testing.T) {
	l, _ := trainingOf(Build(warrior(10), warriorStore(t), synthOpt))
	if l.NPC != "" || l.Zone != "" || l.HasXY || strings.Contains(strings.Join(l.Details, " "), "Trainer") {
		t.Errorf("trainer invented: %+v", l)
	}
	// A captured NPC that is not a warrior trainer is not one.
	c := warrior(10)
	c.NPCs = []character.NPC{{Name: "Innkeeper Bell", Zone: "Dunemark"}, {Name: "Mage Trainer Ola", Zone: "Dunemark"}}
	if l, _ = trainingOf(Build(c, warriorStore(t), synthOpt)); l.NPC != "" {
		t.Errorf("trainer invented from %+v", c.NPCs)
	}
	// A captured warrior trainer is named, in the character's zone first, with its position.
	x, y := 42.46, 31.0
	c.Location = &character.Location{Zone: "Dunemark"}
	c.NPCs = []character.NPC{{Name: "Old Warrior Trainer", Zone: "Far Peaks"}, {Name: "Karg Warrior Trainer", Zone: "Dunemark", X: &x, Y: &y}}
	l, _ = trainingOf(Build(c, warriorStore(t), synthOpt))
	if l.NPC != "Karg Warrior Trainer" || l.Zone != "Dunemark" || !l.HasXY {
		t.Errorf("trainer = %+v", l)
	}
}

// Every next_quests, upgrades, dungeons and talents entry carries details (1-4 lines).
func TestDetailsForEveryKind(t *testing.T) {
	c := synthChar()
	c.Profile = &character.Profile{Role: "healer", Focus: "dungeons", Spec: "Discipline"}
	d := Build(c, synthStore(t), synthOpt)
	kinds := map[string]bool{}
	check := func(what string, details []string) {
		kinds[what] = true
		if len(details) < 1 || len(details) > MaxDetails {
			t.Errorf("%s: %d details", what, len(details))
		}
		for _, x := range details {
			if x == "" || utf8.RuneCountInString(x) > maxDetailLength {
				t.Errorf("%s: detail %q", what, x)
			}
		}
	}
	for _, s := range d.Sections {
		for _, l := range s.Lines {
			switch {
			case s.ID == NextQuests && l.Kind != KindNote, s.ID == Upgrades:
				k := s.ID + "/" + l.Kind
				check(k, l.Details)
			}
		}
	}
	for _, l := range d.Dungeons {
		check("dungeons", l.Details)
	}
	for _, l := range d.Talents {
		check("talents", l.Details)
	}
	for _, k := range []string{"next_quests/turnin", "next_quests/progress", "next_quests/talent", "upgrades/", "dungeons", "talents"} {
		if !kinds[k] {
			t.Errorf("no %s entry checked; got %v", k, kinds)
		}
	}
	// The file passes the slot check and parses, with details a list.
	out := one(d).Lua()
	if err := SlotCheck(out); err != nil {
		t.Error(err)
	}
	if _, err := lua.Parse(out); err != nil {
		t.Error(err)
	}
}

func TestTrainingLuaFields(t *testing.T) {
	opt := synthOpt
	opt.PrevLevel = 8
	d := Build(warrior(10), warriorStore(t), opt)
	out := one(d).Lua()
	for _, want := range []string{`schema = "1.9"`, `asks = { "Which new ability should I train first?"`, `ref = "training:10"`, `kind = "training"`, `level = 10`, `spells = { { id = 6546, name = "Rend", rank = 2, new = false }, { id = 355, name = "Taunt", new = true } }`, `details = {`} {
		if !strings.Contains(out, want) {
			t.Errorf("lacks %s:\n%s", want, out)
		}
	}
	if err := SlotCheck(out); err != nil {
		t.Error(err)
	}
	if _, err := lua.Parse(out); err != nil {
		t.Error(err)
	}
	t.Log("\n" + out)
}

func TestTurnInDetailsListWhatItUnlocks(t *testing.T) {
	c := character.Character{Name: "Hero"}
	l := Line{Kind: KindTurnIn, Title: "Quest 30", NPC: "Elder Test", Level: 9, Unlocks: []string{"Quest 31 (from Elder Test, here)"}}
	d := questContext(c, l, "ready")
	if !slices.Contains(d, "Unlocks: Quest 31 (from Elder Test, here)") {
		t.Fatalf("details %q", d)
	}
	l.Kind = KindPickup
	if d := questContext(c, l, "ready"); slices.ContainsFunc(d, func(s string) bool { return strings.HasPrefix(s, "Unlocks:") }) {
		t.Fatalf("a pick-up says what it unlocks: %q", d)
	}
}

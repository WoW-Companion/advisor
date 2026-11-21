package character

import (
	"errors"
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/WoW-Companion/advisor/internal/lua"
)

func fixture(t *testing.T, name string) File {
	t.Helper()
	f, err := LoadFile(filepath.Join("..", "..", "testdata", "foreverscribe", name))
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func deref(p *int) int {
	if p == nil {
		return -1
	}
	return *p
}

func TestFullCapture(t *testing.T) {
	f := fixture(t, "level30_caster.lua")
	if f.Addon != "0.10.1" || len(f.Characters) != 1 {
		t.Fatalf("file = %+v", f)
	}
	c := f.Characters[0]
	if c.Key != "Testrealm:Testmage" || c.Name != "Testmage" || deref(c.Level) != 30 || c.Class != "MAGE" {
		t.Errorf("identity = %+v", c)
	}
	if deref(c.RestedXP) != 2800 || *c.CapturedAt != 1700100000 {
		t.Errorf("progress = %v %v", c.RestedXP, c.CapturedAt)
	}
	head := c.Equipment["head"]
	if head.Name != "Test Hood of Wisdom" || deref(head.ItemLevel) != 28 || deref(head.Enchant) != 2999 {
		t.Errorf("head = %+v", head)
	}
	if len(c.Bags) != 2 || !c.BankCaptured || len(c.Bank) != 1 || c.Bank[0].Count != 200 {
		t.Errorf("storage: bags %v bank %v captured %v", c.Bags, c.Bank, c.BankCaptured)
	}
	if len(c.Quests) != 2 || len(c.Quests[0].Objectives) != 1 || len(c.CompletedQuests) != 3 {
		t.Errorf("quests = %+v", c.Quests)
	}
	if len(c.Reputations) != 1 || c.Reputations[0].Name != "Test Stormwind" {
		t.Errorf("reputation = %+v", c.Reputations)
	}
	if !f.Client.Empty() || !c.Client.Empty() {
		t.Errorf("a capture without a client block has none: %+v", c.Client)
	}
}

// Bugs 1 to 6 of a real capture, each against the tree_priest fixture.
func TestRealCaptureShapes(t *testing.T) {
	c := fixture(t, "tree_priest.lua").Characters[0]

	// 1. Talents use the tree format: groups with spent points, a currency with unspent points.
	tr := c.Talents.Trees[0]
	if len(tr.Groups) != 3 || tr.Groups[1].Name != "Test Discipline" || tr.Groups[1].Spent != 14 {
		t.Errorf("groups = %+v", tr.Groups)
	}
	if deref(tr.Unspent) != 1 || deref(tr.MaxPoints) != 15 || deref(tr.Points) != 14 || deref(c.Talents.Unspent) != 1 {
		t.Errorf("currency = %+v unspent %v", tr, c.Talents.Unspent)
	}
	if tr.Talents[0].Group != "Test Discipline" || tr.Talents[0].Rank != 5 {
		t.Errorf("talent = %+v", tr.Talents[0])
	}

	// 2. Each profession once, however many lists name it.
	if len(c.Professions) != 2 || c.Professions[0].Name != "Tailoring" || c.Professions[1].Name != "Cooking" {
		t.Errorf("professions = %+v", c.Professions)
	}
	if len(c.Skills) != 4 {
		t.Errorf("skills stay complete: %d", len(c.Skills))
	}

	// 3. The bank was never opened: absent, not empty.
	if c.BankCaptured || c.Bank != nil || !c.BagsCaptured {
		t.Errorf("bank captured = %v", c.BankCaptured)
	}

	// 4. Rested is experience points.
	if deref(c.RestedXP) != 6 {
		t.Errorf("rested = %v", c.RestedXP)
	}

	// 5. The realm is empty: not invented, and the character is keyed by name.
	if c.Realm != "" || c.Key != "Testpriest" {
		t.Errorf("realm %q key %q", c.Realm, c.Key)
	}

	// 6. The client block.
	if c.Client != (Client{Version: "9.9.9", Build: "99999", TOC: "99999"}) {
		t.Errorf("client = %+v", c.Client)
	}
}

func TestProfessionsDedupe(t *testing.T) {
	id := func(n int) *int { return &n }
	listed := []Skill{{Name: "Mining", SkillID: id(186)}}
	skills := []Skill{
		{Name: "Mining", SkillID: id(186), Group: "Professions"},
		{Name: "Herbalism", Group: "Primary Skills"}, {Name: "herbalism", Group: "Professions"},
		{Name: "Swords", Group: "Weapon Skills"},
	}
	got := professions(listed, skills)
	if len(got) != 2 || got[0].Name != "Mining" || got[1].Name != "Herbalism" {
		t.Errorf("professions = %+v", got)
	}
}

func TestLoadErrors(t *testing.T) {
	if _, err := LoadFile(filepath.Join(t.TempDir(), "missing.lua")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("missing file err = %v", err)
	}
	if _, err := Load(map[string]any{"Other": lua.Table{}}); err == nil {
		t.Error("a file without ForeverScribeDB must fail")
	}
}

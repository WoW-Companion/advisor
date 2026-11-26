package brief

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/WoW-Companion/advisor/internal/character"
)

var (
	fresh = time.Unix(1700100000+60, 0)
	stale = time.Unix(1700100000+3*3600+5, 0)
)

func load(t *testing.T, name string) character.File {
	t.Helper()
	f, err := character.LoadFile(filepath.Join("..", "..", "testdata", "foreverscribe", name))
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func render(t *testing.T, fixture, who string, now time.Time) string {
	t.Helper()
	f := load(t, fixture)
	c, err := Choose(f, who)
	if err != nil {
		t.Fatal(err)
	}
	return Character(c, f.ClientOf(c), now)
}

func TestFormatMoney(t *testing.T) {
	n := func(i int) *int { return &i }
	for _, c := range []struct {
		in   *int
		want string
	}{{nil, "not captured"}, {n(0), "0g 0s 0c"}, {n(1234567), "123g 45s 67c"}, {n(99), "0g 0s 99c"}} {
		if got := FormatMoney(c.in); got != c.want {
			t.Errorf("FormatMoney = %q, want %q", got, c.want)
		}
	}
}

func TestBriefContent(t *testing.T) {
	cases := []struct {
		name, fixture, who string
		present, absent    []string
	}{
		{"full capture", "level30_caster.lua", "", []string{
			"Level: 30", "Class: MAGE", "Race: Gnome", "Realm: Testrealm",
			"XP 12345/56000, rested 2800 XP", "Money: 123g 45s 67c",
			"Spec: Test Class Tree (3 points)", "Talent points: Test Class Tree 3; 2 points unspent",
			"- head: Test Hood of Wisdom, ilvl 28", "- mainhand: Test Staff, ilvl 31", "empty slots: neck,",
			"- Tailoring: 150/225", "- Enchanting: 75/150",
			"- bank: 1 stack, 200 items", "- bags: 2 stacks, 25 items",
			"- Test Quest: Fetch (level 29, Testzone) [in progress]", "- Test Quest: Report (level 30, Testzone) [complete]",
			"Client: client not captured", "Source: ForeverScribe capture at 2023-11-16 02:00 UTC",
		}, []string{"Staves", "WARNING"}},
		{"real-capture shapes", "tree_priest.lua", "testpriest", []string{
			"# Testpriest", "Realm: not captured", "XP 1000/8000, rested 6 XP",
			"Spec: Test Discipline (14 points)",
			"Talent points: Test Discipline 14, Test Holy 0, Test Shadow Magic 0; 1 point unspent",
			"- Tailoring: 90/150", "- Cooking: 40/75",
			"- bank not captured yet: open your bank once in game",
			"Client: 9.9.9 (build 99999, toc 99999)",
		}, []string{"Wands", "- bank: "}},
		{"level 1", "level1.lua", "testchar", []string{
			"Spec: unknown (no talents captured)", "- chest: Test Shirt", "Professions:\n- not captured",
			"- none in the quest log", "Money: 0g 0s 0c", "rested XP not captured",
		}, nil},
		{"sparse", "two_characters.lua", "Testbeta", []string{
			"Level: 8", "XP not captured, rested XP not captured", "Equipped gear:\n- not captured",
			"Current quests:\n- not captured", "- bags: not captured",
		}, nil},
	}
	for _, c := range cases {
		text := render(t, c.fixture, c.who, fresh)
		for _, s := range c.present {
			if !strings.Contains(text, s) {
				t.Errorf("%s: missing %q in:\n%s", c.name, s, text)
			}
		}
		for _, s := range c.absent {
			if strings.Contains(text, s) {
				t.Errorf("%s: unexpected %q", c.name, s)
			}
		}
	}
}

func TestStaleCaptureWarns(t *testing.T) {
	if !strings.Contains(render(t, "level30_caster.lua", "", stale), "WARNING: this capture is 3h old; /reload in game to refresh.") {
		t.Error("a 3h old capture must warn")
	}
}

func TestSpec(t *testing.T) {
	n := func(i int) *int { return &i }
	tree := func(gs ...character.TalentGroup) *character.Talents {
		return &character.Talents{Trees: []character.TalentTree{{Groups: gs}}}
	}
	cases := []struct {
		name string
		in   *character.Talents
		want string
	}{
		{"none", nil, "unknown (no talents captured)"},
		{"zero points", tree(character.TalentGroup{Name: "A"}), "unknown (no talent points captured)"},
		{"tie", tree(character.TalentGroup{Name: "A", Spent: 2}, character.TalentGroup{Name: "B", Spent: 2}), "unknown (A, B tied at 2 points)"},
		{"top group", tree(character.TalentGroup{Name: "A", Spent: 1}, character.TalentGroup{Name: "B", Spent: 5}), "B (5 points)"},
		{"classic tab", &character.Talents{Trees: []character.TalentTree{{Name: "Fire", Points: n(7)}}}, "Fire (7 points)"},
	}
	for _, c := range cases {
		if got, _ := spec(c.in); got != c.want {
			t.Errorf("%s: spec = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestChooseAndList(t *testing.T) {
	f := load(t, "two_characters.lua")
	rows := List(f)
	if len(rows) != 2 || rows[0].Name != "Testbeta" || rows[1].Realm != "Testrealm" || *rows[0].CapturedAt != "2023-11-18 09:33 UTC" {
		t.Errorf("List = %+v", rows)
	}
	for _, c := range []struct{ who, msg string }{{"", "name one"}, {"Nobody", "No captured character 'Nobody'"}} {
		if _, err := Choose(f, c.who); err == nil || !strings.Contains(err.Error(), c.msg) {
			t.Errorf("Choose(%q) err = %v", c.who, err)
		}
	}
	if c, err := Choose(load(t, "tree_priest.lua"), "Testpriest"); err != nil || c.Key != "Testpriest" {
		t.Errorf("a character with no realm is chosen by name: %v", err)
	}
}

func TestCharacterBriefSaysWhereTheHearthstoneIsBound(t *testing.T) {
	c := character.Character{Name: "Bound", Hearth: " Razor Hill "}
	if got := Character(c, character.Client{}, fresh); !strings.Contains(got, "\nHearthstone bound to: Razor Hill\n") {
		t.Errorf("brief names no bind:\n%s", got)
	}
	c.Hearth = ""
	if got := Character(c, character.Client{}, fresh); strings.Contains(got, "Hearthstone") {
		t.Errorf("a bind was guessed:\n%s", got)
	}
}

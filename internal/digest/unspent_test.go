package digest

import (
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
)

// firstNext is the first line of the NEXT section.
func firstNext(t *testing.T, c character.Character) Line {
	t.Helper()
	d := Build(c, synthStore(t), synthOpt)
	for _, s := range d.Sections {
		if s.ID == NextQuests && len(s.Lines) > 0 {
			return s.Lines[0]
		}
	}
	t.Fatal("no NEXT section")
	return Line{}
}

func TestUnspentTalentPointsLead(t *testing.T) {
	spentOne := character.TalentTree{Name: "Discipline", Groups: []character.TalentGroup{{Name: "Discipline", Spent: 5}}, Talents: []character.Talent{{Name: "Penance", Rank: 1, MaxRank: ip(1)}}}
	cases := []struct {
		name    string
		talents *character.Talents
		want    string // prefix of the first line; "" means no talent line leads
	}{
		{"unspent with nothing spent", &character.Talents{Unspent: ip(1), Trees: []character.TalentTree{{Name: "Discipline", Groups: []character.TalentGroup{{Name: "Discipline", Spent: 5}}, Talents: []character.Talent{{Name: "Penance", MaxRank: ip(1)}}}}}, "Spend 1 talent point: Penance (Discipline)"},
		{"spent talents are skipped", &character.Talents{Unspent: ip(1), Trees: []character.TalentTree{spentOne}}, "Spend 1 talent point: Power Word: Shield (Discipline)"},
		{"empty talents hedge", &character.Talents{Unspent: ip(1)}, "Spend 1 talent point. Open your talents (N); I can't see what you've already picked yet"},
		{"zero unspent", &character.Talents{Unspent: ip(0), Trees: []character.TalentTree{spentOne}}, ""},
		{"absent", nil, ""},
	}
	for _, tc := range cases {
		c := synthChar()
		c.Talents = tc.talents
		l := firstNext(t, c)
		if tc.want == "" {
			if l.Kind == KindTalent {
				t.Errorf("%s: talent line leads: %q", tc.name, l.Text)
			}
			continue
		}
		if l.Kind != KindTalent || !strings.HasPrefix(l.Text, tc.want) {
			t.Errorf("%s: first line %q (%s), want %q", tc.name, l.Text, l.Kind, tc.want)
		}
		if l.Points != 1 {
			t.Errorf("%s: badge points = %d", tc.name, l.Points)
		}
		if strings.Contains(tc.name, "hedge") {
			if strings.Contains(l.Why, "Penance") || strings.Contains(strings.Join(l.Details, "|"), "Penance") {
				t.Errorf("hedged line names a pick: %+v", l)
			}
			if d := Build(c, synthStore(t), synthOpt); len(d.Talents) != 0 {
				t.Errorf("hedged panel still lists rank-1 talents: %+v", d.Talents)
			}
		}
	}
}

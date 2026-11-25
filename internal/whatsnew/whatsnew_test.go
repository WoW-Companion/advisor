package whatsnew

import (
	"regexp"
	"strings"
	"testing"

	advisor "github.com/WoW-Companion/advisor"
)

func TestParseCapsVersionsAndLines(t *testing.T) {
	var b strings.Builder
	b.WriteString("# Changelog\n\n")
	for v := 9; v >= 1; v-- {
		b.WriteString("## 0.1." + string(rune('0'+v)) + "\n\n")
		for i := 0; i < 6; i++ {
			b.WriteString("- Line `x` one. Second sentence here.\n")
		}
	}
	got := Parse(b.String())
	if len(got) != Versions || got[0].Version != "0.1.9" {
		t.Fatalf("releases = %+v", got)
	}
	n := len(got)
	for _, r := range got {
		n += len(r.Lines)
		for _, l := range r.Lines {
			if l != "Line x one." {
				t.Errorf("line %q is not the plain first sentence", l)
			}
		}
	}
	if n > MaxLines {
		t.Errorf("%d lines, max %d", n, MaxLines)
	}
}

// The bundled changelog parses, newest first.
func TestBundledHasRecentVersions(t *testing.T) {
	got := Bundled()
	if len(got) == 0 || len(got[0].Lines) == 0 {
		t.Fatalf("no bundled changelog: %+v", got)
	}
}

func TestSectionIsOnlyThatVersion(t *testing.T) {
	md := "# Changelog\n\n## 0.2.0\n\n- New `thing`. More.\n- Second.\n\n## 0.1.0\n\n- Old.\n"
	if got := Section(md, "v0.2.0"); got != "- New thing. More.\n- Second." {
		t.Errorf("section = %q", got)
	}
	if got := Section(md, "0.1.0"); got != "- Old." {
		t.Errorf("last section = %q", got)
	}
	if got := Section(md, "0.3.0"); got != "" {
		t.Errorf("missing version = %q", got)
	}
}

// Nothing a player is shown as notes carries a mission id.
func TestShownNotesHaveNoMissionIds(t *testing.T) {
	ids := regexp.MustCompile(`\b(ADV|ADDON|DATA|SITE|HARN)-\d+\b`)
	for _, r := range Bundled() {
		for _, l := range append(r.Lines, BundledSection(r.Version)) {
			if ids.MatchString(l) {
				t.Errorf("%s shows %q", r.Version, l)
			}
		}
	}
	if ids.MatchString(advisor.Changelog) {
		t.Error("CHANGELOG.md carries a mission id")
	}
}

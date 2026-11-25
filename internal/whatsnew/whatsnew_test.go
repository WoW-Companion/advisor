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

const rangeLog = `# Changelog

## 0.7.58

- Fifty-eight a.
- Fifty-eight b.

## 0.7.57

- Fifty-seven.

## 0.7.56

- Fifty-six **a**.

## 0.7.55

- Running.
`

func TestRangeIncludesSkippedNewestFirstAndExcludesPulledAndRunning(t *testing.T) {
	got := Range(rangeLog, "0.7.55", "0.7.58", []string{"v0.7.57"})
	want := "## 0.7.58\n- Fifty-eight a.\n- Fifty-eight b.\n## 0.7.56\n- Fifty-six a."
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
	if got := Range(rangeLog, "0.7.55", "0.7.57", nil); got != "## 0.7.57\n- Fifty-seven.\n## 0.7.56\n- Fifty-six a." {
		t.Errorf("upper bound is inclusive, newer excluded: %q", got)
	}
	if got := Range(rangeLog, "0.7.58", "0.7.58", nil); got != "" {
		t.Errorf("nothing newer than the running version: %q", got)
	}
}

func TestRangeCapsVersionsAndBullets(t *testing.T) {
	var b strings.Builder
	for v := 9; v >= 1; v-- {
		b.WriteString("## 0.1." + string(rune('0'+v)) + "\n\n- one\n- two\n- three\n\n")
	}
	got := Range(b.String(), "0.1.0", "0.1.9", nil)
	if n := strings.Count(got, "## "); n != RangeVersions {
		t.Errorf("%d versions, want %d:\n%s", n, RangeVersions, got)
	}
	if n := strings.Count(got, "\n- ") + 1; n != RangeBullets {
		t.Errorf("%d bullets, want %d:\n%s", n, RangeBullets, got)
	}
	if !strings.HasSuffix(got, "…and 19 more changes") || !strings.HasPrefix(got, "## 0.1.9\n") {
		t.Errorf("tail or head wrong:\n%s", got)
	}
	if got := Range(rangeLog, "0.7.55", "0.7.58", nil); strings.Contains(got, "more changes") {
		t.Errorf("nothing hidden, no tail: %q", got)
	}
}

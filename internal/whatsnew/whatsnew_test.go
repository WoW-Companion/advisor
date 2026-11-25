package whatsnew

import (
	"os"
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
	if got := Section(md, "v0.2.0"); got != "- New thing.\n- More.\n- Second." {
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
	if n := strings.Count(got, "\n- "); n != RangeBullets {
		t.Errorf("%d bullets, want %d:\n%s", n, RangeBullets, got)
	}
	if !strings.HasSuffix(got, "…and 19 more changes") || !strings.HasPrefix(got, "## 0.1.9\n") {
		t.Errorf("tail or head wrong:\n%s", got)
	}
	if got := Range(rangeLog, "0.7.55", "0.7.58", nil); strings.Contains(got, "more changes") {
		t.Errorf("nothing hidden, no tail: %q", got)
	}
}

func TestSplitKeepsAllText(t *testing.T) {
	for _, s := range []string{
		"One thing. Second thing! Third? 4 more.",
		"Single sentence with no end",
		"Dr. smith stays. Next one.",
		"Josef in Undercity (level 20). Then \"Quoted\" bit.",
	} {
		got := Split(s)
		if strings.Join(got, " ") != s {
			t.Errorf("%q lost text: %q", s, got)
		}
	}
	if got := Split("A. B."); len(got) != 2 || got[0] != "A." {
		t.Errorf("split = %q", got)
	}
	if len(Split("  ")) != 0 {
		t.Error("blank gives bullets")
	}
}

func TestSectionAndRangeSplitOldParagraphs(t *testing.T) {
	md := "## 0.2.0\n\n- First change. Second change.\n"
	if got := Section(md, "0.2.0"); got != "- First change.\n- Second change." {
		t.Errorf("section = %q", got)
	}
	if got := Range(md, "0.1.0", "0.2.0", nil); got != "## 0.2.0\n- First change.\n- Second change." {
		t.Errorf("range = %q", got)
	}
}

func TestLintFlagsLongBulletsAndIds(t *testing.T) {
	long := strings.Repeat("word ", 25)
	cases := map[string]string{
		"over":  "## 0.9.0\n\n- " + long + "\n",
		"id":    "## 0.9.0\n\n- Fixed ADV-159 thing.\n",
		"file":  "## 0.9.0\n\n- Edited updates.json again.\n",
		"dev":   "## 0.9.0\n\n- The sidecar restarts.\n",
		"odds":  "## 0.9.0\n\n- Skill-ups now work 75% of the time.\n",
		"oldok": "## 0.1.0\n\n- " + long + " ADV-1\n",
		"clean": "## 0.9.0\n\n- Profession plans need fewer crafts.\n",
	}
	for name, md := range cases {
		got := Lint(md)
		if want := name != "oldok" && name != "clean"; (len(got) > 0) != want {
			t.Errorf("%s: problems = %v", name, got)
		}
	}
}

// Every note a player can see follows the writing rule, and the newest version is the one in VERSION.
func TestBundledNotesFollowTheWritingRule(t *testing.T) {
	if p := Lint(advisor.Changelog); len(p) > 0 {
		t.Errorf("release notes break the rule (see docs/releasing.md):\n%s", strings.Join(p, "\n"))
	}
	top := Bundled()[0].Version
	if v, _ := os.ReadFile("../../VERSION"); strings.TrimSpace(string(v)) != top {
		t.Errorf("newest CHANGELOG version is %s but VERSION is %q: a release is the next version, with its notes", top, strings.TrimSpace(string(v)))
	}
}

package whatsnew

import (
	"fmt"
	"regexp"
	"strings"
)

// MaxBulletLen is the longest a release-note bullet may be: one change, said briefly.
const MaxBulletLen = 100

// LintFrom is the first version whose notes follow the bullet rule; older entries are history.
const LintFrom = "0.7.58"

var (
	missionID = regexp.MustCompile(`\b[A-Z]{2,6}-\d+\b`)
	fileName  = regexp.MustCompile(`(?i)\b[\w./-]+\.(go|js|rs|md|json|toml|lua|yml|yaml|html|css|exe|sh)\b`)
	devWords  = regexp.MustCompile(`(?i)\b(sidecar|endpoint|regex|json|api|refactor|struct|goroutine|commit|pull request|golden|schema)\b`)
	odds      = regexp.MustCompile(`\d+\s*%|\b\d+\s*/\s*\d+\b|\b(one|two|three) (in|out of) (two|three|four|five|ten)\b`)
)

// Lint checks the notes of every version from LintFrom up: each bullet is one change of at most
// MaxBulletLen characters, with no mission id, file name, developer word or odds. It returns one
// problem per offending bullet, "<version>: <why>: <bullet>"; nil means the notes are good.
func Lint(md string) []string {
	lo, _ := semver(LintFrom)
	var probs []string
	ver, in := "", false
	for _, line := range strings.Split(strings.ReplaceAll(md, "\r", ""), "\n") {
		if m := heading.FindStringSubmatch(line); m != nil {
			n, ok := semver(m[1])
			ver, in = m[1], ok && !less(n, lo)
			continue
		}
		if !in || !strings.HasPrefix(line, "- ") {
			continue
		}
		probs = append(probs, lintBullet(ver, strings.TrimSpace(line[2:]))...)
	}
	return probs
}

// LintFragment applies the same rules to the bullets of one change fragment (changes/<ID>.md), so an
// over-long or developer-worded line fails in its own change. Each problem is "<name>: <why>: <bullet>".
func LintFragment(name, md string) []string {
	var probs []string
	for _, line := range strings.Split(strings.ReplaceAll(md, "\r", ""), "\n") {
		if strings.HasPrefix(line, "- ") {
			probs = append(probs, lintBullet(name, strings.TrimSpace(line[2:]))...)
		}
	}
	return probs
}

func lintBullet(ver, b string) []string {
	var probs []string
	add := func(why string) { probs = append(probs, fmt.Sprintf("%s: %s: %s", ver, why, b)) }
	if n := len([]rune(b)); n > MaxBulletLen {
		add(fmt.Sprintf("%d characters, over %d", n, MaxBulletLen))
	}
	if missionID.MatchString(b) {
		add("mission id")
	}
	if fileName.MatchString(b) {
		add("file name")
	}
	if devWords.MatchString(b) {
		add("developer word")
	}
	if odds.MatchString(b) {
		add("numbers or odds")
	}
	return probs
}

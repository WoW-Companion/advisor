// Package whatsnew turns the bundled CHANGELOG.md into the short, player-friendly list the
// overlay's Updates section shows: the last few versions, each a few plain lines.
package whatsnew

import (
	"regexp"
	"sort"
	"strconv"
	"strings"

	advisor "github.com/WoW-Companion/advisor"
)

const (
	// Versions is how many versions are shown, newest first.
	Versions = 5
	// MaxLines is the most lines in all, version headings included.
	MaxLines = 25
	// perVersion is the most bullets under one version.
	perVersion = 4
)

// Release is one version and its short lines.
type Release struct {
	Version string   `json:"version"`
	Lines   []string `json:"lines"`
}

var (
	heading  = regexp.MustCompile(`^## (\d+\.\d+\.\d+\S*)\s*$`)
	markup   = regexp.MustCompile("[`*]")
	sentence = regexp.MustCompile(`^(.+?[.!?])(\s|$)`)
)

// Bundled is the list built from the CHANGELOG.md this binary was built with.
func Bundled() []Release { return Parse(advisor.Changelog) }

// Parse reads a changelog (## <version> headings over "- " bullets) into at most Versions releases
// and MaxLines lines. A bullet is cut to its first whole sentence, never mid-sentence.
func Parse(md string) []Release {
	var out []Release
	for _, line := range strings.Split(md, "\n") {
		if m := heading.FindStringSubmatch(line); m != nil {
			if len(out) == Versions {
				break
			}
			out = append(out, Release{Version: m[1]})
			continue
		}
		if len(out) == 0 || !strings.HasPrefix(line, "- ") {
			continue
		}
		r := &out[len(out)-1]
		if len(r.Lines) < perVersion {
			if t := plain(line[2:]); t != "" {
				r.Lines = append(r.Lines, t)
			}
		}
	}
	// Trim to MaxLines: headings count, and the oldest versions give up their bullets first.
	total := len(out)
	for _, r := range out {
		total += len(r.Lines)
	}
	for i := len(out) - 1; total > MaxLines && i >= 0; i-- {
		for len(out[i].Lines) > 1 && total > MaxLines {
			out[i].Lines = out[i].Lines[:len(out[i].Lines)-1]
			total--
		}
	}
	return out
}

// plain is a bullet's first sentence with the markdown marks removed.
func plain(s string) string {
	s = strings.TrimSpace(markup.ReplaceAllString(s, ""))
	if m := sentence.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	return s
}

// boundary is where a sentence ends and the next begins: ". " (or ! ?) before a capital or digit.
var boundary = regexp.MustCompile(`([.!?])\s+([A-Z0-9"“(])`)

// Split turns one paragraph-style note into one bullet per sentence, so notes written before the
// one-change-per-bullet rule still read as a list. Nothing is dropped: joining the parts with a
// space gives back the text. A note with a single sentence comes back as is.
func Split(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	var out []string
	start := 0
	for _, m := range boundary.FindAllStringSubmatchIndex(s, -1) {
		out = append(out, s[start:m[3]]) // up to and including the punctuation
		start = m[4]
	}
	return append(out, s[start:])
}

// Section is one version's part of a changelog, as markdown bullets ("- text" per line, the marks
// removed), or "" when the changelog has no such version. It is the only text a player sees as
// that version's notes: never a release body or a commit message.
func Section(md, version string) string {
	version = strings.TrimPrefix(strings.TrimSpace(version), "v")
	in := false
	var out []string
	for _, line := range strings.Split(strings.ReplaceAll(md, "\r", ""), "\n") {
		if m := heading.FindStringSubmatch(line); m != nil {
			if in {
				break
			}
			in = m[1] == version
			continue
		}
		if in && strings.HasPrefix(line, "- ") {
			for _, t := range Split(strings.TrimSpace(markup.ReplaceAllString(line[2:], ""))) {
				out = append(out, "- "+t)
			}
		}
	}
	return strings.Join(out, "\n")
}

// BundledSection is Section of the CHANGELOG.md this binary was built with.
func BundledSection(version string) string { return Section(advisor.Changelog, version) }

const (
	// RangeVersions is the most versions "What's new" lists when the player skipped some.
	RangeVersions = 3
	// RangeBullets is the most bullet lines in all across those versions.
	RangeBullets = 8
)

// semver reads x.y.z (a -suffix is ignored); ok is false for anything else.
func semver(v string) (n [3]int, ok bool) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	f := strings.Split(strings.SplitN(v, "-", 2)[0], ".")
	if len(f) != 3 {
		return n, false
	}
	for i, s := range f {
		x, err := strconv.Atoi(s)
		if err != nil || x < 0 {
			return n, false
		}
		n[i] = x
	}
	return n, true
}

func less(a, b [3]int) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}

// Range is the notes for every version after from, up to and including to, newest first, each
// headed by its version ("## 0.7.58" over "- " bullets, the marks removed). Versions in skip (pulled
// ones) are left out. At most RangeVersions versions and RangeBullets bullets are listed; when more
// is hidden the text ends with "…and N more changes". "" when no version in range has notes.
func Range(md, from, to string, skip []string) string {
	lo, ok1 := semver(from)
	hi, ok2 := semver(to)
	if !ok1 || !ok2 {
		return ""
	}
	pulled := map[string]bool{}
	for _, s := range skip {
		pulled[strings.TrimPrefix(strings.TrimSpace(s), "v")] = true
	}
	type sec struct {
		version string
		n       [3]int
		bullets []string
	}
	var secs []sec
	for _, line := range strings.Split(strings.ReplaceAll(md, "\r", ""), "\n") {
		if m := heading.FindStringSubmatch(line); m != nil {
			n, ok := semver(m[1])
			if ok && less(lo, n) && !less(hi, n) && !pulled[m[1]] {
				secs = append(secs, sec{version: m[1], n: n})
			} else {
				secs = append(secs, sec{}) // out of range: its bullets are ignored
			}
			continue
		}
		if len(secs) == 0 || secs[len(secs)-1].version == "" || !strings.HasPrefix(line, "- ") {
			continue
		}
		for _, t := range Split(strings.TrimSpace(markup.ReplaceAllString(line[2:], ""))) {
			s := &secs[len(secs)-1]
			s.bullets = append(s.bullets, "- "+t)
		}
	}
	var in []sec
	total := 0
	for _, s := range secs {
		if s.version != "" && len(s.bullets) > 0 {
			in = append(in, s)
			total += len(s.bullets)
		}
	}
	sort.SliceStable(in, func(i, j int) bool { return less(in[j].n, in[i].n) })
	var out []string
	shown := 0
	for i, s := range in {
		if i == RangeVersions || shown == RangeBullets {
			break
		}
		b := s.bullets
		if room := RangeBullets - shown; len(b) > room {
			b = b[:room]
		}
		out = append(out, "## "+s.version)
		out = append(out, b...)
		shown += len(b)
	}
	if hidden := total - shown; hidden > 0 {
		out = append(out, "…and "+strconv.Itoa(hidden)+" more changes")
	}
	return strings.Join(out, "\n")
}

// BundledRange is Range of the CHANGELOG.md this binary was built with.
func BundledRange(from, to string, skip []string) string {
	return Range(advisor.Changelog, from, to, skip)
}

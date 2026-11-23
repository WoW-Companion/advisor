// Package whatsnew turns the bundled CHANGELOG.md into the short, player-friendly list the
// overlay's Updates section shows: the last few versions, each a few plain lines.
package whatsnew

import (
	"regexp"
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

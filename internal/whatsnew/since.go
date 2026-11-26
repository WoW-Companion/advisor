package whatsnew

import (
	"sort"
	"strings"
	"time"
)

const (
	// SinceVersions is the most versions one component lists in the "What's new" view.
	SinceVersions = 50
	// SinceDays is the oldest a listed version may be, when its date is known.
	SinceDays = 180
)

// Since is the notes of every version after from, up to and including to, newest first, one Release
// per version with all its bullets ("- " and marks removed). Pulled versions in skip are left out.
// At most SinceVersions versions are kept, and none published more than SinceDays before now when
// published has its date (a version with no known date is kept). more is true when versions in range
// were left out by either cap. An empty from means nothing was seen: no versions.
func Since(md, from, to string, skip []string, published map[string]time.Time, now time.Time) (out []Release, more bool) {
	lo, ok1 := semver(from)
	hi, ok2 := semver(to)
	if !ok1 || !ok2 {
		return nil, false
	}
	pulled := map[string]bool{}
	for _, s := range skip {
		pulled[strings.TrimPrefix(strings.TrimSpace(s), "v")] = true
	}
	type sec struct {
		r Release
		n [3]int
	}
	var secs []sec
	cur := -1
	for _, line := range strings.Split(strings.ReplaceAll(md, "\r", ""), "\n") {
		if m := heading.FindStringSubmatch(line); m != nil {
			cur = -1
			if n, ok := semver(m[1]); ok && less(lo, n) && !less(hi, n) && !pulled[m[1]] {
				secs = append(secs, sec{r: Release{Version: m[1], Lines: []string{}}, n: n})
				cur = len(secs) - 1
			}
			continue
		}
		if cur < 0 || !strings.HasPrefix(line, "- ") {
			continue
		}
		secs[cur].r.Lines = append(secs[cur].r.Lines, Split(strings.TrimSpace(markup.ReplaceAllString(line[2:], "")))...)
	}
	sort.SliceStable(secs, func(i, j int) bool { return less(secs[j].n, secs[i].n) })
	cutoff := now.AddDate(0, 0, -SinceDays)
	for _, s := range secs {
		if len(s.r.Lines) == 0 {
			continue
		}
		if t, ok := published[s.r.Version]; ok && !t.IsZero() && t.Before(cutoff) {
			more = true
			continue
		}
		if len(out) == SinceVersions {
			more = true
			continue
		}
		out = append(out, s.r)
	}
	return out, more
}

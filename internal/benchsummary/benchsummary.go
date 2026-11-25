// Package benchsummary counts the same and differ results of the advice benchmark and prints them as
// one line, so the share of advice that agrees with the game data and the Forever guides can be read per
// run. A benchmark prints a count line per area; Total adds the area lines up.
package benchsummary

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// Prefix starts every count line a benchmark area prints.
const Prefix = "advice-benchmark-count"

// Count is the number of results that match and the number that differ. A result is "match" or begins
// with "differ"; anything else (a skipped check) counts as neither.
func Count(results []string) (same, differ int) {
	for _, r := range results {
		switch {
		case r == "match":
			same++
		case strings.HasPrefix(r, "differ"):
			differ++
		}
	}
	return same, differ
}

// AreaLine is the count line one benchmark area prints: "advice-benchmark-count professions same=174 differ=37".
func AreaLine(area string, results []string) string {
	same, differ := Count(results)
	return fmt.Sprintf("%s %s same=%d differ=%d", Prefix, area, same, differ)
}

// Total reads the count lines among the lines of r and returns the one summary line: the totals first,
// then each area, "advice-benchmark: 211 checks, 185 same, 26 differ (professions 174/37, stats 99/0)".
// It reports ok false when no count line is found.
func Total(r io.Reader) (line string, ok bool) {
	var same, differ int
	var areas []string
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) != 4 || f[0] != Prefix {
			continue
		}
		var s, d int
		if _, err := fmt.Sscanf(f[2]+" "+f[3], "same=%d differ=%d", &s, &d); err != nil {
			continue
		}
		same, differ = same+s, differ+d
		areas = append(areas, fmt.Sprintf("%s %d/%d", f[1], s, d))
	}
	if len(areas) == 0 {
		return "", false
	}
	return fmt.Sprintf("advice-benchmark: %d checks, %d same, %d differ (%s)", same+differ, same, differ, strings.Join(areas, ", ")), true
}

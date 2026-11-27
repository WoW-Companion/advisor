package ui

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
)

var pxRE = regexp.MustCompile(`^-?\d+(\.\d+)?(px)?$`)

// The overlay window is transparent with no OS shadow, so an outer shadow reaching past 2px is
// clipped at the window edge. No rule may carry one.
func TestNoOuterShadowBeyondTwoPx(t *testing.T) {
	css := regexp.MustCompile(`(?s)/\*.*?\*/`).ReplaceAllString(read(t, "style.css"), "")
	for _, m := range ruleRE.FindAllStringSubmatch(css, -1) {
		for _, decl := range strings.Split(m[2], ";") {
			name, val, ok := strings.Cut(decl, ":")
			if !ok || strings.TrimSpace(name) != "box-shadow" {
				continue
			}
			depth, start := 0, 0
			var layers []string
			for i, c := range val {
				switch c {
				case '(':
					depth++
				case ')':
					depth--
				case ',':
					if depth == 0 {
						layers = append(layers, val[start:i])
						start = i + 1
					}
				}
			}
			layers = append(layers, val[start:])
			for _, l := range layers {
				l = regexp.MustCompile(`rgba?\([^)]*\)`).ReplaceAllString(l, "")
				f := strings.Fields(l)
				if len(f) > 0 && f[0] == "inset" {
					continue
				}
				var nums []float64
				for _, w := range f {
					if pxRE.MatchString(w) {
						n, _ := strconv.ParseFloat(strings.TrimSuffix(w, "px"), 64)
						if n < 0 {
							n = -n
						}
						nums = append(nums, n)
					}
				}
				for _, n := range nums {
					if n > 2 {
						t.Errorf("%s: outer box-shadow layer %q reaches past 2px", strings.TrimSpace(m[1]), strings.TrimSpace(l))
					}
				}
			}
		}
	}
}

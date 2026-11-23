package whatsnew

import (
	"strings"
	"testing"
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

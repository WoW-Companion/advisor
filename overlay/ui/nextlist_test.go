package ui

import (
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

// The Next list entries run as real code under Node: each kind, the text fallback and no clipping.
func TestNextListLogicUnderNode(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	if out, err := exec.Command(node, "nextlist_logic_test.js").CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
}

// The page loads the module and the Next list may wrap but never clip its entries.
func TestNextListWrapsAndNeverClips(t *testing.T) {
	if !strings.Contains(read(t, "index.html"), `src="nextlist.js"`) || !strings.Contains(read(t, "app.js"), "WowcNext.render") {
		t.Error("the Next list is not rendered by nextlist.js")
	}
	css := regexp.MustCompile(`(?s)/\*.*?\*/`).ReplaceAllString(read(t, "style.css"), "")
	for _, m := range ruleRE.FindAllStringSubmatch(css, -1) {
		sel := strings.TrimSpace(m[1])
		if strings.Contains(sel, "#next-line") || !strings.Contains(sel, ".next") {
			continue // the collapsed one-line summary clips by design
		}
		if strings.Contains(m[2], "text-overflow") || strings.Contains(m[2], "nowrap") {
			t.Errorf("%s clips the Next list: %s", sel, m[2])
		}
	}
	if !strings.Contains(css, "overflow-wrap") {
		t.Error("Next entries must wrap")
	}
}

package ask

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A talent question gets the curated-build rule and no fetch tool: every preset's argv is the same
// with or without the question, and no prompt or shipped text names a guide site.
func TestTalentAnswerNeedsNoFetch(t *testing.T) {
	for name, cmd := range Presets {
		argv := Argv(cmd, "What talents should I get?", "")
		if got := strings.ToLower(strings.Join(argv, " ")); strings.Contains(got, "webfetch") || strings.Contains(got, "icy-veins") {
			t.Errorf("%s: argv gives a fetch: %v", name, argv)
		}
	}
	cfg := Config{Command: Presets["claude"]}
	p := cfg.Prompt(Question{Text: "What talents should I get?"})
	if !strings.Contains(p, "talent_plan") {
		t.Errorf("the prompt does not point at talent_plan: %s", p)
	}
}

func TestNoGuideLookupTextAnywhere(t *testing.T) {
	var files []string
	for _, root := range []string{"../../clients", "../../docs", "../../overlay"} {
		filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
			if err == nil && !d.IsDir() && !strings.Contains(p, "node_modules") && !strings.Contains(p, "/target/") && !strings.Contains(p, "/dist/") {
				files = append(files, p)
			}
			return nil
		})
	}
	goFiles, _ := filepath.Glob("../*/*.go")
	files = append(files, goFiles...)
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") || strings.HasSuffix(f, ".png") || strings.HasSuffix(f, ".ico") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		s := strings.ToLower(string(b))
		for _, bad := range []string{"icy-veins", "icy veins", "guide url", "guide_url", "webfetch"} {
			if strings.Contains(s, bad) {
				t.Errorf("%s names %q", f, bad)
			}
		}
	}
}

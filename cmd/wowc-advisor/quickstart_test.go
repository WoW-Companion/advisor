package main

import (
	"os"
	"regexp"
	"testing"
)

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// The quickstart must only name tools and subcommands that exist, and name every tool.
func TestQuickstartMatchesCode(t *testing.T) {
	doc := read(t, "../../docs/quickstart.md")

	tools := map[string]bool{}
	for _, m := range regexp.MustCompile(`Name:\s+"(\w+)"`).FindAllStringSubmatch(read(t, "../../internal/server/server.go"), -1) {
		tools[m[1]] = true
	}
	subs := map[string]bool{}
	for _, m := range regexp.MustCompile(`case "(\w+)":`).FindAllStringSubmatch(read(t, "main.go"), -1) {
		subs[m[1]] = true
	}
	if len(tools) < 13 || len(subs) < 6 {
		t.Fatalf("found %d tools and %d subcommands in the code", len(tools), len(subs))
	}

	for name := range tools {
		if !regexp.MustCompile("\\| `" + name + "` \\|").MatchString(doc) {
			t.Errorf("quickstart table is missing tool %s", name)
		}
	}
	for _, m := range regexp.MustCompile(`(?m)^\| `+"`(\\w+)`"+` \|`).FindAllStringSubmatch(doc, -1) {
		if !tools[m[1]] {
			t.Errorf("quickstart lists tool %q that the server does not register", m[1])
		}
	}
	for _, m := range regexp.MustCompile(`wowc-advisor(?:\.exe)? ([a-z]\w*)`).FindAllStringSubmatch(doc, -1) {
		if !subs[m[1]] {
			t.Errorf("quickstart uses subcommand %q that main.go does not handle", m[1])
		}
	}
}

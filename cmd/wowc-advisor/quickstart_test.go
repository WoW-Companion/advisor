package main

import (
	"os"
	"regexp"
	"strings"
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

// Every overlay setting the quickstart names must exist in the overlay's UI, and the default
// shortcuts it names must be the ones app.js ships.
func TestQuickstartOverlaySettingsExist(t *testing.T) {
	doc := read(t, "../../docs/quickstart.md")
	html := read(t, "../../overlay/ui/index.html")
	js := read(t, "../../overlay/ui/app.js")

	for _, label := range []string{"Opacity", "Solid on hover", "AI client", "Re-scan", "Text size", "Click-through", "Clear"} {
		if !strings.Contains(doc, label) {
			t.Errorf("quickstart does not name the %q setting", label)
		}
		if !strings.Contains(html, label) {
			t.Errorf("overlay/ui/index.html has no %q", label)
		}
	}
	for _, size := range []string{"Small", "Medium", "Large"} {
		if !regexp.MustCompile(`>` + size + `<`).MatchString(html) {
			t.Errorf("overlay/ui has no text size %q", size)
		}
	}
	for field, want := range map[string]string{"keyVis": "Ctrl+Shift+W", "keyCt": "Ctrl+Shift+T"} {
		if !regexp.MustCompile(field + `: '` + regexp.QuoteMeta(want) + `'`).MatchString(js) {
			t.Errorf("overlay/ui/app.js default %s is not %s", field, want)
		}
		if !regexp.MustCompile("`" + regexp.QuoteMeta(want) + "`").MatchString(doc) {
			t.Errorf("quickstart does not name the shortcut %s", want)
		}
	}
	for _, id := range []string{`id="settings"`, `id="next-head"`, `id="chat-toggle"`, `id="badge"`, `id="agent-rescan"`, `id="solid-hover"`} {
		if !regexp.MustCompile(id).MatchString(html) {
			t.Errorf("overlay/ui/index.html lost %s", id)
		}
	}
}

// The overlay's version (tauri.conf.json, Cargo.toml, its Cargo.lock entry) is the VERSION file's,
// so the installer name, the tray tooltip and `wowc-advisor version` agree with the release.
func TestOverlayVersionMatchesVersionFile(t *testing.T) {
	want := strings.TrimSpace(read(t, "../../VERSION"))
	for path, re := range map[string]string{
		"../../overlay/src-tauri/tauri.conf.json": `(?m)^\s*"version":\s*"([^"]+)"`,
		"../../overlay/src-tauri/Cargo.toml":      `(?m)^version\s*=\s*"([^"]+)"`,
		"../../overlay/src-tauri/Cargo.lock":      `(?m)^name = "wow-companion"\nversion = "([^"]+)"`,
	} {
		m := regexp.MustCompile(re).FindStringSubmatch(read(t, path))
		if m == nil || m[1] != want {
			t.Errorf("%s: version %v, want %s (the VERSION file)", path, m, want)
		}
	}
}

// The release workflow must attach the installer under the name the docs tell players to look for.
func TestDocsPointAtReleaseInstaller(t *testing.T) {
	for _, path := range []string{"../../docs/quickstart.md", "../../overlay/README.md", "../../README.md"} {
		doc := read(t, path)
		if !strings.Contains(doc, "WoW-Companion-Setup-") || !strings.Contains(doc, "releases") {
			t.Errorf("%s does not point at the Setup exe on the releases page", path)
		}
		if strings.Contains(doc, "overlay does not ship") || strings.Contains(doc, "Releases do not include") {
			t.Errorf("%s still says the releases lack the overlay", path)
		}
	}
	if wf := read(t, "../../.github/workflows/release.yml"); !strings.Contains(wf, "WoW-Companion-Setup-") {
		t.Error("release.yml does not publish WoW-Companion-Setup-<version>-x64.exe")
	}
}

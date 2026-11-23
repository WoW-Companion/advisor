package main

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/ask"
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
	for _, m := range regexp.MustCompile(`Name:\s+"(\w+)"`).FindAllStringSubmatch(read(t, "../../internal/server/server.go")+read(t, "../../internal/server/profile.go")+read(t, "../../internal/server/statcheck.go"), -1) {
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

// The setup order is ForeverScribe (required), the overlay (recommended), then the addon (optional);
// the in-game /ask flow is legacy and the docs say Answers.lua needs the deprecated --answers-file.
func TestDocsOverlayFirstOrder(t *testing.T) {
	doc := read(t, "../../docs/quickstart.md")
	last := -1
	for _, h := range []string{"## 1. Install ForeverScribe (required)", "## 2. Get WoW Companion (the overlay, recommended)", "## 5. Optional: the WoW Companion addon"} {
		i := strings.Index(doc, h)
		if i < 0 || i < last {
			t.Errorf("quickstart heading %q is missing or out of order", h)
		}
		last = i
	}
	if strings.Contains(doc, "`WoWCompanion_Answers`, ") || strings.Contains(doc, "three WoW Companion folders") {
		t.Error("quickstart still asks for the WoWCompanion_Answers folder")
	}
	if !strings.Contains(doc, "Legacy: the in-game `/ask` flow") {
		t.Error("quickstart has no legacy FAQ entry for the in-game /ask flow")
	}
	main := read(t, "main.go")
	for _, f := range []string{"--answers-file"} {
		if !strings.Contains(main, f) || !strings.Contains(read(t, "../../docs/ask.md"), f) {
			t.Errorf("%s is not in the usage and docs/ask.md", f)
		}
	}
	for _, p := range []string{"../../README.md", "../../overlay/README.md"} {
		if d := read(t, p); !strings.Contains(d, "optional") || !strings.Contains(d, "WoWCompanion_Answers") && p != "../../README.md" {
			t.Errorf("%s does not call the addon optional", p)
		}
	}
}

// Every step the one-page Getting Started names must match the overlay's setup checklist, its
// settings and the commands that exist; the feedback form and the links must be in place.
func TestGettingStartedMatchesOverlay(t *testing.T) {
	doc := read(t, "../../docs/getting-started.md")
	html := read(t, "../../overlay/ui/index.html")
	setup := read(t, "../../internal/gamesetup/gamesetup.go")

	// Each checklist item (by its title in gamesetup) is named in the page, as the page words it.
	titles := regexp.MustCompile(`Item\{ID: ID\w+, Title: "([^"]+)"`).FindAllStringSubmatch(setup, -1)
	if len(titles) < 5 {
		t.Fatalf("found %d checklist titles in gamesetup.go", len(titles))
	}
	words := map[string]string{
		"WoW: Forever found":             "game found",
		"ForeverScribe installed":        "ForeverScribe installed",
		"ForeverScribe has data":         "ForeverScribe has data",
		"WoW Companion addon (optional)": "WoW Companion addon (optional",
		"Display mode":                   "display mode",
		"Questie (optional)":             "Questie (optional",
		"Game data":                      "game data",
	}
	seen := map[string]bool{}
	for _, m := range titles {
		if seen[m[1]] {
			continue
		}
		seen[m[1]] = true
		w, ok := words[m[1]]
		if !ok {
			t.Errorf("checklist item %q is not covered by getting-started", m[1])
			continue
		}
		if !strings.Contains(strings.ToLower(doc), strings.ToLower(w)) {
			t.Errorf("getting-started does not name checklist item %q", m[1])
		}
	}
	if !strings.Contains(setup, `Label = "Open ForeverScribe page"`) && !strings.Contains(setup, `"Open ForeverScribe page"`) {
		t.Error("the checklist has no \"Open ForeverScribe page\" button")
	}
	if !strings.Contains(doc, "Open ForeverScribe page") {
		t.Error("getting-started does not name the \"Open ForeverScribe page\" button")
	}

	// Settings and controls the page names exist in the overlay UI.
	for _, label := range []string{"Re-scan", "Setup checklist"} {
		if !strings.Contains(doc, label) || !strings.Contains(html, label) {
			t.Errorf("%q must be in both getting-started and the overlay UI", label)
		}
	}
	for _, id := range []string{`id="setup"`, `id="setup-open"`, `id="agent-rescan"`} {
		if !strings.Contains(html, id) {
			t.Errorf("overlay/ui/index.html lost %s", id)
		}
	}
	if !strings.Contains(doc, "Windowed (Fullscreen)") || !strings.Contains(doc, "Display Mode") {
		t.Error("getting-started lost the Windowed (Fullscreen) step")
	}

	// Commands: the page uses only subcommands main.go handles, and an installer name the release publishes.
	subs := map[string]bool{}
	for _, m := range regexp.MustCompile(`case "(\w+)":`).FindAllStringSubmatch(read(t, "main.go"), -1) {
		subs[m[1]] = true
	}
	for _, m := range regexp.MustCompile(`wowc-advisor(?:\.exe)? ([a-z]\w*)`).FindAllStringSubmatch(doc, -1) {
		if !subs[m[1]] {
			t.Errorf("getting-started uses subcommand %q that main.go does not handle", m[1])
		}
	}
	if !strings.Contains(doc, "WoW-Companion-Setup-<version>-x64.exe") {
		t.Error("getting-started does not name the installer")
	}

	// Links: README lists Getting Started before the quickstart; the overlay's link opens only the allowed page.
	readme := read(t, "../../README.md")
	g, q := strings.Index(readme, "docs/getting-started.md"), strings.Index(readme, "docs/quickstart.md")
	if g < 0 || q < 0 || g > q {
		t.Error("README must link getting-started before the quickstart")
	}
	const page = "https://github.com/WoW-Companion/advisor/blob/main/docs/getting-started.md"
	if !strings.Contains(html, `data-page="`+page+`"`) {
		t.Error("the Setup panel has no Getting Started link")
	}
	if !strings.Contains(read(t, "../../overlay/src-tauri/src/main.rs"), `"`+page+`"`) {
		t.Error("open_install_page does not allow the Getting Started page")
	}

	// The feedback form is an issue form with the fields the page promises, and no workflow.
	form := read(t, "../../.github/ISSUE_TEMPLATE/alpha-feedback.yml")
	for _, f := range []string{"tried", "happened", "expected", "version", "client", "checklist", "screenshots", "privacy"} {
		if !strings.Contains(form, "id: "+f+"\n") {
			t.Errorf("alpha-feedback.yml has no field %q", f)
		}
	}
	if !strings.Contains(form, "No account, character or other players' names in my screenshots") || !strings.Contains(form, "required: true") {
		t.Error("alpha-feedback.yml lost the screenshot privacy checkbox")
	}
	if !strings.Contains(doc, "template=alpha-feedback.yml") {
		t.Error("getting-started does not link the feedback form")
	}
}

// The AI-client advice must name Antigravity CLI with its official install link in the docs, the
// checklist text and the overlay, and call Gemini CLI legacy.
func TestAntigravityAdviceInDocs(t *testing.T) {
	const link = "https://antigravity.google/docs/getting-started?tab=cli"
	for _, p := range []string{"../../docs/getting-started.md", "../../overlay/ui/index.html"} {
		if d := read(t, p); !strings.Contains(d, link) {
			t.Errorf("%s does not link Antigravity CLI's install page", p)
		}
	}
	for _, p := range []string{"../../docs/getting-started.md", "../../docs/quickstart.md"} {
		if d := read(t, p); !strings.Contains(d, "legacy") {
			t.Errorf("%s does not call Gemini CLI legacy", p)
		}
	}
	if d := read(t, "../../internal/gamesetup/ai.go"); !strings.Contains(d, "Antigravity") {
		t.Error("the setup checklist's install note does not name Antigravity CLI")
	}
	if d := read(t, "../../overlay/src-tauri/src/main.rs"); !strings.Contains(d, link) {
		t.Error("the overlay may not open Antigravity CLI's install page")
	}
}

// Every client the Getting Started guide has a section for is a preset in code, every preset has a
// section, and the install links the overlay opens are the ones the guide gives.
func TestClientGuideMatchesPresets(t *testing.T) {
	doc := read(t, "../../docs/getting-started.md")
	sec := doc[strings.Index(doc, "## Set up your AI client"):]
	sec = sec[:strings.Index(sec, "## Permissions and safety")]

	// Guide heading -> preset id; the overlay's own names come from CLIENT_NAMES in app.js.
	heads := map[string]string{"Claude Code": "claude", "Codex": "codex", "Antigravity CLI": "agy", "Gemini CLI": "gemini"}
	found := map[string]bool{}
	for _, m := range regexp.MustCompile(`(?m)^### ([^\n(]+?)(?: \(.*\))?$`).FindAllStringSubmatch(sec, -1) {
		id, ok := heads[m[1]]
		if !ok {
			t.Errorf("guide has a section for %q, which is not a known client", m[1])
			continue
		}
		if _, ok := ask.Presets[id]; !ok {
			t.Errorf("guide names %q but there is no %q preset", m[1], id)
		}
		found[id] = true
	}
	for id := range ask.Presets {
		if !found[id] {
			t.Errorf("preset %q has no section in the guide", id)
		}
	}
	js := read(t, "../../overlay/ui/app.js")
	for id := range ask.Presets {
		if !regexp.MustCompile(id + `: '`).MatchString(js) {
			t.Errorf("overlay CLIENT_NAMES has no %q", id)
		}
	}

	// Install links: each one in the overlay's notice and its allowlist is in the guide, and the
	// overlay offers every guide link that is an official install page.
	html := read(t, "../../overlay/ui/index.html")
	rs := read(t, "../../overlay/src-tauri/src/main.rs")
	for _, m := range regexp.MustCompile(`data-page="([^"]+)"`).FindAllStringSubmatch(html, -1) {
		u := m[1]
		if strings.Contains(u, "WoW-Companion/advisor") {
			continue
		}
		if !strings.Contains(doc, "("+u+")") {
			t.Errorf("overlay links %s, which the guide does not", u)
		}
		if !strings.Contains(rs, `"`+u+`"`) {
			t.Errorf("open_install_page does not allow %s", u)
		}
	}
	for _, u := range []string{"https://code.claude.com/docs/en/setup", "https://github.com/openai/codex", "https://antigravity.google/docs/getting-started?tab=cli"} {
		if !strings.Contains(sec, "("+u+")") || !strings.Contains(html, `data-page="`+u+`"`) {
			t.Errorf("%s must be in both the guide and the overlay notice", u)
		}
	}
	if !strings.Contains(read(t, "../../internal/gamesetup/ai.go"), "Antigravity") {
		t.Error("checklist install note does not name Antigravity")
	}
}

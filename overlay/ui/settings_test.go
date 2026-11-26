package ui

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// No anchor may fall back to the browser's default link colour (dark blue on the dark panel): one
// global rule must give every link the gold accent, an underline, and hover and focus states.
func TestLinksNeverUseDefaultColour(t *testing.T) {
	css := regexp.MustCompile(`(?s)/\*.*?\*/`).ReplaceAllString(read(t, "style.css"), "")
	var rule string
	for _, m := range ruleRE.FindAllStringSubmatch(css, -1) {
		sel := strings.TrimSpace(m[1])
		if sel == "a, a:link, a:visited" {
			rule = m[2]
		}
	}
	if !strings.Contains(rule, "color: var(--gold)") || !strings.Contains(rule, "underline") {
		t.Errorf("links need a gold, underlined base rule; got %q", rule)
	}
	for _, want := range []string{"a:hover", "a:focus-visible"} {
		if !strings.Contains(css, want) {
			t.Errorf("style.css lacks %s", want)
		}
	}
	// A later per-link colour must not undo the rule with a non-gold value other than inherit.
	for _, m := range ruleRE.FindAllStringSubmatch(css, -1) {
		sel := strings.TrimSpace(m[1])
		if regexp.MustCompile(`(^|[\s,])a([\s:.\[,]|$)|\ba\b`).MatchString(sel) && sel != "a, a:link, a:visited" && sel != "a:hover" && sel != "a:focus-visible" {
			if regexp.MustCompile(`color:\s*(#|rgb|[a-z]+\b)`).MatchString(m[2]) && !strings.Contains(m[2], "inherit") {
				t.Errorf("%s sets its own link colour", sel)
			}
		}
	}
}

// Every setting inside the settings panel is a grid row: a label cell and a control cell.
func TestSettingRowsUseGrid(t *testing.T) {
	html, css := read(t, "index.html"), read(t, "style.css")
	if !regexp.MustCompile(`\.settings \.row \{[^}]*display: grid; grid-template-columns: minmax\(0, 1fr\) `).MatchString(css) {
		t.Error(".settings .row must be a two-column grid with a shrinkable label column")
	}
	s := html[strings.Index(html, `id="settings"`):strings.Index(html, `class="bar"`)]
	labels := regexp.MustCompile(`<label[^>]*>`).FindAllString(s, -1)
	if len(labels) < 10 {
		t.Fatalf("only %d settings labels found", len(labels))
	}
	for _, l := range labels {
		if !strings.Contains(l, `class="row"`) {
			t.Errorf("%s is not a grid row", l)
		}
	}
	rows := regexp.MustCompile(`(?s)<label class="row"[^>]*>(.*?)</label>`).FindAllStringSubmatch(s, -1)
	for _, r := range rows {
		if !strings.HasPrefix(r[1], `<span class="lbl">`) || !strings.Contains(r[1], `<span class="ctl">`) {
			t.Errorf("row lacks label and control cells: %.60s", r[1])
		}
	}
}

// The display names live in one table in app.js and Go says the same names in the checklist.
func TestClientDisplayNames(t *testing.T) {
	js, html := read(t, "app.js"), read(t, "index.html")
	for _, n := range []string{"Claude Code", "Codex", "Antigravity", "Gemini CLI"} {
		if strings.Count(js, "'"+n+"'") != 1 {
			t.Errorf("app.js must name %q exactly once (the table)", n)
		}
	}
	if strings.Contains(js, "[0].toUpperCase() + conn.agent") {
		t.Error("app.js still builds a client name by capitalising the id")
	}
	if !strings.Contains(html, ">Claude Code</option>") || !strings.Contains(html, ">Gemini CLI (legacy, API key only)</option>") {
		t.Error("the picker must use display names")
	}
}

// Groups: six, the AI one open by default, state kept in cfg.groups; the page has one scroll, its
// own, and the version line closes it.
func TestSettingsGroups(t *testing.T) {
	html, js := read(t, "index.html"), read(t, "app.js")
	got := regexp.MustCompile(`<details class="grp" data-group="(\w+)"`).FindAllStringSubmatch(html, -1)
	var ids []string
	for _, m := range got {
		ids = append(ids, m[1])
	}
	if strings.Join(ids, ",") != "appearance,ai,style,game,shortcuts,updates" {
		t.Errorf("groups = %v", ids)
	}
	if regexp.MustCompile(`<details[^>]*\sopen`).MatchString(html) {
		t.Error("open state comes from cfg, not the markup")
	}
	for _, want := range []string{"groups: { ai: true }", "g === 'ai'", "cfg.groups[g] = d.open; save()"} {
		if !strings.Contains(js, want) {
			t.Errorf("app.js lacks %q", want)
		}
	}
	page := html[strings.Index(html, `id="settings"`):strings.Index(html, `class="bar"`)]
	if strings.Index(page, `id="version"`) < strings.LastIndex(page, `</details>`) {
		t.Error("the version line must sit at the bottom of the page")
	}
}

// Settings is one full-height page with one scroll: the page itself. No inner box scrolls, the gear
// shows an active state and a "Close settings" tooltip, Esc and Done close, and the chat scroll returns.
func TestSettingsPage(t *testing.T) {
	html, js := read(t, "index.html"), read(t, "app.js")
	css := regexp.MustCompile(`(?s)/\*.*?\*/`).ReplaceAllString(read(t, "style.css"), "")
	page := html[strings.Index(html, `id="settings"`):strings.Index(html, `class="bar"`)]
	if strings.Contains(page, "settings-body") || strings.Contains(css, ".settings-body") {
		t.Error("the fixed-height settings container is gone")
	}
	if !strings.Contains(page, `id="settings-done"`) || !strings.Contains(page, ">Done<") {
		t.Error("the page needs a Done button")
	}
	scrollers := 0
	for _, m := range ruleRE.FindAllStringSubmatch(css, -1) {
		sel := strings.TrimSpace(m[1])
		if strings.Contains(m[2], "overflow-y: auto") || strings.Contains(m[2], "overflow: auto") || strings.Contains(m[2], "overflow-y: scroll") {
			if strings.Contains(sel, ".settings") {
				scrollers++
				if sel != ".settings" {
					t.Errorf("%s scrolls inside the settings page", sel)
				}
			}
		}
	}
	if scrollers != 1 {
		t.Errorf("settings has %d scroll containers, want 1", scrollers)
	}
	if !regexp.MustCompile(`\.settings \{[^}]*flex: 1 1 auto`).MatchString(css) {
		t.Error("the page must fill the window body")
	}
	if !regexp.MustCompile(`\.panel\.settings-open > :is\([^)]*#log[^)]*#form[^)]*\) \{ display: none; \}`).MatchString(css) {
		t.Error("the page must replace the bar, Next list and chat (kept mounted, not removed)")
	}
	if !strings.Contains(css, `#settings-btn[aria-pressed=true]`) || !strings.Contains(css, `#settings-btn:focus-visible`) {
		t.Error("the gear needs active and focus-visible looks")
	}
	for _, want := range []string{"'Close settings' : 'Settings'", "setAttribute('aria-pressed'", "ev.key === 'Escape'", "$('settings-done').onclick", "log.scrollTop = chatScroll"} {
		if !strings.Contains(js, want) {
			t.Errorf("app.js lacks %q", want)
		}
	}
}

// Six collapsed sections must fit at the default window height: each closed group is one summary row.
func TestSettingsCollapsedFitDefaultHeight(t *testing.T) {
	conf := read(t, "../src-tauri/tauri.conf.json")
	m := regexp.MustCompile(`"height":\s*(\d+)`).FindStringSubmatch(conf)
	if m == nil {
		t.Fatal("no default window height in tauri.conf.json")
	}
	h, _ := strconv.Atoi(m[1])
	// title bar 42 + page padding 20 + head 30 + 6 groups at about 36 + version 30 + gaps 56
	const need = 42 + 20 + 30 + 6*36 + 30 + 56
	if h < need {
		t.Errorf("default height %d is under the %d the collapsed page needs", h, need)
	}
}

// "Copy diagnostics" builds its text in the advisor and puts it on the clipboard: no file is written, and
// the button carries the one-line note and shows "Copied".
func TestCopyDiagnosticsWiring(t *testing.T) {
	html, js, rs := read(t, "index.html"), read(t, "app.js"), read(t, "../src-tauri/src/main.rs")
	for _, want := range []string{`id="copy-diag"`, "Copies versions and recent errors, without character names, for sharing when something goes wrong."} {
		if !strings.Contains(html, want) {
			t.Errorf("index.html lacks %q", want)
		}
	}
	for _, want := range []string{"invoke('diagnostics_files')", "'/api/diagnostics'", "navigator.clipboard.writeText", "'Copied'"} {
		if !strings.Contains(js, want) {
			t.Errorf("app.js lacks %q", want)
		}
	}
	if strings.Contains(js, "copy_diagnostics") || strings.Contains(rs, "write_diagnostics") || strings.Contains(rs, "download_dir") {
		t.Error("diagnostics must not be written to a file any more")
	}
	if !strings.Contains(rs, "diagnostics_files,") {
		t.Error("main.rs does not register diagnostics_files")
	}
}

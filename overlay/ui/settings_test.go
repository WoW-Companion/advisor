package ui

import (
	"regexp"
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

// Groups: six, the AI one open by default, state kept in cfg.groups; the version line sits outside
// the scrolling body so it is always visible.
func TestSettingsGroups(t *testing.T) {
	html, js, css := read(t, "index.html"), read(t, "app.js"), read(t, "style.css")
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
	body := html[strings.Index(html, `class="settings-body"`):strings.Index(html, `id="version"`)]
	if strings.Count(body, "</div>") != 1 || strings.Contains(body, `id="version"`) {
		t.Error("the version line must follow the scrolling body")
	}
	if !regexp.MustCompile(`\.settings-body \{[^}]*overflow-x: hidden; overflow-y: auto`).MatchString(css) {
		t.Error("the settings body must scroll vertically only")
	}
}

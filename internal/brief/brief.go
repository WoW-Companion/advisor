// Package brief renders plain-text briefs of captured characters. Every line comes from the
// capture; gaps are stated.
package brief

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/WoW-Companion/advisor/internal/character"
)

// StaleAfter is the capture age after which a brief carries a warning.
const StaleAfter = time.Hour

// Slots are the standard inventory slots, by the lowercase names ForeverScribe uses.
var Slots = []string{
	"head", "neck", "shoulder", "back", "chest", "shirt", "tabard", "wrist", "hands", "waist",
	"legs", "feet", "finger1", "finger2", "trinket1", "trinket2", "mainhand", "offhand", "ranged",
}

const (
	notCaptured     = "not captured"
	bankNotCaptured = "bank not captured yet: open your bank once in game"
)

// LookupError means the requested character cannot be chosen; the message says what to do.
type LookupError struct{ Msg string }

func (e *LookupError) Error() string { return e.Msg }

func utc(ts int64) string { return time.Unix(ts, 0).UTC().Format("2006-01-02 15:04 UTC") }

// UTC formats a capture time the way briefs do.
func UTC(ts int64) string { return utc(ts) }

func orNot(s string) string {
	if s == "" {
		return notCaptured
	}
	return s
}

func intOr(p *int, alt string) string {
	if p == nil {
		return alt
	}
	return fmt.Sprint(*p)
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// FormatMoney renders copper as gold, silver and copper.
func FormatMoney(copper *int) string {
	if copper == nil {
		return notCaptured
	}
	return fmt.Sprintf("%dg %ds %dc", *copper/10000, *copper%10000/100, *copper%100)
}

// ClientLine describes the client that wrote the capture; "" when none was recorded.
func ClientLine(c character.Client) string {
	if c.Empty() {
		return ""
	}
	var extra []string
	if c.Build != "" {
		extra = append(extra, "build "+c.Build)
	}
	if c.TOC != "" {
		extra = append(extra, "toc "+c.TOC)
	}
	v := orNot(c.Version)
	if c.Version == "" {
		v = "version not captured"
	}
	if len(extra) > 0 {
		v += " (" + strings.Join(extra, ", ") + ")"
	}
	return v
}

// Entry is one row of list_characters.
type Entry struct {
	Name       string  `json:"name"`
	Key        string  `json:"key"`
	Level      *int    `json:"level"`
	Class      string  `json:"class"`
	Realm      string  `json:"realm"`
	CapturedAt *string `json:"captured_at"`
}

func displayName(c character.Character) string {
	if c.Name != "" {
		return c.Name
	}
	return c.Key
}

// List summarises every captured character.
func List(f character.File) []Entry {
	out := []Entry{}
	for _, c := range f.Characters {
		e := Entry{Name: displayName(c), Key: c.Key, Level: c.Level, Class: c.Class, Realm: c.Realm}
		if c.CapturedAt != nil {
			s := utc(*c.CapturedAt)
			e.CapturedAt = &s
		}
		out = append(out, e)
	}
	return out
}

func names(f character.File) string {
	n := make([]string, len(f.Characters))
	for i, c := range f.Characters {
		n[i] = displayName(c)
	}
	return strings.Join(n, ", ")
}

// Choose picks the character a request names (or the only one).
func Choose(f character.File, name string) (character.Character, error) {
	chars := f.Characters
	if len(chars) == 0 {
		return character.Character{}, &LookupError{"The SavedVariables file holds no characters."}
	}
	name = strings.TrimSpace(name)
	if name == "" {
		if len(chars) == 1 {
			return chars[0], nil
		}
		return character.Character{}, &LookupError{fmt.Sprintf("Several characters were captured (%s); name one.", names(f))}
	}
	var found []character.Character
	for _, c := range chars {
		if strings.EqualFold(c.Name, name) || strings.EqualFold(c.Key, name) {
			found = append(found, c)
		}
	}
	switch len(found) {
	case 1:
		return found[0], nil
	case 0:
		return character.Character{}, &LookupError{fmt.Sprintf("No captured character '%s'; captured: %s.", name, names(f))}
	}
	return character.Character{}, &LookupError{fmt.Sprintf("'%s' matches several characters; use one of: %s.", name, names(f))}
}

type group struct {
	name   string
	points int
}

// groups are the talent groups with their points, summed over trees, in tree order.
func groups(t *character.Talents) []group {
	var out []group
	idx := map[string]int{}
	for _, tree := range t.Trees {
		if tree.IsProfession() {
			continue // the spec is a class tree matter
		}
		if len(tree.Groups) == 0 {
			if tree.Name != "" || (tree.Points != nil && *tree.Points > 0) {
				name := tree.Name
				if name == "" {
					name = "unnamed tree"
				}
				out = append(out, group{name, intOrZero(tree.Points)})
			}
			continue
		}
		gs := append([]character.TalentGroup(nil), tree.Groups...)
		sort.SliceStable(gs, func(i, j int) bool {
			a, b := gs[i].Order, gs[j].Order
			return a != nil && b != nil && *a < *b
		})
		for _, g := range gs {
			if i, ok := idx[g.Name]; ok {
				out[i].points += g.Spent
				continue
			}
			idx[g.Name] = len(out)
			out = append(out, group{g.Name, g.Spent})
		}
	}
	return out
}

func intOrZero(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

// Spec is the group holding the most points, and every group's points.
func spec(t *character.Talents) (spec, points string) {
	if t == nil || len(t.Trees) == 0 {
		return "unknown (no talents captured)", ""
	}
	gs := groups(t)
	var parts []string
	top, best := 0, []string{}
	for _, g := range gs {
		parts = append(parts, fmt.Sprintf("%s %d", g.name, g.points))
		if g.points > top {
			top, best = g.points, []string{g.name}
		} else if g.points == top && g.points > 0 {
			best = append(best, g.name)
		}
	}
	points = strings.Join(parts, ", ")
	if unspent := t.Unspent; unspent != nil && *unspent > 0 {
		note := fmt.Sprintf("%d %s unspent", *unspent, plural(*unspent, "point", "points"))
		if points == "" {
			points = note
		} else {
			points += "; " + note
		}
	}
	switch {
	case top == 0:
		return "unknown (no talent points captured)", points
	case len(best) > 1:
		return fmt.Sprintf("unknown (%s tied at %d points)", strings.Join(best, ", "), top), points
	}
	return fmt.Sprintf("%s (%d points)", best[0], top), points
}

func gearLine(slot string, it character.EquippedItem) string {
	name := it.Name
	if name == "" {
		if it.ItemID != nil && *it.ItemID != 0 {
			name = fmt.Sprintf("item %d", *it.ItemID)
		} else {
			name = "name not captured"
		}
	}
	if it.ItemLevel != nil {
		name += fmt.Sprintf(", ilvl %d", *it.ItemLevel)
	}
	return fmt.Sprintf("- %s: %s", slot, name)
}

func gear(c character.Character) []string {
	if c.Equipment == nil {
		return []string{"- " + notCaptured}
	}
	var lines, empty []string
	known := map[string]bool{}
	for _, s := range Slots {
		known[s] = true
		if it, ok := c.Equipment[s]; ok {
			lines = append(lines, gearLine(s, it))
		} else {
			empty = append(empty, s)
		}
	}
	var other []string
	for s := range c.Equipment {
		if !known[s] {
			other = append(other, s)
		}
	}
	sort.Strings(other)
	for _, s := range other {
		lines = append(lines, gearLine(s, c.Equipment[s]))
	}
	if len(empty) > 0 {
		lines = append(lines, "- empty slots: "+strings.Join(empty, ", "))
	}
	return lines
}

func quests(c character.Character) []string {
	if !c.QuestsCaptured {
		return []string{"- " + notCaptured}
	}
	if len(c.Quests) == 0 {
		return []string{"- none in the quest log"}
	}
	var out []string
	for _, q := range c.Quests {
		state := "in progress"
		if q.Complete != nil && *q.Complete {
			state = "complete"
		} else if q.Failed != nil && *q.Failed {
			state = "failed"
		}
		title := q.Title
		if title == "" {
			title = fmt.Sprintf("quest %d", q.QuestID)
		}
		zone := ""
		if q.Zone != "" {
			zone = ", " + q.Zone
		}
		line := fmt.Sprintf("- %s (level %s%s) [%s]", title, intOr(q.Level, "unknown"), zone, state)
		if p := q.ObjectiveProgress(); p != "" && state == "in progress" {
			line += " — " + p
		}
		out = append(out, line)
	}
	return out
}

func professions(c character.Character) []string {
	if c.Professions == nil {
		return []string{"- " + notCaptured}
	}
	if len(c.Professions) == 0 {
		return []string{"- none captured"}
	}
	var out []string
	for _, p := range c.Professions {
		out = append(out, fmt.Sprintf("- %s: %s/%s", p.Name, intOr(p.Rank, "?"), intOr(p.Max, "?")))
	}
	return out
}

func progress(c character.Character) string {
	xp := "XP not captured"
	if c.XP != nil && c.XPMax != nil {
		xp = fmt.Sprintf("XP %d/%d", *c.XP, *c.XPMax)
	}
	rested := "rested XP not captured"
	if c.RestedXP != nil {
		rested = fmt.Sprintf("rested %d XP", *c.RestedXP)
	}
	return xp + ", " + rested
}

func stacks(items []character.ItemStack) string {
	total := 0
	for _, s := range items {
		total += s.Count
	}
	return fmt.Sprintf("%d %s, %d items", len(items), plural(len(items), "stack", "stacks"), total)
}

func storage(c character.Character) []string {
	bags := "- bags: " + notCaptured
	if c.BagsCaptured {
		bags = "- bags: " + stacks(c.Bags)
	}
	bank := "- " + bankNotCaptured
	if c.BankCaptured {
		bank = "- bank: " + stacks(c.Bank)
	}
	return []string{bags, bank}
}

// Character renders the brief of one character. client is the capture's client (the character's,
// else the file's).
func Character(c character.Character, client character.Client, now time.Time) string {
	specName, points := spec(c.Talents)
	lines := []string{
		"# " + displayName(c),
		"Level: " + intOr(c.Level, notCaptured),
		"Class: " + orNot(c.Class),
		"Race: " + orNot(c.Race),
		"Realm: " + orNot(c.Realm),
		"Progress: " + progress(c),
		"Money: " + FormatMoney(c.Money),
		"Spec: " + specName,
	}
	if points != "" {
		lines = append(lines, "Talent points: "+points)
	}
	lines = append(lines, "Equipped gear:")
	lines = append(lines, gear(c)...)
	lines = append(lines, "Professions:")
	lines = append(lines, professions(c)...)
	lines = append(lines, "Storage:")
	lines = append(lines, storage(c)...)
	lines = append(lines, "Current quests:")
	lines = append(lines, quests(c)...)
	if pos := c.LastKnownPosition(); pos != nil {
		lines = append(lines, "Last known position: "+pos.Label())
	}
	if fl := flightLine(c, now); fl != "" {
		lines = append(lines, fl)
	}
	cl := ClientLine(client)
	if cl == "" {
		cl = "client not captured"
	}
	lines = append(lines, "", "Client: "+cl)
	if c.CapturedAt == nil {
		lines = append(lines, "Source: ForeverScribe capture, time not captured. /reload in game to refresh.")
		return strings.Join(lines, "\n")
	}
	lines = append(lines, "Source: ForeverScribe capture at "+utc(*c.CapturedAt))
	if age := now.Unix() - *c.CapturedAt; time.Duration(age)*time.Second > StaleAfter {
		lines = append(lines, fmt.Sprintf("WARNING: this capture is %dh old; /reload in game to refresh.", age/3600))
	}
	return strings.Join(lines, "\n")
}

// TopGroups are the talent groups holding the most points (several when tied) and that count;
// none when no points are spent.
func TopGroups(t *character.Talents) (names []string, points int) {
	if t == nil {
		return nil, 0
	}
	for _, g := range groups(t) {
		switch {
		case g.points > points:
			names, points = []string{g.name}, g.points
		case g.points == points && g.points > 0:
			names = append(names, g.name)
		}
	}
	return names, points
}

// flightLine lists the flight masters the player opened, for travel hints; "" when none were seen.
func flightLine(c character.Character, now time.Time) string {
	fs := c.Seen.FlightsFor(c.Faction)
	if len(fs) == 0 {
		return ""
	}
	var parts []string
	for _, f := range fs {
		s := f.Name
		if z := c.ZoneOfMap(f.Map); z != "" {
			s += " (" + z + ")"
		}
		parts = append(parts, s)
	}
	return "Flight points you know (" + character.SeenByYou + ", " + character.SeenAgo(fs[0].T, now) + "): " + strings.Join(parts, "; ")
}

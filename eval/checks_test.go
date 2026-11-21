package eval

// The properties a question is judged by. Each one asks a real tool function for its answer
// (advice.WhatsNext, brief.Character, gear.Find, digest.Build: the functions the MCP tools call)
// and compares it with what the capture and the dataset say, worked out here independently: the
// oracle never reads the tool's own output to learn what to expect.

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/WoW-Companion/advisor/internal/advice"
	"github.com/WoW-Companion/advisor/internal/brief"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/digest"
	"github.com/WoW-Companion/advisor/internal/gear"
	"github.com/WoW-Companion/advisor/internal/lua"
	"github.com/WoW-Companion/advisor/internal/store"
)

type check struct {
	tool string
	run  func(e *env) error
}

var checks = map[string]check{
	"first_turn_in_first":         {toolWhatsNext, firstTurnInFirst},
	"no_grey_quest":               {toolWhatsNext, noGreyQuest},
	"no_too_hard_quest":           {toolWhatsNext, noTooHardQuest},
	"steps_have_sources":          {toolWhatsNext, stepsHaveSources},
	"talent_step_matches_unspent": {toolWhatsNext, talentStepMatchesUnspent},
	"failed_quest_last":           {toolWhatsNext, failedQuestLast},
	"unknown_level_flagged":       {toolWhatsNext, unknownLevelFlagged},
	"brief_states_identity":       {toolBrief, briefStatesIdentity},
	"brief_names_empty_slots":     {toolBrief, briefNamesEmptySlots},
	"brief_flags_bank_gap":        {toolBrief, briefFlagsBankGap},
	"brief_stale_warning":         {toolBrief, briefStaleWarning},
	"no_wrong_armor":              {toolUpgrades, noWrongArmor},
	"sources_or_unknown":          {toolUpgrades, sourcesOrUnknown},
	"within_equip_window":         {toolUpgrades, withinEquipWindow},
	"respects_class_lock":         {toolUpgrades, respectsClassLock},
	"only_upgrades_best_first":    {toolUpgrades, onlyUpgradesBestFirst},
	"states_basis":                {toolUpgrades, statesBasis},
	"weapons_fit_class":           {toolUpgrades, weaponsFitClass},
	"digest_passes_slot_rules":    {toolDigest, digestPassesSlotRules},
	"digest_caps":                 {toolDigest, digestCaps},
	"digest_mentions_unspent":     {toolDigest, digestMentionsUnspent},
	"digest_matches_whats_next":   {toolDigest, digestMatchesWhatsNext},
}

// ---- shared ----

func level(e *env) int { return *e.c.Level }

func unspent(e *env) int {
	if t := e.c.Talents; t != nil && t.Unspent != nil {
		return *t.Unspent
	}
	return 0
}

// row reads a dataset row; nil when the table or row is missing.
func row(e *env, table string, id int64) store.Row {
	r, err := e.ds.Get(table, id)
	if err != nil {
		return nil
	}
	return r
}

func num(r store.Row, col string) int {
	n, _ := r[col].(int64)
	return int(n)
}

// questLevel is the level the capture gives a quest, else the dataset; ok is false when neither knows.
func questLevel(e *env, q character.Quest) (int, bool) {
	if q.Level != nil {
		return *q.Level, true
	}
	if r := row(e, "quests", int64(q.QuestID)); r != nil {
		if n := num(r, "level"); n > 0 {
			return n, true
		}
	}
	return 0, false
}

func steps(e *env) []advice.Step { return advice.WhatsNext(e.c, e.ds) }

func isAbandon(s advice.Step) bool { return strings.HasPrefix(s.Title, advice.AbandonPrefix) }

func capturedQuest(e *env, id int) (character.Quest, bool) {
	for _, q := range e.c.Quests {
		if q.QuestID == id {
			return q, true
		}
	}
	return character.Quest{}, false
}

func lines(s string) []string { return strings.Split(s, "\n") }

func has(text, want string) bool {
	return strings.Contains(strings.ToLower(text), strings.ToLower(want))
}

// ---- whats_next ----

// The turn-ins come first, all of them, the zone with the most of them leading and the quests of
// unknown zone last; with no turn-ins, the list does not open with one.
func firstTurnInFirst(e *env) error {
	want := 0
	for _, q := range e.c.Quests {
		if q.Complete != nil && *q.Complete {
			want++
		}
	}
	ss := steps(e)
	got := 0
	for i, s := range ss {
		if s.Kind != advice.KindTurnIn {
			continue
		}
		if i != got {
			return fmt.Errorf("turn-in %q is step %d, behind a step that is not a turn-in", s.Title, i+1)
		}
		got++
	}
	if got != want {
		return fmt.Errorf("%d completed quests but %d turn-in steps", want, got)
	}
	perZone := map[string]int{}
	for _, s := range ss[:got] {
		perZone[s.Zone]++
	}
	for i, s := range ss[:got] {
		if i > 0 && s.Zone != "" && ss[i-1].Zone == "" {
			return fmt.Errorf("turn-in %q (zone %s) comes after a turn-in of unknown zone", s.Title, s.Zone)
		}
		if i > 0 && s.Zone != ss[i-1].Zone && s.Zone != "" && perZone[s.Zone] > perZone[ss[i-1].Zone] {
			return fmt.Errorf("zone %s has %d turn-ins but is listed after zone %s with %d", s.Zone, perZone[s.Zone], ss[i-1].Zone, perZone[ss[i-1].Zone])
		}
	}
	return nil
}

func openQuestSteps(e *env) []advice.Step {
	var out []advice.Step
	for _, s := range steps(e) {
		if s.Kind == advice.KindQuest && !isAbandon(s) {
			out = append(out, s)
		}
	}
	return out
}

// No open quest is outside the level window around the character (grey below, too hard above).
func questWindow(e *env, low bool) error {
	for _, s := range openQuestSteps(e) {
		q, ok := capturedQuest(e, s.QuestID)
		if !ok {
			return fmt.Errorf("step %q names quest %d, which the capture does not hold", s.Title, s.QuestID)
		}
		lv, known := questLevel(e, q)
		switch {
		case !known:
		case low && lv < level(e)-5:
			return fmt.Errorf("grey quest %q (level %d) is offered to a level %d character", s.Title, lv, level(e))
		case !low && lv > level(e)+3:
			return fmt.Errorf("too-hard quest %q (level %d) is offered to a level %d character", s.Title, lv, level(e))
		}
	}
	return nil
}

func noGreyQuest(e *env) error    { return questWindow(e, true) }
func noTooHardQuest(e *env) error { return questWindow(e, false) }

// Every step names where its facts come from, in the data and in the rendered text.
func stepsHaveSources(e *env) error {
	ss := steps(e)
	for _, s := range ss {
		if len(s.Sources) == 0 {
			return fmt.Errorf("step %q has no source", s.Title)
		}
		for _, src := range s.Sources {
			if !strings.HasPrefix(src, "capture: ") && !strings.HasPrefix(src, "dataset: ") {
				return fmt.Errorf("step %q has source %q, which is neither the capture nor the dataset", s.Title, src)
			}
		}
	}
	for _, l := range lines(advice.Render(e.c, ss, len(ss))) {
		if len(l) > 0 && l[0] >= '1' && l[0] <= '9' && !strings.Contains(l, "[source: ") {
			return fmt.Errorf("rendered line %q shows no source", l)
		}
	}
	return nil
}

// A talent step exists exactly when points are unspent, and the text says "unspent".
func talentStepMatchesUnspent(e *env) error {
	n, count := unspent(e), 0
	ss := steps(e)
	for _, s := range ss {
		if s.Kind == advice.KindTalent {
			count++
			if !strings.Contains(s.Title, fmt.Sprint(n)) {
				return fmt.Errorf("talent step %q does not name the %d unspent points", s.Title, n)
			}
		}
	}
	if (n > 0) != (count == 1) || count > 1 {
		return fmt.Errorf("%d unspent points but %d talent steps", n, count)
	}
	if text := advice.Render(e.c, ss, len(ss)); (n > 0) != has(text, "unspent") {
		return fmt.Errorf("%d unspent points but the answer's mention of them is %v", n, !(n > 0))
	}
	return nil
}

// Failed quests are each offered for abandoning, after everything else.
func failedQuestLast(e *env) error {
	want := 0
	for _, q := range e.c.Quests {
		if q.Failed != nil && *q.Failed && !(q.Complete != nil && *q.Complete) {
			want++
		}
	}
	ss := steps(e)
	got := 0
	for i, s := range ss {
		if isAbandon(s) {
			got++
			if i < len(ss)-want {
				return fmt.Errorf("abandon step %q is step %d of %d, ahead of other advice", s.Title, i+1, len(ss))
			}
		}
	}
	if got != want {
		return fmt.Errorf("%d failed quests but %d abandon steps", want, got)
	}
	return nil
}

// A quest whose level neither the capture nor the dataset holds is listed, flagged "quest level
// unknown", after the quests whose level is known; it is not dropped and not guessed.
func unknownLevelFlagged(e *env) error {
	want := 0
	for _, q := range e.c.Quests {
		_, known := questLevel(e, q)
		if !known && !(q.Complete != nil && *q.Complete) && !(q.Failed != nil && *q.Failed) {
			want++
		}
	}
	open := openQuestSteps(e)
	got, firstUnknown := 0, -1
	for i, s := range open {
		if s.Reason == "quest level unknown" {
			got++
			if firstUnknown < 0 {
				firstUnknown = i
			}
		} else if firstUnknown >= 0 {
			return fmt.Errorf("quest %q (level known) is listed after a quest of unknown level", s.Title)
		}
	}
	if got != want {
		return fmt.Errorf("%d quests of unknown level in the capture but %d flagged \"quest level unknown\"", want, got)
	}
	return nil
}

// ---- character_brief ----

func brieflines(e *env, now time.Time) string {
	return brief.Character(e.c, e.client, now)
}

// topGroup is the talent group with the most points, worked out from the capture; "" on a tie or none.
func topGroup(c character.Character) (string, int) {
	best, name, tied := 0, "", false
	if c.Talents == nil {
		return "", 0
	}
	for _, tr := range c.Talents.Trees {
		for _, g := range tr.Groups {
			switch {
			case g.Spent > best:
				best, name, tied = g.Spent, g.Name, false
			case g.Spent == best && best > 0:
				tied = true
			}
		}
	}
	if tied {
		return "", best
	}
	return name, best
}

// The brief states the level, class, spec and unspent points the capture holds.
func briefStatesIdentity(e *env) error {
	text := brieflines(e, e.now)
	for _, want := range []string{fmt.Sprintf("Level: %d", level(e)), "Class: " + e.c.Class, "# " + e.c.Name} {
		if !strings.Contains(text, want) {
			return fmt.Errorf("brief lacks %q", want)
		}
	}
	spec := "Spec: unknown"
	if g, pts := topGroup(e.c); g != "" {
		spec = fmt.Sprintf("Spec: %s (%d points)", g, pts)
	}
	if !strings.Contains(text, spec) {
		return fmt.Errorf("brief lacks %q", spec)
	}
	var points string
	for _, l := range lines(text) {
		if strings.HasPrefix(l, "Talent points:") {
			points = l
		}
	}
	if n := unspent(e); n > 0 && !strings.Contains(points, fmt.Sprintf("%d point", n)) || n == 0 && has(points, "unspent") {
		return fmt.Errorf("%d unspent points but the talent line reads %q", n, points)
	}
	return nil
}

// Every empty equipment slot is named, and no worn slot is called empty.
func briefNamesEmptySlots(e *env) error {
	var empty string
	for _, l := range lines(brieflines(e, e.now)) {
		if strings.HasPrefix(l, "- empty slots: ") {
			empty = strings.TrimPrefix(l, "- empty slots: ")
		}
	}
	listed := map[string]bool{}
	for _, s := range strings.Split(empty, ", ") {
		listed[s] = true
	}
	for _, slot := range brief.Slots {
		_, worn := e.c.Equipment[slot]
		if worn && listed[slot] {
			return fmt.Errorf("slot %s is worn but listed as empty", slot)
		}
		if !worn && !listed[slot] {
			return fmt.Errorf("slot %s is empty but not listed (line: %q)", slot, empty)
		}
	}
	return nil
}

// A bank that was never captured is said so, with what to do; a captured bank is counted.
func briefFlagsBankGap(e *env) error {
	text := brieflines(e, e.now)
	gap := has(text, "bank not captured yet: open your bank once")
	switch {
	case !e.c.BankCaptured && !gap:
		return fmt.Errorf("the bank was never captured but the brief does not say so")
	case e.c.BankCaptured && (gap || !strings.Contains(text, "- bank: ")):
		return fmt.Errorf("the bank was captured but the brief reads as if it was not")
	}
	return nil
}

// A capture older than an hour is flagged with how to refresh it; a fresh one is not.
func briefStaleWarning(e *env) error {
	if text := brieflines(e, e.now); strings.Contains(text, "WARNING") {
		return fmt.Errorf("a 10 minute old capture is called stale")
	}
	old := brieflines(e, e.now.Add(5*time.Hour))
	if !strings.Contains(old, "WARNING") || !strings.Contains(old, "/reload") {
		return fmt.Errorf("a 5 hour old capture is not flagged with a /reload hint")
	}
	return nil
}

// ---- upgrade_finder ----

// upgrades runs the search the way the tool does: the spec is the one top talent group, if any.
func upgrades(e *env) (gear.Result, error) {
	group := ""
	if names, _ := brief.TopGroups(e.c.Talents); len(names) == 1 {
		group = names[0]
	}
	return gear.Find(e.c, e.ds, group, gear.Options{})
}

// each calls f on every candidate, with its dataset row.
func each(e *env, f func(slot string, c gear.Candidate, r store.Row) error) error {
	res, err := upgrades(e)
	if err != nil {
		return err
	}
	for slot, cands := range res.Slots {
		for _, c := range cands {
			r := row(e, "items", c.Item.ID)
			if r == nil {
				return fmt.Errorf("%s: candidate %d is not in the dataset", slot, c.Item.ID)
			}
			if err := f(slot, c, r); err != nil {
				return fmt.Errorf("%s: %q: %w", slot, c.Item.Name, err)
			}
		}
	}
	return nil
}

var armorSlots = map[string]bool{"head": true, "shoulder": true, "chest": true, "waist": true, "legs": true, "feet": true, "wrist": true, "hands": true}

// armorOf is the armor type a class wears (the item subclass): cloth 1, leather 2.
var armorOf = map[string]int{"PRIEST": 1, "ROGUE": 2}

var armorName = map[int]string{1: "cloth", 2: "leather", 3: "mail", 4: "plate"}

// Armor in the body slots is of the class's own type: no cloth to a Rogue, no leather to a Priest.
func noWrongArmor(e *env) error {
	return each(e, func(slot string, c gear.Candidate, r store.Row) error {
		if !armorSlots[slot] {
			return nil
		}
		if got := num(r, "subclass_id"); num(r, "class_id") != 4 || got != armorOf[e.c.Class] {
			return fmt.Errorf("%s armor offered to a %s for %s", armorName[got], e.c.Class, slot)
		}
		return nil
	})
}

// Every candidate says where it comes from, or says "source unknown" where the dataset has none.
func sourcesOrUnknown(e *env) error {
	return each(e, func(slot string, c gear.Candidate, r store.Row) error {
		rows, err := e.ds.Filter("loot_sources", store.Range{Column: "item_id", Min: c.Item.ID, Max: c.Item.ID})
		if err != nil {
			return err
		}
		text := c.Describe()
		switch {
		case len(rows) > 0 && (len(c.Sources) == 0 || !strings.Contains(text, "from ")):
			return fmt.Errorf("the dataset knows %d sources but the answer says %q", len(rows), text)
		case len(rows) == 0 && !strings.Contains(text, "source unknown"):
			return fmt.Errorf("no source in the dataset and the answer does not say \"source unknown\": %q", text)
		}
		return nil
	})
}

// No item requires more than 3 levels above the character.
func withinEquipWindow(e *env) error {
	return each(e, func(slot string, c gear.Candidate, r store.Row) error {
		if req := num(r, "required_level"); req > level(e)+3 {
			return fmt.Errorf("requires level %d, character is %d", req, level(e))
		}
		return nil
	})
}

// An item locked to another class is never offered.
func respectsClassLock(e *env) error {
	id := map[string]int{"PRIEST": 5, "ROGUE": 4}[e.c.Class]
	return each(e, func(slot string, c gear.Candidate, r store.Row) error {
		if mask := num(r, "allowable_class_mask"); mask > 0 && mask&(1<<(id-1)) == 0 {
			return fmt.Errorf("locked to class mask %d, character is %s", mask, e.c.Class)
		}
		return nil
	})
}

// Candidates beat what is worn (or fill an empty slot), best first, and none is poor quality.
func onlyUpgradesBestFirst(e *env) error {
	res, err := upgrades(e)
	if err != nil {
		return err
	}
	for slot, cands := range res.Slots {
		for i, c := range cands {
			if c.Delta <= 0 {
				return fmt.Errorf("%s: %q is no upgrade (delta %.1f)", slot, c.Item.Name, c.Delta)
			}
			if c.Empty != (e.c.Equipment[slot].ItemID == nil && e.c.Equipment[slot].ItemLevel == nil) {
				return fmt.Errorf("%s: %q is marked empty=%v against the capture", slot, c.Item.Name, c.Empty)
			}
			if c.Item.Quality <= 0 {
				return fmt.Errorf("%s: %q is poor quality", slot, c.Item.Name)
			}
			if i > 0 && c.Delta > cands[i-1].Delta {
				return fmt.Errorf("%s: %q (+%.1f) is listed after %q (+%.1f)", slot, c.Item.Name, c.Delta, cands[i-1].Item.Name, cands[i-1].Delta)
			}
		}
	}
	return nil
}

// The answer states how it ranked: by stat types when it could, by item level (with the reason)
// when not, and each line agrees.
func statesBasis(e *env) error {
	res, err := upgrades(e)
	if err != nil {
		return err
	}
	text := gear.Render(e.c, res)
	if res.ItemLevelOnly {
		if res.Reason == "" || !strings.Contains(text, "item level only") || !strings.Contains(text, res.Reason) {
			return fmt.Errorf("item-level ranking is not explained in %q", lines(text)[1])
		}
	} else if !strings.Contains(text, "stat types") {
		return fmt.Errorf("stat ranking does not say it uses stat types: %q", lines(text)[1])
	}
	for slot, cands := range res.Slots {
		for _, c := range cands {
			if !strings.Contains(c.Describe(), "by "+c.Basis) {
				return fmt.Errorf("%s: %q does not state its basis %q", slot, c.Item.Name, c.Basis)
			}
		}
	}
	return nil
}

// weaponsOf are the weapon subclasses a class may use: axe1H 0, bow 2, gun 3, mace1H 4, sword1H 7,
// staff 10, fist 13, dagger 15, thrown 16, crossbow 18, wand 19.
var weaponsOf = map[string][]int{"PRIEST": {15, 4, 10, 19}, "ROGUE": {0, 2, 3, 4, 7, 13, 15, 16, 18}}

// Weapons are of a subclass the class can use, and no shield is offered.
func weaponsFitClass(e *env) error {
	return each(e, func(slot string, c gear.Candidate, r store.Row) error {
		if num(r, "class_id") == 4 && (slot == "mainhand" || slot == "offhand" || slot == "ranged") {
			return fmt.Errorf("armor offered for %s", slot)
		}
		if num(r, "class_id") != 2 {
			return nil
		}
		for _, sub := range weaponsOf[e.c.Class] {
			if num(r, "subclass_id") == sub {
				return nil
			}
		}
		return fmt.Errorf("weapon subclass %d is not usable by a %s", num(r, "subclass_id"), e.c.Class)
	})
}

// ---- update_digest ----

func buildDigest(e *env) digest.Digest {
	return digest.Build(e.c, e.ds, digest.Options{Now: e.now, Client: e.client})
}

var (
	luaString = regexp.MustCompile(`"(?:[^"\\\n]|\\[\s\S])*"`)
	luaStart  = regexp.MustCompile(`^\s*WoWCompanionDigest\s*=\s*\{`)
	luaWord   = regexp.MustCompile(`([A-Za-z_]\w*)\s*(=?=?)`)
)

// slotRules is the slot addon's data-only rule (addon tests/slotcheck.lua): with comments dropped
// and each string replaced by 0, one `WoWCompanionDigest = {...}` assignment, no parentheses, no
// bare identifier as a value, no `..`, `:` or `#`.
func slotRules(src string) error {
	var lines []string
	for _, l := range strings.Split(src, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(l), "--") {
			lines = append(lines, l)
		}
	}
	code := luaString.ReplaceAllString(strings.Join(lines, "\n"), "0")
	if !luaStart.MatchString(code) || strings.Count(code, "WoWCompanionDigest") != 1 {
		return fmt.Errorf("not a single WoWCompanionDigest = {...} assignment")
	}
	if strings.ContainsAny(code, "():#") || strings.Contains(code, "..") {
		return fmt.Errorf("call, parenthesis or operator in the digest")
	}
	for _, w := range luaWord.FindAllStringSubmatch(strings.TrimPrefix(strings.TrimSpace(code), "WoWCompanionDigest"), -1) {
		if w[1] != "true" && w[1] != "false" && w[1] != "nil" && w[2] != "=" {
			return fmt.Errorf("identifier %q is not a key", w[1])
		}
	}
	return nil
}

// The digest is the data-only file the slot addon accepts, and parses back as the same character.
func digestPassesSlotRules(e *env) error {
	src := buildDigest(e).Lua()
	if err := slotRules(src); err != nil {
		return err
	}
	vars, err := lua.Parse(src)
	if err != nil {
		return fmt.Errorf("does not parse: %w", err)
	}
	d, _ := vars["WoWCompanionDigest"].(lua.Table)
	if d["character"] != e.c.Name || d["schema"] != int64(digest.Schema) && d["schema"] != float64(digest.Schema) {
		return fmt.Errorf("round trip gave character %v, schema %v", d["character"], d["schema"])
	}
	return nil
}

// Sections and lines stay within the panel's caps.
func digestCaps(e *env) error {
	max := map[string]int{digest.NextQuests: digest.MaxQuests, digest.Upgrades: digest.MaxUpgrades, digest.KeepMaterials: digest.MaxMaterials, digest.Notes: digest.MaxNotes}
	for _, s := range buildDigest(e).Sections {
		limit, known := max[s.ID]
		if !known || len(s.Lines) == 0 || len(s.Lines) > limit {
			return fmt.Errorf("section %q has %d lines (cap %d, known=%v)", s.ID, len(s.Lines), limit, known)
		}
		for _, l := range s.Lines {
			if n := len([]rune(l.Text)); n == 0 || n > digest.MaxText {
				return fmt.Errorf("section %q line %q is %d characters (cap %d)", s.ID, l.Text, n, digest.MaxText)
			}
		}
	}
	return nil
}

func section(d digest.Digest, id string) []digest.Line {
	for _, s := range d.Sections {
		if s.ID == id {
			return s.Lines
		}
	}
	return nil
}

// Unspent talent points are in the notes exactly when the capture has them.
func digestMentionsUnspent(e *env) error {
	mentioned := false
	for _, l := range section(buildDigest(e), digest.Notes) {
		mentioned = mentioned || has(l.Text, "unspent")
	}
	if n := unspent(e); (n > 0) != mentioned {
		return fmt.Errorf("%d unspent points but the digest notes mention them: %v", n, mentioned)
	}
	return nil
}

// next_quests is whats_next's turn-ins and open quests in its order (abandon steps are left out),
// up to the cap, and a note counts the failed quests.
func digestMatchesWhatsNext(e *env) error {
	var want []int
	for _, s := range steps(e) {
		if (s.Kind == advice.KindTurnIn || s.Kind == advice.KindQuest) && !isAbandon(s) {
			want = append(want, s.QuestID)
		}
	}
	if len(want) > digest.MaxQuests {
		want = want[:digest.MaxQuests]
	}
	d := buildDigest(e)
	got := section(d, digest.NextQuests)
	if len(got) != len(want) {
		return fmt.Errorf("digest lists %d quests, whats_next gives %d", len(got), len(want))
	}
	for i, l := range got {
		if l.Quest != want[i] {
			return fmt.Errorf("quest line %d is quest %d, whats_next has quest %d there", i+1, l.Quest, want[i])
		}
	}
	failed := 0
	for _, q := range e.c.Quests {
		if q.Failed != nil && *q.Failed {
			failed++
		}
	}
	noted := false
	for _, l := range section(d, digest.Notes) {
		noted = noted || has(l.Text, "failed")
	}
	if (failed > 0) != noted {
		return fmt.Errorf("%d failed quests but the notes mention them: %v", failed, noted)
	}
	return nil
}

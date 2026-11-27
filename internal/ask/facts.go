package ask

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/WoW-Companion/advisor/internal/advice"
	"github.com/WoW-Companion/advisor/internal/brief"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/digest"
	"github.com/WoW-Companion/advisor/internal/gear"
	"github.com/WoW-Companion/advisor/internal/levelup"
	"github.com/WoW-Companion/advisor/internal/professions"
	"github.com/WoW-Companion/advisor/internal/profile"
	"github.com/WoW-Companion/advisor/internal/questie"
	"github.com/WoW-Companion/advisor/internal/route"
	"github.com/WoW-Companion/advisor/internal/store"
	"github.com/WoW-Companion/advisor/internal/talents"
	"github.com/WoW-Companion/advisor/internal/unlock"
)

// Pre-fetched facts: the character's brief and next steps are put in the prompt, so most questions
// need no tool call (each tool round trip resends the whole context). The tools stay available.
const (
	// FactsBudget is the most characters of pre-fetched facts in a prompt (about 1.5k tokens).
	FactsBudget = 6000
	// FactsSteps is how many next steps are pre-fetched.
	FactsSteps = 5
	// FactsPlanSteps is how many route plan stops are pre-fetched (the plan's count covers the rest).
	FactsPlanSteps = 8
)

// Facts is the pre-fetched facts block for q's character, or "" when the character cannot be
// chosen or the capture cannot be read (the agent then uses its tools, as before). The block is cut
// at FactsBudget characters.
func Facts(cfg Config, q Question) string {
	cc, ok := load(cfg, q.Character)
	if !ok {
		return ""
	}
	c, f := cc.c, cc.f
	if cc.ds == nil {
		return NoDataFacts + "\n\n" + subjectFacts(cfg, cc, q) + capRunes(brief.Character(c, f.ClientOf(c), time.Now()), FactsBudget)
	}
	all := advice.WhatsNextWith(c, cc.ds, cc.qx)
	var plan route.Plan
	if cc.qx != nil {
		plan = route.BuildWith(c, cc.qx, route.OptionsFor(cfg.SavedVariables, cc.ds))
		all = advice.ByQuestOrder(all, func(s advice.Step) int { return s.QuestID }, route.QuestOrder(plan))
	}
	next := advice.Render(c, advice.Limit(all, FactsSteps), len(all))
	if cc.qx != nil {
		next += "\n\n" + route.Render(plan, FactsPlanSteps)
	}
	var giver string
	if g, ok := digest.FindGiver(c, cc.qx, digest.PlanQuests(all), digest.PlanNPCs(all)); ok {
		giver = "Quest giver with something for the character now (not in the plan): " + g.Text() + "."
	}
	return capRunes(subjectFacts(cfg, cc, q)+brief.Character(c, f.ClientOf(c), time.Now())+"\n\n"+styleLine(c)+join(seasonalFacts(c, cc.qx))+join(bagFacts(c, cc.ds))+join(protectedFacts(cc, q.Text))+join(WeaponFacts(c, cc.ds, q.Text))+join(professionPlanFacts(cc, q.Text))+join(talentFacts(c, cc.ds))+join(talentBuildFacts(c, cc.ds, q.Text))+join(StandingFacts(c, q.Text))+join(LootRollFacts(c, cc.ds, q.Text))+join(ItemTooltipFacts(cc.ds, q.Text))+join(DisenchantFacts(cc.ds, q.Text))+join(GatherFacts(cc.ds, q.Text))+join(RecipeSkillFacts(cc.ds, q.Text))+join(SetFacts(cc.ds, q.Text))+join(CreatureFacts(cc.ds, q.Text))+join(LimitFacts(q.Text))+join(NamedQuestFacts(cc.ds, q.Text))+join(TrainerWhereFacts(c, q.Text))+join(SummonStoneFacts(cc.ds, q.Text))+join(ProficiencyFacts(c, cc.ds, q.Text))+"\n\n"+next+join(trainFacts(cc))+join(giver), FactsBudget)
}

func join(s string) string {
	if s == "" {
		return ""
	}
	return "\n\n" + s
}

// loaded is a character read for a question.
type loaded struct {
	c  character.Character
	f  character.File
	ds store.DataStore
	qx *questie.Index
	sv string // the capture's path, for the route options and the quest XP table
}

// load reads the capture and picks the question's character, with its saved play style; ok is false
// when the capture cannot be read or the character chosen (the agent then uses its tools).
func load(cfg Config, key string) (loaded, bool) {
	if cfg.SavedVariables == "" {
		return loaded{}, false
	}
	f, err := character.LoadFile(cfg.SavedVariables)
	if err != nil {
		return loaded{}, false
	}
	c, err := brief.Choose(f, key)
	if err != nil {
		return loaded{}, false
	}
	l := loaded{c: character.AttachTotals(unlock.AddonSV(cfg.SavedVariables), profile.Attach(profile.Path(), c)), f: f, sv: cfg.SavedVariables}
	if cfg.Dataset != "" {
		l.ds = store.NewDir(cfg.Dataset)
	}
	l.qx = questie.ForSavedVariables(cfg.SavedVariables)
	return l, true
}

// digestOf is the digest the player's panel shows for the loaded character.
func (l loaded) digest() digest.Digest {
	opt := digest.Options{Now: time.Now(), Client: l.f.ClientOf(l.c), Questie: l.qx}
	if l.qx != nil {
		p := route.BuildWith(l.c, l.qx, route.OptionsFor(l.sv, l.ds))
		opt.Unlocks, opt.Order = route.UnlockNotes(p), route.QuestOrder(p)
	}
	return digest.Build(l.c, l.ds, opt)
}

// ResolveRef is q's ref: the one it carries, else that of the current suggested question of the
// character that the text matches exactly (case and punctuation aside); "" when neither.
func ResolveRef(cfg Config, q Question) string {
	if q.Ref != "" {
		return q.Ref
	}
	cc, ok := load(cfg, q.Character)
	if !ok {
		return ""
	}
	return cc.digest().RefFor(q.Text)
}

// GroundingRules tell the agent how to use a subject's facts: they are the answer's first source, ids
// are the key for lookups (a quest or item can be missing from the dataset by name), and a miss is
// never reported to the player as "no match".
const GroundingRules = "How to answer: answer from these facts first. Look anything up by id (quest id, NPC id, item id), " +
	"never by name alone. A lookup that finds nothing is not an answer: never tell the player there was \"no match\" or that " +
	"you cannot tell what this is. When the facts are thin, say plainly what they do show (who gives it, where, the level, " +
	"whether it is a class quest, why it is listed) and what it usually means. Never invent specifics such as a reward, " +
	"an ability name or a location that is not in the facts. Name NPCs, coordinates, prices and skill levels only from the facts; when one is missing, say what kind of place to look (\"your class trainer in Undercity\")."

// JustReadRule is added whenever a question arrives with a ref: the player clicked the entry and has
// read its card, so the answer is only what the card does not say, in the short answer shape.
const JustReadRule = "The player has just read this entry's card, so don't repeat its facts. Answer only the new part of their question, in the short shape above."

// subjectFacts is the block that expands q's ref into the entry's facts and its place in the plan, with
// a closing blank line; "" without a ref or when the entry is gone. The entry's own facts (title,
// kind, ids, giver, zone, coordinates, level, class flag, why, details, plan step) are always put in.
func subjectFacts(cfg Config, cc loaded, q Question) string {
	d := cc.digest()
	ref := q.Ref
	if ref == "" {
		ref = d.RefFor(q.Text)
	}
	sub, ok := d.Subject(ref)
	if !ok {
		return ""
	}
	var b strings.Builder
	b.WriteString("The player's short question is about this entry of their panel; answer it with this context.\nSubject: " + sub.Text + "\n")
	b.WriteString(entryFacts(cfg, cc, sub))
	for _, d := range sub.Details {
		b.WriteString("- " + d + "\n")
	}
	if sub.Place > 0 {
		fmt.Fprintf(&b, "Place in the plan: step %d of %d next steps.\n", sub.Place, sub.Of)
	} else {
		b.WriteString("Place in the plan: not one of the plan's steps.\n")
	}
	b.WriteString(GroundingRules + "\n")
	b.WriteString(JustReadRule + "\n")
	if sub.Line != nil && len(sub.Line.Asks) > 0 && digest.SameAsk(q.Text, sub.Line.Asks[0]) {
		if r := PathStepsRule(sub.Line.PathKind); r != "" {
			b.WriteString(r + "\n")
		}
	}
	return b.String() + "\n"
}

// entryFacts lists the structured facts of a subject, one "Label: value" line each; nothing unknown.
func entryFacts(cfg Config, cc loaded, sub digest.Subject) string {
	var b strings.Builder
	put := func(k, v string) {
		if v != "" {
			b.WriteString(k + ": " + v + "\n")
		}
	}
	put("Ref", sub.Ref)
	put("Kind", sub.Kind)
	l := sub.Line
	if l == nil {
		return b.String()
	}
	put("Title", l.Title)
	if l.Quest > 0 {
		put("Quest id", fmt.Sprint(l.Quest))
	}
	if l.Item > 0 {
		put("Item id", fmt.Sprint(l.Item))
	}
	if strings.HasPrefix(sub.Ref, "npc:") {
		put("NPC id", strings.TrimPrefix(sub.Ref, "npc:"))
	}
	put("Giver or turn-in NPC", l.NPC)
	put("Zone", l.Zone)
	if l.Level > 0 {
		put("Level", fmt.Sprint(l.Level))
	}
	if l.Class {
		put("Class quest", "yes (this character's own class)")
	}
	if l.Elite {
		put("Elite or group quest", "yes")
	}
	put("Why it is listed", l.Why)
	if l.Quest > 0 {
		for _, q := range cc.c.Quests {
			if q.QuestID == l.Quest {
				put("Quest status", q.Readiness()+" (never say turn in unless it is ready to turn in)")
			}
		}
	}
	for _, o := range l.Objectives {
		put("Objective", o.Text)
	}
	if l.Quest > 0 && cc.qx != nil && cc.ds != nil {
		p := route.BuildWith(cc.c, cc.qx, route.OptionsFor(cfg.SavedVariables, cc.ds))
		for _, s := range p.Steps {
			if s.QuestID == l.Quest {
				put("Plan step", fmt.Sprintf("%d. %s%s", s.N, s.Label, planCoords(s)))
				break
			}
		}
	}
	return b.String()
}

// styleLine is one line of the saved play style, or the reminder that none is saved.
func styleLine(c character.Character) string {
	if c.Profile == nil {
		return "Play style: not saved yet (ask once before talent advice)."
	}
	role, focus := c.Profile.Role, c.Profile.Focus
	if role == "" {
		role = "role not set"
	}
	if focus == "" {
		focus = "focus not set"
	}
	s := "Play style: " + role + ", " + focus
	if c.Profile.Spec != "" {
		s += ", " + c.Profile.Spec
	}
	return s + "."
}

func capRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return strings.TrimRight(string([]rune(s)[:n]), " \n") + "\n[cut; use the tools for the rest]"
}

// NoDataFacts stands in for the next steps when no game dataset has downloaded.
const NoDataFacts = "Game data not downloaded yet; see Setup. Tell the player this, in those words, rather than guessing quests, zones or gear."

// PromptWithFacts is PromptFor plus the pre-fetched facts, when there are any.
func PromptWithFacts(q Question, length, facts string) string {
	p := PromptFor(q, length)
	if facts == "" {
		return p
	}
	i := strings.LastIndex(p, "\n\nQuestion: ")
	return p[:i] + "\n\nThe character's current facts are below, already fetched: answer from them and call a tool only for something they lack.\n---\n" + facts + "\n---" + p[i:]
}

// Prompt is what the agent is asked for q under cfg: PromptFor with the pre-fetched facts.
func (c Config) Prompt(q Question) string {
	p, _ := c.PromptGround(q)
	return p
}

// PromptGround is Prompt and the grounding of a know-how question (the zero Grounding for any other).
func (c Config) PromptGround(q Question) (string, Grounding) {
	p := PromptWithFacts(q, c.Options.Length, Facts(c, q))
	g := GroundFor(c, q)
	if TalentQuestion(q.Text) {
		if i := strings.Index(p, " "+FormatRule); i >= 0 {
			p = p[:i] + talentRule() + p[i+1:]
		}
		return p, Grounding{}
	}
	if WeaponQuestion(q.Text) {
		// Weapon rules differ from Classic: the facts are the source, never general knowledge, and no
		// "general WoW knowledge" footer is put under the answer.
		if i := strings.Index(p, " "+FormatRule); i >= 0 {
			p = p[:i] + weaponRule + p[i+1:]
		}
		return p, Grounding{}
	}
	if !g.KnowHow {
		return p, g
	}
	p = strings.Replace(p, "; do not guess.", ". Facts about the character come only from the tools.", 1)
	rule := KnowHowRule
	if q.Icy {
		rule += IcyRule
	}
	if i := strings.Index(p, " "+FormatRule); i >= 0 {
		p = p[:i] + rule + p[i+1:]
	}
	if g.Pack != "" {
		if i := strings.LastIndex(p, "\n\nQuestion: "); i >= 0 {
			p = p[:i] + "\n\nFacts about what the question asks (ours; they win over your memory):\n---\n" + strings.TrimRight(g.Pack, "\n") + "\n---" + p[i:]
		}
	}
	return p, g
}

// PathStepsRule is added when the player asks an upgrade's first question, the one that follows the
// path the card suggests: they want the steps, not the card's one line again. It keeps the short
// answer shape (one answer line, then at most 3 steps) and takes its facts from upgrade_paths.
func PathStepsRule(kind string) string {
	const shape = " Call upgrade_paths with the item id for the facts. Give one answer line, then at most 3 steps as short bullets; do not repeat the card's one line."
	switch kind {
	case "craft":
		return "The player asks how to make this item. Steps, in order: where the trainer is if the recipe is not known yet (the nearest one for the player's faction, with zone and coordinates, from where_to_learn); the materials with the counts they have and still need, and where to get each missing one (mining nodes, a vendor or the Auction House); where to craft it (a forge and anvil for blacksmithing)." + shape
	case "quest":
		return "The player asks how to get this item from its quest. Steps: where to pick up the quest, what it asks, where to hand it in and choose the reward." + shape
	case "auction":
		return "The player asks how to find this on the Auction House. Steps: where the Auction House is, the Browse filters (slot, level range, quality), and what to look for in the results. Never name a price." + shape
	case "dungeon":
		return "The player asks how to get this item from a dungeon boss. Steps: where the dungeon entrance is, how to reach the boss, and how often it drops." + shape
	case "creature":
		return "The player asks where to farm this item. Steps: which creature carries it, where it lives, and how often it drops." + shape
	}
	return ""
}

// planRunes is the most characters of a profession plan put in the facts.
const planRunes = 2600

var levelingWords = []string{"level", "leveling", "levelling", "skill", "train", "raise", "power", "grind", "get to", "from 1", "up my", "how do i get", "how to get", "plan", "recipes", "what should i make", "what do i make"}

// ProfessionAsked is professionAsked for the benchmark: the profession of the character that a leveling question names, "" for any other question.
func ProfessionAsked(c character.Character, text string) string { return professionAsked(c, text) }

// professionAsked is the profession of the character that a leveling question names ("how do I level
// tailoring?"), "" when the question is not about leveling one of their professions.
func professionAsked(c character.Character, text string) string {
	if i := strings.LastIndex(text, currentMarker); i >= 0 {
		text = text[i+len(currentMarker):]
	}
	t := " " + spokenForm(text) + " "
	if !hasAny(t, levelingWords) && !getToSkill.MatchString(t) {
		return ""
	}
	for _, p := range c.Professions {
		n := strings.ToLower(p.Name)
		if strings.Contains(t, " "+n+" ") {
			return p.Name
		}
		for _, a := range professionAliases[n] {
			if strings.Contains(t, " "+a+" ") {
				return p.Name
			}
		}
	}
	return ""
}

// getToSkill is "get cooking to 150": a way of asking to raise a skill that names the target, which the leveling words do not.
var getToSkill = regexp.MustCompile(`\b(get|raise|bring|take|push)\b.{0,30}\bto \d{2,3}\b`)

// professionAliases are the other words a player uses for a secondary profession in a leveling question.
var professionAliases = map[string][]string{
	"first aid": {"firstaid", "bandaging", "bandage skill", "bandages skill"},
	"cooking":   {"cook", "cooking skill"},
	"fishing":   {"angling"},
}

// spokenForm is a question lower-cased with every character that is no letter, digit or apostrophe turned into a space, so "first-aid?" and "fishing!" read as the words they are
// and a profession name is found before any punctuation or a line break.
func spokenForm(text string) string {
	b := []rune(strings.ToLower(text))
	for i, r := range b {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '\'' {
			b[i] = ' '
		}
	}
	return strings.ReplaceAll(strings.Join(strings.Fields(string(b)), " ")+" ", "'s ", " ")
}

// professionPlanFacts is the profession_plan result for the profession a leveling question is about
// (bands, recipes, materials, trainer), so the answer is the real plan and not general tips; "" for any
// other question.
func professionPlanFacts(cc loaded, text string) string {
	prof := professionAsked(cc.c, text)
	if prof == "" || cc.ds == nil {
		return ""
	}
	if route := professions.RouteFacts(cc.c, cc.ds, cc.qx, prof); route != "" {
		return "The character HAS " + prof + " (it is one of their professions); never say they lack it. Skill-up paths come from these route facts, never from general knowledge; use the counts and trainers as written. " + route
	}
	pa := professions.ComputePathWith(cc.c, cc.ds, cc.qx, prof, 0, 1)
	body := professions.RenderPath(pa)
	if strings.TrimSpace(body) == "" {
		return ""
	}
	return "No fetched efficient route exists for " + prof + " in the game data, so say the path below is estimated. Profession trainers are never class trainers.\nThe character HAS " + prof + " (it is one of their professions); never say they lack it. Their " + prof + " leveling plan from profession_plan; answer from it, not from general tips:\n" + capRunes(body, planRunes)
}

// talentFacts states unspent class talent points, for "what next" answers to mention first; "" when there are none.
func talentFacts(c character.Character, ds store.DataStore) string {
	if c.Talents == nil || c.Talents.Unspent == nil || *c.Talents.Unspent <= 0 {
		return ""
	}
	return "Unspent talent points (mention this FIRST in any \"what next\" answer; spending them takes no travel): " + advice.TalentTitle(c, ds, *c.Talents.Unspent) + "."
}

// talentBuildFacts is the Icy Veins leveling build's state for a talent question: the progress line, the guide's
// key talents with the reason for each, and the off-build note; "" for any other question or with no checked build.
func talentBuildFacts(c character.Character, ds store.DataStore, question string) string {
	if ds == nil || !TalentQuestion(question) {
		return ""
	}
	p := talents.Plan(c, ds, talents.Options{})
	if p.Missing != "" {
		if p.NoBuild {
			return "Talent build: " + p.Missing + " Mention only the unspent points; name no specific pick."
		}
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Talent build (Icy Veins WoW Forever leveling, %s): %s.", p.Spec, talents.Progress(p))
	for _, pr := range p.Priorities {
		fmt.Fprintf(&b, " Key talent %d: %s (%s).", pr.Rank, pr.Name, pr.Reason)
	}
	if n := talents.OffBuildNote(p); n != "" {
		b.WriteString(" " + n + " (say so once; do not suggest a respec).")
	}
	return b.String()
}

// trainFacts is the nearest place to train class spells, when the character has spells to train, ; "" otherwise.
func trainFacts(cc loaded) string {
	c := cc.c
	if cc.ds == nil || c.Level == nil {
		return ""
	}
	from := *c.Level
	if c.KnownSpells != nil {
		from = 1
	}
	if len(levelup.Spells(c, cc.ds, from, *c.Level)) == 0 {
		return ""
	}
	p, ok := levelup.TrainAt(c, cc.qx)
	if !ok {
		return ""
	}
	return p.Line(c.Class) + "."
}

// bagFacts lists the items already in the bags that beat what is worn, for gear answers to put first;
// "" when there are none.
func bagFacts(c character.Character, ds store.DataStore) string {
	group := ""
	if names, _ := brief.TopGroups(c.Talents); len(names) == 1 {
		group = names[0]
	}
	var lines []string
	for _, u := range gear.BagUpgrades(c, ds, group) {
		lines = append(lines, "- "+u.Line())
	}
	if len(lines) == 0 {
		return ""
	}
	return "Equip these now (already in the character's bags, free; for any gear or upgrade question list them first, before quest rewards, crafts, vendor and Auction House):\n" + strings.Join(lines, "\n")
}

// planCoords is a plan stop's coordinates, or nothing for a stop known only by its zone.
func planCoords(s route.Stop) string {
	if s.ZoneOnly {
		return ""
	}
	return fmt.Sprintf(" (%.0f, %.0f)", s.X, s.Y)
}

// seasonalFacts says which holiday events run today and the player's setting. With the setting on, the
// event's quests are already in the plan; with it off the AI may explain the event and offer to add it
// (seasonal_events), never listing quests as steps.
func seasonalFacts(c character.Character, qx *questie.Index) string {
	now := digest.ClockOf(c, time.Now())
	run := questie.RunningEvents(now)
	if len(run) == 0 {
		return ""
	}
	set := questie.Seasonal(c.Seasonal)
	var b strings.Builder
	b.WriteString("\nHoliday event quests: " + set.Describe() + " (off is the default; change it only with the seasonal_events tool when the player clearly asks).\n")
	who := advice.WhoOf(c)
	for _, e := range run {
		fmt.Fprintf(&b, "Running today: %s (until %s).", e.Name, e.Until())
		if set.Wants(e.Key) && qx != nil && c.Level != nil {
			for _, q := range qx.EventQuests(e.Key, who) {
				fmt.Fprintf(&b, "\n- %s (level %d), starts with %s", q.Name, q.Level, q.Start)
			}
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

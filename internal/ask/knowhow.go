package ask

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/WoW-Companion/advisor/internal/advice"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/dungeons"
	"github.com/WoW-Companion/advisor/internal/levelup"
	"github.com/WoW-Companion/advisor/internal/professions"
	"github.com/WoW-Companion/advisor/internal/store"
)

// Kind is the kind of question: about the player's own state, or about how the game works.
type Kind int

const (
	// KindState is personal ("what should I do", "my gear", "can I make X"): answered strictly from
	// the player's data.
	KindState Kind = iota
	// KindKnowHow is about how the game works (strategy, tactics, boss mechanics, rotations, how a
	// profession works, lore, thinking ahead): the model may use its own WoW knowledge, framed by our facts.
	KindKnowHow
)

// Topic is what a know-how question is about.
type Topic string

const (
	TopicNone       Topic = ""
	TopicDungeon    Topic = "dungeon"
	TopicClass      Topic = "class"
	TopicProfession Topic = "profession"
	TopicLevelUp    Topic = "levelup"
	TopicSession    Topic = "session"
	TopicTriage     Topic = "triage"
)

// currentMarker precedes the player's own question when a chat history is prepended.
const currentMarker = "Current question: "

// Class is a classified question.
type Class struct {
	Kind  Kind
	Topic Topic
	// Subject is the dungeon, class or profession named (or the saved character's class); "" for none.
	Subject string
}

var knowHowWords = []string{"strategy", "strategies", "tactic", "tips", "tip for", "how do i beat", "how to beat", "how do i kill",
	"how to kill", "how do i handle", "mechanic", "rotation", "lore", "thinking ahead", "how does", "how do i play", "how should i play",
	"how do you play", "general strategy", "what's it like", "what is it like", "is it hard", "boss", "how do i tank", "how to tank",
	"how to level", "how do i level", "how do i work", "how does a", "how do i run", "how do i survive"}

var stateWords = []string{"what should i do", "what do i do", "what next", "what's next", "whats next", "my gear", "my bags", "my bank",
	"my quest", "my talent", "my stats", "my inventory", "can i make", "can i craft", "should i equip", "should i wear", "an upgrade", "upgrades",
	"which upgrade", "next quest", "what quest", "where do i get", "how do i get", "where can i get", "what do i need for"}

var professionNames = []string{"alchemy", "blacksmithing", "enchanting", "engineering", "herbalism", "leatherworking", "mining", "skinning",
	"tailoring", "cooking", "first aid", "fishing"}

var classNames = []string{"warrior", "paladin", "hunter", "rogue", "priest", "shaman", "mage", "warlock", "druid"}

// Classify sorts a question by keywords and its subject. A know-how word wins; else a personal phrase
// makes it a state question; when unsure it is know-how (the facts still override what the model
// remembers). ds and the character are optional: they let a dungeon name, or "my class", be found.
func Classify(text string, ds store.DataStore, c *character.Character) Class {
	t := " " + strings.ToLower(text) + " "
	var cl Class
	switch {
	case hasAny(t, knowHowWords):
		cl.Kind = KindKnowHow
	case hasAny(t, stateWords):
		cl.Kind = KindState
	default:
		cl.Kind = KindKnowHow
	}
	if subj, ok := sessionSubject(t); ok {
		cl.Kind, cl.Topic, cl.Subject = KindKnowHow, TopicSession, subj
		return cl
	}
	if subj, ok := triageSubject(t, c); ok {
		cl.Kind, cl.Topic, cl.Subject = KindKnowHow, TopicTriage, subj
		return cl
	}
	if subj, ok := levelUpSubject(t); ok {
		cl.Kind, cl.Topic, cl.Subject = KindKnowHow, TopicLevelUp, subj
		return cl
	}
	if d := dungeons.Mentioned(ds, nil, text); d != "" {
		cl.Topic, cl.Subject = TopicDungeon, d
		if cl.Kind == KindState && !hasAny(t, []string{"my ", "can i ", "what should i do", "where do i get", "how do i get"}) {
			cl.Kind = KindKnowHow
		}
		return cl
	}
	for _, p := range professionNames {
		if strings.Contains(t, " "+p+" ") || strings.Contains(t, " "+p+"?") || strings.Contains(t, " "+p+".") {
			cl.Topic, cl.Subject = TopicProfession, p
			return cl
		}
	}
	for _, k := range classNames {
		if strings.Contains(t, " "+k) {
			cl.Topic, cl.Subject = TopicClass, k
			return cl
		}
	}
	if c != nil && c.Class != "" && hasAny(t, []string{"rotation", "how do i play", "how should i play", "my class", "my spec", "how do i tank", "how to tank"}) {
		cl.Topic, cl.Subject = TopicClass, strings.ToLower(c.Class)
	}
	return cl
}

var atLevel = regexp.MustCompile(`\b(?:level|lvl)\s*(\d{1,2})\b`)
var nextLevels = []string{"next level", "next lvl", "level up", "levelling up", "leveling up", "level after", "new at level", "get at level", "learn at level", "train at level", "next two levels", "next 2 levels"}

// levelUpSubject reports a "what's new at my next level?" question: Subject is "next", "next2" (the
// next two levels) or the level asked about. It must ask for what the level brings, not how to level.
func levelUpSubject(t string) (string, bool) {
	asks := hasAny(t, []string{"what's new", "whats new", "what is new", "what do i get", "what will i get", "what do i learn", "what will i learn",
		"what can i train", "what spells", "what abilities", "what new", "what comes", "what does", "what's coming", "whats coming", "what to train", "worth training"})
	if !asks {
		return "", false
	}
	if hasAny(t, []string{"next two levels", "next 2 levels", "two levels"}) {
		return "next2", true
	}
	if m := atLevel.FindStringSubmatch(t); m != nil {
		return m[1], true
	}
	if hasAny(t, nextLevels) {
		return "next", true
	}
	return "", false
}

// numberWords are the spelled-out numbers a time can be given in ("forty five minutes", "three hours").
var numberWords = map[string]float64{"one": 1, "two": 2, "three": 3, "four": 4, "five": 5, "six": 6, "seven": 7, "eight": 8, "nine": 9, "ten": 10,
	"eleven": 11, "twelve": 12, "thirteen": 13, "fourteen": 14, "fifteen": 15, "sixteen": 16, "seventeen": 17, "eighteen": 18, "nineteen": 19,
	"twenty": 20, "thirty": 30, "forty": 40, "fifty": 50, "sixty": 60, "seventy": 70, "eighty": 80, "ninety": 90}

const (
	unitWord = `one|two|three|four|five|six|seven|eight|nine`
	tensWord = `(?:twenty|thirty|forty|fifty|sixty|seventy|eighty|ninety)(?:[ -](?:` + unitWord + `))?`
	teenWord = `ten|eleven|twelve|thirteen|fourteen|fifteen|sixteen|seventeen|eighteen|nineteen`
	spelled  = tensWord + `|` + teenWord + `|` + unitWord
	andHalf  = `\s+and\s+a\s+half`
)

var minutesRe = regexp.MustCompile(`\b(\d{1,3}|` + spelled + `|an?)\s*(?:min|mins|minutes?)\b`)
var hoursRe = regexp.MustCompile(`\b(\d{1,2}(?:\.\d+)?|` + spelled + `|a couple of|a couple|half an?|half|an?)(` + andHalf + `)?\s*(?:hours?|hrs?)\b(` + andHalf + `)?`)
var clockRe = regexp.MustCompile(`\b(\d{1,2})\s*h\s*(\d{2})\b`)
var timeIntent = []string{"i've got", "i have", "ive got", "i got", "i only have", "only have", "have about", "got about", "to play", "to spare", "quick session", "short session", "play for", "session"}

// quantity is the number a time phrase starts with: digits, a spelled number, "a" or "an" (one), "half" or "a couple of".
func quantity(w string) float64 {
	switch w {
	case "a", "an":
		return 1
	case "half", "half a", "half an":
		return 0.5
	case "a couple", "a couple of":
		return 2
	}
	if n, err := strconv.ParseFloat(w, 64); err == nil {
		return n
	}
	var total float64
	for _, part := range strings.FieldsFunc(w, func(r rune) bool { return r == ' ' || r == '-' }) {
		total += numberWords[part]
	}
	return total
}

// statedMinutes is the time the player says they have, in minutes: every hours and minutes figure of the
// question added ("2 hours 30 minutes", "an hour and a half", "1.5 hours", "1h30"), 0 when it names none.
func statedMinutes(t string) int {
	total := 0.0
	for _, m := range hoursRe.FindAllStringSubmatch(t, -1) {
		h := quantity(m[1])
		if m[2] != "" || m[3] != "" {
			h += 0.5
		}
		total += h * 60
	}
	for _, m := range minutesRe.FindAllStringSubmatch(t, -1) {
		total += quantity(m[1])
	}
	if total == 0 {
		for _, m := range clockRe.FindAllStringSubmatch(t, -1) {
			h, _ := strconv.Atoi(m[1])
			n, _ := strconv.Atoi(m[2])
			total += float64(h*60 + n)
		}
	}
	return int(math.Round(total))
}

// sessionSubject reports an "I've got 30 minutes" question; the subject is "min:30" (minutes, 5 to
// 240). It needs a time and a sign the player is saying how long they can play.
func sessionSubject(t string) (string, bool) {
	mins := statedMinutes(t)
	if mins == 0 && hasAny(t, []string{"quick session", "short session"}) {
		mins = 30
	}
	if mins == 0 || !hasAny(t, timeIntent) && !hasAny(t, []string{"what should i do", "what can i do", "what can i get done", "plan"}) {
		return "", false
	}
	return "min:" + strconv.Itoa(min(max(mins, 5), 240)), true
}

// triageSubject reports a quest-triage question: "abandon" for what to drop or a full log, or
// "quest:<title>" for "is <quest> worth it?" about a quest in the capture's log.
func triageSubject(t string, c *character.Character) (string, bool) {
	if hasAny(t, []string{"abandon", "log is full", "log full", "quest log is full", "what should i drop", "what to drop", "which quests should i drop", "clear my log", "clean up my log", "free up quest", "free up my log"}) {
		return "abandon", true
	}
	if !hasAny(t, []string{"worth it", "worth doing", "worth my time", "should i skip", "should i do", "should i bother", "worth finishing"}) {
		return "", false
	}
	if c != nil {
		best := ""
		for _, q := range c.Quests {
			if n := strings.ToLower(q.Title); len(n) > len(best) && strings.Contains(t, n) {
				best = n
			}
		}
		if best != "" {
			return "quest:" + best, true
		}
	}
	if hasAny(t, []string{"quest"}) {
		return "quest:", true
	}
	return "", false
}

func hasAny(t string, words []string) bool {
	for _, w := range words {
		if strings.Contains(t, w) {
			return true
		}
	}
	return false
}

// Grounding is what the facts of one know-how answer covered; the footer is built from it.
type Grounding struct {
	Topic    Topic
	Subject  string
	Levels   bool // a level range
	Bosses   bool // bosses or creatures
	Quests   bool // dungeon quests with givers
	Drops    bool // drops for the character
	Spells   bool // the class's spells at the character's level
	Notes    bool // the profession notes
	IcyURL   string
	Pack     string // the facts handed to the model; "" when there are none
	KnowHow  bool
	Browsing bool // the answer was checked against Icy Veins
	Seen     bool // some of the facts are ones the player saw in game (trainers, vendors, gathering, quest givers)
}

// Footer is the small source line under a know-how answer, built from what grounded it, never from
// the model: "Tactics: general WoW knowledge · Levels, bosses, quests and drops: your game". Each part
// shows only when present; "" when neither is.
func (g Grounding) Footer() string {
	var parts []string
	if g.KnowHow {
		parts = append(parts, "Tactics: general WoW knowledge")
	}
	var have []string
	for _, p := range []struct {
		ok bool
		s  string
	}{{g.Levels, "levels"}, {g.Bosses, "bosses"}, {g.Quests, "quests"}, {g.Drops, "drops"}, {g.Spells, "spells"}, {g.Notes, "profession notes"}} {
		if p.ok {
			have = append(have, p.s)
		}
	}
	if len(have) > 0 {
		s := strings.Join(have[:len(have)-1], ", ")
		if s != "" {
			s += " and "
		}
		s += have[len(have)-1]
		parts = append(parts, strings.ToUpper(s[:1])+s[1:]+": your game")
	}
	if g.Seen {
		parts = append(parts, "Trainers, vendors, gathering and quest givers: "+character.SeenByYou)
	}
	if g.IcyURL != "" {
		parts = append(parts, "Icy Veins")
	}
	return strings.Join(parts, " · ")
}

// Offer reports whether the answer may carry the Check Icy Veins button: a know-how answer about a
// dungeon, class or profession that has not already been checked.
func (g Grounding) Offer() bool {
	return g.KnowHow && !g.Browsing && g.Topic != TopicNone && g.Topic != TopicLevelUp && g.Topic != TopicSession && g.Topic != TopicTriage
}

// GroundFor classifies q and, for a know-how question, builds the grounding pack. State questions
// return the zero Grounding.
func GroundFor(cfg Config, q Question) Grounding {
	cc, ok := load(cfg, q.Character)
	if !ok {
		return Grounding{}
	}
	return ground(cc, q)
}

// ground is GroundFor for a loaded character.
func ground(cc loaded, q Question) Grounding {
	text := q.Text
	if i := strings.LastIndex(text, currentMarker); i >= 0 {
		text = text[i+len(currentMarker):]
	}
	cl := Classify(text, cc.ds, &cc.c)
	if cl.Kind != KindKnowHow {
		return Grounding{}
	}
	if cl.Topic == TopicNone && text != q.Text { // a follow-up: the topic may be in what came before
		cl.Topic, cl.Subject = Classify(q.Text, cc.ds, &cc.c).Topic, Classify(q.Text, cc.ds, &cc.c).Subject
	}
	g := Grounding{KnowHow: true, Topic: cl.Topic, Subject: cl.Subject, Browsing: q.Icy}
	var b strings.Builder
	switch cl.Topic {
	case TopicDungeon:
		p, togo, err := dungeons.Named(cc.c, cc.ds, cc.qx, cl.Subject)
		if err == nil && p != nil {
			g.Levels = p.MinLevel != nil && p.MaxLevel != nil
			g.Bosses = len(p.Bosses) > 0 || len(p.Inside) > 0
			g.Quests = len(p.Pickups) > 0
			g.Drops = len(p.Drops) > 0
			b.WriteString(dungeons.RenderNamed(cc.c, *p, togo))
			if len(p.Bosses) > 0 || len(p.Inside) > 0 {
				b.WriteString("Attach tactics only to the bosses and creatures listed above.\n")
			}
		}
	case TopicClass:
		if s := classFacts(cc.c, cc.ds); s != "" {
			g.Spells = true
			b.WriteString(s)
		}
	case TopicLevelUp:
		if c := cc.c; c.Level != nil {
			from, to := *c.Level+1, *c.Level+1
			switch n, err := strconv.Atoi(cl.Subject); {
			case cl.Subject == "next2":
				to++
			case err == nil && n > *c.Level:
				to = min(n, *c.Level+10)
			case err == nil:
				from, to = n, n
			}
			s, spells, quests := levelup.Facts(c, cc.ds, cc.qx, from, to)
			g.Spells, g.Quests = spells, quests
			b.WriteString(s)
		}
	case TopicSession, TopicTriage:
		if s, ok := sessionFacts(cc, q, cl); ok {
			g.Quests = true
			b.WriteString(s)
		}
	case TopicProfession:
		if s := professionFacts(cc.ds, cl.Subject); s != "" {
			g.Notes = true
			b.WriteString(s)
		}
	}
	g.Pack = b.String()
	g.Seen = strings.Contains(g.Pack, character.SeenByYou)
	return g
}

// classFacts lists the class's spells up to the character's level (class_spells), with the saved role.
func classFacts(c character.Character, ds store.DataStore) string {
	if ds == nil || c.Level == nil {
		return ""
	}
	cid := int64(advice.WhoOf(c).ClassID)
	if cid == 0 {
		return ""
	}
	l := levelup.Learned(c, ds, *c.Level)
	if len(l) == 0 {
		return ""
	}
	var names []string
	for _, s := range l {
		names = append(names, fmt.Sprintf("%s (%d)", s.Name, s.Level))
	}
	return fmt.Sprintf("Spells a %s of level %d has learned, with the level each was learned at: %s.\nOnly talk about abilities in this list or ones you are sure exist in WoW Forever.\n",
		strings.ToLower(c.Class), *c.Level, strings.Join(names, ", "))
}

// professionFacts is the curated notes for one profession: the ranks and their level gates, pairings
// and the special case.
func professionFacts(ds store.DataStore, prof string) string {
	n := professions.LoadNotes(ds)
	var b strings.Builder
	for _, t := range n.Tiers {
		fmt.Fprintf(&b, "%s: skill %d-%d, from character level %d.\n", t.Name, t.SkillMin, t.SkillCap, t.MinLevel)
	}
	has := b.Len() > 0 && ds != nil
	if s := n.Special[prof]; s != "" {
		b.WriteString(s + "\n")
		has = true
	}
	for _, p := range n.Pairings {
		if strings.EqualFold(p.A, prof) || strings.EqualFold(p.B, prof) {
			fmt.Fprintf(&b, "Pairs with %s: %s\n", other(p, prof), p.Text)
			has = true
		}
	}
	for _, f := range n.Feeders {
		if strings.EqualFold(f.From, prof) || strings.EqualFold(f.To, prof) {
			b.WriteString(f.Text + "\n")
			has = true
		}
	}
	if !has {
		return ""
	}
	return b.String()
}

func other(p professions.Pairing, prof string) string {
	if strings.EqualFold(p.A, prof) {
		return p.B
	}
	return p.A
}

// KnowHowRule is added to the prompt of a know-how question. It replaces the "answer from the facts
// only" stance for the how-the-game-works part: the model uses its own WoW knowledge, framed by the
// facts below it. The source line is added by the server, so the body never says where facts came from.
const KnowHowRule = " This is a question about how the game works, not about the player's own state: answer it with your own WoW knowledge, " +
	"framed by the facts given below it. Our facts always win: use their levels, entrances, bosses, quests and spells where they differ from what you remember. " +
	"Attach tactics only to the bosses and creatures listed in the facts, and leave out anything you are not sure exists in WoW Forever. " +
	"Tailor it to the saved class, role and spec (a Fury warrior is a damage dealer by default: threat, and the interrupts they have at their level). " +
	"When the player is under the level, give one line, \"You're N; it opens around M.\", then the full plan for when they are ready. " +
	"Write the answer itself plainly and never comment on where the information comes from: a source line is added under it for you. "

// IcyRule is added when the player pressed Check Icy Veins: the same question, refined from the guide.
const IcyRule = " The player asked you to check Icy Veins: read the matching WoW Forever guide at icy-veins.com/wow-forever/ with your web or fetch tool, " +
	"refine the tactics from it, and keep our facts over anything it says that differs. Never copy its text; say it in your own words. Fetch nothing else. "

package ask

import (
	"fmt"
	"regexp"
	"sort"
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
	if g.IcyURL != "" {
		parts = append(parts, "Icy Veins")
	}
	return strings.Join(parts, " · ")
}

// Offer reports whether the answer may carry the Check Icy Veins button: a know-how answer about a
// dungeon, class or profession that has not already been checked.
func (g Grounding) Offer() bool {
	return g.KnowHow && !g.Browsing && g.Topic != TopicNone && g.Topic != TopicLevelUp
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
	case TopicProfession:
		if s := professionFacts(cc.ds, cl.Subject); s != "" {
			g.Notes = true
			b.WriteString(s)
		}
	}
	g.Pack = b.String()
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
	rows, err := ds.Filter("class_spells", store.Range{Column: "class_id", Min: cid, Max: cid}, store.Range{Column: "level", Min: int64(1), Max: int64(*c.Level)})
	if err != nil || len(rows) == 0 {
		return ""
	}
	type sp struct {
		lv   int64
		name string
	}
	var l []sp
	seen := map[string]bool{}
	for _, r := range rows {
		n, _ := r["name"].(string)
		lv, _ := r["level"].(int64)
		sup, _ := r["superseded_spell_id"].(int64)
		if n == "" || seen[n] || sup != 0 {
			continue
		}
		seen[n] = true
		l = append(l, sp{lv, n})
	}
	sort.SliceStable(l, func(i, j int) bool { return l[i].lv < l[j].lv })
	var names []string
	for _, s := range l {
		names = append(names, fmt.Sprintf("%s (%d)", s.name, s.lv))
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

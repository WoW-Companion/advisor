package digest

import (
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/gear"
	"github.com/WoW-Companion/advisor/internal/paths"
)

// card is one entry as the player reads it: its kind, the words that name its subject, and the
// fields that already state facts.
type card struct {
	id, kind string
	asks     []string
	subject  []string // text, title, slot, giver, zone, the worn item: names, free to repeat
	facts    []string // why, how and details: what the card already says
}

var (
	wordRe     = regexp.MustCompile(`[A-Za-z']{4,}`)
	quotedRe   = regexp.MustCompile(`'[^']+'`)
	pronounRe  = regexp.MustCompile(`(?i)\b(this|these|those|that|it|its|they|them|he|she)\b`)
	plainWords = map[string]bool{"what": true, "which": true, "should": true, "would": true, "does": true, "will": true, "with": true, "from": true,
		"much": true, "more": true, "then": true, "than": true, "have": true, "need": true, "there": true, "while": true, "soon": true, "worth": true,
		"best": true, "slot": true, "play": true, "style": true, "take": true, "next": true, "after": true, "come": true, "comes": true, "hand": true,
		"stay": true, "pick": true, "lead": true, "leads": true, "nearby": true, "anything": true, "other": true, "before": true, "gold": true,
		"change": true, "changes": true, "once": true, "ready": true, "bring": true, "wait": true, "group": true, "item": true, "items": true,
		"instead": true, "keep": true, "craft": true, "silver": true, "time": true, "save": true, "alongside": true, "finish": true, "trip": true,
		"stat": true, "stats": true, "worn": true, "costs": true, "matters": true, "matter": true, "make": true, "making": true, "pick'": true, "i'm": true}
)

// stem is the first five letters of a lower-case word, so "suffix" meets "suffixes".
func stem(w string) string {
	w = strings.ToLower(w)
	if len(w) > 5 {
		return w[:5]
	}
	return w
}

func stems(ss ...string) map[string]bool {
	out := map[string]bool{}
	for _, s := range ss {
		for _, w := range wordRe.FindAllString(s, -1) {
			out[stem(w)] = true
		}
	}
	return out
}

// shopping is the cloak shopping list of the mission's example, with a second suffix.
func cloakShopping(c character.Character) Line {
	sh := paths.Shopping{Slot: "back", Type: "cloak", MinLevel: 7, MaxLevel: 10, Quality: "green",
		Suffixes: []string{"of Strength (Strength)", "of the Bear (Stamina, Strength)"}, Examples: []string{"Rugged Cloak"},
		Desc: "any green cloak, level 7–10, 'of Strength'", Steps: []string{"Open the Auction House and search for a cloak."}, Best: gear.Candidate{Empty: false}}
	return shoppingLine(c, "back", sh)
}

func cardsOf(t *testing.T) map[string][]card {
	t.Helper()
	out := map[string][]card{}
	for name, d := range map[string]Digest{
		"scout":  scoutDigest(t, true),
		"synth":  Build(synthChar(), synthStore(t), synthOpt),
		"rogue":  Build(synthRogue(), synthStore(t), synthOpt),
		"nodata": Build(warrior(10), nil, synthOpt),
	} {
		var cs []card
		for _, s := range d.Sections {
			for _, l := range s.Lines {
				kind := l.Kind + "/" + l.PathKind + "/" + strings.SplitN(l.Ref, ":", 2)[0]
				if l.Ref == talentsRef {
					kind = "talent"
				}
				cs = append(cs, card{l.Ref, kind, l.Asks, []string{l.Text, l.Title, l.Slot, l.NPC, l.Zone, l.Replaces},
					append([]string{l.Why, l.How}, l.Details...)})
			}
		}
		for _, l := range d.Dungeons {
			cs = append(cs, card{l.Ref, "dungeon", l.Asks, []string{l.Name, l.Line}, append([]string{l.Why}, l.Details...)})
		}
		for _, l := range d.Talents {
			cs = append(cs, card{l.Ref, "talent", l.Asks, []string{l.Line}, append([]string{l.Why}, l.Details...)})
		}
		if d.Stats != nil {
			cs = append(cs, card{"stats", "stats", d.Stats.Asks, append([]string{d.Stats.Profile}, d.Stats.Insights...), nil})
		}
		out[name] = cs
	}
	c := warrior(10)
	c.Equipment = map[string]character.EquippedItem{"back": {Name: "Flimsy Chain Cloak"}}
	shop := cloakShopping(c)
	out["cloak"] = []card{{shop.Ref, "shopping", shop.Asks, []string{shop.Text, shop.Slot, shop.Replaces},
		append([]string{shop.Why, shop.How}, shop.Details...)}}
	return out
}

func TestAsksFollowTheClickedEntry(t *testing.T) {
	byAsk := map[string]string{}
	for name, cs := range cardsOf(t) {
		for _, cd := range cs {
			id := name + "/" + cd.id
			subject := stems(cd.subject...)
			facts := stems(cd.facts...)
			for i, a := range cd.asks {
				// An upgrade's first ask restates its path's own words on purpose ("from Near Wolf in
				// Wolf Den", "on the Auction House"): it is the path's action.
				pathAsk := i == 0 && (cd.kind == "shopping" || strings.Count(cd.kind, "/") == 2 && strings.Split(cd.kind, "/")[1] != "")
				names := false
				for w := range stems(a) {
					names = names || subject[w]
				}
				if !names && !strings.ContainsAny(a, "'") {
					t.Errorf("%s: ask %q names no subject of its entry", id, a)
				}
				if m := pronounRe.FindString(a); m != "" {
					t.Errorf("%s: ask %q holds %q with no antecedent", id, a, m)
				}
				bare := quotedRe.ReplaceAllString(a, "")
				for _, w := range wordRe.FindAllString(bare, -1) {
					if s := stem(w); !pathAsk && facts[s] && !subject[s] && !plainWords[strings.ToLower(w)] {
						t.Errorf("%s: ask %q asks again what the card states (%q)", id, a, w)
					}
				}
				if k, seen := byAsk[a]; seen && k != cd.kind {
					t.Errorf("%s: ask %q is shared by kinds %q and %q", id, a, k, cd.kind)
				}
				byAsk[a] = cd.kind
				if utf8.RuneCountInString(a) > MaxAskLength {
					t.Errorf("%s: ask %q is too long", id, a)
				}
			}
		}
	}
}

// The golden asks: the card's facts beside what is asked next.
func TestGoldenAsks(t *testing.T) {
	c := warrior(10)
	c.Equipment = map[string]character.EquippedItem{"back": {Name: "Flimsy Chain Cloak"}}
	cases := []struct {
		name string
		got  []string
		want []string
	}{
		{"Auction House cloak", cloakShopping(c).Asks, []string{
			"How do I find a green cloak 'of Strength' on the Auction House?",
			"For my cloak slot, is 'of Strength' my best pick for now?",
			"Would 'of the Bear' do as well as 'of Strength' for my cloak?"}},
		{"craft", upgradeAsks("Gemmed Copper Gauntlets", "Hands", "Fury warrior", paths.Path{Kind: paths.Craft}, []string{"Copper Bar"}), []string{
			"How do I make Gemmed Copper Gauntlets?",
			"Is it worth making Gemmed Copper Gauntlets now, or should I save the Copper Bar?",
			"How much does Gemmed Copper Gauntlets help my Fury warrior?"}},
		{"quest", upgradeAsks("Red Sash", "Waist", "", paths.Path{Kind: paths.Quest, Quest: "Sash Wanted"}, nil)[:1], []string{
			"How do I get Red Sash from Sash Wanted?"}},
		{"auction item", upgradeAsks("Red Sash", "Waist", "", paths.Path{Kind: paths.Auction}, nil)[:1], []string{
			"How do I find Red Sash on the Auction House?"}},
		{"dungeon", upgradeAsks("Red Sash", "Waist", "", paths.Path{Kind: paths.Dungeon, Boss: "Edwin", Dungeon: "Deadmines"}, nil)[:1], []string{
			"How do I get Red Sash from Edwin in Deadmines?"}},
		{"creature", upgradeAsks("Red Sash", "Waist", "", paths.Path{Kind: paths.Creature, Creature: "Defias Thug"}, nil)[:1], []string{
			"Where do I farm Red Sash from Defias Thug?"}},
		{"pick-up", questAsks(Line{Kind: KindPickup, Title: "Wind Hollow Essence"}), []string{
			"What does Wind Hollow Essence lead to?",
			"Should I take Wind Hollow Essence before the other quests nearby?"}},
		{"turn-in", questAsks(Line{Kind: KindTurnIn, Title: "Unwelcome Spirits"}), []string{
			"What comes after I hand in Unwelcome Spirits?",
			"Should I finish anything nearby before handing in Unwelcome Spirits?"}},
		{"talents", talentAsks(1, "Penance"), []string{
			"Why does the Penance talent suit my play style?",
			"What changes in play once I take the Penance talent?"}},
		{"giver", giverAsks("Endaria Mistgaze", "Lantern in the Mist"), []string{
			"Is Lantern in the Mist worth the trip to Endaria Mistgaze now?",
			"What does Lantern in the Mist lead to?"}},
	}
	for _, tc := range cases {
		if strings.Join(tc.got, "|") != strings.Join(tc.want, "|") {
			t.Errorf("%s asks:\n got %q\nwant %q", tc.name, tc.got, tc.want)
		}
	}
}

func TestFirstAskFollowsPathAndStaysWithinThree(t *testing.T) {
	for _, p := range []paths.Path{{Kind: paths.Craft}, {Kind: paths.Quest, Quest: "Q"}, {Kind: paths.Auction}, {Kind: paths.Dungeon, Boss: "B", Dungeon: "D"}, {Kind: paths.Creature, Creature: "C"}} {
		asks := upgradeAsks("Red Sash", "Waist", "Fury warrior", p, []string{"Copper Bar"})
		if len(asks) > MaxAsks || !strings.Contains(asks[0], "Red Sash") || strings.HasPrefix(asks[0], "How much") {
			t.Errorf("%s: %q", p.Kind, asks)
		}
	}
}

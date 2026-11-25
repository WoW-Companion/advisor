package ask

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/parquet-go/parquet-go"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/provider"
	"github.com/WoW-Companion/advisor/internal/questie"
	"github.com/WoW-Companion/advisor/internal/store"
)

type instRow struct {
	MapID      int32  `parquet:"map_id"`
	Name       string `parquet:"name"`
	Type       string `parquet:"instance_type"`
	MaxPlayers *int16 `parquet:"max_players,optional"`
	MinLevel   *int16 `parquet:"min_level,optional"`
	MaxLevel   *int16 `parquet:"max_level,optional"`
	Source     string `parquet:"level_source"`
	Zone       string `parquet:"entrance_zone_name"`
}
type bossRow struct {
	MapID      int32  `parquet:"map_id"`
	Name       string `parquet:"name"`
	CreatureID *int32 `parquet:"creature_id,optional"`
}
type iqRow struct {
	MapID   int32  `parquet:"map_id"`
	QuestID int64  `parquet:"quest_id"`
	Reason  string `parquet:"reason"`
}
type questRow struct {
	ID      int64  `parquet:"id"`
	Name    string `parquet:"name"`
	Level   int32  `parquet:"level"`
	Giver   string `parquet:"giver_name"`
	Faction string `parquet:"faction"`
}

func table[T any](t *testing.T, rows []T) store.SizedReaderAt {
	t.Helper()
	var buf bytes.Buffer
	if err := parquet.Write(&buf, rows); err != nil {
		t.Fatal(err)
	}
	return store.SizedReaderAt{ReaderAt: bytes.NewReader(buf.Bytes()), Size: int64(buf.Len())}
}

func i16(n int16) *int16 { return &n }
func i32(n int32) *int32 { return &n }

// khLoaded is a level 10 Horde Fury warrior with a synthetic dataset and QuestieDB: Test Chasm (13-18,
// entrance in Orgrimmar, two bosses), and a map row with a level but no entrance and no group size.
func khLoaded(t *testing.T) loaded {
	lvl := 10
	c := character.Character{Name: "Testwar", Class: "WARRIOR", Faction: "Horde", Level: &lvl, Profile: &character.Profile{Role: "dps", Spec: "Fury"}}
	ds := store.New(store.ReaderSource{
		"instances": table(t, []instRow{
			{1, "Test Chasm", "dungeon", i16(5), i16(13), i16(18), "curated", "Orgrimmar"},
			{2, "Ruins of Nowhere", "dungeon", nil, i16(27), i16(30), "client", ""},
		}),
		"instance_bosses": table(t, []bossRow{{1, "Boss Alpha", i32(100)}, {1, "Boss Beta", i32(101)}}),
		"instance_quests": table(t, []iqRow{{1, 950, "category"}}),
		"quests":          table(t, []questRow{{950, "Searching for the Test", 14, "Giver Gruk", "Horde"}}),
	})
	qx := &questie.Index{
		Areas:    map[int]questie.Area{901: {Zone: "Orgrimmar"}},
		Npcs:     map[int]questie.Entity{100: {Name: "Boss Alpha", Rank: 3}, 101: {Name: "Boss Beta", Rank: 3}, 102: {Name: "Test Shaman", Rank: 1}},
		Dungeons: []questie.Dungeon{{Area: 910, Name: "Test Chasm", Parent: 901, Entrance: true, Creatures: []int{100, 101, 102}}},
	}
	return loaded{c: c, ds: ds, qx: qx}
}

func TestClassify(t *testing.T) {
	cc := khLoaded(t)
	for _, tc := range []struct {
		text    string
		kind    Kind
		topic   Topic
		subject string
	}{
		{"What general strategy should I use for Test Chasm?", KindKnowHow, TopicDungeon, "Test Chasm"},
		{"I get that, but I'm curious… I'm thinking ahead.", KindKnowHow, TopicNone, ""},
		{"how do I beat the bosses in test chasm", KindKnowHow, TopicDungeon, "Test Chasm"},
		{"What should I do next?", KindState, TopicNone, ""},
		{"Can I make Copper Bars?", KindState, TopicNone, ""},
		{"Is my gear good enough?", KindState, TopicNone, ""},
		{"What's the best rotation for a warrior?", KindKnowHow, TopicClass, "warrior"},
		{"What is the best rotation?", KindKnowHow, TopicClass, "warrior"},
		{"How does mining work and when do I get smelting?", KindKnowHow, TopicProfession, "mining"},
		{"Tell me about the lore of Orgrimmar", KindKnowHow, TopicNone, ""},
	} {
		got := Classify(tc.text, cc.ds, &cc.c)
		if got.Kind != tc.kind || got.Topic != tc.topic || got.Subject != tc.subject {
			t.Errorf("%q = %+v, want kind %d topic %q subject %q", tc.text, got, tc.kind, tc.topic, tc.subject)
		}
	}
}

func TestGroundingPackHasBossesQuestsAndNoLevelGate(t *testing.T) {
	cc := khLoaded(t)
	g := ground(cc, Question{Text: "What general strategy should I use for Test Chasm?"})
	if !g.KnowHow || g.Topic != TopicDungeon {
		t.Fatalf("grounding %+v", g)
	}
	for _, want := range []string{"You're 10; it opens around 13.", "about levels 13–18", "Orgrimmar", "Boss Alpha", "Boss Beta", "Test Shaman", "Searching for the Test", "Giver Gruk", "Attach tactics only"} {
		if !strings.Contains(g.Pack, want) {
			t.Errorf("pack lacks %q:\n%s", want, g.Pack)
		}
	}
	if !g.Levels || !g.Bosses || !g.Quests {
		t.Errorf("flags %+v", g)
	}
	if g := ground(cc, Question{Text: "What should I do next?"}); g.KnowHow || g.Pack != "" {
		t.Errorf("a state question was grounded for know-how: %+v", g)
	}
}

func TestFooterIsBuiltFromTheGrounding(t *testing.T) {
	for _, tc := range []struct {
		g    Grounding
		want string
	}{
		{Grounding{KnowHow: true, Levels: true, Bosses: true, Quests: true, Drops: true}, "Tactics: general WoW knowledge · Levels, bosses, quests and drops: your game"},
		{Grounding{KnowHow: true, Levels: true, Bosses: true}, "Tactics: general WoW knowledge · Levels and bosses: your game"},
		{Grounding{KnowHow: true}, "Tactics: general WoW knowledge"},
		{Grounding{KnowHow: true, Bosses: true, IcyURL: "https://www.icy-veins.com/wow-forever/x"}, "Tactics: general WoW knowledge · Bosses: your game · Icy Veins"},
		{Grounding{}, ""},
	} {
		if got := tc.g.Footer(); got != tc.want {
			t.Errorf("footer %q, want %q", got, tc.want)
		}
	}
}

func TestIcyButtonFollowsCapability(t *testing.T) {
	g := Grounding{KnowHow: true, Topic: TopicDungeon}
	if !g.Offer() {
		t.Error("no offer for a dungeon know-how answer")
	}
	if (Grounding{KnowHow: true}).Offer() || (Grounding{KnowHow: true, Topic: TopicDungeon, Browsing: true}).Offer() || (Grounding{Topic: TopicDungeon}).Offer() {
		t.Error("offered where it should not be")
	}
	if !CanBrowse(Config{API: &API{}}) || !CanBrowse(Config{Command: Presets["claude"]}) {
		t.Error("an API client and Claude Code can browse")
	}
	for _, name := range []string{"codex", "gemini", "agy"} {
		if CanBrowse(Config{Command: Presets[name]}) {
			t.Errorf("%s was thought able to browse", name)
		}
	}
	if CanBrowse(Config{}) {
		t.Error("no client browses")
	}
}

func TestIcyFetchRefusesOtherHostsAndPaths(t *testing.T) {
	for _, u := range []string{"https://example.com/wow-forever/x", "https://www.icy-veins.com/wow-classic/x", "https://www.icy-veins.com/", "https://www.icy-veins.com/wow-forever/",
		"http://www.icy-veins.com/wow-forever/x", "https://www.icy-veins.com.evil.example/wow-forever/x", "https://www.icy-veins.com/wow-forever/../wow/x",
		"https://user@www.icy-veins.com/wow-forever/x", "https://www.icy-veins.com:8443/wow-forever/x", "file:///etc/passwd", ""} {
		if _, err := AllowedIcyURL(u); err == nil {
			t.Errorf("%q was allowed", u)
		}
	}
	if _, err := AllowedIcyURL("https://www.icy-veins.com/wow-forever/ragefire-chasm"); err != nil {
		t.Errorf("a guide page was refused: %v", err)
	}
	tools := newIcyTools(nil, nil)
	args, _ := json.Marshal(map[string]string{"url": "https://example.com/wow-forever/x"})
	text, isErr, err := tools.Call(context.Background(), IcyTool, args)
	if err != nil || !isErr || !strings.Contains(text, "icy-veins.com/wow-forever") || len(tools.Pages()) != 0 {
		t.Errorf("refusal = %q %v %v", text, isErr, err)
	}
}

func TestIcyToolListedAndClaudeArgv(t *testing.T) {
	defs, err := newIcyTools(emptyTools{}, nil).List(context.Background())
	if err != nil || len(defs) != 1 || defs[0].Name != IcyTool {
		t.Errorf("tools %v %v", defs, err)
	}
	argv := WithBrowse(Argv(Presets["claude"], "p", "mcp.json"), Presets["claude"])
	j := strings.Join(argv, " ")
	if !strings.Contains(j, "--tools WebFetch") || !strings.Contains(j, "WebFetch(domain:www.icy-veins.com)") {
		t.Errorf("argv %s", j)
	}
	if strings.Join(WithBrowse(Presets["codex"], Presets["codex"]), " ") != strings.Join(Presets["codex"], " ") {
		t.Error("codex argv changed")
	}
	if IcyPage([]string{"https://example.com/a", "https://www.icy-veins.com/wow-forever/x"}) != "https://www.icy-veins.com/wow-forever/x" || IcyPage([]string{"https://example.com/a"}) != "" {
		t.Error("IcyPage")
	}
}

type emptyTools struct{}

func (emptyTools) List(context.Context) ([]provider.ToolDef, error) { return nil, nil }
func (emptyTools) Call(context.Context, string, json.RawMessage) (string, bool, error) {
	return "", false, nil
}

// The wording rules never tell the model to withhold what it knows, or to narrate its sources.
func TestKnowHowRulesDoNotForbidKnowledge(t *testing.T) {
	cfg := Config{SavedVariables: syntheticCapture}
	q := Question{Character: "Testrealm:Testmage", Text: "What general strategy should I use for tactics in general?"}
	p, g := cfg.PromptGround(q)
	if !g.KnowHow {
		t.Fatalf("not know-how: %+v", g)
	}
	low := strings.ToLower(p)
	for _, bad := range []string{"general knowledge", "i have no data", "no data", "do not guess", "never use what you know", "do not talk about the dataset", "from memory"} {
		if strings.Contains(low, bad) {
			t.Errorf("the know-how prompt has %q", bad)
		}
	}
	for _, want := range []string{"your own WoW knowledge", "Our facts always win", "bosses and creatures listed"} {
		if !strings.Contains(p, want) {
			t.Errorf("the know-how prompt lacks %q", want)
		}
	}
	if _, g := cfg.PromptGround(Question{Character: q.Character, Text: "What should I do next?"}); g.KnowHow {
		t.Error("a state question got know-how")
	}
}

func TestClassifyLevelUpQuestions(t *testing.T) {
	for text, want := range map[string]string{
		"What's new at my next level?":             "next",
		"What do I get at level 12?":               "12",
		"what will I learn in the next two levels": "next2",
	} {
		cl := Classify(text, nil, nil)
		if cl.Topic != TopicLevelUp || cl.Subject != want || cl.Kind != KindKnowHow {
			t.Errorf("%q = %+v, want levelup %s", text, cl, want)
		}
	}
	for _, text := range []string{"How do I level faster?", "What should I do at level 12?", "what does Deadmines drop"} {
		if cl := Classify(text, nil, nil); cl.Topic == TopicLevelUp {
			t.Errorf("%q classified as level-up", text)
		}
	}
	if (Grounding{KnowHow: true, Topic: TopicLevelUp}).Offer() {
		t.Error("level-up answers offer Icy Veins")
	}
}

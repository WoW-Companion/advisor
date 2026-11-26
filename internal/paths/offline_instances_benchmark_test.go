package paths_test

import (
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/WoW-Companion/advisor/internal/ask"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/dungeons"
	"github.com/WoW-Companion/advisor/internal/paths"
	"github.com/WoW-Companion/advisor/internal/quests"
	"github.com/WoW-Companion/advisor/internal/store"
)

// The offline dungeon-boss loot, raid-lockout, battleground and honor, and quest-chain benchmark: what the advisor
// answers about a dungeon's or raid's bosses and what they drop, about a raid's lockout and attunement, about a
// battleground and an honor reward, and about a quest's place in a quest line, beside the dataset's rows, with no
// network. It reads testdata/places_snapshot (instances, instance_bosses, instance_quests, quest_lines,
// quests_observed) and testdata/usability_snapshot (items, the dataset's only gear rows with stats). The dataset
// links no boss to a creature (every instance_bosses.creature_id is null), so no boss has a drop in it: the cases of a
// boss's drop and its rate run on a synthetic dungeon, boss and creature rows composed here over real items and
// real loot_sources columns, and the cases of the real bosses check that the advisor lists the bosses and says their
// drops are not known. The dataset holds no lockout, attunement, battleground, honor, rank or PvP vendor row, so the
// cases it cannot answer check that the advisor says so and states none. A difference the dataset's own rows cause
// is marked "(data)". make offline-benchmark runs it; WOWC_BENCH_REPORT receives the tables.

const (
	instBossSrc   = "dataset instance_bosses (name, order_index, difficulty_id); game dungeon guides (public: the boss each dungeon ends on)"
	instDropSrc   = "dataset loot_sources (kills, drops, rate), instance_bosses (creature_id); composed rows over real items"
	instLinkSrc   = "dataset instance_bosses (creature_id is null on every row), loot_sources"
	instRaidSrc   = "dataset instances (instance_type raid, max_players, min_level, entrance_zone_name); game raid sizes and boss counts (public)"
	instLockSrc   = "the dataset has no lockout, reset or attunement column or table (instances, instance_quests)"
	instBGSrc     = "dataset instances (types dungeon and raid only: no battleground map)"
	instHonorSrc  = "dataset items, loot_sources, vendor_sources (no honor cost, rank or PvP vendor row); game honor system (public)"
	instChainSrc  = "dataset quest_lines (line_id, position, quest_id), quests_observed (title, level, category)"
	instClassSrc  = "dataset quests_observed (class heading), quest_lines (no class line)"
	synthKeepMap  = 99001
	synthAltMode  = 198
	instNoneWords = "the dataset has no answer"
)

var (
	instOnce  sync.Once
	instCache store.DataStore
)

// instStore serves the places snapshot's tables first (instances, bosses, quests, creatures) and the usability
// snapshot's for the rest (items, loot, class data).
func instStore() store.DataStore {
	instOnce.Do(func() {
		instCache = store.New(qbMix{first: store.DirSource(qbPlacesDir), then: store.DirSource(usabDir)})
	})
	return instCache
}

// instOverlay adds composed rows to the dataset's tables: the dataset's own rows are untouched.
type instOverlay struct {
	store.DataStore
	rows map[string][]store.Row
}

func (o instOverlay) Filter(table string, ranges ...store.Range) ([]store.Row, error) {
	out, err := o.DataStore.Filter(table, ranges...)
	for _, row := range o.rows[table] {
		if mbMatch(row, ranges) {
			out = append(out, row)
		}
	}
	return out, err
}

// Real items a level 40 warrior with nothing worn can use: the composed drops are these.
const (
	iSteelHelm, iMithrilPants, iMithrilGauntlet, iKnightlyLongsword, iPendant, iMithrilShoulder = 7922, 7921, 7919, 864, 4614, 7918
)

type synthDrop struct {
	boss, item  string
	itemID      int64
	kills, drop int64
}

// synthDrops are the composed loot rows: boss, item, loot windows and drops. rate is drops / kills, the dataset's definition.
var synthDrops = []synthDrop{
	{"Synthetic Warden Alpha", "Steel Plate Helm", iSteelHelm, 40, 10},
	{"Synthetic Warden Alpha", "Heavy Mithril Pants", iMithrilPants, 200, 1},
	{"Synthetic Warden Alpha", "Knightly Longsword", iKnightlyLongsword, 12, 12},
	{"Synthetic Warden Beta", "Heavy Mithril Gauntlet", iMithrilGauntlet, 1000, 3},
	{"Synthetic Warden Beta", "Pendant of Myzrael", iPendant, 60, 6},
}

var synthCreature = map[string]int64{"Synthetic Warden Alpha": 990001, "Synthetic Warden Beta": 990002, "Synthetic Alternate Boss": 990009}

func synthStore() store.DataStore {
	row := func(kv ...any) store.Row {
		r := store.Row{}
		for i := 0; i < len(kv); i += 2 {
			r[kv[i].(string)] = kv[i+1]
		}
		return r
	}
	var loot []store.Row
	for _, d := range synthDrops {
		loot = append(loot, row("item_id", d.itemID, "source_kind", "creature", "source_id", synthCreature[d.boss], "kills", d.kills, "drops", d.drop,
			"rate", float64(d.drop)/float64(d.kills), "reporters", int64(4), "tier", "confirmed"))
	}
	// a boss of the keep's alternate mode: it drops a real item that must not be offered for the ordinary dungeon
	loot = append(loot, row("item_id", int64(iMithrilShoulder), "source_kind", "creature", "source_id", synthCreature["Synthetic Alternate Boss"], "kills", int64(10), "drops", int64(5), "rate", 0.5, "reporters", int64(4), "tier", "confirmed"))
	return instOverlay{instStore(), map[string][]store.Row{
		"instances": {row("map_id", int64(synthKeepMap), "name", "Synthetic Hollow Keep", "instance_type", "dungeon", "max_players", int64(5),
			"min_level", int64(38), "max_level", int64(48), "level_source", "curated", "entrance_zone_name", "Synthetic Vale")},
		"instance_bosses": {
			row("map_id", int64(synthKeepMap), "encounter_id", int64(990101), "name", "Synthetic Warden Alpha", "order_index", int64(0), "difficulty_id", int64(0), "creature_id", synthCreature["Synthetic Warden Alpha"]),
			row("map_id", int64(synthKeepMap), "encounter_id", int64(990102), "name", "Synthetic Warden Beta", "order_index", int64(1000), "difficulty_id", int64(1), "creature_id", synthCreature["Synthetic Warden Beta"]),
			row("map_id", int64(synthKeepMap), "encounter_id", int64(990103), "name", "Synthetic Warden Gamma", "order_index", int64(2000), "difficulty_id", int64(0)),
			row("map_id", int64(synthKeepMap), "encounter_id", int64(990104), "name", "Synthetic Alternate Boss", "order_index", int64(3000), "difficulty_id", int64(synthAltMode), "creature_id", synthCreature["Synthetic Alternate Boss"]),
		},
		"loot_sources": loot,
	}}
}

var (
	synthOnce sync.Once
	synthPick *dungeons.DungeonPick
)

// synthKeep is the synthetic dungeon as the advisor describes it to a level 40 warrior with nothing worn.
func synthKeep(t *testing.T) dungeons.DungeonPick {
	t.Helper()
	synthOnce.Do(func() {
		level := 40
		c := character.Character{Faction: "Horde", Class: "WARRIOR", Level: &level}
		synthPick, _, _ = dungeons.Named(c, synthStore(), nil, "Synthetic Hollow Keep")
	})
	if synthPick == nil {
		t.Fatal("the synthetic dungeon was not found")
	}
	return *synthPick
}

// instBosses are the names of a map's encounters of the ordinary difficulties (0 and 1), once each, in the dataset's order.
func instBosses(t *testing.T, ds store.DataStore, name string) []string {
	t.Helper()
	m := rowByName(t, ds, "instances", name)
	rows, err := ds.Filter("instance_bosses", store.Range{Column: "map_id", Min: num(m["map_id"]), Max: num(m["map_id"])})
	if err != nil {
		t.Fatal(err)
	}
	sort.SliceStable(rows, func(i, j int) bool { return num(rows[i]["order_index"]) < num(rows[j]["order_index"]) })
	var out []string
	for _, r := range rows {
		if num(r["difficulty_id"]) <= 1 && !slices.Contains(out, text(r["name"])) {
			out = append(out, text(r["name"]))
		}
	}
	return out
}

// bossCases are dungeons: the boss each is known to end on, and an alternate-mode encounter name the list must not hold.
var bossCases = []struct{ dungeon, last, notBoss string }{
	{"Deadmines", "Edwin VanCleef", ""},
	{"Blackfathom Deeps", "Aku'mai", "Baron Aquanis"},
	{"Gnomeregan", "Mekgineer Thermaplugg", "Mechanical Menagerie"},
	{"Sunken Temple", "Shade of Eranikus", "Dreamscythe and Weaver"},
	{"Shadowfang Keep", "Archmage Arugal", ""},
	{"Scarlet Monastery", "High Inquisitor Whitemane", ""},
	{"Razorfen Downs", "Amnennar the Coldbringer", ""},
	{"Zul'Farrak", "Chief Ukorz Sandscalp", ""},
	{"Stratholme", "Baron Rivendare", ""},
}

// TestOfflineBenchmarkBossListA and B: the bosses of a dungeon asked by name, in the dataset's order, with the alternate
// modes' encounters left out, and the boss the game ends the dungeon on among them.
func TestOfflineBenchmarkBossListA(t *testing.T) { bossList(t, "a", bossCases[:5]) }
func TestOfflineBenchmarkBossListB(t *testing.T) { bossList(t, "b", bossCases[5:]) }

func bossList(t *testing.T, part string, cases []struct{ dungeon, last, notBoss string }) {
	ds := instStore()
	var out []benchRow
	c := character.Character{Faction: "Horde", Class: "WARRIOR"}
	for _, tc := range cases {
		p, _, err := dungeons.Named(c, ds, nil, tc.dungeon)
		if err != nil || p == nil {
			t.Fatalf("%s: %v %v", tc.dungeon, p, err)
		}
		want := instBosses(t, ds, tc.dungeon)
		got := strings.Join(p.Bosses, ", ")
		ok := got == strings.Join(want, ", ") && slices.Contains(p.Bosses, tc.last) && (tc.notBoss == "" || !slices.Contains(p.Bosses, tc.notBoss))
		out = append(out, benchRow{tc.dungeon + ": its bosses", got, fmt.Sprintf("%d bosses of difficulty 0 and 1 in order; ends on %s", len(want), tc.last), instBossSrc, resultIf(ok, "a boss is missing, extra or out of order")})
	}
	finish(t, "advice-dungeon-bosses-"+part, "Offline dungeon bosses ("+part+")", out)
}

// TestOfflineBenchmarkBossListData: encounters the dataset lists that are not bosses, and a name it spells another way.
// The advisor lists what the dataset lists: these are the dataset's rows.
func TestOfflineBenchmarkBossListData(t *testing.T) {
	ds := instStore()
	c := character.Character{Faction: "Horde", Class: "WARRIOR"}
	var out []benchRow
	for _, tc := range []struct{ dungeon, extra, ref string }{
		{"Blackrock Depths", "The Vault", "a room event, not a boss"},
		{"Blackrock Depths", "Ring of Law", "an arena event, not a boss"},
		{"Molten Core", "The Molten Core", "the instance's own name, not a boss"},
		{"Naxxramas", "Spirit of Mograine", "a phase of The Four Horsemen, not a boss"},
		{"Blackfathom Deeps", "Geilhast", "the boss is Gelihast"},
	} {
		p, _, _ := dungeons.Named(c, ds, nil, tc.dungeon)
		listed := p != nil && slices.Contains(p.Bosses, tc.extra)
		res := "match"
		if listed {
			res = "differ (data): the dataset's encounter row is " + tc.ref
		}
		out = append(out, benchRow{tc.dungeon + ": " + tc.extra, fmt.Sprintf("listed: %v", listed), tc.ref, instBossSrc, res})
	}
	finish(t, "advice-dungeon-bosses-data", "Offline dungeon bosses: rows the dataset holds that are not the game's bosses", out)
}

var dropPercent = regexp.MustCompile(`drops (under 0\.1%|[\d.]+%) of kills`)

// TestOfflineBenchmarkBossDropRate: a boss's drop and its rate in the synthetic dungeon: the percent the advisor words is the
// dataset's rate (drops / kills), a rare drop is never "0%", and the drops of an alternate-mode boss are not offered.
func TestOfflineBenchmarkBossDropRate(t *testing.T) {
	p := synthKeep(t)
	var out []benchRow
	for _, d := range synthDrops {
		ref := 100 * float64(d.drop) / float64(d.kills)
		var got dungeons.Drop
		for _, x := range p.Drops {
			if x.ItemID == d.itemID {
				got = x
			}
		}
		line := got.String()
		m := dropPercent.FindStringSubmatch(line)
		ok := got.Boss == d.boss && m != nil
		if ok {
			if m[1] == "under 0.1%" {
				ok = ref < 0.1
			} else {
				n, _ := strconv.ParseFloat(strings.TrimSuffix(m[1], "%"), 64)
				tol := 0.5
				if ref < 1 {
					tol = 0.05
				}
				ok = n > 0 && n >= ref-tol && n <= ref+tol
			}
		}
		out = append(out, benchRow{d.boss + ": " + d.item + " rate", line, fmt.Sprintf("%d drops in %d loot windows = %.2f%%", d.drop, d.kills, ref), instDropSrc, resultIf(ok, "the percent is not the dataset's rate, or is 0%")})
	}
	var alt []string
	for _, x := range p.Drops {
		if x.Boss == "Synthetic Alternate Boss" || x.ItemID == iMithrilShoulder {
			alt = append(alt, x.Item)
		}
	}
	out = append(out, benchRow{"the alternate-mode boss's drop", fmt.Sprintf("%d offered", len(alt)), "none: the boss is not one of the dungeon's ordinary encounters", instDropSrc, resultIf(len(alt) == 0 && !slices.Contains(p.Bosses, "Synthetic Alternate Boss"), "an alternate-mode boss or its drop is offered")})
	out = append(out, benchRow{"a boss the dataset cannot link to a creature", strings.Join(p.UnlinkedBosses, ", "), "drops not known: Synthetic Warden Gamma has no creature_id", instLinkSrc, resultIf(slices.Equal(p.UnlinkedBosses, []string{"Synthetic Warden Gamma"}), "an unlinked boss was given drops or left out")})
	finish(t, "advice-boss-drop-rate", "Offline boss loot: the drop and its rate", out)
}

// TestOfflineBenchmarkBossDropNoAnswer: the dataset's real bosses have no creature link, so no drop: the advisor says the
// drops are not known and offers none.
func TestOfflineBenchmarkBossDropNoAnswer(t *testing.T) {
	ds := instStore()
	level := 40
	c := character.Character{Faction: "Horde", Class: "WARRIOR", Level: &level}
	var out []benchRow
	for i, name := range []string{"Deadmines", "Uldaman", "Scarlet Monastery", "Maraudon"} {
		if i > 0 {
			c.Level = nil // the gear scan is the cost: one dungeon is asked with a level, the rest without
		}
		p, togo, err := dungeons.Named(c, ds, nil, name)
		if err != nil || p == nil {
			t.Fatalf("%s: %v", name, err)
		}
		m := rowByName(t, ds, "instances", name)
		rows, _ := ds.Filter("instance_bosses", store.Range{Column: "map_id", Min: num(m["map_id"]), Max: num(m["map_id"])})
		linked := 0
		for _, r := range rows {
			if r["creature_id"] != nil {
				linked++
			}
		}
		txt := dungeons.RenderNamed(c, *p, togo)
		ok := linked == 0 && len(p.Drops) == 0 && len(p.UnlinkedBosses) == len(p.Bosses) && strings.Contains(txt, "Drops not known yet: ") && !strings.Contains(txt, "Drops for you")
		out = append(out, benchRow{name + ": what its bosses drop", fmt.Sprintf("%d drops; %d bosses with drops not known yet", len(p.Drops), len(p.UnlinkedBosses)), fmt.Sprintf("%d of %d encounter rows link a creature: %s", linked, len(rows), instNoneWords), instLinkSrc, resultIf(ok, "a drop was offered or a boss left out of the not-known list")})
		out = append(out, benchRow{name + ": the game has boss drops", "none known", "game dungeon guides: every boss drops loot", instLinkSrc, "differ (data): no instance_bosses row carries a creature_id, so no boss's drops can be read"})
	}
	finish(t, "advice-boss-drop-no-answer", "Offline boss loot, no answer in the dataset", out)
}

// raidCases are the raids of the game: the group size and the boss count the guides give.
var raidCases = []struct {
	name         string
	size, bosses int
}{
	{"Molten Core", 40, 10}, {"Onyxia's Lair", 40, 1}, {"Zul'Gurub", 20, 10}, {"Blackwing Lair", 40, 8},
	{"Ruins of Ahn'Qiraj", 20, 6}, {"Ahn'Qiraj Temple", 40, 9}, {"Naxxramas", 40, 15},
}

// TestOfflineBenchmarkRaidFacts: a raid asked by name: its group size, level, entrance and bosses.
func TestOfflineBenchmarkRaidFacts(t *testing.T) {
	ds := instStore()
		c := character.Character{Faction: "Horde", Class: "WARRIOR"}
	var out []benchRow
	for _, tc := range raidCases {
		r := rowByName(t, ds, "instances", tc.name)
		p, togo, err := dungeons.Named(c, ds, nil, tc.name)
		if err != nil || p == nil {
			out = append(out, benchRow{tc.name + ": the raid", "no answer", "a raid row in instances", instRaidSrc, "differ: the raid was not found"})
			continue
		}
		txt := dungeons.RenderNamed(c, *p, togo)
		size := "no group size"
		if p.MaxPlayers != nil {
			size = fmt.Sprintf("%d players", *p.MaxPlayers)
		}
		want := fmt.Sprintf("%d players, %s, level %d", tc.size, text(r["entrance_zone_name"]), num(r["min_level"]))
		got := fmt.Sprintf("%s, %s, level %d", size, p.EntranceZone, *p.MinLevel)
		ok := p.Raid && got == want && strings.Contains(txt, "in "+text(r["entrance_zone_name"]))
		out = append(out, benchRow{tc.name + ": size, entrance and level", got, want, instRaidSrc, resultIf(ok, "the group size, the entrance or the level")})
		have := len(instBosses(t, ds, tc.name))
		res := "match"
		if len(p.Bosses) != have || !p.Raid {
			res = "differ: the list is not the dataset's"
		} else if have != tc.bosses {
			res = fmt.Sprintf("differ (data): the dataset lists %d encounters, the game has %d bosses", have, tc.bosses)
		}
		out = append(out, benchRow{tc.name + ": its bosses", fmt.Sprintf("%d listed", len(p.Bosses)), fmt.Sprintf("%d bosses (dataset %d encounter rows)", tc.bosses, have), instRaidSrc, res})
	}
	finish(t, "advice-raid-facts", "Offline raids: size, level, entrance and bosses", out)
}

var instReset = regexp.MustCompile(`(?i)\b(daily|dailies|weekly|lockout|locked|resets?|every (day|week)|per (day|week)|\d+[- ]day|saved|attun\w*)\b`)

// attuneCases are the raids that need an attunement in the game.
var attuneCases = []struct{ name, quest string }{
	{"Onyxia's Lair", "the Drakefire Amulet chain"}, {"Molten Core", "the Molten Core attunement"},
	{"Blackwing Lair", "the Blackhand's Command chain"}, {"Naxxramas", "the Argent Dawn reputation gate"},
}

// stated is a text's lines that are not the advisor saying the lockout is not known.
func stated(txt string) string {
	var keep []string
	for _, l := range strings.Split(txt, "\n") {
		if !strings.HasSuffix(strings.TrimSpace(l), "not known yet") {
			keep = append(keep, l)
		}
	}
	return strings.Join(keep, "\n")
}

// TestOfflineBenchmarkRaidLockout: the dataset has no lockout or reset period for any raid: the advisor says so, states none, and
// does not word a saved-instance rule.
func TestOfflineBenchmarkRaidLockout(t *testing.T) {
	ds := instStore()
		c := character.Character{Faction: "Horde", Class: "WARRIOR"}
	var out []benchRow
	for _, tc := range raidCases {
		p, togo, _ := dungeons.Named(c, ds, nil, tc.name)
		txt := ""
		if p != nil {
			txt = dungeons.RenderNamed(c, *p, togo)
		}
		m := instReset.FindString(stated(txt))
		ok := p != nil && strings.Contains(txt, "Lockout and attunement: not known yet") && m == ""
		got := "no answer at all"
		if p != nil {
			got = "Lockout and attunement: not known yet"
		}
		out = append(out, benchRow{tc.name + ": when the lockout resets", got, instNoneWords + ": instances has no reset column", instLockSrc, resultIf(ok, "no statement that the lockout is not known, or the text says \""+m+"\"")})
	}
	finish(t, "advice-raid-lockout", "Offline raid lockout: not in the dataset", out)
}

// TestOfflineBenchmarkRaidAttunement: no instance_quests row names an attunement for a raid: the advisor lists no quest to
// take before the raid and states no attunement.
func TestOfflineBenchmarkRaidAttunement(t *testing.T) {
	ds := instStore()
		c := character.Character{Faction: "Horde", Class: "WARRIOR"}
	var out []benchRow
	for _, tc := range attuneCases {
		r := rowByName(t, ds, "instances", tc.name)
		rows, _ := ds.Filter("instance_quests", store.Range{Column: "map_id", Min: num(r["map_id"]), Max: num(r["map_id"])})
		p, togo, _ := dungeons.Named(c, ds, nil, tc.name)
		txt := ""
		if p != nil {
			txt = dungeons.RenderNamed(c, *p, togo)
		}
		ok := p != nil && len(rows) == 0 && len(p.Pickups) == 0 && !strings.Contains(txt, "Quests to grab first") && instReset.FindString(stated(txt)) == ""
		out = append(out, benchRow{tc.name + ": the attunement (" + tc.quest + ")", fmt.Sprintf("%d quests to take first", len(p.Pickups)), fmt.Sprintf("game: needs %s; %d instance_quests rows: %s", tc.quest, len(rows), instNoneWords), instLockSrc, resultIf(ok, "a quest or an attunement was named")})
	}
	finish(t, "advice-raid-attunement", "Offline raid attunement: not in the dataset", out)
}

// TestOfflineBenchmarkInstanceQuestions: a question about a raid's lockout names the raid as its subject, so its facts are the raid's;
// a question about lockouts or honor with no place named has no subject, and none is invented.
func TestOfflineBenchmarkInstanceQuestions(t *testing.T) {
	ds := instStore()
	level := 60
	c := character.Character{Faction: "Horde", Class: "WARRIOR", Level: &level}
	var out []benchRow
	for _, tc := range []struct{ q, subject string }{
		{"what is the Molten Core lockout?", "Molten Core"},
		{"do I need an attunement for Blackwing Lair?", "Blackwing Lair"},
		{"what does Onyxia's Lair drop?", "Onyxia's Lair"},
		{"how much honor does a Warsong Gulch win give?", ""},
		{"what rank do I need for the honor vendor?", ""},
	} {
		cl := ask.Classify(tc.q, ds, &c)
		got := fmt.Sprintf("topic %q, subject %q", cl.Topic, cl.Subject)
		want := fmt.Sprintf("topic %q, subject %q", ask.TopicDungeon, tc.subject)
		if tc.subject == "" {
			want = `no dungeon subject: the dataset has no battleground or honor row`
		}
		ok := cl.Subject == tc.subject && (tc.subject == "" && cl.Topic != ask.TopicDungeon || cl.Topic == ask.TopicDungeon)
		out = append(out, benchRow{"the question \"" + tc.q + "\"", got, want, instLockSrc, resultIf(ok, "the subject")})
	}
	finish(t, "advice-instance-questions", "Offline raid, battleground and honor questions", out)
}

// bgCases are the battlegrounds of the game.
var bgCases = []string{"Warsong Gulch", "Arathi Basin", "Alterac Valley"}

// honorItems are real items of the dataset the game gives for honor, as rank gear, a battleground consumable or a mount.
var honorItems = []struct{ name, kind string }{
	{"Knight-Captain's Chain Girdle", "rank gear bought with honor from the faction's PvP vendor"},
	{"Knight-Lieutenant's Chain Boots", "rank gear bought with honor from the faction's PvP vendor"},
	{"First Sergeant's Silk Cuffs", "rank gear bought with honor from the faction's PvP vendor"},
	{"Sergeant Major's Plate Wristguards", "rank gear bought with honor from the faction's PvP vendor"},
	{"Mantle of Honor", "an honor reward"},
}

// TestOfflineBenchmarkBattleground: a battleground is no instance row: the advisor describes none, and names no level range,
// size or entrance for it.
func TestOfflineBenchmarkBattleground(t *testing.T) {
	ds := instStore()
	level := 60
	c := character.Character{Faction: "Horde", Class: "WARRIOR", Level: &level}
	var out []benchRow
	for _, name := range bgCases {
		rows, _ := ds.Filter("instances", store.Range{Column: "name", Min: name, Max: name})
		p, _, err := dungeons.Named(c, ds, nil, name)
		men := dungeons.Mentioned(ds, nil, "when does "+name+" open for queue?")
		out = append(out, benchRow{name + ": the battleground's level range and size", fmt.Sprintf("plan %v, mentioned %q", p != nil, men), fmt.Sprintf("%d instances rows: %s", len(rows), instNoneWords), instBGSrc, resultIf(len(rows) == 0 && err == nil && p == nil && men == "", "a battleground was invented as an instance")})
	}
	res, err := dungeons.PlanWith(c, ds, nil)
	bg := false
	if err == nil {
		for _, p := range append(res.Picks, res.Unknown...) {
			for _, n := range bgCases {
				bg = bg || strings.EqualFold(p.Name, n)
			}
		}
	}
	out = append(out, benchRow{"the dungeon plan at level 60", fmt.Sprintf("a battleground listed: %v", bg), "no battleground among the instances", instBGSrc, resultIf(err == nil && !bg, "a battleground was planned")})
	finish(t, "advice-battleground", "Offline battlegrounds: no answer in the dataset", out)
}

var honorWords = regexp.MustCompile(`(?i)\b(honou?r|rank|marks?|badges?|battleground|pvp|arena|reputation|standing)\b`)

// TestOfflineBenchmarkHonorReward: an honor reward is asked for by its item: the dataset names no source for it, so the advisor says it
// cannot place the item, never calls it a random drop not worth chasing, and states no honor cost or rank.
func TestOfflineBenchmarkHonorReward(t *testing.T) {
	ds := instStore()
	level := 60
	c := character.Character{Faction: "Horde", Class: "WARRIOR", Level: &level}
	var out []benchRow
	for _, tc := range honorItems {
		it := rowByName(t, ds, "items", tc.name)
		id := num(it["id"])
		loot, _ := ds.Filter("loot_sources", store.Range{Column: "item_id", Min: id, Max: id})
		vend, _ := ds.Filter("vendor_sources", store.Range{Column: "item_id", Min: id, Max: id})
		res := paths.For(c, ds, id, paths.Options{})
		txt := strings.ReplaceAll(res.Reason+" "+res.Text(), tc.name, "") // the item's own name may hold "Honor"
		ok := len(loot) == 0 && len(vend) == 0 && !res.Dependable() && strings.Contains(res.Reason, "lists no source") && !strings.Contains(txt, "worth chasing") && !honorWords.MatchString(txt)
		out = append(out, benchRow{tc.name + ": where to get it", res.Reason, fmt.Sprintf("game: %s; dataset: %d loot rows, %d vendor rows", tc.kind, len(loot), len(vend)), instHonorSrc, resultIf(ok, "the item was called not worth chasing, or an honor cost or rank was stated")})
	}
	finish(t, "advice-honor-reward", "Offline honor rewards: no source in the dataset", out)
}

// TestOfflineBenchmarkHonorConsumables: items of a battleground are mounts or consumables with no source row: a mount says the data lists no
// source, and neither is given a price or a mark cost.
func TestOfflineBenchmarkHonorConsumables(t *testing.T) {
	ds := store.New(qbMix{first: store.DirSource(consDir), then: store.DirSource(usabDir)})
	level := 60
	c := character.Character{Faction: "Horde", Class: "WARRIOR", Level: &level}
	var out []benchRow
	for _, name := range []string{"Stormpike Battle Charger", "Black Battlestrider", "Warsong Gulch Iron Ration"} {
		it := rowByName(t, ds, "items", name)
		id := num(it["id"])
		loot, _ := ds.Filter("loot_sources", store.Range{Column: "item_id", Min: id, Max: id})
		vend, _ := ds.Filter("vendor_sources", store.Range{Column: "item_id", Min: id, Max: id})
		res := paths.For(c, ds, id, paths.Options{})
		costs := regexp.MustCompile(`(?i)\d+\s*(marks?|honou?r|badges?)`).MatchString(res.Reason + res.Text())
		ok := len(loot) == 0 && len(vend) == 0 && !res.Dependable() && strings.Contains(res.Reason, "no source") && !costs && !strings.Contains(res.Reason, "worth chasing")
		out = append(out, benchRow{name + ": where to get it", res.Reason, fmt.Sprintf("game: a battleground reward; dataset: %d loot rows, %d vendor rows", len(loot), len(vend)), instHonorSrc, resultIf(ok, "a source, a cost or a verdict was stated")})
	}
	finish(t, "advice-honor-consumables", "Offline battleground rewards: mounts and consumables", out)
}

// refChain is one quest_lines line: its name and quests in position order.
type refChain struct {
	name string
	ids  []int64
}

func refLine(t *testing.T, ds store.DataStore, id int64) refChain {
	t.Helper()
	rows, err := ds.Filter("quest_lines", store.Range{Column: "line_id", Min: id, Max: id})
	if err != nil || len(rows) == 0 {
		t.Fatalf("line %d: %v", id, err)
	}
	sort.SliceStable(rows, func(i, j int) bool { return num(rows[i]["position"]) < num(rows[j]["position"]) })
	l := refChain{name: text(rows[0]["name"])}
	for _, r := range rows {
		l.ids = append(l.ids, num(r["quest_id"]))
	}
	return l
}

// chainCases are quests of the dataset's quest lines: the line and the quest.
var chainCases = []struct{ line, quest int64 }{
	{6026, 92742}, {6026, 92745}, {6026, 92747}, {6026, 92819}, {5963, 91564}, {6293, 96912}, {6293, 96916}, {6293, 96921},
}

// TestOfflineBenchmarkQuestChains: a quest's step, the line's length and the next quest, beside the line's rows.
func TestOfflineBenchmarkQuestChains(t *testing.T) {
	ds := qbStore()
	var out []benchRow
	for _, tc := range chainCases {
		l := refLine(t, ds, tc.line)
		pos := slices.Index(l.ids, tc.quest) + 1
		c := quests.OfWith(ds, nil, tc.quest)
		got := firstLine(quests.Render(c, ""))
		wantNext := "it is the last step"
		if pos < len(l.ids) {
			wantNext = fmt.Sprintf("next is quest %d", l.ids[pos])
		}
		want := fmt.Sprintf("step %d of %d in %s (line %d); %s", pos, len(l.ids), l.name, tc.line, wantNext)
		ok := c.Found && c.Position == pos && c.Total() == len(l.ids) && strings.Contains(got, want)
		out = append(out, benchRow{fmt.Sprintf("%s: quest %d", l.name, tc.quest), got, want, instChainSrc, resultIf(ok, "the step, the length or the next quest")})
	}
	finish(t, "advice-quest-chains", "Offline quest chains: the step and the next quest", out)
}

// TestOfflineBenchmarkQuestChainNames: the steps of a line carry the names the dataset has; a step the dataset names nothing for is
// listed by its number, never by a made-up title.
func TestOfflineBenchmarkQuestChainNames(t *testing.T) {
	ds := qbStore()
	var out []benchRow
	for _, tc := range []struct{ line, quest int64 }{{6026, 92742}, {6293, 96912}} {
		l := refLine(t, ds, tc.line)
		c := quests.OfWith(ds, nil, tc.quest)
		txt := quests.Render(c, "")
		named, ok := 0, true
		for i, id := range l.ids {
			rows, _ := ds.Filter("quests_observed", store.Range{Column: "quest_id", Min: id, Max: id})
			title := ""
			if len(rows) > 0 {
				title = text(rows[0]["title"])
			}
			if title != "" {
				named++
				ok = ok && strings.Contains(txt, fmt.Sprintf("%d. %s (quest %d)", i+1, title, id))
			} else {
				ok = ok && strings.Contains(txt, fmt.Sprintf("%d. quest %d\n", i+1, id))
			}
		}
		out = append(out, benchRow{l.name + ": the steps' names", fmt.Sprintf("%d of %d steps named", named, len(l.ids)), fmt.Sprintf("%d titles in quests_observed; the rest by number", named), instChainSrc, resultIf(ok, "a step has a title the dataset does not hold, or lost one it has")})
		if named < len(l.ids) {
			out = append(out, benchRow{l.name + ": the steps the dataset names nothing for", fmt.Sprintf("%d by number", len(l.ids)-named), "every step named", instChainSrc, fmt.Sprintf("differ (data): quests_observed holds no title for %d of the %d steps", len(l.ids)-named, len(l.ids))})
		}
	}
	finish(t, "advice-quest-chain-names", "Offline quest chains: the names of the steps", out)
}

// classChain are class quests of one class heading: the dataset ties them by heading and level, not by a line.
var classChain = []int64{1509, 1511, 1512, 1513, 1515, 1803}

// TestOfflineBenchmarkClassChainNoAnswer: the dataset has no quest line of a class's quests: the advisor places none in a line, names no
// next quest, and leaves the order to the quest's own wording.
func TestOfflineBenchmarkClassChainNoAnswer(t *testing.T) {
	ds := qbStore()
	var out []benchRow
	for _, id := range classChain {
		q := qbQuest(t, id)
		lines, _ := ds.Filter("quest_lines", store.Range{Column: "quest_id", Min: id, Max: id})
		c := quests.OfWith(ds, nil, id)
		got := quests.Render(c, "")
		ok := len(lines) == 0 && c.NextQuestID == 0 && strings.Contains(got, "No follow-up quest is listed") && !strings.Contains(got, "step ")
		out = append(out, benchRow{fmt.Sprintf("%s (%d): the next quest of the %s line", text(q["title"]), id, text(q["category"])), firstLine(got), fmt.Sprintf("%d quest_lines rows; the dataset has no class line", len(lines)), instClassSrc, resultIf(ok, "a line or a next quest was named")})
	}
	finish(t, "advice-class-chain-no-answer", "Offline class quest chains: no line in the dataset", out)
}

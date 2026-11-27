package paths_test

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/ask"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/dungeons"
	"github.com/WoW-Companion/advisor/internal/store"
)

// The offline instance entry benchmark: what a player needs before going into a dungeon or raid, as the advisor words it for a question in the
// player's own words, beside the dataset's instances, instance_quests and items rows and the game's guides, with no network. The cases are the attunement
// of a raid or dungeon, its key and the rogue's lockpicking alternative, its minimum and recommended level, its group size (5, 10, 20 and 40 players),
// the short names players call an instance by, and the questions the dataset cannot answer. It reads testdata/places_snapshot (instances, instance_quests)
// and testdata/gating_snapshot (the key items). The dataset holds no attunement, key-to-door, lockpicking or lockout row, so a case it cannot answer
// checks that the advisor says "not known yet" (or carries the limit line) and names no chain, key or skill from memory; the attunement, key and lockpicking
// rows of the game are listed beside it as "(data)". A difference the dataset's own rows cause is marked "(data)". make offline-benchmark runs it;
// WOWC_BENCH_REPORT receives the tables.

const (
	ieAttuneSrc = "dataset instances, instance_quests (no attunement, quest chain or reputation gate row); Warcraft Wiki, Classic raid attunements (Molten Core, Onyxia's Lair, Blackwing Lair, Naxxramas, Ahn'Qiraj Temple) and Upper Blackrock Spire"
	ieKeySrc    = "dataset items (class_id 13 key, class_id 0 Skeleton Key; no row links a key to a door or says how it is obtained), instances (no key column); Warcraft Wiki key item and dungeon pages; Warcraft Wiki, Lockpicking"
	ieLevelSrc  = "dataset instances (min_level, max_level, level_source curated); Warcraft Wiki, Classic dungeon level ranges"
	ieSizeSrc   = "dataset instances (max_players, instance_type); Warcraft Wiki, Classic dungeon and raid group sizes (5, 10, 20 and 40 players)"
	ieNoneSrc   = "dataset instances (min_level, max_level null), instance_quests; no lockout, role, gear level or difficulty table"
	ieNameSrc   = "dataset instances (name); the short names players use in the game's chat (Warcraft Wiki, Abbreviations)"
)

// ieChar is a Horde warrior of a level, with no gear: the cases ask about the instance, not the gear.
func ieChar(level int) character.Character {
	return character.Character{Name: "Entrytest", Faction: "Horde", Class: "WARRIOR", Level: &level}
}

// ieGround is what the advisor puts before the model for a question about an instance: the question's class, and the instance's facts when the
// question is know-how about a dungeon (the same classification and rendering as the ask path).
func ieGround(t *testing.T, c character.Character, q string) (ask.Class, string, *dungeons.DungeonPick) {
	t.Helper()
	ds := gateStore()
	cl := ask.Classify(q, ds, &c)
	if cl.Kind != ask.KindKnowHow || cl.Topic != ask.TopicDungeon {
		return cl, "", nil
	}
	p, togo, err := dungeons.Named(c, ds, nil, cl.Subject)
	if err != nil || p == nil {
		return cl, "", nil
	}
	return cl, dungeons.RenderNamed(c, *p, togo), p
}

// partyWords is the group size as the advisor words it: a dungeon's party is five, Blackrock Spire has a five-player and a ten-player part, a raid keeps its size.
func partyWords(name string, raid bool, size int64) string {
	if name == "Blackrock Spire" && !raid && size == 10 {
		return "5 players in Lower Blackrock Spire, 10 in Upper"
	}
	return fmt.Sprintf("%d players", size)
}

// ieShape is the classification the case wants: know-how about the named instance.
func ieShape(cl ask.Class, subject string) bool {
	return cl.Kind == ask.KindKnowHow && cl.Topic == ask.TopicDungeon && cl.Subject == subject
}

func ieDesc(cl ask.Class) string {
	kind := "state question (no instance facts)"
	if cl.Kind == ask.KindKnowHow {
		kind = "know-how"
	}
	return fmt.Sprintf("%s, topic %q, subject %q", kind, cl.Topic, cl.Subject)
}

// ieNamed is a chain, key, skill or reputation word the data does not hold: it may not appear in the facts except as the "not known yet" line.
var ieNamed = regexp.MustCompile(`(?i)attun\w*[^.]*(?:quest|chain)|drakefire|argent dawn|scepter|seal of ascension|skeleton key|crescent key|shadowforge key|key to the city|lothos|lockpicking skill|pick lock`)

var ieAttuneCases = []struct {
	q, subject, game string
	raid             bool
}{
	{"do I need an attunement for Molten Core?", "Molten Core", "the Molten Core attunement quest (Attunement to the Core, from Lothos Riftwaker in Blackrock Mountain)", true},
	{"can I enter Onyxia's Lair without the attunement?", "Onyxia's Lair", "the Drakefire Amulet quest chain", true},
	{"how do I get attuned to Blackwing Lair?", "Blackwing Lair", "the Blackhand's Command quest chain (the Blackrock Spire orb)", true},
	{"what do I need for Naxxramas?", "Naxxramas", "the Argent Dawn reputation gate", true},
	{"how do I get into the Temple of Ahn'Qiraj?", "Ahn'Qiraj Temple", "the gates open by a server-wide event; the Scepter of the Shifting Sands quest chain", true},
	{"how do I get into Upper Blackrock Spire?", "Blackrock Spire", "the Seal of Ascension quest chain", false},
	{"do I need an attunement for ZG?", "Zul'Gurub", "none: Zul'Gurub needs no attunement", true},
}

// TestOfflineBenchmarkInstanceAttunement: a question about the attunement or entry quest of a raid or dungeon, in the words a player uses ("how do I get attuned",
// "what do I need for", "how do I get into"), reaches the instance's facts and the line that the dataset has no attunement, and no chain is named from memory. Before the fix
// the questions that began "how do I get" or "what do I need for" were taken for questions about the player's own character and got no instance facts at all.
func TestOfflineBenchmarkInstanceAttunement(t *testing.T) {
	ds := gateStore()
	var out []benchRow
	for _, tc := range ieAttuneCases {
		cl, txt, p := ieGround(t, ieChar(60), tc.q)
		line := "Key or entry requirement: not known yet"
		if tc.raid {
			line = "Lockout and attunement: not known yet"
		}
		rows, _ := ds.Filter("instance_quests", store.Range{Column: "map_id", Min: int64(0), Max: int64(1 << 40)})
		linked := 0
		if p != nil {
			for _, r := range rows {
				if num(r["map_id"]) == p.MapID {
					linked++
				}
			}
		}
		ok := ieShape(cl, tc.subject) && p != nil && strings.Contains(txt, line) && ieNamed.FindString(stated(txt)) == "" && (!tc.raid || len(p.Pickups) == 0)
		out = append(out, benchRow{fmt.Sprintf("%q: the attunement", tc.q), ieDesc(cl) + "; " + line, fmt.Sprintf("game: %s; dataset: %d instance_quests rows for the map", tc.game, linked), ieAttuneSrc,
			resultIf(ok, "no instance facts, no statement that the attunement is not known, or a chain was named")})
		if !strings.HasPrefix(tc.game, "none") {
			out = append(out, benchRow{fmt.Sprintf("%s: the attunement chain itself", tc.subject), "none named", tc.game, ieAttuneSrc, "differ (data): the dataset holds no attunement, quest chain or reputation gate for the map, so the advisor names none"})
		}
	}
	finish(t, "advice-instance-attunement", "Offline instance attunement: asked in the player's words, no chain guessed", out)
}

var ieKeyCases = []struct {
	q, subject, key, game string
}{
	{"what key do I need for Scholomance?", "Scholomance", "Skeleton Key", "the Skeleton Key (a rogue with 150 lockpicking opens the door instead)"},
	{"where do I get the Crescent Key for Dire Maul?", "Dire Maul", "Crescent Key", "the Crescent Key (opens a locked Dire Maul gate; a Lower Blackrock Spire drop)"},
	{"how do I get the Shadowforge Key for Blackrock Depths?", "Blackrock Depths", "Shadowforge Key", "the Shadowforge Key (opens a vault door inside; no key needed to enter)"},
	{"do I need a key for Stratholme?", "Stratholme", "Key to the City", "no key to enter; the Key to the City opens the service gate"},
	{"where can I get the key for Upper Blackrock Spire?", "Blackrock Spire", "Unforged Seal of Ascension", "the Seal of Ascension (from the Unforged Seal quest chain)"},
}

// TestOfflineBenchmarkInstanceKeys: a question about the key a dungeon needs or where to get it reaches the dungeon's facts, which say the key or entry requirement is not known,
// and names no key, door or source from memory. The key itself is an item row the dataset holds (with no link to a door); the advisor names it only when the player does.
func TestOfflineBenchmarkInstanceKeys(t *testing.T) {
	ds := gateStore()
	var out []benchRow
	for _, tc := range ieKeyCases {
		cl, txt, p := ieGround(t, ieChar(60), tc.q)
		it := rowByName(t, ds, "items", tc.key)
		ok := ieShape(cl, tc.subject) && p != nil && strings.Contains(txt, "Key or entry requirement: not known yet") && ieNamed.FindString(stated(txt)) == ""
		out = append(out, benchRow{fmt.Sprintf("%q: the key", tc.q), ieDesc(cl) + "; Key or entry requirement: not known yet", fmt.Sprintf("game: %s; dataset: item %d %q, class %d", tc.game, num(it["id"]), tc.key, num(it["class_id"])), ieKeySrc,
			resultIf(ok, "no instance facts, no statement that the key is not known, or a key was named")})
	}
	out = append(out, benchRow{"the key of each door", "none named", "game: each key opens one door of a dungeon", ieKeySrc, "differ (data): no instance row or item row links a key to a door or a dungeon"})
	out = append(out, benchRow{"how a key is obtained", "none named", "game: a quest reward, a drop or a vendor, by key", ieKeySrc, "differ (data): no loot_sources or vendor_sources row for a key item in the snapshot"})
	finish(t, "advice-instance-keys", "Offline instance keys: asked in the player's words, no key guessed", out)
}

// TestOfflineBenchmarkInstanceLockpicking: the rogue's alternative to a dungeon key (Pick Lock on the door) is asked with the dungeon: the advisor carries both the instance's facts and
// the line that the data holds no lockpicking skill, spell or lock table, and states no skill number.
func TestOfflineBenchmarkInstanceLockpicking(t *testing.T) {
	var out []benchRow
	for _, tc := range []struct{ q, subject string }{
		{"can a rogue pick the lock on the Scholomance door instead of the key?", "Scholomance"},
		{"what lockpicking skill does a rogue need to open the Scholomance door?", "Scholomance"},
	} {
		c := ieChar(60)
		c.Class = "ROGUE"
		cl, txt, p := ieGround(t, c, tc.q)
		f := ask.LimitFacts(tc.q)
		ok := ieShape(cl, tc.subject) && p != nil && strings.Contains(txt, "Key or entry requirement: not known yet") && strings.Contains(f, "Lockpicking:") && strings.Contains(f, "Say so") && !regexp.MustCompile(`\b150\b|\b1[0-9][0-9]\b`).MatchString(f+stated(txt))
		out = append(out, benchRow{fmt.Sprintf("%q: the rogue's lockpicking alternative", tc.q), ieDesc(cl) + "; " + firstLine(f), "game: a rogue with 150 lockpicking opens the Scholomance door without the Skeleton Key; dataset: no lockpicking skill, spell or lock row", ieKeySrc,
			resultIf(ok, "no instance facts or no lockpicking limit line, or a skill number was stated")})
	}
	out = append(out, benchRow{"the lockpicking skill the Scholomance door needs", "none named", "game: lockpicking 150 for the Scholomance door", ieKeySrc, "differ (data): the dataset has no lockpicking skill, Pick Lock spell or lock table"})
	finish(t, "advice-instance-lockpicking", "Offline instance keys: the rogue's lockpicking alternative", out)
}

var ieLevelCases = []struct {
	q, name  string
	level    int
	lo, hi   int
	wantTogo int
}{
	{"what is the minimum level for Zul'Farrak?", "Zul'Farrak", 60, 44, 54, 0},
	{"what level should I be for Maraudon?", "Maraudon", 60, 40, 52, 0},
	{"what's the recommended level range for Uldaman?", "Uldaman", 60, 35, 45, 0},
	{"is my level high enough for Razorfen Downs?", "Razorfen Downs", 30, 35, 45, 5},
	{"what level do I need to enter Molten Core?", "Molten Core", 60, 60, 60, 0},
}

// TestOfflineBenchmarkInstanceLevels: the minimum and recommended level of a dungeon or raid, asked in the player's words, is the dataset's range and the game's, and a character under it
// is told how many levels to go.
func TestOfflineBenchmarkInstanceLevels(t *testing.T) {
	ds := gateStore()
	var out []benchRow
	for _, tc := range ieLevelCases {
		cl, txt, p := ieGround(t, ieChar(tc.level), tc.q)
		r := rowByName(t, ds, "instances", tc.name)
		got := "no answer"
		if p != nil && p.MinLevel != nil && p.MaxLevel != nil {
			got = fmt.Sprintf("levels %d-%d", *p.MinLevel, *p.MaxLevel)
		}
		want := fmt.Sprintf("levels %d-%d; dataset %d-%d", tc.lo, tc.hi, num(r["min_level"]), num(r["max_level"]))
		ok := ieShape(cl, tc.name) && p != nil && p.MinLevel != nil && *p.MinLevel == tc.lo && *p.MaxLevel == tc.hi && num(r["min_level"]) == int64(tc.lo) && num(r["max_level"]) == int64(tc.hi)
		if ok && tc.wantTogo > 0 {
			ok = strings.Contains(txt, fmt.Sprintf("it opens around %d", tc.lo))
		}
		out = append(out, benchRow{fmt.Sprintf("%q (level %d): the level", tc.q, tc.level), ieDesc(cl) + "; " + got, want, ieLevelSrc, resultIf(ok, "the level range is not the dataset's and the game's, or the levels to go are not said")})
	}
	finish(t, "advice-instance-levels", "Offline instance levels: minimum and recommended range", out)
}

var ieSizeCases = []struct {
	q, name string
	want    string
	table   int64
}{
	{"is Stratholme a 5-man?", "Stratholme", "5 players", 5},
	{"how many players can go into Deadmines?", "Deadmines", "5 players", 10},
	{"is Sunken Temple a 5 man or a raid?", "Sunken Temple", "5 players", 20},
	{"how many people for Upper Blackrock Spire?", "Blackrock Spire", "5 players in Lower Blackrock Spire, 10 in Upper", 10},
	{"how many players for Zul'Gurub?", "Zul'Gurub", "20 players", 20},
	{"what is the group size for the Ruins of Ahn'Qiraj?", "Ruins of Ahn'Qiraj", "20 players", 20},
	{"how big is the Molten Core raid group?", "Molten Core", "40 players", 40},
	{"what is the group size for the Temple of Ahn'Qiraj?", "Ahn'Qiraj Temple", "40 players", 40},
}

// TestOfflineBenchmarkInstanceGroupSize: the group size of a dungeon (5 players), of a part of one that takes 10, and of a raid (20 or 40 players), asked in the player's words. A dungeon's party is
// five: the dataset's own cap for Deadmines is 10 and for Sunken Temple 20, which the advisor no longer words as a group a player can bring (the difference is the dataset's, marked beside).
func TestOfflineBenchmarkInstanceGroupSize(t *testing.T) {
	ds := gateStore()
	var out []benchRow
	for _, tc := range ieSizeCases {
		cl, txt, p := ieGround(t, ieChar(60), tc.q)
		r := rowByName(t, ds, "instances", tc.name)
		ok := ieShape(cl, tc.name) && p != nil && num(r["max_players"]) == tc.table && strings.Contains(firstLineMatching(txt, "players"), tc.want)
		out = append(out, benchRow{fmt.Sprintf("%q: the group size", tc.q), ieDesc(cl) + "; " + tc.want, fmt.Sprintf("game: %s; dataset max_players %d", tc.want, tc.table), ieSizeSrc, resultIf(ok, "the group size the advisor words is not the game's")})
		if want := tc.want; strings.HasPrefix(want, "5 players") && tc.table != 5 && tc.name != "Blackrock Spire" {
			out = append(out, benchRow{fmt.Sprintf("%s: the dataset's group size", tc.name), fmt.Sprintf("%d players", tc.table), tc.want, ieSizeSrc, fmt.Sprintf("differ (data): the dataset's max_players is %d for a five-player dungeon; the advisor words 5", tc.table)})
		}
	}
	finish(t, "advice-instance-group-size", "Offline instance group size: 5, 10, 20 and 40 players", out)
}

var ieNameCases = []struct{ q, name string }{
	{"level for ZF", "Zul'Farrak"}, {"what level is WC?", "Wailing Caverns"}, {"what is the key for SM?", "Scarlet Monastery"}, {"how many players for Uld?", "Uldaman"},
	{"is ST a 5 man?", "Sunken Temple"}, {"how do I get into VC?", "Deadmines"}, {"what level is DME?", "Dire Maul"}, {"how do I get into UBRS?", "Blackrock Spire"},
}

// TestOfflineBenchmarkInstanceShortNames: the short names players call an instance by reach the instance. Before the fix ZF, WC, SM, Uld, ST, VC and the Dire Maul wings named no instance.
func TestOfflineBenchmarkInstanceShortNames(t *testing.T) {
	ds := gateStore()
	var out []benchRow
	for _, tc := range ieNameCases {
		got := dungeons.Mentioned(ds, nil, tc.q)
		out = append(out, benchRow{fmt.Sprintf("%q: the instance", tc.q), fmt.Sprintf("%q", got), tc.name, ieNameSrc, resultIf(got == tc.name, "the short name named another instance or none")})
	}
	for _, q := range []string{"how do I get MC to proc on my target?", "how many players are in the dm channel?"} {
		got := dungeons.Mentioned(ds, nil, q)
		out = append(out, benchRow{fmt.Sprintf("%q: no instance", q), fmt.Sprintf("%q", got), "none: MC (Mind Control) and DM (Deadmines or Dire Maul) fit more than one thing", ieNameSrc, resultIf(got == "", "an ambiguous short name was taken for an instance")})
	}
	finish(t, "advice-instance-short-names", "Offline instance short names", out)
}

// TestOfflineBenchmarkInstanceNoAnswer: what the dataset has no row for: the entry level of a map with no level, a raid's reset, the roles a raid needs, a dungeon's gear level and its difficulty.
// The advisor says "not known yet" or carries the limit line, and states no number or name from memory.
func TestOfflineBenchmarkInstanceNoAnswer(t *testing.T) {
	var out []benchRow
	for _, tc := range []struct{ q, subject string }{
		{"what level for the Emerald Dream raid?", "Emerald Dream"},
		{"what is the minimum level for Caverns of Time?", "Caverns of Time"},
	} {
		cl, txt, p := ieGround(t, ieChar(60), tc.q)
		ok := ieShape(cl, tc.subject) && p != nil && strings.Contains(txt, "Entry level: not known yet") && p.MinLevel == nil && p.MaxLevel == nil
		out = append(out, benchRow{fmt.Sprintf("%q: the entry level", tc.q), ieDesc(cl) + "; Entry level: not known yet", "dataset: min_level and max_level are null for the map; " + instNoneWords, ieNoneSrc, resultIf(ok, "a level was stated, or no statement that it is not known")})
	}
	for _, tc := range []struct{ q, head string }{
		{"when does my Blackwing Lair raid ID reset?", "Raid lockouts and resets:"},
		{"how many tanks and healers do I need for Molten Core?", "Group roles and threat:"},
		{"what is the recommended gear level for Blackrock Depths?", "Dungeon difficulty and modes:"},
		{"what item level do I need for Molten Core?", "Dungeon difficulty and modes:"},
		{"is Scholomance heroic or normal?", "Dungeon difficulty and modes:"},
	} {
		f := ask.LimitFacts(tc.q)
		ok := strings.Contains(f, tc.head) && strings.Contains(f, "Say so") && strings.Contains(f, "from memory")
		out = append(out, benchRow{fmt.Sprintf("%q: the limit line", tc.q), firstLine(f), instNoneWords + ": say so and give no figure from memory", ieNoneSrc, resultIf(ok, "no limit line saying the dataset has no answer")})
	}
	finish(t, "advice-instance-no-answer", "Offline instance questions the dataset cannot answer", out)
}

// TestOfflineBenchmarkInstanceQuestionsStayPersonal: a question about the player's own plan that names an instance stays a question about the character, and a question about an item's level in
// an instance gets no gear-level line.
func TestOfflineBenchmarkInstanceQuestionsStayPersonal(t *testing.T) {
	var out []benchRow
	c := ieChar(30)
	for _, q := range []string{"what should I do in Maraudon?", "where do I get gear in Deadmines?", "what quest should I do next in Zul'Farrak?"} {
		cl := ask.Classify(q, gateStore(), &c)
		f := ask.LimitFacts(q)
		ok := cl.Kind == ask.KindState || cl.Topic != ask.TopicDungeon
		ok = ok && !strings.Contains(f, "Dungeon difficulty and modes:")
		out = append(out, benchRow{fmt.Sprintf("%q: a personal question", q), ieDesc(cl) + "; no gear-level line: " + fmt.Sprint(f == ""), "the player's own plan or an item's level: no entry facts, no gear-level line", ieLevelSrc, resultIf(ok, "a personal question was answered as an entry question")})
	}
	f := ask.LimitFacts("what is the item level of the Deadmines drops?")
	out = append(out, benchRow{"\"what is the item level of the Deadmines drops?\": an item's level", "no gear-level line: " + fmt.Sprint(f == ""), "the data holds each item's level: no limit line", ieLevelSrc, resultIf(f == "", "an item's level was met with the limit line")})
	finish(t, "advice-instance-personal", "Offline instance questions: a personal question stays personal", out)
}

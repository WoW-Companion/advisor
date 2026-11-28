package paths_test

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/ask"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/dungeons"
	"github.com/WoW-Companion/advisor/internal/levelup"
	"github.com/WoW-Companion/advisor/internal/store"
)

// The offline dungeon, reputation and class-trainer sweep: what the advisor says to synthetic characters (several classes, levels
// across 1 to 60, both factions) beside the dataset's rows and the game's own rules, with no network. The dungeon part sets the
// plan of every character (which dungeons fit, in what order, the "too low" answer, the range and entrance worded) and the plan
// for a named dungeon beside the instances rows; the reputation part sets the standing and the points to the next standing at every
// standing and place in it, and the standing the advisor finds when a player writes the faction's short name ("Timbermaw",
// "Gnomeregan") beside the faction's full name; the trainer part sets the spells and ranks a class trainer teaches at every level of
// every class beside the class_spells rows, the new-spell or new-rank wording at a window and a capture's known spells, and the
// teleports of the other side's cities. testdata/levelling_snapshot holds the rows (instances, class_spells, talent_nodes). The
// reference is written apart from the advisor: the window a dungeon fits (its minimum level up to three levels past its maximum),
// the side of a dungeon by the capital its entrance stands in, the game's reputation thresholds, and, for a ranked class spell, the
// level its row gives. The dataset holds no faction table, no standing a vendor or an item asks for and no class-trainer NPC, so
// those parts check what the capture and the dataset do hold. A difference the dataset's own rows cause is marked "(data)". make
// offline-benchmark runs it; WOWC_BENCH_REPORT receives the tables.

const (
	swDunSrc = "dataset instances (instance_type, min_level, max_level, max_players, entrance_zone_name, level_source); dungeon level lists of Wowhead Classic and the Warcraft Wiki"
	swRepSrc = "game reputation thresholds (Hated -42000, Hostile -6000, Unfriendly -3000, Neutral 0, Friendly 3000, Honored 9000, Revered 21000, Exalted 42000, bar full at 43000); capture reputations"
	swTrnSrc = "dataset class_spells (class_id, name, rank, level, spell_id), talent_nodes (spell_id); Wowhead Classic class trainer lists"
)

var swLevels = []int{1, 10, 12, 13, 15, 18, 20, 25, 28, 30, 35, 40, 45, 50, 55, 58, 60}
var swClasses = []string{"WARRIOR", "MAGE", "ROGUE", "PRIEST", "PALADIN", "DRUID"}

func swChar(faction, class string, level int) character.Character {
	return character.Character{Name: "Benchtest", Faction: faction, Class: class, Level: &level}
}

// ---- dungeons ----

type swDungeon struct {
	name, zone, side string
	lo, hi, id       int
}

// swCapitalSide is the side a dungeon belongs to when its entrance stands in a capital.
var swCapitalSide = map[string]string{"orgrimmar": "Horde", "undercity": "Horde", "thunder bluff": "Horde",
	"stormwind city": "Alliance", "ironforge": "Alliance", "darnassus": "Alliance"}

// swDungeons are the instances rows a player runs as a dungeon: typed dungeon, with a level range, and an entrance zone or a group size.
func swDungeons(t *testing.T) []swDungeon {
	t.Helper()
	rows, err := levStore().Filter("instances", store.Range{Column: "map_id", Min: int64(0)})
	if err != nil {
		t.Fatal(err)
	}
	var out []swDungeon
	for _, r := range rows {
		if text(r["instance_type"]) != "dungeon" || r["min_level"] == nil || r["max_level"] == nil || text(r["entrance_zone_name"]) == "" && r["max_players"] == nil {
			continue
		}
		z := text(r["entrance_zone_name"])
		out = append(out, swDungeon{name: text(r["name"]), zone: z, side: swCapitalSide[strings.ToLower(z)], lo: int(num(r["min_level"])), hi: int(num(r["max_level"])), id: int(num(r["map_id"]))})
	}
	return out
}

// swFits is the dungeons a character of the side and level may be sent to, in the order a guide lists them: the ones the level is inside
// first, then the ones it has outgrown, lowest range first.
func swFits(all []swDungeon, side string, level int) (fits []swDungeon, first *swDungeon) {
	var open []swDungeon
	for _, d := range all {
		if d.side != "" && d.side != side {
			continue
		}
		open = append(open, d)
		if level >= d.lo && level <= d.hi+3 {
			fits = append(fits, d)
		}
	}
	over := func(d swDungeon) int { return max(level-d.hi, 0) }
	sort.SliceStable(fits, func(i, j int) bool {
		a, b := fits[i], fits[j]
		if over(a) != over(b) {
			return over(a) < over(b)
		}
		if a.lo != b.lo {
			return a.lo < b.lo
		}
		if a.hi != b.hi {
			return a.hi < b.hi
		}
		return a.id < b.id
	})
	sort.SliceStable(open, func(i, j int) bool {
		if open[i].lo != open[j].lo {
			return open[i].lo < open[j].lo
		}
		return open[i].id < open[j].id
	})
	if len(fits) == 0 && len(open) > 0 && level < open[0].lo {
		first = &open[0]
	}
	return fits, first
}

func swNames(l []swDungeon) string {
	var n []string
	for _, d := range l {
		n = append(n, d.name)
	}
	if len(n) == 0 {
		return "none"
	}
	return strings.Join(n, ", ")
}

// TestOfflineBenchmarkSweepDungeonPlan: for six classes, both sides and 17 levels the dungeons the plan names, in order, are the rows that
// fit the level (minimum up to three past the maximum) and the side; below every range the answer names the lowest dungeon and the levels to go.
func TestOfflineBenchmarkSweepDungeonPlan(t *testing.T) {
	all := swDungeons(t)
	if len(all) < 15 {
		t.Fatalf("only %d dungeons in the snapshot", len(all))
	}
	var out []benchRow
	for _, side := range []string{"Horde", "Alliance"} {
		for _, class := range swClasses {
			for _, lv := range swLevels {
				c := swChar(side, class, lv)
				res, err := dungeons.Plan(c, levStore())
				if err != nil {
					t.Fatal(err)
				}
				fits, first := swFits(all, side, lv)
				var got []string
				for _, p := range res.Picks {
					got = append(got, p.Name)
				}
				adv, ref := strings.Join(got, ", "), swNames(fits)
				if len(got) == 0 {
					adv = "none"
				}
				txt := dungeons.Render(c, res, 3)
				ok := adv == ref
				note := "a dungeon missing, extra or out of order"
				if first != nil {
					ok = ok && res.First != nil && res.First.Name == first.name && res.LevelsToGo == first.lo-lv && strings.Contains(txt, "Too low for dungeons yet")
					ref += fmt.Sprintf("; too low: %s in %d levels", first.name, first.lo-lv)
					if res.First != nil {
						adv += fmt.Sprintf("; too low: %s in %d levels", res.First.Name, res.LevelsToGo)
					}
					note = "the first dungeon or the levels to go"
				} else if len(fits) > 0 {
					top := fits[0]
					want := fmt.Sprintf("%s (about levels %d–%d; entrance in %s", top.name, top.lo, top.hi, top.zone)
					ok = ok && strings.Contains(txt, want)
					note = "a dungeon missing, extra or out of order, or the range or entrance not worded"
				}
				out = append(out, benchRow{fmt.Sprintf("%s %s %d: the dungeon plan", strings.ToLower(side), strings.ToLower(class), lv), adv, ref, swDunSrc, resultIf(ok, note)})
			}
		}
	}
	finish(t, "advice-sweep-dungeon-plan", "Offline dungeon plan for synthetic characters", out)
}

// TestOfflineBenchmarkSweepDungeonNamed: asking for a dungeon by name gives its range and entrance, the side it belongs to, and for a
// character under its range the level it opens at, for both sides and four levels.
func TestOfflineBenchmarkSweepDungeonNamed(t *testing.T) {
	all := swDungeons(t)
	var out []benchRow
	for _, side := range []string{"Horde", "Alliance"} {
		for _, lv := range []int{10, 30, 45, 60} {
			for _, d := range all {
				c := swChar(side, "WARRIOR", lv)
				p, togo, err := dungeons.Named(c, levStore(), nil, d.name)
				if err != nil || p == nil {
					out = append(out, benchRow{fmt.Sprintf("%s %d: %s by name", strings.ToLower(side), lv, d.name), fmt.Sprint(err), "the dungeon's plan", swDunSrc, "differ: not found by its own name"})
					continue
				}
				txt := dungeons.RenderNamed(c, *p, togo)
				ok := strings.Contains(txt, fmt.Sprintf("about levels %d–%d", d.lo, d.hi)) && strings.Contains(txt, "in "+d.zone)
				ref := fmt.Sprintf("about levels %d–%d, in %s", d.lo, d.hi, d.zone)
				switch {
				case lv < d.lo:
					ok = ok && strings.Contains(txt, fmt.Sprintf("it opens around %d", d.lo))
					ref += fmt.Sprintf("; opens around %d", d.lo)
				case lv > d.hi+3:
					ok = ok && strings.Contains(txt, fmt.Sprintf("tops out around %d", d.hi))
					ref += fmt.Sprintf("; tops out around %d", d.hi)
				}
				if d.side != "" && d.side != side {
					ok = ok && strings.Contains(txt, "Side: "+d.side)
					ref += "; side " + d.side
				}
				out = append(out, benchRow{fmt.Sprintf("%s %d: %s by name", strings.ToLower(side), lv, d.name), firstLine(txt), ref, swDunSrc, resultIf(ok, "the range, entrance, side or the level it opens at is not worded")})
			}
		}
	}
	finish(t, "advice-sweep-dungeon-named", "Offline dungeon asked by name", out)
}

// ---- reputation ----

// swStandings are the game's standings: the id, the name, the points it starts at and the points it ends at.
var swStandings = []struct {
	id      int
	name    string
	lo, hi  int
	nextTxt string
}{
	{1, "Hated", -42000, -6000, "Hostile"}, {2, "Hostile", -6000, -3000, "Unfriendly"}, {3, "Unfriendly", -3000, 0, "Neutral"}, {4, "Neutral", 0, 3000, "Friendly"},
	{5, "Friendly", 3000, 9000, "Honored"}, {6, "Honored", 9000, 21000, "Revered"}, {7, "Revered", 21000, 42000, "Exalted"}, {8, "Exalted", 42000, 43000, ""},
}

// TestOfflineBenchmarkSweepStandingProgress: at the first point, the middle and the last point of every standing, with the capture's bar
// and without it, the standing line says the points in it and the points to the next standing as the game's thresholds give them.
func TestOfflineBenchmarkSweepStandingProgress(t *testing.T) {
	var out []benchRow
	for _, s := range swStandings {
		width := s.hi - s.lo
		for _, off := range []int{0, width / 2, width - 1} {
			for _, bar := range []bool{true, false} {
				st, val := s.id, s.lo+off
				r := character.Reputation{FactionID: 72, Name: "Stormwind", Standing: &st, Value: &val}
				if bar {
					r.Min, r.Max = ip(s.lo), ip(s.hi)
				}
				var want string
				if s.id == 8 {
					want = fmt.Sprintf("Stormwind: Exalted (%d of 1000 points: nothing further to earn)", off)
				} else {
					want = fmt.Sprintf("Stormwind: %s (%d of %d points in the standing; %d more to %s)", s.name, off, width, width-off, s.nextTxt)
				}
				got := r.Progress()
				out = append(out, benchRow{fmt.Sprintf("%s at %d of %d, bar captured %v", strings.ToLower(s.name), off, width, bar), got, want, swRepSrc, resultIf(got == want, "the points in or to the next standing")})
			}
		}
	}
	finish(t, "advice-sweep-standing-progress", "Offline reputation standing at every standing and point", out)
}

// swFactions are the Classic reputations with the short name a player writes for each ("" when the full name is the short name).
var swFactions = []struct{ full, short string }{
	{"Stormwind", ""}, {"Ironforge", ""}, {"Darnassus", ""}, {"Gnomeregan Exiles", "Gnomeregan"}, {"Orgrimmar", ""}, {"Undercity", ""},
	{"Thunder Bluff", ""}, {"Darkspear Trolls", "Darkspear"}, {"Argent Dawn", "Argent"}, {"Brood of Nozdormu", "Nozdormu"}, {"Cenarion Circle", "Cenarion"},
	{"Hydraxian Waterlords", "Hydraxian"}, {"Thorium Brotherhood", "Thorium"}, {"Timbermaw Hold", "Timbermaw"}, {"Zandalar Tribe", "Zandalar"},
	{"Bloodsail Buccaneers", "Bloodsail"}, {"Ravenholdt", ""}, {"Gadgetzan", ""}, {"Everlook", ""}, {"Ratchet", ""}, {"Booty Bay", ""},
	{"Wintersaber Trainers", "Wintersaber"}, {"Darkmoon Faire", "Darkmoon"}, {"Shen'dralar", ""}, {"Gelkis Clan Centaur", "Gelkis"},
	{"Magram Clan Centaur", "Magram"}, {"Stormpike Guard", "Stormpike"}, {"Frostwolf Clan", "Frostwolf"}, {"Silverwing Sentinels", "Silverwing"},
	{"Warsong Outriders", "Warsong"}, {"The League of Arathor", "Arathor"}, {"The Defilers", "Defilers"},
}

// swCaptured is a character who has met every faction above: 32 reputations, each at a different place in Friendly or Honored.
func swCaptured(faction string) character.Character {
	c := swChar(faction, "WARRIOR", 60)
	for i, f := range swFactions {
		st, val := 5, 3000+i*100
		c.Reputations = append(c.Reputations, character.Reputation{FactionID: 100 + i, Name: f.full, Standing: &st, Value: &val, Min: ip(3000), Max: ip(9000)})
	}
	return c
}

// TestOfflineBenchmarkSweepStandingNames: a question that names a faction in full, or by the short name players use, gets that faction's
// standing and no other's, whichever place the faction has in the capture's list of reputations (the answer lists eight at most).
func TestOfflineBenchmarkSweepStandingNames(t *testing.T) {
	var out []benchRow
	for _, side := range []string{"Alliance", "Horde"} {
		c := swCaptured(side)
		for i, f := range swFactions {
			asks := []string{fmt.Sprintf("How far am I from Revered with %s? What is my reputation standing?", f.full)}
			if f.short != "" {
				asks = append(asks, fmt.Sprintf("What is my %s reputation? How long to Exalted?", f.short))
			}
			for _, q := range asks {
				facts := ask.StandingFacts(c, q)
				own := c.Reputations[i].Progress()
				others := 0
				for j, r := range c.Reputations {
					if j != i && strings.Contains(facts, r.Progress()) {
						others++
					}
				}
				out = append(out, benchRow{fmt.Sprintf("%s: %q (reputation %d of %d)", strings.ToLower(side), q, i+1, len(swFactions)), fmt.Sprintf("holds the faction's own line: %v; other factions' lines: %d", strings.Contains(facts, own), others),
					"the named faction's standing line alone", swRepSrc, resultIf(strings.Contains(facts, own) && others == 0, "the named faction's standing missing or mixed with others")})
			}
		}
		// A reputation question that names no faction lists the captured standings, eight at most.
		facts := ask.StandingFacts(c, "What are my reputations?")
		lines := strings.Count(facts, "\n- ")
		out = append(out, benchRow{strings.ToLower(side) + ": a reputation question naming no faction", fmt.Sprintf("%d faction lines", lines), "up to 8 faction lines and the limit line", swRepSrc,
			resultIf(lines == 8 && strings.Contains(facts, "holds no faction reputation"), "not eight lines, or the limit line is missing")})
		// What a standing buys, or how to earn it, is not in the game data: every answer carries the line that says so.
		for _, q := range []string{"What do I get at Exalted with Argent Dawn?", "How do I grind Honored with Timbermaw?", "Which items need Revered?"} {
			f := ask.StandingFacts(c, q)
			out = append(out, benchRow{strings.ToLower(side) + ": " + q, firstLine(f), "the limit line: no faction reputation, no standing an item asks for, no grind route", swRepSrc,
				resultIf(strings.HasPrefix(f, ask.NoStandingGrindFacts), "the limit line is missing")})
		}
	}
	finish(t, "advice-sweep-standing-names", "Offline reputation answers by faction name", out)
}

// ---- class trainer ----

// swTrainerExceptions are ranked rows at a level that no class trainer teaches though the row looks like a rank: the first rank of Touch
// of Weakness (a quest teaches it), the first rank of a talent bought with points (Mana Tide Totem), a rune ability listed at level 1
// (Shadow Cleave) and the effect half of a spell (Hellfire Effect).
func swNotTrained(name string, rank, level int64) bool {
	return name == "Touch of Weakness" && rank == 1 || level <= 1 && (name == "Mana Tide Totem" || name == "Shadow Cleave") || strings.HasSuffix(name, " Effect")
}

// swTrainerRanks is, for a class, the trainer-taught ranked spell at each level, the highest rank where two ranks share a level: a ranked
// row at level 2 or later, or a level 1 row whose name has later ranks.
func swTrainerRanks(t *testing.T, class string) map[string]store.Row {
	t.Helper()
	rows := levRows(t, "class_spells", levelup.ClassIDs[class])
	later := map[string]bool{}
	for _, r := range rows {
		if num(r["rank"]) > 0 && num(r["level"]) > 1 {
			later[text(r["name"])] = true
		}
	}
	best := map[string]store.Row{}
	for _, r := range rows {
		rk, lv, n := num(r["rank"]), num(r["level"]), text(r["name"])
		if rk < 1 || lv <= 1 && !later[n] || swNotTrained(n, rk, lv) {
			continue
		}
		k := fmt.Sprintf("%d/%s", lv, n)
		if o, ok := best[k]; !ok || num(o["rank"]) < rk {
			best[k] = r
		}
	}
	return best
}

var swAllClasses = []string{"WARRIOR", "PALADIN", "HUNTER", "ROGUE", "PRIEST", "SHAMAN", "MAGE", "WARLOCK", "DRUID"}

// TestOfflineBenchmarkSweepTrainerRanks: for every class, at every level of 1 to 60, the ranked spells the trainer list names are the ones the
// class_spells rows give for that level, once each at the highest rank, and nothing ranked is named that the rows do not give.
func TestOfflineBenchmarkSweepTrainerRanks(t *testing.T) {
	var out []benchRow
	for _, class := range swAllClasses {
		ref := swTrainerRanks(t, class)
		missing, extra, checked := []string{}, []string{}, 0
		got := map[string]int64{}
		for lv := 1; lv <= 60; lv++ {
			c := swChar("Horde", class, lv)
			for _, s := range levelup.Spells(c, levStore(), lv, lv) {
				if s.Rank > 0 {
					got[fmt.Sprintf("%d/%s", lv, s.Name)] = int64(s.Rank)
				}
			}
		}
		for k, r := range ref {
			checked++
			if g := got[k]; g != num(r["rank"]) {
				missing = append(missing, fmt.Sprintf("%s rank %d (offered %d)", k, num(r["rank"]), g))
			}
		}
		for k, g := range got {
			if r, ok := ref[k]; !ok || num(r["rank"]) != g {
				extra = append(extra, fmt.Sprintf("%s rank %d", k, g))
			}
		}
		sort.Strings(missing)
		sort.Strings(extra)
		out = append(out, benchRow{strings.ToLower(class) + ": the ranked spells at levels 1 to 60", fmt.Sprintf("%d missing, %d extra", len(missing), len(extra)), fmt.Sprintf("%d ranked spells at their row's level", checked), swTrnSrc,
			resultIf(len(missing) == 0 && len(extra) == 0, strings.Join(append(missing, extra...), "; "))})
	}
	finish(t, "advice-sweep-trainer-ranks", "Offline class trainer spells and ranks at every level", out)
}

// TestOfflineBenchmarkSweepTrainerNewOrRank: a higher rank is "a new rank" only for a character who has a lower rank of the spell. Over
// levels 1 to the level, a character who knows no rank has a new spell; one who knows every lower rank has a new rank.
func TestOfflineBenchmarkSweepTrainerNewOrRank(t *testing.T) {
	var out []benchRow
	for _, class := range swAllClasses {
		bad, checked := []string{}, 0
		rows := levRows(t, "class_spells", levelup.ClassIDs[class])
		for _, lv := range []int{20, 40, 60} {
			none := swChar("Horde", class, lv)
			none.KnownSpells = map[int64]bool{}
			some := swChar("Horde", class, lv)
			some.KnownSpells = map[int64]bool{}
			for _, r := range rows { // everything learned below the level: the lower ranks of every spell
				if num(r["level"]) < int64(lv) && num(r["rank"]) > 0 {
					some.KnownSpells[num(r["spell_id"])] = true
				}
			}
			for _, s := range levelup.Spells(none, levStore(), 1, lv) {
				if s.Rank > 1 {
					checked++
					if !s.New {
						bad = append(bad, fmt.Sprintf("level %d: %s offered as a new rank to a character who knows none", lv, s.Label()))
					}
				}
			}
			for _, s := range levelup.Spells(some, levStore(), lv, lv) {
				if s.Rank > 1 && s.Level == lv {
					checked++
					lower := false
					for _, r := range rows {
						lower = lower || text(r["name"]) == s.Name && num(r["rank"]) > 0 && num(r["rank"]) < int64(s.Rank) && num(r["level"]) < int64(lv)
					}
					if lower == s.New {
						bad = append(bad, fmt.Sprintf("level %d: %s new=%v with a lower rank known=%v", lv, s.Label(), s.New, lower))
					}
				}
			}
		}
		sort.Strings(bad)
		out = append(out, benchRow{strings.ToLower(class) + ": new spell or new rank", fmt.Sprintf("%d of %d offers differ", len(bad), checked), "a new spell with no lower rank known, a new rank with one", swTrnSrc,
			resultIf(len(bad) == 0, strings.Join(bad[:min(len(bad), 3)], "; "))})
	}
	finish(t, "advice-sweep-trainer-new-rank", "Offline class trainer: new spell or new rank", out)
}

// TestOfflineBenchmarkSweepTrainerCities: a mage's Teleport and Portal spells lead to one city each, and a mage trains the ones of their own
// side's capitals only: at every level the trainer list holds the side's teleports and none of the other side's.
func TestOfflineBenchmarkSweepTrainerCities(t *testing.T) {
	cities := map[string][]string{"Alliance": {"Stormwind", "Ironforge", "Darnassus"}, "Horde": {"Orgrimmar", "Undercity", "Thunder Bluff"}}
	other := map[string]string{"Alliance": "Horde", "Horde": "Alliance"}
	var out []benchRow
	for _, side := range []string{"Alliance", "Horde"} {
		own, foreign := []string{}, []string{}
		for _, spell := range []string{"Teleport: ", "Portal: "} {
			for _, c := range cities[side] {
				own = append(own, spell+c)
			}
			for _, c := range cities[other[side]] {
				foreign = append(foreign, spell+c)
			}
		}
		listed := map[string]bool{}
		for _, s := range levelup.Spells(swChar(side, "MAGE", 60), levStore(), 1, 60) {
			listed[s.Name] = true
		}
		missingOwn, hasForeign := 0, 0
		for _, n := range own {
			if !listed[n] {
				missingOwn++
			}
		}
		for _, n := range foreign {
			if listed[n] {
				hasForeign++
			}
		}
		out = append(out, benchRow{strings.ToLower(side) + " mage: teleports and portals", fmt.Sprintf("own missing %d of %d, other side's listed %d of %d", missingOwn, len(own), hasForeign, len(foreign)),
			"all of the side's six, none of the other side's", swTrnSrc, resultIf(missingOwn == 0 && hasForeign == 0, "a teleport or portal of the wrong side")})
	}
	finish(t, "advice-sweep-trainer-cities", "Offline mage teleports by side", out)
}

// swTrainerCities are the capitals with a class trainer in the Classic Warcraft Wiki class trainer lists, by side and class.
var swTrainerCities = map[string]map[string][]string{
	"Horde": {"WARRIOR": {"Undercity", "Orgrimmar", "Thunder Bluff"}, "MAGE": {"Undercity", "Orgrimmar"}, "PRIEST": {"Undercity", "Orgrimmar", "Thunder Bluff"},
		"WARLOCK": {"Undercity", "Orgrimmar"}, "ROGUE": {"Undercity", "Orgrimmar"}, "HUNTER": {"Orgrimmar", "Thunder Bluff"}, "SHAMAN": {"Orgrimmar", "Thunder Bluff"},
		"DRUID": {"Thunder Bluff"}, "PALADIN": {"Silvermoon City"}},
	"Alliance": {"WARRIOR": {"Stormwind City", "Ironforge", "Darnassus"}, "PALADIN": {"Stormwind City", "Ironforge"}, "ROGUE": {"Stormwind City", "Ironforge", "Darnassus"},
		"PRIEST": {"Stormwind City", "Ironforge", "Darnassus"}, "MAGE": {"Stormwind City", "Ironforge"}, "WARLOCK": {"Stormwind City", "Ironforge"},
		"HUNTER": {"Ironforge", "Darnassus"}, "SHAMAN": {"Ironforge"}, "DRUID": {"Darnassus"}},
}

// TestOfflineBenchmarkSweepTrainerPlace: for every class and side, from eleven starting zones, the city the trainer line names is one that
// holds a trainer of the class, says "you are here" in a city that does, and tells the player to cross to the other continent when
// that is where the trainer is.
func TestOfflineBenchmarkSweepTrainerPlace(t *testing.T) {
	zones := map[string][]string{
		"Horde":    {"Durotar", "Mulgore", "Tirisfal Glades", "The Barrens", "Orgrimmar", "Undercity", "Thunder Bluff", "Silverpine Forest", "Hillsbrad Foothills", "Stranglethorn Vale", "Eversong Woods"},
		"Alliance": {"Elwynn Forest", "Dun Morogh", "Teldrassil", "Westfall", "Stormwind City", "Ironforge", "Darnassus", "Loch Modan", "Wetlands", "Redridge Mountains", "Duskwood"},
	}
	var out []benchRow
	for _, side := range []string{"Horde", "Alliance"} {
		for _, class := range swAllClasses {
			want := swTrainerCities[side][class]
			if got := levelup.ClassCities(side, class); strings.Join(got, ",") != strings.Join(want, ",") {
				out = append(out, benchRow{strings.ToLower(side + " " + class + ": cities with a trainer"), strings.Join(got, ", "), strings.Join(want, ", "), swTrnSrc, "differ: the cities differ from the guide's list"})
				continue
			}
			bad, n := []string{}, 0
			for _, z := range zones[side] {
				c := swChar(side, class, 30)
				c.Location = &character.Location{Zone: z}
				p, ok := levelup.TrainAt(c, nil)
				n++
				inList := false
				for _, w := range want {
					inList = inList || strings.EqualFold(w, p.City)
				}
				here := strings.EqualFold(z, p.City)
				if !ok || !inList || here != p.Here || here != strings.Contains(p.Line(class), "(you are here)") {
					bad = append(bad, fmt.Sprintf("%s: %s (here %v)", z, p.City, p.Here))
				}
			}
			out = append(out, benchRow{strings.ToLower(side + " " + class + ": the trainer city from 11 zones"), fmt.Sprintf("%d of %d zones differ", len(bad), n), "a capital whose list holds the class; \"you are here\" in it", swTrnSrc,
				resultIf(len(bad) == 0, strings.Join(bad, "; "))})
		}
	}
	finish(t, "advice-sweep-trainer-place", "Offline class trainer city from every starting zone", out)
}

// TestOfflineBenchmarkSweepTrainerTalents: a spell the dataset's talent_nodes give as a talent (by spell id, in a tree of any class or none)
// is bought with a talent point, so the trainer list of the class never names it (a rank-less spell; the ranked ones the trainer teaches
// at a level, Wrack at 40, are trainer spells even where a node holds their id), at any level of 1 to 60.
func TestOfflineBenchmarkSweepTrainerTalents(t *testing.T) {
	nodes, err := levStore().Filter("talent_nodes", store.Range{Column: "spell_id", Min: 1, Max: 1 << 40})
	if err != nil {
		t.Fatal(err)
	}
	talent := map[int64]string{}
	for _, n := range nodes {
		talent[num(n["spell_id"])] = text(n["name"])
	}
	var out []benchRow
	for _, class := range swAllClasses {
		var bad []string
		for _, s := range levelup.Spells(swChar("Horde", class, 60), levStore(), 1, 60) {
			if name, ok := talent[s.ID]; ok && s.Rank == 0 {
				bad = append(bad, fmt.Sprintf("%s at %d", name, s.Level))
			}
		}
		out = append(out, benchRow{strings.ToLower(class) + ": talents in the trainer list", fmt.Sprintf("%d talents listed", len(bad)), "no spell that talent_nodes gives as a talent", swTrnSrc,
			resultIf(len(bad) == 0, strings.Join(bad, "; "))})
	}
	finish(t, "advice-sweep-trainer-talents", "Offline class trainer list holds no talent", out)
}

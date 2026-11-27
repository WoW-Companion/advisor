package paths_test

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/ask"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/dungeons"
	"github.com/WoW-Companion/advisor/internal/gather"
	"github.com/WoW-Companion/advisor/internal/gear"
	"github.com/WoW-Companion/advisor/internal/paths"
	"github.com/WoW-Companion/advisor/internal/store"
)

// The offline gathering, fishing, weapon-skill and honor benchmark: the skill a herb or ore node needs and the zones the advisor names for
// it, where fishing catches an item and what the advisor says an uncatchable or uncharted fish is, which weapon skills a class learns and
// where a Weapon Master teaches them, and the level a PvP rank piece or battleground item is worn from, beside the dataset's rows and the
// game's own rules, with no network. It reads testdata/tradeskills_snapshot (gathering nodes, yields and loot), testdata/companions_snapshot
// (fishing catches), testdata/usability_snapshot (weapon skills, Weapon Masters, rank gear) and testdata/consumables_snapshot (battleground
// rations and Marks of Honor). The dataset holds no honor, rank, battleground bracket, fishing pool or fishing skill table, so those cases
// check that the advisor says so and states none. A difference the dataset's own rows cause is marked "(data)". make offline-benchmark runs
// it; WOWC_BENCH_REPORT receives the tables.

const (
	gnNodeSrc  = "dataset gathering_nodes (skill_required, zone_name), gathering_yields; game: Warcraft Wiki Mining and Herbalism skill tables, Wowhead Classic node pages"
	gnFishSrc  = "dataset fishing_catches (zone_name, item_id, casts, rate); game home zones of the two factions"
	gnWeapSrc  = "dataset weapon_skills (class_id, learnable, default_known), weapon_masters (city, cost_copper, min_level); game: Warcraft Wiki class weapon proficiencies"
	gnHonorSrc = "dataset items (required_level, allowable_class_mask) of the PvP rank pieces; game: honor rank gear is worn from level 55 to 60 (Wowhead Classic PvP sets)"
	gnBGSrc    = "dataset items (required_level) of the battleground rations, bandages and Marks of Honor"
	gnNoneSrc  = "the dataset has no honor, rank, battleground bracket, fishing pool, fishing skill or herb node row for these"
)

// ---- herb and ore nodes: the skill they need and the zones ----

// gnGuide is the skill each node needs by the game's skill tables.
var gnGuide = map[string]int64{"Copper Vein": 1, "Tin Vein": 65, "Silver Vein": 75, "Iron Deposit": 125, "Peacebloom": 1, "Silverleaf": 1, "Earthroot": 15, "Briarthorn": 70}

type gnNode struct {
	name  string
	skill int64 // the lowest skill_required of the node's rows, 0 when none holds one
	zones []string
}

// gnNodes are the nodes the dataset's gathering_yields names for an item, with the zones of their gathering_nodes rows.
func gnNodes(t *testing.T, ds store.DataStore, item int64) []gnNode {
	t.Helper()
	ys, err := ds.Filter("gathering_yields", store.Range{Column: "item_id", Min: item, Max: item})
	if err != nil {
		t.Fatal(err)
	}
	var out []gnNode
	for _, y := range ys {
		rows, _ := ds.Filter("gathering_nodes", store.Range{Column: "node_id", Min: num(y["node_id"]), Max: num(y["node_id"])})
		var n gnNode
		for _, r := range rows {
			if n.name == "" {
				n.name = text(r["name"])
			}
			if s := num(r["skill_required"]); s > 0 && (n.skill == 0 || s < n.skill) {
				n.skill = s
			}
			if z := text(r["zone_name"]); z != "" && !slices.Contains(n.zones, z) {
				n.zones = append(n.zones, z)
			}
		}
		out = append(out, n)
	}
	return out
}

// gnReagent is the advisor's words for one reagent of one craft.
func gnReagent(t *testing.T, ds store.DataStore, c character.Character, craft, reagent string) string {
	t.Helper()
	for _, p := range paths.For(c, ds, num(rowByName(t, ds, "items", craft)["id"]), paths.Options{}).Paths {
		for _, g := range p.Reagents {
			if g.Name == reagent {
				return g.How
			}
		}
	}
	t.Fatalf("%s: no reagent line for %s", craft, reagent)
	return ""
}

type gnCase struct {
	craft, reagent, prof string
	skill                int
}

var gnNodeCases = []gnCase{
	{"Rough Blasting Powder", "Rough Stone", "Mining", 1}, {"Coarse Blasting Powder", "Coarse Stone", "Mining", 20}, {"Coarse Blasting Powder", "Coarse Stone", "Mining", 70},
	{"Heavy Blasting Powder", "Heavy Stone", "Mining", 100}, {"Heavy Blasting Powder", "Heavy Stone", "Mining", 130}, {"Swiftness Potion", "Briarthorn", "Herbalism", 20},
	{"Swiftness Potion", "Briarthorn", "Herbalism", 70}, {"Minor Troll's Blood Elixir", "Earthroot", "Herbalism", 1}, {"Minor Troll's Blood Elixir", "Earthroot", "Herbalism", 20},
	{"Minor Troll's Blood Elixir", "Peacebloom", "Herbalism", 1},
}

// TestOfflineBenchmarkGatherNodeSkill: a reagent a node yields is gathered when the character's skill reaches the lowest skill the dataset
// gives for the nodes, with the zones of the nodes the skill reaches; below it the advisor says what the nodes need and what the
// character has. A node with no row of its own never passes for one any skill can take. The dataset's skill is beside the game's.
func TestOfflineBenchmarkGatherNodeSkill(t *testing.T) { gatherNodeSkill(t, "ore", gnNodeCases[:5]) }

// TestOfflineBenchmarkGatherNodeSkillHerbs is the same for herbs.
func TestOfflineBenchmarkGatherNodeSkillHerbs(t *testing.T) {
	gatherNodeSkill(t, "herb", gnNodeCases[5:])
}

func gatherNodeSkill(t *testing.T, part string, cases []gnCase) {
	ds := tsStore()
	var out []benchRow
	for _, tc := range cases {
		it := rowByName(t, ds, "items", tc.reagent)
		nodes := gnNodes(t, ds, num(it["id"]))
		least, guide := int64(0), int64(0)
		for _, n := range nodes {
			if n.skill > 0 && (least == 0 || n.skill < least) {
				least = n.skill
			}
			if g, ok := gnGuide[n.name]; ok && (guide == 0 || g < guide) {
				guide = g
			}
		}
		how := gnReagent(t, ds, tsChar("Horde", 30, tsSkill{"Engineering", 150, 300}, tsSkill{"Alchemy", 150, 300}, tsSkill{tc.prof, tc.skill, 300}), tc.craft, tc.reagent)
		var ok bool
		var want string
		if int64(tc.skill) >= least {
			var zones []string
			for _, n := range nodes {
				if n.skill <= int64(tc.skill) {
					zones = append(zones, n.zones...)
				}
			}
			listed := strings.Split(strings.TrimPrefix(how[strings.LastIndex(how, " in ")+4:], ""), ", ")
			ok = strings.HasPrefix(how, "gather it with your "+tc.prof) && strings.Contains(how, " in ") && len(listed) == min(3, len(zones))
			for _, z := range listed {
				ok = ok && slices.Contains(zones, z)
			}
			want = fmt.Sprintf("gather it: %s needs %s %d, zones %s", nodes[0].name, tc.prof, least, strings.Join(zones[:min(3, len(zones))], ", "))
		} else {
			ok = strings.Contains(how, fmt.Sprintf("gathering it needs %s %d; yours is %d", tc.prof, least, tc.skill)) && !strings.HasPrefix(how, "gather it")
			want = fmt.Sprintf("not gatherable yet: needs %s %d, yours is %d", tc.prof, least, tc.skill)
		}
		res := resultIf(ok, "gathering offered below the skill the nodes need, or the zones or the skill said wrongly")
		if res == "match" && least != guide {
			res = fmt.Sprintf("differ (data): the dataset gives %d, the game %d", least, guide)
		}
		out = append(out, benchRow{fmt.Sprintf("%s for %s at %s %d", tc.reagent, tc.craft, tc.prof, tc.skill), how, fmt.Sprintf("%s (game %d)", want, guide), gnNodeSrc, res})
	}
	finish(t, "advice-gather-node-skill-"+part, "Offline gathering: the skill a "+part+" node needs and its zones", out)
}

// TestOfflineBenchmarkGatherNodeUnknown: a herb the dataset holds no node for is still gathered by a herbalist, but the advisor says it
// cannot tell what skill it needs; a node the dataset names only in French with no skill is listed as the dataset names it (a data gap).
func TestOfflineBenchmarkGatherNodeUnknown(t *testing.T) {
	ds := tsStore()
	var out []benchRow
	c := tsChar("Horde", 30, tsSkill{"Alchemy", 150, 300}, tsSkill{"Herbalism", 1, 300})
	hows := map[string]string{}
	for _, p := range paths.For(c, ds, 3827, paths.Options{}).Paths { // 3827 is Mana Potion
		for _, g := range p.Reagents {
			hows[g.Name] = g.How
		}
	}
	for _, reagent := range []struct {
		name string
		id   int64
	}{{"Kingsblood", 3356}, {"Stranglekelp", 3820}} {
		it, err := ds.Get("items", reagent.id)
		if err != nil || text(it["name"]) != reagent.name {
			t.Fatalf("item %d is not %s", reagent.id, reagent.name)
		}
		nodes := gnNodes(t, ds, reagent.id)
		how := hows[reagent.name]
		ok := len(nodes) == 0 && strings.HasPrefix(how, "gather it with your Herbalism") && strings.Contains(how, "can't say what skill it needs") && !regexp.MustCompile(`\d`).MatchString(how)
		out = append(out, benchRow{reagent.name + ": the skill it needs, no node in the dataset", how, fmt.Sprintf("%d gathering_yields rows; no skill to state", len(nodes)), gnNoneSrc, resultIf(ok, "a skill or a node stated, or no word that the skill is unknown")})
	}
	ys, _ := ds.Filter("gathering_nodes", store.Range{Column: "node_id", Min: int64(2040), Max: int64(2040)})
	ok := len(ys) > 0 && text(ys[0]["name"]) == "Gisement de mithril" && num(ys[0]["skill_required"]) == 0
	res := "differ (data): the dataset names Mithril Deposit only in French and gives it no skill (the game: 175)"
	if !ok {
		res = "differ: the Mithril Deposit row is no longer the French row without a skill; update this case"
	}
	out = append(out, benchRow{"Mithril Deposit: name and skill", "no reagent of the snapshot's crafts reaches it", fmt.Sprintf("%d row(s): %q, skill_required %d", len(ys), text(ys[0]["name"]), num(ys[0]["skill_required"])), gnNodeSrc, res})
	finish(t, "advice-gather-node-unknown", "Offline gathering: a node the dataset does not describe", out)
}

// ---- fishing ----

var gnFishItems = []struct{ item, faction string }{
	{"Raw Longjaw Mud Snapper", "Horde"}, {"Raw Longjaw Mud Snapper", "Alliance"}, {"Raw Rockscale Cod", "Horde"}, {"Raw Sagefish", "Alliance"},
	{"Firefin Snapper", "Horde"}, {"Raw Spotted Yellowtail", "Alliance"},
}

// TestOfflineBenchmarkFishingSource: the zones the advisor gives for a fish are the dataset's catch zones, best rate first, with the
// source named as the game data's fishing reports and never a Questie database the player has none of; a fish asked about as an item to
// get is called a fishing catch with those zones, not a random drop or a thing with no way to get it.
func TestOfflineBenchmarkFishingSource(t *testing.T) {
	ds := compStore()
	g := gather.New(nil, ds)
	var out []benchRow
	for _, tc := range gnFishItems {
		it := rowByName(t, ds, "items", tc.item)
		id := num(it["id"])
		want := catchZonesOf(t, id, tc.faction, gather.MaxZones)
		res := g.Find(compChar(tc.faction, "MAGE", 40, "Fishing", 100, 150), tc.item, "fish")
		var got []string
		for _, z := range res.Zones {
			got = append(got, z.Zone)
		}
		ok := slices.Equal(got, want) && strings.HasSuffix(res.Text, "("+gather.FromCatches+").") && !strings.Contains(res.Text, "Questie") && !res.FromQuestie
		out = append(out, benchRow{fmt.Sprintf("%s (%s): where to fish, and the source named", tc.item, tc.faction), res.Text, strings.Join(want, ", ") + " from the dataset's fishing reports", gnFishSrc,
			resultIf(ok, "the zones, or a Questie database named as the source")})
		why := paths.For(compChar(tc.faction, "MAGE", 40, "Fishing", 100, 150), ds, id, paths.Options{})
		best, dependable := why.Best()
		ok = (dependable && best.Kind != "") || strings.Contains(why.Reason, "fishing catch") && strings.Contains(why.Reason, want[0])
		out = append(out, benchRow{fmt.Sprintf("%s (%s): how to get it", tc.item, tc.faction), firstLine(why.Reason + " " + best.Kind), "a fishing catch in " + want[0], gnFishSrc,
			resultIf(ok, "called a random drop, or no way to get it, though the dataset has catches")})
	}
	finish(t, "advice-fishing-source", "Offline fishing: the catch zones and the way to get a fish", out)
}

// ---- weapon skills ----

// gnWeaponGuide is what each class learns by the game's Classic proficiency table, by skill name; the dataset is for WoW Forever, which
// differs for rogues (One-Handed Axes).
var gnWeaponGuide = map[string][]string{
	"PRIEST": {"Daggers", "One-Handed Maces", "Staves", "Wands"}, "MAGE": {"Daggers", "One-Handed Swords", "Staves", "Wands"},
	"WARLOCK": {"Daggers", "One-Handed Swords", "Staves", "Wands"}, "ROGUE": {"Bows", "Crossbows", "Daggers", "Fist Weapons", "Guns", "One-Handed Maces", "One-Handed Swords", "Thrown"},
	"DRUID":  {"Daggers", "Fist Weapons", "One-Handed Maces", "Polearms", "Staves", "Two-Handed Maces"},
	"SHAMAN": {"Daggers", "Fist Weapons", "One-Handed Axes", "One-Handed Maces", "Staves", "Two-Handed Axes", "Two-Handed Maces"},
}

var gnWeaponCases = []struct{ class, faction string }{
	{"PRIEST", "Horde"}, {"MAGE", "Alliance"}, {"DRUID", "Horde"}, {"ROGUE", "Alliance"}, {"SHAMAN", "Alliance"}, {"WARLOCK", "Horde"},
}

var gnLearnable = regexp.MustCompile(`(?m)^- The class can learn: (.*)\.$`)

// TestOfflineBenchmarkWeaponSkillFacts: the weapon skills the advisor says a class can learn are the dataset's learnable rows, and each one
// the character lacks lists the faction's masters that teach it with the city and price, beside the game's proficiency table. A skill no
// master teaches (Wands) says there is no master in the data.
func TestOfflineBenchmarkWeaponSkillFacts(t *testing.T) {
	ds := usabStore()
	masters, _ := ds.Filter("weapon_masters", store.Range{Column: "npc_id", Min: int64(0), Max: int64(1 << 40)})
	var out []benchRow
	for _, tc := range gnWeaponCases {
		c := usabChar(tc.class, 40, tc.faction)
		c.SkillsCaptured = true
		c.Skills = []character.Skill{{Name: "Unarmed", Group: "Weapon Skills"}}
		facts := ask.WeaponFacts(c, ds, "which weapons can I use and where do I train them?")
		m := gnLearnable.FindStringSubmatch(facts)
		if m == nil {
			t.Fatalf("%s: no learnable line in %q", tc.class, facts)
		}
		got := strings.Split(m[1], ", ")
		var learnable []string
		rows, _ := ds.Filter("weapon_skills", store.Range{Column: "class_id", Min: classID(tc.class), Max: classID(tc.class)})
		for _, r := range rows {
			if b, _ := r["learnable"].(bool); b {
				learnable = append(learnable, text(r["skill_name"]))
			}
		}
		slices.Sort(learnable)
		res := resultIf(slices.Equal(got, learnable), "the skills the class can learn")
		if res == "match" && !slices.Equal(learnable, gnWeaponGuide[tc.class]) {
			res = "differ (data): WoW Forever's rows differ from the Classic table: " + strings.Join(diffOf(learnable, gnWeaponGuide[tc.class]), ", ")
		}
		out = append(out, benchRow{fmt.Sprintf("%s (%s): the weapon skills it can learn", strings.ToLower(tc.class), tc.faction), m[1], strings.Join(learnable, ", ") + "; Classic: " + strings.Join(gnWeaponGuide[tc.class], ", "), gnWeapSrc, res})

		bad := ""
		for _, r := range rows {
			if b, _ := r["learnable"].(bool); !b {
				continue
			}
			name, skill := text(r["skill_name"]), num(r["skill_line_id"])
			var line string
			for _, l := range strings.Split(facts, "\n") {
				if strings.HasPrefix(l, "  - "+name+": ") {
					line = l
				}
			}
			var city []string
			for _, ms := range masters {
				if text(ms["faction"]) != tc.faction {
					continue
				}
				for i, s := range ints64(ms["skills"]) {
					if s == skill {
						ref := fmt.Sprintf("%s in %s, %s for %s", text(ms["name"]), text(ms["city"]), text(ms["place"]), gear.CostText(ints64(ms["cost_copper"])[i]))
						if lvl := ints64(ms["min_level"])[i]; lvl > 1 {
							ref += fmt.Sprintf(" (level %d)", lvl)
						}
						city = append(city, ref)
					}
				}
			}
			switch {
			case line == "":
				bad += name + ": not listed; "
			case len(city) == 0 && !strings.Contains(line, "no master in the data"):
				bad += name + ": a master named though none teaches it; "
			default:
				for _, ref := range city {
					if !strings.Contains(line, strings.SplitN(ref, " in ", 2)[0]) || !strings.Contains(line, ref[strings.LastIndex(ref, " for ")+5:]) {
						bad += name + ": master, price or level; "
					}
				}
			}
		}
		out = append(out, benchRow{fmt.Sprintf("%s (%s): where each skill is trained", strings.ToLower(tc.class), tc.faction), strings.TrimSpace(firstLine(facts[strings.Index(facts, "train at a Weapon Master")+strings.Index(facts[strings.Index(facts, "train at a Weapon Master"):], "\n")+1:])),
			"the faction's weapon_masters rows (city, price, level)", gnWeapSrc, resultIf(bad == "", bad)})
	}
	finish(t, "advice-weapon-skill-facts", "Offline weapon skills: what a class learns and who teaches it", out)
}

func diffOf(a, b []string) []string {
	var out []string
	for _, x := range a {
		if !slices.Contains(b, x) {
			out = append(out, "+"+x)
		}
	}
	for _, x := range b {
		if !slices.Contains(a, x) {
			out = append(out, "-"+x)
		}
	}
	return out
}

// ---- honor: PvP rank gear and battleground items ----

var gnRankCases = []struct {
	class, item string
	level       int
}{
	{"DRUID", "Knight-Captain's Dragonhide Armsplints", 54}, {"DRUID", "Knight-Captain's Dragonhide Armsplints", 55}, {"HUNTER", "Knight-Lieutenant's Chain Boots", 56},
	{"HUNTER", "Knight-Lieutenant's Chain Boots", 58}, {"SHAMAN", "Legionnaire's Mail Cinch", 50}, {"WARRIOR", "Legionnaire's Mail Cinch", 60},
}

// TestOfflineBenchmarkHonorRankGearLevel: a rank piece is worn from the dataset's required level for the class its mask names: not offered
// more than two levels short of it, "at level N" in the two levels before, "requires level N" from then on, and never to a class the
// mask leaves out.
func TestOfflineBenchmarkHonorRankGearLevel(t *testing.T) {
	ds := usabStore()
	var out []benchRow
	for _, tc := range gnRankCases {
		r := usabItem(t, tc.item)
		rl, mask := num(r["required_level"]), num(r["allowable_class_mask"])
		res, err := gear.Find(usabChar(tc.class, tc.level, "Horde"), ds, "", gear.Options{NoSources: true})
		if err != nil {
			t.Fatal(err)
		}
		cd, offered := found(res, num(r["id"]))
		got := "not offered"
		if offered {
			got = cd.LevelText()
		}
		var want string
		var ok bool
		switch {
		case !maskAllows(mask, classID(tc.class)):
			want, ok = fmt.Sprintf("class mask %d leaves the class out: never offered", mask), !offered
		case rl > int64(tc.level)+2:
			want, ok = fmt.Sprintf("required_level %d: too far above level %d to offer", rl, tc.level), !offered
		case rl > int64(tc.level):
			want, ok = fmt.Sprintf("at level %d", rl), got == fmt.Sprintf("at level %d", rl)
		default:
			want, ok = fmt.Sprintf("requires level %d", rl), got == fmt.Sprintf("requires level %d", rl)
		}
		out = append(out, benchRow{fmt.Sprintf("%s (%s, level %d): the level a rank piece is worn from", tc.item, strings.ToLower(tc.class), tc.level), got, want, gnHonorSrc, resultIf(ok, "the level or the class it is offered at")})
	}
	finish(t, "advice-honor-rank-gear", "Offline honor: the level a PvP rank piece is worn from", out)
}

// TestOfflineBenchmarkHonorBattlegroundItems: a battleground ration, bandage or Mark of Honor has no vendor or loot row in the dataset (the
// honor and mark vendors are not in it): the advisor says the game data lists no source, never that it is a random drop not worth chasing.
func TestOfflineBenchmarkHonorBattlegroundItems(t *testing.T) {
	ds := consStore()
	var out []benchRow
	for _, name := range []string{"Warsong Gulch Field Ration", "Warsong Gulch Mageweave Bandage", "Arathi Basin Mark of Honor", "Warsong Gulch Mark of Honor"} {
		r := rowByName(t, ds, "items", name)
		id := num(r["id"])
		v, _ := ds.Filter("vendor_sources", store.Range{Column: "item_id", Min: id, Max: id})
		l, _ := ds.Filter("loot_sources", store.Range{Column: "item_id", Min: id, Max: id})
		res := paths.For(usabChar("WARRIOR", 60, "Horde"), ds, id, paths.Options{})
		ok := len(v)+len(l) == 0 && !res.Dependable() && strings.Contains(res.Reason, "lists no source") && !strings.Contains(res.Reason, "worth chasing")
		out = append(out, benchRow{name + ": how to get it", res.Reason, fmt.Sprintf("%d vendor rows, %d loot rows (required_level %d)", len(v), len(l), num(r["required_level"])), gnBGSrc,
			resultIf(ok, "a source or a random drop named for an item the dataset has no source for")})
	}
	finish(t, "advice-honor-battleground-items", "Offline honor: how to get a battleground item", out)
}

// ---- no answer ----

// TestOfflineBenchmarkGatherFishWeaponHonorNoAnswer: a question about honor ranks, a battleground's level bracket, a fishing pool or the
// fishing skill a zone needs has no dataset table behind it: the advisor classes it as a know-how question with no topic that holds a
// number (or the profession topic, whose facts hold only ranks), never as a dungeon, and finds no battleground among its dungeons.
func TestOfflineBenchmarkGatherFishWeaponHonorNoAnswer(t *testing.T) {
	ds := levStore()
	var out []benchRow
	for _, q := range []string{"what level can i join warsong gulch?", "which battleground bracket am i in at level 37?", "how many honor points do i need for rank 10?"} {
		cl := ask.Classify(q, ds, nil)
		out = append(out, benchRow{"\"" + q + "\"", fmt.Sprintf("kind %d, topic %q, subject %q", cl.Kind, cl.Topic, cl.Subject), "a know-how question with no topic: no honor or bracket table", gnNoneSrc,
			resultIf(cl.Kind == ask.KindKnowHow && cl.Topic == ask.TopicNone && cl.Subject == "", "a topic or a dungeon invented for it")})
	}
	for _, bg := range []string{"Warsong Gulch", "Arathi Basin", "Alterac Valley"} {
		p, _, err := dungeons.Named(levChar("WARRIOR", 40), ds, nil, bg)
		out = append(out, benchRow{bg + ": a level range among the dungeons", fmt.Sprintf("found %v, err %v", p != nil, err != nil), "no instances row for a battleground", gnNoneSrc, resultIf(p == nil, "a level range given for a battleground")})
	}
	for _, q := range []string{"which fishing pool in the barrens has oily blackmouth?", "what fishing skill do i need for stranglethorn?"} {
		cl := ask.Classify(q, ds, nil)
		ok := cl.Kind == ask.KindKnowHow && (cl.Topic == ask.TopicNone || cl.Topic == ask.TopicProfession && cl.Subject == "fishing")
		out = append(out, benchRow{"\"" + q + "\"", fmt.Sprintf("kind %d, topic %q, subject %q", cl.Kind, cl.Topic, cl.Subject), "know-how: the dataset has no pool or zone skill column", gnNoneSrc, resultIf(ok, "a state or dungeon answer for a pool question")})
	}
	why := paths.For(compChar("Horde", "MAGE", 40, "Fishing", 1, 75), compStore(), num(rowByName(t, compStore(), "items", "Raw Longjaw Mud Snapper")["id"]), paths.Options{})
	out = append(out, benchRow{"Raw Longjaw Mud Snapper at Fishing 1: skill needed", why.Reason, "no fishing skill per zone in the dataset", gnNoneSrc, resultIf(!regexp.MustCompile(`(?i)skill \d|needs fishing`).MatchString(why.Reason), "a fishing skill was stated")})
	finish(t, "advice-gather-fish-weapon-honor-no-answer", "Offline gathering, fishing, weapons and honor: no answer in the dataset", out)
}

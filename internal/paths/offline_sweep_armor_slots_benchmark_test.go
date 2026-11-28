package paths_test

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/WoW-Companion/advisor/internal/ask"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/gear"
	"github.com/WoW-Companion/advisor/internal/inventory"
	itemdata "github.com/WoW-Companion/advisor/internal/items"
	"github.com/WoW-Companion/advisor/internal/store"
)

// The offline armor-slot sweep: what the gear finder offers a synthetic character for the head, shoulder, chest, wrist, hands, waist, legs, feet
// and back slots (all nine classes, damage, tank and healer builds, levels across 1 to 60 with 39 and 40 either side of the plate and mail switch,
// both factions), with the slots empty and with an item worn in every slot; what the equip-from-bags list says about a piece of each armor type in
// the bags; and what the rules facts say to a player's everyday armor questions, beside the dataset's item rows, with no network.
// testdata/usability_snapshot holds the real rows. The reference is written apart from the finder: the item's armor subclass (cloth 1, leather 2,
// mail 3, plate 4) against the type the class wears (warrior and paladin mail then plate, hunter and shaman leather then mail, both from 40; rogue and
// druid leather; priest, mage and warlock cloth), the item's class mask, quality, bonding, faction and required level, and for a worn slot the item
// level above the worn piece's. The dataset holds no armor proficiency spell, so the level 40 switch is the game's rule (Icy Veins WoW Forever class
// guides: Plate Mail and Mail are learned from the class trainer at 40). make offline-benchmark runs it; WOWC_BENCH_REPORT receives the tables.

const (
	asSrc      = "dataset items (class_id 4, subclass_id, slot, allowable_class_mask, required_level, item_level, quality, bonding); game armor proficiency (plate and mail from 40)"
	asBagSrc   = "dataset items (subclass_id, slot, required_level, allowable_class_mask); game armor proficiency (plate and mail from 40)"
	asPhraseSr = "game armor proficiency (cloth, leather, mail, plate; plate and mail from level 40); Icy Veins WoW Forever class guides"
)

// asSlots are the armor slots by capture name, with the inventory types that fill each (a robe fills the chest).
var asSlots = []struct {
	name string
	inv  []int64
}{
	{"head", []int64{1}}, {"shoulder", []int64{3}}, {"chest", []int64{5, 20}}, {"wrist", []int64{9}}, {"hands", []int64{10}},
	{"waist", []int64{6}}, {"legs", []int64{7}}, {"feet", []int64{8}}, {"back", []int64{16}},
}

var asSlotOf = func() map[int64]string {
	m := map[int64]string{}
	for _, s := range asSlots {
		for _, i := range s.inv {
			m[i] = s.name
		}
	}
	return m
}()

// asItem is the part of an armor row the reference reads.
type asItem struct {
	id                         int64
	name, slot                 string
	sub, mask, req, ilvl, qual int64
	bond                       int64
}

var (
	asOnce sync.Once
	asRows []asItem
)

// asItems reads every armor row of the armor slots once for the package.
func asItems(t *testing.T) []asItem {
	t.Helper()
	asOnce.Do(func() {
		rows, err := usabStore().Filter("items", store.Range{Column: "id", Min: 0, Max: 1 << 40})
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range rows {
			slot, ok := asSlotOf[num(r["slot"])]
			if !ok || num(r["class_id"]) != 4 {
				continue
			}
			asRows = append(asRows, asItem{num(r["id"]), text(r["name"]), slot, num(r["subclass_id"]), num(r["allowable_class_mask"]), num(r["required_level"]), num(r["item_level"]), num(r["quality"]), num(r["bonding"])})
		}
	})
	return asRows
}

// asArmorAt is the armor subclass the class wears at a level.
func asArmorAt(class string, level int) int64 { return armorWorn(class, level) }

// asAllowed is the armor rows the class may be offered in the slot at the level by the dataset's rows: a type it wears at the level the piece needs (or its
// own level, the later), its class mask, a quality worth wearing, no quest binding, its faction, and a required level within two of its own. Cloaks are cloth
// for every class.
func asAllowed(t *testing.T, class string, classID int64, level int, faction string) map[string][]asItem {
	t.Helper()
	out := map[string][]asItem{}
	for _, it := range asItems(t) {
		if itemdata.Unobtainable(it.name) || itemdata.WrongFaction(it.name, faction) || it.ilvl <= 1 || it.qual <= 0 || it.qual == 6 || it.bond == 4 || it.bond == 5 {
			continue
		}
		req, _ := gear.ReqLevelOf(int(it.req), int(it.ilvl))
		if req > level+2 || !maskAllows(it.mask, classID) {
			continue
		}
		if it.slot != "back" && it.sub != asArmorAt(class, max(level, req)) {
			continue
		}
		if it.slot == "back" && it.sub != 1 && it.sub != 0 {
			continue
		}
		out[it.slot] = append(out[it.slot], it)
	}
	return out
}

type asBuild struct{ spec, role string }

var asBuilds = map[string][]asBuild{
	"WARRIOR": {{"Fury", "damage"}, {"Protection", "tank"}}, "PALADIN": {{"Retribution", "damage"}, {"Holy", "healer"}},
	"HUNTER": {{"Beast Mastery", "damage"}}, "ROGUE": {{"Combat", "damage"}}, "PRIEST": {{"Holy", "healer"}},
	"SHAMAN": {{"Enhancement", "damage"}, {"Restoration", "healer"}}, "MAGE": {{"Frost", "damage"}}, "WARLOCK": {{"Affliction", "damage"}},
	"DRUID": {{"Feral Combat", "damage"}, {"Restoration", "healer"}},
}

var asLevels = []int{4, 12, 22, 33, 39, 40, 51, 60}

func asOrdered(c []gear.Candidate) bool {
	for k := 1; k < len(c); k++ {
		if c[k].Delta > c[k-1].Delta {
			return false
		}
	}
	return true
}

func asIDs(its []asItem) map[int64]asItem {
	m := map[int64]asItem{}
	for _, it := range its {
		m[it.id] = it
	}
	return m
}

func asDiff(app []gear.Candidate, ref map[int64]asItem) (extra, missing []int64) {
	got := map[int64]bool{}
	for _, c := range app {
		got[c.Item.ID] = true
		if _, ok := ref[c.Item.ID]; !ok {
			extra = append(extra, c.Item.ID)
		}
	}
	for id := range ref {
		if !got[id] {
			missing = append(missing, id)
		}
	}
	sort.Slice(extra, func(i, j int) bool { return extra[i] < extra[j] })
	sort.Slice(missing, func(i, j int) bool { return missing[i] < missing[j] })
	return
}

func asBad(extra, missing []int64) string {
	bad := ""
	if len(extra) > 0 {
		bad += fmt.Sprintf(" offers %v the rows do not allow;", extra)
	}
	if len(missing) > 0 {
		bad += fmt.Sprintf(" leaves out %v the rows allow;", missing)
	}
	return strings.TrimSuffix(strings.TrimSpace(bad), ";")
}

// asWorn picks the piece a synthetic character wears in a slot: the middle one by item level of what the class may wear there, so some pieces beat it and
// some do not.
func asWorn(its []asItem) (asItem, bool) {
	if len(its) < 3 {
		return asItem{}, false
	}
	s := append([]asItem(nil), its...)
	sort.Slice(s, func(i, j int) bool {
		if s[i].ilvl != s[j].ilvl {
			return s[i].ilvl < s[j].ilvl
		}
		return s[i].id < s[j].id
	})
	return s[len(s)/2], true
}

// asFind: for every build, level and faction of the class, the pieces the finder offers in each armor slot are the pieces the rows allow, with the slots empty
// and with a middle piece worn in each slot (only the pieces of a higher item level are upgrades, best first).
func asFind(t *testing.T, area, title, class string) {
	t.Helper()
	var out []benchRow
	ds := usabStore()
	classID := grClassOf(class)
	for _, b := range asBuilds[class] {
		for i, lv := range asLevels {
			faction := []string{"Alliance", "Horde"}[i%2]
			level := lv
			ref := asAllowed(t, class, classID, level, faction)
			empty := character.Character{Name: "Sweep", Class: class, Faction: faction, Level: &level, Profile: &character.Profile{Role: b.role, Focus: "leveling", Spec: b.spec}}
			res, err := gear.Find(empty, ds, "", gear.Options{NoSources: true})
			if err != nil {
				t.Fatal(err)
			}
			// worn by item level alone (a capture with no item id): exactly the pieces of a higher item level are upgrades
			wornIL := map[string]int64{}
			byLevel, byRow := empty, empty
			byLevel.Equipment, byRow.Equipment = map[string]character.EquippedItem{}, map[string]character.EquippedItem{}
			wornID := map[string]int64{}
			for _, sl := range asSlots {
				if w, ok := asWorn(ref[sl.name]); ok {
					id, il := int(w.id), int(w.ilvl)
					byLevel.Equipment[sl.name] = character.EquippedItem{Slot: sl.name, ItemLevel: &il, Name: w.name}
					byRow.Equipment[sl.name] = character.EquippedItem{Slot: sl.name, ItemID: &id, ItemLevel: &il, Name: w.name}
					wornIL[sl.name], wornID[sl.name] = w.ilvl, w.id
				}
			}
			resL, err := gear.Find(byLevel, ds, "", gear.Options{NoSources: true})
			if err != nil {
				t.Fatal(err)
			}
			resR, err := gear.Find(byRow, ds, "", gear.Options{NoSources: true})
			if err != nil {
				t.Fatal(err)
			}
			for _, sl := range asSlots {
				// empty slot: every allowed piece
				extra, missing := asDiff(res.Slots[sl.name], asIDs(ref[sl.name]))
				out = append(out, benchRow{fmt.Sprintf("%s: %s %s level %d, %s: what fits the empty %s slot", sl.name, class, b.spec, lv, faction, sl.name),
					fmt.Sprintf("%d offered", len(res.Slots[sl.name])), fmt.Sprintf("%d allowed", len(ref[sl.name])), asSrc, resultIf(extra == nil && missing == nil, asBad(extra, missing))})
				il, ok := wornIL[sl.name]
				if !ok {
					continue
				}
				// worn, known by item level: the allowed pieces of a higher item level than the worn piece
				want := map[int64]asItem{}
				for _, it := range ref[sl.name] {
					if it.ilvl > il {
						want[it.id] = it
					}
				}
				extra, missing = asDiff(resL.Slots[sl.name], want)
				bad := asBad(extra, missing)
				if !asOrdered(resL.Slots[sl.name]) {
					bad += "; not best first"
				}
				out = append(out, benchRow{fmt.Sprintf("%s: %s %s level %d, %s: upgrades over a worn item level %d in the %s slot", sl.name, class, b.spec, lv, faction, il, sl.name),
					fmt.Sprintf("%d offered", len(resL.Slots[sl.name])), fmt.Sprintf("%d of a higher item level", len(want)), asSrc, resultIf(bad == "", bad)})
				// worn, a row of the dataset: weighed by stats, so a piece of a lower item level may beat it; every offer is an allowed piece, never the worn one, with a gain
				extra, _ = asDiff(resR.Slots[sl.name], asIDs(ref[sl.name]))
				bad = ""
				if len(extra) > 0 {
					bad = fmt.Sprintf("offers %v the rows do not allow", extra)
				}
				for _, cd := range resR.Slots[sl.name] {
					if cd.Item.ID == wornID[sl.name] || cd.Delta <= 0 && !cd.Empty {
						bad += fmt.Sprintf("; offers the worn or a no-gain piece %d", cd.Item.ID)
					}
				}
				if !asOrdered(resR.Slots[sl.name]) {
					bad += "; not best first"
				}
				out = append(out, benchRow{fmt.Sprintf("%s: %s %s level %d, %s: upgrades over the worn piece %d (weighed by stats) in the %s slot", sl.name, class, b.spec, lv, faction, wornID[sl.name], sl.name),
					fmt.Sprintf("%d offered", len(resR.Slots[sl.name])), "allowed pieces with a gain, best first", asSrc, resultIf(bad == "", strings.TrimPrefix(bad, "; "))})
			}
		}
	}
	finish(t, area, title, out)
}

func TestOfflineBenchmarkSweepArmorSlotsWarrior(t *testing.T) {
	asFind(t, "gear-sweep-armor-slots-warrior", "Offline armor-slot sweep: warrior", "WARRIOR")
}

func TestOfflineBenchmarkSweepArmorSlotsPaladin(t *testing.T) {
	asFind(t, "gear-sweep-armor-slots-paladin", "Offline armor-slot sweep: paladin", "PALADIN")
}

func TestOfflineBenchmarkSweepArmorSlotsHunter(t *testing.T) {
	asFind(t, "gear-sweep-armor-slots-hunter", "Offline armor-slot sweep: hunter", "HUNTER")
}

func TestOfflineBenchmarkSweepArmorSlotsRogue(t *testing.T) {
	asFind(t, "gear-sweep-armor-slots-rogue", "Offline armor-slot sweep: rogue", "ROGUE")
}

func TestOfflineBenchmarkSweepArmorSlotsPriest(t *testing.T) {
	asFind(t, "gear-sweep-armor-slots-priest", "Offline armor-slot sweep: priest", "PRIEST")
}

func TestOfflineBenchmarkSweepArmorSlotsShaman(t *testing.T) {
	asFind(t, "gear-sweep-armor-slots-shaman", "Offline armor-slot sweep: shaman", "SHAMAN")
}

func TestOfflineBenchmarkSweepArmorSlotsMage(t *testing.T) {
	asFind(t, "gear-sweep-armor-slots-mage", "Offline armor-slot sweep: mage", "MAGE")
}

func TestOfflineBenchmarkSweepArmorSlotsWarlock(t *testing.T) {
	asFind(t, "gear-sweep-armor-slots-warlock", "Offline armor-slot sweep: warlock", "WARLOCK")
}

func TestOfflineBenchmarkSweepArmorSlotsDruid(t *testing.T) {
	asFind(t, "gear-sweep-armor-slots-druid", "Offline armor-slot sweep: druid", "DRUID")
}

var asBagLevels = []int{9, 25, 39, 40, 50}

// asBagPick is the piece of the armor type in the slot with the highest item level whose required level is at most max (an unrestricted one), and a piece of the
// same type that needs more than the level; ok is false when the snapshot holds none.
func asBagPick(t *testing.T, class string, classID int64, slot string, sub int64, level int, faction string, tooHigh bool) (asItem, bool) {
	t.Helper()
	var best asItem
	found := false
	for _, it := range asItems(t) {
		if it.slot != slot || it.sub != sub || !maskAllows(it.mask, classID) || it.qual <= 0 || it.qual == 6 || it.bond == 4 || it.bond == 5 || it.ilvl <= 1 ||
			itemdata.Unobtainable(it.name) || itemdata.WrongFaction(it.name, faction) {
			continue
		}
		r, _ := gear.ReqLevelOf(int(it.req), int(it.ilvl))
		if tooHigh != (r > level) {
			continue
		}
		if !found || it.ilvl > best.ilvl || it.ilvl == best.ilvl && it.id < best.id {
			best, found = it, true
		}
	}
	return best, found
}

// TestOfflineBenchmarkSweepArmorSlotsFromBags: a piece of each armor type in the bags for each armor slot of an unequipped character, and what the equip-from-bags
// list offers: only the piece of the armor type the class wears at its level, and only when its required level is not above the character's level.
func TestOfflineBenchmarkSweepArmorSlotsFromBags(t *testing.T) {
	ds := usabStore()
	var out []benchRow
	for _, cls := range usabClasses {
		for _, lv := range asBagLevels {
			level := lv
			faction := []string{"Alliance", "Horde"}[lv%2]
			c := character.Character{Name: "Sweep", Class: cls.token, Faction: faction, Level: &level, Equipment: map[string]character.EquippedItem{}, Profile: &character.Profile{Role: "damage", Focus: "leveling"}}
			type bagged struct {
				it       asItem
				slot     string
				sub      int64
				tooHigh  bool
				mustFit  bool
				fitsType bool
			}
			var all []bagged
			for _, sl := range asSlots {
				for _, sub := range []int64{1, 2, 3, 4} {
					for _, high := range []bool{false, true} {
						if it, ok := asBagPick(t, cls.token, cls.id, sl.name, sub, level, faction, high); ok {
							all = append(all, bagged{it, sl.name, sub, high, !high && sub == asArmorAt(cls.token, level), sub == asArmorAt(cls.token, level)})
							c.Bags = append(c.Bags, character.ItemStack{ItemID: int(it.id), Name: it.name, Count: 1})
						}
					}
				}
			}
			got := map[string]string{}
			for _, u := range gear.BagUpgrades(c, ds, "") {
				got[u.Slot] = u.Name
			}
			for _, sl := range asSlots {
				// the pieces of the slot in the bags that the class may wear now: the right type with a required level up to the character's
				var want []string
				var held []string
				for _, b := range all {
					if b.slot != sl.name {
						continue
					}
					r, _ := gear.ReqLevelOf(int(b.it.req), int(b.it.ilvl))
					held = append(held, fmt.Sprintf("%s (type %d, needs %d)", b.it.name, b.sub, r))
					if b.mustFit && b.slot != "back" {
						want = append(want, b.it.name)
					}
				}
				if sl.name == "back" {
					want = nil
					for _, b := range all {
						if b.slot == "back" && !b.tooHigh && (b.sub == 1 || b.sub == 0) {
							want = append(want, b.it.name)
						}
					}
				}
				// the list names one piece per slot: the one it likes best of those allowed; a slot with none allowed names none
				ok := (len(want) == 0 && got[sl.name] == "") || (len(want) > 0 && got[sl.name] != "" && contains(want, got[sl.name]))
				ad := got[sl.name]
				if ad == "" {
					ad = "nothing to equip"
				}
				wnt := "nothing to equip"
				if len(want) > 0 {
					wnt = "one of " + strings.Join(want, ", ")
				}
				out = append(out, benchRow{fmt.Sprintf("%s: %s level %d: bags hold %s", sl.name, strings.ToLower(cls.token), lv, strings.Join(held, "; ")), ad, wnt, asBagSrc,
					resultIf(ok, "the piece of the wrong armor type, one that needs a higher level, or a usable piece left out")})
			}
		}
	}
	finish(t, "gear-sweep-armor-slots-bags", "Offline armor-slot sweep: the equip-from-bags list", out)
}

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

// TestOfflineBenchmarkSweepArmorSlotsFromBagsWorn: a piece of the class's armor type in the bags for a slot with a piece worn there, known by item level and
// quality only (no stats to weigh): the list offers it when its item level is above the worn piece's, or equal with a higher quality, and never otherwise.
func TestOfflineBenchmarkSweepArmorSlotsFromBagsWorn(t *testing.T) {
	ds := usabStore()
	var out []benchRow
	for _, cls := range usabClasses {
		for _, lv := range []int{25, 39, 40, 55} {
			level := lv
			faction := []string{"Alliance", "Horde"}[lv%2]
			for _, sl := range asSlots {
				piece, ok := asBagPick(t, cls.token, cls.id, sl.name, asArmorAt(cls.token, level), level, faction, false)
				if !ok || sl.name == "back" && piece.sub != 1 {
					continue
				}
				for _, d := range []struct {
					what     string
					il, q    int
					offered  bool
					wornName string
				}{
					{"worn 3 item levels lower", int(piece.ilvl) - 3, int(piece.qual), true, ""},
					{"worn 3 item levels higher", int(piece.ilvl) + 3, int(piece.qual), false, ""},
					{"worn at the same item level and quality", int(piece.ilvl), int(piece.qual), false, ""},
					{"worn at the same item level, one quality lower", int(piece.ilvl), int(piece.qual) - 1, true, ""},
				} {
					il, q := d.il, d.q
					c := character.Character{Name: "Sweep", Class: cls.token, Faction: faction, Level: &level, Profile: &character.Profile{Role: "damage", Focus: "leveling"},
						Equipment: map[string]character.EquippedItem{sl.name: {Slot: sl.name, ItemLevel: &il, Quality: &q, Name: "Worn piece"}},
						Bags:      []character.ItemStack{{ItemID: int(piece.id), Name: piece.name, Count: 1}}}
					var got string
					for _, u := range gear.BagUpgrades(c, ds, "") {
						if u.Slot == sl.name {
							got = u.Name
						}
					}
					ad, w := "nothing to equip", "nothing to equip"
					if got != "" {
						ad = got
					}
					if d.offered {
						w = piece.name
					}
					out = append(out, benchRow{fmt.Sprintf("%s: %s level %d: %s (%s at item level %d, quality %d) in the bags, %s", sl.name, strings.ToLower(cls.token), lv, piece.name, "type "+fmt.Sprint(piece.sub), piece.ilvl, piece.qual, d.what), ad, w,
						asBagSrc + "; docs: item level decides, quality breaks a tie", resultIf(ad == w, "a piece offered that is no gain, or an upgrade left out")})
				}
			}
		}
	}
	finish(t, "gear-sweep-armor-slots-bags-worn", "Offline armor-slot sweep: the equip-from-bags list against a worn piece", out)
}

// asLadder is the armor line the rules facts must give a class: the game's rule (plate and mail from 40), as the sentence the facts use.
var asLadder = map[string]string{
	"WARRIOR": "Armor: mail until level 40, then plate.", "PALADIN": "Armor: mail until level 40, then plate.",
	"HUNTER": "Armor: leather until level 40, then mail.", "SHAMAN": "Armor: leather until level 40, then mail.",
	"ROGUE": "Armor: leather at every level.", "DRUID": "Armor: leather at every level.",
	"PRIEST": "Armor: cloth at every level.", "MAGE": "Armor: cloth at every level.", "WARLOCK": "Armor: cloth at every level.",
}

// asTemplates are everyday ways to ask which armor a class wears and when it changes; %s is the class (a singular, with "a" or "my" before it where the sentence
// has one) and %p its plural.
var asTemplates = []string{
	"what armor can my %s wear?", "can a %s wear plate?", "when does a %s get plate armor?", "when do %p get mail?", "what level do %p get plate?",
	"when does a %s switch to plate?", "what armor type should my %s use?", "which armor does a %s wear", "can my %s wear cloth?",
	"is leather or mail better for a %s?", "can a %s wear chain mail", "what kind of armor can %p equip", "armor proficiency for %p",
	"mail or plate for my %s at level 45?", "does a %s wear leather", "when can my %s start wearing mail?",
}

// asSlotQuestions are everyday ways to ask whether a class wears a piece for an armor slot; %s is the class, the second %s the slot word.
var asSlotQuestions = []struct{ q, slot string }{
	{"what helm can a %s wear?", "head"}, {"can my %s wear a helmet", "head"}, {"what shoulders can a %s wear", "shoulder"}, {"can my %s wear pauldrons?", "shoulder"},
	{"what chest armor can a %s wear?", "chest"}, {"can a %s wear a breastplate", "chest"}, {"what bracers can my %s wear", "wrist"}, {"can a %s wear wristguards", "wrist"},
	{"what gloves can a %s wear?", "hands"}, {"can my %s wear gauntlets", "hands"}, {"what belt can a %s wear", "waist"}, {"can a %s wear a girdle?", "waist"},
	{"what pants can my %s wear?", "legs"}, {"can a %s wear leggings", "legs"}, {"what boots can a %s wear", "feet"}, {"can my %s wear greaves?", "feet"},
	{"can a %s wear a plate helm", "head"}, {"can my %s wear a chain helm?", "head"}, {"can a %s wear leather boots", "feet"}, {"can my %s wear cloth gloves", "hands"},
	{"can my %s wear a cloak?", "back"}, {"what cloak can a %s wear", "back"},
}

var asPlural = map[string]string{"WARRIOR": "warriors", "PALADIN": "paladins", "HUNTER": "hunters", "ROGUE": "rogues", "PRIEST": "priests", "SHAMAN": "shamans", "MAGE": "mages", "WARLOCK": "warlocks", "DRUID": "druids"}

// asNotArmor are questions that use an armor or slot word and ask nothing about armor proficiency.
var asNotArmor = []string{
	"where is the chest in the Deadmines?", "how do I open the locked chest in Duskwood", "where do I buy a plate of Lean Wolf Steak", "when do I get my mail from the mailbox?",
	"what level do I get the mail from the quest", "when does the next boat leave Ratchet", "how do I get the leather for my belt", "can I wear a seatbelt in the zeppelin",
	"what is the best leather for Leatherworking at skill 150", "where do I find cloth to level tailoring", "can I mail my gloves to my alt?", "which feet of the dragon is lava",
}

// TestOfflineBenchmarkSweepArmorSlotPhrasings: everyday ways to ask which armor each class wears, when the plate and mail classes gain their type, and whether a
// class wears a piece for an armor slot reach the rules facts, and the facts give the class's armor line (the game's rule: plate and mail from 40).
func TestOfflineBenchmarkSweepArmorSlotPhrasings(t *testing.T) {
	ds := usabStore()
	var out []benchRow
	check := func(slot, class, q string) {
		asker := "MAGE"
		if class == "MAGE" {
			asker = "WARRIOR"
		}
		facts := ask.ProficiencyFacts(usabChar(asker, 30, "Horde"), ds, q)
		want := titleCase(class) + ":"
		bad := ""
		if !strings.Contains(facts, want) {
			bad = "no rule lines for " + strings.ToLower(class)
		} else if !strings.Contains(facts, asLadder[class]) {
			bad = "facts lack " + asLadder[class]
		}
		got := "the rules facts give the class's armor line"
		if bad != "" {
			got = bad
		}
		out = append(out, benchRow{fmt.Sprintf("%s: %q", slot, q), got, asLadder[class] + " (" + strings.ToLower(class) + ")", asPhraseSr, resultIf(bad == "", bad)})
	}
	for _, cls := range usabClasses {
		for _, tp := range asTemplates {
			q := strings.ReplaceAll(strings.ReplaceAll(tp, "%p", asPlural[cls.token]), "%s", strings.ToLower(cls.token))
			check("armor type", cls.token, q)
		}
		for _, sq := range asSlotQuestions {
			check(sq.slot, cls.token, fmt.Sprintf(sq.q, strings.ToLower(cls.token)))
		}
	}
	for _, q := range asNotArmor {
		f := ask.ProficiencyFacts(usabChar("MAGE", 30, "Horde"), ds, q)
		out = append(out, benchRow{fmt.Sprintf("not an armor question: %q", q), fmt.Sprintf("facts added: %v", f != ""), "no class rule lines added", asPhraseSr, resultIf(f == "", "rule lines for a question that is none")})
	}
	finish(t, "gear-sweep-armor-slots-phrasings", "Offline armor-slot sweep: armor type and armor slot questions in a player's words", out)
}

func titleCase(class string) string { return class[:1] + strings.ToLower(class[1:]) }

// asReviewClass is the bag review for a piece of each armor type in each slot, for a class at three levels: keep what the class wears (from the level the piece needs,
// the type judged at that level) and sell what it never wears, beside the dataset's rows and the game's armor rule.
func asReviewClass(t *testing.T, area, title string, classes ...string) {
	t.Helper()
	ds := usabStore()
	var out []benchRow
	for _, class := range classes {
		classID := grClassOf(class)
		for _, lv := range asReviewLevels(class) {
			level := lv
			faction := []string{"Alliance", "Horde"}[lv%2]
			for _, sl := range asSlots {
				for _, sub := range []int64{1, 2, 3, 4} {
					for _, high := range []bool{false, true} {
						if high && sub != asArmorAt(class, level) && sub != asArmorAt(class, 60) {
							continue
						}
						it, ok := asBagPick(t, class, classID, sl.name, sub, level, faction, high)
						if !ok {
							continue
						}
						req, _ := gear.ReqLevelOf(int(it.req), int(it.ilvl))
						c := usabChar(class, level, faction)
						c.Name, c.BagsCaptured, c.BankCaptured = "Benchtest", true, true
						c.Bags = []character.ItemStack{{ItemID: int(it.id), Count: 1, Name: it.name}}
						res := inventory.ReviewAll([]character.Character{c}, ds)
						act, reason := inventory.Action(""), "no line"
						if len(res.Lines) == 1 {
							act, reason = res.Lines[0].Action, string(res.Lines[0].Action)+": "+res.Lines[0].Reason
						}
						// the game: a back piece is cloth for every class; other slots by the type the class wears at the level the piece needs
						wears := sl.name == "back" || sub == asArmorAt(class, max(level, req))
						want := inventory.Sell
						if wears {
							want = inventory.Keep_
						}
						ok2 := act == want && (!wears || req <= level || strings.Contains(reason, fmt.Sprintf("level %d", req)))
						out = append(out, benchRow{fmt.Sprintf("%s: %s level %d, %s (type %d, needs level %d) in the bags", sl.name, strings.ToLower(class), level, it.name, sub, req), reason,
							fmt.Sprintf("%s (wears it from level %d: %v)", want, req, wears), asSrc, resultIf(ok2, "the action, or the level it can be worn from")})
					}
				}
			}
		}
	}
	finish(t, area, title, out)
}

func TestOfflineBenchmarkSweepArmorSlotsReviewWarrior(t *testing.T) {
	asReviewClass(t, "gear-sweep-armor-slots-review-warrior", "Offline armor-slot sweep: the bag review for armor, warrior", "WARRIOR")
}

func TestOfflineBenchmarkSweepArmorSlotsReviewPaladin(t *testing.T) {
	asReviewClass(t, "gear-sweep-armor-slots-review-paladin", "Offline armor-slot sweep: the bag review for armor, paladin", "PALADIN")
}

func TestOfflineBenchmarkSweepArmorSlotsReviewHunter(t *testing.T) {
	asReviewClass(t, "gear-sweep-armor-slots-review-hunter", "Offline armor-slot sweep: the bag review for armor, hunter", "HUNTER")
}

func TestOfflineBenchmarkSweepArmorSlotsReviewRogue(t *testing.T) {
	asReviewClass(t, "gear-sweep-armor-slots-review-rogue", "Offline armor-slot sweep: the bag review for armor, rogue", "ROGUE")
}

func TestOfflineBenchmarkSweepArmorSlotsReviewPriest(t *testing.T) {
	asReviewClass(t, "gear-sweep-armor-slots-review-priest", "Offline armor-slot sweep: the bag review for armor, priest", "PRIEST")
}

func TestOfflineBenchmarkSweepArmorSlotsReviewShaman(t *testing.T) {
	asReviewClass(t, "gear-sweep-armor-slots-review-shaman", "Offline armor-slot sweep: the bag review for armor, shaman", "SHAMAN")
}

func TestOfflineBenchmarkSweepArmorSlotsReviewMage(t *testing.T) {
	asReviewClass(t, "gear-sweep-armor-slots-review-mage", "Offline armor-slot sweep: the bag review for armor, mage", "MAGE")
}

func TestOfflineBenchmarkSweepArmorSlotsReviewWarlock(t *testing.T) {
	asReviewClass(t, "gear-sweep-armor-slots-review-warlock", "Offline armor-slot sweep: the bag review for armor, warlock", "WARLOCK")
}

func TestOfflineBenchmarkSweepArmorSlotsReviewDruid(t *testing.T) {
	asReviewClass(t, "gear-sweep-armor-slots-review-druid", "Offline armor-slot sweep: the bag review for armor, druid", "DRUID")
}

// asReviewLevels: the plate and mail classes are reviewed at 41, just past the level their armor type changes (with the pieces that need 42 and 43 still ahead);
// the others at 30.
func asReviewLevels(class string) []int {
	if class == "WARRIOR" || class == "PALADIN" || class == "HUNTER" || class == "SHAMAN" {
		return []int{41}
	}
	return []int{30}
}

const asSrcLoot = "dataset loot_sources (source_kind, source_id, kills, drops), npcs (name, level)"

// TestOfflineBenchmarkSweepArmorSlotsSources: where the finder says an armor piece comes from beside the dataset's loot_sources rows: a creature's drop chance is its drops over
// its kills (unknown only when it has no kills), a vendor's stock and a recipe have no drop chance and show no bare id, and the piece lists at most three sources.
func TestOfflineBenchmarkSweepArmorSlotsSources(t *testing.T) {
	ds := usabStore()
	var out []benchRow
	for _, cls := range usabClasses {
		for _, lv := range []int{20, 45} {
			level := lv
			c := character.Character{Name: "Sweep", Class: cls.token, Faction: "Horde", Level: &level, Profile: &character.Profile{Role: "damage", Focus: "leveling"}}
			res, err := gear.Find(c, ds, "", gear.Options{Limit: 2})
			if err != nil {
				t.Fatal(err)
			}
			for _, sl := range asSlots {
				for _, cd := range res.Slots[sl.name] {
					rows, err := ds.Filter("loot_sources", store.Range{Column: "item_id", Min: cd.Item.ID, Max: cd.Item.ID})
					if err != nil {
						t.Fatal(err)
					}
					bad := ""
					if len(cd.Sources) != min(len(rows), gear.MaxSources) {
						bad = fmt.Sprintf("%d sources listed for %d rows", len(cd.Sources), len(rows))
					}
					var lines []string
					for _, s := range cd.Sources {
						line := s.String()
						lines = append(lines, line)
						switch s.Kind {
						case "sold by", "crafted":
							if strings.Contains(line, "drop chance") || strings.Contains(line, fmt.Sprint(s.ID)) {
								bad += "; a vendor or recipe described as a drop or by its id: " + line
							}
						case "creature":
							var kills, drops int64
							for _, r := range rows {
								if text(r["source_kind"]) == "creature" && num(r["source_id"]) == s.ID {
									kills, drops = num(r["kills"]), num(r["drops"])
								}
							}
							wantChance := "drop chance unknown"
							if kills > 0 {
								wantChance = fmt.Sprintf("drops %.1f%% of kills", 100*float64(drops)/float64(kills))
							}
							if !strings.Contains(line, wantChance) {
								bad += "; chance differs from drops over kills: " + line
							}
						}
					}
					ad := strings.Join(lines, " | ")
					if ad == "" {
						ad = "source unknown"
					}
					out = append(out, benchRow{fmt.Sprintf("%s: %s level %d: where to get %s", sl.name, strings.ToLower(cls.token), lv, cd.Item.Name), ad, fmt.Sprintf("%d loot_sources rows, at most %d listed", len(rows), gear.MaxSources), asSrcLoot,
						resultIf(bad == "", strings.TrimPrefix(bad, "; "))})
				}
			}
		}
	}
	finish(t, "gear-sweep-armor-slots-sources", "Offline armor-slot sweep: where the finder says armor comes from", out)
}

const asSetSrc = "dataset item_sets (set_id, item_id, name), items (slot, class_id, subclass_id, required_level, bonding, allowable_class_mask); the dataset holds no set bonus"

var asSetQuestions = []string{"which pieces make up the %s set?", "what are the pieces of the %s set", "what slots does the %s set fill, and what armor type is each piece?"}

// TestOfflineBenchmarkSweepArmorSlotsSetPieces: for every item set of the snapshot, each everyday way to ask for its pieces gets every piece the dataset holds, named with its slot,
// armor type, class limit, required level and binding from its item row, and a piece with no item row is said to be unjudgeable.
func TestOfflineBenchmarkSweepArmorSlotsSetPieces(t *testing.T) {
	ds := store.NewDir("../../testdata/items_snapshot")
	rows, err := ds.Filter("item_sets", store.Range{Column: "set_id", Min: 0, Max: 1 << 40})
	if err != nil {
		t.Fatal(err)
	}
	type set struct {
		id     int64
		name   string
		pieces []store.Row
	}
	byID := map[int64]*set{}
	var order []int64
	for _, r := range rows {
		id := num(r["set_id"])
		if byID[id] == nil {
			byID[id] = &set{id: id, name: text(r["name"])}
			order = append(order, id)
		}
		byID[id].pieces = append(byID[id].pieces, r)
	}
	nameCount := map[string]int{}
	for _, s := range byID {
		nameCount[s.name]++
	}
	sort.Slice(order, func(i, j int) bool { return order[i] < order[j] })
	var out []benchRow
	for _, id := range order {
		s := byID[id]
		if nameCount[s.name] > 1 {
			continue // two sets with one name cannot be told apart by a question
		}
		var want []string
		missing := 0
		for _, r := range s.pieces {
			it, err := ds.Get("items", num(r["item_id"]))
			if err != nil || it == nil || text(it["name"]) == "" {
				missing++
				continue
			}
			want = append(want, fmt.Sprintf("%s (%s)", text(it["name"]), gear.PieceText(it)))
		}
		for _, qt := range asSetQuestions {
			q := fmt.Sprintf(qt, s.name)
			facts := ask.SetFacts(ds, q)
			var lack []string
			for _, w := range want {
				if !strings.Contains(facts, w) {
					lack = append(lack, w)
				}
			}
			if missing > 0 && !strings.Contains(facts, fmt.Sprintf("%d of its items are not in the items table", missing)) {
				lack = append(lack, "the pieces the items table lacks")
			}
			if !strings.Contains(facts, fmt.Sprintf("%d pieces in the dataset", len(s.pieces))) {
				lack = append(lack, "the piece count")
			}
			bad := ""
			if len(lack) > 0 {
				bad = fmt.Sprintf("facts lack %q", lack)
			}
			// the slot and armor type each piece row holds, said independently of the finder's text
			for _, r := range s.pieces {
				it, err := ds.Get("items", num(r["item_id"]))
				if err != nil || it == nil {
					continue
				}
				if w := gear.ArmorTypeWord(int(num(it["class_id"])), int(num(it["subclass_id"]))); w != "" && num(it["slot"]) != 16 && !strings.Contains(facts, text(it["name"])+" ("+asSlotWord(num(it["slot"]))+", "+w) {
					bad += fmt.Sprintf("; %s is %s %s", text(it["name"]), asSlotWord(num(it["slot"])), w)
				}
			}
			out = append(out, benchRow{fmt.Sprintf("set pieces: %q", q), fmt.Sprintf("%d pieces named", len(want)), fmt.Sprintf("%d of %d pieces with slot, armor type and level", len(want), len(s.pieces)), asSetSrc, resultIf(bad == "", strings.TrimPrefix(bad, "; "))})
		}
	}
	finish(t, "gear-sweep-armor-slots-set-pieces", "Offline armor-slot sweep: the pieces of an item set", out)
}

func asSlotWord(inv int64) string {
	switch inv {
	case 1:
		return "head"
	case 3:
		return "shoulder"
	case 5, 20:
		return "chest"
	case 6:
		return "waist"
	case 7:
		return "legs"
	case 8:
		return "feet"
	case 9:
		return "wrist"
	case 10:
		return "hands"
	case 16:
		return "back"
	}
	return fmt.Sprint(inv)
}

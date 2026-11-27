package paths_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/ask"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/inventory"
	"github.com/WoW-Companion/advisor/internal/levelup"
	"github.com/WoW-Companion/advisor/internal/petfood"
	"github.com/WoW-Companion/advisor/internal/store"
)

// The offline hunter pet benchmark: what the advisor does with the food a hunter's pet eats, what it says of a pet's abilities, ranks, training and taming, beside the dataset's
// rows and the game's own rules, with no network. It reads testdata/consumables_snapshot (food, class 0 subclass 5, and meat and fish, class 7 subclass 8), testdata/usability_snapshot
// (class_spells for the hunter and the druid, talent_nodes, npcs). The dataset holds no pet diet (no item row says what a pet eats), no beast family name or tameable flag (npcs has
// a numeric family_id), no pet ability row (the Growl, Claw, Cower, Dash and Prowl rows in class_spells are the druid's), no pet training cost, loyalty or stable rule, so the cases of those check
// that the answer is framed with the line that says so (ask.LimitFacts) and that nothing is stated from memory. What it does hold, the level of each hunter pet spell and rank, is checked
// against the advisor's training list. A difference the dataset's own rows cause is marked "(data)". make offline-benchmark runs it; WOWC_BENCH_REPORT receives the tables.

const (
	hpFoodSrc  = "dataset items (class_id 0 subclass_id 5 food, class_id 7 subclass_id 8 meat and fish; bonding, required_level, sell_price; no diet or food-type column); game: a hunter's pet eats Meat, Fish, Bread, Cheese, Fruit or Fungus by family (Warcraft Wiki, Pet food)"
	hpSpellSrc = "dataset class_spells (class_id 3: name, rank, level, skill_line); Warcraft Wiki hunter spell pages"
	hpDruidSrc = "dataset class_spells (class_id 11 Feral Combat: Claw, Cower, Dash, Growl, Prowl; no class_id 3 row of those names); game: a pet's Growl, Claw, Cower, Dash and Prowl are the pet's own abilities, taught by pet skill books and Beast Training (Warcraft Wiki, Pet abilities)"
	hpTalSrc   = "dataset talent_nodes and class_spells (class_id 3 Beast Mastery); game: Intimidation, Bestial Wrath and Improved Mend Pet are talents (Warcraft Wiki, Beast Mastery)"
	hpNoneSrc  = "the dataset has no such row, column or table (npcs: creature_type, family_id as a number only; class_spells: the hunter's own spells; items: no diet)"
)

// hpHunter is a hunter holding the named foods, from the consumables snapshot.
func hpHunter(t *testing.T, class, name string, count int, foods ...string) character.Character {
	t.Helper()
	c := usabChar(class, 30, "Horde")
	c.Name, c.BagsCaptured, c.BankCaptured, c.Equipment = name, true, true, map[string]character.EquippedItem{}
	for _, f := range foods {
		c.Bags = append(c.Bags, character.ItemStack{ItemID: int(num(rowByName(t, consStore(), "items", f)["id"])), Count: count, Name: f})
	}
	return c
}

func hpLine(res inventory.CrossResult, who, item string) inventory.Line {
	l, _ := kaLine(res, who, item)
	return l
}

var hpFoods = []string{"Tough Jerky", "Haunch of Meat", "Smoked Bear Meat", "Chunk of Boar Meat", "Boar Ribs", "Lean Wolf Flank", "Gooey Spider Leg", "Strider Meat",
	"Slitherskin Mackerel", "Raw Slitherskin Mackerel", "Shiny Red Apple", "Darnassian Bleu", "Spongy Morel"}

// TestOfflineBenchmarkPetFoodReview: a hunter keeps the food a pet eats, the raw meat and fish included, whatever the bag review would do with it for any other class.
// The review before the fix left every one of them "no rule places it".
func TestOfflineBenchmarkPetFoodReview(t *testing.T) {
	ds := consStore()
	var out []benchRow
	for _, f := range hpFoods {
		row := rowByName(t, ds, "items", f)
		diet, _ := petfood.Of(f)
		res := inventory.Review(hpHunter(t, "HUNTER", "Hunt", 20, f), ds)
		var g inventory.Group
		var why string
		for gr, es := range res.Groups {
			for _, e := range es {
				g, why = gr, e.Reason
			}
		}
		out = append(out, benchRow{f + " in a hunter's bags: the bag review", fmt.Sprintf("%s: %s", g, why), fmt.Sprintf("item class %d subclass %d, bonding %d; game: %s, pet food: kept", num(row["class_id"]), num(row["subclass_id"]), num(row["bonding"]), diet), hpFoodSrc,
			resultIf(g == inventory.Keep && strings.Contains(why, "pet food ("+diet+")"), "pet food was not kept as pet food")})
	}
	finish(t, "advice-pet-food-review", "Offline hunter pets: a hunter keeps pet food", out)
}

// TestOfflineBenchmarkPetFoodAlts: the review across a hunter and other characters: the hunter's food is not sold, another class's pet food goes to the hunter alt, and with no
// hunter nothing changes.
func TestOfflineBenchmarkPetFoodAlts(t *testing.T) {
	ds := consStore()
	var out []benchRow
	for _, f := range []string{"Boar Ribs", "Lean Wolf Flank", "Raw Slitherskin Mackerel", "Strider Meat"} {
		res := inventory.ReviewAll([]character.Character{hpHunter(t, "HUNTER", "Hunt", 20, f), hpHunter(t, "MAGE", "Mage", 1)}, ds)
		l := hpLine(res, "Hunt", f)
		out = append(out, benchRow{f + " in a hunter's bags, a mage alt: the review across both", fmt.Sprintf("%s %q: %s", l.Action, l.To, l.Reason), "keep: the hunter's pet eats it; not sold on the Auction House for a Cooking nobody has", hpFoodSrc,
			resultIf(l.Action == inventory.Keep_ && !l.AH && l.To == "", "pet food was sold or mailed away")})
	}
	for _, f := range []string{"Boar Ribs", "Tough Jerky", "Darnassian Bleu", "Raw Slitherskin Mackerel"} {
		res := inventory.ReviewAll([]character.Character{hpHunter(t, "MAGE", "Mage", 20, f), hpHunter(t, "HUNTER", "Hunt", 1)}, ds)
		l := hpLine(res, "Mage", f)
		out = append(out, benchRow{f + " in a mage's bags, a hunter alt: the review across both", fmt.Sprintf("%s %q: %s", l.Action, l.To, l.Reason), "mail to the hunter (bonding 0: it can be mailed)", hpFoodSrc,
			resultIf(l.Action == inventory.Mail && l.To == "Hunt", "pet food was not mailed to the hunter alt")})
	}
	res := inventory.ReviewAll([]character.Character{hpHunter(t, "HUNTER", "Hunt", 20, "Tough Jerky", "Haunch of Meat")}, ds)
	for _, f := range []string{"Tough Jerky", "Haunch of Meat"} {
		l := hpLine(res, "Hunt", f)
		out = append(out, benchRow{f + ": a hunter holding Tough Jerky and Haunch of Meat", fmt.Sprintf("%s: %s", l.Action, l.Reason), "keep both: each is food a pet eats; the lower one is no surplus", hpFoodSrc,
			resultIf(l.Action == inventory.Keep_, "the lesser food was sold as a surplus of food")})
	}
	res = inventory.ReviewAll([]character.Character{hpHunter(t, "MAGE", "Mage", 20, "Boar Ribs"), hpHunter(t, "WARRIOR", "War", 1)}, ds)
	l := hpLine(res, "Mage", "Boar Ribs")
	out = append(out, benchRow{"Boar Ribs in a mage's bags, no hunter and no cook: the review across both", fmt.Sprintf("%s %q: %s", l.Action, l.To, l.Reason), "not mailed to a non-hunter; sold: no character's class or profession uses it", hpFoodSrc,
		resultIf(l.Action == inventory.Sell && l.To == "", "pet food was mailed to a class with no pet")})
	finish(t, "advice-pet-food-alts", "Offline hunter pets: pet food across characters", out)
}

// TestOfflineBenchmarkPetFoodNotOthers: the pet food rule is a hunter's: another class's bag review does not keep the same item as pet food, and the dataset's food rows say nothing of a diet.
func TestOfflineBenchmarkPetFoodNotOthers(t *testing.T) {
	ds := consStore()
	var out []benchRow
	for _, class := range []string{"MAGE", "WARRIOR", "WARLOCK"} {
		res := inventory.Review(hpHunter(t, class, "Other", 20, "Boar Ribs"), ds)
		var why string
		for _, es := range res.Groups {
			for _, e := range es {
				why = e.Reason
			}
		}
		out = append(out, benchRow{"Boar Ribs in a " + strings.ToLower(class) + "'s bags: the bag review", why, "no pet food rule for a class that tames no beast", hpFoodSrc, resultIf(!strings.Contains(why, "pet food"), "a non-hunter kept pet food for a pet")})
	}
	cols := brColumns(ds, "items", "id")
	bad := brHasColumn(cols, "diet", "food_type", "pet")
	out = append(out, benchRow{"items: a column that says what a pet eats", fmt.Sprintf("%d columns: none names a diet", len(cols)), "no such column", hpNoneSrc, resultIf(bad == "", "a column names "+bad)})
	out = append(out, benchRow{"Boar Ribs: which diet the dataset files it under", "the advisor names Meat from its own short list (package petfood), only to keep it", "game: Meat (Warcraft Wiki, Pet food); the dataset: no diet column",
		hpFoodSrc, "differ (data): the dataset holds no pet diet, so the keep rule runs on a hand-written list of pet foods, not on the data"})
	finish(t, "advice-pet-food-others", "Offline hunter pets: other classes and the dataset's food rows", out)
}

// ---- pet abilities and ranks ----

// TestOfflineBenchmarkPetSpellRanks: the ranks of the hunter's pet spells and the level each is learned at.
func TestOfflineBenchmarkPetSpellRanks(t *testing.T) {
	var out []benchRow
	for _, tc := range []struct {
		name string
		rank int64
	}{{"Mend Pet", 2}, {"Mend Pet", 4}, {"Mend Pet", 5}, {"Mend Pet", 6}, {"Mend Pet", 7}, {"Scare Beast", 2}, {"Scare Beast", 3}} {
		out = append(out, utilTrained(t, "HUNTER", "Horde", tc.name, tc.rank))
	}
	finish(t, "advice-pet-spell-ranks", "Offline hunter pets: Mend Pet and Scare Beast ranks", out)
}

// TestOfflineBenchmarkPetSpellSteps: what a hunter is told to train over a span of levels: a rank step lists the new rank once, and a level before it lists none.
func TestOfflineBenchmarkPetSpellSteps(t *testing.T) {
	var out []benchRow
	for _, tc := range []struct {
		from, to int
		name     string
		rank     int
	}{{19, 20, "Mend Pet", 2}, {27, 28, "Mend Pet", 3}, {43, 44, "Mend Pet", 5}, {59, 60, "Mend Pet", 7}, {29, 30, "Scare Beast", 2}} {
		got := utilSpells("HUNTER", "Horde", tc.from, tc.to)
		out = append(out, benchRow{fmt.Sprintf("horde hunter, levels %d to %d: %s rank %d", tc.from, tc.to, tc.name, tc.rank), strings.Join(got, ", "), fmt.Sprintf("class_spells: %s (Rank %d) at level %d", tc.name, tc.rank, tc.to), hpSpellSrc,
			resultIf(utilHas(got, tc.name, tc.rank), "the rank step was not listed")})
	}
	got := utilSpells("HUNTER", "Horde", 21, 27)
	n := 0
	for _, s := range got {
		if strings.HasPrefix(s, "Mend Pet") {
			n++
		}
	}
	out = append(out, benchRow{"horde hunter, levels 21 to 27: Mend Pet", fmt.Sprintf("%d Mend Pet steps of %d spells", n, len(got)), "class_spells: no Mend Pet rank between level 20 and 28", hpSpellSrc, resultIf(n == 0, "a Mend Pet rank before its level")})
	finish(t, "advice-pet-spell-steps", "Offline hunter pets: the rank steps a hunter is told to train", out)
}

// TestOfflineBenchmarkPetTalentSpells: Intimidation, Bestial Wrath, Kill Command and Improved Mend Pet sit in class_spells beside the trained spells but come from talent points (or a rune):
// no trainer teaches them, so the training list leaves them out.
func TestOfflineBenchmarkPetTalentSpells(t *testing.T) {
	var out []benchRow
	got := utilSpells("HUNTER", "Horde", 1, 60)
	for _, name := range []string{"Intimidation", "Bestial Wrath", "Improved Mend Pet", "Kill Command"} {
		r := utilRow(t, "HUNTER", name, 0)
		listed := false
		for _, s := range got {
			if strings.HasPrefix(s, name+" ") {
				listed = true
			}
		}
		out = append(out, benchRow{"horde hunter, levels 1 to 60: " + name + " in the training list", fmt.Sprintf("listed: %v", listed), fmt.Sprintf("class_spells: %s at level %d (%s), not taught by a trainer", name, num(r["level"]), text(r["skill_line_name"])), hpTalSrc, resultIf(!listed, name+" was listed as a trained spell")})
	}
	finish(t, "advice-pet-talent-spells", "Offline hunter pets: talent spells are not trained", out)
}

// TestOfflineBenchmarkPetAbilityNames: a pet's own abilities are no hunter spell. The dataset's Growl, Claw, Cower, Dash and Prowl are the druid's (class 11): a hunter's list does not carry them, and a hunter's question
// about them is framed with the line that says the data holds no pet ability.
func TestOfflineBenchmarkPetAbilityNames(t *testing.T) {
	var out []benchRow
	got := strings.Join(utilSpells("HUNTER", "Horde", 1, 60), ", ")
	for _, name := range []string{"Growl", "Claw", "Cower", "Dash", "Prowl"} {
		hunter := 0
		for _, r := range classRows(t, classID("HUNTER"), 1, 60) {
			if text(r["name"]) == name {
				hunter++
			}
		}
		druid := 0
		for _, r := range classRows(t, classID("DRUID"), 1, 60) {
			if text(r["name"]) == name {
				druid++
			}
		}
		q := "what level does my pet learn " + name + "?"
		f := ask.LimitFacts(q)
		out = append(out, benchRow{name + ": a hunter's training list, and the question " + q, fmt.Sprintf("in the list: %v; line: %v", strings.Contains(got, name+" "), strings.HasPrefix(f, "Hunter pets")), fmt.Sprintf("class_spells: %d hunter rows, %d druid rows", hunter, druid), hpDruidSrc,
			resultIf(hunter == 0 && druid > 0 && !strings.Contains(got, name+" ") && strings.HasPrefix(f, "Hunter pets") && strings.Contains(f, "Say so"), "a pet ability was listed as a hunter spell or the question had no line")})
	}
	finish(t, "advice-pet-ability-names", "Offline hunter pets: a pet's abilities are not a hunter's spells", out)
}

// ---- taming and pet training ----

// TestOfflineBenchmarkPetTaming: the spells that make a hunter's pet, and when: they come at level 10 together, once each, and a hunter under 10 is told none.
func TestOfflineBenchmarkPetTaming(t *testing.T) {
	var out []benchRow
	got := utilSpells("HUNTER", "Horde", 10, 10)
	for _, name := range []string{"Tame Beast", "Call Pet", "Dismiss Pet", "Feed Pet", "Revive Pet"} {
		n := 0
		for _, s := range got {
			if strings.HasPrefix(s, name+" ") {
				n++
			}
		}
		out = append(out, benchRow{"horde hunter, level 10: " + name + " in the training step", fmt.Sprintf("%d time(s)", n), fmt.Sprintf("class_spells: %s at level %d", name, num(utilRow(t, "HUNTER", name, 0)["level"])), hpSpellSrc, resultIf(n == 1, "listed not exactly once")})
	}
	learned := levelup.Learned(usabChar("HUNTER", 60, "Horde"), usabStore(), 60)
	n := 0
	for _, s := range learned {
		if s.Name == "Tame Beast" {
			n++
		}
	}
	rows := 0
	for _, r := range classRows(t, classID("HUNTER"), 1, 60) {
		if text(r["name"]) == "Tame Beast" {
			rows++
		}
	}
	out = append(out, benchRow{"horde hunter, level 60: spells learned, Tame Beast", fmt.Sprintf("%d time(s)", n), fmt.Sprintf("class_spells: %d Tame Beast rows (spell ids 1515 and 13481), both level 10", rows), hpSpellSrc, resultIf(n == 1 && rows >= 2, "Tame Beast listed twice or not at all")})
	for _, tc := range []struct {
		name string
		lv   int
	}{{"Beast Lore", 24}, {"Eyes of the Beast", 14}} {
		before := utilSpells("HUNTER", "Horde", 1, tc.lv-1)
		at := utilSpells("HUNTER", "Horde", tc.lv, tc.lv)
		out = append(out, benchRow{fmt.Sprintf("horde hunter: %s before level %d and at it", tc.name, tc.lv), fmt.Sprintf("before: %v, at: %v", utilHas(before, tc.name, 0), utilHas(at, tc.name, 0)), fmt.Sprintf("class_spells: %s at level %d", tc.name, tc.lv), hpSpellSrc,
			resultIf(!utilHas(before, tc.name, 0) && utilHas(at, tc.name, 0), "trained at another level")})
	}
	finish(t, "advice-pet-taming", "Offline hunter pets: taming and the pet spells at level 10", out)
}

// TestOfflineBenchmarkPetTamingNoAnswer: how a pet is tamed and trained: the data holds no tameable beast, family or training row, so the advisor frames the question and names none.
func TestOfflineBenchmarkPetTamingNoAnswer(t *testing.T) {
	var out []benchRow
	ds := usabStore()
	beasts, _ := ds.Filter("npcs", store.Range{Column: "creature_type", Min: int64(1), Max: int64(1)})
	withFamily, names := 0, brColumns(ds, "npcs", "id")
	for _, r := range beasts {
		if r["family_id"] != nil {
			withFamily++
		}
	}
	bad := brHasColumn(names, "tame", "family_name", "diet", "exotic")
	out = append(out, benchRow{"npcs: a beast's family and whether it can be tamed", fmt.Sprintf("%d beast rows, %d with a numeric family_id; columns: no family name or tameable flag", len(beasts), withFamily), "family_id is a number with no name; no tameable column", hpNoneSrc, resultIf(bad == "", "a column names "+bad)})
	out = append(out, benchRow{"npcs: which family a beast belongs to, by name", "the advisor names no family: the data gives an id only", "game: Wolf, Cat, Boar, Bear and the other families (Warcraft Wiki, Hunter pets)", hpNoneSrc,
		"differ (data): the dataset's family_id has no family table behind it, so a beast's family cannot be named or its tameability or diet read"})
	for _, q := range []string{"how do I tame a beast?", "which beasts can I tame at level 10?", "what level can a hunter tame an elite?", "where do I train my pet's skills?"} {
		f := ask.LimitFacts(q)
		out = append(out, benchRow{"the question: " + q, firstLine(f), "no tame, family or training rows: framed, nothing from memory", hpNoneSrc, resultIf(strings.HasPrefix(f, "Hunter pets") && strings.Contains(f, "Say so") && strings.Contains(f, "name no food, family, tameable beast"), "no line that says the data has none")})
	}
	finish(t, "advice-pet-taming-none", "Offline hunter pets: taming and training the dataset cannot answer", out)
}

// TestOfflineBenchmarkPetNoAnswer: the questions on a pet's diet, abilities, loyalty and the stable that the dataset holds nothing for.
func TestOfflineBenchmarkPetNoAnswer(t *testing.T) {
	var out []benchRow
	for _, q := range []string{
		"what does my wolf eat?", "what food does my cat like?", "can my pet eat bread?", "what is the diet of a boar?",
		"what abilities does my hunter's pet learn?", "what rank of Bite should my pet have?", "how many pet skills can my pet learn?", "which pets are best for a hunter?",
		"what is my pet's happiness?", "how do I raise my pet's loyalty?", "how many stable slots does a hunter have?", "which family is exotic?",
	} {
		f := ask.LimitFacts(q)
		out = append(out, benchRow{"the question: " + q, firstLine(f), "no diet, ability, loyalty or stable row: framed, nothing from memory", hpNoneSrc,
			resultIf(strings.HasPrefix(f, "Hunter pets") && strings.Contains(f, "Say so") && !strings.Contains(f, "Companion pets"), "no line that says the data has none, or the companion pet line")})
	}
	for _, q := range []string{"at what level do I learn Feed Pet?", "what level is Mend Pet rank 3?", "when do I get Call Pet?", "how many companion pets can I keep?", "what abilities does my imp learn?"} {
		f := ask.LimitFacts(q)
		out = append(out, benchRow{"scope: " + q, firstLine(f), "answered from class_spells or by another line, not the pet line", hpSpellSrc, resultIf(!strings.HasPrefix(f, "Hunter pets"), "the pet line framed a question the data answers")})
	}
	finish(t, "advice-pet-none", "Offline hunter pets: no-answer questions", out)
}

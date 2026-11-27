package ask

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/dungeons"
	"github.com/WoW-Companion/advisor/internal/gear"
	"github.com/WoW-Companion/advisor/internal/levelup"
	"github.com/WoW-Companion/advisor/internal/store"
)

// classOrder is the nine classes in the order the facts name them.
var classOrder = []string{"warrior", "paladin", "hunter", "rogue", "priest", "shaman", "mage", "warlock", "druid"}

// classesNamed are the classes a question names (as a word, plural or with 's), at most three, in the order they come in the text.
func classesNamed(text string) []string {
	t := " " + strings.ToLower(text) + " "
	var out []string
	for _, k := range classOrder {
		if classWord(t, k) {
			out = append(out, k)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return strings.Index(t, out[i]) < strings.Index(t, out[j]) })
	if len(out) > 3 {
		out = out[:3]
	}
	return out
}

// trainerWhereAsk is a question about where a class trainer stands or who it is: "where is the mage trainer", "who is the warrior trainer in Orgrimmar",
// "where do I train my spells". A question about a price is the class trainer price topic's, a profession trainer or a Weapon Master is none.
var trainerWhereAsk = regexp.MustCompile(`(?i)\b(?:where|who|which|find|locat\w+|nearest|closest|whom|directions?|coordinates?|positions?|spot|stands?|standing|how (?:do|can) i get to)\b.{0,50}\b(?:class trainers?|` + classTrainer + `|trainers? for (?:a |an |the |my )?(?:warrior|paladin|hunter|rogue|priest|shaman|mage|warlock|druid)s?)\b|\bwhere\b.{0,25}\b(?:train|learn)\b.{0,25}\b(?:my |new |class )?(?:class )?(?:spells?|abilit(?:y|ies))\b`)

// trainerWhereRule is added to a trainer location answer: the city list is the only source for a city, and no NPC or position is held.
const trainerWhereRule = "Class trainer location: the game data holds no class trainer NPC, name or position (trainers lists profession trainers only, weapon_masters the Weapon Masters). " +
	"The advisor's own list of the capitals with a class trainer (the Classic class trainer lists) is the only source for a city: %s " +
	"Name a city only from it, say plainly that a city not on it has no trainer for that class, and name no trainer, NPC or coordinates unless a 'Train at:' line names one from the player's own QuestieDB or a trainer they opened."

// TrainerWhereFacts is the line for a question about where a class trainer is: the capitals that hold one, for the class the question names (else the
// character's), both sides; "" for any other question.
func TrainerWhereFacts(c character.Character, text string) string {
	if !trainerWhereAsk.MatchString(text) {
		return ""
	}
	classes := classesNamed(text)
	if len(classes) == 0 && c.Class != "" {
		classes = []string{strings.ToLower(c.Class)}
	}
	var lists []string
	for _, k := range classes {
		var sides []string
		for _, f := range []string{"Horde", "Alliance"} {
			if cs := levelup.ClassCities(f, k); len(cs) > 0 {
				sides = append(sides, f+": "+strings.Join(cs, ", "))
			} else {
				sides = append(sides, f+": none")
			}
		}
		lists = append(lists, "a "+k+" trainer stands in ("+strings.Join(sides, "; ")+").")
	}
	if len(lists) == 0 {
		lists = []string{"no class is named, so name none."}
	}
	return fmt.Sprintf(trainerWhereRule, strings.Join(lists, " "))
}

// summonAsk is a question about a dungeon's summoning stone (also called a meeting stone): where it stands, whether a dungeon has one, how to summon the party at the entrance.
var summonAsk = regexp.MustCompile(`(?i)\b(?:summoning|meeting) stones?\b|\bsummon\w*\b.{0,40}\b(?:party|group|friends?|members?|guild\w*|everyone|them|the rest)\b.{0,40}\b(?:dungeon|raid|instance|entrance|outside|stone)\b|\b(?:party|group|friends?|members?|everyone|them)\b.{0,30}\b(?:summon\w*)\b.{0,30}\b(?:dungeon|raid|instance|entrance|outside|stone)\b`)

// SummonStoneFacts is the line for a question about a summoning stone: the dataset holds the dungeon and its entrance zone, level range and group size, and no stone row or position.
func SummonStoneFacts(ds store.DataStore, text string) string {
	if !summonAsk.MatchString(text) {
		return ""
	}
	var b strings.Builder
	b.WriteString("Summoning stones (also called meeting stones): the game data holds no summoning stone row, no stone position and no list of which dungeons or raids have one. Say so; give no stone coordinates and do not promise a stone stands at an entrance. ")
	name := dungeons.Mentioned(ds, nil, text)
	if name == "" {
		b.WriteString("No dungeon is named, so name none. The data does hold each dungeon's entrance zone, level range and group size, for the dungeon the player names.")
		return b.String()
	}
	rows, err := ds.Filter("instances", store.Range{Column: "map_id", Min: int64(0)})
	if err == nil {
		for _, r := range rows {
			if n, _ := r["name"].(string); n != name {
				continue
			}
			zone, _ := r["entrance_zone_name"].(string)
			if zone == "" {
				break
			}
			fmt.Fprintf(&b, "What the data holds for %s: its entrance is in %s (level %d-%d, %d players). Give that as where to go, and the stone, if the game has one there, is for the player to look for by the entrance.", name, zone, numOf(r["min_level"]), numOf(r["max_level"]), dungeons.PartySize(name, r["instance_type"] == "raid", int(numOf(r["max_players"]))))
			return b.String()
		}
	}
	fmt.Fprintf(&b, "%s is in the data but its row names no entrance zone, so name none.", name)
	return b.String()
}

func numOf(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case int32:
		return int64(n)
	case int16:
		return int64(n)
	case int:
		return int64(n)
	}
	return 0
}

// proficiencyAsk is a question about the armor a class can wear (armor type, shield, proficiency); the weapon words are WeaponQuestion's.
var proficiencyAsk = regexp.MustCompile(`(?i)\b(?:wear|wearing|wears|wore|equip\w*|put on|armou?r (?:types?|proficienc\w+|skills?)|proficien\w+|allowed|able to use)\b.{0,60}\b(?:armou?r|cloth|leather|chain ?mail|mail|plate|shields?)\b|\b(?:armou?r|cloth|leather|chain ?mail|mail|plate|shields?)\b.{0,40}\b(?:wear|wearing|wearers?|proficien\w+|allowed|equip\w*|usable|can use)\b|\b(?:what|which)\b.{0,12}\b(?:armou?r|armou?r types?)\b.{0,30}\b(?:can|does|do|may|will)\b`)

// ProficiencyFacts is the weapon and armor rules of each class a question names, for a question about what a class can wear or wield, when the question names a class
// other than the character's (the character's own weapon rules are WeaponFacts's) or asks about armor. "" for any other question.
func ProficiencyFacts(c character.Character, ds store.DataStore, text string) string {
	armor := proficiencyAsk.MatchString(text)
	weapon := WeaponQuestion(text)
	if !armor && !weapon {
		return ""
	}
	classes := classesNamed(text)
	own := strings.ToLower(c.Class)
	if len(classes) == 0 && armor && own != "" {
		classes = []string{own}
	}
	var out []string
	for _, k := range classes {
		if !armor && k == own {
			continue // WeaponFacts holds the character's own weapon rules
		}
		out = append(out, classRules(k, ds, armor, weapon))
	}
	if len(out) == 0 {
		return ""
	}
	return "Weapon and armor rules by class (use these, never your memory of Classic; the game data holds the weapon skills a class can learn and each item's armor type and class limit, and no armor proficiency table):\n" + strings.Join(out, "\n")
}

// classRules is the one class's lines: the weapons it can learn (from weapon_skills) and its armor ladder (the advisor's own class rules).
func classRules(class string, ds store.DataStore, armor, weapon bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "- %s:", titleWord(class))
	if weapon || armor {
		f := gear.LoadWeaponSkills(character.Character{Class: strings.ToUpper(class)}, ds)
		if !f.Have {
			b.WriteString(" no weapon-skill facts in this game data; say you don't have them.")
		} else {
			var learn []string
			for id := range f.Learnable {
				learn = append(learn, f.Names[id])
			}
			sort.Strings(learn)
			fmt.Fprintf(&b, " can learn these weapon skills: %s (a weapon of any other kind cannot be used; Dual Wield is a class spell, not a weapon skill).", strings.Join(learn, ", "))
		}
	}
	if low, gain, high, shield, ok := gear.ArmorLadder(class); ok {
		if gain > 0 {
			fmt.Fprintf(&b, " Armor: %s until level %d, then %s.", low, gain, high)
		} else {
			fmt.Fprintf(&b, " Armor: %s at every level.", high)
		}
		if shield {
			b.WriteString(" Can carry a shield.")
		} else {
			b.WriteString(" Cannot use a shield.")
		}
		b.WriteString(" The class may also be able to wear lighter armor than its own type, but recommend only its own type.")
	}
	return b.String()
}

func titleWord(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

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

// armorLadderAsk is a question about when a class gains an armor type, which type it should wear or which of two types is better: "when do warriors get plate
// armor", "what level do paladins get plate", "when does a warrior switch to plate", "what armor type should my shaman use", "mail or plate for my warrior at level
// 45". The bare word mail also names the post, so armorMailAsk keeps it to a question that names a class.
var armorLadderAsk = regexp.MustCompile(`(?i)\b(?:when|what level|which level|at what level|how soon)\b.{0,40}\b(?:get|gets|getting|learn\w*|gain\w*|switch\w*|chang\w*|mov\w*|start\w*|unlock\w*|wear\w*|equip\w*|upgrad\w*)\b.{0,30}\b(?:armou?r|cloth|leather|chain ?mail|plate ?(?:mail)?)\b|\b(?:what|which)\b.{0,10}\b(?:kind|type|sort)s? of armou?r\b|\barmou?r types?\b.{0,30}\b(?:should|use|best|wear|for|better)\b|\b(?:cloth|leather|mail|chain ?mail|plate)\b\s*(?:or|vs\.?|versus|and)\s*(?:cloth|leather|mail|chain ?mail|plate)\b.{0,20}\b(?:better|best|for|should|wear)\b|\b(?:better|best|should|wear)\b.{0,30}\b(?:cloth|leather|mail|chain ?mail|plate)\b\s*(?:or|vs\.?|versus)\s*(?:cloth|leather|mail|chain ?mail|plate)\b`)

// armorMailAsk is the same question about bare mail ("when do shamans get mail", "mail or plate for my warrior"): a question that names a class, since mail is also the post.
var armorMailAsk = regexp.MustCompile(`(?i)\b(?:when|what level|which level|at what level|how soon)\b.{0,40}\b(?:get|gets|getting|learn\w*|gain\w*|switch\w*|chang\w*|mov\w*|start\w*|unlock\w*|wear\w*|equip\w*|upgrad\w*)\b.{0,30}\bmail\b|\bmail\b\s*(?:or|vs\.?|versus|and)\s*(?:cloth|leather|plate)\b|\b(?:cloth|leather|plate)\b\s*(?:or|vs\.?|versus|and)\s*mail\b`)

// armorSlotWord is an armor slot as a player says it. A chest, the feet and the waist are left to a question that has a wear or equip verb beside them.
const armorSlotWord = `(?:helm(?:et)?s?|hats?|caps?|coifs?|shoulders?|shoulderpads?|pauldrons?|spaulders?|mantles?|chest ?(?:pieces?|armou?r|plates?)?|breastplates?|tunics?|vests?|hauberks?|robes?|bracers?|bracelets?|wristguards?|wrist armou?r|gloves?|gauntlets?|mitts?|belts?|girdles?|waistbands?|pants|trousers|leggings|legguards?|leg armou?r|boots|greaves|sabatons?|shoes|sandals|cloaks?|capes?)`

// armorSlotAsk is a question whether a class wears a piece for an armor slot: "what helm can a rogue wear", "can my mage wear a chain helm", "can a priest wear plate boots". The verb is
// wear or equip, never use, which asks about a chest in a dungeon as readily as about armor.
var armorSlotAsk = regexp.MustCompile(`(?i)\b(?:wear|wearing|wears|equip\w*|put on)\b.{0,30}\b` + armorSlotWord + `\b|\b` + armorSlotWord + `\b.{0,30}\b(?:wear|wearing|wearers?|equip\w*|put on)\b`)

// shieldAsk is a question about who may carry a shield: "can my paladin use a shield", "can priests use shields", "does a rogue get a shield". A
// spell that is named for a shield (Power Word: Shield, Shield Slam) is no such question, as it names no one to carry it.
var shieldAsk = regexp.MustCompile(`(?i)\b(?:use|carry|hold|wield|equip\w*|get|have|wear|put on|take)\s+(?:(?:a|an|the|my|any|one|their)\s+(?:\w+\s+)?shield\b|shields\b)|\bshield\b.{0,30}\b(?:or|vs\.?|versus)\b.{0,12}\b(?:two[- ]?hand\w*|2h|dual[- ]?wield\w*|weapon)\b|\b(?:which|what|who)\b.{0,30}\b(?:classes|class)\b.{0,25}\bshields\b`)

// jewelleryAsk is a question about whether a class may wear a ring, necklace, trinket or cloak: "can my warrior wear a ring", "what trinkets can a mage use".
var jewelleryAsk = regexp.MustCompile(`(?i)\b(?:can|could|may|able|allowed|does|do|will)\b.{0,50}\b(?:wear|use|equip\w*)\b.{0,25}\b(?:rings?|necklaces?|amulets?|trinkets?|cloaks?)\b|\b(?:what|which)\b.{0,25}\b(?:rings?|necklaces?|amulets?|trinkets?|cloaks?)\b.{0,30}\b(?:can|may)\b.{0,30}\b(?:use|wear|equip)|\b(?:rings?|necklaces?|amulets?|trinkets?|cloaks?)\b.{0,30}\b(?:class(?:es)? (?:restrict\w*|limit\w*)|restricted|armou?r type)\b`)

// relicAsk is a question about the relic slot, which the capture holds in the ranged slot for a paladin, druid or shaman.
var relicAsk = regexp.MustCompile(`(?i)\b(?:can|could|may|able|allowed|use|wear|equip\w*|slot|class(?:es)?)\b.{0,40}\b(?:relics?|librams?|idols?)\b|\b(?:relics?|librams?|idols?)\b.{0,30}\b(?:slot|class(?:es)?|use|equip\w*|wear|best)\b|\branged slot\b`)

// allClassesAsk is a question about every class at once: "which classes can use shields".
var allClassesAsk = regexp.MustCompile(`(?i)\b(?:which|what|who)\b.{0,20}\bclasses\b|\ball (?:the )?classes\b|\bany class(?:es)?\b|\beach class\b`)

// ProficiencyFacts is the weapon and armor rules of each class a question names, for a question about what a class can wear or wield, when the question names a class
// other than the character's (the character's own weapon rules are WeaponFacts's) or asks about armor. "" for any other question.
func ProficiencyFacts(c character.Character, ds store.DataStore, text string) string {
	classes := classesNamed(text)
	armor := proficiencyAsk.MatchString(text) || shieldAsk.MatchString(text) || jewelleryAsk.MatchString(text) || armorLadderAsk.MatchString(text) || armorSlotAsk.MatchString(text) ||
		len(classes) > 0 && armorMailAsk.MatchString(text)
	weapon := WeaponQuestion(text)
	relic := relicAsk.MatchString(text)
	if !armor && !weapon && !relic {
		return ""
	}
	own := strings.ToLower(c.Class)
	if len(classes) == 0 && allClassesAsk.MatchString(text) {
		classes = classOrder
	} else if len(classes) == 0 && (armor || relic) && own != "" {
		classes = []string{own}
	}
	var out []string
	for _, k := range classes {
		if !armor && !relic && k == own {
			continue // WeaponFacts holds the character's own weapon rules
		}
		out = append(out, classRules(k, ds, armor, weapon, relic))
	}
	if len(out) == 0 {
		return ""
	}
	return "Weapon and armor rules by class (use these, never your memory of Classic; the game data holds the weapon skills a class can learn and each item's armor type and class limit, and no armor proficiency table):\n" + strings.Join(out, "\n")
}

// classRules is the one class's lines: the weapons it can learn (from weapon_skills) and its armor ladder (the advisor's own class rules).
func classRules(class string, ds store.DataStore, armor, weapon, relic bool) string {
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
		b.WriteString(" Rings, necklaces, cloaks and trinkets have no armor type, so any class may wear them (an item's own class limit and required level still apply).")
	}
	if relic {
		b.WriteString(relicRule(class))
	}
	return b.String()
}

// relicRule is what the class holds in its relic slot: the dataset lists each relic by type and the class that holds it, and has no relic effect, so one relic
// cannot be ranked against another.
func relicRule(class string) string {
	kind := map[string]string{"paladin": "a libram", "druid": "an idol", "shaman": "a totem"}[strings.ToLower(class)]
	if kind == "" {
		return " Has no relic slot: a libram, idol or totem is for the paladin, druid or shaman alone, and the ranged slot takes a bow, gun, crossbow, thrown weapon or wand where the class may use one."
	}
	return " Relic slot (the ranged slot): takes " + kind + " and no other relic. The game data holds no relic effect, so say that no relic can be ranked above another and name none as the best."
}

func titleWord(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

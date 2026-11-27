package ask

import (
	"regexp"
	"strings"

	"github.com/WoW-Companion/advisor/internal/names"
	"github.com/WoW-Companion/advisor/internal/quests"
	"github.com/WoW-Companion/advisor/internal/store"
)

// The quest giver, turn-in, prerequisite and minimum level questions. The dataset names the NPC that gave a quest (an id in quests_observed) for a few quests, holds no turn-in NPC,
// no NPC position, no prerequisite or breadcrumb flag and no minimum level, so a question that asks for one gets the line that says so, and the player's own Questie (named in the
// quest tool's answer) is the only source of the answer.

// questGiverAsk is a question about who gives a quest or where it starts.
var questGiverAsk = regexp.MustCompile(`(?i)\bquest ?givers?\b|\b(?:who|which npc|what npc|where)\b.{0,30}\b(?:gives?|offers?|starts?|begins?|hands? out|picks? up|pick up|accepts?)\b.{0,25}\bquests?\b|\b(?:who|where|which npc)\b.{0,30}\bquests?\b.{0,30}\b(?:starts?|begins?|comes? from|is given|is offered|is picked up)\b|\bwhere\b.{0,20}\bget\b.{0,10}\b(?:the|that|this) quest\b`)

// questTurnInAsk is a question about who or where to hand a quest in to.
var questTurnInAsk = regexp.MustCompile(`(?i)\b(?:where|who|which npc|what npc)\b.{0,40}\b(?:turn(?:ed|ing)?|hand(?:ed|ing)?)\b.{0,25}\bin\b|\bturn-?in (?:npc|point|location|place)s?\b|\b(?:who|which npc|what npc|where)\b.{0,25}\b(?:takes?|accepts?|completes?|finish(?:es)?|ends?)\b.{0,25}\bquests?\b|\bwhere (?:does|do)\b.{0,30}\bquests?\b.{0,20}\b(?:end|finish)\b`)

// questPrereqAsk is a question about what comes before a quest, what it leads to, or a breadcrumb quest.
var questPrereqAsk = regexp.MustCompile(`(?i)\bprereq\w*\b|\bpre-?quests?\b|\bbread ?crumbs?\b|\bfollow[- ]?ups?\b|\b(?:what|which)\b.{0,15}\bquests?\b.{0,25}\b(?:leads?|sends?|unlocks?|opens?|comes? before|precedes?)\b|\b(?:what|which) quest\b.{0,20}\bbefore\b|\b(?:need|have|must|should) to (?:finish|complete|do)\b.{0,20}\bbefore\b|\bquests?\b.{0,20}\bunlock\w*\b`)

// questLevelLoose, questGiverLoose, questTurnInLoose and questPrereqLoose are the wider forms of the same questions that carry no word "quest": the player names the quest by its title ("what is
// the minimum level for Dusky Crab Cakes?"). They count only when the question names a quest the dataset holds.
var (
	questLevelLoose  = regexp.MustCompile(`(?i)\b(?:min(?:imum)?|lowest|required)\b.{0,15}\blevels?\b|\bwhat level\b.{0,25}\b(?:to|do i need|must i|can i)\b|\blevels?\b.{0,20}\b(?:to|do i need to) (?:start|accept|pick up|take|get)\b`)
	questGiverLoose  = regexp.MustCompile(`(?i)\b(?:who|which npc|what npc|where)\b.{0,25}\b(?:gives?|offers?|starts?|begins?|picks? up|pick up|accepts?|get)\b|\bwho\b.{0,15}\bgave\b`)
	questTurnInLoose = regexp.MustCompile(`(?i)\b(?:who|where|which npc)\b.{0,30}\b(?:turn(?:ed|ing)?|hand(?:ed|ing)?|takes?|accepts?|finish(?:es)?|ends?)\b`)
	questPrereqLoose = regexp.MustCompile(`(?i)\b(?:before|after|leads? to|unlocks?|comes? (?:before|after)|needed first|first)\b`)
)

const (
	questMinLevelFact = "Quest minimum level: the game data holds no minimum level to pick up or accept a quest (quests has a level column with no values, and no minimum level), only the level players report for it (quests_observed). Say so; give no level needed to start a quest from memory, and do not take its reported level for a minimum."
	questGiverFact    = "Quest givers and where a quest starts: the game data names the NPC that gave a quest (an id in quests_observed, with a name only when the npcs table holds the NPC) for a few quests only, and holds no NPC position, no coordinates and no zone for an NPC. Say so; name no quest giver, place or coordinates from memory. The quest tool names the giver and its zone where the player's own Questie knows the quest."
	questTurnInFact   = "Quest turn-in: the game data holds no turn-in NPC for any quest (turn_in_npc_id of quests_observed is empty on every row) and no NPC position, so it says nothing on where a quest is handed in or whether that is the NPC that gave it. Say so; name no turn-in NPC, place or coordinates from memory. The quest tool names the turn-in and its zone where the player's own Questie knows the quest."
	questPrereqFact   = "Quest prerequisites, follow-ups and breadcrumbs: the game data holds no prerequisite quest, no follow-up or breadcrumb flag and no quest that sends the player to a dungeon; its quest_lines hold the order of three lines only (none a breadcrumb, none a class's) and instance_quests only tags the quests of a dungeon. Say so; name no prerequisite, next quest or breadcrumb from memory. The quest tool lists what a quest comes after and leads to where one of those lines or the player's own Questie holds the quest."
)

// questLimitFacts is the no-data line of each quest giver, turn-in, prerequisite or breadcrumb question the text asks (the first three topics of the quest questions; the minimum level has
// its own line in limitTopics).
func questLimitFacts(text string) []string {
	var out []string
	if questGiverAsk.MatchString(text) {
		out = append(out, questGiverFact)
	}
	if questTurnInAsk.MatchString(text) {
		out = append(out, questTurnInFact)
	}
	if questPrereqAsk.MatchString(text) {
		out = append(out, questPrereqFact)
	}
	return out
}

// questTitleMinRune is the shortest title (letters and digits) a question may name a quest by: shorter ones are common words.
const questTitleMinRune = 8

// NamedQuestFacts is the no-data line of a quest question that names a quest by its title and has no word "quest" ("what is the minimum level for Dusky Crab Cakes?"): the lines LimitFacts
// does not already give. The prerequisite line is left out for a quest that sits in one of the dataset's quest lines, whose order the quest tool gives. "" when the text names no quest of
// quests_observed or asks none of these things.
func NamedQuestFacts(ds store.DataStore, text string) string {
	if ds == nil || !(questLevelLoose.MatchString(text) || questGiverLoose.MatchString(text) || questTurnInLoose.MatchString(text) || questPrereqLoose.MatchString(text)) {
		return ""
	}
	ids := namedQuests(ds, text)
	if len(ids) == 0 {
		return ""
	}
	inLine := true
	for _, id := range ids {
		inLine = inLine && quests.Of(ds, id).Found
	}
	have := LimitFacts(text)
	var want []string
	add := func(on bool, line string) {
		if on && !strings.Contains(have, firstWords(line)) {
			want = append(want, line)
		}
	}
	add(questLevelLoose.MatchString(text), questMinLevelFact)
	add(questGiverLoose.MatchString(text), questGiverFact)
	add(questTurnInLoose.MatchString(text), questTurnInFact)
	add(questPrereqLoose.MatchString(text) && !inLine, questPrereqFact)
	return strings.Join(want, "\n")
}

// firstWords is a fact's heading, the words before its first colon.
func firstWords(line string) string {
	head, _, _ := strings.Cut(line, ":")
	return head + ":"
}

// namedQuests are the quests of quests_observed whose title the text holds (an exact title, whole words, no case or accents).
func namedQuests(ds store.DataStore, text string) []int64 {
	rows, err := ds.Filter("quests_observed", store.Range{Column: "quest_id", Min: int64(1)})
	if err != nil {
		return nil
	}
	norm := " " + names.Normalize(text) + " "
	var ids []int64
	for _, r := range rows {
		title, _ := r["title"].(string)
		n := names.Normalize(title)
		if len([]rune(strings.ReplaceAll(n, " ", ""))) >= questTitleMinRune && strings.Contains(norm, " "+n+" ") {
			id, _ := r["quest_id"].(int64)
			ids = append(ids, id)
		}
	}
	return ids
}

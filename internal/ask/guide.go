package ask

import "strings"

// TalentQuestion reports whether a question is about talents.
func TalentQuestion(text string) bool { return strings.Contains(strings.ToLower(text), "talent") }

// talentRule is added to the prompt of a talent question. The next pick comes from the dataset's
// Icy Veins leveling builds through the talent_plan tool (and the facts); nothing is looked up on the web.
func talentRule() string {
	return " Talents: call get_character_profile first. If needs_profile is true, answer with only its short multiple-choice question and stop; " +
		"when the player has answered, call set_character_profile with it. Then take the pick from the talent_plan tool (the Icy Veins leveling build for the saved spec) and the talent facts: " +
		"talent picks come from those facts, never from general knowledge. " +
		"answer in the short shape \"Take <talent> (<rank>): <the build's reason>.\" with at most 2 bullets, the next one or two picks after it and why it fits the play style. Talk about the next pick, never about points unspent, points remaining or the level band. " +
		"Points the player spent off the build are mentioned once, plainly, never with a respec suggestion unless they ask. " +
		"When talent_plan has no checked build, say so in one line, mention only the unspent points and name no specific pick. Never invent talent names. "
}

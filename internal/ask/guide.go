package ask

import "strings"

// TalentQuestion reports whether a question is about talents.
func TalentQuestion(text string) bool { return strings.Contains(strings.ToLower(text), "talent") }

// talentRule is added to the prompt of a talent question. The next pick comes from the dataset's
// curated builds through the talent_plan tool (and the facts); nothing is looked up on the web.
func talentRule() string {
	return " Talents: call get_character_profile first. If needs_profile is true, answer with only its short multiple-choice question and stop; " +
		"when the player has answered, call set_character_profile with it. Then take the pick from the talent_plan tool (the curated build for the saved spec): " +
		"answer in the short shape \"Take <talent> (<rank>): <the build's reason>.\" with at most 2 bullets, the next picks in the band and why it fits the play style. " +
		"When talent_plan has no build, say so in one line and name the options it lists as options, not a recommendation. Never invent talent names. "
}

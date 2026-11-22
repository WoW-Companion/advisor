package ask

import (
	"slices"
	"strings"
)

// GuideDomain is the one site the talent-plan fetch may reach: the Icy Veins WoW: Forever guides.
const GuideDomain = "icy-veins.com"

// guideRule is Claude Code's permission rule for fetching pages of one domain only.
const guideRule = "WebFetch(domain:" + GuideDomain + ")"

// TalentQuestion reports whether a question is about talents, which is when the player's AI client
// may need to fetch a guide to build or refresh a talent plan.
func TalentQuestion(text string) bool { return strings.Contains(strings.ToLower(text), "talent") }

// CanFetchGuide reports whether WithGuide would give this question the guide fetch.
func CanFetchGuide(command []string, question string) bool {
	return ClientName(command) == "claude" && TalentQuestion(question) && slices.Equal(command, Presets["claude"])
}

// WithGuide gives a talent question the guide fetch, where the client has a narrow way to allow
// it. Only Claude Code does (checked against `claude --help`, version 2.1.323): --tools WebFetch
// lists the one built-in tool, and a second --allowedTools value, WebFetch(domain:icy-veins.com),
// pre-approves fetches of that domain only. Every other client, and every other question, gets argv
// unchanged and no web tool; the answer then shows the guide link. A custom command is never changed.
func WithGuide(argv, command []string, question string) []string {
	if !CanFetchGuide(command, question) {
		return argv
	}
	out := slices.Clone(argv)
	for i := 0; i+1 < len(out); i++ {
		switch {
		case out[i] == "--tools" && out[i+1] == "":
			out[i+1] = "WebFetch"
		case out[i] == "--allowedTools" && i+1 < len(out):
			out = slices.Insert(out, i+2, guideRule)
			i++
		}
	}
	return out
}

// talentRule is added to the prompt of a talent question. canFetch says whether the client was
// given the restricted web tool.
func talentRule(canFetch bool) string {
	s := " Talents: call get_character_profile first. If needs_profile is true, answer with only its short multiple-choice question and stop; " +
		"when the player has answered, call set_character_profile with it. With a saved plan, relay its next picks as the tools give them, " +
		"crediting the plan's guide link. To build or refresh the plan (only when none is saved, the player asks to refresh it, or the spec changed): call talent_tree"
	if canFetch {
		s += ", fetch the guide page (the class overview " + "https://www.icy-veins.com/wow-forever/<class>-class-overview" + " lists the spec guides; fetch only " + GuideDomain + " pages), " +
			"then call set_character_profile with a plan of node names copied exactly from talent_tree and target ranks: no guide text. Say the plan is a one-off larger call. "
	} else {
		s += ", then call set_character_profile with a plan only if the player gave you one; you cannot fetch a guide here, so show the guide link from get_character_profile (guide_overview_url) instead. "
	}
	return s + "Never invent talent names."
}

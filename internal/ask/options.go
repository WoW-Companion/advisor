package ask

import (
	"path/filepath"
	"regexp"
	"strings"
)

// Answer length, applied in the system prompt the same way for every client.
const (
	LengthBrief    = "brief"
	LengthStandard = "standard" // the default
	LengthDetailed = "detailed"
)

// Model tiers. Efficient is the default: the client's small, fast tier.
const (
	TierEfficient = "efficient"
	TierBalanced  = "balanced"
	TierBest      = "best"
	TierCustom    = "custom" // Options.Custom, a model name the player typed
)

// MaxTurns caps the agent's turns per ask where the CLI has a flag for it (Claude Code only).
const MaxTurns = 10

// Options are the per-ask settings the player controls. The zero value is Standard and Efficient.
type Options struct {
	Length string // brief, standard (default) or detailed
	Tier   string // efficient (default), balanced, best or custom
	Custom string // the model name for TierCustom
}

// Lengths and Tiers list the valid values, default first.
var (
	Lengths = []string{LengthStandard, LengthBrief, LengthDetailed}
	Tiers   = []string{TierEfficient, TierBalanced, TierBest, TierCustom}
)

var modelName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/\[\]-]{0,99}$`)

// Normalize replaces an unknown value by its default; a custom tier without a usable model name
// (it is passed as a flag value, so it must look like a model name) falls back to Efficient.
func (o Options) Normalize() Options {
	if !contains(Lengths, o.Length) {
		o.Length = LengthStandard
	}
	if !contains(Tiers, o.Tier) {
		o.Tier = TierEfficient
	}
	o.Custom = strings.TrimSpace(o.Custom)
	if o.Tier == TierCustom && !modelName.MatchString(o.Custom) {
		o.Tier = TierEfficient
	}
	return o
}

func contains(l []string, s string) bool {
	for _, v := range l {
		if v == s {
			return true
		}
	}
	return false
}

// lengthRule is the instruction each length adds to the prompt, and the most characters an answer
// of that length is kept to.
var lengthRule = map[string]struct {
	text  string
	limit int
}{
	LengthBrief:    {"Be brief: a few short lines, only the top 1-3 actions, no background.", 600},
	LengthStandard: {"Give a short numbered plan with a one-line reason for each step.", MaxAnswer},
	LengthDetailed: {"Give a fuller plan: numbered steps, the reason for each, and alternatives where they matter.", 3000},
}

// AnswerLimit is the most characters an answer of this length is kept to.
func (o Options) AnswerLimit() int { return lengthRule[o.Normalize().Length].limit }

// Per-client tiers. Sources (read 2026-11-21):
//   - Claude Code, https://code.claude.com/docs/en/model-config: --model takes the aliases haiku
//     (fast, efficient), sonnet (daily tasks) and opus (complex reasoning); they follow the newest
//     version. --max-turns limits agentic turns in print mode (CLI reference).
//   - Gemini CLI, https://github.com/google-gemini/gemini-cli docs/cli/cli-reference.md: --model/-m
//     takes the aliases flash-lite (lightest), flash (quick, well-rounded) and pro (complex reasoning).
//   - Codex CLI, https://developers.openai.com/codex/config-reference: `model` is a name with no
//     stable alias, so no model is passed and the client's own default runs; the tiers set
//     model_reasoning_effort (low, medium, high), the documented knob for cost. --model is used only
//     for the Custom tier.
type clientTiers struct {
	flag  string // the model flag
	tiers map[string]string
	note  string // shown in the settings tooltip
}

var tierTable = map[string]clientTiers{
	"claude": {"--model", map[string]string{TierEfficient: "haiku", TierBalanced: "sonnet", TierBest: "opus"},
		"Claude Code aliases: Efficient = haiku, Balanced = sonnet, Best = opus."},
	"gemini": {"--model", map[string]string{TierEfficient: "flash-lite", TierBalanced: "flash", TierBest: "pro"},
		"Gemini CLI aliases: Efficient = flash-lite, Balanced = flash, Best = pro."},
	"codex": {"--model", nil,
		"Codex has no stable model aliases, so it keeps its own default model; Efficient, Balanced and Best set its reasoning effort (low, medium, high)."},
}

var codexEffort = map[string]string{TierEfficient: "low", TierBalanced: "medium", TierBest: "high"}

// ClientName is the preset name (claude, codex, gemini) of an agent command, or "" for any other.
func ClientName(command []string) string {
	if len(command) == 0 {
		return ""
	}
	b := strings.ToLower(filepath.Base(command[0]))
	b = strings.TrimSuffix(b, filepath.Ext(b))
	if _, ok := tierTable[b]; ok {
		return b
	}
	return ""
}

// TierNote is the tooltip text on the model setting for a client ("" for an unknown one).
func TierNote(client string) string { return tierTable[client].note }

// ModelArgs are the arguments that select the model (and cap the turns) for the command's client,
// and the index in the argv they go at: right after the program, or after Codex's exec subcommand.
// A command that is not a known client gets none.
func ModelArgs(command []string, o Options) (args []string, at int) {
	c := ClientName(command)
	o = o.Normalize()
	if c == "" {
		return nil, 0
	}
	at = 1
	switch c {
	case "claude":
		args = append(args, "--max-turns", itoa(MaxTurns))
	case "codex":
		at = 2
	}
	if o.Tier == TierCustom {
		return append(args, tierTable[c].flag, o.Custom), at
	}
	if c == "codex" {
		return append(args, "-c", `model_reasoning_effort="`+codexEffort[o.Tier]+`"`), at
	}
	if m := tierTable[c].tiers[o.Tier]; m != "" {
		args = append(args, tierTable[c].flag, m)
	}
	return args, at
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}

// WithModel returns argv with ModelArgs spliced in.
func WithModel(argv []string, command []string, o Options) []string {
	args, at := ModelArgs(command, o)
	if len(args) == 0 || at > len(argv) {
		return argv
	}
	out := append([]string{}, argv[:at]...)
	out = append(out, args...)
	return append(out, argv[at:]...)
}

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
	// Reasoning is the reasoning level (low, medium or high); low is the default. It is passed only
	// to a client that has a flag for it, see reasoningFor.
	Reasoning string

	noReasoning bool // the CLI rejected the reasoning flag: leave it out (set by the retry)
}

// Lengths and Tiers list the valid values, default first.
var (
	Lengths = []string{LengthStandard, LengthBrief, LengthDetailed}
	Tiers   = []string{TierEfficient, TierBalanced, TierBest, TierCustom}
	// Reasonings are the reasoning levels, lowest first.
	Reasonings = []string{"low", "medium", "high"}
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
	if !contains(Reasonings, o.Reasoning) {
		o.Reasoning = Reasonings[0]
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

// Per-client models and reasoning. Sources (read 2026-11-21):
//   - Claude Code, https://code.claude.com/docs/en/model-config: --model takes the aliases haiku
//     (fast, efficient), sonnet (daily tasks) and opus (complex reasoning); they follow the newest
//     version. --max-turns limits agentic turns in print mode (CLI reference,
//     https://code.claude.com/docs/en/cli-reference). --effort <level> sets the effort level for the
//     session (low, medium, high, xhigh, max; `claude --help`); we offer low, medium and high.
//   - Gemini CLI, https://github.com/google-gemini/gemini-cli docs/cli/cli-reference.md: --model/-m
//     takes the aliases flash-lite (lightest), flash (quick, well-rounded) and pro (complex reasoning).
//     It has no reasoning flag (thinking is set in settings.json only), so none is passed.
//   - Antigravity CLI, https://antigravity.google/docs/cli/headless/: --model takes a slug listed by
//     `agy models` (no stable aliases documented), so no model is passed and the client's own default
//     runs (--model only for Custom); --effort takes low, medium, high (xhigh and max on some models).
//   - Codex CLI, https://developers.openai.com/codex/config-reference: `model` is a name with no
//     stable alias, so no model is passed and the client's own default runs (--model only for Custom);
//     `model_reasoning_effort` (low, medium, high; set with -c) is the reasoning knob.
type clientTiers struct {
	flag   string // the model flag
	tiers  map[string]string
	labels map[string]string // the model dropdown text per tier
	note   string            // shown in the model tooltip
	// effortNote is the reasoning tooltip; empty: the client has no reasoning flag (see reasoningArgs).
	effortNote string // shown in the reasoning tooltip
}

var tierTable = map[string]clientTiers{
	"claude": {"--model", map[string]string{TierEfficient: "haiku", TierBalanced: "sonnet", TierBest: "opus"},
		map[string]string{TierEfficient: "Haiku (small)", TierBalanced: "Sonnet (mid)", TierBest: "Opus (large)"},
		"Claude Code model aliases: Haiku is small, Sonnet mid, Opus large.",
		"Claude Code's --effort level. Low is the default."},
	"gemini": {"--model", map[string]string{TierEfficient: "flash-lite", TierBalanced: "flash", TierBest: "pro"},
		map[string]string{TierEfficient: "Flash-Lite (small)", TierBalanced: "Flash (mid)", TierBest: "Pro (large)"},
		"Gemini CLI model aliases: Flash-Lite is small, Flash mid, Pro large.",
		""},
	"agy": {"--model", nil,
		map[string]string{TierEfficient: "Antigravity default (small)"},
		"Antigravity's model slugs change (see `agy models`), so it keeps its own default model. Pick Custom to name one.",
		"Antigravity's --effort level. Low is the default."},
	"codex": {"--model", nil,
		map[string]string{TierEfficient: "Codex default (small)"},
		"Codex has no stable model aliases, so it keeps its own default model. Pick Custom to name one.",
		"Codex's model_reasoning_effort. Low is the default."},
}

// Choice is one entry of a client's model dropdown.
type Choice struct {
	Tier  string `json:"tier"`  // the Options.Tier it selects
	Label string `json:"label"` // "Haiku (small)"
}

// Choices is what the settings dropdowns offer for a client.
type Choices struct {
	Models        []Choice `json:"models"`                   // empty: the client's model is not ours to set
	ModelNote     string   `json:"model_note"`               // the model dropdown's tooltip
	Reasoning     []string `json:"reasoning"`                // empty: the client has no reasoning option
	ReasoningNote string   `json:"reasoning_note,omitempty"` // the reasoning dropdown's tooltip
}

// ChoicesFor lists the model and reasoning options for a client name; a client that is not a known
// preset gets none, and the notes say why.
func ChoicesFor(client string) Choices {
	t, ok := tierTable[client]
	if !ok {
		return Choices{ModelNote: "A custom command sets its own model and reasoning: add the flags to the command.", ReasoningNote: "A custom command sets its own reasoning: add the flags to the command."}
	}
	c := Choices{ModelNote: t.note, ReasoningNote: t.effortNote}
	for _, tier := range Tiers {
		if l, ok := t.labels[tier]; ok {
			c.Models = append(c.Models, Choice{tier, l})
		}
	}
	c.Models = append(c.Models, Choice{TierCustom, "Custom…"})
	if t.effortNote != "" {
		c.Reasoning = Reasonings
	} else {
		c.ReasoningNote = "Gemini CLI has no reasoning flag, so it uses its own default."
	}
	return c
}

// reasoningArgs are the arguments that set the reasoning level for a client, nil when it has none.
func reasoningArgs(client, level string) []string {
	switch client {
	case "claude":
		return []string{"--effort", level}
	case "agy":
		return []string{"--effort", level}
	case "codex":
		return []string{"-c", `model_reasoning_effort="` + level + `"`}
	}
	return nil
}

// ModelUsed names the model an ask with these options runs, as far as the command line says: the
// alias or custom name, or "default model" when the client picks its own.
func ModelUsed(command []string, o Options) string {
	c, o := ClientName(command), o.Normalize()
	switch {
	case c == "":
		return ""
	case o.Tier == TierCustom:
		return o.Custom
	case tierTable[c].tiers[o.Tier] != "":
		return tierTable[c].tiers[o.Tier]
	}
	return "default model"
}

// DisplayName is the name people know a client by (Claude Code, Codex, Antigravity, Gemini CLI);
// any other id comes back as given. The overlay keeps the same table in app.js.
func DisplayName(id string) string {
	switch strings.ToLower(id) {
	case "claude":
		return "Claude Code"
	case "codex":
		return "Codex"
	case "agy", "antigravity":
		return "Antigravity"
	case "gemini":
		return "Gemini CLI"
	}
	return id
}

// ClientName is the preset name (claude, codex, agy, gemini) of an agent command, or "" for any other.
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
		args = append(args, tierTable[c].flag, o.Custom)
	} else if m := tierTable[c].tiers[o.Tier]; m != "" {
		args = append(args, tierTable[c].flag, m)
	}
	if !o.noReasoning {
		args = append(args, reasoningArgs(c, o.Reasoning)...)
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

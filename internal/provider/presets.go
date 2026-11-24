package provider

import (
	"context"
	"slices"
	"strings"
)

// Kinds of adapter.
const (
	KindAnthropic = "anthropic"
	KindOpenAI    = "openai" // OpenAI-compatible Chat Completions
)

// Preset is a provider the player can pick.
type Preset struct {
	ID      string            `json:"id"`
	Name    string            `json:"name"`
	Kind    string            `json:"kind"`
	BaseURL string            `json:"base_url"`
	NeedKey bool              `json:"needs_key"`
	Local   bool              `json:"local"`  // a server on the player's PC: list its installed models
	Models  map[string]string `json:"models"` // default model id per tier (efficient, balanced, best)
	Doc     string            `json:"doc"`    // where the model ids were read
	Note    string            `json:"note,omitempty"`
	Extra   map[string]any    `json:"-"` // fields added to every request
}

// Presets lists the providers. Model ids were read from each provider's own docs on 2026-11-24:
//   - Anthropic: https://platform.claude.com/docs/en/models/overview (Claude API IDs)
//   - OpenAI: https://developers.openai.com/api/docs/models (chat completions stays supported for tool
//     calling per https://developers.openai.com/api/docs/guides/migrate-to-responses; reasoning_effort "none"
//     is sent because tool calling in Chat Completions needs it on the newer models)
//   - OpenRouter: https://openrouter.ai/api/v1/models (ids are vendor/model; free models are chosen from
//     the models list, as the free set changes)
//   - Ollama and LM Studio: no defaults; the installed models come from the server's /models endpoint.
var Presets = []Preset{
	{ID: "anthropic", Name: "Anthropic (Claude)", Kind: KindAnthropic, BaseURL: "https://api.anthropic.com/v1", NeedKey: true,
		Models: map[string]string{"efficient": "claude-haiku-5-5", "balanced": "claude-sonnet-5-5", "best": "claude-opus-5-5"},
		Doc:    "https://platform.claude.com/docs/en/models/overview"},
	{ID: "openai", Name: "OpenAI", Kind: KindOpenAI, BaseURL: "https://api.openai.com/v1", NeedKey: true,
		Models: map[string]string{"efficient": "gpt-6-luna", "balanced": "gpt-6.2-sol", "best": "gpt-6.2-astra"},
		Doc:    "https://developers.openai.com/api/docs/models", Extra: map[string]any{"reasoning_effort": "none"}},
	{ID: "openrouter", Name: "OpenRouter", Kind: KindOpenAI, BaseURL: "https://openrouter.ai/api/v1", NeedKey: true,
		Models: map[string]string{"efficient": "anthropic/claude-haiku-5.5", "balanced": "anthropic/claude-sonnet-5.5", "best": "anthropic/claude-opus-5.5"},
		Doc:    "https://openrouter.ai/api/v1/models", Note: "Pick a free model from the list to pay nothing."},
	{ID: "ollama", Name: "Ollama (on this PC)", Kind: KindOpenAI, BaseURL: "http://localhost:11434/v1", Local: true,
		Doc: "https://docs.ollama.com/api/openai-compatibility", Note: "Pick a model that supports tools."},
	{ID: "lmstudio", Name: "LM Studio (on this PC)", Kind: KindOpenAI, BaseURL: "http://localhost:1234/v1", Local: true,
		Doc: "https://lmstudio.ai/docs/developer/openai-compat", Note: "Pick a model that supports tools."},
	{ID: "custom", Name: "Other OpenAI-compatible server", Kind: KindOpenAI, Local: false,
		Doc: "", Note: "Enter the server's base URL (ending in /v1) and a model."},
}

// PresetByID finds a preset.
func PresetByID(id string) (Preset, bool) {
	i := slices.IndexFunc(Presets, func(p Preset) bool { return p.ID == id })
	if i < 0 {
		return Preset{}, false
	}
	return Presets[i], true
}

// KeyTarget is the credential's name in the key store: wowc-advisor:<provider>.
func KeyTarget(provider string) string { return "wowc-advisor:" + provider }

// ModelFor picks the model id: the custom one when the tier is custom, else the preset's default
// for the tier (balanced and best fall back to efficient's, which may be empty for local servers).
func (p Preset) ModelFor(tier, custom string) string {
	if tier == "custom" {
		return strings.TrimSpace(custom)
	}
	return p.Models[tier]
}

// NewAdapter builds the adapter for a preset (or a custom base URL) with key.
func NewAdapter(p Preset, baseURL, key string) Adapter {
	if baseURL == "" {
		baseURL = p.BaseURL
	}
	ep := Endpoint{BaseURL: baseURL, Key: key, Extra: p.Extra}
	if p.Kind == KindAnthropic {
		return Anthropic{ep}
	}
	return OpenAI{ep}
}

// Lister is an adapter that can list models.
type Lister interface {
	Models(ctx context.Context) ([]string, error)
}

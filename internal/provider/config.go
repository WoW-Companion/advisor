package provider

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SettingsSchema is the current layout version of providers.json.
const SettingsSchema = 1

// Client kinds in the ordered client list.
const (
	ClientAPI = "api"
	ClientCLI = "cli"
)

// ClientRef is one configured AI client. It holds no key: only whether one is saved is ever shown.
type ClientRef struct {
	Kind     string `json:"kind"`               // "api" or "cli"
	Provider string `json:"provider,omitempty"` // a preset id, for an API client
	BaseURL  string `json:"base_url,omitempty"` // overrides the preset's (required for "custom")
	Model    string `json:"model,omitempty"`    // a model id; empty: the tier's default
	Tier     string `json:"tier,omitempty"`     // efficient, balanced or best; empty: the overlay's model setting
	Agent    string `json:"agent,omitempty"`    // for a CLI client: a preset name or a command
}

// Settings is the saved client list: tried in order, the first is the active one. Empty means the
// CLI-only setup the advisor always had (the agent from ask.json or detection).
type Settings struct {
	// Schema is the layout version of providers.json. Changes are additive only (see docs/releasing.md).
	Schema  int         `json:"schema,omitempty"`
	Clients []ClientRef `json:"clients"`
}

// SettingsPath is providers.json in the data folder.
func SettingsPath() string {
	if d := DataDir(); d != "" {
		return filepath.Join(d, "providers.json")
	}
	return ""
}

// Load reads the settings; a missing file is empty settings.
func Load(path string) (Settings, error) {
	var s Settings
	if path == "" {
		return s, nil
	}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		return Settings{}, fmt.Errorf("%s: %w", path, err)
	}
	return s, nil
}

// Save writes the settings (no keys are ever part of them).
func Save(path string, s Settings) error {
	if path == "" {
		return errors.New("this system has no user config directory")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	s.Schema = max(s.Schema, SettingsSchema)
	b, _ := json.MarshalIndent(s, "", "  ")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Validate checks one client and fills its defaults.
func (c ClientRef) Validate() (ClientRef, error) {
	c.BaseURL, c.Model, c.Agent = strings.TrimSpace(c.BaseURL), strings.TrimSpace(c.Model), strings.TrimSpace(c.Agent)
	switch c.Kind {
	case ClientCLI:
		if c.Agent == "" {
			return c, errors.New("a CLI client needs its agent")
		}
		return ClientRef{Kind: ClientCLI, Agent: c.Agent}, nil
	case ClientAPI:
		p, ok := PresetByID(c.Provider)
		if !ok {
			return c, fmt.Errorf("unknown provider %q", c.Provider)
		}
		if c.BaseURL == "" {
			c.BaseURL = p.BaseURL
		}
		if !strings.HasPrefix(c.BaseURL, "http://") && !strings.HasPrefix(c.BaseURL, "https://") {
			return c, errors.New("the base URL must start with http:// or https://")
		}
		if c.BaseURL == p.BaseURL {
			c.BaseURL = "" // the preset's own address is not stored
		}
		if p.ID == "custom" && c.BaseURL == "" {
			return c, errors.New("a custom server needs its base URL")
		}
		switch c.Tier {
		case "", "efficient", "balanced", "best":
		default:
			return c, errors.New("the tier is efficient, balanced or best")
		}
		if len(c.Model) > 100 || strings.ContainsAny(c.Model, " \r\n\t") {
			return c, errors.New("that doesn't look like a model id")
		}
		return c, nil
	}
	return c, errors.New("a client is an api or a cli")
}

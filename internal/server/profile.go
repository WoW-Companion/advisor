package server

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/profile"
	"github.com/WoW-Companion/advisor/internal/store"
	"github.com/WoW-Companion/advisor/internal/talents"
)

// ProfileInput is the set_character_profile argument.
type ProfileInput struct {
	Character string   `json:"character,omitempty" jsonschema:"character name; omit when only one was captured"`
	Role      string   `json:"role,omitempty" jsonschema:"damage, tank, healer or balanced; required the first time"`
	Focus     string   `json:"focus,omitempty" jsonschema:"leveling, dungeons, pvp or mixed; required the first time"`
	Spec      string   `json:"spec,omitempty" jsonschema:"the spec tree the player chose, for example Fury"`
	StatPref  string   `json:"stat_pref,omitempty" jsonschema:"optional stat preference: balanced, damage (more damage) or survival (more survivability)"`
	Clear     []string `json:"clear,omitempty" jsonschema:"fields to forget when the player says so: any of role, focus, spec, stat_pref. An omitted field keeps its value; only clear forgets it. Clearing both role and focus removes the saved play style"`
}

// ProfileOutput is the get_character_profile and set_character_profile result.
type ProfileOutput struct {
	Character    string             `json:"character"`
	Profile      *character.Profile `json:"profile"`
	NeedsProfile bool               `json:"needs_profile" jsonschema:"no play style is saved: ask the player first"`
	Text         string             `json:"text" jsonschema:"plain-text rendering"`
	SavedNote    string             `json:"saved_note,omitempty" jsonschema:"set only after a save: the line the player sees at the end of the answer"`
}

// TreeInput is the talent_tree argument.
type TreeInput struct {
	Character string `json:"character,omitempty" jsonschema:"character name; omit when only one was captured"`
}

// TreeOutput is the talent_tree result.
type TreeOutput struct {
	Character string `json:"character"`
	Text      string `json:"text" jsonschema:"plain-text rendering"`
}

func displayName(c character.Character) string {
	if c.Name != "" {
		return c.Name
	}
	return c.Key
}

func renderProfile(c character.Character) string {
	var b strings.Builder
	if c.Profile == nil {
		b.WriteString(profile.Ask() + "\n")
	} else {
		p := c.Profile
		var parts []string
		for _, v := range [][2]string{{"role", p.Role}, {"focus", p.Focus}} {
			if v[1] == "" {
				v[1] = "not set"
			}
			parts = append(parts, v[0]+" "+v[1])
		}
		b.WriteString("Play style: " + strings.Join(parts, ", "))
		if p.Spec != "" {
			b.WriteString(", spec " + p.Spec)
		}
		if p.StatPref != "" {
			b.WriteString(", stat preference " + p.StatPref)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// SavedNote is the line that ends an answer after set_character_profile succeeded.
func SavedNote(spec string) string {
	what := "your play style"
	if spec != "" {
		what = spec
	}
	return "Saved: " + what + ". Change it any time in Settings → Play Style."
}

func profileOutput(c character.Character) ProfileOutput {
	return ProfileOutput{Character: displayName(c), Profile: c.Profile, NeedsProfile: profile.Incomplete(c.Profile), Text: renderProfile(c)}
}

func addProfileTools(s *mcp.Server, cfg Config) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "get_character_profile",
		Description: "The character's saved play style (role, focus, spec). When none is saved, the result is the " +
			"multiple-choice question to put to the player before any talent advice. Stored locally on the player's PC.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in TreeInput) (*mcp.CallToolResult, ProfileOutput, error) {
		f, errResult := load(cfg.SavedVariables)
		if errResult != nil {
			return errResult, ProfileOutput{}, nil
		}
		c, err := chosen(cfg, f, in.Character)
		if err != nil {
			return toolError(err.Error()), ProfileOutput{}, nil
		}
		return nil, profileOutput(c), nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "set_character_profile",
		Description: "Save the character's play style after asking the player (role and focus are required the first time; omitted fields keep their saved value, and to forget a choice the player asks to drop, list it in clear (clearing both role and focus removes the play style); spec must name one of the class's trees, such as Fury, and anything else is refused with the valid names), " +
			"and optionally a talent plan built from the player's chosen guide: node names (exactly as talent_tree lists them) and target ranks only, never guide text. " +
			"Nodes not in the captured tree are dropped and listed. Saved locally on the player's PC.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in ProfileInput) (*mcp.CallToolResult, ProfileOutput, error) {
		f, errResult := load(cfg.SavedVariables)
		if errResult != nil {
			return errResult, ProfileOutput{}, nil
		}
		c, err := chosen(cfg, f, in.Character)
		if err != nil {
			return toolError(err.Error()), ProfileOutput{}, nil
		}
		spec := in.Spec
		if strings.TrimSpace(spec) != "" {
			var ds store.DataStore
			if cfg.Dataset != "" {
				ds = store.NewDir(cfg.Dataset)
			}
			if spec, err = talents.NormalizeSpec(talents.Specs(ds, c), c.Class, spec); err != nil {
				return toolError("Not saved: " + err.Error()), ProfileOutput{}, nil
			}
		}
		clearing := make([]string, 0, len(in.Clear))
		for _, f := range in.Clear {
			f = strings.ToLower(strings.TrimSpace(f))
			if !slices.Contains([]string{"role", "focus", "spec", "stat_pref"}, f) {
				return toolError("Not saved: clear can only name role, focus, spec or stat_pref"), ProfileOutput{}, nil
			}
			clearing = append(clearing, f)
		}
		saved, err := profile.Update(cfg.profilesPath(), c.Key, func(e *profile.Entry) error {
			p := character.Profile{}
			if e.Profile != nil {
				p = *e.Profile
			}
			had := e.Profile != nil
			for _, v := range []struct {
				name string
				to   *string
				from string
			}{{"role", &p.Role, in.Role}, {"focus", &p.Focus, in.Focus}, {"spec", &p.Spec, spec}, {"stat_pref", &p.StatPref, in.StatPref}} {
				switch {
				case strings.TrimSpace(v.from) != "":
					*v.to = v.from
				case slices.Contains(clearing, v.name):
					*v.to = ""
				}
			}
			p, err := profile.Clean(p)
			if err != nil {
				return err
			}
			if !had && (p.Role == "" || p.Focus == "") {
				return errors.New("role and focus are both needed the first time")
			}
			if p.Role == "" && p.Focus == "" {
				e.Profile = nil // both cleared: no play style is saved
				return nil
			}
			e.Profile = &p
			return nil
		})
		if err != nil {
			return toolError("Not saved: " + err.Error()), ProfileOutput{}, nil
		}
		c.Profile = saved.Profile
		out := profileOutput(c)
		out.SavedNote = SavedNote(spec)
		if saved.Profile == nil {
			out.SavedNote = "Cleared: no play style is saved for this character. Set it any time in Settings → Play Style."
		}
		out.Text += out.SavedNote + "\n"
		return nil, out, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "talent_tree",
		Description: "The captured class talent tree: every node's exact name, branch, rank and max rank, and whether it can be bought. " +
			"Use these names, and only these, when naming a talent. Source: ForeverScribe capture.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in TreeInput) (*mcp.CallToolResult, TreeOutput, error) {
		f, errResult := load(cfg.SavedVariables)
		if errResult != nil {
			return errResult, TreeOutput{}, nil
		}
		c, err := chosen(cfg, f, in.Character)
		if err != nil {
			return toolError(err.Error()), TreeOutput{}, nil
		}
		return nil, TreeOutput{displayName(c), talents.RenderTree(c)}, nil
	})
}

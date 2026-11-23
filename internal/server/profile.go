package server

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/profile"
	"github.com/WoW-Companion/advisor/internal/talents"
)

// ProfileInput is the set_character_profile argument.
type ProfileInput struct {
	Character string `json:"character,omitempty" jsonschema:"character name; omit when only one was captured"`
	Role      string `json:"role,omitempty" jsonschema:"damage, tank, healer or balanced; required the first time"`
	Focus     string `json:"focus,omitempty" jsonschema:"leveling, dungeons, pvp or mixed; required the first time"`
	Spec      string `json:"spec,omitempty" jsonschema:"the spec tree the player chose, for example Fury"`
	StatPref  string `json:"stat_pref,omitempty" jsonschema:"optional stat preference: balanced, damage (more damage) or survival (more survivability)"`
}

// ProfileOutput is the get_character_profile and set_character_profile result.
type ProfileOutput struct {
	Character    string             `json:"character"`
	Profile      *character.Profile `json:"profile"`
	NeedsProfile bool               `json:"needs_profile" jsonschema:"no play style is saved: ask the player first"`
	Text         string             `json:"text" jsonschema:"plain-text rendering"`
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
		fmt.Fprintf(&b, "Play style: %s, %s", p.Role, p.Focus)
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

func profileOutput(c character.Character) ProfileOutput {
	return ProfileOutput{displayName(c), c.Profile, c.Profile == nil, renderProfile(c)}
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
		Description: "Save the character's play style after asking the player (role and focus are required the first time; omitted fields keep their saved value), " +
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
		saved, err := profile.Update(cfg.profilesPath(), c.Key, func(e *profile.Entry) error {
			p := character.Profile{}
			if e.Profile != nil {
				p = *e.Profile
			}
			for _, v := range []struct {
				to   *string
				from string
			}{{&p.Role, in.Role}, {&p.Focus, in.Focus}, {&p.Spec, in.Spec}, {&p.StatPref, in.StatPref}} {
				if strings.TrimSpace(v.from) != "" {
					*v.to = v.from
				}
			}
			p, err := profile.Clean(p)
			if err != nil {
				return err
			}
			e.Profile = &p
			return nil
		})
		if err != nil {
			return toolError("Not saved: " + err.Error()), ProfileOutput{}, nil
		}
		c.Profile = saved.Profile
		return nil, profileOutput(c), nil
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

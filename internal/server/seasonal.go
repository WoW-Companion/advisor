package server

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/WoW-Companion/advisor/internal/advice"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/digest"
	"github.com/WoW-Companion/advisor/internal/profile"
	"github.com/WoW-Companion/advisor/internal/questie"
)

// SeasonalInput is the seasonal_events argument.
type SeasonalInput struct {
	Character string `json:"character,omitempty" jsonschema:"character name; omit when only one was captured"`
	Set       string `json:"set,omitempty" jsonschema:"on or off, only when the player clearly asks to add or drop holiday event quests (for example 'how can I take part in Hallow's End', 'add the festival quests', 'stop showing event quests'). Omit it when the player only asks what an event is: explain it and offer to add it instead"`
	Event     string `json:"event,omitempty" jsonschema:"limit set to one event, such as Midsummer; omit for every event"`
}

// SeasonalOutput is the seasonal_events result.
type SeasonalOutput struct {
	Character string `json:"character"`
	Setting   string `json:"setting" jsonschema:"off, on, or on for the named events"`
	Changed   bool   `json:"changed"`
	Text      string `json:"text" jsonschema:"plain-text rendering: the events running today, their quests at the player's level and where they start"`
	SavedNote string `json:"saved_note,omitempty" jsonschema:"set only after a change: say it in one sentence"`
}

func seasonalText(c character.Character, qx *questie.Index, now time.Time) string {
	var b strings.Builder
	set := questie.Seasonal(c.Seasonal)
	b.WriteString("Holiday event quests are " + set.Describe() + " for this character (off by default).\n")
	run := questie.RunningEvents(now)
	if len(run) == 0 {
		b.WriteString("No holiday event is running today.\n")
	}
	who := advice.WhoOf(c)
	who.Now = now
	for _, e := range run {
		fmt.Fprintf(&b, "%s is running (until %s).\n", e.Name, e.Until())
		if c.Level == nil || qx == nil {
			continue
		}
		for _, q := range qx.EventQuests(e.Key, who) {
			fmt.Fprintf(&b, "- %s (level %d), starts with %s\n", q.Name, q.Level, q.Start)
		}
	}
	return b.String()
}

func addSeasonalTools(s *mcp.Server, cfg Config) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "seasonal_events",
		Description: "Holiday event quests (Midsummer, Hallow's End, Winter Veil...) are opt-in per character and off by default. " +
			"Reads the setting, the events running today, their quests at the player's level and where they start; with set it turns them on or off " +
			"(optionally for one event). Only change it when the player clearly asks; when they only ask what an event is, explain it and offer to add it. " +
			"After a change, confirm in one sentence what changed, and say they can say 'drop them' anytime.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in SeasonalInput) (*mcp.CallToolResult, SeasonalOutput, error) {
		f, errResult := load(cfg.SavedVariables)
		if errResult != nil {
			return errResult, SeasonalOutput{}, nil
		}
		c, err := chosen(cfg, f, in.Character)
		if err != nil {
			return toolError(err.Error()), SeasonalOutput{}, nil
		}
		now := digest.ClockOf(c, time.Now())
		out := SeasonalOutput{Character: displayName(c)}
		if set := strings.ToLower(strings.TrimSpace(in.Set)); set != "" {
			if set != "on" && set != "off" {
				return toolError("Not saved: set must be on or off"), SeasonalOutput{}, nil
			}
			key, name := "", "holiday event"
			if strings.TrimSpace(in.Event) != "" {
				e, ok := questie.FindEvent(in.Event)
				if !ok {
					return toolError("Not saved: unknown event " + in.Event), SeasonalOutput{}, nil
				}
				key, name = e.Key, e.Name
			}
			next := questie.Seasonal(c.Seasonal).With(key, set == "on")
			if _, err := profile.Update(cfg.profilesPath(), c.Key, func(e *profile.Entry) error {
				e.Seasonal = string(next)
				return nil
			}); err != nil {
				return toolError("Not saved: " + err.Error()), SeasonalOutput{}, nil
			}
			out.Changed = next != questie.Seasonal(c.Seasonal)
			c.Seasonal = string(next)
			if set == "on" {
				out.SavedNote = name + " quests are now in your plan. Say 'drop them' anytime."
				if key == "" {
					out.SavedNote = "Holiday event quests are now in your plan while an event runs. Say 'drop them' anytime."
				}
			} else {
				out.SavedNote = name + " quests are out of your plan. Ask me anytime to add them back."
				if key == "" {
					out.SavedNote = "Holiday event quests are out of your plan. Ask me anytime to add them back."
				}
			}
		}
		out.Setting = questie.Seasonal(c.Seasonal).Describe()
		qx := questie.ForSavedVariables(cfg.SavedVariables)
		out.Text = seasonalText(c, qx, now)
		if out.SavedNote != "" {
			out.Text += out.SavedNote + "\n"
		}
		return nil, out, nil
	})
}

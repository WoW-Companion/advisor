package server

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/WoW-Companion/advisor/internal/brief"
	"github.com/WoW-Companion/advisor/internal/stats"
	"github.com/WoW-Companion/advisor/internal/store"
)

// StatCheckInput is the stat_check argument.
type StatCheckInput struct {
	Character string `json:"character,omitempty" jsonschema:"character name; omit when only one was captured"`
}

// StatCheckOutput is the stat_check result.
type StatCheckOutput struct {
	Character string `json:"character"`
	stats.Result
	Text string `json:"text" jsonschema:"the insights in plain words; with needs=play_style, the choices to offer"`
}

func addStatCheck(s *mcp.Server, cfg Config) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "stat_check",
		Description: "The stat check: the character's stat priorities for their saved play style (class, spec, role, activity and stat preference), " +
			"one to three plain insights (what to raise and why, what is wasted, what is fine) and the worn items worth swapping first. " +
			"Call it for any stat or 'what should I look for' question. Priorities are ballpark and players differ: say so in one clause at most. " +
			"The totals, lean and weights are internal: never quote numbers or percentages. With no play style it returns needs=play_style: offer the choices. " +
			"Upgrade ranking and Auction House suffixes use the same priorities.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in StatCheckInput) (*mcp.CallToolResult, StatCheckOutput, error) {
		f, errResult := load(cfg.SavedVariables)
		if errResult != nil {
			return errResult, StatCheckOutput{}, nil
		}
		c, err := chosen(cfg, f, in.Character)
		if err != nil {
			return toolError(err.Error()), StatCheckOutput{}, nil
		}
		var ds store.DataStore
		if cfg.Dataset != "" {
			ds = store.NewDir(cfg.Dataset)
		}
		group := ""
		if names, _ := brief.TopGroups(c.Talents); len(names) == 1 {
			group = names[0]
		}
		r := stats.Check(c, ds, group)
		out := StatCheckOutput{Character: displayName(c), Result: r}
		switch {
		case r.Needs != "":
			out.Text = stats.Hook + ": role (damage, tank, healer) and activity (leveling, dungeons, PvP, mixed)."
		default:
			out.Text = r.Profile + "\n"
			for _, i := range r.Insights {
				out.Text += "- " + i + "\n"
			}
		}
		return nil, out, nil
	})
}

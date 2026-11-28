package server

import (
	"context"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/WoW-Companion/advisor/internal/questie"
	"github.com/WoW-Companion/advisor/internal/store"
	"github.com/WoW-Companion/advisor/internal/vendors"
)

// FindVendorInput is the find_vendor argument.
type FindVendorInput struct {
	Character string `json:"character,omitempty" jsonschema:"character name; omit when only one was captured"`
	Query     string `json:"query,omitempty" jsonschema:"what to find: an item (Instant Poison), a keyword (poison, reagents) or a subname (Shady Goods, Trade Supplies); omit to list every NPC of the kind in the zone"`
	Zone      string `json:"zone,omitempty" jsonschema:"a zone or subzone to look in (Booty Bay, Stranglethorn Vale); omit to look everywhere"`
	Here      bool   `json:"here,omitempty" jsonschema:"true for 'here' or 'near me': look in the character's current place"`
	Kind      string `json:"kind,omitempty" jsonschema:"vendor (default), trainer or flight (flight masters)"`
}

func addVendorTools(s *mcp.Server, cfg Config) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "find_vendor",
		Description: "Answers 'who sells X?', 'is there a poison / reagent / trade-goods vendor in <place>?', 'which vendor is here?', 'where is the trainer / flight master?' for one captured character (by name; omit if only one). " +
			"Searches the dataset's vendor offers and NPCs plus the player's QuestieDB subnames and vendor types and the shops the player's characters opened, neutral vendors included. " +
			"Each answer names the NPC, its subname, the zone and coordinates and what it sells. Call it BEFORE saying a vendor, trainer or flight master is unknown: " +
			"never say you cannot confirm one without calling this first. When it finds nothing, say it is not in your data.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in FindVendorInput) (*mcp.CallToolResult, vendors.Result, error) {
		f, errResult := load(cfg.SavedVariables)
		if errResult != nil {
			return errResult, vendors.Result{}, nil
		}
		c, err := chosen(cfg, f, in.Character)
		if err != nil {
			return toolError(err.Error()), vendors.Result{}, nil
		}
		var ds store.DataStore
		if cfg.Dataset != "" {
			ds = store.NewDir(cfg.Dataset)
		}
		qx := questie.ForSavedVariables(cfg.SavedVariables)
		q := vendors.Query{Text: in.Query, Zone: strings.TrimSpace(in.Zone), Kind: in.Kind, Faction: c.Faction, Observed: vendors.ObservedOf(f.Characters...)}
		if p := c.LastKnownPosition(); p != nil {
			q.At = questie.Where{MapID: p.MapID, Zone: p.Zone, X: p.X, Y: p.Y}
		} else if c.Location != nil {
			q.At = questie.Where{Zone: c.Location.Zone}
		}
		if in.Here && q.Zone == "" && c.Location != nil {
			q.Zone = c.Location.Subzone
			res := vendors.Search(ds, qx, q)
			if len(res.Found) == 0 && c.Location.Zone != "" && q.Zone != c.Location.Zone {
				q.Zone = c.Location.Zone
				res = vendors.Search(ds, qx, q)
			}
			return nil, res, nil
		}
		return nil, vendors.Search(ds, qx, q), nil
	})
}

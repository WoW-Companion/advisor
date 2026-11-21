// Package server is the MCP server and its tool registry, shared by every transport.
package server

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/WoW-Companion/advisor/internal/advice"
	"github.com/WoW-Companion/advisor/internal/brief"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/dataset"
	"github.com/WoW-Companion/advisor/internal/gear"
	"github.com/WoW-Companion/advisor/internal/lua"
	"github.com/WoW-Companion/advisor/internal/store"
)

// Version is the advisor's version.
const Version = "0.2.0"

const (
	DatasetEnv        = "WOWC_DATASET"
	SavedVariablesEnv = "WOWC_SAVEDVARIABLES"
)

// Config says where the dataset and the character file are; empty means none. Empty fields fall
// back to the environment (see FromEnv).
type Config struct {
	Dataset        string
	SavedVariables string
}

// FromEnv fills empty fields from $WOWC_DATASET and $WOWC_SAVEDVARIABLES.
func (c Config) FromEnv() Config {
	if c.Dataset == "" {
		c.Dataset = os.Getenv(DatasetEnv)
	}
	if c.SavedVariables == "" {
		c.SavedVariables = os.Getenv(SavedVariablesEnv)
	}
	return c
}

// ClientInfo is the game client a capture was written by.
type ClientInfo struct {
	Version string `json:"version"`
	Build   string `json:"build"`
	TOC     string `json:"toc"`
}

// Info is the server_info result.
type Info struct {
	Version              string      `json:"version"`
	Dataset              string      `json:"dataset" jsonschema:"dataset status"`
	DatasetPath          *string     `json:"dataset_path"`
	ManifestVersion      *string     `json:"manifest_version"`
	GameBuild            *string     `json:"game_build"`
	DatasetMatchesClient *bool       `json:"dataset_matches_client" jsonschema:"dataset build equals the capture's client build"`
	DatasetWarning       *string     `json:"dataset_warning"`
	SavedVariables       string      `json:"savedvariables" jsonschema:"character file status"`
	SavedVariablesPath   *string     `json:"savedvariables_path"`
	Characters           *int        `json:"characters"`
	CharactersCapturedAt *string     `json:"characters_captured_at"`
	Client               *ClientInfo `json:"client" jsonschema:"game client that wrote the capture"`
}

func strp(s string) *string { return &s }

func clientInfo(c character.Client) *ClientInfo {
	if c.Empty() {
		return nil
	}
	return &ClientInfo{c.Version, c.Build, c.TOC}
}

// fileClient is the file's client block, else the first character's that has one.
func fileClient(f character.File) character.Client {
	if !f.Client.Empty() {
		return f.Client
	}
	for _, c := range f.Characters {
		if !c.Client.Empty() {
			return c.Client
		}
	}
	return character.Client{}
}

// Describe reports on this server, the dataset it found and the character file it was given.
func Describe(cfg Config) Info {
	info := Info{
		Version:        Version,
		Dataset:        "no dataset configured",
		SavedVariables: "no SavedVariables file configured",
	}
	if cfg.SavedVariables != "" {
		info.SavedVariablesPath = strp(cfg.SavedVariables)
		describeCharacters(&info, cfg.SavedVariables)
	}
	if cfg.Dataset == "" {
		return info
	}
	info.DatasetPath = strp(cfg.Dataset)
	manifest, err := store.ReadManifest(cfg.Dataset)
	switch {
	case err != nil:
		info.Dataset = "could not read manifest: " + err.Error()
	case manifest == nil:
		info.Dataset = "no manifest found"
	default:
		info.Dataset = "ok"
		for _, key := range []string{"dataset_version", "version"} {
			if v, ok := manifest[key]; ok {
				info.ManifestVersion = strp(toString(v))
				break
			}
		}
		if v, ok := manifest["game_build"]; ok {
			info.GameBuild = strp(toString(v))
		}
	}
	compareBuilds(&info)
	return info
}

// compareBuilds says whether the dataset is for the build the capture was written by, and warns
// plainly when it is not.
func compareBuilds(info *Info) {
	if info.GameBuild == nil || info.Client == nil || info.Client.Build == "" {
		return
	}
	ok := dataset.Matches(*info.GameBuild, info.Client.Version, info.Client.Build)
	info.DatasetMatchesClient = &ok
	if !ok {
		info.DatasetWarning = strp("dataset is for game build " + *info.GameBuild + " but the capture was written by client build " +
			info.Client.Build + "; ids and values may differ. Run `wowc-advisor dataset update` to fetch the matching build.")
	}
}

// CaptureClient is the game client that wrote the capture at path, empty when it cannot be read.
func CaptureClient(path string) character.Client {
	if path == "" {
		return character.Client{}
	}
	f, err := character.LoadFile(path)
	if err != nil {
		return character.Client{}
	}
	return fileClient(f)
}

func toString(v any) string {
	if f, ok := v.(float64); ok && f == float64(int64(f)) {
		return itoa(int64(f))
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmtAny(v)
}

func describeCharacters(info *Info, path string) {
	loaded, err := character.LoadFile(path)
	var perr *lua.ParseError
	switch {
	case errors.Is(err, fs.ErrNotExist):
		info.SavedVariables = "file not found"
	case errors.As(err, &perr):
		info.SavedVariables = "could not parse: " + err.Error()
	case err != nil:
		info.SavedVariables = "unreadable: " + err.Error()
	default:
		info.SavedVariables = "ok"
		n := len(loaded.Characters)
		info.Characters = &n
		if t := loaded.CapturedAt(); t != nil {
			info.CharactersCapturedAt = strp(time.Unix(*t, 0).UTC().Format(time.RFC3339))
		}
		info.Client = clientInfo(fileClient(loaded))
	}
}

func toolError(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: msg}}}
}

func load(path string) (character.File, *mcp.CallToolResult) {
	if path == "" {
		return character.File{}, toolError("No SavedVariables file configured (set WOWC_SAVEDVARIABLES).")
	}
	f, err := character.LoadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return f, toolError("SavedVariables file not found: " + path)
	case err != nil:
		return f, toolError("Could not read the SavedVariables file: " + err.Error())
	}
	return f, nil
}

type noInput struct{}

// ListOutput is the list_characters result.
type ListOutput struct {
	Characters []brief.Entry `json:"characters"`
}

// BriefInput is the character_brief argument.
type BriefInput struct {
	Character string `json:"character,omitempty" jsonschema:"character name; omit when only one was captured"`
}

// NextInput is the whats_next argument.
type NextInput struct {
	Character string `json:"character,omitempty" jsonschema:"character name; omit when only one was captured"`
	Limit     int    `json:"limit,omitempty" jsonschema:"most steps to return (default 8)"`
}

// NextOutput is the whats_next result.
type NextOutput struct {
	Character string        `json:"character"`
	Steps     []advice.Step `json:"steps"`
	Total     int           `json:"total" jsonschema:"steps before the limit"`
	Text      string        `json:"text" jsonschema:"plain-text rendering of the steps"`
}

// UpgradeInput is the upgrade_finder argument.
type UpgradeInput struct {
	Character string `json:"character,omitempty" jsonschema:"character name; omit when only one was captured"`
	Slot      string `json:"slot,omitempty" jsonschema:"one slot (head, neck, shoulder, back, chest, wrist, hands, waist, legs, feet, finger1, finger2, trinket1, trinket2, mainhand, offhand, ranged); omit for all"`
	Limit     int    `json:"limit,omitempty" jsonschema:"most upgrades per slot (default 3)"`
}

// UpgradeOutput is the upgrade_finder result.
type UpgradeOutput struct {
	Character     string                      `json:"character"`
	Spec          string                      `json:"spec,omitempty" jsonschema:"spec the stat weights are for; empty when none apply"`
	ItemLevelOnly bool                        `json:"item_level_only" jsonschema:"true when upgrades are ranked by item level alone"`
	Slots         map[string][]gear.Candidate `json:"slots"`
	Text          string                      `json:"text" jsonschema:"plain-text rendering"`
}

// noUpgrades is the result accompanying an error; the output schema wants a map, not null.
var noUpgrades = UpgradeOutput{Slots: map[string][]gear.Candidate{}}

// DefaultUpgradeLimit is how many upgrades per slot upgrade_finder returns when none is asked for.
const DefaultUpgradeLimit = 3

// DefaultLimit is how many steps whats_next returns when none is asked for.
const DefaultLimit = 8

// New builds the server with its tools. The character file is read on every call.
func New(cfg Config) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "wowc-advisor", Version: Version}, nil)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "server_info",
		Description: "Return the advisor version, dataset and game build, the character file's status and the game client that wrote the capture.",
	}, func(context.Context, *mcp.CallToolRequest, noInput) (*mcp.CallToolResult, Info, error) {
		return nil, Describe(cfg), nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_characters",
		Description: "List the characters in the ForeverScribe capture: name, level, class, realm, capture time.",
	}, func(context.Context, *mcp.CallToolRequest, noInput) (*mcp.CallToolResult, ListOutput, error) {
		f, errResult := load(cfg.SavedVariables)
		if errResult != nil {
			return errResult, ListOutput{}, nil
		}
		return nil, ListOutput{brief.List(f)}, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "character_brief",
		Description: "Brief of one captured character (by name; omit if only one): level, class, race, XP, money, " +
			"spec and talent points, gear, professions, bank and bags, quests, the game client and the capture time. " +
			"Source: ForeverScribe capture.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in BriefInput) (*mcp.CallToolResult, any, error) {
		f, errResult := load(cfg.SavedVariables)
		if errResult != nil {
			return errResult, nil, nil
		}
		c, err := brief.Choose(f, in.Character)
		if err != nil {
			return toolError(err.Error()), nil, nil
		}
		text := brief.Character(c, f.ClientOf(c), time.Now())
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, nil, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "whats_next",
		Description: "Ranked next steps for one captured character (by name; omit if only one; limit defaults to 8): " +
			"quests to turn in, unspent talent points, open quests near their level, failed quests to abandon. " +
			"Each step gives its reason and its source (capture field or dataset row); unknown facts are said to be unknown.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in NextInput) (*mcp.CallToolResult, NextOutput, error) {
		f, errResult := load(cfg.SavedVariables)
		if errResult != nil {
			return errResult, NextOutput{}, nil
		}
		c, err := brief.Choose(f, in.Character)
		if err != nil {
			return toolError(err.Error()), NextOutput{}, nil
		}
		var ds store.DataStore
		if cfg.Dataset != "" {
			ds = store.NewDir(cfg.Dataset)
		}
		all := advice.WhatsNext(c, ds)
		limit := in.Limit
		if limit <= 0 {
			limit = DefaultLimit
		}
		steps := advice.Limit(all, limit)
		if steps == nil {
			steps = []advice.Step{}
		}
		name := c.Name
		if name == "" {
			name = c.Key
		}
		return nil, NextOutput{name, steps, len(all), advice.Render(c, steps, len(all))}, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "upgrade_finder",
		Description: "Gear upgrades for one captured character (by name; omit if only one) per slot (optional slot; limit defaults to 3 per slot): " +
			"items their class can use and they can equip within 3 levels that beat what they wear, with where each drops. " +
			"Ranked by item level weighted by the spec's stat types, or by item level alone when the dataset has no stats; the answer says which.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in UpgradeInput) (*mcp.CallToolResult, UpgradeOutput, error) {
		f, errResult := load(cfg.SavedVariables)
		if errResult != nil {
			return errResult, noUpgrades, nil
		}
		c, err := brief.Choose(f, in.Character)
		if err != nil {
			return toolError(err.Error()), noUpgrades, nil
		}
		slot := strings.ToLower(strings.TrimSpace(in.Slot))
		if slot != "" && !slices.Contains(brief.Slots, slot) {
			return toolError("Unknown slot '" + in.Slot + "'; use one of: " + strings.Join(brief.Slots, ", ") + "."), noUpgrades, nil
		}
		if cfg.Dataset == "" {
			return toolError("No dataset configured (set WOWC_DATASET)."), noUpgrades, nil
		}
		limit := in.Limit
		if limit <= 0 {
			limit = DefaultUpgradeLimit
		}
		group := ""
		if names, _ := brief.TopGroups(c.Talents); len(names) == 1 {
			group = names[0]
		}
		res, err := gear.Find(c, store.NewDir(cfg.Dataset), group, gear.Options{Slot: slot, Limit: limit})
		if err != nil {
			return toolError("Could not find upgrades: " + err.Error()), noUpgrades, nil
		}
		name := c.Name
		if name == "" {
			name = c.Key
		}
		return nil, UpgradeOutput{name, res.Spec, res.ItemLevelOnly, res.Slots, gear.Render(c, res)}, nil
	})
	return s
}

// ServeStdio serves over stdin and stdout until the client disconnects.
func ServeStdio(ctx context.Context, s *mcp.Server) error {
	return s.Run(ctx, &mcp.StdioTransport{})
}

// HTTPHandler serves streamable HTTP; mount it at /mcp.
func HTTPHandler(s *mcp.Server) http.Handler {
	return mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return s }, nil)
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func fmtAny(v any) string { return fmt.Sprint(v) }

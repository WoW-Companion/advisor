// Package server is the MCP server and its tool registry, shared by every transport.
package server

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/WoW-Companion/advisor/internal/advice"
	"github.com/WoW-Companion/advisor/internal/brief"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/dataset"
	"github.com/WoW-Companion/advisor/internal/digest"
	"github.com/WoW-Companion/advisor/internal/doctor"
	"github.com/WoW-Companion/advisor/internal/dungeons"
	"github.com/WoW-Companion/advisor/internal/farm"
	"github.com/WoW-Companion/advisor/internal/gear"
	"github.com/WoW-Companion/advisor/internal/inventory"
	"github.com/WoW-Companion/advisor/internal/lua"
	"github.com/WoW-Companion/advisor/internal/professions"
	"github.com/WoW-Companion/advisor/internal/setup"
	"github.com/WoW-Companion/advisor/internal/store"
	"github.com/WoW-Companion/advisor/internal/talents"
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
	DigestPath     string // the digest file to write; empty derives it from SavedVariables
	Cache          string // dataset cache directory for doctor; empty is the per-user cache
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

// PlanInput is the profession_plan argument.
type PlanInput struct {
	Character  string `json:"character,omitempty" jsonschema:"character name; omit when only one was captured"`
	Profession string `json:"profession,omitempty" jsonschema:"one profession (for example Tailoring); omit for all the character's professions"`
}

// PlanOutput is the profession_plan result.
type PlanOutput struct {
	Character string                 `json:"character"`
	Plans     []professions.Leveling `json:"plans"`
	Text      string                 `json:"text" jsonschema:"plain-text rendering"`
}

// TalentInput is the talent_plan argument.
type TalentInput struct {
	Character string `json:"character,omitempty" jsonschema:"character name; omit when only one was captured"`
	Spec      string `json:"spec,omitempty" jsonschema:"spec to plan for (for example Shadow Magic); omit to use the group with the most points spent"`
	Limit     int    `json:"limit,omitempty" jsonschema:"how many points to list (default 5)"`
}

// TalentOutput is the talent_plan result.
type TalentOutput struct {
	Character string         `json:"character"`
	Plan      talents.Result `json:"plan"`
	Text      string         `json:"text" jsonschema:"plain-text rendering"`
}

// DungeonInput is the dungeon_plan argument.
type DungeonInput struct {
	Character string `json:"character,omitempty" jsonschema:"character name; omit when only one was captured"`
	Limit     int    `json:"limit,omitempty" jsonschema:"how many ranked dungeons to list (default 3)"`
}

// DungeonOutput is the dungeon_plan result.
type DungeonOutput struct {
	Character string                 `json:"character"`
	Picks     []dungeons.DungeonPick `json:"picks"`
	Unknown   []dungeons.DungeonPick `json:"level_unknown"`
	Text      string                 `json:"text" jsonschema:"plain-text rendering"`
}

// FarmInput is the farm_spots argument.
type FarmInput struct {
	Character string `json:"character,omitempty" jsonschema:"character name; omit when only one was captured"`
	Item      string `json:"item" jsonschema:"item id or name"`
	Limit     int    `json:"limit,omitempty" jsonschema:"how many spots to list (default 5)"`
}

// FarmOutput is the farm_spots result.
type FarmOutput struct {
	Character string      `json:"character"`
	ItemID    int64       `json:"item_id"`
	Item      string      `json:"item"`
	Spots     []farm.Spot `json:"spots"`
	Crafted   bool        `json:"crafted" jsonschema:"the item can also be crafted; see profession_plan"`
	Text      string      `json:"text" jsonschema:"plain-text rendering"`
}

// DefaultFarmLimit is how many spots farm_spots lists when none is asked for.
const DefaultFarmLimit = 5

// InventoryInput is the inventory_review argument.
type InventoryInput struct {
	Character string `json:"character,omitempty" jsonschema:"character name; omit when only one was captured"`
}

// InventoryOutput is the inventory_review result.
type InventoryOutput struct {
	Character      string                       `json:"character"`
	Groups         map[string][]inventory.Entry `json:"groups" jsonschema:"stacks per group: equip, keep, quest, vendor, bank, unknown"`
	VendorValue    *int64                       `json:"vendor_value_copper" jsonschema:"copper the priced vendor stacks sell for; null when no price is known"`
	VendorUnpriced int                          `json:"vendor_unpriced" jsonschema:"vendor stacks with no known price"`
	SlotsFreed     int                          `json:"bag_slots_freed"`
	Notes          []string                     `json:"notes"`
	Text           string                       `json:"text" jsonschema:"plain-text rendering"`
}

// DefaultDungeonLimit is how many ranked dungeons dungeon_plan lists when none is asked for.
const DefaultDungeonLimit = 3

// DefaultUpgradeLimit is how many upgrades per slot upgrade_finder returns when none is asked for.
const DefaultUpgradeLimit = 3

// DefaultLimit is how many steps whats_next returns when none is asked for.
const DefaultLimit = 8

// DigestInput is the update_digest argument.
type DigestInput struct {
	Character string `json:"character,omitempty" jsonschema:"character name; omit when only one was captured"`
}

// DigestOutput is the update_digest result.
type DigestOutput struct {
	Character string         `json:"character"`
	Path      string         `json:"path" jsonschema:"the Digest.lua file written"`
	Lines     map[string]int `json:"lines" jsonschema:"lines written, per section id"`
	Text      string         `json:"text" jsonschema:"plain-text summary, with the /reload reminder"`
}

// WriteDigest builds the digest for one captured character and writes it atomically to the slot
// addon. The slot addon must already be installed; nothing is created.
func WriteDigest(cfg Config, name string) (DigestOutput, error) {
	f, errResult := load(cfg.SavedVariables)
	if errResult != nil {
		return DigestOutput{}, errors.New(errResult.Content[0].(*mcp.TextContent).Text)
	}
	c, err := brief.Choose(f, name)
	if err != nil {
		return DigestOutput{}, err
	}
	path := cfg.DigestPath
	if path == "" {
		if path, err = digest.DigestPath(cfg.SavedVariables); err != nil {
			return DigestOutput{}, err
		}
		if err := digest.CheckInstalled(path); err != nil {
			return DigestOutput{}, err
		}
	}
	opt := digest.Options{Now: time.Now(), Client: f.ClientOf(c)}
	var ds store.DataStore
	if cfg.Dataset != "" {
		ds = store.NewDir(cfg.Dataset)
		if m, err := store.ReadManifest(cfg.Dataset); err == nil && m != nil {
			if v, ok := m["game_build"]; ok {
				opt.DatasetBuild = toString(v)
			}
		}
	}
	d := digest.Build(c, ds, opt)
	if err := digest.Write(path, d); err != nil {
		return DigestOutput{}, fmt.Errorf("could not write the digest: %w", err)
	}
	out := DigestOutput{Character: d.Character, Path: path, Lines: map[string]int{}}
	var counts []string
	for _, s := range d.Sections {
		out.Lines[s.ID] = len(s.Lines)
		counts = append(counts, fmt.Sprintf("%s %d", s.ID, len(s.Lines)))
	}
	if len(counts) == 0 {
		counts = []string{"no advice"}
	}
	out.Text = fmt.Sprintf("Wrote the digest for %s to %s (%s). Type /reload in game to show it.", d.Character, path, strings.Join(counts, ", "))
	return out, nil
}

// Diagnose checks the whole setup for the configured SavedVariables file.
func Diagnose(cfg Config) doctor.Report {
	env := setup.Env{GOOS: runtime.GOOS, AppData: os.Getenv("APPDATA")}
	env.Home, _ = os.UserHomeDir()
	env.Cwd, _ = os.Getwd()
	return doctor.Run(doctor.Options{SavedVariables: cfg.SavedVariables, Cache: cfg.Cache, Env: env})
}

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

	mcp.AddTool(s, &mcp.Tool{
		Name: "profession_plan",
		Description: "Leveling plan per profession for one captured character (by name; omit if only one; optional profession): " +
			"the next 3-5 recipes that still give skill-ups at their skill, orange before yellow, with each reagent, how many the bags and bank hold and what is missing. " +
			"Missing thresholds, reagents or a missing bank capture are said to be unknown.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in PlanInput) (*mcp.CallToolResult, PlanOutput, error) {
		none := PlanOutput{Plans: []professions.Leveling{}}
		f, errResult := load(cfg.SavedVariables)
		if errResult != nil {
			return errResult, none, nil
		}
		c, err := brief.Choose(f, in.Character)
		if err != nil {
			return toolError(err.Error()), none, nil
		}
		if cfg.Dataset == "" {
			return toolError("No dataset configured (set WOWC_DATASET)."), none, nil
		}
		ds := store.NewDir(cfg.Dataset)
		plans := []professions.Leveling{}
		want := strings.TrimSpace(in.Profession)
		var have []string
		for _, p := range c.Professions {
			have = append(have, p.Name)
			if want == "" || strings.EqualFold(p.Name, want) {
				plans = append(plans, professions.Plan(c, ds, p.Name))
			}
		}
		if want != "" && len(plans) == 0 {
			return toolError("Profession '" + want + "' is not one of this character's professions (" + strings.Join(have, ", ") + ")."), none, nil
		}
		name := c.Name
		if name == "" {
			name = c.Key
		}
		return nil, PlanOutput{name, plans, professions.Render(plans)}, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "talent_plan",
		Description: "Where the next talent points go for one captured character (by name; omit if only one; optional spec and limit, default 5): " +
			"compares the points spent with the curated build for their spec and level band and lists the next points in order with the build's reason. " +
			"Points spent off-build are listed, not judged. With no build for the spec or band it says so and lists the builds available.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in TalentInput) (*mcp.CallToolResult, TalentOutput, error) {
		none := TalentOutput{}
		f, errResult := load(cfg.SavedVariables)
		if errResult != nil {
			return errResult, none, nil
		}
		c, err := brief.Choose(f, in.Character)
		if err != nil {
			return toolError(err.Error()), none, nil
		}
		if cfg.Dataset == "" {
			return toolError("No dataset configured (set WOWC_DATASET)."), none, nil
		}
		plan := talents.Plan(c, store.NewDir(cfg.Dataset), talents.Options{Spec: in.Spec, Limit: in.Limit})
		name := c.Name
		if name == "" {
			name = c.Key
		}
		return nil, TalentOutput{name, plan, talents.Render(plan)}, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "dungeon_plan",
		Description: "Which dungeons fit one captured character (by name; omit if only one; optional limit, default 3): " +
			"dungeons whose level range is within 3 levels of theirs, ranked by how many boss drops are upgrades for their spec, then level fit, " +
			"with the boss, drop chance and any open quests that lead there. Dungeons with no level range are listed apart, unranked; bosses with no creature link are named, not guessed.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in DungeonInput) (*mcp.CallToolResult, DungeonOutput, error) {
		none := DungeonOutput{}
		f, errResult := load(cfg.SavedVariables)
		if errResult != nil {
			return errResult, none, nil
		}
		c, err := brief.Choose(f, in.Character)
		if err != nil {
			return toolError(err.Error()), none, nil
		}
		if cfg.Dataset == "" {
			return toolError("No dataset configured (set WOWC_DATASET)."), none, nil
		}
		limit := in.Limit
		if limit <= 0 {
			limit = DefaultDungeonLimit
		}
		res, err := dungeons.Plan(c, store.NewDir(cfg.Dataset))
		if err != nil {
			return toolError("Could not plan dungeons: " + err.Error()), none, nil
		}
		name := c.Name
		if name == "" {
			name = c.Key
		}
		picks := res.Picks
		if len(picks) > limit {
			picks = picks[:limit]
		}
		return nil, DungeonOutput{name, picks, res.Unknown, dungeons.Render(c, res, limit)}, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "farm_spots",
		Description: "Where to farm an item for one captured character (by name; omit if only one; item id or name; optional limit, default 5): " +
			"the creatures and objects that drop it, those in the character's level range first, then by drop chance (drops per loot window), with level range and zone. " +
			"Unknown zones, levels and chances are said to be unknown; crafted sources are left out and point to profession_plan.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in FarmInput) (*mcp.CallToolResult, FarmOutput, error) {
		none := FarmOutput{Spots: []farm.Spot{}}
		f, errResult := load(cfg.SavedVariables)
		if errResult != nil {
			return errResult, none, nil
		}
		c, err := brief.Choose(f, in.Character)
		if err != nil {
			return toolError(err.Error()), none, nil
		}
		if cfg.Dataset == "" {
			return toolError("No dataset configured (set WOWC_DATASET)."), none, nil
		}
		if strings.TrimSpace(in.Item) == "" {
			return toolError("Give an item id or name."), none, nil
		}
		ds := store.NewDir(cfg.Dataset)
		id, item, cands := farm.Resolve(ds, in.Item)
		if len(cands) > 0 {
			return toolError("'" + in.Item + "' matches several items: " + strings.Join(cands, ", ") + ". Use the id."), none, nil
		}
		if id == 0 {
			return toolError("No item matches '" + in.Item + "'."), none, nil
		}
		limit := in.Limit
		if limit <= 0 {
			limit = DefaultFarmLimit
		}
		spots := farm.Spots(c, ds, id)
		crafted := farm.Crafted(ds, id)
		name := c.Name
		if name == "" {
			name = c.Key
		}
		text := farm.Render(item, spots, crafted, limit)
		if len(spots) > limit {
			spots = spots[:limit]
		}
		return nil, FarmOutput{name, id, item, spots, crafted, text}, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "inventory_review",
		Description: "What to do with the bags and bank of one captured character (by name; omit if only one): each stack goes to the first group that fits, " +
			"equip now (a gear upgrade), keep for professions, quest item, vendor (grey quality), bank, or unknown, with its reason, " +
			"plus the vendor value and bag slots freed. Prices, quest links and items the dataset lacks are reported as unknown, never guessed. A dataset is optional but without it most items are unknown.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in InventoryInput) (*mcp.CallToolResult, InventoryOutput, error) {
		none := InventoryOutput{Groups: map[string][]inventory.Entry{}, Notes: []string{}}
		f, errResult := load(cfg.SavedVariables)
		if errResult != nil {
			return errResult, none, nil
		}
		c, err := brief.Choose(f, in.Character)
		if err != nil {
			return toolError(err.Error()), none, nil
		}
		var ds store.DataStore
		if cfg.Dataset != "" {
			ds = store.NewDir(cfg.Dataset)
		}
		r := inventory.Review(c, ds)
		name := c.Name
		if name == "" {
			name = c.Key
		}
		out := InventoryOutput{Character: name, Groups: map[string][]inventory.Entry{}, VendorUnpriced: r.VendorUnpriced,
			SlotsFreed: r.SlotsFreed, Notes: append([]string{}, r.Notes...), Text: inventory.Render(r)}
		for g, l := range r.Groups {
			out.Groups[string(g)] = l
		}
		if n := len(r.Groups[inventory.Vendor]); n > r.VendorUnpriced {
			out.VendorValue = &r.VendorValue
		}
		return nil, out, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "update_digest",
		Description: "Write the in-game digest for one captured character (by name; omit if only one) into the WoWCompanion_Digest addon: " +
			"next quests, gear upgrades, materials to keep and notes, from whats_next and upgrade_finder. " +
			"Replaces Digest.lua; the player then types /reload in game.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in DigestInput) (*mcp.CallToolResult, DigestOutput, error) {
		out, err := WriteDigest(cfg, in.Character)
		if err != nil {
			return toolError(err.Error()), DigestOutput{Lines: map[string]int{}}, nil
		}
		return nil, out, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "doctor",
		Description: "Check the whole setup and say how to fix each failure: the SavedVariables file, the capture's age, the three WoW Companion addons, " +
			"the dataset cache and its build against the capture's client, whether the digest and answers files can be written, and which AI client configs mention wowc-advisor (informational).",
	}, func(context.Context, *mcp.CallToolRequest, noInput) (*mcp.CallToolResult, doctor.Report, error) {
		return nil, Diagnose(cfg), nil
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

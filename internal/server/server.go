// Package server is the MCP server and its tool registry, shared by every transport.
package server

import (
	"cmp"
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
	"github.com/WoW-Companion/advisor/internal/items"
	"github.com/WoW-Companion/advisor/internal/lua"
	"github.com/WoW-Companion/advisor/internal/names"
	"github.com/WoW-Companion/advisor/internal/paths"
	"github.com/WoW-Companion/advisor/internal/professions"
	"github.com/WoW-Companion/advisor/internal/profile"
	"github.com/WoW-Companion/advisor/internal/questie"
	"github.com/WoW-Companion/advisor/internal/quests"
	"github.com/WoW-Companion/advisor/internal/route"
	"github.com/WoW-Companion/advisor/internal/setup"
	"github.com/WoW-Companion/advisor/internal/store"
	"github.com/WoW-Companion/advisor/internal/svfind"
	"github.com/WoW-Companion/advisor/internal/talents"
	"github.com/WoW-Companion/advisor/internal/unlock"
	"github.com/WoW-Companion/advisor/internal/wording"
)

// Version is the advisor's version. Release builds set it from main at link time.
var Version = "dev"

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
	SkipUnchanged  bool   // leave the file alone when only the generated times would differ
	Cache          string // dataset cache directory for doctor; empty is the per-user cache

	Profiles string // the play-style store; empty is profile.Path()

	// SVSource says where SavedVariables came from once FromEnv has run: SVFlag, SVEnv or SVAuto.
	SVSource string
	// SVNote is the one line telling which file auto-detect picked; empty otherwise.
	SVNote string
}

// Where SavedVariables came from.
const (
	SVFlag = "flag"
	SVEnv  = "env"
	SVAuto = "auto"
)

// FromEnv fills empty fields from $WOWC_DATASET and $WOWC_SAVEDVARIABLES. An empty SavedVariables
// that the environment leaves empty is searched for in the standard WoW install folders; an explicit
// flag or variable always wins.
func (c Config) FromEnv() Config {
	if c.Dataset == "" {
		c.Dataset = os.Getenv(DatasetEnv)
	}
	switch {
	case c.SavedVariables != "":
		c.SVSource = SVFlag
	case os.Getenv(SavedVariablesEnv) != "":
		c.SavedVariables, c.SVSource = os.Getenv(SavedVariablesEnv), SVEnv
	default:
		if res := svfind.Detect(); res.Path != "" {
			c.SavedVariables, c.SVSource, c.SVNote = res.Path, SVAuto, res.Note(SavedVariablesEnv)
		}
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

func (c Config) profilesPath() string {
	if c.Profiles != "" {
		return c.Profiles
	}
	return profile.Path()
}

// withTotals attaches the stat totals the addon recorded (WoWCompanionDB.stats), when it did.
func withTotals(cfg Config, c character.Character) character.Character {
	return character.AttachTotals(unlock.AddonSV(cfg.SavedVariables), c)
}

// chosen is brief.Choose with the player's saved play style and talent plan attached.
func chosen(cfg Config, f character.File, name string) (character.Character, error) {
	c, err := brief.Choose(f, name)
	if err != nil {
		return c, err
	}
	return withTotals(cfg, profile.Attach(cfg.profilesPath(), c)), nil
}

func toolError(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: msg}}}
}

func load(path string) (character.File, *mcp.CallToolResult) {
	if path == "" {
		return character.File{}, toolError("No SavedVariables file configured: " + svfind.Detect().NotFound(SavedVariablesEnv))
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
	Limit     int    `json:"limit,omitempty" jsonschema:"most upgrades per slot (default 3, or 1 with brief)"`
	Brief     bool   `json:"brief,omitempty" jsonschema:"compact answer: the text rendering only, one upgrade per slot unless limit is set"`
}

// UpgradeOutput is the upgrade_finder result.
type UpgradeOutput struct {
	Character     string                      `json:"character"`
	Spec          string                      `json:"spec,omitempty" jsonschema:"spec the stat weights are for; empty when none apply"`
	ItemLevelOnly bool                        `json:"item_level_only" jsonschema:"true when upgrades are ranked by item level alone"`
	Slots         map[string][]gear.Candidate `json:"slots"`
	Text          string                      `json:"text" jsonschema:"plain-text rendering"`
}

// PathsInput is the upgrade_paths argument: an item, or a slot.
type PathsInput struct {
	Character string `json:"character,omitempty" jsonschema:"character name; omit when only one was captured"`
	Item      string `json:"item,omitempty" jsonschema:"item name, exact or part of it; give this or slot"`
	Slot      string `json:"slot,omitempty" jsonschema:"one slot (head, neck, shoulder, back, chest, wrist, hands, waist, legs, feet, finger1, finger2, trinket1, trinket2, mainhand, offhand, ranged): the best upgrades there that can be got"`
}

// PathsOutput is the upgrade_paths result: names only, never ids.
type PathsOutput struct {
	Character string         `json:"character"`
	Results   []paths.Result `json:"results" jsonschema:"one per item, each with its ways to get it, best first"`
	Text      string         `json:"text" jsonschema:"plain-words rendering"`
}

// noPaths is the result accompanying an error; the output schema wants a list, not null.
var noPaths = PathsOutput{Results: []paths.Result{}}

// stripIDs removes the "(id)" the name resolver appends: a player-facing list names items only.
func stripIDs(l []string) []string {
	out := make([]string, len(l))
	for i, s := range l {
		if j := strings.LastIndex(s, " ("); j > 0 {
			s = s[:j]
		}
		out[i] = s
	}
	return out
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
	Limit     int    `json:"limit,omitempty" jsonschema:"how many ranked dungeons to list (default 3, or 2 with brief)"`
	Brief     bool   `json:"brief,omitempty" jsonschema:"compact answer: the text rendering only, not the same data again as fields"`
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

// ChainInput is the quest_chain argument.
type ChainInput struct {
	Quest string `json:"quest" jsonschema:"quest id or name"`
}

// ChainOutput is the quest_chain result.
type ChainOutput struct {
	Quest string       `json:"quest" jsonschema:"quest name, empty when the dataset has none"`
	Chain quests.Chain `json:"chain"`
	Text  string       `json:"text" jsonschema:"plain-text rendering"`
}

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
	Character string `json:"character,omitempty" jsonschema:"a character that must be among those captured; every captured character is written either way"`
}

// DigestOutput is the update_digest result.
type DigestOutput struct {
	Characters []string                  `json:"characters" jsonschema:"the characters written, most recently captured first"`
	Skipped    []string                  `json:"skipped,omitempty" jsonschema:"captured characters left out: past the 30 most recent, or a repeated name"`
	Path       string                    `json:"path" jsonschema:"the Digest.lua file written"`
	Lines      map[string]map[string]int `json:"lines" jsonschema:"lines written, per character and section id"`
	Text       string                    `json:"text" jsonschema:"plain-text summary, with the /reload reminder"`
	Unchanged  bool                      `json:"unchanged,omitempty" jsonschema:"the file already held this digest and was not rewritten"`
	PlanPath   string                    `json:"plan_path,omitempty" jsonschema:"the Plan.lua file written beside it: the quest route per character"`
}

// MaxPlanText is how many stops quest_plan's text lists; the plan itself holds them all.
const MaxPlanText = 20

// RouteOutput is the quest_plan result.
type RouteOutput struct {
	Plan route.Plan `json:"plan"`
	Text string     `json:"text" jsonschema:"plain-text route"`
}

// NotCapturedError is what a digest asked for an uncaptured character says.
func NotCapturedError(name string) error {
	return fmt.Errorf("%s isn't in your SavedVariables yet. In game, /reload on that character so ForeverScribe saves it, then run this again.", name)
}

// WriteDigest builds the digest of every captured character (at most digest.MaxCharacters, the most
// recently captured first) and writes it atomically to the slot addon. A non-empty name must be among
// the captured characters, or nothing is written. The slot addon must already be installed; nothing
// is created.
func WriteDigest(cfg Config, name string) (DigestOutput, error) {
	f, errResult := load(cfg.SavedVariables)
	if errResult != nil {
		EnsureDataFiles(cfg)
		return DigestOutput{}, errors.New(errResult.Content[0].(*mcp.TextContent).Text)
	}
	if len(f.Characters) == 0 {
		EnsureDataFiles(cfg)
		return DigestOutput{}, errors.New("The SavedVariables file holds no characters.")
	}
	if name = strings.TrimSpace(name); name != "" && !slices.ContainsFunc(f.Characters, func(c character.Character) bool {
		return strings.EqualFold(c.Name, name) || strings.EqualFold(c.Key, name)
	}) {
		return DigestOutput{}, NotCapturedError(name)
	}
	path := cfg.DigestPath
	if path == "" {
		var err error
		if path, err = digest.DigestPath(cfg.SavedVariables); err != nil {
			return DigestOutput{}, err
		}
		if err := digest.CheckInstalled(path); err != nil {
			return DigestOutput{}, err
		}
	}
	var ds store.DataStore
	var datasetBuild string
	if cfg.Dataset != "" {
		ds = store.NewDir(cfg.Dataset)
		if m, err := store.ReadManifest(cfg.Dataset); err == nil && m != nil {
			if v, ok := m["game_build"]; ok {
				datasetBuild = toString(v)
			}
		}
	}
	now := time.Now()
	qx := questie.ForSavedVariables(cfg.SavedVariables)
	kept, skipped := digest.Select(f.Characters)
	env := digest.Envelope{Generated: now.Unix()}
	out := DigestOutput{Path: path, Lines: map[string]map[string]int{}}
	var parts []string
	plans := route.Envelope{Generated: now.Unix()}
	ropt := route.OptionsFor(unlock.AddonSV(cfg.SavedVariables), ds)
	for _, c := range kept {
		c = withTotals(cfg, profile.Attach(cfg.profilesPath(), c))
		plan := route.BuildWith(c, qx, ropt)
		plans.Plans = append(plans.Plans, plan)
		d := digest.Build(c, ds, digest.Options{Now: now, Client: f.ClientOf(c), DatasetBuild: datasetBuild, Questie: qx, Unlocks: route.UnlockNotes(plan)})
		env.Characters = append(env.Characters, d)
		out.Characters = append(out.Characters, d.Character)
		out.Lines[d.Character] = map[string]int{}
		var counts []string
		for _, s := range d.Sections {
			out.Lines[d.Character][s.ID] = len(s.Lines)
			counts = append(counts, fmt.Sprintf("%s %d", s.ID, len(s.Lines)))
		}
		if len(counts) == 0 {
			counts = []string{"no advice"}
		}
		parts = append(parts, fmt.Sprintf("%s: %s", d.Character, strings.Join(counts, ", ")))
	}
	var newest int64
	for _, c := range kept {
		if c.CapturedAt != nil {
			newest = max(newest, *c.CapturedAt)
		}
	}
	if cfg.SkipUnchanged && digest.SameTextSince(path, env.Lua(), newest) {
		out.Unchanged = true
	} else if err := digest.Write(path, env); err != nil {
		return DigestOutput{}, fmt.Errorf("could not write the digest: %w", err)
	}
	planPath := digest.SiblingPath(path, route.File)
	if cfg.SkipUnchanged && digest.SameTextSince(planPath, plans.Lua(), newest) {
		out.PlanPath = planPath
	} else if err := digest.WriteFile(planPath, plans.Lua()); err != nil {
		return DigestOutput{}, fmt.Errorf("could not write the route plan: %w", err)
	} else {
		out.PlanPath = planPath
	}
	out.Text = fmt.Sprintf("Wrote the digest for %d characters to %s.\n%s\n", len(kept), path, strings.Join(parts, "\n"))
	if len(skipped) > 0 {
		for _, c := range skipped {
			out.Skipped = append(out.Skipped, cmp.Or(c.Name, c.Key))
		}
		out.Text += fmt.Sprintf("Skipped (at most %d, the most recently captured first): %s.\n", digest.MaxCharacters, strings.Join(out.Skipped, ", "))
	}
	out.Text += "Type /reload in game to show it."
	return out, nil
}

// EnsureDataFiles writes a valid empty stub for each of Digest.lua and Plan.lua that is missing from
// the installed slot addon, so the game never lists a file that isn't there. It does nothing when the
// addon folder is absent, and never touches a file that exists. The error is the first write failure.
func EnsureDataFiles(cfg Config) error {
	path := cfg.DigestPath
	if path == "" {
		var err error
		if path, err = digest.DigestPath(cfg.SavedVariables); err != nil {
			return err
		}
	}
	if digest.CheckInstalled(path) != nil {
		return nil
	}
	now := time.Now().Unix()
	var errs []error
	for _, f := range []struct{ path, text string }{
		{path, digest.Envelope{Generated: now}.Lua()},
		{digest.SiblingPath(path, route.File), route.Envelope{Generated: now}.Lua()},
	} {
		if _, err := os.Stat(f.path); errors.Is(err, fs.ErrNotExist) {
			errs = append(errs, digest.WriteFile(f.path, f.text))
		}
	}
	return errors.Join(errs...)
}

// Diagnose checks the whole setup for the configured SavedVariables file.
func Diagnose(cfg Config) doctor.Report {
	env := setup.Env{GOOS: runtime.GOOS, AppData: os.Getenv("APPDATA")}
	env.Home, _ = os.UserHomeDir()
	env.Cwd, _ = os.Getwd()
	return doctor.Run(doctor.Options{SavedVariables: cfg.SavedVariables, SVSource: cfg.SVSource, Cache: cfg.Cache, Env: env})
}

// New builds the server with its tools. The character file is read on every call.
func New(cfg Config) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "wowc-advisor", Version: Version}, nil)

	addProfileTools(s, cfg)

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
		c, err := chosen(cfg, f, in.Character)
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
		c, err := chosen(cfg, f, in.Character)
		if err != nil {
			return toolError(err.Error()), NextOutput{}, nil
		}
		var ds store.DataStore
		if cfg.Dataset != "" {
			ds = store.NewDir(cfg.Dataset)
		}
		all := advice.WhatsNextWith(c, ds, questie.ForSavedVariables(cfg.SavedVariables))
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
		Name: "quest_plan",
		Description: "Efficient ordered quest route for one captured character (by name; omit if only one): pick-ups, objectives and turn-ins " +
			"with map coordinates, on the character's current map, then the next zone to travel to; quests too high a level are listed as later. " +
			"Follow-up quests that turning in the plan's quests unlocks are routed too, marked locked, after the turn-in that unlocks them. " +
			"The order comes from a distance optimiser over the player's own Questie data, not from a guess. Needs Questie installed.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in BriefInput) (*mcp.CallToolResult, RouteOutput, error) {
		f, errResult := load(cfg.SavedVariables)
		if errResult != nil {
			return errResult, RouteOutput{}, nil
		}
		c, err := chosen(cfg, f, in.Character)
		if err != nil {
			return toolError(err.Error()), RouteOutput{}, nil
		}
		var ds store.DataStore
		if cfg.Dataset != "" {
			ds = store.NewDir(cfg.Dataset)
		}
		p := route.BuildWith(c, questie.ForSavedVariables(cfg.SavedVariables), route.OptionsFor(unlock.AddonSV(cfg.SavedVariables), ds))
		return nil, RouteOutput{p, route.Render(p, MaxPlanText)}, nil
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
		c, err := chosen(cfg, f, in.Character)
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
			if in.Brief {
				limit = 1
			}
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
		out := UpgradeOutput{name, res.Spec, res.ItemLevelOnly, res.Slots, gear.Render(c, res)}
		if in.Brief { // the text carries the same data; send it once
			out.Slots = map[string][]gear.Candidate{} // not nil: the output schema wants an object, and null fails it
		}
		return nil, out, nil
	})

	addStatCheck(s, cfg)

	mcp.AddTool(s, &mcp.Tool{
		Name: "upgrade_paths",
		Description: "How the character can dependably get an item (give item) or the best gettable upgrades for a slot (give slot): ranked ways, " +
			"quest in the log, craft with have/need counts from bags and bank, Auction House (common world drops and Bind on Equip gear, assumed cheap; never a price), other quests, dungeon boss, named creature. " +
			"For a slot it can also return a shopping list: the base item, level range and stat suffixes to search for on the Auction House, with the in-game search steps; give those steps in plain words and never quote a price. " +
			"Call it for any 'how do I get ...' or 'which upgrade first' question; never say a source is unknown without calling it. " +
			"An item with no way is a random drop, not worth chasing, and the answer names the best gettable item for its slot instead.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in PathsInput) (*mcp.CallToolResult, PathsOutput, error) {
		f, errResult := load(cfg.SavedVariables)
		if errResult != nil {
			return errResult, noPaths, nil
		}
		c, err := chosen(cfg, f, in.Character)
		if err != nil {
			return toolError(err.Error()), noPaths, nil
		}
		if cfg.Dataset == "" {
			return toolError("No dataset configured (set WOWC_DATASET)."), noPaths, nil
		}
		ds := store.NewDir(cfg.Dataset)
		slot := strings.ToLower(strings.TrimSpace(in.Slot))
		if slot != "" && !slices.Contains(brief.Slots, slot) {
			return toolError("Unknown slot '" + in.Slot + "'; use one of: " + strings.Join(brief.Slots, ", ") + "."), noPaths, nil
		}
		if slot == "" && strings.TrimSpace(in.Item) == "" {
			return toolError("Give an item name or a slot."), noPaths, nil
		}
		name := c.Name
		if name == "" {
			name = c.Key
		}
		group := ""
		if names, _ := brief.TopGroups(c.Talents); len(names) == 1 {
			group = names[0]
		}
		popt := paths.Options{PlanQuests: digest.PlanQuests(advice.WhatsNextWith(c, ds, questie.ForSavedVariables(cfg.SavedVariables)))}
		out := PathsOutput{Character: name, Results: []paths.Result{}}
		var text []string
		if strings.TrimSpace(in.Item) != "" {
			id, iname, cands := names.Resolve(ds, names.Items, in.Item)
			if id == 0 {
				msg := "No item matches '" + in.Item + "'."
				if len(cands) > 0 {
					msg = "Several items match; name one of: " + strings.Join(stripIDs(cands), "; ") + "."
				}
				return toolError(msg), noPaths, nil
			}
			r := paths.For(c, ds, id, popt)
			r.Item = iname
			if r.Slot = paths.SlotOf(c, ds, group, id); !r.Dependable() && r.Slot != "" {
				if alt, err := paths.InSlot(c, ds, group, r.Slot, popt, 1); err == nil && len(alt) > 0 {
					r.Reason += ". The best " + wording.Slot(r.Slot) + " you can get instead is " + alt[0].Result.Item + ": " + alt[0].Result.Paths[0].Line
					out.Results = append(out.Results, r, alt[0].Result)
					text = append(text, r.Text(), alt[0].Result.Text())
					out.Text = strings.Join(text, "\n\n")
					return nil, out, nil
				}
			}
			out.Results = append(out.Results, r)
			text = append(text, r.Text())
		}
		if slot != "" {
			got, err := paths.InSlot(c, ds, group, slot, popt, 3)
			if err != nil {
				return toolError("Could not find upgrades: " + err.Error()), noPaths, nil
			}
			if sh, ok := paths.ShoppingList(c, ds, group, slot, popt); ok {
				r := sh.Result()
				out.Results = append(out.Results, r)
				text = append(text, r.Text())
			}
			if len(got) == 0 && len(out.Results) == 0 {
				text = append(text, "No upgrade for "+wording.Slot(slot)+" has a dependable way to be got right now.")
			}
			for _, g := range got {
				out.Results = append(out.Results, g.Result)
				text = append(text, g.Result.Text())
			}
		}
		out.Text = strings.Join(text, "\n\n")
		return nil, out, nil
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
		c, err := chosen(cfg, f, in.Character)
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
		c, err := chosen(cfg, f, in.Character)
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
		text := talents.Render(plan)
		if c.Profile == nil {
			text = profile.Ask() + "\n\n" + text
		} else {
			text = renderProfile(c) + "\nThe advisor's curated build, for comparison:\n" + text
		}
		return nil, TalentOutput{name, plan, text}, nil
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
		c, err := chosen(cfg, f, in.Character)
		if err != nil {
			return toolError(err.Error()), none, nil
		}
		if cfg.Dataset == "" {
			return toolError("No dataset configured (set WOWC_DATASET)."), none, nil
		}
		limit := in.Limit
		if limit <= 0 {
			limit = DefaultDungeonLimit
			if in.Brief {
				limit = 2
			}
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
		out := DungeonOutput{name, picks, res.Unknown, dungeons.Render(c, res, limit)}
		if in.Brief {
			out.Picks, out.Unknown = nil, nil
		}
		return nil, out, nil
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
		c, err := chosen(cfg, f, in.Character)
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
		if items.Unobtainable(item) {
			return toolError(item + ": " + items.Message + "."), none, nil
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
		Name: "quest_chain",
		Description: "Where a quest sits in its quest line (quest id or name): the line's name, every step in order and the quest's position, from the dataset. " +
			"When the dataset has no line for the quest (WoW Forever's own quests, ids in the 90000s) it falls back to the player's QuestieDB: " +
			"what the quest follows, what it leads to, and its level, giver, turn-in and objective, marked source \"questie\". " +
			"Says so when neither holds a follow-up. Reads no character.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in ChainInput) (*mcp.CallToolResult, ChainOutput, error) {
		none := ChainOutput{Chain: quests.Chain{Steps: []quests.Step{}}}
		if cfg.Dataset == "" {
			return toolError("No dataset configured (set WOWC_DATASET)."), none, nil
		}
		if strings.TrimSpace(in.Quest) == "" {
			return toolError("Give a quest id or name."), none, nil
		}
		ds := store.NewDir(cfg.Dataset)
		qx := questie.ForSavedVariables(cfg.SavedVariables)
		id, name, cands := quests.ResolveWith(ds, qx, in.Quest)
		if len(cands) > 0 {
			return toolError("'" + in.Quest + "' matches several quests: " + strings.Join(cands, ", ") + ". Use the id."), none, nil
		}
		if id == 0 {
			return toolError("No quest matches '" + in.Quest + "'."), none, nil
		}
		c := quests.OfWith(ds, qx, id)
		return nil, ChainOutput{name, c, quests.Render(c, name)}, nil
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
		c, err := chosen(cfg, f, in.Character)
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
		Description: "Write the in-game digest of every captured character into the WoWCompanion_Digest addon, so alts get advice too (most recent 30; `character`, if given, must be one of them): " +
			"next quests, gear upgrades, materials to keep and notes, from whats_next and upgrade_finder. " +
			"Replaces Digest.lua; the player then types /reload in game.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in DigestInput) (*mcp.CallToolResult, DigestOutput, error) {
		out, err := WriteDigest(cfg, in.Character)
		if err != nil {
			return toolError(err.Error()), DigestOutput{Lines: map[string]map[string]int{}}, nil
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

// Package server is the MCP server and its tool registry, shared by every transport.
package server

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/WoW-Companion/advisor/internal/brief"
	"github.com/WoW-Companion/advisor/internal/character"
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
		if v, ok := manifest["version"]; ok {
			info.ManifestVersion = strp(toString(v))
		}
		if v, ok := manifest["game_build"]; ok {
			info.GameBuild = strp(toString(v))
		}
	}
	return info
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

// New builds the server with its three tools. The character file is read on every call.
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

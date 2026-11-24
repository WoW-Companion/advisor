package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/WoW-Companion/advisor/internal/ask"
	"github.com/WoW-Companion/advisor/internal/server"
	"github.com/WoW-Companion/advisor/internal/svfind"
)

// askFlags are the flag values the ask and companion commands share.
type askFlags struct {
	cache, agent, confPath string
	timeout                time.Duration
	max                    int
	opts                   ask.Options
	optional               bool // run without an agent: detect one, or leave Command empty
}

// askSetup resolves the SavedVariables file, the agent and the paths, and writes the agent's MCP
// config. cleanup removes it.
func askSetup(ctx context.Context, cfg *server.Config, f askFlags, stderr io.Writer) (ac ask.Config, cleanup func(), err error) {
	cleanup = func() {}
	*cfg = resolve(*cfg, stderr)
	if cfg.SavedVariables == "" {
		return ac, cleanup, errors.New("no SavedVariables file: pass --savedvariables or set $" + server.SavedVariablesEnv + "; " + svfind.Detect().NotFound(server.SavedVariablesEnv))
	}
	if f.confPath == "" {
		f.confPath = defaultConfigPath()
	}
	var ag ask.Agent
	if f.optional {
		ag, err = ask.ResolveAuto(f.agent, f.confPath)
	} else {
		ag.Command, err = ask.ResolveAgent(f.agent, f.confPath)
	}
	if err != nil {
		return ac, cleanup, err
	}
	ac = ask.Config{Command: ag.Command, Name: ag.Name, Source: ag.Source, Options: f.opts, Timeout: f.timeout, MaxPerRun: f.max}
	ac.Server = func() *mcp.Server { return server.New(*cfg) }
	if cfg.Dataset == "" {
		cfg.Dataset = autoDataset(ctx, f.cache, cfg.SavedVariables, stderr)
	}
	exe, err := os.Executable()
	if err != nil {
		return ac, cleanup, err
	}
	env := map[string]string{server.SavedVariablesEnv: cfg.SavedVariables}
	if cfg.Dataset != "" {
		env[server.DatasetEnv] = cfg.Dataset
	}
	dir, err := os.MkdirTemp("", "wowc-ask-")
	if err != nil {
		return ac, cleanup, err
	}
	cleanup = func() { os.RemoveAll(dir) }
	ac.MCPConfig = filepath.Join(dir, "mcp.json")
	if err := os.WriteFile(ac.MCPConfig, ask.MCPConfig(exe, env), 0o600); err != nil {
		cleanup()
		return ac, func() {}, err
	}
	if err := ask.WriteAgyWorkspace(ac.MCPConfig, ask.MCPConfig(exe, env)); err != nil {
		cleanup()
		return ac, func() {}, err
	}
	if err := os.WriteFile(ask.PolicyPath(ac.MCPConfig), ask.GeminiPolicy(), 0o600); err != nil {
		cleanup()
		return ac, func() {}, err
	}
	return ac, cleanup, nil
}

// parseOptions checks the --length and --model flag values; a --model that is not a tier is a
// custom model name.
func parseOptions(length, model string) (ask.Options, error) {
	o := ask.Options{Length: length, Tier: model}
	if !slices.Contains(ask.Lengths, length) {
		return o, fmt.Errorf("--length %q: use brief, standard or detailed", length)
	}
	if !slices.Contains(ask.Tiers, model) {
		o.Tier, o.Custom = ask.TierCustom, model
	}
	if o.Normalize() != o && o.Tier == ask.TierCustom {
		return o, fmt.Errorf("--model %q: use efficient, balanced, best or a model name", model)
	}
	return o.Normalize(), nil
}

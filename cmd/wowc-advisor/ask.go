package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/WoW-Companion/advisor/internal/ask"
	"github.com/WoW-Companion/advisor/internal/server"
	"github.com/WoW-Companion/advisor/internal/svfind"
)

// askCmd answers the questions queued in game into Answers.lua (deprecated, --answers-file): one
// pass, or --watch until interrupted. The overlay (companion) is the recommended way.
func askCmd(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("ask", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var cfg server.Config
	fs.StringVar(&cfg.Dataset, "dataset", "", "dataset directory (default: $"+server.DatasetEnv+", else the cached dataset)")
	fs.StringVar(&cfg.SavedVariables, "savedvariables", "", "ForeverScribe.lua file (default: $"+server.SavedVariablesEnv+")")
	cache := fs.String("cache", "", "dataset cache directory (default: per-user cache)")
	agent := fs.String("agent", "", "agent command or preset: claude, codex, agy, gemini (default: \"agent\" in the config file)")
	confPath := fs.String("config", "", "config file (default: <user config dir>/wowc-advisor/ask.json)")
	addonSV := fs.String("addon-savedvariables", "", "the addon's WoWCompanion.lua (default: next to ForeverScribe.lua)")
	answersFile := fs.Bool("answers-file", false, "DEPRECATED: write Answers.lua for the old WoWCompanion_Answers addon (the overlay is recommended: it answers in-game questions itself)")
	answers := fs.String("answers-path", "", "Answers.lua to write with --answers-file (default: derived from the SavedVariables path)")
	length := fs.String("length", ask.LengthStandard, "answer length: brief, standard or detailed")
	model := fs.String("model", ask.TierEfficient, "model tier: efficient, balanced, best, or a model name")
	timeout := fs.Duration("timeout", ask.DefaultTimeout, "time limit for each agent run")
	max := fs.Int("max", ask.DefaultMaxPerRun, "most questions answered per pass")
	watch := fs.Bool("watch", false, "keep running and answer whenever the SavedVariables file changes")
	every := fs.Duration("interval", 2*time.Second, "how often --watch checks the file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return errors.New("ask takes no arguments\n" + usage)
	}
	if !*answersFile && *answers == "" {
		return errors.New("ask now only writes Answers.lua for the old WoWCompanion_Answers addon: pass --answers-file to keep doing that (deprecated).\n" +
			"The recommended way is the overlay (wowc-advisor companion): it answers questions queued in game, in the overlay, and writes nothing to the game folder.")
	}
	opts, err := parseOptions(*length, *model)
	if err != nil {
		return err
	}
	ac, cleanup, err := askSetup(ctx, &cfg, askFlags{answersFile: true, opts: opts, cache: *cache, agent: *agent, confPath: *confPath, addonSV: *addonSV, answers: *answers, timeout: *timeout, max: *max}, stderr)
	if err != nil {
		return err
	}
	defer cleanup()
	if *watch {
		fmt.Fprintf(stdout, "watching %s for questions (Ctrl-C to stop)\n", ac.SavedVariables)
		return ask.Watch(ctx, ac, *every, stdout)
	}
	res, err := ask.Run(ctx, ac)
	if err != nil {
		return err
	}
	fmt.Fprintln(stdout, ask.Summary(res, ac.AnswersPath))
	return nil
}

// askFlags are the flag values the ask and companion commands share.
type askFlags struct {
	cache, agent, confPath, addonSV, answers string
	answersFile                              bool // write Answers.lua (deprecated; ask always, the companion with --answers-file)
	timeout                                  time.Duration
	max                                      int
	opts                                     ask.Options
	optional                                 bool // run without an agent: detect one, or leave Command empty
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
	ac = ask.Config{Command: ag.Command, Name: ag.Name, Source: ag.Source, Options: f.opts, Timeout: f.timeout, MaxPerRun: f.max, SavedVariables: f.addonSV, AnswersPath: f.answers}
	derivedSV, derivedAnswers, derr := ask.Paths(cfg.SavedVariables)
	if ac.SavedVariables == "" {
		ac.SavedVariables = derivedSV
	}
	if ac.AnswersPath == "" && f.answersFile {
		if derr != nil {
			return ac, cleanup, derr
		}
		ac.AnswersPath = derivedAnswers
		if err := ask.CheckInstalled(ac.AnswersPath); err != nil {
			return ac, cleanup, err
		}
	}
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

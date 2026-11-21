package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/WoW-Companion/advisor/internal/ask"
	"github.com/WoW-Companion/advisor/internal/server"
)

// askCmd answers the questions queued in game: one pass, or --watch until interrupted.
func askCmd(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("ask", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var cfg server.Config
	fs.StringVar(&cfg.Dataset, "dataset", "", "dataset directory (default: $"+server.DatasetEnv+", else the cached dataset)")
	fs.StringVar(&cfg.SavedVariables, "savedvariables", "", "ForeverScribe.lua file (default: $"+server.SavedVariablesEnv+")")
	cache := fs.String("cache", "", "dataset cache directory (default: per-user cache)")
	agent := fs.String("agent", "", "agent command or preset: claude, codex, gemini (default: \"agent\" in the config file)")
	confPath := fs.String("config", "", "config file (default: <user config dir>/wowc-advisor/ask.json)")
	addonSV := fs.String("addon-savedvariables", "", "the addon's WoWCompanion.lua (default: next to ForeverScribe.lua)")
	answers := fs.String("answers-path", "", "Answers.lua to write (default: derived from the SavedVariables path)")
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
	cfg = cfg.FromEnv()
	if cfg.SavedVariables == "" {
		return errors.New("no SavedVariables file: pass --savedvariables or set $" + server.SavedVariablesEnv)
	}
	if *confPath == "" {
		if dir, err := os.UserConfigDir(); err == nil {
			*confPath = filepath.Join(dir, "wowc-advisor", "ask.json")
		}
	}
	argv, err := ask.ResolveAgent(*agent, *confPath)
	if err != nil {
		return err
	}
	ac := ask.Config{Command: argv, Timeout: *timeout, MaxPerRun: *max, SavedVariables: *addonSV, AnswersPath: *answers}
	derivedSV, derivedAnswers, derr := ask.Paths(cfg.SavedVariables)
	if ac.SavedVariables == "" {
		ac.SavedVariables = derivedSV
	}
	if ac.AnswersPath == "" {
		if derr != nil {
			return derr
		}
		ac.AnswersPath = derivedAnswers
		if err := ask.CheckInstalled(ac.AnswersPath); err != nil {
			return err
		}
	}
	if cfg.Dataset == "" {
		cfg.Dataset = autoDataset(ctx, *cache, cfg.SavedVariables, stderr)
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	env := map[string]string{server.SavedVariablesEnv: cfg.SavedVariables}
	if cfg.Dataset != "" {
		env[server.DatasetEnv] = cfg.Dataset
	}
	dir, err := os.MkdirTemp("", "wowc-ask-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	ac.MCPConfig = filepath.Join(dir, "mcp.json")
	if err := os.WriteFile(ac.MCPConfig, ask.MCPConfig(exe, env), 0o600); err != nil {
		return err
	}
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

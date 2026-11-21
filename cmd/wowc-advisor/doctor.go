package main

import (
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/WoW-Companion/advisor/internal/server"
)

// errChecksFailed is returned after the report is printed, so the exit status says it failed.
var errChecksFailed = errors.New("some doctor checks failed")

func doctorCmd(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var cfg server.Config
	fs.StringVar(&cfg.SavedVariables, "savedvariables", "", "ForeverScribe.lua file (default: $"+server.SavedVariablesEnv+")")
	fs.StringVar(&cfg.Cache, "cache", "", "dataset cache directory (default: per-user cache)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("doctor takes no arguments\n" + usage)
	}
	rep := server.Diagnose(cfg.FromEnv())
	fmt.Fprint(stdout, rep.Text)
	if !rep.OK {
		return errChecksFailed
	}
	return nil
}

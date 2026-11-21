package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"

	"github.com/WoW-Companion/advisor/internal/release"
	"github.com/WoW-Companion/advisor/internal/server"
)

// errChecksFailed is returned after the report is printed, so the exit status says it failed.
var errChecksFailed = errors.New("some doctor checks failed")

func doctorCmd(args []string, stdout, stderr io.Writer) error {
	return doctor(context.Background(), args, stdout, stderr, http.DefaultClient, release.LatestURL)
}

func doctor(ctx context.Context, args []string, stdout, stderr io.Writer, client *http.Client, latestURL string) error {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var cfg server.Config
	fs.StringVar(&cfg.SavedVariables, "savedvariables", "", "ForeverScribe.lua file (default: $"+server.SavedVariablesEnv+")")
	fs.StringVar(&cfg.Cache, "cache", "", "dataset cache directory (default: per-user cache)")
	check := fs.Bool("check", false, "also ask GitHub whether a newer release exists (one unauthenticated request; nothing is downloaded)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("doctor takes no arguments\n" + usage)
	}
	fmt.Fprintln(stdout, versionLine())
	if *check {
		fmt.Fprintln(stdout, updateLine(ctx, client, latestURL))
	}
	rep := server.Diagnose(cfg.FromEnv())
	fmt.Fprint(stdout, rep.Text)
	if !rep.OK {
		return errChecksFailed
	}
	return nil
}

// updateLine says whether a newer release exists. A failed lookup is a note, never a failed check.
func updateLine(ctx context.Context, client *http.Client, url string) string {
	res, err := release.Check(ctx, client, url, server.Version)
	switch {
	case err != nil:
		return "update check: could not look up the latest release: " + err.Error()
	case !res.Known:
		return "update check: latest release is " + res.Latest + "; this is a development build"
	case res.Newer:
		return "update check: " + res.Latest + " is available, see https://github.com/WoW-Companion/advisor/releases/latest"
	}
	return "update check: up to date (" + res.Latest + ")"
}

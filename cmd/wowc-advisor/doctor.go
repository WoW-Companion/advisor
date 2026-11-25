package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"path/filepath"

	"github.com/WoW-Companion/advisor/internal/gamesetup"

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
	fs.StringVar(&cfg.SavedVariables, "savedvariables", "", "WoWCompanion.lua (or ForeverScribe.lua) file (default: $"+server.SavedVariablesEnv+")")
	fs.StringVar(&cfg.Cache, "cache", "", "dataset cache directory (default: per-user cache)")
	check := fs.Bool("check", false, "also ask GitHub whether a newer release exists (one unauthenticated request; nothing is downloaded)")
	fixAddon := fs.Bool("fix-addon", false, "install or update the WoW Companion addon from its latest GitHub release (checksum verified; only writes under Interface/AddOns)")
	gameDir := fs.String("game-dir", "", "WoW folder for --fix-addon (default: found from the SavedVariables path or the standard install folders)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *fixAddon {
		return fixAddonCmd(ctx, stdout, client, *gameDir, cfg.FromEnv().SavedVariables)
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

// fixAddonCmd installs or updates the addon; it says what it did and never touches Digest.lua.
func fixAddonCmd(ctx context.Context, stdout io.Writer, client *http.Client, gameDir, sv string) error {
	game, ok := gamesetup.Resolve(gamesetup.Options{GameDir: gameDir, SavedVariables: sv})
	if !ok {
		return errors.New("WoW folder not found: pass --game-dir <your World of Warcraft folder>")
	}
	rel, err := gamesetup.LatestAddon(ctx, client, gamesetup.AddonLatestURL)
	if err != nil {
		return fmt.Errorf("could not look up the latest addon release: %w", err)
	}
	have, installed := gamesetup.InstalledAddonVersion(game)
	if !installed {
		have = ""
	}
	pulls := release.FetchPulls(ctx, client, release.PullsURL).Addon
	plan := gamesetup.PlanAddon(have, rel.Version, pulls)
	if plan.Version == "" {
		if plan.Pulled {
			fmt.Fprintf(stdout, "WoW Companion addon %s has a known problem%s and no safe version is available yet; nothing was changed.\n", have, reasonText(plan.Reason))
			return nil
		}
		fmt.Fprintf(stdout, "WoW Companion addon %s is up to date in %s\n", have, game)
		return nil
	}
	if plan.Version != rel.Version {
		if rel, err = gamesetup.LatestAddon(ctx, client, gamesetup.AddonTagURL(plan.Version)); err != nil {
			return fmt.Errorf("could not look up addon version %s: %w", plan.Version, err)
		}
	}
	note, err := gamesetup.InstallAddonNote(ctx, client, game, rel)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Installed WoW Companion addon %s into %s\n%s\n", rel.Version, filepath.Join(game, "Interface", "AddOns"), note)
	return nil
}

func reasonText(r string) string {
	if r == "" {
		return ""
	}
	return " (" + r + ")"
}

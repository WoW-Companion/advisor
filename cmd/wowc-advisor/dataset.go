package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/WoW-Companion/advisor/internal/dataset"
	"github.com/WoW-Companion/advisor/internal/server"
)

func openStore(cache string) (dataset.Store, error) {
	if cache == "" {
		var err error
		if cache, err = dataset.DefaultRoot(); err != nil {
			return dataset.Store{}, err
		}
	}
	return dataset.Store{Root: cache}, nil
}

// autoDataset is the dataset directory for serve when none was given: the cached dataset for the
// capture's build, downloaded on first run. It returns "" (and says why) when there is none.
func autoDataset(ctx context.Context, cache, savedVariables string, stderr io.Writer) string {
	st, err := openStore(cache)
	if err != nil {
		fmt.Fprintln(stderr, "wowc-advisor: no dataset cache:", err)
		return ""
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	res, err := st.Ensure(ctx, server.CaptureClient(savedVariables).Build, false)
	if err != nil {
		fmt.Fprintln(stderr, "wowc-advisor: no dataset:", err)
		return ""
	}
	reportEnsure(stderr, res)
	if !res.Downloaded && res.Offline == nil {
		// A cached dataset is used as it is by Ensure, so a republish (new levels, entrances) would
		// never reach a player whose companion is not running. Check once, briefly, and keep the
		// cache on any error.
		rctx, rcancel := context.WithTimeout(ctx, 15*time.Second)
		defer rcancel()
		if build, updated, err := st.Refresh(rctx, server.CaptureClient(savedVariables).Build); err != nil {
			fmt.Fprintln(stderr, "wowc-advisor: dataset update check failed (keeping the cached dataset):", err)
		} else if updated {
			fmt.Fprintf(stderr, "wowc-advisor: dataset %s was republished; using the new copy\n", build)
			return st.Dir(build)
		}
	}
	return res.Dir
}

func reportEnsure(w io.Writer, res dataset.Result) {
	switch {
	case res.Downloaded:
		fmt.Fprintf(w, "wowc-advisor: downloaded dataset %s to %s\n", res.Build, res.Dir)
	case res.Offline != nil:
		fmt.Fprintf(w, "wowc-advisor: offline (%v); using cached dataset %s\n", res.Offline, res.Build)
	}
}

func datasetCmd(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || (args[0] != "update" && args[0] != "status") {
		return errors.New(usage)
	}
	fs := flag.NewFlagSet("dataset "+args[0], flag.ContinueOnError)
	fs.SetOutput(stderr)
	cache := fs.String("cache", "", "dataset cache directory (default: per-user cache)")
	sv := fs.String("savedvariables", "", "ForeverScribe.lua file (default: $"+server.SavedVariablesEnv+")")
	build := fs.String("build", "", "game build to fetch (default: the capture's build, else the newest release)")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	st, err := openStore(*cache)
	if err != nil {
		return err
	}
	client := server.CaptureClient(resolve(server.Config{SavedVariables: *sv}, stderr).SavedVariables)
	if args[0] == "update" {
		want := *build
		if want == "" {
			want = client.Build
		}
		res, err := st.Ensure(ctx, want, true)
		if err != nil {
			return err
		}
		reportEnsure(stderr, res)
		fmt.Fprintf(stdout, "dataset %s ready at %s\n", res.Build, res.Dir)
		return nil
	}
	good, bad := st.Cached()
	fmt.Fprintf(stdout, "cache: %s\n", st.Root)
	if client.Build != "" {
		fmt.Fprintf(stdout, "capture client: %s build %s\n", client.Version, client.Build)
	}
	for _, b := range good {
		note := ""
		if client.Build != "" && dataset.Matches(b, client.Version, client.Build) {
			note = " (matches the capture)"
		}
		fmt.Fprintf(stdout, "%s: verified%s\n", b, note)
	}
	for b, err := range bad {
		fmt.Fprintf(stdout, "%s: FAILED verification: %v\n", b, err)
	}
	if len(good) == 0 && len(bad) == 0 {
		fmt.Fprintln(stdout, "no dataset cached; run `wowc-advisor dataset update`")
	}
	return nil
}

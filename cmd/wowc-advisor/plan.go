package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/WoW-Companion/advisor/internal/professions"
	"github.com/WoW-Companion/advisor/internal/server"
	"github.com/WoW-Companion/advisor/internal/store"
)

// openPlanDataset opens a dataset directory or the base URL of a published dataset.
func openPlanDataset(where string) (store.DataStore, error) {
	if strings.HasPrefix(where, "http://") || strings.HasPrefix(where, "https://") {
		return store.New(store.HTTPSource{Base: where, Client: &http.Client{Timeout: time.Minute}}), nil
	}
	if st, err := os.Stat(where); err != nil || !st.IsDir() {
		return nil, fmt.Errorf("dataset %q is not a directory or an http(s) URL", where)
	}
	return store.NewDir(where), nil
}

// professionPlanCmd prints a whole skill-up path for a synthetic fresh character, with material totals and cost.
func professionPlanCmd(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("profession-plan", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var o professions.BenchOptions
	fs.StringVar(&o.Profession, "profession", "", "profession to plan (required)")
	fs.IntVar(&o.From, "from", 1, "skill to start at")
	fs.IntVar(&o.To, "to", 0, "skill to reach (default: the most the level allows)")
	fs.IntVar(&o.Level, "level", 60, "character level")
	fs.StringVar(&o.Faction, "faction", "", "horde or alliance")
	fs.StringVar(&o.Gather, "gather", "", "a gathering profession the character has (default: none)")
	where := fs.String("dataset", "", "dataset directory or base URL (default: $"+server.DatasetEnv+")")
	asJSON := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if o.Profession == "" {
		return errors.New("profession-plan needs --profession")
	}
	switch strings.ToLower(o.Faction) {
	case "", "horde", "alliance":
	default:
		return fmt.Errorf("--faction must be horde or alliance, not %q", o.Faction)
	}
	if o.From < 1 || o.To < 0 || o.Level < 1 {
		return errors.New("--from and --level must be at least 1, --to at least 0")
	}
	if *where == "" {
		*where = os.Getenv(server.DatasetEnv)
	}
	if *where == "" {
		return errors.New("profession-plan needs --dataset (a directory or URL)")
	}
	ds, err := openPlanDataset(*where)
	if err != nil {
		return err
	}
	b := professions.NewBenchmark(ds, o)
	if *asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(b)
	}
	_, err = io.WriteString(stdout, professions.RenderBenchmark(b))
	return err
}

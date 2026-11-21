// Command wowc-advisor serves the WoW Companion advisor over MCP.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"

	"github.com/WoW-Companion/advisor/internal/server"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stderr); err != nil {
		if !errors.Is(err, flag.ErrHelp) {
			fmt.Fprintln(os.Stderr, "wowc-advisor:", err)
		}
		os.Exit(2)
	}
}

const usage = `usage: wowc-advisor serve [--stdio | --http] [--host ADDR] [--port N] [--dataset DIR] [--cache DIR] [--savedvariables FILE]
       wowc-advisor dataset update [--build BUILD] [--cache DIR] [--savedvariables FILE]
       wowc-advisor dataset status [--cache DIR] [--savedvariables FILE]
       wowc-advisor digest [NAME] [--dataset DIR] [--cache DIR] [--savedvariables FILE] [--digest-path FILE]
       wowc-advisor version`

func run(ctx context.Context, args []string, stderr io.Writer) error {
	if len(args) == 0 {
		return errors.New(usage)
	}
	switch args[0] {
	case "version":
		fmt.Fprintln(os.Stdout, "wowc-advisor", server.Version)
		return nil
	case "serve":
		return serve(ctx, args[1:], stderr)
	case "dataset":
		return datasetCmd(ctx, args[1:], os.Stdout, stderr)
	case "digest":
		return digestCmd(ctx, args[1:], os.Stdout, stderr)
	}
	return fmt.Errorf("unknown command %q\n%s", args[0], usage)
}

func digestCmd(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("digest", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var cfg server.Config
	fs.StringVar(&cfg.Dataset, "dataset", "", "dataset directory (default: $"+server.DatasetEnv+", else the cached dataset)")
	fs.StringVar(&cfg.SavedVariables, "savedvariables", "", "ForeverScribe.lua file (default: $"+server.SavedVariablesEnv+")")
	fs.StringVar(&cfg.DigestPath, "digest-path", "", "Digest.lua to write (default: derived from the SavedVariables path)")
	cache := fs.String("cache", "", "dataset cache directory (default: per-user cache)")
	// The name may come before or after the flags.
	name := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		name, args = args[0], args[1:]
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 1 || fs.NArg() == 1 && name != "" {
		return errors.New("digest takes at most one character name\n" + usage)
	}
	if fs.NArg() == 1 {
		name = fs.Arg(0)
	}
	cfg = cfg.FromEnv()
	if cfg.Dataset == "" {
		cfg.Dataset = autoDataset(ctx, *cache, cfg.SavedVariables, stderr)
	}
	out, err := server.WriteDigest(cfg, name)
	if err != nil {
		return err
	}
	fmt.Fprintln(stdout, out.Text)
	return nil
}

func serve(ctx context.Context, args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(stderr)
	stdio := fs.Bool("stdio", false, "serve over stdio (default)")
	httpMode := fs.Bool("http", false, "serve streamable HTTP at /mcp")
	host := fs.String("host", "127.0.0.1", "HTTP bind address")
	port := fs.Int("port", 8000, "HTTP port")
	var cfg server.Config
	fs.StringVar(&cfg.Dataset, "dataset", "", "dataset directory (default: $"+server.DatasetEnv+")")
	fs.StringVar(&cfg.SavedVariables, "savedvariables", "", "ForeverScribe.lua file (default: $"+server.SavedVariablesEnv+")")
	cache := fs.String("cache", "", "dataset cache directory (default: per-user cache)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *stdio && *httpMode {
		return errors.New("--stdio and --http are mutually exclusive")
	}
	cfg = cfg.FromEnv()
	if cfg.Dataset == "" {
		cfg.Dataset = autoDataset(ctx, *cache, cfg.SavedVariables, stderr)
	}
	s := server.New(cfg)
	if !*httpMode {
		return server.ServeStdio(ctx, s)
	}
	mux := http.NewServeMux()
	mux.Handle("/mcp", server.HTTPHandler(s))
	addr := net.JoinHostPort(*host, strconv.Itoa(*port))
	srv := &http.Server{Addr: addr, Handler: mux}
	go func() {
		<-ctx.Done()
		srv.Shutdown(context.Background())
	}()
	fmt.Fprintf(stderr, "wowc-advisor %s: streamable HTTP on http://%s/mcp\n", server.Version, addr)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

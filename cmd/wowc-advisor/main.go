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

const usage = `usage: wowc-advisor serve [--stdio | --http] [--host ADDR] [--port N] [--dataset DIR] [--savedvariables FILE]
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
	}
	return fmt.Errorf("unknown command %q\n%s", args[0], usage)
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
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *stdio && *httpMode {
		return errors.New("--stdio and --http are mutually exclusive")
	}
	s := server.New(cfg.FromEnv())
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

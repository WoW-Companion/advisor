package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/WoW-Companion/advisor/internal/ask"
	"github.com/WoW-Companion/advisor/internal/companion"
	"github.com/WoW-Companion/advisor/internal/dataset"
	"github.com/WoW-Companion/advisor/internal/release"
	"github.com/WoW-Companion/advisor/internal/server"
)

// companionCmd serves the live chat window until interrupted.
func companionCmd(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("companion", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var cfg server.Config
	fs.StringVar(&cfg.Dataset, "dataset", "", "dataset directory (default: $"+server.DatasetEnv+", else the cached dataset)")
	fs.StringVar(&cfg.SavedVariables, "savedvariables", "", "WoWCompanion.lua (or ForeverScribe.lua) file (default: $"+server.SavedVariablesEnv+")")
	f := askFlags{optional: true}
	fs.StringVar(&f.cache, "cache", "", "dataset cache directory (default: per-user cache)")
	fs.StringVar(&f.agent, "agent", "", "agent command or preset: claude, codex, agy, gemini, or auto (default: \"agent\" in the config file, else the first of claude, codex, agy, gemini found)")
	fs.StringVar(&f.confPath, "config", "", "config file (default: <user config dir>/wowc-advisor/ask.json)")
	fs.DurationVar(&f.timeout, "timeout", ask.DefaultTimeout, "time limit for each agent run")
	length := fs.String("length", ask.LengthStandard, "answer length: brief, standard or detailed (the overlay setting wins once chosen)")
	model := fs.String("model", ask.TierEfficient, "model tier: efficient, balanced, best, or a model name")
	noUsage := fs.Bool("no-usage", false, "do not show the token usage line under answers")
	gameDir := fs.String("game-dir", "", "WoW folder for the setup checklist (default: found from the SavedVariables path or the standard install folders)")
	port := fs.Int("port", 4317, "first port to try; the next free one is used when it is taken")
	noOpen := fs.Bool("no-open", false, "do not open the browser")
	handshake := fs.Bool("handshake", false, "for the desktop app: pick a free port, print one JSON line {\"url\",\"token\",\"agent\",\"agent_source\",\"version\"} and never open a browser")
	autoDigest := fs.Bool("auto-digest", false, "rewrite the in-game digest whenever ForeverScribe saves (default: on with --handshake, off otherwise)")
	lan := fs.Bool("lan", false, "listen on all interfaces so a phone or another PC can open it (anyone with the URL can ask your agent)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return errors.New("companion takes no arguments\n" + usage)
	}
	var err error
	if f.opts, err = parseOptions(*length, *model); err != nil {
		return err
	}
	autoSet := false
	fs.Visit(func(f *flag.Flag) { autoSet = autoSet || f.Name == "auto-digest" })
	if !autoSet {
		*autoDigest = *handshake
	}
	if *handshake && *lan {
		return errors.New("--handshake serves the desktop app on this machine only; it cannot be combined with --lan")
	}
	ac, cleanup, err := askSetup(ctx, &cfg, f, stderr)
	if err != nil {
		return err
	}
	defer cleanup()
	bind := "127.0.0.1"
	if *lan {
		bind = "0.0.0.0"
	}
	start := *port
	if *handshake {
		start = 0 // any free port
	}
	ln, err := listenFrom(bind, start)
	if err != nil {
		return err
	}
	s := companion.New(companion.Config{
		ForeverScribe: cfg.SavedVariables, Ask: ac, LAN: *lan, App: *handshake, Dataset: cfg.Dataset, Cache: dataset.Store{Root: f.cache}, AutoDigest: *autoDigest, NoUsage: *noUsage, GameDir: *gameDir,
		Update: func(ctx context.Context) (release.Result, error) {
			return release.Check(ctx, http.DefaultClient, release.LatestURL, server.Version)
		},
		Stream: len(ac.Command) > 0 && strings.TrimSuffix(strings.ToLower(filepath.Base(ac.Command[0])), strings.ToLower(filepath.Ext(ac.Command[0]))) == "claude",
	})
	p := ln.Addr().(*net.TCPAddr).Port
	if *handshake {
		fmt.Fprintln(stdout, handshakeLine(p, s.Token, ac.Name, ac.Source, ask.DetectedPath(ac.Source, ac.Command), server.Version))
		return s.Serve(ctx, ln)
	}
	local := fmt.Sprintf("http://127.0.0.1:%d/?token=%s", p, s.Token)
	fmt.Fprintf(stdout, "companion: %s\n(Ctrl-C to stop; ask questions in the overlay)\n", local)
	if *lan {
		fmt.Fprintf(stdout, "WARNING: --lan lets anyone on your network who has this URL ask your AI agent questions.\nLAN URL: http://%s:%d/?token=%s\n", lanIP(), p, s.Token)
	}
	if !*noOpen {
		openBrowser(local)
	}
	return s.Serve(ctx, ln)
}

// handshakeLine is the one line the desktop app reads from the sidecar's stdout.
// agent is the AI client's short name ("" when none), source where it came from: flag, config, auto or none,
// path the detected executable ("" otherwise), which the settings tooltip names, version the advisor's own.
func handshakeLine(port int, token, agent, source, path, version string) string {
	if source == "" {
		source = ask.SourceNone
	}
	b, _ := json.Marshal(struct {
		URL         string `json:"url"`
		Token       string `json:"token"`
		Agent       string `json:"agent"`
		AgentSource string `json:"agent_source"`
		AgentPath   string `json:"agent_path"`
		Version     string `json:"version"`
	}{fmt.Sprintf("http://127.0.0.1:%d", port), token, agent, source, path, version})
	return string(b)
}

// listenFrom listens on the first free port from start on; start 0 asks the system for one.
func listenFrom(host string, start int) (net.Listener, error) {
	if start == 0 {
		return net.Listen("tcp", net.JoinHostPort(host, "0"))
	}
	var err error
	for p := start; p < start+50 && p <= 65535; p++ {
		var ln net.Listener
		if ln, err = net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(p))); err == nil {
			return ln, nil
		}
	}
	return nil, fmt.Errorf("no free port from %d: %w", start, err)
}

// lanIP is this machine's first non-loopback IPv4 address.
func lanIP() string {
	addrs, _ := net.InterfaceAddrs()
	for _, a := range addrs {
		if n, ok := a.(*net.IPNet); ok && !n.IP.IsLoopback() && n.IP.To4() != nil {
			return n.IP.String()
		}
	}
	return "<this-pc-address>"
}

func openBrowser(url string) {
	var c *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		c = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		c = exec.Command("open", url)
	default:
		c = exec.Command("xdg-open", url)
	}
	if c.Start() == nil {
		go c.Wait()
	}
}

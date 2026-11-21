package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/WoW-Companion/advisor/internal/ask"
	"github.com/WoW-Companion/advisor/internal/companion"
	"github.com/WoW-Companion/advisor/internal/server"
)

// companionCmd serves the live chat window until interrupted.
func companionCmd(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("companion", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var cfg server.Config
	fs.StringVar(&cfg.Dataset, "dataset", "", "dataset directory (default: $"+server.DatasetEnv+", else the cached dataset)")
	fs.StringVar(&cfg.SavedVariables, "savedvariables", "", "ForeverScribe.lua file (default: $"+server.SavedVariablesEnv+")")
	f := askFlags{}
	fs.StringVar(&f.cache, "cache", "", "dataset cache directory (default: per-user cache)")
	fs.StringVar(&f.agent, "agent", "", "agent command or preset: claude, codex, gemini (default: \"agent\" in the config file)")
	fs.StringVar(&f.confPath, "config", "", "config file (default: <user config dir>/wowc-advisor/ask.json)")
	fs.StringVar(&f.addonSV, "addon-savedvariables", "", "the addon's WoWCompanion.lua (default: next to ForeverScribe.lua)")
	fs.StringVar(&f.answers, "answers-path", "", "Answers.lua to write (default: derived from the SavedVariables path)")
	fs.DurationVar(&f.timeout, "timeout", ask.DefaultTimeout, "time limit for each agent run")
	fs.IntVar(&f.max, "max", ask.DefaultMaxPerRun, "most in-game questions answered per pass")
	port := fs.Int("port", 4317, "first port to try; the next free one is used when it is taken")
	noOpen := fs.Bool("no-open", false, "do not open the browser")
	handshake := fs.Bool("handshake", false, "for the desktop app: pick a free port, print one JSON line {\"url\",\"token\"} and never open a browser")
	lan := fs.Bool("lan", false, "listen on all interfaces so a phone or another PC can open it (anyone with the URL can ask your agent)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return errors.New("companion takes no arguments\n" + usage)
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
		ForeverScribe: cfg.SavedVariables, Ask: ac, LAN: *lan, App: *handshake, Dataset: cfg.Dataset,
		Stream: filepath.Base(strings.TrimSuffix(strings.ToLower(ac.Command[0]), ".exe")) == "claude",
	})
	p := ln.Addr().(*net.TCPAddr).Port
	if *handshake {
		fmt.Fprintln(stdout, handshakeLine(p, s.Token))
		return s.Serve(ctx, ln)
	}
	local := fmt.Sprintf("http://127.0.0.1:%d/?token=%s", p, s.Token)
	fmt.Fprintf(stdout, "companion: %s\n(Ctrl-C to stop; it also answers questions asked in game, so do not run ask --watch alongside)\n", local)
	if *lan {
		fmt.Fprintf(stdout, "WARNING: --lan lets anyone on your network who has this URL ask your AI agent questions.\nLAN URL: http://%s:%d/?token=%s\n", lanIP(), p, s.Token)
	}
	if !*noOpen {
		openBrowser(local)
	}
	return s.Serve(ctx, ln)
}

// handshakeLine is the one line the desktop app reads from the sidecar's stdout.
func handshakeLine(port int, token string) string {
	b, _ := json.Marshal(struct {
		URL   string `json:"url"`
		Token string `json:"token"`
	}{fmt.Sprintf("http://127.0.0.1:%d", port), token})
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

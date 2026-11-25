package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"

	"github.com/WoW-Companion/advisor/internal/setup"
)

const savedVariablesPlaceholder = "/path/to/WoWCompanion.lua"

func setupCmd(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("setup", flag.ContinueOnError)
	fs.SetOutput(stderr)
	write := fs.Bool("write", false, "add the entry to the client's config file (a backup is kept)")
	goos := fs.String("os", runtime.GOOS, "operating system to render for (windows, darwin, linux)")
	binary := fs.String("binary", "", "path of the wowc-advisor binary (default: this one)")
	var sv string
	fs.StringVar(&sv, "savedvariables", "", "WoWCompanion.lua (or ForeverScribe.lua) file (required with --write)")
	// The client may come before or after the flags.
	client := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		client, args = args[0], args[1:]
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if client == "" && fs.NArg() == 1 {
		client = fs.Arg(0)
	} else if fs.NArg() != 0 {
		return errors.New("setup takes one client\n" + usage)
	}
	if client == "" {
		return errors.New("setup needs a client: " + strings.Join(setup.Clients, ", "))
	}
	if *binary == "" {
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		*binary = exe
	}
	if sv == "" {
		if *write {
			return errors.New("--write needs --savedvariables FILE")
		}
		sv = savedVariablesPlaceholder
	}
	e := setup.Entry{Binary: *binary, SavedVariables: sv}
	env := setup.Env{GOOS: *goos, AppData: os.Getenv("APPDATA")}
	env.Home, _ = os.UserHomeDir()
	env.Cwd, _ = os.Getwd()
	path, err := setup.Path(client, env)
	if err != nil {
		return err
	}
	if *write {
		path, backup, err := setup.Write(client, e, env)
		if err != nil {
			return err
		}
		fmt.Fprintln(stdout, "wrote", path)
		if backup != "" {
			fmt.Fprintln(stdout, "backup", backup)
		}
		return nil
	}
	snippet, err := setup.Snippet(client, e)
	if err != nil {
		return err
	}
	fmt.Fprintf(stderr, "add this to %s (or run again with --write)\n", path)
	fmt.Fprint(stdout, snippet)
	return nil
}

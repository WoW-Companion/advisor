package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// defaultConfigPath is the config file: <user config dir>/wowc-advisor/ask.json, that is
// %AppData%\wowc-advisor\ask.json on Windows, ~/Library/Application Support/wowc-advisor/ask.json
// on macOS and $XDG_CONFIG_HOME (else ~/.config)/wowc-advisor/ask.json on Linux.
func defaultConfigPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "wowc-advisor", "ask.json")
}

// configCmd is `config path` and `config set agent VALUE`.
func configCmd(args []string, stdout io.Writer) error {
	switch {
	case len(args) == 1 && args[0] == "path":
		p := defaultConfigPath()
		if p == "" {
			return errors.New("this system has no user config directory")
		}
		fmt.Fprintln(stdout, p)
		return nil
	case len(args) >= 3 && args[0] == "set" && args[1] == "agent":
		p := defaultConfigPath()
		if p == "" {
			return errors.New("this system has no user config directory")
		}
		if err := setConfigAgent(p, strings.Join(args[2:], " ")); err != nil {
			return err
		}
		fmt.Fprintln(stdout, "agent set in", p)
		return nil
	}
	return errors.New("usage: wowc-advisor config path | config set agent PRESET|COMMAND|auto\n" + usage)
}

// setConfigAgent writes "agent" into the config file, keeping its other keys. An empty value removes it.
func setConfigAgent(path, value string) error {
	conf := map[string]json.RawMessage{}
	switch raw, err := os.ReadFile(path); {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return err
	default:
		if err := json.Unmarshal(raw, &conf); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
	}
	if value = strings.TrimSpace(value); value == "" {
		delete(conf, "agent")
	} else {
		conf["agent"], _ = json.Marshal(value)
	}
	out, err := json.MarshalIndent(conf, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(out, '\n'), 0o600)
}

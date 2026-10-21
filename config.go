package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type Config struct {
	Sources          []InventorySource                        `json:"sources,omitempty"`
	Variables        map[string]string                        `json:"variables,omitempty"`
	Overrides        map[string]map[string]ConnectionOverride `json:"overrides,omitempty"`
	TunnelsConfig    string                                   `json:"tunnels_config,omitempty"`
	Destination      string                                   `json:"destination"`
	Fallback         string                                   `json:"fallback"`
	Editor           []string                                 `json:"editor"`
	EditDestination  string                                   `json:"edit_destination"`
	Terminal         []string                                 `json:"terminal"`
	FallbackTerminal []string                                 `json:"fallback_terminal"`
	FloatingTerminal []string                                 `json:"floating_terminal"`
	RDP              RDPConfig                                `json:"rdp"`
}

func defaultConfig() Config {
	return Config{
		Destination: "tmux-pane", Fallback: "terminal", EditDestination: "tmux-pane",
		Editor: []string{"nvim"}, Terminal: []string{"auto"},
		FallbackTerminal: []string{"kitty"}, FloatingTerminal: []string{"auto"},
		RDP: defaultRDPConfig(),
	}
}

func configPath() (string, error) {
	dir, err := os.UserConfigDir()
	return filepath.Join(dir, "go-where", "config.json"), err
}

func loadConfig(path string) (Config, error) {
	c := defaultConfig()
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	defer f.Close()
	d := json.NewDecoder(f)
	d.DisallowUnknownFields()
	if err := d.Decode(&c); err != nil {
		return c, fmt.Errorf("%s: %w", path, err)
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return c, fmt.Errorf("%s: expected one JSON object", path)
	}
	return c, c.validate()
}

func (c Config) validate() error {
	ids, names, paths := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, s := range c.Sources {
		if !inventoryName(s.ID) || s.ID == "ssh-config" || s.Name == "" || s.Path == "" || s.Type != "sqlite" {
			return fmt.Errorf("invalid SQLite source %q", s.Name)
		}
		p, err := sourcePath(s)
		if err != nil {
			return err
		}
		if ids[s.ID] || names[s.Name] || paths[p] {
			return fmt.Errorf("duplicate source ID, name or path: %s", s.Name)
		}
		ids[s.ID], names[s.Name], paths[p] = true, true, true
	}
	for _, dest := range []string{c.Destination, c.EditDestination} {
		switch dest {
		case "tmux-pane", "tmux-window", "tmux-session", "terminal", "current":
		default:
			return fmt.Errorf("unknown destination %q", dest)
		}
	}
	if c.EditDestination == "tmux-session" {
		return fmt.Errorf("edit_destination cannot be tmux-session")
	}
	if c.Fallback != "terminal" && c.Fallback != "current" {
		return fmt.Errorf("fallback must be terminal or current")
	}
	for _, argv := range [][]string{c.Editor, c.Terminal, c.FloatingTerminal} {
		if len(argv) == 0 || argv[0] == "" {
			return fmt.Errorf("editor and terminal commands must be nonempty argument arrays")
		}
	}
	for _, argv := range [][]string{c.Terminal, c.FloatingTerminal} {
		if argv[0] == "auto" && len(argv) != 1 {
			return fmt.Errorf("auto must be the sole terminal argument")
		}
	}
	if len(c.FallbackTerminal) > 0 && (c.FallbackTerminal[0] == "" || c.FallbackTerminal[0] == "auto") {
		return fmt.Errorf("fallback_terminal must be an explicit command or an empty array to disable it")
	}
	return c.RDP.validate()
}

func ensureConfig(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if os.IsExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	e := json.NewEncoder(f)
	e.SetIndent("", "  ")
	err = e.Encode(defaultConfig())
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}

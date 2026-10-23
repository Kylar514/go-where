package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type RDPFolder struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Enabled bool   `json:"enabled,omitempty"`
}

type RDPSessionSettings struct {
	Clipboard  *bool       `json:"clipboard,omitempty"`
	Fullscreen *bool       `json:"fullscreen,omitempty"`
	Folders    []RDPFolder `json:"folders,omitempty"`
}

func (s RDPSessionSettings) validate() error {
	seen := map[string]bool{}
	for _, f := range s.Folders {
		if !inventoryName(f.Name) || seen[f.Name] || f.Path == "" || clean(f.Path) != f.Path || strings.ContainsAny(f.Path, ",\r\n") {
			return fmt.Errorf("shared folders require unique names and literal paths without commas")
		}
		seen[f.Name] = true
	}
	return nil
}

func (s RDPSessionSettings) clientArgs() ([]string, error) {
	var args []string
	if s.Clipboard != nil {
		flag := "-clipboard"
		if *s.Clipboard {
			flag = "+clipboard"
		}
		args = append(args, flag)
	}
	if s.Fullscreen != nil {
		flag := "-f"
		if *s.Fullscreen {
			flag = "/f"
		}
		args = append(args, flag)
	}
	for _, f := range s.Folders {
		if !f.Enabled {
			continue
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		path := expandHome(f.Path, home)
		if !filepath.IsAbs(path) {
			return nil, fmt.Errorf("shared folder %s needs an absolute local path", f.Name)
		}
		info, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("shared folder %s is unavailable: %w; disable or edit it in RDP Session Settings", f.Name, err)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("shared folder %s is not a directory", f.Name)
		}
		args = append(args, "/drive:"+f.Name+","+path)
	}
	return args, nil
}

func boolSettingLabel(value *bool) string {
	if value == nil {
		return "Existing client settings"
	}
	if *value {
		return "On"
	}
	return "Off"
}

func editRDPSessionSettings(c *Config) error {
	for {
		s := c.RDP.Session
		options := []choice{{"clipboard", "Clipboard: " + boolSettingLabel(s.Clipboard)}, {"fullscreen", "Full screen: " + boolSettingLabel(s.Fullscreen)}, {"folder-new", "Add shared folder…"}}
		for i, f := range s.Folders {
			options = append(options, choice{fmt.Sprintf("folder:%d", i), fmt.Sprintf("Shared folder %s: %s — %t", f.Name, f.Path, f.Enabled)})
		}
		id, back, err := choose("RDP Session Settings", options, "Applies to every RDP session on this device. Folder paths never enter shared inventory. Sharing is opt-in; unavailable enabled folders stop launch rather than silently substituting paths.")
		if err != nil || back {
			return err
		}
		if id == "clipboard" || id == "fullscreen" {
			value, b, e := chooseOne("RDP "+id, []choice{{"keep", "Use existing client settings"}, {"on", "On"}, {"off", "Off"}}, "Personal settings only.", "keep")
			if e != nil {
				return e
			}
			if b {
				continue
			}
			var p *bool
			if value != "keep" {
				v := value == "on"
				p = &v
			}
			if id == "clipboard" {
				c.RDP.Session.Clipboard = p
			} else {
				c.RDP.Session.Fullscreen = p
			}
			continue
		}
		if id == "folder-new" {
			name, b, e := inventoryPrompt("Remote folder name", "")
			if e != nil {
				return e
			}
			if b {
				continue
			}
			path, b, e := inventoryPrompt("Local folder path", "")
			if e != nil {
				return e
			}
			if b {
				continue
			}
			next := s
			next.Folders = append(append([]RDPFolder(nil), s.Folders...), RDPFolder{Name: name, Path: path})
			if e = next.validate(); e != nil {
				if e = notice(e.Error()); e != nil {
					return e
				}
				continue
			}
			c.RDP.Session = next
			continue
		}
		var index int
		if _, err = fmt.Sscanf(id, "folder:%d", &index); err != nil || index < 0 || index >= len(s.Folders) {
			continue
		}
		f := s.Folders[index]
		action, b, e := choose("Shared folder — "+f.Name, []choice{{"toggle", fmt.Sprintf("Enabled: %t", f.Enabled)}, {"path", "Local path: " + f.Path}, {"remove", "Remove mapping"}}, "Enabling exposes this directory to the remote session. No directories are created automatically.")
		if e != nil {
			return e
		}
		if b {
			continue
		}
		next := append([]RDPFolder(nil), s.Folders...)
		switch action {
		case "toggle":
			next[index].Enabled = !f.Enabled
		case "remove":
			next = append(next[:index], next[index+1:]...)
		case "path":
			v, b, e := inventoryPrompt("Local folder path", f.Path)
			if e != nil {
				return e
			}
			if b {
				continue
			}
			next[index].Path = v
		}
		candidate := s
		candidate.Folders = next
		if e = candidate.validate(); e != nil {
			if e = notice(e.Error()); e != nil {
				return e
			}
			continue
		}
		c.RDP.Session = candidate
	}
}

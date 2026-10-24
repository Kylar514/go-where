package main

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
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
		if *s.Fullscreen {
			args = append(args, "/f")
		}
		// Fullscreen is a FreeRDP flag, not a boolean toggle. Windowed uses
		// its default with all explicit fullscreen flags removed upstream.
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
	if value != nil && *value {
		return "Enabled"
	}
	return "Disabled"
}

func displayModeLabel(value *bool) string {
	if value != nil && *value {
		return "Full screen"
	}
	return "Windowed"
}

func editGlobalRDPSession(path string) error {
	original, before, draft, err := settingsDraft(path)
	if err != nil {
		return err
	}
	previousSave, previousState := saveActiveSettings, settingsSaveState
	defer func() { saveActiveSettings, settingsSaveState = previousSave, previousState }()
	saveActiveSettings = func() error {
		if err := saveSettingsDraft(path, original, before, draft); err != nil {
			return err
		}
		data, err := readSettingsData(path)
		if err != nil {
			return err
		}
		original = data
		before = cloneAgentSettings(draft)
		return nil
	}
	settingsSaveState = func() string {
		if !reflect.DeepEqual(before, draft) {
			return "Unsaved global changes"
		}
		return "Global settings saved"
	}
	return editRDPSessionSettings(&draft)
}

// Recognized legacy switches become explicit settings. Preserve every other
// argument, including richer clipboard modes that this UI cannot represent.
func explicitRDPSession(r RDPConfig) RDPConfig {
	clipboard, fullscreen := false, false
	filter := func(args []string) []string {
		if args == nil {
			return nil
		}
		out := []string{}
		for _, arg := range args {
			if strings.HasPrefix(strings.ToLower(arg), "/drive:") {
				name, path, ok := strings.Cut(arg[len("/drive:"):], ",")
				folder := RDPFolder{Name: name, Path: path, Enabled: true}
				if ok && (RDPSessionSettings{Folders: []RDPFolder{folder}}).validate() == nil {
					found := false
					for _, f := range r.Session.Folders {
						if f.Name == name || f.Path == path {
							found = true
							break
						}
					}
					if !found {
						r.Session.Folders = append(append([]RDPFolder(nil), r.Session.Folders...), folder)
					}
					continue
				}
			}
			switch strings.ToLower(arg) {
			case "+clipboard", "/clipboard":
				clipboard = true
			case "-clipboard":
				clipboard = false
			case "/f", "+f", "/fullscreen":
				fullscreen = true
			case "-f", "-fullscreen":
				fullscreen = false
			default:
				out = append(out, arg)
			}
		}
		return out
	}
	r.Client = filter(r.Client)
	r.ClientArgs = filter(r.ClientArgs)
	if r.Session.Clipboard == nil {
		r.Session.Clipboard = &clipboard
	}
	if r.Session.Fullscreen == nil {
		r.Session.Fullscreen = &fullscreen
	}
	return r
}

func editRDPSessionSettings(c *Config) error {
	c.RDP = explicitRDPSession(c.RDP)
	for {
		s := c.RDP.Session
		options := []choice{{"clipboard", "Clipboard: " + boolSettingLabel(s.Clipboard)}, {"fullscreen", "Display mode: " + displayModeLabel(s.Fullscreen)}, {"folder-new", "Add shared folder…"}}
		for i, f := range s.Folders {
			options = append(options, choice{fmt.Sprintf("folder:%d", i), fmt.Sprintf("Shared folder %s: %s — %s", f.Name, f.Path, boolSettingLabel(&f.Enabled))})
		}
		text := "Applies to every RDP session on this device. Folder paths never enter shared inventory. Sharing is opt-in; unavailable enabled folders stop launch rather than silently substituting paths."
		if len(c.RDP.ClientArgs) > 0 {
			text += "\n\nPreserved additional client arguments: " + strings.Join(c.RDP.ClientArgs, " ")
		}
		m := newDialog("RDP Session Settings", options, text)
		m.footerActions = []choice{{"select", "Edit field"}, {"save", "Save"}}
		m.optionDetails = map[string]string{
			"clipboard":  "Clipboard\nEnabled shares clipboard data with the remote session. Disabled prevents clipboard redirection. Applies globally on this device.",
			"fullscreen": "Display mode\nChoose Windowed or Full screen for every RDP session on this device.",
			"folder-new": "Add shared folder\nRedirect a local directory as a named drive inside Windows. New mappings start Disabled; local paths never enter shared inventory.",
		}
		for i, f := range s.Folders {
			m.optionDetails[fmt.Sprintf("folder:%d", i)] = fmt.Sprintf("Shared folder: %s\nLocal path: %s\nSharing: %s\nEnabling exposes this directory to every RDP session on this device. Missing enabled directories stop launch.", f.Name, f.Path, boolSettingLabel(&f.Enabled))
		}
		id, back, err := runChoice(m, menuKey(m.title))
		if err != nil || back {
			return err
		}
		if id == "save" {
			if c.settingsPath != "" {
				original, before, draft, e := settingsDraft(c.settingsPath)
				if e != nil {
					return e
				}
				draft.RDP.Session = c.RDP.Session
				draft.RDP.Client = c.RDP.Client
				draft.RDP.ClientArgs = c.RDP.ClientArgs
				if e = saveSettingsDraft(c.settingsPath, original, before, draft); e != nil {
					return e
				}
			}
			continue
		}
		if id == "clipboard" || id == "fullscreen" {
			choices := []choice{{"on", "Enabled"}, {"off", "Disabled"}}
			title := "Clipboard"
			current := "off"
			if s.Clipboard != nil && *s.Clipboard {
				current = "on"
			}
			if id == "fullscreen" {
				title = "Display mode"
				choices = []choice{{"off", "Windowed"}, {"on", "Full screen"}}
				current = "off"
				if s.Fullscreen != nil && *s.Fullscreen {
					current = "on"
				}
			}
			value, b, e := chooseOne(title, choices, "Applies to every RDP session on this device.", current)
			if e != nil {
				return e
			}
			if b {
				continue
			}
			v := value == "on"
			p := &v
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
		if e := editSharedFolder(&s, index); e != nil {
			return e
		}
		c.RDP.Session = s
	}
}

func editSharedFolder(s *RDPSessionSettings, index int) error {
	f := s.Folders[index]
	for {
		m := newDialog("Shared folder — "+f.Name, []choice{{"toggle", "Sharing: " + boolSettingLabel(&f.Enabled)}, {"path", "Local path: " + f.Path}, {"remove", "Remove mapping"}}, "Apply updates the session draft; Session Save persists it.")
		m.footerActions = []choice{{"select", "Edit field"}, {"save", "Apply"}}
		m.optionDetails = map[string]string{"toggle": "Sharing\nEnabling exposes this directory to every RDP session on this device.", "path": "Local path\nDirectory on this computer redirected to Windows. Tab completes local paths. No directories are created.", "remove": "Remove mapping\nRemoves this redirection after confirmation. Never deletes the directory or files."}
		action, b, e := runChoice(m, menuKey(m.title))
		if e != nil {
			return e
		}
		if b {
			return nil
		}
		switch action {
		case "toggle":
			f.Enabled = !f.Enabled
		case "remove":
			yes, e := confirmSettingsDelete(f.Name, "RDP folder mappings (files are not deleted)")
			if e != nil {
				return e
			}
			if yes {
				s.Folders = append(append([]RDPFolder(nil), s.Folders[:index]...), s.Folders[index+1:]...)
				return nil
			}
		case "path":
			v, b, e := inventoryPrompt("Local folder path", f.Path)
			if e != nil {
				return e
			}
			if b {
				continue
			}
			f.Path = v
		case "save":
			candidate := *s
			candidate.Folders = append([]RDPFolder(nil), s.Folders...)
			candidate.Folders[index] = f
			if e = candidate.validate(); e != nil {
				if e = notice(e.Error()); e != nil {
					return e
				}
				continue
			}
			*s = candidate
			return nil
		}
	}
}

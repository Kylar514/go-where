package main

import (
	"fmt"
	"maps"
	"strings"
)

type hotkeyAction struct {
	id, label, scope string
	keys             []string
}

var hotkeyActions = []hotkeyAction{
	{"debug", "Debug launch", "picker", []string{"alt+d"}},
	{"source_settings", "Sources: open Settings", "sources", []string{"ctrl+s"}},
	{"source_create", "Create source", "sources", []string{"ctrl+n"}},
	{"source_attach", "Add existing source", "sources", []string{"ctrl+o"}},
	{"source_remove", "Remove highlighted source", "sources", []string{"ctrl+alt+d"}},
	{"delete_host", "Delete saved host", "menu", []string{"delete"}},
	{"quit", "Exit picker", "picker", []string{"esc", "ctrl+c"}},
	{"settings", "Settings", "picker", []string{"ctrl+s"}},
	{"sources", "Sources", "picker", []string{"ctrl+b"}},
	{"new", "New host/tunnel", "picker", []string{"ctrl+n"}},
	{"edit", "Edit host/selection", "picker", []string{"ctrl+e"}},
	{"launch", "Launch/review/toggle", "picker", []string{"enter"}},
	{"reload", "Reload", "picker", []string{"ctrl+r"}},
	{"help", "Help", "picker", []string{"?", "f1"}},
	{"modes", "Mode chooser", "picker", []string{"ctrl+o"}},
	{"target", "Launch location", "picker", []string{"ctrl+g"}},
	{"reset_layout", "Reset layout", "picker", []string{"ctrl+z"}},
	{"account", "Retired RDP account binding", "picker", nil},
	{"profile", "Connect as", "picker", []string{"alt+a"}},
	{"previous_mode", "Previous tab", "picker", []string{"left", "ctrl+h"}},
	{"next_mode", "Next tab", "picker", []string{"right", "ctrl+l"}},
	{"ssh_mode", "SSH tab", "picker", []string{"alt+1"}},
	{"rdp_mode", "RDP tab", "picker", []string{"alt+2"}},
	{"tunnel_mode", "Tunnels tab", "picker", []string{"alt+3"}},
	{"scp_mode", "SCP tab", "picker", []string{"alt+4"}},
	{"legacy_mode", "Legacy SCP tab", "picker", []string{"alt+5"}},
	{"rsync_mode", "Rsync tab", "picker", []string{"alt+6"}},
	{"up", "Move up", "both", []string{"up", "ctrl+k"}},
	{"down", "Move down", "both", []string{"down", "ctrl+j"}},
	{"page_up", "List page up", "both", []string{"ctrl+u", "pgup"}},
	{"page_down", "List page down", "both", []string{"ctrl+d", "pgdown"}},
	{"details_up", "Details up", "both", []string{"shift+up"}},
	{"details_down", "Details down", "both", []string{"shift+down"}},
	{"details_page_up", "Details page up", "both", []string{"shift+pgup"}},
	{"details_page_down", "Details page down", "both", []string{"shift+pgdown"}},
	{"mark", "Mark and move up", "picker", []string{"tab"}},
	{"mark_previous", "Mark and move down", "picker", []string{"shift+tab"}},
	{"pane", "Launch in pane", "picker", []string{"ctrl+alt+p"}},
	{"window", "Launch in window", "picker", []string{"ctrl+alt+w"}},
	{"session", "Launch in session", "picker", []string{"ctrl+alt+n"}},
	{"terminal", "Launch in terminal", "picker", []string{"ctrl+alt+t"}},
	{"raw_settings", "Raw settings in pane", "picker", []string{"ctrl+alt+s"}},
	{"raw_edit", "Legacy edit in pane", "picker", []string{"ctrl+alt+e"}},
	{"back", "Menu back/discard", "menu", []string{"esc", "ctrl+c", "ctrl+h", "left"}},
	{"select", "Menu select/toggle", "menu", []string{"enter", "ctrl+l"}},
	{"menu_save", "Save Settings", "menu", []string{"ctrl+s"}},
	{"menu_details_up", "Legacy menu Details up", "menu", nil},
	{"menu_details_down", "Legacy menu Details down", "menu", nil},
}

var menuHotkeys Config

func effectiveKeys(c Config, a hotkeyAction) []string {
	if keys, ok := c.Hotkeys[a.id]; ok {
		return keys
	}
	return a.keys
}

func canonicalKey(c Config, key, scope string) string {
	key = strings.ReplaceAll(key, "alt+ctrl+", "ctrl+alt+")
	old := false
	for _, a := range hotkeyActions {
		if scope == "sources" && a.id == "menu_save" {
			continue
		}
		if a.scope != scope && a.scope != "both" && !(scope == "sources" && a.scope == "menu") {
			continue
		}
		for _, k := range effectiveKeys(c, a) {
			if k == key {
				return a.keys[0]
			}
		}
		for _, k := range a.keys {
			if k == key {
				old = true
			}
		}
	}
	if old {
		return "unbound"
	}
	return key
}

func validHotkey(key string) bool {
	if key == "?" {
		return true
	}
	parts := strings.Split(key, "+")
	if len(parts) > 3 {
		return false
	}
	seen := map[string]bool{}
	for i, p := range parts {
		if i < len(parts)-1 {
			if p != "ctrl" && p != "alt" && p != "shift" {
				return false
			}
			if seen[p] {
				return false
			}
			seen[p] = true
			continue
		}
		if len(p) == 1 && p[0] >= 'a' && p[0] <= 'z' {
			return len(parts) > 1
		}
		if len(p) == 1 && p[0] >= '0' && p[0] <= '9' {
			return len(parts) > 1
		}
		switch p {
		case "delete", "enter", "esc", "tab", "up", "down", "left", "right", "pgup", "pgdown", "home", "end", "f1", "f2", "f3", "f4", "f5", "f6", "f7", "f8", "f9", "f10", "f11", "f12":
			return true
		}
	}
	return false
}

func validateHotkeys(c Config) error {
	known := map[string]bool{}
	for _, scope := range []string{"picker", "menu", "sources"} {
		seen := map[string]string{}
		for _, a := range hotkeyActions {
			if scope == "sources" && a.id == "menu_save" {
				continue
			}
			known[a.id] = true
			if a.scope != scope && a.scope != "both" && !(scope == "sources" && a.scope == "menu") {
				continue
			}
			for _, key := range effectiveKeys(c, a) {
				if !validHotkey(key) {
					return fmt.Errorf("invalid hotkey %q for %s", key, a.label)
				}
				if previous, ok := seen[key]; ok {
					return fmt.Errorf("%s: hotkey %s conflicts with %s", a.label, key, previous)
				}
				seen[key] = a.label
			}
		}
	}
	for id := range c.Hotkeys {
		if !known[id] {
			return fmt.Errorf("unknown hotkey action %q", id)
		}
	}
	return nil
}

func keyHint(c Config, id string) string {
	for _, a := range hotkeyActions {
		if a.id == id {
			return strings.Join(effectiveKeys(c, a), "/")
		}
	}
	return ""
}

func configuredHelp(c Config) string {
	var lines []string
	for _, a := range hotkeyActions {
		if a.scope != "menu" {
			if a.id == "account" {
				continue
			}
			lines = append(lines, fmt.Sprintf("%-24s %s", keyHint(c, a.id), a.label))
		}
	}
	return strings.Join(lines, "\n")
}

func editHotkeys(c *Config) error {
	before := maps.Clone(c.Hotkeys)
	for {
		options := []choice{{"restore", "Restore all defaults"}}
		details := map[string]string{"restore": "Restore default bindings after confirmation. No new shortcuts are introduced."}
		groups := map[string]int{}
		for group, scope := range []string{"picker", "both", "menu", "sources"} {
			for _, a := range hotkeyActions {
				if a.scope != scope || a.id == "account" || strings.HasPrefix(a.id, "menu_details_") {
					continue
				}
				options = append(options, choice{a.id, a.label + ": " + keyHint(*c, a.id)})
				groups[a.id] = group + 1
				details[a.id] = a.label + "\n\nScope: " + a.scope + "\nCurrent: " + keyHint(*c, a.id) + "\nDefault: " + strings.Join(a.keys, ", ") + "\n\nEnter comma-separated bindings. Blank disables this action; its mouse control remains available. Conflicts are rejected."
			}
		}
		m := newDialog("Hotkeys", options, "")
		m.optionDetails = details
		m.optionGroups = groups
		m.footerActions = []choice{{"select", "Edit binding"}, {"save", "Save"}}
		id, back, err := runChoice(m, menuKey(m.title))
		if err != nil || back {
			c.Hotkeys = before
			return err
		}
		if id == "save" {
			if c.settingsPath != "" {
				original, old, draft, e := settingsDraft(c.settingsPath)
				if e != nil {
					return e
				}
				draft.Hotkeys = c.Hotkeys
				if e = saveSettingsDraft(c.settingsPath, original, old, draft); e != nil {
					return e
				}
			}
			before = maps.Clone(c.Hotkeys)
			continue
		}
		if id == "restore" {
			_, b, e := choose("Restore hotkeys?", []choice{{"confirm", "Restore defaults"}}, "All customized bindings will be replaced. Save is still required.")
			if e != nil {
				return e
			}
			if b {
				continue
			}
			c.Hotkeys = nil
			continue
		}
		value, b, err := inventoryPrompt("Bindings (comma-separated)", strings.ReplaceAll(keyHint(*c, id), "/", ", "))
		if err != nil {
			return err
		}
		if b {
			continue
		}
		keys := []string{}
		for _, key := range strings.Split(value, ",") {
			if key = strings.ToLower(strings.TrimSpace(key)); key != "" {
				keys = append(keys, key)
			}
		}
		draft := *c
		draft.Hotkeys = maps.Clone(c.Hotkeys)
		if draft.Hotkeys == nil {
			draft.Hotkeys = map[string][]string{}
		}
		draft.Hotkeys[id] = keys
		if err = validateHotkeys(draft); err != nil {
			if err = notice(err.Error()); err != nil {
				return err
			}
			continue
		}
		c.Hotkeys = draft.Hotkeys
	}
}

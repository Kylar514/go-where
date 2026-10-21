package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

func inventoryPrompt(label, value string) (string, bool, error) {
	return pathPrompt(label, value, false, "")
}

func registerSource(settings string, s InventorySource) error {
	if real, err := filepath.EvalSymlinks(settings); err == nil {
		settings = real
	}
	if err := os.MkdirAll(filepath.Dir(settings), 0700); err != nil {
		return err
	}
	// Serialize go-where's registry writes; never replace a config from a stale UI snapshot.
	lock, err := os.OpenFile(settings+".lock", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("cannot lock settings (another source edit may be running): %w", err)
	}
	lock.Close()
	defer os.Remove(settings + ".lock")
	original, readErr := os.ReadFile(settings)
	if readErr != nil && !os.IsNotExist(readErr) {
		return readErr
	}
	c, err := loadConfig(settings)
	if err != nil {
		return err
	}
	c.Sources = append(c.Sources, s)
	if err = c.validate(); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(settings), ".go-where-settings-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	e := json.NewEncoder(f)
	e.SetIndent("", "  ")
	if err = e.Encode(c); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	current, currentErr := os.ReadFile(settings)
	if (currentErr != nil && !os.IsNotExist(currentErr)) || !bytes.Equal(original, current) {
		return fmt.Errorf("settings changed during source registration; retry")
	}
	return os.Rename(f.Name(), settings)
}

func attachInventory(name, path string) (InventorySource, error) {
	s := InventorySource{Name: name, Type: "sqlite", Path: path}
	p, err := sourcePath(s)
	if err != nil {
		return s, err
	}
	s.Path = p
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(p), RawQuery: "mode=ro"}
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return s, err
	}
	defer db.Close()
	if err = db.QueryRow("SELECT id FROM source").Scan(&s.ID); err != nil {
		return s, err
	}
	check, err := openInventory(s)
	if err != nil {
		return s, err
	}
	check.Close()
	return s, nil
}

func sourceMenu(settings string, c Config, state *PickerState) error {
	options := []choice{{"", "All sources"}, {"ssh-config", "SSH config"}}
	for _, s := range c.Sources {
		options = append(options, choice{s.ID, s.Name + " — SQLite: " + s.Path})
	}
	options = append(options, choice{"create", "Create local SQLite source…"}, choice{"attach", "Add existing SQLite source…"})
	id, back, err := choose("Sources", options, "SQLite sources are independent of your personal SSH config. No database is created until you explicitly confirm. Managed records support SSH only in this milestone; Diglet tunnels remain a separate view.")
	if err != nil || back {
		return err
	}
	if id != "create" && id != "attach" {
		state.Source = id
		state.Focus = ""
		return nil
	}
	name, back, err := inventoryPrompt("Source name", "Personal")
	if err != nil || back {
		return err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("source name is required")
	}
	for _, s := range c.Sources {
		if s.Name == name {
			return fmt.Errorf("source name %q already exists", name)
		}
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	path, back, err := pathPrompt("SQLite file", filepath.Join(dir, "go-where", "personal.db"), true, "")
	if err != nil || back {
		return err
	}
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("database path is required")
	}
	action := "Create"
	if id == "attach" {
		action = "Add existing"
	}
	_, back, err = choose(action+" source", []choice{{"yes", action + " source"}}, name+"\n"+path+"\nExisting files are never overwritten. Adding a source does not import SSH config.")
	if err != nil || back {
		return err
	}
	var s InventorySource
	if id == "create" {
		s, err = createInventory(name, path)
	} else {
		s, err = attachInventory(name, path)
	}
	if err != nil {
		return err
	}
	if err = registerSource(settings, s); err != nil {
		return fmt.Errorf("database remains at %s; could not register it: %w", s.Path, err)
	}
	state.Source = s.ID
	state.Focus = ""
	return nil
}

func writableSource(c Config, preferred string) (InventorySource, bool, error) {
	if preferred != "" && preferred != "ssh-config" {
		s, err := findSource(c, preferred)
		return s, false, err
	}
	if len(c.Sources) == 0 {
		return InventorySource{}, false, fmt.Errorf("create a SQLite source first with Ctrl+B → Create local SQLite source")
	}
	options := []choice{}
	for _, s := range c.Sources {
		options = append(options, choice{s.ID, s.Name})
	}
	id, back, err := choose("Save connection in…", options, "")
	if err != nil || back {
		return InventorySource{}, back, err
	}
	s, err := findSource(c, id)
	return s, false, err
}

func editConnection(c Config, state *PickerState, h *Host) error {
	preferred := state.Source
	if h != nil {
		preferred = h.SourceID
	}
	s, back, err := writableSource(c, preferred)
	if err != nil || back {
		return err
	}
	r := Connection{ID: inventoryID(), Port: 22}
	existing := h != nil
	if existing {
		records, e := sourceConnections(s)
		if e != nil {
			return e
		}
		found := false
		for _, record := range records {
			if record.ID == h.ConnectionID {
				r = record
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("connection no longer exists")
		}
	}
	for {
		options := []choice{
			{"alias", "Alias: " + r.Alias}, {"address", "Address: " + r.Address}, {"port", fmt.Sprintf("Port: %d", r.Port)},
			{"user", "User: " + r.User}, {"key", "Identity file: " + r.IdentityFile}, {"jump", "Jump connection ID: " + r.JumpID},
			{"tags", "Tags: " + strings.Join(r.Tags, ", ")}, {"notes", "Notes: " + r.Notes}, {"save", "Save connection"},
		}
		if existing {
			options = append(options, choice{"duplicate", "Duplicate as new connection"}, choice{"delete", "Delete connection…"})
		}
		id, back, e := choose("SSH connection — "+s.Name, options, "User, address and key may contain ${NAME} variables from local settings. Blank user/key uses OpenSSH's local user/agent/default keys, not SSH config. Jump hosts must belong to this source. Esc discards unsaved edits.")
		if e != nil || back {
			return e
		}
		switch id {
		case "save":
			err = changeConnection(s, r, false)
			if err == nil {
				state.Mode = "ssh"
				state.Source = s.ID
				state.Focus = s.ID + ":" + r.ID
				return nil
			}
			if e = notice(err.Error()); e != nil {
				return e
			}
			continue
		case "duplicate":
			r.ID = inventoryID()
			r.Alias += "-copy"
			existing = false
			continue
		case "delete":
			_, back, e = choose("Delete "+r.Alias+"?", []choice{{"delete", "Delete permanently"}}, "Connections used as jump hosts cannot be deleted until their references are changed.")
			if e != nil {
				return e
			}
			if back {
				continue
			}
			return changeConnection(s, r, true)
		case "jump":
			records, e := sourceConnections(s)
			if e != nil {
				return e
			}
			choices := []choice{{"", "None (direct)"}}
			for _, j := range records {
				if j.ID != r.ID {
					choices = append(choices, choice{j.ID, j.Alias + " — " + j.Address})
				}
			}
			v, b, e := choose("Jump host in "+s.Name, choices, "")
			if e != nil {
				return e
			}
			if !b {
				r.JumpID = v
			}
			continue
		}
		var label, value string
		switch id {
		case "alias":
			label, value = "Alias", r.Alias
		case "address":
			label, value = "Address / DNS name", r.Address
		case "port":
			label, value = "SSH port", fmt.Sprint(r.Port)
		case "user":
			label, value = "SSH user (optional, supports ${NAME})", r.User
		case "key":
			label, value = "Identity file (optional, supports ${NAME})", r.IdentityFile
		case "tags":
			label, value = "Tags (comma-separated)", strings.Join(r.Tags, ", ")
		case "notes":
			label, value = "Notes", r.Notes
		}
		value, back, e = inventoryPrompt(label, value)
		if e != nil {
			return e
		}
		if back {
			continue
		}
		switch id {
		case "alias":
			r.Alias = strings.TrimSpace(value)
		case "address":
			r.Address = strings.TrimSpace(value)
		case "user":
			r.User = value
		case "key":
			r.IdentityFile = value
		case "notes":
			r.Notes = value
		case "port":
			p, e := connectionPort(value)
			if e != nil {
				if e = notice(e.Error()); e != nil {
					return e
				}
			} else {
				r.Port = p
			}
		case "tags":
			r.Tags = nil
			for _, tag := range strings.Split(value, ",") {
				if t := strings.TrimSpace(tag); t != "" {
					r.Tags = append(r.Tags, t)
				}
			}
		}
	}
}

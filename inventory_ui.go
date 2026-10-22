package main

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/tailscale/hujson"
)

func inventoryPrompt(label, value string) (string, bool, error) {
	return pathPrompt(label, value, false, "")
}

func registerSource(settings string, s InventorySource) error {
	return modifySettings(settings, nil, func(v *hujson.Value, c Config) error {
		c.Sources = append(c.Sources, s)
		if err := c.validate(); err != nil {
			return err
		}
		node := v.Find("/sources")
		if node == nil {
			return setSetting(v, []string{"sources"}, []InventorySource{s})
		}
		if _, ok := node.Value.(*hujson.Array); !ok {
			return setSetting(v, []string{"sources"}, []InventorySource{s})
		}
		return patchSetting(v, "add", "/sources/-", s)
	})
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
	defer menuSession()()
	for {
		options := []choice{{"", "All enabled sources"}}
		if sourceEnabled(c, "ssh-config") {
			options = append(options, choice{"ssh-config", "SSH config"})
		}
		for _, s := range c.Sources {
			if !sourceEnabled(c, s.ID) {
				continue
			}
			options = append(options, choice{s.ID, s.Name + " — SQLite: " + s.Path})
		}
		options = append(options, choice{"create", "Create local SQLite source…"}, choice{"attach", "Add existing SQLite source…"})
		id, back, err := choose("Sources", options, "SQLite sources hold SSH, RDP and tunnel connections independently of personal files. No database is created until you explicitly confirm. Legacy file-backed connections retain their file editors.")
		if err != nil || back {
			return err
		}
		if id == "settings" {
			raw, e := settingsMenuAt(settings, "sources")
			if e != nil {
				return e
			}
			if raw {
				return notice("Use Settings from the main picker to open the raw editor.")
			}
			c, e = loadConfig(settings)
			if e != nil {
				return e
			}
			menuHotkeys = c
			continue
		}
		if strings.HasPrefix(id, "remove:") {
			return removeSourceRegistration(settings, c, state, strings.TrimPrefix(id, "remove:"))
		}
		if id != "create" && id != "attach" {
			state.Source = id
			state.Focus = ""
			return nil
		}
		return addInventorySource(settings, c, state, id)
	}
}

func addInventorySource(settings string, c Config, state *PickerState, id string) error {
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

func writableSource(c Config, preferred string, settings ...string) (InventorySource, bool, error) {
	if preferred != "" && preferred != "ssh-config" {
		s, err := findSource(c, preferred)
		return s, false, err
	}
	path := ""
	if len(settings) > 0 {
		path = settings[0]
	}
	if len(c.Sources) == 0 && path == "" {
		return InventorySource{}, false, fmt.Errorf("create a SQLite source first with Ctrl+B → Create local SQLite source")
	}
	for {
		options := []choice{}
		for _, s := range c.Sources {
			options = append(options, choice{s.ID, s.Name})
		}
		if path != "" {
			options = append(options, choice{"create", "Create new source…"})
		}
		id, back, err := choose("Save connection in…", options, "")
		if err != nil || back {
			return InventorySource{}, back, err
		}
		if id == "create" {
			state := PickerState{}
			if err = addInventorySource(path, c, &state, "create"); err != nil {
				if err = notice(err.Error()); err != nil {
					return InventorySource{}, false, err
				}
				continue
			}
			if state.Source == "" {
				continue
			}
			c, err = loadConfig(path)
			if err != nil {
				return InventorySource{}, false, err
			}
			s, err := findSource(c, state.Source)
			return s, false, err
		}
		s, err := findSource(c, id)
		return s, false, err
	}
}

func editConnection(c Config, state *PickerState, h *Host) error {
	return editConnectionDraft(c, state, h, nil)
}

func editConnectionDraft(c Config, state *PickerState, h *Host, profile *Connection) error {
	defer menuSession()()
	preferred := state.Source
	if h != nil {
		preferred = h.SourceID
	}
	if len(c.Sources) == 0 && !(h == nil && profile == nil && state.Mode == "tunnels" && state.ConfigPath != "") {
		return fmt.Errorf("create a SQLite source first with Ctrl+B")
	}
	var s InventorySource
	if len(c.Sources) > 0 {
		s = c.Sources[0]
	}
	if candidate, e := findSource(c, preferred); e == nil {
		s = candidate
	}
	var err error
	if h == nil && profile == nil && state.Mode == "tunnels" {
		var back bool
		s, back, err = writableSource(c, "", state.ConfigPath)
		if err != nil || back {
			return err
		}
		if _, e := findSource(c, s.ID); e != nil {
			c.Sources = append(c.Sources, s)
		}
	}
	r := Connection{ID: inventoryID(), Type: "ssh", Port: 22}
	if h == nil && state.Mode == "tunnels" {
		r = connectionWithType(r, "tunnel")
	}
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
	originalKind := r.kind()
	if profile != nil {
		r = *profile
		originalKind = r.kind()
		existing = false
	}
	if profile != nil && saveActiveHost != nil && profile.kind() != "tunnel" {
		parentSave, parentState := saveActiveHost, hostSaveState
		defer func() { saveActiveHost, hostSaveState = parentSave, parentState }()
		saveActiveHost = func() error {
			previous := *profile
			*profile = r
			if err := parentSave(); err != nil {
				*profile = previous
				return err
			}
			return nil
		}
		hostSaveState = func() string {
			if !reflect.DeepEqual(r, *profile) {
				return "Unsaved changes"
			}
			return parentState()
		}
	}
	if r.kind() == "tunnel" && profile == nil {
		previousSave, previousState := saveActiveTunnel, tunnelSaveState
		defer func() { saveActiveTunnel, tunnelSaveState = previousSave, previousState }()
		before := r
		saved := false
		saveActiveTunnel = func() error {
			if existing && h != nil {
				if err := ensureInventoryTunnelStopped(*h); err != nil {
					return err
				}
			}
			if err := changeConnection(s, r, false); err != nil {
				return err
			}
			before = r
			saved = true
			existing = true
			state.Source = s.ID
			state.Mode = "tunnels"
			state.Focus = s.ID + ":" + r.ID
			return nil
		}
		tunnelSaveState = func() string {
			if !reflect.DeepEqual(before, r) {
				return "Unsaved changes"
			}
			if saved {
				return "Saved"
			}
			return "No unsaved changes"
		}
	}
	for {
		options := connectionFields(r, s, c)
		if profile != nil {
			var fields []choice
			for _, field := range options {
				switch field.ID {
				case "type", "source", "alias", "address", "tags", "notes", "local_agent", "local_compatibility":
					continue
				}
				if field.ID == "jump" && profile.kind() != "tunnel" {
					continue
				}
				fields = append(fields, field)
			}
			options = fields
		}
		if existing {
			options = append(options, choice{"duplicate", "Duplicate as new connection"}, choice{"delete", "Delete connection…"})
		}
		title := "Connection — " + s.Name
		text := "Type changes fields in this editor. ${NAME} variables are local settings; no passwords are stored here. SSH references stay within this source. RDP uses local client/account preferences. Tunnel binds are loopback-only. Esc discards unsaved edits."
		if r.kind() == "tunnel" && profile == nil && saveActiveTunnel != nil {
			title = "Tunnel — " + s.Name
			text = "SSH tunnels reuse a saved host/profile.\n\nLocal binds must be loopback addresses. Remote targets are reached from the SSH host.\n\nSave tunnel persists the draft. Back discards changes since the last save."
			var fields []choice
			for _, field := range options {
				if field.ID != "type" && field.ID != "save" {
					fields = append(fields, field)
				}
			}
			options = fields
		}
		id, back, e := choose(title, options, text)
		if e != nil || back {
			return e
		}
		switch id {
		case "compatibility":
			if e = editSSHCompatibility(&r); e != nil {
				return e
			}
			continue
		case "ssh_agent":
			v, b, e := chooseAgent(c, false, r.SSHAgent)
			if e != nil {
				return e
			}
			if !b {
				r.SSHAgent = v
			}
			continue
		case "agent_info":
			continue
		case "local_agent", "local_compatibility":
			if !existing || originalKind != "ssh" || state.ConfigPath == "" {
				if e = notice("Save an SSH connection first, then edit its local override."); e != nil {
					return e
				}
				continue
			}
			editor := editAgentOverrideDraft
			if id == "local_compatibility" {
				editor = editCompatibilityOverrideDraft
			}
			if e = editConnectionOverride(state.ConfigPath, *h, editor); e != nil {
				if e = notice(e.Error()); e != nil {
					return e
				}
			}
			c, e = loadConfig(state.ConfigPath)
			if e != nil {
				return e
			}
			continue
		case "account_info":
			continue
		case "remote_target":
			hosts, e := sourceInventoryHosts(s)
			if e != nil {
				return e
			}
			choices := []choice{{"custom", "Custom address…"}}
			for _, host := range hosts {
				choices = append(choices, choice{host.ID, host.Alias + " — " + host.Address})
			}
			current := r.RemoteHostID
			if current == "" {
				current = "custom"
			}
			v, b, e := chooseOne("Tunnel remote target", choices, "Saved hosts share their address and local host-address override. The target is reached from the SSH host.", current)
			if e != nil {
				return e
			}
			if b {
				continue
			}
			if v == "custom" {
				r.RemoteHostID = ""
			} else {
				r.RemoteHostID = v
			}
			continue
		case "type":
			kind, b, e := chooseOne("Connection type", []choice{{"ssh", "SSH"}, {"rdp", "RDP"}, {"tunnel", "Tunnel"}}, "", r.kind())
			if e != nil {
				return e
			}
			if b || kind == r.kind() {
				continue
			}
			if existing || r.User != "" || r.IdentityFile != "" || r.JumpID != "" || r.Domain != nil || r.Account != "" || tunnelBackendConfigured(r) {
				_, b, e = choose("Change type?", []choice{{"change", "Change to " + kind}}, "Type-specific fields will be reset. Name, tags and notes are retained; SSH/RDP address, user and jump are retained when compatible. No change is stored until Save.")
				if e != nil {
					return e
				}
			}
			if !b {
				r = connectionWithType(r, kind)
			}
			continue
		case "source":
			if existing {
				if e = notice("An existing connection stays in its source. Duplicate first to choose a different source."); e != nil {
					return e
				}
				continue
			}
			selected, b, e := writableSource(c, "", state.ConfigPath)
			if e != nil {
				return e
			}
			if b {
				continue
			}
			if selected.ID != s.ID {
				if r.JumpID != "" || r.SSHID != "" || r.RemoteHostID != "" {
					_, b, e = choose("Change source?", []choice{{"change", "Change source"}}, "Jump/SSH connection references will be cleared. Nothing is saved until Save.")
					if e != nil {
						return e
					}
				}
				if !b {
					s = selected
					r.JumpID = ""
					r.SSHID = ""
					r.RemoteHostID = ""
				}
			}
			continue
		case "tunnel_type":
			v, b, e := chooseOne("Tunnel backend", []choice{{"ssh", "SSH"}, {"kubectl", "kubectl"}}, "", r.TunnelType)
			if e != nil {
				return e
			}
			if !b && v != r.TunnelType {
				if tunnelBackendConfigured(r) {
					_, b, e = choose("Change tunnel backend?", []choice{{"change", "Change backend"}}, "SSH host/remote target and Kubernetes resource/context fields will be reset. Bind and ports are retained.")
					if e != nil {
						return e
					}
				}
				if !b {
					r.TunnelType = v
					r.SSHID = ""
					r.RemoteHost = ""
					r.RemoteHostID = ""
					r.Resource = ""
					r.Namespace = ""
					r.Context = ""
					if v == "ssh" {
						r.RemoteHost = "localhost"
					}
				}
			}
			continue
		case "account":
			choices := []choice{{"", "Custom — enter username/domain; client prompts for password"}}
			for _, a := range c.RDP.Accounts {
				choices = append(choices, choice{a.Name, a.Name})
			}
			v, b, e := chooseOne("RDP account", choices, "Saved accounts are local profiles. Custom values are retained when switching, but only the selected mode is used. A saved account without a domain uses the editable connection domain.", r.Account)
			if e != nil {
				return e
			}
			if !b {
				r.Account = v
			}
			continue
		case "save":
			if profile != nil {
				*profile = r
				return nil
			}
			if existing && h != nil && originalKind == "tunnel" {
				if e = ensureInventoryTunnelStopped(*h); e != nil {
					if e = notice(e.Error()); e != nil {
						return e
					}
					continue
				}
			}
			err = changeConnection(s, r, false)
			if err == nil {
				state.Mode = r.kind()
				if state.Mode == "tunnel" {
					state.Mode = "tunnels"
				}
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
			r.HostID = ""
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
			if h != nil && originalKind == "tunnel" {
				if e = ensureInventoryTunnelStopped(*h); e != nil {
					return e
				}
			}
			return changeConnection(s, r, true)
		case "jump", "ssh_id":
			records, e := sourceConnections(s)
			if e != nil {
				return e
			}
			hosts, e := sourceInventoryHosts(s)
			if e != nil {
				return e
			}
			hostNames := map[string]string{}
			for _, host := range hosts {
				hostNames[host.ID] = host.Alias
			}
			choices := []choice{{"", "None (direct)"}}
			if id == "ssh_id" {
				choices = nil
			}
			choices = append(choices, choice{"new-host", "Create new host with SSH…"})
			for _, j := range records {
				if j.ID != r.ID && j.kind() == "ssh" {
					choices = append(choices, choice{j.ID, hostNames[j.HostID] + " / " + j.ProfileName + " — " + j.Address})
				}
			}
			title := "Jump SSH profile in " + s.Name
			if id == "ssh_id" {
				title = "Tunnel SSH profile in " + s.Name
			}
			current := r.JumpID
			if id == "ssh_id" {
				current = r.SSHID
			}
			v, b, e := chooseOne(title, choices, "References stay pinned to this profile, even if the host's default changes.", current)
			if e != nil {
				return e
			}
			if !b {
				if v == "new-host" {
					next := PickerState{Source: s.ID, Mode: "ssh", ConfigPath: state.ConfigPath}
					if e = editInventoryHost(c, &next, nil); e != nil {
						return e
					}
					continue
				}
				if id == "ssh_id" {
					r.SSHID = v
				} else {
					r.JumpID = v
				}
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
			label, value = "Port", fmt.Sprint(r.Port)
		case "user":
			label, value = "Username (optional, supports ${NAME})", r.User
		case "key":
			label, value = "Identity file (optional, supports ${NAME})", r.IdentityFile
		case "tags":
			label, value = "Tags (comma-separated)", strings.Join(r.Tags, ", ")
		case "notes":
			label, value = "Notes", r.Notes
		case "domain":
			label = "RDP domain (empty = no domain)"
			if r.Domain != nil {
				value = *r.Domain
			}
		case "bind":
			label, value = "Loopback bind address", r.Bind
		case "local_port":
			label, value = "Local port", fmt.Sprint(r.LocalPort)
		case "remote_host":
			label, value = "Remote host (as reached by SSH host)", r.RemoteHost
		case "remote_port":
			label, value = "Remote port", fmt.Sprint(r.RemotePort)
		case "resource":
			label, value = "Kubernetes resource (e.g. service/web)", r.Resource
		case "namespace":
			label, value = "Kubernetes namespace", r.Namespace
		case "context":
			label, value = "Kubernetes context", r.Context
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
		case "domain":
			r.Domain = &value
		case "bind":
			r.Bind = value
		case "remote_host":
			r.RemoteHost = value
			r.RemoteHostID = ""
		case "resource":
			r.Resource = value
		case "namespace":
			r.Namespace = value
		case "context":
			r.Context = value
		case "port", "local_port", "remote_port":
			p, e := connectionPort(value)
			if e != nil {
				if e = notice(e.Error()); e != nil {
					return e
				}
			} else {
				switch id {
				case "local_port":
					r.LocalPort = p
				case "remote_port":
					r.RemotePort = p
				default:
					r.Port = p
				}
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

func tunnelBackendConfigured(r Connection) bool {
	return r.SSHID != "" || r.RemoteHostID != "" || (r.RemoteHost != "" && r.RemoteHost != "localhost") || r.Resource != "" || r.Namespace != "" || r.Context != ""
}

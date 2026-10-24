package main

import (
	"fmt"
	"maps"
	"os"
	"reflect"
	"sort"
	"strings"
)

var saveActiveSettings func() error
var settingsSaveState func() string

func agentNames(c Config) []string {
	names := make([]string, 0, len(c.SSHAgents))
	for name := range c.SSHAgents {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func chooseAgent(c Config, override bool, current string) (string, bool, error) {
	options := []choice{}
	if override {
		options = append(options, choice{"inherit", "Inherit connection's agent setting"})
	} else {
		options = append(options, choice{"", "Unspecified — system default"})
	}
	options = append(options, choice{"default", "System default — normal OpenSSH agent behavior"}, choice{"none", "Disabled — do not use an SSH agent"})
	for _, name := range agentNames(c) {
		options = append(options, choice{name, "Profile: " + name})
	}
	return chooseOne("SSH agent", options, "This selects a signing agent, not agent forwarding or legacy SSH algorithms. Definitions are local settings. Missing profiles/endpoints fail rather than falling back.", current)
}

func cloneAgentSettings(c Config) Config {
	c.LocalProfiles = maps.Clone(c.LocalProfiles)
	if c.PersonalConnections != nil {
		next := map[string]map[string]PersonalConnection{}
		for source, values := range c.PersonalConnections {
			next[source] = maps.Clone(values)
		}
		c.PersonalConnections = next
	}
	if c.RDP.Accounts != nil {
		c.RDP.Accounts = append([]RDPAccount{}, c.RDP.Accounts...)
	}
	if c.RDP.Session.Folders != nil {
		c.RDP.Session.Folders = append([]RDPFolder{}, c.RDP.Session.Folders...)
	}
	c.Identities = maps.Clone(c.Identities)
	if c.IdentityAssignments != nil {
		assignments := map[string]SourceIdentities{}
		for source, a := range c.IdentityAssignments {
			a.Profiles = maps.Clone(a.Profiles)
			a.Connections = maps.Clone(a.Connections)
			assignments[source] = a
		}
		c.IdentityAssignments = assignments
	}
	c.Hotkeys = maps.Clone(c.Hotkeys)
	if c.HostOverrides != nil {
		hosts := make(map[string]map[string]HostOverride, len(c.HostOverrides))
		for s, overrides := range c.HostOverrides {
			hosts[s] = maps.Clone(overrides)
		}
		c.HostOverrides = hosts
	}
	c.SSHAgents = maps.Clone(c.SSHAgents)
	if c.Overrides != nil {
		bySource := make(map[string]map[string]ConnectionOverride, len(c.Overrides))
		for s, records := range c.Overrides {
			bySource[s] = maps.Clone(records)
		}
		c.Overrides = bySource
	}
	return c
}

func settingsDraft(path string) (original []byte, before, draft Config, err error) {
	original, err = readSettingsData(path)
	if os.IsNotExist(err) {
		original = []byte{}
		before = defaultConfig()
		err = nil
	} else if err == nil {
		before, err = decodeConfig(original)
		before.databaseBacked = databaseSettings(path)
		before.settingsPath = path
	}
	if err != nil {
		return
	}
	draft = cloneAgentSettings(before)
	draft.settingsPath = path
	return
}

func settingsMenu(path string) (raw bool, err error) {
	return settingsMenuAt(path, "")
}

func settingsMenuAt(path, initial string) (raw bool, err error) {
	defer menuSession()()
	previousKeys := menuHotkeys
	if loaded, e := loadConfig(path); e == nil {
		menuHotkeys = loaded
	}
	defer func() { menuHotkeys = previousKeys }()
	original, before, draft, err := settingsDraft(path)
	if err != nil {
		return false, err
	}
	previousSave, previousState := saveActiveSettings, settingsSaveState
	defer func() { saveActiveSettings, settingsSaveState = previousSave, previousState }()
	saved := false
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
		saved = true
		return nil
	}
	settingsSaveState = func() string {
		if !reflect.DeepEqual(before, draft) {
			return "Unsaved changes"
		}
		if saved {
			return "Saved"
		}
		return "No unsaved changes"
	}
	if initial == "sources" {
		if err = editSourcePreferences(&draft); err != nil {
			return false, err
		}
	}
	for {
		mode := draft.StartupMode
		if mode == "" {
			mode = "ssh"
		}
		options := []choice{
			{"general", "Startup tab: " + startupTabLabel(mode)},
			{"agents", "SSH agents — local socket/pipe profiles"},
			{"identities", "Local identities — user/key/agent and bulk assignments"},
			{"policies", "Connection policies — shared SSH/RDP settings"},
			{"hotkeys", "Hotkeys — configurable action bindings"},
			{"sources", "Select sources — inventories loaded at launch"},
			{"overrides", "Connection settings & overrides — SSH agents / compatibility"},
			{"raw", "Edit raw settings (JSON with comments)"},
			{"save", "Save settings and return"}, {"cancel", "Cancel changes"},
		}
		if databaseSettings(path) {
			options = []choice{{"hosts", "Hosts"}, {"ssh-profiles", "SSH Profiles"}, {"identities", "SSH Identities"}, {"rdp-profiles", "RDP Profiles"}, {"rdp-identities", "RDP Identities"}, {"rdp-session", "RDP Session Settings"}, {"agents", "SSH Agents"}, {"sources", "Enabled Sources"}, {"general", "Startup tab"}, {"hotkeys", "Hotkeys"}, {"save", "Save settings and return"}, {"cancel", "Cancel changes"}}
		}
		values, details := settingsSummaries(draft)
		if databaseSettings(path) {
			for i, o := range options {
				if value, ok := values[o.ID]; ok {
					options[i].Label = o.Label + ": " + value
				}
			}
		}
		m := newDialog("Settings", options, "Personal settings stay in a draft until saved. Host and shared profile editors save inventory separately.")
		m.optionDetails = details
		id, back, e := runChoice(m, menuKey(m.title))
		if e != nil || back || id == "cancel" {
			return false, e
		}
		switch id {
		case "hosts":
			if e = manageInventoryHosts(draft, path); e != nil {
				return false, e
			}
			data, loaded, _, e := settingsDraft(path)
			if e != nil {
				return false, e
			}
			original = data
			before.PersonalConnections = loaded.PersonalConnections
			draft.PersonalConnections = loaded.PersonalConnections
			before.RDP.Session = loaded.RDP.Session
			draft.RDP.Session = loaded.RDP.Session
		case "ssh-profiles", "rdp-profiles":
			if e = manageConnectionProfiles(draft, path, strings.Split(id, "-")[0]); e != nil {
				return false, e
			}
			data, loaded, _, e := settingsDraft(path)
			if e != nil {
				return false, e
			}
			original = data
			before.LocalProfiles = loaded.LocalProfiles
			draft.LocalProfiles = loaded.LocalProfiles
			before.IdentityAssignments = loaded.IdentityAssignments
			draft.IdentityAssignments = loaded.IdentityAssignments
			before.PersonalConnections = loaded.PersonalConnections
			// Profile management saves independently; keep other settings drafts.
			draft.PersonalConnections = loaded.PersonalConnections
		case "rdp-identities":
			if e = editRDPIdentities(&draft); e != nil {
				return false, e
			}
			data, loaded, _, e := settingsDraft(path)
			if e != nil {
				return false, e
			}
			original = data
			before.CredentialKeyID = loaded.CredentialKeyID
			draft.CredentialKeyID = loaded.CredentialKeyID
			before.RDP.Accounts = loaded.RDP.Accounts
			before.PersonalConnections = loaded.PersonalConnections
			before.Overrides = loaded.Overrides
		case "rdp-session":
			if e = editRDPSessionSettings(&draft); e != nil {
				return false, e
			}
			data, loaded, _, e := settingsDraft(path)
			if e != nil {
				return false, e
			}
			original = data
			before.RDP.Session = loaded.RDP.Session
			before.RDP.Client = loaded.RDP.Client
			before.RDP.ClientArgs = loaded.RDP.ClientArgs
		case "policies":
			if e = editSourcePolicies(draft, path); e != nil {
				return false, e
			}
		case "identities":
			if e = editLocalIdentities(&draft); e != nil {
				return false, e
			}
			data, loaded, _, e := settingsDraft(path)
			if e != nil {
				return false, e
			}
			original = data
			before.Identities = loaded.Identities
			before.IdentityAssignments = loaded.IdentityAssignments
			before.PersonalConnections = loaded.PersonalConnections
		case "sources":
			if e = editSourcePreferences(&draft); e != nil {
				return false, e
			}
			data, loaded, _, e := settingsDraft(path)
			if e != nil {
				return false, e
			}
			original = data
			before.DisabledSources = loaded.DisabledSources
			before.StartupSource = loaded.StartupSource
		case "hotkeys":
			if e = editHotkeys(&draft); e != nil {
				return false, e
			}
			data, loaded, _, e := settingsDraft(path)
			if e != nil {
				return false, e
			}
			original = data
			before.Hotkeys = loaded.Hotkeys
		case "general":
			m := newDialog("Startup tab", []choice{{"ssh", "SSH"}, {"rdp", "RDP"}, {"tunnels", "Tunnels"}, {"transfers", "Transfers"}}, "Choose the tab shown on startup. An explicit command such as go-where rdp takes precedence.")
			m.singleChoice = true
			m.current = mode
			m.footerActions = []choice{{"select", "Select"}}
			for i, o := range m.options {
				if o.ID == mode {
					m.cursor = i
					break
				}
			}
			v, b, e := runChoice(m, menuKey(m.title))
			if e != nil {
				return false, e
			}
			if !b {
				draft.StartupMode = v
			}
		case "agents":
			if e = editAgentProfiles(&draft); e != nil {
				if e = notice(e.Error()); e != nil {
					return false, e
				}
			}
			data, loaded, _, e := settingsDraft(path)
			if e != nil {
				return false, e
			}
			original = data
			before.SSHAgents = loaded.SSHAgents
			before.Identities = loaded.Identities
			before.LocalProfiles = loaded.LocalProfiles
			before.Overrides = loaded.Overrides
		case "overrides":
			if e = editConnectionOverrides(&draft); e != nil {
				if e = notice(e.Error()); e != nil {
					return false, e
				}
			}
		case "raw":
			if !reflect.DeepEqual(before, draft) {
				_, b, e := choose("Discard draft and open raw settings?", []choice{{"discard", "Discard draft and open raw editor"}}, "Unsaved changes will be discarded.")
				if e != nil {
					return false, e
				}
				if b {
					continue
				}
			}
			return true, nil
		case "save":
			if e = saveActiveSettings(); e != nil {
				if e = notice(e.Error()); e != nil {
					return false, e
				}
				continue
			}
			return false, nil
		}
	}
}

func editAgentProfiles(c *Config) error {
	for {
		options := []choice{{"add", "Add SSH agent profile…"}}
		for _, name := range agentNames(*c) {
			p := c.SSHAgents[name]
			endpoint := p.Socket
			if p.SocketEnv != "" {
				endpoint = "environment: " + p.SocketEnv
			}
			options = append(options, choice{"profile:" + name, name + " — " + endpoint})
		}
		m := newDialog("SSH Agents", options, "Device-local signing endpoints, not agent forwarding. Select an agent to edit.")
		m.footerActions = []choice{{"select", "Edit"}, {"delete", "Delete"}}
		m.optionDetails = map[string]string{"add": "Create a named local signing-agent endpoint. No key material is stored here."}
		for _, name := range agentNames(*c) {
			p := c.SSHAgents[name]
			m.optionDetails["profile:"+name] = "Agent: " + name + "\nEnvironment variable: " + p.SocketEnv + "\nSocket / pipe: " + p.Socket + "\n\nResolved at launch. Missing endpoints fail rather than falling back. This does not enable agent forwarding."
		}
		id, back, err := runChoice(m, menuKey(m.title))
		if err != nil || back {
			return err
		}
		if strings.HasPrefix(id, "delete:") {
			name := strings.TrimPrefix(strings.TrimPrefix(id, "delete:"), "profile:")
			if _, ok := c.SSHAgents[name]; !ok {
				continue
			}
			uses, e := agentReferences(*c, name, "")
			if e != nil {
				return e
			}
			if len(uses) > 0 {
				if e = notice("Agent is used by:\n" + strings.Join(uses, "\n") + "\nReassign these before deleting."); e != nil {
					return e
				}
				continue
			}
			yes, e := confirmSettingsDelete(name, "Local settings")
			if e != nil {
				return e
			}
			if yes {
				next := cloneAgentSettings(*c)
				if next.Overrides == nil {
					next.Overrides = map[string]map[string]ConnectionOverride{}
				}
				delete(next.SSHAgents, name)
				if e = saveAgentChanges(c, next); e != nil {
					return e
				}
			}
			continue
		}
		name := strings.TrimPrefix(id, "profile:")
		creating := id == "add"
		if creating {
			var b bool
			name, b, err = inventoryPrompt("Agent profile name", "legacy")
			if err != nil {
				return err
			}
			if b {
				continue
			}
			if _, ok := c.SSHAgents[name]; ok {
				if err = notice("That profile already exists."); err != nil {
					return err
				}
				continue
			}
		}
		p := c.SSHAgents[name]
		originalName := name
		if creating {
			p = SSHAgentProfile{SocketEnv: "SSH_AUTH_SOCK_LEGACY"}
		}
		for {
			kind, value := "Environment variable", p.SocketEnv
			if p.SocketEnv == "" {
				kind, value = "Socket / Windows pipe", p.Socket
			}
			actions := []choice{{"name", "Name: " + name}, {"kind", "Endpoint type: " + kind}, {"value", "Endpoint: " + value}}
			action, b, e := objectEditor("SSH Agent — "+name, actions, "Save commits only this agent and its renamed references. Environment variables resolve at launch; literal socket/pipe paths are not shell-expanded. No key material is stored.")
			if e != nil {
				return e
			}
			if b {
				break
			}
			switch action {
			case "name":
				v, b, e := inventoryPrompt("Agent name", name)
				if e != nil {
					return e
				}
				if !b {
					name = v
				}
			case "kind":
				current := "env"
				if p.SocketEnv == "" {
					current = "socket"
				}
				v, b, e := chooseOne("Endpoint type", []choice{{"env", "Environment variable"}, {"socket", "Literal socket / Windows pipe"}}, "", current)
				if e != nil {
					return e
				}
				if b {
					continue
				}
				if v == "env" && p.SocketEnv == "" {
					p = SSHAgentProfile{SocketEnv: "SSH_AUTH_SOCK_LEGACY"}
				} else if v == "socket" && p.Socket == "" {
					p = SSHAgentProfile{Socket: "~/agent.sock"}
				}
			case "value":
				v, b, e := inventoryPrompt(kind, value)
				if e != nil {
					return e
				}
				if !b {
					if p.SocketEnv != "" {
						p.SocketEnv = v
					} else {
						p.Socket = v
					}
				}
			case "delete":
				_, b, e := choose("Remove agent profile?", []choice{{"delete", "Remove profile"}}, "References will fail clearly until changed or removed; they will NOT fall back to another agent. Settings Save is still required.")
				if e != nil {
					return e
				}
				if b {
					continue
				}
				delete(c.SSHAgents, name)
				action = "done"
			case "save":
				if e = validateAgentProfile(name, p); e != nil {
					if e = notice(e.Error()); e != nil {
						return e
					}
					continue
				}
				next := cloneAgentSettings(*c)
				if next.SSHAgents == nil {
					next.SSHAgents = map[string]SSHAgentProfile{}
				}
				if name != originalName {
					if _, ok := next.SSHAgents[name]; ok {
						if e = notice("Agent name already exists."); e != nil {
							return e
						}
						continue
					}
					if _, e = agentReferences(next, originalName, name); e != nil {
						return e
					}
					delete(next.SSHAgents, originalName)
				}
				next.SSHAgents[name] = p
				if e = saveAgentChanges(c, next); e != nil {
					return e
				}
				action = "done"
			}
			if action == "done" {
				break
			}
		}
	}
}

func editAgentOverrideDraft(c *Config, h Host) error {
	s, r, err := storedRecord(h, *c)
	if err != nil {
		return err
	}
	if r.kind() != "ssh" {
		return fmt.Errorf("agent overrides belong to SSH records, including jump hosts referenced by RDP/tunnels")
	}
	current := "inherit"
	if o := c.Overrides[s.ID][r.ID]; o.SSHAgent != nil {
		current = *o.SSHAgent
	}
	id, back, err := chooseAgent(*c, true, current)
	if err != nil || back {
		return err
	}
	if c.Overrides == nil {
		c.Overrides = map[string]map[string]ConnectionOverride{}
	}
	if c.Overrides[s.ID] == nil {
		c.Overrides[s.ID] = map[string]ConnectionOverride{}
	}
	o := c.Overrides[s.ID][r.ID]
	if id == "inherit" {
		o.SSHAgent = nil
	} else {
		o.SSHAgent = &id
	}
	c.Overrides[s.ID][r.ID] = o
	return nil
}

func editConnectionOverrides(c *Config) error {
	hosts, err := inventoryHosts(*c)
	if err != nil {
		return err
	}
	var records []Host
	var options []choice
	for _, h := range hosts {
		if h.Type == "ssh" || h.Type == "" {
			records = append(records, h)
			options = append(options, choice{hostIdentity(h), h.Alias + " [" + h.SourceName + "]"})
		}
	}
	if len(options) == 0 {
		return notice("Create an SSH connection in a SQLite source first.")
	}
	id, back, err := choose("Local connection overrides", options, "Agent and compatibility choices belong to SSH records. For RDP or tunnels, override their referenced SSH jump/connection.")
	if err != nil || back {
		return err
	}
	for _, h := range records {
		if hostIdentity(h) == id {
			kind, b, e := choose("Override — "+h.Alias, []choice{{"agent", "SSH agent"}, {"compatibility", "SSH compatibility (identities / additional KEX)"}}, "")
			if e != nil || b {
				return e
			}
			if kind == "compatibility" {
				return editCompatibilityOverrideDraft(c, h)
			}
			return editAgentOverrideDraft(c, h)
		}
	}
	return nil
}

func editConnectionAgentOverride(path string, h Host) error {
	return editConnectionOverride(path, h, editAgentOverrideDraft)
}

func editConnectionOverride(path string, h Host, edit func(*Config, Host) error) error {
	defer menuSession()()
	original, before, draft, err := settingsDraft(path)
	if err != nil {
		return err
	}
	if err = edit(&draft, h); err != nil {
		return err
	}
	if reflect.DeepEqual(before, draft) {
		return nil
	}
	_, back, err := choose("Save local override separately?", []choice{{"save", "Save local settings override"}}, "This saves ONLY local settings, independently of connection edits. Cancelling the connection editor later will not undo this explicit save.")
	if err != nil || back {
		return err
	}
	return saveSettingsDraft(path, original, before, draft)
}

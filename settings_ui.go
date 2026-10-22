package main

import (
	"fmt"
	"maps"
	"os"
	"reflect"
	"sort"
	"strings"
)

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
	original, err = os.ReadFile(path)
	if os.IsNotExist(err) {
		original = []byte{}
		before = defaultConfig()
		err = nil
	} else if err == nil {
		before, err = decodeConfig(original)
	}
	if err != nil {
		return
	}
	draft = cloneAgentSettings(before)
	return
}

func settingsMenu(path string) (raw bool, err error) {
	defer menuSession()()
	original, before, draft, err := settingsDraft(path)
	if err != nil {
		return false, err
	}
	for {
		mode := draft.StartupMode
		if mode == "" {
			mode = "ssh"
		}
		id, back, e := choose("Settings", []choice{
			{"general", "General — startup mode: " + mode},
			{"agents", "SSH agents — local socket/pipe profiles"},
			{"overrides", "Connection settings & overrides — SSH agents / compatibility"},
			{"raw", "Edit raw settings (JSON with comments)"},
			{"save", "Save settings and return"}, {"cancel", "Cancel changes"},
		}, "Changes stay in a draft until you save.\n\n• Save applies your changes.\n• Cancel leaves settings unchanged.\n• Raw editing discards the current draft.\n\nJSONC comments and trailing commas are supported.\n\nAccount management and configurable hotkeys are not available yet.")
		if e != nil || back || id == "cancel" {
			return false, e
		}
		switch id {
		case "general":
			v, b, e := chooseOne("Startup mode", []choice{{"ssh", "SSH"}, {"rdp", "RDP"}, {"tunnels", "Tunnels"}, {"scp", "Transfers — SCP"}, {"rsync", "Transfers — rsync"}, {"legacy-scp", "Transfers — legacy SCP"}}, "An explicit CLI mode, e.g. go-where rdp, wins over this preference.", mode)
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
			if e = saveSettingsDraft(path, original, before, draft); e != nil {
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
		id, back, err := choose("SSH agent profiles", options, "System default uses OpenSSH's normal agent discovery (SSH_AUTH_SOCK on Linux). Windows endpoints depend on the installed OpenSSH client. Named profiles are never exported to shared inventory.")
		if err != nil || back {
			return err
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
		if creating {
			p = SSHAgentProfile{SocketEnv: "SSH_AUTH_SOCK_LEGACY"}
		}
		for {
			kind, value := "Environment variable", p.SocketEnv
			if p.SocketEnv == "" {
				kind, value = "Socket / Windows pipe", p.Socket
			}
			actions := []choice{{"kind", "Endpoint type: " + kind}, {"value", "Endpoint: " + value}, {"save", "Apply profile to settings draft"}}
			if !creating {
				actions = append(actions, choice{"delete", "Remove profile from settings draft…"})
			}
			action, b, e := choose("Agent profile — "+name, actions, "Only endpoint definitions are stored, not key material. No environment or shell expansion is performed on literal endpoints. Socket environment variables are resolved at launch.")
			if e != nil {
				return e
			}
			if b {
				break
			}
			switch action {
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
				if c.SSHAgents == nil {
					c.SSHAgents = map[string]SSHAgentProfile{}
				}
				c.SSHAgents[name] = p
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

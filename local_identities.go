package main

import (
	"fmt"
	"maps"
	"sort"
	"strings"
)

type LocalIdentity struct {
	User         string `json:"user,omitempty"`
	IdentityFile string `json:"identity_file,omitempty"`
	SSHAgent     string `json:"ssh_agent,omitempty"`
}
type SourceIdentities struct {
	Default     string            `json:"default,omitempty"`
	Profiles    map[string]string `json:"profiles,omitempty"`
	Connections map[string]string `json:"connections,omitempty"`
}

func identityChoice(c Config, source string, r Connection) (string, string) {
	a := c.IdentityAssignments[source]
	if name, ok := a.Connections[r.ID]; ok {
		return name, "connection identity"
	}
	if name, ok := a.Profiles[r.ProfileID]; ok {
		return name, "shared-profile identity"
	}
	if a.Default != "" {
		return a.Default, "source identity"
	}
	return "", "existing connection defaults"
}

func applyLocalIdentity(c Config, source string, r Connection) (Connection, error) {
	if r.kind() != "ssh" {
		return r, nil
	}
	name, _ := identityChoice(c, source, r)
	if name == "" {
		return r, nil
	}
	p := LocalIdentity{}
	if name != "default" {
		var ok bool
		p, ok = c.Identities[name]
		if !ok {
			return r, fmt.Errorf("local identity %q is not configured", name)
		}
	}
	r.User, r.IdentityFile, r.SSHAgent = p.User, p.IdentityFile, p.SSHAgent
	return r, nil
}

func validateIdentities(c Config) error {
	for name, p := range c.Identities {
		if !inventoryName(name) || name == "default" || name == "inherit" {
			return fmt.Errorf("invalid/reserved identity name %q", name)
		}
		if clean(p.User) != p.User || clean(p.IdentityFile) != p.IdentityFile || !validAgentChoice(p.SSHAgent) {
			return fmt.Errorf("invalid local identity %q", name)
		}
	}
	for _, a := range c.IdentityAssignments {
		names := []string{a.Default}
		for _, name := range a.Profiles {
			names = append(names, name)
		}
		for _, name := range a.Connections {
			names = append(names, name)
		}
		for _, name := range names {
			if name == "" || name == "default" {
				continue
			}
			if _, ok := c.Identities[name]; !ok {
				return fmt.Errorf("assigned local identity %q is missing", name)
			}
		}
	}
	return nil
}

func chooseIdentity(c Config, current string) (string, bool, error) {
	options := []choice{{"inherit", "Inherit / remove assignment"}, {"default", "OpenSSH defaults — OS user and normal agent"}}
	names := []string{}
	for name := range c.Identities {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		options = append(options, choice{name, name})
	}
	return chooseOne("Local identity", options, "Identities are device-local and reusable across sources. Individual field overrides take precedence. No keys or passwords are stored here.", current)
}

func editLocalIdentities(c *Config) error {
	for {
		options := []choice{{"add", "Create local identity…"}, {"assign", "Assign identity to source/profiles/connections…"}}
		if c.databaseBacked {
			options = options[:1]
		}
		names := []string{}
		for name := range c.Identities {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			options = append(options, choice{"identity:" + name, name + ": " + c.Identities[name].User})
		}
		m := newDialog("SSH Identities", options, "Device-local identities. Delete removes an unused identity from the draft; Settings Save persists it. Referenced identities cannot be deleted.")
		m.footerActions = []choice{{"select", "Edit"}, {"delete", "Delete"}}
		id, back, err := runChoice(m, menuKey(m.title))
		if err != nil || back {
			return err
		}
		if strings.HasPrefix(id, "delete:") {
			name := strings.TrimPrefix(strings.TrimPrefix(id, "delete:"), "identity:")
			if _, ok := c.Identities[name]; !ok {
				continue
			}
			if identityReferenced(*c, "ssh", name) {
				if err = notice("Identity is referenced; remove or change its assignments first."); err != nil {
					return err
				}
				continue
			}
			yes, e := confirmSettingsDelete(name, "Local settings draft")
			if e != nil {
				return e
			}
			if yes {
				delete(c.Identities, name)
			}
			continue
		}
		if id == "assign" {
			if err = assignIdentity(c); err != nil {
				return err
			}
			continue
		}
		name := strings.TrimPrefix(id, "identity:")
		if id == "add" {
			var b bool
			name, b, err = inventoryPrompt("Identity name", "")
			if err != nil {
				return err
			}
			if b {
				continue
			}
			if !inventoryName(name) || name == "default" || name == "inherit" {
				if err = notice("Use a non-reserved identity name."); err != nil {
					return err
				}
				continue
			}
			if _, ok := c.Identities[name]; ok {
				if err = notice("Identity already exists."); err != nil {
					return err
				}
				continue
			}
		}
		p := c.Identities[name]
		originalName := name
		for {
			fields := []choice{{"name", "Name: " + name}, {"user", "Username: " + p.User}, {"key", "Identity file: " + p.IdentityFile}}
			if !c.databaseBacked {
				fields = append(fields, choice{"agent", "SSH agent: " + agentChoiceLabel(p.SSHAgent)})
			}
			action, b, e := objectEditor("SSH Identity — "+name, fields, "Save commits this identity and its renamed references only. Other Settings changes stay unsaved. Existing agent values are preserved.")
			if e != nil {
				return e
			}
			if b {
				break
			}
			if action == "save" || action == "apply" {
				if e = saveIdentityEditor(c, "ssh", originalName, name, p, RDPAccount{}); e != nil {
					if e = notice(e.Error()); e != nil {
						return e
					}
					continue
				}
				break
			}
			if action == "agent" {
				v, b, e := chooseAgent(*c, false, p.SSHAgent)
				if e != nil {
					return e
				}
				if !b {
					p.SSHAgent = v
				}
				continue
			}
			value := p.User
			if action == "name" {
				value = name
			}
			if action == "key" {
				value = p.IdentityFile
			}
			v, b, e := inventoryPrompt(action, value)
			if e != nil {
				return e
			}
			if !b {
				if action == "name" {
					name = v
				} else if action == "key" {
					p.IdentityFile = v
				} else {
					p.User = v
				}
			}
		}
	}
}

func assignIdentity(c *Config) error {
	s, b, err := writableSource(*c, "")
	if err != nil || b {
		return err
	}
	scope, b, err := choose("Identity assignment", []choice{{"source", "Entire source"}, {"profiles", "Shared SSH profiles"}, {"connections", "Selected SSH connections (bulk)"}}, "Individual connection assignments override profile and source assignments.")
	if err != nil || b {
		return err
	}
	name, b, err := chooseIdentity(*c, "inherit")
	if err != nil || b {
		return err
	}
	a := c.IdentityAssignments[s.ID]
	a.Profiles = maps.Clone(a.Profiles)
	a.Connections = maps.Clone(a.Connections)
	if scope == "source" {
		if name == "inherit" {
			a.Default = ""
		} else {
			a.Default = name
		}
	} else {
		rows, err := sourceConnections(s)
		if err != nil {
			return err
		}
		items := []multiItem{}
		seen := map[string]bool{}
		for _, r := range rows {
			if r.kind() != "ssh" {
				continue
			}
			id, label := r.ID, r.Alias+" / "+r.ProfileName
			if scope == "profiles" {
				id, label = r.ProfileID, r.ProfileName
			}
			if seen[id] {
				continue
			}
			seen[id] = true
			items = append(items, multiItem{ID: id, Label: label})
		}
		m := newMultiDialog("Assign identity", items, "Check every connection/profile to assign. Esc discards selection; Done applies to local Settings draft.")
		if scope == "connections" {
			m.hostRows = map[string]Host{}
			hosts, e := sourceInventoryHosts(s)
			if e != nil {
				return e
			}
			byID := map[string]InventoryHost{}
			for _, h := range hosts {
				byID[h.ID] = h
			}
			for _, r := range rows {
				h := byID[r.HostID]
				m.hostRows[r.ID] = Host{Alias: h.Alias, Hostname: h.Address, Tags: h.Tags, Comment: h.Notes, SourceID: s.ID, SourceName: s.Name, Color: h.Color}
			}
			m.text = "Assign identity: " + name + "\n\nTab marks and moves up; Shift+Tab marks and moves down. Marks survive filtering. Drag column separators to resize.\n\nDone updates local settings only; shared inventory is untouched."
		}
		m.allowManual = false
		if _, err = runTUI(m); err != nil {
			return err
		}
		if m.back {
			return nil
		}
		target := a.Connections
		if scope == "profiles" {
			target = a.Profiles
		}
		if target == nil {
			target = map[string]string{}
		}
		for _, item := range m.items {
			if !item.Checked {
				continue
			}
			if name == "inherit" {
				delete(target, item.ID)
			} else {
				target[item.ID] = name
			}
		}
		if scope == "profiles" {
			a.Profiles = target
		} else {
			a.Connections = target
		}
	}
	if c.IdentityAssignments == nil {
		c.IdentityAssignments = map[string]SourceIdentities{}
	}
	c.IdentityAssignments[s.ID] = a
	return nil
}

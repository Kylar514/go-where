package main

import (
	"fmt"
	"maps"
)

func defaultConnectionIndex(profiles []Connection, kind string) int {
	for i, p := range profiles {
		if p.kind() == kind && p.Default {
			return i
		}
	}
	return -1
}

func directConnectionRows(c Config, s InventorySource, profiles []Connection) []choice {
	var rows []choice
	for _, kind := range []string{"ssh", "rdp"} {
		label := "SSH"
		if kind == "rdp" {
			label = "RDP"
		}
		i := defaultConnectionIndex(profiles, kind)
		name, identity, jump := "Not configured", "Not configured", "Direct"
		if i >= 0 {
			p := profiles[i]
			name = p.ProfileName
			jump = sshReferenceLabel(c, s, p.JumpID)
			if p.JumpID == "" {
				jump = "Direct"
			}
			if kind == "ssh" {
				identity, _ = identityChoice(c, s.ID, p)
				if identity == "" {
					identity = "Existing / OpenSSH defaults"
				}
			} else {
				identity = "Client prompt"
				if o := c.Overrides[s.ID][p.ID]; o.Account != nil {
					identity = *o.Account
				} else if p.Account != "" {
					identity = p.Account
				}
			}
		}
		rows = append(rows, choice{kind + "-profile", label + " connection: " + name}, choice{kind + "-identity", label + " identity: " + identity}, choice{kind + "-jump", label + " jump host: " + jump})
	}
	return rows
}

func chooseConnectionProfile(s InventorySource, kind, current string) (Connection, bool, error) {
	profiles, err := sourceProfiles(s)
	if err != nil {
		return Connection{}, false, err
	}
	options := []choice{{"none", "None — disable connection"}, {"new", "Create new shared profile…"}}
	for _, p := range profiles {
		if p.kind() == kind {
			options = append(options, choice{p.ID, p.ProfileName})
		}
	}
	id, back, err := chooseOne("Connection profile", options, "Shared port and protocol/compatibility settings. Host routing and local identity are separate.", current)
	if err != nil || back {
		return Connection{}, back, err
	}
	if id == "none" {
		return Connection{}, false, nil
	}
	if id == "new" {
		name, b, e := inventoryPrompt("Profile name", "")
		if e != nil || b {
			return Connection{}, b, e
		}
		p := Connection{ID: inventoryID(), Type: kind, ProfileName: name, Port: 22}
		if kind == "rdp" {
			p.Port = 3389
		}
		return p, false, nil
	}
	for _, p := range profiles {
		if p.ID == id {
			return p, false, nil
		}
	}
	return Connection{}, false, fmt.Errorf("profile no longer exists")
}

func chooseJump(c Config, s InventorySource, current, exclude string) (string, bool, error) {
	rows, err := sourceConnections(s)
	if err != nil {
		return "", false, err
	}
	options := []choice{{"", "Direct (no jump)"}}
	for _, p := range rows {
		if p.kind() == "ssh" && p.ID != exclude {
			options = append(options, choice{p.ID, sshReferenceLabel(c, s, p.ID)})
		}
	}
	return chooseOne("Jump host", options, "Route belongs to this host connection, not the shared profile.", current)
}

func assignConnectionIdentity(c *Config, s InventorySource, p Connection) error {
	if p.kind() == "ssh" {
		current, _ := identityChoice(*c, s.ID, p)
		name, back, err := chooseIdentity(*c, current)
		if err != nil || back {
			return err
		}
		a := c.IdentityAssignments[s.ID]
		a.Connections = maps.Clone(a.Connections)
		if a.Connections == nil {
			a.Connections = map[string]string{}
		}
		if name == "inherit" {
			delete(a.Connections, p.ID)
		} else {
			a.Connections[p.ID] = name
		}
		if c.IdentityAssignments == nil {
			c.IdentityAssignments = map[string]SourceIdentities{}
		}
		c.IdentityAssignments[s.ID] = a
		return nil
	}
	options := []choice{{"inherit", "Inherit existing identity"}, {"", "Client password prompt"}}
	for _, a := range c.RDP.Accounts {
		options = append(options, choice{a.Name, a.Name})
	}
	name, back, err := chooseOne("Local RDP identity", options, "Local username/domain/credential provider. Shared profile domain is used unless this identity overrides it. Password storage is not added here.", p.Account)
	if err != nil || back {
		return err
	}
	o := c.Overrides[s.ID][p.ID]
	if name == "inherit" {
		o.Account = nil
	} else {
		o.Account = &name
	}
	if c.Overrides == nil {
		c.Overrides = map[string]map[string]ConnectionOverride{}
	}
	if c.Overrides[s.ID] == nil {
		c.Overrides[s.ID] = map[string]ConnectionOverride{}
	}
	c.Overrides[s.ID][p.ID] = o
	return nil
}

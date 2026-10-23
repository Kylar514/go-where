package main

import (
	"fmt"
	"strings"
)

func manageInventoryHosts(c Config, path string) error {
	for {
		options := []choice{{"new", "New host…"}}
		byID := map[string]Host{}
		for _, s := range c.Sources {
			if !sourceEnabled(c, s.ID) {
				continue
			}
			hosts, err := sourceInventoryHosts(s)
			if err != nil {
				return err
			}
			for _, h := range hosts {
				key := s.ID + ":" + h.ID
				byID[key] = Host{SourceID: s.ID, SourceName: s.Name, InventoryHostID: h.ID, Alias: h.Alias, Hostname: h.Address}
				options = append(options, choice{key, h.Alias + " — " + s.Name})
			}
		}
		id, back, err := choose("Hosts", options, "All inventory hosts, including hosts with no SSH or RDP connection.")
		if err != nil || back {
			return err
		}
		state := PickerState{ConfigPath: path}
		var h *Host
		if id != "new" {
			value := byID[id]
			h = &value
		}
		if h != nil && databaseSettings(path) {
			action, b, e := choose("Host — "+h.Alias, []choice{{"personal", "Connection profiles and identities"}, {"shared", "Edit inventory host"}}, "Personal selections save locally. Inventory editing changes the source.")
			if e != nil {
				return e
			}
			if b {
				continue
			}
			if action == "personal" {
				if e = editPersonalHosts(c, path, []Host{*h}); e != nil {
					return e
				}
				continue
			}
		}
		if err = editInventoryHost(c, &state, h); err != nil {
			return err
		}
	}
}

func manageConnectionProfiles(c Config, path, kind string) error {
	for {
		options := []choice{{"new", "New profile…"}}
		type entry struct {
			s InventorySource
			p Connection
		}
		entries := map[string]entry{}
		for id, p := range c.LocalProfiles {
			if p.kind() != kind {
				continue
			}
			key := "local:" + id
			entries[key] = entry{InventorySource{ID: "local", Name: "Local"}, p}
			options = append(options, choice{key, p.ProfileName + " — Local"})
		}
		for _, s := range c.Sources {
			if !sourceEnabled(c, s.ID) {
				continue
			}
			profiles, err := sourceProfiles(s)
			if err != nil {
				return err
			}
			for _, p := range profiles {
				if p.kind() != kind {
					continue
				}
				key := s.ID + ":" + p.ID
				entries[key] = entry{s, p}
				options = append(options, choice{key, p.ProfileName + " — " + s.Name})
			}
		}
		id, back, err := choose(strings.ToUpper(kind)+" Profiles", options, "Connection settings from all enabled sources. Editing a shared profile affects every host using it.")
		if err != nil || back {
			return err
		}
		var s InventorySource
		var p Connection
		if id == "new" {
			if databaseSettings(path) {
				s = InventorySource{ID: "local", Name: "Local"}
			} else {
				s, back, err = writableSource(c, "", path)
				if err != nil || back {
					return err
				}
			}
			name, b, e := inventoryPrompt("Profile name", "")
			if e != nil {
				return e
			}
			if b {
				continue
			}
			p = Connection{ID: inventoryID(), ProfileName: name, Type: kind, Port: 22}
			if kind == "rdp" {
				p.Port = 3389
			}
		} else {
			s = entries[id].s
			p = entries[id].p
		}
		for {
			options := []choice{{"name", "Name: " + p.ProfileName}, {"port", fmt.Sprintf("Port: %d", p.Port)}}
			if kind == "ssh" {
				options = append(options, choice{"agent", "Agent: " + agentChoiceLabel(p.SSHAgent)}, choice{"compatibility", "Compatibility…"})
			} else {
				domain := ""
				if p.Domain != nil {
					domain = *p.Domain
				}
				options = append(options, choice{"domain", "Domain: " + domain})
			}
			options = append(options, choice{"save", "Save profile"})
			action, b, e := choose(strings.ToUpper(kind)+" Profile — "+p.ProfileName, options, "Saved to "+s.Name+". Existing legacy login settings are preserved; identities are managed separately.")
			if e != nil {
				return e
			}
			if b {
				break
			}
			switch action {
			case "agent":
				v, b, e := chooseAgent(c, false, p.SSHAgent)
				if e != nil {
					return e
				}
				if !b {
					p.SSHAgent = v
				}
			case "compatibility":
				if e = editSSHCompatibility(&p); e != nil {
					return e
				}
			case "save":
				if s.ReadOnly {
					if e = notice("This source is read-only. Create a local profile instead."); e != nil {
						return e
					}
					continue
				}
				if s.ID == "local" {
					original, before, draft, e := settingsDraft(path)
					if e != nil {
						return e
					}
					if draft.LocalProfiles == nil {
						draft.LocalProfiles = map[string]Connection{}
					}
					draft.LocalProfiles[p.ID] = p
					if e = saveSettingsDraft(path, original, before, draft); e != nil {
						return e
					}
					c = draft
					break
				}
				if e = saveReusableProfile(s, p); e != nil {
					return e
				}
			default:
				value := p.ProfileName
				if action == "port" {
					value = fmt.Sprint(p.Port)
				}
				if action == "domain" {
					value = ""
					if p.Domain != nil {
						value = *p.Domain
					}
				}
				v, b, e := inventoryPrompt(action, value)
				if e != nil {
					return e
				}
				if b {
					continue
				}
				switch action {
				case "name":
					p.ProfileName = v
				case "domain":
					p.Domain = &v
				case "port":
					port, e := connectionPort(v)
					if e != nil {
						if e = notice(e.Error()); e != nil {
							return e
						}
						continue
					}
					p.Port = port
				}
			}
		}
	}
}

func editRDPIdentities(c *Config) error {
	for {
		options := []choice{{"new", "New identity…"}}
		for _, a := range c.RDP.Accounts {
			options = append(options, choice{a.Name, a.Name})
		}
		id, back, err := choose("RDP Identities", options, "Device-local usernames and existing credential providers. New password storage is deferred.")
		if err != nil || back {
			return err
		}
		index := -1
		a := RDPAccount{}
		for i, v := range c.RDP.Accounts {
			if v.Name == id {
				index = i
				a = v
			}
		}
		if id == "new" {
			name, b, e := inventoryPrompt("Identity name", "")
			if e != nil {
				return e
			}
			if b {
				continue
			}
			a.Name = name
		}
		for {
			action, b, e := choose("RDP Identity — "+a.Name, []choice{{"user", "Username: " + a.User}, {"apply", "Apply"}}, "Domain belongs to the connection profile. Existing identity domain and credential-provider settings are preserved during migration.")
			if e != nil {
				return e
			}
			if b {
				break
			}
			if action == "apply" {
				next := append([]RDPAccount(nil), c.RDP.Accounts...)
				if index < 0 {
					next = append(next, a)
				} else {
					next[index] = a
				}
				r := c.RDP
				r.Accounts = next
				if e = r.validate(); e != nil {
					if e = notice(e.Error()); e != nil {
						return e
					}
					continue
				}
				c.RDP = r
				break
			}
			v, b, e := inventoryPrompt("Username", a.User)
			if e != nil {
				return e
			}
			if !b {
				a.User = v
			}
		}
	}
}

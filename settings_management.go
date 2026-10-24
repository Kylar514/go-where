package main

import (
	"fmt"
	"github.com/charmbracelet/bubbles/textinput"
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
				byID[key] = Host{SourceID: s.ID, SourceName: s.Name, InventoryHostID: h.ID, Alias: h.Alias, Hostname: h.Address, Tags: h.Tags, Comment: h.Notes}
				options = append(options, choice{key, h.Alias + " — " + s.Name})
			}
		}
		m := newDialog("Hosts", options, "")
		m.footerActions = []choice{{"select", "Open host"}}
		m.optionDetails = map[string]string{"new": "New host\nCreate a host in a writable inventory database. You will choose its save destination. Profiles and identities can be configured in its editor."}
		connections := map[string][]Connection{}
		for id, h := range byID {
			protocols := []string{}
			s, e := findSource(c, h.SourceID)
			if e != nil {
				return e
			}
			rows, ok := connections[s.ID]
			if !ok {
				rows, e = effectiveSourceConnections(c, s)
				if e != nil {
					return e
				}
				connections[s.ID] = rows
			}
			for _, r := range rows {
				if r.HostID == h.InventoryHostID && r.Default {
					protocols = append(protocols, strings.ToUpper(r.kind()))
				}
			}
			m.optionDetails[id] = fmt.Sprintf("Host: %s\nAddress: %s\nSource: %s\nTags: %s\nNotes: %s\nConfigured: %s\nOpen to edit metadata, profiles, identities and routing. Read-only inventory fields stay protected.", h.Alias, h.Hostname, h.SourceName, strings.Join(h.Tags, ", "), h.Comment, strings.Join(protocols, ", "))
		}
		id, back, err := runChoice(m, menuKey(m.title))
		if err != nil || back {
			return err
		}
		state := PickerState{ConfigPath: path}
		var h *Host
		if id != "new" {
			value := byID[id]
			h = &value
		}
		if h != nil {
			if err = editPersonalHosts(c, path, []Host{*h}); err != nil {
				return err
			}
			c, err = loadConfig(path)
			if err != nil {
				return err
			}
			continue
		}
		if err = createUnifiedHost(c, &state); err != nil {
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
		m := newDialog(strings.ToUpper(kind)+" Profiles", options, "Connection settings from all enabled sources. Delete saves to its database immediately; referenced and read-only profiles are protected.")
		m.footerActions = []choice{{"select", "Edit"}, {"delete", "Delete"}}
		id, back, err := runChoice(m, menuKey(m.title))
		if err != nil || back {
			return err
		}
		if strings.HasPrefix(id, "delete:") {
			key := strings.TrimPrefix(id, "delete:")
			entry, ok := entries[key]
			if !ok {
				continue
			}
			yes, e := reviewDeleteProfile(c, path, entry.s, entry.p)
			if e != nil {
				if e = notice(e.Error()); e != nil {
					return e
				}
				continue
			}
			if !yes {
				continue
			}
			loaded, e := loadConfig(path)
			if e != nil {
				return e
			}
			c.PersonalConnections = loaded.PersonalConnections
			c.LocalProfiles = loaded.LocalProfiles
			c.IdentityAssignments = loaded.IdentityAssignments
			if entry.s.ID == "local" {
				delete(c.LocalProfiles, entry.p.ID)
			}
			continue
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
			action, b, e := objectEditor(strings.ToUpper(kind)+" Profile — "+p.ProfileName, options, "Save writes this profile to "+s.Name+" only. Database: "+s.Path+". Existing legacy login fields are preserved; identities are managed separately.")
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
		m := newDialog("RDP Identities", options, "Device-local usernames and encrypted passwords. Delete removes an unused identity from the draft; Settings Save persists it. The shared encryption key is retained.")
		m.footerActions = []choice{{"select", "Edit"}, {"delete", "Delete"}}
		id, back, err := runChoice(m, menuKey(m.title))
		if err != nil || back {
			return err
		}
		if strings.HasPrefix(id, "delete:") {
			name := strings.TrimPrefix(id, "delete:")
			if name == "new" {
				continue
			}
			if identityReferenced(*c, "rdp", name) {
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
				for i, a := range c.RDP.Accounts {
					if a.Name == name {
						c.RDP.Accounts = append(c.RDP.Accounts[:i:i], c.RDP.Accounts[i+1:]...)
						break
					}
				}
			}
			continue
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
			passwordState := "Not saved"
			if a.Password != nil {
				passwordState = "Saved (encrypted)"
			}
			options := []choice{{"name", "Name: " + a.Name}, {"user", "Username: " + a.User}, {"password", "Password: " + passwordState}}
			if a.Password != nil {
				options = append(options, choice{"show-password", "Show password…"}, choice{"clear-password", "Clear password"})
			}
			action, b, e := objectEditor("RDP Identity — "+a.Name, options, "Save commits this identity and renamed references only. Other Settings changes stay unsaved. Passwords stay encrypted; existing domain settings are preserved.")
			if e != nil {
				return e
			}
			if b {
				break
			}
			if action == "clear-password" {
				a.Password = nil
				continue
			}
			if action == "password" {
				if len(a.PasswordCommand) > 0 {
					_, b, e := choose("Replace credential provider?", []choice{{"replace", "Use an encrypted saved password instead"}}, "Applying this identity will remove its existing password provider. Cancel keeps the provider unchanged.")
					if e != nil {
						return e
					}
					if b {
						continue
					}
				}
				m := newPathModel("New password", "", false)
				m.input.EchoMode = textinput.EchoPassword
				m.input.EchoCharacter = '•'
				if _, e = runTUI(m); e != nil {
					return e
				}
				if m.back {
					continue
				}
				password := m.input.Value()
				m.input.SetValue("")
				if password == "" {
					continue
				}
				if a.ID == "" {
					a.ID = inventoryID()
				}
				p, e := saveIdentityPassword(c.settingsPath, a.ID, password)
				password = ""
				if e != nil {
					if e = notice(e.Error()); e != nil {
						return e
					}
					continue
				}
				a.Password = &p
				a.PasswordCommand = nil
				_, loaded, _, e := settingsDraft(c.settingsPath)
				if e != nil {
					return e
				}
				c.CredentialKeyID = loaded.CredentialKeyID
				continue
			}
			if action == "show-password" {
				key, e := loadCredentialKey(a.Password.KeyID)
				if e != nil {
					if e = notice(e.Error()); e != nil {
						return e
					}
					continue
				}
				password, e := decryptIdentityPassword(key, a.ID, *a.Password)
				clear(key)
				if e != nil {
					return e
				}
				m := newPathModel("Password — Back to hide", password, false)
				password = ""
				if _, e = runTUI(m); e != nil {
					return e
				}
				m.input.SetValue("")
				continue
			}
			if action == "name" {
				value, b, e := inventoryPrompt("Identity name", a.Name)
				if e != nil {
					return e
				}
				if !b {
					a.Name = value
				}
				continue
			}
			if action == "save" || action == "apply" {
				originalName := ""
				if index >= 0 {
					originalName = c.RDP.Accounts[index].Name
				}
				if e = saveIdentityEditor(c, "rdp", originalName, a.Name, LocalIdentity{}, a); e != nil {
					if e = notice(e.Error()); e != nil {
						return e
					}
					continue
				}
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

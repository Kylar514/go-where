package main

import (
	"fmt"
	"strings"
)

func objectEditor(title string, fields []choice, scope string) (string, bool, error) {
	m := newDialog(title, fields, scope)
	m.footerActions = []choice{{"select", "Edit field"}, {"save", "Save"}}
	m.optionDetails = map[string]string{}
	descriptions := map[string]string{
		"kind":           "Choose an environment variable or a literal Unix socket / Windows pipe. Environment values resolve when connecting.",
		"value":          "The environment variable name or local socket/pipe path used to access the signing agent. This is not a key-file path.",
		"name":           "Name used in lists and selectors. Renaming keeps the object's identity and existing references intact.",
		"user":           "Username supplied when logging in. Blank SSH usernames use OpenSSH defaults.",
		"key":            "Local identity-file path. A public key can select its matching private key held by your SSH agent. No key material is copied to inventory.",
		"password":       "Change the encrypted saved password. Input is masked. Leaving it blank keeps the existing password; Clear removes it. No password is shown in Details.",
		"show-password":  "Explicitly reveal the saved password in a temporary screen. Leaving that screen hides it again.",
		"clear-password": "Remove this identity's saved password on Save. The OS encryption key is retained. Login uses a credential provider or client prompt when no saved password exists.",
		"port":           "Service port used by every host selecting this connection profile. Must be between 1 and 65535.",
		"domain":         "RDP login domain supplied by the connection profile. An empty domain supplies no profile domain.",
		"agent":          "Signing-agent endpoint selected for SSH connections. This is not agent forwarding; endpoint definitions stay on this device.",
		"compatibility":  "SSH identity-selection and algorithm settings. Legacy exceptions affect connections using this profile, not global OpenSSH configuration.",
	}
	for _, field := range fields {
		_, value, _ := strings.Cut(field.Label, ": ")
		m.optionDetails[field.ID] = descriptions[field.ID]
		if value != "" {
			m.optionDetails[field.ID] += "\n\nCurrent: " + value
		}
	}
	return runChoice(m, menuKey(title))
}

func renameIdentityReferences(c *Config, kind, old, next string) error {
	if next == "" || next == "default" || next == "inherit" || clean(next) != next {
		return fmt.Errorf("use a non-reserved identity name")
	}
	if kind == "ssh" && !inventoryName(next) {
		return fmt.Errorf("use letters, digits, dots, underscores or hyphens")
	}
	if next == old {
		return nil
	}
	if kind == "ssh" {
		if _, ok := c.Identities[next]; ok {
			return fmt.Errorf("identity name already exists")
		}
	} else {
		for _, a := range c.RDP.Accounts {
			if a.Name == next {
				return fmt.Errorf("identity name already exists")
			}
		}
	}
	for source, values := range c.PersonalConnections {
		for key, v := range values {
			if !strings.HasSuffix(key, ":"+kind) {
				continue
			}
			v.Identities = append([]string(nil), v.Identities...)
			for i, id := range v.Identities {
				if id == old {
					v.Identities[i] = next
				}
			}
			if v.DefaultIdentity == old {
				v.DefaultIdentity = next
			}
			c.PersonalConnections[source][key] = v
		}
	}
	if kind == "ssh" {
		for source, a := range c.IdentityAssignments {
			if a.Default == old {
				a.Default = next
			}
			for id, name := range a.Profiles {
				if name == old {
					a.Profiles[id] = next
				}
			}
			for id, name := range a.Connections {
				if name == old {
					a.Connections[id] = next
				}
			}
			c.IdentityAssignments[source] = a
		}
	}
	if kind == "rdp" {
		for _, values := range c.Overrides {
			for id, o := range values {
				if o.Account != nil && *o.Account == old {
					value := next
					o.Account = &value
					values[id] = o
				}
			}
		}
		// Legacy shared account names remain source-faithful. A local mapping
		// keeps those connections working after a private identity rename.
		for _, s := range c.Sources {
			rows, err := sourceConnections(s)
			if err != nil {
				return err
			}
			for _, r := range rows {
				if r.Account != old {
					continue
				}
				if c.Overrides == nil {
					c.Overrides = map[string]map[string]ConnectionOverride{}
				}
				if c.Overrides[s.ID] == nil {
					c.Overrides[s.ID] = map[string]ConnectionOverride{}
				}
				o := c.Overrides[s.ID][r.ID]
				if o.Account != nil && *o.Account != old {
					continue
				}
				value := next
				o.Account = &value
				c.Overrides[s.ID][r.ID] = o
			}
		}
	}
	return nil
}

func saveIdentityEditor(c *Config, kind, old, name string, ssh LocalIdentity, rdp RDPAccount) error {
	next := cloneAgentSettings(*c)
	if err := renameIdentityReferences(&next, kind, old, name); err != nil {
		return err
	}
	if kind == "ssh" {
		if next.Identities == nil {
			next.Identities = map[string]LocalIdentity{}
		}
		delete(next.Identities, old)
		next.Identities[name] = ssh
	} else {
		rdp.Name = name
		found := false
		for i, a := range next.RDP.Accounts {
			if a.Name == old {
				next.RDP.Accounts[i] = rdp
				found = true
				break
			}
		}
		if !found {
			next.RDP.Accounts = append(next.RDP.Accounts, rdp)
		}
	}
	if err := next.validate(); err != nil {
		return err
	}
	if c.settingsPath != "" {
		original, before, persisted, err := settingsDraft(c.settingsPath)
		if err != nil {
			return err
		}
		if err = renameIdentityReferences(&persisted, kind, old, name); err != nil {
			return err
		}
		if kind == "ssh" {
			if persisted.Identities == nil {
				persisted.Identities = map[string]LocalIdentity{}
			}
			delete(persisted.Identities, old)
			persisted.Identities[name] = ssh
		} else {
			found := false
			for i, a := range persisted.RDP.Accounts {
				if a.Name == old {
					persisted.RDP.Accounts[i] = rdp
					found = true
					break
				}
			}
			if !found {
				persisted.RDP.Accounts = append(persisted.RDP.Accounts, rdp)
			}
		}
		if err = saveSettingsDraft(c.settingsPath, original, before, persisted); err != nil {
			return err
		}
	}
	*c = next
	return nil
}

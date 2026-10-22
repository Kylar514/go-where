package main

import (
	"fmt"
	"maps"
	"strconv"
	"strings"
)

func localValue(stored string, override *string) string {
	if override == nil {
		return "Inherit: " + stored
	}
	if *override == "" {
		return "Cleared (local)"
	}
	return *override + " (local)"
}

func editLocalProfileFields(c *Config, s InventorySource, r Connection, alias string) error {
	for {
		o := c.Overrides[s.ID][r.ID]
		port := strconv.Itoa(r.Port)
		if o.Port != nil {
			port = strconv.Itoa(*o.Port) + " (local)"
		} else {
			port = "Inherit: " + port
		}
		options := []choice{{"user", "Username: " + localValue(r.User, o.User)}, {"port", "Port: " + port}, {"jump", "Jump connection: " + localValue(r.JumpID, o.JumpID)}, {"variables", "Variables…"}}
		if r.kind() == "ssh" {
			options = append(options, choice{"key", "Identity file: " + localValue(r.IdentityFile, o.IdentityFile)}, choice{"agent", "SSH agent…"}, choice{"compatibility", "SSH compatibility…"})
		} else {
			domain := ""
			if r.Domain != nil {
				domain = *r.Domain
			}
			options = append(options, choice{"account", "Account: " + localValue(r.Account, o.Account)}, choice{"domain", "Domain: " + localValue(domain, o.Domain)})
		}
		id, back, err := choose(strings.ToUpper(r.kind())+" local overrides", options, "Changes remain in the same local-settings draft.\n\nInherit follows the shared profile. Cleared key means no configured identity; cleared jump means direct. Saved RDP accounts take precedence over user/domain fields.\n\nNo passwords are stored here.")
		if err != nil || back {
			return err
		}
		if id == "agent" || id == "compatibility" {
			editor := editAgentOverrideDraft
			if id == "compatibility" {
				editor = editCompatibilityOverrideDraft
			}
			if err = editor(c, Host{SourceID: s.ID, ConnectionID: r.ID, Alias: alias}); err != nil {
				return err
			}
			continue
		}
		if id == "variables" {
			name, b, e := inventoryPrompt("Variable name", "")
			if e != nil {
				return e
			}
			if b {
				continue
			}
			if !inventoryVariable.MatchString("${"+name+"}") || strings.ContainsAny(name, "${}") {
				if e = notice("Use a variable name such as SSHKEY or ADMIN_USER."); e != nil {
					return e
				}
				continue
			}
			mode, b, e := chooseOne("Variable override", []choice{{"set", "Set local value"}, {"inherit", "Inherit global variable"}}, "Values are literal, not shell commands.", "set")
			if e != nil {
				return e
			}
			if b {
				continue
			}
			o.Variables = maps.Clone(o.Variables)
			if mode == "inherit" {
				delete(o.Variables, name)
			} else {
				value, b, e := inventoryPrompt("Variable value", o.Variables[name])
				if e != nil {
					return e
				}
				if b {
					continue
				}
				if clean(value) != value || strings.Contains(value, "$") {
					return fmt.Errorf("variable values must be literal text")
				}
				if o.Variables == nil {
					o.Variables = map[string]string{}
				}
				o.Variables[name] = value
			}
		} else {
			mode, b, e := chooseOne("Override "+id, []choice{{"inherit", "Inherit shared setting"}, {"set", "Set local value"}}, "Empty values clear username, key, account or domain. Port must be 1–65535.", "set")
			if e != nil {
				return e
			}
			if b {
				continue
			}
			if id == "jump" && mode == "set" {
				rows, e := sourceConnections(s)
				if e != nil {
					return e
				}
				choices := []choice{{"", "Direct (no jump)"}}
				for _, p := range rows {
					if p.kind() == "ssh" && p.ID != r.ID {
						choices = append(choices, choice{p.ID, p.Alias + " / " + p.ProfileName})
					}
				}
				value, b, e := chooseOne("Local jump", choices, "Route uses the selected host/profile attachment.", r.JumpID)
				if e != nil {
					return e
				}
				if b {
					continue
				}
				o.JumpID = &value
			} else if id == "port" {
				if mode == "inherit" {
					o.Port = nil
				} else {
					value, b, e := inventoryPrompt("Local port", strconv.Itoa(r.Port))
					if e != nil {
						return e
					}
					if b {
						continue
					}
					p, e := connectionPort(value)
					if e != nil {
						if e = notice(e.Error()); e != nil {
							return e
						}
						continue
					}
					o.Port = &p
				}
			} else {
				field := &o.User
				stored := r.User
				switch id {
				case "key":
					field = &o.IdentityFile
					stored = r.IdentityFile
				case "jump":
					field = &o.JumpID
					stored = r.JumpID
				case "account":
					field = &o.Account
					stored = r.Account
				case "domain":
					field = &o.Domain
					if r.Domain != nil {
						stored = *r.Domain
					}
				}
				if mode == "inherit" {
					*field = nil
				} else {
					value, b, e := inventoryPrompt("Local "+id, stored)
					if e != nil {
						return e
					}
					if b {
						continue
					}
					if clean(value) != value {
						return fmt.Errorf("control characters are not allowed")
					}
					*field = &value
				}
			}
		}
		if c.Overrides == nil {
			c.Overrides = map[string]map[string]ConnectionOverride{}
		}
		if c.Overrides[s.ID] == nil {
			c.Overrides[s.ID] = map[string]ConnectionOverride{}
		}
		c.Overrides[s.ID][r.ID] = o
	}
}

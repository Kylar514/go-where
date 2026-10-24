package main

import "fmt"

func agentReferences(c Config, name, replacement string) ([]string, error) {
	if replacement != "" && c.Overrides == nil {
		c.Overrides = map[string]map[string]ConnectionOverride{}
	}
	var uses []string
	for id, p := range c.Identities {
		if p.SSHAgent == name {
			uses = append(uses, "Identity: "+id)
			if replacement != "" {
				p.SSHAgent = replacement
				c.Identities[id] = p
			}
		}
	}
	for id, p := range c.LocalProfiles {
		if p.SSHAgent == name {
			uses = append(uses, "Local profile: "+p.ProfileName)
			if replacement != "" {
				p.SSHAgent = replacement
				c.LocalProfiles[id] = p
			}
		}
	}
	for source, values := range c.Overrides {
		for id, o := range values {
			if o.SSHAgent != nil && *o.SSHAgent == name {
				uses = append(uses, "Local connection: "+source+" / "+id)
				if replacement != "" {
					v := replacement
					o.SSHAgent = &v
					values[id] = o
				}
			}
		}
	}
	for _, s := range c.Sources {
		rows, err := sourceConnections(s)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			if r.SSHAgent != name {
				continue
			}
			uses = append(uses, s.Name+" / "+r.ProfileName)
			if replacement != "" {
				if c.Overrides == nil {
					return nil, fmt.Errorf("initialize override mappings before renaming a shared agent reference")
				}
				if c.Overrides[s.ID] == nil {
					c.Overrides[s.ID] = map[string]ConnectionOverride{}
				}
				o := c.Overrides[s.ID][r.ID]
				if o.SSHAgent == nil {
					v := replacement
					o.SSHAgent = &v
					c.Overrides[s.ID][r.ID] = o
				}
			}
		}
	}
	return uses, nil
}

func saveAgentChanges(c *Config, next Config) error {
	if err := next.validate(); err != nil {
		return err
	}
	if c.settingsPath != "" {
		original, before, draft, err := settingsDraft(c.settingsPath)
		if err != nil {
			return err
		}
		draft.SSHAgents = next.SSHAgents
		draft.Identities = next.Identities
		draft.LocalProfiles = next.LocalProfiles
		draft.Overrides = next.Overrides
		if err = saveSettingsDraft(c.settingsPath, original, before, draft); err != nil {
			return err
		}
	}
	*c = next
	return nil
}

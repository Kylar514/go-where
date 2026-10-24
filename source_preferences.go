package main

import "fmt"

func sourceEnabled(c Config, id string) bool {
	for _, disabled := range c.DisabledSources {
		if disabled == id {
			return false
		}
	}
	return true
}

func validateSourcePreferences(c Config) error {
	known := map[string]bool{"ssh-config": true}
	for _, s := range c.Sources {
		known[s.ID] = true
	}
	seen := map[string]bool{}
	for _, id := range c.DisabledSources {
		if !known[id] || seen[id] {
			return fmt.Errorf("invalid or duplicate disabled source %q", id)
		}
		seen[id] = true
	}
	if c.StartupSource != "" && (!known[c.StartupSource] || !sourceEnabled(c, c.StartupSource)) {
		return fmt.Errorf("startup source must be registered and enabled")
	}
	return nil
}

func editSourcePreferences(c *Config) error {
	items := []multiItem{{ID: "ssh-config", Label: "SSH config", Note: "OpenSSH", Checked: sourceEnabled(*c, "ssh-config")}}
	for _, s := range c.Sources {
		items = append(items, multiItem{ID: s.ID, Label: s.Name, Note: "SQLite", Checked: sourceEnabled(*c, s.ID)})
	}
	m := newMultiDialog("Enabled Sources", items, "Checked sources load at launch. Select all / Deselect all includes hidden filtered sources. No selection is valid. Source files remain untouched. Save commits enablement only.")
	m.itemDetails = map[string]string{"ssh-config": "OpenSSH inventory\nLocation: ~/.ssh/config\nDisabling stops loading this source; it does not edit the file."}
	for _, s := range c.Sources {
		permission := "Read-write"
		if s.ReadOnly {
			permission = "Read-only"
		}
		m.itemDetails[s.ID] = "Source: " + s.Name + "\nType: SQLite\nLocation: " + s.Path + "\nAccess: " + permission + "\nEnabling loads its hosts and profiles at launch."
	}
	m.sourceSelection = true
	apply := func() {
		c.DisabledSources = nil
		c.StartupSource = ""
		for _, item := range m.items {
			if !item.Checked {
				c.DisabledSources = append(c.DisabledSources, item.ID)
			}
		}
	}
	for {
		if _, err := runTUI(m); err != nil {
			return err
		}
		apply()
		if m.back || m.action == "done" {
			return nil
		}
		if m.action == "save" && c.settingsPath != "" {
			original, before, draft, err := settingsDraft(c.settingsPath)
			if err != nil {
				return err
			}
			draft.DisabledSources = c.DisabledSources
			draft.StartupSource = c.StartupSource
			if err := saveSettingsDraft(c.settingsPath, original, before, draft); err != nil {
				if err = notice(err.Error()); err != nil {
					return err
				}
			}
		}
		m.done, m.back = false, false
		m.action = ""
	}
}

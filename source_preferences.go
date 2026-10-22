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
	m := newMultiDialog("Select sources", items, "Checked sources load at launch.\n\nSelect all / Deselect all includes hidden filtered sources. No selection is valid. Source files remain untouched.\n\nSave settings persists the entire Settings draft. Back applies checks to the draft without saving.")
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
		if m.action == "save" && saveActiveSettings != nil {
			if err := saveActiveSettings(); err != nil {
				if err = notice(err.Error()); err != nil {
					return err
				}
			}
		}
		m.done, m.back = false, false
		m.action = ""
	}
}

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
	m.title = "Sources"
	m.sourceManagement = c.settingsPath != ""
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
		if m.action == "create" || m.action == "attach" || m.action == "remove" {
			if m.action == "remove" {
				if m.cursor >= len(m.filtered) {
					continue
				}
				id := m.items[m.filtered[m.cursor]].ID
				if id == "ssh-config" {
					if err := notice("OpenSSH config can be disabled but not unregistered."); err != nil {
						return err
					}
				} else {
					s, err := findSource(*c, id)
					if err != nil {
						return err
					}
					if err = removeSourceRegistration(c.settingsPath, *c, &PickerState{}, s.ID); err != nil {
						return err
					}
				}
			} else {
				state := PickerState{}
				if err := addInventorySource(c.settingsPath, *c, &state, m.action); err != nil {
					return err
				}
			}
			loaded, err := loadConfig(c.settingsPath)
			if err != nil {
				return err
			}
			c.Sources = loaded.Sources
			c.DisabledSources = loaded.DisabledSources
			c.StartupSource = loaded.StartupSource
			selected := ""
			if m.cursor < len(m.filtered) {
				selected = m.items[m.filtered[m.cursor]].ID
			}
			checked := map[string]bool{}
			for _, item := range m.items {
				checked[item.ID] = item.Checked
			}
			m.items = []multiItem{{ID: "ssh-config", Label: "SSH config", Note: "OpenSSH", Checked: checked["ssh-config"]}}
			for _, s := range c.Sources {
				value, ok := checked[s.ID]
				if !ok {
					value = sourceEnabled(*c, s.ID)
				}
				m.items = append(m.items, multiItem{ID: s.ID, Label: s.Name, Note: "SQLite", Checked: value})
				m.itemDetails[s.ID] = "Source: " + s.Name + "\nDatabase: " + s.Path
			}
			m.refilter()
			for i, index := range m.filtered {
				if m.items[index].ID == selected {
					m.cursor = i
					break
				}
			}
			m.done, m.back = false, false
			m.action = ""
			continue
		}
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

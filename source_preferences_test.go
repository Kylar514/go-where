package main

import (
	tea "github.com/charmbracelet/bubbletea"
	"path/filepath"
	"testing"
)

func TestSourceSelectAllIncludesFilteredRows(t *testing.T) {
	m := newMultiDialog("Select sources", []multiItem{{ID: "a", Label: "Alpha"}, {ID: "b", Label: "Beta"}}, "")
	m.sourceSelection = true
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	m.query.SetValue("Alpha")
	m.refilter()
	m.activate(len(m.filtered))
	if !m.items[0].Checked || !m.items[1].Checked {
		t.Fatal("select all omitted filtered row")
	}
	m.activate(len(m.filtered) + 1)
	if m.items[0].Checked || m.items[1].Checked {
		t.Fatal("deselect all failed")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if m.action != "save" {
		t.Fatal("source save shortcut failed")
	}
}

func TestDisabledSourcesNotReadAndPreferencesSave(t *testing.T) {
	c := defaultConfig()
	c.Sources = []InventorySource{{ID: "missing", Name: "Missing", Type: "sqlite", Path: filepath.Join(t.TempDir(), "missing.db")}}
	c.DisabledSources = []string{"missing", "ssh-config"}
	if err := c.validate(); err != nil {
		t.Fatal(err)
	}
	hosts, err := inventoryHosts(c)
	if err != nil || len(hosts) != 0 {
		t.Fatal("disabled source read", hosts, err)
	}
	c.StartupSource = "missing"
	if c.validate() == nil {
		t.Fatal("disabled startup accepted")
	}
	c.StartupSource = ""
	scriptTUI(t, "toggle:ssh-config,done")
	if err = editSourcePreferences(&c); err != nil {
		t.Fatal(err)
	}
	if c.StartupSource != "" || !sourceEnabled(c, "ssh-config") || sourceEnabled(c, "missing") {
		t.Fatal(c)
	}
	path := filepath.Join(t.TempDir(), "settings.json")
	if err = saveSettingsDraft(path, []byte{}, defaultConfig(), c); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadConfig(path)
	if err != nil || loaded.StartupSource != c.StartupSource || sourceEnabled(loaded, "missing") {
		t.Fatal(loaded, err)
	}
}

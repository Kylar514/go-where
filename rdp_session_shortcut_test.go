package main

import (
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestHostSessionShortcutSavesGlobalNotHostDraft(t *testing.T) {
	s := testInventory(t, "Hosts")
	if err := changeConnection(s, Connection{ID: "c", Alias: "host", Address: "192.0.2.1", Port: 22}, false); err != nil {
		t.Fatal(err)
	}
	rows, _ := sourceConnections(s)
	path := filepath.Join(t.TempDir(), "profile.db")
	original, before, c, err := settingsDraft(path)
	if err != nil {
		t.Fatal(err)
	}
	c.Sources = []InventorySource{s}
	if err = saveSettingsDraft(path, original, before, c); err != nil {
		t.Fatal(err)
	}
	oldRun := runTUI
	defer func() { runTUI = oldRun }()
	step := 0
	runTUI = func(model tea.Model) (tea.Model, error) {
		step++
		switch m := model.(type) {
		case *dialogModel:
			switch step {
			case 1:
				m.result = "notes"
			case 3:
				found := false
				for _, o := range m.options {
					if o.ID == "rdp-session" && o.Label == "RDP session: Global" {
						found = true
					}
				}
				if !found {
					t.Fatal(m.options)
				}
				m.result = "rdp-session"
			case 4:
				m.result = "clipboard"
			case 5:
				m.result = "on"
			case 6:
				m.result = "__save_settings"
			case 7:
				m.back = true
			case 8:
				m.back = true
			default:
				t.Fatalf("unexpected dialog %d %s", step, m.title)
			}
		case *pathModel:
			if step != 2 {
				t.Fatal(step)
			}
			m.input.SetValue("unsaved note")
		}
		return model, nil
	}
	if err = editPersonalHosts(c, path, []Host{{SourceID: s.ID, InventoryHostID: rows[0].HostID}}); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.RDP.Session.Clipboard == nil || !*loaded.RDP.Session.Clipboard {
		t.Fatal("global change not saved")
	}
	hosts, _ := sourceInventoryHosts(s)
	if hosts[0].Notes != "" {
		t.Fatal("global save persisted host draft")
	}
}

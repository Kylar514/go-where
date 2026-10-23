package main

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestEmptyHostTunnelMenuNewPrefillsSource(t *testing.T) {
	s := testInventory(t, "Homelab")
	if err := changeConnection(s, Connection{ID: "c", Alias: "host", Address: "192.0.2.1", Port: 22}, false); err != nil {
		t.Fatal(err)
	}
	rows, _ := sourceConnections(s)
	c := defaultConfig()
	c.Sources = []InventorySource{s}
	old := runTUI
	defer func() { runTUI = old }()
	step := 0
	runTUI = func(model tea.Model) (tea.Model, error) {
		d := model.(*dialogModel)
		step++
		switch step {
		case 1:
			if d.title != "Tunnels — host" || len(d.options) != 1 || d.options[0].ID != "new" || !strings.Contains(d.optionDetails["new"], "No tunnels configured") {
				t.Fatal(d.title, d.options, d.optionDetails)
			}
			d.result = "new"
		case 2:
			d.result = s.ID
		case 3:
			found := false
			for _, o := range d.options {
				if o.ID == "source-host" && strings.Contains(o.Label, "host — Homelab") {
					found = true
				}
			}
			if !found {
				t.Fatal(d.options)
			}
			d.back = true
		case 4:
			d.back = true
		default:
			t.Fatal("extra menu", step)
		}
		return model, nil
	}
	if err := editHostTunnels(c, filepath.Join(t.TempDir(), "profile.db"), []Host{{SourceID: s.ID, InventoryHostID: rows[0].HostID}}); err != nil {
		t.Fatal(err)
	}
}

func TestBulkHostEditorOmitsTunnels(t *testing.T) {
	s := testInventory(t, "Hosts")
	for _, r := range []Connection{{ID: "a", Alias: "a", Address: "192.0.2.1", Port: 22}, {ID: "b", Alias: "b", Address: "192.0.2.2", Port: 22}} {
		if err := changeConnection(s, r, false); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(t.TempDir(), "profile.db")
	original, before, c, err := settingsDraft(path)
	if err != nil {
		t.Fatal(err)
	}
	c.Sources = []InventorySource{s}
	if err = saveSettingsDraft(path, original, before, c); err != nil {
		t.Fatal(err)
	}
	rows, _ := sourceConnections(s)
	var hosts []Host
	for _, r := range rows {
		hosts = append(hosts, Host{SourceID: s.ID, InventoryHostID: r.HostID})
	}
	old := runTUI
	defer func() { runTUI = old }()
	runTUI = func(model tea.Model) (tea.Model, error) {
		d := model.(*dialogModel)
		for _, o := range d.options {
			if o.ID == "tunnels" {
				t.Fatal("bulk tunnels exposed")
			}
		}
		d.back = true
		return d, nil
	}
	if err = editPersonalHosts(c, path, hosts); err != nil {
		t.Fatal(err)
	}
}

package main

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestUnifiedHostDeleteAndCancel(t *testing.T) {
	s := testInventory(t, "Delete hosts")
	if err := changeConnection(s, Connection{ID: "c", Alias: "host", Address: "192.0.2.1", Port: 22}, false); err != nil {
		t.Fatal(err)
	}
	rows, _ := sourceConnections(s)
	hostID := rows[0].HostID
	path := filepath.Join(t.TempDir(), "profile.db")
	original, before, c, err := settingsDraft(path)
	if err != nil {
		t.Fatal(err)
	}
	c.Sources = []InventorySource{s}
	c.PersonalConnections = map[string]map[string]PersonalConnection{s.ID: {hostID + ":ssh": {Disabled: true}}}
	if err = saveSettingsDraft(path, original, before, c); err != nil {
		t.Fatal(err)
	}
	h := Host{SourceID: s.ID, InventoryHostID: hostID}
	scriptTUI(t, "notes", "enter\ndraft", "delete-host", "cancel", "save", "back")
	if err = editPersonalHosts(c, path, []Host{h}); err != nil {
		t.Fatal(err)
	}
	hosts, _ := sourceInventoryHosts(s)
	if len(hosts) != 1 || hosts[0].Notes != "draft" {
		t.Fatal("cancel lost parent draft", hosts)
	}
	profiles, _ := sourceProfiles(s)
	original, before, c, err = settingsDraft(path)
	if err != nil {
		t.Fatal(err)
	}
	c.PersonalConnections[s.ID]["other:route"] = PersonalConnection{Jump: &ProfileReference{Source: s.ID, ID: hostID}}
	if err = saveSettingsDraft(path, original, before, c); err != nil {
		t.Fatal(err)
	}
	scriptTUI(t, "confirm")
	if deleted, e := reviewDeleteHost(path, s, hosts[0]); deleted || e == nil {
		t.Fatal("referenced host deleted")
	}
	original, before, c, err = settingsDraft(path)
	if err != nil {
		t.Fatal(err)
	}
	delete(c.PersonalConnections[s.ID], "other:route")
	if err = saveSettingsDraft(path, original, before, c); err != nil {
		t.Fatal(err)
	}
	scriptTUI(t, "delete-host", "confirm")
	if err = editPersonalHosts(c, path, []Host{h}); err != nil {
		t.Fatal(err)
	}
	hosts, _ = sourceInventoryHosts(s)
	if len(hosts) != 0 {
		t.Fatal("host retained", hosts)
	}
	after, _ := sourceProfiles(s)
	if len(after) != len(profiles) {
		t.Fatal("deleted reusable profiles")
	}
	loaded, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := loaded.PersonalConnections[s.ID][hostID+":ssh"]; ok {
		t.Fatal("stale personal selection")
	}
}

func TestHostDeleteButtonAndHotkey(t *testing.T) {
	m := newDialog("Edit host", []choice{{"name", "Name: host"}}, "")
	m.footerActions = []choice{{"select", "Edit field"}, {"save", "Save"}, {"delete-host", "Delete host"}}
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	if !strings.Contains(m.View(), "delete Delete host") {
		t.Fatal("missing hint", m.View())
	}
	m.Update(tea.KeyMsg{Type: tea.KeyDelete})
	if m.result != "delete-host" {
		t.Fatal(m.result)
	}
	m.done = false
	m.result = ""
	m.Update(tea.MouseMsg{X: 30, Y: m.footerY(), Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if m.result != "delete-host" {
		t.Fatal("mouse delete", m.result)
	}
	s := testInventory(t, "Read only")
	s.ReadOnly = true
	if deleteInventoryHost(s, "host") == nil {
		t.Fatal("read-only guard missing")
	}
}

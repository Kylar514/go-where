package main

import (
	"fmt"
	"path/filepath"
	"testing"
)

func TestLocalEditorIgnoresUnrelatedUnavailableSource(t *testing.T) {
	s := testInventory(t, "Local")
	if err := changeConnection(s, Connection{ID: "ssh", Alias: "host", Address: "192.0.2.1", Port: 22}, false); err != nil {
		t.Fatal(err)
	}
	rows, err := sourceConnections(s)
	if err != nil {
		t.Fatal(err)
	}
	remote := InventorySource{ID: "offline", Name: "Offline", Type: "api", URL: "https://127.0.0.1:1", CredentialID: "absent"}
	get := apiTokenGet
	defer func() { apiTokenGet = get }()
	apiTokenGet = func(string, string) (string, error) { return "", fmt.Errorf("missing credential") }
	path := filepath.Join(t.TempDir(), "profile.db")
	original, before, c, err := settingsDraft(path)
	if err != nil {
		t.Fatal(err)
	}
	c.Sources = []InventorySource{remote, s}
	if err = saveSettingsDraft(path, original, before, c); err != nil {
		t.Fatal(err)
	}
	h := Host{SourceID: s.ID, InventoryHostID: rows[0].HostID, ConnectionID: rows[0].ID, Type: "ssh"}
	scriptTUI(t, "notes", "enter\nupdated", "save", "back")
	if err = editPersonalHosts(c, path, []Host{h}); err != nil {
		t.Fatal("unrelated source blocked edit", err)
	}
	hosts, err := sourceInventoryHosts(s)
	if err != nil || hosts[0].Notes != "updated" {
		t.Fatal(hosts, err)
	}
	options, _, err := profileChoices(c, "ssh")
	if err != nil || len(options) < 3 {
		t.Fatal("profile choices blocked", options, err)
	}
	scriptTUI(t, s.ID+":"+h.InventoryHostID)
	if _, back, err := chooseTunnelHost(c, nil); err != nil || back {
		t.Fatal("host selector blocked", err)
	}
	if _, err = defaultTunnelHost(c, ProfileReference{Source: remote.ID, ID: "missing"}); err == nil {
		t.Fatal("explicit unavailable dependency ignored")
	}
	if _, _, err = hostTunnelEntries(c, []Host{{SourceID: remote.ID, InventoryHostID: "missing"}}); err == nil {
		t.Fatal("required source ignored")
	}
}

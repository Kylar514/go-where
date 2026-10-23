package main

import (
	"path/filepath"
	"testing"
)

func TestSaveHostIgnoresUnrelatedMissingKey(t *testing.T) {
	s := testInventory(t, "Work")
	for _, r := range []Connection{{ID: "host", Alias: "host", Address: "192.0.2.1", Port: 22}, {ID: "other", Alias: "admin-gateway", Address: "192.0.2.2", Port: 22, IdentityFile: "${IMPORT_KEY_MISSING}"}} {
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
	var hostID string
	for _, r := range rows {
		if r.ID == "host" {
			hostID = r.HostID
		}
	}
	scriptTUI(t, "notes", "ok\nupdated", "save", "back")
	if err = editPersonalHosts(c, path, []Host{{SourceID: s.ID, InventoryHostID: hostID}}); err != nil {
		t.Fatal(err)
	}
	hosts, _ := sourceInventoryHosts(s)
	found := false
	for _, h := range hosts {
		if h.ID == hostID && h.Notes == "updated" {
			found = true
		}
	}
	if !found {
		t.Fatal("host not saved")
	}
	if _, err = managedJob(Host{SourceID: s.ID, ConnectionID: "other"}, c); err == nil {
		t.Fatal("launch missing-variable check disappeared")
	}
}

func TestEditedRouteValidationRejectsCyclesWithoutResolvingKeys(t *testing.T) {
	s := testInventory(t, "Routes")
	for _, r := range []Connection{{ID: "a", Alias: "a", Address: "192.0.2.1", Port: 22}, {ID: "b", Alias: "b", Address: "192.0.2.2", Port: 22, IdentityFile: "${MISSING}"}} {
		if err := changeConnection(s, r, false); err != nil {
			t.Fatal(err)
		}
	}
	rows, _ := sourceConnections(s)
	ids := map[string]string{}
	for _, r := range rows {
		ids[r.ID] = r.HostID
	}
	c := defaultConfig()
	c.Sources = []InventorySource{s}
	c.PersonalConnections = map[string]map[string]PersonalConnection{s.ID: {ids["a"] + ":route": {Jump: &ProfileReference{s.ID, ids["b"]}}}}
	hosts := []Host{{SourceID: s.ID, InventoryHostID: ids["a"]}}
	if err := validateEditedHostRoutes(c, hosts); err != nil {
		t.Fatal("route validated launch credentials", err)
	}
	c.PersonalConnections[s.ID][ids["b"]+":route"] = PersonalConnection{Jump: &ProfileReference{s.ID, ids["a"]}}
	if err := validateEditedHostRoutes(c, hosts); err == nil {
		t.Fatal("cycle accepted")
	}
}

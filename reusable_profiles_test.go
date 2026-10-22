package main

import (
	"strings"
	"testing"
)

func TestReusableProfilesShareSettingsNotRoutes(t *testing.T) {
	s := testInventory(t, "Shared")
	h1 := InventoryHost{ID: "one", Alias: "one", Address: "192.0.2.1"}
	h2 := InventoryHost{ID: "two", Alias: "two", Address: "192.0.2.2"}
	p := Connection{ID: "first", Alias: "first", HostID: h1.ID, Type: "ssh", ProfileName: "Standard", Default: true, Port: 22, User: "operator"}
	if err := saveInventoryHost(s, h1, []Connection{p}); err != nil {
		t.Fatal(err)
	}
	rows, err := sourceConnections(s)
	if err != nil {
		t.Fatal(err)
	}
	first := rows[0]
	second := first
	second.ID = "second"
	second.Alias = "second"
	second.HostID = h2.ID
	second.JumpID = first.ID
	if err = saveInventoryHost(s, h2, []Connection{second}); err != nil {
		t.Fatal(err)
	}
	shared, err := sourceProfiles(s)
	if err != nil || len(shared) != 1 {
		t.Fatal(shared, err)
	}
	rows, err = sourceConnections(s)
	if err != nil {
		t.Fatal(err)
	}
	var stale Connection
	for _, r := range rows {
		if r.ID == "second" {
			stale = r
		}
		if r.ID == "first" {
			first = r
		}
	}
	first.User = "admin"
	if err = saveInventoryHost(s, h1, []Connection{first}); err != nil {
		t.Fatal(err)
	}
	rows, err = sourceConnections(s)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.User != "admin" {
			t.Fatal("shared settings did not propagate", r)
		}
		if r.ID == "second" && (r.Address != h2.Address || r.JumpID != "first") {
			t.Fatal("route/address changed", r)
		}
	}
	// A stale host metadata save must not overwrite an updated shared profile.
	h2.Notes = "metadata only"
	if err = saveInventoryHost(s, h2, []Connection{stale}); err != nil {
		t.Fatal(err)
	}
	rows, _ = sourceConnections(s)
	for _, r := range rows {
		if r.User != "admin" {
			t.Fatal("metadata save overwrote shared profile")
		}
	}
	stale.User = "conflict"
	if err = saveInventoryHost(s, h2, []Connection{stale}); err == nil || !strings.Contains(err.Error(), "changed while editing") {
		t.Fatal("stale profile edit accepted", err)
	}
}

func TestReusableProfileMigrationV6PreservesReferences(t *testing.T) {
	s := testInventory(t, "Migration")
	r := Connection{ID: "original", Alias: "original", Address: "192.0.2.1", Port: 22, User: "operator"}
	if err := changeConnection(s, r, false); err != nil {
		t.Fatal(err)
	}
	db, err := openInventory(s)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("DROP TABLE profiles"); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE connections SET data='{"id":"original","alias":"original","host_id":"original","profile_name":"Default","default":true,"port":22,"user":"operator"}'`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("PRAGMA user_version=6"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	rows, err := sourceConnections(s)
	if err != nil || len(rows) != 1 || rows[0].ID != "original" || rows[0].ProfileID != "original" || rows[0].User != "operator" {
		t.Fatal(rows, err)
	}
}

func TestHostProfileMenuListsSourceProfilesDirectly(t *testing.T) {
	s := testInventory(t, "Direct profiles")
	first := InventoryHost{ID: "one", Alias: "one", Address: "192.0.2.1"}
	p := Connection{ID: "ssh", Alias: "ssh", HostID: first.ID, Type: "ssh", ProfileName: "Standard", Default: true, Port: 22}
	if err := saveInventoryHost(s, first, []Connection{p}); err != nil {
		t.Fatal(err)
	}
	shared, err := sourceProfiles(s)
	if err != nil {
		t.Fatal(err)
	}
	c := defaultConfig()
	c.Sources = []InventorySource{s}
	second := InventoryHost{ID: "two", Alias: "two", Address: "192.0.2.2"}
	var assignments []Connection
	scriptTUI(t, "available:"+shared[0].ID, "back")
	if err = editHostProfiles(c, s, second, &assignments, "ssh", ""); err != nil {
		t.Fatal(err)
	}
	if len(assignments) != 1 || assignments[0].ProfileID != shared[0].ID || !assignments[0].Default {
		t.Fatal(assignments)
	}
}

func TestHostProfileSummaryReflectsDraft(t *testing.T) {
	profiles := []Connection{{Type: "ssh", ProfileName: "Standard", Default: true}, {Type: "ssh", ProfileName: "Admin"}, {Type: "rdp", ProfileName: "Desktop", Default: true}}
	if got := hostProfileSummary(profiles, "ssh"); got != "Standard ★, Admin" {
		t.Fatal(got)
	}
	if got := hostProfileSummary(profiles, "rdp"); got != "Desktop ★" {
		t.Fatal(got)
	}
	profiles[0].Default = false
	profiles[1].Default = true
	profiles[1].ProfileName = "Root"
	if got := hostProfileSummary(profiles, "ssh"); got != "Standard, Root ★" {
		t.Fatal(got)
	}
	if got := hostProfileSummary(nil, "ssh"); got != "Not configured" {
		t.Fatal(got)
	}
}

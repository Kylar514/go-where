package main

import "testing"

func TestLocalSSHAndRDPFieldsResolveWithoutChangingInventory(t *testing.T) {
	s := testInventory(t, "Overrides")
	r := Connection{ID: "ssh", Alias: "ssh", Address: "192.0.2.1", Port: 22, User: "shared"}
	if err := changeConnection(s, r, false); err != nil {
		t.Fatal(err)
	}
	c := defaultConfig()
	c.Sources = []InventorySource{s}
	scriptTUI(t, "user", "set", "enter\nlocal", "port", "set", "enter\n2222", "key", "set", "enter\n/tmp/test-key", "back")
	if err := editLocalProfileFields(&c, s, r, "host"); err != nil {
		t.Fatal(err)
	}
	resolved, err := resolvedConnection(c, s, r)
	if err != nil || resolved.User != "local" || resolved.Port != 2222 || resolved.IdentityFile != "/tmp/test-key" {
		t.Fatal(resolved, err)
	}
	rdp := Connection{ID: "rdp", Type: "rdp", Alias: "rdp", Address: "192.0.2.2", Port: 3389, User: "shared"}
	scriptTUI(t, "user", "set", "enter\nadmin", "domain", "set", "enter\nLAB", "back")
	if err := editLocalProfileFields(&c, s, rdp, "desktop"); err != nil {
		t.Fatal(err)
	}
	resolved, err = resolvedConnection(c, s, rdp)
	if err != nil || resolved.User != "admin" || resolved.Domain == nil || *resolved.Domain != "LAB" {
		t.Fatal(resolved, err)
	}
	rows, err := sourceConnections(s)
	if err != nil || rows[0].User != "shared" || rows[0].Port != 22 {
		t.Fatal("inventory modified", rows, err)
	}
}

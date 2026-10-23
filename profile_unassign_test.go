package main

import "testing"

func TestProfileNoneAndUnassignStayInDraft(t *testing.T) {
	s := testInventory(t, "Profiles")
	c := defaultConfig()
	c.Sources = []InventorySource{s}
	h := InventoryHost{ID: "host", Alias: "host", Address: "192.0.2.1"}
	profiles := []Connection{{ID: "one", Type: "ssh", ProfileName: "One", Default: true}, {ID: "two", Type: "ssh", ProfileName: "Two"}, {ID: "rdp", Type: "rdp", ProfileName: "Desktop", Default: true}}
	scriptTUI(t, "unassign:one", "back")
	if err := editHostProfiles(c, s, h, &profiles, "ssh", ""); err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 2 || profiles[0].ID != "two" || !profiles[0].Default {
		t.Fatal(profiles)
	}
	scriptTUI(t, "none", "back")
	if err := editHostProfiles(c, s, h, &profiles, "ssh", ""); err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 || profiles[0].Type != "rdp" {
		t.Fatal(profiles)
	}
}

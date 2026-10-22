package main

import (
	"path/filepath"
	"testing"
)

func TestCombinedLocalOverridesDraftCancel(t *testing.T) {
	s := testInventory(t, "Local")
	h := InventoryHost{ID: "host", Alias: "host", Address: "192.0.2.1"}
	p := Connection{ID: "ssh", Alias: "ssh", HostID: h.ID, ProfileName: "Default", Default: true, Type: "ssh", Port: 22}
	if err := saveInventoryHost(s, h, []Connection{p}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := registerSource(path, s); err != nil {
		t.Fatal(err)
	}
	scriptTUI(t, "address", "custom", "enter\n192.0.2.2", "ssh", "agent", "none", "back", "back")
	if err := editHostLocalOverrides(path, s, h); err != nil {
		t.Fatal(err)
	}
	c, err := loadConfig(path)
	if err != nil || len(c.HostOverrides) != 0 || len(c.Overrides) != 0 {
		t.Fatal("cancel persisted local draft", c, err)
	}
}

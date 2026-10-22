package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRemoveSourceKeepsDatabaseAndCleansLocalSettings(t *testing.T) {
	s := testInventory(t, "Remove")
	path := filepath.Join(t.TempDir(), "settings.json")
	c := defaultConfig()
	c.Sources = []InventorySource{s}
	c.DisabledSources = []string{s.ID}
	address := "192.0.2.1"
	c.HostOverrides = map[string]map[string]HostOverride{s.ID: {"host": {Address: &address}}}
	if err := saveSettingsDraft(path, []byte{}, defaultConfig(), c); err != nil {
		t.Fatal(err)
	}
	state := PickerState{Source: s.ID, Marked: []string{s.ID + ":host", "other:host"}}
	scriptTUI(t, "remove", "back")
	if err := removeSourceRegistration(path, c, &state, s.ID); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadConfig(path)
	if err != nil || len(loaded.Sources) != 1 {
		t.Fatal("cancel removed registration")
	}
	scriptTUI(t, "remove", "discard")
	if err = removeSourceRegistration(path, loaded, &state, s.ID); err != nil {
		t.Fatal(err)
	}
	loaded, err = loadConfig(path)
	if err != nil || len(loaded.Sources) != 0 || len(loaded.DisabledSources) != 0 || len(loaded.HostOverrides[s.ID]) != 0 {
		t.Fatal(loaded, err)
	}
	if _, err = os.Stat(s.Path); err != nil {
		t.Fatal("database deleted", err)
	}
	if state.Source != "" || len(state.Marked) != 1 {
		t.Fatal("session state not cleaned", state)
	}
}

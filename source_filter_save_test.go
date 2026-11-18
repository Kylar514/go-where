package main

import (
	"path/filepath"
	"testing"
)

func TestCreateHostPreservesAllSourcesFilter(t *testing.T) {
	s := testInventory(t, "New hosts")
	other := testInventory(t, "Other hosts")
	if err := saveInventoryHost(other, InventoryHost{ID: "existing", Alias: "existing", Address: "192.0.2.2"}, nil); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "profile.db")
	original, before, c, err := settingsDraft(path)
	if err != nil {
		t.Fatal(err)
	}
	c.Sources = []InventorySource{s, other}
	if err = saveSettingsDraft(path, original, before, c); err != nil {
		t.Fatal(err)
	}
	scriptTUI(t, s.ID, "name", "enter\nnew-host", "address", "enter\n192.0.2.1", "save", "back")
	state := PickerState{ConfigPath: path, Mode: "ssh"}
	if err = createUnifiedHost(c, &state); err != nil {
		t.Fatal(err)
	}
	if state.Source != "" {
		t.Fatal("Save narrowed All sources", state.Source)
	}
	hosts, err := inventoryHosts(c)
	if err != nil || len(hosts) != 2 {
		t.Fatal(hosts, err)
	}
	m := newPickerModel(hosts, "", "current", Origin{}, state, c)
	if len(m.visible) != 2 {
		t.Fatal("other source hidden", m.visible)
	}
}

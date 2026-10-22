package main

import (
	"path/filepath"
	"testing"
)

func TestSaveChooserCreatesSourceAndReturnsAfterCancel(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s := testInventory(t, "Personal")
	if err := registerSource(path, s); err != nil {
		t.Fatal(err)
	}
	c, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	scriptTUI(t, "create", "back\nignored", s.ID)
	chosen, back, err := writableSource(c, "", path)
	if err != nil || back || chosen.ID != s.ID {
		t.Fatal("creation cancel did not return to chooser", chosen, back, err)
	}
	db := filepath.Join(t.TempDir(), "other.db")
	scriptTUI(t, "create", "enter\nOther", "enter\n"+db, "yes")
	chosen, back, err = writableSource(c, "", path)
	if err != nil || back || chosen.Name != "Other" {
		t.Fatal(chosen, back, err)
	}
	loaded, err := loadConfig(path)
	if err != nil || len(loaded.Sources) != 2 {
		t.Fatal(loaded, err)
	}
	// Cancelling the host editor leaves the explicitly created empty source.
	state := PickerState{ConfigPath: path}
	db = filepath.Join(t.TempDir(), "third.db")
	scriptTUI(t, "create", "enter\nThird", "enter\n"+db, "yes", "back")
	if err = editInventoryHost(loaded, &state, nil); err != nil {
		t.Fatal(err)
	}
	loaded, err = loadConfig(path)
	if err != nil || len(loaded.Sources) != 3 {
		t.Fatal(loaded, err)
	}
	records, err := sourceConnections(loaded.Sources[2])
	if err != nil || len(records) != 0 {
		t.Fatal(records, err)
	}
}

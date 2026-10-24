package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestProfileReassignDeletePreservesAttachment(t *testing.T) {
	s := testInventory(t, "Inventory")
	if err := changeConnection(s, Connection{ID: "host", Alias: "host", Address: "192.0.2.1", Port: 22}, false); err != nil {
		t.Fatal(err)
	}
	rows, err := sourceConnections(s)
	if err != nil {
		t.Fatal(err)
	}
	old := *rows[0].OriginalProfile
	replacement := Connection{ID: "replacement", ProfileName: "Replacement", Type: "ssh", Port: 2222}
	if err = saveReusableProfile(s, replacement); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "profile.db")
	original, before, c, err := settingsDraft(path)
	if err != nil {
		t.Fatal(err)
	}
	c.Sources = []InventorySource{s}
	c.PersonalConnections = map[string]map[string]PersonalConnection{s.ID: {rows[0].HostID + ":ssh": {Profile: &ProfileReference{s.ID, old.ID}}}}
	if err = saveSettingsDraft(path, original, before, c); err != nil {
		t.Fatal(err)
	}
	uses, err := profileUsage(c, s, old.ID)
	if err != nil || len(uses) != 2 || !strings.Contains(strings.Join(uses, "\n"), "host — Inventory") {
		t.Fatal(uses, err)
	}
	if err = reassignDeleteProfile(path, s, old, replacement); err != nil {
		t.Fatal(err)
	}
	after, err := sourceConnections(s)
	if err != nil {
		t.Fatal(err)
	}
	if after[0].ID != rows[0].ID || after[0].HostID != rows[0].HostID || after[0].Default != rows[0].Default || after[0].Port != 2222 {
		t.Fatal(after)
	}
	loaded, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.PersonalConnections[s.ID][rows[0].HostID+":ssh"].Profile.ID != replacement.ID {
		t.Fatal("personal selection not reassigned")
	}
	profiles, _ := sourceProfiles(s)
	for _, p := range profiles {
		if p.ID == old.ID {
			t.Fatal("old profile not deleted")
		}
	}
}

func TestDeleteReviewNoDoesNotDelete(t *testing.T) {
	s := testInventory(t, "Profiles")
	p := Connection{ID: "p", ProfileName: "Profile", Port: 22}
	if err := saveReusableProfile(s, p); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "profile.db")
	c := defaultConfig()
	c.Sources = []InventorySource{s}
	scriptTUI(t, "no")
	deleted, err := reviewDeleteProfile(c, path, s, p)
	if err != nil || deleted {
		t.Fatal(deleted, err)
	}
	profiles, err := sourceProfiles(s)
	if err != nil || len(profiles) != 1 {
		t.Fatal(profiles, err)
	}
}

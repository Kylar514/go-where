package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestSettingsDatabaseMigrationAndStaleSave(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, "config.json")
	path := filepath.Join(dir, "profile.db")
	data := []byte("{ // original settings\n\"startup_mode\":\"rdp\",\"identities\":{\"work\":{\"user\":\"operator\"}}}")
	if err := os.WriteFile(legacy, data, 0600); err != nil {
		t.Fatal(err)
	}
	c, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.StartupMode != "rdp" || c.Identities["work"].User != "operator" {
		t.Fatal("migration lost settings", c)
	}
	original, before, draft, err := settingsDraft(path)
	if err != nil {
		t.Fatal(err)
	}
	draft.StartupMode = "ssh"
	if err = saveSettingsDraft(path, original, before, draft); err != nil {
		t.Fatal(err)
	}
	if err = saveSettingsDraft(path, original, before, c); err == nil {
		t.Fatal("stale draft accepted")
	}
	if b, _ := os.ReadFile(legacy); !bytes.Equal(b, data) {
		t.Fatal("legacy backup modified")
	}
	loaded, err := loadConfig(path)
	if err != nil || loaded.StartupMode != "ssh" {
		t.Fatal("settings not persisted", err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm()&0077 != 0 {
		t.Fatal("private database permissions", err)
	}
}

func TestSettingsDatabaseRejectsInventoryAndInvalidImport(t *testing.T) {
	s := testInventory(t, "Not settings")
	if _, err := loadConfig(s.Path); err == nil {
		t.Fatal("inventory accepted as settings")
	}
	dir := t.TempDir()
	legacy := filepath.Join(dir, "config.json")
	path := filepath.Join(dir, "profile.db")
	if err := os.WriteFile(legacy, []byte("{bad"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadConfig(path); err == nil {
		t.Fatal("invalid legacy silently ignored")
	}
	if err := os.WriteFile(legacy, []byte("{\"startup_mode\":\"rdp\"}"), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := loadConfig(path)
	if err != nil || c.StartupMode != "rdp" {
		t.Fatal("failed import could not retry", err)
	}
}

package main

import (
	"bytes"
	"path/filepath"
	"testing"
)

func TestRDPIdentityRenamePreservesPasswordAndOnlySavesIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profile.db")
	original, before, c, err := settingsDraft(path)
	if err != nil {
		t.Fatal(err)
	}
	key := bytes.Repeat([]byte{9}, 32)
	password, err := encryptIdentityPassword(key, "key", "stable", "secret")
	if err != nil {
		t.Fatal(err)
	}
	a := RDPAccount{ID: "stable", Name: "old", User: "operator", Password: &password}
	c.RDP.Accounts = []RDPAccount{a}
	c.PersonalConnections = map[string]map[string]PersonalConnection{"source": {"host:rdp": {Identities: []string{"old"}, DefaultIdentity: "old"}}}
	if err = saveSettingsDraft(path, original, before, c); err != nil {
		t.Fatal(err)
	}
	c.StartupMode = "rdp"
	if err = saveIdentityEditor(&c, "rdp", "old", "new", LocalIdentity{}, a); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.StartupMode != "" || loaded.RDP.Accounts[0].Name != "new" || loaded.RDP.Accounts[0].ID != "stable" {
		t.Fatal(loaded.RDP.Accounts, loaded.StartupMode)
	}
	plain, err := decryptIdentityPassword(key, "stable", *loaded.RDP.Accounts[0].Password)
	if err != nil || plain != "secret" {
		t.Fatal("password changed", err)
	}
	if loaded.PersonalConnections["source"]["host:rdp"].DefaultIdentity != "new" {
		t.Fatal("reference not renamed")
	}
}

func TestSSHIdentityRenameUpdatesLegacyAssignments(t *testing.T) {
	c := defaultConfig()
	c.Identities = map[string]LocalIdentity{"old": {User: "operator"}}
	c.IdentityAssignments = map[string]SourceIdentities{"s": {Default: "old", Profiles: map[string]string{"p": "old"}}}
	if err := saveIdentityEditor(&c, "ssh", "old", "new", c.Identities["old"], RDPAccount{}); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Identities["old"]; ok {
		t.Fatal("old identity remains")
	}
	if c.IdentityAssignments["s"].Profiles["p"] != "new" || c.IdentityAssignments["s"].Default != "new" {
		t.Fatal(c.IdentityAssignments)
	}
}

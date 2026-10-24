package main

import "testing"

func TestProfileDeletionProtectsReferencesAndReadOnly(t *testing.T) {
	s := testInventory(t, "Profiles")
	p := Connection{ID: "unused", ProfileName: "Unused", Port: 22}
	if err := saveReusableProfile(s, p); err != nil {
		t.Fatal(err)
	}
	c := defaultConfig()
	c.Sources = []InventorySource{s}
	c.PersonalConnections = map[string]map[string]PersonalConnection{s.ID: {"h:ssh": {Profile: &ProfileReference{s.ID, p.ID}}}}
	if err := deleteConnectionProfile(c, "", s, p.ID); err == nil {
		t.Fatal("referenced profile deleted")
	}
	c.PersonalConnections = nil
	ro := s
	ro.ReadOnly = true
	if err := deleteConnectionProfile(c, "", ro, p.ID); err == nil {
		t.Fatal("read-only profile deleted")
	}
	if err := deleteConnectionProfile(c, "", s, p.ID); err != nil {
		t.Fatal(err)
	}
	profiles, _ := sourceProfiles(s)
	for _, p := range profiles {
		if p.ID == "unused" {
			t.Fatal("profile not deleted")
		}
	}
}

func TestIdentityReferenceGuard(t *testing.T) {
	c := defaultConfig()
	c.PersonalConnections = map[string]map[string]PersonalConnection{"source": {"host:rdp": {Identities: []string{"work"}, DefaultIdentity: "work"}}}
	if !identityReferenced(c, "rdp", "work") || identityReferenced(c, "ssh", "work") {
		t.Fatal("identity protocol guard")
	}
	c.IdentityAssignments = map[string]SourceIdentities{"source": {Default: "ssh-work"}}
	if !identityReferenced(c, "ssh", "ssh-work") {
		t.Fatal("legacy assignment ignored")
	}
}

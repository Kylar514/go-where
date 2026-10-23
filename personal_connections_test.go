package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestPersonalProfileAndIdentityAcrossSources(t *testing.T) {
	a := testInventory(t, "Hosts")
	b := testInventory(t, "Profiles")
	if err := changeConnection(a, Connection{ID: "host-connection", Alias: "host", Address: "192.0.2.1", Port: 22, User: "legacy"}, false); err != nil {
		t.Fatal(err)
	}
	p := Connection{ID: "alternate", ProfileName: "Alternate", Type: "ssh", Port: 2222, SSHAgent: "custom"}
	if err := saveReusableProfile(b, p); err != nil {
		t.Fatal(err)
	}
	rows, err := sourceConnections(a)
	if err != nil {
		t.Fatal(err)
	}
	r := rows[0]
	c := defaultConfig()
	c.Sources = []InventorySource{a, b}
	c.Identities = map[string]LocalIdentity{"work": {User: "operator", IdentityFile: "/keys/work"}}
	c.PersonalConnections = map[string]map[string]PersonalConnection{a.ID: {personalKey(r): {Profile: &ProfileReference{b.ID, p.ID}, Identities: []string{"work"}, DefaultIdentity: "work"}}}
	got, err := applyPersonalConnection(c, a.ID, r)
	if err != nil {
		t.Fatal(err)
	}
	if got.Port != 2222 || got.User != "operator" || got.IdentityFile != "/keys/work" || got.SSHAgent != "custom" || got.ID != r.ID || got.HostID != r.HostID {
		t.Fatal(got)
	}
	unchanged, err := sourceConnections(a)
	if err != nil || unchanged[0].Port != 22 || unchanged[0].User != "legacy" {
		t.Fatal("personal selections modified source", err)
	}
	c.DisabledSources = []string{b.ID}
	if _, err = applyPersonalConnection(c, a.ID, r); err == nil {
		t.Fatal("disabled selected source silently substituted")
	}
}

func TestPersonalDefaultMustBeSelected(t *testing.T) {
	c := defaultConfig()
	c.PersonalConnections = map[string]map[string]PersonalConnection{"source": {"host:ssh": {Identities: []string{"default"}, DefaultIdentity: "missing"}}}
	if validatePersonalConnections(c) == nil {
		t.Fatal("unselected default accepted")
	}
}

func TestReadOnlySourceAllowsPersonalSelectionNotWrites(t *testing.T) {
	s := testInventory(t, "Read only")
	if err := changeConnection(s, Connection{ID: "c", Alias: "host", Address: "192.0.2.1", Port: 22}, false); err != nil {
		t.Fatal(err)
	}
	s.ReadOnly = true
	rows, err := sourceConnections(s)
	if err != nil || len(rows) != 1 {
		t.Fatal(rows, err)
	}
	if err = saveReusableProfile(s, Connection{ID: "p", ProfileName: "Profile", Port: 22}); err == nil {
		t.Fatal("read-only source accepted write")
	}
	c := defaultConfig()
	c.Sources = []InventorySource{s}
	c.LocalProfiles = map[string]Connection{"p": {ID: "p", ProfileName: "Local", Port: 2222}}
	c.PersonalConnections = map[string]map[string]PersonalConnection{s.ID: {personalKey(rows[0]): {Profile: &ProfileReference{"local", "p"}}}}
	got, err := applyPersonalConnection(c, s.ID, rows[0])
	if err != nil || got.Port != 2222 {
		t.Fatal(got, err)
	}
}

func TestPersonalSingleAndBulkUseSameEditor(t *testing.T) {
	s := testInventory(t, "Hosts")
	for _, r := range []Connection{{ID: "a", Alias: "a", Address: "192.0.2.1", Port: 22}, {ID: "b", Alias: "b", Address: "192.0.2.2", Port: 22}} {
		if err := changeConnection(s, r, false); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(t.TempDir(), "profile.db")
	original, before, draft, err := settingsDraft(path)
	if err != nil {
		t.Fatal(err)
	}
	draft.Sources = []InventorySource{s}
	draft.LocalProfiles = map[string]Connection{"local-profile": {ID: "local-profile", ProfileName: "Custom", Port: 2222}}
	if err = saveSettingsDraft(path, original, before, draft); err != nil {
		t.Fatal(err)
	}
	rows, err := sourceConnections(s)
	if err != nil {
		t.Fatal(err)
	}
	var hosts []Host
	for _, r := range rows {
		hosts = append(hosts, Host{SourceID: s.ID, ConnectionID: r.ID, InventoryHostID: r.HostID})
	}
	scriptTUI(t, "ssh-profile", "local:local-profile", "ssh-identities", "toggle:default,done", "default", "save", "save", "back")
	if err = editPersonalHosts(draft, path, hosts); err != nil {
		t.Fatal(err)
	}
	c, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		selection := c.PersonalConnections[s.ID][personalKey(r)]
		if selection.Profile == nil || selection.Profile.ID != "local-profile" || selection.DefaultIdentity != "default" {
			t.Fatal(selection)
		}
		if _, ok := c.PersonalConnections[s.ID][r.HostID+":rdp"]; ok {
			t.Fatal("untouched protocol modified")
		}
	}
}

func TestManagedSnapshotUsesCrossSourcePersonalProfile(t *testing.T) {
	s := testInventory(t, "Snapshot")
	if err := changeConnection(s, Connection{ID: "c", Alias: "host", Address: "192.0.2.1", Port: 22, User: "legacy"}, false); err != nil {
		t.Fatal(err)
	}
	rows, err := sourceConnections(s)
	if err != nil {
		t.Fatal(err)
	}
	r := rows[0]
	c := defaultConfig()
	c.Sources = []InventorySource{s}
	c.LocalProfiles = map[string]Connection{"local": {ID: "local", ProfileName: "Local", Port: 2222, SSHAgent: "none"}}
	c.PersonalConnections = map[string]map[string]PersonalConnection{s.ID: {personalKey(r): {Profile: &ProfileReference{"local", "local"}, Identities: []string{"default"}, DefaultIdentity: "default"}}}
	j, err := managedJob(Host{SourceID: s.ID, ConnectionID: r.ID}, c)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(j.Configuration, "Port 2222") || !strings.Contains(j.Configuration, `IdentityAgent "none"`) || strings.Contains(j.Configuration, "User ") {
		t.Fatal(j.Configuration)
	}
}

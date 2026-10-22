package main

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSharedHostProfilesReferencesAndOverrides(t *testing.T) {
	s := testInventory(t, "Hosts")
	h := InventoryHost{ID: "server", Alias: "server", Address: "192.0.2.1", Tags: []string{"lab"}}
	ssh := Connection{ID: "ssh", Alias: "ssh", HostID: h.ID, ProfileName: "Default", Default: true, Type: "ssh", Port: 22, User: "operator"}
	admin := Connection{ID: "admin", Alias: "admin", HostID: h.ID, ProfileName: "Admin", Type: "ssh", Port: 2222, User: "admin"}
	rdp := Connection{ID: "rdp", Alias: "rdp", HostID: h.ID, ProfileName: "Default", Default: true, Type: "rdp", Port: 3389, User: "desktop-user", JumpID: admin.ID}
	profiles := []Connection{ssh, admin, rdp}
	if err := saveInventoryHost(s, h, profiles); err != nil {
		t.Fatal(err)
	}
	c := defaultConfig()
	c.Sources = []InventorySource{s}
	rows, err := inventoryHosts(c)
	if err != nil || len(rows) != 2 || rows[0].Alias != "server" || rows[1].Alias != "server" {
		t.Fatalf("default rows: %+v %v", rows, err)
	}
	ref := Host{SourceID: s.ID, ConnectionID: ssh.ID, Alias: h.Alias, Type: "ssh"}
	m := newPickerModel(rows, "", "current", Origin{}, PickerState{Mode: "ssh"}, c)
	m.finish("connect-as")
	if len(m.selected) != 1 || m.key != "connect-as" {
		t.Fatal("Connect as lost picker selection")
	}
	scriptTUI(t, "admin")
	chosen, back, err := chooseHostProfiles(c, []Host{ref})
	if err != nil || back || chosen[0].ConnectionID != admin.ID {
		t.Fatal(chosen, back, err)
	}
	scriptTUI(t, "back")
	if _, back, err = chooseHostProfiles(c, []Host{ref}); err != nil || !back {
		t.Fatal("profile cancellation", err)
	}
	// Changing a default never repoints saved routes.
	profiles[0].Default = false
	profiles[1].Default = true
	h.Address = "192.0.2.2"
	if err = saveInventoryHost(s, h, profiles); err != nil {
		t.Fatal(err)
	}
	rjob, err := hostRDPJob(c, Host{SourceID: s.ID, ConnectionID: rdp.ID, Type: "rdp"}, "")
	if err != nil || rjob.Host != h.Address || !strings.Contains(rjob.SSHSnapshot, `User "admin"`) || !strings.Contains(rjob.SSHSnapshot, "Port 2222") {
		t.Fatalf("RDP reference: %+v %v", rjob, err)
	}
	tunnel := Connection{ID: "tunnel", Alias: "forward", Type: "tunnel", TunnelType: "ssh", SSHID: ssh.ID, RemoteHostID: h.ID, Bind: "127.0.0.1", LocalPort: 8080, RemotePort: 80}
	if err = changeConnection(s, tunnel, false); err != nil {
		t.Fatal(err)
	}
	local := "192.0.2.3"
	c.HostOverrides = map[string]map[string]HostOverride{s.ID: {h.ID: {Address: &local}}}
	job, err := managedJob(ref, c)
	if err != nil || !strings.Contains(job.Configuration, local) {
		t.Fatal(job, err)
	}
	rjob, err = hostRDPJob(c, Host{SourceID: s.ID, ConnectionID: rdp.ID, Type: "rdp"}, "")
	if err != nil || rjob.Host != local {
		t.Fatal("RDP ignored shared override", rjob, err)
	}
	conn, err := inventoryTunnel(Host{SourceID: s.ID, ConnectionID: tunnel.ID, Type: "tunnel"}, c)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := base64.RawURLEncoding.DecodeString(conn.Resource)
	var spec inventoryTunnelSpec
	if err = json.Unmarshal(data, &spec); err != nil || !strings.Contains(spec.Configuration, `User "operator"`) || !strings.Contains(strings.Join(spec.Argv, " "), local+":80") {
		t.Fatal("tunnel did not reuse shared host", spec, err)
	}
	// Profile overrides remain more specific than shared host overrides.
	specific := "192.0.2.4"
	c.Overrides = map[string]map[string]ConnectionOverride{s.ID: {ssh.ID: {Address: &specific}}}
	job, err = managedJob(ref, c)
	if err != nil || !strings.Contains(job.Configuration, specific) {
		t.Fatal(job, err)
	}
	// Reject removing a referenced profile, a host, duplicate names or invalid defaults.
	if saveInventoryHost(s, h, []Connection{profiles[1], profiles[2]}) == nil {
		t.Fatal("removed tunnel's SSH profile")
	}
	if deleteInventoryHost(s, h.ID) == nil {
		t.Fatal("deleted referenced host")
	}
	bad := append([]Connection(nil), profiles...)
	bad[0].ProfileName = "Admin"
	if saveInventoryHost(s, h, bad) == nil {
		t.Fatal("duplicate profile name accepted")
	}
	bad = append([]Connection(nil), profiles...)
	bad[0].Default = true
	if saveInventoryHost(s, h, bad) == nil {
		t.Fatal("multiple defaults accepted")
	}
	bad = append([]Connection(nil), profiles...)
	bad[1].Default = false
	if saveInventoryHost(s, h, bad) == nil {
		t.Fatal("missing default accepted")
	}
	rows2, err := sourceConnections(s)
	if err != nil || len(rows2) != 4 {
		t.Fatal("failed save lost records", rows2, err)
	}
	// Address exists only in hosts storage, not duplicate profile JSON.
	db, err := openInventory(s)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var raw string
	if err = db.QueryRow("SELECT data FROM connections WHERE id='ssh'").Scan(&raw); err != nil || strings.Contains(raw, "192.0.2.") {
		t.Fatal("duplicated address", raw, err)
	}
}

func TestHostMigrationPreservesIDsWithoutMerging(t *testing.T) {
	for _, version := range []int{1, 2, 3, 4, 5} {
		t.Run(string(rune('0'+version)), func(t *testing.T) {
			s := testInventory(t, "Migration")
			db, err := openInventory(s)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = db.Exec("DROP TABLE hosts"); err != nil {
				t.Fatal(err)
			}
			if _, err = db.Exec("DROP TABLE profiles"); err != nil {
				t.Fatal(err)
			}
			a := Connection{ID: "a", Alias: "a", Address: "192.0.2.1", Port: 22, User: "operator", SSHAgent: "legacy", ExtraHostKeyAlgorithms: []string{"ssh-rsa"}, ExtraPubkeyAlgorithms: []string{"ssh-rsa"}}
			b := Connection{ID: "b", Alias: "b", Address: a.Address, Port: 22, JumpID: a.ID}
			for _, r := range []Connection{a, b} {
				data, _ := json.Marshal(r)
				if _, err = db.Exec("INSERT INTO connections(id,alias,data) VALUES(?,?,?)", r.ID, r.Alias, string(data)); err != nil {
					t.Fatal(err)
				}
			}
			if _, err = db.Exec("PRAGMA user_version=" + string(rune('0'+version))); err != nil {
				t.Fatal(err)
			}
			db.Close()
			records, err := sourceConnections(s)
			if err != nil || len(records) != 2 {
				t.Fatal(records, err)
			}
			hosts, err := sourceInventoryHosts(s)
			if err != nil || len(hosts) != 2 {
				t.Fatal("same IP merged", hosts, err)
			}
			if records[0].ID != "a" || records[0].HostID != "a" || !records[0].Default || records[0].SSHAgent != "legacy" || len(records[0].ExtraPubkeyAlgorithms) != 1 || records[1].JumpID != "a" {
				t.Fatal("migration lost settings", records)
			}
		})
	}
}

func TestHostEditorCreatesMultipleProtocolsAndCancel(t *testing.T) {
	s := testInventory(t, "Editor")
	c := defaultConfig()
	c.Sources = []InventorySource{s}
	state := PickerState{Source: s.ID, Mode: "ssh"}
	scriptTUI(t, s.ID, "alias", "enter\nserver", "address", "enter\n192.0.2.1", "ssh", "add", "enter\nDefault", "settings", "user", "enter\noperator", "save", "back", "add", "enter\nAdmin", "settings", "user", "enter\nadmin", "port", "enter\n2222", "save", "back", "back", "rdp", "add", "enter\nAdmin", "settings", "user", "enter\ndesktop-admin", "save", "back", "back", "save-host", "back")
	if err := editInventoryHost(c, &state, nil); err != nil {
		t.Fatal(err)
	}
	records, err := sourceConnections(s)
	if err != nil || len(records) != 3 {
		t.Fatal(records, err)
	}
	hosts, err := sourceInventoryHosts(s)
	if err != nil || len(hosts) != 1 {
		t.Fatal(hosts, err)
	}
	if records[0].HostID != records[1].HostID || records[1].HostID != records[2].HostID {
		t.Fatal("profiles duplicated host")
	}
	rows, err := inventoryHosts(c)
	if err != nil || len(rows) != 2 {
		t.Fatal(rows, err)
	}
	scriptTUI(t, "address", "enter\n192.0.2.9", "back")
	if err = editInventoryHost(c, &state, &rows[0]); err != nil {
		t.Fatal(err)
	}
	hosts, err = sourceInventoryHosts(s)
	if err != nil || hosts[0].Address != "192.0.2.1" {
		t.Fatal("cancel wrote host", hosts, err)
	}
	// Bare hosts remain visible and editable before protocols are configured.
	scriptTUI(t, s.ID, "alias", "enter\nbare", "address", "enter\n192.0.2.5", "save-host", "back")
	if err = editInventoryHost(c, &state, nil); err != nil {
		t.Fatal(err)
	}
	rows, err = inventoryHosts(c)
	if err != nil || len(rows) != 3 {
		t.Fatal("bare host hidden", rows, err)
	}
	for _, row := range rows {
		if row.Type == "host" {
			scriptTUI(t, "delete", "delete")
			if err = editInventoryHost(c, &state, &row); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestSharedHostOverrideSettingsRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	original := []byte("{ // keep\n}")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	before, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	draft := cloneAgentSettings(before)
	address := "192.0.2.10"
	draft.HostOverrides = map[string]map[string]HostOverride{"source": {"host": {Address: &address}}}
	if err = saveSettingsDraft(path, original, before, draft); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadConfig(path)
	if err != nil || *loaded.HostOverrides["source"]["host"].Address != address {
		t.Fatal(loaded, err)
	}
	copy := cloneAgentSettings(loaded)
	copy.HostOverrides["source"]["host"] = HostOverride{}
	if loaded.HostOverrides["source"]["host"].Address == nil {
		t.Fatal("draft mutated original")
	}
}

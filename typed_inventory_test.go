package main

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestTypedInventoryMigrationAndReferences(t *testing.T) {
	s := testInventory(t, "Types")
	old := Connection{ID: "legacy", Alias: "old", Address: "192.0.2.1", Port: 22, User: "${USER}", Tags: []string{"lab"}}
	if err := changeConnection(s, old, false); err != nil {
		t.Fatal(err)
	}
	db, err := openInventory(s)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(old)
	if _, err = db.Exec("UPDATE connections SET data=? WHERE id=?", string(data), old.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("DROP TABLE hosts"); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("PRAGMA user_version=1"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	records, err := sourceConnections(s)
	if err != nil || len(records) != 1 || records[0].Type != "ssh" || records[0].ID != old.ID || records[0].User != old.User || records[0].Tags[0] != "lab" {
		t.Fatalf("lossy migration: %v %v", records, err)
	}
	rdp := Connection{ID: "desktop", Type: "rdp", Alias: "desktop", Address: "192.0.2.2", Port: 3389, JumpID: old.ID}
	tun := Connection{ID: "forward", Type: "tunnel", Alias: "web-forward", TunnelType: "ssh", Bind: "127.0.0.10", LocalPort: 8080, RemoteHost: "192.0.2.3", RemotePort: 80, SSHID: old.ID}
	for _, r := range []Connection{rdp, tun} {
		if err = changeConnection(s, r, false); err != nil {
			t.Fatal(err)
		}
	}
	if err = changeConnection(s, old, true); err == nil {
		t.Fatal("deleted referenced SSH host")
	}
	old.Type = "rdp"
	if err = changeConnection(s, old, false); err == nil {
		t.Fatal("changed referenced SSH host into RDP")
	}
	rdp.JumpID = rdp.ID
	if err = changeConnection(s, rdp, false); err == nil {
		t.Fatal("RDP used as SSH jump")
	}
	bad := tun
	bad.ID = "bad"
	bad.Alias = "bad"
	bad.RemotePort = 0
	if err = changeConnection(s, bad, false); err == nil {
		t.Fatal("invalid tunnel saved")
	}
	db, err = openInventory(s)
	if err != nil {
		t.Fatal(err)
	}
	var version int
	db.QueryRow("PRAGMA user_version").Scan(&version)
	if version != 6 {
		t.Fatal("migration did not update version")
	}
	db.Exec("PRAGMA user_version=99")
	db.Close()
	if _, err = openInventory(s); err == nil {
		t.Fatal("future schema modified")
	}
}

func TestMigrationFailureRollsBack(t *testing.T) {
	s := testInventory(t, "Broken")
	db, err := openInventory(s)
	if err != nil {
		t.Fatal(err)
	}
	r := Connection{ID: "old", Alias: "old", Address: "192.0.2.1", Port: 22, JumpID: "missing"}
	b, _ := json.Marshal(r)
	db.Exec("INSERT INTO connections(id,alias,data) VALUES(?,?,?)", r.ID, r.Alias, string(b))
	db.Exec("PRAGMA user_version=1")
	db.Close()
	if _, err = openInventory(s); err == nil {
		t.Fatal("invalid graph migrated")
	}
	db, err = sql.Open("sqlite", s.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var version int
	db.QueryRow("PRAGMA user_version").Scan(&version)
	if version != 1 {
		t.Fatal("failed migration changed schema")
	}
}

func TestDynamicEditorCreatesRDPAndTunnel(t *testing.T) {
	s := testInventory(t, "Editor")
	c := defaultConfig()
	c.Sources = []InventorySource{s}
	state := PickerState{Source: s.ID, Mode: "ssh"}
	scriptTUI(t, "type", "rdp", "alias", "enter\ndesktop", "address", "enter\n192.0.2.2", "domain", "enter\nLAB", "save")
	if err := editConnection(c, &state, nil); err != nil {
		t.Fatal(err)
	}
	if state.Mode != "rdp" {
		t.Fatal("saved RDP did not focus its mode")
	}
	scriptTUI(t, "type", "tunnel", "tunnel_type", "kubectl", "alias", "enter\nweb", "resource", "enter\nservice/web", "save")
	if err := editConnection(c, &state, nil); err != nil {
		t.Fatal(err)
	}
	if state.Mode != "tunnels" {
		t.Fatal("saved tunnel did not focus its mode")
	}
	records, err := sourceConnections(s)
	if err != nil || len(records) != 2 {
		t.Fatalf("typed save: %v %v", records, err)
	}
	for _, r := range records {
		fields := connectionFields(r, s, c)
		keys := map[string]bool{}
		for _, f := range fields {
			keys[f.ID] = true
		}
		if !keys["type"] || !keys["source"] || keys["key"] {
			t.Fatal("dynamic common/type fields wrong")
		}
		if r.kind() == "rdp" && (!keys["domain"] || keys["resource"] || r.Port != 3389) {
			t.Fatal("RDP fields/defaults wrong")
		}
		if r.kind() == "tunnel" && (!keys["resource"] || keys["domain"] || keys["ssh_id"]) {
			t.Fatal("kubectl fields wrong")
		}
	}
	// Cancel a type conversion without changing the stored record.
	var desktop Connection
	for _, r := range records {
		if r.kind() == "rdp" {
			desktop = r
		}
	}
	h := Host{SourceID: s.ID, ConnectionID: desktop.ID, Type: "rdp"}
	scriptTUI(t, "type", "ssh", "change", "back")
	if err = editConnection(c, &state, &h); err != nil {
		t.Fatal(err)
	}
	records, _ = sourceConnections(s)
	for _, r := range records {
		if r.ID == desktop.ID && r.kind() != "rdp" {
			t.Fatal("cancel saved type change")
		}
	}
}

func TestManagedRDPAndTunnelResolution(t *testing.T) {
	s := testInventory(t, "Resolved")
	domain := "${DOMAIN}"
	ssh := Connection{ID: "gateway", Type: "ssh", Alias: "gateway", Address: "gateway.example", Port: 2222, User: "gateway-user", IdentityFile: filepath.Join(t.TempDir(), "key")}
	rdp := Connection{ID: "desktop", Type: "rdp", Alias: "desktop", Address: "192.0.2.2", Port: 3390, User: "record-user", Domain: &domain, JumpID: ssh.ID}
	tun := Connection{ID: "web", Type: "tunnel", Alias: "web", TunnelType: "ssh", SSHID: ssh.ID, Bind: "127.0.0.10", LocalPort: 8089, RemoteHost: "192.0.2.2", RemotePort: 80}
	for _, r := range []Connection{ssh, rdp, tun} {
		if err := changeConnection(s, r, false); err != nil {
			t.Fatal(err)
		}
	}
	c := defaultConfig()
	c.Sources = []InventorySource{s}
	c.Variables = map[string]string{"DOMAIN": "LAB"}
	c.RDP.PasswordCommand = []string{"never-reuse-default-secret"}
	h := Host{Alias: rdp.Alias, Type: "rdp", SourceID: s.ID, ConnectionID: rdp.ID}
	j, err := hostRDPJob(c, h, "")
	if err != nil {
		t.Fatal(err)
	}
	if j.User != "record-user" || j.Domain != "LAB" || j.Port != 3390 || j.Jump != "gw-0" || !strings.Contains(j.SSHSnapshot, "gateway-user") || len(j.PasswordCommand) != 0 {
		t.Fatalf("RDP resolution wrong: %+v", j)
	}
	accountDomain := "OTHER"
	c.activeRDPAccount = RDPAccount{Name: "Chosen", User: "chosen-user", Domain: &accountDomain, PasswordCommand: []string{"chosen-secret"}}
	j, err = hostRDPJob(c, h, "")
	if err != nil || j.User != "chosen-user" || j.Domain != "OTHER" || len(j.PasswordCommand) == 0 {
		t.Fatal("explicit account did not override record")
	}
	h = Host{Alias: tun.Alias, Type: "tunnel", SourceID: s.ID, ConnectionID: tun.ID}
	conn, err := inventoryTunnel(h, c)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := base64.RawURLEncoding.DecodeString(conn.Resource)
	var spec inventoryTunnelSpec
	json.Unmarshal(data, &spec)
	if !strings.Contains(strings.Join(spec.Argv, " "), "127.0.0.10:8089:192.0.2.2:80") || !strings.Contains(spec.Configuration, "gateway-user") {
		t.Fatal("tunnel did not resolve SSH host/bind/target")
	}
	tun.Bind = "0.0.0.0"
	if err = changeConnection(s, tun, false); err == nil {
		t.Fatal("public bind saved without consent")
	}
	// Typed records appear only in their own mode; tunnel Ctrl+E opens the record.
	hosts, _ := inventoryHosts(c)
	m := newPickerModel(hosts, "", "terminal", Origin{}, PickerState{Mode: "rdp", Source: s.ID}, c)
	if len(m.visible) != 1 || m.hosts[m.visible[0]].Type != "rdp" {
		t.Fatal("typed RDP filtering failed")
	}
	m.switchMode("tunnels")
	if len(m.visible) != 1 {
		t.Fatal("inventory tunnel missing")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlE})
	if m.key != "ctrl-e" || len(m.selected) != 1 || m.selected[0].ConnectionID != tun.ID {
		t.Fatal("tunnel edit still routed to YAML")
	}
	// Direct RDP reaches the existing session runner without SSH-config dependence.
	rdp.JumpID = ""
	changeConnection(s, rdp, false)
	dir := t.TempDir()
	t.Setenv("CALLS", filepath.Join(dir, "calls"))
	fakeCommand(t, dir, "rdp-client", `printf '%s\n' "$@" > "$CALLS"`)
	c.activeRDPAccount = RDPAccount{}
	c.RDP.Client = []string{filepath.Join(dir, "rdp-client")}
	j, err = hostRDPJob(c, Host{Alias: rdp.Alias, Type: "rdp", SourceID: s.ID, ConnectionID: rdp.ID}, "")
	if err != nil {
		t.Fatal(err)
	}
	if err = runRDPSession(j); err != nil {
		t.Fatal(err)
	}
	out, _ := os.ReadFile(filepath.Join(dir, "calls"))
	if !strings.Contains(string(out), "/v:192.0.2.2:3390") || !strings.Contains(string(out), "/u:record-user") {
		t.Fatalf("managed RDP did not reach client: %s", out)
	}
}

package main

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func testInventory(t *testing.T, name string) InventorySource {
	t.Helper()
	s, err := createInventory(name, filepath.Join(t.TempDir(), "inventory.db"))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSQLiteSourcesAndGraph(t *testing.T) {
	a, b := testInventory(t, "Personal"), testInventory(t, "Lab")
	missing := a
	missing.Path = filepath.Join(t.TempDir(), "missing.db")
	if _, err := sourceConnections(missing); err == nil {
		t.Fatal("missing DB opened")
	}
	if _, err := os.Stat(missing.Path); !os.IsNotExist(err) {
		t.Fatal("opening a source created a DB")
	}
	if _, err := createInventory("replace", a.Path); err == nil {
		t.Fatal("overwrote existing DB")
	}
	jump := Connection{ID: "jump", Alias: "gateway", Address: "gateway.example", Port: 2222}
	host := Connection{ID: "host", Alias: "web", Address: "192.0.2.40", Port: 22, JumpID: jump.ID}
	for _, r := range []Connection{jump, host} {
		if err := changeConnection(a, r, false); err != nil {
			t.Fatal(err)
		}
	}
	if err := changeConnection(b, Connection{ID: "other", Alias: "web", Address: "192.0.2.41", Port: 22}, false); err != nil {
		t.Fatal(err)
	}
	if err := changeConnection(a, jump, true); err == nil {
		t.Fatal("deleted referenced jump")
	}
	jump.JumpID = "host"
	if err := changeConnection(a, jump, false); err == nil {
		t.Fatal("saved jump cycle")
	}
	jump.JumpID = ""
	records, err := sourceConnections(a)
	if err != nil || len(records) != 2 {
		t.Fatalf("transaction rollback: %v %v", records, err)
	}
	c := defaultConfig()
	c.Sources = []InventorySource{a, b}
	hosts, err := inventoryHosts(c)
	if err != nil || len(hosts) != 3 {
		t.Fatalf("load sources: %v %v", hosts, err)
	}
	var one, two Host
	for _, h := range hosts {
		if h.Alias == "web" {
			if h.SourceID == a.ID {
				one = h
			} else {
				two = h
			}
		}
	}
	if hostIdentity(one) == hostIdentity(two) {
		t.Fatal("duplicate aliases share identity")
	}
	m := newPickerModel(hosts, "", "terminal", Origin{}, PickerState{Mode: "ssh", Source: b.ID}, c)
	if len(m.visible) != 1 {
		t.Fatal("source filter failed")
	}
	m.marked[hostIdentity(one)] = true
	m.marked[hostIdentity(two)] = true
	if selected := m.selection(false); len(selected) != 1 || selected[0].SourceID != b.ID {
		t.Fatal("marks crossed source filter")
	}
	m.switchMode("scp")
	if len(m.visible) != 0 {
		t.Fatal("managed records leaked into unsupported transfer mode")
	}
	settings := filepath.Join(t.TempDir(), "config.json")
	if err = registerSource(settings, a); err != nil {
		t.Fatal(err)
	}
	if err = registerSource(settings, b); err != nil {
		t.Fatal(err)
	}
	if err = registerSource(settings, a); err == nil {
		t.Fatal("registered source twice")
	}
	loaded, err := loadConfig(settings)
	if err != nil || len(loaded.Sources) != 2 {
		t.Fatalf("source registry did not persist: %v", err)
	}
	attached, err := attachInventory("Renamed", a.Path)
	if err != nil || attached.ID != a.ID {
		t.Fatalf("reattach identity: %v", err)
	}
	if err = changeConnection(a, host, true); err != nil {
		t.Fatal(err)
	}
	if err = changeConnection(a, jump, true); err != nil {
		t.Fatal(err)
	}
}

func TestManagedResolutionLaunchAndCleanup(t *testing.T) {
	s := testInventory(t, "Work")
	key := filepath.Join(t.TempDir(), "key with spaces")
	jump := Connection{ID: "jump", Alias: "gateway", Address: "gateway.example", Port: 2222, User: "${JUMPUSER}", IdentityFile: "${JUMPKEY}"}
	r := Connection{ID: "web", Alias: "web", Address: "192.0.2.40", Port: 2200, User: "${USER}", IdentityFile: "${KEY}", JumpID: jump.ID}
	for _, record := range []Connection{jump, r} {
		if err := changeConnection(s, record, false); err != nil {
			t.Fatal(err)
		}
	}
	c := defaultConfig()
	c.Sources = []InventorySource{s}
	c.Variables = map[string]string{"USER": "default-user", "KEY": key, "JUMPUSER": "gateway-user", "JUMPKEY": key + "-jump"}
	user := "override-user"
	c.Overrides = map[string]map[string]ConnectionOverride{s.ID: {r.ID: {User: &user}}}
	h := Host{Alias: r.Alias, SourceID: s.ID, ConnectionID: r.ID}
	j, err := managedJob(h, c)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`HostName "192.0.2.40"`, `User "override-user"`, `User "gateway-user"`, "Port 2222", "Port 2200", "ProxyJump gw-1", `IdentityFile "` + key + `"`} {
		if !strings.Contains(j.Configuration, want) {
			t.Fatalf("missing %s: %s", want, j.Configuration)
		}
	}
	delete(c.Variables, "JUMPKEY")
	if _, err = managedJob(h, c); err == nil || !strings.Contains(err.Error(), "JUMPKEY") {
		t.Fatal("missing jump variable not reported")
	}
	c.Variables["JUMPKEY"] = key + "-jump"
	c.Overrides[s.ID][r.ID] = ConnectionOverride{JumpID: &r.ID}
	if _, err = managedJob(h, c); err == nil {
		t.Fatal("override introduced jump cycle")
	}
	c.Overrides = nil
	bad := "192.0.2.1;touch /bad"
	c.Overrides = map[string]map[string]ConnectionOverride{s.ID: {r.ID: {Address: &bad}}}
	if _, err = managedJob(h, c); err == nil {
		t.Fatal("unsafe hostname accepted")
	}
	if _, err = resolveValue("${MISSING}", nil); err == nil {
		t.Fatal("unresolved template accepted")
	}
	if _, err = resolveValue("${X}", map[string]string{"X": "${Y}"}); err == nil {
		t.Fatal("recursive template accepted")
	}
	dir := t.TempDir()
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	t.Setenv("CALLS", filepath.Join(dir, "calls"))
	fakeCommand(t, dir, "ssh", `[ "$1" = -F ] || exit 3
[ "$3" = -- ] || exit 4
[ "$4" = gw-0 ] || exit 5
printf '%s\n' "$2" > "$CALLS"
cat "$2" >> "$CALLS"`)
	data, _ := json.Marshal(j)
	if err = runManagedSSH(base64.RawURLEncoding.EncodeToString(data)); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "calls"))
	snapshot, body, _ := strings.Cut(string(b), "\n")
	if body != j.Configuration {
		t.Fatal("worker changed resolved snapshot")
	}
	if _, err = os.Stat(snapshot); !os.IsNotExist(err) {
		t.Fatal("snapshot left behind after SSH exited")
	}
	fakeCommand(t, dir, "ssh", `printf '%s\n' "$2" > "$CALLS"; exit 1`)
	if err = runManagedSSH(base64.RawURLEncoding.EncodeToString(data)); err == nil {
		t.Fatal("SSH failure lost")
	}
	b, _ = os.ReadFile(filepath.Join(dir, "calls"))
	if _, err = os.Stat(strings.TrimSpace(string(b))); !os.IsNotExist(err) {
		t.Fatal("failed SSH leaked snapshot")
	}
}

func TestInventoryCreateEditViaTUI(t *testing.T) {
	settings := filepath.Join(t.TempDir(), "settings.json")
	path := filepath.Join(t.TempDir(), "personal.db")
	scriptTUI(t, "create", "enter\nPersonal", "enter\n"+path, "yes")
	state := PickerState{Mode: "ssh"}
	if err := sourceMenu(settings, defaultConfig(), &state); err != nil {
		t.Fatal(err)
	}
	c, err := loadConfig(settings)
	if err != nil {
		t.Fatal(err)
	}
	scriptTUI(t, "alias", "enter\nweb", "address", "enter\n192.0.2.10", "user", "enter\n${ADMIN}", "save")
	if err = editConnection(c, &state, nil); err != nil {
		t.Fatal(err)
	}
	records, err := sourceConnections(c.Sources[0])
	if err != nil || len(records) != 1 || records[0].User != "${ADMIN}" {
		t.Fatalf("editor did not persist record: %v %v", records, err)
	}
	h := Host{SourceID: c.Sources[0].ID, ConnectionID: records[0].ID}
	scriptTUI(t, "duplicate", "save")
	if err = editConnection(c, &state, &h); err != nil {
		t.Fatal(err)
	}
	records, err = sourceConnections(c.Sources[0])
	if err != nil || len(records) != 2 || records[0].ID == records[1].ID {
		t.Fatal("duplicate must create a new stable ID")
	}
	scriptTUI(t, "delete", "delete")
	if err = editConnection(c, &state, &h); err != nil {
		t.Fatal(err)
	}
	records, _ = sourceConnections(c.Sources[0])
	if len(records) != 1 {
		t.Fatal("delete did not persist")
	}
	// Source shortcuts do not trigger a connection launch.
	m := newPickerModel(nil, "", "terminal", Origin{}, state, c)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 25})
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlB})
	if m.key != "sources" {
		t.Fatal("missing source chooser shortcut")
	}
}

func TestManagedConfigWithOpenSSH(t *testing.T) {
	ssh, err := exec.LookPath("ssh")
	if err != nil {
		t.Skip("install OpenSSH to verify generated configuration with ssh -G")
	}
	s := testInventory(t, "OpenSSH check")
	key := filepath.Join(t.TempDir(), "key with spaces")
	for _, r := range []Connection{
		{ID: "jump", Alias: "gateway", Address: "127.0.0.1", Port: 1, User: "jump-user", IdentityFile: key + "-jump"},
		{ID: "target", Alias: "target", Address: "192.0.2.1", Port: 2222, User: "target-user", IdentityFile: key, JumpID: "jump"},
	} {
		if err = changeConnection(s, r, false); err != nil {
			t.Fatal(err)
		}
	}
	c := defaultConfig()
	c.Sources = []InventorySource{s}
	j, err := managedJob(Host{SourceID: s.ID, ConnectionID: "target"}, c)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "config with spaces")
	if err = os.WriteFile(path, []byte(j.Configuration), 0600); err != nil {
		t.Fatal(err)
	}
	for _, hop := range []struct{ alias, user, host, port, key string }{
		{"gw-0", "target-user", "192.0.2.1", "2222", key},
		{"gw-1", "jump-user", "127.0.0.1", "1", key + "-jump"},
	} {
		out, err := exec.Command(ssh, "-G", "-F", path, hop.alias).Output()
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"user " + hop.user + "\n", "hostname " + hop.host + "\n", "port " + hop.port + "\n", "identityfile " + hop.key + "\n"} {
			if !strings.Contains(string(out), want) {
				t.Fatalf("OpenSSH did not resolve %q: %s", want, out)
			}
		}
	}
}

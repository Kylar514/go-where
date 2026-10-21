package main

import (
	"encoding/base64"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPerHopSSHAgentSelection(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix socket fixture; Windows named-pipe support requires native validation")
	}
	dir := t.TempDir()
	endpoint := filepath.Join(dir, "legacy% agent.sock")
	listener, err := net.Listen("unix", endpoint)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	t.Setenv("GO_WHERE_TEST_AGENT", endpoint)
	s := testInventory(t, "Agents")
	jump := Connection{ID: "gateway", Alias: "gateway", Address: "192.0.2.1", Port: 22, SSHAgent: "none"}
	r := Connection{ID: "server", Alias: "server", Address: "192.0.2.2", Port: 22, JumpID: jump.ID, SSHAgent: "legacy"}
	for _, record := range []Connection{jump, r} {
		if err = changeConnection(s, record, false); err != nil {
			t.Fatal(err)
		}
	}
	c := defaultConfig()
	c.Sources = []InventorySource{s}
	c.SSHAgents = map[string]SSHAgentProfile{"legacy": {SocketEnv: "GO_WHERE_TEST_AGENT"}}
	h := Host{SourceID: s.ID, ConnectionID: r.ID}
	j, err := managedJob(h, c)
	if err != nil {
		t.Fatal(err)
	}
	destination, gateway, _ := strings.Cut(j.Configuration, "Host gw-1")
	if !strings.Contains(destination, `IdentityAgent "`+strings.ReplaceAll(endpoint, "%", "%%")+`"`) || !strings.Contains(gateway, `IdentityAgent "none"`) {
		t.Fatalf("per-hop agent settings lost: %s", j.Configuration)
	}
	if ssh, e := exec.LookPath("ssh"); e == nil {
		path := filepath.Join(dir, "ssh-config")
		if e = os.WriteFile(path, []byte(j.Configuration), 0600); e != nil {
			t.Fatal(e)
		}
		out, e := exec.Command(ssh, "-G", "-F", path, "gw-0").CombinedOutput()
		if e != nil || !strings.Contains(string(out), "identityagent ") {
			t.Fatalf("OpenSSH rejected agent config: %s %v", out, e)
		}
	}
	rdp := Connection{ID: "desktop", Type: "rdp", Alias: "desktop", Address: "192.0.2.3", Port: 3389, JumpID: r.ID}
	tun := Connection{ID: "forward", Type: "tunnel", Alias: "forward", TunnelType: "ssh", Bind: "127.0.0.1", LocalPort: 8089, RemoteHost: "localhost", RemotePort: 80, SSHID: r.ID}
	for _, record := range []Connection{rdp, tun} {
		if err = changeConnection(s, record, false); err != nil {
			t.Fatal(err)
		}
	}
	rdpJob, err := hostRDPJob(c, Host{Alias: rdp.Alias, Type: "rdp", SourceID: s.ID, ConnectionID: rdp.ID}, "")
	if err != nil || !strings.Contains(rdpJob.SSHSnapshot, "IdentityAgent") {
		t.Fatalf("RDP jump lost agent settings: %v", err)
	}
	tunnelJob, err := inventoryTunnel(Host{SourceID: s.ID, ConnectionID: tun.ID}, c)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := base64.RawURLEncoding.DecodeString(tunnelJob.Resource)
	var spec inventoryTunnelSpec
	json.Unmarshal(data, &spec)
	if !strings.Contains(spec.Configuration, "IdentityAgent") {
		t.Fatal("tunnel lost referenced SSH agent settings")
	}
	choice := "default"
	c.Overrides = map[string]map[string]ConnectionOverride{s.ID: {r.ID: {SSHAgent: &choice}}}
	name, origin := effectiveAgent(c, s.ID, r)
	if name != "default" || origin != "local override" {
		t.Fatal("override precedence wrong")
	}
	// Intentional system override must not resolve the ignored legacy profile.
	t.Setenv("GO_WHERE_TEST_AGENT", "")
	j, err = managedJob(h, c)
	if err != nil {
		t.Fatal(err)
	}
	destination, _, _ = strings.Cut(j.Configuration, "Host gw-1")
	if strings.Contains(destination, "IdentityAgent") {
		t.Fatal("system default emitted a custom agent")
	}
	c.Overrides = nil
	if _, err = managedJob(h, c); err == nil || !strings.Contains(err.Error(), "GO_WHERE_TEST_AGENT") {
		t.Fatal("unset named agent silently fell back")
	}
	c.SSHAgents = nil
	if _, err = managedJob(h, c); err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatal("unknown profile silently fell back")
	}
	c.SSHAgents = map[string]SSHAgentProfile{"legacy": {Socket: filepath.Join(dir, "not-socket")}}
	os.WriteFile(c.SSHAgents["legacy"].Socket, []byte("not an agent"), 0600)
	if _, err = managedJob(h, c); err == nil || !strings.Contains(err.Error(), "not a Unix socket") {
		t.Fatal("regular file treated as agent")
	}
	for _, bad := range []SSHAgentProfile{{}, {SocketEnv: "BAD;COMMAND"}, {SocketEnv: "A", Socket: "/x"}, {Socket: "${HOME}/agent"}} {
		if validateAgentProfile("legacy", bad) == nil {
			t.Fatalf("invalid profile accepted: %+v", bad)
		}
	}
}

func TestLocalAgentOverrideUIKeepsDatabaseDefault(t *testing.T) {
	s := testInventory(t, "Overrides")
	r := Connection{ID: "server", Alias: "server", Address: "192.0.2.1", Port: 22, SSHAgent: "legacy"}
	if err := changeConnection(s, r, false); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "config.json")
	if err := registerSource(path, s); err != nil {
		t.Fatal(err)
	}
	h := Host{SourceID: s.ID, ConnectionID: r.ID, Alias: r.Alias}
	scriptTUI(t, "none", "save")
	if err := editConnectionAgentOverride(path, h); err != nil {
		t.Fatal(err)
	}
	c, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if name, origin := effectiveAgent(c, s.ID, r); name != "none" || origin != "local override" {
		t.Fatal("local override not saved")
	}
	rows, _ := sourceConnections(s)
	if rows[0].SSHAgent != "legacy" {
		t.Fatal("local override modified the database")
	}
	scriptTUI(t, "inherit", "save")
	if err = editConnectionAgentOverride(path, h); err != nil {
		t.Fatal(err)
	}
	c, err = loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if name, origin := effectiveAgent(c, s.ID, r); name != "legacy" || origin != "connection default" {
		t.Fatal("inherit did not restore database default")
	}
	before, _ := os.ReadFile(path)
	scriptTUI(t, "default", "back")
	if err = editConnectionAgentOverride(path, h); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("cancel saved override")
	}
}

package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	core "github.com/kylar514/go-where/internal/tunnelcore"
)

func TestMarkedTunnelEnterStartsSelection(t *testing.T) {
	m := newPickerModel(nil, "", "current", Origin{}, PickerState{Mode: "tunnels"}, defaultConfig())
	m.tunnels = []core.Connection{{Name: "a"}, {Name: "b"}}
	m.marked["a"], m.marked["b"] = true, true
	if m.finish("enter") == nil || !m.pending["a"] || !m.pending["b"] {
		t.Fatal("Enter did not start marked set")
	}
}

func TestMarkedTunnelEnterTogglesMixedSelection(t *testing.T) {
	m := newPickerModel(nil, "", "current", Origin{}, PickerState{Mode: "tunnels"}, defaultConfig())
	m.tunnels = []core.Connection{{Name: "running"}, {Name: "inactive"}, {Name: "occupied"}, {Name: "pending"}, {Name: "unselected"}}
	for _, name := range []string{"running", "inactive", "occupied", "pending"} {
		m.marked[name] = true
	}
	m.tunnelStatus["running"] = tunnelInfo{status: 2, pid: 123}
	m.tunnelStatus["occupied"] = tunnelInfo{status: 3}
	m.pending["pending"] = true
	// Broken definitions must not prevent stopping an already owned process.
	m.tunnelErrors["running"] = fmt.Errorf("unavailable source")
	if m.finish("enter") == nil || !m.pending["running"] || !m.pending["inactive"] {
		t.Fatal("mixed selection did not schedule stop and start", m.pending)
	}
	if m.pending["occupied"] || m.pending["unselected"] {
		t.Fatal("scheduled unrelated/occupied tunnel")
	}
	if !m.marked["running"] || !m.marked["inactive"] {
		t.Fatal("toggle cleared marks")
	}
}

func TestTunnelSourceHostColorsAndTrustPolicy(t *testing.T) {
	s := testInventory(t, "Tunnels")
	h := InventoryHost{ID: "host", Alias: "server", Address: "192.0.2.1", Color: "#ef4444", BracketColor: "same"}
	r := Connection{ID: "ssh", Alias: "ssh", HostID: h.ID, ProfileName: "Default", Default: true, Type: "ssh", Port: 22}
	if err := saveInventoryHost(s, h, []Connection{r}); err != nil {
		t.Fatal(err)
	}
	tun := Connection{ID: "tunnel", Alias: "server-db", Type: "tunnel", TunnelType: "ssh", SourceHost: &ProfileReference{s.ID, h.ID}, RemoteHost: "127.0.0.1", Bind: "127.0.10.1", LocalPort: 27017, RemotePort: 27017}
	if err := changeConnection(s, tun, false); err != nil {
		t.Fatal(err)
	}
	c := defaultConfig()
	c.Sources = []InventorySource{s}
	hosts, err := inventoryHosts(c)
	if err != nil {
		t.Fatal(err)
	}
	for _, host := range hosts {
		if host.Type == "tunnel" {
			if host.Color != h.Color || host.BracketColor != h.BracketColor {
				t.Fatal("host colors lost", host)
			}
			conn, err := inventoryTunnel(host, c)
			if err != nil {
				t.Fatal(err)
			}
			data, _ := base64.RawURLEncoding.DecodeString(conn.Resource)
			var spec inventoryTunnelSpec
			_ = json.Unmarshal(data, &spec)
			if !strings.HasPrefix(spec.Configuration, "Host *\n    BatchMode yes\n    StrictHostKeyChecking accept-new") || strings.Contains(spec.Configuration, "StrictHostKeyChecking no") {
				t.Fatal(spec.Configuration)
			}
		}
	}
	tun.Color = "#3b82f6"
	tun.BracketColor = "none"
	if err = changeConnection(s, tun, false); err != nil {
		t.Fatal(err)
	}
	hosts, err = inventoryHosts(c)
	if err != nil {
		t.Fatal(err)
	}
	for _, host := range hosts {
		if host.Type == "tunnel" && host.Color != tun.Color {
			t.Fatal("override ignored")
		}
	}
}

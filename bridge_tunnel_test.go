package main

import (
	"encoding/base64"
	"encoding/json"
	"testing"
)

func TestBridgeSourceStartupFailureStopsBeforeDestination(t *testing.T) {
	dir := t.TempDir()
	fakeCommand(t, dir, "ssh", "exit 1")
	spec := inventoryTunnelSpec{Bridge: &bridgeTunnelSpec{Bind: "127.0.0.1", Address: "localhost", SourcePort: 5432, DestinationPort: 15432}}
	data, _ := json.Marshal(spec)
	if err := runBridgeSupervisor([]string{base64.RawURLEncoding.EncodeToString(data)}); err == nil {
		t.Fatal("source startup failure ignored")
	}
}

func TestBridgeUsesDefaultSSHHostsAndPrivateSnapshots(t *testing.T) {
	s := testInventory(t, "Bridge")
	for _, r := range []Connection{{ID: "a", Alias: "source", Address: "192.0.2.1", Port: 22}, {ID: "b", Alias: "destination", Address: "192.0.2.2", Port: 22}} {
		if err := changeConnection(s, r, false); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := sourceConnections(s)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]string{}
	for _, r := range rows {
		ids[r.ID] = r.HostID
	}
	r := Connection{ID: "bridge", Alias: "database", Type: "tunnel", TunnelType: "bridge", SourceHost: &ProfileReference{s.ID, ids["a"]}, DestinationHost: &ProfileReference{s.ID, ids["b"]}, RemoteHost: "localhost", Bind: "127.0.0.1", RemotePort: 5432, LocalPort: 15432}
	if err = changeConnection(s, r, false); err != nil {
		t.Fatal(err)
	}
	c := defaultConfig()
	c.Sources = []InventorySource{s}
	conn, err := inventoryTunnel(Host{SourceID: s.ID, ConnectionID: r.ID}, c)
	if err != nil {
		t.Fatal(err)
	}
	data, err := base64.RawURLEncoding.DecodeString(conn.Resource)
	if err != nil {
		t.Fatal(err)
	}
	var spec inventoryTunnelSpec
	if err = json.Unmarshal(data, &spec); err != nil {
		t.Fatal(err)
	}
	if !conn.Supervised || spec.Bridge == nil || spec.Bridge.Source.Configuration == "" || spec.Bridge.Destination.Configuration == "" || spec.Bridge.SourcePort != 5432 || spec.Bridge.DestinationPort != 15432 {
		t.Fatal(spec)
	}
	r.Bind = "0.0.0.0"
	if _, err = inventoryBridgeTunnel(Host{SourceID: s.ID, ConnectionID: r.ID}, c, r); err == nil {
		t.Fatal("public listener accepted")
	}
}

func TestBridgeRejectsUntrustedEndpointsBeforeLaunching(t *testing.T) {
	spec := inventoryTunnelSpec{Bridge: &bridgeTunnelSpec{Bind: "0.0.0.0", Address: "localhost", SourcePort: 5432, DestinationPort: 15432}}
	data, _ := json.Marshal(spec)
	if err := runBridgeSupervisor([]string{base64.RawURLEncoding.EncodeToString(data)}); err == nil {
		t.Fatal("invalid bridge accepted")
	}
}

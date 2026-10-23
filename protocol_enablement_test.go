package main

import (
	"strings"
	"testing"
)

func TestPersonalRDPEnablementAndUnifiedRoute(t *testing.T) {
	s := testInventory(t, "Hosts")
	for _, r := range []Connection{{ID: "h", Alias: "host", Address: "192.0.2.1", Port: 22}, {ID: "g", Alias: "gateway", Address: "192.0.2.2", Port: 22}} {
		if err := changeConnection(s, r, false); err != nil {
			t.Fatal(err)
		}
	}
	rows, _ := sourceConnections(s)
	ids := map[string]string{}
	for _, r := range rows {
		ids[r.ID] = r.HostID
	}
	c := defaultConfig()
	s.ReadOnly = true
	c.Sources = []InventorySource{s}
	c.LocalProfiles = map[string]Connection{"rdp": {ID: "rdp", ProfileName: "Desktop", Type: "rdp", Port: 3389}}
	c.PersonalConnections = map[string]map[string]PersonalConnection{s.ID: {ids["h"] + ":rdp": {Profile: &ProfileReference{"local", "rdp"}}, ids["h"] + ":route": {Jump: &ProfileReference{s.ID, ids["g"]}}}}
	hosts, err := inventoryHosts(c)
	if err != nil {
		t.Fatal(err)
	}
	var rdp Host
	for _, h := range hosts {
		if h.Type == "rdp" && h.InventoryHostID == ids["h"] {
			rdp = h
		}
	}
	if rdp.ConnectionID == "" {
		t.Fatal("RDP not enabled")
	}
	j, err := hostRDPJob(c, rdp, "")
	if err != nil || !strings.Contains(j.SSHSnapshot, "192.0.2.2") {
		t.Fatal(j, err)
	}
	ssh, err := managedJob(Host{SourceID: s.ID, ConnectionID: "h"}, c)
	if err != nil || !strings.Contains(ssh.Configuration, "192.0.2.2") {
		t.Fatal(ssh, err)
	}
}

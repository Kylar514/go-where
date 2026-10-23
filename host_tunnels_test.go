package main

import (
	"strings"
	"testing"
)

func TestHostTunnelFilterTransportTargetAndNotJumpAncestors(t *testing.T) {
	s := testInventory(t, "Tunnels")
	for _, r := range []Connection{{ID: "a", Alias: "gateway", Address: "192.0.2.1", Port: 22}, {ID: "b", Alias: "transport", Address: "192.0.2.2", Port: 22, JumpID: "a"}, {ID: "c", Alias: "target", Address: "192.0.2.3", Port: 22}} {
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
	if err = changeConnection(s, Connection{ID: "t", Alias: "database", Type: "tunnel", TunnelType: "ssh", SSHID: "b", RemoteHostID: ids["c"], Bind: "127.0.0.1", LocalPort: 15432, RemotePort: 5432}, false); err != nil {
		t.Fatal(err)
	}
	c := defaultConfig()
	c.Sources = []InventorySource{s}
	for _, test := range []struct {
		id, role string
		count    int
	}{{"a", "", 0}, {"b", "SSH transport", 1}, {"c", "Remote target", 1}} {
		entries, roles, err := hostTunnelEntries(c, []Host{{SourceID: s.ID, InventoryHostID: ids[test.id]}})
		if err != nil || len(entries) != test.count {
			t.Fatal(test, entries, err)
		}
		if len(entries) > 0 && !strings.Contains(roles[hostIdentity(entries[0])], test.role) {
			t.Fatal(roles)
		}
	}
	entries, _, err := hostTunnelEntries(c, []Host{{SourceID: s.ID, InventoryHostID: ids["b"]}, {SourceID: s.ID, InventoryHostID: ids["c"]}})
	if err != nil || len(entries) != 1 {
		t.Fatal("duplicate union entry", entries, err)
	}
}

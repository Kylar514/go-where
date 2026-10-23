package main

import (
	"strings"
	"testing"
)

func TestRemoteTransferEndpointsIsolationAndRsyncGuard(t *testing.T) {
	s := testInventory(t, "Transfers")
	for _, r := range []Connection{{ID: "a", Alias: "source", Address: "192.0.2.1", User: "gw-user", Port: 22}, {ID: "b", Alias: "destination", Address: "192.0.2.2", User: "other", Port: 2222}} {
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
	c.Sources = []InventorySource{s}
	tx := transferEndpoints{Mode: "scp", Source: &ProfileReference{s.ID, ids["a"]}, Destination: &ProfileReference{s.ID, ids["b"]}, SourcePath: "/source path", DestinationPath: "/destination path"}
	j, err := endpointTransferJob(tx, c)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(j.Configuration, "Host src-gw-0") || !strings.Contains(j.Configuration, "Host dst-gw-0") || !strings.Contains(j.Configuration, `User "gw-user"`) {
		t.Fatal(j.Configuration)
	}
	if j.Argv[len(j.Argv)-2] != "src-gw-0:/source path" || j.Argv[len(j.Argv)-1] != "dst-gw-0:/destination path" {
		t.Fatal(j.Argv)
	}
	if _, cleanup, err := prepareTransferJob(j); err != nil {
		t.Fatal(err)
	} else {
		cleanup()
	}
	tx.Mode = "rsync"
	if _, err = endpointTransferJob(tx, c); err == nil {
		t.Fatal("remote rsync accepted")
	}
	tx.Mode = "legacy-scp"
	tx.SourcePath = "/path; touch /bad"
	j, err = endpointTransferJob(tx, c)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(j.Argv[len(j.Argv)-2], "'/path; touch /bad'") {
		t.Fatal(j.Argv)
	}
}

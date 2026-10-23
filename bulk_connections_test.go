package main

import (
	"strings"
	"testing"
)

func TestBulkRoutesAtomicAndProfileAssignments(t *testing.T) {
	s := testInventory(t, "Bulk")
	for _, r := range []Connection{{ID: "a", Alias: "a", Address: "192.0.2.1", Port: 22}, {ID: "b", Alias: "b", Address: "192.0.2.2", Port: 22}} {
		if err := changeConnection(s, r, false); err != nil {
			t.Fatal(err)
		}
	}
	before, err := sourceConnections(s)
	if err != nil {
		t.Fatal(err)
	}
	after := append([]Connection(nil), before...)
	after[0].JumpID = "b"
	after[1].JumpID = "a"
	if applyBulkConnections(s, before, after) == nil {
		t.Fatal("cycle accepted")
	}
	rows, _ := sourceConnections(s)
	if rows[0].JumpID != "" || rows[1].JumpID != "" {
		t.Fatal("failed batch partially applied")
	}
	after[0].JumpID = ""
	after[1].JumpID = "a"
	if err = applyBulkConnections(s, before, after); err != nil {
		t.Fatal(err)
	}
	if err = applyBulkConnections(s, before, after); err == nil || !strings.Contains(err.Error(), "changed") {
		t.Fatal("stale batch accepted", err)
	}
}

func TestDirectHostFieldsShowIdentityAndRoutes(t *testing.T) {
	c := defaultConfig()
	c.Identities = map[string]LocalIdentity{"work": {User: "operator"}}
	c.IdentityAssignments = map[string]SourceIdentities{"s": {Default: "work"}}
	rows := directConnectionRows(c, InventorySource{ID: "s"}, []Connection{{ID: "a", Type: "ssh", Default: true, ProfileName: "Standard"}})
	labels := ""
	for _, r := range rows {
		labels += r.Label + "\n"
	}
	if !strings.Contains(labels, "SSH identity: work") || !strings.Contains(labels, "SSH jump host: Direct") || !strings.Contains(labels, "SSH connection: Standard") {
		t.Fatal(labels)
	}
}

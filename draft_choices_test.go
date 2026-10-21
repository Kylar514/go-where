package main

import "testing"

func TestTunnelBackendSelectionOnlyConfirmsDataLoss(t *testing.T) {
	s := testInventory(t, "Choices")
	c := defaultConfig()
	c.Sources = []InventorySource{s}
	state := PickerState{Source: s.ID, Mode: "tunnels"}
	scriptTUI(t, "tunnel_type", "kubectl", "alias", "enter\nforward", "resource", "enter\nservice/web", "save")
	if err := editConnection(c, &state, nil); err != nil {
		t.Fatal(err)
	}
	records, err := sourceConnections(s)
	if err != nil || len(records) != 1 || records[0].TunnelType != "kubectl" {
		t.Fatal(records, err)
	}
	draft := records[0]
	// Cancelling a destructive backend change keeps populated settings.
	scriptTUI(t, "tunnel_type", "ssh", "back", "save")
	if err = editConnectionDraft(c, &state, nil, &draft); err != nil {
		t.Fatal(err)
	}
	if draft.Resource != "service/web" || draft.TunnelType != "kubectl" {
		t.Fatal("cancel lost data", draft)
	}
	// The same guard protects populated new drafts, not just saved records.
	for _, r := range []Connection{{SSHID: "profile"}, {RemoteHostID: "host"}, {RemoteHost: "192.0.2.1"}, {Resource: "service/web"}, {Namespace: "namespace"}, {Context: "context"}} {
		if !tunnelBackendConfigured(r) {
			t.Fatal("populated backend not protected", r)
		}
	}
	if tunnelBackendConfigured(Connection{RemoteHost: "localhost"}) {
		t.Fatal("default target caused redundant confirmation")
	}
}

package main

import (
	"strings"
	"testing"
)

func TestRDPAccountEditorModesAndResolution(t *testing.T) {
	s := testInventory(t, "Accounts")
	domain := "${CUSTOM_DOMAIN}"
	r := Connection{ID: "desktop", Type: "rdp", Alias: "desktop", Address: "192.0.2.4", Port: 3389, User: "${CUSTOM_USER}", Domain: &domain}
	if err := changeConnection(s, r, false); err != nil {
		t.Fatal(err)
	}
	c := defaultConfig()
	c.Sources = []InventorySource{s}
	savedDomain := "LAB"
	c.RDP.User = "global-user"
	c.RDP.PasswordCommand = []string{"must-not-use-global-secret"}
	c.RDP.Accounts = []RDPAccount{
		{Name: "Admin", User: "profile-user", Domain: &savedDomain, PasswordCommand: []string{"profile-secret"}},
		{Name: "Follow", User: "follow-user"},
	}
	h := Host{Type: "rdp", Alias: r.Alias, SourceID: s.ID, ConnectionID: r.ID}
	state := PickerState{Source: s.ID, Mode: "rdp"}
	fields := func(record Connection) map[string]string {
		result := map[string]string{}
		for _, f := range connectionFields(record, s, c) {
			result[f.ID] += f.Label + "\n"
		}
		return result
	}
	read := func() Connection {
		t.Helper()
		rows, err := sourceConnections(s)
		if err != nil {
			t.Fatal(err)
		}
		return rows[0]
	}
	if f := fields(r); f["user"] == "" || f["domain"] == "" || !strings.Contains(f["account"], "Custom") {
		t.Fatal("Custom fields unavailable")
	}
	scriptTUI(t, "account", "Admin", "save")
	if err := editConnection(c, &state, &h); err != nil {
		t.Fatal(err)
	}
	r = read()
	if r.Account != "Admin" || r.User != "${CUSTOM_USER}" || r.Domain == nil || *r.Domain != domain {
		t.Fatal("account selection discarded custom values")
	}
	if f := fields(r); f["user"] != "" || f["domain"] != "" || !strings.Contains(f["account_info"], "profile-user") || !strings.Contains(f["account_info"], "LAB") {
		t.Fatal("saved profile did not hide competing inputs")
	}
	// Hidden, unresolved custom values must not block the saved account.
	j, err := hostRDPJob(c, h, "")
	if err != nil || j.User != "profile-user" || j.Domain != "LAB" || len(j.PasswordCommand) != 1 || j.PasswordCommand[0] != "profile-secret" {
		t.Fatalf("saved account resolution: %+v %v", j, err)
	}
	scriptTUI(t, "account", "Follow", "save")
	if err = editConnection(c, &state, &h); err != nil {
		t.Fatal(err)
	}
	r = read()
	if f := fields(r); f["user"] != "" || f["domain"] == "" {
		t.Fatal("domain-less profile must expose connection domain only")
	}
	if _, err = hostRDPJob(c, h, ""); err == nil || !strings.Contains(err.Error(), "CUSTOM_DOMAIN") {
		t.Fatal("editable connection domain was not resolved")
	}
	c.Variables = map[string]string{"CUSTOM_DOMAIN": "SITE"}
	j, err = hostRDPJob(c, h, "")
	if err != nil || j.User != "follow-user" || j.Domain != "SITE" || len(j.PasswordCommand) != 0 {
		t.Fatalf("domain inheritance: %+v %v", j, err)
	}
	scriptTUI(t, "account", "\tCustom\n", "save")
	if err = editConnection(c, &state, &h); err != nil {
		t.Fatal(err)
	}
	r = read()
	if r.Account != "" || r.User != "${CUSTOM_USER}" || *r.Domain != domain {
		t.Fatal("Custom values did not survive switching back")
	}
	c.Variables["CUSTOM_USER"] = "custom-user"
	j, err = hostRDPJob(c, h, "")
	if err != nil || j.User != "custom-user" || j.Domain != "SITE" || len(j.PasswordCommand) != 0 {
		t.Fatalf("Custom must not use default secret: %+v %v", j, err)
	}
	r.User = ""
	r.Domain = nil
	if err = changeConnection(s, r, false); err != nil {
		t.Fatal(err)
	}
	j, err = hostRDPJob(c, h, "")
	if err != nil || j.User != "global-user" || len(j.PasswordCommand) != 0 {
		t.Fatal("blank Custom fields must not inherit a secret command")
	}
	r.User = "${CUSTOM_USER}"
	r.Domain = &domain
	if err = changeConnection(s, r, false); err != nil {
		t.Fatal(err)
	}
	scriptTUI(t, "account", "Admin", "back")
	if err = editConnection(c, &state, &h); err != nil {
		t.Fatal(err)
	}
	if read().Account != "" {
		t.Fatal("cancel persisted account change")
	}
	// Explicit Alt+A selection overrides Custom and its unresolved fields.
	c.Variables = nil
	c.activeRDPAccount = c.RDP.Accounts[0]
	j, err = hostRDPJob(c, h, "")
	if err != nil || j.User != "profile-user" || j.Domain != "LAB" {
		t.Fatalf("one-launch override: %+v %v", j, err)
	}
	r.Account = "Missing"
	if err = changeConnection(s, r, false); err != nil {
		t.Fatal(err)
	}
	c.activeRDPAccount = RDPAccount{}
	if _, err = hostRDPJob(c, h, ""); err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatal("missing profile silently fell back")
	}
}

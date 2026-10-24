package main

import (
	"strings"
	"testing"
)

func TestIdentityDefaultsAndAssignmentPrecedence(t *testing.T) {
	s := testInventory(t, "Identity")
	r := Connection{ID: "host", Alias: "host", Address: "192.0.2.1", Port: 22, User: "existing", IdentityFile: "/tmp/existing-key"}
	if err := changeConnection(s, r, false); err != nil {
		t.Fatal(err)
	}
	c := defaultConfig()
	c.Sources = []InventorySource{s}
	rows, err := sourceConnections(s)
	if err != nil {
		t.Fatal(err)
	}
	r = rows[0]
	job, err := managedJob(Host{SourceID: s.ID, ConnectionID: r.ID}, c)
	if err != nil || !strings.Contains(job.Configuration, `User "existing"`) {
		t.Fatal("existing identity lost", job, err)
	}
	c.Identities = map[string]LocalIdentity{"work": {User: "operator", IdentityFile: "/tmp/work-key", SSHAgent: "none"}, "admin": {User: "admin"}}
	c.IdentityAssignments = map[string]SourceIdentities{s.ID: {Default: "work", Profiles: map[string]string{r.ProfileID: "admin"}}}
	resolved, err := resolvedConnection(c, s, r)
	if err != nil || resolved.User != "admin" || resolved.IdentityFile != "" {
		t.Fatal(resolved, err)
	}
	a := c.IdentityAssignments[s.ID]
	a.Connections = map[string]string{r.ID: "default"}
	c.IdentityAssignments[s.ID] = a
	job, err = managedJob(Host{SourceID: s.ID, ConnectionID: r.ID}, c)
	if err != nil || strings.Contains(job.Configuration, "User ") || strings.Contains(job.Configuration, "IdentityFile") || strings.Contains(job.Configuration, "IdentityAgent") {
		t.Fatal("OpenSSH defaults not restored", job, err)
	}
	user := "exception"
	c.Overrides = map[string]map[string]ConnectionOverride{s.ID: {r.ID: {User: &user}}}
	resolved, err = resolvedConnection(c, s, r)
	if err != nil || resolved.User != "exception" {
		t.Fatal("field override lost", resolved, err)
	}
	copy := cloneAgentSettings(c)
	assignment := copy.IdentityAssignments[s.ID]
	assignment.Connections[r.ID] = "work"
	if c.IdentityAssignments[s.ID].Connections[r.ID] != "default" {
		t.Fatal("draft mutated original")
	}
	c.IdentityAssignments[s.ID] = SourceIdentities{Default: "missing"}
	if validateIdentities(c) == nil {
		t.Fatal("missing identity accepted")
	}
}

func TestIdentityEditorAndSourceAssignment(t *testing.T) {
	s := testInventory(t, "Identity UI")
	c := defaultConfig()
	c.Sources = []InventorySource{s}
	scriptTUI(t, "add", "enter\nwork", "user", "enter\noperator", "save", "assign", s.ID, "source", "work", "back")
	if err := editLocalIdentities(&c); err != nil {
		t.Fatal(err)
	}
	if c.Identities["work"].User != "operator" || c.IdentityAssignments[s.ID].Default != "work" {
		t.Fatal(c)
	}
}

func TestBulkIdentityAssignmentAndPerHopResolution(t *testing.T) {
	s := testInventory(t, "Bulk identities")
	for _, r := range []Connection{{ID: "jump", Alias: "jump", Address: "192.0.2.1", Port: 22}, {ID: "server", Alias: "server", Address: "192.0.2.2", Port: 22, JumpID: "jump"}} {
		if err := changeConnection(s, r, false); err != nil {
			t.Fatal(err)
		}
	}
	c := defaultConfig()
	c.Sources = []InventorySource{s}
	c.Identities = map[string]LocalIdentity{"work": {User: "operator", SSHAgent: "none"}, "gateway": {User: "gateway-user"}}
	scriptTUI(t, s.ID, "connections", "work", "toggle:server,done")
	if err := assignIdentity(&c); err != nil {
		t.Fatal(err)
	}
	a := c.IdentityAssignments[s.ID]
	a.Default = "gateway"
	c.IdentityAssignments[s.ID] = a
	job, err := managedJob(Host{SourceID: s.ID, ConnectionID: "server"}, c)
	if err != nil {
		t.Fatal(err)
	}
	first, second, _ := strings.Cut(job.Configuration, "Host gw-1")
	if !strings.Contains(first, `User "operator"`) || !strings.Contains(first, `IdentityAgent "none"`) || !strings.Contains(second, `User "gateway-user"`) {
		t.Fatal("per-hop identity lost", job.Configuration)
	}
}

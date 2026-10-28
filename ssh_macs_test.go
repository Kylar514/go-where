package main

import (
	"fmt"
	"strings"
	"testing"
)

func TestMACCompatibilityEditorPersistenceAndHopIsolation(t *testing.T) {
	old := sshOutput
	defer func() { sshOutput = old; resetKexCache() }()
	sshOutput = func(args ...string) ([]byte, error) {
		if len(args) == 2 && args[0] == "-Q" && args[1] == "mac" {
			return []byte("hmac-sha1\nhmac-sha2-256\n"), nil
		}
		if args[0] == "-G" {
			return []byte("macs hmac-sha2-256,hmac-sha1\n"), nil
		}
		return nil, fmt.Errorf("unexpected query")
	}
	resetKexCache()
	entries, _ := algorithmCandidates("MACs", nil)
	found := false
	for _, e := range entries {
		if e.Name == "hmac-sha1" {
			found = true
			if !e.Weak || !strings.Contains(e.Note, "already enabled") {
				t.Fatal(e)
			}
		}
	}
	if !found {
		t.Fatal("hmac-sha1 hidden on Linux defaults")
	}
	s := testInventory(t, "MACs")
	jump := Connection{ID: "jump", Alias: "jump", Address: "192.0.2.1", Port: 22}
	r := Connection{ID: "server", Alias: "server", Address: "192.0.2.2", Port: 22, JumpID: "jump"}
	scriptTUI(t, "macs", "toggle:hmac-sha1,done", "save")
	if err := editSSHCompatibility(&r); err != nil {
		t.Fatal(err)
	}
	for _, record := range []Connection{jump, r} {
		if err := changeConnection(s, record, false); err != nil {
			t.Fatal(err)
		}
	}
	c := defaultConfig()
	c.Sources = []InventorySource{s}
	h := Host{SourceID: s.ID, ConnectionID: r.ID, Alias: r.Alias, Type: "ssh"}
	_, saved, err := storedRecord(h, c)
	if err != nil || len(saved.ExtraMACAlgorithms) != 1 {
		t.Fatal(saved, err)
	}
	job, err := managedJob(h, c)
	if err != nil {
		t.Fatal(err)
	}
	blocks := strings.Split(job.Configuration, "Host gw-1")
	if len(blocks) != 2 || !strings.Contains(blocks[0], "MACs +hmac-sha1") || strings.Contains(blocks[1], "MACs") {
		t.Fatal("MAC leaked to jump", job.Configuration)
	}
	if !strings.Contains(managedDetails(h, c), "Additional MAC algorithms: hmac-sha1") {
		t.Fatal("missing Details")
	}
	scriptTUI(t, "macs", "custom", "toggle:hmac-sha1,done", "save")
	if err = editCompatibilityOverrideDraft(&c, h); err != nil {
		t.Fatal(err)
	}
	if o := c.Overrides[s.ID][r.ID].ExtraMACAlgorithms; o == nil || len(*o) != 0 {
		t.Fatal("explicit empty override lost")
	}
	job, err = managedJob(h, c)
	if err != nil || strings.Contains(job.Configuration, "MACs") {
		t.Fatal(job.Configuration, err)
	}
	scriptTUI(t, "macs", "inherit", "save")
	if err = editCompatibilityOverrideDraft(&c, h); err != nil {
		t.Fatal(err)
	}
	if len(effectiveMACAlgorithms(c, s.ID, saved)) != 1 {
		t.Fatal("inherit lost profile MAC")
	}
	bad := []string{"hmac-sha1\nProxyCommand bad"}
	c.Overrides[s.ID][r.ID] = ConnectionOverride{ExtraMACAlgorithms: &bad}
	if c.validate() == nil {
		t.Fatal("injected MAC accepted")
	}
	if checkAlgorithmsSupported("MACs", []string{"unsupported"}) == nil {
		t.Fatal("unsupported MAC accepted")
	}
}

package main

import (
	"strings"
	"testing"
)

func TestHostEditorDetailsScopeAndCurrentValues(t *testing.T) {
	options := []choice{{"name", "Name: Mixed"}, {"address", "Address: Mixed"}, {"tags", "Tags: env"}, {"notes", "Notes: note"}, {"color", "Color: #06b6d4"}, {"ssh-profile", "SSH profile: Standard"}, {"ssh-identities", "SSH identities: Work"}, {"rdp-profile", "RDP profile: Desktop"}, {"rdp-identities", "RDP identities: Admin"}, {"rdp-session", "RDP session: Global"}}
	d := hostEditorDetails(options, 3, false)
	for _, o := range options {
		_, value, _ := strings.Cut(o.Label, ": ")
		if !strings.Contains(d[o.ID], "Current: "+value) {
			t.Fatal(o.ID, d[o.ID])
		}
	}
	if !strings.Contains(d["tags"], "Comma-separated") || !strings.Contains(d["name"], "read-only") {
		t.Fatal(d)
	}
	if !strings.Contains(d["ssh-profile"], "all 3 selected hosts") || !strings.Contains(d["rdp-session"], "not just the selected hosts") {
		t.Fatal(d)
	}
}

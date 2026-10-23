package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestTunnelDetailsFollowHighlightedField(t *testing.T) {
	r := Connection{TunnelType: "bridge", RemoteHost: "localhost", RemotePort: 5432, Bind: "127.0.0.1", LocalPort: 15432}
	options := []choice{{"source-host", "Source host: Services"}, {"destination-host", "Destination host: Grafana"}, {"source-address", "Source address: localhost"}, {"destination-port", "Destination port: 15432"}, {"save-to", "Save to: Homelab"}}
	d := newDialog("Tunnel — Homelab", options, "")
	d.optionDetails = tunnelEditorDetails(options, r, InventorySource{Path: "/example/homelab.db"})
	d.Update(tea.WindowSizeMsg{Width: 120, Height: 32})
	if !strings.Contains(d.preview.View(), "default SSH") {
		t.Fatal(d.preview.View())
	}
	d.cursor = 2
	d.Update(tea.WindowSizeMsg{Width: 120, Height: 32})
	if !strings.Contains(d.preview.View(), "Source address") || !strings.Contains(d.optionDetails["source-address"], "localhost means the source host") {
		t.Fatal(d.preview.View())
	}
	for _, o := range options {
		if !strings.Contains(d.optionDetails[o.ID], "Current:") || !strings.Contains(d.optionDetails[o.ID], "Available at: Grafana") {
			t.Fatal(o.ID, d.optionDetails[o.ID])
		}
	}
	if !strings.Contains(d.optionDetails["save-to"], "/example/homelab.db") || !strings.Contains(d.optionDetails["save-to"], "Writable") {
		t.Fatal(d.optionDetails)
	}
}

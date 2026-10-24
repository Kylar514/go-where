package main

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"strings"
	"testing"
)

func TestTunnelNameSwatchColumnsAndResize(t *testing.T) {
	s := testInventory(t, "Homelab")
	h := InventoryHost{ID: "host", Alias: "host", Address: "192.0.2.1", Color: "#06b6d4", BracketColor: "#ef4444"}
	p := Connection{ID: "ssh", Alias: "ssh", HostID: h.ID, ProfileName: "Default", Default: true, Type: "ssh", Port: 22}
	if err := saveInventoryHost(s, h, []Connection{p}); err != nil {
		t.Fatal(err)
	}
	r := Connection{ID: "tunnel", Alias: "kaiju-01", Type: "tunnel", TunnelType: "ssh", SSHID: p.ID, Bind: "127.0.0.1", LocalPort: 8080, RemoteHost: "localhost", RemotePort: 80}
	if err := changeConnection(s, r, false); err != nil {
		t.Fatal(err)
	}
	c := defaultConfig()
	c.Sources = []InventorySource{s}
	hosts, err := inventoryHosts(c)
	if err != nil {
		t.Fatal(err)
	}
	m := newPickerModel(hosts, "", "current", Origin{}, PickerState{Mode: "tunnels"}, c)
	for _, host := range hosts {
		if host.BracketColor != h.BracketColor {
			t.Fatal("bracket color lost in tunnel projection")
		}
	}
	m.Update(tea.WindowSizeMsg{Width: 148, Height: 28})
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "[ ] [■] kaiju-01") {
		t.Fatal("tunnel checkbox/swatches overlap", v)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if len(m.markedTunnels()) != 1 {
		t.Fatal("Tab did not mark tunnel")
	}
	v = ansi.Strip(m.View())
	if !strings.Contains(v, "[✓] [■] kaiju-01") || !strings.Contains(v, "Start selected") || !strings.Contains(v, "Stop selected") {
		t.Fatal("marked tunnel controls missing", v)
	}
	m.search.SetValue("no match")
	m.refilter()
	if len(m.markedTunnels()) != 1 {
		t.Fatal("filter lost marks")
	}
	m.search.SetValue("")
	m.refilter()
	m.View()
	for _, value := range []string{"[■] kaiju-01", "LOOPBACK", "REMOTE PORT", "LOCAL PORT", "127.0.0.1", "8080"} {
		if !strings.Contains(v, value) {
			t.Fatal("missing tunnel value", value, v)
		}
	}
	for _, hit := range m.hits {
		if hit.action == "resize-column" {
			m.Update(tea.MouseMsg{X: hit.x, Y: hit.y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
			m.Update(tea.MouseMsg{X: hit.x + 3, Y: hit.y, Button: tea.MouseButtonLeft, Action: tea.MouseActionMotion})
			m.Update(tea.MouseMsg{Action: tea.MouseActionRelease})
			break
		}
	}
	if len(m.state.TunnelColumnWidths) == 0 || m.key != "" {
		t.Fatal("tunnel resize failed")
	}
}

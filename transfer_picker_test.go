package main

import (
	tea "github.com/charmbracelet/bubbletea"
	"strings"
	"testing"
)

func TestTransferTabsIncludeManagedSSHAndMarkedHosts(t *testing.T) {
	s := testInventory(t, "Transfers")
	r := Connection{ID: "server", Alias: "server", Address: "192.0.2.1", Port: 22}
	if err := changeConnection(s, r, false); err != nil {
		t.Fatal(err)
	}
	c := defaultConfig()
	c.Sources = []InventorySource{s}
	c.TunnelsConfig = t.TempDir() + "/missing"
	hosts, err := inventoryHosts(c)
	if err != nil {
		t.Fatal(err)
	}
	hosts = append(hosts, Host{SourceID: s.ID, ConnectionID: "rdp", Type: "rdp", Alias: "desktop"}, Host{SourceID: s.ID, Type: "host", Alias: "bare"})
	for _, mode := range []string{"scp", "legacy-scp", "rsync"} {
		m := newPickerModel(hosts, "", "current", Origin{}, PickerState{Mode: mode}, c)
		m.Update(tea.WindowSizeMsg{Width: 120, Height: 28})
		if len(m.visible) != 1 || m.hosts[m.visible[0]].ConnectionID != r.ID {
			t.Fatal(mode, "wrong eligible hosts", m.visible)
		}
		m.marked[hostIdentity(hosts[0])] = true
		if selected := m.selection(false); len(selected) != 1 || selected[0].ConnectionID != r.ID {
			t.Fatal(mode, "marked host excluded")
		}
		if v := m.preview.View(); !strings.Contains(v, "Transfer method:") {
			t.Fatal(mode, "missing transfer description", v)
		}
		m.search.SetValue("no match")
		m.refilter()
		m.updatePreview()
		v := m.preview.View()
		if strings.Contains(v, "RDP lists") || !strings.Contains(v, "SSH-capable") {
			t.Fatal(mode, "incorrect empty description", v)
		}
	}
}

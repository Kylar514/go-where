package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	tunnelcore "github.com/kylar514/go-where/internal/tunnelcore"
)

func clickSortHeader(t *testing.T, m *pickerModel, column int) {
	t.Helper()
	m.View()
	for _, hit := range m.hits {
		if hit.action == "sort-column" && hit.index == column {
			m.Update(tea.MouseMsg{X: hit.x, Y: hit.y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
			if m.key != "" || m.dragKind != "" {
				t.Fatal("header click launched or resized")
			}
			return
		}
	}
	t.Fatal("missing sortable header", column)
}

func TestPickerColumnSorting(t *testing.T) {
	hosts := []Host{{Alias: "zebra", Hostname: "alpha", Notes: []string{"RDP"}}, {Alias: "Alpha", Hostname: "zebra", Notes: []string{"RDP"}}, {Alias: "alpha", Hostname: "middle", Notes: []string{"RDP"}}}
	for _, mode := range []string{"ssh", "rdp"} {
		m := newPickerModel(hosts, "", "current", Origin{}, PickerState{Mode: mode}, defaultConfig())
		m.Update(tea.WindowSizeMsg{Width: 148, Height: 28})
		clickSortHeader(t, m, 0)
		if m.visible[0] != 1 || m.visible[1] != 2 || m.visible[m.cursor] != 0 {
			t.Fatal("case-insensitive order or focus", m.visible, m.cursor)
		}
		if !strings.Contains(ansi.Strip(m.View()), "HOST ▲") {
			t.Fatal("missing ascending indicator")
		}
		clickSortHeader(t, m, 0)
		if m.visible[0] != 0 || !strings.Contains(ansi.Strip(m.View()), "HOST ▼") {
			t.Fatal("descending order", m.visible)
		}
		clickSortHeader(t, m, 1)
		if m.visible[0] != 0 || m.visible[1] != 2 {
			t.Fatal("address sorting", m.visible)
		}
		m.search.SetValue("alpha")
		m.refilter()
		if m.visible[0] != 0 {
			t.Fatal("filter lost sorting")
		}
		m.search.SetValue("")
		m.refilter()
		m.finish("ctrl-e")
		state := m.state
		state.Focus = hostIdentity(hosts[m.visible[m.cursor]])
		next := newPickerModel(hosts, "", "current", Origin{}, state, defaultConfig())
		if next.visible[0] != 0 || next.visible[1] != 2 {
			t.Fatal("editor return lost sort")
		}
		next.switchMode("transfers")
		next.switchMode(mode)
		if next.visible[0] != 0 || next.state.Sorts[mode].Column != 1 {
			t.Fatal("tab lost sort")
		}
		for _, width := range []int{200, 148, 80, 40} {
			next.Update(tea.WindowSizeMsg{Width: width, Height: 28})
			listWidth, _ := next.paneWidths()
			for column := range next.columnWidths(listWidth) {
				clickSortHeader(t, next, column)
			}
		}
	}
}

func TestTunnelSortNarrowColumnsAndResize(t *testing.T) {
	m := newPickerModel(nil, "", "current", Origin{}, PickerState{Mode: "tunnels"}, defaultConfig())
	m.tunnels = []tunnelcore.Connection{{Name: "zebra", RemotePort: 80, LocalPort: 9000}, {Name: "Alpha", RemotePort: 443, LocalPort: 1000}}
	m.refilter()
	for _, width := range []int{148, 80, 40} {
		m.Update(tea.WindowSizeMsg{Width: width, Height: 28})
		listWidth, _ := m.paneWidths()
		columns := m.columnWidths(listWidth)
		column := m.sortColumn(1, len(columns))
		clickSortHeader(t, m, column)
		if m.state.Sorts["tunnels"].Column != column {
			t.Fatal("narrow column identity")
		}
		if column == 3 && m.visible[0] != 1 {
			t.Fatal("ports must sort alphabetically", m.visible)
		}
		m.View()
		for _, hit := range m.hits {
			if hit.action == "resize-column" {
				before := m.state.Sorts["tunnels"]
				m.Update(tea.MouseMsg{X: hit.x, Y: hit.y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
				m.Update(tea.MouseMsg{X: hit.x + 2, Y: hit.y, Button: tea.MouseButtonLeft, Action: tea.MouseActionMotion})
				m.Update(tea.MouseMsg{Action: tea.MouseActionRelease})
				if m.state.Sorts["tunnels"] != before {
					t.Fatal("divider changed sort")
				}
				break
			}
		}
	}
	m.toggleSort(5)
	m.state.Sorts["tunnels"] = pickerSort{Column: 5}
	m.tunnelStatus = map[string]tunnelInfo{"zebra": {status: 2}}
	m.sortVisible()
	if m.visible[0] != 0 {
		t.Fatal("status sorted by symbol instead of text")
	}
	selected := m.visible[m.cursor]
	m.Update(tunnelStatusMsg{statuses: map[string]tunnelInfo{"Alpha": {status: 2}}})
	if m.visible[0] != 1 || m.visible[m.cursor] != selected {
		t.Fatal("status refresh lost order or focus")
	}
	m.switchMode("ssh")
	if _, ok := m.state.Sorts["ssh"]; ok {
		t.Fatal("sort leaked across tabs")
	}
}

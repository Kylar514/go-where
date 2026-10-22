package main

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"testing"
)

func TestMouseResizeAndSessionReset(t *testing.T) {
	c := defaultConfig()
	c.TunnelsConfig = t.TempDir() + "/missing"
	m := newPickerModel([]Host{{Alias: "demo"}}, "", "current", Origin{}, PickerState{Mode: "ssh"}, c)
	m.Update(tea.WindowSizeMsg{Width: 148, Height: 28})
	m.View()
	for _, action := range []string{"resize-pane", "resize-column"} {
		var target hitRegion
		found := false
		for _, hit := range m.hits {
			if hit.action == action {
				target = hit
				found = true
				break
			}
		}
		if !found {
			t.Fatal("no resize handle", action)
		}
		before, _ := m.paneWidths()
		widths := m.columnWidths(before)
		m.Update(tea.MouseMsg{X: target.x, Y: target.y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		m.Update(tea.MouseMsg{X: target.x - 8, Y: target.y, Button: tea.MouseButtonLeft, Action: tea.MouseActionMotion})
		m.Update(tea.MouseMsg{X: target.x - 8, Y: target.y, Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease})
		if m.key != "" || m.dragKind != "" {
			t.Fatal("drag launched or remained active")
		}
		after, _ := m.paneWidths()
		if action == "resize-pane" && after >= before {
			t.Fatal("divider did not move")
		}
		if action == "resize-column" && m.columnWidths(after)[0] >= widths[0] {
			t.Fatal("column did not move")
		}
		v := m.View()
		if lipgloss.Width(v) > 148 || lipgloss.Height(v) > 28 {
			t.Fatal("resize overflow")
		}
	}
	next := newPickerModel(m.hosts, "", "current", Origin{}, m.state, c)
	next.Update(tea.WindowSizeMsg{Width: 148, Height: 28})
	if next.state.ListPercent == 0 || len(next.state.ColumnWidths) == 0 {
		t.Fatal("session layout lost")
	}
	next.Update(tea.KeyMsg{Type: tea.KeyCtrlZ})
	if next.state.ListPercent != 0 || next.state.ColumnWidths != nil {
		t.Fatal("reset failed")
	}
}

package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestChecklistDetailsPanelAndDivider(t *testing.T) {
	for _, sources := range []bool{false, true} {
		m := newMultiDialog("Checklist", []multiItem{{ID: "a", Label: "Alpha"}}, strings.Repeat("Readable explanatory text.\n", 60))
		m.sourceSelection = sources
		m.Update(tea.WindowSizeMsg{Width: 120, Height: 28})
		v := m.View()
		if !strings.Contains(v, "╭─ Details") || !strings.Contains(v, "█") || lipgloss.Width(v) > 120 || lipgloss.Height(v) != 28 {
			t.Fatal("checklist panel layout", v)
		}
		listWidth, _ := m.resize.menuPaneWidths()
		m.Update(tea.MouseMsg{X: listWidth + 5, Y: m.listStart() + 2, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		if m.items[0].Checked || m.done {
			t.Fatal("Details click activated checklist")
		}
		m.Update(tea.MouseMsg{X: listWidth + 5, Y: m.listStart() + 2, Button: tea.MouseButtonWheelDown})
		if m.preview.YOffset == 0 {
			t.Fatal("Details wheel did not scroll")
		}
		m.Update(tea.MouseMsg{X: listWidth, Y: m.listStart(), Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		m.Update(tea.MouseMsg{X: listWidth - 8, Y: m.listStart(), Button: tea.MouseButtonLeft, Action: tea.MouseActionMotion})
		m.Update(tea.MouseMsg{X: listWidth - 8, Y: m.listStart(), Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease})
		after, _ := m.resize.menuPaneWidths()
		if after >= listWidth || m.done || m.items[0].Checked {
			t.Fatal("divider drag failed")
		}
		v = m.View()
		if lipgloss.Width(v) > 120 || lipgloss.Height(v) != 28 {
			t.Fatal("drag overflow", v)
		}
	}
}

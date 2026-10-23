package main

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"strings"
	"testing"
)

func TestIdentityAssignmentHostTableAndMarking(t *testing.T) {
	m := newMultiDialog("Assign identity", []multiItem{{ID: "a", Label: "internal / policy"}, {ID: "b", Label: "second / policy"}}, "Assign work identity")
	m.allowManual = false
	m.hostRows = map[string]Host{"a": {Alias: "real-host", Hostname: "192.0.2.1", SourceName: "Work", Tags: []string{"Production"}, Comment: "Application"}, "b": {Alias: "second-host"}}
	m.Update(tea.WindowSizeMsg{Width: 148, Height: 28})
	v := ansi.Strip(m.View())
	for _, word := range []string{"HOST", "SOURCE", "TAGS", "NOTES", "real-host", "Production", "Application"} {
		if !strings.Contains(v, word) {
			t.Fatal("table missing", word, v)
		}
	}
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if !m.items[0].Checked || m.cursor != 1 {
		t.Fatal("Tab failed to mark/move up")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if !m.items[1].Checked || m.cursor != 0 {
		t.Fatal("Shift+Tab failed to mark/move down")
	}
	m.View()
	if len(m.columnHits) == 0 {
		t.Fatal("no column resize handles")
	}
}

package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestHelpWideDetailsAndDescriptiveFooter(t *testing.T) {
	m := newPickerModel(nil, "", "current", Origin{}, PickerState{Mode: "ssh"}, defaultConfig())
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m.action("help")
	d := m.dialog
	if !d.readingScreen || len(d.options) != 0 {
		t.Fatal("help retained list row")
	}
	list, details := d.menuPaneWidths()
	if details <= list {
		t.Fatal(list, details)
	}
	view := d.View()
	if !strings.Contains(view, "Back │") || !strings.Contains(view, "Reset layout") {
		t.Fatal(view)
	}
	d.resetMenuLayout()
	if d.layout.listPercent != 20 {
		t.Fatal("reset did not restore wide help")
	}
}

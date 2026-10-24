package main

import (
	tea "github.com/charmbracelet/bubbletea"
	"strings"
	"testing"
)

func TestDetailsKeysDoNotMoveListAndHelpUsesPlainKeys(t *testing.T) {
	options := []choice{}
	for i := 0; i < 30; i++ {
		options = append(options, choice{strings.Repeat("x", i+1), "row"})
	}
	d := newDialog("Menu", options, strings.Repeat("Details line\n", 80))
	d.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	d.Update(tea.KeyMsg{Type: tea.KeyShiftDown})
	if d.cursor != 0 || d.preview.YOffset != 1 {
		t.Fatal(d.cursor, d.preview.YOffset)
	}
	d.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	if d.cursor == 0 || d.preview.YOffset != 1 {
		t.Fatal("list paging scrolled details", d.cursor, d.preview.YOffset)
	}
	d.readingScreen = true
	offset := d.preview.YOffset
	d.Update(tea.KeyMsg{Type: tea.KeyDown})
	if d.preview.YOffset != offset+1 {
		t.Fatal("help arrows did not scroll")
	}
}

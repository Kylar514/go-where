package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestPickerEditAndDescriptivePrimaryHints(t *testing.T) {
	c := defaultConfig()
	c.Hotkeys = map[string][]string{"edit": {"f2", "ctrl+e"}}
	m := newPickerModel([]Host{{Alias: "host"}}, "", "current", Origin{}, PickerState{Mode: "ssh"}, c)
	m.Update(tea.WindowSizeMsg{Width: 148, Height: 28})
	m.marked["host"] = true
	edit := 0
	for _, b := range m.buttons() {
		if b.label == "Edit" {
			edit++
		}
		if b.label == "Bulk edit" {
			t.Fatal("duplicate Edit action")
		}
	}
	if edit != 1 {
		t.Fatal(edit)
	}
	bar, _, _ := m.controls(m.buttons(), 0)
	lines := strings.Split(ansi.Strip(bar), "\n")
	if !strings.Contains(lines[3], "↵ Connect │") || !strings.Contains(lines[3], "f2 Edit") {
		t.Fatal(bar)
	}
	if strings.Contains(lines[3], "ctrl+e") {
		t.Fatal("alternative hint shown")
	}
	view := strings.Split(ansi.Strip(m.View()), "\n")
	for i, line := range view {
		if strings.Contains(line, "results ·") {
			if strings.Contains(line, "ctrl+") || i == 0 || !strings.Contains(strings.Join(view[:i], "\n"), "f2 Edit") {
				t.Fatal(view)
			}
		}
	}
}

func TestButtonLegendWrapsWholeEntries(t *testing.T) {
	c := defaultConfig()
	c.Hotkeys = map[string][]string{"help": {}}
	lines := buttonLegend(c, []pickerButton{{"Connect", "connect"}, {"Edit", "edit-tunnel"}, {"Help", "help"}}, 14)
	if len(lines) != 2 || lines[0] != "↵ Connect" || lines[1] != "ctrl+e Edit" {
		t.Fatal(lines)
	}
}

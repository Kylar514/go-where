package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestSaveHostFromCurrentProfileDraft(t *testing.T) {
	s := testInventory(t, "Host save")
	c := defaultConfig()
	c.Sources = []InventorySource{s}
	state := PickerState{Source: s.ID, Mode: "ssh"}
	scriptTUI(t, s.ID, "alias", "enter\nserver", "address", "enter\n192.0.2.1", "ssh", "add", "enter\nDefault", "settings", "user", "enter\noperator", "save-host", "back", "back", "back", "back")
	if err := editInventoryHost(c, &state, nil); err != nil {
		t.Fatal(err)
	}
	rows, err := sourceConnections(s)
	if err != nil || len(rows) != 1 || rows[0].User != "operator" {
		t.Fatal("current profile edits not saved", rows, err)
	}
	if saveActiveHost != nil || hostSaveState != nil {
		t.Fatal("host save scope leaked")
	}
}

func TestColorMenuSwatchesAndHostSaveKey(t *testing.T) {
	d := newDialog("Colors", []choice{{"#ef4444", "Red  #ef4444"}, {"", "Clear color"}}, "")
	d.singleChoice, d.current, d.colorChoices = true, "#ef4444", true
	d.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	v := ansi.Strip(d.View())
	if !strings.Contains(v, "● [■] Red") || strings.Contains(v, "■ Clear") {
		t.Fatal(v)
	}
	d.saveHost = true
	d.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if d.result != "__save_host" {
		t.Fatal("host Ctrl+S failed")
	}
}

package main

import (
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestEditUsesMarkedHostsAcrossSourcesAndFiltering(t *testing.T) {
	hosts := []Host{{Alias: "one", SourceID: "a", ConnectionID: "one", Type: "ssh"}, {Alias: "two", SourceID: "b", ConnectionID: "two", Type: "ssh"}}
	m := newPickerModel(hosts, "", "terminal", Origin{}, PickerState{Mode: "ssh"}, defaultConfig())
	for _, h := range hosts {
		m.marked[hostIdentity(h)] = true
	}
	m.search.SetValue("one")
	m.refilter()
	m.finish("ctrl-e")
	if len(m.selected) != 2 {
		t.Fatal("edit ignored filtered marks", m.selected)
	}
	m.marked = map[string]bool{}
	m.finish("ctrl-e")
	if len(m.selected) != 1 || m.selected[0].Alias != "one" {
		t.Fatal("unmarked edit did not use highlighted host", m.selected)
	}
}

func TestCombinedEditorGroupGapAndColorSwatch(t *testing.T) {
	d := newDialog("Edit host — one", []choice{{"name", "Name: one"}, {"color", "Color: #06b6d4"}, {"ssh-profile", "SSH profile: Standard"}, {"rdp-profile", "RDP profile: Desktop"}}, "")
	if !reflect.DeepEqual(d.visualSlots(), []int{0, 1, -1, 2, 3}) {
		t.Fatal(d.visualSlots())
	}
	d.cursor = 1
	d.Update(tea.WindowSizeMsg{Width: 110, Height: 24})
	view := ansi.Strip(d.View())
	if !strings.Contains(view, ansi.Strip(colorSwatch("#06b6d4"))+" #06b6d4") {
		t.Fatal(view)
	}
}

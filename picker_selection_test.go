package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	tunnelcore "github.com/kylar514/go-where/internal/tunnelcore"
)

func TestPickerSelectAllToggle(t *testing.T) {
	for _, mode := range []string{"ssh", "rdp", "tunnels"} {
		c := defaultConfig()
		hosts := []Host{{Alias: "alpha", Notes: []string{"RDP"}}, {Alias: "beta", Notes: []string{"RDP"}}}
		m := newPickerModel(hosts, "alpha", "current", Origin{}, PickerState{Mode: mode}, c)
		if mode == "tunnels" {
			m.tunnels = []tunnelcore.Connection{{Name: "alpha"}, {Name: "beta"}}
			m.refilter()
		}
		m.Update(tea.WindowSizeMsg{Width: 148, Height: 30})
		m.marked["unrelated-tab-only"] = true
		if !strings.Contains(m.View(), "ctrl+a Select all") {
			t.Fatal("missing toggle hint", mode)
		}
		cursor, offset := m.cursor, m.offset
		m.Update(tea.KeyMsg{Type: tea.KeyCtrlA})
		keys := m.modeMarkKeys()
		if !m.marked[keys[0]] || m.marked[keys[1]] || m.cursor != cursor || m.offset != offset || m.search.Value() != "alpha" || m.key != "" {
			t.Fatal("Select all ignored filter/navigation", mode, m.marked)
		}
		m.marked[keys[1]] = true
		m.search.SetValue("no matches")
		m.refilter()
		if !strings.Contains(m.View(), "ctrl+a Deselect all") {
			t.Fatal("hidden marks not detected")
		}
		found := false
		for _, hit := range m.hits {
			if hit.action == "select-all" {
				m.Update(tea.MouseMsg{X: hit.x, Y: hit.y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
				found = true
				break
			}
		}
		if !found || m.hasModeMarks() || !m.marked["unrelated-tab-only"] || m.key != "" {
			t.Fatal("Deselect all failed", mode, m.marked)
		}
		m.Update(tea.KeyMsg{Type: tea.KeyCtrlA})
		if m.hasModeMarks() {
			t.Fatal("empty filter selected hidden rows")
		}
		for _, width := range []int{40, 80, 148} {
			m.Update(tea.WindowSizeMsg{Width: width, Height: 30})
			view := m.View()
			if lipgloss.Width(view) > width || lipgloss.Height(view) > 30 {
				t.Fatal("toggle overflow", mode, width, view)
			}
		}
	}
}

func TestSelectionToggleHotkeyAndTabIsolation(t *testing.T) {
	c := defaultConfig()
	c.Hotkeys = map[string][]string{"select_all": {"alt+x"}}
	if err := validateHotkeys(c); err != nil {
		t.Fatal(err)
	}
	hosts := []Host{{SourceID: "source", ConnectionID: "ssh", Alias: "ssh", Type: "ssh"}, {SourceID: "source", ConnectionID: "rdp", Alias: "rdp", Type: "rdp"}}
	m := newPickerModel(hosts, "", "current", Origin{}, PickerState{Mode: "rdp"}, c)
	m.marked[hostIdentity(hosts[0])] = true
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}, Alt: true})
	if !m.marked[hostIdentity(hosts[1])] || !m.marked[hostIdentity(hosts[0])] {
		t.Fatal("remapped select all")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}, Alt: true})
	if m.marked[hostIdentity(hosts[1])] || !m.marked[hostIdentity(hosts[0])] {
		t.Fatal("cleared another tab's marks")
	}
	c.Hotkeys["select_all"] = []string{"ctrl+s"}
	if validateHotkeys(c) == nil {
		t.Fatal("hotkey conflict accepted")
	}
}

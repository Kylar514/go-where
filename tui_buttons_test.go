package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestClickableControlsAndDoubleClick(t *testing.T) {
	c := defaultConfig()
	c.TunnelsConfig = t.TempDir() + "/missing"
	for _, size := range [][2]int{{148, 28}, {90, 24}, {60, 18}, {30, 12}} {
		m := newPickerModel([]Host{{Alias: "a"}, {Alias: "b"}}, "", "current", Origin{}, PickerState{Mode: "ssh"}, c)
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		view := m.View()
		if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
			t.Fatalf("overflow %v: %dx%d", size, lipgloss.Width(view), lipgloss.Height(view))
		}
		if size[0] >= 90 && !strings.Contains(view, "│ SSH │ RDP │") {
			t.Fatal("missing outlined tabs")
		}
		for _, tab := range pickerTabs {
			found := false
			for _, hit := range m.hits {
				if hit.action == tab.action {
					found = true
					m.Update(tea.MouseMsg{X: hit.x, Y: hit.y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
					if m.state.Mode != strings.TrimPrefix(tab.action, "mode:") {
						t.Fatal("tab hit failed", tab)
					}
					break
				}
			}
			if !found {
				t.Fatal("tab not clickable", tab)
			}
		}
	}
	for _, mode := range []string{"ssh", "scp", "legacy-scp", "rsync"} {
		m := newPickerModel([]Host{{Alias: "a"}, {Alias: "b"}}, "", "current", Origin{}, PickerState{Mode: mode}, c)
		m.Update(tea.WindowSizeMsg{Width: 100, Height: 28})
		m.marked["b"] = true
		m.View()
		for _, hit := range m.hits {
			if hit.action == "row" && hit.index == 0 {
				click := tea.MouseMsg{X: 5, Y: hit.y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
				m.Update(click)
				if m.key != "" {
					t.Fatal("single click launched")
				}
				m.Update(click)
				if m.key != "enter" || len(m.selected) != 1 || m.selected[0].Alias != "a" {
					t.Fatal("double click used marked group", mode, m.selected)
				}
				if m.lastClick != "" {
					t.Fatal("double click not consumed")
				}
				break
			}
		}
	}
}

func TestConnectedStripsHaveSharedBordersAndOpenActiveTab(t *testing.T) {
	m := &pickerModel{width: 100, height: 28, state: PickerState{Mode: "rdp"}}
	strip, hits, height := m.controls(pickerTabs, 0)
	lines := strings.Split(strip, "\n")
	if height != 2 || !strings.Contains(lines[0], "│ SSH │ RDP │") || strings.Contains(lines[0], "││") {
		t.Fatal(strip)
	}
	if !strings.Contains(lines[1], "┘     └") {
		t.Fatal("active tab underline was not opened", strip)
	}
	for _, hit := range hits {
		if hit.action == "mode:rdp" && hit.y == 1 {
			if hit.width != 6 {
				t.Fatal("shared edge has wrong hit width", hit)
			}
		}
	}
	strip, _, _ = m.controls(m.buttons(), 0)
	lines = strings.Split(strip, "\n")
	if len(lines) != 1 || !strings.Contains(lines[0], "│ Connect │ Connect as │") || strings.Contains(lines[0], "││") {
		t.Fatal("action bar disconnected", strip)
	}
	m.state.Mode = "rsync"
	strip, _, _ = m.controls(pickerTabs, 0)
	if !strings.Contains(strings.Split(strip, "\n")[1], "┘       └─") {
		t.Fatal("last active tab does not join baseline", strip)
	}
	m.state.Mode = "ssh"
	strip, _, _ = m.controls(pickerTabs, 0)
	if !strings.HasPrefix(strings.Split(strip, "\n")[1], "┘     └") {
		t.Fatal("first active tab does not join baseline", strip)
	}
}

func TestLaunchAndSourceControlsReflectCurrentMode(t *testing.T) {
	c := defaultConfig()
	c.TunnelsConfig = t.TempDir() + "/missing"
	c.RDP.Destination = "background"
	m := newPickerModel(nil, "", "terminal", Origin{}, PickerState{Mode: "rdp", Source: "ssh-config"}, c)
	m.Update(tea.WindowSizeMsg{Width: 148, Height: 28})
	v := m.View()
	if strings.Contains(v, "Destination:") || !strings.Contains(v, "Launch: background") || !strings.Contains(v, "Sources: SSH config") {
		t.Fatal(v)
	}
	m.switchMode("ssh")
	if !strings.Contains(m.View(), "Launch: terminal") {
		t.Fatal("SSH launch label ignored destination")
	}
	m.switchMode("tunnels")
	if strings.Contains(m.View(), "Launch:") {
		t.Fatal("tunnels have launch-location control")
	}
	scriptTUI(t, "background")
	dest, _, back, err := chooseTarget(Origin{}, "terminal", true)
	if err != nil || back || dest != "background" {
		t.Fatal(dest, back, err)
	}
}

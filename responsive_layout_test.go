package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestResponsivePickerDetailsAndMore(t *testing.T) {
	for _, width := range []int{40, 60, 80, 120, 148} {
		m := newPickerModel([]Host{{Alias: "host", Comment: "important description"}}, "", "current", Origin{}, PickerState{Mode: "ssh"}, defaultConfig())
		m.Update(tea.WindowSizeMsg{Width: width, Height: 24})
		view := m.View()
		if lipgloss.Width(view) > width || lipgloss.Height(view) > 24 {
			t.Fatal(width, view)
		}
		if width < 140 {
			if !strings.Contains(view, "Details") || !strings.Contains(view, "More") {
				t.Fatal("missing compact controls")
			}
			m.action("details")
			if !strings.Contains(m.dialog.View(), "important") {
				t.Fatal("missing description", m.dialog.View())
			}
			m.Update(tea.KeyMsg{Type: tea.KeyEsc})
			if m.dialog != nil || m.key != "" {
				t.Fatal("Details lost parent")
			}
			m.action("more")
			found := false
			for _, o := range m.dialog.options {
				if o.ID == "new-tunnel" {
					found = true
				}
			}
			if !found {
				t.Fatal("More lost New")
			}
			m.Update(tea.KeyMsg{Type: tea.KeyEsc})
		}
	}
}

func TestNarrowMenuDetails(t *testing.T) {
	m := newDialog("Editor", []choice{{"name", "Name: example"}}, "Explanation")
	m.optionDetails = map[string]string{"name": "Field description"}
	m.Update(tea.WindowSizeMsg{Width: 40, Height: 16})
	m.Update(tea.MouseMsg{X: 2, Y: 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if !m.detailsOnly || !strings.Contains(m.View(), "Field description") {
		t.Fatal(m.View())
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.done || m.detailsOnly {
		t.Fatal("Back exited parent")
	}
}

func TestDetailsBackMouseUsesRenderedRow(t *testing.T) {
	for _, reading := range []bool{false, true} {
		m := newDialog("Editor", []choice{{"name", "Name: example"}}, "Explanation")
		m.Update(tea.WindowSizeMsg{Width: 60, Height: 24})
		m.detailsOnly, m.readingScreen = true, reading
		m.detailsText = "Details content"
		m.refreshDetailsLayout()
		lines := strings.Split(m.View(), "\n")
		y := -1
		for i, line := range lines {
			if strings.Contains(line, "│ Back │") {
				y = i
				break
			}
		}
		if y < 0 {
			t.Fatal("Back missing")
		}
		m.Update(tea.MouseMsg{X: 20, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		if !m.detailsOnly || m.done {
			t.Fatal("hint text acted as Back")
		}
		m.Update(tea.MouseMsg{X: 3, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		if reading {
			if !m.done || !m.back {
				t.Fatal("picker Details failed to close")
			}
		} else if m.detailsOnly || m.done || m.cursor != 0 {
			t.Fatal("menu Details failed to return")
		}
	}
}

func TestDetailsUsesFullWidthAndStandardFrame(t *testing.T) {
	m := newPickerModel([]Host{{Alias: "host", Comment: "This description should fit on one line in the full width Details view."}}, "", "current", Origin{}, PickerState{Mode: "ssh"}, defaultConfig())
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	m.action("details")
	view := m.dialog.View()
	if !strings.Contains(view, "╭─ Details") || !strings.Contains(view, "│ Back │") || !strings.Contains(view, "This description should fit on one line in the full width Details view.") {
		t.Fatal(view)
	}
	if lipgloss.Width(view) > 100 || lipgloss.Height(view) > 24 {
		t.Fatal("Details overflow")
	}
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 16})
	view = m.dialog.View()
	if lipgloss.Width(view) > 60 || lipgloss.Height(view) > 16 {
		t.Fatal("resized Details overflow", view)
	}
}

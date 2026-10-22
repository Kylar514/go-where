package main

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"testing"
)

func TestMenuResizeColumnsAndDetails(t *testing.T) {
	d := newDialog("Host — Test", []choice{{"alias", "Name: server"}, {"address", "Address: 192.0.2.1"}}, "Details text")
	d.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	for _, action := range []string{"column", "pane"} {
		d.View()
		var hit hitRegion
		found := false
		for _, h := range d.resizeHits {
			if h.action == action {
				hit = h
				found = true
				break
			}
		}
		if !found {
			t.Fatal("handle missing", action)
		}
		before := d.layout
		d.Update(tea.MouseMsg{X: hit.x, Y: hit.y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		d.Update(tea.MouseMsg{X: hit.x + 5, Y: hit.y, Button: tea.MouseButtonLeft, Action: tea.MouseActionMotion})
		d.Update(tea.MouseMsg{X: hit.x + 5, Y: hit.y, Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease})
		if d.layout == before || d.done || d.resizeKind != "" {
			t.Fatal("resize selected row or did not change layout")
		}
		v := d.View()
		if lipgloss.Width(v) > 120 || lipgloss.Height(v) > 24 {
			t.Fatal("layout overflow")
		}
	}
	d.resetMenuLayout()
	if d.layout != (menuLayout{}) {
		t.Fatal("reset failed")
	}
	d = newDialog("Sources", []choice{{"s", "Source — SQLite: /path/db"}}, "")
	d.sourceActions = true
	d.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	d.View()
	for _, h := range d.resizeHits {
		if h.action == "column" && h.index == 1 {
			d.Update(tea.MouseMsg{X: h.x, Y: h.y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
			d.Update(tea.MouseMsg{X: h.x + 3, Y: h.y, Button: tea.MouseButtonLeft, Action: tea.MouseActionMotion})
			d.Update(tea.MouseMsg{Action: tea.MouseActionRelease})
			break
		}
	}
	if d.layout.typeWidth == 0 {
		t.Fatal("source type boundary did not resize")
	}
}

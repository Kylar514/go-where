package main

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"strings"
	"testing"
)

func TestSourceShortcutsAndBelowButtonHints(t *testing.T) {
	settings := newDialog("Sources", []choice{{"", "All sources"}}, "")
	settings.sourceActions = true
	settings.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	settings.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if settings.result != "settings" {
		t.Fatal("Sources Ctrl+S should open Settings")
	}
	for _, tc := range []struct {
		key  tea.KeyMsg
		want string
	}{{tea.KeyMsg{Type: tea.KeyCtrlN}, "create"}, {tea.KeyMsg{Type: tea.KeyCtrlO}, "attach"}, {tea.KeyMsg{Type: tea.KeyCtrlD, Alt: true}, "remove:s"}} {
		d := newDialog("Sources", []choice{{"s", "Source — SQLite: /example.db"}}, "")
		d.sourceActions = true
		d.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
		d.Update(tc.key)
		if d.result != tc.want {
			t.Fatal(d.result, tc.want)
		}
	}
	d := newDialog("Sources", []choice{{"", "All sources"}}, "")
	d.sourceActions = true
	d.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	d.Update(tea.KeyMsg{Type: tea.KeyCtrlD, Alt: true})
	if d.done {
		t.Fatal("removed All sources")
	}
	v := ansi.Strip(menuFooter(24, 100, []string{"Back", "Create source"}, "ctrl+h Back | ctrl+n Create"))
	lines := strings.Split(v, "\n")
	if len(lines) != 4 || !strings.Contains(lines[3], "ctrl+n Create") || strings.Contains(lines[1], "ctrl+n") {
		t.Fatal(v)
	}
	c := defaultConfig()
	c.Hotkeys = map[string][]string{"source_create": {"ctrl+u"}}
	if validateHotkeys(c) == nil {
		t.Fatal("source/menu conflict not caught")
	}
}

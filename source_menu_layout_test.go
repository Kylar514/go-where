package main

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"strings"
	"testing"
)

func TestSourcesUseColumnsAndFooterActions(t *testing.T) {
	d := newDialog("Sources", []choice{{"", "All sources"}, {"ssh-config", "SSH config"}, {"personal", "Personal — SQLite: /example/personal.db"}}, "")
	d.sourceActions = true
	d.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	v := ansi.Strip(d.View())
	if !strings.Contains(v, "Combined") || !strings.Contains(v, "OpenSSH") || !strings.Contains(v, "SQLite") || !strings.Contains(v, "Create source") || !strings.Contains(v, "Add existing") {
		t.Fatal(v)
	}
	d.Update(tea.MouseMsg{X: 12, Y: d.footerY(), Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if d.result != "create" {
		t.Fatal("create click failed")
	}
	d = newDialog("SSH agent profiles", []choice{{"p", "legacy — environment: LEGACY_AGENT"}}, "")
	if !d.formRows() {
		t.Fatal("remaining data menu missed")
	}
	if !strings.Contains(d.formLabel(d.options[0], 60), " │  environment: LEGACY_AGENT") {
		t.Fatal("data not separated")
	}
}

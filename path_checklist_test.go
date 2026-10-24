package main

import (
	tea "github.com/charmbracelet/bubbletea"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalCompletionAndIdentityFooter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "key.pub")
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	p := newPathModel("Identity file", filepath.Join(dir, "ke"), true)
	p.Update(tea.KeyMsg{Type: tea.KeyTab})
	if p.input.Value() != path {
		t.Fatal(p.input.Value())
	}
	m := newMultiDialog("SSH Identities", []multiItem{{ID: "work", Label: "Work", Note: "Long private key explanation"}}, "")
	m.identitySelection = true
	m.allowManual = false
	m.Update(tea.WindowSizeMsg{Width: 110, Height: 28})
	view := m.View()
	if strings.Contains(view, "Done —") || !strings.Contains(view, "Apply") {
		t.Fatal(view)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if !m.done || m.action != "done" {
		t.Fatal("Apply hotkey failed")
	}
}

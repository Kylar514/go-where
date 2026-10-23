package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestTransfersTabMethodsDetailsAndReturn(t *testing.T) {
	m := newPickerModel(nil, "", "current", Origin{}, PickerState{Mode: "ssh"}, defaultConfig())
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m.switchMode("transfers")
	if len(m.visible) != 3 || m.dialog != nil || !strings.Contains(m.View(), "│ SSH │ RDP │ Tunnels │ Transfers │") {
		t.Fatal(m.View())
	}
	if !strings.Contains(m.preview.View(), "SFTP") {
		t.Fatal(m.preview.View())
	}
	m.cursor = 2
	m.updatePreview()
	if !strings.Contains(m.preview.View(), "incrementally") {
		t.Fatal(m.preview.View())
	}
	m.finish("enter")
	if m.key != "transfer-method:rsync" {
		t.Fatal(m.key)
	}
	m.switchMode("ssh")
	if m.state.Mode != "ssh" {
		t.Fatal(m.state.Mode)
	}
}

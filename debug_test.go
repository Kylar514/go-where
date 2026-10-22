package main

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestDebugActionUsesSameManagedSnapshot(t *testing.T) {
	s := testInventory(t, "Debug")
	r := Connection{ID: "ssh", Alias: "ssh", Address: "192.0.2.1", Port: 22, User: "operator"}
	if err := changeConnection(s, r, false); err != nil {
		t.Fatal(err)
	}
	c := defaultConfig()
	c.Sources = []InventorySource{s}
	c.TunnelsConfig = t.TempDir() + "/missing"
	h := Host{SourceID: s.ID, ConnectionID: r.ID, Type: "ssh"}
	normal, err := managedJob(h, c)
	if err != nil {
		t.Fatal(err)
	}
	c.debugLaunch = true
	argv, err := managedSSHCommand(h, c)
	if err != nil {
		t.Fatal(err)
	}
	data, err := base64.RawURLEncoding.DecodeString(argv[2])
	if err != nil {
		t.Fatal(err)
	}
	var job managedSSHJob
	if err = json.Unmarshal(data, &job); err != nil {
		t.Fatal(err)
	}
	if !job.Debug || job.Configuration != normal.Configuration {
		t.Fatal("debug changed managed settings")
	}
	m := newPickerModel([]Host{h}, "", "current", Origin{}, PickerState{Mode: "ssh"}, c)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}, Alt: true})
	if m.key != "debug" || !m.state.Debug || len(m.selected) != 1 {
		t.Fatal("debug shortcut lost selection")
	}
	m.finish("enter")
	if m.state.Debug {
		t.Fatal("debug leaked into normal launch")
	}
	if debugDestination(Origin{Pane: "%1"}) != "tmux-pane" || debugDestination(Origin{}) != "terminal" {
		t.Fatal("debug destination is not visible")
	}
}

func TestDebugTransfersVerboseAndRejectManagedAlias(t *testing.T) {
	for _, argv := range [][]string{{"scp", "-O", "file", "host:/tmp"}, {"rsync", "-a", "-e", "ssh -F config", "file", "host:/tmp"}} {
		original := strings.Join(argv, " ")
		result := verboseTransfer(argv)
		if !strings.Contains(strings.Join(result, " "), "-vv") || strings.Join(argv, " ") != original {
			t.Fatal("verbosity missing or original mutated", result)
		}
	}
	if _, err := transferCommand(Transfer{}, Host{SourceID: "db", Alias: "same-as-personal-config"}, "", false); err == nil {
		t.Fatal("database transfer silently used config alias")
	}
}

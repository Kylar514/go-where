package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestPlacementBelowUsesRecognizableAnchor(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	fakeCommand(t, dir, "tmux", "printf '$1\\t@2\\t%%3\\tsystem\\tdev\\tnvim\\t/work\\tEditor\\t6\\n'")
	old := runTUI
	defer func() { runTUI = old }()
	step := 0
	runTUI = func(model tea.Model) (tea.Model, error) {
		d := model.(*dialogModel)
		step++
		if step == 1 {
			d.result = "tmux-pane-below"
		} else {
			if !strings.Contains(d.options[0].Label, "system 6:dev nvim") || strings.Contains(d.options[0].Label, "2.3") {
				t.Fatal(d.options)
			}
			d.result = d.options[0].ID
		}
		return d, nil
	}
	dest, o, back, err := chooseTarget(Origin{Pane: "%3", Session: "$1", Window: "@2"}, "terminal")
	if err != nil || back || dest != "tmux-pane-below" || o.Pane != "%3" {
		t.Fatal(dest, o, back, err)
	}
}

func TestBelowLaunchUsesVerticalSplit(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "calls")
	t.Setenv("PATH", dir)
	t.Setenv("CALLS", log)
	fakeCommand(t, dir, "tmux", `printf '%s\n' "$@" > "$CALLS"`)
	err := launchCommand("tmux-pane-below", []string{"ssh", "host"}, Origin{Pane: "%3", Window: "@2", Session: "$1"}, defaultConfig(), false)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(log)
	if err != nil || !strings.Contains(string(b), "split-window\n-v\n-t\n$1:@2.%3") {
		t.Fatal(string(b), err)
	}
}

func TestNewHostUsesUnifiedEditor(t *testing.T) {
	s := testInventory(t, "Hosts")
	path := filepath.Join(t.TempDir(), "profile.db")
	original, before, c, err := settingsDraft(path)
	if err != nil {
		t.Fatal(err)
	}
	c.Sources = []InventorySource{s}
	if err = saveSettingsDraft(path, original, before, c); err != nil {
		t.Fatal(err)
	}
	scriptTUI(t, "name", "enter\nnew-host", "address", "enter\n192.0.2.10", "save", "back")
	state := PickerState{Source: s.ID, ConfigPath: path, Mode: "rdp"}
	if err = createUnifiedHost(c, &state); err != nil {
		t.Fatal(err)
	}
	hosts, err := sourceInventoryHosts(s)
	if err != nil || len(hosts) != 1 || hosts[0].Alias != "new-host" {
		t.Fatal(hosts, err)
	}
}

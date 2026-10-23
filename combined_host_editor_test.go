package main

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestCombinedHostFieldsSavePreservesConnections(t *testing.T) {
	s := testInventory(t, "Inventory")
	if err := changeConnection(s, Connection{ID: "c", Alias: "host", Address: "192.0.2.1", Port: 22}, false); err != nil {
		t.Fatal(err)
	}
	rows, err := sourceConnections(s)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "profile.db")
	original, before, c, err := settingsDraft(path)
	if err != nil {
		t.Fatal(err)
	}
	c.Sources = []InventorySource{s}
	if err = saveSettingsDraft(path, original, before, c); err != nil {
		t.Fatal(err)
	}
	h := Host{SourceID: s.ID, InventoryHostID: rows[0].HostID}
	scriptTUI(t, "notes", "ok\nupdated note", "save-host", "back")
	if err = editPersonalHosts(c, path, []Host{h}); err != nil {
		t.Fatal(err)
	}
	after, err := sourceConnections(s)
	if err != nil {
		t.Fatal(err)
	}
	if after[0].Notes != "updated note" {
		t.Fatal(after)
	}
	after[0].Notes = rows[0].Notes
	if !reflect.DeepEqual(rows, after) {
		t.Fatal("host save changed connection settings")
	}
	hosts, err := sourceInventoryHosts(s)
	if err != nil {
		t.Fatal(err)
	}
	if err = saveHostFields(s, hosts, map[string]string{"address": "192.0.2.2"}); err != nil {
		t.Fatal(err)
	}
	if err = saveHostFields(s, hosts, map[string]string{"notes": "stale"}); err == nil {
		t.Fatal("stale shared save accepted")
	}
}

func TestCombinedEditorDefaultAndPickerActions(t *testing.T) {
	s := testInventory(t, "Homelab")
	if err := changeConnection(s, Connection{ID: "c", Alias: "host", Address: "192.0.2.1", Port: 22}, false); err != nil {
		t.Fatal(err)
	}
	rows, err := sourceConnections(s)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "profile.db")
	original, before, c, err := settingsDraft(path)
	if err != nil {
		t.Fatal(err)
	}
	c.Sources = []InventorySource{s}
	if err = saveSettingsDraft(path, original, before, c); err != nil {
		t.Fatal(err)
	}
	oldRun := runTUI
	defer func() { runTUI = oldRun }()
	parentSave := saveActiveSettings
	defer func() { saveActiveSettings = parentSave }()
	saveActiveSettings = func() error { t.Fatal("picker invoked parent save"); return nil }
	step := 0
	runTUI = func(model tea.Model) (tea.Model, error) {
		d := model.(*dialogModel)
		step++
		d.Update(tea.WindowSizeMsg{Width: 110, Height: 30})
		switch step {
		case 1:
			if !strings.HasPrefix(d.title, "Edit host — ") {
				t.Fatal(d.title)
			}
			if !strings.Contains(d.options[5].Label, "Host default — ") {
				t.Fatal(d.options)
			}
			d.result = "ssh-profile"
		case 2:
			if d.current != "existing" || !strings.Contains(d.options[0].Label, rows[0].ProfileName+" (Homelab)") {
				t.Fatal(d.current, d.options)
			}
			if len(d.footerActions) != 1 || d.footerActions[0].Label != "Select" {
				t.Fatal(d.footerActions)
			}
			if strings.Contains(d.View(), "Save settings") {
				t.Fatal(d.View())
			}
			d.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
			if d.done {
				t.Fatal("selector saved on Ctrl+S")
			}
			d.back = true
		case 3:
			d.back = true
		default:
			t.Fatal("extra menu")
		}
		return d, nil
	}
	if err = editPersonalHosts(c, path, []Host{{SourceID: s.ID, InventoryHostID: rows[0].HostID}}); err != nil {
		t.Fatal(err)
	}
}

func TestCombinedReadOnlyHostStillSelectsProfiles(t *testing.T) {
	s := testInventory(t, "Read only")
	if err := changeConnection(s, Connection{ID: "c", Alias: "host", Address: "192.0.2.1", Port: 22}, false); err != nil {
		t.Fatal(err)
	}
	rows, _ := sourceConnections(s)
	s.ReadOnly = true
	path := filepath.Join(t.TempDir(), "profile.db")
	original, before, c, err := settingsDraft(path)
	if err != nil {
		t.Fatal(err)
	}
	c.Sources = []InventorySource{s}
	if err = saveSettingsDraft(path, original, before, c); err != nil {
		t.Fatal(err)
	}
	oldRun := runTUI
	defer func() { runTUI = oldRun }()
	runTUI = func(model tea.Model) (tea.Model, error) {
		d := model.(*dialogModel)
		if !strings.Contains(d.options[0].Label, "read-only") {
			t.Fatal(d.options)
		}
		for _, a := range d.footerActions {
			if a.ID == "save-host" {
				t.Fatal("read-only shared save shown")
			}
		}
		d.back = true
		return d, nil
	}
	if err = editPersonalHosts(c, path, []Host{{SourceID: s.ID, InventoryHostID: rows[0].HostID}}); err != nil {
		t.Fatal(err)
	}
}

func TestExplicitPickerFooterMouseSelect(t *testing.T) {
	d := newDialog("SSH Profiles", []choice{{"existing", "Use host default"}, {"other", "Other"}}, "")
	d.footerActions = []choice{{"select", "Select"}}
	d.singleChoice = true
	d.current = "existing"
	d.cursor = 1
	d.Update(tea.WindowSizeMsg{Width: 110, Height: 24})
	d.Update(tea.MouseMsg{X: 10, Y: d.footerY(), Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if !d.done || d.result != "other" {
		t.Fatal("Select button did not select highlighted profile", d.result)
	}
}

package main

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestSettingsSummariesValuesAndScope(t *testing.T) {
	c := defaultConfig()
	c.StartupMode = "rdp"
	c.Identities = map[string]LocalIdentity{"work": {User: "operator"}}
	c.LocalProfiles = map[string]Connection{"p": {ID: "p", ProfileName: "Local", Port: 22}}
	on := true
	c.RDP.Session.Clipboard = &on
	values, details := settingsSummaries(c)
	for id, want := range map[string]string{"general": "RDP", "identities": "1 configured", "ssh-profiles": "1 available", "sources": "None enabled", "hotkeys": "Defaults"} {
		if values[id] != want {
			t.Fatal(id, values[id])
		}
	}
	if !strings.Contains(values["rdp-session"], "Clipboard enabled") || !strings.Contains(details["general"], "takes precedence") {
		t.Fatal(values, details)
	}
}

func TestStartupTabSummaryUpdatesUnsavedDraft(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profile.db")
	oldRun := runTUI
	defer func() { runTUI = oldRun }()
	step := 0
	runTUI = func(model tea.Model) (tea.Model, error) {
		d := model.(*dialogModel)
		step++
		switch step {
		case 1:
			for _, o := range d.options {
				if o.ID == "general" && o.Label != "Startup tab: SSH" {
					t.Fatal(o)
				}
			}
			d.result = "general"
		case 2:
			if d.title != "Startup tab" || d.current != "ssh" {
				t.Fatal(d.title, d.current)
			}
			d.result = "rdp"
		case 3:
			for i, o := range d.options {
				if o.ID == "general" {
					if o.Label != "Startup tab: RDP" {
						t.Fatal(o)
					}
					for j, index := range d.filtered {
						if index == i {
							d.cursor = j
						}
					}
				}
			}
			d.Update(tea.WindowSizeMsg{Width: 110, Height: 28})
			if !strings.Contains(d.preview.View(), "Current: RDP") {
				t.Fatal(d.preview.View())
			}
			d.back = true
		default:
			t.Fatal("extra dialog")
		}
		return d, nil
	}
	if _, err := settingsMenu(path); err != nil {
		t.Fatal(err)
	}
	c, err := loadConfig(path)
	if err != nil || c.StartupMode != "" {
		t.Fatal("cancel saved startup selection", c.StartupMode, err)
	}
}

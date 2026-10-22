package main

import (
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestHotkeysSubmenuSavesWithoutLeaving(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	old := runTUI
	t.Cleanup(func() { runTUI = old })
	step := 0
	runTUI = func(model tea.Model) (tea.Model, error) {
		switch m := model.(type) {
		case *pathModel:
			m.input.SetValue("f2")
		case *dialogModel:
			switch step {
			case 0:
				m.result = "hotkeys"
			case 1:
				m.result = "settings"
			case 3:
				if m.title != "Hotkeys" {
					t.Fatal(m.title)
				}
				m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
				if m.result != "__save_settings" {
					t.Fatal("submenu shortcut did not request save")
				}
			case 4:
				c, err := loadConfig(path)
				if err != nil || keyHint(c, "settings") != "f2" {
					t.Fatal("submenu save not persisted", c, err)
				}
				if m.title != "Hotkeys" || settingsSaveState() != "Saved" {
					t.Fatal("save left submenu")
				}
				m.back = true
			case 5:
				m.result = "cancel"
			default:
				t.Fatal("unexpected dialog", step, m.title)
			}
		}
		step++
		return model, nil
	}
	if _, err := settingsMenu(path); err != nil {
		t.Fatal(err)
	}
	if step != 6 {
		t.Fatal(step)
	}
}

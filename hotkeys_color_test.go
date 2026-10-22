package main

import (
	tea "github.com/charmbracelet/bubbletea"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHotkeysValidationAndDispatch(t *testing.T) {
	c := defaultConfig()
	if err := validateHotkeys(c); err != nil {
		t.Fatal(err)
	}
	c.Hotkeys = map[string][]string{"settings": {"f2"}, "back": {"f3"}}
	if err := validateHotkeys(c); err != nil {
		t.Fatal(err)
	}
	if canonicalKey(c, "f2", "picker") != "ctrl+s" || canonicalKey(c, "ctrl+s", "picker") != "unbound" || canonicalKey(c, "f3", "menu") != "esc" {
		t.Fatal("rebinding failed")
	}
	c.TunnelsConfig = t.TempDir() + "/missing"
	m := newPickerModel(nil, "", "current", Origin{}, PickerState{Mode: "ssh"}, c)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	m.Update(tea.KeyMsg{Type: tea.KeyF2})
	if m.key != "ctrl-s" {
		t.Fatal("picker ignored configured key")
	}
	previous := menuHotkeys
	menuHotkeys = c
	t.Cleanup(func() { menuHotkeys = previous })
	d := newDialog("Menu", nil, "")
	d.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	d.Update(tea.KeyMsg{Type: tea.KeyF3})
	if !d.back {
		t.Fatal("menu ignored back binding")
	}
	for _, bad := range []map[string][]string{{"settings": {"ctrl+n"}}, {"unknown": {"f2"}}, {"settings": {"a"}}, {"settings": {"f2", "f2"}}} {
		c.Hotkeys = bad
		if validateHotkeys(c) == nil {
			t.Fatal("bad bindings accepted", bad)
		}
	}
	if canonicalKey(defaultConfig(), "f1", "picker") != "?" {
		t.Fatal("default alias failed")
	}
}

func TestHotkeysSettingsSaveAndCancel(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	scriptTUI(t, "hotkeys", "settings", "enter\nf2", "back", "save")
	if _, err := settingsMenu(path); err != nil {
		t.Fatal(err)
	}
	c, err := loadConfig(path)
	if err != nil || keyHint(c, "settings") != "f2" {
		t.Fatal(c, err)
	}
	before, _ := os.ReadFile(path)
	scriptTUI(t, "hotkeys", "restore", "back", "cancel")
	if _, err = settingsMenu(path); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("cancel wrote settings")
	}
}

func TestHostColorPersistenceAndValidation(t *testing.T) {
	s := testInventory(t, "Color")
	h := InventoryHost{ID: "host", Alias: "host", Address: "192.0.2.1", Color: "#3a1620"}
	p := Connection{ID: "ssh", Alias: "ssh", HostID: h.ID, ProfileName: "Default", Default: true, Port: 22, Type: "ssh"}
	if err := saveInventoryHost(s, h, []Connection{p}); err != nil {
		t.Fatal(err)
	}
	c := defaultConfig()
	c.Sources = []InventorySource{s}
	hosts, err := inventoryHosts(c)
	if err != nil || hosts[0].Color != h.Color {
		t.Fatal(hosts, err)
	}
	m := newPickerModel(hosts, "", "current", Origin{}, PickerState{Mode: "ssh"}, c)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	if !strings.Contains(m.View(), "■") || !strings.Contains(managedDetails(hosts[0], c), h.Color) {
		t.Fatal("color not displayed")
	}
	for _, bad := range []string{"red", "#abc", "#1234567", "#12zz34", "#123456\n"} {
		if validateHostColor(bad) == nil {
			t.Fatal("bad color accepted", bad)
		}
	}
	h.Color = "bad"
	if saveInventoryHost(s, h, []Connection{p}) == nil {
		t.Fatal("invalid persisted color")
	}
	scriptTUI(t, "custom", "enter\n#ABCDEF", "apply")
	value, back, err := chooseHostColor("")
	if err != nil || back || value != "#abcdef" {
		t.Fatal(value, back, err)
	}
	scriptTUI(t, "")
	value, back, err = chooseHostColor(value)
	if err != nil || back || value != "" {
		t.Fatal("clear failed")
	}
}

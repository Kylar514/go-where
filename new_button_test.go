package main

import "testing"

func TestNewButtonUsesExistingAction(t *testing.T) {
	for _, mode := range []string{"ssh", "rdp", "tunnels", "transfers"} {
		m := newPickerModel(nil, "", "current", Origin{}, PickerState{Mode: mode}, defaultConfig())
		m.width, m.height = 120, 30
		found := false
		for _, b := range m.buttons() {
			if b.label == "New" {
				found = true
				if b.action != "new-tunnel" || buttonPrimaryHint(m.config, b.action) != "ctrl+n" {
					t.Fatal(b)
				}
			}
		}
		if found != (mode != "transfers") {
			t.Fatal(mode, found)
		}
	}
}

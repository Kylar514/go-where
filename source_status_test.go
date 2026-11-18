package main

import "testing"

func TestSourceWarningIsCompact(t *testing.T) {
	c := defaultConfig()
	c.Sources = []InventorySource{{ID: "first", Name: "shared"}, {ID: "second", Name: "lab"}}
	state := PickerState{UnavailableSources: map[string]string{"first": "long connection failure explaining everything", "second": "another long failure"}}
	m := newPickerModel(nil, "", "current", Origin{}, state, c)
	if m.status != "shared — Unavailable · No cache · +1 sources" {
		t.Fatal(m.status)
	}
	delete(state.UnavailableSources, "second")
	m = newPickerModel(nil, "", "current", Origin{}, state, c)
	if m.status != "shared — Unavailable · No cache" {
		t.Fatal(m.status)
	}
}

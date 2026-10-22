package main

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"strings"
	"testing"
)

func TestGroupedFormRowsAndDeleteControl(t *testing.T) {
	d := newDialog("Host — Test", []choice{{"alias", "Name: server"}, {"address", "Address: 192.0.2.1"}, {"ssh", "SSH profiles: Standard ★ …"}, {"color", "Color: #123456 …"}}, "")
	d.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	n := d.formLabel(d.options[0], 60)
	a := d.formLabel(d.options[1], 60)
	if strings.Index(n, "server") != strings.Index(a, "192.0.2.1") {
		t.Fatal("values not aligned", n, a)
	}
	slots := d.visualSlots()
	gaps := 0
	for _, i := range slots {
		if i < 0 {
			gaps++
		}
	}
	if gaps != 2 {
		t.Fatal("categories lack gaps", slots)
	}
	d.Update(tea.KeyMsg{Type: tea.KeyUp})
	if d.cursor != 1 {
		t.Fatal("navigation selected gap")
	}
	for row, i := range d.visibleSlots() {
		if i < 0 {
			d.Update(tea.MouseMsg{X: 2, Y: d.listStart() + row, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
			if d.done {
				t.Fatal("gap click activated row")
			}
		}
	}
	d.saveHost, d.deleteHost = true, true
	v := ansi.Strip(d.View())
	if !strings.Contains(v, "Delete host") {
		t.Fatal("delete not in footer")
	}
	d.Update(tea.KeyMsg{Type: tea.KeyDelete})
	if d.result != "delete" {
		t.Fatal("delete binding failed")
	}
}

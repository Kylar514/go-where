package main

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"strings"
	"testing"
)

func TestPaletteSwatchWidths(t *testing.T) {
	for _, color := range []string{"#000000", "#ffffff", "#06b6d4"} {
		if ansi.StringWidth(paletteSwatch(color)) != 3 {
			t.Fatal(color, paletteSwatch(color))
		}
	}
	if ansi.Strip(paletteSwatch("#000000")) != "[■]" || ansi.Strip(paletteSwatch("#ffffff")) != "[■]" {
		t.Fatal("neutral swatches lack brackets")
	}
}

func TestExpandedColorPaletteAndSelectorFooter(t *testing.T) {
	old := runTUI
	defer func() { runTUI = old }()
	runTUI = func(model tea.Model) (tea.Model, error) {
		d := model.(*dialogModel)
		count := 0
		for _, o := range d.options {
			if hostHexColor.MatchString(o.ID) {
				count++
				if err := validateHostColor(o.ID); err != nil {
					t.Fatal(err)
				}
			}
		}
		if count != 25 {
			t.Fatal(count)
		}
		if len(d.footerActions) != 1 || d.footerActions[0].Label != "Select" {
			t.Fatal(d.footerActions)
		}
		d.Update(tea.WindowSizeMsg{Width: 110, Height: 30})
		if !strings.Contains(d.View(), "↵ Select") {
			t.Fatal(d.View())
		}
		d.result = "#000000"
		return d, nil
	}
	value, back, err := chooseHostColor("")
	if err != nil || back || value != "#000000" {
		t.Fatal(value, back, err)
	}
}

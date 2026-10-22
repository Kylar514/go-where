package main

import (
	"fmt"
	"github.com/charmbracelet/lipgloss"
	"regexp"
	"strings"
)

var hostHexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func validateHostColor(color string) error {
	if color != "" && !hostHexColor.MatchString(color) {
		return fmt.Errorf("color must be #RRGGBB, e.g. #3a1620")
	}
	return nil
}
func colorSwatch(color string) string {
	if color == "" {
		return " "
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render("■")
}
func chooseHostColor(current string) (string, bool, error) {
	for {
		options := []choice{{"custom", "Enter hex color…"}, {"", "Clear color"}, {"#ef4444", "Red"}, {"#f59e0b", "Amber"}, {"#22c55e", "Green"}, {"#3b82f6", "Blue"}, {"#a855f7", "Purple"}, {"#06b6d4", "Cyan"}}
		for i := range options {
			if hostHexColor.MatchString(options[i].ID) {
				options[i].Label += "  " + options[i].ID
			}
		}
		menu := newDialog("Host color", options, "Current: "+current+"\n\nColors mark hosts, not access permissions. Dark colors may be difficult to see. Limited-color terminals approximate RGB colors.")
		menu.singleChoice, menu.current, menu.colorChoices = true, current, true
		for i, option := range options {
			if option.ID == current {
				menu.cursor = i
				break
			}
		}
		id, back, err := runChoice(menu, menuKey("Host color"))
		if err != nil || back {
			return current, back, err
		}
		if id != "custom" {
			return id, false, nil
		}
		value, b, err := inventoryPrompt("Color (#RRGGBB)", current)
		if err != nil {
			return current, false, err
		}
		if b {
			continue
		}
		value = strings.TrimSpace(value)
		if err = validateHostColor(value); err != nil {
			if err = notice(err.Error()); err != nil {
				return current, false, err
			}
			continue
		}
		value = strings.ToLower(value)
		preview := newDialog("Color preview", []choice{{"apply", "Apply color to host draft"}}, value+"\n\nExact RGB depends on terminal support.")
		preview.swatch = value
		_, b, err = runChoice(preview, menuKey("Color preview"))
		if err != nil {
			return current, false, err
		}
		if !b {
			return value, false, nil
		}
	}
}

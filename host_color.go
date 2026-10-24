package main

import (
	"fmt"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
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

func paletteSwatch(color string) string {
	border := ""
	if strings.EqualFold(color, "#000000") {
		border = "#ffffff"
	}
	if strings.EqualFold(color, "#ffffff") {
		border = "#000000"
	}
	if border == "" {
		return " " + colorSwatch(color) + " "
	}
	edge := lipgloss.NewStyle().Reverse(false).Foreground(lipgloss.Color(border))
	return edge.Render("[") + lipgloss.NewStyle().Reverse(false).Foreground(lipgloss.Color(color)).Render("■") + edge.Render("]")
}

func (m *pickerModel) setHostPreview(text string, h Host) {
	text = ansi.Wrap(cleanLines(text), max(1, m.preview.Width), "")
	if h.Color != "" {
		text = strings.Replace(text, "■", colorSwatch(h.Color), 1)
	}
	m.preview.SetContent(text)
}
func chooseHostColor(current string) (string, bool, error) {
	for {
		options := []choice{{"custom", "Custom hex…"}, {"", "Clear color"}, {"#ef4444", "Red"}, {"#f97316", "Orange"}, {"#f59e0b", "Amber"}, {"#eab308", "Yellow"}, {"#84cc16", "Lime"}, {"#22c55e", "Green"}, {"#10b981", "Emerald"}, {"#14b8a6", "Teal"}, {"#06b6d4", "Cyan"}, {"#0ea5e9", "Sky"}, {"#3b82f6", "Blue"}, {"#6366f1", "Indigo"}, {"#8b5cf6", "Violet"}, {"#a855f7", "Purple"}, {"#d946ef", "Magenta"}, {"#ec4899", "Pink"}, {"#ffffff", "White"}, {"#c0c0c0", "Silver"}, {"#808080", "Gray"}, {"#000000", "Black"}}
		for i := range options {
			if hostHexColor.MatchString(options[i].ID) {
				options[i].Label += "  " + options[i].ID
			}
		}
		menu := newDialog("Host color", options, "Current: "+current+"\n\nColors mark hosts, not access permissions. Dark colors may be difficult to see. Limited-color terminals approximate RGB colors.")
		menu.singleChoice, menu.current, menu.colorChoices = true, current, true
		menu.footerActions = []choice{{"select", "Select"}}
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

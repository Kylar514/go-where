package main

import (
	"fmt"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"regexp"
	"strconv"
	"strings"
)

var hostHexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func validateHostColor(color string) error {
	if color != "" && !hostHexColor.MatchString(color) {
		return fmt.Errorf("color must be #RRGGBB, e.g. #3a1620")
	}
	return nil
}

func validateBracketColor(color string) error {
	if color == "same" || color == "none" {
		return nil
	}
	return validateHostColor(color)
}
func colorSwatch(color string) string {
	if color == "" {
		return " "
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render("■")
}

func paletteSwatch(color string) string {
	border := color
	if strings.EqualFold(color, "#000000") || strings.EqualFold(color, "#ffffff") {
		border = contrastingBracket(color)
	}
	return hostColorMarker(color, border)
}

func contrastingBracket(color string) string {
	n, _ := strconv.ParseUint(strings.TrimPrefix(color, "#"), 16, 24)
	if 299*(n>>16)+587*((n>>8)&255)+114*(n&255) >= 160000 {
		return "#000000"
	}
	return "#ffffff"
}

func hostColorMarker(color, border string) string {
	if color == "" {
		return "   "
	}
	if border == "none" {
		return " " + lipgloss.NewStyle().Reverse(false).Foreground(lipgloss.Color(color)).Render("■") + " "
	}
	if border == "same" {
		border = color
	}
	if border == "" {
		border = contrastingBracket(color)
	}
	edge := lipgloss.NewStyle().Reverse(false).Foreground(lipgloss.Color(border))
	return edge.Render("[") + lipgloss.NewStyle().Reverse(false).Foreground(lipgloss.Color(color)).Render("■") + edge.Render("]")
}

func (m *pickerModel) setHostPreview(text string, h Host) {
	text = ansi.Wrap(cleanLines(text), max(1, m.preview.Width), "")
	if h.Color != "" {
		text = strings.Replace(text, "[■]", hostColorMarker(h.Color, h.BracketColor), 1)
	}
	m.preview.SetContent(text)
}
func chooseHostColor(current string) (string, bool, error) {
	return choosePaletteColor(current, false)
}

func choosePaletteColor(current string, brackets bool) (string, bool, error) {
	title := "Block color"
	if brackets {
		title = "Bracket color"
	}
	for {
		options := paletteOptions(brackets)
		for i := range options {
			if hostHexColor.MatchString(options[i].ID) {
				options[i].Label += "  " + options[i].ID
			}
		}
		menu := newDialog(title, options, "Colors mark hosts, not access permissions. Dark colors may be difficult to see. Limited-color terminals approximate RGB colors.\n\nClick Swatch for hue order, Name for A–Z, or Hex for value order. Click again to reverse.")
		menu.singleChoice, menu.current, menu.colorChoices = true, current, true
		menu.palette = true
		menu.screenID = "palette:" + title
		menu.footerActions = []choice{{"select", "Select"}}
		menu.refilter()
		for i, index := range menu.filtered {
			if menu.options[index].ID == current {
				menu.cursor = i
				break
			}
		}
		id, back, err := runChoice(menu, menu.screenID)
		if err != nil || back {
			return current, back, err
		}
		if id != "custom" {
			return id, false, nil
		}
		custom := current
		if !hostHexColor.MatchString(custom) {
			custom = ""
		}
		value, b, err := inventoryPrompt("Color (#RRGGBB)", custom)
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

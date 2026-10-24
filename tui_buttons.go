package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type pickerButton struct{ label, action string }

func compactControl(labels ...string) string {
	var top, bottom []string
	for _, label := range labels {
		top = append(top, strings.Repeat("─", lipgloss.Width(label)+2))
		bottom = append(bottom, strings.Repeat("─", lipgloss.Width(label)+2))
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color("250")).Render("╭" + strings.Join(top, "┬") + "╮\n│ " + strings.Join(labels, " │ ") + " │\n╰" + strings.Join(bottom, "┴") + "╯")
}

func menuFooter(height, width int, labels []string, hint string) string {
	hint = strings.ReplaceAll(hint, " · ", " | ")
	bar := compactControl(labels...)
	lines := strings.Split(bar, "\n")
	if strings.Contains(lines[1], "Delete host") {
		lines[1] = strings.Replace(lines[1], "Delete host", tuiError.Render("Delete host"), 1)
	}
	if strings.Contains(lines[1], "Remove source") {
		lines[1] = strings.Replace(lines[1], "Remove source", tuiError.Render("Remove source"), 1)
	}
	if height < 14 {
		return fit(lines[1], width) + "\n" + fit(hint, width)
	}
	return strings.Join(lines, "\n") + "\n" + fit(tuiDim.Render(hint), width)
}

func footerExtra(height int) int {
	if height < 14 {
		return 1
	}
	return 3
}

func footerButtonY(height int) int {
	if height < 14 {
		return height - 2
	}
	return height - 3
}

var pickerTabs = []pickerButton{{"SSH", "mode:ssh"}, {"RDP", "mode:rdp"}, {"Tunnels", "mode:tunnels"}, {"Transfers", "mode:transfers"}}

func (m *pickerModel) tabs() []pickerButton {
	if m.hostTunnelTitle != "" {
		return []pickerButton{{m.hostTunnelTitle, "host-tunnels"}}
	}
	return pickerTabs
}

func (m *pickerModel) buttons() []pickerButton {
	if m.state.Mode == "transfers" {
		return []pickerButton{{"Select method", "connect"}, {"Settings", "settings"}, {"Help", "help"}}
	}
	if m.hostTunnelTitle != "" {
		return []pickerButton{{"Back", "back"}, {"New", "new-tunnel"}, {"Edit", "edit-tunnel"}, {"Start selected", "start-selected"}, {"Stop selected", "stop-selected"}}
	}
	source := "All"
	if m.state.Source == "ssh-config" {
		source = "SSH config"
	} else if s, err := findSource(m.config, m.state.Source); err == nil {
		source = s.Name
	}
	sourceButton := pickerButton{"Sources: " + source, "sources"}
	primary := "Connect"
	if m.state.Mode == "scp" || m.state.Mode == "legacy-scp" || m.state.Mode == "rsync" {
		primary = "Review transfer"
	}
	if m.height < 16 {
		return []pickerButton{{"Go", "connect"}, {"Edit", "edit-tunnel"}, {"Settings", "settings"}, {"Help", "help"}}
	}
	if m.state.Mode == "tunnels" {
		if len(m.markedTunnels()) > 0 {
			return []pickerButton{{"Start selected", "start-selected"}, {"Stop selected", "stop-selected"}, {"Edit", "edit-tunnel"}, {"Settings", "settings"}, sourceButton, {"Help", "help"}}
		}
		primary = "Start"
		if len(m.visible) > 0 {
			info := m.tunnelStatus[m.tunnels[m.visible[m.cursor]].Name]
			if info.status == 1 || info.status == 2 {
				primary = "Stop"
			}
		}
		return []pickerButton{{primary, "connect"}, {"Debug", "debug"}, {"Edit", "edit-tunnel"}, {"Settings", "settings"}, sourceButton, {"Help", "help"}}
	}
	buttons := []pickerButton{{primary, "connect"}, {"Debug", "debug"}, {"Connect as", "connect-as"}}
	buttons = append(buttons, pickerButton{"Edit", "edit-tunnel"})
	dest := m.dest
	if m.state.Mode == "rdp" {
		dest = rdpDestination(m.config, false)
	}
	buttons = append(buttons, pickerButton{"Launch: " + strings.ReplaceAll(dest, "-", " "), "target"}, pickerButton{"Settings", "settings"}, sourceButton)
	if m.state.ListPercent != 0 || len(m.state.ColumnWidths) > 0 {
		buttons = append(buttons, pickerButton{"Reset layout", "reset-layout"})
	}
	return append(buttons, pickerButton{"Help", "help"})
}

// Render and hit-test from the same dimensions, including wrapped rows/borders.
func (m *pickerModel) controls(buttons []pickerButton, y int) (string, []hitRegion, int) {
	width := max(1, m.width-2)
	rounded := len(buttons) > 0 && strings.HasPrefix(buttons[0].action, "mode:")
	height := 1
	if rounded {
		height = 3
		if m.height < 16 {
			height = 2
		}
	} else if m.height >= 16 {
		height = 3
	}
	var lines []string
	var hits []hitRegion
	var group []pickerButton
	used := 1
	flush := func() {
		if len(group) == 0 {
			return
		}
		tabs := strings.HasPrefix(group[0].action, "mode:")
		var top, middle, bottom strings.Builder
		x := 0
		border := lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
		for i, button := range group {
			active := button.action == "mode:"+m.state.Mode || button.action == "mode:transfers" && isTransferMode(m.state.Mode)
			style := lipgloss.NewStyle().Foreground(lipgloss.Color("250"))
			edge := border
			if active {
				style = style.Foreground(lipgloss.Color("75"))
			}
			if button.action == "help" {
				style = style.Foreground(lipgloss.Color("245"))
			}
			label := " " + button.label + " "
			w := lipgloss.Width(label)
			if rounded {
				leftTop := "┬"
				if i == 0 {
					leftTop = "╭"
				}
				top.WriteString(edge.Render(leftTop + strings.Repeat("─", w)))
				leftBottom := "┴"
				if i == 0 {
					leftBottom = "└"
				}
				if tabs && i > 0 && (group[i-1].action == "mode:"+m.state.Mode || group[i-1].action == "mode:transfers" && isTransferMode(m.state.Mode)) {
					leftBottom = "└"
				}
				middle.WriteString(edge.Render("│") + style.Render(label))
				if tabs && active {
					join := "┘"
					bottom.WriteString(edge.Render(join) + style.Render(strings.Repeat(" ", w)))
				} else {
					bottom.WriteString(edge.Render(leftBottom + strings.Repeat("─", w)))
				}
			} else {
				leftTop, leftBottom := "┬", "┴"
				if i == 0 {
					leftTop, leftBottom = "╭", "╰"
				}
				top.WriteString(edge.Render(leftTop + strings.Repeat("─", w)))
				bottom.WriteString(edge.Render(leftBottom + strings.Repeat("─", w)))
				if i == 0 {
					middle.WriteString(border.Render("│"))
				} else {
					middle.WriteString(border.Render("│"))
				}
				middle.WriteString(style.Render(label))
			}
			for row := 0; row < height; row++ {
				hits = append(hits, hitRegion{x, y + len(lines) + row, w + 1, button.action, 0})
			}
			x += w + 1
		}
		last := group[len(group)-1]
		edge := border
		if rounded {
			top.WriteString(edge.Render("╮"))
			middle.WriteString(edge.Render("│"))
			if tabs && (last.action == "mode:"+m.state.Mode || last.action == "mode:transfers" && isTransferMode(m.state.Mode)) {
				bottom.WriteString(edge.Render("└"))
			} else {
				bottom.WriteString(edge.Render("┴"))
			}
			if tabs {
				bottom.WriteString(border.Render(strings.Repeat("─", max(0, width-x-1))))
			}
			if height == 3 {
				lines = append(lines, top.String())
			}
			lines = append(lines, middle.String(), bottom.String())
		} else {
			middle.WriteString(border.Render("│"))
			if height == 3 {
				top.WriteString(border.Render("╮"))
				bottom.WriteString(border.Render("╯"))
				lines = append(lines, top.String(), middle.String(), bottom.String())
			} else {
				lines = append(lines, middle.String())
			}
		}
		// Include the final shared edge in the last segment's click target.
		for i := len(hits) - height; i < len(hits); i++ {
			hits[i].width++
		}
		group = nil
		used = 1
	}
	for _, button := range buttons {
		if m.width < 55 {
			switch button.label {
			case "Tunnels":
				button.label = "Tun"
			case "Legacy SCP":
				button.label = "Old SCP"
			case "Review transfer":
				button.label = "Review"
			case "Connect as":
				button.label = "As"
			case "Settings":
				button.label = "Setup"
			}
		}
		w := lipgloss.Width(button.label) + 3
		if len(group) > 0 && used+w > width {
			flush()
		}
		group = append(group, button)
		used += w
	}
	flush()
	if !rounded {
		for _, line := range buttonLegend(m.config, buttons, width) {
			lines = append(lines, tuiDim.Render(line))
		}
	}
	return strings.Join(lines, "\n"), hits, len(lines)
}

func buttonLegend(c Config, buttons []pickerButton, width int) []string {
	var lines []string
	line := ""
	for _, button := range buttons {
		key := buttonPrimaryHint(c, button.action)
		if key == "" {
			continue
		}
		if key == "enter" {
			key = "↵"
		}
		label := button.label
		if button.action == "target" {
			label = "Launch location"
		}
		entry := key + " " + label
		if line != "" && lipgloss.Width(line+" │ "+entry) > width {
			lines = append(lines, line)
			line = ""
		}
		if line != "" {
			line += " │ "
		}
		line += entry
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}

func buttonPrimaryHint(c Config, action string) string {
	id := map[string]string{"connect": "launch", "debug": "debug", "connect-as": "profile", "edit-tunnel": "edit", "settings": "settings", "sources": "sources", "help": "help", "target": "target", "account": "account", "reset-layout": "reset_layout", "back": "back", "new-tunnel": "new"}[action]
	if id == "" {
		return ""
	}
	for _, spec := range hotkeyActions {
		if spec.id == id {
			keys := spec.keys
			if override, ok := c.Hotkeys[id]; ok {
				keys = override
			}
			if len(keys) > 0 {
				return keys[0]
			}
		}
	}
	return ""
}

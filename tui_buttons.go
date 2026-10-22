package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type pickerButton struct{ label, action string }

func compactControl(labels ...string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("250")).Render("│ " + strings.Join(labels, " │ ") + " │")
}

var pickerTabs = []pickerButton{{"SSH", "mode:ssh"}, {"RDP", "mode:rdp"}, {"Tunnels", "mode:tunnels"}, {"SCP", "mode:scp"}, {"Legacy SCP", "mode:legacy-scp"}, {"Rsync", "mode:rsync"}}

func (m *pickerModel) buttons() []pickerButton {
	primary := "Connect"
	if m.state.Mode == "scp" || m.state.Mode == "legacy-scp" || m.state.Mode == "rsync" {
		primary = "Review transfer"
	}
	if m.height < 16 {
		return []pickerButton{{"Go", "connect"}, {"Settings", "settings"}, {"Help", "help"}}
	}
	if m.state.Mode == "tunnels" {
		primary = "Start"
		if len(m.visible) > 0 {
			info := m.tunnelStatus[m.tunnels[m.visible[m.cursor]].Name]
			if info.status == 1 || info.status == 2 {
				primary = "Stop"
			}
		}
		return []pickerButton{{primary, "connect"}, {"Settings", "settings"}, {"Sources", "sources"}, {"Help", "help"}}
	}
	buttons := []pickerButton{{primary, "connect"}, {"Connect as", "connect-as"}}
	if m.state.Mode == "rdp" {
		buttons = append(buttons, pickerButton{"Account", "account"})
	}
	buttons = append(buttons, pickerButton{"Target", "target"}, pickerButton{"Settings", "settings"}, pickerButton{"Sources", "sources"})
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
		height = 2
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
		var middle, bottom strings.Builder
		x := 0
		border := lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
		for i, button := range group {
			active := button.action == "mode:"+m.state.Mode
			style := lipgloss.NewStyle().Foreground(lipgloss.Color("250"))
			edge := border
			if active {
				style = style.Foreground(lipgloss.Color("75")).Bold(true)
			}
			if button.action == "help" {
				style = style.Foreground(lipgloss.Color("245"))
			}
			label := " " + button.label + " "
			w := lipgloss.Width(label)
			if rounded {
				leftBottom := "┴"
				if i == 0 {
					leftBottom = "└"
				}
				if tabs && i > 0 && group[i-1].action == "mode:"+m.state.Mode {
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
			middle.WriteString(edge.Render("│"))
			if tabs && last.action == "mode:"+m.state.Mode {
				bottom.WriteString(edge.Render("└"))
			} else {
				bottom.WriteString(edge.Render("┴"))
			}
			if tabs {
				bottom.WriteString(border.Render(strings.Repeat("─", max(0, width-x-1))))
			}
			lines = append(lines, middle.String(), bottom.String())
		} else {
			middle.WriteString(border.Render("│"))
			lines = append(lines, middle.String())
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
	return strings.Join(lines, "\n"), hits, len(lines)
}

package main

import (
	"encoding/base64"
	"encoding/json"
	"github.com/charmbracelet/lipgloss"
	"strconv"
	"strings"
)

func (m *pickerModel) tunnelColumnWidths(width int) []int {
	count := 5
	if width < 100 {
		count = 4
	}
	if width < 70 {
		count = 2
	}
	if width < 48 {
		count = 1
	}
	budget := max(count*4, width-11-2*(count-1))
	widths := make([]int, count)
	for i := range widths {
		widths[i] = budget / count
	}
	widths[0] += budget % count
	if count == 5 && budget >= 75 {
		name := min(28, budget-47)
		widths = []int{name, 15, 11, 10, budget - name - 36}
	} else if count == 4 && budget >= 55 {
		name := min(28, budget-36)
		widths = []int{name, 15, 11, budget - name - 26}
	} else if count == 2 {
		widths = []int{max(4, budget-15), 15}
	}
	if len(m.state.TunnelColumnWidths) >= count {
		remaining := budget
		for i := 0; i < count-1; i++ {
			widths[i] = min(max(4, m.state.TunnelColumnWidths[i]), max(4, remaining-4*(count-i-1)))
			remaining -= widths[i]
		}
		widths[count-1] = max(4, remaining)
	}
	return widths
}

func (m *pickerModel) tunnelColumnValues(index int) []string {
	values := []string{"NAME", "SOURCE", "LOOPBACK", "REMOTE PORT", "LOCAL PORT", "STATUS"}
	if index >= 0 {
		c := m.tunnels[index]
		name, source, bind := c.Name, "", "127.0.0.1"
		if h, ok := m.tunnelHosts[c.Name]; ok {
			name, source = h.Alias, h.SourceName
		}
		data, _ := base64.RawURLEncoding.DecodeString(c.Resource)
		var spec inventoryTunnelSpec
		if json.Unmarshal(data, &spec) == nil {
			for i, arg := range spec.Argv {
				if arg == "-L" && i+1 < len(spec.Argv) {
					parts := strings.Split(spec.Argv[i+1], ":")
					if len(parts) > 0 {
						bind = parts[0]
					}
				}
				if arg == "--address" && i+1 < len(spec.Argv) {
					bind = spec.Argv[i+1]
				}
			}
		}
		values = []string{name, source, bind, strconv.Itoa(c.RemotePort), strconv.Itoa(c.LocalPort), m.tunnelLabel(c.Name)}
	}
	return values
}

func (m *pickerModel) tunnelColumns(index, width int, widths []int) string {
	values := m.tunnelColumnValues(index)
	visible := make([]string, len(widths))
	for i := range visible {
		visible[i] = values[m.sortColumn(i, len(widths))]
	}
	values = visible
	if index < 0 {
		m.sortHeading(values, widths)
	}
	var cells []string
	for i, w := range widths {
		v := fit(clean(values[i]), w)
		cells = append(cells, v+strings.Repeat(" ", max(0, w-lipgloss.Width(v))))
	}
	separator := "  "
	if index < 0 {
		separator = " │"
	}
	prefix := "           "
	if index >= 0 {
		mark := "[ ]"
		if m.marked[m.tunnels[index].Name] {
			mark = "[✓]"
		}
		prefix = " " + mark + " " + m.tunnelStatusMarker(m.tunnels[index].Name) + "     "
	}
	return fit(prefix+strings.Join(cells, separator), width)
}

func (m *pickerModel) tunnelStatusMarker(name string) string {
	if m.tunnelErrors[name] != nil {
		return tuiError.Render("!")
	}
	if m.pending[name] || m.tunnelStatus[name].status == 1 {
		return tuiAccent.Render("◌")
	}
	switch m.tunnelStatus[name].status {
	case 2:
		return lipgloss.NewStyle().Reverse(false).Foreground(lipgloss.Color("#22c55e")).Render("●")
	case 3:
		return tuiError.Render("!")
	}
	return tuiDim.Render("○")
}

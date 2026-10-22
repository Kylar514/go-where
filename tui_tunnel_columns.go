package main

import (
	"encoding/base64"
	"encoding/json"
	"github.com/charmbracelet/lipgloss"
	"strconv"
	"strings"
)

func (m *pickerModel) tunnelColumnWidths(width int) []int {
	count := 6
	if width < 82 {
		count = 4
	}
	if width < 48 {
		count = 2
	}
	budget := max(count*4, width-7-2*(count-1))
	widths := make([]int, count)
	for i := range widths {
		widths[i] = budget / count
	}
	widths[0] += budget % count
	if count == 6 && budget >= 70 {
		widths = []int{18, 12, 15, 11, 10, budget - 66}
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

func (m *pickerModel) tunnelColumns(index, width int, widths []int) string {
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
	if len(widths) == 4 {
		values = []string{values[0], values[3], values[4], values[5]}
	} else if len(widths) == 2 {
		values = []string{values[0], values[5]}
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
	prefix := "       "
	if index >= 0 {
		mark := "[ ]"
		if m.marked[m.tunnels[index].Name] {
			mark = "[✓]"
		}
		prefix = " " + mark + "   "
	}
	return fit(prefix+strings.Join(cells, separator), width)
}
